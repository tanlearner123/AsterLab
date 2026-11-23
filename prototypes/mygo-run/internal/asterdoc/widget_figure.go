package asterdoc

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	_ "golang.org/x/image/tiff"

	"asterlab-mygo-prototype/internal/theme"
)

type svgTextLabel struct {
	X      float64
	Y      float64
	Anchor string
	Text   string
	Size   float64
}

var (
	reViewBox   = regexp.MustCompile(`viewBox\s*=\s*["']([^"']+)["']`)
	reSVGToken  = regexp.MustCompile(`(?s)<g\b([^>]*)>|</g>|<text\b([^>]*)>(.*?)</text>`)
	reTranslate = regexp.MustCompile(`translate\(\s*([-+]?[0-9]*\.?[0-9]+)(?:[\s,]+([-+]?[0-9]*\.?[0-9]+))?\s*\)`)
	reAttrX     = regexp.MustCompile(`\bx\s*=\s*["']([^"']+)["']`)
	reAttrY     = regexp.MustCompile(`\by\s*=\s*["']([^"']+)["']`)
	reAttrSize  = regexp.MustCompile(`\bfont-size\s*=\s*["']([^"']+)["']`)
	reAttrAnch  = regexp.MustCompile(`\btext-anchor\s*=\s*["']([^"']+)["']`)
	reTagStrip  = regexp.MustCompile(`<[^>]+>`)
)

func parseTranslateAttr(attrs string) (float64, float64) {
	m := reTranslate.FindStringSubmatch(attrs)
	if len(m) < 2 {
		return 0, 0
	}
	tx, _ := strconv.ParseFloat(m[1], 64)
	var ty float64
	if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
		ty, _ = strconv.ParseFloat(m[2], 64)
	}
	return tx, ty
}

func estimateSVGTextWidth(text string, fsize float32) float32 {
	var units float32
	for _, r := range text {
		if r > 0x7F {
			units += 0.96
		} else {
			units += 0.54
		}
	}
	return units * fsize
}

func parseSVGMetricsAndLabels(rawSVG string) (vbX, vbY, vbW, vbH float64, labels []svgTextLabel) {
	vbW, vbH = 360, 260
	if m := reViewBox.FindStringSubmatch(rawSVG); len(m) == 2 {
		parts := strings.Fields(m[1])
		if len(parts) == 4 {
			if x, err := strconv.ParseFloat(parts[0], 64); err == nil {
				vbX = x
			}
			if y, err := strconv.ParseFloat(parts[1], 64); err == nil {
				vbY = y
			}
			if w, err := strconv.ParseFloat(parts[2], 64); err == nil && w > 0 {
				vbW = w
			}
			if h, err := strconv.ParseFloat(parts[3], 64); err == nil && h > 0 {
				vbH = h
			}
		}
	}
	var stack [][2]float64
	var curX, curY float64
	for _, match := range reSVGToken.FindAllStringSubmatch(rawSVG, -1) {
		tok := match[0]
		switch {
		case strings.HasPrefix(tok, "</g"):
			if n := len(stack); n > 0 {
				curX -= stack[n-1][0]
				curY -= stack[n-1][1]
				stack = stack[:n-1]
			}
		case strings.HasPrefix(tok, "<g"):
			gAttrs := match[1]
			if strings.HasSuffix(strings.TrimSpace(gAttrs), "/") {
				continue
			}
			tx, ty := parseTranslateAttr(gAttrs)
			stack = append(stack, [2]float64{tx, ty})
			curX += tx
			curY += ty
		default:
			attrs, body := match[2], strings.TrimSpace(reTagStrip.ReplaceAllString(match[3], ""))
			if body == "" {
				continue
			}
			var x, y float64
			size := 12.0
			anchor := "start"
			if xm := reAttrX.FindStringSubmatch(attrs); len(xm) == 2 {
				x, _ = strconv.ParseFloat(xm[1], 64)
			}
			if ym := reAttrY.FindStringSubmatch(attrs); len(ym) == 2 {
				y, _ = strconv.ParseFloat(ym[1], 64)
			}
			if sm := reAttrSize.FindStringSubmatch(attrs); len(sm) == 2 {
				size, _ = strconv.ParseFloat(strings.TrimSuffix(sm[1], "px"), 64)
			}
			if am := reAttrAnch.FindStringSubmatch(attrs); len(am) == 2 {
				anchor = am[1]
			}
			labels = append(labels, svgTextLabel{
				X:      x + curX,
				Y:      y + curY,
				Anchor: anchor,
				Text:   body,
				Size:   size,
			})
		}
	}
	return vbX, vbY, vbW, vbH, labels
}

