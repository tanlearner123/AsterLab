package asterdoc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"gopkg.in/yaml.v3"

	"asterlab-mygo-prototype/internal/theme"
	plateui "asterlab-mygo-prototype/internal/uikit/plate"
)

// nlHoverState tracks the active Alt+Hover (or Alt+Enter) natural-language sentence preview,
// enabling instant zero-delay real Alt sweeps, left-to-right colorful luminous line sweep,
// and non-occluding below-line popover placement.
type nlHoverState struct {
	editorKey   string
	lineIdx     int
	lineStart   int
	lineEnd     int
	globalStart int
	globalEnd   int
	lineText    string
	sug         *NLSuggestion   // currently active suggestion (sugs[activeIdx])
	sugs        []*NLSuggestion // >=1 suggestions when multi-action clause split fires
	activeIdx   int
	openedAt    time.Time
	hideAt      time.Time
	byShortcut  bool
	overCard    bool
}

// buildPrismSweepStops constructs a smooth left-to-right scrolling multi-stop Oklab gradient
// using AsterLab's registered theme & scientific semantic palette colors (zero literal hex).
func buildPrismSweepStops(tok theme.Tokens, dark bool, phase float32, alphaBase, alphaPeak float32) []ui.GradientStop {
	palIdx := map[string]int{"p0": 0, "p1": 1, "p2": 2}
	cViolet := tok.Accent
	cSky := plateui.GroupColorIndexed("p0", palIdx, dark)
	cEmerald := tok.Success
	cAmber := tok.Warning

	clamp01 := func(x float32) float32 {
		if x < 0 {
			return 0
		}
		if x > 1 {
			return 1
		}
		return x
	}
	// Traveling luminous wave sweeps smoothly from left (-0.18) across to right (+1.18)
	center := -0.18 + 1.36*phase
	s0 := clamp01(center - 0.34)
	s1 := clamp01(center - 0.16)
	s2 := clamp01(center)
	s3 := clamp01(center + 0.16)
	s4 := clamp01(center + 0.34)

	return []ui.GradientStop{
		{Color: cViolet.Alpha(alphaBase), At: 0},
		{Color: cViolet.Alpha(alphaBase * 1.15), At: s0},
		{Color: cSky.Alpha(alphaPeak * 0.92), At: s1},
		{Color: cViolet.Alpha(alphaPeak), At: s2},
		{Color: cEmerald.Alpha(alphaPeak * 0.88), At: s3},
		{Color: cAmber.Alpha(alphaBase * 1.35), At: s4},
		{Color: cViolet.Alpha(alphaBase), At: 1},
	}
}

// nlPrismPhase computes the continuous left-to-right scroll phase in [0, 1) for the hovered sentence.
func nlPrismPhase(c *ui.Context, openedAt time.Time) float32 {
	if theme.ReducedMotionEnabled() {
		return 0.28
	}
	elapsed := c.Now().Sub(openedAt).Seconds()
	if elapsed < 0 {
		elapsed = 0
	}
	return float32(math.Mod(elapsed/1.8, 1.0))
}

// nlPrismInkColor returns a rich, legible rainbow ink color at normalized sentence position pos01
// as a colorful prism wave rolls smoothly from left to right (phase in [0, 1)).
func nlPrismInkColor(tok theme.Tokens, dark bool, pos01, phase float32) ui.Color {
	palIdx := map[string]int{"p0": 0, "p1": 1, "p2": 2}
	stops := [4]ui.Color{
		tok.Accent,                                      // AsterLab Violet
		plateui.GroupColorIndexed("p0", palIdx, dark),   // Scientific Cobalt Blue
		tok.Success,                                     // Crisp Emerald
		tok.Danger,                                      // Warm Rose/Magenta bridging back to Violet
	}
	// Subtracting phase makes the color wave visually travel LEFT -> RIGHT across characters.
	u := float64(pos01)*0.85 - float64(phase)
	u = u - math.Floor(u)
	scaled := u * float64(len(stops))
	i0 := int(scaled) % len(stops)
	i1 := (i0 + 1) % len(stops)
	frac := float32(scaled - math.Floor(scaled))
	return theme.MixColor(stops[i0], stops[i1], frac)
}

