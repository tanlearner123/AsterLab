package shell

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/vault"
)

func (s *Shell) ensureRightDrawerDefaults() {
	if s.RightTab < 0 || s.RightTab > 2 {
		s.RightTab = 0
	}
	if s.RightCalc.Stock == "" {
		s.RightCalc = RightCalcState{
			Mode:       0,
			Stock:      "100",
			StockUnit:  "ug/mL",
			Target:     "50",
			TargetUnit: "ng/mL",
			FinalVol:   "4",
			VolUnit:    "mL",
			MolarMass:  "1449.25",
		}
	}
}

func (s *Shell) rightDrawerHeader(c *ui.Context) {
	tok := theme.GetTokens(c.Theme())
	s.ensureRightDrawerDefaults()
	titles := []string{"步骤大纲", "速算与计时", "关联与诊断"}
	components.Text(c, titles[s.RightTab]).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
}

func (s *Shell) rightDrawer(c *ui.Context) {
	tok := theme.GetTokens(c.Theme())
	s.ensureRightDrawerDefaults()
	var topInset float32
	if s.window != nil {
		topInset = sidebar.HeaderHeight(c)
	}

	drawerCol := ui.Column(c).Fill().Padding(topInset, 0, 0, 0)
	if !c.Vibrancy() {
		drawerCol.Background(tok.Surface.Alpha(0.92))
	}
	drawerCol.Children(func() {
		ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Children(func() {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing4)).
				Radius(float32(tok.ControlRadius)).Background(tok.Elevated).
				Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
				ariaLabels := []string{"步骤大纲", "速算与计时", "关联与诊断"}
				for idx, lbl := range []string{"大纲", "速算计时", "关联诊断"} {
					idx, lbl := idx, lbl
					active := s.RightTab == idx
					btn := ui.ButtonBase(c.Key(fmt.Sprintf("rd-tab-%d", idx))).
						Grow(1).MinWidth(0).Center().
						Padding(float32(tok.Spacing4), float32(tok.Spacing4)).
						Radius(float32(tok.ControlRadius)).Label(ariaLabels[idx])
					fg := tok.TextMuted
					if active {
						btn.Background(tok.Surface).Border(1, tok.Border.Alpha(0.75))
						fg = tok.Text
					} else if btn.Hovered() {
						btn.Background(tok.Text.Alpha(0.04))
						fg = tok.Text
					}
					btn.Children(func() {
						ui.Text(c, lbl).FontSize(float32(tok.TextData)).Bold().TextColor(fg).SingleLine().Ellipsis("…")
					})
					if btn.Clicked() {
						s.RightTab = idx
					}
				}
			})
		})
		ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.5))

		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).Gap(float32(tok.Spacing4)).Children(func() {
				switch s.RightTab {
				case 0:
					s.renderDrawerOutline(c, tok)
				case 1:
					s.renderDrawerTimerAndCalc(c, tok)
				case 2:
					s.renderDrawerLinksAndDiagnostics(c, tok)
				}
			})
		})
	})
}

