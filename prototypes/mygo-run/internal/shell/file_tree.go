package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/vault"
)

type fileDragPayload struct {
	RelPath string
	Title   string
	IsDir   bool
	Date    string
}

// renderLeftRibbonAndTree draws the 40px Obsidian-style icon Ribbon on the far left
// alongside the Vault Project File Tree inside the left sidebar slot.
func (s *Shell) navigation(c *ui.Context) {
	s.EnsureTabs()
	tok := theme.GetTokens(c.Theme())
	var inset float32
	if s.window != nil {
		inset = sidebar.HeaderHeight(c)
	}

	ui.Row(c).Fill().Padding(inset, 0, 0, 0).Gap(0).Children(func() {
		// 1. 40px Compact Ribbon Bar
		s.renderRibbon(c, tok)
		ui.Box(c).Width(1).FillHeight().Background(tok.Border.Alpha(0.45))
		// 2. Vault Project File Tree
		ui.Column(c).Grow(1).MinWidth(0).FillHeight().Children(func() {
			s.renderFileTreePane(c, tok)
		})
	})
}

func (s *Shell) renderRibbon(c *ui.Context, tok theme.Tokens) {
	activeTab := s.ActiveTab()
	calActive := activeTab != nil && activeTab.Kind == TabCalendar
	pantryActive := activeTab != nil && activeTab.Kind == TabPantry

	ribbonBg := tok.Elevated.Alpha(0.75)
	if c.Vibrancy() {
		ribbonBg = tok.Text.Alpha(0.025)
	}
	ui.Column(c).Width(float32(tok.Spacing48)).FillHeight().
		Padding(float32(tok.Spacing8), float32(tok.Spacing4)).
		Gap(float32(tok.Spacing8)).AlignItems(ui.Center).
		Background(ribbonBg).Children(func() {
		// AsterLab Brand Mark (optical small variant) -> opens About card
		brandBtn := components.IconButton(c, theme.LogoParsedSmall(), "关于 AsterLab", components.IconButtonPlain)
		brandBtn.TextColor(tok.Accent)
		if s.VaultSettingsOpen && s.SettingsView.Category == "about" {
			brandBtn.Background(tok.AccentSoftBg)
		}
		if brandBtn.Clicked() {
			s.SettingsView.Category = "about"
			s.VaultSettingsOpen = true
		}
		ui.Box(c).Width(float32(tok.Spacing24)).Height(1).Background(tok.Border.Alpha(0.7))

		// Toggle file tree (Ctrl+B)
		treeBtn := components.IconButton(c, lucide.FolderTree(), "切换文件树 (Ctrl+B)", components.IconButtonPlain)
		if s.window != nil && s.window.LeftExpanded() {
			treeBtn.Background(tok.AccentSoftBg)
		}
		if treeBtn.Clicked() && s.window != nil {
			s.window.SetLeftExpanded(!s.window.LeftExpanded())
		}

		// Calendar Home
		calBtn := components.IconButton(c, lucide.CalendarDays(), "实验日程 (Ctrl+1)", components.IconButtonPlain)
		if calActive {
			calBtn.Background(tok.AccentSoftBg)
		}
		if calBtn.Clicked() {
			s.OpenCalendarTab()
		}

		// Storage Pantry
		pantryBtn := components.IconButton(c, lucide.Package(), "储藏室", components.IconButtonPlain)
		if pantryActive {
			pantryBtn.Background(tok.AccentSoftBg)
		}
		if pantryBtn.Clicked() {
			s.OpenPantryTab()
		}

		// Quick Search / Switcher (Ctrl+O / Ctrl+K)
		searchBtn := components.IconButton(c, lucide.Search(), "快速打开与命令 (Ctrl+K)", components.IconButtonPlain)
		if searchBtn.Clicked() {
			s.PaletteOpen = true
			s.PaletteCursor = 0
		}

		ui.Spacer(c)

		// Vault Settings
		setBtn := components.IconButton(c, lucide.Settings(), "设置 (Ctrl+,)", components.IconButtonPlain)
		if s.VaultSettingsOpen {
			setBtn.Background(tok.AccentSoftBg)
		}
		if setBtn.Clicked() {
			s.VaultSettingsOpen = true
		}
	})
}