// buildRainbowTextReplacements overlays pure per-glyph rolling rainbow text colors onto the hovered
// sentence line (zero boxes, zero rectangles, zero underline clutter) while preserving existing
// live-markdown replacements such as `- ` bullets.
func buildRainbowTextReplacements(
	base []editor.Replacement,
	buf string,
	lnStart, lnEnd int,
	tok theme.Tokens,
	dark bool,
	phase float32,
) []editor.Replacement {
	if lnStart < 0 || lnEnd > len(buf) || lnStart >= lnEnd {
		return base
	}
	overlapsBase := func(s, e int) bool {
		for _, r := range base {
			if s < r.End && e > r.Start {
				return true
			}
		}
		return false
	}

	type glyphSpan struct {
		start, end int
		text       string
	}
	var glyphs []glyphSpan
	for off := lnStart; off < lnEnd; {
		r, sz := utf8.DecodeRuneInString(buf[off:lnEnd])
		if sz <= 0 {
			break
		}
		if r != ' ' && r != '\t' && !overlapsBase(off, off+sz) {
			glyphs = append(glyphs, glyphSpan{start: off, end: off + sz, text: buf[off : off+sz]})
		}
		off += sz
	}
	if len(glyphs) == 0 {
		return base
	}

	out := make([]editor.Replacement, 0, len(base)+len(glyphs))
	out = append(out, base...)
	denom := float32(max(1, len(glyphs)-1))
	for i, g := range glyphs {
		pos01 := float32(i) / denom
		out = append(out, editor.Replacement{
			Start: g.start,
			End:   g.end,
			Text:  g.text,
			Color: nlPrismInkColor(tok, dark, pos01, phase),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out
}

// renderNLLineSweepHighlight places a completely invisible PassThrough anchor over the hovered
// sentence line (no box, no border, no background — pure rolling rainbow glyphs carry the visual
// highlight) so the preview popover anchors cleanly right BELOW the sentence with zero occlusion.
func (v *View) renderNLLineSweepHighlight(
	c *ui.Context,
	tok theme.Tokens,
	dark bool,
	lineTop float32,
	lineH float32,
	openedAt time.Time,
) ui.Element {
	if !theme.ReducedMotionEnabled() {
		c.After(28 * time.Millisecond)
	}
	return ui.Box(c.Key("nl-sweep-line")).Absolute().
		Left(0).Right(0).
		Top(lineTop).Height(lineH).
		PassThrough()
}

// renderNLHoverOverlay mounts the non-occluding preview card strictly BELOW the glowing sentence
// with a snappy spring entrance, keeping the hovered sentence 100% visible at all times.
func (v *View) renderNLHoverOverlay(
	c *ui.Context,
	tok theme.Tokens,
	dark bool,
	altHeld bool,
	anchor ui.Element,
	st *nlHoverState,
	onDismiss func(),
) {
	if st == nil || st.sug == nil {
		return
	}
	phase := float32(0.42)
	if !theme.ReducedMotionEnabled() {
		elapsed := c.Now().Sub(st.openedAt).Seconds()
		if elapsed < 0 {
			elapsed = 0
		}
		phase = float32(math.Mod(elapsed/1.7, 1.0))
	}

	ui.Overlay(c, func() {
		panel := ui.Box(c.Key("nl-hover-card")).
			AttachTo(anchor, ui.AnchorBottomLeft, ui.AnchorTopLeft).
			Margin(float32(tok.Spacing8), 0, 0, 0).
			Background(tok.Surface).
			Border(1, tok.Accent.Alpha(0.26)).
			Radius(float32(tok.PanelRadius)).
			Shadow(0, 10, 28, 0, tok.Text.Alpha(0.14)).
			Clip().
			Transition(theme.DialogSurfaceTransition()).
			Role(ui.RoleTooltip).
			Cursor(ui.CursorDefault)

		// While Alt is held, let pointer pass straight through the card so the user can sweep
		// smoothly DOWN from Line 1 -> Line 2 -> Line 3 without the card ever blocking scanning!
		// The moment Alt is released, the card becomes solid & clickable (260ms grace bridge).
		if altHeld && !st.byShortcut {
			panel.PassThrough()
			st.overCard = false
		} else {
			st.overCard = panel.Hovered()
		}
		if panel.Shortcut(0, ui.KeyEscape) && onDismiss != nil {
			onDismiss()
		}

		panel.Children(func() {
			// Sleek 2.5px scrolling prism top hairline echoing the glowing sentence directly above
			ui.Box(c).FillWidth().Height(2.5).LinearGradient(ui.LinearGradient{
				Angle: 90,
				Oklab: true,
				Stops: buildPrismSweepStops(tok, dark, phase, 0.30, 0.92),
			})
			ui.Box(c).Padding(float32(tok.Spacing12)).Children(func() {
				v.renderNLSuggestionPopoverMulti(c, tok, st, onDismiss)
			})
		})
	})
}

// ApplyNLSuggestionToSource inserts (or replaces) the suggested AsterML v2 module(s)
// right at the hovered sentence's byte range in docSource.
func ApplyNLSuggestionToSource(docSource string, lineStart, lineEnd int, replaceLine bool, mods []map[string]any) (string, error) {
	if len(mods) == 0 {
		return docSource, fmt.Errorf("no modules to apply")
	}
	var blocks []string
	for _, m := range mods {
		raw, err := yaml.Marshal(m)
		if err != nil {
			return docSource, err
		}
		fenced := "```asterlab\n" + strings.TrimSpace(string(raw)) + "\n```"
		blocks = append(blocks, fenced)
	}
	joined := strings.Join(blocks, "\n\n")
	if replaceLine {
		return ReplaceByteRange(docSource, lineStart, lineEnd, joined)
	}
	// Keep original sentence intact and insert the live module(s) cleanly below it
	insertPos := lineEnd
	if insertPos > len(docSource) {
		insertPos = len(docSource)
	}
	prefix := "\n\n"
	if insertPos < len(docSource) && docSource[insertPos] == '\n' {
		insertPos++
		prefix = "\n"
	}
	suffix := "\n\n"
	if insertPos < len(docSource) && strings.HasPrefix(docSource[insertPos:], "\n") {
		suffix = "\n"
	}
	return docSource[:insertPos] + prefix + joined + suffix + docSource[insertPos:], nil
}

// renderNLSuggestionPopover renders the sleek Alt+Hover dropdown preview card showing:
//   - Jev routing badge & calibrated confidence
//   - Mini visual component diagram (timer timeline / 2D LaTeX formula / colored mini plate / FOV grid)
//   - Pre-filled extracted parameter pills
//   - One-click [✨ 下方插入组件] and [替换原句] actions
// renderNLSuggestionPopoverMulti wraps renderNLSuggestionPopover with multi-action tabs
// and a one-click [全部插入] bar when a compound sentence splits into >=2 actionable widgets.
func (v *View) renderNLSuggestionPopoverMulti(
	c *ui.Context,
	tok theme.Tokens,
	st *nlHoverState,
	onDismiss func(),
) {
	if st == nil || st.sug == nil {
		return
	}
	sugs := st.sugs
	if len(sugs) == 0 {
		sugs = []*NLSuggestion{st.sug}
	}
	if st.activeIdx < 0 || st.activeIdx >= len(sugs) {
		st.activeIdx = 0
	}
	st.sug = sugs[st.activeIdx]

	ui.Column(c).Width(float32(tok.Spacing48)*8.2).Gap(float32(tok.Spacing8)).Children(func() {
		if len(sugs) > 1 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				inlineBadge(c, fmt.Sprintf("检测到 %d 个并行动作", len(sugs)), components.BadgeOptions{Tone: components.BadgeAccent})
				for i, item := range sugs {
					i, item := i, item
					isSel := i == st.activeIdx
					tab := ui.ButtonBase(c.Key(fmt.Sprintf("nl-mtab:%d", i))).
						Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
						AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label(fmt.Sprintf("切换第 %d 个动作：%s", i+1, item.KindLabel))
					fg := tok.TextMuted
					if isSel {
						tab.Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
						fg = tok.Accent
					} else {
						tab.Background(tok.Elevated).Border(1, tok.Border.Alpha(0.75))
					}
					tab.Children(func() {
						ui.Text(c, fmt.Sprintf("%d. %s", i+1, item.KindLabel)).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(fg).SingleLine()
					})
					if tab.Clicked() {
						st.activeIdx = i
						st.sug = item
					}
				}
			})
		}

		v.renderNLSuggestionPopoverInner(c, tok, st.sug, sugs, st.globalStart, st.globalEnd, onDismiss)
	})
}

