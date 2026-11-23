package shell

import (
	"fmt"
	"strconv"
	"strings"
)

// ViewMode distinguishes the 3 unified document modes inside a file tab.
type ViewMode uint8

const (
	ModeEdit ViewMode = iota
	ModeLab
	ModeSource
	// ModeRead is a compatibility alias for ModeLab (anti-accidental-edit lab execution mode).
	ModeRead = ModeLab
)

// TabKind distinguishes Obsidian workspace tab surfaces.
type TabKind uint8

const (
	TabCalendar TabKind = iota
	TabFile
	TabPantry
	// Legacy aliases preserved for backward test/caller compatibility.
	TabExperiment = TabFile
	TabTools      = TabPantry
)

// WorkspaceTab represents one open workspace tab inside a PaneNode leaf.
type WorkspaceTab struct {
	ID        string          `json:"id"`
	Kind      TabKind         `json:"kind"`
	Title     string          `json:"title"`
	FilePath  string          `json:"filePath,omitempty"` // vault-relative `.asterml` path
	DesignID  string          `json:"designId,omitempty"`
	RunID     string          `json:"runId,omitempty"`
	Mode      ViewMode        `json:"mode,omitempty"`
	Stage     Page            `json:"stage,omitempty"`
	StageLock ExperimentStage `json:"stageLock,omitempty"`
	Pinned    bool            `json:"pinned,omitempty"`
}

// PaneNode is a recursive peer split-tree node: either a SplitBranch (First/Second)
// or a leaf TabGroup (Tabs + ActiveTabID). Every leaf pane is completely equal.
type PaneNode struct {
	ID          string         `json:"id"`
	Vertical    bool           `json:"vertical,omitempty"` // false = left/right, true = top/bottom
	Ratio       float32        `json:"ratio,omitempty"`    // percent (10..90), default 50
	First       *PaneNode      `json:"first,omitempty"`
	Second      *PaneNode      `json:"second,omitempty"`
	Tabs        []WorkspaceTab `json:"tabs,omitempty"`
	ActiveTabID string         `json:"activeTabId,omitempty"`
}

func (n *PaneNode) isLeaf() bool {
	return n != nil && n.First == nil && n.Second == nil
}

// NavEntry records one navigation step in the Back/Forward history stack.
type NavEntry struct {
	PaneID   string
	TabID    string
	FilePath string
	Mode     ViewMode
}

// QuickPantryItem is a lightweight projection of a pantry reagent for Quick Switcher & Palette.
type QuickPantryItem struct {
	ID            string
	Name          string
	Location      string
	Status        string // unopened, opened, near_expiry, expired, exhausted
	ExpiresOn     string
	LotNumber     string
	CatalogNumber string
}

// OutlineItem represents one chapter heading or executable step inside the focused document.
type OutlineItem struct {
	ID        string
	Title     string
	IsStep    bool
	Number    string
	Depth     int
	StepDepth int
	Meta      string
	Active    bool
}

// PaletteCommand represents a host-registered quick action inside Ctrl+K Command Palette.
type PaletteCommand struct {
	ID       string
	Title    string
	Subtitle string
	Shortcut string
	OnRun    func()
}

// RightCalcState stores live wet-lab quick calculator inputs inside the companion drawer.
type RightCalcState struct {
	Mode       int    // 0 = 稀释 (C1·V1 = C2·V2), 1 = 称量 (m = C·V·MW)
	Stock      string // default "100"
	StockUnit  string // ug/mL, mg/mL, mM, M
	Target     string // default "50"
	TargetUnit string // ng/mL, ug/mL, uM, mM
	FinalVol   string // default "4"
	VolUnit    string // mL, uL
	MolarMass  string // g/mol, default "1449.25"
}

// WorkspacePersistState serializes the Obsidian pane tree and sidebar visibility into `.asterlab/workspace.json`.
type WorkspacePersistState struct {
	RootPane      *PaneNode `json:"rootPane,omitempty"`
	ActivePaneID  string    `json:"activePaneId,omitempty"`
	LeftCollapsed bool      `json:"leftCollapsed,omitempty"`
	RightOpen     bool      `json:"rightOpen,omitempty"`
	RightTab      int       `json:"rightTab,omitempty"`
	CalendarMode  int       `json:"calendarMode,omitempty"`
}

const (
	calendarTabID = "calendar"
	pantryTabID   = "pantry"
	defaultPaneID = "pane-root"
)

func fileTabID(paneID, filePath string) string {
	return fmt.Sprintf("file:%s:%s", paneID, filePath)
}

func experimentTabID(exp ExperimentItem) string {
	if p := strings.TrimSpace(exp.FilePath); p != "" {
		return "file:" + p
	}
	if id := strings.TrimSpace(exp.DesignID); id != "" {
		return "exp:" + id
	}
	if id := strings.TrimSpace(exp.ActiveRunID); id != "" {
		return "exp-run:" + id
	}
	if id := strings.TrimSpace(exp.ID); id != "" {
		return "exp:" + id
	}
	return "exp:untitled"
}

func defaultStageForExperiment(exp ExperimentItem) Page {
	return Design
}

func normalizeExperimentStage(p Page) Page {
	return Design
}

