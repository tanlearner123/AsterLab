package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/astercore"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/tools"
	"asterlab-mygo-prototype/internal/vault"
)

const releaseBlankTemplateDoc = `---
title: 空白实验
---

`

func defaultTemplateSeeds() map[string]string {
	return map[string]string{
		"空白实验.asterml": releaseBlankTemplateDoc,
	}
}

type vaultPrefs struct {
	LastVaultRoot string `json:"lastVaultRoot,omitempty"`
}

func vaultPrefsPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "vault-prefs.json")
}

func loadLastVaultRoot() string {
	if strings.TrimSpace(os.Getenv("ASTERLAB_VAULT_DIR")) != "" {
		return ""
	}
	p := vaultPrefsPath()
	if p == "" {
		return ""
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	var prefs vaultPrefs
	if json.Unmarshal(raw, &prefs) != nil {
		return ""
	}
	root := strings.TrimSpace(prefs.LastVaultRoot)
	if root == "" {
		return ""
	}
	if info, err := os.Stat(root); err == nil && info.IsDir() {
		return root
	}
	return ""
}

func saveLastVaultRoot(root string) {
	root = strings.TrimSpace(root)
	p := vaultPrefsPath()
	if root == "" || p == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	if raw, err := json.MarshalIndent(vaultPrefs{LastVaultRoot: root}, "", "  "); err == nil {
		_ = asterdoc.WriteFileAtomic(p, raw)
	}
}

func (a *app) initVaultWorkspace(vaultRoot string) error {
	if strings.TrimSpace(vaultRoot) == "" {
		if saved := loadLastVaultRoot(); saved != "" {
			vaultRoot = saved
		}
	}
	v, err := vault.Open(vaultRoot)
	if err != nil {
		return err
	}
	_ = v.SeedTemplates(defaultTemplateSeeds())
	_ = v.SeedAgentGuide()
	// Only migrate legacy `%LOCALAPPDATA%/AsterLab/data/experiments` into the default
	// built-in Vault; never pollute user-chosen / freshly created Vault directories.
	if dataDir := workspaceDataDir(); dataDir != "" &&
		strings.EqualFold(filepath.Clean(v.Root), filepath.Clean(vault.DefaultVaultDir())) {
		_, _ = v.MigrateLegacyExperiments(dataDir)
	}
	a.vault = v
	a.vaultIndex = vault.NewVaultIndex(v)
	a.docStore = vault.NewDocStore(v)
	a.ensureLiveViewsMap()
	if a.tools != nil {
		a.tools.ResetPantryState()
	}

	if a.shell != nil {
		a.shell.Error = ""
		a.shell.VaultPathInput = v.Root
		a.shell.HideAssetsInTree = v.Config.HideAssetsInTree
		a.shell.DefaultProjectDir = v.Config.DefaultProjectDir
		a.shell.ThemeSource = v.Config.ThemeSource
		lgVal := true
		if v.Config.LiquidGlass != nil {
			lgVal = *v.Config.LiquidGlass
		}
		a.shell.SetInitialThemeLook(v.Config.ThemePreset, lgVal)
		a.shell.TextScale = v.Config.TextScale
		a.shell.Density = v.Config.Density
		a.shell.ReduceMotion = v.Config.ReduceMotion
		a.shell.DefaultOpenMode = v.Config.DefaultOpenMode
		a.shell.DefaultPlateFormat = v.Config.DefaultPlateFormat
		a.shell.DefaultExcessPct = v.Config.DefaultExcessPct
		if v.Config.AutoArchiveAssets != nil {
			a.shell.AutoArchiveAssets = *v.Config.AutoArchiveAssets
		} else {
			a.shell.AutoArchiveAssets = true
		}
		if len(v.Config.Hotkeys) > 0 {
			a.shell.Hotkeys = v.Config.Hotkeys
		}
		autoHide := true
		if v.Config.MobileAutoHideDock != nil {
			autoHide = *v.Config.MobileAutoHideDock
		}
		editTb := true
		if v.Config.MobileEditToolbar != nil {
			editTb = *v.Config.MobileEditToolbar
		}
		a.shell.SetInitialMobilePrefs(v.Config.MobilePullAction, autoHide, editTb)
		a.shell.SyncThemeLook(nil)
		a.shell.ResetWorkspaceForVault()
		var persist shell.WorkspacePersistState
		if err := v.LoadWorkspaceJSON(&persist); err == nil {
			repaired := a.shell.RestorePersistState(persist)
			pruned := a.shell.PruneMissingFileTabs(func(rel string) bool {
				_, statErr := os.Stat(v.AbsPath(rel))
				return statErr == nil
			})
			if repaired || pruned {
				a.saveVaultWorkspace()
			}
		}
	}
	a.refreshVaultTreeAndCalendar()
	a.lastVaultSig, a.lastPantrySig = computeVaultDiskSignatures(v.Root)
	return nil
}

func (a *app) saveVaultSettingsFromShell() {
	if a.vault == nil || a.shell == nil {
		return
	}
	a.vault.Config.HideAssetsInTree = a.shell.HideAssetsInTree
	a.vault.Config.DefaultProjectDir = a.shell.DefaultProjectDir
	a.vault.Config.ThemeSource = a.shell.ThemeSource
	a.vault.Config.ThemePreset = a.shell.ThemePreset
	lg := a.shell.LiquidGlass
	a.vault.Config.LiquidGlass = &lg
	a.vault.Config.TextScale = a.shell.TextScale
	a.vault.Config.Density = a.shell.Density
	a.vault.Config.ReduceMotion = a.shell.ReduceMotion
	a.vault.Config.DefaultOpenMode = a.shell.DefaultOpenMode
	a.vault.Config.DefaultPlateFormat = a.shell.DefaultPlateFormat
	a.vault.Config.DefaultExcessPct = a.shell.DefaultExcessPct
	autoArch := a.shell.AutoArchiveAssets
	a.vault.Config.AutoArchiveAssets = &autoArch
	if len(a.shell.Hotkeys) > 0 {
		a.vault.Config.Hotkeys = a.shell.Hotkeys
	}
	a.vault.Config.MobilePullAction = a.shell.MobilePullAction
	autoHide := a.shell.MobileAutoHideDock
	a.vault.Config.MobileAutoHideDock = &autoHide
	editTb := a.shell.MobileEditToolbar
	a.vault.Config.MobileEditToolbar = &editTb
	_ = a.vault.SaveConfig()
}

func (a *app) refreshVaultTreeAndCalendar() {
	if a.vault == nil || a.shell == nil {
		return
	}
	tree, files, err := a.vault.ScanTree()
	if err != nil {
		return
	}
	a.shell.VaultTree = tree
	a.shell.VaultFiles = files
	a.shell.VaultFolders = a.vault.ProjectFolders()
	a.shell.TrashItems = a.vault.ListTrash()
	if a.vaultIndex != nil {
		_ = a.vaultIndex.Refresh()
		a.shell.VaultTimers = a.vaultIndex.ActiveTimers()
	}

	var exps []shell.ExperimentItem
	var designs []shell.DesignItem
	for _, f := range files {
		if f.IsTemplate || f.IsAsset {
			continue
		}
		stage := shell.StageReady
		switch f.Status {
		case "in_progress":
			stage = shell.StageInProgress
		case "completed":
			stage = shell.StageCompleted
		}
		folder := f.Folder
		if folder == "" {
			folder = "根目录"
		}
		exps = append(exps, shell.ExperimentItem{
			ID:       f.RelPath,
			DesignID: f.RelPath,
			FilePath: f.RelPath,
			Folder:   folder,
			Title:    f.Title,
			Date:     f.Date,
			Stage:    stage,
			Summary:  folder,
			FileName: filepath.Base(f.RelPath),
		})
		designs = append(designs, shell.DesignItem{
			ID:    f.RelPath,
			Title: f.Title,
			Meta:  f.Date,
		})
	}
	a.shell.Experiments = exps
	a.shell.Designs = designs
	a.syncPantrySnapshotToShell()
}

func (a *app) syncPantrySnapshotToShell() {
	if a.shell == nil {
		return
	}
	if a.tools == nil {
		a.shell.PantryItems = nil
		return
	}
	if !a.tools.PantryLoaded {
		a.tools.ReloadPantry()
	}
	if len(a.tools.Pantry.Items) == 0 {
		a.shell.PantryItems = nil
		return
	}
	qItems := make([]shell.QuickPantryItem, 0, len(a.tools.Pantry.Items))
	for _, p := range a.tools.Pantry.Items {
		loc := a.tools.FormatPantryDisplayLocation(p)
		if loc == "" {
			loc = p.Location
		}
		qItems = append(qItems, shell.QuickPantryItem{
			ID:        p.ID,
			Name:      p.Name,
			Location:  loc,
			Status:    p.Status,
			ExpiresOn: p.ExpiresOn,
			LotNumber: p.LabelCode,
		})
	}
	a.shell.PantryItems = qItems
}

func (a *app) saveVaultWorkspace() {
	if a.vault == nil || a.shell == nil {
		return
	}
	st := a.shell.ExportPersistState()
	_ = a.vault.SaveWorkspaceJSON(st)
}

func (a *app) openVaultFileInActivePane(relPath, title string) {
	if a.shell == nil || strings.TrimSpace(relPath) == "" {
		return
	}
	a.shell.EnsureTabs()
	paneID := a.shell.ActivePaneID
	if title == "" && a.vault != nil {
		for _, f := range a.shell.VaultFiles {
			if f.RelPath == relPath {
				title = f.Title
				break
			}
		}
	}
	mode := a.shell.DefaultFileOpenMode()
	a.shell.OpenFileInPane(paneID, relPath, title, mode, false)
	a.saveVaultWorkspace()
}

func (a *app) openNewExperimentDialog(folderRel, dateYMD string) {
	if a.vault != nil && strings.TrimSpace(folderRel) == "" {
		switch a.vault.Config.DefaultProjectDir {
		case "/":
			folderRel = ""
		case "":
			folderRel = a.vault.Config.LastProjectDir
		default:
			folderRel = a.vault.Config.DefaultProjectDir
		}
	}
	a.newExpFolder = folderRel
	a.newExpTitle = ""
	a.newExpDate = strings.TrimSpace(dateYMD)
	if a.newExpTemplate == "" {
		a.newExpTemplate = "templates/空白实验.asterml"
	}
	a.newExpNewFolder = ""
	a.newExpErr = ""
	a.newExpOpen = true
}

func (a *app) renderVaultNewExperimentDialog(c *ui.Context) {
	if !a.newExpOpen || a.vault == nil {
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := a.newExpOpen
	folders := append([]string{""}, a.vault.ProjectFolders()...)
	var templates []vault.FileEntry
	if a.shell != nil {
		for _, f := range a.shell.VaultFiles {
			if f.IsTemplate {
				templates = append(templates, f)
			}
		}
	}

	ui.Box(c.Key("vault-new-exp-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "新建实验"}, func(panel ui.Element) {
		panel.Width(float32(tok.Spacing48)*11).Gap(float32(tok.Spacing12)).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.FlaskConical()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
				ui.Text(c, "新建实验").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			})

			// Title
			titleCol := ui.Column(c.Key("new-exp-title-col")).FillWidth().Gap(float32(tok.Spacing4))
			theme.ApplyNudge(titleCol, a.newExpNudgeSeq)
			titleCol.Children(func() {
				ui.Text(c, "实验名称").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
				tb := components.TextBox(c, &a.newExpTitle, components.TextBoxOptions{}).Placeholder("例如：CCK-8 药物敏感性测定")
				if tb.Submitted() {
					if strings.TrimSpace(a.newExpTitle) == "" {
						a.newExpErr = "请输入实验名称"
						a.newExpNudgeSeq++
					} else {
						a.confirmCreateVaultExperiment()
					}
				}
			})

			// Location (Project folder picker + inline new folder) + Date
			ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "保存位置（项目文件夹）").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
					sel := components.Select(c, &a.newExpFolder)
					sel.Trigger.Children(func() {
						lbl := a.newExpFolder
						if lbl == "" {
							lbl = "根目录"
						}
						ui.Text(c, lbl).FontSize(float32(tok.TextData)).TextColor(tok.Text)
					})
					sel.Popup(func(p ui.Element) {
						for _, f := range folders {
							f := f
							lbl := f
							if lbl == "" {
								lbl = "根目录"
							}
							sel.Item(f).Children(func() {
								ui.Text(c, lbl).FontSize(float32(tok.TextData)).TextColor(tok.Text)
							})
						}
						sel.Item("__new__").Children(func() {
							ui.Text(c, "+ 新建项目文件夹…").FontSize(float32(tok.TextData)).TextColor(tok.Accent)
						})
					})
				})
				ui.Column(c).Width(float32(tok.Spacing48)*3.3).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "日期（YYYY-MM-DD，可空）").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
					components.TextBox(c, &a.newExpDate, components.TextBoxOptions{}).Placeholder("2026-11-20")
				})
			})

			if a.newExpFolder == "__new__" {
				ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "新项目文件夹名").FontSize(float32(tok.TextData)).TextColor(tok.Accent)
					components.TextBox(c, &a.newExpNewFolder, components.TextBoxOptions{}).Placeholder("例如：肿瘤耐药筛选")
				})
			}

			// Template picker
			if len(templates) > 0 {
				ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "选用模板").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
					tSel := components.Select(c, &a.newExpTemplate)
					tSel.Trigger.Children(func() {
						lbl := strings.TrimSuffix(filepath.Base(a.newExpTemplate), ".asterml")
						if lbl == "" || lbl == "." {
							lbl = "空白实验"
						}
						tSel.Trigger.Label(lbl)
						ui.Text(c, lbl).FontSize(float32(tok.TextData)).TextColor(tok.Text)
					})
					tSel.Popup(func(p ui.Element) {
						hasBlank := false
						for _, tpl := range templates {
							if tpl.Name == "空白实验" {
								hasBlank = true
								break
							}
						}
						if !hasBlank {
							tSel.Item("").Children(func() {
								ui.Text(c, "空白实验").FontSize(float32(tok.TextData)).TextColor(tok.Text)
							})
						}
						for _, tpl := range templates {
							tpl := tpl
							tSel.Item(tpl.RelPath).Children(func() {
								ui.Text(c, tpl.Name).FontSize(float32(tok.TextData)).TextColor(tok.Text)
							})
						}
					})
				})
			}
			if a.newExpErr != "" {
				ui.Text(c, a.newExpErr).FontSize(float32(tok.TextData)).TextColor(tok.Danger)
			}

			ui.Row(c).FillWidth().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
				if components.Button(c, "取消", components.ButtonGhost).Clicked() {
					a.newExpOpen = false
				}
				if components.Button(c, "创建并打开", components.ButtonPrimary).Clicked() {
					a.confirmCreateVaultExperiment()
				}
			})
		})
	})
	})
	if !open {
		a.newExpOpen = false
	}
}

