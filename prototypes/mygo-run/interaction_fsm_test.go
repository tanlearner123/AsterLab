package main

import (
	"fmt"
	"math/rand"
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

const fsmTestDocAlpha = `---
title: Van-Lys 液体共培养涂板计数计划
scheduled_date: 2026-11-24
stage: planned
---

` + "```asterlab" + `
kind: experiment
id: exp-alpha
schema: 1
title: Van-Lys 液体共培养涂板计数计划
objective: 比较 Vancomycin 与 Lysostaphin 联用对 MRSA 的液体共培养杀菌效果（CFU/mL）。
experimental_unit: tube
` + "```" + `

` + "```asterlab" + `
kind: group_table
id: gt-alpha
schema: 1
title: 实验分组
rows:
  - id: g-ctrl
    name: PBS 阴性对照
    role: control
    condition: PBS
    independent_n: 3
  - id: g-van
    name: Vancomycin 单药
    role: treatment
    condition: 2 ug/mL Van
    independent_n: 3
  - id: g-combo
    name: Van+Lys 联用组
    role: treatment
    condition: 2 ug/mL Van + 1 ug/mL Lys
    independent_n: 3
` + "```" + `

#@ 01 菌液配制与共培养

- 调整菌液至 OD600 = 0.1，稀释至 1e6 CFU/mL。

` + "```asterlab" + `
kind: param_table
id: pt-alpha
schema: 1
title: 共培养参数
rows:
  - key: temp
    label: 培养温度
    value: 37
    unit: Cel
  - key: rpm
    label: 摇床转速
    value: 200
    unit: rpm
  - key: duration
    label: 共培养时长
    value: 24
    unit: h
` + "```" + `

` + "```asterlab" + `
kind: timer
id: tm-alpha
schema: 1
title: 共培养孵育计时
mode: countdown
status: idle
duration:
  value: 120
  unit: min
` + "```" + `

#@ 02 梯度稀释涂板与菌落计数

` + "```asterlab" + `
kind: data_table
id: dt-alpha
schema: 1
title: 涂板菌落读数表
columns:
  - key: group
    label: 分组
    value_kind: text
  - key: dilution
    label: 稀释倍数
    value_kind: number
    unit: "1"
  - key: cfu
    label: 菌落数
    value_kind: number
    unit: "1"
rows:
  - group: PBS 阴性对照
    dilution: 10000
  - group: Van+Lys 联用组
    dilution: 100
` + "```" + `
`

const fsmTestDocBeta = `---
title: Van-Lys 滤纸片扩散抑菌圈计划
scheduled_date: 2026-11-25
stage: in_progress
---

` + "```asterlab" + `
kind: experiment
id: exp-beta
schema: 1
title: Van-Lys 滤纸片扩散抑菌圈计划
objective: 评估 Vancomycin 与 Lysostaphin 复配滤纸片在 90 mm 琼脂平皿上的抑菌圈直径（mm）。
experimental_unit: plate
` + "```" + `

` + "```asterlab" + `
kind: group_table
id: gt-beta
schema: 1
title: 纸片分组
rows:
  - id: g-blank
    name: 空白纸片
    role: control
    condition: PBS 10 uL
    independent_n: 3
  - id: g-combo
    name: 联用纸片
    role: treatment
    condition: Van+Lys 10 uL
    independent_n: 3
` + "```" + `

#@ 01 涂布平皿与贴片

` + "```asterlab" + `
kind: plate
id: pl-beta
schema: 1
title: 24 孔配液预排
format: 24
groups:
  g-blank: [A1, A2, A3]
  g-combo: [B1, B2, B3]
` + "```" + `

` + "```asterlab" + `
kind: checklist
id: ck-beta
schema: 1
title: 贴片质控清单
items:
  - id: c1
    label: 确认平皿表面干燥无冷凝水
    checked: false
    blocking: true
  - id: c2
    label: 纸片间距大于 24 mm
    checked: false
    blocking: false
` + "```" + `
`

func newWiredFSMTestApp(t *testing.T, vaultRoot string) (*app, *shell.Shell) {
	t.Helper()
	t.Setenv("LABWORKBENCH_DATA_DIR", t.TempDir())
	t.Setenv("ASTERLAB_VAULT_DIR", "")

	sh := &shell.Shell{
		TodayDate:    "2026-11-23",
		SelectedDate: "2026-11-23",
	}
	a := &app{shell: sh}
	sh.OnCreate = func() { a.openNewExperimentDialog("", sh.SelectedDate) }
	sh.OnCreateForDate = func(d string) { a.openNewExperimentDialog("", d) }
	sh.OnVaultCreateFile = func(folder, d string) { a.openNewExperimentDialog(folder, d) }
	sh.OnVaultCreateFolder = func(parent, name string) {
		if a.vault != nil {
			if _, err := a.vault.CreateFolder(parent, name); err == nil {
				a.refreshVaultTreeAndCalendar()
			}
		}
	}
	sh.OnOpenExperiment = func(item shell.ExperimentItem) {
		if item.FilePath != "" {
			a.openVaultFileInActivePane(item.FilePath, item.Title)
		}
	}
	sh.OnActivateTab = func(_ shell.WorkspaceTab) {
		a.saveVaultWorkspace()
	}
	sh.OnGlobalSearch = func(q string) []vault.SearchHit {
		if a.vaultIndex == nil {
			return nil
		}
		return a.vaultIndex.SearchFullText(q, 50)
	}
	sh.OnOpenSearchHit = func(relPath, query string) {
		if query != "" {
			if a.pendingFindQuery == nil {
				a.pendingFindQuery = map[string]string{}
			}
			a.pendingFindQuery[relPath] = query
		}
		a.openVaultFileInActivePane(relPath, "")
	}
	sh.OnSelectOutlineStep = func(stepID string) {
		a.designSelected = stepID
		a.scrollToStepID = stepID
	}
	sh.OnLocatePantry = func(itemID string) {
		if a.tools == nil {
			a.tools = a.newToolsWorkspace()
		}
		a.tools.LocatePantryItem(itemID)
		sh.OpenPantryTab()
	}
	sh.OnSwitchVaultPath = func(newPath string) {
		a.switchVaultRoot(newPath)
	}
	sh.OnUndo = func() { a.undoActiveLiveDoc() }
	sh.OnRedo = func() { a.redoActiveLiveDoc() }
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
	a.tools = a.newToolsWorkspace()
	if err := a.initVaultWorkspace(vaultRoot); err != nil {
		t.Fatalf("initVaultWorkspace(%s): %v", vaultRoot, err)
	}
	return a, sh
}

func assertBoundedFrame(t *testing.T, st *ui.Tester, label string) {
	t.Helper()
	start := time.Now()
	st.Frame()
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("UI frame %q exceeded 2s freeze budget (%v)", label, elapsed)
	}
}

