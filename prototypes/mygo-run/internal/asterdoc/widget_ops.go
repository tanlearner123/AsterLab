package asterdoc

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
)

// TimerRuntimeState passes live timer execution status from RunWorkspace into asterdoc.View.
type TimerRuntimeState struct {
	TimerID     string
	Status      string // "idle" | "running" | "paused" | "completed"
	PlannedS    int
	RemainingS  int
	AccumulatedS int
	PhaseIndex  int
	RoundIndex  int
}

// AssetThumb describes a linked experimental file/image for attachment@1 in Run mode.
type AssetThumb struct {
	AssetID   string
	Label     string
	FilePath  string
	WellID    string
	FovIndex  int
}

func (v *View) renderChecklist(c *ui.Context, data map[string]any) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "操作检查清单")
	items := mapSlice(data["items"])
	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)
	interactiveRun := v.Mode == ModeRun && !v.ReadOnly

	if v.UnifiedLive {
		interactiveLive := !v.ReadOnly && (v.Mode == ModeDesign || v.Mode == ModeProtocol || v.Mode == ModeRun)
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
			if title != "" && title != "操作检查清单" {
				ui.Text(c, title).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
			}
			for idx, it := range items {
				idx, it := idx, it
				cid := firstNonEmpty(textValue(it, "id"), fmt.Sprintf("chk-%d", idx+1))
				label := textValue(it, "label")
				required, _ := it["required"].(bool)
				blocking, _ := it["blocking"].(bool)
				req := required || blocking
				doneInline, _ := it["done"].(bool)
				checkedVal := doneInline || v.CheckedItems[cid] || v.CheckedItems[v.StepID+":"+cid]

				chkPtr := v.ensureWidgetBool("chk:" + moduleID + ":" + cid)
				*chkPtr = checkedVal

				row := ui.Row(c.Key(fmt.Sprintf("live-chk:%s:%d:%s", moduleID, idx, cid))).FillWidth().
					Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
					Radius(float32(tok.ControlRadius)).
					AlignItems(ui.Center).Gap(float32(tok.Spacing8)).
					Transition(theme.AccordionTransition())
				rowHover := row.Hovered()
				targetRowBg := ui.Transparent
				if rowHover {
					targetRowBg = tok.Elevated
				}
				row.Background(row.AnimateColor("chk-rbg", targetRowBg, theme.MotionMicroFade))
				targetTxtCol := tok.Text
				if *chkPtr {
					targetTxtCol = tok.TextMuted
				}
				animTxtCol := row.AnimateColor("chk-fg", targetTxtCol, theme.MotionMicroFade)
				row.Children(func() {
					if interactiveLive {
						if components.Checkbox(c, chkPtr, "").Changed() {
							next := cloneRowsSlice(items)
							if idx < len(next) {
								next[idx]["done"] = *chkPtr
								v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
							}
							if v.CheckedItems == nil {
								v.CheckedItems = map[string]bool{}
							}
							v.CheckedItems[cid] = *chkPtr
							if v.OnToggleCheck != nil {
								v.OnToggleCheck(v.StepID, cid, *chkPtr)
							}
						}
					} else {
						mark := "☐"
						if *chkPtr {
							mark = "☑"
						}
						ui.Text(c, mark).FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
					}

					if editableDesign {
						lblDraft := v.ensureWidgetDraft("chk_lbl:"+moduleID+":"+cid, label)
						ui.Box(c).Grow(1).MinWidth(0).Children(func() {
							ti := BorderlessTextInput(c, tok, lblDraft, float32(tok.TextBody), false, "输入检查项内容，按 Enter 新增下一项…").
								TextColor(animTxtCol)
							ti.OnTextKey(func(k ui.TextKey) bool {
								if k.Key == ui.KeyBackspace && strings.TrimSpace(k.Text()) == "" && len(items) > 1 {
									next := removeSliceIndex(items, idx)
									v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
									return true
								}
								return false
							})
							if ti.Submitted() {
								next := cloneRowsSlice(items)
								if idx < len(next) {
									next[idx]["label"] = strings.TrimSpace(*lblDraft)
								}
								newItem := map[string]any{
									"id":    fmt.Sprintf("chk-%d", len(next)+1),
									"label": "",
									"done":  false,
								}
								if idx+1 <= len(next) {
									tail := append([]map[string]any{newItem}, next[idx+1:]...)
									next = append(next[:idx+1], tail...)
								} else {
									next = append(next, newItem)
								}
								v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
							} else if !ti.Focused() && strings.TrimSpace(*lblDraft) != strings.TrimSpace(label) {
								next := cloneRowsSlice(items)
								if idx < len(next) {
									next[idx]["label"] = strings.TrimSpace(*lblDraft)
									v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
								}
							}
						})
					} else {
						txtColor := tok.Text
						if *chkPtr {
							txtColor = tok.TextMuted
						}
						ui.Box(c).FillWidth().Children(func() {
							ui.Text(c, label).FontSize(float32(tok.TextBody)).TextColor(txtColor)
						})
					}

					if req {
						components.Badge(c, "必检", components.BadgeOptions{Tone: components.BadgeWarning})
					}
					if editableDesign {
						actOp := float32(0)
						if rowHover || v.MobileCompact {
							actOp = 1
						}
						actWrap := ui.Row(c.Key("chk-acts:"+moduleID+":"+cid)).Shrink(0).AlignItems(ui.Center).Gap(float32(tok.Spacing4))
						actWrap.Opacity(actWrap.AnimateTo("caop", actOp, theme.MotionMicroFade)).Children(func() {
							reqTip := map[bool]string{true: "取消必检", false: "设为必检"}[req]
							if components.IconButton(c, lucide.ShieldCheck(), reqTip, components.IconButtonCompact).Clicked() {
								next := cloneRowsSlice(items)
								if idx < len(next) {
									next[idx]["required"] = !req
									next[idx]["blocking"] = !req
									v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
								}
							}
							if len(items) > 1 {
								if components.IconButton(c, lucide.X(), "删除检查项", components.IconButtonCompact).Clicked() {
									next := removeSliceIndex(items, idx)
									v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
								}
							}
						})
					}
				})
			}
			if editableDesign {
				if components.Button(c, "+ 添加检查项", components.ButtonGhost).Clicked() {
					next := cloneRowsSlice(items)
					next = append(next, map[string]any{
						"id":    fmt.Sprintf("chk-%d", len(next)+1),
						"label": "新检查项",
						"done":  false,
					})
					v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
				}
			}
		})
		return
	}

	doneCount := 0
	for _, it := range items {
		cid := textValue(it, "id")
		if v.CheckedItems[cid] || v.CheckedItems[v.StepID+":"+cid] || v.SkippedItems[cid] != "" {
			doneCount++
		}
	}

	moduleCardFrame(v, c, lucide.SquareCheck(), title, fmt.Sprintf("%d / %d", doneCount, len(items)), nil, func() {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			for idx, it := range items {
				idx := idx
				cid := textValue(it, "id")
				label := textValue(it, "label")
				required, _ := it["required"].(bool)
				allowSkip, _ := it["allow_rule_skip"].(bool)
				skipRule := textValue(it, "skip_rule")
				skipReason := v.SkippedItems[cid]

				key := cid
				if v.CheckedItems[v.StepID+":"+cid] {
					key = v.StepID + ":" + cid
				}
				checkedVal := v.CheckedItems[key]
				chkPtr := v.ensureWidgetBool("chk:" + moduleID + ":" + cid)
				if v.CheckedItems != nil {
					*chkPtr = checkedVal
				}

				ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing8)).
					Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
					AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if interactiveRun && skipReason == "" {
						if components.Checkbox(c, chkPtr, label).Changed() {
							if v.CheckedItems == nil {
								v.CheckedItems = map[string]bool{}
							}
							v.CheckedItems[cid] = *chkPtr
							if v.OnToggleCheck != nil {
								v.OnToggleCheck(v.StepID, cid, *chkPtr)
							}
						}
					} else {
						mark := "○ "
						if *chkPtr {
							mark = "✓ "
						}
						ui.Text(c, mark+label).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
					}
					ui.Spacer(c)
					if required {
						components.Badge(c, "必检", components.BadgeOptions{Tone: components.BadgeWarning})
					}
					if skipReason != "" {
						components.Badge(c, "已豁免 · "+skipReason, components.BadgeOptions{Tone: components.BadgeAccent})
					} else if allowSkip && interactiveRun && !*chkPtr {
						btnLabel := "按规则跳过"
						if skipRule != "" {
							btnLabel = "按规则跳过（" + skipRule + "）"
						}
						if components.Button(c, btnLabel, components.ButtonOutline).Clicked() && v.OnRuleSkip != nil {
							v.OnRuleSkip(v.StepID, cid, firstNonEmpty(skipRule, "rule_skip"))
						}
					} else if allowSkip && !interactiveRun {
						components.Badge(c, firstNonEmpty(skipRule, "可规则豁免"))
					}
					if editableDesign && len(items) > 1 {
						if components.IconButton(c, lucide.Trash(), "删除检查项", components.IconButtonCompact).Clicked() {
							next := removeSliceIndex(items, idx)
							v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
						}
					}
				})
			}

			if editableDesign {
				newLabel := v.ensureWidgetDraft("chk:"+moduleID+":new", "")
				reqBool := v.ensureWidgetBool("chk:" + moduleID + ":req")
				ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
					ui.Box(c).FillWidth().Children(func() {
						components.TextBox(c, newLabel, components.TextBoxOptions{}).Placeholder("新增检查项描述…")
					})
					components.Checkbox(c, reqBool, "必检")
					if components.Button(c, "添加项", components.ButtonSecondary).Clicked() && strings.TrimSpace(*newLabel) != "" {
						next := append([]map[string]any{}, items...)
						next = append(next, map[string]any{
							"id":       fmt.Sprintf("chk-%s-%d", sanitizeIDFrag(*newLabel), len(items)+1),
							"label":    strings.TrimSpace(*newLabel),
							"required": *reqBool,
						})
						v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
						*newLabel = ""
					}
				})
			}
		})
	})
}

func cloneRowsSlice(in []map[string]any) []map[string]any {
	out := make([]map[string]any, len(in))
	for i, m := range in {
		cp := make(map[string]any, len(m))
		for k, val := range m {
			cp[k] = val
		}
		out[i] = cp
	}
	return out
}

