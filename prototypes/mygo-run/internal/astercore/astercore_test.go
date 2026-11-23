package astercore

import (
	"strings"
	"testing"

	"asterlab-mygo-prototype/internal/asterdoc"
)

func TestAssessDocumentGradedDiagnostics(t *testing.T) {
	badSrc := `---
title: ""
status: "draft"
---

` + "```" + `asterlab
id: step-1
kind: step
schema: 1
title: 稀释与分组
blocks:
  - id: blk-grp
    kind: group_table
    schema: 1
    rows:
      - id: grp-treated
        name: Treated
        role: treatment
        independent_n: 0
  - id: blk-calc
    kind: calculator
    schema: 1
    mode: dilution
    stock:
      value: 10
      unit: mM
    target:
      value: 25
      unit: mM
    final_volume:
      value: 5
      unit: mL
    rows:
      - key: c1
        value: 10
      - key: c2
        value: 25
` + "```" + `
`
	doc, err := asterdoc.Parse(badSrc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	rep := AssessDocument(doc, nil)
	if rep.Ready {
		t.Fatalf("expected Ready=false when blocking issues exist")
	}
	if rep.Blocking == 0 || rep.Confirm == 0 {
		t.Fatalf("expected blocking + needs_confirmation diagnostics, got %+v", rep)
	}
	hasCode := func(code string) bool {
		for _, d := range rep.Diagnostics {
			if d.Code == code {
				return true
			}
		}
		return false
	}
	for _, want := range []string{"TARGET_EXCEEDS_STOCK", "ARRIVE_MISSING_N", "NO_CONTROL_GROUP", "UNSCHEDULED_DATE"} {
		if !hasCode(want) {
			t.Fatalf("expected diagnostic %s, got %+v", want, rep.Diagnostics)
		}
	}
}

func TestAnalyzeDocumentCCK8AndQPCR(t *testing.T) {
	cck8Src := `---
title: "CCK-8 细胞活力"
date: "2026-11-20"
---

` + "```" + `asterlab
id: step-1
kind: step
schema: 1
title: 分组与读板
blocks:
  - id: blk-grp
    kind: group_table
    schema: 1
    rows:
      - id: grp-ctrl
        name: Control
        role: control
        independent_n: 3
      - id: grp-treated
        name: Treated
        role: treatment
        independent_n: 3
  - id: blk-dt
    kind: data_table
    schema: 1
    title: "OD450"
    columns:
      - key: well
        label: 孔位
        value_kind: text
      - key: group
        label: 分组
        value_kind: text
      - key: value
        label: OD450
        value_kind: number
    rows:
      - well: A1
        group: Control
        value: 1.00
      - well: A2
        group: Control
        value: 1.00
      - well: B1
        group: Treated
        value: 0.50
      - well: B2
        group: Treated
        value: 0.50
` + "```" + `
`
	doc, err := asterdoc.Parse(cck8Src)
	if err != nil {
		t.Fatalf("Parse CCK8: %v", err)
	}
	sums := AnalyzeDocument(doc, "cck8-doc")
	if len(sums) != 1 || sums[0].Kind != "cck8" || sums[0].CCK8 == nil {
		t.Fatalf("expected 1 cck8 summary, got %+v", sums)
	}

	qpcrSrc := `---
title: "RT-qPCR 相对定量"
date: "2026-11-20"
---

` + "```" + `asterlab
id: step-1
kind: step
schema: 1
title: Ct 读数
blocks:
  - id: blk-grp
    kind: group_table
    schema: 1
    rows:
      - id: grp-ctrl
        name: Ctrl
        role: control
        independent_n: 3
      - id: grp-drug
        name: Drug
        role: treatment
        independent_n: 3
  - id: blk-dt
    kind: data_table
    schema: 1
    title: "qPCR Cq"
    columns:
      - key: sample
        label: 样本
        value_kind: text
      - key: group
        label: 分组
        value_kind: text
      - key: gene
        label: 基因
        value_kind: text
      - key: value
        label: Cq
        value_kind: number
    rows:
      - sample: Ctrl
        group: Ctrl
        gene: GAPDH
        value: 18.0
      - sample: Ctrl
        group: Ctrl
        gene: TP53
        value: 22.0
      - sample: Drug
        group: Drug
        gene: GAPDH
        value: 18.0
      - sample: Drug
        group: Drug
        gene: TP53
        value: 20.0
` + "```" + `
`
	qDoc, err := asterdoc.Parse(qpcrSrc)
	if err != nil {
		t.Fatalf("Parse qPCR: %v", err)
	}
	qSums := AnalyzeDocument(qDoc, "qpcr-doc")
	if len(qSums) != 1 || qSums[0].Kind != "qpcr" || qSums[0].QPCR == nil {
		t.Fatalf("expected 1 qpcr summary, got %+v", qSums)
	}
}

func TestV020AnalyzeDocumentSciUnitMultiPlateBlankAndExcluded(t *testing.T) {
	src := `---
title: "SciUnit 统计闭环测试（多板 + Blank 本底 + 异常孔剔除 + 生/技重复区分）"
date: "2026-11-22"
---

` + "```asterlab" + `
id: gt-main
kind: group_table
schema: 1
rows:
  - id: g_ctrl
    name: Control
    role: control
    independent_n: 2
  - id: g_drug
    name: Drug
    role: treatment
    independent_n: 2
  - id: g_blank
    name: Blank
    role: control
    independent_n: 1
` + "```" + `

` + "```asterlab" + `
id: plate-1
kind: plate
schema: 1
format: 12
groups:
  g_ctrl: [A1, A2]
  g_drug: [B1, B2, B3]
  g_blank: [C1, C2]
roles:
  C1: blank
  C2: blank
excluded_wells: [B2]
samples:
  A1: Mouse-1
  A2: Mouse-2
  B1: Mouse-3
  B3: Mouse-3
` + "```" + `

` + "```asterlab" + `
id: dt-plate1
kind: data_table
schema: 1
title: "板1 荧光读数"
container_ref: plate-1
columns:
  - key: well
    label: 孔位
    value_kind: text
  - key: value
    label: Intensity
    value_kind: number
rows:
  - well: A1
    value: 110
  - well: A2
    value: 110
  - well: B1
    value: 60
  - well: B2
    value: 9999
  - well: B3
    value: 60
  - well: C1
    value: 10
  - well: C2
    value: 10
` + "```" + `
`
	doc, err := asterdoc.Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sums := AnalyzeDocument(doc, "sciunit-stat")
	if len(sums) != 1 {
		t.Fatalf("expected 1 summary, got %+v", sums)
	}
	s := sums[0]
	if s.PlateID != "plate-1" {
		t.Fatalf("expected PlateID=plate-1, got %q", s.PlateID)
	}
	if s.ExcludedN != 1 {
		t.Fatalf("expected ExcludedN=1 (B2 excluded), got %d", s.ExcludedN)
	}
	if s.BlankN != 2 || s.BlankMean == nil || *s.BlankMean != 10 {
		t.Fatalf("expected BlankN=2 BlankMean=10, got BlankN=%d BlankMean=%v", s.BlankN, s.BlankMean)
	}
	// General fluorescence intensity table MUST remain descriptive (NOT misclassified as CCK-8 viability!)
	// while still excluding B2=9999, computing BlankMean=10, and separating TechReplicates=2 vs BioSamples=1!
	if s.Kind != "descriptive" {
		t.Fatalf("expected descriptive summary for fluorescence intensity table, got %+v", s)
	}
	for _, g := range s.Groups {
		if g.Group == "Drug" {
			if g.TechReplicates != 2 || g.BioSamples != 1 {
				t.Fatalf("expected Drug TechReplicates=2 BioSamples=1, got %+v", g)
			}
			if g.BlankCorrMean == nil || *g.BlankCorrMean != 50 {
				t.Fatalf("expected Drug BlankCorrMean = 60 - 10 = 50, got %v", g.BlankCorrMean)
			}
		}
	}

	// Explicit CCK-8 / OD450 table DOES trigger relative viability analysis with B2=9999 excluded!
	cckSrc := strings.Replace(src, `"板1 荧光读数"`, `"板1 CCK-8 OD450 读数"`, 1)
	cckDoc, err := asterdoc.Parse(cckSrc)
	if err != nil {
		t.Fatal(err)
	}
	cSums := AnalyzeDocument(cckDoc, "sciunit-cck8")
	if len(cSums) != 1 || cSums[0].Kind != "cck8" || cSums[0].CCK8 == nil {
		t.Fatalf("expected cck8 summary for explicit CCK-8 OD450 table, got %+v", cSums)
	}
	for _, g := range cSums[0].CCK8.Groups {
		if g.Group == "Drug" && g.N != 2 {
			t.Fatalf("expected Drug N=2 after excluding B2=9999, got %+v", g)
		}
	}
}
