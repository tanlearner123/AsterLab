package shell

import (
	"fmt"
	"runtime"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"asterlab-mygo-prototype/internal/theme"
)

type hotkeySpec struct {
	ID      string
	Action  string
	Group   string
	Default string
}

var defaultHotkeySpecs = []hotkeySpec{
	{ID: "cmd.palette", Action: "打开命令面板 / 快速切换实验", Group: "导航与搜索", Default: "Ctrl+K / Ctrl+O"},
	{ID: "nav.global_search", Action: "全局全文搜索 Vault", Group: "导航与搜索", Default: "Ctrl+Shift+F"},
	{ID: "vault.new_exp", Action: "新建实验文档", Group: "导航与搜索", Default: "Ctrl+N"},
	{ID: "nav.calendar", Action: "跳回实验日程主页", Group: "导航与搜索", Default: "Ctrl+1"},
	{ID: "nav.back", Action: "浏览历史后退", Group: "导航与搜索", Default: "Alt+←"},
	{ID: "nav.forward", Action: "浏览历史前进", Group: "导航与搜索", Default: "Alt+→"},
	{ID: "nav.left_sidebar", Action: "展开 / 收起左侧实验库", Group: "工作区与分屏", Default: "Ctrl+B"},
	{ID: "nav.right_drawer", Action: "展开 / 收起右侧步骤大纲与速算", Group: "工作区与分屏", Default: "Ctrl+Shift+R"},
	{ID: "pane.split_right", Action: "当前文档向右分屏对照", Group: "工作区与分屏", Default: "Ctrl+\\"},
	{ID: "tab.close", Action: "关闭当前非固定标签页", Group: "工作区与分屏", Default: "Ctrl+W"},
	{ID: "doc.toggle_mode", Action: "切换编辑 / 阅读模式", Group: "实验文档编辑", Default: "Ctrl+E"},
	{ID: "doc.undo", Action: "撤销上一步修改", Group: "实验文档编辑", Default: "Ctrl+Z"},
	{ID: "doc.redo", Action: "重做已撤销修改", Group: "实验文档编辑", Default: "Ctrl+Y"},
	{ID: "tree.rename", Action: "重命名选中项目或实验", Group: "文件树管理", Default: "F2"},
	{ID: "tree.delete", Action: "移入实验库回收站", Group: "文件树管理", Default: "Del"},
	{ID: "app.settings", Action: "打开 Obsidian 风格设置中心", Group: "系统", Default: "Ctrl+,"},
}

func (s *Shell) ensureSettingsDefaults() {
	if s.ThemeSource == "" {
		s.ThemeSource = "system"
	}
	s.SyncThemeLook(nil)
	if s.Density == "" {
		s.Density = "comfortable"
	}
	if s.DefaultOpenMode == "" {
		s.DefaultOpenMode = "edit"
	}
	if s.DefaultPlateFormat == 0 {
		s.DefaultPlateFormat = 96
	}
	if s.DefaultExcessPct == 0 {
		s.DefaultExcessPct = 10
	}
	s.EnsureMobileDefaults()
	if s.Hotkeys == nil {
		s.Hotkeys = map[string]string{}
	}
	for _, spec := range defaultHotkeySpecs {
		if strings.TrimSpace(s.Hotkeys[spec.ID]) == "" {
			s.Hotkeys[spec.ID] = spec.Default
		}
	}
}

func (s *Shell) notifySettingsChanged() {
	if s.OnSettingsChanged != nil {
		s.OnSettingsChanged()
	}
}

// HotkeyDisplay returns the active shortcut badge text for a command ID.
func (s *Shell) HotkeyDisplay(id string) string {
	s.ensureSettingsDefaults()
	if v := strings.TrimSpace(s.Hotkeys[id]); v != "" {
		return v
	}
	for _, spec := range defaultHotkeySpecs {
		if spec.ID == id {
			return spec.Default
		}
	}
	return ""
}