func (v *View) renderCalculator(c *ui.Context, data map[string]any) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "科学计算器")
	mode := textValue(data, "mode")
	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)

	if v.UnifiedLive {
		v.renderUnifiedCalculator(c, tok, moduleID, title, mode, data, editableDesign)
		return
	}

	// Apply interactive scratch overrides cleanly without mutating frozen source
	evalData := make(map[string]any, len(data))
	for k, val := range data {
		evalData[k] = val
	}
	switch mode {
	case "dilution":
		v.applyQuantityScratch(moduleID, evalData, "stock")
		v.applyQuantityScratch(moduleID, evalData, "target")
		v.applyQuantityScratch(moduleID, evalData, "final_volume")
	case "serial_dilution":
		if fStr, ok := v.calcScratchFloat(moduleID, "dilution_factor"); ok && fStr > 1 {
			evalData["dilution_factor"] = fStr
		}
		if sStr, ok := v.calcScratchFloat(moduleID, "steps_count"); ok && sStr >= 1 {
			evalData["steps_count"] = int(sStr)
		}
	case "group_mastermix":
		v.applyQuantityScratch(moduleID, evalData, "volume_per_unit")
		if ex, ok := v.calcScratchFloat(moduleID, "excess_ratio"); ok && ex >= 0 {
			evalData["excess_ratio"] = ex
		}
	case "sample_normalize":
		v.applyQuantityScratch(moduleID, evalData, "target_mass")
		v.applyQuantityScratch(moduleID, evalData, "volume_per_unit")
	case "formula":
		if s, ok := v.calcScratchString(moduleID, "formula"); ok {
			evalData["formula"] = s
		}
		if s, ok := v.calcScratchString(moduleID, "unit"); ok {
			evalData["unit"] = s
		}
	}

	res := EvalCalculatorWithActuals(v.Doc, evalData, v.CalcActuals)
	moduleCardFrame(v, c, lucide.Calculator(), title, calcModeLabel(mode), nil, func() {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			if editableDesign {
				modeDraft := v.ensureWidgetDraft("calc_mode:"+moduleID, mode)
				if *modeDraft != mode && mode != "" {
					*modeDraft = mode
				}
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, "计算模式").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					mSel := components.Select(c, modeDraft)
					mSel.Trigger.Children(func() {
						ui.Text(c, calcModeLabel(*modeDraft)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
					})
					mSel.Popup(func(panel ui.Element) {
						for _, opt := range []string{"dilution", "serial_dilution", "group_mastermix", "sample_normalize", "formula"} {
							opt := opt
							mSel.Item(opt).Children(func() {
								ui.Text(c, calcModeLabel(opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						}
					})
					if mSel.Trigger.Changed() && *modeDraft != "" && *modeDraft != mode {
						v.emitAnyModuleEdit(moduleID, map[string]any{"mode": *modeDraft})
					}
					ui.Spacer(c)
					if components.Button(c, "保存计算设定", components.ButtonSecondary).Clicked() {
						props := map[string]any{"mode": *modeDraft}
						switch *modeDraft {
						case "dilution":
							props["stock"] = evalData["stock"]
							props["target"] = evalData["target"]
							props["final_volume"] = evalData["final_volume"]
						case "serial_dilution":
							props["dilution_factor"] = evalData["dilution_factor"]
							props["steps_count"] = evalData["steps_count"]
						case "group_mastermix":
							props["volume_per_unit"] = evalData["volume_per_unit"]
							props["excess_ratio"] = evalData["excess_ratio"]
						case "sample_normalize":
							props["target_mass"] = evalData["target_mass"]
							props["volume_per_unit"] = evalData["volume_per_unit"]
						case "formula":
							if s := textValue(evalData, "formula"); s != "" {
								props["formula"] = s
							}
							if u := textValue(evalData, "unit"); u != "" {
								props["unit"] = u
							}
						}
						v.emitAnyModuleEdit(moduleID, props)
					}
				})
			}

			// Interactive scratchpad inputs row
			ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing8)).Children(func() {
				switch mode {
				case "dilution":
					v.renderCalcQtyInput(c, moduleID, evalData, "stock", "母液")
					v.renderCalcQtyInput(c, moduleID, evalData, "target", "目标")
					v.renderCalcQtyInput(c, moduleID, evalData, "final_volume", "终体积")
				case "serial_dilution":
					v.renderCalcNumInput(c, moduleID, "dilution_factor", "稀释倍数", evalData["dilution_factor"])
					v.renderCalcNumInput(c, moduleID, "steps_count", "级数", evalData["steps_count"])
				case "group_mastermix":
					v.renderCalcQtyInput(c, moduleID, evalData, "volume_per_unit", "单孔/单份体积")
					v.renderCalcNumInput(c, moduleID, "excess_ratio", "余量比例", evalData["excess_ratio"])
				case "sample_normalize":
					v.renderCalcQtyInput(c, moduleID, evalData, "target_mass", "目标上样量")
					v.renderCalcQtyInput(c, moduleID, evalData, "volume_per_unit", "终体积")
				case "formula":
					v.renderCalcTextInput(c, moduleID, "formula", "计算公式", textValue(evalData, "formula"), float32(tok.Spacing48)*4.5)
					v.renderCalcTextInput(c, moduleID, "unit", "结果单位", textValue(evalData, "unit"), float32(tok.Spacing48)*2.0)
				}
			})

			if len(res.UsedActuals) > 0 {
				ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing4)).Children(func() {
					components.Badge(c, "已按实测值换算 · "+strings.Join(res.UsedActuals, ", "), components.BadgeOptions{Tone: components.BadgeSuccess})
				})
			}

			if res.Err != nil {
				v.moduleError(c, res.Err)
				return
			}
			if res.Summary != "" {
				ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
					Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Children(func() {
					ui.Text(c, res.Summary).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Accent).FontFeatures("tnum")
				})
			}
			if len(res.Rows) > 0 {
				calcCols := []ui.TableColumn{
					{ID: "item", Title: "分项", MinWidth: 140},
					{ID: "amount", Title: "计算值", Width: float32(tok.Spacing48) * 2.5, Align: ui.End},
					{ID: "note", Title: "计算说明", MinWidth: 160},
				}
				components.Table(c, nil, calcCols, func() {
					for rIdx, row := range res.Rows {
						rIdx, row := rIdx, row
						amtStr := formatNumber(row.Amount)
						if row.Unit != "" {
							amtStr += " " + row.Unit
						}
						components.TableRow(c, rIdx, row.Label, false, func(col int) {
							switch col {
							case 0:
								ui.Text(c, row.Label).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
							case 1:
								ui.Text(c, amtStr).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).FontFeatures("tnum")
							case 2:
								ui.Text(c, firstNonEmpty(row.Note, "—")).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
							}
						})
					}
				})
			}
		})
	})
}

func sanitizeCalcVarKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range raw {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_':
			b.WriteRune(r)
		case r == '-' || r == ' ':
			b.WriteByte('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out != "" && out[0] >= '0' && out[0] <= '9' {
		out = "v_" + out
	}
	return out
}

func renameFormulaIdentifier(expr, oldKey, newKey string) string {
	if oldKey == "" || newKey == "" || oldKey == newKey || expr == "" {
		return expr
	}
	re, err := regexp.Compile(`\b` + regexp.QuoteMeta(oldKey) + `\b`)
	if err != nil {
		return expr
	}
	return re.ReplaceAllString(expr, newKey)
}

func friendlyCalcError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if idx := strings.Index(msg, `missing variable "`); idx >= 0 {
		rest := msg[idx+len(`missing variable "`):]
		if end := strings.IndexByte(rest, '"'); end > 0 {
			return fmt.Sprintf("公式引用了未定义代号「%s」，请检查上方「代号」列", rest[:end])
		}
	}
	return "公式计算待修正：" + msg
}

// syncDraftInput coordinates a borderless text input draft buffer with external module state.
// Unlike naive `!focused` syncs that wipe edits on blur, it tracks previous focus state so
// clicking away (blur) or pressing Enter commits user edits before external state syncs back.
func (v *View) syncDraftInput(draftKey, orig string, tb ui.Element, buf *string) (string, bool) {
	focPtr := v.ensureWidgetBool("foc:" + draftKey)
	wasFocused := *focPtr
	focusedNow := tb.Focused()
	*focPtr = focusedNow

	if tb.Submitted() || (wasFocused && !focusedNow) {
		val := strings.TrimSpace(*buf)
		if val != strings.TrimSpace(orig) {
			return val, true
		}
		return "", false
	}
	if !focusedNow && *buf != orig {
		*buf = orig
	}
	return "", false
}

