package asterdoc

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
	plateui "asterlab-mygo-prototype/internal/uikit/plate"
)

// SlashCommandItem represents one host-registered command exposed in Live Preview `/`
// menus, inter-block `+` menus, and mobile insert drawers (populated from astercmd.Registry).
type SlashCommandItem struct {
	CommandID   string
	Label       string
	Detail      string
	Slash       string
	Kind        string
	Preset      string
	SearchTerms string
}

type liveEditorEntry struct {
	state                *editor.State
	syncedText           string
	lastOwnedSource      string
	lastVersion          uint64
	lastActiveLn         int
	lastFocused          bool
	lastDark             bool
	lastReadOnly         bool
	lastCompletionVer    uint64
	dismissedSlashVer    uint64
	toolbarSlashStart    int
	toolbarSlashLen      int
	toolbarSlashActive   bool
	wasFocused           bool
	docOffset            int
	baseRepls            []editor.Replacement
	lastNLTextSig        string
}

var (
	reMDHeading   = regexp.MustCompile(`^(#{1,4})([ \t]+)`)
	reMDTask      = regexp.MustCompile(`^([ \t]*[-*+][ \t]+)\[([ xX])\]`)
	reMDBullet    = regexp.MustCompile(`^([ \t]*)([-*+])([ \t]+)`)
	reMDBold      = regexp.MustCompile(`\*\*([^*\r\n]+?)\*\*`)
	reMDItalic    = regexp.MustCompile(`(?:^|[^*])\*([^*\r\n]+?)\*(?:[^*]|$)`)
	reMDCode      = regexp.MustCompile("`([^`\r\n]+?)`")
	reMDStrike    = regexp.MustCompile(`~~([^~\r\n]+?)~~`)
	reWikiBracket = regexp.MustCompile(`(!?)\[\[([^\]|\r\n]+?)(?:\|([^\]\r\n]*))?\]\]`)
)

func liveEditorTheme(t *ui.Theme, tok theme.Tokens) *editor.Theme {
	ct := editor.DefaultTheme(t)
	ct.Background = tok.Canvas
	ct.Text = tok.Text
	ct.Cursor = tok.Accent
	ct.Selection = tok.Accent.Alpha(0.22)
	ct.InactiveSelection = tok.Accent.Alpha(0.12)
	ct.ActiveLine = ui.Transparent
	ct.Error = tok.Danger
	ct.Warning = tok.Warning
	ct.Info = tok.Accent
	ct.Hint = tok.TextMuted
	ct.Tokens[editor.TokenHeading] = editor.TokenStyle{Color: tok.Text, Bold: true}
	ct.Tokens[editor.TokenStrong] = editor.TokenStyle{Color: tok.Text, Bold: true}
	ct.Tokens[editor.TokenEmphasis] = editor.TokenStyle{Color: tok.Text, Italic: true}
	ct.Tokens[editor.TokenLink] = editor.TokenStyle{Color: tok.Accent}
	ct.Tokens[editor.TokenCode] = editor.TokenStyle{Color: tok.Accent}
	ct.Tokens[editor.TokenComment] = editor.TokenStyle{Color: tok.TextMuted}
	return ct
}

// ShouldTriggerSlashAt reports whether a `/` completion menu should open at cursor offset
// in text. It triggers ONLY at the beginning of a line (optional leading whitespace) followed
// by `/` and word/query characters — never inside `1/1000`, file paths, URLs, or code.
func ShouldTriggerSlashAt(text string, cursor int) (startOff int, query string, ok bool) {
	if cursor <= 0 || cursor > len(text) {
		return 0, "", false
	}
	lineStart := strings.LastIndexByte(text[:cursor], '\n') + 1
	before := text[lineStart:cursor]
	trimmedLeft := strings.TrimLeft(before, " \t")
	if !strings.HasPrefix(trimmedLeft, "/") {
		return 0, "", false
	}
	slashIdx := lineStart + (len(before) - len(trimmedLeft))
	q := text[slashIdx+1 : cursor]
	// Reject path-like or numeric-like strings containing another '/' or whitespace
	if strings.ContainsAny(q, "/ \t\\:") {
		return 0, "", false
	}
	return slashIdx, q, true
}