func (s *Shell) renderObsidianSettingsDialog(c *ui.Context) {
	if !s.VaultSettingsOpen {
		return
	}
	s.ensureSettingsDefaults()
	t := c.Theme()
	w, h := c.Size()
	if w <= 0 {
		w = 1080
	}
	if h <= 0 {
		h = 720
	}
	var inset float32 = 20
	if s.window != nil {
		inset = sidebar.HeaderHeight(c)
	}
	width := min(t.Space(205), w*0.9, max(0, w-2*inset))
	height := min(t.Space(175), h*0.86)
	narrow := width < t.Space(170) || s.IsMobileLayout(c)

	open := s.VaultSettingsOpen
	tok := theme.GetTokens(t)
	ui.Box(c.Key("obsidian-settings-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "AsterLab 设置", Inset: inset}, func(panel ui.Element) {
			panel.Width(width).Height(height).Padding(0).Gap(0).Clip()
			// Prevent high-contrast underlying document text from bleeding through translucent
			// glass modals in Dark mode by backing the settings container with solid Surface + crisp rim.
			if tok.Dark {
				panel.Background(tok.Surface).Border(1, tok.BorderStrong)
			}
			components.Settings(c, &s.SettingsView, s.obsidianSettingsCategories(c), components.SettingsOptions{
				Narrow:            narrow,
				Title:             "设置",
				SearchPlaceholder: "搜索外观、实验库、快捷键、默认板型…",
				CategoryLabel:     "设置分类",
				ResultsTitle:      "匹配设置项",
				EmptyText:         "未找到匹配的设置项",
				Close: func() {
					open = false
				},
			})
		})
	})
	s.VaultSettingsOpen = open
}

