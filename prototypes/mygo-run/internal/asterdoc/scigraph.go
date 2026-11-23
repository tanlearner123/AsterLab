package asterdoc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SciContainer represents one live plate/container node in the document's Scientific Graph.
type SciContainer struct {
	ID            string
	Title         string
	Format        int
	Assigned      bool // false when plate has zero assigned group wells (unassigned blank plate!)
	TotalWells    int
	SampleWells   int // Assigned wells excluding role == "blank" AND excluded_wells
	BlankWells    int
	ExcludedWells int
	BlankSet      map[string]bool
	ExcludedSet   map[string]bool
	SampleByWell  map[string]string
	GroupWells    map[string][]string
	ByteOffset    int
}

// SciUnit represents one unified experimental unit (one physical well/sample slot bound to its
// experimental group, replicate index, and blank/excluded status) consumed across calculators,
// capture grids, and data tables within the same document.
type SciUnit struct {
	PlateID   string
	WellID    string
	GroupID   string
	GroupName string
	SampleID  string
	Role      string // "treatment" | "control" | "blank" | "excluded"
	RepIndex  int    // 1-based technical replicate index within its group
	Blank     bool
	Excluded  bool
}

// SciGroup represents one experimental group node from group_table@1.
type SciGroup struct {
	ID         string
	Name       string
	Role       string
	Condition  string
	Repeats    int
	ByteOffset int
}

// SciReagent captures upstream quantitative reagent facts extracted from calculators/params
// so downstream steps and Alt+Hover fallback can inherit known stock concentrations or MW.
type SciReagent struct {
	Name           string
	StockVal       float64
	StockUnit      string
	TargetVal      float64
	TargetUnit     string
	ConcWV         float64
	MW             float64
	SourceModuleID string
	ByteOffset     int
}

// SciAnchor captures a declared step timepoint/reference anchor such as {#t0} or {#obs-6h},
// plus its optional live/persisted RecordedAt wall-clock timestamp when executed in Run/Lab mode.
type SciAnchor struct {
	ID         string
	StepTitle  string
	ByteOffset int
	RecordedAt time.Time
}

// SciCalcOutput records one evaluated output (or parameter) row from an upstream calculator@1
// so downstream calculators can reactively chain via `@calc.<module_id>.<key>`.
type SciCalcOutput struct {
	ModuleID   string
	Title      string
	Key        string
	Label      string
	Amount     float64
	Unit       string
	IsFormula  bool
	ByteOffset int
}

// SciModuleNode records one module's position and kind in document order.
type SciModuleNode struct {
	ID    string
	Kind  string
	Title string
	Start int
	End   int
}

// SciGraph is the document-level reactive scientific semantic graph compiled from the AsterML AST
// in < 0.5 ms and cached by source hash. It serves as the single truth for:
//   - Cross-module live sync (plate SampleWells -> calculator @plate.sample_wells & attachment grids)
//   - Deterministic structural zone gating (blocking headings/metadata/materials/review from NL popups)
//   - Dynamic upstream context inheritance (reagents, plate, anchors, enclosing step, previous line)
type SciGraph struct {
	SourceHash      string
	Source          string
	Containers      []SciContainer
	Groups          []SciGroup
	Units           []SciUnit
	UnitByPlateWell map[string]SciUnit // key: plateID + ":" + strings.ToUpper(wellID)
	Reagents        []SciReagent
	Anchors         []SciAnchor
	CalcOutputs     []SciCalcOutput
	Modules         []SciModuleNode
	Outline         []*OutlineNode
	Steps           *StepTree
}

// NLZone classifies a line's structural role in the AsterML document.
type NLZone string

const (
	ZoneStepBody   NLZone = "step_body"
	ZoneDesignPlan NLZone = "design_plan"
	ZoneHeading    NLZone = "heading"
	ZoneMetadata   NLZone = "metadata"
	ZoneMaterials  NLZone = "materials"
	ZoneReview     NLZone = "review"
)

// ScientificLineContext is the dynamic upstream context snapshot at a specific byte offset.
type ScientificLineContext struct {
	Offset           int
	Zone             NLZone
	ChapterTitle     string
	StepTitle        string
	PrevLine         string // Immediately preceding non-empty prose line within the same step/section
	ActivePlate      *SciContainer
	UpstreamGroups   []SciGroup
	UpstreamReagents []SciReagent
	UpstreamAnchors  []SciAnchor
	NextModuleKind   string
}

// AllowedForNL reports whether this structural zone is eligible for Alt+Hover NL->Module conversion.
func (ctx ScientificLineContext) AllowedForNL() bool {
	return ctx.Zone == ZoneStepBody || ctx.Zone == ZoneDesignPlan
}

// AllowsKind checks whether a routed module kind is appropriate for this structural zone.
func (ctx ScientificLineContext) AllowsKind(kind string) bool {
	switch ctx.Zone {
	case ZoneStepBody:
		return kind == "timer" || kind == "calculator" || kind == "plate_group" || kind == "attachment"
	case ZoneDesignPlan:
		return kind == "plate_group" || kind == "calculator"
	default:
		return false
	}
}

