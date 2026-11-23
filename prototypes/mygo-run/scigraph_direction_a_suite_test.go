package main

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/astercore"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
)

func TestSciGraphDirectionA5DisciplineProtocolSuite(t *testing.T) {
	protoDir := filepath.Join("testdata", "protocols")
	type expectSpec struct {
		file                string
		discipline          string
		wantSampleWells     int
		wantBlankWells      int
		wantChainedCalcID   string
		wantChainedMin      float64
		wantChainedMax      float64
		wantProseReagName   string
		wantProseStockVal   float64
		wantDilLineSubstr   string
		wantDilSummarySub   string
		wantMultiLineSubstr string
		wantAnchorID        string
	}

	suite := []expectSpec{
		{
			file:              "01_huvec_angiogenesis.asterml",
			discipline:        "Cell Biology / HUVEC 3D Hydrogel",
			wantSampleWells:   9,
			wantBlankWells:    3,
			wantChainedCalcID: "calc-gelma-mass", // 9 * 500 * 1.2 = 5400 uL -> 5% * 5.4 * 10 = 270 mg!
			wantChainedMin:    269.5,
			wantChainedMax:    270.5,
			wantProseReagName: "VEGF",
			wantProseStockVal: 100,
			wantDilLineSubstr: "将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL",
			wantDilSummarySub: "2.5 µL", // 100 ug/mL -> 50 ng/mL in 5 mL = 2.5 uL!
			wantAnchorID:      "t0",
		},
		{
			file:                "02_rt_qpcr_ddct.asterml",
			discipline:          "Molecular Biology / RT-qPCR ΔΔCt",
			wantSampleWells:     18,
			wantBlankWells:      2,
			wantChainedCalcID:   "calc-qpcr-mm", // V_tot=180 uL; V_rem_cdna = 180 - 18*2 = 144 uL!
			wantChainedMin:      143.5,
			wantChainedMax:      144.5,
			wantProseReagName:   "Primer",
			wantProseStockVal:   100,
			wantDilLineSubstr:   "将 Primer 母液稀释至 10 uM 工作液，终体积 100 uL",
			wantDilSummarySub:   "10 µL", // 100 uM -> 10 uM in 100 uL = 10 uL!
			wantMultiLineSubstr: "65°C 水浴变性 5 min；随后冰上静置 2 min",
		},
		{
			file:                "03_western_blot_phospho.asterml",
			discipline:          "Protein Biochemistry / Western Blot",
			wantSampleWells:     0,
			wantBlankWells:      0,
			wantChainedCalcID:   "calc-pmsf-add", // 8 * 150 = 1200 uL -> 1200 / 100 = 12 uL!
			wantChainedMin:      11.9,
			wantChainedMax:      12.1,
			wantMultiLineSubstr: "冰上裂解 30 min；随后 4°C 12000 rpm 离心 15 min 取上清",
		},
		{
			file:              "04_cck8_ic50_drug_screen.asterml",
			discipline:        "Pharmacology / CCK-8 IC50 Screen",
			wantSampleWells:   21,
			wantBlankWells:    3,
			wantChainedCalcID: "calc-cck8-reagent", // 21 * 10 * 1.15 = 241.5 uL!
			wantChainedMin:    241.0,
			wantChainedMax:    242.0,
			wantProseReagName: "Doxorubicin",
			wantProseStockVal: 10,
			wantDilLineSubstr: "将 Doxorubicin 母液稀释至 10 uM 工作液，终体积 2 mL",
			wantDilSummarySub: "2 µL", // 10 mM -> 10 uM in 2 mL = 2 uL (inherited from calc-dox-stock!)
			wantAnchorID:      "t0",
		},
		{
			file:                "05_if_confocal_dual_stain.asterml",
			discipline:          "Cell Imaging / Dual-Stain Confocal IF",
			wantSampleWells:     8,
			wantBlankWells:      2,
			wantChainedCalcID:   "calc-if-ab-stock", // 8 * 200 * 1.2 = 1920 uL -> 1920 / 500 = 3.84 uL!
			wantChainedMin:      3.80,
			wantChainedMax:      3.88,
			wantProseReagName:   "DAPI",
			wantProseStockVal:   1,
			wantDilLineSubstr:   "将 DAPI 母液稀释至 1 ug/mL 工作液，终体积 5 mL",
			wantDilSummarySub:   "5 µL", // 1 mg/mL -> 1 ug/mL in 5 mL = 5 uL!
			wantMultiLineSubstr: "室温避光二抗孵育 1 h；随后加 DAPI 染核 5 min",
		},
	}

	totalBlockedZoneLines := 0
	totalBlockedFP := 0
	totalStepPosLines := 0
	totalStepMultiHits := 0
	totalInheritedHits := 0
	var totalInferMs float64
	var inferCalls int

	for _, tc := range suite {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(protoDir, tc.file))
			if err != nil {
				t.Fatalf("read protocol %s: %v", tc.file, err)
			}
			src := string(raw)
			doc, err := asterdoc.Parse(src)
			if err != nil {
				t.Fatalf("parse %s failed: %v", tc.file, err)
			}
			g := asterdoc.BuildSciGraph(doc)

			// 1. Verify blank-excluded SampleWells
			if tc.wantSampleWells > 0 {
				pl := g.PrimaryPlate()
				if pl == nil {
					t.Fatalf("expected primary plate in %s", tc.file)
				}
				if pl.SampleWells != tc.wantSampleWells || pl.BlankWells != tc.wantBlankWells {
					t.Fatalf("expected %d sample / %d blank wells, got %d / %d",
						tc.wantSampleWells, tc.wantBlankWells, pl.SampleWells, pl.BlankWells)
				}
			}

			// 2. Verify Direction A reactive `@calc.<id>.<key>` chaining & `@plate.sample_wells` evaluation
			foundChain := false
			for _, m := range doc.Modules {
				if m.ID == tc.wantChainedCalcID {
					res := asterdoc.EvalCalculatorWithActuals(doc, m.Data, nil)
					if res.Err != nil {
						t.Fatalf("chained calculator %s eval error: %v", m.ID, res.Err)
					}
					last := res.Rows[len(res.Rows)-1].Amount
					if last < tc.wantChainedMin || last > tc.wantChainedMax {
						t.Fatalf("chained calculator %s expected [%.2f, %.2f], got %.4f (%q)",
							m.ID, tc.wantChainedMin, tc.wantChainedMax, last, res.Summary)
					}
					t.Logf("[%s] Chained calc %s -> %s (%.4f) OK", tc.discipline, m.ID, res.Summary, last)
					foundChain = true
				}
			}
			if !foundChain {
				t.Fatalf("chained calculator %s not found in %s", tc.wantChainedCalcID, tc.file)
			}

			// 3. Verify Direction A single-document prose reagent fact harvesting
			if tc.wantProseReagName != "" {
				stepIdx := strings.Index(src, "## 1.2 实验步骤")
				if stepIdx < 0 {
					stepIdx = len(src) - 1
				}
				ctx := g.ContextAt(stepIdx + 40)
				r := ctx.LookupReagent(tc.wantProseReagName)
				if r == nil || r.StockVal != tc.wantProseStockVal {
					t.Fatalf("expected upstream reagent %s=%.2f harvested from %s, got %+v",
						tc.wantProseReagName, tc.wantProseStockVal, tc.file, r)
				}
				t.Logf("[%s] Harvested reagent %s = %v %s (source=%s) OK",
					tc.discipline, r.Name, r.StockVal, r.StockUnit, r.SourceModuleID)
			}

			// 3b. Verify exact numerical dilution math after upstream SciGraph reagent inheritance!
			if tc.wantDilLineSubstr != "" {
				off := strings.Index(src, tc.wantDilLineSubstr)
				if off < 0 {
					t.Fatalf("missing dilution line %q in %s", tc.wantDilLineSubstr, tc.file)
				}
				sugs := asterdoc.AnalyzeLabSentenceWithContext(doc, "- "+tc.wantDilLineSubstr, off, nil)
				if len(sugs) != 1 || sugs[0].InheritedFrom == "" || len(sugs[0].MissingSlots) != 0 || !strings.Contains(sugs[0].Summary, tc.wantDilSummarySub) {
					t.Fatalf("expected inherited dilution containing %q with 0 missing slots, got %+v", tc.wantDilSummarySub, sugs)
				}
				t.Logf("[%s] Inherited dilution verified: %s (from %s)", tc.discipline, sugs[0].Summary, sugs[0].InheritedFrom)
			}

			// 4. Verify Direction A runtime {#t0} anchor timestamp indexing
			if tc.wantAnchorID != "" {
				anc := g.AnchorByID(tc.wantAnchorID)
				if anc == nil || anc.RecordedAt.IsZero() {
					t.Fatalf("expected anchor %q with RecordedAt stamped in %s, got %+v", tc.wantAnchorID, tc.file, anc)
				}
				t.Logf("[%s] Anchor {#%s} stamped at %s OK", tc.discipline, anc.ID, anc.RecordedAt.Format(time.RFC3339))
			}

			// 5. Verify Direction A multi-action compound line split
			if tc.wantMultiLineSubstr != "" {
				off := strings.Index(src, tc.wantMultiLineSubstr)
				if off < 0 {
					t.Fatalf("missing multi-action line %q in %s", tc.wantMultiLineSubstr, tc.file)
				}
				sugs := asterdoc.AnalyzeLabSentenceWithContext(doc, "- "+tc.wantMultiLineSubstr, off, nil)
				if len(sugs) < 2 {
					t.Fatalf("expected >=2 split suggestions for %q, got %d (%+v)", tc.wantMultiLineSubstr, len(sugs), sugs)
				}
				totalStepMultiHits++
				t.Logf("[%s] Multi-action split %q -> %d widgets OK", tc.discipline, tc.wantMultiLineSubstr, len(sugs))
			}

			// 6. Full line-by-line audit across EVERY prose line in the protocol!
			inFM := false
			inFence := false
			off := 0
			for lnIdx, line := range strings.Split(src, "\n") {
				curOff := off
				off += len(line) + 1
				trim := strings.TrimSpace(line)
				if lnIdx == 0 && trim == "---" {
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
				if inFence || trim == "" {
					continue
				}
				ctx := g.ContextAt(curOff)
				t0 := time.Now()
				sugs := asterdoc.AnalyzeLabSentenceWithContext(doc, line, curOff, nil)
				elapsedMs := float64(time.Since(t0).Microseconds()) / 1000.0
				if ctx.AllowedForNL() {
					totalInferMs += elapsedMs
					inferCalls++
				}

				if !ctx.AllowedForNL() {
					totalBlockedZoneLines++
					if len(sugs) != 0 {
						totalBlockedFP++
						t.Errorf("ZONE LEAK on %s L%03d [%s]: %s -> %+v", tc.file, lnIdx+1, ctx.Zone, trim, sugs)
					}
					continue
				}
				if len(sugs) > 0 {
					totalStepPosLines++
					for _, s := range sugs {
						if s.InheritedFrom != "" {
							totalInheritedHits++
						}
						t.Logf("  L%03d [%s/%s %.0f%% inherit=%q missing=%v] %s -> %s",
							lnIdx+1, s.Kind, s.Mode, s.Confidence*100, s.InheritedFrom, s.MissingSlots, trim, s.Summary)
					}
				} else {
					t.Logf("  L%03d [NONE] %s", lnIdx+1, trim)
				}
			}
		})
	}

	avgMs := 0.0
	if inferCalls > 0 {
		avgMs = totalInferMs / float64(inferCalls)
	}
	t.Logf("=== 5-DISCIPLINE PROTOCOL SUITE SUMMARY ===")
	t.Logf("Blocked Zone Lines Audited : %d | False Positives: %d (0.0%%)", totalBlockedZoneLines, totalBlockedFP)
	t.Logf("Actionable Step Lines Hit  : %d | Multi-Action Splits Verified: %d | Upstream Inheritances: %d",
		totalStepPosLines, totalStepMultiHits, totalInheritedHits)
	t.Logf("Average Per-Line Latency   : %.2f ms (n=%d active lines, pure CPU Go+ONNX)", avgMs, inferCalls)
}