func (a *app) confirmCreateVaultExperiment() {
	if a.vault == nil {
		return
	}
	if a.newExpFolder == "__new__" && strings.TrimSpace(a.newExpNewFolder) == "" {
		a.newExpErr = "请填写新项目文件夹名称"
		a.newExpNudgeSeq++
		return
	}
	if strings.TrimSpace(a.newExpDate) != "" && asterdoc.NormalizeDateYMD(a.newExpDate) == "" {
		a.newExpErr = "日期格式请填写 YYYY-MM-DD（或留空）"
		a.newExpNudgeSeq++
		return
	}
	a.newExpErr = ""
	folder := a.newExpFolder
	if folder == "__new__" {
		name := strings.TrimSpace(a.newExpNewFolder)
		if name != "" {
			if created, err := a.vault.CreateFolder("", name); err == nil {
				folder = created
			}
		} else {
			folder = ""
		}
	}
	title := strings.TrimSpace(a.newExpTitle)
	tplSrc := releaseBlankTemplateDoc
	if a.newExpTemplate != "" {
		if ld, err := a.docStore.Open(a.newExpTemplate); err == nil && strings.TrimSpace(ld.Source) != "" {
			tplSrc = ld.Source
		}
	}
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(a.newExpTemplate), ".asterml")
		if title == "" || title == "." {
			title = "新实验"
		}
	}
	entry, err := a.vault.CreateExperiment(folder, title, strings.TrimSpace(a.newExpDate), tplSrc)
	a.newExpOpen = false
	if err != nil {
		a.syncErr = fmt.Sprintf("创建实验失败：%v", err)
		return
	}
	a.vault.Config.LastProjectDir = folder
	_ = a.vault.SaveConfig()
	a.refreshVaultTreeAndCalendar()
	a.openVaultFileInActivePane(entry.RelPath, entry.Title)
}