// ComputeLivePreviewDecorations builds MyGo editor LineStyles, Replacements, and Diagnostics
// for Obsidian-style Live Preview: non-active lines hide Markdown syntax markers, render
// headings at display sizes, replace `- [ ]`/`- [x]` with interactive native checkboxes, and
// replace `{@...}` scientific relation blocks with interactive chips; the active cursor line
// reveals raw Markdown syntax for frictionless editing.
func ComputeLivePreviewDecorations(
	text string,
	activeLine int,
	focused bool,
	t *ui.Theme,
	tok theme.Tokens,
	doc *Document,
	docOffset int,
	allowCheckToggle bool,
	onToggleCheck func(start, end int, checked bool),
	onClickTarget func(objID, sub string),
) (styles []editor.LineStyle, repls []editor.Replacement, diags []editor.Diagnostic) {
	fontFamily := "sans-serif"
	if t != nil && t.Font != "" {
		fontFamily = t.Font
	}
	lines := strings.SplitAfter(text, "\n")
	lineStart := 0
	inCodeFence := false
	fenceTicks := 0

	var resolvedAll []ResolvedRelation
	var stepTree *StepTree
	if doc != nil {
		resolvedAll = ResolveRelations(doc)
		stepTree = ParseStepTree(doc.Source, doc)
	}
	resolvedBySpan := make(map[int]ResolvedRelation, len(resolvedAll))
	for _, rr := range resolvedAll {
		if rr.SourceKind == "inline" {
			resolvedBySpan[rr.SpanStart] = rr
		}
	}

	for lineIdx, rawLine := range lines {
		lineText := strings.TrimRight(rawLine, "\r\n")
		trimmed := strings.TrimSpace(lineText)
		isActive := focused && lineIdx == activeLine

		if !inCodeFence {
			if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
				inCodeFence = true
				fenceTicks = ticks
				lineStart += len(rawLine)
				continue
			}
		} else {
			if isClosingBacktickFence(trimmed, fenceTicks) {
				inCodeFence = false
				fenceTicks = 0
			}
			lineStart += len(rawLine)
			continue
		}

		// 1a. AsterML orthogonal hierarchical step header (`#@ 主步骤` / `##@ 子步骤`)
		if hs, isStep := ParseHeadingStepLine(lineText); isStep {
			var sn *StepNode
			if stepTree != nil {
				sn = stepTree.ByLineStart[docOffset+lineStart]
			}
			isMainStep := sn == nil || sn.Depth == 0
			fSize := float32(tok.TextSection)
			lh := float32(1.48)
			weight := 600
			if isMainStep {
				fSize = float32(tok.TextSection) + 2
				lh = 1.56
				weight = 700
			}
			styles = append(styles, editor.LineStyle{
				Line:       lineIdx,
				Font:       ui.Font{Family: fontFamily, Size: fSize, Weight: weight},
				LineHeight: lh,
			})
			if !isActive {
				pillLabel := "STEP 1"
				if sn != nil {
					if sn.Depth == 0 {
						pillLabel = "STEP " + sn.Number
					} else {
						pillLabel = sn.Number
					}
				}
				if isMainStep {
					// Scheme B Mainline Stage Header Pill: accent-tinted badge with 3px bold left stripe
					pillW := estimateStepPillWidth(pillLabel, float32(tok.TextMeta)) + float32(tok.Spacing4)
					repls = append(repls, editor.Replacement{
						Start: lineStart,
						End:   lineStart + hs.PrefixEnd,
						Width: pillW,
						Label: pillLabel,
						Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
							e.Row().AlignItems(ui.Center).Justify(ui.Start).Padding(0, float32(tok.Spacing8), 0, 0).Children(func() {
								ui.Row(c).Height(float32(tok.Spacing24)).
									AlignItems(ui.Center).AlignSelf(ui.Center).
									Radius(float32(tok.ControlRadius)).
									Background(tok.AccentSoftBg).Border(1, tok.Accent.Alpha(0.32)).
									Clip().Children(func() {
									ui.Box(c).Width(3).FillHeight().Background(tok.Accent)
									ui.Row(c).Padding(0, float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
										ui.Text(c, pillLabel).FontSize(float32(tok.TextMeta)).Bold().
											TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
									})
								})
							})
						},
					})
				} else {
					// Scheme B2 Sub-Step Header Pill: crisp quiet muted number pill (continuous rail is drawn by container!)
					pillW := estimateStepPillWidth(pillLabel, float32(tok.TextMeta)) - float32(tok.Spacing4)
					repls = append(repls, editor.Replacement{
						Start: lineStart,
						End:   lineStart + hs.PrefixEnd,
						Width: pillW,
						Label: pillLabel,
						Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
							e.Row().AlignItems(ui.Center).Justify(ui.Start).Padding(0, float32(tok.Spacing8), 0, 0).Children(func() {
								ui.Row(c).Height(float32(tok.Spacing24)-4).Padding(0, float32(tok.Spacing8)*0.85).
									AlignItems(ui.Center).AlignSelf(ui.Center).
									Radius(float32(tok.ControlRadius)).
									Background(tok.Elevated).Border(1, tok.BorderStrong.Alpha(0.7)).Children(func() {
									ui.Text(c, pillLabel).FontSize(float32(tok.TextMeta)).Bold().
										TextColor(tok.TextMuted).FontFeatures("tnum").SingleLine()
								})
							})
						},
					})
				}
				if hs.AttrStart >= 0 && hs.AttrEnd > hs.AttrStart {
					attrChip := ""
					if hs.Anchor != "" {
						attrChip = "#" + hs.Anchor
					} else if hs.Optional {
						attrChip = "可选"
					}
					if attrChip != "" {
						aW := estimateStepPillWidth(attrChip, float32(tok.TextMeta)) + float32(tok.Spacing4)
						repls = append(repls, editor.Replacement{
							Start: lineStart + hs.AttrStart,
							End:   lineStart + hs.AttrEnd,
							Width: aW,
							Label: attrChip,
							Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
								e.Row().AlignItems(ui.Center).Padding(0, 0, 0, float32(tok.Spacing4)).Children(func() {
									ui.Row(c).Height(float32(tok.Spacing24)-4).Padding(0, float32(tok.Spacing8)*0.75).
										AlignItems(ui.Center).AlignSelf(ui.Center).
										Radius(float32(tok.ControlRadius)).
										Background(tok.Elevated).Border(1, tok.Border).Children(func() {
										ui.Text(c, attrChip).Font("monospace").FontSize(float32(tok.TextMeta)).
											TextColor(tok.TextMuted).SingleLine()
									})
								})
							},
						})
					} else {
						repls = append(repls, editor.Replacement{
							Start: lineStart + hs.AttrStart,
							End:   lineStart + hs.AttrEnd,
						})
					}
				}
			}
			lineStart += len(rawLine)
			continue
		}

		// 1a-hr. Thematic break (`---`) -> sleek horizontal divider closing preceding step cleanly
		if reThematicBreakLine.MatchString(lineText) && !isActive {
			repls = append(repls, editor.Replacement{
				Start: lineStart,
				End:   lineStart + len(lineText),
				Width: float32(tok.Spacing48) * 10,
				Label: "分隔线",
				Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
					e.Row().FillWidth().AlignItems(ui.Center).Children(func() {
						ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.75))
					})
				},
			})
			lineStart += len(rawLine)
			continue
		}

		// 1b. Plain Headings (# .. ####) -> Editorial typographic scale + crisp breathing
		if m := reMDHeading.FindStringSubmatchIndex(lineText); m != nil {
			level := m[3] - m[2]
			var fSize float32
			var lh float32 = 1.45
			switch level {
			case 1:
				fSize = float32(tok.TextPage)
				lh = 1.5
			case 2:
				fSize = float32(tok.TextSection) + 2
				lh = 1.48
			case 3:
				fSize = float32(tok.TextSection)
				lh = 1.45
			default:
				fSize = float32(tok.TextBody) + 0.5
				lh = 1.45
			}
			styles = append(styles, editor.LineStyle{
				Line:       lineIdx,
				Font:       ui.Font{Family: fontFamily, Size: fSize, Weight: 700},
				LineHeight: lh,
			})
			if !isActive {
				repls = append(repls, editor.Replacement{
					Start: lineStart + m[0],
					End:   lineStart + m[1],
				})
			}
		}

		// 2. Task list `- [ ]` / `- [x]` -> Sleek 14x14 optically-centered checkbox on non-active lines
		if m := reMDTask.FindStringSubmatchIndex(lineText); m != nil {
			if !isActive {
				checked := lineText[m[4]:m[5]] != " "
				boxStart := lineStart + m[0]
				boxEnd := lineStart + m[1]
				repls = append(repls, editor.Replacement{
					Start:       boxStart,
					End:         boxEnd,
					Width:       float32(tok.Spacing24) - 2,
					Interactive: allowCheckToggle,
					Label:       map[bool]string{true: "已完成", false: "未完成"}[checked],
					Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
						e.Row().AlignItems(ui.Center).Justify(ui.Start).Padding(0, float32(tok.Spacing4), 0, 0).Children(func() {
							boxSz := float32(tok.Spacing12) + 2 // crisp 14dp optical checkbox matching 14dp body text
							chkEl := ui.ButtonBase(c).Size(boxSz, boxSz).Radius(float32(tok.Spacing4)).
								AlignSelf(ui.Center).KeepFocus().Label(r.Label)
							if checked {
								chkEl.Background(tok.Accent)
							} else {
								chkEl.Background(tok.Surface).Border(1.5, tok.BorderStrong)
							}
							chkEl.DrawOver(func(p *ui.Painter, rect ui.Rect) {
								if checked {
									var path ui.Path
									path.MoveTo(rect.X+rect.W*0.22, rect.Y+rect.H*0.52).
										LineTo(rect.X+rect.W*0.42, rect.Y+rect.H*0.72).
										LineTo(rect.X+rect.W*0.78, rect.Y+rect.H*0.30)
									p.StrokePath(&path, 1.75, tok.AccentText)
								}
							})
							if allowCheckToggle && onToggleCheck != nil && chkEl.Clicked() {
								onToggleCheck(r.Start, r.End, !checked)
							}
						})
					},
				})
			}
		} else if m := reMDBullet.FindStringSubmatchIndex(lineText); m != nil && !isActive {
			// Bullet `- ` -> crisp `•` dot
			repls = append(repls, editor.Replacement{
				Start: lineStart + m[4],
				End:   lineStart + m[5],
				Text:  "•",
				Color: tok.Accent,
			})
		} else if om := reStepOrderedLine.FindStringSubmatchIndex(lineText); om != nil && !isActive {
			// Clean tabular accent number for ordered list items `1. `
			numTok := strings.TrimSpace(lineText[:om[2]])
			repls = append(repls, editor.Replacement{
				Start: lineStart,
				End:   lineStart + om[2],
				Width: float32(tok.Spacing24),
				Label: numTok,
				Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
					e.Row().AlignItems(ui.Center).Justify(ui.Start).Children(func() {
						ui.Text(c, numTok).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Accent).FontFeatures("tnum").SingleLine()
					})
				},
			})
		}

		// 3. Inline Markdown syntax hiding on non-active lines (**bold**, `code`, ~~strike~~, *italic*)
		if !isActive {
			for _, m := range reMDBold.FindAllStringSubmatchIndex(lineText, -1) {
				repls = append(repls,
					editor.Replacement{Start: lineStart + m[0], End: lineStart + m[0] + 2},
					editor.Replacement{Start: lineStart + m[1] - 2, End: lineStart + m[1]},
				)
			}
			for _, m := range reMDStrike.FindAllStringSubmatchIndex(lineText, -1) {
				repls = append(repls,
					editor.Replacement{Start: lineStart + m[0], End: lineStart + m[0] + 2},
					editor.Replacement{Start: lineStart + m[1] - 2, End: lineStart + m[1]},
				)
			}
			for _, m := range reMDCode.FindAllStringSubmatchIndex(lineText, -1) {
				repls = append(repls,
					editor.Replacement{Start: lineStart + m[0], End: lineStart + m[0] + 1},
					editor.Replacement{Start: lineStart + m[1] - 1, End: lineStart + m[1]},
				)
			}
			for _, m := range reMDItalic.FindAllStringSubmatchIndex(lineText, -1) {
				// m[2]..m[3] is the inner text; '*' sits immediately before m[2] and after m[3]
				if m[2] > 0 && m[3] < len(lineText) {
					repls = append(repls,
						editor.Replacement{Start: lineStart + m[2] - 1, End: lineStart + m[2]},
						editor.Replacement{Start: lineStart + m[3], End: lineStart + m[3] + 1},
					)
				}
			}
			for _, m := range reWikiBracket.FindAllStringSubmatchIndex(lineText, -1) {
				// Hide `[[` and `]]` (and leading `!` if inline)
				openEnd := m[4]
				closeStart := m[5]
				if m[6] >= 0 {
					// Keep alias display if present
					openEnd = m[6]
					closeStart = m[7]
				}
				repls = append(repls,
					editor.Replacement{Start: lineStart + m[0], End: lineStart + openEnd},
					editor.Replacement{Start: lineStart + closeStart, End: lineStart + m[1]},
				)
			}
		}

		// 4. Inline `{@...}` scientific relation attributes (diagnostics always; chip replacement on non-active lines)
		lineRels := ExtractInlineRelations(lineText, true)
		for _, ir := range lineRels {
			if ir.AttrEnd <= ir.AttrStart {
				continue
			}
			localAttrStart := lineStart + ir.AttrStart
			localAttrEnd := lineStart + ir.AttrEnd
			globalSpanStart := docOffset + lineStart + ir.SpanStart
			rr, hasResolved := resolvedBySpan[globalSpanStart]
			hasIssue := hasResolved && len(rr.Issues) > 0
			if hasIssue {
				diags = append(diags, editor.Diagnostic{
					Start:    localAttrStart,
					End:      localAttrEnd,
					Severity: editor.SeverityWarning,
					Message:  strings.Join(rr.Issues, "；"),
					Source:   "科学关联校验",
				})
			}
			if !isActive {
				chipLabel := formatRelationChipLabel(ir, rr, hasResolved)
				chipW := estimateChipWidth(chipLabel, float32(tok.TextMeta))
				bgCol := tok.AccentSoftBg
				fgCol := tok.Accent
				bdCol := tok.Accent.Alpha(0.35)
				if hasIssue {
					bgCol = tok.Warning.Alpha(0.14)
					fgCol = tok.Warning
					bdCol = tok.Warning.Alpha(0.55)
				}
				var clickObj, clickSub string
				if len(rr.Contexts) > 0 {
					clickObj = rr.Contexts[0].ObjectID
					clickSub = rr.Contexts[0].SubTarget
				} else if len(ir.Targets) > 0 {
					clickObj = ir.Targets[0].ObjectID
					if len(ir.Targets[0].SubTargets) > 0 {
						clickSub = ir.Targets[0].SubTargets[0]
					}
				}
				repls = append(repls, editor.Replacement{
					Start:       localAttrStart,
					End:         localAttrEnd,
					Width:       chipW,
					Interactive: true,
					Label:       chipLabel,
					Content: func(c *ui.Context, e ui.Element, r editor.Replacement) {
						e.Row().AlignItems(ui.Center).Children(func() {
							pill := ui.ButtonBase(c).
								Padding(0, float32(tok.Spacing8)).
								Radius(float32(tok.ControlRadius)).
								Background(bgCol).Border(1, bdCol).
								KeepFocus().Label(chipLabel)
							pill.Children(func() {
								ui.Text(c, chipLabel).FontSize(float32(tok.TextMeta)).Bold().TextColor(fgCol).SingleLine()
							})
							if pill.Clicked() && onClickTarget != nil && clickObj != "" {
								onClickTarget(clickObj, clickSub)
							}
						})
					},
				})
			}
		}

		lineStart += len(rawLine)
	}
	return styles, repls, diags
}

