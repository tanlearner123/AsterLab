package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/diag"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/tools"
	"asterlab-mygo-prototype/internal/vault"
)

func TestEcosystemOnboardingFeedbackAndCrashBanner(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("LABWORKBENCH_DATA_DIR", dataDir)
	logDir := filepath.Join(dataDir, "logs")
	if err := diag.Init(logDir); err != nil {
		t.Fatalf("diag.Init: %v", err)
	}
	defer diag.Close()

	// Seed an unread panic record so startup surfaces the non-blocking recovery banner.
	diag.RecordPanic("sync-worker", "disk error under C:\\Users\\Bob\\Lab", []byte("trace line"))

	root := filepath.Join(t.TempDir(), "vault")
	v, err := vault.Open(root)
	if err != nil {
		t.Fatalf("vault.Open: %v", err)
	}
	_ = v.SeedTemplates(defaultTemplateSeeds())
	ds := vault.NewDocStore(v)

	sh := &shell.Shell{TodayDate: "2026-11-21", SelectedDate: "2026-11-21"}
	a := &app{shell: sh, vault: v, vaultIndex: vault.NewVaultIndex(v), docStore: ds}
	a.tools = tools.New(tools.Callbacks{
		ListPantry:      a.localListPantry,
		UpsertPantry:    a.localUpsertPantry,
		DeletePantry:    a.localDeletePantry,
		ExportPantry:    a.localExportPantryLabels,
		LoadPantryMeta:  func() (map[string]tools.PantryItemMeta, error) { return map[string]tools.PantryItemMeta{}, nil },
		SavePantryMeta:  func(map[string]tools.PantryItemMeta) error { return nil },
		LoadPantryBoxes: func() ([]tools.PantryBox, error) { return nil, nil },
		SavePantryBoxes: func([]tools.PantryBox) error { return nil },
	})
	sh.RenderTabContent = func(c *ui.Context, pane *shell.PaneNode, tab shell.WorkspaceTab) {
		a.renderVaultTabContent(c, pane, tab)
	}
	sh.RenderPantryContent = func(c *ui.Context) { a.renderStandalonePantry(c) }
	sh.RenderOverlays = func(c *ui.Context) {
		a.renderVaultNewExperimentDialog(c)
		a.renderImportDialog(c)
	}

	a.refreshVaultTreeAndCalendar()
	a.wireUpdaterCallbacks()
	a.wireFeedbackAndWelcome("")

	if !sh.WelcomeOpen {
		t.Fatal("expected first-run welcome dialog open on empty vault")
	}
	if !strings.Contains(sh.CrashNotice, "检测到上次运行发生意外异常") || strings.Contains(sh.CrashNotice, "Bob") {
		t.Fatalf("expected sanitized crash recovery banner notice, got %q", sh.CrashNotice)
	}

	sh.EnsureTabs()
	tt := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)
	tt.Frame()
	if !tt.HasText("欢迎使用 AsterLab") || !tt.HasText("体验「CCK-8 细胞活力」完整示范实验") {
		t.Fatalf("expected onboarding dialog content, got: %q", tt.Texts())
	}

	// Simulate user selecting the demo onboarding option.
	sh.OnOnboardingChoice("demo")
	tt.Frame()
	tt.Advance(150 * time.Millisecond)
	tt.Frame()
	if sh.WelcomeOpen || !hasSeenOnboarding() {
		t.Fatal("expected onboarding marked seen after choice")
	}
	if !tt.HasText("L929 CCK-8 细胞活力测定（示范）") {
		t.Fatalf("expected seeded demo experiment opened cleanly, got: %q", tt.Texts())
	}
	ledger, lErr := a.localListPantry()
	if lErr != nil || len(ledger.Items) < 2 {
		t.Fatalf("expected seeded demo reagents in local pantry, err=%v items=%+v", lErr, ledger.Items)
	}

	// Verify feedback & diagnostics modal renders cleanly and report scrubs usernames.
	sh.FeedbackOpen = true
	tt.Frame()
	if !tt.HasText("反馈与诊断中心") || !tt.HasText("导出脱敏诊断包 (.zip)") {
		t.Fatalf("expected feedback & diagnostics modal controls, got: %q", tt.Texts())
	}
	sh.FeedbackOpen = false

	// Verify external .asterml file opening jumps right to the clicked file.
	extAbs := v.AbsPath("示范实验项目")
	matches, _ := filepath.Glob(filepath.Join(extAbs, "*.asterml"))
	if len(matches) == 0 {
		t.Fatalf("expected seeded .asterml file in %s", extAbs)
	}
	if gotArg := astermlFileArg([]string{"mygo-run.exe", matches[0]}); gotArg != matches[0] {
		t.Fatalf("astermlFileArg = %q, want %q", gotArg, matches[0])
	}
	a.openExternalAstermlFile(matches[0])
	if _, err := os.Stat(onboardingPath()); err != nil {
		t.Fatalf("expected onboarding.json written: %v", err)
	}
}