func (v *View) renderNLSuggestionPopover(
	c *ui.Context,
	tok theme.Tokens,
	sug *NLSuggestion,
	globalLineStart int,
	globalLineEnd int,
	onDismiss func(),
) {
	if sug == nil {
		return
	}
	ui.Column(c).Width(float32(tok.Spacing48)*8.2).Gap(float32(tok.Spacing8)).Children(func() {
		v.renderNLSuggestionPopoverInner(c, tok, sug, []*NLSuggestion{sug}, globalLineStart, globalLineEnd, onDismiss)
	})
}

func (v *View) renderNLSuggestionPopoverInner(
	c *ui.Context,
	tok theme.Tokens,
	sug *NLSuggestion,
	allSugs []*NLSuggestion,
	globalLineStart int,
	globalLineEnd int,
	onDismiss func(),
) {
	t := c.Theme()
	dark := t != nil && t.Dark
	if sug == nil {
		return
	}

	ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
		// 1. Header row: Sparkles + Component Kind + Inherited Badge + Confidence Badge
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Row(c).Size(float32(tok.Spacing24), float32(tok.Spacing24)).Center().
				Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Shrink(0).Children(func() {
				ui.Icon(c, lucide.Sparkles()).Size(float32(tok.Spacing12)+2, float32(tok.Spacing12)+2).TextColor(tok.Accent)
			})
			ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
				ui.Text(c, sug.KindLabel).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
				subTxt := "Alt 悬停语义识别 · 预填参数预览"
				if sug.InheritedFrom != "" {
					subTxt = "已自动继承上下文 " + sug.InheritedFrom
				}
				ui.Text(c, subTxt).FontSize(float32(tok.TextMeta)-1).TextColor(tok.TextMuted).SingleLine().Ellipsis("…")
			})
			if sug.InheritedFrom != "" {
				inlineBadge(c, "继承 "+sug.InheritedFrom, components.BadgeOptions{Tone: components.BadgeSuccess})
			}
			confText := fmt.Sprintf("%s · %.0f%%", sug.Source, sug.Confidence*100)
			inlineBadge(c, confText, components.BadgeOptions{Tone: components.BadgeAccent})
		})

		// 2. Mini Visual Component Diagram (简易组件渲染图)
		ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).
			Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
			Gap(float32(tok.Spacing8)).Children(func() {
			switch sug.Kind {
			case "timer":
				v.renderMiniTimerDiagram(c, tok, sug)
			case "calculator":
				v.renderMiniCalcDiagram(c, tok, sug)
			case "plate_group":
				v.renderMiniPlateDiagram(c, tok, sug, dark)
			case "attachment":
				v.renderMiniAttachmentDiagram(c, tok, sug)
			}
		})

		// 3. Extracted Pre-filled Parameter Chips (with explicit amber fallback highlight for missing slots)
		if len(sug.Params) > 0 {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				for _, kv := range sug.Params {
					kv := kv
					chipBg := tok.Surface
					chipBorder := tok.Border.Alpha(0.75)
					valColor := tok.Text
					if kv.IsDefault {
						chipBg = tok.WarningBg
						chipBorder = tok.Warning.Alpha(0.55)
						valColor = tok.WarningText
					}
					ui.Row(c).Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).Background(chipBg).Border(1, chipBorder).
						AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Text(c, kv.Label).FontSize(float32(tok.TextMeta)-1).TextColor(tok.TextMuted).SingleLine()
						ui.Text(c, kv.Value).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(valColor).FontFeatures("tnum").SingleLine()
						if kv.IsDefault {
							ui.Text(c, "待补").FontSize(float32(tok.TextMeta)-2).Bold().TextColor(tok.WarningText).SingleLine()
						}
					})
				}
			})
		}

		// 3b. Missing-Parameter Fallback Banner (缺参兜底提示) or Formula-Uncertain Human-in-the-Loop Banner
		if len(sug.MissingSlots) > 0 {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing4)*1.2, float32(tok.Spacing8)).
				Radius(float32(tok.ControlRadius)).Background(tok.WarningBg).Border(1, tok.Warning.Alpha(0.45)).
				AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				ui.Icon(c, lucide.CircleAlert()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).TextColor(tok.WarningText).Shrink(0)
				msg := fmt.Sprintf("缺参兜底：未写明 %s，已填占位值，插入后可直接修改", strings.Join(sug.MissingSlots, "、"))
				ui.Text(c, msg).FontSize(float32(tok.TextMeta)-1).TextColor(tok.WarningText).Grow(1).MinWidth(0)
			})
		} else if sug.FormulaUncertain {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing4)*1.2, float32(tok.Spacing8)).
				Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35)).
				AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				ui.Icon(c, lucide.BookOpen()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).TextColor(tok.Accent).Shrink(0)
				ui.Text(c, "已提取关键科学变量；公式未确定，插入后可在「公式库」一键套用模板").
					FontSize(float32(tok.TextMeta)-1).TextColor(tok.Accent).Grow(1).MinWidth(0)
			})
		}

		// 4. Action Footer: [✨ 下方插入组件] + optional [全部插入 (N)] + [替换原句]
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing4)*0.5, 0, 0, 0).Children(func() {
			if len(allSugs) > 1 {
				insAllBtn := ui.ButtonBase(c.Key("nl-ins-all")).Grow(1).Height(float32(tok.Spacing24)+float32(tok.Spacing4)).
					Center().Radius(float32(tok.ControlRadius)).Background(tok.Accent).Label(fmt.Sprintf("全部插入 (%d 个组件)", len(allSugs)))
				if insAllBtn.Hovered() {
					insAllBtn.Background(tok.Accent.Alpha(0.88))
				}
				insAllBtn.Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Icon(c, lucide.Plus()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).TextColor(tok.AccentText)
						ui.Text(c, fmt.Sprintf("全部插入 (%d)", len(allSugs))).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.AccentText).SingleLine()
					})
				})
				if insAllBtn.Clicked() && v.Doc != nil && v.OnChange != nil {
					var allMods []map[string]any
					for _, s := range allSugs {
						allMods = append(allMods, s.Modules...)
					}
					if next, err := ApplyNLSuggestionToSource(v.Doc.Source, globalLineStart, globalLineEnd, false, allMods); err == nil {
						v.OnChange(next)
					}
					if onDismiss != nil {
						onDismiss()
					}
				}
			}

			insBtn := ui.ButtonBase(c.Key("nl-ins-below")).Grow(1).Height(float32(tok.Spacing24)+float32(tok.Spacing4)).
				Center().Radius(float32(tok.ControlRadius)).Label("下方插入组件")
			if len(allSugs) > 1 {
				insBtn.Background(tok.Surface).Border(1, tok.Border)
				if insBtn.Hovered() {
					insBtn.Background(tok.Elevated)
				}
				insBtn.Children(func() {
					ui.Text(c, "仅插当前").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).SingleLine()
				})
			} else {
				insBtn.Background(tok.Accent)
				if insBtn.Hovered() {
					insBtn.Background(tok.Accent.Alpha(0.88))
				}
				insBtn.Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Icon(c, lucide.Plus()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).TextColor(tok.AccentText)
						ui.Text(c, "下方插入组件").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.AccentText).SingleLine()
					})
				})
			}
			if insBtn.Clicked() && v.Doc != nil && v.OnChange != nil {
				if next, err := ApplyNLSuggestionToSource(v.Doc.Source, globalLineStart, globalLineEnd, false, sug.Modules); err == nil {
					v.OnChange(next)
				}
				if onDismiss != nil {
					onDismiss()
				}
			}

			repBtn := ui.ButtonBase(c.Key("nl-replace-line")).Height(float32(tok.Spacing24)+float32(tok.Spacing4)).
				Padding(0, float32(tok.Spacing12)).Center().Radius(float32(tok.ControlRadius)).
				Background(tok.Surface).Border(1, tok.Border).Label("替换原句")
			if repBtn.Hovered() {
				repBtn.Background(tok.Elevated)
			}
			repBtn.Children(func() {
				ui.Text(c, "替换原句").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
			})
			if repBtn.Clicked() && v.Doc != nil && v.OnChange != nil {
				if next, err := ApplyNLSuggestionToSource(v.Doc.Source, globalLineStart, globalLineEnd, true, sug.Modules); err == nil {
					v.OnChange(next)
				}
				if onDismiss != nil {
					onDismiss()
				}
			}
		})
	})
}