func formatRelationChipLabel(ir InlineRelation, rr ResolvedRelation, hasResolved bool) string {
	var parts []string
	if hasResolved && len(rr.Contexts) > 0 {
		for _, tc := range rr.Contexts {
			base := firstNonEmpty(tc.ObjectTitle, tc.ObjectID)
			if tc.SubTarget != "" {
				parts = append(parts, base+"·"+tc.SubTarget)
			} else {
				parts = append(parts, base)
			}
		}
	} else {
		for _, t := range ir.Targets {
			if len(t.SubTargets) > 0 {
				parts = append(parts, t.ObjectID+"·"+strings.Join(t.SubTargets, ","))
			} else {
				parts = append(parts, t.ObjectID)
			}
		}
	}
	if ir.Timepoint != "" {
		parts = append(parts, ir.Timepoint)
	}
	if ir.FOV > 0 {
		parts = append(parts, fmt.Sprintf("FOV%d", ir.FOV))
	}
	if len(parts) == 0 {
		return ir.RawAttr
	}
	return strings.Join(parts, " · ")
}

func estimateStepPillWidth(label string, fontSize float32) float32 {
	var w float32 = 24
	for _, r := range label {
		switch {
		case r > 0x7F:
			w += fontSize * 1.02
		case r == ' ':
			w += fontSize * 0.42
		default:
			w += fontSize * 0.68
		}
	}
	if w < 44 {
		w = 44
	}
	return w
}

func estimateChipWidth(label string, fontSize float32) float32 {
	var w float32 = 22
	for _, r := range label {
		if r > 0x7F {
			w += fontSize * 1.02
		} else {
			w += fontSize * 0.62
		}
	}
	if w < 48 {
		w = 48
	}
	return w
}

// renderLiveMarkdownSegment renders a top-level Markdown prose segment (or step prose@1)
// using MyGo's native editor.View with Obsidian Live Preview decorations, IME-safe `/`
// Slash commands, `{@` scientific target completions, and unified relation Hover cards.
func (v *View) renderLiveMarkdownSegment(
	c *ui.Context,
	key string,
	text string,
	docOffset int,
	readOnly bool,
	allowCheckToggle bool,
	placeholder string,
	stepID string,
	onCommit func(nextText string),
) {
	v.renderLiveMarkdownSegmentFull(c, key, "科学笔记正文", text, docOffset, readOnly, allowCheckToggle, placeholder, stepID, false, func(nextText string) string {
		if onCommit != nil {
			onCommit(nextText)
		}
		return ""
	}, nil)
}

