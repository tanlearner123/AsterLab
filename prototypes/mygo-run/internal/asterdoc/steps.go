package asterdoc

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// StepItem represents one ordered entry inside a step's content flow:
// either a plain Markdown list action (`- ...` / `1. ...` / `- [ ] ...`)
// or an embedded scientific module (`calculator`, `timer`, `plate`, etc.).
type StepItem struct {
	Kind        string `json:"kind"` // "action" | "module"
	Text        string `json:"text,omitempty"`
	ModuleID    string `json:"moduleId,omitempty"`
	ModuleKind  string `json:"moduleKind,omitempty"`
	Ordered     bool   `json:"ordered,omitempty"`
	HasCheckbox bool   `json:"hasCheckbox,omitempty"`
	Checked     bool   `json:"checked,omitempty"`
	Line        int    `json:"line"`
	ByteOffset  int    `json:"byteOffset"`
}

// StepNode represents one unified executable step in an `.asterml` document,
// derived either from the orthogonal heading syntax (`#@ 主步骤` / `##@ 子步骤`)
// or from a legacy fenced `step@1` YAML module.
type StepNode struct {
	ID             string      `json:"id"`
	Anchor         string      `json:"anchor,omitempty"`
	Title          string      `json:"title"`
	Number         string      `json:"number"` // Hierarchical display number: "1", "1.1", "2"
	Level          int         `json:"level"`  // Step syntax depth (1 for `#@`, 2 for `##@`, etc.)
	Depth          int         `json:"depth"`  // 0 = root mainline step, 1 = child substep, ...
	ParentID       string      `json:"parentId,omitempty"`
	SourceKind     string      `json:"sourceKind"` // "heading" | "module"
	Optional       bool        `json:"optional,omitempty"`
	Line           int         `json:"line"`      // 1-based line number
	LineStart      int         `json:"lineStart"` // Exact byte start of heading/fence
	LineEnd        int         `json:"lineEnd"`   // Exact byte end of heading line
	ScopeEnd       int         `json:"scopeEnd"`  // Exact byte end of this step's owned scope
	OwnChecksDone  int         `json:"ownChecksDone"`
	OwnChecksTotal int         `json:"ownChecksTotal"`
	ChecksDone     int         `json:"checksDone"`  // Own + rolled-up descendants
	ChecksTotal    int         `json:"checksTotal"` // Own + rolled-up descendants
	RunningTimers  int         `json:"runningTimers"`
	OwnedModuleIDs []string    `json:"ownedModuleIds,omitempty"`
	Items          []StepItem  `json:"items,omitempty"`
	Children       []*StepNode `json:"children,omitempty"`
}

// StepTree is the parsed hierarchical step projection for an `.asterml` document.
type StepTree struct {
	Roots       []*StepNode          `json:"roots"`
	Flat        []*StepNode          `json:"flat"`
	ByID        map[string]*StepNode `json:"-"`
	ByLineStart map[int]*StepNode    `json:"-"`
}

// OutlineNode represents one row in the unified Document Outline, seamlessly combining
// plain Markdown section headings (`## 1.1 实验目的`, `## 1.3 实验步骤`, `## 2. 实验讨论`)
// and orthogonal workflow steps (`#@ 主步骤`, `##@ 子步骤`) in exact document order.
type OutlineNode struct {
	ID            string `json:"id"`
	Title         string `json:"title"`
	IsStep        bool   `json:"isStep"`
	Number        string `json:"number,omitempty"` // "1", "1.1" for steps; "" for plain headings
	Depth         int    `json:"depth"`            // Nesting depth in the unified outline
	StepDepth     int    `json:"stepDepth"`        // Nesting depth inside step-only view (0 = main, 1 = sub)
	HeadingLevel  int    `json:"headingLevel"`     // 1..6 for plain headings
	Line          int    `json:"line"`
	LineStart     int    `json:"lineStart"`
	ActionCount   int    `json:"actionCount,omitempty"`
	ChecksDone    int    `json:"checksDone,omitempty"`
	ChecksTotal   int    `json:"checksTotal,omitempty"`
	RunningTimers int    `json:"runningTimers,omitempty"`
}

