package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/astercmd"
	"asterlab-mygo-prototype/internal/astercore"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/theme"
)

type cachedAssayResult struct {
	sourceLen int
	source    string
	summaries []astercore.AssaySummary
}

func (a *app) cachedDocumentAnalysis(relPath, source string, doc *asterdoc.Document) []astercore.AssaySummary {
	if doc == nil {
		return nil
	}
	if a.cachedAssays == nil {
		a.cachedAssays = map[string]cachedAssayResult{}
	}
	if prev, ok := a.cachedAssays[relPath]; ok && prev.sourceLen == len(source) && prev.source == source {
		return prev.summaries
	}
	res := astercore.AnalyzeDocument(doc, relPath)
	a.cachedAssays[relPath] = cachedAssayResult{
		sourceLen: len(source),
		source:    source,
		summaries: res,
	}
	return res
}

func (a *app) ensureLiveViewsMap() {
	if a.liveViews == nil {
		a.liveViews = map[string]*asterdoc.View{}
	}
	if a.liveEditors == nil {
		a.liveEditors = map[string]*editor.State{}
	}
}

func (a *app) buildPantryLocationMap() map[string]string {
	out := map[string]string{}
	if a.tools == nil || len(a.tools.Pantry.Items) == 0 {
		return out
	}
	for _, it := range a.tools.Pantry.Items {
		loc := a.tools.FormatPantryDisplayLocation(it)
		if loc == "" {
			loc = it.Location
		}
		if loc == "" {
			continue
		}
		if it.LabelCode != "" {
			out[it.LabelCode] = loc
		}
		if it.ID != "" {
			out[it.ID] = loc
		}
		if it.Name != "" {
			out[it.Name] = loc
		}
	}
	return out
}

func (a *app) ensureLiveSourceEditor(relPath, src string) *editor.State {
	a.ensureLiveViewsMap()
	normSrc := strings.ReplaceAll(src, "\r\n", "\n")
	ed := a.liveEditors[relPath]
	if ed == nil {
		buf := editor.NewBuffer(normSrc)
		if lang := editor.LanguageFor("design.md"); lang != nil {
			buf.SetLanguage(lang)
		}
		ed = &editor.State{
			Buffer:   buf,
			DiffBase: editor.NewBuffer(normSrc),
		}
		a.liveEditors[relPath] = ed
		a.liveEditorSynced[relPath] = normSrc
		return ed
	}
	// If doc changed externally or via visual pane, keep buffer synchronized when it wasn't just edited
	if prev, ok := a.liveEditorSynced[relPath]; !ok || (prev != normSrc && strings.ReplaceAll(ed.Buffer.String(), "\r\n", "\n") == prev) {
		ed.Buffer = editor.NewBuffer(normSrc)
		if lang := editor.LanguageFor("design.md"); lang != nil {
			ed.Buffer.SetLanguage(lang)
		}
		ed.DiffBase = editor.NewBuffer(normSrc)
		a.liveEditorSynced[relPath] = normSrc
	}
	return ed
}

