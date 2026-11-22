package asterdoc

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// RelationTarget references one scientific object (and optional sub-targets such as
// plate well coordinates) inside an inline `{@...}` attribute block.
type RelationTarget struct {
	ObjectID   string   `json:"objectId"`
	SubTargets []string `json:"subTargets,omitempty"`
}

// InlineRelation is the single canonical AST node for `![[asset]]{@...}`,
// `![alt](asset){@...}`, `[[file]]{@...}`, and `[label](file){@...}`.
// Live Preview chips, Hover cards, Plate well reverse lookups, VaultIndex,
// and Agent CLI commands all consume this exact structure.
type InlineRelation struct {
	SyntaxKind string            `json:"syntaxKind"` // "wiki_embed" | "wiki_link" | "md_image" | "md_link"
	Asset      string            `json:"asset"`
	Label      string            `json:"label,omitempty"`
	Width      string            `json:"width,omitempty"`
	Targets    []RelationTarget  `json:"targets,omitempty"`
	Timepoint  string            `json:"timepoint,omitempty"`
	FOV        int               `json:"fov,omitempty"`
	Role       string            `json:"role,omitempty"`
	Note       string            `json:"note,omitempty"`
	Extra      map[string]string `json:"extra,omitempty"`
	RawAttr    string            `json:"rawAttr,omitempty"`
	SpanStart  int               `json:"spanStart"` // Full match start (including ![[...]]{...})
	SpanEnd    int               `json:"spanEnd"`   // Full match end
	AttrStart  int               `json:"attrStart"` // Byte offset of '{' (or SpanEnd if no attr)
	AttrEnd    int               `json:"attrEnd"`   // Byte offset after '}' (or SpanEnd if no attr)
	Line       int               `json:"line"`      // 1-based line number in document
}

// HasRelation reports whether this link/embed carries explicit scientific relation attributes.
func (r InlineRelation) HasRelation() bool {
	return len(r.Targets) > 0 || r.Timepoint != "" || r.FOV > 0 || r.Role != "" || r.Note != "" || len(r.Extra) > 0
}

// TargetContext carries resolved scientific metadata for one RelationTarget so
// Hover cards, Plate popovers, and Agents share identical understanding.
type TargetContext struct {
	ObjectID    string            `json:"objectId"`
	SubTarget   string            `json:"subTarget,omitempty"`
	ObjectKind  string            `json:"objectKind,omitempty"`
	ObjectTitle string            `json:"objectTitle,omitempty"`
	GroupID     string            `json:"groupId,omitempty"`
	GroupName   string            `json:"groupName,omitempty"`
	GroupRole   string            `json:"groupRole,omitempty"`
	WellRole    string            `json:"wellRole,omitempty"`
	Factors     map[string]string `json:"factors,omitempty"`
	Excluded    bool              `json:"excluded,omitempty"`
	Valid       bool              `json:"valid"`
	Diagnostic  string            `json:"diagnostic,omitempty"`
}

// ResolvedRelation enriches an InlineRelation (or block-level figure/attachment link)
// with live resolved target metadata and diagnostics.
type ResolvedRelation struct {
	InlineRelation
	SourceKind string          `json:"sourceKind"` // "inline" | "attachment" | "figure"
	ModuleID   string          `json:"moduleId,omitempty"`
	Contexts   []TargetContext `json:"contexts,omitempty"`
	Issues     []string        `json:"issues,omitempty"`
}

var (
	reInlineAnnotated = regexp.MustCompile(
		`(!?)\[\[([^\]|\r\n]+?)(?:\|([^\]\r\n]*))?\]\](\{[^}\r\n]*\})?` +
			`|(!?)\[([^\]\r\n]*)\]\(([^)\r\n]+)\)(\{[^}\r\n]*\})?`,
	)
	reShorthandTime = regexp.MustCompile(`(?i)^(?:\d+(?:\.\d+)?(?:ms|s|sec|min|m|h|hr|d|wk)|D\d+|T\d+)$`)
	reWellToken     = regexp.MustCompile(`^[A-Za-z][0-9]{1,2}$`)
)