func (a *app) moveVaultFile(srcRel, destFolderRel string) {
	if a.vault == nil || srcRel == "" {
		return
	}
	newRel, err := a.vault.Move(srcRel, destFolderRel)
	if err != nil {
		a.syncErr = fmt.Sprintf("移动失败：%v", err)
		return
	}
	if a.docStore != nil {
		a.docStore.RenameKey(srcRel, newRel)
	}
	if a.shell != nil {
		a.shell.SyncFileRename(srcRel, newRel, "")
	}
	a.refreshVaultTreeAndCalendar()
	a.saveVaultWorkspace()
}

func (a *app) deleteVaultFile(relPath string) {
	if a.vault == nil || relPath == "" {
		return
	}
	if _, err := a.vault.Trash(relPath); err != nil {
		a.syncErr = fmt.Sprintf("移入回收站失败：%v", err)
		return
	}
	if a.docStore != nil {
		a.docStore.Evict(relPath)
	}
	if a.shell != nil {
		a.shell.CloseTabsForFile(relPath)
	}
	a.refreshVaultTreeAndCalendar()
	a.saveVaultWorkspace()
}

func (a *app) restoreVaultTrash(trashName string) {
	if a.vault == nil || trashName == "" {
		return
	}
	restoredRel, err := a.vault.RestoreFromTrash(trashName)
	if err != nil {
		a.syncErr = fmt.Sprintf("恢复文件失败：%v", err)
		return
	}
	a.refreshVaultTreeAndCalendar()
	if strings.HasSuffix(strings.ToLower(restoredRel), ".asterml") {
		a.openVaultFileInActivePane(restoredRel, "")
	}
}