// LookupReagent finds the most recent upstream reagent matching name (case-insensitive substring).
// Structured module facts (calculator/param_table) always take precedence over lightweight prose facts!
func (ctx ScientificLineContext) LookupReagent(name string) *SciReagent {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil
	}
	var proseFallback *SciReagent
	for i := len(ctx.UpstreamReagents) - 1; i >= 0; i-- {
		r := &ctx.UpstreamReagents[i]
		rn := strings.ToLower(strings.TrimSpace(r.Name))
		if rn != "" && (rn == want || strings.Contains(rn, want) || strings.Contains(want, rn)) {
			if r.SourceModuleID != "prose" {
				return r
			}
			if proseFallback == nil {
				proseFallback = r
			}
		}
	}
	return proseFallback
}

// LookupReagentForDilution finds the best upstream stock reagent whose concentration is
// physically higher than the target concentration (C1 > C2 in the same unit family), preventing
// working concentrations from ever being mistakenly inherited as stock solutions.
func (ctx ScientificLineContext) LookupReagentForDilution(name string, targetVal float64, targetUnit string) *SciReagent {
	want := strings.ToLower(strings.TrimSpace(name))
	if want == "" {
		return nil
	}
	s2, f2 := concFactorToBase(targetUnit)
	tBase := targetVal * s2

	satisfiesPhys := func(r *SciReagent) bool {
		if r == nil || r.StockVal <= 0 || r.StockUnit == "" {
			return false
		}
		if targetVal <= 0 {
			return true
		}
		s1, f1 := concFactorToBase(r.StockUnit)
		if f1 == f2 && (f1 == "mass_conc" || f1 == "molar") {
			return r.StockVal*s1 > tBase
		}
		// Cross-dimension (mass_conc <-> molar): STRICTLY forbidden unless molecular weight MW > 0!
		if (f1 == "mass_conc" && f2 == "molar") || (f1 == "molar" && f2 == "mass_conc") {
			if r.MW <= 0 {
				return false
			}
			if f1 == "mass_conc" && f2 == "molar" {
				// ng/mL -> nM: (ng/mL) / MW(g/mol) * 1000 = nM
				c1nM := (r.StockVal * s1) * 1000.0 / r.MW
				return c1nM > tBase
			}
			// nM -> ng/mL: nM * MW / 1000
			c1ngML := (r.StockVal * s1) * r.MW / 1000.0
			return c1ngML > tBase
		}
		return false
	}

	var proseFallback *SciReagent
	for i := len(ctx.UpstreamReagents) - 1; i >= 0; i-- {
		r := &ctx.UpstreamReagents[i]
		rn := strings.ToLower(strings.TrimSpace(r.Name))
		if rn != "" && (rn == want || strings.Contains(rn, want) || strings.Contains(want, rn)) {
			if !satisfiesPhys(r) {
				continue
			}
			if r.SourceModuleID != "prose" {
				return r
			}
			if proseFallback == nil {
				proseFallback = r
			}
		}
	}
	return proseFallback
}
// ONLY when the sentence is short or contains implicit anaphoric references ("同上"/"刚才"/"再做").
func (ctx ScientificLineContext) MicroContextText(sentence string) string {
	s := strings.TrimSpace(sentence)
	if s == "" || ctx.StepTitle == "" {
		return s
	}
	if needsStepMicroContext(s) {
		return fmt.Sprintf("[%s] %s", ctx.StepTitle, s)
	}
	return s
}

var (
	reAnaphoricCue  = regexp.MustCompile(`(同上|刚才|上述|上次|上一步|再做|再照|再孵育|补足|继续|按之前|按上面|该溶液|该步骤)`)
	reMetaKeyBullet = regexp.MustCompile(`^[ \t]*(?:[-*+][ \t]+)?\*\*[^*\r\n]{1,18}\*\*\s*[：:]`)
	rePlainMetaKey  = regexp.MustCompile(`^[ \t]*(?:[-*+][ \t]+)?(?:研究体系|单一自变量|实验目的|实验目标|检测指标|试剂耗材|耗材清单|实验背景|设备参数|注意事项|结论|总结|讨论|复盘)\s*[：:]`)
	reMaterialsChap = regexp.MustCompile(`(?i)(试剂|耗材|材料|仪器|设备|materials|reagents|equipment)`)
	reReviewChap    = regexp.MustCompile(`(?i)(讨论|复盘|总结|结论|数据分析|结果分析|discussion|troubleshooting|analysis|review|notes)`)
	reDesignChap    = regexp.MustCompile(`(?i)(设计|分组|孔板|布局|因子|配液|计算|方案概览|design|layout|setup)`)
	reKnownReagName   = regexp.MustCompile(`(?i)\b(VEGF|EGF|bFGF|TGF-?β1?|GelMA|LAP|FBS|DMEM|RPMI|PBS|DPBS|DMSO|PFA|BSA|Triton|PMSF|RIPA|IPTG|EDTA|HEPES|Tris|NaCl|DTT|DAPI|CCK-?8|Matrigel|SYBR|Doxorubicin|Cisplatin|Rapamycin|Ampicillin|Kanamycin|Puromycin|Primer|引物|一抗|二抗|多聚甲醛)\b`)
	reCustomReagStock = regexp.MustCompile(`(?i)([A-Za-z][A-Za-z0-9_-]{1,24})\s*(?:母液|储液|原液|stock|浓缩液)`)
	reCustomReagDil   = regexp.MustCompile(`(?i)(?:将|把|取)\s*([A-Za-z][A-Za-z0-9_-]{1,24})\s*(?:母液|储液|原液)?\s*(?:稀释|配制|配成|梯度)`)
	reProseStockCue   = regexp.MustCompile(`(?i)(母液|储液|原液|stock|浓缩液|MW|分子量)`)
	reProseWorkingCue = regexp.MustCompile(`(?i)(工作浓度|终浓度|工作液|处理浓度|稀释至|稀释到|稀释成)`)
)