// EnsureTabs guarantees RootPane exists with a pinned Calendar tab and keeps flat mirror fields synced.
func (s *Shell) EnsureTabs() {
	if s == nil {
		return
	}
	if s.SplitPct <= 0 {
		s.SplitPct = 50
	}
	if s.RootPane == nil {
		initialTabs := []WorkspaceTab{{
			ID:     calendarTabID,
			Kind:   TabCalendar,
			Title:  "实验日程",
			Pinned: true,
		}}
		activeID := calendarTabID
		if len(s.Tabs) > 0 {
			initialTabs = nil
			hasCal := false
			for _, t := range s.Tabs {
				if t.Kind == TabCalendar {
					t.Pinned = true
					hasCal = true
				}
				initialTabs = append(initialTabs, t)
			}
			if !hasCal {
				initialTabs = append([]WorkspaceTab{{
					ID:     calendarTabID,
					Kind:   TabCalendar,
					Title:  "实验日程",
					Pinned: true,
				}}, initialTabs...)
			}
			if s.ActiveTabID != "" {
				activeID = s.ActiveTabID
			}
		}
		s.RootPane = &PaneNode{
			ID:          defaultPaneID,
			Ratio:       50,
			Tabs:        initialTabs,
			ActiveTabID: activeID,
		}
		s.ActivePaneID = defaultPaneID
	}
	s.sanitizePaneTree()
	if s.findPane(s.ActivePaneID) == nil {
		if first := s.firstLeaf(s.RootPane); first != nil {
			s.ActivePaneID = first.ID
		}
	}
	// Guarantee at least one pinned calendar tab exists across the tree (or in first leaf when missing).
	if s.findTabInTree(calendarTabID) == nil {
		if leaf := s.firstLeaf(s.RootPane); leaf != nil {
			cal := WorkspaceTab{ID: calendarTabID, Kind: TabCalendar, Title: "实验日程", Pinned: true}
			leaf.Tabs = append([]WorkspaceTab{cal}, leaf.Tabs...)
			if leaf.ActiveTabID == "" {
				leaf.ActiveTabID = calendarTabID
			}
		}
	}
	s.syncLegacyMirror()
	if len(s.History) == 0 {
		if t := s.ActiveTab(); t != nil {
			s.History = []NavEntry{{PaneID: s.ActivePaneID, TabID: t.ID, FilePath: t.FilePath, Mode: t.Mode}}
			s.HistoryIdx = 0
		}
	}
}

// DefaultFileOpenMode returns the user's configured default mode for newly opened `.asterml` tabs.
func (s *Shell) DefaultFileOpenMode() ViewMode {
	if s != nil && s.DefaultOpenMode == "read" {
		return ModeRead
	}
	return ModeEdit
}