func (v *View) renderUnifiedCalculator(c *ui.Context, tok theme.Tokens, moduleID, title, mode string, data map[string]any, editableDesign bool) {
	rows := mapSlice(data["rows"])
	res := EvalCalculatorWithActuals(v.Doc, data, v.CalcActuals)

	// Separate input variable rows from formula output rows cleanly
	var inputIdxs []int
	var formulaIdxs []int
	definedNormVars := map[string]bool{}
	for idx, rm := range rows {
		if strings.TrimSpace(textValue(rm, "formula")) != "" {
			formulaIdxs = append(formulaIdxs, idx)
		} else {
			inputIdxs = append(inputIdxs, idx)
			if k := textValue(rm, "key"); k != "" {
				definedNormVars[normalizeVarLookup(k)] = true
			}
		}
	}

	customOpen := v.ensureWidgetBool("calc_custom_open:" + moduleID)
	activeFormulaIdxStr := v.ensureWidgetDraft("calc_active_fidx:"+moduleID, "0")
	activeFPos, _ := strconv.Atoi(*activeFormulaIdxStr)
	if activeFPos < 0 || activeFPos >= len(formulaIdxs) {
		activeFPos = 0
	}

	// Detect which built-in preset matches the current title for active tab highlight
	detectPreset := func() string {
		tl := strings.ToLower(title)
		switch {
		case strings.Contains(tl, "连续") || strings.Contains(tl, "倍比"):
			return "serial_dilution"
		case strings.Contains(tl, "比例稀释") || strings.Contains(tl, "1:n"):
			return "ratio_dilution"
		case strings.Contains(tl, "稀释"):
			return "dilution"
		case strings.Contains(tl, "% w/v") || strings.Contains(tl, "w/v"):
			return "weighing_wv"
		case strings.Contains(tl, "称量") || strings.Contains(tl, "摩尔"):
			return "weighing"
		case strings.Contains(tl, "预混"):
			return "mastermix"
		case strings.Contains(tl, "细胞") || strings.Contains(tl, "接种") || strings.Contains(tl, "铺板"):
			return "seeding"
		case strings.Contains(tl, "离心") || strings.Contains(tl, "rcf") || strings.Contains(tl, "rpm"):
			return "rcf_rpm"
		case strings.Contains(tl, "自定义") || strings.Contains(tl, "latex"):
			return "custom"
		default:
			return ""
		}
	}
	activePreset := detectPreset()

	var primaryPlate *SciContainer
	var upstreamCalcs []SciCalcOutput
	var docGraph *SciGraph
	if v.Doc != nil {
		if g := BuildSciGraph(v.Doc); g != nil {
			docGraph = g
			primaryPlate = g.PrimaryPlate()
			upstreamCalcs = g.UpstreamCalcOutputs(moduleID)
		}
	}
	hasPlate := primaryPlate != nil

	applyCalcPreset := func(presetID string) {
		*customOpen = presetID == "custom"
		tpl := DefaultCalculatorPreset(v.Doc, v.StepID, presetID)
		v.emitAnyModuleEdit(moduleID, map[string]any{
			"title": tpl["title"],
			"mode":  tpl["mode"],
			"rows":  tpl["rows"],
		})
	}

	renderUnifiedModuleShell(v, c, lucide.Calculator(), moduleID, title, "", editableDesign, func() {
		if res.Summary != "" {
			inlineBadge(c, res.Summary, components.BadgeOptions{Tone: components.BadgeAccent})
		}
	}, func() {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing12)).Children(func() {
			// 1. Clean Mode Switcher Pills + Universal Formula Library Dropdown + Custom LaTeX
			if editableDesign {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
					quickModes := []struct {
						id    string
						label string
					}{
						{"dilution", "稀释 C₁V₁"},
						{"weighing_wv", "% w/v 称量"},
						{"weighing", "摩尔称量"},
						{"mastermix", "孔板预混"},
						{"seeding", "细胞接种"},
					}
					for _, md := range quickModes {
						md := md
						isAct := activePreset == md.id
						pill := ui.ButtonBase(c.Key("calc-mode:"+moduleID+":"+md.id)).
							Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
							AlignItems(ui.Center).AlignSelf(ui.Center).
							Radius(float32(tok.ControlRadius)).Label("切换计算模式：" + md.label)
						fg := tok.TextMuted
						if isAct {
							pill.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
							fg = tok.Accent
						} else {
							pill.Background(tok.Elevated).Border(1, tok.Border.Alpha(0.75))
							if pill.Hovered() {
								pill.Background(tok.Text.Alpha(0.05))
								fg = tok.Text
							}
						}
						pill.Children(func() {
							ui.Text(c, md.label).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg).SingleLine()
						})
						if pill.Clicked() {
							applyCalcPreset(md.id)
						}
					}

					// Universal Formula Library Menu (9 curated wet-lab templates inside calculator@1)
					libIsOther := activePreset == "ratio_dilution" || activePreset == "serial_dilution" || activePreset == "rcf_rpm"
					libBtn := ui.ButtonBase(c.Key("calc-lib:"+moduleID)).
						Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
						AlignItems(ui.Center).AlignSelf(ui.Center).
						Radius(float32(tok.ControlRadius)).Label("打开公式库")
					libFg := tok.TextMuted
					if libIsOther {
						libBtn.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
						libFg = tok.Accent
					} else {
						libBtn.Background(tok.Elevated).Border(1, tok.Border.Alpha(0.75))
						if libBtn.Hovered() {
							libBtn.Background(tok.Text.Alpha(0.05))
							libFg = tok.Text
						}
					}
					libBtn.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							ui.Icon(c, lucide.BookOpen()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(libFg)
							ui.Text(c, "公式库").FontSize(float32(tok.TextMeta)).Bold().TextColor(libFg).SingleLine()
							ui.Icon(c, lucide.ChevronDown()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(libFg)
						})
					})
					components.MenuButton(c, libBtn, func(m *components.Menu) {
						lastCat := ""
						for _, pr := range CalculatorFormulaPresets() {
							if lastCat != "" && pr.Category != lastCat {
								m.Separator()
							}
							lastCat = pr.Category
							itemText := pr.Label + "  (" + pr.ShortFormula + ")"
							if m.Item(itemText).Chosen() {
								applyCalcPreset(pr.ID)
							}
						}
					})

					// Custom LaTeX toggle pill (never permanently locked open!)
					custAct := *customOpen
					custPill := ui.ButtonBase(c.Key("calc-mode:"+moduleID+":custom")).
						Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
						AlignItems(ui.Center).AlignSelf(ui.Center).
						Radius(float32(tok.ControlRadius)).Label("切换自定义 LaTeX 公式编辑")
					custFg := tok.TextMuted
					if custAct {
						custPill.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
						custFg = tok.Accent
					} else {
						custPill.Background(tok.Elevated).Border(1, tok.Border.Alpha(0.75))
						if custPill.Hovered() {
							custPill.Background(tok.Text.Alpha(0.05))
							custFg = tok.Text
						}
					}
					custPill.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							ui.Icon(c, lucide.Sigma()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(custFg)
							ui.Text(c, "自定义 LaTeX").FontSize(float32(tok.TextMeta)).Bold().TextColor(custFg).SingleLine()
						})
					})
					if custPill.Clicked() {
						*customOpen = !*customOpen
						if len(formulaIdxs) == 0 {
							applyCalcPreset("custom")
						}
					}
				})
			}

			// Fallback for legacy non-row modes (dilution / group_mastermix / serial_dilution / sample_normalize)
			if len(rows) == 0 {
				if res.Err != nil {
					ui.Text(c, friendlyCalcError(res.Err)).FontSize(float32(tok.TextMeta)).TextColor(tok.Warning)
				}
				for _, r := range res.Rows {
					r := r
					amt := formatNumber(r.Amount)
					if r.Unit != "" {
						amt += " " + DisplayUnit(r.Unit)
					}
					ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
						Radius(float32(tok.ControlRadius)).Background(tok.Elevated).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Text(c, r.Label).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
						ui.Spacer(c)
						ui.Text(c, amt).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Accent).FontFeatures("tnum")
					})
				}
				return
			}

			// Helper: mutate the active formula expression cleanly
			activeRowIdx := -1
			if len(formulaIdxs) > 0 {
				activeRowIdx = formulaIdxs[activeFPos]
			}
			appendToActiveFormula := func(snippet string) {
				if activeRowIdx < 0 || activeRowIdx >= len(rows) {
					return
				}
				cur := strings.TrimSpace(textValue(rows[activeRowIdx], "formula"))
				nextExpr := cur
				if nextExpr == "" {
					nextExpr = snippet
				} else if strings.HasSuffix(nextExpr, "{}") {
					// Smart insertion into empty braces like \frac{}{} -> \frac{snippet}{}
					pos := strings.LastIndex(nextExpr, "{}")
					nextExpr = nextExpr[:pos+1] + snippet + nextExpr[pos+1:]
				} else if strings.Contains(nextExpr, "{}") {
					pos := strings.Index(nextExpr, "{}")
					nextExpr = nextExpr[:pos+1] + snippet + nextExpr[pos+1:]
				} else {
					nextExpr = nextExpr + " " + snippet
				}
				next := cloneRowsSlice(rows)
				next[activeRowIdx]["formula"] = nextExpr
				fBuf := v.ensureWidgetDraft(fmt.Sprintf("calc_f:%s:%d", moduleID, activeRowIdx), nextExpr)
				*fBuf = nextExpr
				v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
			}

			// 2. VISUAL CENTER HERO FORMULA STAGE (正中 2D LaTeX 数学公式 + 高亮结果!)
			ui.Column(c).FillWidth().
				Padding(float32(tok.Spacing16), float32(tok.Spacing16)).
				Radius(float32(tok.PanelRadius)).
				Background(tok.Elevated).Border(1, tok.Border).
				AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
				if activeRowIdx >= 0 {
					rm := rows[activeRowIdx]
					rkey := firstNonEmpty(textValue(rm, "key"), "y")
					rlbl := firstNonEmpty(textValue(rm, "label"), "计算结果")
					runit := textValue(rm, "unit")
					formulaStr := strings.TrimSpace(textValue(rm, "formula"))

					// Subtle top caption row inside hero stage (output label + quick edit toggle)
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						if len(formulaIdxs) > 1 {
							for fPos, rIdx := range formulaIdxs {
								fPos, rIdx := fPos, rIdx
								fLbl := firstNonEmpty(textValue(rows[rIdx], "label"), fmt.Sprintf("结果 %d", fPos+1))
								isSel := fPos == activeFPos
								tab := ui.ButtonBase(c.Key(fmt.Sprintf("calc-ftab:%s:%d", moduleID, fPos))).
									Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
									AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label("查看公式：" + fLbl)
								tfg := tok.TextMuted
								if isSel {
									tab.Background(tok.Surface).Border(1, tok.Border)
									tfg = tok.Text
								}
								tab.Children(func() {
									ui.Text(c, fLbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(tfg).SingleLine()
								})
								if tab.Clicked() {
									*activeFormulaIdxStr = strconv.Itoa(fPos)
								}
							}
						} else {
							ui.Text(c, rlbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
						}
						ui.Spacer(c)
						if formulaStr != "" {
							copyBtn := ui.ButtonBase(c.Key("calc-copy-f:"+moduleID)).
								Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
								AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label("复制 LaTeX")
							if copyBtn.Hovered() {
								copyBtn.Background(tok.Text.Alpha(0.05))
							}
							copyBtn.Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
									ui.Icon(c, lucide.Copy()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.TextMuted)
									ui.Text(c, "复制 LaTeX").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
								})
							})
							if copyBtn.Clicked() {
								c.Services().WriteClipboard(formulaStr)
							}
						}
						if editableDesign {
							edLbl := "编辑公式"
							if *customOpen {
								edLbl = "收起公式栏"
							}
							toggleBtn := ui.ButtonBase(c.Key("calc-edit-f:"+moduleID)).
								Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
								AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label(edLbl)
							if toggleBtn.Hovered() {
								toggleBtn.Background(tok.Text.Alpha(0.05))
							}
							toggleBtn.Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
									ui.Icon(c, lucide.Pencil()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
									ui.Text(c, edLbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).SingleLine()
								})
							})
							if toggleBtn.Clicked() {
								*customOpen = !*customOpen
							}
						}
					})

					// Centered 2D Math Equation:  LHS = [Clickable 2D LaTeX AST] = [Bold Result]
					dispAmt := "—"
					if activeRowIdx < len(res.Rows) && res.Err == nil {
						dispAmt = formatNumber(res.Rows[activeRowIdx].Amount)
						if runit != "" {
							dispAmt += " " + DisplayUnit(runit)
						}
					}
					ui.Row(c).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Wrap().
						Padding(float32(tok.Spacing8), 0).Gap(float32(tok.Spacing12)).Children(func() {
						// LHS symbol (e.g. V₁)
						lhsNode := makeIdentNode(rkey)
						RenderMathNode(c, tok, lhsNode, float32(tok.TextSection)+2, tok.Text)
						ui.Text(c, "=").FontSize(float32(tok.TextSection)+2).TextColor(tok.TextMuted).SingleLine()
						// 2D Rendered LaTeX Math AST (clickable in Design mode to toggle editor!)
						render2D := func() {
							if ast, parseErr := ParseLatexMath(formulaStr); parseErr == nil {
								RenderMathNode(c, tok, ast, float32(tok.TextSection)+2, tok.Text)
							} else {
								ui.Text(c, formulaStr).Font("monospace").FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
							}
						}
						if editableDesign {
							eqBtn := ui.ButtonBase(c.Key("calc-2d-edit:"+moduleID)).
								Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
								Radius(float32(tok.ControlRadius)).Label("点击编辑 LaTeX 公式")
							if eqBtn.Hovered() {
								eqBtn.Background(tok.AccentSoftBg.Alpha(0.55))
							}
							eqBtn.Children(render2D)
							components.Tooltip(c, eqBtn, "点击展开/收起 LaTeX 公式编辑器")
							if eqBtn.Clicked() {
								*customOpen = !*customOpen
							}
						} else {
							render2D()
						}
						ui.Text(c, "=").FontSize(float32(tok.TextSection)+2).TextColor(tok.TextMuted).SingleLine()
						// Highlighted Computed Result Pill
						ui.Row(c).Padding(float32(tok.Spacing4)*1.2, float32(tok.Spacing12)).
							Radius(float32(tok.ControlRadius)).
							Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35)).
							AlignItems(ui.Center).Children(func() {
							ui.Text(c, dispAmt).FontSize(float32(tok.TextSection)+2).Bold().
								TextColor(tok.Accent).FontFeatures("tnum").SingleLine().
								ContentTransition(theme.NumericRoll(false))
						})
					})

					// Readable Bench Action Summary Strip (especially helpful when multiple outputs exist!)
					if res.Err == nil && len(formulaIdxs) > 0 {
						var recipeParts []string
						for _, fIdx := range formulaIdxs {
							if fIdx < len(res.Rows) {
								fl := firstNonEmpty(textValue(rows[fIdx], "label"), textValue(rows[fIdx], "key"))
								fu := textValue(rows[fIdx], "unit")
								valTxt := formatNumber(res.Rows[fIdx].Amount)
								if fu != "" {
									valTxt += " " + DisplayUnit(fu)
								}
								recipeParts = append(recipeParts, fmt.Sprintf("%s %s", fl, valTxt))
							}
						}
						if len(recipeParts) > 0 {
							ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
								Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border.Alpha(0.65)).
								AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
								ui.Icon(c, lucide.FlaskConical()).Size(float32(tok.Spacing12)+2, float32(tok.Spacing12)+2).TextColor(tok.Accent)
								ui.Text(c, "台面配液操作："+strings.Join(recipeParts, "   +   ")).
									FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).FontFeatures("tnum")
							})
						}
					}
				}
			})

			// 3. WORD-STYLE LATEX FORMULA EDITOR TOOLBAR & LINEAR INPUT (strictly controlled by *customOpen!)
			if editableDesign && activeRowIdx >= 0 && *customOpen {
				rm := rows[activeRowIdx]
				formulaStr := strings.TrimSpace(textValue(rm, "formula"))
				outLbl := firstNonEmpty(textValue(rm, "label"), "计算结果")
				outUnit := textValue(rm, "unit")
				fKey := fmt.Sprintf("calc_f:%s:%d", moduleID, activeRowIdx)
				fBuf := v.ensureWidgetDraft(fKey, formulaStr)

				ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).
					Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border).
					Gap(float32(tok.Spacing8)).Children(func() {
					// Word-style quick LaTeX structure buttons
					ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Text(c, "插入符号").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted).Padding(0, float32(tok.Spacing4), 0, 0)
						snippets := []struct {
							label string
							code  string
							tip   string
						}{
							{"a/b", `\frac{}{}`, "插入分数"},
							{"x²", `^{2}`, "插入平方/上标"},
							{"xᵢ", `_{}`, "插入下标"},
							{"√x", `\sqrt{}`, "插入平方根"},
							{"×", `\times`, "插入乘号"},
							{"÷", `\div`, "插入除号"},
							{"log₁₀", `\log_{10}()`, "插入常用对数"},
							{"10ⁿ", `10^{}`, "插入科学计数幂"},
							{"( )", `()`, "插入括号"},
						}
						for _, sn := range snippets {
							sn := sn
							sBtn := ui.ButtonBase(c.Key("calc-sym:"+moduleID+":"+sn.label)).
								Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
								AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
								Background(tok.Elevated).Border(1, tok.Border).Label(sn.tip)
							if sBtn.Hovered() {
								sBtn.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
							}
							sBtn.Children(func() {
								ui.Text(c, sn.label).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).SingleLine()
							})
							components.Tooltip(c, sBtn, sn.tip+" ("+sn.code+")")
							if sBtn.Clicked() {
								appendToActiveFormula(sn.code)
							}
						}
					})

					// Linear LaTeX Formula Input + Output Name & Unit
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Box(c).Grow(1).MinWidth(0).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.BorderStrong.Alpha(0.65)).Children(func() {
							tb := BorderlessTextInput(c, tok, fBuf, float32(tok.TextData), false, `输入 LaTeX 公式，如 \frac{C_{2} \times V_{2}}{C_{1}}，点击下方变量即可插入`).
								Font("monospace").TextColor(tok.Text)
							if val, ok := v.syncDraftInput(fKey, formulaStr, tb, fBuf); ok && val != "" {
								next := cloneRowsSlice(rows)
								next[activeRowIdx]["formula"] = val
								v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
							}
						})
						// Output label & unit quick inputs
						ui.Box(c).Width(float32(tok.Spacing48)*2.0).Shrink(0).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).Children(func() {
							olKey := fmt.Sprintf("calc_olbl:%s:%d", moduleID, activeRowIdx)
							olBuf := v.ensureWidgetDraft(olKey, outLbl)
							olt := BorderlessTextInput(c, tok, olBuf, float32(tok.TextMeta), false, "结果名称")
							if val, ok := v.syncDraftInput(olKey, outLbl, olt, olBuf); ok {
								next := cloneRowsSlice(rows)
								next[activeRowIdx]["label"] = firstNonEmpty(val, outLbl)
								v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
							}
						})
						ui.Box(c).Width(float32(tok.Spacing48)*1.3).Shrink(0).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).Children(func() {
							ouKey := fmt.Sprintf("calc_ou:%s:%d", moduleID, activeRowIdx)
							ouBuf := v.ensureWidgetDraft(ouKey, outUnit)
							out := BorderlessTextInput(c, tok, ouBuf, float32(tok.TextMeta), false, "结果单位")
							if val, ok := v.syncDraftInput(ouKey, outUnit, out, ouBuf); ok {
								next := cloneRowsSlice(rows)
								next[activeRowIdx]["unit"] = val
								v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
							}
						})
					})

					// Inline LaTeX command / variable autocomplete suggestion strip when user types `\`
					curDraft := *fBuf
					if bs := strings.LastIndexByte(curDraft, '\\'); bs >= 0 && !strings.ContainsAny(curDraft[bs:], " {}()") {
						prefix := strings.ToLower(curDraft[bs:])
						cmds := []struct {
							cmd  string
							ins  string
							desc string
						}{
							{`\frac`, `\frac{}{}`, "分数"},
							{`\sqrt`, `\sqrt{}`, "根号"},
							{`\times`, `\times`, "乘号 ×"},
							{`\cdot`, `\cdot`, "点乘 ·"},
							{`\div`, `\div`, "除号 ÷"},
							{`\log`, `\log_{10}()`, "对数 log₁₀"},
							{`\ln`, `\ln()`, "自然对数 ln"},
							{`\mu`, `\mu`, "希腊字母 µ"},
							{`\Delta`, `\Delta`, "增量 Δ"},
							{`\pi`, `\pi`, "圆周率 π"},
						}
						var matched []struct{ cmd, ins, desc string }
						for _, cm := range cmds {
							if strings.HasPrefix(strings.ToLower(cm.cmd), prefix) {
								matched = append(matched, cm)
							}
						}
						if len(matched) > 0 {
							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
								ui.Text(c, "联想：").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								for _, m := range matched {
									m := m
									cBtn := ui.ButtonBase(c.Key("calc-ac:"+moduleID+":"+m.cmd)).
										Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
										AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
										Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.3)).Label("补全 " + m.cmd)
									cBtn.Children(func() {
										ui.Text(c, m.cmd+" · "+m.desc).FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).SingleLine()
									})
									if cBtn.Clicked() {
										replaced := curDraft[:bs] + m.ins
										*fBuf = replaced
										next := cloneRowsSlice(rows)
										next[activeRowIdx]["formula"] = replaced
										v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
									}
								}
							})
						}
					}
				})
			}

			// 4. Auto-detect undefined variables in formula -> gentle 1-click [+ 添加变量] pills (zero scary red English!)
			if editableDesign && activeRowIdx >= 0 {
				var missingVars []string
				for _, vName := range ExtractLatexVars(textValue(rows[activeRowIdx], "formula")) {
					if !definedNormVars[normalizeVarLookup(vName)] {
						missingVars = append(missingVars, vName)
					}
				}
				if len(missingVars) > 0 {
					ui.Row(c).FillWidth().Wrap().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
						Radius(float32(tok.ControlRadius)).Background(tok.Warning.Alpha(0.10)).Border(1, tok.Warning.Alpha(0.45)).
						AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Icon(c, lucide.Sparkles()).Size(float32(tok.Spacing12)+2, float32(tok.Spacing12)+2).TextColor(tok.Warning)
						ui.Text(c, "检测到公式中的新变量，一键添加输入框：").FontSize(float32(tok.TextMeta)).TextColor(tok.Warning)
						for _, mv := range missingVars {
							mv := mv
							addV := ui.ButtonBase(c.Key("calc-addmv:"+moduleID+":"+mv)).
								Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
								AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
								Background(tok.Surface).Border(1, tok.Warning.Alpha(0.55)).Label("添加变量 " + mv)
							addV.Children(func() {
								ui.Text(c, "+ "+FormatVarSymbolPretty(mv)).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Warning).SingleLine()
							})
							if addV.Clicked() {
								next := cloneRowsSlice(rows)
								// Insert new input variable BEFORE formula rows
								insertPos := len(inputIdxs)
								newRow := map[string]any{
									"key":   mv,
									"label": "变量 " + FormatVarSymbolPretty(mv),
									"kind":  "input",
									"value": 1,
									"unit":  "",
								}
								head := append([]map[string]any{}, next[:insertPos]...)
								head = append(head, newRow)
								next = append(head, next[insertPos:]...)
								v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
							}
						}
					})
				} else if res.Err != nil {
					ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
						Radius(float32(tok.ControlRadius)).Background(tok.Warning.Alpha(0.10)).Border(1, tok.Warning.Alpha(0.45)).
						AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Icon(c, lucide.TriangleAlert()).Size(float32(tok.Spacing12)+2, float32(tok.Spacing12)+2).TextColor(tok.Warning)
						ui.Text(c, friendlyCalcError(res.Err)).FontSize(float32(tok.TextMeta)).TextColor(tok.Warning)
					})
				}
			}

			// 5. BOTTOM INPUT VARIABLES ZONE (clearly separated from outputs; click variable pill to insert into formula!)
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, "输入变量").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
					if editableDesign {
						ui.Text(c, "点击符号胶囊可直接插入公式").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted.Alpha(0.75))
					}
					ui.Spacer(c)
					if editableDesign {
						if hasPlate {
							if components.Button(c, "+ 引入孔板实验孔数", components.ButtonGhost).Clicked() {
								next := cloneRowsSlice(rows)
								insertPos := len(inputIdxs)
								refVal := "@plate.sample_wells"
								refLbl := "有效实验孔数"
								if primaryPlate != nil && primaryPlate.BlankWells == 0 {
									refVal = "@plate.wells"
									refLbl = "孔板已分配孔数"
								}
								newRow := map[string]any{
									"key":   "N",
									"label": refLbl,
									"kind":  "input",
									"value": refVal,
									"unit":  "孔",
								}
								head := append([]map[string]any{}, next[:insertPos]...)
								head = append(head, newRow)
								next = append(head, next[insertPos:]...)
								v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
							}
						}
						if len(upstreamCalcs) > 0 {
							upBtn := ui.ButtonBase(c.Key("calc-upref:"+moduleID)).
								Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
								AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
								Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.32)).Label("引用上游计算结果")
							upBtn.Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
									ui.Icon(c, lucide.Link()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
									ui.Text(c, "+ 引用上游计算").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Accent).SingleLine()
								})
							})
							components.MenuButton(c, upBtn, func(m *components.Menu) {
								for _, uco := range upstreamCalcs {
									uco := uco
									uStr := formatNumber(uco.Amount)
									if uco.Unit != "" {
										uStr += " " + DisplayUnit(uco.Unit)
									}
									itemLbl := fmt.Sprintf("%s · %s (%s)", uco.Title, uco.Label, uStr)
									if m.Item(itemLbl).Chosen() {
										next := cloneRowsSlice(rows)
										insertPos := len(inputIdxs)
										newKey := sanitizeCalcVar(uco.Key)
										if definedNormVars[normalizeVarLookup(newKey)] {
											newKey = fmt.Sprintf("%s_%d", newKey, len(inputIdxs)+1)
										}
										newRow := map[string]any{
											"key":   newKey,
											"label": uco.Label + "（联动）",
											"kind":  "input",
											"value": fmt.Sprintf("@calc.%s.%s", uco.ModuleID, uco.Key),
											"unit":  uco.Unit,
										}
										head := append([]map[string]any{}, next[:insertPos]...)
										head = append(head, newRow)
										next = append(head, next[insertPos:]...)
										v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
									}
								}
							})
						}
						if components.Button(c, "+ 添加变量", components.ButtonGhost).Clicked() {
							next := cloneRowsSlice(rows)
							n := len(inputIdxs) + 1
							insertPos := len(inputIdxs)
							newRow := map[string]any{
								"key":   fmt.Sprintf("x_%d", n),
								"label": fmt.Sprintf("参数 %d", n),
								"kind":  "input",
								"value": 1,
								"unit":  "",
							}
							head := append([]map[string]any{}, next[:insertPos]...)
							head = append(head, newRow)
							next = append(head, next[insertPos:]...)
							v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
						}
					}
				})

				for _, idx := range inputIdxs {
					idx := idx
					rm := rows[idx]
					rkey := firstNonEmpty(textValue(rm, "key"), fmt.Sprintf("x_%d", idx+1))
					rlbl := firstNonEmpty(textValue(rm, "label"), rkey)
					runit := textValue(rm, "unit")
					valStr := formatAnyValue(rm["value"])

					rowBox := ui.Row(c.Key(fmt.Sprintf("calc-in:%s:%d", moduleID, idx))).FillWidth().
						Padding(float32(tok.Spacing4)*1.25, float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).
						AlignItems(ui.Center).Gap(float32(tok.Spacing8))
					rowHover := rowBox.Hovered()
					rowBg := ui.Transparent
					if rowHover {
						rowBg = tok.Text.Alpha(0.03)
					}
					rowBox.Background(rowBox.AnimateColor("calc-inbg", rowBg, theme.MotionMicroFade))
					rowBox.Children(func() {
						// Clickable Math Symbol Pill -> inserts `C_{1}` into active formula on click!
						symPill := ui.ButtonBase(c.Key(fmt.Sprintf("calc-ins:%s:%d", moduleID, idx))).
							Height(float32(tok.Spacing24)).MinWidth(float32(tok.Spacing32)+float32(tok.Spacing12)).
							Padding(0, float32(tok.Spacing8)).Center().
							Radius(float32(tok.ControlRadius)).
							Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.32)).
							Shrink(0).Label("插入变量 " + rkey)
						if symPill.Hovered() {
							symPill.Background(tok.Accent.Alpha(0.18))
						}
						symPill.Children(func() {
							ui.Text(c, FormatVarSymbolPretty(rkey)).FontSize(float32(tok.TextData)).Bold().Italic().
								TextColor(tok.Accent).SingleLine()
						})
						components.Tooltip(c, symPill, "点击插入 "+FormatVarLatexInsert(rkey)+" 到上方公式")
						if symPill.Clicked() && editableDesign {
							*customOpen = true
							appendToActiveFormula(FormatVarLatexInsert(rkey))
						}

						// Human Label
						ui.Box(c).Grow(1).MinWidth(0).Children(func() {
							if editableDesign {
								lblKey := fmt.Sprintf("calc_lbl:%s:%d", moduleID, idx)
								lblBuf := v.ensureWidgetDraft(lblKey, rlbl)
								ti := BorderlessTextInput(c, tok, lblBuf, float32(tok.TextBody), false, "变量名称")
								if val, ok := v.syncDraftInput(lblKey, rlbl, ti, lblBuf); ok {
									next := cloneRowsSlice(rows)
									next[idx]["label"] = firstNonEmpty(val, rlbl)
									v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
								}
							} else {
								ui.Text(c, rlbl).FontSize(float32(tok.TextBody)).TextColor(tok.Text).SingleLine().Ellipsis("…")
							}
						})

						// Distinct Bordered Numeric Input Box (unmistakably an INPUT!)
						dispVal := valStr
						if !editableDesign && strings.HasPrefix(valStr, "@plate") {
							if primaryPlate != nil {
								if valStr == "@plate.sample_wells" {
									dispVal = fmt.Sprintf("%d 实验孔", primaryPlate.SampleWells)
								} else if valStr == "@plate.blank_wells" {
									dispVal = fmt.Sprintf("%d 空白孔", primaryPlate.BlankWells)
								} else {
									dispVal = fmt.Sprintf("%d 孔", primaryPlate.TotalWells)
								}
							} else {
								dispVal = "自动统计孔数"
							}
						} else if !editableDesign && strings.HasPrefix(valStr, "@calc.") && docGraph != nil {
							for _, co := range docGraph.CalcOutputs {
								if fmt.Sprintf("@calc.%s.%s", co.ModuleID, co.Key) == valStr {
									dispVal = formatNumber(co.Amount) + " (联动)"
									break
								}
							}
						}
						ui.Box(c).Width(float32(tok.Spacing48)*2.3).Shrink(0).
							Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.BorderStrong.Alpha(0.55)).Children(func() {
							if !v.ReadOnly {
								vKey := fmt.Sprintf("calc_v:%s:%d", moduleID, idx)
								vBuf := v.ensureWidgetDraft(vKey, valStr)
								tb := BorderlessTextInput(c, tok, vBuf, float32(tok.TextData), true, "输入数值").FontFeatures("tnum")
								if val, ok := v.syncDraftInput(vKey, valStr, tb, vBuf); ok {
									next := cloneRowsSlice(rows)
									if num, err := strconv.ParseFloat(val, 64); err == nil {
										next[idx]["value"] = num
									} else {
										next[idx]["value"] = val
									}
									v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
								}
							} else {
								ui.Text(c, dispVal).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).FontFeatures("tnum").SingleLine()
							}
						})

						// Unit pill / input
						ui.Box(c).Width(float32(tok.Spacing48)*1.5).Shrink(0).Children(func() {
							if editableDesign {
								ui.Box(c).FillWidth().Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).
									Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border.Alpha(0.7)).Children(func() {
									uKey := fmt.Sprintf("calc_u:%s:%d", moduleID, idx)
									uBuf := v.ensureWidgetDraft(uKey, runit)
									ut := BorderlessTextInput(c, tok, uBuf, float32(tok.TextMeta), false, "单位").TextColor(tok.TextMuted)
									if val, ok := v.syncDraftInput(uKey, runit, ut, uBuf); ok {
										next := cloneRowsSlice(rows)
										next[idx]["unit"] = val
										v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
									}
								})
							} else {
								ui.Text(c, DisplayUnit(runit)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
							}
						})

						if editableDesign && len(inputIdxs) > 1 {
							delOp := float32(0)
							if rowHover || v.MobileCompact {
								delOp = 1
							}
							delWrap := ui.Box(c.Key(fmt.Sprintf("calc-del:%s:%d", moduleID, idx))).Width(float32(tok.Spacing24)).Shrink(0)
							delWrap.Opacity(delWrap.AnimateTo("cdop", delOp, theme.MotionMicroFade)).Children(func() {
								if components.IconButton(c, lucide.X(), "删除变量", components.IconButtonCompact).Clicked() {
									next := removeSliceIndex(rows, idx)
									v.emitAnyModuleEdit(moduleID, map[string]any{"rows": next})
								}
							})
						}
					})
				}
			})
		})
	})
}

