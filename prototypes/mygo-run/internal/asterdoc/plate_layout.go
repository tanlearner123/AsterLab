package asterdoc

import (
	"fmt"
	"sort"
	"strings"
)

// PlateLayoutDirection controls sequential well filling order matching wet-lab pipetting ergonomics.
type PlateLayoutDirection string

const (
	LayoutRowMajor PlateLayoutDirection = "row" // Sequential across rows (A1, A2, A3... — 12-channel / standard)
	LayoutColMajor PlateLayoutDirection = "col" // Sequential down columns (A1, B1, C1... — 8-channel pipette friendly)
)

// PlateLayoutGroupSpec describes one logical experimental group independent of physical coordinates.
// Semantic models / NL extractors emit ONLY this logical spec; the deterministic layout engine
// assigns exact physical coordinates with zero overlap or out-of-bounds hallucinations.
type PlateLayoutGroupSpec struct {
	ID         string
	Name       string
	Role       string // "treatment" | "control" | "blank"
	Replicates int
}

// PlateLayoutSpec is the input to the deterministic microplate layout engine.
type PlateLayoutSpec struct {
	Format       int
	Groups       []PlateLayoutGroupSpec
	Direction    PlateLayoutDirection // defaults to LayoutRowMajor
	EdgeBlank    bool                 // true only when explicitly requested by user/prose or right-click toggle
	BlankGroupID string               // optional group ID assigned to perimeter/edge blank wells
}

// PlateLayoutPlateResult holds one deterministically assigned physical plate.
type PlateLayoutPlateResult struct {
	PlateIndex  int
	Format      int
	Rows        int
	Cols        int
	Groups      map[string][]string
	Roles       map[string]string
	SampleWells int
	BlankWells  int
}

// PlateLayoutPlan holds the full layout output, transparently spilling into >=2 plates
// whenever total experimental wells exceed single-plate usable capacity (zero silent truncation!).
type PlateLayoutPlan struct {
	Plates         []PlateLayoutPlateResult
	UsablePerPlate int
	TotalSamples   int
	EdgeBlankUsed  bool
	Note           string
}

// edgeBlankWellSet returns the deterministic edge-evaporation guard wells for a plate geometry:
//   - Format >= 24 (rows >= 4 && cols >= 6): full outer perimeter ring (row A/last, col 1/last).
//   - Format == 12 (3x4): rightmost column (A4, B4, C4) so 9 inner wells remain usable (3x3).
//   - Format <= 6: no perimeter possible (returns empty map).
func edgeBlankWellSet(geom Geometry) map[string]bool {
	out := map[string]bool{}
	if geom.Rows >= 4 && geom.Columns >= 6 {
		for r := 0; r < geom.Rows; r++ {
			for col := 0; col < geom.Columns; col++ {
				if r == 0 || r == geom.Rows-1 || col == 0 || col == geom.Columns-1 {
					out[fmt.Sprintf("%s%d", rowLabel(r), col+1)] = true
				}
			}
		}
		return out
	}
	if geom.Rows == 3 && geom.Columns == 4 {
		// 12-well plate: right column (col 4) as standard PBS edge buffer -> 9 sample wells A1-C3
		for r := 0; r < geom.Rows; r++ {
			out[fmt.Sprintf("%s4", rowLabel(r))] = true
		}
	}
	return out
}

// orderedUsableWells returns all non-edge-blank well coordinates in strict sequential order
// (row-major or col-major — never randomized).
func orderedUsableWells(geom Geometry, dir PlateLayoutDirection, edgeBlankSet map[string]bool) []string {
	var wells []string
	if dir == LayoutColMajor {
		for col := 0; col < geom.Columns; col++ {
			for r := 0; r < geom.Rows; r++ {
				w := fmt.Sprintf("%s%d", rowLabel(r), col+1)
				if !edgeBlankSet[w] {
					wells = append(wells, w)
				}
			}
		}
		return wells
	}
	for r := 0; r < geom.Rows; r++ {
		for col := 0; col < geom.Columns; col++ {
			w := fmt.Sprintf("%s%d", rowLabel(r), col+1)
			if !edgeBlankSet[w] {
				wells = append(wells, w)
			}
		}
	}
	return wells
}