func parseTrailingSeq(id string) uint64 {
	idx := strings.LastIndexByte(id, '-')
	if idx < 0 || idx+1 >= len(id) {
		return 0
	}
	n, err := strconv.ParseUint(id[idx+1:], 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func findAnyPaneNodeRec(n *PaneNode, id string) *PaneNode {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	if l := findAnyPaneNodeRec(n.First, id); l != nil {
		return l
	}
	return findAnyPaneNodeRec(n.Second, id)
}

func (s *Shell) nextPaneID() string {
	for {
		s.paneSeq++
		id := fmt.Sprintf("pane-%d", s.paneSeq)
		if findAnyPaneNodeRec(s.RootPane, id) == nil {
			return id
		}
	}
}

func (s *Shell) nextTabID(prefix string) string {
	if prefix == "" {
		prefix = "tab"
	}
	for {
		s.tabSeq++
		id := fmt.Sprintf("%s-%d", prefix, s.tabSeq)
		if s.findTabInTree(id) == nil {
			return id
		}
	}
}

// sanitizePaneTree enforces strict global uniqueness of all pane IDs and tab IDs across
// RootPane, advances paneSeq/tabSeq past any restored numeric suffixes, deduplicates
// singleton tabs (calendar/pantry), and repairs dangling ActiveTabID references.
// Returns true if any structural corruption or duplicate ID was repaired.
func (s *Shell) sanitizePaneTree() bool {
	if s == nil || s.RootPane == nil {
		return false
	}
	repaired := false

	// Pass 1: advance sequence counters past every existing numeric suffix in the tree.
	var scanSeq func(n *PaneNode)
	scanSeq = func(n *PaneNode) {
		if n == nil {
			return
		}
		if seq := parseTrailingSeq(n.ID); seq > s.paneSeq {
			s.paneSeq = seq
		}
		if n.isLeaf() {
			for _, t := range n.Tabs {
				if seq := parseTrailingSeq(t.ID); seq > s.tabSeq {
					s.tabSeq = seq
				}
			}
			return
		}
		scanSeq(n.First)
		scanSeq(n.Second)
	}
	scanSeq(s.RootPane)

	// Pass 2: deduplicate pane IDs, tab IDs, and singleton calendar/pantry tabs.
	seenPanes := map[string]bool{}
	seenTabs := map[string]bool{}
	seenCalendar := false
	seenPantry := false

	var visit func(n *PaneNode)
	visit = func(n *PaneNode) {
		if n == nil {
			return
		}
		if strings.TrimSpace(n.ID) == "" || seenPanes[n.ID] {
			oldID := n.ID
			for {
				s.paneSeq++
				cand := fmt.Sprintf("pane-%d", s.paneSeq)
				if !seenPanes[cand] {
					n.ID = cand
					break
				}
			}
			if s.ActivePaneID == oldID && oldID != "" && !seenPanes[oldID] {
				s.ActivePaneID = n.ID
			}
			repaired = true
		}
		seenPanes[n.ID] = true

		if n.isLeaf() {
			cleaned := make([]WorkspaceTab, 0, len(n.Tabs))
			activeValid := false
			for _, tab := range n.Tabs {
				if tab.Kind == TabCalendar {
					if seenCalendar {
						repaired = true
						continue
					}
					seenCalendar = true
					if tab.ID != calendarTabID {
						tab.ID = calendarTabID
						repaired = true
					}
					tab.Pinned = true
				} else if tab.Kind == TabPantry {
					if seenPantry {
						repaired = true
						continue
					}
					seenPantry = true
					if tab.ID != pantryTabID {
						tab.ID = pantryTabID
						repaired = true
					}
				} else {
					if strings.TrimSpace(tab.ID) == "" || seenTabs[tab.ID] {
						wasActive := n.ActiveTabID == tab.ID && !activeValid
						for {
							s.tabSeq++
							cand := fmt.Sprintf("file-%d", s.tabSeq)
							if !seenTabs[cand] {
								tab.ID = cand
								break
							}
						}
						if wasActive {
							n.ActiveTabID = tab.ID
						}
						repaired = true
					}
				}
				seenTabs[tab.ID] = true
				if tab.ID == n.ActiveTabID {
					activeValid = true
				}
				cleaned = append(cleaned, tab)
			}
			n.Tabs = cleaned
			if !activeValid && len(n.Tabs) > 0 {
				n.ActiveTabID = n.Tabs[0].ID
				repaired = true
			}
			return
		}
		visit(n.First)
		visit(n.Second)
	}
	visit(s.RootPane)
	return repaired
}

// ResetWorkspaceForVault clears all open tabs, split panes, navigation history, and
// document-scoped companion state when switching to a different Vault.
func (s *Shell) ResetWorkspaceForVault() {
	if s == nil {
		return
	}
	s.RootPane = nil
	s.ActivePaneID = ""
	s.Tabs = nil
	s.ActiveTabID = ""
	s.History = nil
	s.HistoryIdx = -1
	s.SplitOpen = false
	s.SecondaryTab = nil
	s.paneSeq = 0
	s.tabSeq = 0
	s.Selected = ""
	s.SelectedTreePath = ""
	s.CurrentDocRelPath = ""
	s.OutlineSteps = nil
	s.CurrentBacklinks = nil
	s.CurrentDiagnostics = nil
	s.CurrentSnapshots = nil
	s.StatusSaveLabel = ""
	s.StatusMetaLabel = ""
	s.StatusWarnLabel = ""
	s.DayPopoverOpen = false
	s.PaletteOpen = false
	s.GlobalSearchOpen = false
	s.Error = ""
	s.EnsureTabs()
}

// PruneMissingFileTabs removes any TabFile referencing a file that does not exist in
// the active Vault, collapsing empty split panes and guaranteeing a valid active tab.
// Returns true if any stale tab was pruned.
func (s *Shell) PruneMissingFileTabs(exists func(relPath string) bool) bool {
	if s == nil || s.RootPane == nil || exists == nil {
		return false
	}
	type stale struct{ paneID, tabID string }
	var toDrop []stale
	var visit func(n *PaneNode)
	visit = func(n *PaneNode) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			for _, t := range n.Tabs {
				if t.Kind == TabFile && (strings.TrimSpace(t.FilePath) == "" || !exists(t.FilePath)) {
					toDrop = append(toDrop, stale{paneID: n.ID, tabID: t.ID})
				}
			}
			return
		}
		visit(n.First)
		visit(n.Second)
	}
	visit(s.RootPane)
	if len(toDrop) == 0 {
		return false
	}
	for _, st := range toDrop {
		pane := s.findPane(st.paneID)
		if pane == nil {
			continue
		}
		for i := range pane.Tabs {
			if pane.Tabs[i].ID == st.tabID {
				pane.Tabs[i].Pinned = false
				break
			}
		}
		s.CloseTabInPane(st.paneID, st.tabID)
	}
	s.EnsureTabs()
	return true
}

func (s *Shell) firstLeaf(n *PaneNode) *PaneNode {
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		return n
	}
	if l := s.firstLeaf(n.First); l != nil {
		return l
	}
	return s.firstLeaf(n.Second)
}

func (s *Shell) findPane(id string) *PaneNode {
	if id == "" || s.RootPane == nil {
		return nil
	}
	return findPaneRec(s.RootPane, id)
}

func findPaneRec(n *PaneNode, id string) *PaneNode {
	if n == nil {
		return nil
	}
	if n.isLeaf() && n.ID == id {
		return n
	}
	if l := findPaneRec(n.First, id); l != nil {
		return l
	}
	return findPaneRec(n.Second, id)
}

func (s *Shell) findTabInTree(tabID string) *WorkspaceTab {
	return findTabRec(s.RootPane, tabID)
}

func findTabRec(n *PaneNode, tabID string) *WorkspaceTab {
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		for i := range n.Tabs {
			if n.Tabs[i].ID == tabID {
				return &n.Tabs[i]
			}
		}
		return nil
	}
	if t := findTabRec(n.First, tabID); t != nil {
		return t
	}
	return findTabRec(n.Second, tabID)
}

