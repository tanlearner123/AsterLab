package asterdoc

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
	plateui "asterlab-mygo-prototype/internal/uikit/plate"
)

// renderEmbedCard renders a standalone image embed segment `![[assets/x.png]]{@plate-1:A1 6h fov=1}`
// with: (1) crisp image preview + zoom dialog, (2) interactive relation chips driven by the
// single InlineRelation AST, (3) unified Hover Card showing plate/well group & factors,
// (4) `+ 关联` popover (works in both ModeEdit and ModeLab), and (5) inline source reveal toggle.
func (v *View) renderEmbedCard(c *ui.Context, seg DocSegment, resolvedBySpan map[int]ResolvedRelation, canEditSource, canEditRelation bool) {
	if seg.Embed == nil {
		return
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	dark := t != nil && t.Dark
	emb := *seg.Embed
	cardKey := fmt.Sprintf("embed-%d-%s", seg.Start, emb.Asset)
	srcEditOpen := v.ensureWidgetBool("emb_src:" + cardKey)
	linkPopOpen := v.ensureWidgetBool("emb_link:" + cardKey)

	rr, hasResolved := resolvedBySpan[emb.SpanStart]
	if !hasResolved {
		rr = ResolvedRelation{InlineRelation: emb, SourceKind: "inline"}
	}

	// If user toggled raw inline syntax edit on this embed line, show single-line LiveMarkdownEditor
	if *srcEditOpen && canEditSource {
		ui.Column(c.Key("emb-raw-wrap:" + cardKey)).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
			v.renderLiveMarkdownSegment(c, "emb-raw:"+cardKey, strings.TrimRight(seg.Text, "\r\n"), seg.Start, false, false, "![[./assets/image.png]]{@plate-1:A1}", "", func(nextLine string) {
				if v.Doc == nil || v.OnChange == nil {
					return
				}
				suffix := ""
				if strings.HasSuffix(seg.Text, "\n") {
					suffix = "\n"
				}
				if nextSrc, err := ReplaceByteRange(v.Doc.Source, seg.Start, seg.End, nextLine+suffix); err == nil {
					v.OnChange(nextSrc)
				}
			})
			ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
				if components.Button(c, "完成编辑", components.ButtonGhost).Clicked() {
					*srcEditOpen = false
				}
			})
		})
		return
	}

	idxMap := plateGroupPaletteMap(v.Doc, nil)
	metaSummary := "普通插图"
	if len(rr.Contexts) > 0 {
		metaSummary = fmt.Sprintf("已关联 %d 处对象", len(rr.Contexts))
	}
	card := ui.Column(c.Key("emb-card:" + cardKey)).FillWidth().
		Padding(float32(tok.Spacing12), float32(tok.Spacing16)).Radius(float32(tok.PanelRadius)).
		Background(tok.Surface).Border(1, tok.Border).Gap(float32(tok.Spacing12))

	cardHover := card.Hovered()
	card.Children(func() {
		// Unified Module Header Row (matches group_table / plate / param_table)
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.Image()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
				ui.Text(c, firstNonEmpty(emb.Label, filepath.Base(emb.Asset))).
					FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
				ui.Spacer(c)
				ui.Text(c, metaSummary).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").SingleLine()
				if canEditSource && (cardHover || v.MobileCompact) {
					if components.IconButton(c, lucide.Code(), "编辑行内关联语法", components.IconButtonCompact).Clicked() {
						*srcEditOpen = true
					}
				}
				if components.IconButton(c, lucide.Maximize2(), "放大查看", components.IconButtonCompact).Clicked() {
					v.ExpandedFigureID = cardKey
					v.FigureZoom = 1
					v.figureOpen = true
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.55))
		})

		// 1. Image canvas (reuses unified async figure loader & zoom dialog)
		maxH := float32(260)
		if wInt, err := strconv.Atoi(emb.Width); err == nil && wInt >= 80 && wInt <= 600 {
			maxH = float32(wInt) * 0.75
		}
		v.drawFigureCanvas(c, cardKey, emb.Asset, "", nil, maxH, 1)

		// 2. Interactive relation chips strip + unified hover card + `+ 关联` editor
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			if len(rr.Contexts) > 0 {
				for ci, tc := range rr.Contexts {
					ci, tc := ci, tc
					chipKey := fmt.Sprintf("emb-chip:%s:%d", cardKey, ci)
					chipHoverOpen := v.ensureWidgetBool("emb_hov:" + chipKey)
					lbl := firstNonEmpty(tc.ObjectTitle, tc.ObjectID)
					if tc.SubTarget != "" {
						lbl = firstNonEmpty(tc.ObjectTitle, tc.ObjectID) + " · " + tc.SubTarget
					}
					if tc.GroupName != "" {
						lbl += " (" + tc.GroupName + ")"
					}
					bg := tok.AccentSoftBg
					fg := tok.Accent
					bd := tok.Accent.Alpha(0.35)
					if !tc.Valid {
						bg = tok.Warning.Alpha(0.14)
						fg = tok.Warning
						bd = tok.Warning.Alpha(0.55)
						lbl = "⚠ " + lbl
					}
					chip := ui.ButtonBase(c.Key(chipKey)).
						Height(float32(tok.Spacing24)).
						Padding(0, float32(tok.Spacing8)).
						AlignItems(ui.Center).
						AlignSelf(ui.Center).
						Radius(float32(tok.ControlRadius)).
						Background(bg).Border(1, bd).Label(lbl)
					chip.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							if tc.GroupID != "" && tc.Valid {
								ui.Box(c).Size(float32(tok.Spacing8), float32(tok.Spacing8)).
									Radius(float32(tok.ControlRadius)).
									Background(plateui.GroupColorIndexed(tc.GroupID, idxMap, dark))
							}
							ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg).SingleLine()
						})
					})
					if chip.Clicked() {
						if tc.Valid {
							v.focusRelationTarget(tc.ObjectID, tc.SubTarget)
						}
						*chipHoverOpen = !*chipHoverOpen
					}
					components.Popover(c.Key("emb-hov-pop:"+chipKey), chip, chipHoverOpen, func(panel ui.Element) {
						panel.Padding(float32(tok.Spacing12)).Children(func() {
							v.renderRelationHoverContent(c, tok, rr)
						})
					})
				}
			} else if !emb.HasRelation() {
				ui.Text(c, "普通插图（未建立科学关联）").
					FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).AlignSelf(ui.Center)
			}

			if emb.Timepoint != "" {
				inlineBadge(c, emb.Timepoint)
			}
			if emb.FOV > 0 {
				inlineBadge(c, fmt.Sprintf("FOV %d", emb.FOV))
			}
			if emb.Role != "" {
				inlineBadge(c, relationRoleLabel(emb.Role))
			}

			ui.Spacer(c)

			if canEditRelation {
				linkBtn := ui.ButtonBase(c.Key("emb-link-btn:"+cardKey)).
					Height(float32(tok.Spacing24)).
					Padding(0, float32(tok.Spacing8)).
					AlignItems(ui.Center).
					AlignSelf(ui.Center).
					Radius(float32(tok.ControlRadius)).
					Label("+ 关联")
				if linkBtn.Hovered() {
					linkBtn.Background(tok.Text.Alpha(0.05))
				}
				linkBtn.Children(func() {
					ui.Text(c, "+ 关联").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted).SingleLine()
				})
				if linkBtn.Clicked() {
					*linkPopOpen = !*linkPopOpen
				}
				v.renderRelationEditorPopover(c, tok, cardKey, linkBtn, linkPopOpen, emb)
			}
		})
	})

	// Lightbox zoom dialog
	openFig := v.figureOpen && v.ExpandedFigureID == cardKey
	ui.Box(c.Key("emb-dlg:" + cardKey)).Children(func() {
		components.Dialog(c, &openFig, components.DialogOptions{Label: filepath.Base(emb.Asset)}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*14).Gap(float32(tok.Spacing12)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, filepath.Base(emb.Asset)).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
					if emb.HasRelation() {
						var badgeParts []string
						for _, tg := range emb.Targets {
							tLbl := tg.ObjectID
							if v.Doc != nil {
								for _, m := range v.Doc.Modules {
									if m.ID == tg.ObjectID {
										tLbl = firstNonEmpty(textValue(m.Data, "title"), tg.ObjectID)
										break
									}
								}
							}
							if len(tg.SubTargets) > 0 {
								tLbl += " · " + strings.Join(tg.SubTargets, ", ")
							}
							badgeParts = append(badgeParts, tLbl)
						}
						if emb.Timepoint != "" {
							badgeParts = append(badgeParts, emb.Timepoint)
						}
						if emb.FOV > 0 {
							badgeParts = append(badgeParts, fmt.Sprintf("视野 %d", emb.FOV))
						}
						if emb.Role != "" {
							badgeParts = append(badgeParts, relationRoleLabel(emb.Role))
						}
						components.Badge(c, strings.Join(badgeParts, " · "), components.BadgeOptions{Tone: components.BadgeAccent})
					}
					ui.Spacer(c)
					if components.Button(c, "关闭", components.ButtonOutline).Clicked() {
						openFig = false
					}
				})
				ui.ScrollBoth(c).FillWidth().MaxHeight(460).Children(func() {
					v.drawFigureCanvas(c, cardKey, emb.Asset, "", nil, 360, v.FigureZoom)
				})
			})
		})
	})
	if !openFig && v.ExpandedFigureID == cardKey {
		v.figureOpen = false
	}
}

func relationRoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "obs", "observation", "experimental_observation":
		return "观察图"
	case "raw", "raw_data":
		return "原始数据"
	case "ref", "reference":
		return "参考图"
	default:
		return role
	}
}

func (v *View) renderRelationEditorPopover(c *ui.Context, tok theme.Tokens, key string, anchor ui.Element, open *bool, emb InlineRelation) {
	components.Popover(c.Key("rel-ed-pop:"+key), anchor, open, func(panel ui.Element) {
		panel.Width(float32(tok.Spacing48)*6.5).Padding(float32(tok.Spacing12)).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Text(c, "关联到科学对象").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text)

			// Gather candidate plates / steps / groups in current doc
			var objOptions []string
			var objLabels = map[string]string{}
			if v.Doc != nil {
				flat, _ := flattenModules(v.Doc.Modules)
				for _, m := range flat {
					if m.Kind == "plate" || m.Kind == "step" {
						objOptions = append(objOptions, m.ID)
						objLabels[m.ID] = firstNonEmpty(textValue(m.Data, "title"), m.ID)
					}
				}
				for _, g := range v.Doc.GroupList() {
					objOptions = append(objOptions, g.ID)
					objLabels[g.ID] = "组别: " + firstNonEmpty(g.Name, g.ID)
				}
			}
			initObj := ""
			initWells := ""
			if len(emb.Targets) > 0 {
				initObj = emb.Targets[0].ObjectID
				initWells = strings.Join(emb.Targets[0].SubTargets, ",")
			} else if len(objOptions) > 0 {
				initObj = objOptions[0]
			}
			objBuf := v.ensureWidgetDraft("rel_obj:"+key, initObj)
			wellBuf := v.ensureWidgetDraft("rel_well:"+key, initWells)
			tpBuf := v.ensureWidgetDraft("rel_tp:"+key, emb.Timepoint)
			fovStr := ""
			if emb.FOV > 0 {
				fovStr = strconv.Itoa(emb.FOV)
			}
			fovBuf := v.ensureWidgetDraft("rel_fov:"+key, fovStr)
			roleBuf := v.ensureWidgetDraft("rel_role:"+key, firstNonEmpty(emb.Role, "obs"))

			if len(objOptions) > 0 {
				sel := components.Select(c, objBuf)
				StyleSelectQuiet(c, tok, sel)
				sel.Trigger.Children(func() {
					ui.Text(c, firstNonEmpty(objLabels[*objBuf], *objBuf, "选择孔板 / 步骤 / 组别")).
						FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
				})
				sel.Popup(func(p ui.Element) {
					for _, oid := range objOptions {
						oid := oid
						sel.Item(oid).Children(func() {
							ui.Text(c, objLabels[oid]).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
						})
					}
				})
			} else {
				components.TextBox(c, objBuf, components.TextBoxOptions{}).Placeholder("目标对象 ID（如 plate-1）")
			}

			ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					components.TextBox(c, wellBuf, components.TextBoxOptions{}).Placeholder("孔位（如 A1 或 A1,A2）")
				})
				ui.Box(c).Width(float32(tok.Spacing48)*1.6).Children(func() {
					components.TextBox(c, tpBuf, components.TextBoxOptions{}).Placeholder("时间点 6h")
				})
			})
			ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					components.TextBox(c, fovBuf, components.TextBoxOptions{}).Placeholder("视野 FOV（如 1）")
				})
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					roleSel := components.Select(c, roleBuf)
					StyleSelectQuiet(c, tok, roleSel)
					roleSel.Trigger.Children(func() {
						ui.Text(c, firstNonEmpty(relationRoleLabel(*roleBuf), "选择角色")).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
					})
					roleSel.Popup(func(p ui.Element) {
						for _, opt := range []struct{ key, lbl string }{
							{"obs", "观察图"},
							{"raw", "原始数据"},
							{"ref", "参考图"},
						} {
							opt := opt
							roleSel.Item(opt.key).Children(func() {
								ui.Text(c, opt.lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						}
					})
				})
			})

			ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
				if emb.HasRelation() {
					if components.Button(c, "移除关联", components.ButtonGhost).Clicked() {
						if v.Doc != nil && v.OnChange != nil {
							if next, err := SetInlineRelationAtSpan(v.Doc.Source, emb.SpanStart, emb.SpanEnd, InlineRelation{}); err == nil {
								v.OnChange(next)
							}
						}
						*open = false
					}
				}
				ui.Spacer(c)
				if components.Button(c, "保存关联", components.ButtonPrimary).Clicked() {
					nextRel := InlineRelation{
						Timepoint: strings.TrimSpace(*tpBuf),
						Role:      strings.TrimSpace(*roleBuf),
					}
					if f, err := strconv.Atoi(strings.TrimSpace(*fovBuf)); err == nil && f > 0 {
						nextRel.FOV = f
					}
					if oid := strings.TrimSpace(*objBuf); oid != "" {
						var subs []string
						for _, w := range strings.Split(*wellBuf, ",") {
							w = strings.TrimSpace(w)
							if w != "" {
								subs = append(subs, strings.ToUpper(w))
							}
						}
						nextRel.Targets = []RelationTarget{{ObjectID: oid, SubTargets: subs}}
					}
					if v.Doc != nil && v.OnChange != nil {
						if next, err := SetInlineRelationAtSpan(v.Doc.Source, emb.SpanStart, emb.SpanEnd, nextRel); err == nil {
							v.OnChange(next)
						}
					}
					*open = false
				}
			})
		})
	})
}