// ParsedHeadingStep holds the parsed tokens of a single `#@ 标题` / `##@ 子步骤` line.
type ParsedHeadingStep struct {
	Level      int
	PrefixEnd  int // byte offset right after `#@ ` (title start)
	TitleStart int // byte offset where title begins
	TitleEnd   int // byte offset where title ends (before optional trailing `{...}`)
	Title      string
	Anchor     string
	Optional   bool
	AttrStart  int // byte offset of `{` if trailing `{...}` present, else -1
	AttrEnd    int // byte offset after `}` if present, else -1
	RawAttr    string
}

var (
	reHeadingStepPrefix = regexp.MustCompile(`^(#{1,6})@([ \t]+)`)
	rePlainHeadingLine  = regexp.MustCompile(`^(#{1,6})[ \t]+(.+)$`)
	reThematicBreakLine = regexp.MustCompile(`^[ \t]*((?:-[ \t]*){3,}|(?:\*[ \t]*){3,}|(?:_[ \t]*){3,})$`)
	reTrailingStepAttr  = regexp.MustCompile(`[ \t]+\{([^}\r\n]+)\}[ \t]*$`)
	reStepTaskLine      = regexp.MustCompile(`^[ \t]*[-*+][ \t]+\[([ xX])\][ \t]*(.*)$`)
	reStepBulletLine    = regexp.MustCompile(`^[ \t]*[-*+][ \t]+(.+)$`)
	reStepOrderedLine   = regexp.MustCompile(`^[ \t]*\d+[.)][ \t]+(.+)$`)
)

// ParseHeadingStepLine parses one line of Markdown (without trailing newline) as an
// orthogonal AsterML step header (`#@ 主步骤`, `##@ 子步骤`). Returns ok=false for
// normal headings (`## 标题`), tags (`#tag`), emails, or prose.
func ParseHeadingStepLine(line string) (ParsedHeadingStep, bool) {
	m := reHeadingStepPrefix.FindStringSubmatchIndex(line)
	if m == nil {
		return ParsedHeadingStep{}, false
	}
	level := m[3] - m[2]
	prefixEnd := m[1]
	rest := line[prefixEnd:]
	if strings.TrimSpace(rest) == "" {
		return ParsedHeadingStep{}, false
	}

	out := ParsedHeadingStep{
		Level:      level,
		PrefixEnd:  prefixEnd,
		TitleStart: prefixEnd,
		TitleEnd:   len(line),
		AttrStart:  -1,
		AttrEnd:    -1,
	}

	if am := reTrailingStepAttr.FindStringSubmatchIndex(rest); am != nil {
		attrContent := strings.TrimSpace(rest[am[2]:am[3]])
		braceRel := strings.IndexByte(rest[am[0]:am[1]], '{')
		if braceRel >= 0 {
			out.AttrStart = prefixEnd + am[0] + braceRel
			out.AttrEnd = prefixEnd + am[1]
			out.RawAttr = strings.TrimSpace(line[out.AttrStart:out.AttrEnd])
		}
		out.TitleEnd = prefixEnd + am[0]
		out.Title = strings.TrimSpace(rest[:am[0]])
		for _, tok := range strings.Fields(attrContent) {
			if strings.HasPrefix(tok, "#") && len(tok) > 1 {
				out.Anchor = strings.TrimSpace(tok[1:])
			} else if strings.EqualFold(tok, "optional") {
				out.Optional = true
			}
		}
	} else {
		out.Title = strings.TrimSpace(rest)
	}
	if out.Title == "" {
		return ParsedHeadingStep{}, false
	}
	return out, true
}

// FormatHeadingStep formats a canonical `#@ 标题` (mainline, level=1) or `##@ 子步骤` (level=2) line.
func FormatHeadingStep(level int, title, anchor string) string {
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	line := strings.Repeat("#", level) + "@ " + strings.TrimSpace(title)
	if a := strings.TrimSpace(strings.TrimPrefix(anchor, "#")); a != "" {
		line += " {#" + a + "}"
	}
	return line
}