func assertUniquePaneTreeIDs(t *testing.T, root *shell.PaneNode) {
	t.Helper()
	seenPanes := map[string]bool{}
	seenTabs := map[string]bool{}
	var walk func(n *shell.PaneNode)
	walk = func(n *shell.PaneNode) {
		if n == nil {
			return
		}
		if n.ID == "" || seenPanes[n.ID] {
			t.Fatalf("duplicate or empty pane ID %q in pane tree", n.ID)
		}
		seenPanes[n.ID] = true
		if n.First == nil && n.Second == nil {
			activeFound := false
			for _, tab := range n.Tabs {
				if tab.ID == "" || seenTabs[tab.ID] {
					t.Fatalf("duplicate or empty tab ID %q (title=%q file=%q) in pane %q", tab.ID, tab.Title, tab.FilePath, n.ID)
				}
				seenTabs[tab.ID] = true
				if tab.ID == n.ActiveTabID {
					activeFound = true
				}
			}
			if len(n.Tabs) > 0 && !activeFound {
				t.Fatalf("pane %q ActiveTabID=%q not found in tabs %+v", n.ID, n.ActiveTabID, n.Tabs)
			}
			return
		}
		walk(n.First)
		walk(n.Second)
	}
	walk(root)
}

// TestFSM_CorruptWorkspaceRecoveryAndMultiDocOpening reproduces the exact real-world
// bug where restored duplicate tab IDs (`file-1`, `file-1`, `file-2`) and stale tabs
// from another Vault jammed opening two new experiment documents.
func TestFSM_CorruptWorkspaceRecoveryAndMultiDocOpening(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), "CorruptRecoveryVault")
	if err := os.MkdirAll(filepath.Join(vaultRoot, ".asterlab"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(vaultRoot, "Fig3.5-抗菌实验"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Fig3.5-抗菌实验", "Van-Lys-液体共培养.asterml"), []byte(fsmTestDocAlpha), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Fig3.5-抗菌实验", "Van-Lys-滤纸片抑菌圈.asterml"), []byte(fsmTestDocBeta), 0o644); err != nil {
		t.Fatal(err)
	}
	// Plant the exact corrupt workspace.json pattern observed on user's machine:
	// duplicate `file-1` IDs + missing files leaked from a previous Vault + existing `file-2`.
	corruptWS := `{
  "rootPane": {
    "id": "pane-root",
    "ratio": 50,
    "tabs": [
      {"id": "calendar", "kind": 0, "title": "实验日程", "pinned": true},
      {"id": "file-1", "kind": 1, "title": "CCK-8细胞增殖活力测定", "filePath": "templates/CCK-8细胞增殖活力测定.asterml"},
      {"id": "pantry", "kind": 2, "title": "储藏室"},
      {"id": "file-1", "kind": 1, "title": "111", "filePath": "111.asterml"},
      {"id": "file-2", "kind": 1, "title": "空白实验", "filePath": "templates/空白实验.asterml"}
    ],
    "activeTabId": "file-1"
  },
  "activePaneId": "pane-root",
  "rightOpen": true
}`
	if err := os.WriteFile(filepath.Join(vaultRoot, ".asterlab", "workspace.json"), []byte(corruptWS), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	assertUniquePaneTreeIDs(t, sh.RootPane)
	// Stale missing files (CCK-8, 111.asterml) must be pruned automatically.
	for _, tab := range sh.ActivePane().Tabs {
		if tab.FilePath == "111.asterml" || strings.Contains(tab.FilePath, "CCK-8") {
			t.Fatalf("stale missing file tab was not pruned on vault init: %+v", tab)
		}
	}

	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)
	assertBoundedFrame(t, st, "initial-clean-frame")

	// 1. Open Experiment #1 by clicking it in the left File Tree.
	st.Click("Van-Lys-液体共培养")
	assertBoundedFrame(t, st, "open-doc-1")
	assertUniquePaneTreeIDs(t, sh.RootPane)
	if tab := sh.ActiveTab(); tab == nil || tab.FilePath != "Fig3.5-抗菌实验/Van-Lys-液体共培养.asterml" {
		t.Fatalf("expected active tab to be doc 1, got: %+v", tab)
	}
	if !st.HasText("Van-Lys 液体共培养涂板计数计划") || !st.HasText("共培养孵育计时") {
		t.Fatalf("expected doc 1 body to render cleanly, got: %q", st.Texts())
	}

	// 2. Open Experiment #2 by clicking it in the left File Tree — MUST NOT freeze or stick on doc 1!
	st.Click("Van-Lys-滤纸片抑菌圈")
	assertBoundedFrame(t, st, "open-doc-2")
	assertUniquePaneTreeIDs(t, sh.RootPane)
	if tab := sh.ActiveTab(); tab == nil || tab.FilePath != "Fig3.5-抗菌实验/Van-Lys-滤纸片抑菌圈.asterml" {
		t.Fatalf("expected active tab to be doc 2, got: %+v", tab)
	}
	if !st.HasText("Van-Lys 滤纸片扩散抑菌圈计划") || !st.HasText("贴片质控清单") {
		t.Fatalf("expected doc 2 body to render cleanly, got: %q", st.Texts())
	}

	// 3. Rapidly switch back and forth between Tab 1 and Tab 2 (10 cycles) — transition-pair stress test.
	var tab1ID, tab2ID string
	for _, tab := range sh.ActivePane().Tabs {
		if tab.FilePath == "Fig3.5-抗菌实验/Van-Lys-液体共培养.asterml" {
			tab1ID = tab.ID
		}
		if tab.FilePath == "Fig3.5-抗菌实验/Van-Lys-滤纸片抑菌圈.asterml" {
			tab2ID = tab.ID
		}
	}
	if tab1ID == "" || tab2ID == "" || tab1ID == tab2ID {
		t.Fatalf("expected two distinct tab IDs for doc1 and doc2, got tab1=%q tab2=%q", tab1ID, tab2ID)
	}
	for i := 0; i < 10; i++ {
		sh.SwitchTabInPane(sh.ActivePaneID, tab1ID, true)
		assertBoundedFrame(t, st, fmt.Sprintf("rapid-switch-1-%d", i))
		if !st.HasText("Van-Lys 液体共培养涂板计数计划") {
			t.Fatalf("cycle %d: switched to tab1 but doc1 content missing", i)
		}
		sh.SwitchTabInPane(sh.ActivePaneID, tab2ID, true)
		assertBoundedFrame(t, st, fmt.Sprintf("rapid-switch-2-%d", i))
		if !st.HasText("Van-Lys 滤纸片扩散抑菌圈计划") {
			t.Fatalf("cycle %d: switched to tab2 but doc2 content missing", i)
		}
	}

	// 4. Re-opening an already-open doc via Calendar / Command Palette must reuse its tab (no duplicates).
	countBefore := len(sh.ActivePane().Tabs)
	a.openVaultFileInActivePane("Fig3.5-抗菌实验/Van-Lys-液体共培养.asterml", "")
	assertBoundedFrame(t, st, "reopen-existing-doc-1")
	if len(sh.ActivePane().Tabs) != countBefore {
		t.Fatalf("expected re-opening existing doc to reuse tab (count %d), got %d", countBefore, len(sh.ActivePane().Tabs))
	}
	if sh.ActiveTab().ID != tab1ID {
		t.Fatalf("expected focus to jump to existing tab1 (%q), got %q", tab1ID, sh.ActiveTab().ID)
	}
}