func (s *Shell) renderFileTreePane(c *ui.Context, tok theme.Tokens) {
	// Clean desktop header toolbar with proportional standard icons
	ui.Row(c).FillWidth().Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
		AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
		components.Text(c, "实验库").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted).
			Grow(1).MinWidth(0)
		if components.IconButton(c, lucide.FilePlus(), "新建实验 (Ctrl+N)", components.IconButtonPlain).Clicked() {
			s.triggerVaultNewFile("")
		}
		if components.IconButton(c, lucide.FolderPlus(), "新建项目文件夹", components.IconButtonPlain).Clicked() {
			s.startNewFolderPrompt("")
		}
		if components.IconButton(c, lucide.ChevronsDownUp(), "折叠全部文件夹", components.IconButtonPlain).Clicked() {
			for k := range s.folderOpen {
				s.folderOpen[k] = false
			}
		}
	})
	ui.Divider(c)
	s.renderFileTreeScrollBody(c, tok)
}

func (s *Shell) renderFileTreeScrollBody(c *ui.Context, tok theme.Tokens) {
	if s.folderOpen == nil {
		s.folderOpen = map[string]bool{}
	}

	scrollArea := ui.Scroll(c).FillWidth().Grow(1).MinHeight(0)
	scrollArea.Children(func() {
		ui.Column(c).FillWidth().Padding(float32(tok.Spacing4)).Gap(float32(tok.Spacing4)).Children(func() {
			if s.VaultTree == nil || len(s.VaultTree.Children) == 0 {
				ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).Gap(float32(tok.Spacing4)).Children(func() {
					components.Text(c, "暂无实验文件").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					if components.Button(c, "新建实验", components.ButtonGhost).Clicked() {
						s.triggerVaultNewFile("")
					}
				})
				return
			}
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing4) * 0.5).Children(func() {
				for _, node := range s.VaultTree.Children {
					s.renderTreeNode(c, tok, node, 0)
				}
			})
		})
	})

	// Drop onto empty root tree area moves item to Vault root ""
	if dragged, over := ui.DragOver[fileDragPayload](scrollArea); over && dragged.RelPath != "" {
		scrollArea.Border(1, tok.Accent)
	}
	if dragged, dropped := ui.Drop[fileDragPayload](scrollArea); dropped && dragged.RelPath != "" {
		if s.OnVaultMove != nil {
			s.OnVaultMove(dragged.RelPath, "")
		}
	}

	// Context menu on file tree background
	components.ContextMenu(c, scrollArea, func(m *components.Menu) {
		if m.Item("新建实验").Shortcut("Ctrl+N").Chosen() {
			s.triggerVaultNewFile("")
		}
		if m.Item("从模板新建…").Chosen() {
			if s.OnOpenTemplatePicker != nil {
				s.OnOpenTemplatePicker(s.ensureSelectedDate())
			}
		}
		if m.Item("新建项目文件夹").Chosen() {
			s.startNewFolderPrompt("")
		}
		m.Separator()
		if m.Item("在资源管理器中显示").Chosen() && s.OnRevealInExplorer != nil {
			s.OnRevealInExplorer("")
		}
	})
}

func renderTreeIndentGuides(c *ui.Context, tok theme.Tokens, depth int) {
	for d := 0; d < depth; d++ {
		ui.Row(c).Width(float32(tok.Spacing16)).Height(float32(tok.RowHeight)).AlignItems(ui.Center).Children(func() {
			ui.Box(c).Width(float32(tok.Spacing8)).Height(1)
			ui.Box(c).Width(1).FillHeight().Background(tok.Border.Alpha(0.55))
		})
	}
}

