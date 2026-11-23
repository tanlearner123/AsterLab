package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/tools"
	"asterlab-mygo-prototype/internal/vault"
	"asterlab-mygo-prototype/modules/runs"
)

func TestLocalPantryStoreCRUDAndLifecycle(t *testing.T) {
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())
	a := &app{}

	ledger, err := a.localListPantry()
	if err != nil {
		t.Fatalf("localListPantry empty: %v", err)
	}
	if len(ledger.Items) != 0 || ledger.NearExpiryDays != pantryNearExpiryWindowDays {
		t.Fatalf("unexpected empty ledger: %+v", ledger)
	}

	if _, err := a.localUpsertPantry(runs.PantryUpsert{Name: "   "}); err == nil {
		t.Fatal("expected error for blank reagent name")
	}

	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	shelf := uint32(90)
	created, err := a.localUpsertPantry(runs.PantryUpsert{
		Name:          "CCK-8 试剂盒",
		Location:      "4°C A1",
		PurchasedOn:   today.AddDate(0, 0, -10).Format("2006-01-02"),
		ShelfLifeDays: &shelf,
	})
	if err != nil {
		t.Fatalf("create reagent: %v", err)
	}
	if created.Revision != 1 || created.LabelCode != "PN-001" || created.Status != "unopened" || created.ExpiryBasis != "shelf_life" {
		t.Fatalf("unexpected created item: %+v", created)
	}

	openedShelf := uint32(15)
	rev1 := created.Revision
	updated, err := a.localUpsertPantry(runs.PantryUpsert{
		ID:                  created.ID,
		ExpectedRevision:    &rev1,
		Name:                created.Name,
		Location:            created.Location,
		PurchasedOn:         created.PurchasedOn,
		OpenedOn:            today.Format("2006-01-02"),
		ShelfLifeDays:       created.ShelfLifeDays,
		OpenedShelfLifeDays: &openedShelf,
	})
	if err != nil {
		t.Fatalf("update opened reagent: %v", err)
	}
	if updated.Revision != 2 || updated.Status != "near_expiry" || updated.ExpiryBasis != "opened_shelf_life" {
		t.Fatalf("unexpected opened item state: %+v", updated)
	}

	staleRev := int64(1)
	if _, err := a.localUpsertPantry(runs.PantryUpsert{
		ID:               created.ID,
		ExpectedRevision: &staleRev,
		Name:             "stale",
	}); err == nil || !strings.Contains(err.Error(), "CONFLICT:") {
		t.Fatalf("expected revision conflict error, got: %v", err)
	}

	expiredDays := uint32(5)
	expired, err := a.localUpsertPantry(runs.PantryUpsert{
		Name:          "过期的胰酶",
		Location:      "-20°C B2",
		PurchasedOn:   today.AddDate(0, 0, -10).Format("2006-01-02"),
		ShelfLifeDays: &expiredDays,
	})
	if err != nil || expired.Status != "expired" {
		t.Fatalf("expected expired status, err=%v item=%+v", err, expired)
	}

	rev2 := updated.Revision
	exhausted, err := a.localUpsertPantry(runs.PantryUpsert{
		ID:               updated.ID,
		ExpectedRevision: &rev2,
		Name:             updated.Name,
		Location:         updated.Location,
		Exhausted:        true,
	})
	if err != nil || exhausted.Status != "exhausted" {
		t.Fatalf("expected exhausted status, err=%v item=%+v", err, exhausted)
	}

	labels, err := a.localExportPantryLabels()
	if err != nil || len(labels.Labels) != 2 {
		t.Fatalf("expected 2 exported labels, err=%v labels=%+v", err, labels)
	}

	if err := a.localDeletePantry(runs.PantryDelete{ID: expired.ID}); err != nil {
		t.Fatalf("delete expired item: %v", err)
	}
	afterDel, err := a.localListPantry()
	if err != nil || len(afterDel.Items) != 1 || afterDel.Items[0].ID != updated.ID {
		t.Fatalf("unexpected ledger after delete: err=%v items=%+v", err, afterDel.Items)
	}
}