func (s *Shell) renderDrawerOutline(c *ui.Context, tok theme.Tokens) {
	if len(s.OutlineSteps) == 0 {
		components.Text(c, "打开实验文件后，此处显示文档大纲与步骤以便点击跳转。").
			FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		return
	}
	// Compact segmented filter: [ 全部 | 仅步骤 ]
	hasSteps := false
	hasHeadings := false
	for _, it := range s.OutlineSteps {
		if it.IsStep {
			hasSteps = true
		} else {
			hasHeadings = true
		}
	}
	if hasSteps && hasHeadings {
		ui.Row(c).FillWidth().Padding(0, 0, float32(tok.Spacing8), 0).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			for idx, lbl := range []string{"全部", "仅步骤"} {
				idx, lbl := idx, lbl
				active := (idx == 1) == s.OutlineOnlySteps
				pill := ui.ButtonBase(c.Key(fmt.Sprintf("rd-ol-flt-%d", idx))).
					Height(float32(tok.Spacing24)).Padding(0, float32(tok.Spacing8)).
					AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label("大纲筛选：" + lbl)
				fg := tok.TextMuted
				if active {
					pill.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.32))
					fg = tok.Accent
				} else if pill.Hovered() {
					pill.Background(tok.Text.Alpha(0.04))
					fg = tok.Text
				}
				pill.Children(func() {
					ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg).SingleLine()
				})
				if pill.Clicked() {
					s.OutlineOnlySteps = idx == 1
				}
			}
		})
	}

	for _, item := range s.OutlineSteps {
		item := item
		if s.OutlineOnlySteps && !item.IsStep {
			continue
		}
		depth := item.Depth
		if s.OutlineOnlySteps {
			depth = item.StepDepth
		}
		fg := tok.TextMuted
		if item.Active {
			fg = tok.Accent
		} else if !item.IsStep && depth == 0 {
			fg = tok.Text
		} else if item.IsStep && item.StepDepth == 0 {
			fg = tok.Text
		}
		leftPad := float32(tok.Spacing8) + float32(min(depth, 4))*float32(tok.Spacing12)
		btn := ui.ButtonBase(c.Key("rd-step-"+item.ID)).FillWidth().Justify(ui.Start).
			Padding(float32(tok.Spacing4)*1.25, float32(tok.Spacing8), float32(tok.Spacing4)*1.25, leftPad).
			Radius(float32(tok.ControlRadius)).Label("跳至 " + item.Title)
		bg := ui.Transparent
		if item.Active {
			bg = tok.AccentSoftBg
		} else if btn.Hovered() {
			bg = tok.Text.Alpha(0.045)
			fg = tok.Text
		}
		btn.Background(btn.AnimateColor("rd-bg", bg, theme.MotionMicroFade))
		theme.ApplyTactilePress(btn)
		btn.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				if item.IsStep {
					if item.StepDepth == 0 {
						// Mainline step mini badge
						ui.Row(c).Height(float32(tok.Spacing16)+2).Padding(0, float32(tok.Spacing4)*1.2).
							AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).
							Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.28)).Shrink(0).Children(func() {
							ui.Text(c, item.Number).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
						})
					} else {
						// Sub-step quiet tabular number
						ui.Text(c, item.Number).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted).FontFeatures("tnum").Shrink(0)
					}
				} else if item.Active {
					ui.Box(c).Width(2).Height(float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).Background(tok.Accent)
				}
				titleTxt := components.Text(c, item.Title).FontSize(float32(tok.TextData)).TextColor(fg).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
				if !item.IsStep && depth == 0 {
					titleTxt.Bold()
				}
				if item.Meta != "" {
					ui.Text(c, item.Meta).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").Shrink(0)
				}
			})
		})
		if btn.Clicked() && s.OnSelectOutlineStep != nil {
			s.OnSelectOutlineStep(item.ID)
		}
	}
}

func (s *Shell) renderDrawerTimerAndCalc(c *ui.Context, tok theme.Tokens) {
	if len(s.VaultTimers) > 0 {
		components.Text(c, "Vault 全局计时器").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
		now := time.Now()
		for _, vt := range s.VaultTimers {
			vt := vt
			ui.Row(c.Key("rd-vt-"+vt.RelPath+"-"+vt.TimerID)).FillWidth().Padding(float32(tok.Spacing8)).
				Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(.25)).
				AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.Timer()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					components.Text(c, fmt.Sprintf("%s · %s", firstNonEmptyStr(vt.DocTitle, vt.RelPath), vt.Title)).
						FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
					components.Text(c, formatVaultTimerClock(vt, now)).
						FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).FontFeatures("tnum")
				})
				if s.OnOpenSearchHit != nil && components.Button(c, "打开", components.ButtonGhost).Clicked() {
					s.OnOpenSearchHit(vt.RelPath, "")
				}
			})
		}
		ui.Divider(c)
	}
	if len(s.ActiveTimers) > 0 {
		components.Text(c, "运行中计时器").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
		for _, tm := range s.ActiveTimers {
			tm := tm
			ui.Row(c.Key("rd-tm-"+tm.TimerID)).FillWidth().Padding(float32(tok.Spacing8)).
				Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(.25)).
				AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.Timer()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
				ui.Column(c).Grow(1).MinWidth(0).Children(func() {
					components.Text(c, tm.RunTitle).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
					components.Text(c, formatTimerClock(tm)).FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).FontFeatures("tnum")
				})
				if s.OnTimerSelect != nil && components.Button(c, "跳转", components.ButtonGhost).Clicked() {
					s.OnTimerSelect(tm.RunID)
				}
			})
		}
		ui.Divider(c)
	}
	s.renderDrawerQuickCalc(c, tok)
}