func (s *Shell) obsidianSettingsCategories(c *ui.Context) []components.SettingsCategory {
	tok := theme.GetTokens(c.Theme())

	applyThemeChoice := func(choice string) {
		s.ThemeSource = choice
		if s.window != nil {
			switch choice {
			case "light":
				mygo.Theme.SetSource(mygo.ThemeLight)
			case "dark":
				mygo.Theme.SetSource(mygo.ThemeDark)
			default:
				mygo.Theme.SetSource(mygo.ThemeSystem)
			}
		}
		s.notifySettingsChanged()
		c.Invalidate()
	}

	vaultCat := components.SettingsCategory{
		ID:          "vault",
		Title:       "实验库与文件",
		Section:     "工作区",
		Icon:        lucide.FolderOpen(),
		Description: "本地单库根目录、透明 .asterml 组织规则、新建默认位置与附件归档策略。",
		Groups: []components.SettingsGroup{
			{
				ID:    "vault-location",
				Title: "库位置与目录树",
				Items: []components.SettingsItem{
					{
						ID:          "vault-path",
						Title:       "当前实验库路径",
						Description: "所有 .asterml 实验、templates 模板与 assets 附件均直接存于该目录。",
						Keywords:    []string{"vault", "path", "目录", "路径", "切换"},
						Stacked:     true,
						Build: func(c *ui.Context) {
							ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Grow(1).MinWidth(0).Children(func() {
									components.TextBox(c, &s.VaultPathInput, components.TextBoxOptions{}).
										Placeholder("输入本地实验库文件夹路径")
								})
								if s.OnPickVaultPath != nil && components.Button(c, "浏览…", components.ButtonOutline).Clicked() {
									if picked := strings.TrimSpace(s.OnPickVaultPath()); picked != "" {
										s.VaultPathInput = picked
										if s.OnSwitchVaultPath != nil {
											s.OnSwitchVaultPath(picked)
										}
									}
								}
								if components.Button(c, "切换路径", components.ButtonSecondary).Clicked() && s.OnSwitchVaultPath != nil {
									s.OnSwitchVaultPath(strings.TrimSpace(s.VaultPathInput))
									s.VaultSettingsOpen = false
								}
								if s.OnRevealInExplorer != nil && components.Button(c, "打开目录", components.ButtonOutline).Clicked() {
									s.OnRevealInExplorer("")
								}
							})
						},
					},
					{
						ID:          "hide-assets",
						Title:       "隐藏 assets 附件目录",
						Description: "在左侧实验库树中隐藏图片与原始附件目录（不影响文内引用）。",
						Keywords:    []string{"assets", "隐藏", "附件", "树"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.HideAssetsInTree, "隐藏 assets", components.SwitchOptions{HideLabel: true}).Changed() {
								if s.OnToggleHideAssets != nil {
									s.OnToggleHideAssets(s.HideAssetsInTree)
								}
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "auto-archive-assets",
						Title:       "粘贴 / 拖入图片自动落库 ./assets/",
						Description: "Ctrl+V 截图或拖入实验实拍图时自动复制至当前库 assets 目录并写相对路径。",
						Keywords:    []string{"paste", "drag", "assets", "图片", "相对路径"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.AutoArchiveAssets, "自动落库附件", components.SwitchOptions{HideLabel: true}).Changed() {
								s.notifySettingsChanged()
							}
						},
					},
				},
			},
			{
				ID:    "default-folder",
				Title: "新建归属策略",
				Items: []components.SettingsItem{
					{
						ID:          "default-proj",
						Title:       "新建实验默认项目文件夹",
						Description: "日历点击新建或 Ctrl+N 创建时预选的归属目录。",
						Keywords:    []string{"project", "folder", "默认", "新建", "项目"},
						Build: func(c *ui.Context) {
							choices := append([]string{"", "/"}, s.VaultFolders...)
							sel := components.Select(c, &s.DefaultProjectDir)
							labelFor := func(v string) string {
								switch v {
								case "":
									return "记住上次所在项目"
								case "/":
									return "始终放在根目录"
								default:
									return v
								}
							}
							sel.Trigger.MinWidth(c.Theme().Space(40)).Children(func() {
								components.Text(c, labelFor(s.DefaultProjectDir)).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, ch := range choices {
									ch := ch
									sel.Item(ch).Children(func() {
										components.Text(c, labelFor(ch)).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								s.notifySettingsChanged()
							}
						},
					},
				},
			},
			{
				ID:    "vault-trash",
				Title: "回收站与文件恢复",
				Items: []components.SettingsItem{
					{
						ID:          "trash-restore-list",
						Title:       "最近删除的实验与目录",
						Description: "删除的文件保存在 .asterlab/trash/，可随时一键还原至原路径。",
						Keywords:    []string{"trash", "restore", "回收站", "恢复", "删除"},
						Stacked:     true,
						Build: func(c *ui.Context) {
							if len(s.TrashItems) == 0 {
								components.Text(c, "回收站为空").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								return
							}
							ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
								limit := len(s.TrashItems)
								if limit > 8 {
									limit = 8
								}
								for i := 0; i < limit; i++ {
									it := s.TrashItems[i]
									ui.Row(c.Key("trash-it-"+it.TrashName)).FillWidth().AlignItems(ui.Center).
										Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
										Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Gap(float32(tok.Spacing8)).Children(func() {
										ui.Column(c).Grow(1).MinWidth(0).Children(func() {
											components.Text(c, it.OrigRel).FontSize(float32(tok.TextBody)).TextColor(tok.Text).SingleLine().Ellipsis("…")
											components.Text(c, it.DeletedAt.Format("2006-01-02 15:04")).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
										})
										if components.Button(c, "还原", components.ButtonSecondary).Clicked() && s.OnVaultRestoreTrash != nil {
											s.OnVaultRestoreTrash(it.TrashName)
										}
									})
								}
							})
						},
					},
					{
						ID:          "snapshot-recovery-list",
						Title:       "当前实验历史版本快照 (File Recovery)",
						Description: "自动防抖留存的离线版本快照（.asterlab/snapshots/），可一键恢复至历史时间点。",
						Keywords:    []string{"snapshot", "recovery", "history", "快照", "历史版本", "恢复"},
						Stacked:     true,
						Build: func(c *ui.Context) {
							if s.CurrentDocRelPath == "" {
								components.Text(c, "先打开一份 .asterml 实验文档，即可在此浏览并恢复其历史快照。").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								return
							}
							if len(s.CurrentSnapshots) == 0 {
								components.Text(c, "当前文档尚无历史快照（每次编辑稳定保存后自动留存，最多保留 20 个）。").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								return
							}
							ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
								limit := len(s.CurrentSnapshots)
								if limit > 8 {
									limit = 8
								}
								for i := 0; i < limit; i++ {
									sn := s.CurrentSnapshots[i]
									ui.Row(c.Key("snap-"+sn.ID)).FillWidth().AlignItems(ui.Center).
										Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
										Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Gap(float32(tok.Spacing8)).Children(func() {
										ui.Column(c).Grow(1).MinWidth(0).Children(func() {
											components.Text(c, sn.CreatedAt.Format("2006-01-02 15:04:05")).FontSize(float32(tok.TextBody)).TextColor(tok.Text).FontFeatures("tnum")
											components.Text(c, fmt.Sprintf("%d 字节 · %s", sn.Size, sn.RelPath)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine().Ellipsis("…").FontFeatures("tnum")
										})
										if components.Button(c, "恢复此版本", components.ButtonSecondary).Clicked() && s.OnRestoreSnapshot != nil {
											s.OnRestoreSnapshot(s.CurrentDocRelPath, sn.ID)
										}
									})
								}
							})
						},
					},
				},
			},
		},
	}

	appearanceCat := components.SettingsCategory{
		ID:          "appearance",
		Title:       "外观与主题",
		Section:     "工作区",
		Icon:        lucide.Palette(),
		Description: "Obsidian 风格纸面明暗主题、字号缩放、间距紧密度与系统级物理动效偏好。",
		Groups: []components.SettingsGroup{
			{
				ID:    "theme-group",
				Title: "色彩与材质",
				Items: []components.SettingsItem{
					{
						ID:          "theme-mode",
						Title:       "基础明暗模式",
						Description: "切换浅色画布、深空夜视或跟随操作系统明暗外观。",
						Keywords:    []string{"theme", "dark", "light", "system", "主题", "深色", "浅色"},
						Build: func(c *ui.Context) {
							options := []string{"system", "light", "dark"}
							names := map[string]string{"system": "跟随系统", "light": "浅色模式", "dark": "深色模式"}
							sel := components.Select(c, &s.ThemeSource)
							sel.Trigger.MinWidth(c.Theme().Space(32)).Children(func() {
								components.Text(c, names[s.ThemeSource]).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, opt := range options {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, names[opt]).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								applyThemeChoice(s.ThemeSource)
							}
						},
					},
					{
						ID:          "theme-preset",
						Title:       "界面色盘预设",
						Description: "切换纯净白纸（Obsidian 极简，无边框冷灰卡片）、冷银灰底（Cool Zinc）或暖润实验纸。",
						Keywords:    []string{"preset", "obsidian", "zinc", "warm", "配色", "画布", "冷灰", "纯白"},
						Build: func(c *ui.Context) {
							options := []string{theme.PresetObsidian, theme.PresetZinc, theme.PresetWarm}
							names := map[string]string{
								theme.PresetObsidian: "纯净白纸 · Obsidian（推荐）",
								theme.PresetZinc:     "冷银极简 · Cool Zinc",
								theme.PresetWarm:     "经典暖润实验纸",
							}
							if s.ThemePreset == "" || s.ThemePreset == "aurora" {
								s.ThemePreset = theme.PresetObsidian
							}
							sel := components.Select(c, &s.ThemePreset)
							sel.Trigger.MinWidth(c.Theme().Space(36)).Children(func() {
								components.Text(c, firstNonEmptyStr(names[s.ThemePreset], names[theme.PresetObsidian])).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, opt := range options {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, names[opt]).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								s.SyncThemeLook(c)
								s.notifySettingsChanged()
								c.Invalidate()
							}
						},
					},
					{
						ID:          "liquid-glass",
						Title:       "Liquid Glass 液态玻璃材质",
						Description: "开启 MyGo 原生折射透镜、微光边缘高光与 glass.Group 胶囊融合；关闭可切回经典平铺磨砂。",
						Keywords:    []string{"glass", "liquid", "blur", "refraction", "液态玻璃", "毛玻璃", "质感"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.LiquidGlass, "Liquid Glass 材质", components.SwitchOptions{HideLabel: true}).Changed() {
								s.SyncThemeLook(c)
								s.notifySettingsChanged()
								c.Invalidate()
							}
						},
					},
					{
						ID:          "text-scale",
						Title:       "界面缩放比例（字体与图标联动）",
						Description: "全局同步缩放字体、图标、行高与按钮尺寸；支持 Ctrl+= 放大、Ctrl+- 缩小、Ctrl+0 恢复跟随系统。",
						Keywords:    []string{"font", "scale", "zoom", "字号", "图标", "缩放"},
						Build: func(c *ui.Context) {
							scaleStr := fmt.Sprintf("%.2f", s.TextScale)
							if s.TextScale <= 0 {
								scaleStr = "0.00"
							}
							options := []string{"0.00", "0.90", "1.00", "1.10", "1.15", "1.25", "1.35", "1.50"}
							names := map[string]string{
								"0.00": "跟随系统",
								"0.90": "90% 紧凑",
								"1.00": "100% 标准",
								"1.10": "110% 舒适",
								"1.15": "115% 清晰",
								"1.25": "125% 大字",
								"1.35": "135% 超清",
								"1.50": "150% 特大",
							}
							curLabel := names[scaleStr]
							if curLabel == "" {
								curLabel = fmt.Sprintf("%.0f%% 自定义", s.TextScale*100)
							}
							sel := components.Select(c, &scaleStr)
							sel.Trigger.MinWidth(c.Theme().Space(36)).Children(func() {
								components.Text(c, curLabel).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, opt := range options {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, names[opt]).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								switch scaleStr {
								case "0.90":
									s.TextScale = 0.90
								case "1.00":
									s.TextScale = 1.00
								case "1.10":
									s.TextScale = 1.10
								case "1.15":
									s.TextScale = 1.15
								case "1.25":
									s.TextScale = 1.25
								case "1.35":
									s.TextScale = 1.35
								case "1.50":
									s.TextScale = 1.50
								default:
									s.TextScale = 0
								}
								s.SyncThemeLook(c)
								s.notifySettingsChanged()
								c.Invalidate()
							}
						},
					},
					{
						ID:          "reduce-motion",
						Title:       "减弱物理动效（无障碍）",
						Description: "跳过四档物理弹簧与数字滚轮位移动画，仅保留瞬时切换与色彩渐变。",
						Keywords:    []string{"motion", "animation", "reduce", "动效", "无障碍"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.ReduceMotion, "减弱动效", components.SwitchOptions{HideLabel: true}).Changed() {
								s.SyncThemeLook(c)
								s.notifySettingsChanged()
								c.Invalidate()
							}
						},
					},
				},
			},
		},
	}

	editorCat := components.SettingsCategory{
		ID:          "editor",
		Title:       "文档与实验默认",
		Section:     "实验台",
		Icon:        lucide.SlidersHorizontal(),
		Description: "设置 .asterml 默认打开模式、孔板快速创建默认规格与配液默认余量系数。",
		Groups: []components.SettingsGroup{
			{
				ID:    "doc-defaults",
				Title: "实验文档行为",
				Items: []components.SettingsItem{
					{
						ID:          "default-open-mode",
						Title:       "打开实验默认视图",
						Description: "从文件树或日历打开 .asterml 时默认进入编辑态或纯净阅读态（仍可原位勾选与计时）。",
						Keywords:    []string{"mode", "edit", "read", "编辑", "阅读", "默认"},
						Build: func(c *ui.Context) {
							idx := 0
							if s.DefaultOpenMode == "read" {
								idx = 1
							}
							components.Tabs(c, &idx, components.TabsSegmented, "编辑态", "阅读态")
							next := "edit"
							if idx == 1 {
								next = "read"
							}
							if next != s.DefaultOpenMode {
								s.DefaultOpenMode = next
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "default-plate-format",
						Title:       "新插入孔板默认规格",
						Description: "在步骤中通过斜杠 / 或 + 插入孔板时的默认孔数规格。",
						Keywords:    []string{"plate", "96", "384", "24", "孔板", "默认"},
						Build: func(c *ui.Context) {
							opts := []int{6, 12, 24, 48, 96, 384}
							sel := components.Select(c, &s.DefaultPlateFormat)
							sel.Trigger.MinWidth(c.Theme().Space(28)).Children(func() {
								components.Text(c, fmt.Sprintf("%d 孔板", s.DefaultPlateFormat)).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, opt := range opts {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, fmt.Sprintf("%d 孔板", opt)).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "default-excess",
						Title:       "配液计算器默认余量",
						Description: "插入公式计算器或估算预混液时默认附带的移液损耗余量百分比。",
						Keywords:    []string{"excess", "calculator", "余量", "配液"},
						Build: func(c *ui.Context) {
							opts := []int{10, 15, 20}
							sel := components.Select(c, &s.DefaultExcessPct)
							sel.Trigger.MinWidth(c.Theme().Space(28)).Children(func() {
								components.Text(c, fmt.Sprintf("+%d%% 余量", s.DefaultExcessPct)).SingleLine()
							})
							sel.Popup(func(panel ui.Element) {
								for _, opt := range opts {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, fmt.Sprintf("+%d%% 余量", opt)).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								s.notifySettingsChanged()
							}
						},
					},
				},
			},
		},
	}

	hotkeyGroups := []components.SettingsGroup{
		{
			ID:    "hotkeys-search",
			Title: "快捷键总览与自定义",
			Actions: func(c *ui.Context) {
				if components.Button(c, "恢复默认快捷键", components.ButtonGhost).Clicked() {
					for _, spec := range defaultHotkeySpecs {
						s.Hotkeys[spec.ID] = spec.Default
					}
					s.notifySettingsChanged()
				}
			},
			Items: func() []components.SettingsItem {
				var items []components.SettingsItem
				for _, spec := range defaultHotkeySpecs {
					spec := spec
					items = append(items, components.SettingsItem{
						ID:          "hk-" + spec.ID,
						Title:       spec.Action,
						Description: spec.Group,
						Keywords:    []string{"hotkey", "shortcut", spec.Action, spec.Group, spec.Default},
						Build: func(c *ui.Context) {
							cur := s.HotkeyDisplay(spec.ID)
							ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
								ui.Box(c).Width(c.Theme().Space(32)).Children(func() {
									components.TextBox(c, &cur, components.TextBoxOptions{}).Placeholder(spec.Default)
								})
								if strings.TrimSpace(cur) != s.Hotkeys[spec.ID] && strings.TrimSpace(cur) != "" {
									s.Hotkeys[spec.ID] = strings.TrimSpace(cur)
									s.notifySettingsChanged()
								}
							})
						},
					})
				}
				return items
			}(),
		},
	}
	hotkeysCat := components.SettingsCategory{
		ID:          "hotkeys",
		Title:       "快捷键",
		Section:     "交互与设备",
		Icon:        lucide.Keyboard(),
		Description: "参照 Obsidian 的全键盘流操作映射：支持快捷浏览、搜索过滤、自定义键位提示或一键还原。",
		Groups:      hotkeyGroups,
	}

	mobileCat := components.SettingsCategory{
		ID:          "mobile",
		Title:       "移动端与触控适配",
		Section:     "交互与设备",
		Icon:        lucide.Smartphone(),
		Description: fmt.Sprintf("Obsidian 风格单窗格触控体验：悬浮液态玻璃底栏、左右边缘跟手抽屉、下拉快捷动作与编辑工具条（当前平台 %s，窗口宽度 < 680dp 自动切换）。", runtime.GOOS),
		Groups: []components.SettingsGroup{
			{
				ID:    "mobile-layout",
				Title: "响应式布局与预览",
				Items: []components.SettingsItem{
					{
						ID:          "mobile-preview",
						Title:       "强制启用移动端单栏与悬浮底栏",
						Description: "即使在桌面宽屏窗口下也可即时切换预览移动端手势抽屉、悬浮液态玻璃底栏与标签页卡片切换器。",
						Keywords:    []string{"mobile", "android", "ios", "touch", "移动端", "自适应"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.ForceMobilePreview, "移动预览模式", components.SwitchOptions{HideLabel: true}).Changed() {
								s.MobileDrawer = ""
								s.MobileTabSwitcherOpen = false
							}
						},
					},
				},
			},
			{
				ID:    "mobile-gestures",
				Title: "手势与工具条（Obsidian Mobile）",
				Items: []components.SettingsItem{
					{
						ID:          "mobile-pull-action",
						Title:       "顶部下拉快捷动作",
						Description: "在页面顶部继续向下拉动并松手时自动触发的快捷操作。",
						Keywords:    []string{"pull", "quick action", "下拉", "快捷手势", "命令面板"},
						Build: func(c *ui.Context) {
							labels := map[string]string{
								"palette": "命令面板 / 快速切换",
								"new_exp": "新建今日实验",
								"calc":    "打开速算与计时器",
								"tree":    "展开实验库抽屉",
							}
							sel := components.Select(c, &s.MobilePullAction)
							sel.Trigger.Children(func() {
								components.Text(c, firstNonEmptyStr(labels[s.MobilePullAction], "命令面板 / 快速切换")).SingleLine()
							})
							sel.Popup(func(ui.Element) {
								for _, opt := range []string{"palette", "new_exp", "calc", "tree"} {
									opt := opt
									sel.Item(opt).Children(func() {
										components.Text(c, labels[opt]).SingleLine()
									})
								}
							})
							if sel.Trigger.Changed() {
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "mobile-autohide-dock",
						Title:       "向下滚动正文自动收起悬浮底栏",
						Description: "向下阅读长实验文档或日程时平滑隐退底栏释放视野，轻滑回拉立即浮现。",
						Keywords:    []string{"autohide", "dock", "scroll", "悬浮", "底栏", "隐藏"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.MobileAutoHideDock, "自动隐藏底栏", components.SwitchOptions{HideLabel: true}).Changed() {
								if !s.MobileAutoHideDock {
									s.MobileDockHidden = false
								}
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "mobile-edit-toolbar",
						Title:       "编辑态显示底部横滑实验工具条",
						Description: "打开实验并处于编辑态时，将底部导航切换为可横滑的操作条（撤销、+步骤、核对项、参数、孔板、计时）。",
						Keywords:    []string{"toolbar", "edit", "mobile", "工具栏", "编辑"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.MobileEditToolbar, "移动编辑工具条", components.SwitchOptions{HideLabel: true}).Changed() {
								s.MobileDockForceNav = false
								s.notifySettingsChanged()
							}
						},
					},
				},
			},
		},
	}

	aboutCat := components.SettingsCategory{
		ID:          "about",
		Title:       "关于 AsterLab",
		Section:     "系统",
		Icon:        theme.LogoParsedSmall(),
		Description: "本地单真源 .asterml 实验工作台 × Pure-Go astercore 科学核心（零 SQLite / Rust 依赖）。",
		Groups: []components.SettingsGroup{
			{
				ID:    "app-update",
				Title: "软件版本与自动更新",
				Items: []components.SettingsItem{
					{
						ID:          "update-status",
						Title:       "当前版本与发布通道",
						Description: "检查 GitHub Releases 中的最新 Pure-Go Windows 版本并自动校验 SHA256 后替换重启。",
						Keywords:    []string{"update", "version", "release", "升级", "更新", "版本"},
						Stacked:     true,
						Build: func(c *ui.Context) {
							curVer := firstNonEmptyStr(s.AppVersion, "0.1.0-dev")
							commitStr := strings.TrimSpace(s.BuildCommit)
							ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
								ui.Row(c).Wrap().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
									verLabel := "v" + strings.TrimPrefix(curVer, "v")
									if commitStr != "" && commitStr != "dev" {
										verLabel += " (" + commitStr + ")"
									}
									components.Badge(c, verLabel)
									switch s.UpdateStatus {
									case "checking":
										components.Text(c, "正在检查更新…").FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
									case "up_to_date":
										components.Text(c, "已是最新版本").FontSize(float32(tok.TextData)).TextColor(tok.Success)
									case "available":
										components.Badge(c, "发现新版本 "+s.UpdateLatestVersion)
									case "downloading":
										components.Text(c, "正在下载并校验更新包…").FontSize(float32(tok.TextData)).TextColor(tok.Accent)
									case "restarting":
										components.Text(c, "更新已就绪，正在重启应用…").FontSize(float32(tok.TextData)).TextColor(tok.Accent)
									case "error":
										if s.UpdateErr != "" {
											components.Text(c, s.UpdateErr).FontSize(float32(tok.TextData)).TextColor(tok.Danger)
										}
									}
								})
								ui.Row(c).Wrap().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
									chkBtn := components.Button(c, "检查更新", components.ButtonSecondary)
									if s.UpdateStatus == "checking" || s.UpdateStatus == "downloading" || s.UpdateStatus == "restarting" {
										chkBtn.Disabled(true)
									}
									if chkBtn.Clicked() && s.OnCheckUpdate != nil {
										s.OnCheckUpdate()
									}
									if s.UpdateStatus == "available" && s.OnInstallUpdate != nil {
										if components.Button(c, "立即更新并重启", components.ButtonPrimary).Clicked() {
											s.OnInstallUpdate()
										}
									}
									if s.UpdateDownloadURL != "" && s.OnOpenUpdatePage != nil {
										if components.Button(c, "查看发布说明", components.ButtonGhost).Clicked() {
											s.OnOpenUpdatePage()
										}
									}
								})
								if strings.TrimSpace(s.UpdateNotes) != "" && s.UpdateStatus == "available" {
									ui.Box(c).FillWidth().Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
										Background(tok.Elevated).Children(func() {
										ui.Text(c, s.UpdateNotes).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									})
								}
							})
						},
					},
					{
						ID:          "update-auto-check",
						Title:       "启动时自动检查更新",
						Description: "桌面启动后在后台静默检查是否有可用新版本，仅在状态栏轻提示。",
						Keywords:    []string{"auto update", "自动检查"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.UpdateAutoCheck, "自动检查", components.SwitchOptions{HideLabel: true}).Changed() {
								s.notifySettingsChanged()
							}
						},
					},
					{
						ID:          "update-prerelease",
						Title:       "接收预发布版本 (Pre-release)",
						Description: "包含功能预览版与候选测试版；关闭则仅检查正式 Release。",
						Keywords:    []string{"prerelease", "beta", "rc", "预发布"},
						Build: func(c *ui.Context) {
							if components.Switch(c, &s.UpdateIncludePre, "预发布", components.SwitchOptions{HideLabel: true}).Changed() {
								s.notifySettingsChanged()
							}
						},
					},
				},
			},
			{
				ID:    "diag-feedback",
				Title: "诊断、日志与问题反馈",
				Items: []components.SettingsItem{
					{
						ID:          "diag-actions",
						Title:       "反馈异常或导出脱敏诊断包",
						Description: "导出只含脱敏运行日志与异常堆栈的诊断 ZIP（绝不包含 .asterml 实验内容），或打开反馈提交弹窗。",
						Keywords:    []string{"feedback", "bug", "log", "crash", "diagnostic", "反馈", "日志", "诊断"},
						Build: func(c *ui.Context) {
							ui.Row(c).Wrap().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
								if components.Button(c, "导出诊断包 (.zip)", components.ButtonSecondary).Clicked() && s.OnExportDiagBundle != nil {
									s.OnExportDiagBundle()
								}
								if components.Button(c, "反馈问题 / 建议", components.ButtonPrimary).Clicked() {
									s.FeedbackOpen = true
								}
							})
						},
					},
				},
			},
			{
				ID:    "brand-identity",
				Title: "品牌标识与架构",
				Items: []components.SettingsItem{
					{
						ID:          "brand-hero",
						Title:       "AsterLab 三流几何徽标",
						Description: "三条并发轨道映照科研数据流：实验设计（Violet）· 步骤执行（Sky）· 分析汇总（Emerald）；支持大/小双尺寸光学适配。",
						Keywords:    []string{"logo", "brand", "asterlab", "svg", "tri-stream", "图标"},
						Stacked:     true,
						Build: func(c *ui.Context) {
							t := c.Theme()
							mark := theme.LogoParsedTriStream(tok.Dark)
							if s.AboutMonochromeLogo {
								mark = theme.LogoParsedStandard()
							}
							ui.Row(c).Gap(float32(tok.Spacing12)).AlignItems(ui.Center).Children(func() {
								ui.Box(c).
									Width(t.Space(16)).Height(t.Space(14)).
									Radius(float32(tok.ControlRadius)).
									Background(tok.Elevated).
									Border(1, tok.Border).
									Clip().Children(func() {
									ui.Box(c).Absolute().Fill().Padding(t.Space(1)).Center().
										TextColor(tok.TextMuted.Alpha(0.18)).Children(func() {
										ui.Icon(c, theme.LogoParsedOutline()).Width(t.Space(14)).Height(t.Space(12))
									})
									ui.Box(c).Fill().Center().
										TextColor(tok.Text).Children(func() {
										if s.AboutMonochromeLogo {
											ui.Icon(c, mark).Width(t.Space(9)).Height(t.Space(8))
										} else {
											ui.Image(c, mark).Width(t.Space(9)).Height(t.Space(8))
										}
									})
								})
								components.Badge(c, "Pure-Go astercore")
							})
						},
					},
					{
						ID:          "brand-monochrome-toggle",
						Title:       "单色极简徽标模式",
						Description: "切换预览标准单色矢量徽标与设计/执行/分析三流语义着色。",
						Keywords:    []string{"monochrome", "logo", "tri", "单色", "三流"},
						Build: func(c *ui.Context) {
							components.Switch(c, &s.AboutMonochromeLogo, "单色极简", components.SwitchOptions{HideLabel: true})
						},
					},
				},
			},
			{
				ID:    "vault-stats",
				Title: "当前实验库统计",
				Items: []components.SettingsItem{
					{
						ID:       "stats-summary",
						Title:    "文件规模",
						Keywords: []string{"stats", "count", "统计"},
						Build: func(c *ui.Context) {
							expCnt, tplCnt, assetCnt := 0, 0, 0
							for _, f := range s.VaultFiles {
								if f.IsTemplate {
									tplCnt++
								} else if f.IsAsset {
									assetCnt++
								} else {
									expCnt++
								}
							}
							components.Badge(c, fmt.Sprintf("%d 实验 · %d 模板 · %d 附件", expCnt, tplCnt, assetCnt))
						},
					},
				},
			},
		},
	}

	return []components.SettingsCategory{
		vaultCat,
		appearanceCat,
		editorCat,
		hotkeysCat,
		mobileCat,
		aboutCat,
	}
}