func (v *View) renderMiniTimerDiagram(c *ui.Context, tok theme.Tokens, sug *NLSuggestion) {
	if len(sug.Modules) == 0 {
		return
	}
	m := sug.Modules[0]
	mode := textValue(m, "mode")
	title := firstNonEmpty(textValue(m, "title"), "实验计时")
	plannedSec, _, _ := timerBudgetSec(m)

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
		ui.Icon(c, lucide.Timer()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
		clockStr := formatClockHMS(plannedSec)
		if mode == "stopwatch" {
			clockStr = "00:00"
		}
		ui.Text(c, clockStr).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
		ui.Text(c, title).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
	})

	if mode == "cycle" {
		phases := mapSlice(m["phases"])
		rounds := int(numOrZero(m["rounds"]))
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			for i, ph := range phases {
				if i > 0 {
					ui.Text(c, "→").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				}
				pname := textValue(ph, "name")
				pv, pu, _ := quantityParts(ph["duration"])
				ui.Row(c).Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).
					Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Accent.Alpha(0.32)).
					Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, pname).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(tok.Text).SingleLine()
					ui.Text(c, fmt.Sprintf("%s %s", formatNumber(pv), pu)).FontSize(float32(tok.TextMeta)-1).TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
				})
			}
			ui.Row(c).Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).
				Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Children(func() {
				ui.Text(c, fmt.Sprintf("× %d 轮", rounds)).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
			})
		})
	} else {
		ui.Text(c, sug.Summary).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
	}
}