func (v *View) applyQuantityScratch(moduleID string, data map[string]any, field string) {
	if val, ok := v.calcScratchFloat(moduleID, field); ok && val > 0 {
		if qm, ok := data[field].(map[string]any); ok {
			copyMap := make(map[string]any, len(qm))
			for k, mv := range qm {
				copyMap[k] = mv
			}
			copyMap["value"] = val
			data[field] = copyMap
		}
	}
}

func (v *View) calcScratchFloat(moduleID, field string) (float64, bool) {
	if v.calcScratch == nil || v.calcScratch[moduleID] == nil {
		return 0, false
	}
	ptr := v.calcScratch[moduleID][field]
	if ptr == nil || strings.TrimSpace(*ptr) == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(*ptr), 64)
	return f, err == nil
}

func (v *View) ensureCalcScratch(moduleID, field, initVal string) *string {
	if v.calcScratch == nil {
		v.calcScratch = map[string]map[string]*string{}
	}
	if v.calcScratch[moduleID] == nil {
		v.calcScratch[moduleID] = map[string]*string{}
	}
	if v.calcScratch[moduleID][field] == nil {
		s := initVal
		v.calcScratch[moduleID][field] = &s
	}
	return v.calcScratch[moduleID][field]
}

func (v *View) renderCalcQtyInput(c *ui.Context, moduleID string, data map[string]any, field, label string) {
	tok := theme.GetTokens(c.Theme())
	qv, qu, _ := quantityParts(data[field])
	buf := v.ensureCalcScratch(moduleID, field, formatNumber(qv))
	ui.Column(c).Width(float32(tok.Spacing48)*2.5).Gap(float32(tok.Spacing4)).Children(func() {
		ui.Text(c, label+" ("+qu+")").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		components.TextBox(c, buf, components.TextBoxOptions{})
	})
}