func formatVaultTimerClock(vt vault.VaultTimerEntry, now time.Time) string {
	elapsed := vt.ElapsedSec
	if vt.Status == "running" && vt.StartedAt != "" {
		if started, err := time.Parse(time.RFC3339, vt.StartedAt); err == nil {
			if delta := int(now.Sub(started).Seconds()); delta > elapsed {
				elapsed = delta
			}
		}
	}
	if elapsed < 0 {
		elapsed = 0
	}
	formatSec := func(sec int) string {
		h := sec / 3600
		m := (sec % 3600) / 60
		s := sec % 60
		if h > 0 {
			return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
		}
		return fmt.Sprintf("%02d:%02d", m, s)
	}
	stateLbl := "暂停"
	if vt.Status == "running" {
		stateLbl = "进行中"
	}
	if vt.Mode == "countdown" && vt.PlannedSec > 0 {
		rem := vt.PlannedSec - elapsed
		if rem < 0 {
			rem = 0
		}
		return fmt.Sprintf("%s · 剩余 %s", stateLbl, formatSec(rem))
	}
	return fmt.Sprintf("%s · 已用 %s", stateLbl, formatSec(elapsed))
}

func (s *Shell) renderDrawerLinksAndDiagnostics(c *ui.Context, tok theme.Tokens) {
	ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			components.Text(c, "反向链接与复现谱系").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
			ui.Spacer(c)
			components.Badge(c, fmt.Sprintf("%d", len(s.CurrentBacklinks)))
		})
		if len(s.CurrentBacklinks) == 0 {
			components.Text(c, "暂无其他实验通过 [[双链]] 或 repeat_of 引用本文档。").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		} else {
			for i, ref := range s.CurrentBacklinks {
				ref := ref
				kindLbl := "双链引用"
				if ref.Kind == "repeat_of" {
					kindLbl = "复现引用"
				}
				row := ui.Row(c.Key(fmt.Sprintf("rd-link-%d-%s", i, ref.SourceRel))).FillWidth().
					Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
					Background(tok.Elevated).AlignItems(ui.Center).Gap(float32(tok.Spacing8))
				theme.ApplyTactilePress(row)
				row.Children(func() {
					ui.Icon(c, lucide.Link()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
					ui.Column(c).Grow(1).MinWidth(0).Children(func() {
						components.Text(c, firstNonEmptyStr(ref.SourceTitle, ref.SourceRel)).
							FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
						sub := kindLbl
						if ref.SourceDate != "" {
							sub += " · " + ref.SourceDate
						}
						components.Text(c, sub).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
					})
				})
				if row.Clicked() && s.OnOpenSearchHit != nil {
					s.OnOpenSearchHit(ref.SourceRel, "")
				}
			}
		}

		ui.Divider(c)

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			components.Text(c, "科学诊断与校验 (AsterCore)").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
			ui.Spacer(c)
			components.Badge(c, fmt.Sprintf("%d", len(s.CurrentDiagnostics)))
		})
		if len(s.CurrentDiagnostics) == 0 {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
				Background(tok.Elevated).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.CircleCheck()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Success)
				components.Text(c, "未发现结构阻断或 ARRIVE 对照缺失。").FontSize(float32(tok.TextMeta)).TextColor(tok.Success)
			})
			return
		}
		for i, diag := range s.CurrentDiagnostics {
			diag := diag
			sevLbl := "建议"
			sevCol := tok.Accent
			switch diag.Severity {
			case "blocking":
				sevLbl = "阻断"
				sevCol = tok.Danger
			case "needs_confirmation":
				sevLbl = "待确认"
				sevCol = tok.Warning
			}
			ui.Column(c.Key(fmt.Sprintf("rd-diag-%d-%s", i, diag.Code))).FillWidth().
				Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
				Background(tok.Elevated).Border(1, sevCol.Alpha(0.32)).Gap(float32(tok.Spacing4)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, sevLbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(sevCol)
					if diag.Code != "" {
						ui.Text(c, diag.Code).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					}
				})
				components.Text(c, diag.Message).FontSize(float32(tok.TextData)).TextColor(tok.Text)
				if diag.NextAction != "" {
					components.Text(c, "建议: "+diag.NextAction).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				}
			})
		}
	})
}