func (v *View) renderFigure(c *ui.Context, data map[string]any) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	moduleID := textValue(data, "id")
	title := firstNonEmpty(textValue(data, "title"), "示意图 / 实验照片")
	caption := textValue(data, "caption")
	src := textValue(data, "src")
	svgRaw := textValue(data, "svg")
	maxH := float32(220)
	if mh, ok := numericValue(data["max_height"]); ok && mh >= 60 {
		maxH = float32(mh)
	}
	editableDesign := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)

	var annotations []svgTextLabel
	for _, am := range mapSlice(data["annotations"]) {
		ax, _ := numericValue(am["x"])
		ay, _ := numericValue(am["y"])
		lbl := textValue(am, "label")
		if lbl != "" {
			annotations = append(annotations, svgTextLabel{X: ax, Y: ay, Anchor: "middle", Text: lbl, Size: 11})
		}
	}

	badge := "补充视觉表达"
	if svgRaw != "" {
		badge = "矢量 SVG"
	} else if src != "" {
		badge = "本地图片"
	}

	if v.FigureZoom <= 0 {
		v.FigureZoom = 1
	}

	if v.UnifiedLive {
		v.renderUnifiedFigure(c, tok, moduleID, title, caption, src, svgRaw, annotations, maxH, editableDesign)
		return
	}

	card := ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing12)).
		Radius(float32(tok.PanelRadius)).Background(tok.Surface).Border(1, tok.Border).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Icon(c, lucide.Image()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.TextMuted)
			renderInlineEditableTitle(v, c, moduleID, title, editableDesign)
			ui.Spacer(c)
			components.Badge(c, badge)
			zoomBtn := components.IconButton(c, lucide.Maximize2(), "放大查看", components.IconButtonCompact)
			components.Tooltip(c, zoomBtn, "放大查看示意图与文字标注")
			if zoomBtn.Clicked() {
				v.ExpandedFigureID = moduleID
				v.FigureZoom = 1
				v.figureOpen = true
			}
		})

		v.drawFigureCanvas(c, moduleID, src, svgRaw, annotations, maxH, 1)

		if caption != "" {
			ui.Text(c, caption).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		}

		if editableDesign {
			openEd := v.ensureWidgetBool("fig_ed:" + moduleID)
			components.Collapsible(c, "编辑 SVG / 图片路径与图注（支持直接拖入本地文件）", openEd, components.CollapsibleOptions{}, func() {
				srcBuf := v.ensureWidgetDraft("fig:"+moduleID+":src", src)
				capBuf := v.ensureWidgetDraft("fig:"+moduleID+":cap", caption)
				svgBuf := v.ensureWidgetDraft("fig:"+moduleID+":svg", svgRaw)
				ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing8), 0, 0, 0).Children(func() {
					ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
						ui.Box(c).FillWidth().Children(func() {
							components.TextBox(c, srcBuf, components.TextBoxOptions{}).Placeholder("本地图片路径（可选，支持直接拖入文件）")
						})
						if v.OnPickFigureFile != nil {
							if components.Button(c, "选择文件", components.ButtonOutline).Clicked() {
								v.OnPickFigureFile(v.StepID, moduleID)
							}
						}
					})
					components.TextBox(c, capBuf, components.TextBoxOptions{}).Placeholder("图注 caption")
					components.TextBox(c, svgBuf, components.TextBoxOptions{Multiline: true}).
						Placeholder("内嵌 <svg viewBox=\"0 0 360 260\">…</svg> 源码").Lines(4, 10)
					if components.Button(c, "应用示意图", components.ButtonSecondary).Clicked() {
						props := map[string]any{"caption": strings.TrimSpace(*capBuf)}
						if strings.TrimSpace(*srcBuf) != "" {
							props["src"] = strings.TrimSpace(*srcBuf)
						}
						if strings.TrimSpace(*svgBuf) != "" {
							props["svg"] = strings.TrimSpace(*svgBuf)
						}
						v.emitAnyModuleEdit(moduleID, props)
					}
				})
			})
		}

		openFig := v.figureOpen && v.ExpandedFigureID == moduleID
		ui.Box(c.Key("fig-dlg:"+moduleID)).Children(func() {
			components.Dialog(c, &openFig, components.DialogOptions{Label: title}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*14).Gap(float32(tok.Spacing12)).Children(func() {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
					ui.Spacer(c)
					for _, z := range []float32{0.75, 1.0, 1.5, 2.0} {
						z := z
						style := components.ButtonOutline
						if v.FigureZoom == z {
							style = components.ButtonSecondary
						}
						if components.Button(c, fmt.Sprintf("%d%%", int(z*100)), style).Clicked() {
							v.FigureZoom = z
						}
					}
					if components.Button(c, "关闭", components.ButtonOutline).Clicked() {
						openFig = false
					}
				})
				ui.ScrollBoth(c).FillWidth().MaxHeight(460).Children(func() {
					v.drawFigureCanvas(c, moduleID, src, svgRaw, annotations, 320, v.FigureZoom)
				})
				if caption != "" {
					ui.Text(c, caption).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				}
			})
		})
		})
		if !openFig && v.ExpandedFigureID == moduleID {
			v.figureOpen = false
		}
	})
	if editableDesign {
		if card.FileDragOver() {
			card.Border(2, tok.Accent)
		}
		if dropped := card.DroppedFiles(); len(dropped) > 0 {
			picked := dropped[0]
			srcVal := portableFigureAssetSrc(v.BaseDir, picked)
			props := map[string]any{"src": srcVal}
			if strings.EqualFold(filepath.Ext(picked), ".svg") {
				if rawBytes, readErr := os.ReadFile(picked); readErr == nil && strings.Contains(string(rawBytes), "<svg") {
					props["svg"] = string(rawBytes)
				}
			}
			v.emitAnyModuleEdit(moduleID, props)
		}
	}
}

