package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/vault"
)

func buildComplexHUVECSource(imgRel string) string {
	return `---
asterml: "2.1"
title: HUVEC 体外成管实验（GelMA 浓度梯度全流程）
scheduled_date: "2026-11-21"
stage: in_progress
---

## 1.1 因子、分组与 12 孔板布局
- **研究体系**：DMEM 高糖 + 10% FBS + 10 ng/mL VEGF（稀释比 1/1000）
- **单一自变量**：GelMA 浓度（3%、5%、7% w/v），固定溶剂 0.25% (w/v) LAP
- **容器与复孔**：1 块 12 孔板（3×4），9 孔实验组（每组 3 技术复孔），第 4 列加无菌 PBS 防边缘蒸发

` + "```asterlab" + `
id: gt-huvec
kind: group_table
schema: 1
title: 实验分组
rows:
  - id: g-3-gelma
    name: 3% GelMA
    role: treatment
    condition: 3% w/v GelMA + 0.25% LAP
    technical_repeats: 3
  - id: g-5-gelma
    name: 5% GelMA
    role: treatment
    condition: 5% w/v GelMA + 0.25% LAP
    technical_repeats: 3
  - id: g-7-gelma
    name: 7% GelMA
    role: treatment
    condition: 7% w/v GelMA + 0.25% LAP
    technical_repeats: 3
  - id: g-pbs
    name: 空白 / PBS
    role: control
    condition: 无菌 PBS 防蒸发
    technical_repeats: 3
` + "```" + `

` + "```asterlab" + `
id: plate-huvec
kind: plate
schema: 1
title: 12 孔成管布局板
format: 12
groups:
  g-3-gelma: [A1, A2, A3]
  g-5-gelma: [B1, B2, B3]
  g-7-gelma: [C1, C2, C3]
  g-pbs: [A4, B4, C4]
roles:
  A4: blank
  B4: blank
  C4: blank
samples:
  A1: 3% GelMA
  A2: 3% GelMA
  A3: 3% GelMA
  B1: 5% GelMA
  B2: 5% GelMA
  B3: 5% GelMA
  C1: 7% GelMA
  C2: 7% GelMA
  C3: 7% GelMA
replicates:
  A1: 1
  A2: 2
  A3: 3
  B1: 1
  B2: 2
  B3: 3
  C1: 1
  C2: 2
  C3: 3
` + "```" + `

## 1.2 配液与细胞计算清单（居中 LaTeX 公式）

` + "```asterlab" + `
id: calc-vegf
kind: calculator
schema: 1
title: VEGF 工作液稀释配制 (C₁V₁ = C₂V₂)
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
    label: 细胞悬液终体积（含余量）
    kind: input
    value: 5000
    unit: uL
  - key: V_1
    label: 取 VEGF 母液
    kind: formula
    formula: '\frac{C_{2} \times V_{2}}{C_{1}}'
    unit: uL
  - key: V_med
    label: 补加完全培养基
    kind: formula
    formula: 'V_{2} - \frac{C_{2} \times V_{2}}{C_{1}}'
    unit: uL
` + "```" + `

## 1.3 实验步骤

#@ GelMA 前驱液溶解与光交联

` + "```asterlab" + `
id: calc-gelma
kind: calculator
schema: 1
title: 自定义 LaTeX · GelMA 称量配液
mode: formula
rows:
  - key: C_g
    label: 目标 GelMA 浓度
    kind: input
    value: 5
    unit: "% w/v"
  - key: V_g
    label: 单组前驱液体积（含 +1 孔余量）
    kind: input
    value: 2
    unit: mL
  - key: m_g
    label: 称取 GelMA 冻干粉
    kind: formula
    formula: 'C_{g} \times V_{g} \times 10'
    unit: mg
` + "```" + `

##@ 热循环溶解与脱泡
- 按计算清单称取 60 / 100 / 140 mg GelMA 分别加入 2.0 mL 0.25% LAP
- 开始 3 轮冷热交替水浴与冰浴循环

` + "```asterlab" + `
id: timer-cycle
kind: timer
schema: 1
title: 热循环溶解计时
mode: cycle
rounds: 3
status: running
phases:
  - name: 80 °C 水浴加热
    mode: fixed
    duration: {value: 5, unit: min}
  - name: 冰上静置溶解
    mode: fixed
    duration: {value: 10, unit: min}
` + "```" + `

- 肉眼确认溶液完全澄清；若无气泡则按规则跳过 2000 rpm 离心脱泡

` + "```asterlab" + `
id: param-dissolve
kind: param_table
schema: 1
title: 溶解与脱泡条件
rows:
  - key: bath_temp
    label: 水浴温度
    value: 80
    unit: Cel
    actual: 80.2
    record_actual: true
  - key: spin_rpm
    label: 离心脱泡转速（条件执行）
    value: 2000
    unit: rpm
    record_actual: true
` + "```" + `

##@ 铺胶与 405 nm 光交联
1. 按 12 孔板排布加入 0.5 mL/孔前驱液并轻晃铺匀
2. 405 nm 蓝光照射至水凝胶完全固化

` + "```asterlab" + `
id: timer-uv
kind: timer
schema: 1
title: 405 nm 光固化正计时
mode: stopwatch
elapsed_sec: 124
status: completed
` + "```" + `

#@ 浸泡平衡与 HUVEC 接种

##@ 4 °C 完全培养基浸泡平衡
- 每孔加入 1.0 mL 无 VEGF 完全培养基，4 °C 浸泡 ≥ 2 h

` + "```asterlab" + `
id: timer-soak
kind: timer
schema: 1
title: 4 °C 平衡浸泡等待窗
mode: window
anchor: soak_start
offset: {value: 2, unit: h}
schedule_window: "≥ 2 h（可并行处理其他实验）"
` + "```" + `

##@ 复温接种 HUVEC + VEGF {#t0}
- 37 °C 复温并轻柔吸弃浸泡液
- 接种 0.5 mL/孔 HUVEC 悬液（1×10⁵ cells/孔，含 10 ng/mL VEGF），记录绝对时刻 t0

#@ 多时间点明场采图与成管定量

##@ t0 + 6 h 明场显微采图（9 孔 × 6 视野） {#obs-6h}
- 完成 A1–C3 共 9 孔 × 6 视野（54 张）明场采图

` + "```asterlab" + `
id: att-6h-grid
kind: attachment
schema: 1
title: 6 h 孔板网格采图槽位
scope: container_grid
container_ref: plate-huvec
slots_per_cell: 6
media_hint: 每孔随机取 6 个非重叠 10× 明场视野（共 54 张）
` + "```" + `

![[` + imgRel + `]]{@plate-huvec:A1 @obs-6h 6h fov=1 role=obs}

##@ t0 + 12 h / 24 h 随访与成管定量
- 回填 ImageJ Angiogenesis Analyzer 管腔总长度与节点数

` + "```asterlab" + `
id: dt-angiogenesis
kind: data_table
schema: 1
title: HUVEC 成管定量汇总（6 h）
source_ref: "@groups"
columns:
  - key: group
    label: 组别
    value_kind: text
  - key: total_len
    label: 管腔总长度
    value_kind: number
    unit: mm
  - key: junctions
    label: 分支节点数
    value_kind: number
rows:
  - group: 3% GelMA
    total_len: 14.8
    junctions: 42
  - group: 5% GelMA
    total_len: 21.6
    junctions: 68
  - group: 7% GelMA
    total_len: 11.2
    junctions: 31
` + "```" + `
`
}