func (s *Shell) renderDrawerQuickCalc(c *ui.Context, tok theme.Tokens) {
	ui.Row(c).FillWidth().Padding(float32(tok.Spacing4)).
		Radius(float32(tok.ControlRadius)).Background(tok.Elevated).
		Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
		for idx, lbl := range []string{"C1·V1 稀释", "摩尔称量"} {
			idx, lbl := idx, lbl
			active := s.RightCalc.Mode == idx
			btn := ui.ButtonBase(c.Key(fmt.Sprintf("rd-calc-%d", idx))).
				Grow(1).MinWidth(0).Center().
				Padding(float32(tok.Spacing4), float32(tok.Spacing4)).
				Radius(float32(tok.ControlRadius)).Label(lbl)
			fg := tok.TextMuted
			if active {
				btn.Background(tok.Surface).Border(1, tok.Border.Alpha(0.75))
				fg = tok.Text
			} else if btn.Hovered() {
				btn.Background(tok.Text.Alpha(0.04))
				fg = tok.Text
			}
			btn.Children(func() {
				ui.Text(c, lbl).FontSize(float32(tok.TextData)).Bold().TextColor(fg).SingleLine().Ellipsis("…")
			})
			if btn.Clicked() {
				s.RightCalc.Mode = idx
			}
		}
	})

	if s.RightCalc.Mode == 0 {
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			drawerLabeledInput(c, tok, "母液浓度 (C1)", &s.RightCalc.Stock, &s.RightCalc.StockUnit, []string{"mg/mL", "ug/mL", "M", "mM", "uM"})
			drawerLabeledInput(c, tok, "目标浓度 (C2)", &s.RightCalc.Target, &s.RightCalc.TargetUnit, []string{"ug/mL", "ng/mL", "mM", "uM", "nM"})
			drawerLabeledInput(c, tok, "配置总体积 (V2)", &s.RightCalc.FinalVol, &s.RightCalc.VolUnit, []string{"mL", "uL"})

			v1UL, diluentUL, msg := computeQuickDilution(s.RightCalc)
			ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
				Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(.3)).Gap(float32(tok.Spacing4)).Children(func() {
				if msg != "" {
					components.Text(c, msg).FontSize(float32(tok.TextMeta)).TextColor(tok.Danger)
					return
				}
				ui.Text(c, fmt.Sprintf("取母液 V1 = %.2f μL", v1UL)).FontSize(float32(tok.TextSection)).Bold().
					TextColor(tok.Accent).FontFeatures("tnum").ContentTransition(theme.NumericRoll(false))
				ui.Text(c, fmt.Sprintf("补加稀释液 = %.2f μL", diluentUL)).FontSize(float32(tok.TextMeta)).
					TextColor(tok.Text).FontFeatures("tnum").ContentTransition(theme.NumericRoll(false))
			})
		})
		return
	}

	ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
		drawerLabeledInput(c, tok, "目标浓度", &s.RightCalc.Target, &s.RightCalc.TargetUnit, []string{"M", "mM", "uM"})
		drawerLabeledInput(c, tok, "配置体积", &s.RightCalc.FinalVol, &s.RightCalc.VolUnit, []string{"mL", "uL"})
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
			ui.Text(c, "分子量 MW (g/mol)").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			components.TextBox(c, &s.RightCalc.MolarMass, components.TextBoxOptions{}).Placeholder("例如 1449.25")
		})
		massMg, msg := computeQuickMolarMass(s.RightCalc)
		ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
			Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(.3)).Gap(float32(tok.Spacing4)).Children(func() {
			if msg != "" {
				components.Text(c, msg).FontSize(float32(tok.TextMeta)).TextColor(tok.Danger)
				return
			}
			ui.Text(c, fmt.Sprintf("需称量溶质 = %.3f mg", massMg)).FontSize(float32(tok.TextSection)).Bold().
				TextColor(tok.Accent).FontFeatures("tnum").ContentTransition(theme.NumericRoll(false))
		})
	})
}

