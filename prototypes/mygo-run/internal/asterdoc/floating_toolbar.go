package asterdoc

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
)

// RequestFocusTailProse schedules keyboard focus to land on the writable trailing
// Markdown prose segment on the next frame (e.g., right after inserting a module via `/`).
func (v *View) RequestFocusTailProse() {
	v.pendingFocusLiveKey = "__tail__"
	v.pendingInjectSlash = false
}

// TriggerSlashFromToolbar focuses the active (or trailing) writable Markdown editor,
// injects `/` on a clean line if needed, and immediately opens the unified Slash popup.
func (v *View) TriggerSlashFromToolbar() {
	if v.LabMode || v.ReadOnly {
		return
	}
	target := v.activeWritableLiveKey()
	if target == "" {
		target = "__tail__"
	}
	v.pendingFocusLiveKey = target
	v.pendingInjectSlash = true
}

func (v *View) activeWritableLiveKey() string {
	if v.liveEditors == nil {
		return ""
	}
	// Prefer whichever writable live editor currently has keyboard focus
	for k, ent := range v.liveEditors {
		if ent != nil && ent.state != nil && !ent.state.ReadOnly && ent.state.Focused() {
			return k
		}
	}
	// Fallback to the last focused writable editor if still mounted
	if v.lastFocusedLiveKey != "" {
		if ent, ok := v.liveEditors[v.lastFocusedLiveKey]; ok && ent != nil && ent.state != nil && !ent.state.ReadOnly {
			return v.lastFocusedLiveKey
		}
	}
	if v.lastActiveTailKey != "" {
		if ent, ok := v.liveEditors[v.lastActiveTailKey]; ok && ent != nil && ent.state != nil && !ent.state.ReadOnly {
			return v.lastActiveTailKey
		}
	}
	return ""
}

// RenderFloatingToolbar draws the ultra-lightweight centered floating icon-only toolbar
// (5 Markdown formatting icons | 1 unified component `+` trigger that opens the Slash palette).
// In Lab Mode (locked == true), it renders in-place in a muted disabled state with zero layout jump.
func (v *View) RenderFloatingToolbar(c *ui.Context, locked bool) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	dark := t != nil && t.Dark

	bgCol := tok.Surface
	borderCol := tok.Border
	if dark {
		borderCol = tok.BorderStrong
	}

	bar := ui.Row(c.Key("doc-floating-toolbar")).
		AlignItems(ui.Center).
		Gap(float32(tok.Spacing4)).
		Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
		Radius(float32(tok.PanelRadius)).
		Background(bgCol).
		Border(1, borderCol).
		Shadow(0, float32(tok.Spacing4), float32(tok.Spacing16), 0, tok.Text.Alpha(0.10))

	bar.Children(func() {
		toolBtn := func(key string, icon *ui.SVG, tip string, accent bool, onClick func()) {
			btn := components.IconButton(c.Key("ftb-"+key), icon, tip, components.IconButtonCompact)
			btn.KeepFocus()
			if locked {
				btn.Disabled(true).Opacity(0.45)
				components.Tooltip(c, btn, "实验模式已锁定排版，切换编辑模式可修改")
				return
			}
			if accent {
				btn.TextColor(tok.Accent)
			}
			components.Tooltip(c, btn, tip)
			if btn.Clicked() && onClick != nil {
				onClick()
			}
		}

		// Group A: 5 high-frequency Markdown formatting icons (icon-only, hover tooltips)
		toolBtn("heading", lucide.Heading(), "标题层级 H1–H3 (#)", false, func() {
			v.applyMarkdownLineTransform(cycleMarkdownHeading)
		})
		toolBtn("bold", lucide.Bold(), "加粗 (Ctrl+B)", false, func() {
			v.applyMarkdownInlineWrap("**", "**")
		})
		toolBtn("italic", lucide.Italic(), "斜体 (Ctrl+I)", false, func() {
			v.applyMarkdownInlineWrap("*", "*")
		})
		toolBtn("bullet", lucide.List(), "无序列表 (- )", false, func() {
			v.applyMarkdownLineTransform(func(line string) string {
				return toggleMarkdownPrefix(line, "- ")
			})
		})
		toolBtn("todo", lucide.ListChecks(), "待办清单 (- [ ])", false, func() {
			v.applyMarkdownLineTransform(func(line string) string {
				return toggleMarkdownPrefix(line, "- [ ] ")
			})
		})

		// 1px vertical hairline divider between Markdown formatting and Component Inserter
		ui.Box(c).Width(1).Height(float32(tok.Spacing16)).Margin(0, float32(tok.Spacing4)).Background(tok.Border)

		// Group B: Single unified component inserter button -> summons the exact same `/` Slash popup
		toolBtn("insert", lucide.Plus(), "插入科学组件 / 步骤 / 图片 (/)", true, func() {
			v.TriggerSlashFromToolbar()
		})
	})
}