func (v *View) renderUnifiedFigure(c *ui.Context, tok theme.Tokens, moduleID, title, caption, src, svgRaw string, annotations []svgTextLabel, maxH float32, editableDesign bool) {
	pasteFromClipboard := func() {
		if v.ReadOnly {
			return
		}
		data := c.Services().ReadClipboardData(transfer.PNG, transfer.FileList)
		if b, err := data.Read(transfer.PNG); err == nil && len(b) > 0 {
			fname := fmt.Sprintf("paste_%d.png", time.Now().UnixMilli())
			if v.OnImportAssetBytes != nil {
				if srcPath, impErr := v.OnImportAssetBytes(b, fname); impErr == nil && srcPath != "" {
					v.emitAnyModuleEdit(moduleID, map[string]any{"src": srcPath, "svg": ""})
					return
				}
			}
			if v.BaseDir != "" {
				assetsDir := filepath.Join(v.BaseDir, "assets")
				if mkErr := os.MkdirAll(assetsDir, 0o755); mkErr == nil {
					dest := filepath.Join(assetsDir, fname)
					if wErr := os.WriteFile(dest, b, 0o644); wErr == nil {
						v.emitAnyModuleEdit(moduleID, map[string]any{"src": "./assets/" + fname, "svg": ""})
						return
					}
				}
			}
		}
		if files, err := data.Files(); err == nil && len(files) > 0 {
			picked := files[0]
			srcVal := portableFigureAssetSrc(v.BaseDir, picked)
			if v.OnImportAssetBytes != nil {
				if raw, rErr := os.ReadFile(picked); rErr == nil && len(raw) > 0 {
					if srcPath, impErr := v.OnImportAssetBytes(raw, filepath.Base(picked)); impErr == nil && srcPath != "" {
						srcVal = srcPath
					}
				}
			}
			props := map[string]any{"src": srcVal}
			if strings.EqualFold(filepath.Ext(picked), ".svg") {
				if rawBytes, readErr := os.ReadFile(picked); readErr == nil && strings.Contains(string(rawBytes), "<svg") {
					props["svg"] = string(rawBytes)
				}
			}
			v.emitAnyModuleEdit(moduleID, props)
		}
	}

	if v.FigureZoom <= 0 {
		v.FigureZoom = 1
	}

	trimSVG := strings.TrimSpace(svgRaw)
	isPlaceholderSVG := trimSVG == "" || (strings.HasPrefix(trimSVG, "<svg") && strings.HasSuffix(trimSVG, "></svg>") && strings.Count(trimSVG, "<") == 2)
	hasGraphic := strings.TrimSpace(src) != "" || !isPlaceholderSVG
	card := ui.Column(c.Key("live-fig:"+moduleID)).Focusable().FillWidth().Gap(float32(tok.Spacing8)).
		Padding(float32(tok.Spacing12)).
		Radius(float32(tok.PanelRadius)).Background(tok.Surface).Border(1, tok.Border)

	cardHover := card.Hovered()
	if editableDesign && card.FileDragOver() {
		card.Border(2, tok.Accent)
	}
	type figHeroMatch struct{ ID string }
	openFig := v.figureOpen && v.ExpandedFigureID == moduleID
	card.Children(func() {
		if hasGraphic {
			heroBox := ui.Column(c.Key("fig-inline-hero:" + moduleID)).FillWidth()
			if !openFig {
				heroBox.Transition(theme.HeroFlyTransition(figHeroMatch{ID: moduleID}))
			}
			heroBox.Children(func() {
				v.drawFigureCanvas(c, moduleID, src, svgRaw, annotations, maxH, 1)
			})
		} else {
			dropBox := ui.Column(c.Key("live-fig-empty:"+moduleID)).FillWidth().
				Padding(float32(tok.Spacing24), float32(tok.Spacing16)).
				Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
				AlignItems(ui.Center).Gap(float32(tok.Spacing8))
			dropBox.Children(func() {
				ui.Icon(c, lucide.Image()).Size(float32(tok.Spacing24), float32(tok.Spacing24)).TextColor(tok.TextMuted)
				ui.Text(c, "拖入实验照片 / 示意图，或 Ctrl+V 直接粘贴剪贴板截图").
					FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
				ui.Row(c).Gap(float32(tok.Spacing8)).Children(func() {
					if v.OnPickFigureFile != nil && editableDesign {
						if components.Button(c, "选择本地图片", components.ButtonSecondary).Clicked() {
							v.OnPickFigureFile(v.StepID, moduleID)
						}
					}
					if editableDesign {
						if components.Button(c, "粘贴剪贴板图片", components.ButtonOutline).Clicked() {
							pasteFromClipboard()
						}
					}
				})
			})
		}

		// Clean inline caption below figure (clicks/blurs auto-save; zoom button stays accessible on touch/mobile)
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			if editableDesign {
				capBuf := v.ensureWidgetDraft("fig:"+moduleID+":cap", caption)
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					ti := BorderlessTextInput(c, tok, capBuf, float32(tok.TextMeta), false, "点击添加图注（如 显微镜明场 10x、手绘接种流路拍照）…").
						TextColor(tok.TextMuted)
					if ti.Submitted() || (!ti.Focused() && strings.TrimSpace(*capBuf) != strings.TrimSpace(caption)) {
						v.emitAnyModuleEdit(moduleID, map[string]any{"caption": strings.TrimSpace(*capBuf)})
					}
				})
			} else if caption != "" {
				ui.Text(c, caption).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).Grow(1).MinWidth(0)
			} else {
				ui.Spacer(c)
			}
			if hasGraphic {
				zoomOp := float32(0.55)
				if cardHover || v.MobileCompact {
					zoomOp = 1.0
				}
				zoomWrap := ui.Box(c.Key("fig-zoom-wrap:" + moduleID)).Shrink(0)
				zoomWrap.Opacity(zoomWrap.AnimateTo("fz-op", zoomOp, theme.MotionMicroFade)).Children(func() {
					if components.IconButton(c, lucide.Maximize2(), "放大查看", components.IconButtonCompact).Clicked() {
						v.ExpandedFigureID = moduleID
						v.FigureZoom = 1
						v.figureOpen = true
					}
				})
			}
		})
	})

	if editableDesign {
		if card.Clicked() {
			card.Focus()
		}
		if card.Shortcut(ui.Cmd, ui.KeyV) {
			pasteFromClipboard()
		}
		if dropped := card.DroppedFiles(); len(dropped) > 0 {
			picked := dropped[0]
			srcVal := portableFigureAssetSrc(v.BaseDir, picked)
			if v.OnImportAssetBytes != nil {
				if raw, rErr := os.ReadFile(picked); rErr == nil && len(raw) > 0 {
					if srcPath, impErr := v.OnImportAssetBytes(raw, filepath.Base(picked)); impErr == nil && srcPath != "" {
						srcVal = srcPath
					}
				}
			}
			props := map[string]any{"src": srcVal}
			if strings.EqualFold(filepath.Ext(picked), ".svg") {
				if rawBytes, readErr := os.ReadFile(picked); readErr == nil && strings.Contains(string(rawBytes), "<svg") {
					props["svg"] = string(rawBytes)
				}
			}
			v.emitAnyModuleEdit(moduleID, props)
		}
	}

	svgEdOpen := v.ensureWidgetBool("fig_svg_dlg:" + moduleID)
	components.ContextMenu(c, card, func(m *components.Menu) {
		if editableDesign && v.OnPickFigureFile != nil {
			if m.Item("从本地选择图片…").Chosen() {
				v.OnPickFigureFile(v.StepID, moduleID)
			}
		}
		if editableDesign {
			if m.Item("从剪贴板粘贴图片").Shortcut("Ctrl+V").Chosen() {
				pasteFromClipboard()
			}
		}
		if hasGraphic {
			if m.Item("放大查看").Chosen() {
				v.ExpandedFigureID = moduleID
				v.FigureZoom = 1
				v.figureOpen = true
			}
		}
		if editableDesign {
			m.Separator()
			if m.Item("编辑 SVG 源码…").Chosen() {
				*svgEdOpen = true
			}
			if hasGraphic && m.Item("清除当前图像").Chosen() {
				v.emitAnyModuleEdit(moduleID, map[string]any{"src": "", "svg": ""})
			}
		}
	})

	ui.Box(c.Key("fig-dlg:"+moduleID)).Children(func() {
		components.Dialog(c, &openFig, components.DialogOptions{Label: title}, func(panel ui.Element) {
		panel.Width(float32(tok.Spacing48)*14).Gap(float32(tok.Spacing12)).Children(func() {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
				ui.Spacer(c)
				for _, z := range []float32{0.75, 1.0, 1.5, 2.0} {
					z := z
					style := components.ButtonOutline
					if v.FigureZoom == z {
						style = components.ButtonSecondary
					}
					if components.Button(c, fmt.Sprintf("%d%%", int(z*100)), style).Clicked() {
						v.FigureZoom = z
					}
				}
				if components.Button(c, "关闭", components.ButtonOutline).Clicked() {
					openFig = false
				}
			})
			ui.ScrollBoth(c).FillWidth().MaxHeight(460).Children(func() {
				ui.Column(c.Key("fig-modal-hero:"+moduleID)).FillWidth().
					Transition(theme.HeroFlyTransition(figHeroMatch{ID: moduleID})).Children(func() {
					v.drawFigureCanvas(c, moduleID, src, svgRaw, annotations, 320, v.FigureZoom)
				})
			})
			if caption != "" {
				ui.Text(c, caption).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			}
		})
	})
	})
	if !openFig && v.ExpandedFigureID == moduleID {
		v.figureOpen = false
	}

	if editableDesign {
		svgBuf := v.ensureWidgetDraft("fig:"+moduleID+":svg", svgRaw)
		ui.Box(c.Key("fig-svg-ed:"+moduleID)).Children(func() {
			components.Dialog(c, svgEdOpen, components.DialogOptions{Label: "编辑 SVG 矢量源码"}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*11).Gap(float32(tok.Spacing12)).Children(func() {
				components.TextBox(c, svgBuf, components.TextBoxOptions{Multiline: true}).
					Placeholder("<svg viewBox=\"0 0 360 220\">…</svg>").Lines(8, 16)
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Spacer(c)
					if components.Button(c, "取消", components.ButtonOutline).Clicked() {
						*svgEdOpen = false
					}
					if components.Button(c, "应用 SVG", components.ButtonPrimary).Clicked() {
						v.emitAnyModuleEdit(moduleID, map[string]any{"svg": strings.TrimSpace(*svgBuf)})
						*svgEdOpen = false
					}
				})
			})
		})
		})
	}
}