func TestComplexHUVECFullWorkflowAudit(t *testing.T) {
	vRoot := t.TempDir()
	v, err := vault.Open(vRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SeedTemplates(defaultTemplateSeeds()); err != nil {
		t.Fatal(err)
	}
	imgRel, err := v.ImportAssetBytes(makeTinyTestPNG(t), "huvec_a1_6h_fov1.png")
	if err != nil {
		t.Fatal(err)
	}
	richSrc := buildComplexHUVECSource(imgRel)
	relPath := "HUVEC/huvec-full-audit.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(relPath)), []byte(richSrc)); err != nil {
		t.Fatal(err)
	}

	// 1. Parse & Outline & Relation Audit
	doc, err := asterdoc.Parse(richSrc)
	if err != nil {
		t.Fatalf("Parse rich HUVEC failed: %v", err)
	}
	outline := asterdoc.ParseDocumentOutline(richSrc, doc)
	steps := asterdoc.ParseStepTree(richSrc, doc)
	rels := asterdoc.ResolveRelations(doc)
	t.Logf("[1. STRUCTURE] modules=%d outlineRoots=%d topSteps=%d relations=%d",
		len(doc.Modules), len(outline), len(steps.Roots), len(rels))
	for _, r := range rels {
		t.Logf("  REL source=%s module=%s targets=%d contexts=%d issues=%v", r.SourceKind, r.ModuleID, len(r.Targets), len(r.Contexts), r.Issues)
	}

	// 2. Audit EVERY natural-language line in richSrc through SciGraph + live offline ONNX router!
	t.Logf("[2. SCIGRAPH + ONNX NL ROUTING ON EVERY HUVEC PROSE LINE]")
	inFence := false
	inFM := false
	lineOff := 0
	for lnNo, line := range strings.Split(richSrc, "\n") {
		curOff := lineOff
		lineOff += len(line) + 1
		trim := strings.TrimSpace(line)
		if lnNo == 0 && trim == "---" {
			inFM = true
			continue
		}
		if inFM {
			if trim == "---" {
				inFM = false
			}
			continue
		}
		if strings.HasPrefix(trim, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trim == "" || strings.HasPrefix(trim, "## ") || strings.HasPrefix(trim, "# ") || strings.HasPrefix(trim, "![[") {
			continue
		}
		sugs := asterdoc.AnalyzeLabSentenceWithContext(doc, line, curOff, nil)
		if len(sugs) == 0 {
			t.Logf("  L%03d [NONE] %s", lnNo+1, trim)
			continue
		}
		for _, sug := range sugs {
			t.Logf("  L%03d [%s/%s %.0f%% %s inherit=%q] missing=%v | %s -> %s",
				lnNo+1, sug.Kind, sug.Mode, sug.Confidence*100, sug.Source, sug.InheritedFrom, sug.MissingSlots, trim, sug.Summary)
		}
		// Assert metadata bullets are NEVER mis-suggested
		if strings.Contains(trim, "研究体系") || strings.Contains(trim, "单一自变量") || strings.HasPrefix(trim, "#@") || strings.HasPrefix(trim, "##@") {
			t.Fatalf("SciGraph zone gate failed on L%03d: %s", lnNo+1, trim)
		}
	}

	// 3. Audit Calculator evaluation & @plate.sample_wells resolution
	for _, m := range doc.Modules {
		if m.Kind == "calculator" {
			res := asterdoc.EvalCalculatorWithActuals(doc, m.Data, nil)
			t.Logf("[3. CALC EVAL] id=%s summary=%q err=%v", m.ID, res.Summary, res.Err)
			for _, row := range res.Rows {
				t.Logf("    row label=%s amount=%v unit=%s note=%q", row.Label, row.Amount, row.Unit, row.Note)
			}
		}
	}

	// 4. Interactive Real App Shell Audit (Calculator Formula Library, tabs, edit toggle + collapse, @plate.sample_wells!)
	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "HUVEC 全流程审计", shell.ModeEdit, true)
	sh.RightOpen = true
	sh.RightTab = 0

	tt := ui.NewTester(func(c *ui.Context) {
		c.SetTheme(c.Theme())
		sh.RenderSplitView(c)
	}, 1400, 2600)
	tt.Frame()

	// 4a. Check formula output tab switching on calc-vegf ("取 VEGF 母液" -> "补加完全培养基")
	if err := tt.Click("查看公式：补加完全培养基"); err != nil {
		t.Fatalf("[4a] Cannot click formula tab 补加完全培养基: %v", err)
	}
	tt.Frame()
	if !tt.HasText("4997.5 μL") {
		t.Fatalf("[4a] Expected 4997.5 μL after switching tab")
	}
	if !tt.HasText("公式库") || !tt.HasText("复制 LaTeX") {
		t.Fatalf("[4a] Expected Formula Library button & Copy LaTeX button in calculator")
	}
	t.Logf("[4a. CALC TAB + LIB] Verified 4997.5 μL, 公式库, 复制 LaTeX")

	// 4b. Check clicking "切换计算模式：% w/v 称量" on calc-vegf
	beforeLD, _ := a.docStore.Open(relPath)
	beforeSrc := beforeLD.Source
	if err := tt.Click("切换计算模式：% w/v 称量"); err != nil {
		t.Fatalf("[4b] Cannot click 切换计算模式：%% w/v 称量: %v", err)
	}
	tt.Frame()
	afterLD, _ := a.docStore.Open(relPath)
	if beforeSrc == afterLD.Source || !strings.Contains(afterLD.Source, "% w/v 称量配液计算") {
		t.Fatalf("[4b] Mode switch to %% w/v 称量 did NOT update source!")
	}
	t.Logf("[4b. CALC MODE SWITCH] Clicked %% w/v 称量 -> persisted=true")

	// 4c. Check clicking "切换计算模式：孔板预混" -> verify @plate.sample_wells == 9 -> 2070 uL!
	if err := tt.Click("切换计算模式：孔板预混"); err != nil {
		t.Fatalf("[4c] Cannot click 切换计算模式：孔板预混: %v", err)
	}
	tt.Frame()
	afterLD, _ = a.docStore.Open(relPath)
	doc2, _ := asterdoc.Parse(afterLD.Source)
	foundMM := false
	for _, m := range doc2.Modules {
		if m.ID == "calc-vegf" {
			res2 := asterdoc.EvalCalculatorWithActuals(doc2, m.Data, nil)
			t.Logf("[4c. CALC MASTERMIX @plate.sample_wells] summary=%q err=%v rows=%d", res2.Summary, res2.Err, len(res2.Rows))
			if res2.Err != nil || !strings.Contains(res2.Summary, "2070") {
				t.Fatalf("[4c] Expected @plate.sample_wells=9 -> 2070 uL, got %q (err=%v)", res2.Summary, res2.Err)
			}
			foundMM = true
		}
	}
	if !foundMM {
		t.Fatalf("[4c] calc-vegf module not found after mastermix switch")
	}

	// 4d. Check "编辑公式" opens editor AND "收起公式栏" ACTUALLY COLLAPSES it!
	if err := tt.Click("编辑公式"); err != nil {
		t.Fatalf("[4d] Cannot click 编辑公式: %v", err)
	}
	tt.Frame()
	if !tt.HasText("收起公式栏") || !tt.HasText("插入符号") {
		t.Fatalf("[4d] Expected formula editor open after clicking 编辑公式")
	}
	if err := tt.Click("收起公式栏"); err != nil {
		t.Fatalf("[4d] Cannot click 收起公式栏: %v", err)
	}
	tt.Frame()
	if tt.HasText("插入符号") {
		t.Fatalf("[4d] Expected formula editor COLLAPSED after clicking 收起公式栏")
	}
	t.Logf("[4d. CALC EDIT TOGGLE] Open & Collapse verified cleanly!")

	// 4e. Check Read Mode renders cleanly
	sh.SetActiveTabMode(shell.ModeRead)
	tt.Frame()
	if !tt.HasText("HUVEC 体外成管实验") {
		t.Fatalf("[4e] Read mode missing title")
	}
	t.Logf("[4e. READ MODE] Rendered Read Mode cleanly")
	_ = fmt.Sprintf("done")
}