// TestFSM_SplitPaneModesHistoryAndPersistence validates the hierarchical FSM transitions:
// split panes, side-by-side rendering of 2 heavy documents, mode switches (Edit/Lab/Source),
// Back/Forward history, tab close/reorder/pin persistence, and invalid date blur stability.
func TestFSM_SplitPaneModesHistoryAndPersistence(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), "SplitFSMVault")
	if err := os.MkdirAll(filepath.Join(vaultRoot, "Proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "DocA.asterml"), []byte(fsmTestDocAlpha), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "DocB.asterml"), []byte(fsmTestDocBeta), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)

	// Open DocA in pane 1, split right with DocB in pane 2.
	sh.OpenFileInPane(sh.ActivePaneID, "Proj/DocA.asterml", "DocA", shell.ModeEdit, false)
	pane1ID := sh.ActivePaneID
	sh.SplitPaneWithTab(pane1ID, shell.WorkspaceTab{
		Kind:     shell.TabFile,
		Title:    "DocB",
		FilePath: "Proj/DocB.asterml",
		Mode:     shell.ModeEdit,
	}, "right")
	pane2ID := sh.ActivePaneID
	if pane1ID == pane2ID {
		t.Fatal("expected split pane to create distinct active pane ID")
	}
	assertUniquePaneTreeIDs(t, sh.RootPane)
	assertBoundedFrame(t, st, "split-both-docs-edit")

	// Both documents must be rendered simultaneously side-by-side.
	if !st.HasText("Van-Lys 液体共培养涂板计数计划") || !st.HasText("Van-Lys 滤纸片扩散抑菌圈计划") {
		t.Fatalf("expected both split-pane docs rendered side-by-side, got: %q", st.Texts())
	}

	// Cycle all 3 modes (Edit -> Lab -> Source -> Edit) on Pane 2 without hanging or losing content.
	sh.SetActiveTabMode(shell.ModeLab)
	assertBoundedFrame(t, st, "pane2-lab-mode")
	sh.SetActiveTabMode(shell.ModeSource)
	assertBoundedFrame(t, st, "pane2-source-mode")
	if !st.HasText("高级源码视图") {
		t.Fatalf("expected source mode badge when in ModeSource")
	}
	sh.SetActiveTabMode(shell.ModeEdit)
	assertBoundedFrame(t, st, "pane2-back-to-edit")

	// Back / Forward navigation works cleanly across modes & tabs.
	if !sh.CanBack() {
		t.Fatal("expected Back history available after mode transitions")
	}
	sh.GoBack()
	assertBoundedFrame(t, st, "history-back")
	if sh.ActiveTab().Mode != shell.ModeSource {
		t.Fatalf("expected GoBack to restore ModeSource, got %v", sh.ActiveTab().Mode)
	}
	sh.GoForward()
	assertBoundedFrame(t, st, "history-forward")
	if sh.ActiveTab().Mode != shell.ModeEdit {
		t.Fatalf("expected GoForward to restore ModeEdit, got %v", sh.ActiveTab().Mode)
	}

	// Invalid date draft must NOT trigger an infinite render-time disk-write loop.
	if a.liveDateDrafts == nil {
		a.liveDateDrafts = map[string]*string{}
	}
	badDate := "2026/11"
	a.liveDateDrafts["Proj/DocB.asterml"] = &badDate
	ldB, err := a.docStore.Open("Proj/DocB.asterml")
	if err != nil {
		t.Fatal(err)
	}
	beforeSrc := ldB.Source
	for i := 0; i < 4; i++ {
		assertBoundedFrame(t, st, fmt.Sprintf("invalid-date-blur-frame-%d", i))
	}
	if ldB.Source != beforeSrc {
		t.Fatalf("invalid date draft should not corrupt or endlessly rewrite document source")
	}
	if *a.liveDateDrafts["Proj/DocB.asterml"] != "2026-11-25" {
		t.Fatalf("expected invalid unfocused date draft to snap back to docDate, got %q", *a.liveDateDrafts["Proj/DocB.asterml"])
	}

	// Closing DocB collapses split pane cleanly and persists to workspace.json (no zombie tabs on restart!).
	docBTabID := sh.ActiveTab().ID
	sh.CloseTabInPane(pane2ID, docBTabID)
	assertBoundedFrame(t, st, "close-split-pane")
	assertUniquePaneTreeIDs(t, sh.RootPane)
	if sh.SplitOpen {
		t.Fatal("expected split to collapse after closing the only tab in pane 2")
	}

	// Simulate cold restart on the same Vault: DocB must NOT resurrect, and new tabs get unique IDs.
	_, sh2 := newWiredFSMTestApp(t, vaultRoot)
	assertUniquePaneTreeIDs(t, sh2.RootPane)
	for _, tab := range sh2.ActivePane().Tabs {
		if tab.FilePath == "Proj/DocB.asterml" {
			t.Fatalf("closed tab DocB resurrected after restart: %+v", tab)
		}
	}
	sh2.OpenFileInPane(sh2.ActivePaneID, "Proj/DocB.asterml", "DocB", shell.ModeEdit, false)
	assertUniquePaneTreeIDs(t, sh2.RootPane)
}

// TestFSM_AllSurfacesDialogsAndVaultSwitch exercises Calendar, Command Palette,
// Global Search, Right Drawer, Pantry dialogs/boxes, Settings, Feedback, and clean Vault switching.
func TestFSM_AllSurfacesDialogsAndVaultSwitch(t *testing.T) {
	vault1 := filepath.Join(t.TempDir(), "VaultOne")
	vault2 := filepath.Join(t.TempDir(), "VaultTwo")
	if err := os.MkdirAll(filepath.Join(vault1, "抗菌课题"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault1, "抗菌课题", "实验甲.asterml"), []byte(fsmTestDocAlpha), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault1, "抗菌课题", "实验乙.asterml"), []byte(fsmTestDocBeta), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vault1)
	shelf := uint32(180)
	if _, err := a.localUpsertPantry(runs.PantryUpsert{
		Name: "Vancomycin Stock", Location: "-20°C", PurchasedOn: "2026-11-20", ShelfLifeDays: &shelf,
	}); err != nil {
		t.Fatal(err)
	}
	_ = a.savePantryBoxes([]tools.PantryBox{{
		ID: "box-1", Name: "抗菌冻存盒 #1", Kind: tools.BoxCryo9x9, ZoneKey: "-80", Rows: 9, Cols: 9,
	}})
	a.tools.ResetPantryState()
	a.syncPantrySnapshotToShell()

	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)
	assertBoundedFrame(t, st, "calendar-home")

	// 1. Command Palette opens & searches cleanly.
	sh.PaletteOpen = true
	sh.PaletteQuery = "抑菌圈"
	assertBoundedFrame(t, st, "command-palette-open")
	if !st.HasText("Van-Lys 滤纸片扩散抑菌圈计划") {
		t.Fatalf("expected Command Palette to surface matching experiment, got: %q", st.Texts())
	}
	sh.PaletteOpen = false
	sh.PaletteQuery = ""
	assertBoundedFrame(t, st, "command-palette-close")

	// 2. Global Search modal finds keyword across Vault & clicking hit opens the document.
	sh.GlobalSearchOpen = true
	sh.GlobalSearchQuery = "贴片质控清单"
	assertBoundedFrame(t, st, "global-search-open")
	if !st.HasText("Van-Lys 滤纸片扩散抑菌圈计划") {
		t.Fatalf("expected Global Search hit for 贴片质控清单, got: %q", st.Texts())
	}
	st.Click("Van-Lys 滤纸片扩散抑菌圈计划")
	assertBoundedFrame(t, st, "global-search-click-hit")
	if sh.GlobalSearchOpen {
		t.Fatal("expected Global Search modal to close after clicking hit")
	}
	if tab := sh.ActiveTab(); tab == nil || tab.FilePath != "抗菌课题/实验乙.asterml" {
		t.Fatalf("expected Global Search click to open 实验乙.asterml, got: %+v", tab)
	}

	// 3. Right Companion Drawer switches all 3 tabs cleanly.
	sh.SetRightDrawer(true, 0)
	assertBoundedFrame(t, st, "right-drawer-outline")
	sh.SetRightDrawer(true, 1)
	assertBoundedFrame(t, st, "right-drawer-calc")
	sh.SetRightDrawer(true, 2)
	assertBoundedFrame(t, st, "right-drawer-links-diag")
	sh.SetRightDrawer(false, 0)

	// 4. Pantry tab: bottle detail dialog & box interior drill-down work cleanly.
	sh.OpenPantryTab()
	assertBoundedFrame(t, st, "pantry-tab")
	st.Click("Vancomycin Stock")
	assertBoundedFrame(t, st, "pantry-detail-open")
	if !a.tools.PantryDialogOpen || !st.HasText("保存试剂档案") {
		t.Fatalf("expected Pantry detail dialog rendered on click, open=%v", a.tools.PantryDialogOpen)
	}
	a.tools.PantryDialogOpen = false
	assertBoundedFrame(t, st, "pantry-detail-close")

	// Drill down into box interior and back.
	a.tools.OpenPantryBox("box-1", "A1")
	assertBoundedFrame(t, st, "pantry-box-interior")
	if !st.HasText("← 返回货架") || !st.HasText("抗菌冻存盒 #1") {
		t.Fatalf("expected box interior view rendered")
	}
	st.Click("← 返回货架")
	assertBoundedFrame(t, st, "pantry-box-back")

	// 5. Settings & Feedback dialogs open and close without error.
	sh.VaultSettingsOpen = true
	assertBoundedFrame(t, st, "settings-open")
	sh.VaultSettingsOpen = false
	sh.FeedbackOpen = true
	assertBoundedFrame(t, st, "feedback-open")
	sh.FeedbackOpen = false
	assertBoundedFrame(t, st, "dialogs-closed")

	// 6. Switch to brand-new Vault 2: ZERO leaked tabs, ZERO leaked pantry, ZERO leaked outline/history.
	a.switchVaultRoot(vault2)
	assertBoundedFrame(t, st, "switched-to-vault2")
	assertUniquePaneTreeIDs(t, sh.RootPane)
	if len(sh.ActivePane().Tabs) != 1 || sh.ActivePane().Tabs[0].Kind != shell.TabCalendar {
		t.Fatalf("expected fresh Vault2 to start cleanly with only Calendar tab, got: %+v", sh.ActivePane().Tabs)
	}
	if len(a.tools.Pantry.Items) != 0 || len(sh.PantryItems) != 0 {
		t.Fatalf("expected Vault2 pantry isolated, tools=%d shell=%d", len(a.tools.Pantry.Items), len(sh.PantryItems))
	}
	if len(sh.OutlineSteps) != 0 || sh.CurrentDocRelPath != "" {
		t.Fatalf("expected Vault2 companion state cleared, outline=%d doc=%q", len(sh.OutlineSteps), sh.CurrentDocRelPath)
	}
}