func (a *app) syncActiveDocCompanionState(relPath, source string, doc *asterdoc.Document) {
	if a.shell == nil || a.vault == nil || relPath == "" {
		return
	}
	cacheKey := fmt.Sprintf("%s|%d|%s", relPath, len(source), source)
	if a.lastCompanionKey == cacheKey && a.shell.CurrentDocRelPath == relPath {
		return
	}
	a.lastCompanionKey = cacheKey
	a.shell.CurrentDocRelPath = relPath
	a.shell.CurrentSnapshots = a.vault.ListSnapshots(relPath)
	if a.vaultIndex != nil {
		a.shell.CurrentBacklinks = a.vaultIndex.BacklinksFor(relPath)
		a.shell.VaultTimers = a.vaultIndex.ActiveTimers()
	}
	knownPantry := map[string]bool{}
	if a.tools != nil {
		for _, p := range a.tools.Pantry.Items {
			if p.LabelCode != "" {
				knownPantry[strings.TrimSpace(p.LabelCode)] = true
			}
		}
	}
	rep := astercore.AssessDocument(doc, knownPantry)
	diags := make([]shell.DiagnosticItem, 0, len(rep.Diagnostics))
	for _, d := range rep.Diagnostics {
		diags = append(diags, shell.DiagnosticItem{
			Code:       d.Code,
			Severity:   string(d.Severity),
			Path:       d.Path,
			Message:    d.Message,
			NextAction: d.NextAction,
		})
	}
	a.shell.CurrentDiagnostics = diags
}