func (v *View) renderMiniCalcDiagram(c *ui.Context, tok theme.Tokens, sug *NLSuggestion) {
	if len(sug.Modules) == 0 {
		return
	}
	m := sug.Modules[0]
	rows := mapSlice(m["rows"])
	res := EvalCalculatorWithActuals(v.Doc, m, nil)

	// Find primary formula row to render as a crisp 2D LaTeX equation
	for idx, rm := range rows {
		if fStr := strings.TrimSpace(textValue(rm, "formula")); fStr != "" {
			rkey := firstNonEmpty(textValue(rm, "key"), "y")
			runit := textValue(rm, "unit")
			amtStr := "—"
			if idx < len(res.Rows) && res.Err == nil {
				amtStr = formatNumber(res.Rows[idx].Amount)
				if runit != "" {
					amtStr += " " + DisplayUnit(runit)
				}
			}
			ui.Row(c).FillWidth().Justify(ui.Center).AlignItems(ui.Center).Wrap().
				Padding(float32(tok.Spacing4), 0).Gap(float32(tok.Spacing8)).Children(func() {
				RenderMathNode(c, tok, makeIdentNode(rkey), float32(tok.TextData)+1, tok.Text)
				ui.Text(c, "=").FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
				if ast, err := ParseLatexMath(fStr); err == nil {
					RenderMathNode(c, tok, ast, float32(tok.TextData)+1, tok.Text)
				}
				ui.Text(c, "=").FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
				ui.Row(c).Padding(float32(tok.Spacing4)*0.5, float32(tok.Spacing8)).
					Radius(float32(tok.ControlRadius)).Background(tok.AccentSoftBg).Children(func() {
					ui.Text(c, amtStr).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
				})
			})
			break
		}
	}
	ui.Row(c).FillWidth().Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
		Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border.Alpha(0.6)).
		AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
		ui.Icon(c, lucide.FlaskConical()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
		ui.Text(c, sug.Summary).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(tok.Text).FontFeatures("tnum")
	})
}