// PortableFigureAssetPath resolves or copies a user-selected image into
// `<baseDir>/assets/<filename>` when baseDir is set, returning `./assets/<filename>`.
func PortableFigureAssetPath(baseDir, picked string) string {
	return portableFigureAssetSrc(baseDir, picked)
}

func resolveFigurePath(baseDir, src string) string {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" || filepath.IsAbs(trimmed) || baseDir == "" {
		return trimmed
	}
	return filepath.Join(baseDir, filepath.FromSlash(trimmed))
}

func portableFigureAssetSrc(baseDir, picked string) string {
	trimmed := strings.TrimSpace(picked)
	if baseDir == "" || trimmed == "" {
		return trimmed
	}
	if rawBytes, err := os.ReadFile(trimmed); err == nil {
		assetsDir := filepath.Join(baseDir, "assets")
		if mkErr := os.MkdirAll(assetsDir, 0o755); mkErr == nil {
			base := filepath.Base(trimmed)
			dest := filepath.Join(assetsDir, base)
			if writeErr := os.WriteFile(dest, rawBytes, 0o644); writeErr == nil {
				return "./assets/" + base
			}
		}
	}
	if rel, err := filepath.Rel(baseDir, trimmed); err == nil && !strings.HasPrefix(rel, "..") {
		slash := filepath.ToSlash(rel)
		if !strings.HasPrefix(slash, "./") {
			slash = "./" + slash
		}
		return slash
	}
	return trimmed
}

