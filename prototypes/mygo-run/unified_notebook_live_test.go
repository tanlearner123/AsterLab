package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/astercmd"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/vault"
)

func makeTinyTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 240, 140))
	for y := 0; y < 140; y++ {
		for x := 0; x < 240; x++ {
			img.Set(x, y, color.RGBA{R: uint8(35 + x/2), G: uint8(95 + y), B: 175, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestUnifiedLivePreviewLabModeAndInlineRelations(t *testing.T) {
	outDir := os.Getenv("ASTERLAB_VISUAL_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vRoot := t.TempDir()
	v, err := vault.Open(vRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SeedTemplates(defaultTemplateSeeds()); err != nil {
		t.Fatal(err)
	}

	// 1. Verify blank template is genuinely blank (no pre-baked step-01 / step-02)
	blankDoc, err := asterdoc.Parse(releaseBlankTemplateDoc)
	if err != nil {
		t.Fatalf("releaseBlankTemplateDoc failed Parse: %v", err)
	}
	if len(blankDoc.Modules) != 0 {
		t.Fatalf("expected truly blank white-paper template with 0 pre-baked modules, got %d", len(blankDoc.Modules))
	}

	// Write a blank note into the vault for white-paper visual capture
	blankRel := "空白笔记.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, blankRel), []byte(releaseBlankTemplateDoc)); err != nil {
		t.Fatal(err)
	}

	// 2. Build a HUVEC notebook via the unified Semantic Command Bus (astercmd)
	assetRel, err := v.ImportAssetBytes(makeTinyTestPNG(t), "huvec_a1_6h_fov1.png")
	if err != nil {
		t.Fatal(err)
	}
	startSrc := "---\ntitle: HUVEC 成管实验记录\ndate: 2026-11-21\nstatus: in_progress\n---\n\n## 实验目的\n评估 **VEGF 50 ng/mL** 对 HUVEC 体外成管的影响，稀释比 1/1000。\n- [x] 基质胶 37 °C 预聚 30 min\n- [ ] 6h 显微成像记录\n"
	res := astercmd.ExecuteBatch(startSrc, astercmd.BatchRequest{
		Operations: []astercmd.Operation{
			{CommandID: "group.create", ClientRef: "g_ctrl", Args: map[string]any{"name": "对照组", "role": "control"}},
			{CommandID: "group.create", ClientRef: "g_vegf", Args: map[string]any{"name": "VEGF 处理组", "role": "treatment"}},
			{CommandID: "plate.create", ClientRef: "p12", Args: map[string]any{"title": "12 孔成管板", "format": 12}},
			{CommandID: "plate.assign_wells", Args: map[string]any{"plate_id": "p12", "group_id": "g_ctrl", "wells": []string{"A1", "A2"}}},
			{CommandID: "plate.assign_wells", Args: map[string]any{"plate_id": "p12", "group_id": "g_vegf", "wells": []string{"B1", "B2"}}},
			{CommandID: "asset.embed", Args: map[string]any{"asset": assetRel, "targets": []string{"@p12:A1"}, "timepoint": "6h", "fov": 1, "role": "obs"}},
			{CommandID: "step.create", Args: map[string]any{"title": "6h 显微成像采集", "preset": "capture"}},
		},
	})
	if !res.OK {
		t.Fatalf("ExecuteBatch failed: %v", res.Error)
	}
	relPath := "HUVEC/huvec-live.asterml"
	absPath := filepath.Join(vRoot, filepath.FromSlash(relPath))
	if err := asterdoc.WriteFileAtomic(absPath, []byte(res.NextSource)); err != nil {
		t.Fatal(err)
	}

	// Also create a focused inline-relation showcase doc so the embed card, chips, plate, and Lab blank area fit cleanly in one viewport
	compactSrc := "---\ntitle: 显微图像关联与实验模式\ndate: 2026-11-21\nstatus: in_progress\n---\n\n## 现场显微采图\n观察 **A1 对照孔** 6h 管腔形成，直接用极简行内语法关联孔位：\n"
	resCompact := astercmd.ExecuteBatch(compactSrc, astercmd.BatchRequest{
		Operations: []astercmd.Operation{
			{CommandID: "group.create", ClientRef: "g_ctrl", Args: map[string]any{"name": "对照组", "role": "control"}},
			{CommandID: "plate.create", ClientRef: "p12", Args: map[string]any{"title": "12 孔成管板", "format": 12}},
			{CommandID: "plate.assign_wells", Args: map[string]any{"plate_id": "p12", "group_id": "g_ctrl", "wells": []string{"A1", "A2"}}},
			{CommandID: "asset.embed", Args: map[string]any{"asset": assetRel, "targets": []string{"@p12:A1"}, "timepoint": "6h", "fov": 1, "role": "obs", "at_offset": len(compactSrc)}},
		},
	})
	if !resCompact.OK {
		t.Fatalf("compact ExecuteBatch failed: %v", resCompact.Error)
	}
	compactRel := "HUVEC/huvec-embed.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(compactRel)), []byte(resCompact.NextSource)); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-21", SelectedDate: "2026-11-21"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()

	// 3. Capture Blank White-Paper Document (with centered top floating pure-icon toolbar, zero bottom buttons)
	sh.OpenFileInPane(sh.ActivePaneID, blankRel, "空白实验", shell.ModeEdit, true)
	renderVisualPNGSteps(t, outDir, "unified_blank_whitepaper_light.png", 1220, 760, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if st.HasText("+ 步骤预设 ▾") || st.HasText("+ 空白步骤") {
			t.Fatalf("legacy bottom text button bar must be removed")
		}
		if _, ok := st.Find("插入科学组件 / 步骤 / 图片 (/)"); !ok {
			t.Fatalf("expected single '+' component inserter icon button on top floating toolbar")
		}
	})

	// 4. Capture Edit Mode HUVEC Live Preview (Light + Dark)
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "HUVEC 成管实验记录", shell.ModeEdit, true)
	renderVisualPNGSteps(t, outDir, "unified_live_preview_huvec_light.png", 1220, 820, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if st.HasText("+ 步骤预设 ▾") {
			t.Fatalf("legacy bottom text button bar must be removed in Edit mode")
		}
		if !st.HasText("FOV 1") {
			t.Fatalf("expected inline relation badge FOV 1 on embed card")
		}
	})
	renderVisualPNGSteps(t, outDir, "unified_live_preview_huvec_dark.png", 1220, 820, true, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
	})

	// 5. Capture Compact Inline Relation Embed Card + Chips + Plate (Edit & Lab Mode)
	sh.OpenFileInPane(sh.ActivePaneID, compactRel, "显微图像关联与实验模式", shell.ModeEdit, true)
	renderVisualPNGSteps(t, outDir, "unified_embed_relation_card_light.png", 1220, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("FOV 1") || !st.HasText("+ 关联") {
			t.Fatalf("expected inline embed card with FOV 1 and + 关联 button")
		}
	})

	sh.SetActiveTabMode(shell.ModeLab)
	renderVisualPNGSteps(t, outDir, "unified_lab_mode_huvec_light.png", 1220, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("实验模式 · 防误触") {
			t.Fatalf("expected Lab Mode badge in document header")
		}
		if !st.HasText("实验空白补记区（原文已锁防误触，此处可自由记录与贴图关联）") {
			t.Fatalf("expected Lab Mode blank note area")
		}
		// Top floating toolbar stays rendered in-place in Lab Mode (locked/disabled, zero layout shift)
		if _, ok := st.Find("插入科学组件 / 步骤 / 图片 (/)"); !ok {
			t.Fatalf("expected top floating toolbar to remain rendered in-place (locked) in Lab Mode")
		}
	})

	// 6. Capture orthogonal `#@` / `##@` lightweight nested steps + Unified Document Outline (`全部 / 仅步骤`)
	stepDocSrc := "---\nasterml: \"2.1\"\ntitle: HUVEC 成管方案（轻量化步骤）\nscheduled_date: \"2026-11-21\"\nstage: in_progress\n---\n\n## 1.1 实验目的\n评估 GelMA 浓度梯度对 HUVEC 体外成管的影响。\n\n## 1.2 实验步骤\n\n#@ GelMA 配液与交联\n##@ 热循环溶解\n- 称量 3% / 5% / 7% GelMA 冻干粉\n- 80°C 水浴 5 min 与冰上 10 min 交替 3 轮至澄清\n\n##@ 405 nm 光交联\n1. 每孔加入 200 μL 前体液\n2. 405 nm 蓝光照射 120 s 至完全固化\n\n#@ 细胞接种与成管培养\n- 4°C PBS 浸泡清洗 ≥ 2 h\n- 接种 5×10⁴ HUVEC / 孔，分别于 6h / 12h / 24h 成像\n\n---\n补充说明：本批次 GelMA 溶胶时间略快于上一批。\n\n## 2. 实验讨论\n- [ ] 复核 VEGF 批次活性\n"
	stepRel := "HUVEC/huvec-heading-steps.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(stepRel)), []byte(stepDocSrc)); err != nil {
		t.Fatal(err)
	}
	sh.OpenFileInPane(sh.ActivePaneID, stepRel, "HUVEC 成管方案（轻量化步骤）", shell.ModeEdit, true)
	sh.RightOpen = true
	sh.RightTab = 0
	renderVisualPNGSteps(t, outDir, "unified_heading_steps_light.png", 1360, 820, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("STEP 1") {
			t.Fatalf("expected Scheme B STEP 1 stage header pill for #@ step")
		}
		if !st.HasText("1.1") || !st.HasText("1.2") {
			t.Fatalf("expected B2 sub-step pills 1.1 / 1.2 for ##@ substeps")
		}
		if !st.HasText("全部") || !st.HasText("仅步骤") {
			t.Fatalf("expected unified outline filter pills 全部 / 仅步骤")
		}
	})
}