func (s *Shell) renderTreeNode(c *ui.Context, tok theme.Tokens, node *vault.TreeNode, depth int) {
	if node == nil {
		return
	}
	if node.IsDir {
		if _, seen := s.folderOpen[node.RelPath]; !seen {
			// Project folders start expanded; assets/ starts collapsed at bottom.
			s.folderOpen[node.RelPath] = node.Name != vault.AssetsDirName
		}
		open := s.folderOpen[node.RelPath]
		selected := s.SelectedTreePath == node.RelPath

		// Inline F2 rename mode for folder
		if s.renamingRelPath == node.RelPath {
			s.renderInlineTreeRename(c, tok, node.RelPath, node.Name)
			return
		}

		icon := lucide.Folder()
		if open {
			icon = lucide.FolderOpen()
		}
		if node.Name == vault.TemplatesDirName {
			icon = lucide.BookOpen()
		} else if node.Name == vault.AssetsDirName {
			icon = lucide.Paperclip()
		}

		row := ui.ButtonBase(c.Key("tree-dir:"+node.RelPath)).Label(node.Name).
			FillWidth().MinWidth(0).Height(float32(tok.RowHeight)).
			Justify(ui.Start).Radius(float32(tok.ControlRadius)+2).FocusRing(true).
			Padding(0, float32(tok.Spacing8), 0, float32(tok.Spacing4)).
			Selected(selected).TextColor(tok.Text)
		targetBg := ui.Transparent
		if selected {
			targetBg = tok.SelectedRow
		} else if row.Hovered() {
			targetBg = tok.Text.Alpha(0.04)
		}
		row.Background(row.AnimateColor("tree-dbg", targetBg, theme.MotionMicroFade))
		theme.ApplyTactilePress(row)
		row.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				renderTreeIndentGuides(c, tok, depth)
				chev := lucide.ChevronRight()
				if open {
					chev = lucide.ChevronDown()
				}
				ui.Box(c).Size(float32(tok.Spacing16), float32(tok.Spacing16)).Center().Shrink(0).Children(func() {
					ui.Icon(c, chev).Size(float32(tok.IconSm), float32(tok.IconSm)).TextColor(tok.TextMuted)
				})
				icoCol := tok.TextMuted
				if selected {
					icoCol = tok.Text
				}
				ui.Icon(c, icon).Size(float32(tok.IconMd), float32(tok.IconMd)).TextColor(icoCol).ContentTransition(theme.IconMorph())
				dispName := node.Name
				if node.IsSystem {
					switch strings.ToLower(node.Name) {
					case "templates":
						dispName = "实验模板"
					case "assets":
						dispName = "图片与附件"
					}
				}
				components.Text(c, dispName).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text).
					SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			})
		})
		if row.Clicked() {
			s.SelectedTreePath = node.RelPath
			s.folderOpen[node.RelPath] = !open
			open = !open
		}
		if selected && row.Shortcut(0, ui.KeyF2) {
			s.renamingRelPath = node.RelPath
			s.renameDraft = node.Name
		}
		if selected && row.Shortcut(0, ui.KeyDelete) && !node.IsSystem {
			if s.OnVaultDelete != nil {
				s.OnVaultDelete(node.RelPath)
			}
		}

		// Drop a dragged file/folder into this folder to move it
		if dragged, over := ui.DragOver[fileDragPayload](row); over && dragged.RelPath != node.RelPath {
			row.Border(1, tok.Accent)
		}
		if dragged, dropped := ui.Drop[fileDragPayload](row); dropped && dragged.RelPath != node.RelPath {
			if s.OnVaultMove != nil {
				s.OnVaultMove(dragged.RelPath, node.RelPath)
			}
		}
		if !node.IsSystem {
			row.Drag(fileDragPayload{RelPath: node.RelPath, Title: node.Name, IsDir: true})
		}

		components.ContextMenu(c, row, func(m *components.Menu) {
			if m.Item("在此文件夹新建实验").Chosen() {
				s.triggerVaultNewFile(node.RelPath)
			}
			if m.Item("从模板新建…").Chosen() {
				s.CreateTargetFolder = node.RelPath
				if s.OnOpenTemplatePicker != nil {
					s.OnOpenTemplatePicker(s.ensureSelectedDate())
				}
			}
			if m.Item("新建子文件夹").Chosen() {
				s.startNewFolderPrompt(node.RelPath)
			}
			m.Separator()
			if m.Item("重命名").Shortcut("F2").Disabled(node.IsSystem).Chosen() {
				s.renamingRelPath = node.RelPath
				s.renameDraft = node.Name
			}
			if m.Item("在资源管理器中显示").Chosen() && s.OnRevealInExplorer != nil {
				s.OnRevealInExplorer(node.RelPath)
			}
			if m.Item("删除").Shortcut("Del").Disabled(node.IsSystem).Chosen() && s.OnVaultDelete != nil {
				s.OnVaultDelete(node.RelPath)
			}
		})
		if open && len(node.Children) > 0 {
			ui.Column(c.Key("tree-dir-body:"+node.RelPath)).FillWidth().
				Gap(float32(tok.Spacing4) * 0.25).
				Transition(theme.AccordionTransition()).Children(func() {
				for _, ch := range node.Children {
					s.renderTreeNode(c, tok, ch, depth+1)
				}
			})
		}
		return
	}

	// File leaf node
	if s.renamingRelPath == node.RelPath {
		s.renderInlineTreeRename(c, tok, node.RelPath, node.Name)
		return
	}

	entry := node.Entry
	isTemplate := entry != nil && entry.IsTemplate
	isAsset := entry != nil && entry.IsAsset
	activeTab := s.ActiveTab()
	selected := s.SelectedTreePath == node.RelPath || (activeTab != nil && activeTab.Kind == TabFile && activeTab.FilePath == node.RelPath)

	icon := lucide.FlaskConical()
	if isTemplate {
		icon = lucide.FileText()
	} else if isAsset {
		icon = lucide.Image()
	}

	// Short date string right-aligned via custom leaf row inside TreeItem
	shortDate := ""
	if entry != nil && !isTemplate && len(entry.Date) >= 10 {
		shortDate = entry.Date[5:10]
	}

	row := ui.ButtonBase(c.Key("tree-file:"+node.RelPath)).Label(node.Name).
		FillWidth().MinWidth(0).Height(float32(tok.RowHeight)).
		Justify(ui.Start).Radius(float32(tok.ControlRadius)+2).FocusRing(true).
		Padding(0, float32(tok.Spacing8), 0, float32(tok.Spacing4)).
		Selected(selected).TextColor(tok.Text).
		Transition(theme.AccordionTransition())
	targetBg := ui.Transparent
	if selected {
		targetBg = tok.SelectedRow
	} else if row.Hovered() {
		targetBg = tok.Text.Alpha(0.04)
	}
	row.Background(row.AnimateColor("tree-bg", targetBg, theme.MotionMicroFade))
	theme.ApplyTactilePress(row)
	row.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
			renderTreeIndentGuides(c, tok, depth)
			// Reserve exact chevron slot so files align strictly with parent folder labels, deeper by depth
			ui.Box(c).Size(float32(tok.Spacing16), float32(tok.Spacing16)).Shrink(0)
			icoCol := tok.TextMuted
			if selected {
				icoCol = tok.Text
			}
			ui.Icon(c, icon).Size(float32(tok.IconMd), float32(tok.IconMd)).TextColor(icoCol)
			components.Text(c, node.Name).FontSize(float32(tok.TextData)).TextColor(tok.Text).
				SingleLine().Ellipsis("…").Grow(1).MinWidth(0)
			if shortDate != "" {
				ui.Text(c, shortDate).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).
					FontFeatures("tnum").Shrink(0)
			}
		})
	})

	if row.Clicked() {
		s.SelectedTreePath = node.RelPath
		if entry != nil && !isAsset {
			s.OpenFileInPane(s.ActivePaneID, entry.RelPath, entry.Title, s.DefaultFileOpenMode(), false)
		} else if isAsset && s.OnRevealInExplorer != nil {
			s.OnRevealInExplorer(node.RelPath)
		}
	}
	if selected && row.Shortcut(0, ui.KeyF2) {
		s.renamingRelPath = node.RelPath
		s.renameDraft = node.Name
	}
	if selected && row.Shortcut(0, ui.KeyDelete) && s.OnVaultDelete != nil {
		s.OnVaultDelete(node.RelPath)
	}

	if !isAsset {
		dateVal := ""
		titleVal := node.Name
		if entry != nil {
			dateVal = entry.Date
			titleVal = entry.Title
		}
		row.Drag(fileDragPayload{RelPath: node.RelPath, Title: titleVal, Date: dateVal})
	}

	components.ContextMenu(c, row, func(m *components.Menu) {
		if entry != nil && !isAsset {
			if m.Item("打开").Chosen() {
				s.OpenFileInPane(s.ActivePaneID, entry.RelPath, entry.Title, s.DefaultFileOpenMode(), false)
			}
			if m.Item("在新标签页打开").Chosen() {
				s.OpenFileInPane(s.ActivePaneID, entry.RelPath, entry.Title, s.DefaultFileOpenMode(), true)
			}
			if m.Item("向右分屏打开").Chosen() {
				s.SplitPaneWithTab(s.ActivePaneID, WorkspaceTab{
					Kind: TabFile, Title: entry.Title, FilePath: entry.RelPath, Mode: s.DefaultFileOpenMode(),
				}, "right")
			}
			if m.Item("向下分屏打开").Chosen() {
				s.SplitPaneWithTab(s.ActivePaneID, WorkspaceTab{
					Kind: TabFile, Title: entry.Title, FilePath: entry.RelPath, Mode: s.DefaultFileOpenMode(),
				}, "down")
			}
			m.Separator()
			if isTemplate && s.OnOpenTemplatePicker != nil && m.Item("以此模板新建实验").Chosen() {
				s.CreateTargetFolder = ""
				s.CreateTargetTemplate = entry.RelPath
				s.OnOpenTemplatePicker(s.ensureSelectedDate())
			}
			if m.Item("创建副本").Chosen() && s.OnVaultDuplicate != nil {
				s.OnVaultDuplicate(entry.RelPath, "")
			}
			if !isTemplate && m.Item("存为模板").Chosen() && s.OnVaultSaveTemplate != nil {
				s.OnVaultSaveTemplate(entry.RelPath)
			}
			if len(s.VaultFolders) > 0 && s.OnVaultMove != nil {
				m.Group("移动至", func(sub *components.Menu) {
					if sub.Item("根目录 /").Chosen() {
						s.OnVaultMove(entry.RelPath, "")
					}
					for _, dir := range s.VaultFolders {
						dir := dir
						if sub.Item(dir).Chosen() {
							s.OnVaultMove(entry.RelPath, dir)
						}
					}
				})
			}
			m.Separator()
		}
		if m.Item("重命名").Shortcut("F2").Chosen() {
			s.renamingRelPath = node.RelPath
			s.renameDraft = node.Name
		}
		if m.Item("在资源管理器中显示").Chosen() && s.OnRevealInExplorer != nil {
			s.OnRevealInExplorer(node.RelPath)
		}
		if m.Item("删除").Shortcut("Del").Chosen() && s.OnVaultDelete != nil {
			s.OnVaultDelete(node.RelPath)
		}
	})
}