func (v *View) drawFigureCanvas(c *ui.Context, moduleID, src, svgRaw string, annotations []svgTextLabel, maxH float32, zoom float32) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	if zoom <= 0 {
		zoom = 1
	}
	if svgRaw != "" {
		vbX, vbY, vbW, vbH, labels := parseSVGMetricsAndLabels(svgRaw)
		allLabels := append(labels, annotations...)
		drawH := maxH * zoom
		drawW := float32(vbW/vbH) * drawH
		if zoom <= 1 && drawW > 380 {
			drawW = 380
			drawH = float32(vbH/vbW) * drawW
		} else if zoom > 1 && drawW > 960 {
			drawW = 960
			drawH = float32(vbH/vbW) * drawW
		}
		parsedSVG, err := v.cachedSVG(svgRaw)
		if err != nil {
			ui.Row(c).Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
				Background(tok.Danger.Alpha(.08)).Border(1, tok.Danger).Children(func() {
				ui.Text(c, "SVG 解析失败："+err.Error()).FontSize(float32(tok.TextMeta)).TextColor(tok.Danger)
			})
			return
		}
		ui.Row(c).FillWidth().Justify(ui.Center).
			Padding(float32(tok.Spacing12), float32(tok.Spacing12), float32(tok.Spacing16), float32(tok.Spacing12)).
			Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).Children(func() {
			ui.Box(c).Size(drawW, drawH).DrawOver(func(p *ui.Painter, r ui.Rect) {
				sx := float64(r.W) / vbW
				sy := float64(r.H) / vbH
				for _, lbl := range allLabels {
					px := r.X + float32((lbl.X-vbX)*sx)
					py := r.Y + float32((lbl.Y-vbY)*sy)
					fsize := float32(lbl.Size * sx)
					if fsize < float32(tok.TextMeta) {
						fsize = float32(tok.TextMeta)
					}
					if fsize > float32(tok.TextSection)*zoom {
						fsize = float32(tok.TextSection) * zoom
					}
					textW := estimateSVGTextWidth(lbl.Text, fsize)
					if lbl.Anchor == "middle" {
						px -= textW * 0.5
					} else if lbl.Anchor == "end" {
						px -= textW
					}
					p.Text(px, py-fsize*0.35, lbl.Text, fsize, tok.Text)
				}
			}).Children(func() {
				ui.Image(c, parsedSVG).Size(drawW, drawH)
			})
		})
		return
	}

	if src != "" {
		resolved := resolveFigurePath(v.BaseDir, src)
		imgSrc, loadErr := v.cachedImageFile(resolved)
		if loadErr != "" {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
				Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
				Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.Image()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.TextMuted)
				ui.Text(c, "图片引用："+src+"（"+loadErr+"）").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			})
			return
		}
		ui.Row(c).FillWidth().Justify(ui.Center).
			Padding(float32(tok.Spacing12), float32(tok.Spacing12), float32(tok.Spacing16), float32(tok.Spacing12)).
			Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).Children(func() {
			ui.Image(c, imgSrc).Height(maxH * zoom)
		})
	}
}