// ComputePlateLayout deterministically maps logical experimental groups onto 1 or more physical
// plates in sequential row-major or column-major order, honoring optional edge-blank protection
// and automatically spilling into additional plates if capacity is exceeded.
func ComputePlateLayout(spec PlateLayoutSpec) PlateLayoutPlan {
	format := spec.Format
	geom, ok := geometryForPlate(format, "", 0, 0)
	if !ok {
		format = 12
		geom, _ = geometryForPlate(12, "", 0, 0)
	}
	dir := spec.Direction
	if dir != LayoutColMajor {
		dir = LayoutRowMajor
	}

	// Separate experimental groups from any explicit blank group
	var sampleGroups []PlateLayoutGroupSpec
	blankGroupID := strings.TrimSpace(spec.BlankGroupID)
	explicitBlankReps := 0
	totalSampleNeed := 0
	for _, g := range spec.Groups {
		rep := g.Replicates
		if rep < 1 {
			rep = 1
		}
		if strings.EqualFold(g.Role, "blank") || strings.Contains(strings.ToLower(g.ID), "blank") || strings.Contains(strings.ToLower(g.ID), "pbs") {
			if blankGroupID == "" {
				blankGroupID = g.ID
			}
			explicitBlankReps += rep
			continue
		}
		cp := g
		cp.Replicates = rep
		sampleGroups = append(sampleGroups, cp)
		totalSampleNeed += rep
	}

	edgeSet := map[string]bool{}
	edgeUsed := false
	note := ""
	if spec.EdgeBlank {
		cand := edgeBlankWellSet(geom)
		if len(cand) > 0 {
			edgeSet = cand
			edgeUsed = true
		} else {
			note = fmt.Sprintf("%d 孔板规格过小不支持外周留空，已自动紧凑排布", format)
		}
	}

	usableSeq := orderedUsableWells(geom, dir, edgeSet)
	if len(usableSeq) == 0 {
		edgeSet = map[string]bool{}
		edgeUsed = false
		usableSeq = orderedUsableWells(geom, dir, edgeSet)
	}

	// Build sorted list of edge blank wells for deterministic assignment
	var edgeBlankList []string
	for r := 0; r < geom.Rows; r++ {
		for col := 0; col < geom.Columns; col++ {
			w := fmt.Sprintf("%s%d", rowLabel(r), col+1)
			if edgeSet[w] {
				edgeBlankList = append(edgeBlankList, w)
			}
		}
	}

	// Pack groups sequentially; whenever a group would overflow remaining slots on the current plate,
	// open a fresh plate cleanly (multi-plate auto-spillover!).
	newPlate := func(idx int) PlateLayoutPlateResult {
		pr := PlateLayoutPlateResult{
			PlateIndex: idx,
			Format:     format,
			Rows:       geom.Rows,
			Cols:       geom.Columns,
			Groups:     map[string][]string{},
			Roles:      map[string]string{},
		}
		if edgeUsed && len(edgeBlankList) > 0 {
			for _, bw := range edgeBlankList {
				pr.Roles[bw] = "blank"
			}
			pr.BlankWells = len(edgeBlankList)
			if blankGroupID != "" {
				pr.Groups[blankGroupID] = append([]string(nil), edgeBlankList...)
			}
		}
		return pr
	}

	var plates []PlateLayoutPlateResult
	cur := newPlate(1)
	cursor := 0

	// Try to keep each group's replicates contiguous on a single row/column when they fit nicely;
	// otherwise pack seamlessly into consecutive usable slots!
	for _, g := range sampleGroups {
		rem := g.Replicates
		for rem > 0 {
			if cursor >= len(usableSeq) {
				plates = append(plates, cur)
				cur = newPlate(len(plates) + 1)
				cursor = 0
			}
			// If group fits on one fresh line/col and remaining slots in current line/col aren't enough
			// (while plenty of total space remains on this plate), align cleanly to next row/col start!
			if !edgeUsed && rem <= geom.Columns && dir == LayoutRowMajor && cursor%geom.Columns != 0 {
				lineRem := geom.Columns - (cursor % geom.Columns)
				nextLineStart := cursor + lineRem
				if lineRem < rem && nextLineStart+rem <= len(usableSeq) && nextLineStart+totalRemainingAfter(sampleGroups, g.ID) <= len(usableSeq) {
					cursor = nextLineStart
				}
			} else if !edgeUsed && rem <= geom.Rows && dir == LayoutColMajor && cursor%geom.Rows != 0 {
				colRem := geom.Rows - (cursor % geom.Rows)
				nextColStart := cursor + colRem
				if colRem < rem && nextColStart+rem <= len(usableSeq) && nextColStart+totalRemainingAfter(sampleGroups, g.ID) <= len(usableSeq) {
					cursor = nextColStart
				}
			}
			take := rem
			if avail := len(usableSeq) - cursor; take > avail {
				take = avail
			}
			for i := 0; i < take; i++ {
				w := usableSeq[cursor]
				cursor++
				cur.Groups[g.ID] = append(cur.Groups[g.ID], w)
				cur.SampleWells++
			}
			rem -= take
		}
	}

	// If user requested explicit blank replicates WITHOUT full-perimeter edge guard, pack them cleanly at the tail
	if !edgeUsed && explicitBlankReps > 0 && blankGroupID != "" {
		rem := explicitBlankReps
		for rem > 0 {
			if cursor >= len(usableSeq) {
				plates = append(plates, cur)
				cur = newPlate(len(plates) + 1)
				cursor = 0
			}
			w := usableSeq[cursor]
			cursor++
			cur.Groups[blankGroupID] = append(cur.Groups[blankGroupID], w)
			cur.Roles[w] = "blank"
			cur.BlankWells++
			rem--
		}
	}

	plates = append(plates, cur)
	if len(plates) > 1 {
		note = fmt.Sprintf("总计 %d 实验孔超出单块 %d 孔板可用容量 (%d 孔)，已自动拆分为 %d 块板",
			totalSampleNeed, format, len(usableSeq), len(plates))
	}

	return PlateLayoutPlan{
		Plates:         plates,
		UsablePerPlate: len(usableSeq),
		TotalSamples:   totalSampleNeed,
		EdgeBlankUsed:  edgeUsed,
		Note:           note,
	}
}

