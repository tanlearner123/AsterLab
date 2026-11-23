package astercore

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"asterlab-mygo-prototype/internal/analysis"
	"asterlab-mygo-prototype/internal/asterdoc"
)

// AssaySummary holds deterministic derived statistics extracted from a document's
// SciGraph unified experimental units + data_table observations without mutating raw data.
type AssaySummary struct {
	Kind      string               `json:"kind"` // "cck8" | "qpcr" | "descriptive"
	Title     string               `json:"title"`
	PlateID   string               `json:"plate_id,omitempty"`
	Control   string               `json:"control,omitempty"`
	CCK8      *analysis.CCK8Result `json:"cck8,omitempty"`
	QPCR      *analysis.QPCRResult `json:"qpcr,omitempty"`
	Groups    []DescriptiveGroup   `json:"groups,omitempty"`
	ExcludedN int                  `json:"excluded_wells,omitempty"`
	BlankN    int                  `json:"blank_wells,omitempty"`
	BlankMean *float64             `json:"blank_mean,omitempty"`
}

// DescriptiveGroup is a deterministic group summary strictly separating technical replicate
// well observations (N / TechReplicates) from independent biological sample count (BioSamples).
type DescriptiveGroup struct {
	Group          string   `json:"group"`
	N              int      `json:"n"`
	TechReplicates int      `json:"tech_replicates,omitempty"`
	BioSamples     int      `json:"bio_samples,omitempty"`
	Mean           float64  `json:"mean"`
	SD             float64  `json:"sd"`
	BlankCorrMean  *float64 `json:"blank_corr_mean,omitempty"`
}