func TestObsidianVaultPantryAnd27InchVisualAudit(t *testing.T) {
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())
	outDir := os.Getenv("ASTERLAB_VISUAL_DIR")
	if outDir == "" {
		outDir = t.TempDir()
	} else if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	root := filepath.Join(t.TempDir(), "vault")
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	if err := v.SeedTemplates(defaultTemplateSeeds()); err != nil {
		t.Fatalf("SeedTemplates: %v", err)
	}
	ds := vault.NewDocStore(v)

	sh := &shell.Shell{
		TodayDate:    "2026-11-21",
		SelectedDate: "2026-11-21",
	}
	a := &app{
		shell:      sh,
		vault:      v,
		vaultIndex: vault.NewVaultIndex(v),
		docStore:   ds,
	}
	a.tools = tools.New(tools.Callbacks{
		ListPantry:        a.localListPantry,
		UpsertPantry:      a.localUpsertPantry,
		DeletePantry:      a.localDeletePantry,
		ExportPantry:      a.localExportPantryLabels,
		LoadPantryMeta:    func() (map[string]tools.PantryItemMeta, error) { return map[string]tools.PantryItemMeta{}, nil },
		SavePantryMeta:    func(map[string]tools.PantryItemMeta) error { return nil },
		LoadPantryBoxes:   func() ([]tools.PantryBox, error) { return nil, nil },
		SavePantryBoxes:   func([]tools.PantryBox) error { return nil },
	})
	sh.RenderTabContent = func(c *ui.Context, pane *shell.PaneNode, tab shell.WorkspaceTab) {
		a.renderVaultTabContent(c, pane, tab)
	}
	sh.RenderPantryContent = func(c *ui.Context) {
		a.renderStandalonePantry(c)
	}
	sh.RenderOverlays = func(c *ui.Context) {
		a.renderVaultNewExperimentDialog(c)
		a.renderImportDialog(c)
	}
	a.wireUpdaterCallbacks()
	a.refreshVaultTreeAndCalendar()

	// Verify template/SOP design creation works in Pure-Go Vault mode (designSvc == nil),
	// and legacy kind: assay inside a document never dumps a raw 12-field schema table in UnifiedLive.
	docWithAssay := "---\ntitle: CCK-8 试剂测试\n---\n\n```asterlab\nkind: experiment\nid: exp-1\nschema: 1\ntitle: CCK-8 试剂测试\nobjective: 验证分组相对活力\n```\n\n```asterlab\nkind: assay\nid: assay-1\nschema: 1\ntitle: CCK-8 检测\nendpoint: OD450\nunit: AU\ntimepoints: [D1]\n```\n\n```asterlab\nkind: step\nid: step-1\nschema: 1\ntitle: 01 细胞接种与配液\nblocks: []\n```\n"
	a.createAsterDocDesign(docWithAssay, "CCK-8 试剂测试")
	if a.syncErr != "" {
		t.Fatalf("createAsterDocDesign in pure vault mode failed: %s", a.syncErr)
	}

	// Verify Pure-Go Pantry upserts and renders without "运行服务不可用".
	shelf := uint32(180)
	if _, err := a.localUpsertPantry(runs.PantryUpsert{
		Name:          "SYBR Green Master Mix",
		Location:      "-20°C A2",
		PurchasedOn:   "2026-11-01",
		ShelfLifeDays: &shelf,
	}); err != nil {
		t.Fatalf("seed pantry item: %v", err)
	}
	a.tools.ReloadPantry()

	sh.EnsureTabs()
	sh.OpenPantryTab()
	renderVisualPNGSteps(t, outDir, "desktop_27inch_pantry_light.png", 2560, 1440, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Advance(200 * time.Millisecond)
		st.Frame()
		if st.HasText("运行服务不可用") {
			t.Fatalf("pantry rendered service-unavailable error in pure-go vault mode: %q", st.Texts())
		}
		if !st.HasText("SYBR Green Master Mix") || !st.HasText("储藏室货架") {
			t.Fatalf("expected seeded reagent in pantry shelf view, got: %q", st.Texts())
		}
	})

	if tab := sh.ActiveTab(); tab != nil {
		sh.OpenFileInPane(sh.ActivePaneID, "CCK-8 试剂测试.asterml", "CCK-8 试剂测试", shell.ModeEdit, true)
	}
	sh.SetRightDrawer(true, 1)
	renderVisualPNGSteps(t, outDir, "desktop_27inch_edit_light.png", 2560, 1440, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Advance(200 * time.Millisecond)
		st.Frame()
		if !st.HasText("CCK-8 试剂测试") || !st.HasText("点击此处添加组件") {
			t.Fatalf("expected guided empty step dashed dropzone, got: %q", st.Texts())
		}
		if st.HasText("方案版本标识") || st.HasText("结构化列表（在步骤工作台或源码中编辑）") {
			t.Fatalf("unexpected raw schema table in UnifiedLive edit view: %q", st.Texts())
		}
	})
	sh.SetRightDrawer(false, 0)

	// Verify the release blank template renders cleanly as a frameless whitepaper live-markdown canvas.
	sh.OpenFileInPane(sh.ActivePaneID, "templates/空白实验.asterml", "空白实验", shell.ModeEdit, true)
	renderVisualPNGSteps(t, outDir, "desktop_27inch_blank_light.png", 2560, 1440, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Advance(200 * time.Millisecond)
		st.Frame()
		if !st.HasText("空白实验") || !st.HasText("科学笔记正文") || !st.HasText("0 步骤 · 0 模块") {
			t.Fatalf("expected clean whitepaper blank template canvas, got: %q", st.Texts())
		}
	})

	// Verify update badge & Settings "about" update panel render cleanly.
	sh.UpdateStatus = "available"
	sh.UpdateLatestVersion = "v0.1.1"
	sh.UpdateNotes = "- 修复储藏室 Pure-Go 台账\n- 统一 27 寸桌面字号"
	sh.SettingsView.Category = "about"
	sh.VaultSettingsOpen = true
	renderVisualPNGSteps(t, outDir, "desktop_27inch_settings_about_light.png", 2560, 1440, false, func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, func(st *ui.Tester) {
		st.Frame()
		st.Advance(200 * time.Millisecond)
		st.Frame()
		if !st.HasText("软件版本与自动更新") || !st.HasText("立即更新并重启") || !st.HasText("发现新版本 v0.1.1") {
			t.Fatalf("expected update UI controls in About settings, got: %q", st.Texts())
		}
	})
	sh.VaultSettingsOpen = false
}