func totalRemainingAfter(groups []PlateLayoutGroupSpec, curID string) int {
	sum := 0
	seen := false
	for _, g := range groups {
		if g.ID == curID {
			seen = true
		}
		if seen {
			sum += g.Replicates
		}
	}
	return sum
}

// PlateHasRecordedData checks whether plate p already carries experimental execution records
// (excluded wells, sample labels, well-linked data_table observations, or container_grid attachments).
// When true, automatic relayout MUST be blocked to prevent silently re-mapping historical data to wrong groups!
func PlateHasRecordedData(doc *Document, p *Plate) (bool, string) {
	if p == nil {
		return false, ""
	}
	if len(p.ExcludedWells) > 0 {
		return true, fmt.Sprintf("已标记 %d 个异常孔", len(p.ExcludedWells))
	}
	if len(p.Samples) > 0 {
		return true, fmt.Sprintf("已录入 %d 个孔位样本编号", len(p.Samples))
	}
	if doc == nil {
		return false, ""
	}
	flat, _ := flattenModules(doc.Modules)
	for _, m := range flat {
		switch m.Kind {
		case "data_table":
			ref := strings.TrimSpace(firstNonEmpty(textValue(m.Data, "container_ref"), textValue(m.Data, "plate_id")))
			if ref != "" && ref != p.ID {
				continue
			}
			for _, rm := range mapSlice(m.Data["rows"]) {
				w := strings.TrimSpace(firstNonEmpty(textValue(rm, "well"), textValue(rm, "well_id")))
				if w != "" {
					return true, fmt.Sprintf("数据表「%s」已有按孔位记录的读数", firstNonEmpty(textValue(m.Data, "title"), m.ID))
				}
			}
		case "attachment":
			if textValue(m.Data, "container_ref") == p.ID {
				if items, ok := m.Data["items"].([]any); ok && len(items) > 0 {
					return true, fmt.Sprintf("采图网格「%s」已关联 %d 张照片", firstNonEmpty(textValue(m.Data, "title"), m.ID), len(items))
				}
			}
		}
	}
	return false, ""
}