// TestFSM_RealLocalTwoAntimicrobialDocsSmoke copies the user's real two Round-3
// `.asterml` plans (when present on this workstation) into an isolated temp Vault
// and exercises full multi-tab + side-by-side split + mode switches with strict latency checks.
func TestFSM_RealLocalTwoAntimicrobialDocsSmoke(t *testing.T) {
	realDir := `D:\L\AsterLab\Fig3.5-抗菌实验`
	entries, err := os.ReadDir(realDir)
	if err != nil {
		t.Skip("local user vault Fig3.5-抗菌实验 not present on this machine")
	}
	tmpVault := filepath.Join(t.TempDir(), "RealDocsMirror")
	dstDir := filepath.Join(tmpVault, "Fig3.5-抗菌实验")
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var rels []string
	for _, de := range entries {
		if de.IsDir() || !strings.HasSuffix(strings.ToLower(de.Name()), ".asterml") {
			continue
		}
		raw, rErr := os.ReadFile(filepath.Join(realDir, de.Name()))
		if rErr != nil {
			t.Fatal(rErr)
		}
		if wErr := os.WriteFile(filepath.Join(dstDir, de.Name()), raw, 0o644); wErr != nil {
			t.Fatal(wErr)
		}
		rels = append(rels, "Fig3.5-抗菌实验/"+de.Name())
	}
	if len(rels) < 2 {
		t.Skip("fewer than 2 .asterml files in local Fig3.5-抗菌实验")
	}

	_, sh := newWiredFSMTestApp(t, tmpVault)
	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1440, 900)
	assertBoundedFrame(t, st, "real-vault-init")

	// Open both real docs in tabs, switch back & forth, then open side-by-side in split panes.
	sh.OpenFileInPane(sh.ActivePaneID, rels[0], "", shell.ModeEdit, false)
	assertBoundedFrame(t, st, "real-doc-0")
	sh.OpenFileInPane(sh.ActivePaneID, rels[1], "", shell.ModeEdit, false)
	assertBoundedFrame(t, st, "real-doc-1")
	assertUniquePaneTreeIDs(t, sh.RootPane)

	sh.SplitPaneWithTab(sh.ActivePaneID, shell.WorkspaceTab{
		Kind:     shell.TabFile,
		Title:    filepath.Base(rels[0]),
		FilePath: rels[0],
		Mode:     shell.ModeEdit,
	}, "right")
	sh.SetRightDrawer(true, 0)
	assertBoundedFrame(t, st, "real-docs-split-with-drawer")
	assertUniquePaneTreeIDs(t, sh.RootPane)
}