func extractReagentName(text string) string {
	if m := reKnownReagName.FindString(text); m != "" {
		return m
	}
	if sm := reCustomReagStock.FindStringSubmatch(text); len(sm) >= 2 {
		return strings.TrimSpace(sm[1])
	}
	if sm := reCustomReagDil.FindStringSubmatch(text); len(sm) >= 2 {
		return strings.TrimSpace(sm[1])
	}
	return ""
}

func needsStepMicroContext(sentence string) bool {
	if reAnaphoricCue.MatchString(sentence) {
		return true
	}
	// Very short implicit commands inside a step (e.g. "继续 3 轮", "再照 30 秒")
	return len([]rune(sentence)) <= 12
}

var (
	sciGraphMu    sync.RWMutex
	sciGraphCache = map[string]*SciGraph{}
)

// BuildSciGraph compiles (or returns cached) the live Scientific Graph for doc in < 0.5 ms.
func BuildSciGraph(doc *Document) *SciGraph {
	g, _, _ := BuildSciGraphWithStats(doc)
	return g
}

// BuildSciGraphWithStats returns the live Scientific Graph along with whether it hit the
// SHA256 source cache and the exact wall-clock lookup/compile duration.
func BuildSciGraphWithStats(doc *Document) (g *SciGraph, cacheHit bool, dur time.Duration) {
	t0 := time.Now()
	if doc == nil {
		return &SciGraph{}, false, time.Since(t0)
	}
	sum := sha256.Sum256([]byte(doc.Source))
	hash := hex.EncodeToString(sum[:12])

	sciGraphMu.RLock()
	if cached, ok := sciGraphCache[hash]; ok {
		sciGraphMu.RUnlock()
		return cached, true, time.Since(t0)
	}
	sciGraphMu.RUnlock()

	g = compileSciGraph(doc, hash)

	sciGraphMu.Lock()
	if len(sciGraphCache) > 32 {
		sciGraphCache = map[string]*SciGraph{}
	}
	sciGraphCache[hash] = g
	sciGraphMu.Unlock()
	return g, false, time.Since(t0)
}

func compileSciGraph(doc *Document, hash string) *SciGraph {
	g := &SciGraph{
		SourceHash:      hash,
		Source:          doc.Source,
		UnitByPlateWell: map[string]SciUnit{},
		Outline:         ParseDocumentOutline(doc.Source, doc),
		Steps:           ParseStepTree(doc.Source, doc),
	}

	// 1. Collect step anchors ({#t0}, {#obs-6h}, ...) and link any persisted timer timestamp inside that step!
	var walkSteps func(nodes []*StepNode)
	walkSteps = func(nodes []*StepNode) {
		for _, sn := range nodes {
			if sn == nil {
				continue
			}
			if sn.Anchor != "" {
				anc := SciAnchor{
					ID:         sn.Anchor,
					StepTitle:  sn.Title,
					ByteOffset: sn.LineStart,
				}
				// Check if any timer module inside this anchored step has a recorded started_at / completed_at timestamp
				for _, m := range doc.Modules {
					if m.Kind == "timer" && m.Start >= sn.LineStart && m.Start <= sn.ScopeEnd {
						if ts := firstNonEmpty(textValue(m.Data, "started_at"), textValue(m.Data, "completed_at")); ts != "" {
							if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
								anc.RecordedAt = parsed
								break
							}
						}
					}
				}
				g.Anchors = append(g.Anchors, anc)
			}
			walkSteps(sn.Children)
		}
	}
	if g.Steps != nil {
		walkSteps(g.Steps.Roots)
	}

	// 1b. Lightweight single-document prose reagent fact harvesting (strictly lower priority than structured modules).
	// Extracts explicit stock concentrations / MW written in Design or Materials bullets within the SAME file.
	harvestProseReagents(doc.Source, g)

	// 2. Compile modules in document order into Containers, Groups, Reagents, and CalcOutputs
	for _, m := range doc.Modules {
		g.Modules = append(g.Modules, SciModuleNode{
			ID:    m.ID,
			Kind:  m.Kind,
			Title: textValue(m.Data, "title"),
			Start: m.Start,
			End:   m.End,
		})

		switch m.Kind {
		case "plate":
			sc := compilePlateContainer(m)
			g.Containers = append(g.Containers, sc)
		case "group_table":
			for _, rm := range mapSlice(m.Data["rows"]) {
				g.Groups = append(g.Groups, SciGroup{
					ID:         textValue(rm, "id"),
					Name:       textValue(rm, "name"),
					Role:       textValue(rm, "role"),
					Condition:  textValue(rm, "condition"),
					Repeats:    int(numOrZero(rm["technical_repeats"])),
					ByteOffset: m.Start,
				})
			}
		case "calculator":
			if reag, ok := extractReagentFromCalculator(m); ok {
				g.Reagents = append(g.Reagents, reag)
			}
		case "timer":
			// Allow ANY timer declaring `anchor` + `started_at`/`completed_at` to stamp that anchor
			// for all sibling downstream window timers in the same document!
			if ancID := strings.TrimSpace(textValue(m.Data, "anchor")); ancID != "" {
				if ts := firstNonEmpty(textValue(m.Data, "started_at"), textValue(m.Data, "completed_at")); ts != "" {
					if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
						found := false
						for i := range g.Anchors {
							if g.Anchors[i].ID == ancID {
								if g.Anchors[i].RecordedAt.IsZero() {
									g.Anchors[i].RecordedAt = parsed
								}
								found = true
								break
							}
						}
						if !found {
							g.Anchors = append(g.Anchors, SciAnchor{ID: ancID, StepTitle: textValue(m.Data, "title"), ByteOffset: m.Start, RecordedAt: parsed})
						}
					}
				}
			}
		}
	}

	// 2b. Build unified experimental units table (SciUnit) linking every plate well to group/role/rep/excluded
	populateSciUnits(g)

	// 3. Evaluate upstream calculator outputs in document order for reactive `@calc.<id>.<key>` chaining
	populateCalcOutputsDAG(doc, g)
	return g
}