func TestStandalonePantrySingleClickOpensDetailAndBoxDialogs(t *testing.T) {
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())
	root := filepath.Join(t.TempDir(), "pantry-ui-vault")
	a := &app{shell: &shell.Shell{TodayDate: "2026-11-23", SelectedDate: "2026-11-23"}}
	if err := a.initVaultWorkspace(root); err != nil {
		t.Fatalf("initVaultWorkspace: %v", err)
	}
	shelf := uint32(180)
	if _, err := a.localUpsertPantry(runs.PantryUpsert{
		Name:          "Vancomycin Stock",
		Location:      "-20°C 冰箱",
		PurchasedOn:   "2026-11-20",
		ShelfLifeDays: &shelf,
	}); err != nil {
		t.Fatalf("seed pantry: %v", err)
	}
	a.tools = a.newToolsWorkspace()

	st := ui.NewTester(func(c *ui.Context) {
		tools.ViewPantry(c, a.tools)
	}, 1280, 800)
	st.Frame()
	if !st.HasText("Vancomycin Stock") {
		t.Fatalf("expected reagent on standalone pantry shelf, got: %q", st.Texts())
	}

	// Single click on the shelf bottle/label must open AND render the Reagent Detail Dialog.
	st.Click("Vancomycin Stock")
	st.Frame()
	if !a.tools.PantryDialogOpen {
		t.Fatalf("expected single-click to set PantryDialogOpen=true")
	}
	if !st.HasText("保存试剂档案") || !st.HasText("说明书与产品网址") {
		t.Fatalf("expected reagent detail dialog to actually render in ViewPantry, got: %q", st.Texts())
	}
	a.tools.PantryDialogOpen = false
	st.Frame()

	// Single click on "+ 新建收纳盒" must open AND render the Box Dialog.
	st.Click("+ 新建收纳盒")
	st.Frame()
	if !a.tools.PantryBoxDialogOpen {
		t.Fatalf("expected + 新建收纳盒 click to set PantryBoxDialogOpen=true")
	}
	if !st.HasText("保存收纳盒") || !st.HasText("选择容器规格") {
		t.Fatalf("expected box dialog to actually render in ViewPantry, got: %q", st.Texts())
	}
}