// StepOwnerAtOffset returns the innermost StepNode whose scope contains byteOffset, or nil.
func StepOwnerAtOffset(tree *StepTree, byteOffset int) *StepNode {
	if tree == nil {
		return nil
	}
	var owner *StepNode
	for _, nd := range tree.Flat {
		if nd.SourceKind != "heading" {
			continue
		}
		if byteOffset >= nd.LineStart && byteOffset < nd.ScopeEnd {
			if owner == nil || nd.Depth >= owner.Depth {
				owner = nd
			}
		}
	}
	return owner
}

// ParseStepTree builds the unified nested StepTree for source & doc.
// Orthogonal Grammar Rules:
//  1. Step hierarchy (`#@ ` = mainline step, `##@ ` = substep) is completely decoupled from
//     plain Markdown chapter headings (`#`, `##`, `###`). Monotonic step stack assigns
//     document-wide continuous numbering (`1`, `1.1`, `1.2`, `2`).
//  2. Any plain Markdown heading (`# .. ######`) OR thematic break (`---`) unambiguously
//     closes all currently open steps and returns to normal prose.
//  3. Plain unordered/ordered lists and embedded modules inside a step's scope form one
//     continuous ordered StepNode.Items flow (never broken when a component sits between lists).
func ParseStepTree(source string, doc *Document) *StepTree {
	tree := &StepTree{
		ByID:        map[string]*StepNode{},
		ByLineStart: map[int]*StepNode{},
	}
	if source == "" && doc == nil {
		return tree
	}
	if source == "" && doc != nil {
		source = doc.Source
	}
	_, bodyStart, _ := ScanFrontmatter(source)
	if bodyStart > len(source) {
		bodyStart = len(source)
	}

	type fencedMod struct {
		start int
		end   int
		mod   Module
	}
	var topMods []fencedMod
	if doc != nil {
		for _, m := range doc.Modules {
			openIdx := strings.LastIndex(source[:min(m.Start, len(source))], "```asterlab")
			if openIdx < bodyStart {
				openIdx = bodyStart
			}
			topMods = append(topMods, fencedMod{start: openIdx, end: m.End, mod: m})
		}
	}

	// Step-only monotonic stack (decoupled from plain Markdown heading levels!)
	type stepFrame struct {
		level int
		node  *StepNode
	}
	var stack []stepFrame
	usedIDs := map[string]bool{}
	allocID := func(preferred, fallback string) string {
		cand := strings.TrimSpace(preferred)
		if cand == "" {
			cand = fallback
		}
		if !usedIDs[cand] {
			usedIDs[cand] = true
			return cand
		}
		for i := 2; ; i++ {
			next := fmt.Sprintf("%s-%d", cand, i)
			if !usedIDs[next] {
				usedIDs[next] = true
				return next
			}
		}
	}

	closeToStepLevel := func(nextLevel, closeByteOffset int) {
		for len(stack) > 0 && stack[len(stack)-1].level >= nextLevel {
			top := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if top.node != nil && top.node.ScopeEnd == 0 {
				top.node.ScopeEnd = closeByteOffset
			}
		}
	}
	closeAllSteps := func(closeByteOffset int) {
		closeToStepLevel(0, closeByteOffset)
	}
	currentStep := func() *StepNode {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1].node
	}

	legacyHandled := map[string]bool{}
	addLegacyStepsBefore := func(bytePos int) {
		for _, fm := range topMods {
			if fm.mod.Kind != "step" || fm.start > bytePos || legacyHandled[fm.mod.ID] {
				continue
			}
			legacyHandled[fm.mod.ID] = true
			done, total, running := countLegacyStepProgress(fm.mod.Data)
			num := fmt.Sprintf("%d", len(tree.Roots)+1)
			id := allocID(fm.mod.ID, "step-"+num)
			nd := &StepNode{
				ID:             id,
				Title:          firstNonEmpty(textValue(fm.mod.Data, "title"), id),
				Number:         num,
				Level:          1,
				Depth:          0,
				SourceKind:     "module",
				Line:           1 + strings.Count(source[:min(fm.start, len(source))], "\n"),
				LineStart:      fm.start,
				LineEnd:        fm.end,
				ScopeEnd:       fm.end,
				OwnChecksDone:  done,
				OwnChecksTotal: total,
				ChecksDone:     done,
				ChecksTotal:    total,
				RunningTimers:  running,
			}
			tree.Roots = append(tree.Roots, nd)
			tree.Flat = append(tree.Flat, nd)
			tree.ByID[nd.ID] = nd
			tree.ByLineStart[nd.LineStart] = nd
		}
	}

	body := source[bodyStart:]
	lines := strings.SplitAfter(body, "\n")
	off := bodyStart
	lineNum := 1 + strings.Count(source[:bodyStart], "\n")
	inFence := false
	fenceTicks := 0

	for _, rawLine := range lines {
		lineEnd := off + len(rawLine)
		lineText := strings.TrimRight(rawLine, "\r\n")
		trimmed := strings.TrimSpace(lineText)

		if !inFence {
			if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
				addLegacyStepsBefore(off)
				inFence = true
				fenceTicks = ticks
				off = lineEnd
				lineNum++
				continue
			}
		} else {
			if isClosingBacktickFence(trimmed, fenceTicks) {
				inFence = false
				fenceTicks = 0
			}
			off = lineEnd
			lineNum++
			continue
		}

		// 1. Orthogonal step header (`#@ 主步骤` / `##@ 子步骤`)
		if hs, ok := ParseHeadingStepLine(lineText); ok {
			addLegacyStepsBefore(off)
			closeToStepLevel(hs.Level, off)
			parent := currentStep()
			var num string
			depth := 0
			parentID := ""
			if parent == nil {
				num = fmt.Sprintf("%d", len(tree.Roots)+1)
			} else {
				depth = parent.Depth + 1
				parentID = parent.ID
				num = fmt.Sprintf("%s.%d", parent.Number, len(parent.Children)+1)
			}
			id := allocID(hs.Anchor, "step-"+strings.ReplaceAll(num, ".", "-"))
			nd := &StepNode{
				ID:         id,
				Anchor:     hs.Anchor,
				Title:      hs.Title,
				Number:     num,
				Level:      hs.Level,
				Depth:      depth,
				ParentID:   parentID,
				SourceKind: "heading",
				Optional:   hs.Optional,
				Line:       lineNum,
				LineStart:  off,
				LineEnd:    off + len(lineText),
			}
			if parent == nil {
				tree.Roots = append(tree.Roots, nd)
			} else {
				parent.Children = append(parent.Children, nd)
			}
			tree.Flat = append(tree.Flat, nd)
			tree.ByID[nd.ID] = nd
			if nd.Anchor != "" {
				tree.ByID[nd.Anchor] = nd
			}
			tree.ByLineStart[nd.LineStart] = nd
			stack = append(stack, stepFrame{level: hs.Level, node: nd})
			off = lineEnd
			lineNum++
			continue
		}

		// 2. Plain Markdown heading (`# .. ######`) or thematic break (`---`) closes active steps unambiguously
		if rePlainHeadingLine.MatchString(lineText) || reThematicBreakLine.MatchString(lineText) {
			closeAllSteps(off)
			off = lineEnd
			lineNum++
			continue
		}

		// 3. Collect ordered/unordered list items & checkboxes belonging to the innermost open step
		if curStep := currentStep(); curStep != nil {
			if tm := reStepTaskLine.FindStringSubmatch(lineText); tm != nil {
				checked := tm[1] != " "
				curStep.OwnChecksTotal++
				if checked {
					curStep.OwnChecksDone++
				}
				curStep.Items = append(curStep.Items, StepItem{
					Kind:        "action",
					Text:        strings.TrimSpace(tm[2]),
					HasCheckbox: true,
					Checked:     checked,
					Line:        lineNum,
					ByteOffset:  off,
				})
			} else if bm := reStepBulletLine.FindStringSubmatch(lineText); bm != nil {
				curStep.Items = append(curStep.Items, StepItem{
					Kind:       "action",
					Text:       strings.TrimSpace(bm[1]),
					Line:       lineNum,
					ByteOffset: off,
				})
			} else if om := reStepOrderedLine.FindStringSubmatch(lineText); om != nil {
				curStep.Items = append(curStep.Items, StepItem{
					Kind:       "action",
					Text:       strings.TrimSpace(om[1]),
					Ordered:    true,
					Line:       lineNum,
					ByteOffset: off,
				})
			}
		}

		off = lineEnd
		lineNum++
	}

	closeAllSteps(len(source))
	addLegacyStepsBefore(len(source))

	// Assign top-level fenced modules to their innermost owning step AND interleave into StepNode.Items
	for _, fm := range topMods {
		if fm.mod.Kind == "step" || fm.mod.Kind == "workflow" {
			continue
		}
		owner := StepOwnerAtOffset(tree, fm.start)
		if owner == nil {
			continue
		}
		owner.OwnedModuleIDs = append(owner.OwnedModuleIDs, fm.mod.ID)
		owner.Items = append(owner.Items, StepItem{
			Kind:       "module",
			Text:       firstNonEmpty(textValue(fm.mod.Data, "title"), fm.mod.ID),
			ModuleID:   fm.mod.ID,
			ModuleKind: fm.mod.Kind,
			Line:       1 + strings.Count(source[:min(fm.start, len(source))], "\n"),
			ByteOffset: fm.start,
		})
		switch fm.mod.Kind {
		case "checklist":
			for _, item := range mapSlice(fm.mod.Data["items"]) {
				owner.OwnChecksTotal++
				if done, _ := item["done"].(bool); done {
					owner.OwnChecksDone++
				}
			}
		case "timer":
			if strings.EqualFold(textValue(fm.mod.Data, "status"), "running") {
				owner.RunningTimers++
			}
		}
	}

	// Keep each step's mixed flow (lists + components) sorted in exact document order
	for _, nd := range tree.Flat {
		if len(nd.Items) > 1 {
			sort.SliceStable(nd.Items, func(i, j int) bool {
				return nd.Items[i].ByteOffset < nd.Items[j].ByteOffset
			})
		}
	}

	// Roll up child progress bottom-up into parent steps
	var rollUp func(n *StepNode)
	rollUp = func(n *StepNode) {
		n.ChecksDone = n.OwnChecksDone
		n.ChecksTotal = n.OwnChecksTotal
		for _, ch := range n.Children {
			rollUp(ch)
			n.ChecksDone += ch.ChecksDone
			n.ChecksTotal += ch.ChecksTotal
			n.RunningTimers += ch.RunningTimers
		}
	}
	for _, root := range tree.Roots {
		if root.SourceKind == "heading" {
			rollUp(root)
		}
	}
	return tree
}