func populateSciUnits(g *SciGraph) {
	if g == nil {
		return
	}
	groupNames := map[string]string{}
	groupRoles := map[string]string{}
	for _, grp := range g.Groups {
		if grp.ID != "" {
			groupNames[grp.ID] = firstNonEmpty(grp.Name, grp.ID)
			groupRoles[grp.ID] = grp.Role
		}
	}
	for _, c := range g.Containers {
		for gid, wells := range c.GroupWells {
			gName := firstNonEmpty(groupNames[gid], gid)
			gRole := firstNonEmpty(groupRoles[gid], "treatment")
			rep := 0
			for _, w := range wells {
				uw := strings.ToUpper(strings.TrimSpace(w))
				if uw == "" {
					continue
				}
				isBlank := c.BlankSet != nil && c.BlankSet[uw]
				isExcl := c.ExcludedSet != nil && c.ExcludedSet[uw]
				role := gRole
				if isBlank {
					role = "blank"
				} else if isExcl {
					role = "excluded"
				} else {
					rep++
				}
				u := SciUnit{
					PlateID:   c.ID,
					WellID:    uw,
					GroupID:   gid,
					GroupName: gName,
					SampleID:  c.SampleByWell[uw],
					Role:      role,
					RepIndex:  rep,
					Blank:     isBlank,
					Excluded:  isExcl,
				}
				g.Units = append(g.Units, u)
				g.UnitByPlateWell[c.ID+":"+uw] = u
			}
		}
	}
}

// AllSampleWells returns the sum of active experimental wells (excluding blank & excluded wells)
// across all plates in the document (for multi-plate mastermix totals via `@plate.all_sample_wells`).
func (g *SciGraph) AllSampleWells() int {
	if g == nil {
		return 0
	}
	tot := 0
	for _, c := range g.Containers {
		tot += c.SampleWells
	}
	return tot
}

// UnitForWell looks up the unified SciUnit for a plate+well coordinate.
func (g *SciGraph) UnitForWell(plateID, wellID string) (SciUnit, bool) {
	if g == nil || g.UnitByPlateWell == nil {
		return SciUnit{}, false
	}
	uw := strings.ToUpper(strings.TrimSpace(wellID))
	if plateID != "" {
		if u, ok := g.UnitByPlateWell[plateID+":"+uw]; ok {
			return u, true
		}
	}
	for _, c := range g.Containers {
		if u, ok := g.UnitByPlateWell[c.ID+":"+uw]; ok {
			return u, true
		}
	}
	return SciUnit{}, false
}

// SciGroupReadoutStats holds per-group summary statistics computed from SciUnit-linked well readings,
// automatically excluding blank and excluded wells and optionally subtracting blank mean background.
type SciGroupReadoutStats struct {
	GroupID   string
	GroupName string
	Count     int
	Mean      float64
	SD        float64
	CorrMean  float64 // Mean - BlankMean when blank wells have readings
}

// SummarizeWellReadings aggregates numeric well readings by SciUnit group, automatically
// skipping excluded wells and computing blank-subtracted means when blank wells are present.
func (g *SciGraph) SummarizeWellReadings(plateID string, readings map[string]float64) (blankMean float64, hasBlank bool, stats []SciGroupReadoutStats) {
	if g == nil || len(readings) == 0 {
		return 0, false, nil
	}
	var blankVals []float64
	byGroup := map[string][]float64{}
	groupName := map[string]string{}
	var order []string
	seenOrd := map[string]bool{}

	for _, u := range g.Units {
		if plateID != "" && u.PlateID != plateID {
			continue
		}
		val, ok := readings[u.WellID]
		if !ok || u.Excluded {
			continue
		}
		if u.Blank {
			blankVals = append(blankVals, val)
			continue
		}
		if !seenOrd[u.GroupID] {
			seenOrd[u.GroupID] = true
			order = append(order, u.GroupID)
			groupName[u.GroupID] = u.GroupName
		}
		byGroup[u.GroupID] = append(byGroup[u.GroupID], val)
	}
	if len(blankVals) > 0 {
		sum := 0.0
		for _, v := range blankVals {
			sum += v
		}
		blankMean = sum / float64(len(blankVals))
		hasBlank = true
	}
	for _, gid := range order {
		vals := byGroup[gid]
		if len(vals) == 0 {
			continue
		}
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		mean := sum / float64(len(vals))
		sd := 0.0
		if len(vals) > 1 {
			vSum := 0.0
			for _, v := range vals {
				d := v - mean
				vSum += d * d
			}
			sd = sqrtFloat(vSum / float64(len(vals)-1))
		}
		stats = append(stats, SciGroupReadoutStats{
			GroupID:   gid,
			GroupName: groupName[gid],
			Count:     len(vals),
			Mean:      round4(mean),
			SD:        round4(sd),
			CorrMean:  round4(mean - blankMean),
		})
	}
	return blankMean, hasBlank, stats
}