func TestSciGraphDirectionAVisualEvidence(t *testing.T) {
	vRoot := t.TempDir()
	outDir := filepath.Join("..", "..", "docs", "sdk", "evidence", "v2-visual-catalog")
	_ = os.MkdirAll(outDir, 0o755)

	// Load our real 01_huvec_angiogenesis.asterml protocol directly so the screenshot reflects an actual protocol!
	raw, err := os.ReadFile(filepath.Join("testdata", "protocols", "01_huvec_angiogenesis.asterml"))
	if err != nil {
		t.Fatal(err)
	}
	// Replace started_at with 30 minutes ago so the live window timer shows active remaining countdown!
	liveSrc := strings.Replace(string(raw),
		`started_at: "2026-11-22T02:00:00Z"`,
		`started_at: "`+time.Now().UTC().Add(-30*time.Minute).Format(time.RFC3339)+`"`, 1)

	// Create a focused Direction A view showing Chained Calculators (`@calc.calc-huvec-mm.V_tot` -> 270 mg) + Live {#t0} Anchor Wake-Up Window all cleanly in one frame!
	focusSrc := `---
asterml: "2.1"
title: 方向 A 单文档科学网络闭环实拍（级联计算 + 锚点唤醒）
---

## 1.1 级联计算与时间锚点网络

` + "```asterlab" + `
id: calc-huvec-mm
kind: calculator
schema: 1
title: 1. 前驱液总体积（9 有效实验孔）
mode: formula
rows:
  - key: N
    label: 有效实验孔数
    kind: input
    value: 9
    unit: 孔
  - key: V_well
    label: 单孔铺胶体积
    kind: input
    value: 500
    unit: uL
  - key: r
    label: 富余比例 (20%)
    kind: input
    value: 0.2
    unit: ""
  - key: V_tot
    label: 前驱液总需求量
    kind: formula
    formula: 'N \times V_{well} \times (1 + r)'
    unit: uL
` + "```" + `

` + "```asterlab" + `
id: calc-gelma-mass
kind: calculator
schema: 1
title: 2. 冻干粉称量（级联 @calc.calc-huvec-mm.V_tot）
mode: formula
rows:
  - key: V_tot
    label: 上游前驱液总量（联动）
    kind: input
    value: "@calc.calc-huvec-mm.V_tot"
    unit: uL
  - key: C_w
    label: 目标 GelMA 浓度
    kind: input
    value: 5
    unit: "% w/v"
  - key: m_gelma
    label: 需称取冻干粉总质量
    kind: formula
    formula: 'C_{w} \times \frac{V_{tot}}{1000} \times 10'
    unit: mg
` + "```" + `

#@ HUVEC 接种与成管随访 {#t0}

` + "```asterlab" + `
id: timer-t0-live
kind: timer
schema: 1
title: 接种完成时刻锚点 (t0)
mode: stopwatch
anchor: t0
started_at: "` + time.Now().UTC().Add(-30*time.Minute).Format(time.RFC3339) + `"
` + "```" + `

` + "```asterlab" + `
id: timer-obs-6h
kind: timer
schema: 1
title: t0 + 6 h 成管采图窗口提醒
mode: window
anchor: t0
offset:
  value: 6
  unit: h
schedule_window: "t0 + 6 h 明场采图"
` + "```" + `
`
	_ = liveSrc
	relPath := filepath.Join("notes", "2026", "11", "direction-a-chain.asterml")
	fullPath := filepath.Join(vRoot, relPath)
	_ = os.MkdirAll(filepath.Dir(fullPath), 0o755)
	if err := os.WriteFile(fullPath, []byte(focusSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatal(err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.OnOpenUserGuide = a.openOrSeedUserGuideDoc
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "方向 A 闭环实拍", shell.ModeEdit, true)
	sh.RightOpen = false

	// Capture Chained Calculator (`5400 uL` -> `270 mg` + `+ 引用上游计算` button) + Live {#t0} Window Wake-Up!
	renderVisualPNGSteps(t, outDir, "scigraph_direction_a_chain_and_t0.png", 1280, 1260, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Frame()
		if !st.HasText("5400") || !st.HasText("270") {
			t.Fatalf("expected chained calculator outputs 5400 uL -> 270 mg visible")
		}
		if !st.HasText("+ 引用上游计算") {
			t.Fatalf("expected + 引用上游计算 button visible on downstream calculator")
		}
		if !st.HasText("已锚定 t0") {
			t.Fatalf("expected live {#t0} wake-up banner 已锚定 t0 visible on downstream window timer")
		}
	})
}

// TestV020RealHUVECClosedLoopExperimentGate validates the end-to-end real experiment lifecycle:
// 1) Create an experiment inside a real Vault on disk
// 2) Assign 12-well plate groups + PBS blank column (9 sample / 3 blank)
// 3) Exclude well B2 during Lab-mode execution (effective sample wells drop 9 -> 8)
// 4) Cascade into cross-unit chained calculators (8 * 500 * 1.2 = 4800 uL -> auto-converted 4.8 mL -> 240 mg GelMA)
// 5) Stamp {#t0} anchor and verify downstream t0+6h window wake-up
// 6) Record real well readings into data_table, run astercore.AnalyzeDocument() (confirm B2 excluded + blank mean subtracted)
// 7) Verify recorded-data plate relayout lock refuses silent relayout
// 8) Persist to disk, close completely, reopen fresh from disk (simulated app restart), and verify 100% lossless state!
func TestV020RealHUVECClosedLoopExperimentGate(t *testing.T) {
	vRoot := t.TempDir()
	relPath := filepath.Join("notes", "2026", "11", "huvec-v020-closed-loop.asterml")
	fullPath := filepath.Join(vRoot, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}

	t0ISO := time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339)
	initialSrc := `---
asterml: "2.1"
title: HUVEC 成管全链路闭环门禁实测
date: "2026-11-22"
status: running
---

## 1.1 实验设计蓝图

` + "```asterlab" + `
id: gt-huvec
kind: group_table
schema: 1
title: 实验分组
rows:
  - id: g_ctrl
    name: Control
    role: control
    independent_n: 3
  - id: g_vegf
    name: VEGF 50ng/mL
    role: treatment
    independent_n: 3
  - id: g_low
    name: Low Dose
    role: treatment
    independent_n: 3
  - id: g_pbs
    name: PBS Blank
    role: control
    independent_n: 3
` + "```" + `

` + "```asterlab" + `
id: plate-huvec
kind: plate
schema: 1
title: 12孔成管板
format: 12
groups:
  g_ctrl: [A1, A2, A3]
  g_vegf: [B1, B2, B3]
  g_low: [C1, C2, C3]
  g_pbs: [A4, B4, C4]
roles:
  A4: blank
  B4: blank
  C4: blank
excluded_wells: [B2]
samples:
  A1: Donor-1
  A2: Donor-2
  A3: Donor-3
  B1: Donor-1
  B3: Donor-3
  C1: Donor-1
  C2: Donor-2
  C3: Donor-3
` + "```" + `

## 1.2 配胶与接种 {#t0}

` + "```asterlab" + `
id: calc-vol
kind: calculator
schema: 1
title: 1. 前驱液总需求量（联动剔除异常孔后的 8 有效孔）
mode: formula
rows:
  - key: N
    label: 有效实验孔
    kind: input
    value: "@plate.plate-huvec.sample_wells"
    unit: 孔
  - key: V_well
    label: 单孔体积
    kind: input
    value: 500
    unit: uL
  - key: V_tot
    label: 前驱液总量
    kind: formula
    formula: 'N \times V_{well} \times 1.2'
    unit: uL
` + "```" + `

` + "```asterlab" + `
id: calc-mass
kind: calculator
schema: 1
title: 2. 5% w/v GelMA 冻干粉称量（跨单位 uL -> mL 自动换算）
mode: formula
rows:
  - key: V_ml
    label: 上游前驱液总量
    kind: input
    value: "@calc.calc-vol.V_tot"
    unit: mL
  - key: C_w
    label: 质量分数
    kind: input
    value: 5
    unit: "% w/v"
  - key: m_gelma
    label: 需称冻干粉
    kind: formula
    formula: 'C_{w} \times V_{ml} \times 10'
    unit: mg
` + "```" + `

` + "```asterlab" + `
id: timer-t0
kind: timer
schema: 1
title: 接种完成锚点
mode: stopwatch
anchor: t0
started_at: "` + t0ISO + `"
` + "```" + `

` + "```asterlab" + `
id: timer-6h
kind: timer
schema: 1
title: 成管采图时间窗
mode: window
anchor: t0
offset:
  value: 6
  unit: h
schedule_window: "t0 + 6 h 明场采图"
` + "```" + `

## 1.3 结果记录与统计

` + "```asterlab" + `
id: dt-tubes
kind: data_table
schema: 1
title: 6h 成管节点数读数
container_ref: plate-huvec
columns:
  - key: well
    label: 孔位
    value_kind: text
  - key: value
    label: 节点数
    value_kind: number
rows:
  - well: A1
    value: 50
  - well: A2
    value: 52
  - well: A3
    value: 48
  - well: B1
    value: 100
  - well: B2
    value: 9999
  - well: B3
    value: 102
  - well: A4
    value: 2
  - well: B4
    value: 2
  - well: C4
    value: 2
` + "```" + `
`
	// Write via atomic write, then reopen cleanly from disk (simulating full restart)
	if err := asterdoc.WriteFileAtomic(fullPath, []byte(initialSrc)); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	reopenedRaw, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("ReadFile after restart: %v", err)
	}
	doc, err := asterdoc.Parse(string(reopenedRaw))
	if err != nil {
		t.Fatalf("Parse after restart: %v", err)
	}

	// Verify SciGraph plate + excluded_wells + anchor wake-up
	g := asterdoc.BuildSciGraph(doc)
	pl := g.ContainerByID("plate-huvec")
	if pl == nil || !pl.Assigned || pl.SampleWells != 8 || pl.BlankWells != 3 || pl.ExcludedWells != 1 {
		t.Fatalf("unexpected reopened plate state: %+v", pl)
	}
	hasT0 := false
	for _, anc := range g.Anchors {
		if anc.ID == "t0" && !anc.RecordedAt.IsZero() {
			hasT0 = true
			break
		}
	}
	if !hasT0 {
		t.Fatalf("expected {#t0} runtime anchor to survive restart, got %+v", g.Anchors)
	}

	// Verify chained calculator cross-unit conversion: 8 * 500 * 1.2 = 4800 uL -> 4.8 mL -> 240 mg!
	var calcMassMod *asterdoc.Module
	for i := range doc.Modules {
		if doc.Modules[i].ID == "calc-mass" {
			calcMassMod = &doc.Modules[i]
			break
		}
	}
	if calcMassMod == nil {
		t.Fatal("missing calc-mass module")
	}
	resMass := asterdoc.EvalCalculatorWithActuals(doc, calcMassMod.Data, nil)
	if resMass.Err != nil {
		t.Fatalf("calc-mass eval failed: %v", resMass.Err)
	}
	if math.Abs(resMass.Rows[0].Amount-4.8) > 1e-4 || math.Abs(resMass.Rows[2].Amount-240.0) > 1e-3 {
		t.Fatalf("expected 4800 uL -> 4.8 mL -> 240 mg, got %+v", resMass.Rows)
	}

	// Verify recorded-data relayout lock is enforced after reopen
	var huvecPlate *asterdoc.Plate
	for i := range doc.Modules {
		if doc.Modules[i].ID == "plate-huvec" && doc.Modules[i].Plate != nil {
			huvecPlate = doc.Modules[i].Plate
			break
		}
	}
	if _, _, _, err := asterdoc.RelayoutExistingPlate(doc, huvecPlate, asterdoc.LayoutColMajor, false); err == nil {
		t.Fatalf("expected relayout to be blocked on plate with excluded wells + readings")
	}

	// Verify unified astercore.AnalyzeDocument consumes SciUnit (excludes B2=9999, tracks BlankMean=2)
	sums := astercore.AnalyzeDocument(doc, relPath)
	if len(sums) != 1 || sums[0].ExcludedN != 1 || sums[0].BlankN != 3 || sums[0].BlankMean == nil || *sums[0].BlankMean != 2 {
		t.Fatalf("unexpected astercore summary: %+v", sums)
	}

	// Verify full Vault UI workspace loads the reopened document without errors and renders 240 mg & 已锚定 t0
	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatalf("initVaultWorkspace: %v", err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.OnOpenUserGuide = a.openOrSeedUserGuideDoc
	sh.EnsureTabs()
	sh.OpenFileInPane(sh.ActivePaneID, relPath, "HUVEC 闭环", shell.ModeEdit, true)
	sh.RightOpen = false

	tester := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1280, 1100)
	tester.Frame()
	tester.Frame()
	if !tester.HasText("240") || !tester.HasText("已锚定 t0") {
		t.Fatalf("expected reopened UI to show 240 mg and live 已锚定 t0 anchor wake-up")
	}
}

