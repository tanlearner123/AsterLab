package asterdoc

import (
	"math"
	"strings"
	"testing"
)

func TestDeterministicPlateLayoutEngineAndMultiPlateSpillover(t *testing.T) {
	// 1. Default sequential row-major (NO edge blank unless requested, NO randomization)
	planRow := ComputePlateLayout(PlateLayoutSpec{
		Format:    12,
		Direction: LayoutRowMajor,
		EdgeBlank: false,
		Groups: []PlateLayoutGroupSpec{
			{ID: "g1", Name: "3% GelMA", Role: "treatment", Replicates: 3},
			{ID: "g2", Name: "5% GelMA", Role: "treatment", Replicates: 3},
			{ID: "g3", Name: "7% GelMA", Role: "treatment", Replicates: 3},
		},
	})
	if len(planRow.Plates) != 1 || planRow.Plates[0].SampleWells != 9 || planRow.Plates[0].BlankWells != 0 {
		t.Fatalf("unexpected row-major plan: %+v", planRow)
	}
	if strings.Join(planRow.Plates[0].Groups["g1"], ",") != "A1,A2,A3" {
		t.Fatalf("expected g1 on A1,A2,A3, got %v", planRow.Plates[0].Groups["g1"])
	}

	// 2. Column-major (8-channel pipette friendly)
	planCol := ComputePlateLayout(PlateLayoutSpec{
		Format:    12,
		Direction: LayoutColMajor,
		EdgeBlank: false,
		Groups: []PlateLayoutGroupSpec{
			{ID: "g1", Name: "组1", Role: "treatment", Replicates: 3},
			{ID: "g2", Name: "组2", Role: "treatment", Replicates: 3},
		},
	})
	if strings.Join(planCol.Plates[0].Groups["g1"], ",") != "A1,B1,C1" || strings.Join(planCol.Plates[0].Groups["g2"], ",") != "A2,B2,C2" {
		t.Fatalf("expected col-major A1,B1,C1 & A2,B2,C2, got %+v", planCol.Plates[0].Groups)
	}

	// 3. 12-well smart edge-blank (right column A4,B4,C4 -> 9 usable wells A1-C3)
	plan12Edge := ComputePlateLayout(PlateLayoutSpec{
		Format:       12,
		Direction:    LayoutRowMajor,
		EdgeBlank:    true,
		BlankGroupID: "g_pbs",
		Groups: []PlateLayoutGroupSpec{
			{ID: "g1", Name: "3%", Role: "treatment", Replicates: 3},
			{ID: "g2", Name: "5%", Role: "treatment", Replicates: 3},
			{ID: "g3", Name: "7%", Role: "treatment", Replicates: 3},
		},
	})
	if len(plan12Edge.Plates) != 1 || plan12Edge.Plates[0].SampleWells != 9 || plan12Edge.Plates[0].BlankWells != 3 {
		t.Fatalf("expected 12-well edge blank = 9 sample / 3 blank, got %+v", plan12Edge.Plates[0])
	}
	if plan12Edge.Plates[0].Roles["A4"] != "blank" || plan12Edge.Plates[0].Roles["C4"] != "blank" {
		t.Fatalf("expected col 4 marked blank on 12-well plate, got %+v", plan12Edge.Plates[0].Roles)
	}

	// 4. Multi-Plate Auto-Spillover: 96-well plate with outer-ring edge-blank has 60 usable inner wells;
	// requesting 72 experimental wells MUST cleanly spill into 2 plates (60 + 12) with ZERO silent drop!
	var bigGroups []PlateLayoutGroupSpec
	for i := 1; i <= 12; i++ {
		bigGroups = append(bigGroups, PlateLayoutGroupSpec{
			ID: "g" + string(rune('a'+i-1)), Name: "Group", Role: "treatment", Replicates: 6,
		})
	}
	planMulti := ComputePlateLayout(PlateLayoutSpec{
		Format:       96,
		Direction:    LayoutRowMajor,
		EdgeBlank:    true,
		BlankGroupID: "g_edge",
		Groups:       bigGroups,
	})
	if len(planMulti.Plates) != 2 {
		t.Fatalf("expected 72 wells on edge-guarded 96-well plate to spill into 2 plates, got %d", len(planMulti.Plates))
	}
	if planMulti.Plates[0].SampleWells != 60 || planMulti.Plates[1].SampleWells != 12 {
		t.Fatalf("expected 60 + 12 sample wells across 2 plates, got %d + %d",
			planMulti.Plates[0].SampleWells, planMulti.Plates[1].SampleWells)
	}
}