func (v *View) renderCalcNumInput(c *ui.Context, moduleID, field, label string, raw any) {
	tok := theme.GetTokens(c.Theme())
	n, _ := numericValue(raw)
	buf := v.ensureCalcScratch(moduleID, field, formatNumber(n))
	ui.Column(c).Width(float32(tok.Spacing48)*2.2).Gap(float32(tok.Spacing4)).Children(func() {
		ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		components.TextBox(c, buf, components.TextBoxOptions{})
	})
}

func (v *View) calcScratchString(moduleID, field string) (string, bool) {
	if v.calcScratch == nil || v.calcScratch[moduleID] == nil {
		return "", false
	}
	ptr := v.calcScratch[moduleID][field]
	if ptr == nil {
		return "", false
	}
	return strings.TrimSpace(*ptr), true
}

func (v *View) renderCalcTextInput(c *ui.Context, moduleID, field, label, initVal string, width float32) {
	tok := theme.GetTokens(c.Theme())
	buf := v.ensureCalcScratch(moduleID, field, initVal)
	ui.Column(c).Width(width).Gap(float32(tok.Spacing4)).Children(func() {
		ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		components.TextBox(c, buf, components.TextBoxOptions{})
	})
}

func calcModeLabel(mode string) string {
	switch mode {
	case "dilution":
		return "稀释 C1V1=C2V2"
	case "group_mastermix":
		return "分组母液混配"
	case "serial_dilution":
		return "梯度稀释"
	case "sample_normalize":
		return "样本归一化"
	case "formula":
		return "自由公式"
	default:
		return mode
	}
}

func (v *View) renderTimer(c *ui.Context, data map[string]any) {
	tok := theme.GetTokens(c.Theme())
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "实验计时与周期")
	mode := textValue(data, "mode")
	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)
	interactiveRun := v.Mode == ModeRun && !v.ReadOnly && v.OnTimerAction != nil

	if v.UnifiedLive {
		v.renderUnifiedTimer(c, tok, moduleID, title, mode, data, editableDesign)
		return
	}

	plannedSec, spanSec, _ := timerBudgetSec(data)
	rt, hasRT := v.TimerStates[moduleID]
	displaySec := plannedSec
	if displaySec == 0 {
		displaySec = spanSec
	}
	statusText := "待启动"
	if hasRT {
		if rt.RemainingS > 0 {
			displaySec = rt.RemainingS
		} else if rt.AccumulatedS > 0 {
			displaySec = rt.AccumulatedS
		}
		if rt.Status != "" {
			statusText = timerStatusLabel(rt.Status)
		}
	}

	moduleCardFrame(v, c, lucide.Timer(), title, timerModeLabel(mode), nil, func() {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
				ui.Text(c, formatClockHMS(displaySec)).FontSize(float32(tok.TextPage)).Bold().
					TextColor(tok.Text).FontFeatures("tnum")
				components.Badge(c, statusText)
				if mode == "cycle" {
					rounds := int(numOrZero(data["rounds"]))
					if rounds < 1 {
						rounds = 1
					}
					curRound := 1
					if hasRT && rt.RoundIndex > 0 {
						curRound = rt.RoundIndex + 1
					}
					components.Badge(c, fmt.Sprintf("第 %d / %d 轮", curRound, rounds), components.BadgeOptions{Tone: components.BadgeAccent})
				}
				if mode == "window" {
					if anchor := textValue(data, "anchor"); anchor != "" {
						ui.Text(c, "相对锚点 · "+anchor).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					} else if win := textValue(data, "schedule_window"); win != "" {
						ui.Text(c, "时间窗 · "+win).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					}
				}
			})

			if mode == "cycle" {
				phases := mapSlice(data["phases"])
				ui.Row(c).Wrap().Gap(float32(tok.Spacing8)).Children(func() {
					for pIdx, ph := range phases {
						pname := textValue(ph, "name")
						psec := quantitySeconds(ph["duration"])
						active := hasRT && rt.PhaseIndex == pIdx
						bg := tok.Elevated
						fg := tok.TextMuted
						if active {
							bg = tok.AccentSoftBg
							fg = tok.Accent
						}
						ui.Row(c).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(bg).Border(1, tok.Border).
							Gap(float32(tok.Spacing4)).Children(func() {
							ui.Text(c, pname).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg)
							if psec > 0 {
								ui.Text(c, formatClockHMS(psec)).FontSize(float32(tok.TextMeta)).TextColor(fg).FontFeatures("tnum")
							} else {
								ui.Text(c, "手动推进").FontSize(float32(tok.TextMeta)).TextColor(fg)
							}
						})
					}
				})
			}

			if editableDesign {
				qRaw := data["duration"]
				if mode == "window" && data["offset"] != nil {
					qRaw = data["offset"]
				}
				qv, qu, qErr := quantityParts(qRaw)
				if qErr != nil || qu == "" {
					qv, qu = 10, "min"
				}
				roundsInit := int(numOrZero(data["rounds"]))
				if roundsInit < 1 {
					roundsInit = 1
				}
				tmMode := v.ensureWidgetDraft("tm:"+moduleID+":mode", firstNonEmpty(mode, "countdown"))
				if *tmMode != mode && mode != "" {
					*tmMode = mode
				}
				tmVal := v.ensureWidgetDraft("tm:"+moduleID+":val", formatNumber(qv))
				tmUnit := v.ensureWidgetDraft("tm:"+moduleID+":unit", qu)
				tmRounds := v.ensureWidgetDraft("tm:"+moduleID+":rounds", strconv.Itoa(roundsInit))
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).
					Padding(float32(tok.Spacing4), 0, 0, 0).Children(func() {
					ui.Text(c, "模式").FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
					tSel := components.Select(c, tmMode)
					tSel.Trigger.Children(func() {
						ui.Text(c, timerModeLabel(*tmMode)).FontSize(float32(tok.TextData)).TextColor(tok.Text)
					})
					tSel.Popup(func(panel ui.Element) {
						for _, opt := range []string{"countdown", "stopwatch", "cycle", "window"} {
							opt := opt
							tSel.Item(opt).Children(func() {
								ui.Text(c, timerModeLabel(opt)).FontSize(float32(tok.TextData)).TextColor(tok.Text)
							})
						}
					})
					if tSel.Trigger.Changed() && *tmMode != mode {
						v.emitAnyModuleEdit(moduleID, map[string]any{"mode": *tmMode})
					}
					if *tmMode != "stopwatch" && *tmMode != "cycle" {
						ui.Box(c).Width(float32(tok.Spacing48)*1.5).Children(func() {
							components.TextBox(c, tmVal, components.TextBoxOptions{}).Placeholder("数值")
						})
						uSel := components.Select(c, tmUnit)
						uSel.Trigger.Children(func() {
							ui.Text(c, *tmUnit).FontSize(float32(tok.TextData)).TextColor(tok.Text)
						})
						uSel.Popup(func(panel ui.Element) {
							for _, opt := range []string{"s", "min", "h", "d"} {
								opt := opt
								uSel.Item(opt).Children(func() {
									ui.Text(c, opt).FontSize(float32(tok.TextData)).TextColor(tok.Text)
								})
							}
						})
					}
					if *tmMode == "cycle" {
						ui.Text(c, "轮数").FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
						ui.Box(c).Width(float32(tok.Spacing48)*1.2).Children(func() {
							components.TextBox(c, tmRounds, components.TextBoxOptions{}).Placeholder("轮数")
						})
					}
					ui.Spacer(c)
					if components.Button(c, "保存计时设定", components.ButtonSecondary).Clicked() {
						props := map[string]any{"mode": *tmMode}
						numVal, err := strconv.ParseFloat(strings.TrimSpace(*tmVal), 64)
						if err != nil || numVal <= 0 {
							numVal = 10
						}
						switch *tmMode {
						case "window":
							props["offset"] = map[string]any{"value": numVal, "unit": *tmUnit}
						case "cycle":
							rCnt, _ := strconv.Atoi(strings.TrimSpace(*tmRounds))
							if rCnt < 1 {
								rCnt = 1
							}
							props["rounds"] = rCnt
						case "countdown":
							props["duration"] = map[string]any{"value": numVal, "unit": *tmUnit}
						}
						v.emitAnyModuleEdit(moduleID, props)
					}
				})
			}

			if interactiveRun {
				ui.Row(c).Gap(float32(tok.Spacing8)).Children(func() {
					if iconButton(c, lucide.Play(), "开始", components.ButtonSecondary).Clicked() {
						v.OnTimerAction(v.StepID, moduleID, "start")
					}
					if iconButton(c, lucide.Pause(), "暂停", components.ButtonOutline).Clicked() {
						v.OnTimerAction(v.StepID, moduleID, "pause")
					}
					if mode == "cycle" {
						if iconButton(c, lucide.SkipForward(), "下一阶段", components.ButtonOutline).Clicked() {
							v.OnTimerAction(v.StepID, moduleID, "advance")
						}
					}
					if iconButton(c, lucide.Check(), "完成", components.ButtonOutline).Clicked() {
						v.OnTimerAction(v.StepID, moduleID, "complete")
					}
				})
			}
		})
	})
}