func (s *Shell) leafCount() int {
	return countLeavesRec(s.RootPane)
}

func countLeavesRec(n *PaneNode) int {
	if n == nil {
		return 0
	}
	if n.isLeaf() {
		return 1
	}
	return countLeavesRec(n.First) + countLeavesRec(n.Second)
}

func (s *Shell) syncLegacyMirror() {
	pane := s.findPane(s.ActivePaneID)
	if pane == nil {
		pane = s.firstLeaf(s.RootPane)
	}
	if pane == nil {
		return
	}
	if pane.ActiveTabID == "" && len(pane.Tabs) > 0 {
		pane.ActiveTabID = pane.Tabs[0].ID
	}
	s.Tabs = pane.Tabs
	s.ActiveTabID = pane.ActiveTabID
	s.SplitOpen = s.leafCount() > 1
	if t := s.ActiveTab(); t != nil {
		switch t.Kind {
		case TabCalendar:
			s.Page = Project
		case TabPantry:
			s.Page = Tools
		default:
			s.Page = Design
			if t.FilePath != "" {
				s.Selected = t.FilePath
			} else if t.DesignID != "" {
				s.Selected = t.DesignID
			}
		}
	}
}

// ActivePane returns the focused leaf PaneNode.
func (s *Shell) ActivePane() *PaneNode {
	if s == nil {
		return nil
	}
	s.EnsureTabs()
	return s.findPane(s.ActivePaneID)
}

// ActiveTab returns the currently focused workspace tab.
func (s *Shell) ActiveTab() *WorkspaceTab {
	if s == nil || s.RootPane == nil {
		return nil
	}
	pane := s.findPane(s.ActivePaneID)
	if pane == nil {
		pane = s.firstLeaf(s.RootPane)
	}
	if pane == nil {
		return nil
	}
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == pane.ActiveTabID {
			return &pane.Tabs[i]
		}
	}
	if len(pane.Tabs) > 0 {
		pane.ActiveTabID = pane.Tabs[0].ID
		return &pane.Tabs[0]
	}
	return nil
}

func (s *Shell) pushHistory(paneID, tabID, filePath string, mode ViewMode) {
	if tabID == "" {
		return
	}
	if len(s.History) > 0 && s.HistoryIdx >= 0 && s.HistoryIdx < len(s.History) {
		cur := s.History[s.HistoryIdx]
		if cur.PaneID == paneID && cur.TabID == tabID && cur.FilePath == filePath && cur.Mode == mode {
			return
		}
		s.History = append(s.History[:s.HistoryIdx+1], NavEntry{
			PaneID: paneID, TabID: tabID, FilePath: filePath, Mode: mode,
		})
	} else {
		s.History = []NavEntry{{PaneID: paneID, TabID: tabID, FilePath: filePath, Mode: mode}}
	}
	if len(s.History) > 48 {
		s.History = s.History[len(s.History)-48:]
	}
	s.HistoryIdx = len(s.History) - 1
}

func (s *Shell) notifyTabChange(tab WorkspaceTab) {
	s.syncLegacyMirror()
	if s.OnActivateTab != nil {
		s.OnActivateTab(tab)
	}
	s.Invalidate()
}

// SwitchTabInPane activates tabID inside paneID and sets focus to paneID.
func (s *Shell) SwitchTabInPane(paneID, tabID string, recordHistory bool) {
	s.EnsureTabs()
	pane := s.findPane(paneID)
	if pane == nil {
		return
	}
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == tabID {
			pane.ActiveTabID = tabID
			s.ActivePaneID = pane.ID
			if recordHistory {
				s.pushHistory(pane.ID, tabID, pane.Tabs[i].FilePath, pane.Tabs[i].Mode)
			}
			s.notifyTabChange(pane.Tabs[i])
			return
		}
	}
}

// SwitchTab activates tabID in the active pane (or wherever it lives).
func (s *Shell) SwitchTab(tabID string, recordHistory bool) {
	s.EnsureTabs()
	if pane := s.findPane(s.ActivePaneID); pane != nil {
		for _, t := range pane.Tabs {
			if t.ID == tabID {
				s.SwitchTabInPane(pane.ID, tabID, recordHistory)
				return
			}
		}
	}
	if pane := findPaneWithTab(s.RootPane, tabID); pane != nil {
		s.SwitchTabInPane(pane.ID, tabID, recordHistory)
	}
}

func findPaneWithTab(n *PaneNode, tabID string) *PaneNode {
	if n == nil {
		return nil
	}
	if n.isLeaf() {
		for _, t := range n.Tabs {
			if t.ID == tabID {
				return n
			}
		}
		return nil
	}
	if l := findPaneWithTab(n.First, tabID); l != nil {
		return l
	}
	return findPaneWithTab(n.Second, idOrEmpty(tabID))
}

func idOrEmpty(s string) string { return s }