// RelayoutExistingPlate deterministically re-arranges an existing plate@1's groups in row-major
// or col-major order with optional edge-blank protection. It NEVER silently drops wells or
// mutates a plate that already holds recorded experimental observations/photos.
func RelayoutExistingPlate(doc *Document, p *Plate, dir PlateLayoutDirection, edgeBlank bool) (groups map[string][]string, roles map[string]string, overflow int, err error) {
	if p == nil {
		return nil, nil, 0, fmt.Errorf("未找到孔板")
	}
	if hasRec, reason := PlateHasRecordedData(doc, p); hasRec {
		return nil, nil, 0, fmt.Errorf("无法自动重排：%s，为避免历史数据归组错乱已锁定孔位；请先清空记录或新建孔板", reason)
	}
	geom, ok := geometryForPlate(p.Format, p.Topology, p.Rows, p.Columns)
	if !ok {
		return nil, nil, 0, fmt.Errorf("不支持的孔板规格 %d", p.Format)
	}

	// Preserve deterministic group ordering (prefer group_table order, fallback to alphabetical)
	var groupOrder []string
	seen := map[string]bool{}
	if doc != nil {
		for _, g := range doc.GroupList() {
			if len(p.Groups[g.ID]) > 0 && !seen[g.ID] {
				groupOrder = append(groupOrder, g.ID)
				seen[g.ID] = true
			}
		}
	}
	var rest []string
	for gid := range p.Groups {
		if !seen[gid] {
			rest = append(rest, gid)
		}
	}
	sort.Strings(rest)
	groupOrder = append(groupOrder, rest...)

	var specs []PlateLayoutGroupSpec
	blankGroupID := ""
	totalSample := 0
	for _, gid := range groupOrder {
		ws := p.Groups[gid]
		if len(ws) == 0 {
			continue
		}
		allBlank := true
		for _, w := range ws {
			if !strings.EqualFold(p.Roles[strings.ToUpper(w)], "blank") {
				allBlank = false
				break
			}
		}
		if allBlank {
			blankGroupID = gid
			specs = append(specs, PlateLayoutGroupSpec{ID: gid, Name: gid, Role: "blank", Replicates: len(ws)})
		} else {
			totalSample += len(ws)
			specs = append(specs, PlateLayoutGroupSpec{ID: gid, Name: gid, Role: "treatment", Replicates: len(ws)})
		}
	}

	if edgeBlank {
		edgeSet := edgeBlankWellSet(geom)
		usable := geom.Rows*geom.Columns - len(edgeSet)
		if len(edgeSet) == 0 {
			return nil, nil, 0, fmt.Errorf("%d 孔板规格不支持外周留空", p.Format)
		}
		if totalSample > usable {
			return nil, nil, totalSample - usable, fmt.Errorf("外周留空后仅剩 %d 可用孔，当前实验组需 %d 孔（超出 %d 孔）", usable, totalSample, totalSample-usable)
		}
	}

	plan := ComputePlateLayout(PlateLayoutSpec{
		Format:       p.Format,
		Groups:       specs,
		Direction:    dir,
		EdgeBlank:    edgeBlank,
		BlankGroupID: blankGroupID,
	})
	if len(plan.Plates) == 0 {
		return nil, nil, 0, fmt.Errorf("排版结果为空")
	}
	return plan.Plates[0].Groups, plan.Plates[0].Roles, 0, nil
}