func (a *app) restoreSnapshotFromHistory(relPath, snapshotID string) {
	if a.vault == nil || a.docStore == nil || relPath == "" || snapshotID == "" {
		return
	}
	snapSrc, err := a.vault.ReadSnapshot(relPath, snapshotID)
	if err != nil {
		a.syncErr = fmt.Sprintf("读取历史快照失败：%v", err)
		return
	}
	ld, err := a.docStore.Open(relPath)
	if err != nil {
		a.syncErr = fmt.Sprintf("打开文档失败：%v", err)
		return
	}
	_ = a.vault.RecordSnapshot(relPath, ld.Source, true)
	if err := ld.ApplyEdit(snapSrc); err != nil {
		a.syncErr = fmt.Sprintf("恢复快照失败：%v", err)
		return
	}
	ld.NotifySaved(a.vault, true)
	if a.vaultIndex != nil {
		a.vaultIndex.UpsertSource(relPath, ld.Source)
	}
	a.resetLiveDraftCachesForFile(relPath)
	a.lastCompanionKey = ""
	a.refreshVaultTreeAndCalendar()
	a.syncActiveDocCompanionState(relPath, ld.Source, ld.Parsed)
}

func (a *app) resetLiveDraftCachesForFile(relPath string) {
	if a.liveEditorSynced != nil {
		delete(a.liveEditorSynced, relPath)
	}
	if a.liveTitleDrafts != nil {
		delete(a.liveTitleDrafts, relPath)
	}
	if a.liveDateDrafts != nil {
		delete(a.liveDateDrafts, relPath)
	}
	for key, v := range a.liveViews {
		if strings.HasSuffix(key, "|"+relPath) {
			v.ClearTextDrafts()
		}
	}
}