func (a *app) renderVaultTabContent(c *ui.Context, pane *shell.PaneNode, tab shell.WorkspaceTab) {
	tok := theme.GetTokens(c.Theme())
	if a.vault == nil || a.docStore == nil {
		ui.Column(c).Fill().Center().Padding(float32(tok.Spacing24)).Children(func() {
			ui.Text(c, "本地实验库尚未就绪").FontSize(float32(tok.TextSection)).TextColor(tok.TextMuted)
		})
		return
	}
	if strings.TrimSpace(tab.FilePath) == "" {
		ui.Column(c).Fill().Center().Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Text(c, "请选择或新建实验文件").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			ui.Text(c, "在左侧文件树或日历中选择实验，也可以拖拽左侧文件到此处分屏对照。").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		})
		return
	}

	ld, err := a.docStore.Open(tab.FilePath)
	if err != nil {
		ui.Column(c).Fill().Center().Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing12)).Children(func() {
			ui.Text(c, "无法读取实验文件："+tab.FilePath).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Danger)
			ui.Text(c, err.Error()).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			if components.Button(c, "关闭此标签页", components.ButtonOutline).Clicked() && a.shell != nil {
				a.shell.CloseTabInPane(pane.ID, tab.ID)
			}
		})
		return
	}
	if changed, _ := ld.ReloadIfExternalChanged(); changed {
		a.refreshVaultTreeAndCalendar()
	}

	fm, _, _ := asterdoc.ScanFrontmatter(ld.Source)
	docTitle := strings.TrimSpace(fm.Title)
	if docTitle == "" {
		docTitle = strings.TrimSuffix(filepath.Base(tab.FilePath), ".asterml")
	}
	docDate := asterdoc.NormalizeDateYMD(fm.ScheduledDate)
	docStatus := firstNonEmpty(strings.TrimSpace(fm.Stage), "planned")
	if docStatus != "planned" && docStatus != "in_progress" && docStatus != "completed" {
		docStatus = "planned"
	}
	folderRel := filepath.ToSlash(filepath.Dir(tab.FilePath))
	if folderRel == "." {
		folderRel = ""
	}
	folderLabel := folderRel
	if folderLabel == "" {
		folderLabel = "根目录"
	}

	setTabMode := func(m shell.ViewMode) {
		if a.shell == nil {
			return
		}
		for i := range pane.Tabs {
			if pane.Tabs[i].ID == tab.ID {
				pane.Tabs[i].Mode = m
				break
			}
		}
		a.saveVaultWorkspace()
	}

	applyLiveSource := func(next string) {
		if next == ld.Source {
			return
		}
		_ = ld.ApplyEdit(next)
		ld.NotifySaved(a.vault, false)
		if a.vaultIndex != nil {
			a.vaultIndex.UpsertSource(tab.FilePath, ld.Source)
		}
		if a.liveEditorSynced != nil {
			a.liveEditorSynced[tab.FilePath] = strings.ReplaceAll(ld.Source, "\r\n", "\n")
		}
		a.lastCompanionKey = ""
		a.refreshVaultTreeAndCalendar()
		a.syncActiveDocCompanionState(tab.FilePath, ld.Source, ld.Parsed)
	}

	mobileLayout := a.shell != nil && a.shell.IsMobileLayout(c)

	if a.shell != nil {
		a.shell.OnMobileDocToolbarAction = func(curTab shell.WorkspaceTab, action string) {
			curLD, err := a.docStore.Open(curTab.FilePath)
			if err != nil {
				return
			}
			applyDoc := func(next string) {
				if next == curLD.Source {
					return
				}
				_ = curLD.ApplyEdit(next)
				if a.liveEditorSynced != nil {
					a.liveEditorSynced[curTab.FilePath] = strings.ReplaceAll(curLD.Source, "\r\n", "\n")
				}
				a.refreshVaultTreeAndCalendar()
			}
			lastStepID := ""
			if curLD.Parsed != nil {
				for _, m := range curLD.Parsed.Modules {
					if m.Kind == "step" {
						lastStepID = m.ID
					}
				}
			}
			runMobileCmd := func(cmdID string, args map[string]any, toastTitle string) {
				res := astercmd.ExecuteBatch(curLD.Source, astercmd.BatchRequest{
					Mode:        astercmd.ModeEdit,
					PlateFormat: a.shell.DefaultPlateFormat,
					ExcessPct:   a.shell.DefaultExcessPct,
					Operations:  []astercmd.Operation{{CommandID: cmdID, Args: args}},
				})
				if res.OK {
					applyDoc(res.NextSource)
					if toastTitle != "" {
						c.AddToast(ui.Toast{Title: toastTitle, Type: components.ToastSuccess})
					}
				} else if res.Error != nil {
					c.AddToast(ui.Toast{Title: res.Error.Message, Type: components.ToastWarning})
				}
			}
			switch action {
			case "undo":
				if a.shell.OnUndo != nil {
					a.shell.OnUndo()
				}
			case "redo":
				if a.shell.OnRedo != nil {
					a.shell.OnRedo()
				}
			case "add_step":
				runMobileCmd("step.create", map[string]any{"preset": "blank"}, "已添加空白步骤")
			case "add_checklist":
				args := map[string]any{"kind": "checklist"}
				if lastStepID != "" {
					args["step_id"] = lastStepID
				}
				runMobileCmd("block.insert", args, "已插入检查清单")
			case "add_param":
				args := map[string]any{"kind": "param_table"}
				if lastStepID != "" {
					args["step_id"] = lastStepID
				}
				runMobileCmd("block.insert", args, "已插入参数表")
			case "add_plate":
				args := map[string]any{"format": a.shell.DefaultPlateFormat}
				if lastStepID != "" {
					args["step_id"] = lastStepID
				}
				runMobileCmd("plate.create", args, "已插入孔板")
			case "open_timer":
				a.shell.RightTab = 1
				a.shell.MobileDrawer = "outline"
			case "source_mode":
				if curTab.Mode == shell.ModeSource {
					a.shell.SetActiveTabMode(shell.ModeEdit)
				} else {
					a.shell.SetActiveTabMode(shell.ModeSource)
				}
			case "copy_source":
				c.Services().WriteClipboard(curLD.Source)
				c.AddToast(ui.Toast{Title: "已复制 AsterML 原文", Type: components.ToastSuccess})
			case "export_pdf":
				if curLD.Parsed != nil {
					t := docTitle
					if fm2, _, _ := asterdoc.ScanFrontmatter(curLD.Source); strings.TrimSpace(fm2.Title) != "" {
						t = strings.TrimSpace(fm2.Title)
					}
					a.startDocumentPDFExport(c, t, "", "", a.vault.Root, curLD.Parsed)
				}
			case "save_template":
				if tpl, tErr := a.vault.SaveAsTemplate(curTab.FilePath, docTitle); tErr == nil {
					a.refreshVaultTreeAndCalendar()
					c.AddToast(ui.Toast{Title: "已保存为模板：" + tpl.Name, Type: components.ToastSuccess})
				}
			case "duplicate":
				if dup, dErr := a.vault.DuplicateFile(curTab.FilePath, docDate); dErr == nil {
					a.refreshVaultTreeAndCalendar()
					a.shell.OpenFileInPane(pane.ID, dup.RelPath, dup.Title, shell.ModeEdit, true)
				}
			case "delete":
				a.deleteVaultFile(curTab.FilePath)
			}
		}
	}

	pageCol := ui.Column(c.Key("vault-page:"+pane.ID+":"+tab.ID+":"+tab.FilePath)).Fill()
	theme.PaintCanvasBackground(pageCol, tok)
	findOpen := ui.Local(c.Root(), "doc-find-open:"+pane.ID+":"+tab.ID, func() bool { return false })
	findQuery := ui.Local(c.Root(), "doc-find-q:"+pane.ID+":"+tab.ID, func() string { return "" })
	if a.pendingFindQuery != nil {
		if pq := a.pendingFindQuery[tab.FilePath]; pq != "" {
			*findQuery = pq
			*findOpen = true
			delete(a.pendingFindQuery, tab.FilePath)
		}
	}
	if pane.ID == a.shell.ActivePaneID && c.Shortcut(ui.Cmd, ui.KeyF) {
		*findOpen = !*findOpen
	}
	if a.shell != nil && pane.ID == a.shell.ActivePaneID {
		a.syncActiveDocCompanionState(tab.FilePath, ld.Source, ld.Parsed)
		if ld.ParseErr != nil {
			a.shell.StatusSaveLabel = "待语法闭合"
			a.shell.StatusWarnLabel = "AsterML 语法待修复"
		} else {
			a.shell.StatusSaveLabel = "实时已保存"
			a.shell.StatusWarnLabel = ""
		}
		if ld.Parsed != nil {
			stepTree := asterdoc.ParseStepTree(ld.Source, ld.Parsed)
			a.shell.StatusMetaLabel = fmt.Sprintf("%d 步骤 · %d 模块", len(stepTree.Flat), len(ld.Parsed.Modules))
		}
	}

	pageCol.Children(func() {
		// 1. Clean Obsidian 1-row Document Header on Desktop/Tablet (Breadcrumbs + single Read/Edit toggle + Find + More)
		if !mobileLayout {
			ui.Row(c).FillWidth().Padding(float32(tok.Spacing4), float32(tok.Spacing16)).
				Background(tok.Surface.Alpha(0.86)).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				chosenCrumb := -1
				ui.Box(c).Grow(1).MinWidth(0).Children(func() {
					bc := components.Breadcrumbs(c, []string{folderLabel, docTitle}, &chosenCrumb)
					if bc.Changed() && chosenCrumb == 0 && a.shell != nil {
						a.shell.SelectedTreePath = folderRel
					}
				})

				if ld.ParseErr != nil {
					components.Badge(c, "待语法闭合", components.BadgeOptions{Tone: components.BadgeWarning}).AlignSelf(ui.Center)
				}

				if tab.Mode == shell.ModeSource {
					components.Badge(c, "高级源码视图", components.BadgeOptions{Tone: components.BadgeWarning}).AlignSelf(ui.Center)
					if components.Button(c, "返回编辑", components.ButtonSecondary).Clicked() {
						setTabMode(shell.ModeEdit)
						tab.Mode = shell.ModeEdit
					}
				} else {
					// Calm compact 2-segment mode switch: [ 编辑 | 实验 ]
					ui.Row(c.Key("doc-mode-seg:"+tab.ID)).AlignSelf(ui.Center).
						Padding(2).Radius(float32(tok.ControlRadius)).
						Background(tok.Elevated).Border(1, tok.Border).Gap(2).Children(func() {
						edAct := tab.Mode == shell.ModeEdit
						edSeg := ui.ButtonBase(c.Key("doc-mode-ed:"+tab.ID)).
							Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
							AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label("编辑设计模式 (Ctrl+E)")
						if edAct {
							edSeg.Background(tok.Surface)
						}
						edSeg.Children(func() {
							col := tok.TextMuted
							if edAct {
								col = tok.Text
							}
							ui.Text(c, "编辑").FontSize(float32(tok.TextMeta)).Bold().TextColor(col).SingleLine()
						})
						if edSeg.Clicked() && !edAct {
							setTabMode(shell.ModeEdit)
							tab.Mode = shell.ModeEdit
						}

						labAct := tab.Mode == shell.ModeLab
						labSeg := ui.ButtonBase(c.Key("doc-mode-lab:"+tab.ID)).
							Height(float32(tok.Spacing24)-2).Padding(0, float32(tok.Spacing8)).
							AlignItems(ui.Center).Radius(float32(tok.ControlRadius)).Label("实验执行模式 · 防误触 (Ctrl+E)")
						if labAct {
							labSeg.Background(tok.AccentSoftBg)
						}
						labSeg.Children(func() {
							col := tok.TextMuted
							if labAct {
								col = tok.Accent
							}
							ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
								ui.Icon(c, lucide.FlaskConical()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(col)
								ui.Text(c, "实验").FontSize(float32(tok.TextMeta)).Bold().TextColor(col).SingleLine()
							})
						})
						if labSeg.Clicked() && !labAct {
							setTabMode(shell.ModeLab)
							tab.Mode = shell.ModeLab
						}
					})
				}

				findBtn := components.IconButton(c, lucide.Search(), "页内查找 (Ctrl+F)", components.IconButtonPlain)
				if *findOpen {
					findBtn.Background(tok.AccentSoftBg)
				}
				if findBtn.Clicked() {
					*findOpen = !*findOpen
				}

				moreBtn := components.IconButton(c, lucide.Ellipsis(), "更多文档操作", components.IconButtonPlain)
				components.MenuButton(c, moreBtn, func(m *components.Menu) {
					if m.Item("导出 PDF 执行单…").Chosen() && ld.Parsed != nil {
						a.startDocumentPDFExport(c, docTitle, "", "", a.vault.Root, ld.Parsed)
					}
					if m.Item("另存为模板").Chosen() {
						if tpl, err := a.vault.SaveAsTemplate(tab.FilePath, docTitle); err == nil {
							a.refreshVaultTreeAndCalendar()
							c.AddToast(ui.Toast{Title: "已保存为模板：" + tpl.Name, Type: components.ToastSuccess})
						}
					}
					if m.Item("创建副本").Chosen() {
						if dup, err := a.vault.DuplicateFile(tab.FilePath, docDate); err == nil {
							a.refreshVaultTreeAndCalendar()
							a.openVaultFileInActivePane(dup.RelPath, dup.Title)
						}
					}
					m.Separator()
					if m.Item("复制纯文本原文").Chosen() {
						c.Services().WriteClipboard(ld.Source)
						c.AddToast(ui.Toast{Title: "已复制文档原文", Type: components.ToastSuccess})
					}
					srcLbl := "查看源码（高级）"
					if tab.Mode == shell.ModeSource {
						srcLbl = "返回可视化编辑"
					}
					if m.Item(srcLbl).Chosen() {
						if tab.Mode == shell.ModeSource {
							setTabMode(shell.ModeEdit)
							tab.Mode = shell.ModeEdit
						} else {
							setTabMode(shell.ModeSource)
							tab.Mode = shell.ModeSource
						}
					}
					m.Separator()
					if m.Item("在文件管理器中显示").Chosen() {
						a.revealPathInOS(tab.FilePath)
					}
					if m.Item("移入回收站").Chosen() {
						a.deleteVaultFile(tab.FilePath)
					}
				})
			})
			ui.Box(c).FillWidth().Height(1).Background(tok.Border)
		}

		// In-page Find Bar (Ctrl+F)
		if *findOpen && tab.Mode != shell.ModeSource {
			ui.Row(c.Key("doc-find-bar")).FillWidth().Padding(float32(tok.Spacing4), float32(tok.Spacing16)).
				Background(tok.Elevated).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, lucide.Search()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
				ui.Box(c).Width(float32(tok.Spacing48)*6).Children(func() {
					tb := components.TextBox(c, findQuery, components.TextBoxOptions{}).Placeholder("在当前实验步骤与模块中查找…")
					tb.AutoFocus()
					if tb.Shortcut(0, ui.KeyEscape) {
						*findOpen = false
						*findQuery = ""
					}
				})
				if strings.TrimSpace(*findQuery) != "" {
					qLower := strings.ToLower(strings.TrimSpace(*findQuery))
					hits := strings.Count(strings.ToLower(ld.Source), qLower)
					components.Badge(c, fmt.Sprintf("%d 处匹配", hits))
				}
				ui.Spacer(c)
				if components.IconButton(c, lucide.X(), "关闭查找", components.IconButtonCompact).Clicked() {
					*findOpen = false
					*findQuery = ""
				}
			})
			ui.Box(c).FillWidth().Height(1).Background(tok.Border)
		}

		// 2. Source Mode
		if tab.Mode == shell.ModeSource {
			if a.liveEditorSynced == nil {
				a.liveEditorSynced = map[string]string{}
			}
			srcEd := a.ensureLiveSourceEditor(tab.FilePath, ld.Source)
			bufText := strings.ReplaceAll(srcEd.Buffer.String(), "\r\n", "\n")
			curSource := strings.ReplaceAll(ld.Source, "\r\n", "\n")
			if bufText != curSource {
				a.liveEditorSynced[tab.FilePath] = bufText
				ld.ApplySourceDraft(bufText)
				if ld.ParseErr == nil {
					a.refreshVaultTreeAndCalendar()
				}
			}
			ui.Column(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
				if ld.ParseErr != nil {
					ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
						Background(tok.DangerBg).Children(func() {
						ui.Text(c, ld.ParseErr.Error()).FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText)
					})
				}
				ui.Box(c).FillWidth().Grow(1).MinHeight(0).Background(tok.CodeBg).Children(func() {
					editor.View(c, srcEd).Fill()
				})
			})
			return
		}

		// 3. Parse failure fallback in Edit/Read
		if ld.ParseErr != nil || ld.Parsed == nil {
			ui.Column(c).FillWidth().Grow(1).Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing12)).Children(func() {
				errTxt := "文档语法待修复"
				if ld.ParseErr != nil {
					errTxt = ld.ParseErr.Error()
				}
				ui.Row(c).FillWidth().Padding(float32(tok.Spacing12)).Radius(float32(tok.PanelRadius)).
					Background(tok.DangerBg).Border(1, tok.Danger).Children(func() {
					ui.Text(c, errTxt).FontSize(float32(tok.TextBody)).TextColor(tok.DangerText)
				})
				if components.Button(c, "切换到源码模式修复", components.ButtonSecondary).Clicked() {
					setTabMode(shell.ModeSource)
				}
			})
			return
		}

		// 4. Unified Edit / Read Document Canvas (Single Source of Truth)
		a.ensureLiveViewsMap()
		doc := ld.Parsed
		viewKey := pane.ID + "|" + tab.ID + "|" + tab.FilePath
		v := a.liveViews[viewKey]
		if v == nil {
			v = &asterdoc.View{
				Registry:    asterdoc.BuiltinRegistry(),
				UnifiedLive: true,
				Allowed:     map[string]bool{asterdoc.PermDocumentEdit: true},
				AsyncDecode: a.windowReady,
				OnAsyncInvalidate: func() {
					if a.shell != nil {
						a.shell.Invalidate()
					}
				},
			}
			a.liveViews[viewKey] = v
		}
		if v.Allowed == nil {
			v.Allowed = map[string]bool{asterdoc.PermDocumentEdit: true}
		}
		v.Doc = doc
		v.DocumentTitle = docTitle
		v.SearchQuery = *findQuery
		v.BaseDir = a.vault.Root
		v.MobileCompact = mobileLayout
		v.AltHoverPreviewQuery = a.altHoverPreviewQuery
		if a.shell != nil {
			v.DefaultPlateFormat = a.shell.DefaultPlateFormat
			v.DefaultExcessPct = a.shell.DefaultExcessPct
		}
		if a.scrollToStepID != "" && pane.ID == a.shell.ActivePaneID {
			v.ScrollToStepID = a.scrollToStepID
			a.scrollToStepID = ""
		}
		v.PantryLocationMap = a.buildPantryLocationMap()
		if tab.Mode == shell.ModeLab {
			v.Mode = asterdoc.ModeProtocol
			v.LabMode = true
			v.ReadOnly = false // Reserved execution slots & blank note areas stay interactive in Lab Mode!
		} else {
			v.Mode = asterdoc.ModeDesign
			v.LabMode = false
			v.ReadOnly = false
		}
		// Wire unified Semantic Command Bus (astercmd) for Slash menus, `+` buttons, and well photo linking
		cmdMode := astercmd.ModeEdit
		if v.LabMode {
			cmdMode = astercmd.ModeLab
		}
		regEntries := astercmd.DefaultRegistry().SlashEntries("top_level", cmdMode)
		slashItems := make([]asterdoc.SlashCommandItem, 0, len(regEntries))
		for _, re := range regEntries {
			slashItems = append(slashItems, asterdoc.SlashCommandItem{
				CommandID:   re.CommandID,
				Label:       re.Label,
				Detail:      re.Detail,
				Slash:       re.Slash,
				Kind:        re.Kind,
				Preset:      re.Preset,
				SearchTerms: re.SearchTerms,
			})
		}
		v.SlashItems = slashItems
		if a.shell != nil {
			var targets []string
			for _, f := range a.shell.VaultFiles {
				targets = append(targets, f.RelPath)
			}
			v.VaultCompletionTargets = targets
		}
		runSlashCmd := func(item asterdoc.SlashCommandItem, stepID string, insertOffset int) {
			if item.CommandID == "asset.embed" {
				paths, pErr := mygo.Dialog.Open(mygo.OpenDialogOptions{
					Parent: mygo.FocusedWindow(),
					Title:  "选择要嵌入的图片或实验附件",
				})
				if pErr != nil || len(paths) == 0 {
					return
				}
				refPath := paths[0]
				if a.shell == nil || a.shell.AutoArchiveAssets {
					if rel, impErr := a.vault.ImportAssetFile(paths[0]); impErr == nil {
						refPath = rel
					}
				}
				res := astercmd.ExecuteBatch(ld.Source, astercmd.BatchRequest{
					Mode: cmdMode,
					Operations: []astercmd.Operation{{
						CommandID: "asset.embed",
						Args:      map[string]any{"asset": refPath, "at_offset": insertOffset},
					}},
				})
				if res.OK {
					applyLiveSource(res.NextSource)
				}
				return
			}
			args := map[string]any{}
			if stepID != "" {
				args["step_id"] = stepID
			} else if insertOffset >= 0 {
				args["at_offset"] = insertOffset
			}
			if item.Kind != "" && item.CommandID == "block.insert" {
				args["kind"] = item.Kind
			}
			if item.Preset != "" && item.CommandID == "step.create" {
				args["preset"] = item.Preset
			}
			if item.CommandID == "doc.append_note" {
				args["text"] = "现场记录："
			}
			res := astercmd.ExecuteBatch(ld.Source, astercmd.BatchRequest{
				Mode:        cmdMode,
				PlateFormat: v.DefaultPlateFormat,
				ExcessPct:   v.DefaultExcessPct,
				Operations:  []astercmd.Operation{{CommandID: item.CommandID, Args: args}},
			})
			if res.OK {
				applyLiveSource(res.NextSource)
				if stepID == "" {
					v.RequestFocusTailProse()
				}
			} else if res.Error != nil {
				c.AddToast(ui.Toast{Title: res.Error.Message, Type: components.ToastWarning})
			}
		}
		v.OnExecuteSlash = runSlashCmd
		v.OnAttachPhotoToWell = func(plateID string, wells []string) {
			paths, pErr := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: mygo.FocusedWindow(),
				Title:  fmt.Sprintf("为 %s (%s) 选择实验照片", plateID, strings.Join(wells, ",")),
			})
			if pErr != nil || len(paths) == 0 {
				return
			}
			refPath := paths[0]
			if a.shell == nil || a.shell.AutoArchiveAssets {
				if rel, impErr := a.vault.ImportAssetFile(paths[0]); impErr == nil {
					refPath = rel
				}
			}
			targetSpec := fmt.Sprintf("@%s:%s", plateID, strings.Join(wells, ","))
			res := astercmd.ExecuteBatch(ld.Source, astercmd.BatchRequest{
				Mode: cmdMode,
				Operations: []astercmd.Operation{{
					CommandID: "asset.embed",
					Args: map[string]any{
						"asset":   refPath,
						"targets": []string{targetSpec},
						"role":    "obs",
					},
				}},
			})
			if res.OK {
				applyLiveSource(res.NextSource)
				c.AddToast(ui.Toast{Title: fmt.Sprintf("已关联照片到 %s", targetSpec), Type: components.ToastSuccess})
			}
		}
		v.OnChange = func(nextSrc string) {
			applyLiveSource(nextSrc)
		}
		v.OnInsertStepBlock = func(stepID string, afterIdx int, block map[string]any) {
			if next, err := asterdoc.InsertStepBlockInSource(ld.Source, stepID, afterIdx, block); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnUpdateStepBlock = func(stepID, blockID string, props map[string]any) {
			if next, err := asterdoc.UpdateStepBlockInSource(ld.Source, stepID, blockID, props); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnRemoveStepBlock = func(stepID, blockID string) {
			if next, err := asterdoc.RemoveStepBlockInSource(ld.Source, stepID, blockID); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnDuplicateBlock = func(stepID, blockID string) {
			if next, err := asterdoc.DuplicateStepBlockInSource(ld.Source, stepID, blockID); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnMoveStepBlock = func(stepID, blockID, dir string) {
			if next, err := asterdoc.MoveStepBlockInSource(ld.Source, stepID, blockID, dir); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnMoveStepBlockIndex = func(stepID, blockID string, toIndex int) {
			if next, err := asterdoc.MoveStepBlockToIndex(ld.Source, stepID, blockID, toIndex); err == nil {
				applyLiveSource(next)
			}
		}
		v.OnImportAssetBytes = func(data []byte, origName string) (string, error) {
			return a.vault.ImportAssetBytes(data, origName)
		}
		v.OnPickAttachmentFile = func(stepID, blockID string) {
			paths, pErr := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: mygo.FocusedWindow(),
				Title:  "选择实验照片或原始附件",
			})
			if pErr != nil || len(paths) == 0 {
				return
			}
			picked := paths[0]
			refPath := picked
			if a.shell == nil || a.shell.AutoArchiveAssets {
				if rel, impErr := a.vault.ImportAssetFile(picked); impErr == nil {
					refPath = rel
				}
			}
			// Append to block's items[] list inside `.asterml`
			var existing []any
			if ld.Parsed != nil {
				for _, m := range ld.Parsed.Modules {
					if m.ID == blockID {
						if arr, ok := m.Data["items"].([]any); ok {
							existing = append(existing, arr...)
						}
					}
					if m.Kind == "step" && m.ID == stepID {
						if blks, ok := m.Data["blocks"].([]any); ok {
							for _, raw := range blks {
								if bm, ok := raw.(map[string]any); ok && fmt.Sprint(bm["id"]) == blockID {
									if arr, ok := bm["items"].([]any); ok {
										existing = append(existing, arr...)
									}
								}
							}
						}
					}
				}
			}
			existing = append(existing, map[string]any{
				"id":    fmt.Sprintf("att-%d", len(existing)+1),
				"label": filepath.Base(picked),
				"src":   refPath,
			})
			if stepID != "" {
				if next, uErr := asterdoc.UpdateStepBlockInSource(ld.Source, stepID, blockID, map[string]any{"items": existing}); uErr == nil {
					applyLiveSource(next)
				}
			} else if ld.Parsed != nil {
				if next, uErr := ld.Parsed.UpdateModule(blockID, map[string]any{"items": existing}); uErr == nil {
					applyLiveSource(next)
				}
			}
		}
		v.OnPickFigureFile = func(stepID, blockID string) {
			paths, pErr := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent: mygo.FocusedWindow(),
				Title:  "选择示意图或照片（PNG / JPG / SVG）",
			})
			if pErr != nil || len(paths) == 0 {
				return
			}
			picked := paths[0]
			refPath := picked
			if a.shell == nil || a.shell.AutoArchiveAssets {
				if rel, impErr := a.vault.ImportAssetFile(picked); impErr == nil {
					refPath = rel
				}
			}
			props := map[string]any{"src": refPath}
			if strings.EqualFold(filepath.Ext(picked), ".svg") {
				if raw, rErr := os.ReadFile(picked); rErr == nil && strings.Contains(string(raw), "<svg") {
					props["svg"] = string(raw)
				}
			}
			if stepID == "" && ld.Parsed != nil {
				if next, uErr := ld.Parsed.UpdateModule(blockID, props); uErr == nil {
					applyLiveSource(next)
				}
			} else if next, uErr := asterdoc.UpdateStepBlockInSource(ld.Source, stepID, blockID, props); uErr == nil {
				applyLiveSource(next)
			}
		}
		v.OnCopyImage = func(pngBytes []byte) error {
			if len(pngBytes) == 0 {
				return nil
			}
			return mygo.Clipboard.WriteImage(pngBytes)
		}
		v.OnSavePlateExport = func(defaultName, format string, data []byte) error {
			if len(data) == 0 {
				return nil
			}
			title := "导出组件文件"
			switch format {
			case "csv":
				title = "导出孔板 CSV 布局"
			case "png":
				title = "导出组件 PNG 图像"
			case "svg":
				title = "导出组件 SVG 矢量图"
			case "md":
				title = "导出组件 Markdown"
			}
			path, sErr := mygo.Dialog.Save(mygo.SaveDialogOptions{
				Parent:      mygo.FocusedWindow(),
				Title:       title,
				DefaultPath: defaultName,
			})
			if sErr == nil && path != "" {
				if wErr := os.WriteFile(path, data, 0o644); wErr != nil {
					return wErr
				}
				c.AddToast(ui.Toast{Title: "已导出：" + filepath.Base(path), Type: components.ToastSuccess})
			}
			return nil
		}
		v.OnLocatePantryCode = func(code string) {
			if a.tools == nil {
				a.tools = a.newToolsWorkspace()
			}
			a.tools.LocatePantryItem(code)
			if a.shell != nil {
				a.shell.OpenPantryTab()
			}
		}
		v.OnTimerAction = func(stepID, blockID, action string) {
			var curProps map[string]any
			if ld.Parsed != nil {
				for _, m := range ld.Parsed.Modules {
					if m.Kind == "step" && m.ID == stepID {
						if blks, ok := m.Data["blocks"].([]any); ok {
							for _, raw := range blks {
								if bm, ok := raw.(map[string]any); ok && fmt.Sprint(bm["id"]) == blockID {
									curProps = bm
									break
								}
							}
						}
					}
				}
			}
			nowRFC := time.Now().UTC().Format(time.RFC3339)
			patch := map[string]any{}
			switch action {
			case "start":
				patch["status"] = "running"
				patch["started_at"] = nowRFC
			case "pause":
				elapsed := 0
				if curProps != nil {
					if n, ok := curProps["elapsed_sec"].(int); ok {
						elapsed = n
					}
					if sa, _ := curProps["started_at"].(string); sa != "" {
						if started, err := time.Parse(time.RFC3339, sa); err == nil {
							elapsed += int(time.Since(started).Seconds())
						}
					}
				}
				patch["status"] = "paused"
				patch["elapsed_sec"] = elapsed
			case "complete":
				patch["status"] = "completed"
				patch["completed_at"] = nowRFC
			}
			if len(patch) > 0 {
				if next, err := asterdoc.UpdateStepBlockInSource(ld.Source, stepID, blockID, patch); err == nil {
					applyLiveSource(next)
				}
			}
		}

		// Sync unified Document Outline (plain chapters + orthogonal nested steps)
		if pane.ID == a.shell.ActivePaneID {
			docOutline := asterdoc.ParseDocumentOutline(ld.Source, doc)
			var outline []shell.OutlineItem
			for _, on := range docOutline {
				meta := ""
				if on.IsStep {
					if on.RunningTimers > 0 {
						meta = "计时中"
					} else if v.LabMode && on.ChecksTotal > 0 {
						if on.ChecksDone >= on.ChecksTotal {
							meta = "✓"
						} else {
							meta = fmt.Sprintf("%d/%d", on.ChecksDone, on.ChecksTotal)
						}
					}
				}
				outline = append(outline, shell.OutlineItem{
					ID:        on.ID,
					Title:     on.Title,
					IsStep:    on.IsStep,
					Number:    on.Number,
					Depth:     on.Depth,
					StepDepth: on.StepDepth,
					Meta:      meta,
					Active:    on.ID == a.designSelected,
				})
			}
			a.shell.OutlineSteps = outline
		}

		ui.Box(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			docScroll := ui.Scroll(c).Fill()
			if mobileLayout && a.shell != nil {
				docScroll.TrackScroll(a.shell.MobileContentScrollState()).
					ScrollbarInsets(a.shell.MobileTopReserve(c), 0, a.shell.MobileBottomReserve(c), 0)
			}
			padTop := float32(tok.Spacing24)
			if !mobileLayout {
				// Reserve breathing room for the centered floating pure-icon toolbar at top of canvas
				padTop = float32(tok.Spacing48) + float32(tok.Spacing16)
			}
			padH := float32(tok.Spacing24)
			padBot := float32(tok.Spacing48)
			if mobileLayout && a.shell != nil {
				padTop = a.shell.MobileTopReserve(c) + float32(tok.Spacing12)
				padH = float32(tok.Spacing12) + 2
				padBot = a.shell.MobileBottomReserve(c) + float32(tok.Spacing24)
			}
			docScroll.Children(func() {
				// Readable line length adapts smoothly to viewport width (widens gently on 27"+ monitors)
				winW, _ := c.Size()
				maxColW := float32(780)
				wideColW := float32(960)
				if winW >= 1800 {
					maxColW = 860
					wideColW = 1060
				}
				if doc != nil {
					for _, m := range doc.Modules {
						if m.Kind == "plate" || m.Kind == "data_table" {
							maxColW = wideColW
							break
						}
						if m.Kind == "step" {
							if blks, ok := m.Data["blocks"].([]any); ok {
								for _, raw := range blks {
									if bm, ok := raw.(map[string]any); ok {
										k := fmt.Sprint(bm["kind"])
										if k == "plate" || k == "data_table" {
											maxColW = wideColW
											break
										}
									}
								}
							}
						}
					}
				}
				ui.Row(c).FillWidth().Justify(ui.Center).Children(func() {
					ui.Column(c).FillWidth().MaxWidth(maxColW).
						Padding(padTop, padH, padBot, padH).
						Gap(float32(tok.Spacing16)).Children(func() {
						// Clean unframed `# Title` (Obsidian/Notion borderless display heading)
						if tab.Mode == shell.ModeEdit {
							if a.liveTitleDrafts == nil {
								a.liveTitleDrafts = map[string]*string{}
							}
							tBuf := a.liveTitleDrafts[tab.FilePath]
							if tBuf == nil {
								s := docTitle
								tBuf = &s
								a.liveTitleDrafts[tab.FilePath] = tBuf
							}
							ti := asterdoc.BorderlessTextInput(c, tok, tBuf, float32(tok.TextPage), true, "输入实验名称…")
							if ti.Submitted() || (!ti.Focused() && strings.TrimSpace(*tBuf) != "" && strings.TrimSpace(*tBuf) != docTitle) {
								next := asterdoc.SetFrontmatterMetadata(ld.Source, strings.TrimSpace(*tBuf), docDate, docStatus)
								applyLiveSource(next)
							}
						} else {
							ui.Text(c, docTitle).FontSize(float32(tok.TextPage)).Bold().TextColor(tok.Text)
						}

						// Obsidian Properties: borderless soft gray card on Mobile (matching iOS Obsidian), quiet single-line strip on Desktop
						renderPropsFields := func() {
							// Date property
							if a.liveDateDrafts == nil {
								a.liveDateDrafts = map[string]*string{}
							}
							dBuf := a.liveDateDrafts[tab.FilePath]
							if dBuf == nil {
								s := docDate
								dBuf = &s
								a.liveDateDrafts[tab.FilePath] = dBuf
							}
							dateBadge := firstNonEmpty(docDate, "未排期")
							projFolders := append([]string{""}, a.vault.ProjectFolders()...)
							folderVal := folderRel
							statusLabels := map[string]string{
								"planned":     "计划中",
								"in_progress": "进行中",
								"completed":   "已完成",
							}
							sVal := docStatus

							propRow := func(icon *ui.SVG, label string, valueFn func()) {
								ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Padding(float32(tok.Spacing4)*0.5, 0).Children(func() {
									ui.Row(c).Width(float32(tok.Spacing48)*1.8).Shrink(0).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
										ui.Icon(c, icon).Size(float32(tok.Spacing16)*0.9, float32(tok.Spacing16)*0.9).TextColor(tok.TextMuted)
										ui.Text(c, label).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
									})
									ui.Box(c).Grow(1).MinWidth(0).Children(valueFn)
								})
							}

							if mobileLayout {
								ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
									ui.Text(c, "实验属性").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text)
									ui.Column(c).FillWidth().Padding(float32(tok.Spacing12), float32(tok.Spacing16)).
										Radius(float32(tok.PanelRadius)).Background(tok.Elevated).Gap(float32(tok.Spacing8)).Children(func() {
										propRow(lucide.Calendar(), "日期", func() {
											if tab.Mode == shell.ModeEdit {
												dt := asterdoc.BorderlessTextInput(c, tok, dBuf, float32(tok.TextBody), false, "YYYY-MM-DD").
													FontFeatures("tnum")
												if dt.Submitted() || (!dt.Focused() && strings.TrimSpace(*dBuf) != docDate) {
													rawDate := strings.TrimSpace(*dBuf)
													if rawDate == "" {
														next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, "", docStatus)
														applyLiveSource(next)
													} else if normDate := asterdoc.NormalizeDateYMD(rawDate); normDate != "" {
														*dBuf = normDate
														if normDate != docDate {
															next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, normDate, docStatus)
															applyLiveSource(next)
														}
													} else {
														*dBuf = docDate
													}
												}
											} else {
												ui.Text(c, dateBadge).FontSize(float32(tok.TextBody)).TextColor(tok.Text).FontFeatures("tnum")
											}
										})
										propRow(lucide.Folder(), "项目", func() {
											if tab.Mode == shell.ModeEdit {
												pSel := components.Select(c, &folderVal)
												asterdoc.StyleSelectQuiet(c, tok, pSel)
												pSel.Trigger.Children(func() {
													ui.Text(c, folderLabel).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
												})
												pSel.Popup(func(panel ui.Element) {
													for _, pf := range projFolders {
														pf := pf
														lbl := firstNonEmpty(pf, "根目录")
														pSel.Item(pf).Children(func() {
															ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
														})
													}
												})
												if pSel.Trigger.Changed() && folderVal != folderRel {
													a.moveVaultFile(tab.FilePath, folderVal)
												}
											} else {
												ui.Text(c, folderLabel).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
											}
										})
										propRow(lucide.Tag(), "状态", func() {
											sSel := components.Select(c, &sVal)
											asterdoc.StyleSelectQuiet(c, tok, sSel)
											sSel.Trigger.Children(func() {
												components.Badge(c, statusLabels[sVal], components.BadgeOptions{Tone: components.BadgeAccent})
											})
											sSel.Popup(func(panel ui.Element) {
												for _, opt := range []string{"planned", "in_progress", "completed"} {
													opt := opt
													sSel.Item(opt).Children(func() {
														ui.Text(c, statusLabels[opt]).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
													})
												}
											})
											if sSel.Trigger.Changed() && sVal != docStatus {
												next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, docDate, sVal)
												applyLiveSource(next)
											}
										})
									})
								})
								return
							}

							ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
									ui.Icon(c, lucide.Calendar()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.TextMuted)
									if tab.Mode == shell.ModeEdit {
										ui.Box(c).Width(float32(tok.Spacing48)*1.9).Children(func() {
											dt := asterdoc.BorderlessTextInput(c, tok, dBuf, float32(tok.TextMeta), false, "YYYY-MM-DD").
												FontFeatures("tnum")
											if dt.Submitted() || (!dt.Focused() && strings.TrimSpace(*dBuf) != docDate) {
												rawDate := strings.TrimSpace(*dBuf)
												if rawDate == "" {
													next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, "", docStatus)
													applyLiveSource(next)
												} else if normDate := asterdoc.NormalizeDateYMD(rawDate); normDate != "" {
													*dBuf = normDate
													if normDate != docDate {
														next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, normDate, docStatus)
														applyLiveSource(next)
													}
												} else {
													*dBuf = docDate
												}
											}
										})
									} else {
										ui.Text(c, dateBadge).FontSize(float32(tok.TextData)).TextColor(tok.TextMuted).FontFeatures("tnum")
									}
								})

								if tab.Mode == shell.ModeEdit {
									pSel := components.Select(c, &folderVal)
									asterdoc.StyleSelectQuiet(c, tok, pSel)
									pSel.Trigger.Children(func() {
										ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
											ui.Icon(c, lucide.Folder()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.TextMuted)
											ui.Text(c, folderLabel).FontSize(float32(tok.TextData)).TextColor(tok.Text)
										})
									})
									pSel.Popup(func(panel ui.Element) {
										for _, pf := range projFolders {
											pf := pf
											lbl := firstNonEmpty(pf, "根目录")
											pSel.Item(pf).Children(func() {
												ui.Text(c, lbl).FontSize(float32(tok.TextData)).TextColor(tok.Text)
											})
										}
									})
									if pSel.Trigger.Changed() && folderVal != folderRel {
										a.moveVaultFile(tab.FilePath, folderVal)
									}
								} else {
									ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
										ui.Icon(c, lucide.Folder()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.TextMuted)
										ui.Text(c, folderLabel).FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
									})
								}

								sSel := components.Select(c, &sVal)
								asterdoc.StyleSelectQuiet(c, tok, sSel)
								sSel.Trigger.Children(func() {
									ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
										ui.Box(c).Size(float32(tok.Spacing4)*1.5, float32(tok.Spacing4)*1.5).Radius(float32(tok.ControlRadius)).Background(tok.Accent)
										ui.Text(c, statusLabels[sVal]).FontSize(float32(tok.TextData)).TextColor(tok.Accent)
									})
								})
								sSel.Popup(func(panel ui.Element) {
									for _, opt := range []string{"planned", "in_progress", "completed"} {
										opt := opt
										sSel.Item(opt).Children(func() {
											ui.Text(c, statusLabels[opt]).FontSize(float32(tok.TextData)).TextColor(tok.Text)
										})
									}
								})
								if sSel.Trigger.Changed() && sVal != docStatus {
									next := asterdoc.SetFrontmatterMetadata(ld.Source, docTitle, docDate, sVal)
									applyLiveSource(next)
								}
							})
							ui.Box(c).FillWidth().Height(1).Background(tok.Border.Alpha(0.45))
						}
						renderPropsFields()

						// Render the single-source document body & step modules
						v.Render(c)

						// Deterministic Pure-Go Scientific Analysis Projection (CCK-8 / RT-qPCR ΔΔCq / Descriptive)
						if summaries := a.cachedDocumentAnalysis(tab.FilePath, ld.Source, doc); len(summaries) > 0 {
							analOpen := ui.Local(c.Root(), "doc-analysis-open:"+pane.ID+":"+tab.ID, func() bool { return true })
							ui.Column(c.Key("doc-analysis-card:"+tab.FilePath)).FillWidth().
								Padding(float32(tok.Spacing12)).Radius(float32(tok.PanelRadius)).
								Background(tok.Surface).Border(1, tok.Border).Gap(float32(tok.Spacing8)).Children(func() {
								hdrBtn := ui.Row(c.Key("doc-analysis-hdr")).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8))
								chev := lucide.ChevronRight()
								if *analOpen {
									chev = lucide.ChevronDown()
								}
								hdrBtn.Children(func() {
									ui.Icon(c, chev).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
									ui.Text(c, "读数自动汇总").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
									ui.Spacer(c)
									components.Badge(c, fmt.Sprintf("%d 项统计", len(summaries)))
								})
								if hdrBtn.Clicked() {
									*analOpen = !*analOpen
								}
								if *analOpen {
									for sIdx, sum := range summaries {
										sum := sum
										ui.Column(c.Key(fmt.Sprintf("sum-%d-%s", sIdx, sum.Kind))).FillWidth().
											Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
											Background(tok.Elevated).Gap(float32(tok.Spacing4)).Children(func() {
											ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
												ui.Text(c, sum.Title).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text)
												if sum.Control != "" {
													ui.Text(c, "对照："+sum.Control).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
												}
												if sum.BlankN > 0 && sum.BlankMean != nil {
													ui.Text(c, fmt.Sprintf("空白本底 %.3f（%d 孔）", *sum.BlankMean, sum.BlankN)).
														FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
												}
												if sum.ExcludedN > 0 {
													ui.Text(c, fmt.Sprintf("已剔除 %d 异常孔", sum.ExcludedN)).FontSize(float32(tok.TextMeta)).TextColor(tok.Warning).FontFeatures("tnum")
												}
											})
											switch sum.Kind {
											case "cck8":
												if sum.CCK8 != nil {
													for _, g := range sum.CCK8.Groups {
														ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
															ui.Text(c, g.Group).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).Width(140).SingleLine().Ellipsis("…")
															ui.Text(c, fmt.Sprintf("均值 %.3f ± %.3f（%d 复孔）", g.Mean, g.SD, g.N)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").Grow(1)
															ui.Text(c, fmt.Sprintf("相对活力 %.1f%%", g.RelativeActivity*100)).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Accent).FontFeatures("tnum")
														})
													}
												}
											case "qpcr":
												if sum.QPCR != nil {
													for _, g := range sum.QPCR.Groups {
														ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
															ui.Text(c, g.SampleID).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).Width(140).SingleLine().Ellipsis("…")
															ui.Text(c, fmt.Sprintf("ΔΔCq %.2f", g.DeltaDeltaCq)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").Grow(1)
															ui.Text(c, fmt.Sprintf("相对表达 %.2f×", g.FoldChange)).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Accent).FontFeatures("tnum")
														})
													}
												}
											default:
												for _, g := range sum.Groups {
													ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
														ui.Text(c, g.Group).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).Width(140).SingleLine().Ellipsis("…")
														repTxt := fmt.Sprintf("%d 复孔", g.TechReplicates)
														if g.BioSamples > 0 {
															repTxt = fmt.Sprintf("%d 复孔 · 独立 n=%d", g.TechReplicates, g.BioSamples)
														}
														statTxt := fmt.Sprintf("均值 %.3f ± %.3f（%s）", g.Mean, g.SD, repTxt)
														ui.Text(c, statTxt).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum").Grow(1)
														if g.BlankCorrMean != nil {
															ui.Text(c, fmt.Sprintf("扣空白 %.3f", *g.BlankCorrMean)).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Accent).FontFeatures("tnum")
														}
													})
												}
											}
										})
									}
								}
							})
						}

					})
				})
			})
			// Centered top floating pure-icon toolbar on desktop/tablet (locked in-place during Lab Mode)
			if !mobileLayout {
				ui.Row(c.Key("doc-ftb-anchor")).Absolute().Top(float32(tok.Spacing12)).Left(0).Right(0).
					Justify(ui.Center).HitTest(func(float32, float32, ui.Rect) bool { return false }).Children(func() {
					v.RenderFloatingToolbar(c, tab.Mode == shell.ModeLab)
				})
			}
			v.RenderPlateOverlay(c)
		})
	})
}