func TestSciGraphAndCalculatorVisualEvidence(t *testing.T) {
	vRoot := t.TempDir()
	outDir := filepath.Join("..", "..", "docs", "sdk", "evidence", "v2-visual-catalog")
	_ = os.MkdirAll(outDir, 0o755)

	demoDoc := `---
asterml: "2.1"
title: SciGraph 科学引擎与公式库演示
---

## 1.1 实验设计
- **研究体系**：DMEM + 10% FBS + 10 ng/mL VEGF（稀释比 1/1000）

` + "```asterlab" + `
id: gt-demo
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
    name: PBS 边缘防蒸发
    role: control
    technical_repeats: 3
` + "```" + `

` + "```asterlab" + `
id: plate-demo
kind: plate
schema: 1
title: 12 孔板布局（9 实验孔 + 3 PBS 空白孔）
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
id: calc-demo
kind: calculator
schema: 1
title: 孔板预混液配制
mode: formula
rows:
  - key: N
    label: 有效实验孔数
    kind: input
    value: "@plate.sample_wells"
    unit: 孔
  - key: V_0
    label: 单孔加样体积
    kind: input
    value: 200
    unit: uL
  - key: r
    label: 富余比例 (15% = 0.15)
    kind: input
    value: 0.15
    unit: ""
  - key: V_tot
    label: 预混液总体积
    kind: formula
    formula: 'N \times V_{0} \times (1 + r)'
    unit: uL
` + "```" + `

## 1.2 实验步骤

#@ 免疫荧光染色与复染
- 室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min
- 对全部实验孔每孔采集 6 个非重叠荧光视野
`
	relPath := filepath.Join("notes", "2026", "11", "scigraph-demo.asterml")
	fullPath := filepath.Join(vRoot, relPath)
	_ = os.MkdirAll(filepath.Dir(fullPath), 0o755)
	if err := os.WriteFile(fullPath, []byte(demoDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "SciGraph 引擎演示", shell.ModeEdit, true)
	sh.RightOpen = false

	// 1. Capture Calculator Formula Library + @plate.sample_wells (9 experimental wells -> 2070 uL)
	renderVisualPNGSteps(t, outDir, "scigraph_calc_formula_library.png", 1280, 1100, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if err := st.Click("打开公式库"); err != nil {
			t.Fatalf("failed to open Formula Library menu: %v", err)
		}
		st.Frame()
		if !st.HasText("2070") || !st.HasText("连续倍比稀释") {
			t.Fatalf("expected 2070 uL and Formula Library menu items visible")
		}
	})

	// 2. Capture Multi-Action Alt+Hover Preview Card right in viewport ("室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min")
	stepFocusDoc := `---
asterml: "2.1"
title: SciGraph 多动作分路与上下文继承演示
---

## 1.2 实验步骤

#@ 免疫荧光染色与复染
- 室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min
- 4% PFA 室温固定 15 min，然后 0.2% Triton X-100 透膜 10 min
`
	relStep := filepath.Join("notes", "2026", "11", "scigraph-multi.asterml")
	fullStep := filepath.Join(vRoot, relStep)
	if err := os.WriteFile(fullStep, []byte(stepFocusDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	sh.OpenFileInPane(sh.ActivePaneID, relStep, "多动作分路演示", shell.ModeEdit, true)
	a.altHoverPreviewQuery = "室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min"
	renderVisualPNGSteps(t, outDir, "scigraph_multi_action_alt_hover.png", 1280, 760, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Frame()
		if !st.HasText("检测到 2 个并行动作") || !st.HasText("全部插入 (2)") {
			t.Fatalf("expected multi-action tabbed popover with 检测到 2 个并行动作 & 全部插入 (2)")
		}
	})
}