func (v *View) renderUnifiedTimer(c *ui.Context, tok theme.Tokens, moduleID, title, mode string, data map[string]any, editableDesign bool) {
	mode = firstNonEmpty(mode, "countdown")
	plannedSec, spanSec, _ := timerBudgetSec(data)
	if plannedSec == 0 {
		plannedSec = spanSec
	}

	inlineStatus := strings.ToLower(strings.TrimSpace(textValue(data, "status")))
	if inlineStatus == "" {
		if rt, ok := v.TimerStates[moduleID]; ok && rt.Status != "" {
			inlineStatus = strings.ToLower(strings.TrimSpace(rt.Status))
		} else {
			inlineStatus = "idle"
		}
	}
	baseElapsed := int(numOrZero(data["elapsed_sec"]))
	liveElapsed := baseElapsed
	if inlineStatus == "running" {
		if tStart, err := time.Parse(time.RFC3339, textValue(data, "started_at")); err == nil {
			delta := int(time.Since(tStart).Seconds())
			if delta > 0 {
				liveElapsed += delta
			}
		}
	}

	isCountdown := mode != "stopwatch" && plannedSec > 0
	displaySec := liveElapsed
	if isCountdown {
		displaySec = plannedSec - liveElapsed
		if displaySec < 0 {
			displaySec = 0
		}
	}
	if inlineStatus == "running" && v.OnAsyncInvalidate != nil {
		lastWake := ui.Local(c.Root(), "tm-wake:"+moduleID, func() time.Time { return time.Time{} })
		if time.Since(*lastWake) >= 240*time.Millisecond {
			*lastWake = time.Now()
			inv := v.OnAsyncInvalidate
			time.AfterFunc(250*time.Millisecond, inv)
		}
	}

	timerWrap := ui.Column(c.Key("live-tm:"+moduleID)).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		Radius(float32(tok.ControlRadius)).Background(tok.Elevated.Alpha(0.55)).Gap(float32(tok.Spacing8))
	targetClockCol := tok.Text
	if inlineStatus == "running" {
		targetClockCol = tok.Accent
	} else if inlineStatus == "completed" {
		targetClockCol = tok.Success
	}
	clockCol := timerWrap.AnimateColor("tm-col", targetClockCol, theme.MotionSnappy)
	phases := mapSlice(data["phases"])
	rounds := int(numOrZero(data["rounds"]))
	if rounds < 1 {
		rounds = 1
	}
	anchorVal := textValue(data, "anchor")
	schedWin := textValue(data, "schedule_window")

	timerWrap.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Wrap().Gap(float32(tok.Spacing8)).Children(func() {
			ui.Icon(c, lucide.Timer()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(clockCol)
			ui.Text(c, formatClockHMS(displaySec)).FontSize(float32(tok.TextSection)).Bold().
				TextColor(clockCol).
				FontFeatures("tnum").
				ContentTransition(theme.NumericRoll(isCountdown))
			if editableDesign {
				renderInlineEditableTitle(v, c, moduleID, title, true)
			} else {
				ui.Text(c, title).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
			}

			// Inline editable duration/mode in Edit mode (borderless quiet inputs)
			if editableDesign && inlineStatus == "idle" {
				qRaw := data["duration"]
				if mode == "window" && data["offset"] != nil {
					qRaw = data["offset"]
				}
				qv, qu, qErr := quantityParts(qRaw)
				if qErr != nil || qu == "" {
					qv, qu = 10, "min"
				}
				tmMode := v.ensureWidgetDraft("tm:"+moduleID+":mode", mode)
				*tmMode = mode
				mSel := components.Select(c, tmMode)
				StyleSelectQuiet(c, tok, mSel)
				mSel.Trigger.Children(func() {
					ui.Text(c, timerModeLabel(*tmMode)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				})
				mSel.Popup(func(panel ui.Element) {
					for _, opt := range []string{"countdown", "stopwatch", "cycle", "window"} {
						opt := opt
						mSel.Item(opt).Children(func() {
							ui.Text(c, timerModeLabel(opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
						})
					}
				})
				if mSel.Trigger.Changed() && *tmMode != mode {
					patch := map[string]any{"mode": *tmMode}
					switch *tmMode {
					case "countdown":
						if qv <= 0 {
							qv, qu = 10, "min"
						}
						patch["duration"] = map[string]any{"value": qv, "unit": qu}
					case "cycle":
						patch["rounds"] = max(1, rounds)
						if len(phases) == 0 {
							patch["phases"] = []any{
								map[string]any{"name": "阶段 1", "mode": "fixed", "duration": map[string]any{"value": 5, "unit": "min"}},
								map[string]any{"name": "阶段 2", "mode": "fixed", "duration": map[string]any{"value": 10, "unit": "min"}},
							}
						}
					case "window":
						patch["anchor"] = firstNonEmpty(anchorVal, "t0")
						if qv < 0 {
							qv = 0
						}
						patch["offset"] = map[string]any{"value": qv, "unit": firstNonEmpty(qu, "h")}
					}
					v.emitAnyModuleEdit(moduleID, patch)
				}
				tmVal := v.ensureWidgetDraft("tm:"+moduleID+":val", formatNumber(qv))
				tmUnit := v.ensureWidgetDraft("tm:"+moduleID+":unit", qu)
				if mode != "stopwatch" && mode != "cycle" {
					ui.Box(c).Width(float32(tok.Spacing48)*1.1).Children(func() {
						tb := BorderlessTextInput(c, tok, tmVal, float32(tok.TextBody), false, "时长").FontFeatures("tnum")
						if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*tmVal) != formatNumber(qv)) {
							if nv, err := strconv.ParseFloat(strings.TrimSpace(*tmVal), 64); err == nil && (nv > 0 || (mode == "window" && nv >= 0)) {
								field := "duration"
								if mode == "window" {
									field = "offset"
								}
								v.emitAnyModuleEdit(moduleID, map[string]any{field: map[string]any{"value": nv, "unit": *tmUnit}})
							}
						}
					})
					uSel := components.Select(c, tmUnit)
					StyleSelectQuiet(c, tok, uSel)
					uSel.Trigger.Children(func() {
						ui.Text(c, *tmUnit).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					})
					uSel.Popup(func(panel ui.Element) {
						for _, opt := range []string{"s", "min", "h", "d"} {
							opt := opt
							uSel.Item(opt).Children(func() {
								ui.Text(c, opt).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						}
					})
					if uSel.Trigger.Changed() && *tmUnit != qu {
						if nv, err := strconv.ParseFloat(strings.TrimSpace(*tmVal), 64); err == nil && (nv > 0 || (mode == "window" && nv >= 0)) {
							field := "duration"
							if mode == "window" {
								field = "offset"
							}
							v.emitAnyModuleEdit(moduleID, map[string]any{field: map[string]any{"value": nv, "unit": *tmUnit}})
						}
					}
				}
			}

			ui.Spacer(c)
			badgeTone := components.BadgeNeutral
			if inlineStatus == "running" {
				badgeTone = components.BadgeAccent
			} else if inlineStatus == "completed" {
				badgeTone = components.BadgeSuccess
			}
			inlineBadge(c, timerStatusLabel(inlineStatus), components.BadgeOptions{Tone: badgeTone})

			if !v.ReadOnly {
				if inlineStatus != "running" {
					if iconButton(c, lucide.Play(), "启动计时", components.ButtonGhost).Clicked() {
						v.emitAnyModuleEdit(moduleID, map[string]any{
							"status":     "running",
							"started_at": time.Now().UTC().Format(time.RFC3339),
						})
						if v.OnTimerAction != nil {
							v.OnTimerAction(v.StepID, moduleID, "start")
						}
					}
				} else {
					if iconButton(c, lucide.Pause(), "暂停计时", components.ButtonOutline).Clicked() {
						v.emitAnyModuleEdit(moduleID, map[string]any{
							"status":      "paused",
							"elapsed_sec": liveElapsed,
							"started_at":  "",
						})
						if v.OnTimerAction != nil {
							v.OnTimerAction(v.StepID, moduleID, "pause")
						}
					}
				}
				if inlineStatus == "running" || inlineStatus == "paused" {
					if iconButton(c, lucide.Check(), "完成计时", components.ButtonOutline).Clicked() {
						v.emitAnyModuleEdit(moduleID, map[string]any{
							"status":       "completed",
							"elapsed_sec":  liveElapsed,
							"started_at":   "",
							"completed_at": time.Now().UTC().Format(time.RFC3339),
						})
						if v.OnTimerAction != nil {
							v.OnTimerAction(v.StepID, moduleID, "complete")
						}
					}
				}
				if inlineStatus != "idle" {
					if components.IconButton(c, lucide.RotateCcw(), "重置计时", components.IconButtonCompact).Clicked() {
						v.emitAnyModuleEdit(moduleID, map[string]any{
							"status":       "idle",
							"elapsed_sec":  0,
							"started_at":   "",
							"completed_at": "",
						})
					}
				}
			}
		})

		// Multi-stage cycle editor & summary (supports e.g. 80°C 5min -> Ice 10min x 3)
		if mode == "cycle" {
			if editableDesign && inlineStatus == "idle" {
				ui.Column(c.Key("tm-cycle-ed:"+moduleID)).FillWidth().Gap(float32(tok.Spacing4)).
					Padding(float32(tok.Spacing4), 0, 0, float32(tok.Spacing24)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Text(c, "循环轮数").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						rBuf := v.ensureWidgetDraft("tm:"+moduleID+":rounds", strconv.Itoa(rounds))
						ui.Box(c).Width(float32(tok.Spacing48)*1.1).Children(func() {
							tb := BorderlessTextInput(c, tok, rBuf, float32(tok.TextMeta), true, "轮数").FontFeatures("tnum")
							if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*rBuf) != strconv.Itoa(rounds)) {
								if rn, err := strconv.Atoi(strings.TrimSpace(*rBuf)); err == nil && rn >= 1 {
									v.emitAnyModuleEdit(moduleID, map[string]any{"rounds": rn})
								}
							}
						})
						ui.Text(c, "轮").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						ui.Spacer(c)
						if components.Button(c, "+ 添加阶段", components.ButtonGhost).Clicked() {
							nextPh := cloneRowsSlice(phases)
							nextPh = append(nextPh, map[string]any{
								"name":     fmt.Sprintf("阶段 %d", len(nextPh)+1),
								"mode":     "fixed",
								"duration": map[string]any{"value": 5, "unit": "min"},
							})
							v.emitAnyModuleEdit(moduleID, map[string]any{"phases": nextPh})
						}
					})
					for pIdx, ph := range phases {
						pIdx, ph := pIdx, ph
						pName := firstNonEmpty(textValue(ph, "name"), fmt.Sprintf("阶段 %d", pIdx+1))
						pv, pu, _ := quantityParts(ph["duration"])
						if pu == "" {
							pv, pu = 5, "min"
						}
						ui.Row(c.Key(fmt.Sprintf("tm-ph:%s:%d", moduleID, pIdx))).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
							ui.Text(c, fmt.Sprintf("%d.", pIdx+1)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
							ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*2.0).Children(func() {
								nBuf := v.ensureWidgetDraft(fmt.Sprintf("tm:%s:ph:%d:n", moduleID, pIdx), pName)
								tb := BorderlessTextInput(c, tok, nBuf, float32(tok.TextMeta), false, "阶段名称（如 80°C 水浴）").Label(fmt.Sprintf("阶段 %d 名称", pIdx+1))
								if !tb.Focused() && *nBuf != pName {
									*nBuf = pName
								}
								if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*nBuf) != pName) {
									if trimmed := strings.TrimSpace(*nBuf); trimmed != "" {
										nextPh := cloneRowsSlice(phases)
										if pIdx < len(nextPh) {
											nextPh[pIdx]["name"] = trimmed
											v.emitAnyModuleEdit(moduleID, map[string]any{"phases": nextPh})
										}
									}
								}
							})
							ui.Box(c).Width(float32(tok.Spacing48)*1.0).Children(func() {
								vBuf := v.ensureWidgetDraft(fmt.Sprintf("tm:%s:ph:%d:v", moduleID, pIdx), formatNumber(pv))
								tb := BorderlessTextInput(c, tok, vBuf, float32(tok.TextMeta), false, "时长").FontFeatures("tnum").Label(fmt.Sprintf("阶段 %d 时长", pIdx+1))
								if !tb.Focused() && *vBuf != formatNumber(pv) {
									*vBuf = formatNumber(pv)
								}
								if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*vBuf) != formatNumber(pv)) {
									if nv, err := strconv.ParseFloat(strings.TrimSpace(*vBuf), 64); err == nil && nv > 0 {
										nextPh := cloneRowsSlice(phases)
										if pIdx < len(nextPh) {
											nextPh[pIdx]["duration"] = map[string]any{"value": nv, "unit": pu}
											v.emitAnyModuleEdit(moduleID, map[string]any{"phases": nextPh})
										}
									}
								}
							})
							pUnit := v.ensureWidgetDraft(fmt.Sprintf("tm:%s:ph:%d:u", moduleID, pIdx), pu)
							*pUnit = pu
							puSel := components.Select(c, pUnit)
							StyleSelectQuiet(c, tok, puSel)
							puSel.Trigger.Children(func() {
								ui.Text(c, *pUnit).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
							})
							puSel.Popup(func(panel ui.Element) {
								for _, uOpt := range []string{"s", "min", "h"} {
									uOpt := uOpt
									puSel.Item(uOpt).Children(func() {
										ui.Text(c, uOpt).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
									})
								}
							})
							if puSel.Trigger.Changed() && *pUnit != pu {
								nextPh := cloneRowsSlice(phases)
								if pIdx < len(nextPh) {
									nextPh[pIdx]["duration"] = map[string]any{"value": max(1.0, pv), "unit": *pUnit}
									v.emitAnyModuleEdit(moduleID, map[string]any{"phases": nextPh})
								}
							}
							if len(phases) > 1 {
								if components.IconButton(c, lucide.X(), "删除阶段", components.IconButtonCompact).Clicked() {
									nextPh := removeSliceIndex(phases, pIdx)
									v.emitAnyModuleEdit(moduleID, map[string]any{"phases": nextPh})
								}
							}
						})
					}
				})
			} else if len(phases) > 0 {
				var parts []string
				for _, ph := range phases {
					pv, pu, _ := quantityParts(ph["duration"])
					parts = append(parts, fmt.Sprintf("%s %s %s", textValue(ph, "name"), formatNumber(pv), pu))
				}
				ui.Text(c, fmt.Sprintf("循环 %d 轮：%s", rounds, strings.Join(parts, " → "))).
					FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
			}
		}

		// Time-window anchor & schedule window note + live {#t0} wake-up propagation within the same document!
		if mode == "window" {
			ancKey := firstNonEmpty(anchorVal, "t0")
			var anchorTS time.Time
			if v.anchorClock != nil {
				if ts, ok := v.anchorClock[ancKey]; ok {
					anchorTS = ts
				}
			}
			if anchorTS.IsZero() && v.Doc != nil {
				if g := BuildSciGraph(v.Doc); g != nil {
					if anc := g.AnchorByID(ancKey); anc != nil && !anc.RecordedAt.IsZero() {
						anchorTS = anc.RecordedAt
					}
				}
			}
			// Render live wake-up schedule banner when anchor is stamped, or 1-click stamp button!
			ui.Row(c.Key("tm-win-live:"+moduleID)).FillWidth().Wrap().AlignItems(ui.Center).
				Gap(float32(tok.Spacing8)).Padding(0, 0, 0, float32(tok.Spacing24)).Children(func() {
				if !anchorTS.IsZero() {
					offSec := plannedSec
					if offSec <= 0 {
						if ov, ou, err := quantityParts(data["offset"]); err == nil && ov > 0 {
							offSec = durationToSec(ov, normDurationUnit(ou))
						}
					}
					targetTS := anchorTS.Add(time.Duration(offSec) * time.Second)
					nowTS := time.Now()
					remSec := int(targetTS.Sub(nowTS).Seconds())
					bg := tok.AccentSoftBg
					fg := tok.Accent
					statusMsg := fmt.Sprintf("已锚定 %s (%s) → 目标 %s",
						ancKey, anchorTS.Local().Format("15:04"), targetTS.Local().Format("15:04"))
					if remSec > 0 {
						statusMsg += fmt.Sprintf("（剩余 %s）", formatClockHMS(remSec))
						c.After(time.Second)
					} else {
						bg = tok.SuccessBg
						fg = tok.Success
						statusMsg = fmt.Sprintf("已到窗口时刻！%s (%s) → 目标 %s（已超 %s）",
							ancKey, anchorTS.Local().Format("15:04"), targetTS.Local().Format("15:04"), formatClockHMS(-remSec))
					}
					ui.Row(c).Padding(float32(tok.Spacing4)*0.8, float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).Background(bg).Border(1, fg.Alpha(0.35)).
						AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Icon(c, lucide.BellRing()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).TextColor(fg)
						ui.Text(c, statusMsg).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg).FontFeatures("tnum")
					})
				} else if !v.ReadOnly {
					stampBtn := ui.ButtonBase(c.Key("tm-stamp:"+moduleID)).
						Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
						AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
						Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35)).
						Label("标记此刻为 " + ancKey)
					stampBtn.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							ui.Icon(c, lucide.Clock()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
							ui.Text(c, "标记此刻为 "+ancKey).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Accent).SingleLine()
						})
					})
					if stampBtn.Clicked() {
						if v.anchorClock == nil {
							v.anchorClock = map[string]time.Time{}
						}
						now := time.Now().UTC()
						v.anchorClock[ancKey] = now
						v.emitAnyModuleEdit(moduleID, map[string]any{"started_at": now.Format(time.RFC3339)})
					}
				}
			})

			if editableDesign && inlineStatus == "idle" {
				ui.Row(c.Key("tm-win-ed:"+moduleID)).FillWidth().Wrap().AlignItems(ui.Center).
					Gap(float32(tok.Spacing8)).Padding(0, 0, 0, float32(tok.Spacing24)).Children(func() {
					ui.Text(c, "锚点").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					aBuf := v.ensureWidgetDraft("tm:"+moduleID+":anchor", firstNonEmpty(anchorVal, "t0"))
					ui.Box(c).Width(float32(tok.Spacing48)*1.5).Children(func() {
						tb := BorderlessTextInput(c, tok, aBuf, float32(tok.TextMeta), false, "如 t0")
						if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*aBuf) != anchorVal) {
							if av := strings.TrimSpace(*aBuf); av != "" {
								v.emitAnyModuleEdit(moduleID, map[string]any{"anchor": av})
							}
						}
					})
					ui.Text(c, "窗口说明").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					wBuf := v.ensureWidgetDraft("tm:"+moduleID+":win", schedWin)
					ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*2.5).Children(func() {
						tb := BorderlessTextInput(c, tok, wBuf, float32(tok.TextMeta), false, "如 ≥ 2h 或 6h / 12h / 24h")
						if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*wBuf) != schedWin) {
							v.emitAnyModuleEdit(moduleID, map[string]any{"schedule_window": strings.TrimSpace(*wBuf)})
						}
					})
				})
			} else if anchorVal != "" || schedWin != "" {
				desc := "时间锚点 " + firstNonEmpty(anchorVal, "t0")
				if schedWin != "" {
					desc += " · " + schedWin
				}
				ui.Text(c, desc).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
			}
		}
	})
}

func timerStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "", "idle":
		return "待启动"
	case "running":
		return "计时中"
	case "paused":
		return "已暂停"
	case "completed":
		return "已完成"
	default:
		return status
	}
}

func timerModeLabel(mode string) string {
	switch mode {
	case "countdown":
		return "倒计时"
	case "stopwatch":
		return "正计时"
	case "cycle":
		return "多阶段循环"
	case "window":
		return "时间窗提醒"
	default:
		return mode
	}
}

func formatClockHMS(sec int) string {
	if sec < 0 {
		sec = 0
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

func (v *View) renderAttachment(c *ui.Context, data map[string]any) {
	tok := theme.GetTokens(c.Theme())
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "附件与现场拍摄")
	scope := textValue(data, "scope")
	containerRef := textValue(data, "container_ref")
	slots := int(numOrZero(data["slots_per_cell"]))
	if slots < 1 {
		slots = 1
	}
	mediaHint := textValue(data, "media_hint")
	thumbs := v.LinkedAssets[moduleID]
	if len(thumbs) == 0 && v.StepID != "" {
		thumbs = v.LinkedAssets[v.StepID]
	}
	if v.UnifiedLive {
		inlineItems := mapSlice(data["items"])
		for i, it := range inlineItems {
			fp := textValue(it, "src")
			if fp == "" {
				fp = textValue(it, "file_path")
			}
			if fp == "" {
				continue
			}
			thumbs = append(thumbs, AssetThumb{
				AssetID:  firstNonEmpty(textValue(it, "id"), fmt.Sprintf("att-%d", i+1)),
				Label:    firstNonEmpty(textValue(it, "label"), fp),
				FilePath: fp,
				WellID:   textValue(it, "well_id"),
				FovIndex: int(numOrZero(it["fov_index"])),
			})
		}
	}

	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)
	scopeBadge := "步骤证据"
	switch scope {
	case "design_ref":
		scopeBadge = "设计参考文档"
	case "container_grid":
		plateLabel := containerRef
		if pl, _ := v.findPlate(containerRef); pl != nil && strings.TrimSpace(pl.Title) != "" {
			plateLabel = pl.Title
		}
		scopeBadge = fmt.Sprintf("网格采图 · %s × %d 视野", firstNonEmpty(plateLabel, "孔板"), slots)
	}

	moduleCardFrame(v, c, lucide.Paperclip(), title, scopeBadge, nil, func() {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			if editableDesign {
				var docPlates []*Plate
				if v.Doc != nil {
					for _, fm := range flattenAllDocModules(v.Doc) {
						if fm.Kind == "plate" && fm.Plate != nil {
							docPlates = append(docPlates, fm.Plate)
						}
					}
				}
				ui.Row(c.Key("att-cfg:"+moduleID)).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, "采图模式").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					scopeVal := firstNonEmpty(scope, "step_evidence")
					sSel := components.Select(c.Key("att-scope:"+moduleID), &scopeVal)
					StyleSelectQuiet(c, tok, sSel)
					sSel.Trigger.Label("切换采图模式")
					scopeLabelFn := func(s string) string {
						switch s {
						case "design_ref":
							return "设计参考"
						case "container_grid":
							return "孔板网格采图"
						default:
							return "步骤证据"
						}
					}
					sSel.Trigger.Children(func() {
						ui.Text(c, scopeLabelFn(scopeVal)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
					})
					sSel.Popup(func(panel ui.Element) {
						for _, opt := range []string{"step_evidence", "container_grid", "design_ref"} {
							opt := opt
							sSel.Item(opt).Children(func() {
								ui.Text(c, scopeLabelFn(opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						}
					})
					if sSel.Trigger.Changed() && scopeVal != scope {
						patch := map[string]any{"scope": scopeVal}
						if scopeVal == "container_grid" {
							ref := containerRef
							if ref == "" && len(docPlates) > 0 {
								ref = docPlates[0].ID
							}
							if ref != "" {
								patch["container_ref"] = ref
							}
							if slots < 1 {
								patch["slots_per_cell"] = 1
							}
						}
						v.emitAnyModuleEdit(moduleID, patch)
					}

					if scope == "container_grid" {
						if len(docPlates) > 0 {
							refVal := containerRef
							pSel := components.Select(c.Key("att-plate:"+moduleID), &refVal)
							StyleSelectQuiet(c, tok, pSel)
							pSel.Trigger.Label("选择关联孔板")
							plateNameFor := func(id string) string {
								for _, dp := range docPlates {
									if dp.ID == id {
										return firstNonEmpty(dp.Title, fmt.Sprintf("%d 孔板", dp.Format))
									}
								}
								return "选择孔板"
							}
							pSel.Trigger.Children(func() {
								ui.Text(c, plateNameFor(refVal)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
							pSel.Popup(func(panel ui.Element) {
								for _, dp := range docPlates {
									dp := dp
									pSel.Item(dp.ID).Children(func() {
										ui.Text(c, firstNonEmpty(dp.Title, fmt.Sprintf("%d 孔板", dp.Format))).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
									})
								}
							})
							if pSel.Trigger.Changed() && refVal != containerRef {
								v.emitAnyModuleEdit(moduleID, map[string]any{"container_ref": refVal})
							}
						}
						ui.Text(c, "每孔视野").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						sBuf := v.ensureWidgetDraft("att:"+moduleID+":slots", strconv.Itoa(slots))
						ui.Box(c).Width(float32(tok.Spacing48)*1.0).Children(func() {
							tb := BorderlessTextInput(c, tok, sBuf, float32(tok.TextMeta), true, "槽位数").FontFeatures("tnum")
							if tb.Submitted() || (!tb.Focused() && strings.TrimSpace(*sBuf) != strconv.Itoa(slots)) {
								if sn, err := strconv.Atoi(strings.TrimSpace(*sBuf)); err == nil && sn >= 1 {
									v.emitAnyModuleEdit(moduleID, map[string]any{"slots_per_cell": sn})
								}
							}
						})
					}
				})
			}
			if mediaHint != "" {
				ui.Text(c, "采集要求："+mediaHint).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			}
			if scope == "container_grid" {
				if pl, _ := v.findPlate(containerRef); pl != nil {
					if geom, ok := geometryForPlate(pl.Format, pl.Topology, pl.Rows, pl.Columns); ok {
						countsByWell := map[string]int{}
						for _, th := range thumbs {
							if th.WellID != "" {
								countsByWell[strings.ToUpper(th.WellID)]++
							}
						}
						wellGroups := pl.WellGroups()
						var blankSet, excludedSet map[string]bool
						blankCount, exclCount := 0, 0
						if v.Doc != nil {
							if g := BuildSciGraph(v.Doc); g != nil {
								if sc := g.ContainerByID(containerRef); sc != nil {
									blankSet = sc.BlankSet
									blankCount = sc.BlankWells
									excludedSet = sc.ExcludedSet
									exclCount = sc.ExcludedWells
								}
							}
						}
						var previewWells []string
						for r := 0; r < geom.Rows; r++ {
							for col := 0; col < geom.Columns; col++ {
								wID := fmt.Sprintf("%s%d", rowLabel(r), col+1)
								if ((blankSet != nil && blankSet[wID]) || (excludedSet != nil && excludedSet[wID])) && countsByWell[wID] == 0 {
									continue
								}
								if wellGroups[wID] != "" || countsByWell[wID] > 0 {
									previewWells = append(previewWells, wID)
								}
							}
						}
						if len(previewWells) == 0 {
							for r := 0; r < geom.Rows && len(previewWells) < 8; r++ {
								for col := 0; col < geom.Columns && len(previewWells) < 8; col++ {
									previewWells = append(previewWells, fmt.Sprintf("%s%d", rowLabel(r), col+1))
								}
							}
						}
						if len(previewWells) > 12 {
							previewWells = previewWells[:12]
						}
						if blankCount > 0 || exclCount > 0 {
							var exclParts []string
							if blankCount > 0 {
								exclParts = append(exclParts, fmt.Sprintf("%d 个空白/PBS 边缘孔", blankCount))
							}
							if exclCount > 0 {
								exclParts = append(exclParts, fmt.Sprintf("%d 个异常孔", exclCount))
							}
							ui.Text(c, fmt.Sprintf("有效实验孔 %d 孔（已自动排除 %s）", len(previewWells), strings.Join(exclParts, "、"))).
								FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						}
						ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							for _, wID := range previewWells {
								wID := wID
								nCnt := countsByWell[wID]
								label := fmt.Sprintf("%s %d/%d", wID, nCnt, slots)
								bg := tok.Elevated
								fg := tok.TextMuted
								if nCnt >= slots {
									bg = tok.SuccessBg
									fg = tok.Success
								} else if nCnt > 0 {
									bg = tok.AccentSoftBg
									fg = tok.Accent
								}
								if v.Mode == ModeRun && !v.ReadOnly && v.OnLinkAsset != nil {
									pill := ui.ButtonBase(c).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
										Radius(float32(tok.ControlRadius)).Background(bg).Border(1, tok.Border).Label(label)
									pill.Children(func() {
										ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(fg).FontFeatures("tnum")
									})
									if pill.Clicked() {
										v.OnLinkAsset(v.StepID, moduleID, wID)
									}
								} else {
									ui.Row(c).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
										Radius(float32(tok.ControlRadius)).Background(bg).Border(1, tok.Border).Children(func() {
										ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(fg).FontFeatures("tnum")
									})
								}
							}
						})
					}
				}
			}
			if len(thumbs) > 0 {
				ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing8)).Children(func() {
					for _, th := range thumbs {
						th := th
						resolvedPath := resolveFigurePath(v.BaseDir, th.FilePath)
						thumbBtn := ui.ButtonBase(c).Width(float32(tok.Spacing48)*2.7).Padding(float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).Children(func() {
							ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
								if resolvedPath != "" {
									if imgSrc, errText := v.cachedImageFile(resolvedPath); errText == "" && imgSrc != nil {
										ui.Image(c, imgSrc).Height(float32(tok.Spacing48) * 1.5)
									} else {
										ui.Row(c).FillWidth().Height(float32(tok.Spacing48)*1.5).Center().
											Radius(float32(tok.ControlRadius)).Background(tok.Surface).Children(func() {
											ui.Text(c, firstNonEmpty(errText, "图片不可用")).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
										})
									}
								}
								lbl := firstNonEmpty(th.Label, th.AssetID)
								if th.WellID != "" {
									lbl = fmt.Sprintf("%s #%d · %s", th.WellID, th.FovIndex, lbl)
								}
								ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text).FontFeatures("tnum")
							})
						})
						if thumbBtn.Clicked() {
							cp := th
							v.ExpandedAsset = &cp
							v.AssetZoom = 1
							v.assetOpen = true
						}
					}
				})
			} else {
				emptyMsg := "Run 执行时可关联原始培养皿照片、显微视野或文件"
				if scope == "container_grid" {
					emptyMsg = fmt.Sprintf("计划每孔 %d 个采图槽位 · 当前 0 个已关联", slots)
				}
				ui.Text(c, emptyMsg).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
			}
			if !v.ReadOnly && v.UnifiedLive && v.OnPickAttachmentFile != nil {
				if components.Button(c, "+ 挂载图片 / 原始文件", components.ButtonSecondary).Clicked() {
					v.OnPickAttachmentFile(v.StepID, moduleID)
				}
			} else if v.Mode == ModeRun && !v.ReadOnly && v.OnLinkAsset != nil {
				if iconButton(c, lucide.Paperclip(), "关联图片 / 文件", components.ButtonSecondary).Clicked() {
					v.OnLinkAsset(v.StepID, moduleID, "")
				}
			}
			if v.assetOpen && v.ExpandedAsset != nil {
				exp := *v.ExpandedAsset
				if v.AssetZoom <= 0 {
					v.AssetZoom = 1
				}
				ui.Box(c.Key("asset-dlg:"+moduleID)).Children(func() {
					components.Dialog(c, &v.assetOpen, components.DialogOptions{Label: firstNonEmpty(exp.Label, exp.AssetID)}, func(panel ui.Element) {
					panel.Width(float32(tok.Spacing48)*14).Gap(float32(tok.Spacing12)).Children(func() {
						ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
							ui.Text(c, firstNonEmpty(exp.Label, exp.AssetID)).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
							if exp.WellID != "" {
								components.Badge(c, fmt.Sprintf("%s #%d", exp.WellID, exp.FovIndex))
							}
							ui.Spacer(c)
							for _, z := range []float32{1.0, 1.5, 2.0} {
								z := z
								style := components.ButtonOutline
								if v.AssetZoom == z {
									style = components.ButtonSecondary
								}
								if components.Button(c, fmt.Sprintf("%d%%", int(z*100)), style).Clicked() {
									v.AssetZoom = z
								}
							}
							if v.OnOpenExternalAsset != nil {
								if components.Button(c, "系统打开", components.ButtonOutline).Clicked() {
									v.OnOpenExternalAsset(exp)
								}
							}
							if components.Button(c, "关闭", components.ButtonOutline).Clicked() {
								v.assetOpen = false
							}
						})
						if resolvedExp := resolveFigurePath(v.BaseDir, exp.FilePath); resolvedExp != "" {
							imgSrc, errText := v.cachedImageFile(resolvedExp)
							if errText == "" && imgSrc != nil {
								ui.ScrollBoth(c).FillWidth().MaxHeight(460).Children(func() {
									ui.Image(c, imgSrc).Height(300 * v.AssetZoom)
								})
							} else {
								ui.Text(c, "文件预览不可用："+firstNonEmpty(errText, resolvedExp)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
							}
						}
						if exp.AssetID != "" {
							ui.Text(c, fmt.Sprintf("资产 ID: %s · 路径: %s", exp.AssetID, exp.FilePath)).
								FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						}
					})
				})
				})
			}
		})
	})
}