func collectLeafPanes(n *shell.PaneNode, out *[]*shell.PaneNode) {
	if n == nil {
		return
	}
	if n.First == nil && n.Second == nil {
		*out = append(*out, n)
		return
	}
	collectLeafPanes(n.First, out)
	collectLeafPanes(n.Second, out)
}

func assertGlobalShellInvariants(t *testing.T, sh *shell.Shell, stepDesc string) {
	t.Helper()
	assertUniquePaneTreeIDs(t, sh.RootPane)
	var leaves []*shell.PaneNode
	collectLeafPanes(sh.RootPane, &leaves)
	if len(leaves) == 0 {
		t.Fatalf("[%s] RootPane has 0 leaf panes", stepDesc)
	}
	activePaneFound := false
	calCount := 0
	pantryCount := 0
	for _, p := range leaves {
		if p.ID == sh.ActivePaneID {
			activePaneFound = true
		}
		if len(p.Tabs) == 0 {
			t.Fatalf("[%s] empty leaf pane %q was not collapsed", stepDesc, p.ID)
		}
		for _, tab := range p.Tabs {
			if tab.Kind == shell.TabCalendar {
				calCount++
			}
			if tab.Kind == shell.TabPantry {
				pantryCount++
			}
		}
	}
	if !activePaneFound {
		t.Fatalf("[%s] ActivePaneID %q not found among %d leaves", stepDesc, sh.ActivePaneID, len(leaves))
	}
	if calCount != 1 {
		t.Fatalf("[%s] expected exactly 1 TabCalendar across tree, got %d", stepDesc, calCount)
	}
	if pantryCount > 1 {
		t.Fatalf("[%s] expected at most 1 TabPantry across tree, got %d", stepDesc, pantryCount)
	}
	if len(sh.History) > 48 {
		t.Fatalf("[%s] navigation history exceeded cap 48: %d", stepDesc, len(sh.History))
	}
	if len(sh.History) > 0 && (sh.HistoryIdx < 0 || sh.HistoryIdx >= len(sh.History)) {
		t.Fatalf("[%s] invalid HistoryIdx=%d for len=%d", stepDesc, sh.HistoryIdx, len(sh.History))
	}
}

