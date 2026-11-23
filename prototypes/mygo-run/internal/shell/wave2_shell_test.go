package shell

import (
	"testing"
	"time"

	"asterlab-mygo-prototype/internal/vault"
)

func TestGlobalSearchModalAndCompanionDiagnosticsTab(t *testing.T) {
	openedRel := ""
	openedQuery := ""
	restoredSnap := ""

	s := &Shell{
		TodayDate:        "2026-11-20",
		GlobalSearchOpen: true,
		GlobalSearchQuery: "CCK8",
		GlobalSearchHits: []vault.SearchHit{
			{RelPath: "2026/cck8.asterml", Title: "CCK-8 筛选实验", LineNum: 12, Snippet: "OD450 读数与对照组设置", Date: "2026-11-20"},
		},
		OnGlobalSearch: func(q string) []vault.SearchHit {
			return []vault.SearchHit{
				{RelPath: "2026/cck8.asterml", Title: "CCK-8 筛选实验", LineNum: 12, Snippet: "OD450 读数与对照组设置", Date: "2026-11-20"},
			}
		},
		OnOpenSearchHit: func(relPath, query string) {
			openedRel = relPath
			openedQuery = query
		},
		VaultTimers: []vault.VaultTimerEntry{
			{RelPath: "2026/cck8.asterml", DocTitle: "CCK-8 筛选实验", TimerID: "tm-1", Title: "孵育计时", Status: "running", Mode: "countdown", PlannedSec: 3600, StartedAt: time.Now().UTC().Format(time.RFC3339)},
		},
		CurrentDocRelPath: "2026/cck8.asterml",
		CurrentBacklinks: []vault.LinkRef{
			{SourceRel: "2026/followup.asterml", SourceTitle: "复现验证实验", SourceDate: "2026-11-21", Kind: "repeat_of"},
		},
		CurrentDiagnostics: []DiagnosticItem{
			{Code: "ARRIVE_MISSING_N", Severity: "needs_confirmation", Message: "缺少独立 n", NextAction: "补充 independent_n"},
		},
		CurrentSnapshots: []vault.SnapshotMeta{
			{ID: "snap-1", RelPath: "2026/cck8.asterml", CreatedAt: time.Now(), Size: 512},
		},
		OnRestoreSnapshot: func(relPath, snapID string) {
			restoredSnap = relPath + ":" + snapID
		},
	}

	tt := render(s.RenderSplitView, 1400, 880)
	tt.Frame()

	if !tt.HasText("CCK-8 筛选实验") {
		t.Fatalf("expected global search modal hit, got: %q", tt.Texts())
	}
	if !tt.HasText("1 个计时中") {
		t.Fatalf("expected desktop status bar vault timer badge, got: %q", tt.Texts())
	}
	if err := tt.Click("CCK-8 筛选实验"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if openedRel != "2026/cck8.asterml" || openedQuery != "CCK8" {
		t.Fatalf("OnOpenSearchHit mismatch: rel=%q query=%q", openedRel, openedQuery)
	}

	// Open companion drawer tab 2 ("关联与诊断")
	s.SetRightDrawer(true, 2)
	tt.Frame()
	if err := tt.Click("关联与诊断"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("复现验证实验") || !tt.HasText("缺少独立 n") {
		t.Fatalf("expected backlinks and diagnostics in tab 2, got: %q", tt.Texts())
	}

	// Open File Recovery snapshot item in settings and focus it via search so OS font metrics never clip it below the fold
	s.VaultSettingsOpen = true
	s.SettingsView.Query = "文件恢复"
	tt.Frame()
	if !tt.HasText("恢复此版本") {
		t.Fatalf("expected File Recovery snapshot restore button in settings")
	}
	if err := tt.Click("恢复此版本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if restoredSnap != "2026/cck8.asterml:snap-1" {
		t.Fatalf("restoredSnap = %q, want 2026/cck8.asterml:snap-1", restoredSnap)
	}
}