func sqrtFloat(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 16; i++ {
		z = 0.5 * (z + x/z)
	}
	return z
}

func harvestProseReagents(src string, g *SciGraph) {
	if src == "" || g == nil {
		return
	}
	_, bodyStart, _ := ScanFrontmatter(src)
	inFence := false
	off := 0
	for _, line := range strings.Split(src, "\n") {
		curOff := off
		off += len(line) + 1
		if curOff < bodyStart {
			continue
		}
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		// Never mis-harvest working/final concentrations or dilution actions as stock solutions!
		if reProseWorkingCue.MatchString(trim) {
			continue
		}
		if !reProseStockCue.MatchString(trim) {
			continue
		}
		reagName := extractReagentName(trim)
		if reagName == "" {
			continue
		}
		r := SciReagent{
			Name:           reagName,
			SourceModuleID: "prose",
			ByteOffset:     curOff,
		}
		hasFact := false
		if cs := reNLConc.FindStringSubmatch(trim); len(cs) >= 3 {
			if v, err := strconv.ParseFloat(cs[1], 64); err == nil && v > 0 {
				r.StockVal = v
				r.StockUnit = strings.ReplaceAll(strings.ReplaceAll(cs[2], "µ", "u"), "μ", "u")
				hasFact = true
			}
		}
		if mwM := reNLMW.FindStringSubmatch(trim); len(mwM) >= 2 {
			if v, err := strconv.ParseFloat(mwM[1], 64); err == nil && v > 0 {
				r.MW = v
				hasFact = true
			}
		}
		if !hasFact {
			if pm := reNLPercentWV.FindStringSubmatch(trim); len(pm) >= 2 {
				if v, err := strconv.ParseFloat(pm[1], 64); err == nil && v > 0 {
					r.ConcWV = v
					hasFact = true
				}
			}
		}
		if hasFact {
			g.Reagents = append(g.Reagents, r)
		}
	}
}

func populateCalcOutputsDAG(doc *Document, g *SciGraph) {
	if doc == nil || g == nil {
		return
	}
	baseVars := map[string]float64{}
	baseUnits := map[string]string{}
	if pl := g.PrimaryPlate(); pl != nil && pl.Assigned {
		if pl.SampleWells > 0 {
			baseVars["plate_sample_wells"] = float64(pl.SampleWells)
			baseUnits["plate_sample_wells"] = "孔"
		}
		baseVars["plate_blank_wells"] = float64(pl.BlankWells)
		baseUnits["plate_blank_wells"] = "孔"
		if pl.TotalWells > 0 {
			baseVars["plate_wells"] = float64(pl.TotalWells)
			baseVars["n_wells"] = float64(pl.TotalWells)
			baseUnits["plate_wells"] = "孔"
		}
		for gk, ws := range pl.GroupWells {
			active := 0
			for _, w := range ws {
				if !pl.BlankSet[w] && !pl.ExcludedSet[w] {
					active++
				}
			}
			baseVars["group_"+sanitizeCalcVar(gk)+"_wells"] = float64(active)
			baseUnits["group_"+sanitizeCalcVar(gk)+"_wells"] = "孔"
		}
	}
	if allSW := g.AllSampleWells(); allSW > 0 {
		baseVars["plate_all_sample_wells"] = float64(allSW)
		baseUnits["plate_all_sample_wells"] = "孔"
	}
	for _, c := range g.Containers {
		if !c.Assigned {
			continue
		}
		cid := sanitizeCalcVar(c.ID)
		baseVars["plate_"+cid+"_sample_wells"] = float64(c.SampleWells)
		baseVars["plate_"+cid+"_wells"] = float64(c.TotalWells)
		baseVars["plate_"+cid+"_blank_wells"] = float64(c.BlankWells)
		baseUnits["plate_"+cid+"_sample_wells"] = "孔"
	}
	for _, m := range doc.Modules {
		if m.Kind == "param_table" {
			for _, rm := range mapSlice(m.Data["rows"]) {
				k := strings.TrimSpace(textValue(rm, "key"))
				if k == "" {
					continue
				}
				u := strings.TrimSpace(textValue(rm, "unit"))
				if n, ok := numericValue(rm["value"]); ok {
					baseVars[k] = n
					baseVars["param_"+k] = n
					if u != "" {
						baseUnits[k] = u
						baseUnits["param_"+k] = u
					}
				}
				if act, ok := numericValue(rm["actual"]); ok {
					baseVars[k] = act
					baseVars["param_"+k] = act
					if u != "" {
						baseUnits[k] = u
						baseUnits["param_"+k] = u
					}
				}
			}
		}
	}
	for _, m := range doc.Modules {
		if m.Kind != "calculator" {
			continue
		}
		rows := mapSlice(m.Data["rows"])
		if len(rows) == 0 {
			continue
		}
		localVars := make(map[string]float64, len(baseVars)+len(rows))
		for k, v := range baseVars {
			localVars[k] = v
		}
		title := firstNonEmpty(textValue(m.Data, "title"), m.ID)
		midSafe := sanitizeCalcVar(m.ID)
		for _, rm := range rows {
			k := strings.TrimSpace(textValue(rm, "key"))
			lbl := firstNonEmpty(textValue(rm, "label"), k)
			unit := textValue(rm, "unit")
			formula := strings.TrimSpace(textValue(rm, "formula"))
			var val float64
			var ok bool
			if formula == "" {
				valStr := strings.TrimSpace(fmt.Sprintf("%v", rm["value"]))
				if strings.HasPrefix(valStr, "@") {
					exp := expandCalcFormulaRefs(valStr)
					if v, exists := localVars[exp]; exists {
						if fromU := baseUnits[exp]; fromU != "" && unit != "" {
							if conv, cOK, sameDim := ConvertScientificUnit(v, fromU, unit); cOK && sameDim {
								val, ok = conv, true
							}
						} else {
							val, ok = v, true
						}
					}
				}
				if !ok {
					val, ok = numericValue(rm["value"])
				}
			} else {
				if v, err := EvalFormula(expandCalcFormulaRefs(formula), localVars); err == nil {
					val, ok = v, true
				}
			}
			if ok && k != "" {
				localVars[k] = val
				varKey := "calc_" + midSafe + "_" + sanitizeCalcVar(k)
				baseVars[varKey] = val
				if unit != "" {
					baseUnits[varKey] = unit
				}
				g.CalcOutputs = append(g.CalcOutputs, SciCalcOutput{
					ModuleID:   m.ID,
					Title:      title,
					Key:        k,
					Label:      lbl,
					Amount:     round4(val),
					Unit:       unit,
					IsFormula:  formula != "",
					ByteOffset: m.Start,
				})
			}
		}
	}
}