func (a *app) undoActiveLiveDoc() {
	if a.shell == nil || a.docStore == nil {
		return
	}
	tab := a.shell.ActiveTab()
	if tab == nil || tab.Kind != shell.TabFile || tab.FilePath == "" {
		a.designUndo()
		return
	}
	if ld, err := a.docStore.Open(tab.FilePath); err == nil {
		if ld.Undo() {
			a.resetLiveDraftCachesForFile(tab.FilePath)
			a.refreshVaultTreeAndCalendar()
		}
	}
}

func (a *app) redoActiveLiveDoc() {
	if a.shell == nil || a.docStore == nil {
		return
	}
	tab := a.shell.ActiveTab()
	if tab == nil || tab.Kind != shell.TabFile || tab.FilePath == "" {
		a.designRedo()
		return
	}
	if ld, err := a.docStore.Open(tab.FilePath); err == nil {
		if ld.Redo() {
			a.resetLiveDraftCachesForFile(tab.FilePath)
			a.refreshVaultTreeAndCalendar()
		}
	}
}

func (a *app) revealPathInOS(relPath string) {
	if a.vault == nil {
		return
	}
	abs := a.vault.AbsPath(relPath)
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", "/select,", filepath.FromSlash(abs)).Start()
	case "darwin":
		_ = exec.Command("open", "-R", abs).Start()
	default:
		_ = exec.Command("xdg-open", filepath.Dir(abs)).Start()
	}
}

func (a *app) switchVaultRoot(newPath string) {
	if strings.TrimSpace(newPath) == "" {
		return
	}
	a.liveViews = nil
	a.liveEditors = nil
	a.liveEditorSynced = nil
	a.liveTitleDrafts = nil
	a.liveDateDrafts = nil
	a.lastCompanionKey = ""
	if err := a.initVaultWorkspace(newPath); err != nil {
		a.syncErr = fmt.Sprintf("切换实验库失败：%v", err)
		return
	}
	if a.vault != nil {
		saveLastVaultRoot(a.vault.Root)
	}
	a.syncErr = ""
}