// ParseDocumentOutline produces ONE coherent outline combining plain Markdown chapter headings
// and orthogonal workflow steps (`#@ ` / `##@ ` / legacy `step@1`), where steps naturally nest
// beneath their immediately enclosing chapter heading without sharing `#` depth math.
func ParseDocumentOutline(source string, doc *Document) []*OutlineNode {
	if source == "" && doc != nil {
		source = doc.Source
	}
	if source == "" {
		return nil
	}
	fm, bodyStart, _ := ScanFrontmatter(source)
	if bodyStart > len(source) {
		bodyStart = len(source)
	}
	docTitle := strings.TrimSpace(fm.Title)
	stepTree := ParseStepTree(source, doc)

	var out []*OutlineNode
	type chapFrame struct {
		level int
		depth int
	}
	var chapters []chapFrame
	currentChapterBaseDepth := func() int {
		if len(chapters) == 0 {
			return 0
		}
		return chapters[len(chapters)-1].depth + 1
	}

	// Track legacy step@1 modules so they appear at their exact position
	legacyEmitted := map[string]bool{}
	emitLegacyBefore := func(bytePos int) {
		for _, sn := range stepTree.Flat {
			if sn.SourceKind != "module" || sn.LineStart > bytePos || legacyEmitted[sn.ID] {
				continue
			}
			legacyEmitted[sn.ID] = true
			out = append(out, &OutlineNode{
				ID:            sn.ID,
				Title:         sn.Title,
				IsStep:        true,
				Number:        sn.Number,
				Depth:         currentChapterBaseDepth() + sn.Depth,
				StepDepth:     sn.Depth,
				Line:          sn.Line,
				LineStart:     sn.LineStart,
				ActionCount:   len(sn.Items),
				ChecksDone:    sn.ChecksDone,
				ChecksTotal:   sn.ChecksTotal,
				RunningTimers: sn.RunningTimers,
			})
		}
	}

	body := source[bodyStart:]
	lines := strings.SplitAfter(body, "\n")
	off := bodyStart
	lineNum := 1 + strings.Count(source[:bodyStart], "\n")
	inFence := false
	fenceTicks := 0
	headingOrdinal := 0
	firstH1Checked := false

	for _, rawLine := range lines {
		lineEnd := off + len(rawLine)
		lineText := strings.TrimRight(rawLine, "\r\n")
		trimmed := strings.TrimSpace(lineText)

		if !inFence {
			if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
				emitLegacyBefore(off)
				inFence = true
				fenceTicks = ticks
				off = lineEnd
				lineNum++
				continue
			}
		} else {
			if isClosingBacktickFence(trimmed, fenceTicks) {
				inFence = false
				fenceTicks = 0
			}
			off = lineEnd
			lineNum++
			continue
		}

		if _, isStep := ParseHeadingStepLine(lineText); isStep {
			emitLegacyBefore(off)
			if sn := stepTree.ByLineStart[off]; sn != nil {
				out = append(out, &OutlineNode{
					ID:            sn.ID,
					Title:         sn.Title,
					IsStep:        true,
					Number:        sn.Number,
					Depth:         currentChapterBaseDepth() + sn.Depth,
					StepDepth:     sn.Depth,
					Line:          sn.Line,
					LineStart:     sn.LineStart,
					ActionCount:   len(sn.Items),
					ChecksDone:    sn.ChecksDone,
					ChecksTotal:   sn.ChecksTotal,
					RunningTimers: sn.RunningTimers,
				})
			}
			off = lineEnd
			lineNum++
			continue
		}

		if pm := rePlainHeadingLine.FindStringSubmatch(lineText); pm != nil {
			emitLegacyBefore(off)
			lvl := len(pm[1])
			title := strings.TrimSpace(pm[2])
			// Skip duplicate H1 matching document frontmatter title
			if !firstH1Checked && lvl == 1 && docTitle != "" && strings.EqualFold(title, docTitle) {
				firstH1Checked = true
				off = lineEnd
				lineNum++
				continue
			}
			firstH1Checked = true
			for len(chapters) > 0 && chapters[len(chapters)-1].level >= lvl {
				chapters = chapters[:len(chapters)-1]
			}
			depth := len(chapters)
			chapters = append(chapters, chapFrame{level: lvl, depth: depth})
			headingOrdinal++
			out = append(out, &OutlineNode{
				ID:           fmt.Sprintf("heading-%d", headingOrdinal),
				Title:        title,
				IsStep:       false,
				Depth:        depth,
				HeadingLevel: lvl,
				Line:         lineNum,
				LineStart:    off,
			})
			off = lineEnd
			lineNum++
			continue
		}

		off = lineEnd
		lineNum++
	}
	emitLegacyBefore(len(source))
	return out
}

func countLegacyStepProgress(stepData map[string]any) (done, total, running int) {
	for _, bm := range mapSlice(stepData["blocks"]) {
		switch textValue(bm, "kind") {
		case "checklist":
			for _, it := range mapSlice(bm["items"]) {
				total++
				if d, _ := it["done"].(bool); d {
					done++
				}
			}
		case "timer":
			if strings.EqualFold(textValue(bm, "status"), "running") {
				running++
			}
		}
	}
	return done, total, running
}