// AnchorByID looks up a compiled time anchor (e.g. "t0") in the single-document SciGraph.
func (g *SciGraph) AnchorByID(id string) *SciAnchor {
	if g == nil {
		return nil
	}
	want := strings.TrimSpace(strings.TrimPrefix(id, "#"))
	for i := range g.Anchors {
		if strings.EqualFold(strings.TrimSpace(g.Anchors[i].ID), want) {
			cp := g.Anchors[i]
			return &cp
		}
	}
	return nil
}

// UpstreamCalcOutputs returns all evaluated formula outputs from calculators appearing
// strictly BEFORE beforeModuleID in the current document (preventing circular dependencies).
func (g *SciGraph) UpstreamCalcOutputs(beforeModuleID string) []SciCalcOutput {
	if g == nil || len(g.CalcOutputs) == 0 {
		return nil
	}
	limitOff := 1 << 30
	for _, mn := range g.Modules {
		if mn.ID == beforeModuleID {
			limitOff = mn.Start
			break
		}
	}
	var out []SciCalcOutput
	for _, co := range g.CalcOutputs {
		if co.ModuleID != beforeModuleID && co.ByteOffset < limitOff && co.IsFormula {
			out = append(out, co)
		}
	}
	return out
}

func compilePlateContainer(m Module) SciContainer {
	sc := SciContainer{
		ID:           m.ID,
		Title:        firstNonEmpty(textValue(m.Data, "title"), m.ID),
		Format:       int(numOrZero(m.Data["format"])),
		BlankSet:     map[string]bool{},
		ExcludedSet:  map[string]bool{},
		SampleByWell: map[string]string{},
		GroupWells:   map[string][]string{},
		ByteOffset:   m.Start,
	}
	if sm, ok := m.Data["samples"].(map[string]any); ok {
		for w, sv := range sm {
			if uw := strings.ToUpper(strings.TrimSpace(w)); uw != "" {
				if s := strings.TrimSpace(fmt.Sprint(sv)); s != "" {
					sc.SampleByWell[uw] = s
				}
			}
		}
	}
	if m.Plate != nil {
		for w, s := range m.Plate.Samples {
			if uw := strings.ToUpper(strings.TrimSpace(w)); uw != "" && strings.TrimSpace(s) != "" {
				sc.SampleByWell[uw] = strings.TrimSpace(s)
			}
		}
	}
	blankSet := sc.BlankSet
	excludedSet := sc.ExcludedSet
	for _, w := range calcStringSlice(m.Data["excluded_wells"]) {
		if uw := strings.ToUpper(strings.TrimSpace(w)); uw != "" {
			excludedSet[uw] = true
		}
	}
	if m.Plate != nil {
		for _, w := range m.Plate.ExcludedWells {
			if uw := strings.ToUpper(strings.TrimSpace(w)); uw != "" {
				excludedSet[uw] = true
			}
		}
	}
	if rolesMap, ok := m.Data["roles"].(map[string]any); ok {
		for w, rv := range rolesMap {
			if strings.EqualFold(strings.TrimSpace(fmt.Sprint(rv)), "blank") {
				blankSet[strings.ToUpper(strings.TrimSpace(w))] = true
			}
		}
	}
	assignedSet := map[string]bool{}
	if m.Plate != nil {
		if sc.Format == 0 {
			sc.Format = m.Plate.Format
		}
		for gid, wells := range m.Plate.Groups {
			clean := make([]string, 0, len(wells))
			for _, w := range wells {
				uw := strings.ToUpper(strings.TrimSpace(w))
				if uw == "" {
					continue
				}
				clean = append(clean, uw)
				assignedSet[uw] = true
			}
			sc.GroupWells[gid] = clean
		}
	} else if gm, ok := m.Data["groups"].(map[string]any); ok {
		for gid, rawW := range gm {
			ws := calcStringSlice(rawW)
			clean := make([]string, 0, len(ws))
			for _, w := range ws {
				uw := strings.ToUpper(strings.TrimSpace(w))
				if uw == "" {
					continue
				}
				clean = append(clean, uw)
				assignedSet[uw] = true
			}
			sc.GroupWells[gid] = clean
		}
	}
	for w := range assignedSet {
		sc.TotalWells++
		if blankSet[w] {
			sc.BlankWells++
		} else if excludedSet[w] {
			sc.ExcludedWells++
		} else {
			sc.SampleWells++
		}
	}
	sc.Assigned = sc.TotalWells > 0
	return sc
}