func (v *View) renderSubflow(c *ui.Context, data map[string]any) {
	tok := theme.GetTokens(c.Theme())
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "子流程")
	items := mapSlice(data["items"])
	collapsedDefault, _ := data["collapsed"].(bool)
	openPtr := v.ensureWidgetBoolInit("sf:"+moduleID, !collapsedDefault)
	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)

	moduleCardFrame(v, c, lucide.GitBranch(), title, fmt.Sprintf("%d 项", len(items)), nil, func() {
		components.Collapsible(c, fmt.Sprintf("展开子步骤明细（%d）", len(items)), openPtr, components.CollapsibleOptions{}, func() {
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Padding(float32(tok.Spacing4), 0, 0, 0).Children(func() {
				for idx, it := range items {
					idx := idx
					sid := textValue(it, "id")
					stitle := textValue(it, "title")
					snote := textValue(it, "note")
					doneInline, _ := it["done"].(bool)
					donePtr := v.ensureWidgetBool("sf_chk:" + moduleID + ":" + sid)
					if v.UnifiedLive {
						*donePtr = doneInline || v.CheckedItems[sid] || v.CheckedItems[v.StepID+":"+sid]
					}
					ui.Row(c.Key("sf-row:"+moduleID+":"+sid)).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
						AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Transition(theme.AccordionTransition()).Children(func() {
						ui.Text(c, fmt.Sprintf("%d.", idx+1)).FontSize(float32(tok.TextMeta)).Bold().
							TextColor(tok.Accent).FontFeatures("tnum")
						if (v.UnifiedLive || v.Mode == ModeRun) && !v.ReadOnly {
							if components.Checkbox(c, donePtr, stitle).Changed() {
								if v.UnifiedLive {
									next := cloneRowsSlice(items)
									if idx < len(next) {
										next[idx]["done"] = *donePtr
										v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
									}
								}
								if v.OnToggleCheck != nil {
									v.OnToggleCheck(v.StepID, sid, *donePtr)
								}
							}
						} else {
							ui.Text(c, stitle).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
						}
						if snote != "" {
							ui.Spacer(c)
							ui.Text(c, snote).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						} else if editableDesign {
							ui.Spacer(c)
						}
						if editableDesign && len(items) > 1 {
							if components.IconButton(c, lucide.Trash(), "删除子步骤", components.IconButtonCompact).Clicked() {
								next := removeSliceIndex(items, idx)
								v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
							}
						}
					})
				}
				if editableDesign {
					newTitle := v.ensureWidgetDraft("sf:"+moduleID+":new_title", "")
					newNote := v.ensureWidgetDraft("sf:"+moduleID+":new_note", "")
					ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing4), 0, 0, 0).
						AlignItems(ui.Center).Children(func() {
						ui.Box(c).FillWidth().Children(func() {
							components.TextBox(c, newTitle, components.TextBoxOptions{}).Placeholder("新增子步骤描述…")
						})
						ui.Box(c).Width(float32(tok.Spacing48)*2.5).Children(func() {
							components.TextBox(c, newNote, components.TextBoxOptions{}).Placeholder("备注（可选）")
						})
						if components.Button(c, "添加子步", components.ButtonSecondary).Clicked() && strings.TrimSpace(*newTitle) != "" {
							next := append([]map[string]any{}, items...)
							entry := map[string]any{
								"id":    fmt.Sprintf("sf-%s-%d", sanitizeIDFrag(*newTitle), len(items)+1),
								"title": strings.TrimSpace(*newTitle),
							}
							if strings.TrimSpace(*newNote) != "" {
								entry["note"] = strings.TrimSpace(*newNote)
							}
							next = append(next, entry)
							v.emitAnyModuleEdit(moduleID, map[string]any{"items": next})
							*newTitle = ""
							*newNote = ""
						}
					})
				}
			})
		})
	})
}