// ExtractInlineRelations scans Markdown outside code fences and inline code spans,
// returning every annotated (or image-embedded) link in document order.
// When onlyAnnotated is true, plain embeds/links without `{...}` are skipped.
func ExtractInlineRelations(source string, onlyAnnotated bool) []InlineRelation {
	if source == "" {
		return nil
	}
	masked := maskCodeRegions(source)
	matches := reInlineAnnotated.FindAllStringSubmatchIndex(masked, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]InlineRelation, 0, len(matches))
	for _, m := range matches {
		spanStart, spanEnd := m[0], m[1]
		var rel InlineRelation
		rel.SpanStart = spanStart
		rel.SpanEnd = spanEnd
		rel.AttrStart = spanEnd
		rel.AttrEnd = spanEnd
		rel.Line = 1 + strings.Count(source[:spanStart], "\n")

		if m[4] >= 0 { // Wiki syntax: groups 1..4 -> indices 2..9
			isEmbed := source[m[2]:m[3]] == "!"
			if isEmbed {
				rel.SyntaxKind = "wiki_embed"
			} else {
				rel.SyntaxKind = "wiki_link"
			}
			rel.Asset = strings.TrimSpace(source[m[4]:m[5]])
			if m[6] >= 0 {
				opt := strings.TrimSpace(source[m[6]:m[7]])
				if isDimensionOpt(opt) {
					rel.Width = opt
				} else {
					rel.Label = opt
				}
			}
			if m[8] >= 0 {
				rel.AttrStart = m[8]
				rel.AttrEnd = m[9]
				rel.RawAttr = source[m[8]:m[9]]
			}
		} else if m[14] >= 0 { // Markdown syntax: groups 5..8 -> indices 10..17
			isEmbed := source[m[10]:m[11]] == "!"
			if isEmbed {
				rel.SyntaxKind = "md_image"
			} else {
				rel.SyntaxKind = "md_link"
			}
			rel.Label = strings.TrimSpace(source[m[12]:m[13]])
			rel.Asset = strings.TrimSpace(source[m[14]:m[15]])
			if m[16] >= 0 {
				rel.AttrStart = m[16]
				rel.AttrEnd = m[17]
				rel.RawAttr = source[m[16]:m[17]]
			}
		} else {
			continue
		}

		if rel.RawAttr != "" {
			parseRelationAttrInto(&rel, rel.RawAttr)
		}
		if onlyAnnotated && !rel.HasRelation() {
			continue
		}
		out = append(out, rel)
	}
	return out
}

func isDimensionOpt(s string) bool {
	if s == "" {
		return false
	}
	parts := strings.Split(strings.ToLower(s), "x")
	if len(parts) > 2 {
		return false
	}
	for _, p := range parts {
		if p == "" {
			return false
		}
		if _, err := strconv.Atoi(p); err != nil {
			return false
		}
	}
	return true
}

// ParseRelationAttr parses a standalone `{...}` relation attribute string into an InlineRelation.
func ParseRelationAttr(raw string) InlineRelation {
	var rel InlineRelation
	rel.RawAttr = raw
	parseRelationAttrInto(&rel, raw)
	return rel
}

func parseRelationAttrInto(rel *InlineRelation, raw string) {
	inner := strings.TrimSpace(raw)
	if strings.HasPrefix(inner, "{") && strings.HasSuffix(inner, "}") && len(inner) >= 2 {
		inner = strings.TrimSpace(inner[1 : len(inner)-1])
	}
	if inner == "" {
		return
	}
	tokens := tokenizeAttrBody(inner)
	for _, tok := range tokens {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "@") {
			refBody := strings.TrimPrefix(tok, "@")
			if refBody == "" {
				continue
			}
			if idx := strings.IndexByte(refBody, ':'); idx >= 0 {
				objID := strings.TrimSpace(refBody[:idx])
				subRaw := strings.TrimSpace(refBody[idx+1:])
				var subs []string
				for _, s := range strings.Split(subRaw, ",") {
					s = strings.TrimSpace(s)
					if s != "" {
						if reWellToken.MatchString(s) {
							s = strings.ToUpper(s)
						}
						subs = append(subs, s)
					}
				}
				if objID != "" {
					rel.Targets = append(rel.Targets, RelationTarget{
						ObjectID:   objID,
						SubTargets: subs,
					})
				}
			} else {
				rel.Targets = append(rel.Targets, RelationTarget{
					ObjectID: strings.TrimSpace(refBody),
				})
			}
			continue
		}
		if eq := strings.IndexByte(tok, '='); eq > 0 {
			k := strings.ToLower(strings.TrimSpace(tok[:eq]))
			v := unquoteAttrVal(strings.TrimSpace(tok[eq+1:]))
			switch k {
			case "t", "time", "timepoint":
				rel.Timepoint = v
			case "fov", "fov_index":
				if n, err := strconv.Atoi(strings.TrimPrefix(strings.ToLower(v), "fov")); err == nil && n > 0 {
					rel.FOV = n
				}
			case "role":
				rel.Role = v
			case "note":
				rel.Note = v
			default:
				if rel.Extra == nil {
					rel.Extra = map[string]string{}
				}
				rel.Extra[k] = v
			}
			continue
		}
		low := strings.ToLower(tok)
		if strings.HasPrefix(low, "fov") && len(low) > 3 {
			if n, err := strconv.Atoi(low[3:]); err == nil && n > 0 {
				rel.FOV = n
				continue
			}
		}
		if reShorthandTime.MatchString(tok) && rel.Timepoint == "" {
			rel.Timepoint = tok
			continue
		}
	}
}