// OpenCalendarTab activates the Calendar tab in whichever pane holds it (or opens it in active pane).
func (s *Shell) OpenCalendarTab() {
	s.EnsureTabs()
	if pane := findPaneWithTab(s.RootPane, calendarTabID); pane != nil {
		s.SwitchTabInPane(pane.ID, calendarTabID, true)
		return
	}
	pane := s.ActivePane()
	if pane == nil {
		return
	}
	cal := WorkspaceTab{ID: calendarTabID, Kind: TabCalendar, Title: "实验日程", Pinned: true}
	pane.Tabs = append([]WorkspaceTab{cal}, pane.Tabs...)
	s.SwitchTabInPane(pane.ID, calendarTabID, true)
}

// OpenPantryTab opens or focuses the Storage Pantry tab.
func (s *Shell) OpenPantryTab() {
	s.EnsureTabs()
	if pane := findPaneWithTab(s.RootPane, pantryTabID); pane != nil {
		s.SwitchTabInPane(pane.ID, pantryTabID, true)
		return
	}
	pane := s.ActivePane()
	if pane == nil {
		return
	}
	pt := WorkspaceTab{ID: pantryTabID, Kind: TabPantry, Title: "储藏室"}
	pane.Tabs = append(pane.Tabs, pt)
	s.SwitchTabInPane(pane.ID, pantryTabID, true)
}

// OpenToolsTab aliases OpenPantryTab cleanly.
func (s *Shell) OpenToolsTab() {
	s.OpenPantryTab()
}

// OpenFileInPane opens a vault `.asterml` file inside paneID (focusing an existing tab for that
// file in the same pane unless forceNewTab is true). Two different panes CAN open the same file!
func (s *Shell) OpenFileInPane(paneID, filePath, title string, mode ViewMode, forceNewTab bool) {
	s.EnsureTabs()
	pane := s.findPane(paneID)
	if pane == nil {
		pane = s.ActivePane()
	}
	if pane == nil {
		return
	}
	if !forceNewTab {
		for i := range pane.Tabs {
			if pane.Tabs[i].Kind == TabFile && pane.Tabs[i].FilePath == filePath {
				if title != "" {
					pane.Tabs[i].Title = title
				}
				s.SwitchTabInPane(pane.ID, pane.Tabs[i].ID, true)
				return
			}
		}
	}
	tab := WorkspaceTab{
		ID:       s.nextTabID("file"),
		Kind:     TabFile,
		Title:    firstNonEmptyStr(title, fileStem(filePath), "未命名实验"),
		FilePath: filePath,
		Mode:     mode,
	}
	pane.Tabs = append(pane.Tabs, tab)
	s.SwitchTabInPane(pane.ID, tab.ID, true)
}

// OpenExperimentTab opens or focuses an experiment in the active pane.
func (s *Shell) OpenExperimentTab(exp ExperimentItem, stage Page) {
	s.EnsureTabs()
	path := strings.TrimSpace(exp.FilePath)
	if path == "" {
		path = exp.DesignID
	}
	s.OpenFileInPane(s.ActivePaneID, path, firstNonEmptyStr(exp.Title, "未命名实验"), ModeEdit, false)
	if stage != Project {
		s.Page = stage
	}
}

// SetActiveTabMode switches Edit / Read / Source mode on the focused tab.
func (s *Shell) SetActiveTabMode(mode ViewMode) {
	s.EnsureTabs()
	pane := s.ActivePane()
	if pane == nil {
		return
	}
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == pane.ActiveTabID && pane.Tabs[i].Kind == TabFile {
			pane.Tabs[i].Mode = mode
			s.pushHistory(pane.ID, pane.Tabs[i].ID, pane.Tabs[i].FilePath, mode)
			s.notifyTabChange(pane.Tabs[i])
			return
		}
	}
}

// SetActiveExperimentStage keeps compatibility with Cmd+2 shortcut mapping.
func (s *Shell) SetActiveExperimentStage(_ Page) {
	s.SetActiveTabMode(ModeEdit)
}

// SplitPaneWithTab splits targetPaneID along direction ("right"|"left"|"down"|"up")
// and opens tabCopy inside the newly created sibling pane.
func (s *Shell) SplitPaneWithTab(targetPaneID string, tabCopy WorkspaceTab, direction string) {
	s.EnsureTabs()
	target := s.findPane(targetPaneID)
	if target == nil {
		target = s.ActivePane()
	}
	if target == nil {
		return
	}
	// Calendar and Pantry are singleton surfaces: splitting them moves the existing
	// tab into the new split pane instead of creating duplicate Calendar/Pantry tabs.
	if tabCopy.Kind == TabCalendar || tabCopy.Kind == TabPantry {
		singletonID := calendarTabID
		if tabCopy.Kind == TabPantry {
			singletonID = pantryTabID
		}
		if src := findPaneWithTab(s.RootPane, singletonID); src != nil {
			if src.ID == target.ID && len(src.Tabs) <= 1 {
				return
			}
			s.MoveOrSplitTab(src.ID, singletonID, target.ID, direction)
			return
		}
		tabCopy.ID = singletonID
	}
	s.splitPaneNode(target, tabCopy, direction, false)
}

