package asterdoc

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// SegmentKind classifies a top-level contiguous slice of an `.asterml` document body.
type SegmentKind uint8

const (
	SegmentProse SegmentKind = iota
	SegmentEmbed
	SegmentModule
)

// DocSegment is one lossless top-level slice of an `.asterml` body (after YAML frontmatter).
// Start and End are exact byte offsets into the full Document.Source string.
type DocSegment struct {
	Kind   SegmentKind
	Index  int
	Start  int
	End    int
	Text   string
	Module *Module         // non-nil for SegmentModule
	Embed  *InlineRelation // non-nil for SegmentEmbed
}

// SplitDocumentSegments partitions doc.Source (after YAML frontmatter) into an ordered
// stream of Prose, standalone Image Embed, and AsterLab Module segments with exact byte spans.
// Joining all segment Text slices plus the frontmatter reproduces doc.Source byte-for-byte.
func SplitDocumentSegments(source string, doc *Document) []DocSegment {
	return SplitDocumentSegmentsExcludingLine(source, doc, -1)
}

// SplitDocumentSegmentsExcludingLine behaves like SplitDocumentSegments, except when
// activeByteOffset >= 0, the line containing activeByteOffset is kept inside SegmentProse
// instead of being prematurely split into a standalone SegmentEmbed card. This lets users
// type `![[assets/img.png]]{@plate:A1 6h fov=1}` continuously without losing focus after `]]`.
func SplitDocumentSegmentsExcludingLine(source string, doc *Document, activeByteOffset int) []DocSegment {
	_, bodyStart, _ := ScanFrontmatter(source)
	if bodyStart > len(source) {
		bodyStart = len(source)
	}
	if doc == nil {
		doc, _ = Parse(source)
	}

	// Build fence byte ranges [fenceStart, fenceEnd) for every top-level asterlab module
	type fenceRange struct {
		start int
		end   int
		mod   *Module
	}
	var fences []fenceRange
	if doc != nil {
		for i := range doc.Modules {
			m := &doc.Modules[i]
			openIdx := strings.LastIndex(source[:m.Start], "```asterlab")
			if openIdx < bodyStart {
				openIdx = bodyStart
			}
			// Include leading spaces on fence open line if any
			lineStart := openIdx
			for lineStart > bodyStart && source[lineStart-1] != '\n' {
				lineStart--
			}
			closeRel := strings.Index(source[m.End:], "```")
			if closeRel < 0 {
				continue
			}
			closeEnd := m.End + closeRel + 3
			for closeEnd < len(source) && (source[closeEnd] == ' ' || source[closeEnd] == '\t' || source[closeEnd] == '\r') {
				closeEnd++
			}
			if closeEnd < len(source) && source[closeEnd] == '\n' {
				closeEnd++
			}
			fences = append(fences, fenceRange{start: lineStart, end: closeEnd, mod: m})
		}
	}

	var rawSegs []DocSegment
	pos := bodyStart
	appendProseAndEmbeds := func(segStart, segEnd int) {
		if segEnd <= segStart {
			return
		}
		chunk := source[segStart:segEnd]
		lines := strings.SplitAfter(chunk, "\n")
		curProseStart := segStart
		off := segStart
		inCodeFence := false
		codeTicks := 0

		flushProse := func(end int) {
			if end > curProseStart {
				rawSegs = append(rawSegs, DocSegment{
					Kind:  SegmentProse,
					Start: curProseStart,
					End:   end,
					Text:  source[curProseStart:end],
				})
			}
		}

		for _, ln := range lines {
			lineEnd := off + len(ln)
			trimmed := strings.TrimSpace(ln)
			if !inCodeFence {
				if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
					inCodeFence = true
					codeTicks = ticks
					off = lineEnd
					continue
				}
				isActiveLine := activeByteOffset >= 0 && activeByteOffset >= off && activeByteOffset <= lineEnd
				// If line ends with '\n', activeByteOffset == lineEnd belongs to the NEXT line, not this one
				if strings.HasSuffix(ln, "\n") && activeByteOffset == lineEnd {
					isActiveLine = false
				}
				if !isActiveLine {
					if emb, ok := parseStandaloneImageEmbedLine(ln, off); ok {
						flushProse(off)
						embCopy := emb
						rawSegs = append(rawSegs, DocSegment{
							Kind:  SegmentEmbed,
							Start: off,
							End:   lineEnd,
							Text:  ln,
							Embed: &embCopy,
						})
						curProseStart = lineEnd
						off = lineEnd
						continue
					}
				}
			} else {
				if isClosingBacktickFence(trimmed, codeTicks) {
					inCodeFence = false
					codeTicks = 0
				}
			}
			off = lineEnd
		}
		flushProse(segEnd)
	}

	for _, f := range fences {
		if f.start > pos {
			appendProseAndEmbeds(pos, f.start)
		}
		rawSegs = append(rawSegs, DocSegment{
			Kind:   SegmentModule,
			Start:  f.start,
			End:    f.end,
			Text:   source[f.start:f.end],
			Module: f.mod,
		})
		pos = f.end
	}
	if pos < len(source) {
		appendProseAndEmbeds(pos, len(source))
	}
	if len(rawSegs) == 0 {
		// Truly blank document: expose one empty editable prose segment at bodyStart
		rawSegs = append(rawSegs, DocSegment{
			Kind:  SegmentProse,
			Start: bodyStart,
			End:   len(source),
			Text:  source[bodyStart:],
		})
	}
	for i := range rawSegs {
		rawSegs[i].Index = i
	}
	return rawSegs
}