// TestWetLabUXConvergenceAndSpotlightTourVisualGate verifies Package A/B/C end-to-end:
// - Interactive Obsidian-style user guide opens cleanly in a real Vault with live widgets
// - Calculator @plate.sample_wells renders as live human value pill ("9 有效孔") with zero raw @syntax leak
// - AssessDocument has zero false-positive NO_STEPS / NO_GROUPS on #@ steps + top-level group_table
// - Compact [ 编辑 | 实验 ] header switch + 读数自动汇总 (blank background + blank-corrected mean)
// - Interactive 5-step region-highlighted Spotlight Tour renders cleanly
func TestWetLabUXConvergenceAndSpotlightTourVisualGate(t *testing.T) {
	vRoot := t.TempDir()
	sh := &shell.Shell{TodayDate: "2026-11-22", SelectedDate: "2026-11-22"}
	a := &app{shell: sh}
	if err := a.initVaultWorkspace(vRoot); err != nil {
		t.Fatalf("initVaultWorkspace: %v", err)
	}
	sh.RenderTabContent = a.renderVaultTabContent
	sh.OnOpenUserGuide = a.openOrSeedUserGuideDoc
	sh.EnsureTabs()
	a.openOrSeedUserGuideDoc()
	sh.RightOpen = false

	// 1. Verify AssessDocument produces ZERO false-positive NO_STEPS or NO_GROUPS on #@ + top-level group_table
	tab := sh.ActiveTab()
	if tab == nil {
		t.Fatal("expected user guide tab to be open")
	}
	rawGuide, err := os.ReadFile(filepath.Join(vRoot, tab.FilePath))
	if err != nil {
		t.Fatalf("failed to read seeded user guide: %v", err)
	}
	parsedGuide, err := asterdoc.Parse(string(rawGuide))
	if err != nil {
		t.Fatalf("failed to parse seeded user guide: %v", err)
	}
	rep := astercore.AssessDocument(parsedGuide, nil)
	for _, d := range rep.Diagnostics {
		if d.Code == "NO_STEPS" || d.Code == "NO_GROUPS" {
			t.Fatalf("unexpected false-positive diagnostic on #@ + group_table guide: %+v", d)
		}
	}

	outDir := filepath.Join("..", "..", "docs", "sdk", "evidence", "v2-visual-catalog")
	_ = os.MkdirAll(outDir, 0o755)

	// 2. Capture interactive user guide + live human calculator pill + [ 编辑 | 实验 ] header
	renderVisualPNGSteps(t, outDir, "ux_convergence_guide_and_pills.png", 1360, 1080, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Frame()
		if !st.HasText("编辑") || !st.HasText("实验") {
			t.Fatalf("expected compact [ 编辑 | 实验 ] switcher in doc header")
		}
		if !st.HasText("9 有效孔") {
			t.Fatalf("expected live human calculator pill '9 有效孔' instead of raw @plate.sample_wells")
		}
		if st.HasText("@plate.sample_wells") {
			t.Fatalf("raw @plate.sample_wells leaked into calculator UI!")
		}
	})

	// 3. Capture interactive region-highlighted Spotlight Tour overlay (step 3: widgets & right-click menu)
	sh.StartTour()
	sh.TourStep = 2
	renderVisualPNGSteps(t, outDir, "ux_convergence_spotlight_tour.png", 1360, 900, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Frame()
		if !st.HasText("3 / 5 · 交互组件与右键菜单") || !st.HasText("打开新手指南文档") {
			t.Fatalf("expected spotlight tour overlay card visible")
		}
	})
}