// computeVaultDiskSignatures hashes the directory structure + file mtimes/sizes of
// user-visible `.asterml`/assets files and Vault-local `.asterlab/pantry-*.json` files.
// Runs safely off the main UI goroutine so external Agent/editor writes appear automatically.
func computeVaultDiskSignatures(vaultRoot string) (vaultSig, pantrySig string) {
	if strings.TrimSpace(vaultRoot) == "" {
		return "", ""
	}
	var entries []string
	_ = filepath.WalkDir(vaultRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path == vaultRoot {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == vault.SystemDirName || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			rel, rErr := filepath.Rel(vaultRoot, path)
			if rErr == nil {
				entries = append(entries, "D:"+filepath.ToSlash(rel))
			}
			return nil
		}
		if strings.HasPrefix(name, ".") {
			return nil
		}
		rel, rErr := filepath.Rel(vaultRoot, path)
		if rErr != nil {
			return nil
		}
		slash := filepath.ToSlash(rel)
		lower := strings.ToLower(name)
		inAssets := strings.HasPrefix(slash, vault.AssetsDirName+"/")
		if !inAssets && !strings.HasSuffix(lower, ".asterml") {
			return nil
		}
		info, iErr := d.Info()
		if iErr != nil {
			return nil
		}
		entries = append(entries, fmt.Sprintf("F:%s:%d:%d", slash, info.Size(), info.ModTime().UnixNano()))
		return nil
	})
	sort.Strings(entries)
	vh := sha256.New()
	for _, e := range entries {
		vh.Write([]byte(e))
		vh.Write([]byte{'\n'})
	}
	vaultSig = hex.EncodeToString(vh.Sum(nil))

	ph := sha256.New()
	for _, fname := range []string{pantryJSONLFileName, "pantry-ledger.json", "pantry-meta.json", "pantry-boxes.json"} {
		p := filepath.Join(vaultRoot, vault.SystemDirName, fname)
		if info, err := os.Stat(p); err == nil {
			fmt.Fprintf(ph, "%s:%d:%d\n", fname, info.Size(), info.ModTime().UnixNano())
		} else {
			fmt.Fprintf(ph, "%s:missing\n", fname)
		}
	}
	pantrySig = hex.EncodeToString(ph.Sum(nil))
	return vaultSig, pantrySig
}

// syncVaultDiskChangesNow compares current disk signatures against cached signatures
// and refreshes the left file tree, calendar, open LiveDocuments, and Vault Pantry on change.
// Must be called on the main UI thread (or directly in single-threaded tests).
func (a *app) syncVaultDiskChangesNow() bool {
	if a.vault == nil || a.vault.Root == "" {
		return false
	}
	nextVaultSig, nextPantrySig := computeVaultDiskSignatures(a.vault.Root)
	return a.applyVaultDiskSignatures(nextVaultSig, nextPantrySig)
}

func (a *app) applyVaultDiskSignatures(nextVaultSig, nextPantrySig string) bool {
	if a.vault == nil || a.vault.Root == "" {
		return false
	}
	vaultChanged := nextVaultSig != "" && nextVaultSig != a.lastVaultSig
	pantryChanged := nextPantrySig != "" && nextPantrySig != a.lastPantrySig
	if !vaultChanged && !pantryChanged {
		return false
	}
	a.lastVaultSig = nextVaultSig
	a.lastPantrySig = nextPantrySig

	if vaultChanged {
		if a.docStore != nil {
			for _, rel := range a.docStore.ReloadAllExternalChanged() {
				a.resetLiveDraftCachesForFile(rel)
			}
		}
		a.lastCompanionKey = ""
		a.refreshVaultTreeAndCalendar()
	}
	if pantryChanged {
		if a.tools != nil && !a.tools.PantryDialogOpen && !a.tools.PantryBoxDialogOpen {
			a.tools.ResetPantryState()
		}
		a.syncPantrySnapshotToShell()
	}
	return true
}

func (a *app) renderStandalonePantry(c *ui.Context) {
	if a.tools == nil {
		a.tools = a.newToolsWorkspace()
	}
	tools.ViewPantry(c, a.tools)
}