// tokenizeAttrBody splits `{@plate-1:A1,A2 6h fov=1 note="edge field"}` into tokens,
// respecting double-quoted values and keeping `@id:A1,A2` intact as one token.
func tokenizeAttrBody(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch == '"' {
			inQuote = !inQuote
			cur.WriteByte(ch)
			continue
		}
		if inQuote {
			cur.WriteByte(ch)
			continue
		}
		if ch == ' ' || ch == '\t' {
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
			continue
		}
		if ch == ',' {
			// Comma inside `@plate:A1,A2` stays attached; otherwise acts as separator.
			curStr := cur.String()
			if strings.HasPrefix(curStr, "@") && strings.Contains(curStr, ":") {
				cur.WriteByte(ch)
			} else if cur.Len() > 0 {
				out = append(out, curStr)
				cur.Reset()
			}
			continue
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func unquoteAttrVal(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		return v[1 : len(v)-1]
	}
	return v
}

func quoteAttrVal(v string) string {
	if strings.ContainsAny(v, " \t,}") {
		return `"` + strings.ReplaceAll(v, `"`, `'`) + `"`
	}
	return v
}

// FormatRelationAttr serializes an InlineRelation's semantic attributes into
// canonical `{@plate-1:A1 6h fov=1 role=obs}` syntax (empty string if no attributes).
func FormatRelationAttr(rel InlineRelation) string {
	var parts []string
	for _, t := range rel.Targets {
		obj := strings.TrimSpace(t.ObjectID)
		if obj == "" {
			continue
		}
		if len(t.SubTargets) > 0 {
			norm := make([]string, 0, len(t.SubTargets))
			for _, st := range t.SubTargets {
				st = strings.TrimSpace(st)
				if st == "" {
					continue
				}
				if reWellToken.MatchString(st) {
					st = strings.ToUpper(st)
				}
				norm = append(norm, st)
			}
			if len(norm) > 0 {
				parts = append(parts, "@"+obj+":"+strings.Join(norm, ","))
				continue
			}
		}
		parts = append(parts, "@"+obj)
	}
	if tp := strings.TrimSpace(rel.Timepoint); tp != "" {
		if reShorthandTime.MatchString(tp) {
			parts = append(parts, tp)
		} else {
			parts = append(parts, "t="+quoteAttrVal(tp))
		}
	}
	if rel.FOV > 0 {
		parts = append(parts, fmt.Sprintf("fov=%d", rel.FOV))
	}
	if role := strings.TrimSpace(rel.Role); role != "" {
		parts = append(parts, "role="+quoteAttrVal(role))
	}
	if note := strings.TrimSpace(rel.Note); note != "" {
		parts = append(parts, "note="+quoteAttrVal(note))
	}
	if len(rel.Extra) > 0 {
		keys := make([]string, 0, len(rel.Extra))
		for k := range rel.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if val := strings.TrimSpace(rel.Extra[k]); val != "" {
				parts = append(parts, k+"="+quoteAttrVal(val))
			}
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "{" + strings.Join(parts, " ") + "}"
}

// FormatAnnotatedEmbed formats a complete Obsidian wiki embed line with optional width & relation attr.
func FormatAnnotatedEmbed(assetPath, width string, rel InlineRelation) string {
	assetPath = strings.TrimSpace(filepath.ToSlash(assetPath))
	base := "![[" + assetPath
	if w := strings.TrimSpace(width); w != "" {
		base += "|" + w
	}
	base += "]]"
	if attr := FormatRelationAttr(rel); attr != "" {
		return base + attr
	}
	return base
}

// ResolveRelations extracts all inline relations from doc.Source (and any block-level
// attachment/figure container links) and resolves every target against doc.Modules.
func ResolveRelations(doc *Document) []ResolvedRelation {
	if doc == nil {
		return nil
	}
	flat, _ := flattenModules(doc.Modules)
	stepTree := ParseStepTree(doc.Source, doc)
	modByID := make(map[string]Module, len(flat))
	plates := make(map[string]*Plate)
	var plateIDs []string
	groupNames := map[string]GroupInfo{}
	for _, g := range doc.GroupList() {
		groupNames[g.ID] = g
	}
	for _, m := range flat {
		modByID[m.ID] = m
		if m.Kind == "plate" {
			if m.Plate != nil {
				plates[m.ID] = m.Plate
				plateIDs = append(plateIDs, m.ID)
			} else if p, err := plateFromData(m.Data); err == nil {
				cp := p
				plates[m.ID] = &cp
				plateIDs = append(plateIDs, m.ID)
			}
		}
	}
	sort.Strings(plateIDs)

	inlineList := ExtractInlineRelations(doc.Source, true)
	out := make([]ResolvedRelation, 0, len(inlineList))
	for _, ir := range inlineList {
		rr := ResolvedRelation{
			InlineRelation: ir,
			SourceKind:     "inline",
		}
		for _, tgt := range ir.Targets {
			objID := tgt.ObjectID
			subs := tgt.SubTargets
			// Shorthand `@A1` resolves to the unique plate in the document if unambiguous.
			if len(subs) == 0 && reWellToken.MatchString(objID) && len(plateIDs) == 1 {
				subs = []string{strings.ToUpper(objID)}
				objID = plateIDs[0]
			}
			if p, isPlate := plates[objID]; isPlate {
				geom, _ := geometryForPlate(p.Format, p.Topology, p.Rows, p.Columns)
				wellGroups := p.WellGroups()
				excl := map[string]bool{}
				for _, ew := range p.ExcludedWells {
					excl[strings.ToUpper(strings.TrimSpace(ew))] = true
				}
				if len(subs) == 0 {
					rr.Contexts = append(rr.Contexts, TargetContext{
						ObjectID:    p.ID,
						ObjectKind:  "plate",
						ObjectTitle: firstNonEmpty(p.Title, fmt.Sprintf("%d 孔板", p.Format)),
						Valid:       true,
					})
					continue
				}
				for _, well := range subs {
					w := strings.ToUpper(strings.TrimSpace(well))
					tc := TargetContext{
						ObjectID:    p.ID,
						SubTarget:   w,
						ObjectKind:  "plate_well",
						ObjectTitle: firstNonEmpty(p.Title, fmt.Sprintf("%d 孔板", p.Format)),
						WellRole:    p.Roles[w],
						Factors:     p.WellFactors(w),
						Excluded:    excl[w],
					}
					if !ValidPlateWell(w, p.Topology, geom.Rows, geom.Columns) {
						tc.Valid = false
						tc.Diagnostic = fmt.Sprintf("孔位 %s 超出 %s (%dx%d) 范围", w, p.ID, geom.Rows, geom.Columns)
						rr.Issues = append(rr.Issues, tc.Diagnostic)
					} else {
						tc.Valid = true
						if gid := wellGroups[w]; gid != "" {
							tc.GroupID = gid
							if gi, ok := groupNames[gid]; ok {
								tc.GroupName = firstNonEmpty(gi.Name, gi.ID)
								tc.GroupRole = gi.Role
							} else {
								tc.GroupName = gid
							}
						}
					}
					rr.Contexts = append(rr.Contexts, tc)
				}
				continue
			}
			if gi, isGroup := groupNames[objID]; isGroup {
				rr.Contexts = append(rr.Contexts, TargetContext{
					ObjectID:    gi.ID,
					ObjectKind:  "group",
					ObjectTitle: firstNonEmpty(gi.Name, gi.ID),
					GroupID:     gi.ID,
					GroupName:   firstNonEmpty(gi.Name, gi.ID),
					GroupRole:   gi.Role,
					Valid:       true,
				})
				continue
			}
			if m, ok := modByID[objID]; ok {
				rr.Contexts = append(rr.Contexts, TargetContext{
					ObjectID:    m.ID,
					ObjectKind:  m.Kind,
					ObjectTitle: firstNonEmpty(textValue(m.Data, "title"), m.ID),
					Valid:       true,
				})
				continue
			}
			if stepTree != nil {
				if sn, ok := stepTree.ByID[objID]; ok && sn != nil {
					rr.Contexts = append(rr.Contexts, TargetContext{
						ObjectID:    sn.ID,
						ObjectKind:  "step",
						ObjectTitle: fmt.Sprintf("步骤 %s · %s", sn.Number, sn.Title),
						Valid:       true,
					})
					continue
				}
			}
			diag := fmt.Sprintf("关联目标 @%s 在当前文档中未找到", objID)
			rr.Contexts = append(rr.Contexts, TargetContext{
				ObjectID:   objID,
				SubTarget:  strings.Join(subs, ","),
				Valid:      false,
				Diagnostic: diag,
			})
			rr.Issues = append(rr.Issues, diag)
		}
		out = append(out, rr)
	}

	// Also project structured attachment@1 items that declare container_ref + well_id
	for _, m := range flat {
		if m.Kind != "attachment" {
			continue
		}
		containerRef := textValue(m.Data, "container_ref")
		items := mapSlice(m.Data["items"])
		for _, it := range items {
			path := firstNonEmpty(textValue(it, "path"), textValue(it, "src"), textValue(it, "file"))
			well := strings.ToUpper(strings.TrimSpace(textValue(it, "well_id")))
			if path == "" || (containerRef == "" && well == "") {
				continue
			}
			fov := int(numOrZero(it["fov_index"]))
			tp := textValue(it, "timepoint")
			ir := InlineRelation{
				SyntaxKind: "attachment_item",
				Asset:      path,
				Timepoint:  tp,
				FOV:        fov,
				Role:       firstNonEmpty(textValue(it, "role"), "observation"),
			}
			if containerRef != "" {
				var subs []string
				if well != "" {
					subs = []string{well}
				}
				ir.Targets = []RelationTarget{{ObjectID: containerRef, SubTargets: subs}}
			}
			rr := ResolvedRelation{
				InlineRelation: ir,
				SourceKind:     "attachment",
				ModuleID:       m.ID,
			}
			if p, ok := plates[containerRef]; ok {
				tc := TargetContext{
					ObjectID:    p.ID,
					SubTarget:   well,
					ObjectKind:  "plate_well",
					ObjectTitle: firstNonEmpty(p.Title, fmt.Sprintf("%d 孔板", p.Format)),
					Valid:       true,
				}
				if well != "" {
					if gid := p.WellGroups()[well]; gid != "" {
						tc.GroupID = gid
						if gi, ok := groupNames[gid]; ok {
							tc.GroupName = firstNonEmpty(gi.Name, gi.ID)
							tc.GroupRole = gi.Role
						} else {
							tc.GroupName = gid
						}
					}
					tc.Factors = p.WellFactors(well)
				}
				rr.Contexts = append(rr.Contexts, tc)
			}
			out = append(out, rr)
		}
	}
	return out
}

// RelationsForTarget returns all resolved relations pointing to (objectID, subTarget).
// If subTarget is empty, matches any relation referencing objectID.
func RelationsForTarget(rels []ResolvedRelation, objectID, subTarget string) []ResolvedRelation {
	objectID = strings.TrimSpace(objectID)
	subTarget = strings.ToUpper(strings.TrimSpace(subTarget))
	if objectID == "" {
		return nil
	}
	var out []ResolvedRelation
	for _, r := range rels {
		matched := false
		for _, tc := range r.Contexts {
			if tc.ObjectID != objectID {
				continue
			}
			if subTarget == "" || strings.EqualFold(tc.SubTarget, subTarget) {
				matched = true
				break
			}
		}
		if !matched {
			for _, t := range r.Targets {
				if t.ObjectID != objectID {
					continue
				}
				if subTarget == "" {
					matched = true
					break
				}
				for _, s := range t.SubTargets {
					if strings.EqualFold(s, subTarget) {
						matched = true
						break
					}
				}
			}
		}
		if matched {
			out = append(out, r)
		}
	}
	return out
}

// RelationAtOffset finds the resolved inline relation whose span contains byte offset off.
func RelationAtOffset(rels []ResolvedRelation, off int) (ResolvedRelation, bool) {
	for _, r := range rels {
		if r.SourceKind != "inline" {
			continue
		}
		if off >= r.SpanStart && off <= r.SpanEnd {
			return r, true
		}
	}
	return ResolvedRelation{}, false
}

// SetInlineRelationAtSpan replaces or attaches `{...}` on the inline embed/link at [spanStart, spanEnd).
func SetInlineRelationAtSpan(source string, spanStart, spanEnd int, rel InlineRelation) (string, error) {
	if spanStart < 0 || spanEnd > len(source) || spanStart >= spanEnd {
		return "", fmt.Errorf("invalid inline relation span [%d:%d]", spanStart, spanEnd)
	}
	// Re-parse the target span to find exact AttrStart/AttrEnd
	all := ExtractInlineRelations(source, false)
	for _, item := range all {
		if item.SpanStart == spanStart && item.SpanEnd == spanEnd {
			formatted := FormatRelationAttr(rel)
			return source[:item.AttrStart] + formatted + source[item.AttrEnd:], nil
		}
	}
	return "", fmt.Errorf("no embed or link found at span [%d:%d]", spanStart, spanEnd)
}

// SetInlineRelationByAsset finds the first embed/link matching assetPath and sets its `{...}` attribute.
func SetInlineRelationByAsset(source, assetPath string, rel InlineRelation) (string, error) {
	want := strings.TrimSpace(filepath.ToSlash(assetPath))
	wantTrim := strings.TrimPrefix(want, "./")
	all := ExtractInlineRelations(source, false)
	for _, item := range all {
		cur := strings.TrimSpace(filepath.ToSlash(item.Asset))
		if cur == want || strings.TrimPrefix(cur, "./") == wantTrim || filepath.Base(cur) == filepath.Base(wantTrim) {
			formatted := FormatRelationAttr(rel)
			return source[:item.AttrStart] + formatted + source[item.AttrEnd:], nil
		}
	}
	return "", fmt.Errorf("asset link %q not found in document", assetPath)
}

// maskCodeRegions returns a same-length string where YAML frontmatter, fenced code
// blocks (```...```), and inline code spans (`...`) are replaced with spaces, while
// preserving newlines so byte offsets and line numbers remain 100% identical to source.
func maskCodeRegions(source string) string {
	b := []byte(source)
	_, startOffset, hasFM := ScanFrontmatter(source)
	if hasFM && startOffset <= len(b) {
		for i := 0; i < startOffset; i++ {
			if b[i] != '\n' {
				b[i] = ' '
			}
		}
	}
	inFence := false
	fenceTicks := 0
	i := startOffset
	for i < len(b) {
		lineStart := i
		lineEnd := i
		for lineEnd < len(b) && b[lineEnd] != '\n' {
			lineEnd++
		}
		trimmed := strings.TrimSpace(string(b[lineStart:lineEnd]))
		if !inFence {
			if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
				inFence = true
				fenceTicks = ticks
				for k := lineStart; k < lineEnd; k++ {
					b[k] = ' '
				}
			} else {
				// Mask inline code spans `...` and math spans $...$ on this line
				maskInlineSpans(b[lineStart:lineEnd])
			}
		} else {
			for k := lineStart; k < lineEnd; k++ {
				b[k] = ' '
			}
			if isClosingBacktickFence(trimmed, fenceTicks) {
				inFence = false
				fenceTicks = 0
			}
		}
		if lineEnd < len(b) {
			i = lineEnd + 1
		} else {
			break
		}
	}
	return string(b)
}

func maskInlineSpans(line []byte) {
	// Mask inline backtick spans `...`
	for i := 0; i < len(line); i++ {
		if line[i] == '`' {
			start := i
			ticks := 0
			for i < len(line) && line[i] == '`' {
				ticks++
				i++
			}
			end := -1
			for j := i; j <= len(line)-ticks; j++ {
				match := true
				for t := 0; t < ticks; t++ {
					if line[j+t] != '`' {
						match = false
						break
					}
				}
				if match {
					end = j + ticks
					break
				}
			}
			if end > start {
				for k := start; k < end; k++ {
					line[k] = ' '
				}
				i = end - 1
			}
		}
	}
}