func (v *View) cachedSVG(raw string) (*ui.SVG, error) {
	if v.svgCache == nil {
		v.svgCache = map[string]*ui.SVG{}
	}
	if s, ok := v.svgCache[raw]; ok {
		return s, nil
	}
	s, err := ui.ParseSVG([]byte(raw))
	if err != nil {
		return nil, err
	}
	v.svgCache[raw] = s
	return s, nil
}

func (v *View) cachedImageFile(path string) (ui.ImageSource, string) {
	if v.imgCache == nil {
		v.imgCache = map[string]ui.ImageSource{}
	}
	if v.imgErrCache == nil {
		v.imgErrCache = map[string]string{}
	}
	if v.imgLoading == nil {
		v.imgLoading = map[string]bool{}
	}
	if img, ok := v.imgCache[path]; ok {
		return img, ""
	}
	if errText, ok := v.imgErrCache[path]; ok {
		return nil, errText
	}
	if v.imgLoading[path] {
		return nil, "加载缩略图中…"
	}
	if !v.AsyncDecode {
		bytes, err := os.ReadFile(path)
		if err != nil {
			v.imgErrCache[path] = "未找到本地文件"
			return nil, v.imgErrCache[path]
		}
		img, err := ui.DecodeImage(bytes)
		if err != nil {
			v.imgErrCache[path] = "图像解码失败"
			return nil, v.imgErrCache[path]
		}
		v.imgCache[path] = img
		return img, ""
	}
	v.imgLoading[path] = true
	go func(p string) {
		rawBytes, readErr := os.ReadFile(p)
		var decoded ui.ImageSource
		var errText string
		if readErr != nil {
			errText = "未找到本地文件"
		} else {
			img, decErr := ui.DecodeImage(rawBytes)
			if decErr != nil {
				errText = "图像解码失败"
			} else {
				decoded = img
			}
		}
		mygo.RunOnMain(func() {
			delete(v.imgLoading, p)
			if decoded != nil {
				v.imgCache[p] = decoded
			} else {
				v.imgErrCache[p] = errText
			}
			if v.OnAsyncInvalidate != nil {
				v.OnAsyncInvalidate()
			}
		})
	}(path)
	return nil, "加载缩略图中…"
}