func (v *View) applyMarkdownLineTransform(fn func(line string) string) {
	key := v.activeWritableLiveKey()
	if key == "" {
		return
	}
	ent := v.liveEditors[key]
	if ent == nil || ent.state == nil || ent.state.Buffer == nil || ent.state.ReadOnly {
		return
	}
	st := ent.state
	full := st.Buffer.String()
	cursor := min(max(0, st.Cursor()), len(full))
	lineStart := strings.LastIndexByte(full[:cursor], '\n') + 1
	lineEnd := len(full)
	if nl := strings.IndexByte(full[lineStart:], '\n'); nl >= 0 {
		lineEnd = lineStart + nl
	}
	origLine := full[lineStart:lineEnd]
	nextLine := fn(origLine)
	if nextLine == origLine {
		return
	}
	delta := len(nextLine) - len(origLine)
	st.Buffer.Edit(editor.Edit{Start: lineStart, End: lineEnd, Text: nextLine})
	nextCursor := max(lineStart, cursor+delta)
	st.Select(nextCursor, nextCursor)
}

func (v *View) applyMarkdownInlineWrap(openMark, closeMark string) {
	key := v.activeWritableLiveKey()
	if key == "" {
		return
	}
	ent := v.liveEditors[key]
	if ent == nil || ent.state == nil || ent.state.Buffer == nil || ent.state.ReadOnly {
		return
	}
	st := ent.state
	full := st.Buffer.String()
	sel := st.Selection()
	a, b := sel.Start(), sel.End()
	if a > b {
		a, b = b, a
	}
	a = min(max(0, a), len(full))
	b = min(max(0, b), len(full))
	// Toggle unwrap if selection is already wrapped by openMark..closeMark
	if a >= len(openMark) && b+len(closeMark) <= len(full) &&
		full[a-len(openMark):a] == openMark && full[b:b+len(closeMark)] == closeMark {
		inner := full[a:b]
		st.Buffer.Edit(editor.Edit{Start: a - len(openMark), End: b + len(closeMark), Text: inner})
		st.Select(a-len(openMark), b-len(openMark))
		return
	}
	selected := full[a:b]
	if strings.HasPrefix(selected, openMark) && strings.HasSuffix(selected, closeMark) && len(selected) >= len(openMark)+len(closeMark) {
		inner := selected[len(openMark) : len(selected)-len(closeMark)]
		st.Buffer.Edit(editor.Edit{Start: a, End: b, Text: inner})
		st.Select(a, a+len(inner))
		return
	}
	wrapped := openMark + selected + closeMark
	st.Buffer.Edit(editor.Edit{Start: a, End: b, Text: wrapped})
	if a == b {
		// Place caret inside empty markers (e.g., `**|**`)
		st.Select(a+len(openMark), a+len(openMark))
	} else {
		// Keep original text selected inside markers so user can stack Bold + Italic effortlessly
		st.Select(a+len(openMark), b+len(openMark))
	}
}

func cycleMarkdownHeading(line string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	switch {
	case strings.HasPrefix(trimmed, "### "):
		return indent + strings.TrimPrefix(trimmed, "### ")
	case strings.HasPrefix(trimmed, "## "):
		return indent + "### " + strings.TrimPrefix(trimmed, "## ")
	case strings.HasPrefix(trimmed, "# "):
		return indent + "## " + strings.TrimPrefix(trimmed, "# ")
	default:
		return indent + "# " + trimmed
	}
}

func toggleMarkdownPrefix(line, prefix string) string {
	trimmed := strings.TrimLeft(line, " \t")
	indent := line[:len(line)-len(trimmed)]
	if strings.HasPrefix(trimmed, prefix) {
		return indent + strings.TrimPrefix(trimmed, prefix)
	}
	// If toggling `- [ ] ` over `- ` (or vice versa), replace cleanly
	if prefix == "- [ ] " {
		if strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ") {
			return indent + strings.TrimPrefix(strings.TrimPrefix(trimmed, "- [x] "), "- [X] ")
		}
		if strings.HasPrefix(trimmed, "- ") {
			return indent + "- [ ] " + strings.TrimPrefix(trimmed, "- ")
		}
	}
	if prefix == "- " && (strings.HasPrefix(trimmed, "- [ ] ") || strings.HasPrefix(trimmed, "- [x] ") || strings.HasPrefix(trimmed, "- [X] ")) {
		rest := strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(trimmed, "- [ ] "), "- [x] "), "- [X] ")
		return indent + "- " + rest
	}
	return indent + prefix + trimmed
}