func (v *View) renderMiniPlateDiagram(c *ui.Context, tok theme.Tokens, sug *NLSuggestion, dark bool) {
	if len(sug.Modules) < 2 {
		return
	}
	gtMod := sug.Modules[0]
	plMod := sug.Modules[1]
	gtRows := mapSlice(gtMod["rows"])
	format := int(numOrZero(plMod["format"]))
	geom, ok := geometryForPlate(format, "", 0, 0)
	if !ok {
		geom, _ = geometryForPlate(12, "", 0, 0)
	}
	idxMap := map[string]int{}
	for i, rm := range gtRows {
		idxMap[textValue(rm, "id")] = i
	}
	wellGrp := map[string]string{}
	if gm, ok := plMod["groups"].(map[string]any); ok {
		for gid, rawW := range gm {
			for _, w := range calcStringSlice(rawW) {
				wellGrp[strings.ToUpper(w)] = gid
			}
		}
	}

	// Render crisp miniature colored well dot matrix (clamped to max 6x8 visual preview for 96-well)
	showRows := min(geom.Rows, 6)
	showCols := min(geom.Columns, 8)
	dotSz := float32(tok.Spacing16)
	if showCols > 6 {
		dotSz = float32(tok.Spacing12)
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
		ui.Column(c).Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
			Background(tok.Surface).Border(1, tok.Border).Gap(float32(tok.Spacing4)).Shrink(0).Children(func() {
			for r := 0; r < showRows; r++ {
				r := r
				ui.Row(c).Gap(float32(tok.Spacing4)).Children(func() {
					for col := 1; col <= showCols; col++ {
						w := fmt.Sprintf("%s%d", rowLabel(r), col)
						bg := tok.Elevated
						if gid, assigned := wellGrp[w]; assigned {
							bg = plateui.GroupColorIndexed(gid, idxMap, dark)
						}
						ui.Box(c).Size(dotSz, dotSz).Radius(dotSz / 2).Background(bg).Border(1, tok.Border.Alpha(0.5))
					}
				})
			}
		})
		// Group legend column
		ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4)).Children(func() {
			for i, rm := range gtRows {
				if i >= 4 {
					break
				}
				gid := textValue(rm, "id")
				gname := textValue(rm, "name")
				ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Box(c).Size(float32(tok.Spacing8), float32(tok.Spacing8)).Radius(float32(tok.Spacing4)).
						Background(plateui.GroupColorIndexed(gid, idxMap, dark)).Shrink(0)
					ui.Text(c, gname).FontSize(float32(tok.TextMeta)-1).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
				})
			}
		})
	})
}

func (v *View) renderMiniAttachmentDiagram(c *ui.Context, tok theme.Tokens, sug *NLSuggestion) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
		ui.Column(c).Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
			Background(tok.Surface).Border(1, tok.Border).Gap(3).Shrink(0).Children(func() {
			for r := 0; r < 3; r++ {
				ui.Row(c).Gap(3).Children(func() {
					for col := 0; col < 3; col++ {
						ui.Box(c).Size(float32(tok.Spacing12), float32(tok.Spacing12)).Radius(3).
							Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.35))
					}
				})
			}
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4)).Children(func() {
			ui.Text(c, "孔板网格采图槽位").FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text)
			ui.Text(c, sug.Summary).FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).FontFeatures("tnum")
		})
	})
}