func (s *Shell) splitPaneNode(target *PaneNode, tabCopy WorkspaceTab, direction string, preservePin bool) {
	if target == nil {
		return
	}
	if tabCopy.Kind == TabCalendar {
		tabCopy.ID = calendarTabID
		tabCopy.Pinned = true
	} else if tabCopy.Kind == TabPantry {
		tabCopy.ID = pantryTabID
	} else {
		if tabCopy.ID == "" || s.findTabInTree(tabCopy.ID) != nil {
			tabCopy.ID = s.nextTabID("split")
		}
		if !preservePin {
			tabCopy.Pinned = false
		}
	}
	newLeaf := &PaneNode{
		ID:          s.nextPaneID(),
		Ratio:       50,
		Tabs:        []WorkspaceTab{tabCopy},
		ActiveTabID: tabCopy.ID,
	}
	origLeaf := &PaneNode{
		ID:          target.ID,
		Ratio:       50,
		Tabs:        append([]WorkspaceTab(nil), target.Tabs...),
		ActiveTabID: target.ActiveTabID,
	}
	target.ID = s.nextPaneID()
	target.Tabs = nil
	target.ActiveTabID = ""
	target.Ratio = 50
	target.Vertical = direction == "down" || direction == "up"
	if direction == "left" || direction == "up" {
		target.First = newLeaf
		target.Second = origLeaf
	} else {
		target.First = origLeaf
		target.Second = newLeaf
	}
	s.ActivePaneID = newLeaf.ID
	s.pushHistory(newLeaf.ID, tabCopy.ID, tabCopy.FilePath, tabCopy.Mode)
	s.notifyTabChange(tabCopy)
}

// MoveOrSplitTab drags a tab from srcPaneID onto dstPaneID at zone ("center"|"right"|"left"|"down"|"up").
func (s *Shell) MoveOrSplitTab(srcPaneID, tabID, dstPaneID, zone string) {
	s.EnsureTabs()
	srcPane := s.findPane(srcPaneID)
	dstPane := s.findPane(dstPaneID)
	if srcPane == nil || dstPane == nil {
		return
	}
	idx := -1
	var moving WorkspaceTab
	for i := range srcPane.Tabs {
		if srcPane.Tabs[i].ID == tabID {
			idx = i
			moving = srcPane.Tabs[i]
			break
		}
	}
	if idx < 0 {
		return
	}
	if srcPane.ID == dstPane.ID && zone == "center" {
		s.SwitchTabInPane(dstPane.ID, tabID, true)
		return
	}
	if srcPane.ID == dstPane.ID && len(srcPane.Tabs) <= 1 {
		return
	}
	// Detach from source pane
	srcPane.Tabs = append(srcPane.Tabs[:idx], srcPane.Tabs[idx+1:]...)
	if srcPane.ActiveTabID == tabID {
		nextIdx := idx - 1
		if nextIdx < 0 {
			nextIdx = 0
		}
		if nextIdx < len(srcPane.Tabs) {
			srcPane.ActiveTabID = srcPane.Tabs[nextIdx].ID
		} else {
			srcPane.ActiveTabID = ""
		}
	}
	if zone == "center" || zone == "" {
		dstPane.Tabs = append(dstPane.Tabs, moving)
		dstPane.ActiveTabID = moving.ID
		s.ActivePaneID = dstPane.ID
		if len(srcPane.Tabs) == 0 {
			s.collapseEmptyPane(srcPane.ID)
		}
		s.notifyTabChange(moving)
		return
	}
	s.splitPaneNode(dstPane, moving, zone, true)
	if len(srcPane.Tabs) == 0 {
		s.collapseEmptyPane(srcPane.ID)
	}
	if s.findPane(s.ActivePaneID) == nil {
		if first := s.firstLeaf(s.RootPane); first != nil {
			s.ActivePaneID = first.ID
		}
	}
	s.notifyTabChange(moving)
}

// ReorderTabInPane moves a tab inside the same pane before/after targetIdx.
func (s *Shell) ReorderTabInPane(paneID, tabID string, targetIdx int) {
	pane := s.findPane(paneID)
	if pane == nil || targetIdx < 0 || targetIdx >= len(pane.Tabs) {
		return
	}
	from := -1
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == tabID {
			from = i
			break
		}
	}
	if from < 0 || from == targetIdx {
		return
	}
	item := pane.Tabs[from]
	without := append([]WorkspaceTab{}, pane.Tabs[:from]...)
	without = append(without, pane.Tabs[from+1:]...)
	reordered := append([]WorkspaceTab{}, without[:targetIdx]...)
	reordered = append(reordered, item)
	reordered = append(reordered, without[targetIdx:]...)
	pane.Tabs = reordered
	if t := s.ActiveTab(); t != nil {
		s.notifyTabChange(*t)
		return
	}
	s.syncLegacyMirror()
}