func extractReagentFromCalculator(m Module) (SciReagent, bool) {
	title := textValue(m.Data, "title")
	reagName := ""
	if match := reKnownReagName.FindString(title); match != "" {
		reagName = match
	}
	rows := mapSlice(m.Data["rows"])
	if reagName == "" {
		for _, rm := range rows {
			if match := reKnownReagName.FindString(textValue(rm, "label")); match != "" {
				reagName = match
				break
			}
		}
	}
	if reagName == "" {
		return SciReagent{}, false
	}
	r := SciReagent{
		Name:           reagName,
		SourceModuleID: m.ID,
		ByteOffset:     m.Start,
	}
	for _, rm := range rows {
		if strings.TrimSpace(textValue(rm, "formula")) != "" {
			continue
		}
		k := strings.ToLower(textValue(rm, "key"))
		lbl := textValue(rm, "label")
		val, ok := numericValue(rm["value"])
		if !ok || val <= 0 {
			continue
		}
		u := textValue(rm, "unit")
		switch {
		case k == "c_1" || strings.Contains(lbl, "母液"):
			r.StockVal = val
			r.StockUnit = u
		case k == "c_2" || strings.Contains(lbl, "目标工作") || strings.Contains(lbl, "目标浓度"):
			if strings.Contains(strings.ToLower(u), "w/v") || strings.Contains(u, "%") {
				r.ConcWV = val
			} else {
				r.TargetVal = val
				r.TargetUnit = u
			}
		case k == "c_w" || k == "c_g" || strings.Contains(strings.ToLower(u), "w/v"):
			r.ConcWV = val
		case k == "mw" || strings.Contains(lbl, "分子量"):
			r.MW = val
		}
	}
	return r, true
}

// PrimaryPlate returns the first plate container in the document (if any).
func (g *SciGraph) PrimaryPlate() *SciContainer {
	if g == nil || len(g.Containers) == 0 {
		return nil
	}
	cp := g.Containers[0]
	return &cp
}

// ContainerByID looks up a compiled container by module ID (falling back to PrimaryPlate if id is empty).
func (g *SciGraph) ContainerByID(id string) *SciContainer {
	if g == nil || len(g.Containers) == 0 {
		return nil
	}
	if strings.TrimSpace(id) == "" {
		return g.PrimaryPlate()
	}
	for i := range g.Containers {
		if g.Containers[i].ID == id {
			cp := g.Containers[i]
			return &cp
		}
	}
	return nil
}

