package asterdoc

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
)

// renderUnifiedModuleShell enforces one consistent visual anatomy across all structured
// scientific modules (group_table, plate, param_table, data_table, calculator, embed_card, etc.):
//   - Top-level (v.StepID == ""): elevated paper card (Surface + 1px Border + PanelRadius + 12/16px padding)
//     so structured scientific blocks are unmistakably distinguished from plain Markdown prose.
//   - Nested inside a Step card (v.StepID != ""): clean hairline section without nested box cages.
//   - Unified Header Row: [16px Accent Icon] [Editable Title] [Spacer] [Right Meta (tnum)] [Extra Actions]
//     followed by a 1px hairline divider and consistent Spacing12 body rhythm.
func renderUnifiedModuleShell(
	v *View,
	c *ui.Context,
	icon *ui.SVG,
	moduleID string,
	title string,
	metaText string,
	editableTitle bool,
	extraRight func(),
	content func(),
) {
	tok := theme.GetTokens(c.Theme())
	isTopLevel := v == nil || v.StepID == ""
	box := ui.Column(c).FillWidth().Gap(float32(tok.Spacing12))
	if isTopLevel {
		box.Padding(float32(tok.Spacing12), float32(tok.Spacing16)).
			Radius(float32(tok.PanelRadius)).
			Background(tok.Surface).
			Border(1, tok.Border)
	} else {
		box.Padding(float32(tok.Spacing4), 0)
	}
	box.Children(func() {
		if title != "" || metaText != "" || icon != nil || extraRight != nil {
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if icon != nil {
						ui.Icon(c, icon).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
					}
					if title != "" {
						renderInlineEditableTitle(v, c, moduleID, title, editableTitle)
					}
					ui.Spacer(c)
					if metaText != "" {
						ui.Text(c, metaText).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").SingleLine()
					}
					if extraRight != nil {
						extraRight()
					}
				})
				ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.55))
			})
		}
		content()
	})
}

func moduleCardFrame(v *View, c *ui.Context, icon *ui.SVG, title, badgeText string, accentBar *ui.Color, content func()) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	editableTitle := v != nil && v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit) && v.currentModuleID != ""
	if v != nil && v.UnifiedLive {
		modID := ""
		if v != nil {
			modID = v.currentModuleID
		}
		renderUnifiedModuleShell(v, c, icon, modID, title, badgeText, editableTitle, nil, content)
		return
	}
	ui.Row(c).FillWidth().Radius(float32(tok.PanelRadius)).Background(tok.Surface).
		Border(1, tok.Border.Alpha(0.7)).Children(func() {
		if accentBar != nil {
			ui.Box(c).Width(t.Space(0.75)).FillHeight().Background(*accentBar)
		}
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing12)).Children(func() {
			if title != "" || badgeText != "" || icon != nil {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if icon != nil {
						ui.Icon(c, icon).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.TextMuted)
					}
					if title != "" {
						modID := ""
						if v != nil {
							modID = v.currentModuleID
						}
						renderInlineEditableTitle(v, c, modID, title, editableTitle)
					}
					ui.Spacer(c)
					if badgeText != "" {
						components.Badge(c, badgeText)
					}
				})
			}
			content()
		})
	})
}

func renderInlineEditableTitle(v *View, c *ui.Context, moduleID, title string, editable bool) {
	tok := theme.GetTokens(c.Theme())
	if v == nil || !editable || moduleID == "" {
		ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
		return
	}
	if v.titleEditID == moduleID {
		ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			ui.Box(c).Width(float32(tok.Spacing48) * 4).Children(func() {
				tb := components.TextBox(c, &v.titleEditBuf, components.TextBoxOptions{}).Placeholder("模块标题")
				tb.OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyEnter && k.Mods == 0 {
						next := strings.TrimSpace(v.titleEditBuf)
						if next != "" && next != title {
							v.emitAnyModuleEdit(moduleID, map[string]any{"title": next})
						}
						v.titleEditID = ""
						return true
					}
					return false
				})
			})
			if components.Button(c, "确定", components.ButtonSecondary).Clicked() {
				next := strings.TrimSpace(v.titleEditBuf)
				if next != "" && next != title {
					v.emitAnyModuleEdit(moduleID, map[string]any{"title": next})
				}
				v.titleEditID = ""
			}
			if components.IconButton(c, lucide.X(), "取消改名", components.IconButtonCompact).Clicked() {
				v.titleEditID = ""
			}
		})
		return
	}
	ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
		ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
		editBtn := components.IconButton(c, lucide.Pencil(), "修改标题", components.IconButtonCompact)
		components.Tooltip(c, editBtn, "自定义模块标题")
		if editBtn.Clicked() {
			v.titleEditID = moduleID
			v.titleEditBuf = title
		}
	})
}