func TestVaultIsolatedPantryLastVaultMemoryAndNoGlobalPollution(t *testing.T) {
	globalData := t.TempDir()
	t.Setenv("LABWORKBENCH_DATA_DIR", globalData)
	t.Setenv("ASTERLAB_VAULT_DIR", "")

	// Plant legacy global pantry item & legacy experiment in globalData — must NEVER leak into user Vaults.
	legacyJSON := `{"nextSeq":1,"items":[{"id":"legacy-1","labelCode":"PN-001","name":"Legacy Global GelMA","location":"4°C","revision":1}]}`
	if err := os.WriteFile(filepath.Join(globalData, "pantry-ledger.json"), []byte(legacyJSON), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyExpDir := filepath.Join(globalData, "experiments", "old-exp")
	if err := os.MkdirAll(legacyExpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyExpDir, "experiment.asterml"), []byte("---\ntitle: 旧全局实验\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	vaultA := filepath.Join(t.TempDir(), "VaultA")
	vaultB := filepath.Join(t.TempDir(), "VaultB")
	a := &app{shell: &shell.Shell{TodayDate: "2026-11-23", SelectedDate: "2026-11-23"}}
	a.tools = a.newToolsWorkspace()

	a.switchVaultRoot(vaultA)
	if a.syncErr != "" {
		t.Fatalf("switchVaultRoot(vaultA): %s", a.syncErr)
	}
	if len(a.tools.Pantry.Items) != 0 {
		t.Fatalf("expected fresh VaultA pantry to be isolated and empty, got %d items: %+v", len(a.tools.Pantry.Items), a.tools.Pantry.Items)
	}
	if len(a.shell.Experiments) != 0 {
		t.Fatalf("expected fresh VaultA not to inherit legacy global experiments, got: %+v", a.shell.Experiments)
	}

	// Add a reagent inside VaultA and verify it persists strictly under <VaultA>/.asterlab/pantry.jsonl.
	if _, err := a.localUpsertPantry(runs.PantryUpsert{Name: "Lysostaphin 1 mg/mL", Location: "-20°C"}); err != nil {
		t.Fatalf("upsert VaultA reagent: %v", err)
	}
	a.tools.ReloadPantry()
	a.syncPantrySnapshotToShell()
	if _, err := os.Stat(filepath.Join(vaultA, ".asterlab", pantryJSONLFileName)); err != nil {
		t.Fatalf("expected VaultA single-file pantry.jsonl inside .asterlab/: %v", err)
	}
	for _, legacyName := range []string{"pantry-ledger.json", "pantry-meta.json", "pantry-boxes.json"} {
		if _, err := os.Stat(filepath.Join(vaultA, ".asterlab", legacyName)); !os.IsNotExist(err) {
			t.Fatalf("expected fresh VaultA not to create legacy %s alongside pantry.jsonl", legacyName)
		}
	}
	if len(a.shell.PantryItems) != 1 {
		t.Fatalf("expected 1 pantry item synced to shell in VaultA, got %d", len(a.shell.PantryItems))
	}

	// Switch to VaultB: pantry AND right-drawer snapshot must both reset to empty cleanly.
	a.switchVaultRoot(vaultB)
	if len(a.tools.Pantry.Items) != 0 || len(a.shell.PantryItems) != 0 {
		t.Fatalf("expected VaultB pantry isolated from VaultA, tools=%d shell=%d", len(a.tools.Pantry.Items), len(a.shell.PantryItems))
	}

	// Next cold start with empty vaultRoot must automatically reopen VaultB from vault-prefs.json.
	b := &app{shell: &shell.Shell{TodayDate: "2026-11-23", SelectedDate: "2026-11-23"}}
	if err := b.initVaultWorkspace(""); err != nil {
		t.Fatalf("reopen last vault: %v", err)
	}
	if !strings.EqualFold(filepath.Clean(b.vault.Root), filepath.Clean(vaultB)) {
		t.Fatalf("expected startup to remember last switched VaultB (%s), got %s", vaultB, b.vault.Root)
	}
}

func TestVaultLiveDiskWatcherAndAgentGuideDiscovery(t *testing.T) {
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())
	root := filepath.Join(t.TempDir(), "LiveWatchVault")
	a := &app{shell: &shell.Shell{TodayDate: "2026-11-23", SelectedDate: "2026-11-23"}}
	if err := a.initVaultWorkspace(root); err != nil {
		t.Fatalf("initVaultWorkspace: %v", err)
	}

	// 1. AGENTS.md must be seeded automatically for zero-context AI Agents, without cluttering VaultTree.
	guidePath := filepath.Join(root, vault.AgentGuideFileName)
	guideRaw, err := os.ReadFile(guidePath)
	if err != nil {
		t.Fatalf("expected AGENTS.md seeded in vault root: %v", err)
	}
	if !strings.Contains(string(guideRaw), "asterdoc.validate") || !strings.Contains(string(guideRaw), `unit`) {
		t.Fatalf("expected AGENTS.md to include CLI validation and UCUM unit guidance")
	}
	for _, f := range a.shell.VaultFiles {
		if strings.EqualFold(filepath.Base(f.RelPath), vault.AgentGuideFileName) {
			t.Fatalf("AGENTS.md should not clutter experiment VaultFiles: %+v", f)
		}
	}

	// 2. External Agent writes a new `.asterml` file directly to disk inside a new subfolder.
	extDir := filepath.Join(root, "Fig3.5-抗菌实验")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	extFile := filepath.Join(extDir, "外部Agent新实验.asterml")
	v1 := "---\ntitle: 外部 Agent 计划 v1\nscheduled_date: 2026-11-24\nstage: planned\n---\n\n#@ 01 准备菌液\n"
	if err := os.WriteFile(extFile, []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}

	if !a.syncVaultDiskChangesNow() {
		t.Fatal("expected syncVaultDiskChangesNow to detect externally added .asterml file")
	}
	found := false
	for _, exp := range a.shell.Experiments {
		if exp.FilePath == "Fig3.5-抗菌实验/外部Agent新实验.asterml" && exp.Title == "外部 Agent 计划 v1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected externally written file in shell.Experiments, got: %+v", a.shell.Experiments)
	}

	// 3. Open the document, then externally edit it on disk -> watcher must reload LiveDocument automatically.
	ld, err := a.docStore.Open("Fig3.5-抗菌实验/外部Agent新实验.asterml")
	if err != nil {
		t.Fatalf("docStore.Open: %v", err)
	}
	v2 := "---\ntitle: 外部 Agent 计划 v2\nscheduled_date: 2026-11-24\nstage: in_progress\n---\n\n#@ 01 准备菌液\n\n#@ 02 涂板计数\n"
	if err := os.WriteFile(extFile, []byte(v2), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(extFile, future, future)

	if !a.syncVaultDiskChangesNow() {
		t.Fatal("expected syncVaultDiskChangesNow to detect external edit to open .asterml")
	}
	if !strings.Contains(ld.Source, "外部 Agent 计划 v2") {
		t.Fatalf("expected open LiveDocument to auto-reload external disk changes, got: %q", ld.Source)
	}
}

func TestPantryJSONLSingleFileSyncMigrationAndExternalAppend(t *testing.T) {
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())

	// 1. Unified single-file write (boxes + reagent + meta) without volatile derived fields on disk.
	root := filepath.Join(t.TempDir(), "JSONLVault")
	a := &app{shell: &shell.Shell{TodayDate: "2026-11-23", SelectedDate: "2026-11-23"}}
	if err := a.initVaultWorkspace(root); err != nil {
		t.Fatalf("initVaultWorkspace: %v", err)
	}
	a.tools = a.newToolsWorkspace()

	if err := a.savePantryBoxes([]tools.PantryBox{{
		ID: "box-1", Name: "抗菌冻存盒 #1", Kind: tools.BoxCryo9x9, ZoneKey: "-80", Rows: 9, Cols: 9,
	}}); err != nil {
		t.Fatalf("savePantryBoxes: %v", err)
	}
	shelf := uint32(365)
	saved, err := a.localUpsertPantry(runs.PantryUpsert{
		Name: "Vancomycin HCl", Location: "-80°C", PurchasedOn: "2026-11-20", ShelfLifeDays: &shelf,
	})
	if err != nil {
		t.Fatalf("localUpsertPantry: %v", err)
	}
	if err := a.savePantryMeta(map[string]tools.PantryItemMeta{
		saved.ID: {CatalogNo: "Sigma V2002", LotNo: "LOT-2026A", BoxID: "box-1", BoxSlot: "A1"},
	}); err != nil {
		t.Fatalf("savePantryMeta: %v", err)
	}

	jsonlPath := filepath.Join(root, ".asterlab", pantryJSONLFileName)
	rawBytes, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("read pantry.jsonl: %v", err)
	}
	rawStr := string(rawBytes)
	if !strings.Contains(rawStr, `"type":"schema"`) || !strings.Contains(rawStr, `"type":"box"`) || !strings.Contains(rawStr, `"type":"reagent"`) {
		t.Fatalf("expected schema + box + reagent lines in pantry.jsonl, got:\n%s", rawStr)
	}
	if !strings.Contains(rawStr, `"catalogNo":"Sigma V2002"`) || !strings.Contains(rawStr, `"boxSlot":"A1"`) {
		t.Fatalf("expected reagent metadata embedded on reagent line, got:\n%s", rawStr)
	}
	if strings.Contains(rawStr, `"daysRemaining"`) || strings.Contains(rawStr, `"expiresOn"`) {
		t.Fatalf("expected volatile derived fields omitted from pantry.jsonl for clean diffs, got:\n%s", rawStr)
	}

	// 2. External Agent appends a minimal reagent line -> live watcher picks it up automatically.
	appended := rawStr + `{"type":"reagent","name":"Lysostaphin 2 mg/mL","location":"-20°C","purchasedOn":"2026-11-23","shelfLifeDays":180}` + "\n"
	if err := os.WriteFile(jsonlPath, []byte(appended), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	_ = os.Chtimes(jsonlPath, future, future)

	if !a.syncVaultDiskChangesNow() {
		t.Fatal("expected syncVaultDiskChangesNow to detect external append to .asterlab/pantry.jsonl")
	}
	if len(a.tools.Pantry.Items) != 2 {
		t.Fatalf("expected 2 pantry items after external JSONL append, got %d: %+v", len(a.tools.Pantry.Items), a.tools.Pantry.Items)
	}

	// 3. Legacy 3-file JSON auto-migrates losslessly into single-file pantry.jsonl on first load.
	migDir := filepath.Join(t.TempDir(), "LegacyMig")
	if err := os.MkdirAll(migDir, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(migDir, "pantry-ledger.json"), []byte(`{"nextSeq":2,"items":[{"id":"old-1","labelCode":"PN-002","name":"Old PBS","location":"4°C","revision":1}]}`), 0o644)
	_ = os.WriteFile(filepath.Join(migDir, "pantry-meta.json"), []byte(`{"old-1":{"catalogNo":"Gibco 10010","boxId":"old-box","boxSlot":"B3"}}`), 0o644)
	_ = os.WriteFile(filepath.Join(migDir, "pantry-boxes.json"), []byte(`[{"id":"old-box","name":"4C 管架","kind":"tube_rack_15ml","zoneKey":"4C","rows":5,"cols":5}]`), 0o644)

	migrated, err := loadPantryStore(migDir)
	if err != nil {
		t.Fatalf("loadPantryStore legacy migration: %v", err)
	}
	if len(migrated.Items) != 1 || migrated.Items[0].Name != "Old PBS" || len(migrated.Boxes) != 1 || migrated.Meta["old-1"].CatalogNo != "Gibco 10010" {
		t.Fatalf("unexpected migrated store: %+v", migrated)
	}
	if _, err := os.Stat(filepath.Join(migDir, pantryJSONLFileName)); err != nil {
		t.Fatalf("expected pantry.jsonl written during migration: %v", err)
	}
}