// ContextAt builds the dynamic upstream ScientificLineContext at byte offset in < 5 microseconds.
func (g *SciGraph) ContextAt(offset int) ScientificLineContext {
	ctx := ScientificLineContext{Offset: offset, Zone: ZoneStepBody}
	if g == nil || g.Source == "" {
		return ctx
	}
	src := g.Source
	if offset < 0 {
		offset = 0
	}
	if offset > len(src) {
		offset = len(src)
	}

	// 1. Check frontmatter bounds
	_, bodyStart, _ := ScanFrontmatter(src)
	if offset < bodyStart {
		ctx.Zone = ZoneMetadata
		return ctx
	}

	// 2. Locate exact line bounds & raw line text
	lnStart := strings.LastIndexByte(src[:offset], '\n') + 1
	lnEnd := len(src)
	if nl := strings.IndexByte(src[offset:], '\n'); nl >= 0 {
		lnEnd = offset + nl
	}
	rawLine := strings.TrimSpace(src[lnStart:lnEnd])

	// 3. Find enclosing chapter and enclosing step from Outline / StepTree
	var inStep *StepNode
	if g.Steps != nil {
		var findStep func(nodes []*StepNode)
		findStep = func(nodes []*StepNode) {
			for _, sn := range nodes {
				if sn == nil {
					continue
				}
				if offset >= sn.LineStart && offset <= sn.ScopeEnd {
					inStep = sn
					findStep(sn.Children)
				}
			}
		}
		findStep(g.Steps.Roots)
	}
	for _, on := range g.Outline {
		if on == nil || on.LineStart > lnStart {
			continue
		}
		if !on.IsStep {
			ctx.ChapterTitle = on.Title
		}
	}
	if inStep != nil {
		ctx.StepTitle = inStep.Title
	}

	// 4. Deterministic structural zone classification (ZERO fragile keyword guessing!)
	switch {
	case rawLine == "" || strings.HasPrefix(rawLine, "```") || reThematicBreakLine.MatchString(rawLine):
		ctx.Zone = ZoneMetadata
	case strings.HasPrefix(rawLine, "#"):
		// Covers BOTH plain Markdown headings (# / ## / ###) AND step headings (#@ / ##@)!
		ctx.Zone = ZoneHeading
	case reMetaKeyBullet.MatchString(rawLine) || rePlainMetaKey.MatchString(rawLine):
		// Design metadata bullets like `- **研究体系**：...`, `- **单一自变量**：...`, `实验目的：...`
		// Exception: explicit container/plate layout summary bullet (`- **容器与复孔**：1 块 12 孔板...`)
		if reNLPlateFmt.MatchString(rawLine) && (strings.Contains(rawLine, "复孔") || strings.Contains(rawLine, "实验组")) {
			ctx.Zone = ZoneDesignPlan
		} else {
			ctx.Zone = ZoneMetadata
		}
	case reMaterialsChap.MatchString(ctx.ChapterTitle):
		ctx.Zone = ZoneMaterials
	case reReviewChap.MatchString(ctx.ChapterTitle):
		ctx.Zone = ZoneReview
	case inStep != nil:
		ctx.Zone = ZoneStepBody
	case reDesignChap.MatchString(ctx.ChapterTitle):
		ctx.Zone = ZoneDesignPlan
	default:
		ctx.Zone = ZoneStepBody
	}

	// 5. Capture immediately preceding prose line within the same step/section (PrevLine)
	if lnStart > bodyStart {
		minStart := bodyStart
		if inStep != nil && inStep.LineEnd > minStart {
			minStart = inStep.LineEnd
		}
		if minStart < lnStart {
			prevChunk := src[minStart:lnStart]
			lines := strings.Split(strings.TrimRight(prevChunk, "\r\n"), "\n")
			inFence := false
			for i := len(lines) - 1; i >= 0; i-- {
				l := strings.TrimSpace(lines[i])
				if strings.HasPrefix(l, "```") {
					inFence = !inFence
					continue
				}
				if inFence || l == "" || strings.HasPrefix(l, "#") || strings.HasPrefix(l, "![[") {
					continue
				}
				ctx.PrevLine = CleanNLSentence(l)
				break
			}
		}
	}

	// 6. Dynamically inherit ONLY upstream scientific facts (defined at or above offset!)
	for i := range g.Containers {
		if g.Containers[i].ByteOffset <= offset {
			cp := g.Containers[i]
			ctx.ActivePlate = &cp
		}
	}
	for _, grp := range g.Groups {
		if grp.ByteOffset <= offset {
			ctx.UpstreamGroups = append(ctx.UpstreamGroups, grp)
		}
	}
	for _, reag := range g.Reagents {
		if reag.ByteOffset < lnStart {
			ctx.UpstreamReagents = append(ctx.UpstreamReagents, reag)
		}
	}
	for _, anc := range g.Anchors {
		if anc.ByteOffset <= offset {
			ctx.UpstreamAnchors = append(ctx.UpstreamAnchors, anc)
		}
	}
	for _, m := range g.Modules {
		if m.Start > lnEnd && m.Start-lnEnd < 120 {
			ctx.NextModuleKind = m.Kind
			break
		}
	}
	return ctx
}

// CompactSummary serializes the upstream SciGraph into a < 150-token structured string
// suitable for optional cloud LLM / Jev copilot escalation (never dumped blindly into the 24M ONNX sensor).
func (g *SciGraph) CompactSummary(offset int) string {
	ctx := g.ContextAt(offset)
	var parts []string
	if ctx.ChapterTitle != "" {
		parts = append(parts, "章节="+ctx.ChapterTitle)
	}
	if ctx.StepTitle != "" {
		parts = append(parts, "步骤="+ctx.StepTitle)
	}
	if ctx.ActivePlate != nil {
		parts = append(parts, fmt.Sprintf("孔板=%s(%d孔,实验%d孔,空白%d孔)",
			ctx.ActivePlate.ID, ctx.ActivePlate.Format, ctx.ActivePlate.SampleWells, ctx.ActivePlate.BlankWells))
	}
	if len(ctx.UpstreamGroups) > 0 {
		names := make([]string, 0, len(ctx.UpstreamGroups))
		for _, grp := range ctx.UpstreamGroups {
			names = append(names, grp.Name)
		}
		parts = append(parts, "分组="+strings.Join(names, "/"))
	}
	if len(ctx.UpstreamReagents) > 0 {
		rs := make([]string, 0, len(ctx.UpstreamReagents))
		for _, r := range ctx.UpstreamReagents {
			if r.StockVal > 0 {
				rs = append(rs, fmt.Sprintf("%s(母液%s%s)", r.Name, formatNumber(r.StockVal), r.StockUnit))
			} else if r.ConcWV > 0 {
				rs = append(rs, fmt.Sprintf("%s(%s%%w/v)", r.Name, formatNumber(r.ConcWV)))
			}
		}
		if len(rs) > 0 {
			parts = append(parts, "试剂="+strings.Join(rs, ","))
		}
	}
	if len(ctx.UpstreamAnchors) > 0 {
		as := make([]string, 0, len(ctx.UpstreamAnchors))
		for _, a := range ctx.UpstreamAnchors {
			as = append(as, "#"+a.ID)
		}
		parts = append(parts, "锚点="+strings.Join(as, ","))
	}
	if ctx.PrevLine != "" {
		parts = append(parts, "上句="+ctx.PrevLine)
	}
	return strings.Join(parts, " | ")
}