func (v *View) renderProse(c *ui.Context, data map[string]any) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	moduleID := textValue(data, "id")
	title := textValue(data, "title")
	tone := textValue(data, "tone")
	if tone == "" {
		tone = "plain"
	}
	body := textValue(data, "body")
	editable := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)

	var icon *ui.SVG
	var badgeLabel string
	var barColor *ui.Color
	bgColor := tok.Surface
	switch tone {
	case "warning":
		icon = lucide.TriangleAlert()
		badgeLabel = "关键警示"
		col := tok.Warning
		barColor = &col
		bgColor = tok.Warning.Alpha(.07)
	case "tip":
		icon = lucide.Lightbulb()
		badgeLabel = "操作提示"
		col := tok.Accent
		barColor = &col
		bgColor = tok.AccentSoftBg
	case "note":
		icon = lucide.Info()
		badgeLabel = "备注"
		col := tok.InfoText
		barColor = &col
		bgColor = tok.Elevated
	default:
		icon = lucide.FileText()
		badgeLabel = "说明"
	}

	if v.UnifiedLive {
		proseReadOnly := !editable
		var wrapper ui.Element
		if barColor != nil {
			wrapper = ui.Row(c.Key("prose-ed:"+moduleID)).FillWidth().Radius(float32(tok.ControlRadius)).Background(bgColor)
		} else {
			wrapper = ui.Column(c.Key("prose-ed:"+moduleID)).FillWidth().Gap(float32(tok.Spacing4))
		}
		hovered := wrapper.Hovered()
		renderToneBar := func() {
			if !editable {
				return
			}
			toneOp := float32(0)
			if hovered || tone != "plain" || v.MobileCompact {
				toneOp = 1
			}
			bar := ui.Row(c.Key("prose-tone:"+moduleID)).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4))
			if bar.AnimateTo("pt-op", toneOp, theme.MotionMicroFade) > 0.05 {
				bar.Opacity(bar.AnimateTo("pt-op", toneOp, theme.MotionMicroFade)).Children(func() {
					toneChoice := tone
					sel := components.Select(c, &toneChoice)
					StyleSelectQuiet(c, tok, sel)
					sel.Trigger.Children(func() {
						ui.Text(c, proseToneLabel(toneChoice)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					})
					sel.Popup(func(panel ui.Element) {
						for _, opt := range []string{"plain", "note", "tip", "warning"} {
							opt := opt
							sel.Item(opt).Children(func() {
								ui.Text(c, proseToneLabel(opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						}
					})
					if sel.Trigger.Changed() && toneChoice != tone {
						v.emitAnyModuleEdit(moduleID, map[string]any{"tone": toneChoice})
					}
				})
			}
		}
		renderBody := func() {
			v.renderLiveMarkdownSegment(
				c,
				"step-prose:"+moduleID,
				body,
				0,
				proseReadOnly,
				!v.ReadOnly,
				"输入操作步骤说明或注意事项（空行首输入 / 插入组件）…",
				v.StepID,
				func(nextBody string) {
					if nextBody != body {
						v.emitAnyModuleEdit(moduleID, map[string]any{"body": nextBody})
					}
				},
			)
		}
		if barColor != nil {
			wrapper.Children(func() {
				ui.Box(c).Width(t.Space(0.75)).FillHeight().Background(*barColor)
				ui.Column(c).Grow(1).MinWidth(0).Padding(float32(tok.Spacing8)).Gap(float32(tok.Spacing4)).Children(func() {
					renderToneBar()
					renderBody()
				})
			})
		} else {
			wrapper.Children(func() {
				renderToneBar()
				renderBody()
			})
		}
		return
	}

	if editable {
		moduleCardFrame(v, c, icon, firstNonEmpty(title, "文本 / 注意事项"), badgeLabel, barColor, func() {
			titleBuf := v.ensureWidgetDraft("prose:"+moduleID+":title", title)
			bodyBuf := v.ensureWidgetDraft("prose:"+moduleID+":body", body)
			toneChoice := tone
			ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
				ui.Box(c).FillWidth().Children(func() {
					components.TextBox(c, titleBuf, components.TextBoxOptions{}).Placeholder("标题（可选）")
				})
				sel := components.Select(c, &toneChoice)
				sel.Trigger.Children(func() {
					ui.Text(c, proseToneLabel(toneChoice)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
				})
				sel.Popup(func(panel ui.Element) {
					for _, opt := range []string{"plain", "note", "tip", "warning"} {
						opt := opt
						sel.Item(opt).Children(func() {
							ui.Text(c, proseToneLabel(opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
						})
					}
				})
				if sel.Trigger.Changed() {
					v.emitAnyModuleEdit(moduleID, map[string]any{"tone": toneChoice})
				}
			})
			components.TextBox(c, bodyBuf, components.TextBoxOptions{Multiline: true}).
				Placeholder("输入操作说明、注意事项或 SOP 片段…").Lines(3, 8)
			if strings.TrimSpace(*bodyBuf) != "" && (*bodyBuf != body || *titleBuf != title) {
				if components.Button(c, "应用文本", components.ButtonSecondary).Clicked() {
					props := map[string]any{"body": *bodyBuf}
					if strings.TrimSpace(*titleBuf) != "" {
						props["title"] = strings.TrimSpace(*titleBuf)
					}
					v.emitAnyModuleEdit(moduleID, props)
				}
			}
		})
		return
	}

	ui.Row(c).FillWidth().Radius(float32(tok.PanelRadius)).Background(bgColor).
		Border(1, tok.Border).Children(func() {
		if barColor != nil {
			ui.Box(c).Width(t.Space(0.75)).FillHeight().Background(*barColor)
		}
		ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Padding(float32(tok.Spacing12)).Children(func() {
			if title != "" || tone != "plain" {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Icon(c, icon).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.TextMuted)
					if title != "" {
						ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
					} else {
						ui.Text(c, badgeLabel).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
					}
					ui.Spacer(c)
					if tone != "plain" {
						toneBadge(c, tone, badgeLabel)
					}
				})
			}
			for _, para := range strings.Split(body, "\n") {
				line := strings.TrimSpace(para)
				if line == "" {
					continue
				}
				if strings.HasPrefix(line, "- ") {
					line = "• " + strings.TrimSpace(strings.TrimPrefix(line, "- "))
				}
				ui.Text(c, line).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
			}
		})
	})
}

