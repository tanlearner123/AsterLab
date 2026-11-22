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