func TestSciUnitExcludedWellsAndBlankSubtractedReadoutSummary(t *testing.T) {
	src := `---
asterml: "2.1"
title: SciUnit 全链消费实测
---

## 1.1 实验设计

` + "```asterlab" + `
id: gt-u
kind: group_table
schema: 1
title: 分组
rows:
  - id: g_ctrl
    name: Control
    role: control
    technical_repeats: 3
  - id: g_drug
    name: Drug 10uM
    role: treatment
    technical_repeats: 3
  - id: g_blk
    name: Blank
    role: control
    technical_repeats: 2
` + "```" + `

` + "```asterlab" + `
id: plate-u1
kind: plate
schema: 1
title: 板 1
format: 12
groups:
  g_ctrl: [A1, A2, A3]
  g_drug: [B1, B2, B3]
  g_blk: [C1, C2]
roles:
  C1: blank
  C2: blank
excluded_wells: [B2]
` + "```" + `

` + "```asterlab" + `
id: plate-u2
kind: plate
schema: 1
title: 板 2
format: 12
groups:
  g_ctrl: [A1, A2]
` + "```" + `

` + "```asterlab" + `
id: calc-scoped
kind: calculator
schema: 1
title: 多板联动计算
mode: formula
rows:
  - key: N_u1
    label: 板1有效孔(自动剔除2空白+1异常B2=5孔)
    kind: input
    value: "@plate.plate-u1.sample_wells"
    unit: 孔
  - key: N_all
    label: 全部板有效孔合计(5+2=7孔)
    kind: input
    value: "@plate.all_sample_wells"
    unit: 孔
  - key: V_tot
    label: 总预混体积
    kind: formula
    formula: 'N_{all} \times 100'
    unit: uL
` + "```" + `
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	g := BuildSciGraph(doc)
	p1 := g.ContainerByID("plate-u1")
	if p1 == nil || p1.SampleWells != 5 || p1.BlankWells != 2 || p1.ExcludedWells != 1 {
		t.Fatalf("expected plate-u1 = 5 sample / 2 blank / 1 excluded, got %+v", p1)
	}
	if g.AllSampleWells() != 7 {
		t.Fatalf("expected AllSampleWells=7 (5+2), got %d", g.AllSampleWells())
	}

	// Verify scoped multi-plate calculator refs
	res := EvalCalculatorWithActuals(doc, doc.Modules[3].Data, nil)
	if res.Err != nil || res.Rows[len(res.Rows)-1].Amount != 700 {
		t.Fatalf("expected 7*100=700 uL from @plate.all_sample_wells, got %+v (err=%v)", res.Rows, res.Err)
	}

	// Verify SciUnit consumption for plate readout aggregation (skips excluded B2, subtracts Blank mean=0.10!)
	readings := map[string]float64{
		"A1": 1.00, "A2": 1.10, "A3": 1.20, // Control mean = 1.10, corr = 1.00 (n=3)
		"B1": 0.50, "B2": 9.99, "B3": 0.70, // B2 is EXCLUDED -> Drug mean = 0.60, corr = 0.50 (n=2)!
		"C1": 0.08, "C2": 0.12, // Blank mean = 0.10
	}
	bMean, hasB, stats := g.SummarizeWellReadings("plate-u1", readings)
	if !hasB || math.Abs(bMean-0.10) > 1e-4 {
		t.Fatalf("expected blank mean 0.10, got %v (has=%v)", bMean, hasB)
	}
	foundDrug := false
	for _, st := range stats {
		if st.GroupID == "g_drug" {
			foundDrug = true
			if st.Count != 2 || math.Abs(st.Mean-0.60) > 1e-3 || math.Abs(st.CorrMean-0.50) > 1e-3 {
				t.Fatalf("expected g_drug (excluding B2=9.99) n=2 mean=0.60 corr=0.50, got %+v", st)
			}
		}
	}
	if !foundDrug {
		t.Fatalf("missing g_drug in stats: %+v", stats)
	}
}

func TestPhysicalC1GreaterThanC2GuardCuresWorkingConcTrap(t *testing.T) {
	src := `---
asterml: "2.1"
title: 物理守恒护栏测试
---

## 试剂与耗材
- VEGF 母液 100 ug/mL（-80°C 保存）
- 本实验培养体系中 VEGF 工作浓度 10 ng/mL

## 1.2 实验步骤
#@ 加药处理
- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	off := strings.Index(src, "将 VEGF 母液稀释至")
	sugs := AnalyzeLabSentenceWithContext(doc, "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", off, nil)
	if len(sugs) != 1 || !strings.Contains(sugs[0].Summary, "2.5 µL") {
		t.Fatalf("expected C1>C2 guard to inherit 100 ug/mL stock -> 2.5 uL (NOT working conc 10 ng/mL!), got %+v", sugs)
	}
}