// AnalyzeDocument extracts numeric observations from `data_table@1` modules, maps
// them through `SciGraph` unified `SciUnit` metadata (multi-plate aware, blank subtraction,
// excluded_wells omission, technical vs biological replicate clarity), and invokes
// `analysis.AnalyzeCCK8` or `analysis.AnalyzeQPCR` where applicable.
func AnalyzeDocument(doc *asterdoc.Document, docID string) []AssaySummary {
	if doc == nil {
		return nil
	}
	if strings.TrimSpace(docID) == "" {
		docID = "vault-doc"
	}
	prov := analysis.Provenance{BatchID: docID, EventID: "live-projection"}
	graph := asterdoc.BuildSciGraph(doc)

	controlGroup := ""
	for _, g := range doc.GroupList() {
		if g.Role == "control" && !strings.Contains(strings.ToLower(g.ID), "blank") && !strings.Contains(strings.ToLower(g.ID), "pbs") && controlGroup == "" {
			controlGroup = firstNonEmpty(g.Name, g.ID)
		}
	}

	var dataTables []map[string]any
	for _, m := range doc.Modules {
		if m.Kind == "data_table" {
			dataTables = append(dataTables, m.Data)
		}
		if m.Kind == "step" {
			if blks, ok := m.Data["blocks"].([]any); ok {
				for _, raw := range blks {
					bm, _ := raw.(map[string]any)
					if bm == nil {
						continue
					}
					if bk, _ := bm["kind"].(string); bk == "data_table" {
						dataTables = append(dataTables, bm)
					}
				}
			}
		}
	}

	defaultPlateID := ""
	if graph != nil {
		if pl := graph.PrimaryPlate(); pl != nil {
			defaultPlateID = pl.ID
		}
	}

	var summaries []AssaySummary
	for _, dt := range dataTables {
		title := firstNonEmpty(stringVal(dt["title"]), "数据表统计")
		plateID := firstNonEmpty(stringVal(dt["container_ref"]), firstNonEmpty(stringVal(dt["plate_id"]), defaultPlateID))
		rows, _ := dt["rows"].([]any)
		if len(rows) == 0 {
			continue
		}
		// Extract cells via unified SciUnit lookup (multi-plate, excluded_wells, blank separation)
		var cells []analysis.Cell
		var blankVals []float64
		excludedCount := 0
		genes := map[string]bool{}
		samples := map[string]bool{}
		bioByGroup := map[string]map[string]bool{}

		for _, rRaw := range rows {
			rm, _ := rRaw.(map[string]any)
			if rm == nil {
				continue
			}
			well := strings.ToUpper(strings.TrimSpace(firstNonEmpty(stringVal(rm["well"]), stringVal(rm["well_id"]))))
			var unit asterdoc.SciUnit
			hasUnit := false
			if well != "" && graph != nil {
				unit, hasUnit = graph.UnitForWell(plateID, well)
				if hasUnit && unit.Excluded {
					excludedCount++
					continue
				}
			}
			val, hasVal := firstNumericReading(rm)
			if hasUnit && unit.Blank {
				if hasVal {
					blankVals = append(blankVals, val)
				}
				continue
			}
			grp := strings.TrimSpace(stringVal(rm["group"]))
			if grp == "" && hasUnit {
				grp = firstNonEmpty(unit.GroupName, unit.GroupID)
			}
			explicitSample := strings.TrimSpace(firstNonEmpty(stringVal(rm["sample_id"]), stringVal(rm["sample"])))
			if explicitSample == "" && hasUnit && strings.TrimSpace(unit.SampleID) != "" {
				explicitSample = strings.TrimSpace(unit.SampleID)
			}
			sample := firstNonEmpty(explicitSample, grp)
			if grp == "" {
				grp = sample
			}
			if grp == "" {
				continue
			}
			gene := strings.TrimSpace(stringVal(rm["gene"]))
			if gene == "" {
				gene = strings.TrimSpace(stringVal(rm["target"]))
			}
			if !hasVal {
				cells = append(cells, analysis.Cell{
					Group: grp, SampleID: sample, Gene: gene, Status: analysis.Missing,
				})
				continue
			}
			vCopy := val
			cells = append(cells, analysis.Cell{
				Group: grp, SampleID: sample, Gene: gene, Value: &vCopy, Status: analysis.Raw,
			})
			if bioByGroup[grp] == nil {
				bioByGroup[grp] = map[string]bool{}
			}
			if explicitSample != "" && explicitSample != well {
				bioByGroup[grp][explicitSample] = true
			}
			if gene != "" {
				genes[gene] = true
			}
			if sample != "" {
				samples[sample] = true
			}
		}
		if len(cells) == 0 {
			continue
		}

		var blankMeanPtr *float64
		if len(blankVals) > 0 {
			bm := meanSlice(blankVals)
			blankMeanPtr = &bm
		}

		// Try RT-qPCR ΔΔCq if >=2 genes (target + reference) and calibrator exists
		if len(genes) >= 2 && len(samples) >= 2 {
			geneList := sortedKeys(genes)
			refGene := pickReferenceGene(geneList)
			calSample := firstNonEmpty(controlGroup, sortedKeys(samples)[0])
			var targetGene string
			for _, g := range geneList {
				if g != refGene {
					targetGene = g
					break
				}
			}
			if targetGene != "" && refGene != "" {
				if qpcrRes, err := analysis.AnalyzeQPCR(cells, targetGene, refGene, calSample, prov); err == nil {
					summaries = append(summaries, AssaySummary{
						Kind:      "qpcr",
						Title:     fmt.Sprintf("%s · ΔΔCq (%s / %s)", title, targetGene, refGene),
						PlateID:   plateID,
						Control:   calSample,
						QPCR:      &qpcrRes,
						ExcludedN: excludedCount,
						BlankN:    len(blankVals),
						BlankMean: blankMeanPtr,
					})
					continue
				}
			}
		}

		// Only normalize as CCK-8 relative viability when the table/document explicitly indicates a viability assay;
		// general quantitative tables (ELISA, protein conc, fluorescence intensity, tube counts) stay descriptive!
		if controlGroup != "" && isViabilityAssayTable(doc, dt, title) {
			if cckRes, err := analysis.AnalyzeCCK8(cells, controlGroup, prov); err == nil {
				summaries = append(summaries, AssaySummary{
					Kind:      "cck8",
					Title:     fmt.Sprintf("%s · 相对活力分析", title),
					PlateID:   plateID,
					Control:   controlGroup,
					CCK8:      &cckRes,
					ExcludedN: excludedCount,
					BlankN:    len(blankVals),
					BlankMean: blankMeanPtr,
				})
				continue
			}
		}

		// Descriptive fallback (group TechReplicates, BioSamples, Mean, SD, BlankCorrMean)
		byGroup := map[string][]float64{}
		for _, c := range cells {
			if c.Status == analysis.Raw && c.Value != nil {
				byGroup[c.Group] = append(byGroup[c.Group], *c.Value)
			}
		}
		if len(byGroup) > 0 {
			var desc []DescriptiveGroup
			for _, gName := range sortedKeysMap(byGroup) {
				vs := byGroup[gName]
				mVal := meanSlice(vs)
				var corrPtr *float64
				if blankMeanPtr != nil {
					cv := mVal - *blankMeanPtr
					corrPtr = &cv
				}
				bioN := len(bioByGroup[gName])
				desc = append(desc, DescriptiveGroup{
					Group:          gName,
					N:              len(vs),
					TechReplicates: len(vs),
					BioSamples:     bioN,
					Mean:           mVal,
					SD:             sdSlice(vs),
					BlankCorrMean:  corrPtr,
				})
			}
			summaries = append(summaries, AssaySummary{
				Kind:      "descriptive",
				Title:     title,
				PlateID:   plateID,
				Groups:    desc,
				ExcludedN: excludedCount,
				BlankN:    len(blankVals),
				BlankMean: blankMeanPtr,
			})
		}
	}
	return summaries
}