func (v *View) renderLiveMarkdownSegmentFull(
	c *ui.Context,
	key string,
	a11yLabel string,
	text string,
	docOffset int,
	readOnly bool,
	allowCheckToggle bool,
	placeholder string,
	stepID string,
	isTail bool,
	onCommit func(nextText string) (ownedSource string),
	onBlur func(curText string),
) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	if v.liveEditors == nil {
		v.liveEditors = map[string]*liveEditorEntry{}
	}
	if a11yLabel == "" {
		a11yLabel = "科学笔记正文"
	}
	entry := v.liveEditors[key]
	norm := strings.ReplaceAll(text, "\r\n", "\n")
	if entry == nil {
		buf := editor.NewBuffer(norm)
		if lang := editor.LanguageFor("note.md"); lang != nil {
			buf.SetLanguage(lang)
		}
		st := &editor.State{
			Buffer:          buf,
			SoftWrap:        true,
			LineNumbers:     editor.LineNumbersOff,
			FitContent:      true,
			NoScrollbars:    true,
			NoActiveLine:    true,
			NoIndentGuides:  true,
			NoFolding:       true,
			NoAutoClose:     true,
			NoScrollPastEnd: true,
			Placeholder:     placeholder,
			Label:           a11yLabel,
		}
		entry = &liveEditorEntry{
			state:             st,
			syncedText:        norm,
			lastActiveLn:      -1,
			toolbarSlashStart: -1,
			docOffset:         docOffset,
		}
		v.liveEditors[key] = entry
	}
	entry.docOffset = docOffset
	st := entry.state
	st.Label = a11yLabel
	if isTail && !readOnly {
		v.lastActiveTailKey = key
	}

	// Sync external edits (Undo/Redo, Agent CLI writes, or Command Bus mutations) into buffer.
	// NEVER rebuild st.Buffer when v.Doc.Source matches the commit this editor itself just emitted
	// (prevents cursor jumping to offset 0 after first char or Enter).
	curBuf := strings.ReplaceAll(st.Buffer.String(), "\r\n", "\n")
	curDocSrc := ""
	if v.Doc != nil {
		curDocSrc = v.Doc.Source
	}
	isSelfCommit := entry.lastOwnedSource != "" && curDocSrc == entry.lastOwnedSource
	if norm != entry.syncedText && curBuf == entry.syncedText && !isSelfCommit && !entry.toolbarSlashActive {
		st.Buffer = editor.NewBuffer(norm)
		if lang := editor.LanguageFor("note.md"); lang != nil {
			st.Buffer.SetLanguage(lang)
		}
		entry.syncedText = norm
		entry.lastVersion = 0
		entry.lastOwnedSource = ""
		curBuf = norm
	}

	st.ReadOnly = readOnly
	st.Placeholder = placeholder
	st.Theme = liveEditorTheme(t, tok)
	fontFamily := "sans-serif"
	if t != nil && t.Font != "" {
		fontFamily = t.Font
	}
	st.Font = ui.Font{Family: fontFamily, Size: float32(tok.TextBody)}
	st.LineHeight = 1.56

	// Unified Hover Card for inline `{@...}` scientific relations AND instant real Alt+Hover NL->Module preview
	altHeld := c.Modifiers()&ui.Alt != 0
	closeOtherHovers := func() {
		for otherKey, otherEnt := range v.liveEditors {
			if otherKey != key && otherEnt != nil && otherEnt.state != nil {
				otherEnt.state.Run(editor.Cancel)
			}
		}
	}
	lineBoundsForOffset := func(bufStr string, off int) (lineIdx, lnStart, lnEnd int) {
		off = min(max(0, off), len(bufStr))
		lnStart = strings.LastIndexByte(bufStr[:off], '\n') + 1
		lnEnd = len(bufStr)
		if nl := strings.IndexByte(bufStr[off:], '\n'); nl >= 0 {
			lnEnd = off + nl
		}
		lineIdx = strings.Count(bufStr[:lnStart], "\n")
		return lineIdx, lnStart, lnEnd
	}
	lineBoundsForIndex := func(bufStr string, targetLine int) (lnStart, lnEnd int, ok bool) {
		if targetLine < 0 {
			return 0, 0, false
		}
		cur := 0
		pos := 0
		for cur < targetLine {
			nl := strings.IndexByte(bufStr[pos:], '\n')
			if nl < 0 {
				return 0, 0, false
			}
			pos += nl + 1
			cur++
		}
		lnStart = pos
		lnEnd = len(bufStr)
		if nl := strings.IndexByte(bufStr[pos:], '\n'); nl >= 0 {
			lnEnd = pos + nl
		}
		return lnStart, lnEnd, true
	}
	activateNLHoverForLine := func(bufStr string, off int, byShortcut bool) bool {
		if readOnly || v.Doc == nil {
			return false
		}
		lineIdx, lnStart, lnEnd := lineBoundsForOffset(bufStr, off)
		lineText := bufStr[lnStart:lnEnd]
		dec, _ := QueryJevDecisionAsync(lineText, func() {
			if v.OnAsyncInvalidate != nil {
				v.OnAsyncInvalidate()
			}
		})
		jevReady := dec != nil
		// Fast-path: if the exact same line is already active and Jev state didn't just arrive,
		// keep the existing nlHoverState untouched (zero churn, zero Cancel() flicker, zero invalidate storm).
		if v.nlHover != nil &&
			v.nlHover.editorKey == key &&
			v.nlHover.lineIdx == lineIdx &&
			v.nlHover.lineText == lineText &&
			v.nlHover.sug != nil &&
			v.nlHover.jevReady == jevReady {
			v.nlHover.hideAt = time.Time{}
			if byShortcut {
				v.nlHover.byShortcut = true
			}
			return true
		}
		sugs := AnalyzeLabSentenceWithContext(v.Doc, lineText, entry.docOffset+lnStart, dec)
		if len(sugs) == 0 {
			if v.nlHover != nil && v.nlHover.editorKey == key && !byShortcut {
				v.nlHover = nil
			}
			return false
		}
		closeOtherHovers()
		st.Run(editor.Cancel) // keep editor's built-in above-line hover closed so only our below-line prism card shows
		openedAt := c.Now()
		activeIdx := 0
		if v.nlHover != nil && v.nlHover.editorKey == key && v.nlHover.lineIdx == lineIdx && v.nlHover.lineText == lineText {
			openedAt = v.nlHover.openedAt // preserve continuous left-to-right sweep animation phase
			if v.nlHover.activeIdx >= 0 && v.nlHover.activeIdx < len(sugs) {
				activeIdx = v.nlHover.activeIdx
			}
		}
		v.nlHover = &nlHoverState{
			editorKey:   key,
			lineIdx:     lineIdx,
			lineStart:   lnStart,
			lineEnd:     lnEnd,
			globalStart: entry.docOffset + lnStart,
			globalEnd:   entry.docOffset + lnEnd,
			lineText:    lineText,
			sug:         sugs[activeIdx],
			sugs:        sugs,
			activeIdx:   activeIdx,
			openedAt:    openedAt,
			byShortcut:  byShortcut,
			overCard:    v.nlHover != nil && v.nlHover.overCard,
			jevReady:    jevReady,
		}
		c.Invalidate()
		return true
	}

	st.OnHover = func(req editor.HoverRequest) {
		if v.Doc == nil {
			return
		}
		rels := ResolveRelations(v.Doc)
		globalOff := entry.docOffset + req.Offset
		if rr, ok := RelationAtOffset(rels, globalOff); ok {
			closeOtherHovers()
			v.nlHover = nil
			localStart := max(0, rr.SpanStart-entry.docOffset)
			localEnd := min(len(st.Buffer.String()), rr.SpanEnd-entry.docOffset)
			st.ShowHover(localStart, localEnd, "", func(hc *ui.Context) {
				v.renderRelationHoverContent(hc, tok, rr)
			})
			return
		}
		if altHeld {
			activateNLHoverForLine(st.Buffer.String(), req.Offset, false)
		}
	}

	// Deterministic visual-test / demo trigger for Alt+Hover preview dropdown on a matching sentence
	if !readOnly && v.AltHoverPreviewQuery != "" {
		if idx := strings.Index(curBuf, v.AltHoverPreviewQuery); idx >= 0 {
			activateNLHoverForLine(curBuf, idx, true)
		} else if v.nlHover != nil && v.nlHover.editorKey == key && v.nlHover.byShortcut {
			v.nlHover = nil
		}
	}

	buildSlashItems := func() []editor.Completion {
		items := make([]editor.Completion, 0, len(v.SlashItems))
		for _, si := range v.SlashItems {
			items = append(items, editor.Completion{
				Label:         si.Label,
				Detail:        si.Slash,
				Kind:          slashCompletionKind(si),
				Insert:        "__ASTER_SLASH__" + si.CommandID + "|" + si.Kind + "|" + si.Preset + "__",
				FilterText:    si.Slash + " " + si.Label + " " + si.SearchTerms,
				Documentation: si.Detail,
			})
		}
		return items
	}

	buildWikiItems := func() []editor.Completion {
		items := make([]editor.Completion, 0, len(v.VaultCompletionTargets))
		for _, tgt := range v.VaultCompletionTargets {
			items = append(items, editor.Completion{
				Label:  tgt,
				Detail: "实验库文件",
				Kind:   editor.CompletionModule,
				Insert: "[[" + tgt + "]]",
			})
		}
		return items
	}

	triggerPopupAtCursor := func(full string, off int) bool {
		if readOnly {
			return false
		}
		off = min(max(0, off), len(full))
		if slashStart, _, isSlash := ShouldTriggerSlashAt(full, off); isSlash && len(v.SlashItems) > 0 {
			st.ShowCompletions(slashStart, buildSlashItems())
			return true
		}
		if atStart, wrapped, ok := findRelationAtTrigger(full, off); ok && v.Doc != nil {
			items := v.buildRelationTargetCompletions(wrapped)
			if len(items) > 0 {
				st.ShowCompletions(atStart, items)
				return true
			}
		}
		if wikiStart, ok := findWikiTrigger(full, off); ok && len(v.VaultCompletionTargets) > 0 {
			st.ShowCompletions(wikiStart, buildWikiItems())
			return true
		}
		return false
	}

	// Unified Completions: `/` Slash commands, `{@` scientific target refs, `[[` Vault links
	st.OnCompletion = func(req editor.CompletionRequest) {
		if readOnly {
			return
		}
		triggerPopupAtCursor(st.Buffer.String(), req.Offset)
	}

	// TopInspo / Notion-inspired crisp command palette dropdown
	st.RenderCompletions = func(cc *ui.Context, panel ui.Element, m *editor.CompletionMenu) {
		v.renderStyledCompletionMenu(cc, panel, m)
	}

	// Detect live edits and handle Slash command sentinel execution
	if curBuf != entry.syncedText {
		prevSynced := entry.syncedText
		if nextClean, slashCmd, slashPos, matched := extractSlashSentinel(curBuf, v.SlashItems); matched {
			entry.toolbarSlashActive = false
			entry.toolbarSlashStart = -1
			entry.toolbarSlashLen = 0
			st.Buffer = editor.NewBuffer(nextClean)
			if lang := editor.LanguageFor("note.md"); lang != nil {
				st.Buffer.SetLanguage(lang)
			}
			entry.syncedText = nextClean
			entry.lastVersion = 0
			if onCommit != nil {
				entry.lastOwnedSource = onCommit(nextClean)
			}
			if v.OnExecuteSlash != nil {
				insertAt := entry.docOffset + min(slashPos, len(nextClean))
				v.OnExecuteSlash(slashCmd, stepID, insertAt)
			}
			return
		}
		entry.syncedText = curBuf
		if onCommit != nil {
			entry.lastOwnedSource = onCommit(curBuf)
		}
		// Immediately pop completions when user types `/`, `{@`, or `[[`
		// (MyGo's built-in afterTyping only auto-requests on word chars and `.:>`).
		verNow := st.Buffer.Version()
		if !readOnly && st.Focused() && verNow != entry.lastCompletionVer && verNow != entry.dismissedSlashVer {
			sel := st.Selection()
			if sel.Start() == sel.End() && len(curBuf) >= len(prevSynced) {
				if triggerPopupAtCursor(curBuf, st.Cursor()) {
					entry.lastCompletionVer = verNow
				}
			}
		}
	}

	// Recompute Obsidian Live Preview decorations whenever buffer version, caret line, focus, or theme changes
	activeLine := st.CursorPos().Line
	focused := st.Focused()
	if focused && !readOnly {
		v.lastFocusedLiveKey = key
	}
	if entry.wasFocused && !focused && onBlur != nil {
		onBlur(curBuf)
	}
	entry.wasFocused = focused

	dark := t != nil && t.Dark
	ver := st.Buffer.Version()
	if ver != entry.lastVersion || activeLine != entry.lastActiveLn || focused != entry.lastFocused || dark != entry.lastDark || readOnly != entry.lastReadOnly {
		styles, repls, diags := ComputeLivePreviewDecorations(
			curBuf,
			activeLine,
			focused,
			t,
			tok,
			v.Doc,
			entry.docOffset,
			allowCheckToggle,
			func(start, end int, checked bool) {
				raw := st.Buffer.String()
				if start < 0 || end > len(raw) || start >= end {
					return
				}
				seg := raw[start:end]
				nextSeg := seg
				if checked {
					nextSeg = strings.Replace(seg, "[ ]", "[x]", 1)
				} else {
					nextSeg = strings.Replace(strings.Replace(seg, "[x]", "[ ]", 1), "[X]", "[ ]", 1)
				}
				if nextSeg == seg {
					return
				}
				nextAll := raw[:start] + nextSeg + raw[end:]
				st.Buffer.Edit(editor.Edit{Start: start, End: end, Text: nextSeg})
				entry.syncedText = nextAll
				entry.lastVersion = 0
				if onCommit != nil {
					entry.lastOwnedSource = onCommit(nextAll)
				}
			},
			func(objID, sub string) {
				v.focusRelationTarget(objID, sub)
			},
		)
		st.SetLineStyles(styles)
		entry.baseRepls = repls
		st.SetReplacements(repls)
		st.SetDiagnostics(ver, diags)
		entry.lastVersion = ver
		entry.lastActiveLn = activeLine
		entry.lastFocused = focused
		entry.lastDark = dark
		entry.lastReadOnly = readOnly
		entry.lastNLTextSig = ""
	}

	// Apply pure-text rolling rainbow shimmer directly onto the hovered sentence glyphs (zero box!)
	if !readOnly && v.nlHover != nil && v.nlHover.editorKey == key && v.nlHover.sug != nil {
		phase := nlPrismPhase(c, v.nlHover.openedAt)
		bucket := int(phase * 48)
		sig := fmt.Sprintf("%d:%d:%d:%d:%t", ver, v.nlHover.lineStart, v.nlHover.lineEnd, bucket, dark)
		if sig != entry.lastNLTextSig {
			merged := buildRainbowTextReplacements(entry.baseRepls, curBuf, v.nlHover.lineStart, v.nlHover.lineEnd, tok, dark, phase)
			st.SetReplacements(merged)
			entry.lastNLTextSig = sig
		}
	} else if entry.lastNLTextSig != "" {
		st.SetReplacements(entry.baseRepls)
		entry.lastNLTextSig = ""
	}

	ui.Box(c.Key("live-md:" + key)).FillWidth().Children(func() {
		edEl := editor.View(c, st).FillWidth()

		// Instant zero-delay real Alt+Hover tracking (rock-solid while Alt is held OR while pointer is on card/line)
		if !readOnly && v.AltHoverPreviewQuery == "" {
			_, py, insideEd := edEl.PointerPosition()
			hitLine := -1
			if insideEd {
				contentY := float64(py) + st.ScrollY()
				lineCount := strings.Count(curBuf, "\n") + 1
				for ln := 0; ln < lineCount; ln++ {
					top := st.LineTop(ln)
					bot := st.LineTop(ln + 1)
					if bot <= top {
						bot = top + float64(tok.TextBody)*1.56
					}
					if contentY >= top && contentY < bot {
						hitLine = ln
						break
					}
				}
			}
			activeHere := v.nlHover != nil && v.nlHover.editorKey == key
			if altHeld {
				// While Alt is held:
				// 1. If pointer is already on the open card, keep it rock-solid (never kill it!).
				// 2. If pointer is inside the editor on a line, activate/preserve that line.
				// 3. If pointer is outside the editor (e.g. moving toward card), keep existing card rock-solid!
				if activeHere && v.nlHover.overCard {
					v.nlHover.hideAt = time.Time{}
				} else if insideEd && hitLine >= 0 {
					if lnStart, _, ok := lineBoundsForIndex(curBuf, hitLine); ok {
						activateNLHoverForLine(curBuf, lnStart, false)
					}
				} else if activeHere {
					v.nlHover.hideAt = time.Time{}
				}
			} else if activeHere && !v.nlHover.byShortcut {
				// Alt released: stay rock-solid indefinitely while pointer is over the card OR still on the highlighted line.
				onActiveLine := insideEd && hitLine == v.nlHover.lineIdx
				if v.nlHover.overCard || onActiveLine {
					v.nlHover.hideAt = time.Time{}
				} else {
					if v.nlHover.hideAt.IsZero() {
						v.nlHover.hideAt = c.Now().Add(900 * time.Millisecond)
					}
					if wait := v.nlHover.hideAt.Sub(c.Now()); wait > 0 {
						c.After(wait)
					} else {
						v.nlHover = nil
					}
				}
			}
		}

		// Quiet, text-free ✦ micro-icon at right margin of the focused actionable line (L1 progressive disclosure).
		// Clicking it opens the exact same preview card as Alt+Hover / Alt+Enter without cluttering resting text.
		if !readOnly && focused && (v.nlHover == nil || v.nlHover.editorKey != key) {
			if lnStart, lnEnd, ok := lineBoundsForIndex(curBuf, activeLine); ok {
				lnTxt := strings.TrimSpace(curBuf[lnStart:lnEnd])
				if len([]rune(lnTxt)) >= 6 {
					sugs := AnalyzeLabSentenceWithContext(v.Doc, lnTxt, entry.docOffset+lnStart, nil)
					if len(sugs) > 0 && sugs[0] != nil {
						lineTop := float32(st.LineTop(activeLine) - st.ScrollY())
						lineBot := float32(st.LineTop(activeLine+1) - st.ScrollY())
						lineH := lineBot - lineTop
						if lineH < float32(tok.TextBody)*1.3 {
							lineH = float32(tok.TextBody) * 1.56
						}
						btnSz := float32(tok.Spacing24) - 2
						sparkBtn := ui.ButtonBase(c.Key(fmt.Sprintf("nl-spark:%s:%d", key, activeLine))).
							Absolute().Top(lineTop + (lineH-btnSz)/2).Right(2).
							Size(btnSz, btnSz).Center().
							Radius(float32(tok.ControlRadius)).
							Label("智能转为交互组件")
						if sparkBtn.Hovered() {
							sparkBtn.Background(tok.AccentSoftBg)
						}
						sparkBtn.Children(func() {
							ui.Icon(c, lucide.Sparkles()).Size(float32(tok.Spacing12)+1, float32(tok.Spacing12)+1).
								TextColor(tok.Accent.Alpha(0.75))
						})
						components.Tooltip(c, sparkBtn, "转为交互组件（快捷键 Alt+悬停 或 Alt+Enter）")
						if sparkBtn.Clicked() {
							activateNLHoverForLine(curBuf, lnStart, true)
						}
					}
				}
			}
		}

		// Render colorful left-to-right luminous sweep on the exact hovered sentence + non-occluding popover BELOW it
		if !readOnly && v.nlHover != nil && v.nlHover.editorKey == key && v.nlHover.sug != nil {
			lineTop := float32(st.LineTop(v.nlHover.lineIdx) - st.ScrollY())
			lineBot := float32(st.LineTop(v.nlHover.lineIdx+1) - st.ScrollY())
			lineH := lineBot - lineTop
			if lineH < float32(tok.TextBody)*1.3 {
				lineH = float32(tok.TextBody) * 1.56
			}
			nlAnchor := v.renderNLLineSweepHighlight(c, tok, dark, lineTop, lineH, v.nlHover.openedAt)
			v.renderNLHoverOverlay(c, tok, dark, altHeld, nlAnchor, v.nlHover, func() {
				v.AltHoverPreviewQuery = ""
				v.nlHover = nil
				st.Run(editor.Cancel)
			})
		}

		// Formatting hotkeys (Ctrl+B bold, Ctrl+I italic) directly inside Live Preview
		if !readOnly && focused {
			if edEl.Shortcut(ui.Cmd, ui.KeyB) {
				v.lastFocusedLiveKey = key
				v.applyMarkdownInlineWrap("**", "**")
			}
			if edEl.Shortcut(ui.Cmd, ui.KeyI) {
				v.lastFocusedLiveKey = key
				v.applyMarkdownInlineWrap("*", "*")
			}
			if edEl.Shortcut(ui.Alt, ui.KeyEnter) {
				activateNLHoverForLine(st.Buffer.String(), st.Cursor(), true)
			}
			// If user pressed Esc to cancel a toolbar-injected lone `/`, cleanly strip it AND close the popup in 1 Esc
			if entry.toolbarSlashActive && edEl.Shortcut(0, ui.KeyEscape) {
				raw := st.Buffer.String()
				s := entry.toolbarSlashStart
				e := s + entry.toolbarSlashLen
				if s >= 0 && e <= len(raw) && strings.HasSuffix(raw[s:e], "/") {
					st.Buffer.Edit(editor.Edit{Start: s, End: e, Text: ""})
					st.Select(s, s)
					cleaned := st.Buffer.String()
					entry.syncedText = cleaned
					if onCommit != nil {
						entry.lastOwnedSource = onCommit(cleaned)
					}
				}
				entry.toolbarSlashActive = false
				entry.toolbarSlashStart = -1
				entry.toolbarSlashLen = 0
				entry.dismissedSlashVer = st.Buffer.Version()
				st.ShowCompletions(-1, nil)
			}
		}

		// Handle deferred focus relay & single-`+` toolbar Slash trigger
		matchesPending := !readOnly && (v.pendingFocusLiveKey == key || (v.pendingFocusLiveKey == "__tail__" && isTail))
		if matchesPending {
			edEl.Focus()
			v.lastFocusedLiveKey = key
			if v.pendingInjectSlash && len(v.SlashItems) > 0 {
				full := st.Buffer.String()
				cursor := min(max(0, st.Cursor()), len(full))
				// Ensure `/` lands on a clean line start so ShouldTriggerSlashAt is guaranteed true
				lineStart := strings.LastIndexByte(full[:cursor], '\n') + 1
				injectStr := "/"
				slashStart := cursor
				if strings.TrimSpace(full[lineStart:cursor]) != "" {
					injectStr = "\n/"
					slashStart = cursor + 1
				}
				st.Buffer.Edit(editor.Edit{Start: cursor, End: cursor, Text: injectStr})
				nextCursor := cursor + len(injectStr)
				st.Select(nextCursor, nextCursor)
				entry.toolbarSlashStart = cursor
				entry.toolbarSlashLen = len(injectStr)
				entry.toolbarSlashActive = true
				entry.syncedText = st.Buffer.String()
				entry.lastCompletionVer = st.Buffer.Version()
				st.ShowCompletions(slashStart, buildSlashItems())
			}
			v.pendingFocusLiveKey = ""
			v.pendingInjectSlash = false
		}
	})
}