// CloseTabInPane closes tabID in paneID and collapses empty split branches.
func (s *Shell) CloseTabInPane(paneID, tabID string) {
	s.EnsureTabs()
	pane := s.findPane(paneID)
	if pane == nil {
		return
	}
	idx := -1
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == tabID {
			if pane.Tabs[i].Pinned {
				return
			}
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	wasActive := pane.Tabs[idx].ID == pane.ActiveTabID
	pane.Tabs = append(pane.Tabs[:idx], pane.Tabs[idx+1:]...)
	if len(pane.Tabs) == 0 {
		if s.leafCount() > 1 {
			s.collapseEmptyPane(pane.ID)
			if first := s.firstLeaf(s.RootPane); first != nil {
				s.ActivePaneID = first.ID
				if t := s.ActiveTab(); t != nil {
					s.notifyTabChange(*t)
					return
				}
			}
		} else {
			// Ensure root never stays completely blank.
			cal := WorkspaceTab{ID: calendarTabID, Kind: TabCalendar, Title: "实验日程", Pinned: true}
			pane.Tabs = []WorkspaceTab{cal}
			pane.ActiveTabID = calendarTabID
			s.notifyTabChange(cal)
			return
		}
	}
	if wasActive && len(pane.Tabs) > 0 {
		nextIdx := idx - 1
		if nextIdx < 0 {
			nextIdx = 0
		}
		pane.ActiveTabID = pane.Tabs[nextIdx].ID
		s.ActivePaneID = pane.ID
		s.notifyTabChange(pane.Tabs[nextIdx])
		return
	}
	if t := s.ActiveTab(); t != nil {
		s.notifyTabChange(*t)
		return
	}
	s.syncLegacyMirror()
}

// CloseTab closes tabID in whichever pane holds it.
func (s *Shell) CloseTab(tabID string) {
	if pane := findPaneWithTab(s.RootPane, tabID); pane != nil {
		s.CloseTabInPane(pane.ID, tabID)
	}
}

// CloseOtherTabs closes all non-pinned tabs in paneID except keepTabID.
func (s *Shell) CloseOtherTabs(paneID, keepTabID string) {
	pane := s.findPane(paneID)
	if pane == nil {
		return
	}
	kept := make([]WorkspaceTab, 0, len(pane.Tabs))
	for _, t := range pane.Tabs {
		if t.Pinned || t.ID == keepTabID {
			kept = append(kept, t)
		}
	}
	pane.Tabs = kept
	pane.ActiveTabID = keepTabID
	s.SwitchTabInPane(paneID, keepTabID, false)
}

// CloseTabsToRight closes all non-pinned tabs strictly to the right of tabID inside paneID.
func (s *Shell) CloseTabsToRight(paneID, tabID string) {
	pane := s.findPane(paneID)
	if pane == nil {
		return
	}
	pos := -1
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == tabID {
			pos = i
			break
		}
	}
	if pos < 0 {
		return
	}
	kept := append([]WorkspaceTab(nil), pane.Tabs[:pos+1]...)
	for _, t := range pane.Tabs[pos+1:] {
		if t.Pinned {
			kept = append(kept, t)
		}
	}
	pane.Tabs = kept
	hasActive := false
	for _, t := range pane.Tabs {
		if t.ID == pane.ActiveTabID {
			hasActive = true
			break
		}
	}
	if !hasActive && len(pane.Tabs) > 0 {
		pane.ActiveTabID = tabID
	}
	if t := s.ActiveTab(); t != nil {
		s.notifyTabChange(*t)
		return
	}
	s.syncLegacyMirror()
}

// TogglePinTab toggles Pinned on tabID.
func (s *Shell) TogglePinTab(paneID, tabID string) {
	pane := s.findPane(paneID)
	if pane == nil {
		return
	}
	for i := range pane.Tabs {
		if pane.Tabs[i].ID == tabID {
			pane.Tabs[i].Pinned = !pane.Tabs[i].Pinned
			s.notifyTabChange(pane.Tabs[i])
			return
		}
	}
}

func (s *Shell) collapseEmptyPane(emptyPaneID string) {
	if s.RootPane == nil || s.RootPane.isLeaf() {
		return
	}
	var collapse func(parent **PaneNode) bool
	collapse = func(cur **PaneNode) bool {
		n := *cur
		if n == nil || n.isLeaf() {
			return false
		}
		if n.First != nil && n.First.isLeaf() && n.First.ID == emptyPaneID {
			*cur = n.Second
			return true
		}
		if n.Second != nil && n.Second.isLeaf() && n.Second.ID == emptyPaneID {
			*cur = n.First
			return true
		}
		return collapse(&n.First) || collapse(&n.Second)
	}
	collapse(&s.RootPane)
}

// OpenInSplit opens tab in a right split beside the currently active pane.
func (s *Shell) OpenInSplit(tab WorkspaceTab) {
	s.EnsureTabs()
	s.SplitPaneWithTab(s.ActivePaneID, tab, "right")
}

// ToggleSplit splits the active pane to the right (or closes the active split leaf if already split).
func (s *Shell) ToggleSplit() {
	s.EnsureTabs()
	if s.leafCount() > 1 {
		if ap := s.ActivePane(); ap != nil && len(ap.Tabs) == 1 && !ap.Tabs[0].Pinned {
			s.CloseTabInPane(ap.ID, ap.Tabs[0].ID)
			return
		}
	}
	if t := s.ActiveTab(); t != nil {
		cp := *t
		cp.ID = ""
		s.SplitPaneWithTab(s.ActivePaneID, cp, "right")
	}
}

// SyncFileRename updates all open tabs referencing oldRel to newRel and updates titles.
func (s *Shell) SyncFileRename(oldRel, newRel, newTitle string) {
	if s == nil || s.RootPane == nil {
		return
	}
	var visit func(n *PaneNode)
	visit = func(n *PaneNode) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			for i := range n.Tabs {
				if n.Tabs[i].Kind == TabFile && n.Tabs[i].FilePath == oldRel {
					n.Tabs[i].FilePath = newRel
					if newTitle != "" {
						n.Tabs[i].Title = newTitle
					}
				}
			}
			return
		}
		visit(n.First)
		visit(n.Second)
	}
	visit(s.RootPane)
	s.syncLegacyMirror()
}