func parseStandaloneImageEmbedLine(line string, lineOffset int) (InlineRelation, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return InlineRelation{}, false
	}
	if !strings.HasPrefix(trimmed, "![[") && !strings.HasPrefix(trimmed, "![") {
		return InlineRelation{}, false
	}
	rels := ExtractInlineRelations(trimmed, false)
	if len(rels) != 1 {
		return InlineRelation{}, false
	}
	r := rels[0]
	if r.SpanStart != 0 || r.SpanEnd != len(trimmed) {
		return InlineRelation{}, false
	}
	if r.SyntaxKind != "wiki_embed" && r.SyntaxKind != "md_image" {
		return InlineRelation{}, false
	}
	// Only render image extensions as standalone visual Embed Cards; other wiki embeds stay inline.
	if r.SyntaxKind == "wiki_embed" && !isImageAssetExt(r.Asset) {
		return InlineRelation{}, false
	}
	leadWS := strings.Index(line, trimmed)
	if leadWS < 0 {
		leadWS = 0
	}
	r.SpanStart += lineOffset + leadWS
	r.SpanEnd += lineOffset + leadWS
	r.AttrStart += lineOffset + leadWS
	r.AttrEnd += lineOffset + leadWS
	return r, true
}

func isImageAssetExt(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	switch ext {
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".bmp", ".tif", ".tiff", ".svg":
		return true
	}
	return false
}

// ReplaceByteRange replaces [start, end) in source with replacement without touching other bytes.
func ReplaceByteRange(source string, start, end int, replacement string) (string, error) {
	if start < 0 || end > len(source) || start > end {
		return "", fmt.Errorf("invalid byte range [%d:%d] for length %d", start, end, len(source))
	}
	return source[:start] + replacement + source[end:], nil
}