func TestFloatingToolbarSlashSingleEscAndInlineEmbedTyping(t *testing.T) {
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

	// Create a doc containing a 12-well plate so inline relation resolves cleanly
	docSrc := "---\nasterml: \"2.1\"\ntitle: 终验回归测试\n---\n\n```asterlab\nid: plate-1\nkind: plate\nschema: 1\ntitle: 12 孔成管板\nformat: 12\ngroups: {}\n```\n\n"
	relPath := "final-regression.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, relPath), []byte(docSrc)); err != nil {
		t.Fatal(err)
	}
	if _, err := asterdoc.Parse(docSrc); err != nil {
		t.Fatalf("fixture parse failed: %v", err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-21", SelectedDate: "2026-11-21"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "终验回归测试", shell.ModeEdit, true)

	tt := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1280, 860)
	tt.Frame()

	// Gate 2: Click top floating toolbar '+' button -> Slash menu must open & stay open across frames -> 1 single Esc closes it & cleans '/'
	if err := tt.Click("插入科学组件 / 步骤 / 图片 (/)"); err != nil {
		t.Fatalf("click floating toolbar '+': %v", err)
	}
	tt.Frame()
	tt.Frame()
	if !tt.HasText("插入科研组件与步骤") {
		t.Fatalf("expected Slash menu to stay open after clicking floating toolbar '+'")
	}
	tt.Key(0, ui.KeyEscape)
	tt.Frame()
	if tt.HasText("插入科研组件与步骤") {
		t.Fatalf("expected 1 single Esc to close Slash menu immediately")
	}
	ld, err := a.docStore.Open(relPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(strings.TrimSpace(ld.Source), "/") {
		t.Fatalf("expected lone '/' to be stripped after Esc, got:\n%s", ld.Source)
	}

	// Gate 6: Type full inline image + scientific relation syntax character-by-character + Enter
	targetLine := "![[" + imgRel + "]]{@plate-1:A1 6h fov=1 role=obs}"
	if err := tt.Click("科学笔记正文"); err != nil {
		t.Fatalf("click trailing prose editor: %v", err)
	}
	tt.Frame()
	for _, r := range targetLine {
		tt.Type(string(r))
		tt.Frame()
	}
	tt.Key(0, ui.KeyEnter)
	tt.Frame()
	tt.Frame()

	ld, err = a.docStore.Open(relPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ld.Source, targetLine) {
		t.Fatalf("expected full inline image relation line %q in source, got:\n%s", targetLine, ld.Source)
	}
	if !tt.HasText("12 孔成管板 · A1") || !tt.HasText("FOV 1") || !tt.HasText("观察图") {
		t.Fatalf("expected promoted Embed Card with resolved relation chips after pressing Enter")
	}
}

func TestUnifiedRichComplexHUVECShowcase(t *testing.T) {
	outDir := os.Getenv("ASTERLAB_VISUAL_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
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

	richSrc := `---
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
    junctions: 29
` + "```" + `

## 2. 实验讨论与异常复盘
- [ ] 对比 5% vs 3% GelMA 模量区间对内皮管腔稳定性的影响
`

	doc, err := asterdoc.Parse(richSrc)
	if err != nil {
		t.Fatalf("rich HUVEC doc failed Parse: %v", err)
	}
	tree := asterdoc.ParseStepTree(richSrc, doc)
	if len(tree.Roots) != 3 || len(tree.Flat) != 9 {
		t.Fatalf("expected 3 root steps and 9 total nested steps, got roots=%d flat=%d", len(tree.Roots), len(tree.Flat))
	}
	rels := asterdoc.ResolveRelations(doc)
	if len(rels) != 1 || len(rels[0].Contexts) != 2 || !rels[0].Contexts[0].Valid || !rels[0].Contexts[1].Valid {
		t.Fatalf("expected inline relation to resolve BOTH @plate-huvec:A1 and @obs-6h step anchor, got %+v", rels)
	}

	relPath := "HUVEC/huvec-full-protocol.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(relPath)), []byte(richSrc)); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-21", SelectedDate: "2026-11-21"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "HUVEC 体外成管实验", shell.ModeEdit, true)
	sh.RightOpen = true
	sh.RightTab = 0

	// Capture 4 vertical slices of the full complex protocol (Edit mode top/mid/lower + Lab Mode)
	renderVisualPNGSteps(t, outDir, "unified_rich_huvec_p1_design.png", 1380, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
	})
	renderVisualPNGSteps(t, outDir, "unified_rich_huvec_p2_calc_steps.png", 1380, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Scroll(650, 480, 0, 920)
		st.Frame()
	})
	renderVisualPNGSteps(t, outDir, "unified_rich_huvec_p2b_custom_latex.png", 1380, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Scroll(650, 480, 0, 1460)
		st.Frame()
	})
	renderVisualPNGSteps(t, outDir, "unified_rich_huvec_p3_timers_grid.png", 1380, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Scroll(650, 480, 0, 2120)
		st.Frame()
	})

	sh.SetActiveTabMode(shell.ModeLab)
	renderVisualPNGSteps(t, outDir, "unified_rich_huvec_p4_embed_lab.png", 1380, 920, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Scroll(650, 480, 0, 3050)
		st.Frame()
	})
}