func slashCompletionKind(si SlashCommandItem) editor.CompletionKind {
	switch si.Kind {
	case "plate":
		return editor.CompletionModule
	case "group_table", "param_table", "data_table":
		return editor.CompletionField
	case "calculator":
		return editor.CompletionFunction
	case "timer":
		return editor.CompletionConstant
	case "checklist":
		return editor.CompletionKeyword
	case "image", "figure", "attachment":
		return editor.CompletionType
	default:
		return editor.CompletionSnippet
	}
}

func completionIconForItem(it editor.Completion) *ui.SVG {
	slash := strings.TrimSpace(it.Detail)
	switch slash {
	case "/plate":
		return lucide.Grid2x2()
	case "/group":
		return lucide.Table2()
	case "/param":
		return lucide.SlidersHorizontal()
	case "/table":
		return lucide.TableProperties()
	case "/calc":
		return lucide.Calculator()
	case "/timer":
		return lucide.Timer()
	case "/check":
		return lucide.ListChecks()
	case "/image":
		return lucide.Image()
	case "/figure":
		return lucide.Shapes()
	case "/file":
		return lucide.Paperclip()
	case "/subflow":
		return lucide.GitBranch()
	case "/prose":
		return lucide.FileText()
	case "/note":
		return lucide.PenLine()
	}
	if strings.HasPrefix(slash, "/step") {
		return lucide.Layers()
	}
	if strings.HasPrefix(it.Label, "@") {
		return lucide.Link2()
	}
	return lucide.Sparkles()
}