func (v *View) openSlashMenuForBlock(c *ui.Context, anchor ui.Element, moduleID string) {
	st := ui.Local(c.Root(), "slash-menu-step:"+v.StepID+":"+moduleID, func() bool { return false })
	*st = true
}

func stripStandaloneSlashLine(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "/" {
		return "", true
	}
	lines := strings.Split(raw, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "/" {
			kept := append(lines[:i], lines[i+1:]...)
			return strings.Join(kept, "\n"), true
		}
	}
	return raw, false
}

func proseToneLabel(tone string) string {
	switch tone {
	case "note":
		return "备注"
	case "tip":
		return "操作提示"
	case "warning":
		return "关键警示"
	default:
		return "普通段落"
	}
}

func toneBadge(c *ui.Context, tone, label string) {
	switch tone {
	case "warning":
		components.Badge(c, label, components.BadgeOptions{Tone: components.BadgeWarning})
	case "tip":
		components.Badge(c, label, components.BadgeOptions{Tone: components.BadgeAccent})
	default:
		components.Badge(c, label)
	}
}

func firstNonEmpty(values ...string) string {
	for _, s := range values {
		if strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
}

// inlineBadge wraps components.Badge with AlignSelf(ui.Center) so badges inside
// horizontally-centered chip strips, headers, and action bars never float to the
// top edge (overriding onevra Badge's default AlignSelf(ui.Start)).
func inlineBadge(c *ui.Context, text string, options ...components.BadgeOptions) ui.Element {
	return components.Badge(c, text, options...).AlignSelf(ui.Center)
}