// InsertTopLevelModuleInSource inserts a fenced asterlab module at byteOffset (or at
// end of document when byteOffset < 0), ensuring a unique stable ID and valid Parse.
func InsertTopLevelModuleInSource(source string, byteOffset int, module map[string]any) (nextSource, createdID string, err error) {
	doc, err := Parse(source)
	if err != nil {
		return "", "", err
	}
	flat, _ := flattenModules(doc.Modules)
	used := make(map[string]bool, len(flat))
	for _, m := range flat {
		used[m.ID] = true
	}
	modCopy := cloneAnyMap(module)
	if modCopy == nil {
		return "", "", fmt.Errorf("module payload is nil")
	}
	kind := strings.TrimSpace(textValue(modCopy, "kind"))
	if kind == "" {
		return "", "", fmt.Errorf("module kind is required")
	}
	if modCopy["schema"] == nil {
		modCopy["schema"] = 1
	}
	id := strings.TrimSpace(textValue(modCopy, "id"))
	if id == "" || used[id] {
		prefix := sanitizeIDFrag(kind)
		if prefix == "" {
			prefix = "mod"
		}
		cand := prefix + "-1"
		for n := 2; used[cand]; n++ {
			cand = fmt.Sprintf("%s-%d", prefix, n)
		}
		id = cand
		modCopy["id"] = id
	}
	raw, err := yaml.Marshal(modCopy)
	if err != nil {
		return "", "", err
	}
	fence := "```asterlab\n" + string(raw) + "```\n"

	_, bodyStart, _ := ScanFrontmatter(source)
	pos := byteOffset
	if pos < 0 || pos > len(source) {
		pos = len(source)
	}
	if pos < bodyStart {
		pos = bodyStart
	}
	prefix := source[:pos]
	suffix := source[pos:]
	var b strings.Builder
	b.WriteString(prefix)
	if len(prefix) > 0 && !strings.HasSuffix(prefix, "\n\n") {
		if strings.HasSuffix(prefix, "\n") {
			b.WriteString("\n")
		} else {
			b.WriteString("\n\n")
		}
	}
	b.WriteString(fence)
	if len(suffix) > 0 && !strings.HasPrefix(suffix, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(suffix)
	next := b.String()
	if _, err := Parse(next); err != nil {
		return "", "", err
	}
	return next, id, nil
}

// RemoveTopLevelModuleInSource removes a top-level asterlab fence by moduleID while preserving surrounding prose.
func RemoveTopLevelModuleInSource(source, moduleID string) (string, error) {
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	segs := SplitDocumentSegments(source, doc)
	for _, seg := range segs {
		if seg.Kind == SegmentModule && seg.Module != nil && seg.Module.ID == moduleID {
			if seg.Module.Kind == "step" {
				return RemoveStepInSource(source, moduleID)
			}
			next := source[:seg.Start] + source[seg.End:]
			if _, pErr := Parse(next); pErr != nil {
				return "", pErr
			}
			return next, nil
		}
	}
	return "", fmt.Errorf("top-level module %q not found", moduleID)
}

// DuplicateTopLevelModuleInSource clones a top-level module right below itself with a fresh ID.
func DuplicateTopLevelModuleInSource(source, moduleID string) (nextSource, createdID string, err error) {
	doc, err := Parse(source)
	if err != nil {
		return "", "", err
	}
	segs := SplitDocumentSegments(source, doc)
	for _, seg := range segs {
		if seg.Kind == SegmentModule && seg.Module != nil && seg.Module.ID == moduleID {
			clone := cloneAnyMap(seg.Module.Data)
			delete(clone, "id")
			// If duplicating a step, refresh all nested inline block IDs to avoid duplicates.
			if seg.Module.Kind == "step" {
				if blks, ok := clone["blocks"].([]any); ok {
					flat, _ := flattenModules(doc.Modules)
					used := map[string]bool{}
					for _, fm := range flat {
						used[fm.ID] = true
					}
					for i, rawBlk := range blks {
						if bm, ok := rawBlk.(map[string]any); ok {
							bk := sanitizeIDFrag(textValue(bm, "kind"))
							if bk == "" {
								bk = "blk"
							}
							cand := "blk-" + bk
							for n := 2; used[cand]; n++ {
								cand = fmt.Sprintf("blk-%s-%d", bk, n)
							}
							used[cand] = true
							bm["id"] = cand
							blks[i] = bm
						}
					}
					clone["blocks"] = blks
				}
			}
			return InsertTopLevelModuleInSource(source, seg.End, clone)
		}
	}
	return "", "", fmt.Errorf("top-level module %q not found", moduleID)
}

// MoveTopLevelModuleInSource swaps a top-level module with the adjacent module in direction (-1 up, +1 down).
func MoveTopLevelModuleInSource(source, moduleID string, delta int) (string, error) {
	if delta == 0 {
		return source, nil
	}
	doc, err := Parse(source)
	if err != nil {
		return "", err
	}
	segs := SplitDocumentSegments(source, doc)
	var modIndices []int
	curPos := -1
	for i, seg := range segs {
		if seg.Kind == SegmentModule && seg.Module != nil {
			if seg.Module.ID == moduleID {
				curPos = len(modIndices)
			}
			modIndices = append(modIndices, i)
		}
	}
	if curPos < 0 {
		return "", fmt.Errorf("top-level module %q not found", moduleID)
	}
	tgtPos := curPos + delta
	if tgtPos < 0 || tgtPos >= len(modIndices) {
		return source, nil
	}
	a := segs[modIndices[curPos]]
	b := segs[modIndices[tgtPos]]
	if a.Start > b.Start {
		a, b = b, a
	}
	next := source[:a.Start] + b.Text + source[a.End:b.Start] + a.Text + source[b.End:]
	if _, pErr := Parse(next); pErr != nil {
		return "", pErr
	}
	return next, nil
}