func (s *Shell) renderInlineTreeRename(c *ui.Context, tok theme.Tokens, relPath, origName string) {
	ui.Row(c.Key("rename-"+relPath)).FillWidth().Padding(float32(tok.Spacing4)*0.5, float32(tok.Spacing8)).Children(func() {
		tb := components.TextBox(c, &s.renameDraft, components.TextBoxOptions{}).FillWidth()
		tb.AutoFocus()
		tb.OnTextKey(func(k ui.TextKey) bool {
			if k.Key == ui.KeyEnter && k.Mods == 0 {
				next := strings.TrimSpace(s.renameDraft)
				s.renamingRelPath = ""
				if next != "" && next != origName && s.OnVaultRename != nil {
					s.OnVaultRename(relPath, next)
				}
				return true
			}
			if k.Key == ui.KeyEscape {
				s.renamingRelPath = ""
				return true
			}
			return false
		})
	})
}

func (s *Shell) triggerVaultNewFile(folderRel string) {
	if s.OnVaultCreateFile != nil {
		s.OnVaultCreateFile(folderRel, s.ensureSelectedDate())
		return
	}
	s.triggerCreateForDate(s.ensureSelectedDate())
}

func (s *Shell) startNewFolderPrompt(parentRel string) {
	s.newFolderParent = parentRel
	s.newFolderName = ""
	s.newFolderDialogOpen = true
}