func drawerLabeledInput(c *ui.Context, tok theme.Tokens, label string, val *string, unit *string, units []string) {
	ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
		ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		ui.Row(c).FillWidth().Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Grow(1).MinWidth(0).Children(func() {
				components.TextBox(c, val, components.TextBoxOptions{}).FontFeatures("tnum")
			})
			if unit != nil && len(units) > 0 {
				sel := components.Select(c, unit)
				sel.Trigger.Children(func() { components.Text(c, *unit).SingleLine() })
				sel.Popup(func(panel ui.Element) {
					for _, opt := range units {
						item := sel.Item(opt)
						item.Children(func() { components.Text(c, opt).SingleLine() })
					}
				})
			}
		})
	})
}

func quickConcUnitBase(unit string) (family string, factor float64) {
	switch strings.TrimSpace(unit) {
	case "M":
		return "molar", 1
	case "mM":
		return "molar", 1e-3
	case "uM", "µM":
		return "molar", 1e-6
	case "nM":
		return "molar", 1e-9
	case "mg/mL":
		return "mass", 1
	case "ug/mL", "µg/mL":
		return "mass", 1e-3
	case "ng/mL":
		return "mass", 1e-6
	default:
		return "", 0
	}
}

func computeQuickDilution(st RightCalcState) (v1UL, dilUL float64, errMsg string) {
	c1, err1 := strconv.ParseFloat(strings.TrimSpace(st.Stock), 64)
	c2, err2 := strconv.ParseFloat(strings.TrimSpace(st.Target), 64)
	v2, err3 := strconv.ParseFloat(strings.TrimSpace(st.FinalVol), 64)
	if err1 != nil || err2 != nil || err3 != nil || c1 <= 0 || c2 <= 0 || v2 <= 0 {
		return 0, 0, "请输入大于 0 的有效数值"
	}
	fam1, f1 := quickConcUnitBase(st.StockUnit)
	fam2, f2 := quickConcUnitBase(st.TargetUnit)
	if fam1 == "" || fam1 != fam2 {
		return 0, 0, "C1 与 C2 浓度单位量纲不一致"
	}
	v2UL := v2
	if st.VolUnit == "mL" || st.VolUnit == "" {
		v2UL = v2 * 1000.0
	}
	v1UL = v2UL * (c2 * f2) / (c1 * f1)
	if v1UL > v2UL || math.IsNaN(v1UL) || math.IsInf(v1UL, 0) {
		return 0, 0, "母液浓度低于目标浓度，无法稀释"
	}
	return v1UL, v2UL - v1UL, ""
}

func computeQuickMolarMass(st RightCalcState) (massMg float64, errMsg string) {
	c2, err1 := strconv.ParseFloat(strings.TrimSpace(st.Target), 64)
	v2, err2 := strconv.ParseFloat(strings.TrimSpace(st.FinalVol), 64)
	mw, err3 := strconv.ParseFloat(strings.TrimSpace(st.MolarMass), 64)
	if err1 != nil || err2 != nil || err3 != nil || c2 <= 0 || v2 <= 0 || mw <= 0 {
		return 0, "请输入有效浓度、体积和分子量"
	}
	fam, f := quickConcUnitBase(st.TargetUnit)
	if fam != "molar" {
		return 0, "称量模式请选择 M / mM / uM 浓度单位"
	}
	volL := v2 * 1e-3
	if st.VolUnit == "uL" {
		volL = v2 * 1e-6
	}
	grams := (c2 * f) * volL * mw
	return grams * 1000.0, ""
}