// CloseTabsForFile closes any open tabs pointing to deleted filePath.
func (s *Shell) CloseTabsForFile(filePath string) {
	if s == nil || s.RootPane == nil {
		return
	}
	type target struct{ paneID, tabID string }
	var toClose []target
	var visit func(n *PaneNode)
	visit = func(n *PaneNode) {
		if n == nil {
			return
		}
		if n.isLeaf() {
			for _, t := range n.Tabs {
				if t.Kind == TabFile && t.FilePath == filePath {
					toClose = append(toClose, target{n.ID, t.ID})
				}
			}
			return
		}
		visit(n.First)
		visit(n.Second)
	}
	visit(s.RootPane)
	for _, tc := range toClose {
		s.CloseTabInPane(tc.paneID, tc.tabID)
	}
}

// CanBack reports whether navigating backwards in history is available.
func (s *Shell) CanBack() bool {
	return s != nil && s.HistoryIdx > 0 && s.HistoryIdx < len(s.History)
}

// CanForward reports whether navigating forwards in history is available.
func (s *Shell) CanForward() bool {
	return s != nil && s.HistoryIdx >= 0 && s.HistoryIdx+1 < len(s.History)
}

func (s *Shell) applyHistoryIndex(idx int) {
	if idx < 0 || idx >= len(s.History) {
		return
	}
	entry := s.History[idx]
	s.HistoryIdx = idx
	if pane := findPaneWithTab(s.RootPane, entry.TabID); pane != nil {
		for i := range pane.Tabs {
			if pane.Tabs[i].ID == entry.TabID && pane.Tabs[i].Kind == TabFile {
				pane.Tabs[i].Mode = entry.Mode
			}
		}
		s.SwitchTabInPane(pane.ID, entry.TabID, false)
		return
	}
	if entry.FilePath != "" {
		s.OpenFileInPane(s.ActivePaneID, entry.FilePath, fileStem(entry.FilePath), entry.Mode, false)
	}
}

// GoBack steps backward through visited tabs and modes.
func (s *Shell) GoBack() {
	if s.CanBack() {
		s.applyHistoryIndex(s.HistoryIdx - 1)
	}
}

// GoForward steps forward through visited tabs and modes.
func (s *Shell) GoForward() {
	if s.CanForward() {
		s.applyHistoryIndex(s.HistoryIdx + 1)
	}
}

// ToggleRightDrawer opens or collapses the Right Companion Drawer.
func (s *Shell) ToggleRightDrawer() {
	s.RightOpen = !s.RightOpen
	if s.window != nil {
		s.window.SetRightExpanded(s.RightOpen)
	}
}

// SetRightDrawer opens the Right Companion Drawer on a specific tab (0 = 大纲, 1 = 速算与计时, 2 = 关联与诊断).
func (s *Shell) SetRightDrawer(open bool, toolTab int) {
	s.RightOpen = open
	if toolTab >= 0 && toolTab <= 2 {
		s.RightTab = toolTab
	}
	if s.window != nil {
		s.window.SetRightExpanded(open)
	}
}

// ExportPersistState captures the pane tree and UI layout for `.asterlab/workspace.json`.
func (s *Shell) ExportPersistState() WorkspacePersistState {
	leftCol := false
	if s.window != nil {
		leftCol = !s.window.LeftExpanded()
	}
	return WorkspacePersistState{
		RootPane:      s.RootPane,
		ActivePaneID:  s.ActivePaneID,
		LeftCollapsed: leftCol,
		RightOpen:     s.RightOpen,
		RightTab:      s.RightTab,
		CalendarMode:  s.CalendarMode,
	}
}

// RestorePersistState restores a saved `.asterlab/workspace.json` safely,
// repairing any duplicate tab/pane IDs and advancing sequence counters.
// Returns true if any corrupt or duplicate IDs were repaired.
func (s *Shell) RestorePersistState(st WorkspacePersistState) bool {
	if st.RootPane != nil {
		s.RootPane = st.RootPane
		s.ActivePaneID = st.ActivePaneID
	}
	s.RightOpen = st.RightOpen
	if st.RightTab >= 0 && st.RightTab <= 2 {
		s.RightTab = st.RightTab
	}
	if st.CalendarMode == 0 || st.CalendarMode == 1 {
		s.CalendarMode = st.CalendarMode
	}
	repaired := s.sanitizePaneTree()
	s.EnsureTabs()
	return repaired
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func fileStem(relPath string) string {
	if relPath == "" {
		return ""
	}
	idx := strings.LastIndex(relPath, "/")
	base := relPath
	if idx >= 0 {
		base = relPath[idx+1:]
	}
	return strings.TrimSuffix(base, ".asterml")
}

func fmtTabTooltip(t WorkspaceTab) string {
	switch t.Kind {
	case TabCalendar:
		return "实验日程 (Cmd+1)"
	case TabPantry:
		return "储藏室"
	case TabFile:
		modeLabel := "编辑"
		switch t.Mode {
		case ModeLab:
			modeLabel = "实验模式"
		case ModeSource:
			modeLabel = "源码"
		}
		if t.FilePath != "" {
			return fmt.Sprintf("%s · %s", t.FilePath, modeLabel)
		}
		return t.Title
	default:
		return t.Title
	}
}
