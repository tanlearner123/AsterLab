package asterdoc

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestSciGraphCompilesBlankExcludedSampleWellsAndZones(t *testing.T) {
	src := `---
asterml: "2.1"
title: HUVEC 测试
---

## 1.1 实验设计
- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF（稀释比 1/1000）
- **单一自变量**：GelMA 浓度（3%、5%、7% w/v），固定溶剂 0.25% (w/v) LAP
- **容器与复孔**：1 块 12 孔板（3×4），9 孔实验组（每组 3 技术复孔），第 4 列加无菌 PBS 防边缘蒸发

` + "```asterlab" + `
id: gt-huvec
kind: group_table
schema: 1
title: 实验分组
rows:
  - id: g1
    name: 3% GelMA
    role: treatment
    technical_repeats: 3
  - id: g2
    name: 5% GelMA
    role: treatment
    technical_repeats: 3
  - id: g3
    name: 7% GelMA
    role: treatment
    technical_repeats: 3
  - id: g_pbs
    name: PBS 边缘空白
    role: control
    technical_repeats: 3
` + "```" + `

` + "```asterlab" + `
id: plate-huvec
kind: plate
schema: 1
title: 12 孔板
format: 12
groups:
  g1: [A1, A2, A3]
  g2: [B1, B2, B3]
  g3: [C1, C2, C3]
  g_pbs: [A4, B4, C4]
roles:
  A4: blank
  B4: blank
  C4: blank
` + "```" + `

` + "```asterlab" + `
id: calc-vegf
kind: calculator
schema: 1
title: VEGF 稀释配制
mode: formula
rows:
  - key: C_1
    label: VEGF 母液浓度
    kind: input
    value: 100
    unit: ug/mL
  - key: C_2
    label: 目标工作浓度
    kind: input
    value: 0.05
    unit: ug/mL
  - key: V_2
    label: 配制终体积
    kind: input
    value: 5000
    unit: uL
  - key: V_1
    label: 取母液
    kind: formula
    formula: '\frac{C_{2} \times V_{2}}{C_{1}}'
    unit: uL
` + "```" + `

## 1.2 实验步骤

#@ 热循环溶解 {#t0}
- 开始 3 轮冷热交替水浴与冰浴循环
- 将 VEGF 母液稀释至 50 ng/mL 工作液

## 2. 实验讨论
- 上次 37°C 孵育 30 min 后凝胶回缩
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	g := BuildSciGraph(doc)
	if len(g.Containers) != 1 {
		t.Fatalf("expected 1 container, got %d", len(g.Containers))
	}
	pl := g.Containers[0]
	if pl.TotalWells != 12 || pl.SampleWells != 9 || pl.BlankWells != 3 {
		t.Fatalf("expected 12 total / 9 sample / 3 blank wells, got %+v", pl)
	}

	// Verify structural zone gating
	ctxMeta := g.ContextAt(strings.Index(src, "研究体系"))
	if ctxMeta.Zone != ZoneMetadata || ctxMeta.AllowedForNL() {
		t.Fatalf("expected ZoneMetadata blocked for 研究体系, got %+v", ctxMeta)
	}
	ctxVar := g.ContextAt(strings.Index(src, "单一自变量"))
	if ctxVar.Zone != ZoneMetadata || ctxVar.AllowedForNL() {
		t.Fatalf("expected ZoneMetadata blocked for 单一自变量, got %+v", ctxVar)
	}
	ctxPlateBullet := g.ContextAt(strings.Index(src, "容器与复孔"))
	if ctxPlateBullet.Zone != ZoneDesignPlan || !ctxPlateBullet.AllowsKind("plate_group") || ctxPlateBullet.AllowsKind("timer") {
		t.Fatalf("expected ZoneDesignPlan allowing plate_group only (not timer), got %+v", ctxPlateBullet)
	}
	ctxStepHead := g.ContextAt(strings.Index(src, "#@ 热循环溶解"))
	if ctxStepHead.Zone != ZoneHeading || ctxStepHead.AllowedForNL() {
		t.Fatalf("expected ZoneHeading blocked for step title, got %+v", ctxStepHead)
	}
	ctxStepBody := g.ContextAt(strings.Index(src, "将 VEGF 母液稀释至 50 ng/mL"))
	if ctxStepBody.Zone != ZoneStepBody || !ctxStepBody.AllowedForNL() {
		t.Fatalf("expected ZoneStepBody allowed, got %+v", ctxStepBody)
	}
	if ctxStepBody.StepTitle != "热循环溶解" {
		t.Fatalf("expected StepTitle=热循环溶解, got %q", ctxStepBody.StepTitle)
	}
	if !strings.Contains(ctxStepBody.PrevLine, "冷热交替") {
		t.Fatalf("expected PrevLine to capture preceding step sentence, got %q", ctxStepBody.PrevLine)
	}
	if ctxStepBody.ActivePlate == nil || ctxStepBody.ActivePlate.SampleWells != 9 {
		t.Fatalf("expected upstream ActivePlate with 9 SampleWells, got %+v", ctxStepBody.ActivePlate)
	}
	reag := ctxStepBody.LookupReagent("VEGF")
	if reag == nil || reag.StockVal != 100 || reag.StockUnit != "ug/mL" {
		t.Fatalf("expected upstream VEGF stock 100 ug/mL inherited from calc-vegf, got %+v", reag)
	}
	ctxReview := g.ContextAt(strings.Index(src, "上次 37°C 孵育"))
	if ctxReview.Zone != ZoneReview || ctxReview.AllowedForNL() {
		t.Fatalf("expected ZoneReview blocked in 讨论 section, got %+v", ctxReview)
	}
	summary := g.CompactSummary(strings.Index(src, "将 VEGF 母液稀释至 50 ng/mL"))
	if !strings.Contains(summary, "实验9孔") || !strings.Contains(summary, "VEGF(母液100ug/mL)") {
		t.Fatalf("unexpected compact summary: %s", summary)
	}

	// Verify reactive @plate.sample_wells evaluates to 9 (excluding 3 PBS blank wells),
	// while backward-compatible @plate.wells evaluates to 12.
	mmData := DefaultCalculatorPreset(doc, "s1", "mastermix")
	res := EvalCalculatorWithActuals(doc, mmData, nil)
	if res.Err != nil {
		t.Fatalf("mastermix preset eval failed: %v", res.Err)
	}
	// 9 sample wells * 200 uL * 1.15 = 2070 uL
	if len(res.Rows) == 0 || res.Rows[len(res.Rows)-1].Amount < 2069 || res.Rows[len(res.Rows)-1].Amount > 2071 {
		t.Fatalf("expected @plate.sample_wells=9 -> 2070 uL, got %+v", res.Rows)
	}

	// Verify all 9 curated universal calculator presets evaluate cleanly with zero errors
	for _, pr := range CalculatorFormulaPresets() {
		blk := DefaultCalculatorPreset(doc, "s1", pr.ID)
		r := EvalCalculatorWithActuals(doc, blk, nil)
		if r.Err != nil {
			t.Fatalf("calculator preset %q eval error: %v", pr.ID, r.Err)
		}
		if err := specValidator("calculator")(blk); err != nil {
			t.Fatalf("calculator preset %q failed spec validation: %v", pr.ID, err)
		}
	}

	// Verify Phase 2 SciGraph-aware NL pipeline:
	// 1. Structural zone gate blocks metadata, headings, and review sections deterministically
	if sugs := AnalyzeLabSentenceWithContext(doc, "- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF（稀释比 1/1000）", strings.Index(src, "研究体系"), nil); len(sugs) != 0 {
		t.Fatalf("expected metadata zone blocked, got %+v", sugs)
	}
	if sugs := AnalyzeLabSentenceWithContext(doc, "#@ 热循环溶解 {#t0}", strings.Index(src, "#@ 热循环溶解"), nil); len(sugs) != 0 {
		t.Fatalf("expected heading zone blocked, got %+v", sugs)
	}
	if sugs := AnalyzeLabSentenceWithContext(doc, "- 上次 37°C 孵育 30 min 后凝胶回缩", strings.Index(src, "上次 37°C 孵育"), nil); len(sugs) != 0 {
		t.Fatalf("expected review zone blocked, got %+v", sugs)
	}

	// 2. Upstream reagent stock inheritance from @calc-vegf inside step_body
	stepOff := strings.Index(src, "将 VEGF 母液稀释至 50 ng/mL")
	sugsDil := AnalyzeLabSentenceWithContext(doc, "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", stepOff, nil)
	if len(sugsDil) != 1 || sugsDil[0].Kind != "calculator" || sugsDil[0].InheritedFrom != "@calc-vegf" {
		if len(sugsDil) > 0 {
			t.Fatalf("expected VEGF dilution inheriting @calc-vegf, got %+v", *sugsDil[0])
		}
		t.Fatalf("expected VEGF dilution inheriting @calc-vegf, got empty")
	}
	if len(sugsDil[0].MissingSlots) != 0 {
		t.Fatalf("expected zero missing slots after inheriting VEGF stock, got %v", sugsDil[0].MissingSlots)
	}

	// 3. Upstream blank-excluded SampleWells (9, NOT 12) inheritance in attachment.grid
	sugsAtt := AnalyzeLabSentenceWithContext(doc, "- 对所有孔每孔采集 6 个非重叠明场视野", stepOff, nil)
	if len(sugsAtt) != 1 || sugsAtt[0].Kind != "attachment" || !strings.Contains(sugsAtt[0].Summary, "9 有效实验孔") || sugsAtt[0].InheritedFrom != "@plate-huvec" {
		t.Fatalf("expected attachment inheriting 9 SampleWells from @plate-huvec, got %+v", sugsAtt)
	}

	// 4. Ratio dilution (1:1000) & Multi-action clause split
	sugsRatio := AnalyzeLabSentenceWithContext(doc, "- 将二抗按 1:1000 比例稀释至终体积 10 mL", stepOff, nil)
	if len(sugsRatio) != 1 || sugsRatio[0].Kind != "calculator" || sugsRatio[0].Mode != "ratio_dilution" {
		t.Fatalf("expected ratio_dilution for 1:1000, got %+v", sugsRatio)
	}
	sugsMulti := AnalyzeLabSentenceWithContext(doc, "- 室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min", stepOff, nil)
	if len(sugsMulti) != 2 {
		t.Fatalf("expected 2 split suggestions for compound action sentence, got %d: %+v", len(sugsMulti), sugsMulti)
	}

	// 5. Missing duration explicitly flagged on timer.countdown
	sugsMiss := AnalyzeLabSentenceWithContext(doc, "- 37°C 培养箱静置孵育", stepOff, &JevRouteDecision{Kind: "timer", Mode: "countdown", Confidence: 0.9, Noul: 0.9})
	if len(sugsMiss) != 1 || len(sugsMiss[0].MissingSlots) == 0 {
		t.Fatalf("expected MissingSlots flagged for timer without duration, got %+v", sugsMiss)
	}
}

// TestSciGraphRealDynamicTraceLog records a verbatim end-to-end execution trace of SciGraph's
// real runtime lifecycle: cold compile, cache hit on steady frames, live edit -> SHA256 miss ->
// recompile -> reactive cascade across @plate.sample_wells, @calc chaining, prose->structured
// reagent override, Alt+Hover zone gating + ONNX sensing, and {#t0} anchor wake-up.
func TestSciGraphRealDynamicTraceLog(t *testing.T) {
	// Warm up ONNX session once so timings reflect steady-state app usage
	if r := DefaultNLOnnxRouter(); r != nil {
		_, _ = r.RouteDetail("预热")
	}

	v1Src := `---
asterml: "2.1"
title: HUVEC 动态加载追踪实测
---

## 1.1 实验设计
- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF

## 试剂与耗材
- VEGF 母液 100 ug/mL（-80°C 分装）

` + "```asterlab" + `
id: gt-1
kind: group_table
schema: 1
title: 实验分组
rows:
  - id: g1
    name: 5% GelMA
    role: treatment
    technical_repeats: 9
  - id: g_pbs
    name: PBS 空白
    role: control
    technical_repeats: 3
` + "```" + `

` + "```asterlab" + `
id: plate-huvec
kind: plate
schema: 1
title: 12 孔板
format: 12
groups:
  g1: [A1, A2, A3, B1, B2, B3, C1, C2, C3]
  g_pbs: [A4, B4, C4]
roles:
  A4: blank
  B4: blank
  C4: blank
` + "```" + `

` + "```asterlab" + `
id: calc-mm
kind: calculator
schema: 1
title: 前驱液总体积
mode: formula
rows:
  - key: N
    label: 有效实验孔数
    kind: input
    value: "@plate.sample_wells"
    unit: 孔
  - key: V_0
    label: 单孔体积
    kind: input
    value: 500
    unit: uL
  - key: r
    label: 富余比例
    kind: input
    value: 0.2
    unit: ""
  - key: V_tot
    label: 前驱液总量
    kind: formula
    formula: 'N \times V_{0} \times (1 + r)'
    unit: uL
` + "```" + `

` + "```asterlab" + `
id: calc-mass
kind: calculator
schema: 1
title: GelMA 称粉量（级联上游）
mode: formula
rows:
  - key: V_tot
    label: 上游总量
    kind: input
    value: "@calc.calc-mm.V_tot"
    unit: uL
  - key: C_w
    label: 浓度
    kind: input
    value: 5
    unit: "% w/v"
  - key: m
    label: 需称干粉
    kind: formula
    formula: 'C_{w} \times \frac{V_{tot}}{1000} \times 10'
    unit: mg
` + "```" + `

## 1.2 实验步骤

#@ 接种与加药 {#t0}
- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL
- 室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min
- 对全部实验孔每孔采集 6 个非重叠视野
`

	// --- STAGE 1: Cold open & compile ---
	doc1, err := Parse(v1Src)
	if err != nil {
		t.Fatal(err)
	}
	g1, hit1, dur1 := BuildSciGraphWithStats(doc1)
	t.Logf("[T1 COLD OPEN ] hash=%s cache_hit=%v compile=%s | containers=%d(sample=%d,blank=%d) reagents=%d(prose) calc_outputs=%d anchors=%d",
		g1.SourceHash, hit1, dur1, len(g1.Containers), g1.PrimaryPlate().SampleWells, g1.PrimaryPlate().BlankWells,
		len(g1.Reagents), len(g1.CalcOutputs), len(g1.Anchors))

	// --- STAGE 2: Same-frame repeated component reads (Cache HIT!) ---
	_, hit2, dur2 := BuildSciGraphWithStats(doc1)
	resMM1 := EvalCalculatorWithActuals(doc1, doc1.Modules[2].Data, nil)
	resMass1 := EvalCalculatorWithActuals(doc1, doc1.Modules[3].Data, nil)
	t.Logf("[T2 RENDER HIT] hash=%s cache_hit=%v lookup=%s | calc-mm=%q -> calc-mass=%q",
		g1.SourceHash, hit2, dur2, resMM1.Summary, resMass1.Summary)

	// --- STAGE 3: Alt+Hover on metadata line (zone-gated BEFORE ONNX!) ---
	metaOff := strings.Index(v1Src, "研究体系")
	tMeta := time.Now()
	ctxMeta := g1.ContextAt(metaOff)
	sugsMeta := AnalyzeLabSentenceWithContext(doc1, "- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF", metaOff, nil)
	t.Logf("[T3 HOVER META] offset=%d zone=%s allowed=%v elapsed=%s -> suggestions=%d (ONNX skipped!)",
		metaOff, ctxMeta.Zone, ctxMeta.AllowedForNL(), time.Since(tMeta), len(sugsMeta))

	// --- STAGE 4: Alt+Hover on VEGF dilution (inherits prose 100 ug/mL!) ---
	dilOff := strings.Index(v1Src, "将 VEGF 母液稀释至")
	tCtx := time.Now()
	ctxDil := g1.ContextAt(dilOff)
	ctxDur := time.Since(tCtx)
	tNL := time.Now()
	sugsDil1 := AnalyzeLabSentenceWithContext(doc1, "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", dilOff, nil)
	t.Logf("[T4 HOVER DIL1] ContextAt=%s zone=%s step=%q | Analyze=%s -> %s/%s (%.0f%%) inherit=%q missing=%v | %s",
		ctxDur, ctxDil.Zone, ctxDil.StepTitle, time.Since(tNL),
		sugsDil1[0].Kind, sugsDil1[0].Mode, sugsDil1[0].Confidence*100, sugsDil1[0].InheritedFrom, sugsDil1[0].MissingSlots, sugsDil1[0].Summary)

	// --- STAGE 5: Alt+Hover on compound sentence (v1.3.0 is_multi probe -> 2 widgets!) ---
	mulOff := strings.Index(v1Src, "室温避光二抗孵育")
	tMul := time.Now()
	sugsMul := AnalyzeLabSentenceWithContext(doc1, "- 室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min", mulOff, nil)
	t.Logf("[T5 HOVER MULT] Analyze=%s -> split=%d widgets: [1] %s | [2] %s",
		time.Since(tMul), len(sugsMul), sugsMul[0].Summary, sugsMul[1].Summary)

	// --- STAGE 6: LIVE USER EDIT #1 -> Mark C3 as blank too (9 -> 8 sample wells!) ---
	v2Src := strings.Replace(v1Src, "  C4: blank", "  C3: blank\n  C4: blank", 1)
	doc2, err := Parse(v2Src)
	if err != nil {
		t.Fatal(err)
	}
	g2, hitEdit1, durEdit1 := BuildSciGraphWithStats(doc2)
	resMM2 := EvalCalculatorWithActuals(doc2, doc2.Modules[2].Data, nil)
	resMass2 := EvalCalculatorWithActuals(doc2, doc2.Modules[3].Data, nil)
	attOff2 := strings.Index(v2Src, "对全部实验孔每孔采集")
	sugsAtt2 := AnalyzeLabSentenceWithContext(doc2, "- 对全部实验孔每孔采集 6 个非重叠视野", attOff2, nil)
	t.Logf("[T6 EDIT PLATE] hash=%s (old=%s) cache_hit=%v recompile=%s | SampleWells: 9->%d | CASCADE: calc-mm=%q -> calc-mass=%q | attachment=%q",
		g2.SourceHash, g1.SourceHash, hitEdit1, durEdit1, g2.PrimaryPlate().SampleWells,
		resMM2.Summary, resMass2.Summary, sugsAtt2[0].Summary)

	// --- STAGE 7: LIVE USER EDIT #2 -> Insert structured calc-vegf with 50 ug/mL stock (overrides prose 100 ug/mL!) ---
	vegfCalcBlock := "\n```asterlab\nid: calc-vegf\nkind: calculator\nschema: 1\ntitle: VEGF 母液复溶\nmode: formula\nrows:\n  - key: C_1\n    label: VEGF 母液浓度\n    kind: input\n    value: 50\n    unit: ug/mL\n  - key: V\n    label: 分装体积\n    kind: input\n    value: 1\n    unit: mL\n  - key: m\n    label: 总量\n    kind: formula\n    formula: 'C_{1} \\times V'\n    unit: ug\n```\n"
	v3Src := strings.Replace(v2Src, "## 1.2 实验步骤", vegfCalcBlock+"\n## 1.2 实验步骤", 1)
	doc3, err := Parse(v3Src)
	if err != nil {
		t.Fatal(err)
	}
	g3, hitEdit2, durEdit2 := BuildSciGraphWithStats(doc3)
	dilOff3 := strings.Index(v3Src, "将 VEGF 母液稀释至")
	sugsDil3 := AnalyzeLabSentenceWithContext(doc3, "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", dilOff3, nil)
	t.Logf("[T7 OVERRIDE  ] hash=%s cache_hit=%v recompile=%s | VEGF stock 100(prose) -> 50(@calc-vegf) | inherit=%q | %s",
		g3.SourceHash, hitEdit2, durEdit2, sugsDil3[0].InheritedFrom, sugsDil3[0].Summary)

	if g2.PrimaryPlate().SampleWells != 8 || !strings.Contains(resMass2.Summary, "240") {
		t.Fatalf("expected reactive cascade 8 sample wells -> 240 mg, got %d / %q", g2.PrimaryPlate().SampleWells, resMass2.Summary)
	}
	if sugsDil3[0].InheritedFrom != "@calc-vegf" || !strings.Contains(sugsDil3[0].Summary, "5 µL") {
		t.Fatalf("expected structured @calc-vegf override -> 5 uL, got %+v", sugsDil3[0])
	}
	_ = fmt.Sprintf("trace ok")
}

// TestSciGraphInheritanceAblationAudit runs an honest A/B comparison (WITHOUT vs WITH SciGraph
// inheritance) plus adversarial mis-inheritance traps (working conc vs stock conc, 2 plates in
// 1 document, multiple reagents) to measure real accuracy and surface selection edge cases.
func TestSciGraphInheritanceAblationAudit(t *testing.T) {
	src := `---
asterml: "2.1"
title: 继承准确性与陷阱实测
---

## 1.1 实验设计
- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF（终浓度 10 ng/mL）

## 试剂与耗材
- VEGF 母液 100 ug/mL（-80°C 保存）
- DAPI 储液 1 mg/mL（-20°C 避光保存）
- Primer 引物母液 100 uM

` + "```asterlab" + `
id: gt-1
kind: group_table
schema: 1
title: 分组
rows:
  - id: g1
    name: 组1
    role: treatment
    technical_repeats: 9
  - id: g_pbs
    name: PBS
    role: control
    technical_repeats: 3
  - id: g_elisa
    name: ELISA 样品
    role: treatment
    technical_repeats: 24
` + "```" + `

` + "```asterlab" + `
id: plate-culture
kind: plate
schema: 1
title: 12 孔培养板
format: 12
groups:
  g1: [A1, A2, A3, B1, B2, B3, C1, C2, C3]
  g_pbs: [A4, B4, C4]
roles:
  A4: blank
  B4: blank
  C4: blank
` + "```" + `

## 1.2 实验步骤

#@ 步骤 1：细胞培养与染色 {#t0}
- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL
- 将 DAPI 母液稀释至 1 ug/mL 工作液，终体积 5 mL
- 将 Primer 母液稀释至 10 uM 工作液，终体积 100 uL
- 按单孔 100 uL、加 15% 余量配制工作液预混液
- 实测细胞悬液密度 5e5 cells/mL，每孔接种 5e4 细胞，每孔 500 uL
- 对全部实验孔每孔采集 6 个非重叠明场视野

#@ 步骤 2：上清转移至 96 孔 ELISA 板检测 {#t1}

` + "```asterlab" + `
id: plate-elisa
kind: plate
schema: 1
title: 96 孔 ELISA 检测板
format: 96
groups:
  g_elisa: [A1, A2, A3, A4, A5, A6, A7, A8, B1, B2, B3, B4, B5, B6, B7, B8, C1, C2, C3, C4, C5, C6, C7, C8]
` + "```" + `

- 按单孔 50 uL、加 10% 余量配制 TMB 显色预混液
- 对全部实验孔每孔采集 4 个视野
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name     string
		line     string
		wantSub  string
		wantFrom string
	}{
		{"VEGF稀释(工作浓度干扰下选母液)", "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", "2.5 µL", "文档试剂定义"},
		{"DAPI稀释(多试剂不串台)", "- 将 DAPI 母液稀释至 1 ug/mL 工作液，终体积 5 mL", "5 µL", "文档试剂定义"},
		{"Primer稀释(uM单位匹配)", "- 将 Primer 母液稀释至 10 uM 工作液，终体积 100 uL", "10 µL", "文档试剂定义"},
		{"步骤1预混(继承plate-culture 9孔)", "- 按单孔 100 uL、加 15% 余量配制工作液预混液", "9 孔", "@plate-culture"},
		{"步骤1接种(继承plate-culture 9孔)", "- 实测细胞悬液密度 5e5 cells/mL，每孔接种 5e4 细胞，每孔 500 uL", "0.9 mL", "@plate-culture"},
		{"步骤1采图(继承plate-culture 9孔)", "- 对全部实验孔每孔采集 6 个非重叠明场视野", "9 有效实验孔", "@plate-culture"},
		{"步骤2预混(双板切换继承plate-elisa 24孔)", "- 按单孔 50 uL、加 10% 余量配制 TMB 显色预混液", "24 孔", "@plate-elisa"},
		{"步骤2采图(双板切换继承plate-elisa 24孔)", "- 对全部实验孔每孔采集 4 个视野", "24 有效实验孔", "@plate-elisa"},
	}

	okNoCtx, okWithCtx := 0, 0
	missNoCtx, missWithCtx := 0, 0
	for _, tc := range cases {
		off := strings.Index(src, tc.line)
		// A: WITHOUT SciGraph (doc = nil, no context)
		sNo := AnalyzeLabSentenceWithContext(nil, tc.line, -1, nil)
		// B: WITH SciGraph (real doc + exact offset)
		sWith := AnalyzeLabSentenceWithContext(doc, tc.line, off, nil)

		noSum, withSum := "<nil>", "<nil>"
		noMiss, withMiss := 0, 0
		withFrom := ""
		if len(sNo) > 0 {
			noSum = sNo[0].Summary
			noMiss = len(sNo[0].MissingSlots)
			if strings.Contains(noSum, tc.wantSub) && noMiss == 0 {
				okNoCtx++
			}
			missNoCtx += noMiss
		}
		if len(sWith) > 0 {
			withSum = sWith[0].Summary
			withMiss = len(sWith[0].MissingSlots)
			withFrom = sWith[0].InheritedFrom
			if strings.Contains(withSum, tc.wantSub) && withMiss == 0 {
				okWithCtx++
			}
			missWithCtx += withMiss
		}
		t.Logf("%-34s | NO_CTX: miss=%d %s || WITH_CTX: inherit=%q miss=%d %s",
			tc.name, noMiss, noSum, withFrom, withMiss, withSum)
	}
	t.Logf("ABLATION TOTAL (n=%d): NO_CTX exact=%d/%d (missing_slots=%d) vs WITH_CTX exact=%d/%d (missing_slots=%d)",
		len(cases), okNoCtx, len(cases), missNoCtx, okWithCtx, len(cases), missWithCtx)

	// Trap check: what if prose mentions "终浓度 10 ng/mL VEGF" AFTER "VEGF 母液 100 ug/mL"?!
	trapSrc := strings.Replace(src,
		"- VEGF 母液 100 ug/mL（-80°C 保存）",
		"- VEGF 母液 100 ug/mL（-80°C 保存）\n- 本实验培养体系中 VEGF 工作浓度 10 ng/mL", 1)
	trapDoc, _ := Parse(trapSrc)
	trapOff := strings.Index(trapSrc, "将 VEGF 母液稀释至")
	trapSug := AnalyzeLabSentenceWithContext(trapDoc, "- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL", trapOff, nil)
	if len(trapSug) > 0 {
		t.Logf("[TRAP CHECK: 工作浓度写在母液后面] inherit=%q summary=%s", trapSug[0].InheritedFrom, trapSug[0].Summary)
	}
}