func isViabilityAssayTable(doc *asterdoc.Document, dt map[string]any, title string) bool {
	var sb strings.Builder
	sb.WriteString(strings.ToLower(title))
	if doc != nil {
		fm, _, _ := asterdoc.ScanFrontmatter(doc.Source)
		sb.WriteString(" ")
		sb.WriteString(strings.ToLower(fm.Title))
	}
	if cols, ok := dt["columns"].([]any); ok {
		for _, cRaw := range cols {
			if cm, ok := cRaw.(map[string]any); ok {
				sb.WriteString(" ")
				sb.WriteString(strings.ToLower(stringVal(cm["key"])))
				sb.WriteString(" ")
				sb.WriteString(strings.ToLower(stringVal(cm["label"])))
			}
		}
	}
	hay := sb.String()
	for _, kw := range []string{"cck8", "cck-8", "od450", "od 450", "viability", "mtt", "alamar", "活力", "存活率", "增殖"} {
		if strings.Contains(hay, kw) {
			return true
		}
	}
	return false
}

func firstNumericReading(rm map[string]any) (float64, bool) {
	for _, key := range []string{"reading", "value", "od", "od450", "cq", "ct", "viability"} {
		if v, ok := rm[key]; ok {
			if f, fok := parseFloatAny(v); fok {
				return f, true
			}
		}
	}
	for k, v := range rm {
		switch k {
		case "id", "well", "group", "sample", "gene", "target", "note", "unit":
			continue
		}
		if f, fok := parseFloatAny(v); fok {
			return f, true
		}
	}
	return 0, false
}

func parseFloatAny(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float64:
		if !math.IsNaN(n) && !math.IsInf(n, 0) {
			return n, true
		}
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(n), 64); err == nil && !math.IsNaN(f) && !math.IsInf(f, 0) {
			return f, true
		}
	}
	return 0, false
}

func pickReferenceGene(genes []string) string {
	for _, g := range genes {
		low := strings.ToLower(g)
		if low == "gapdh" || low == "actb" || low == "18s" || strings.Contains(low, "ref") {
			return g
		}
	}
	if len(genes) > 0 {
		return genes[len(genes)-1]
	}
	return ""
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedKeysMap(m map[string][]float64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func meanSlice(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}

func sdSlice(v []float64) float64 {
	if len(v) < 2 {
		return 0
	}
	m := meanSlice(v)
	ss := 0.0
	for _, x := range v {
		d := x - m
		ss += d * d
	}
	return math.Sqrt(ss / float64(len(v)-1))
}

func stringVal(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