// renderStyledCompletionMenu renders a clean TopInspo / Notion / Linear-inspired command
// dropdown for `/`, `{@`, and `[[` completions with Lucide icons, crisp hover/selection
// states, and right-aligned command badges.
func (v *View) renderStyledCompletionMenu(c *ui.Context, panel ui.Element, m *editor.CompletionMenu) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	dark := t != nil && t.Dark
	items := m.Items()
	if len(items) == 0 {
		return
	}
	borderCol := tok.Border
	if dark {
		borderCol = tok.BorderStrong
	}
	headerTitle := "插入科研组件与步骤"
	if strings.HasPrefix(items[0].Label, "@") {
		headerTitle = "关联实验对象"
	} else if items[0].Detail == "实验库文件" {
		headerTitle = "引用实验库文件"
	}

	panel.Background(tok.Surface).
		Border(1, borderCol).
		Radius(float32(tok.PanelRadius)).
		Shadow(0, float32(tok.Spacing8), float32(tok.Spacing24), 0, tok.Text.Alpha(0.16)).
		Padding(float32(tok.Spacing4)).
		Width(float32(tok.Spacing48) * 7.2).
		MaxHeight(float32(tok.Spacing48) * 7.5)

	panel.Children(func() {
		// Micro-header with keyboard affordance hints
		ui.Row(c).FillWidth().Padding(float32(tok.Spacing4), float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, headerTitle).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
			ui.Spacer(c)
			ui.Text(c, "↑↓ · Enter · Esc").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
		})
		ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.55)).Margin(0, 0, float32(tok.Spacing4), 0)

		ui.Scroll(c).Key("slash-rows").FillWidth().MaxHeight(float32(tok.Spacing48) * 6.2).Children(func() {
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing4) * 0.5).Children(func() {
				for i, it := range items {
					if i >= 40 {
						break
					}
					sel := i == m.Selected()
					row := m.Item(i).Row().FillWidth().AlignItems(ui.Center).
						Gap(float32(tok.Spacing8)).
						Padding(float32(tok.Spacing4)*1.5, float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius))
					if sel {
						row.Background(tok.AccentSoftBg)
					} else if row.Hovered() {
						row.Background(tok.Text.Alpha(0.04))
					}
					row.Children(func() {
						iconBg := tok.Elevated
						iconCol := tok.TextMuted
						if sel {
							iconBg = tok.Surface
							iconCol = tok.Accent
						}
						ui.Row(c).Size(float32(tok.Spacing24), float32(tok.Spacing24)).Center().
							Radius(float32(tok.ControlRadius)).Background(iconBg).Border(1, tok.Border.Alpha(0.7)).Shrink(0).Children(func() {
							ui.Icon(c, completionIconForItem(it)).Size(float32(tok.Spacing12)+2, float32(tok.Spacing12)+2).TextColor(iconCol)
						})
						ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4) * 0.25).Children(func() {
							lblCol := tok.Text
							if sel {
								lblCol = tok.Accent
							}
							ui.Text(c, it.Label).FontSize(float32(tok.TextData)).Bold().TextColor(lblCol).SingleLine().Ellipsis("…")
							if it.Documentation != "" {
								ui.Text(c, it.Documentation).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine().Ellipsis("…")
							}
						})
						if it.Detail != "" {
							badgeBg := tok.Elevated
							badgeFg := tok.TextMuted
							if sel {
								badgeBg = tok.Accent.Alpha(0.14)
								badgeFg = tok.Accent
							}
							ui.Row(c).Padding(float32(tok.Spacing4)*0.35, float32(tok.Spacing8)*0.75).
								Radius(float32(tok.ControlRadius)).Background(badgeBg).Shrink(0).Children(func() {
								ui.Text(c, it.Detail).FontSize(float32(tok.TextMeta)).TextColor(badgeFg).FontFeatures("tnum").SingleLine()
							})
						}
					})
				}
			})
		})
	})
}