func (s *Shell) renderVaultDialogs(c *ui.Context) {
	tok := theme.GetTokens(c.Theme())
	if s.newFolderDialogOpen {
		open := s.newFolderDialogOpen
		components.Dialog(c, &open, components.DialogOptions{Label: "新建项目文件夹"}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*7.5).Gap(float32(tok.Spacing12)).Children(func() {
				components.Text(c, "新建项目文件夹").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
				tb := components.TextBox(c, &s.newFolderName, components.TextBoxOptions{}).Placeholder("输入项目或课题名称").FillWidth()
				tb.AutoFocus()
				submit := func() {
					name := strings.TrimSpace(s.newFolderName)
					if name != "" && s.OnVaultCreateFolder != nil {
						s.OnVaultCreateFolder(s.newFolderParent, name)
						s.newFolderDialogOpen = false
					}
				}
				tb.OnTextKey(func(k ui.TextKey) bool {
					if k.Key == ui.KeyEnter && k.Mods == 0 {
						submit()
						return true
					}
					return false
				})
				ui.Row(c).FillWidth().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "取消", components.ButtonGhost).Clicked() {
						open = false
					}
					if components.Button(c, "创建", components.ButtonPrimary).Clicked() {
						submit()
					}
				})
			})
		})
		s.newFolderDialogOpen = open
	}

	if s.VaultSettingsOpen {
		s.renderObsidianSettingsDialog(c)
	}
	_ = fmt.Sprintf
}