func TestAltHoverNLToModulePreviewShowcase(t *testing.T) {
	outDir := os.Getenv("ASTERLAB_VISUAL_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vRoot := t.TempDir()
	v, err := vault.Open(vRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SeedTemplates(defaultTemplateSeeds()); err != nil {
		t.Fatal(err)
	}

	nlDocSrc := `---
asterml: "2.1"
title: HUVEC 成管方案（Alt 悬停自然语言转组件演示）
scheduled_date: "2026-11-22"
stage: in_progress
---

## 1.1 实验设计
- 12 孔板，3% / 5% / 7% GelMA 各 3 个技术复孔，第 4 列加无菌 PBS 作空白对照
- 将 100 ug/mL VEGF 母液稀释至 50 ng/mL，共配制 5 mL 工作液
- 将 VEGF 母液稀释至 50 ng/mL 工作液

## 1.2 实验步骤

#@ GelMA 前驱液溶解与交联
##@ 热循环溶解
- 80°C 水浴 5 min 与冰上 10 min 交替 3 轮至澄清
- 用无菌 PBS 轻柔清洗细胞 2 遍后吸弃上清
`
	relPath := "HUVEC/huvec-nl-hover.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(relPath)), []byte(nlDocSrc)); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "Alt 悬停转组件演示", shell.ModeEdit, true)
	sh.RightOpen = true
	sh.RightTab = 0

	// 1. Alt+Hover over cycle timer sentence -> mini visual cycle timeline diagram
	a.altHoverPreviewQuery = "80°C 水浴 5 min"
	renderVisualPNGSteps(t, outDir, "alt_hover_timer_cycle.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("循环计时器 · 3 轮") || !st.HasText("下方插入组件") {
			t.Fatalf("expected Alt+Hover cycle timer preview popover")
		}
	})

	// 2. Alt+Hover over dilution sentence -> mini 2D LaTeX equation V1 = (C2*V2)/C1 = 2.5 uL
	a.altHoverPreviewQuery = "100 ug/mL VEGF"
	renderVisualPNGSteps(t, outDir, "alt_hover_calc_dilution.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("稀释配液计算器") || !st.HasText("2.5 µL") {
			t.Fatalf("expected Alt+Hover dilution calculator preview popover with 2.5 µL")
		}
	})

	// 2b. Alt+Hover over partial-parameter dilution sentence -> Missing-Parameter Fallback UI (待补 + 缺参兜底横幅)
	a.altHoverPreviewQuery = "将 VEGF 母液稀释至 50 ng/mL"
	renderVisualPNGSteps(t, outDir, "alt_hover_missing_param_fallback.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("稀释配液计算器") || !st.HasText("待补") {
			t.Fatalf("expected Alt+Hover missing-parameter fallback popover with 待补 indicator")
		}
	})

	// 3. Alt+Hover over 12-well plate sentence -> mini colored 3x4 plate diagram + click to insert!
	a.altHoverPreviewQuery = "12 孔板，3%"
	renderVisualPNGSteps(t, outDir, "alt_hover_plate_group.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		if !st.HasText("12 孔板 + 4 组") {
			t.Fatalf("expected Alt+Hover 12-well plate + 4 groups preview popover")
		}
		if err := st.Click("下方插入组件"); err != nil {
			t.Fatalf("click 下方插入组件: %v", err)
		}
		st.Frame()
	})

	// 4. After clicking "下方插入组件" -> live group_table + 12-well plate rendered inline!
	a.altHoverPreviewQuery = ""
	renderVisualPNGSteps(t, outDir, "alt_hover_after_insert_plate.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
	})

	// Verify clicking "下方插入组件" actually persisted valid group_table + plate modules into source!
	ld, err := a.docStore.Open(relPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ld.Source, "kind: group_table") || !strings.Contains(ld.Source, "kind: plate") {
		t.Fatalf("expected group_table and plate modules inserted after clicking 下方插入组件, got:\n%s", ld.Source)
	}
}