func (v *View) focusRelationTarget(objectID, subTarget string) {
	if objectID == "" {
		return
	}
	if v.Doc != nil {
		if p, _ := v.findPlate(objectID); p != nil && subTarget != "" {
			v.SetPlateSelection(p.ID, []string{strings.ToUpper(subTarget)})
			return
		}
		for _, m := range v.Doc.Modules {
			if m.Kind == "step" && m.ID == objectID {
				v.ScrollToStepID = m.ID
				return
			}
		}
	}
}

func (v *View) buildRelationTargetCompletions(alreadyInBraces bool) []editor.Completion {
	if v.Doc == nil {
		return nil
	}
	wrap := func(raw string) string {
		if alreadyInBraces {
			return raw
		}
		return "{" + raw + "}"
	}
	var out []editor.Completion
	flat, _ := flattenModules(v.Doc.Modules)
	for _, m := range flat {
		switch m.Kind {
		case "plate":
			title := firstNonEmpty(textValue(m.Data, "title"), m.ID)
			out = append(out, editor.Completion{
				Label:      "@" + title,
				Detail:     "孔板",
				Kind:       editor.CompletionModule,
				Insert:     wrap("@" + m.ID),
				FilterText: "@" + m.ID + " " + title + " 孔板",
			})
			var p *Plate
			if m.Plate != nil {
				p = m.Plate
			} else if parsed, err := plateFromData(m.Data); err == nil {
				cp := parsed
				p = &cp
			}
			if p != nil {
				lblMap := PlateGroupLabelMap(v.Doc)
				for well, gid := range p.WellGroups() {
					gLbl := firstNonEmpty(lblMap[gid], gid)
					out = append(out, editor.Completion{
						Label:      fmt.Sprintf("@%s · %s", title, well),
						Detail:     gLbl,
						Kind:       editor.CompletionField,
						Insert:     wrap("@" + m.ID + ":" + well),
						FilterText: "@" + m.ID + ":" + well + " " + title + " " + well + " " + gLbl,
					})
				}
			}
		case "calculator":
			title := firstNonEmpty(textValue(m.Data, "title"), "计算器")
			out = append(out, editor.Completion{
				Label:      "@" + title,
				Detail:     "计算器",
				Kind:       editor.CompletionFunction,
				Insert:     wrap("@" + m.ID),
				FilterText: "@" + m.ID + " " + title + " 计算器",
			})
		case "step":
			title := firstNonEmpty(textValue(m.Data, "title"), m.ID)
			out = append(out, editor.Completion{
				Label:      "@步骤 · " + title,
				Detail:     "步骤",
				Kind:       editor.CompletionFunction,
				Insert:     wrap("@" + m.ID),
				FilterText: "@" + m.ID + " " + title + " 步骤",
			})
		}
	}
	for _, g := range v.Doc.GroupList() {
		name := firstNonEmpty(g.Name, g.ID)
		out = append(out, editor.Completion{
			Label:      "@" + name,
			Detail:     "实验分组",
			Kind:       editor.CompletionVariable,
			Insert:     wrap("@" + g.ID),
			FilterText: "@" + g.ID + " " + name + " 分组",
		})
	}
	if stTree := ParseStepTree(v.Doc.Source, v.Doc); stTree != nil {
		for _, sn := range stTree.Flat {
			if sn.SourceKind != "heading" {
				continue
			}
			out = append(out, editor.Completion{
				Label:      fmt.Sprintf("@步骤 %s · %s", sn.Number, sn.Title),
				Detail:     "步骤引用",
				Kind:       editor.CompletionFunction,
				Insert:     wrap("@" + sn.ID),
				FilterText: "@" + sn.ID + " " + sn.Title + " " + sn.Number + " 步骤",
			})
		}
	}
	return out
}