// renderPlateWellRelationsStrip displays reverse-linked images/attachments for the selected
// well(s) on plate p (grouped by Timepoint × FOV), plus a one-click `+ 为所选孔关联照片` trigger
// that works in both Edit Mode and Lab Mode.
func (v *View) renderPlateWellRelationsStrip(c *ui.Context, tok theme.Tokens, p *Plate, selected []string, allowAttach bool) {
	if v.Doc == nil || p == nil || len(selected) == 0 {
		return
	}
	allRels := ResolveRelations(v.Doc)
	var matched []ResolvedRelation
	seen := map[string]bool{}
	for _, w := range selected {
		for _, r := range RelationsForTarget(allRels, p.ID, w) {
			key := fmt.Sprintf("%s:%d:%s:%d", r.Asset, r.SpanStart, r.Timepoint, r.FOV)
			if !seen[key] {
				seen[key] = true
				matched = append(matched, r)
			}
		}
	}
	if len(matched) == 0 && !allowAttach {
		return
	}
	sort.Slice(matched, func(i, j int) bool {
		if matched[i].Timepoint != matched[j].Timepoint {
			return matched[i].Timepoint < matched[j].Timepoint
		}
		if matched[i].FOV != matched[j].FOV {
			return matched[i].FOV < matched[j].FOV
		}
		return matched[i].Asset < matched[j].Asset
	})

	wellSummary := strings.Join(selected, ", ")
	if len(selected) > 4 {
		wellSummary = fmt.Sprintf("%s 等 %d 孔", strings.Join(selected[:3], ", "), len(selected))
	}

	ui.Column(c.Key("pl-well-rels:"+p.ID)).FillWidth().
		Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		Radius(float32(tok.ControlRadius)).
		Background(tok.Elevated).Border(1, tok.Border).Gap(float32(tok.Spacing8)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Icon(c, lucide.Images()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
			ui.Text(c, fmt.Sprintf("孔位 %s 关联图像 (%d)", wellSummary, len(matched))).
				FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).FontFeatures("tnum")
			ui.Spacer(c)
			if allowAttach && v.OnAttachPhotoToWell != nil {
				if components.Button(c, "+ 拍照 / 关联图片", components.ButtonSecondary).Clicked() {
					v.OnAttachPhotoToWell(p.ID, append([]string(nil), selected...))
				}
			}
		})
		if len(matched) > 0 {
			ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing8)).Children(func() {
				for mi, r := range matched {
					mi, r := mi, r
					itemKey := fmt.Sprintf("pl-rel-%s-%d", p.ID, mi)
					metaParts := []string{}
					if r.Timepoint != "" {
						metaParts = append(metaParts, r.Timepoint)
					}
					if r.FOV > 0 {
						metaParts = append(metaParts, fmt.Sprintf("FOV %d", r.FOV))
					}
					lbl := filepath.Base(r.Asset)
					if len(metaParts) > 0 {
						lbl = strings.Join(metaParts, " · ") + " · " + lbl
					}
					btn := ui.ButtonBase(c.Key(itemKey)).
						Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).
						Background(tok.Surface).Border(1, tok.Border).Label(lbl)
					btn.Children(func() {
						ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
							ui.Icon(c, lucide.Image()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
							ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text).SingleLine()
						})
					})
					if btn.Clicked() {
						v.ExpandedFigureID = itemKey
						v.FigureZoom = 1
						v.figureOpen = true
					}
					openDlg := v.figureOpen && v.ExpandedFigureID == itemKey
					ui.Box(c.Key("pl-rel-dlg:" + itemKey)).Children(func() {
						components.Dialog(c, &openDlg, components.DialogOptions{Label: lbl}, func(panel ui.Element) {
							panel.Width(float32(tok.Spacing48)*14).Gap(float32(tok.Spacing12)).Children(func() {
								ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
									ui.Text(c, lbl).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
									ui.Spacer(c)
									if components.Button(c, "关闭", components.ButtonOutline).Clicked() {
										openDlg = false
									}
								})
								ui.ScrollBoth(c).FillWidth().MaxHeight(460).Children(func() {
									v.drawFigureCanvas(c, itemKey, r.Asset, "", nil, 360, v.FigureZoom)
								})
							})
						})
					})
					if !openDlg && v.ExpandedFigureID == itemKey {
						v.figureOpen = false
					}
				}
			})
		}
	})
}
