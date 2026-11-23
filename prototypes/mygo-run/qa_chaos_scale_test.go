package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/vault"
	"asterlab-mygo-prototype/modules/runs"
)

// TestQA1_ChaosFS_CorruptTruncatedFilesNeverPanicOrLoseData verifies Layer 1
// (Data Safety & Chaos Filesystem): malformed/truncated/BOM/garbage persistence
// files never crash AsterLab, never wipe valid user data, and surface clean recovery.
func TestQA1_ChaosFS_CorruptTruncatedFilesNeverPanicOrLoseData(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), "ChaosVault")
	sysDir := filepath.Join(vaultRoot, ".asterlab")
	if err := os.MkdirAll(filepath.Join(vaultRoot, "Proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sysDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// 1. Corrupt config.json + truncated workspace.json + unclosed YAML `.asterml` + valid `.asterml`.
	if err := os.WriteFile(filepath.Join(sysDir, "config.json"), []byte("{not-valid-json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sysDir, "workspace.json"), []byte(`{"rootPane":{"id":"pane-root","tabs":[{"id":"file-1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	brokenDoc := "---\ntitle: 损坏实验\n---\n\n```asterlab\nkind: param_table\nid: pt-broken\nschema: 1\nrows:\n  - [unclosed\n"
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "Broken.asterml"), []byte(brokenDoc), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "Healthy.asterml"), []byte(fsmTestDocAlpha), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	assertGlobalShellInvariants(t, sh, "chaos-boot")

	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1280, 800)
	assertBoundedFrame(t, st, "chaos-initial-frame")

	// Opening the broken `.asterml` must show graceful source-repair fallback instead of crashing.
	sh.OpenFileInPane(sh.ActivePaneID, "Proj/Broken.asterml", "损坏实验", shell.ModeEdit, false)
	assertBoundedFrame(t, st, "chaos-open-broken-doc")
	if !st.HasText("切换到源码模式修复") && !st.HasText("待语法闭合") {
		t.Fatalf("expected graceful parse-repair UI for broken .asterml, got: %q", st.Texts())
	}

	// Opening the healthy sibling `.asterml` right after must work cleanly.
	sh.OpenFileInPane(sh.ActivePaneID, "Proj/Healthy.asterml", "正常实验", shell.ModeEdit, false)
	assertBoundedFrame(t, st, "chaos-open-healthy-doc")
	if !st.HasText("Van-Lys 液体共培养涂板计数计划") {
		t.Fatalf("expected healthy doc to render cleanly after opening broken doc")
	}

	// 2. Corrupt pantry.jsonl must return an error and NEVER silently overwrite/wipe user's file.
	pantryPath := filepath.Join(sysDir, pantryJSONLFileName)
	corruptPantry := `{"type":"schema","version":1}` + "\n" + `{"type":"reagent","id":"keep-1","name":"珍贵抗体"` + "\n"
	if err := os.WriteFile(pantryPath, []byte(corruptPantry), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := a.localUpsertPantry(runs.PantryUpsert{Name: "NewItem", Location: "4°C"}); err == nil {
		t.Fatal("expected localUpsertPantry to refuse overwriting corrupt pantry.jsonl")
	}
	stillThere, err := os.ReadFile(pantryPath)
	if err != nil || string(stillThere) != corruptPantry {
		t.Fatalf("corrupt pantry.jsonl was unexpectedly modified or wiped: err=%v content=%q", err, string(stillThere))
	}

	// 3. Valid JSONL with UTF-8 BOM + comments + duplicate ID (last-line-wins) parses cleanly.
	bomJSONL := "\xef\xbb\xbf" +
		"# Vault pantry backup\n" +
		`{"type":"schema","version":1}` + "\n" +
		`{"type":"reagent","id":"r-1","name":"Old Name","location":"4°C","revision":1}` + "\n" +
		`{"type":"reagent","id":"r-1","name":"Updated Name","location":"-20°C","revision":2}` + "\n"
	if err := os.WriteFile(pantryPath, []byte(bomJSONL), 0o644); err != nil {
		t.Fatal(err)
	}
	ledger, err := a.localListPantry()
	if err != nil {
		t.Fatalf("expected UTF-8 BOM + comments + duplicate-id JSONL to parse cleanly: %v", err)
	}
	if len(ledger.Items) != 1 || ledger.Items[0].Name != "Updated Name" || ledger.Items[0].Location != "-20°C" {
		t.Fatalf("expected last-line-wins for duplicate reagent id, got: %+v", ledger.Items)
	}
}

// TestQA3_ScaleAndPerformanceGate_200FilesAndHeavyDoc verifies Layer 3 (Scale &
// Performance): a 200-experiment Vault scans, indexes, watches, and searches within
// tight latency budgets, and a 24-module heavy document renders smoothly.
func TestQA3_ScaleAndPerformanceGate_200FilesAndHeavyDoc(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), "Scale200Vault")
	const totalDocs = 200
	for i := 0; i < totalDocs; i++ {
		folder := filepath.Join(vaultRoot, fmt.Sprintf("Project-%02d", i%10))
		if err := os.MkdirAll(folder, 0o755); err != nil {
			t.Fatal(err)
		}
		var doc string
		if i == 137 {
			doc = strings.Replace(fsmTestDocAlpha, "Van-Lys 液体共培养涂板计数计划", "目标检索实验-Vancomycin-137", 1)
		} else if i%2 == 0 {
			doc = strings.Replace(fsmTestDocAlpha, "Van-Lys 液体共培养涂板计数计划", fmt.Sprintf("抗菌共培养实验 #%03d", i), 1)
		} else {
			doc = strings.Replace(fsmTestDocBeta, "Van-Lys 滤纸片扩散抑菌圈计划", fmt.Sprintf("滤纸片抑菌圈实验 #%03d", i), 1)
		}
		fpath := filepath.Join(folder, fmt.Sprintf("exp-%03d.asterml", i))
		if err := os.WriteFile(fpath, []byte(doc), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	v, err := vault.Open(vaultRoot)
	if err != nil {
		t.Fatal(err)
	}
	t0 := time.Now()
	_, files, err := v.ScanTree()
	scanDur := time.Since(t0)
	if err != nil {
		t.Fatalf("ScanTree(200): %v", err)
	}
	if len(files) < totalDocs {
		t.Fatalf("expected >= %d files scanned, got %d", totalDocs, len(files))
	}
	if scanDur > 4*time.Second {
		t.Fatalf("200-file ScanTree too slow: %v (budget 4s)", scanDur)
	}

	idx := vault.NewVaultIndex(v)
	t1 := time.Now()
	if err := idx.Refresh(); err != nil {
		t.Fatalf("VaultIndex.Refresh(200): %v", err)
	}
	idxDur := time.Since(t1)
	if idxDur > 4*time.Second {
		t.Fatalf("200-file VaultIndex.Refresh too slow: %v (budget 4s)", idxDur)
	}

	// Watcher disk signature must be fast enough to run every 1.2s without CPU spikes.
	t2 := time.Now()
	sig1, _ := computeVaultDiskSignatures(vaultRoot)
	sigDur := time.Since(t2)
	if sig1 == "" || sigDur > 800*time.Millisecond {
		t.Fatalf("200-file computeVaultDiskSignatures too slow: %v (budget 800ms)", sigDur)
	}

	// Full-text search across 200 files must feel instantaneous.
	t3 := time.Now()
	hits := idx.SearchFullText("目标检索实验-Vancomycin-137", 20)
	searchDur := time.Since(t3)
	if len(hits) == 0 {
		t.Fatal("expected full-text search to locate target doc #137 across 200 files")
	}
	if searchDur > 500*time.Millisecond {
		t.Fatalf("200-file SearchFullText too slow: %v (budget 500ms)", searchDur)
	}

	// Build a heavy single document (8 steps, 24 structured modules) and benchmark warm frame render.
	var sb strings.Builder
	sb.WriteString("---\ntitle: 超大型多步骤综合实验压测\nscheduled_date: 2026-11-24\nstage: in_progress\n---\n\n")
	sb.WriteString("```asterlab\nkind: experiment\nid: exp-heavy\nschema: 1\ntitle: 超大型多步骤综合实验压测\nobjective: 验证 24 模块长文档渲染流畅度\nexperimental_unit: well\n```\n\n")
	sb.WriteString("```asterlab\nkind: group_table\nid: gt-heavy\nschema: 1\ntitle: 实验分组\nrows:\n  - id: g-c\n    name: 对照组\n    role: control\n    condition: PBS\n    independent_n: 3\n  - id: g-t\n    name: 处理组\n    role: treatment\n    condition: 10 ug/mL\n    independent_n: 3\n```\n\n")
	for s := 1; s <= 8; s++ {
		fmt.Fprintf(&sb, "#@ %02d 实验阶段 %d\n\n- 阶段 %d 操作说明与注意事项，确保温控与移液一致。\n\n", s, s, s)
		fmt.Fprintf(&sb, "```asterlab\nkind: param_table\nid: pt-h-%d\nschema: 1\ntitle: 阶段 %d 参数\nrows:\n  - key: temp\n    label: 温度\n    value: 37\n    unit: Cel\n  - key: vol\n    label: 体积\n    value: 100\n    unit: uL\n```\n\n", s, s)
		fmt.Fprintf(&sb, "```asterlab\nkind: timer\nid: tm-h-%d\nschema: 1\ntitle: 阶段 %d 孵育\nmode: countdown\nstatus: idle\nduration:\n  value: 30\n  unit: min\n```\n\n", s, s)
		fmt.Fprintf(&sb, "```asterlab\nkind: checklist\nid: ck-h-%d\nschema: 1\ntitle: 阶段 %d 核对\nitems:\n  - id: c1\n    label: 核对标签编号\n    checked: false\n    blocking: false\n```\n\n", s, s)
	}
	heavyRel := "Project-00/Heavy24Modules.asterml"
	if err := os.WriteFile(filepath.Join(vaultRoot, filepath.FromSlash(heavyRel)), []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	_, sh := newWiredFSMTestApp(t, vaultRoot)
	sh.OpenFileInPane(sh.ActivePaneID, heavyRel, "超大型多步骤综合实验压测", shell.ModeEdit, false)
	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)
	// Warmup first frame, then measure 5 steady-state frames.
	st.Frame()
	var maxWarm time.Duration
	for i := 0; i < 5; i++ {
		f0 := time.Now()
		st.Frame()
		if d := time.Since(f0); d > maxWarm {
			maxWarm = d
		}
	}
	if maxWarm > 500*time.Millisecond {
		t.Fatalf("heavy 24-module document warm frame exceeded 500ms budget: %v", maxWarm)
	}
	t.Logf("scale metrics: scan200=%v index200=%v watchSig200=%v search200=%v heavy24MaxFrame=%v",
		scanDur, idxDur, sigDur, searchDur, maxWarm)
}