// TestFSM_StatefulActionSequenceFuzzer implements industry-standard Model-Based
// Stateful Fuzzing: executes 160 deterministic pseudo-random combinations of user
// actions (multi-doc open/switch/split/close/reorder/pin/move, mode toggles, history,
// dialogs, pantry CRUD, external file changes, cold restarts, and Vault switches),
// verifying ALL global invariants and live UI rendering after every single transition.
func TestFSM_StatefulActionSequenceFuzzer(t *testing.T) {
	vaultA := filepath.Join(t.TempDir(), "FuzzVaultA")
	vaultB := filepath.Join(t.TempDir(), "FuzzVaultB")
	for _, dir := range []string{
		filepath.Join(vaultA, "Proj"),
		filepath.Join(vaultB, "Proj"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	docsA := []string{"Proj/Alpha.asterml", "Proj/Beta.asterml", "Proj/Gamma.asterml"}
	docsB := []string{"Proj/Delta.asterml", "Proj/Epsilon.asterml"}
	for i, rel := range docsA {
		src := fsmTestDocAlpha
		if i%2 == 1 {
			src = fsmTestDocBeta
		}
		if err := os.WriteFile(filepath.Join(vaultA, filepath.FromSlash(rel)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for i, rel := range docsB {
		src := fsmTestDocBeta
		if i%2 == 1 {
			src = fsmTestDocAlpha
		}
		if err := os.WriteFile(filepath.Join(vaultB, filepath.FromSlash(rel)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	const seed int64 = 20261123
	rng := rand.New(rand.NewSource(seed))
	a, sh := newWiredFSMTestApp(t, vaultA)
	curVault := vaultA
	curDocs := docsA

	makeTester := func() *ui.Tester {
		return ui.NewTester(func(c *ui.Context) {
			sh.RenderSplitView(c)
		}, 1280, 800)
	}
	st := makeTester()
	var trace []string

	for step := 0; step < 160; step++ {
		var leaves []*shell.PaneNode
		collectLeafPanes(sh.RootPane, &leaves)
		pane := leaves[rng.Intn(len(leaves))]
		tab := pane.Tabs[rng.Intn(len(pane.Tabs))]
		docRel := curDocs[rng.Intn(len(curDocs))]

		op := rng.Intn(17)
		desc := ""
		switch op {
		case 0: // OpenFile (reuse or force new)
			forceNew := rng.Intn(4) == 0
			desc = fmt.Sprintf("step=%d OpenFile(pane=%s file=%s forceNew=%v)", step, pane.ID, docRel, forceNew)
			sh.OpenFileInPane(pane.ID, docRel, "", sh.DefaultFileOpenMode(), forceNew)
		case 1: // SwitchTab
			desc = fmt.Sprintf("step=%d SwitchTab(pane=%s tab=%s)", step, pane.ID, tab.ID)
			sh.SwitchTabInPane(pane.ID, tab.ID, true)
		case 2: // SplitPane (cap leaves <= 3 for realistic desktop layout)
			if len(leaves) < 3 {
				dirs := []string{"right", "down", "left", "up"}
				dir := dirs[rng.Intn(len(dirs))]
				desc = fmt.Sprintf("step=%d SplitPane(pane=%s file=%s dir=%s)", step, pane.ID, docRel, dir)
				sh.SplitPaneWithTab(pane.ID, shell.WorkspaceTab{
					Kind: shell.TabFile, Title: filepath.Base(docRel), FilePath: docRel, Mode: sh.DefaultFileOpenMode(),
				}, dir)
			} else {
				desc = fmt.Sprintf("step=%d OpenCalendarTab", step)
				sh.OpenCalendarTab()
			}
		case 3: // CloseTab
			desc = fmt.Sprintf("step=%d CloseTab(pane=%s tab=%s)", step, pane.ID, tab.ID)
			sh.CloseTabInPane(pane.ID, tab.ID)
		case 4: // CloseOtherTabs
			desc = fmt.Sprintf("step=%d CloseOtherTabs(pane=%s keep=%s)", step, pane.ID, tab.ID)
			sh.CloseOtherTabs(pane.ID, tab.ID)
		case 5: // CloseTabsToRight
			desc = fmt.Sprintf("step=%d CloseTabsToRight(pane=%s from=%s)", step, pane.ID, tab.ID)
			sh.CloseTabsToRight(pane.ID, tab.ID)
		case 6: // ReorderTab
			targetIdx := rng.Intn(len(pane.Tabs))
			desc = fmt.Sprintf("step=%d ReorderTab(pane=%s tab=%s to=%d)", step, pane.ID, tab.ID, targetIdx)
			sh.ReorderTabInPane(pane.ID, tab.ID, targetIdx)
		case 7: // TogglePinTab (skip calendar so it stays pinned)
			if tab.Kind != shell.TabCalendar {
				desc = fmt.Sprintf("step=%d TogglePin(pane=%s tab=%s)", step, pane.ID, tab.ID)
				sh.TogglePinTab(pane.ID, tab.ID)
			} else {
				desc = fmt.Sprintf("step=%d OpenPantryTab", step)
				sh.OpenPantryTab()
			}
		case 8: // MoveOrSplitTab across panes
			dst := leaves[rng.Intn(len(leaves))]
			zones := []string{"center", "right", "down"}
			zone := zones[rng.Intn(len(zones))]
			if len(leaves) >= 3 {
				zone = "center"
			}
			desc = fmt.Sprintf("step=%d MoveOrSplitTab(src=%s tab=%s dst=%s zone=%s)", step, pane.ID, tab.ID, dst.ID, zone)
			sh.MoveOrSplitTab(pane.ID, tab.ID, dst.ID, zone)
		case 9: // Switch active file view mode (Edit / Lab / Source)
			modes := []shell.ViewMode{shell.ModeEdit, shell.ModeLab, shell.ModeSource}
			m := modes[rng.Intn(len(modes))]
			desc = fmt.Sprintf("step=%d SetActiveTabMode(%d)", step, m)
			sh.SetActiveTabMode(m)
		case 10: // Back / Forward navigation
			if rng.Intn(2) == 0 {
				desc = fmt.Sprintf("step=%d GoBack", step)
				sh.GoBack()
			} else {
				desc = fmt.Sprintf("step=%d GoForward", step)
				sh.GoForward()
			}
		case 11: // Toggle Right Drawer & tab
			open := rng.Intn(2) == 0
			rt := rng.Intn(3)
			desc = fmt.Sprintf("step=%d SetRightDrawer(open=%v tab=%d)", step, open, rt)
			sh.SetRightDrawer(open, rt)
		case 12: // Toggle modals/dialogs cleanly (only one modal open at a time)
			sh.PaletteOpen = false
			sh.GlobalSearchOpen = false
			sh.VaultSettingsOpen = false
			sh.FeedbackOpen = false
			a.newExpOpen = false
			switch rng.Intn(5) {
			case 0:
				sh.PaletteOpen = true
				desc = fmt.Sprintf("step=%d OpenPalette", step)
			case 1:
				sh.GlobalSearchOpen = true
				sh.GlobalSearchQuery = "Van"
				desc = fmt.Sprintf("step=%d OpenGlobalSearch", step)
			case 2:
				sh.VaultSettingsOpen = true
				desc = fmt.Sprintf("step=%d OpenSettings", step)
			case 3:
				a.openNewExperimentDialog("Proj", "2026-11-24")
				desc = fmt.Sprintf("step=%d OpenNewExpDialog", step)
			default:
				desc = fmt.Sprintf("step=%d CloseAllModals", step)
			}
		case 13: // Pantry CRUD + Pantry Tab
			desc = fmt.Sprintf("step=%d PantryUpsertAndOpen", step)
			_, _ = a.localUpsertPantry(runs.PantryUpsert{
				Name:     fmt.Sprintf("Reagent-%d", step),
				Location: "-20°C",
			})
			a.tools.ReloadPantry()
			a.syncPantrySnapshotToShell()
			sh.OpenPantryTab()
		case 14: // External Agent disk touch + watcher sync
			desc = fmt.Sprintf("step=%d ExternalDiskTouch", step)
			p := filepath.Join(curVault, filepath.FromSlash(docRel))
			future := time.Now().Add(time.Duration(step+1) * time.Second)
			_ = os.Chtimes(p, future, future)
			a.syncVaultDiskChangesNow()
		case 15: // Cold restart on current Vault (tests workspace.json roundtrip & ID counter sync!)
			desc = fmt.Sprintf("step=%d ColdRestart(vault=%s)", step, filepath.Base(curVault))
			a.saveVaultWorkspace()
			a, sh = newWiredFSMTestApp(t, curVault)
			st = makeTester()
		case 16: // Switch Vault A <-> Vault B
			if curVault == vaultA {
				curVault, curDocs = vaultB, docsB
			} else {
				curVault, curDocs = vaultA, docsA
			}
			desc = fmt.Sprintf("step=%d SwitchVault(%s)", step, filepath.Base(curVault))
			a.switchVaultRoot(curVault)
		}
		trace = append(trace, desc)

		assertGlobalShellInvariants(t, sh, desc)
		start := time.Now()
		st.Frame()
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("seed=%d froze at %s (%v)\nRecent trace:\n%s", seed, desc, elapsed, strings.Join(trace[max(0, len(trace)-15):], "\n"))
		}
		// Close transient modals before next step so clicks/shortcuts aren't stuck behind an old modal.
		sh.PaletteOpen = false
		sh.GlobalSearchOpen = false
		sh.VaultSettingsOpen = false
		sh.FeedbackOpen = false
		a.newExpOpen = false
	}
}