// TestRealAltHoverSweepAndNonOcclusion tests the 100% REAL Alt key + Mouse Pointer sweep path
// (NO AltHoverPreviewQuery test hook!), verifying:
//  1. Holding real Alt + moving pointer onto a recognized sentence opens the preview card INSTANTLY
//     with the colorful left-to-right luminous sweep directly on that line.
//  2. The popover card is positioned strictly BELOW the glowing sentence (zero occlusion!).
//  3. Sweeping with Alt still held onto a hard negative ("用无菌 PBS 轻柔清洗细胞 2 遍后吸弃上清")
//     immediately suppresses/closes the preview card.
//  4. Releasing Alt and moving into the card keeps it open so clicking "下方插入组件" works cleanly.
func TestRealAltHoverSweepAndNonOcclusion(t *testing.T) {
	outDir := os.Getenv("ASTERLAB_VISUAL_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vRoot := t.TempDir()
	v, err := vault.Open(vRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err := v.SeedTemplates(defaultTemplateSeeds()); err != nil {
		t.Fatal(err)
	}
	nlDocSrc := `---
asterml: "2.1"
title: HUVEC 成管方案（真实 Alt 悬停实测）
scheduled_date: "2026-11-22"
stage: in_progress
---

## 1.1 实验设计
- 12 孔板，3% / 5% / 7% GelMA 各 3 个技术复孔，第 4 列加无菌 PBS 作空白对照
- 将 100 ug/mL VEGF 母液稀释至 50 ng/mL，共配制 5 mL 工作液
- 将 VEGF 母液稀释至 50 ng/mL 工作液

## 1.2 实验步骤

#@ GelMA 前驱液溶解与交联
##@ 热循环溶解
- 80°C 水浴 5 min 与冰上 10 min 交替 3 轮至澄清
- 用无菌 PBS 轻柔清洗细胞 2 遍后吸弃上清
`
	relPath := "HUVEC/huvec-real-alt.asterml"
	if err := asterdoc.WriteFileAtomic(filepath.Join(vRoot, filepath.FromSlash(relPath)), []byte(nlDocSrc)); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "真实 Alt 悬停实测", shell.ModeEdit, true)
	sh.RightOpen = true
	sh.RightTab = 0
	a.altHoverPreviewQuery = "" // STRICTLY ZERO FAKE HOOK — 100% real Alt + Mouse!

	// First render to locate exact real coordinates of our live editor segment
	renderVisualPNGSteps(t, outDir, "alt_real_hover_dilution_sweep.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		// Find the live-md editor containing "1.1 实验设计" (lines: 0=heading, 1=plate, 2=dilution, 3=partial dilution)
		box, ok := st.Find("科学笔记正文")
		if !ok {
			t.Fatalf("could not find live markdown editor bounds")
		}
		// Hover line 2 ("将 100 ug/mL VEGF 母液稀释至 50 ng/mL...") with REAL Alt held!
		targetX := box.X + 140
		targetY := box.Y + 68 // below H2 heading (~36px) + line 1 (~24px) -> middle of line 2
		st.Move(targetX, targetY)
		st.Frame()
		// Without Alt held, NO NL preview card should appear!
		if st.HasText("稀释配液计算器") {
			t.Fatalf("NL preview card must NOT open without Alt held")
		}
		// Now hold REAL Alt key WITHOUT moving the mouse -> must open INSTANTLY!
		st.CodeDown(ui.Alt, ui.CodeAltLeft)
		st.Frame()
		st.Advance(420 * time.Millisecond) // advance mid-sweep so colorful left->right shimmer is vivid!
		st.Frame()
		if !st.HasText("稀释配液计算器") || !st.HasText("2.5 µL") {
			t.Fatalf("expected REAL Alt+Hover to open dilution preview card immediately at (%.0f, %.0f) inside box (%.0f, %.0f, %.0fx%.0f)",
				targetX, targetY, box.X, box.Y, box.W, box.H)
		}
		// Mathematically verify ZERO occlusion: popover card (button & title) must be strictly BELOW targetY!
		insRect, okCard := st.Find("下方插入组件")
		if !okCard {
			t.Fatalf("expected 下方插入组件 button in popover card")
		}
		if insRect.Y <= targetY+40 {
			t.Fatalf("POPOVER OCCLUDED HOVERED SENTENCE! targetY=%.1f, card footer Y=%.1f", targetY, insRect.Y)
		}
		t.Logf("REAL Alt+Hover NON-OCCLUSION VERIFIED: hovered line Y=%.1f, card footer Y=%.1f (strictly below!)",
			targetY, insRect.Y)
		// Dismiss Test 1's card cleanly before starting independent Test 2 on the same app
		st.Key(0, ui.KeyEscape)
		st.Frame()
	})

	// Capture REAL Alt+Hover over partial-parameter sentence + verify zero occlusion + sweep to hard negative + release Alt & click insert!
	renderVisualPNGSteps(t, outDir, "alt_real_hover_missing_fallback_sweep.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		box, ok := st.Find("科学笔记正文")
		if !ok {
			t.Fatalf("could not find live markdown editor bounds")
		}
		// Hover line 3 ("将 VEGF 母液稀释至 50 ng/mL 工作液" -> missing C1 & V2!) then hold real Alt!
		line3Y := box.Y + 88
		st.Move(box.X+140, line3Y)
		st.Frame()
		st.CodeDown(ui.Alt, ui.CodeAltLeft)
		st.Frame()
		st.Advance(550 * time.Millisecond)
		st.Frame()
		if !st.HasText("缺参兜底") || !st.HasText("待补") {
			t.Fatalf("expected REAL Alt+Hover missing-parameter fallback card")
		}
		insRect, okCard := st.Find("下方插入组件")
		if !okCard || insRect.Y <= line3Y+40 {
			t.Fatalf("fallback card occluded sentence: line3Y=%.1f cardFooterY=%.1f", line3Y, insRect.Y)
		}
		// Dismiss Test 2's card cleanly before Test 3
		st.Key(0, ui.KeyEscape)
		st.Frame()
	})

	// Test 3: Real Alt on line 1 (12-well plate) -> release Alt -> move mouse into card -> click "+ 下方插入组件"!
	renderVisualPNGSteps(t, outDir, "alt_real_hover_plate_insert.png", 1360, 860, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		box, ok := st.Find("科学笔记正文")
		if !ok {
			t.Fatalf("could not find live markdown editor bounds")
		}
		// Hover line 1 (12-well plate) and hold real Alt -> opens plate card
		st.Move(box.X+140, box.Y+44)
		st.Frame()
		st.CodeDown(ui.Alt, ui.CodeAltLeft)
		st.Frame()
		if !st.HasText("12 孔板 + 4 组") {
			t.Fatalf("expected real Alt+Hover on line 1 to open 12-well plate preview")
		}
		// 1. Hold Alt steadily across 10 shimmer frames -> card must stay rock-solid (zero flicker)
		for i := 0; i < 10; i++ {
			st.Advance(60 * time.Millisecond)
			st.Frame()
			if !st.HasText("12 孔板 + 4 组") {
				t.Fatalf("card flickered/closed on frame %d while Alt was still held!", i)
			}
		}
		// 2. Move pointer DOWN onto the card button WHILE STILL HOLDING ALT -> must stay rock-solid past 1500ms!
		insRect, okBtn := st.Find("下方插入组件")
		if !okBtn {
			t.Fatalf("expected 下方插入组件 button in plate preview card")
		}
		st.Move(insRect.X+insRect.W/2, insRect.Y+insRect.H/2)
		for i := 0; i < 12; i++ {
			st.Advance(130 * time.Millisecond)
			st.Frame()
			if !st.HasText("12 孔板 + 4 组") {
				t.Fatalf("card closed on frame %d while Alt held & pointer moved onto card!", i)
			}
		}
		// 3. Release Alt WHILE pointer remains over the card -> must stay rock-solid well past 900ms grace (1500ms+)!
		st.CodeUp(0, ui.CodeAltLeft)
		for i := 0; i < 12; i++ {
			st.Advance(130 * time.Millisecond)
			st.Frame()
			if !st.HasText("12 孔板 + 4 组") {
				t.Fatalf("card closed on frame %d after releasing Alt while pointer stayed on card!", i)
			}
		}
		// 4. Click "+ 下方插入组件" cleanly with real pointer!
		if err := st.Click("下方插入组件"); err != nil {
			t.Fatalf("real Alt+Hover click 下方插入组件 failed: %v", err)
		}
		st.Frame()
	})
	ld, err := a.docStore.Open(relPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ld.Source, "kind: plate") || !strings.Contains(ld.Source, "kind: group_table") {
		t.Fatalf("expected real Alt+Hover click to insert plate + group_table, got:\n%s", ld.Source)
	}
}