func findRelationAtTrigger(text string, cursor int) (atPos int, alreadyInBraces bool, ok bool) {
	lineStart := strings.LastIndexByte(text[:cursor], '\n') + 1
	before := text[lineStart:cursor]
	// Case 1: Inside `{@ ...` (backward compatible)
	openBrace := strings.LastIndexByte(before, '{')
	closeBrace := strings.LastIndexByte(before, '}')
	if openBrace >= 0 && closeBrace <= openBrace {
		if atIdx := strings.LastIndexByte(before[openBrace:], '@'); atIdx >= 0 {
			absAt := lineStart + openBrace + atIdx
			tail := text[absAt+1 : cursor]
			if !strings.ContainsAny(tail, " \t}") {
				return absAt, true, true
			}
		}
	}
	// Case 2: Zero-brace standalone `@` (preceded by start-of-line or whitespace, NEVER `#@` or email!)
	atIdx := strings.LastIndexByte(before, '@')
	if atIdx < 0 {
		return 0, false, false
	}
	if atIdx > 0 {
		prev := before[atIdx-1]
		if prev != ' ' && prev != '\t' && prev != '(' && prev != '[' && !strings.HasSuffix(before[:atIdx], "（") {
			return 0, false, false
		}
	}
	absAt := lineStart + atIdx
	tail := text[absAt+1 : cursor]
	if strings.ContainsAny(tail, " \t}\n") {
		return 0, false, false
	}
	return absAt, false, true
}

func findWikiTrigger(text string, cursor int) (int, bool) {
	lineStart := strings.LastIndexByte(text[:cursor], '\n') + 1
	before := text[lineStart:cursor]
	open := strings.LastIndex(before, "[[")
	close := strings.LastIndex(before, "]]")
	if open < 0 || close > open {
		return 0, false
	}
	return lineStart + open, true
}

var reSlashSentinel = regexp.MustCompile(`__ASTER_SLASH__([^|]+)\|([^|]*)\|([^_]*)__`)

func extractSlashSentinel(text string, items []SlashCommandItem) (cleaned string, chosen SlashCommandItem, slashPos int, ok bool) {
	m := reSlashSentinel.FindStringSubmatchIndex(text)
	if m == nil {
		return text, SlashCommandItem{}, 0, false
	}
	cmdID := text[m[2]:m[3]]
	kind := text[m[4]:m[5]]
	preset := text[m[6]:m[7]]
	slashPos = m[0]
	cleaned = text[:m[0]] + text[m[1]:]
	// Trim redundant trailing horizontal whitespace on the slash line
	cleaned = strings.TrimRightFunc(cleaned, func(r rune) bool { return unicode.IsSpace(r) && r != '\n' })
	for _, it := range items {
		if it.CommandID == cmdID && it.Kind == kind && it.Preset == preset {
			return cleaned, it, slashPos, true
		}
	}
	return cleaned, SlashCommandItem{CommandID: cmdID, Kind: kind, Preset: preset}, slashPos, true
}

// renderRelationHoverContent renders the unified scientific context card shared by
// Live Preview hover, Embed Card chip hover, and Plate well reverse inspection.
func (v *View) renderRelationHoverContent(c *ui.Context, tok theme.Tokens, rr ResolvedRelation) {
	t := c.Theme()
	dark := t != nil && t.Dark
	idxMap := plateGroupPaletteMap(v.Doc, nil)
	allRels := ResolveRelations(v.Doc)

	ui.Column(c).Gap(float32(tok.Spacing8)).MaxWidth(float32(tok.Spacing48) * 7).Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Icon(c, lucide.Link2()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
			ui.Text(c, filepath.Base(rr.Asset)).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
			if rr.Timepoint != "" {
				components.Badge(c, rr.Timepoint)
			}
			if rr.FOV > 0 {
				components.Badge(c, fmt.Sprintf("FOV %d", rr.FOV))
			}
		})
		for _, tc := range rr.Contexts {
			tc := tc
			ui.Column(c).FillWidth().Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
				Background(tok.Elevated).Gap(float32(tok.Spacing4)).Children(func() {
				if !tc.Valid {
					ui.Text(c, tc.Diagnostic).FontSize(float32(tok.TextMeta)).TextColor(tok.Warning)
					return
				}
				title := tc.ObjectTitle
				if tc.SubTarget != "" {
					title += " · 孔 " + tc.SubTarget
				}
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if tc.GroupID != "" {
						ui.Box(c).Size(float32(tok.Spacing8), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).
							Background(plateui.GroupColorIndexed(tc.GroupID, idxMap, dark))
					}
					ui.Text(c, title).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text)
					ui.Spacer(c)
					if tc.Excluded {
						components.Badge(c, "异常孔", components.BadgeOptions{Tone: components.BadgeWarning})
					}
				})
				var details []string
				if tc.GroupName != "" {
					details = append(details, "组别: "+tc.GroupName)
				}
				if tc.WellRole != "" {
					details = append(details, "角色: "+tc.WellRole)
				}
				for k, val := range tc.Factors {
					details = append(details, k+"="+val)
				}
				if len(details) > 0 {
					ui.Text(c, strings.Join(details, " · ")).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				}
				if tc.SubTarget != "" {
					siblings := RelationsForTarget(allRels, tc.ObjectID, tc.SubTarget)
					ui.Text(c, fmt.Sprintf("该孔已关联 %d 条实验图像/附件记录", len(siblings))).
						FontSize(float32(tok.TextMeta)).TextColor(tok.Accent).FontFeatures("tnum")
				}
			})
		}
		if rr.Note != "" {
			ui.Text(c, "备注: "+rr.Note).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		}
	})
}
