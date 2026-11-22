package shell

import (
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"github.com/oneo-me/onevra/source/shells/sidebar"

	"asterlab-mygo-prototype/internal/theme"
)

const (
	mobileBreakpointWidth    = float32(680)
	mobilePullActionThresh   = float32(56)
	mobileDrawerWidthMax     = float32(320)
	mobileEdgeSwipeZoneWidth = float32(18)
)

// SetInitialMobilePrefs loads persisted mobile shell preferences without letting
// EnsureMobileDefaults overwrite explicitly saved false booleans back to true.
func (s *Shell) SetInitialMobilePrefs(pullAction string, autoHideDock, editToolbar bool) {
	if pullAction == "" {
		pullAction = "palette"
	}
	s.MobilePullAction = pullAction
	s.MobileAutoHideDock = autoHideDock
	s.MobileEditToolbar = editToolbar
	s.mobilePrefsInited = true
}

// EnsureMobileDefaults initializes Obsidian Mobile v2 shell defaults once without
// overriding explicit user preferences loaded from disk.
func (s *Shell) EnsureMobileDefaults() {
	if s.MobilePullAction == "" {
		s.MobilePullAction = "palette"
	}
	if !s.mobilePrefsInited {
		s.mobilePrefsInited = true
		if !s.MobileAutoHideDock {
			s.MobileAutoHideDock = true
		}
		if !s.MobileEditToolbar {
			s.MobileEditToolbar = true
		}
	}
}

// IsMobileLayout reports whether AsterLab should adapt its workspace chrome to
// single-pane touch-friendly mobile ergonomics (narrow viewport < 680dp, mobile OS,
// or user-toggled mobile preview in Settings).
func (s *Shell) IsMobileLayout(c *ui.Context) bool {
	if s.ForceMobilePreview || runtime.GOOS == "android" || runtime.GOOS == "ios" {
		return true
	}
	w, _ := c.Size()
	return w > 0 && w < mobileBreakpointWidth
}

// MobileContentScrollState returns the tracked ui.ScrollState for the currently
// active mobile tab so upward/downward scrolling can drive floating dock auto-hide
// and top pull-down Quick Action cleanly.
func (s *Shell) MobileContentScrollState() *ui.ScrollState {
	if s.mobileScrollStates == nil {
		s.mobileScrollStates = map[string]*ui.ScrollState{}
	}
	key := "default"
	if t := s.ActiveTab(); t != nil && t.ID != "" {
		key = t.ID
	}
	st, ok := s.mobileScrollStates[key]
	if !ok {
		st = &ui.ScrollState{}
		s.mobileScrollStates[key] = st
	}
	return st
}

// MobileTopReserve returns top clearance below the floating iOS Liquid Glass controls
// (left drawer orb + right mode/more pill) when Liquid Glass is active.
func (s *Shell) MobileTopReserve(c *ui.Context) float32 {
	tok := theme.GetTokens(c.Theme())
	if !tok.LiquidGlass {
		return 0
	}
	return 56
}

// MobileBottomReserve returns the bottom clearance needed so scrolled document/calendar
// items clear the centered floating bottom glass capsule cleanly.
func (s *Shell) MobileBottomReserve(c *ui.Context) float32 {
	return 76
}

func (s *Shell) updateMobileScrollBehavior() {
	st := s.MobileContentScrollState()
	if s.mobileLastScrollY == nil {
		s.mobileLastScrollY = map[string]float32{}
	}
	key := "default"
	if t := s.ActiveTab(); t != nil && t.ID != "" {
		key = t.ID
	}
	prevY := s.mobileLastScrollY[key]
	curY := st.Y
	dy := curY - prevY
	s.mobileLastScrollY[key] = curY

	if !s.MobileAutoHideDock || s.MobileDrawer != "" || s.MobileTabSwitcherOpen {
		if !s.MobileAutoHideDock {
			s.MobileDockHidden = false
		}
		return
	}
	if curY <= 12 || dy < -4 {
		s.MobileDockHidden = false
	} else if dy > 6 && curY > 36 {
		s.MobileDockHidden = true
	}
}

func (s *Shell) mobilePullActionLabel() string {
	switch s.MobilePullAction {
	case "new_exp":
		return "新建今日实验"
	case "calc":
		return "速算与计时器"
	case "tree":
		return "展开实验库"
	default:
		return "命令面板 / 快速切换"
	}
}

func (s *Shell) executeMobilePullAction() {
	switch s.MobilePullAction {
	case "new_exp":
		s.MobileDrawer = ""
		s.MobileTabSwitcherOpen = false
		s.triggerVaultNewFile("")
	case "calc":
		s.RightTab = 1
		s.MobileDrawer = "outline"
	case "tree":
		s.MobileDrawer = "tree"
	default:
		s.MobileDrawer = ""
		s.MobileTabSwitcherOpen = false
		s.PaletteOpen = true
		s.PaletteCursor = 0
	}
}

func (s *Shell) renderMobileWorkbench(c *ui.Context, tok theme.Tokens) {
	s.EnsureTabs()
	s.EnsureMobileDefaults()
	tok = s.SyncThemeLook(c)
	s.updateMobileScrollBehavior()

	var topInset float32
	if s.window != nil {
		topInset = sidebar.HeaderHeight(c)
	}
	activeTab := s.ActiveTab()
	activePane := s.findPane(s.ActivePaneID)
	if activePane == nil {
		activePane = s.firstLeaf(s.RootPane)
	}
	vw, _ := c.Size()
	sheetMaxW := mobileDrawerWidthMax
	if vw > 0 && vw*0.84 < sheetMaxW {
		sheetMaxW = vw * 0.84
	}
	if sheetMaxW < 240 {
		sheetMaxW = 240
	}

	rootBox := ui.Box(c.Key("mobile-workbench-root")).Fill().Padding(topInset, 0, 0, 0)
	theme.PaintCanvasBackground(rootBox, tok)
	rootBox.Children(func() {
		ui.Column(c).Fill().Children(func() {
			if !tok.LiquidGlass {
				s.renderMobileDockedTopChrome(c, tok, activeTab)
			}

			bodyRoot := ui.Box(c.Key("mobile-body-stage")).FillWidth().Grow(1).MinHeight(0).Clip()
			bodyRoot.Children(func() {
				// 1A. Primary Tab Content
				if activeTab != nil {
					switch activeTab.Kind {
					case TabCalendar:
						s.calendarHomeView(c)
					case TabPantry:
						if s.RenderPantryContent != nil {
							s.RenderPantryContent(c)
						} else if s.Content != nil {
							s.Content(c)
						}
					default:
						if s.RenderTabContent != nil && activePane != nil {
							s.RenderTabContent(c, activePane, *activeTab)
						} else if s.Content != nil {
							s.Content(c)
						}
					}
				}

				// 1B. Obsidian Pull-Down Quick Action Zone
				s.renderMobilePullDownQuickAction(c, tok)

				// 1C. Left & Right Screen-Edge Swipe Physics Sensors
				if s.MobileDrawer == "" && !s.MobileTabSwitcherOpen {
					s.renderMobileEdgeSwipeSensors(c, sheetMaxW)
				}

				// 1D. Floating Obsidian iOS 26 Top Bar (Open center, left circular orb + right twin-icon pill)
				if tok.LiquidGlass {
					s.renderMobileGlassTopBar(c, tok, activeTab)
				}

				// 1E. Centered Compact Icon-Only Liquid Glass Bottom Capsule
				s.renderMobileFloatingBottomBar(c, tok, bodyRoot, activePane, activeTab)
			})
		})

		// 2. Interactive Edge-Swiped Obsidian Drawer ("tree" or "outline")
		s.renderMobileSwipeDrawers(c, tok, sheetMaxW)

		// 3. Obsidian-style Tab Switcher Card Overview
		if s.MobileTabSwitcherOpen {
			s.renderMobileTabSwitcherOverlay(c, tok, activePane, activeTab)
		}
	})

	s.renderCommandPalette(c)
	s.renderGlobalSearchModal(c)
	s.renderVaultDialogs(c)
	if s.RenderOverlays != nil {
		s.RenderOverlays(c)
	}
}

func (s *Shell) renderMobileGlassTopBar(c *ui.Context, tok theme.Tokens, activeTab *WorkspaceTab) {
	// Soft atmospheric top scroll feather so scrolled headings melt gently at top edge
	components.ScrollEdge(c, components.ScrollEdgeOptions{
		Style:      components.ScrollEdgeSoft,
		Bottom:     false,
		Extent:     54,
		Background: tok.Canvas,
	})

	// Left: Standalone 40x40 circular Liquid Glass sidebar button (matches Obsidian iOS)
	leftOrb := ui.ButtonBase(c.Key("mb-top-sidebar")).Absolute().Top(10).Left(14).
		Size(40, 40).Radius(20).Center().Label("打开实验库抽屉")
	theme.ApplyCapsuleGlass(leftOrb, tok, true)
	theme.ApplyTactilePress(leftOrb)
	leftOrb.Children(func() {
		ui.Icon(c, lucide.PanelLeft()).Size(18, 18).TextColor(tok.Text)
	})
	if leftOrb.Clicked() {
		if s.MobileDrawer == "tree" {
			s.MobileDrawer = ""
		} else {
			s.MobileDrawer = "tree"
		}
	}

	// Right: Clean twin-icon merged Liquid Glass pill (Zero nested boxes!)
	rightPill := ui.Row(c.Key("mb-top-right-pill")).Absolute().Top(10).Right(14).
		Height(40).Padding(0, 6).Radius(20).AlignItems(ui.Center).Gap(2)
	theme.ApplyCapsuleGlass(rightPill, tok, false)
	rightPill.Children(func() {
		pillIcon := func(key string, icon *ui.SVG, label string, active bool, onClick func()) ui.Element {
			btn := ui.ButtonBase(c.Key("mb-tr-"+key)).Size(36, 36).Radius(18).Center().Label(label)
			if btn.Hovered() {
				btn.Background(tok.Text.Alpha(0.05))
			}
			theme.ApplyTactilePress(btn)
			fg := tok.Text
			if active {
				fg = tok.Accent
			}
			btn.Children(func() {
				ui.Icon(c, icon).Size(18, 18).TextColor(fg)
			})
			if btn.Clicked() {
				onClick()
			}
			return btn
		}

		if activeTab != nil && activeTab.Kind == TabFile {
			if activeTab.Mode == ModeEdit {
				pillIcon("mode-lab", lucide.FlaskConical(), "切换到实验模式（防误触）", false, func() {
					s.MobileDockForceNav = false
					s.SetActiveTabMode(ModeLab)
				})
			} else {
				pillIcon("mode-edit", lucide.PenLine(), "切换到编辑态", true, func() {
					s.MobileDockForceNav = false
					s.SetActiveTabMode(ModeEdit)
				})
			}
			moreB := ui.ButtonBase(c.Key("mb-tr-more")).Size(36, 36).Radius(18).Center().Label("文档与更多操作")
			if moreB.Hovered() {
				moreB.Background(tok.Text.Alpha(0.05))
			}
			theme.ApplyTactilePress(moreB)
			moreB.Children(func() {
				ui.Icon(c, lucide.Ellipsis()).Size(18, 18).TextColor(tok.Text)
			})
			s.attachMobileDocMenu(c, moreB, activeTab)
		} else {
			pillIcon("calc", lucide.Calculator(), "速算与计时器", s.MobileDrawer == "outline", func() {
				s.RightTab = 1
				if s.MobileDrawer == "outline" {
					s.MobileDrawer = ""
				} else {
					s.MobileDrawer = "outline"
				}
			})
			pillIcon("search", lucide.Search(), "快速打开 (Ctrl+K)", false, func() {
				s.PaletteOpen = true
				s.PaletteCursor = 0
			})
		}
	})
}

func (s *Shell) attachMobileDocMenu(c *ui.Context, trigger ui.Element, activeTab *WorkspaceTab) {
	components.MenuButton(c, trigger, func(m *components.Menu) {
		if m.Item("步骤大纲与速算").Chosen() {
			s.MobileDrawer = "outline"
		}
		if m.Item("快速打开命令 (Ctrl+K)").Chosen() {
			s.PaletteOpen = true
			s.PaletteCursor = 0
		}
		m.Separator()
		srcLbl := "查看 AsterML 源码"
		if activeTab.Mode == ModeSource {
			srcLbl = "返回可视化编辑"
		}
		if m.Item(srcLbl).Chosen() {
			if s.OnMobileDocToolbarAction != nil {
				s.OnMobileDocToolbarAction(*activeTab, "source_mode")
			} else if activeTab.Mode == ModeSource {
				s.SetActiveTabMode(ModeEdit)
			} else {
				s.SetActiveTabMode(ModeSource)
			}
		}
		if s.OnMobileDocToolbarAction != nil {
			if m.Item("复制 AsterML 原文").Chosen() {
				s.OnMobileDocToolbarAction(*activeTab, "copy_source")
			}
			if m.Item("导出 PDF…").Chosen() {
				s.OnMobileDocToolbarAction(*activeTab, "export_pdf")
			}
			if m.Item("另存为模板").Chosen() {
				s.OnMobileDocToolbarAction(*activeTab, "save_template")
			}
			if m.Item("创建实验副本").Chosen() {
				s.OnMobileDocToolbarAction(*activeTab, "duplicate")
			}
			m.Separator()
			if m.Item("移入回收站").Chosen() {
				s.OnMobileDocToolbarAction(*activeTab, "delete")
			}
		}
	})
}

func (s *Shell) renderMobileDockedTopChrome(c *ui.Context, tok theme.Tokens, activeTab *WorkspaceTab) {
	title := "实验日程"
	if activeTab != nil && activeTab.Title != "" {
		title = activeTab.Title
	}
	ui.Row(c).FillWidth().Height(46).Padding(float32(tok.Spacing4), float32(tok.Spacing12)).
		Background(tok.Surface).
		AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
		treeBtn := components.IconButton(c, lucide.PanelLeft(), "打开实验库抽屉", components.IconButtonCompact)
		if treeBtn.Clicked() {
			if s.MobileDrawer == "tree" {
				s.MobileDrawer = ""
			} else {
				s.MobileDrawer = "tree"
			}
		}

		components.Text(c, title).FontSize(float32(tok.TextBody)).Bold().
			TextColor(tok.Text).SingleLine().Ellipsis("…").Grow(1).MinWidth(0)

		if activeTab != nil && activeTab.Kind == TabFile {
			if activeTab.Mode == ModeEdit {
				labBtn := components.IconButton(c, lucide.FlaskConical(), "切换到实验模式（防误触）", components.IconButtonCompact)
				if labBtn.Clicked() {
					s.MobileDockForceNav = false
					s.SetActiveTabMode(ModeLab)
				}
			} else {
				editBtn := components.IconButton(c, lucide.PenLine(), "切换到编辑态", components.IconButtonCompact)
				if editBtn.Clicked() {
					s.MobileDockForceNav = false
					s.SetActiveTabMode(ModeEdit)
				}
			}
			moreBtn := components.IconButton(c, lucide.Ellipsis(), "文档与更多操作", components.IconButtonCompact)
			s.attachMobileDocMenu(c, moreBtn, activeTab)
		} else {
			if components.IconButton(c, lucide.Search(), "快速打开 (Ctrl+K)", components.IconButtonCompact).Clicked() {
				s.PaletteOpen = true
				s.PaletteCursor = 0
			}
		}
	})
	ui.Box(c).FillWidth().Height(1).Background(tok.Border)
}

func (s *Shell) renderMobilePullDownQuickAction(c *ui.Context, tok theme.Tokens) {
	st := s.MobileContentScrollState()
	if st.Y > 2 {
		s.MobilePullOffset = 0
		return
	}
	topZone := s.MobileTopReserve(c)
	pullZone := ui.Box(c.Key("mb-pull-zone")).Absolute().Left(68).Right(96).Top(topZone).Height(22)
	_, dy, dragging := pullZone.Dragged()
	if dragging && dy > 0 {
		damped := dy * 0.72
		if damped > 92 {
			damped = 92 + (damped-92)*0.25
		}
		s.MobilePullOffset = damped
	} else if s.MobilePullOffset > 0 {
		triggered := s.MobilePullOffset >= mobilePullActionThresh
		s.MobilePullOffset = 0
		if triggered {
			s.executeMobilePullAction()
			return
		}
	}

	if s.MobilePullOffset <= 4 {
		return
	}
	ready := s.MobilePullOffset >= mobilePullActionThresh
	pillFg := tok.TextMuted
	lbl := "继续下拉：" + s.mobilePullActionLabel()
	if ready {
		pillFg = tok.AccentText
		lbl = "松手打开：" + s.mobilePullActionLabel()
	}
	dropY := s.MobilePullOffset * 0.65
	if dropY > 44 {
		dropY = 44
	}
	ui.Row(c.Key("mb-pull-pill")).Absolute().Left(48).Right(48).Top(topZone+6).Center().PassThrough().Children(func() {
		pill := ui.Row(c).Padding(float32(tok.Spacing4)*1.5, float32(tok.Spacing12)).
			Radius(20).Translate(0, dropY).
			AlignItems(ui.Center).Gap(float32(tok.Spacing8))
		if ready {
			theme.ApplyAccentGlass(pill, tok, false)
		} else {
			theme.ApplyCapsuleGlass(pill, tok, false)
		}
		pill.Children(func() {
			ui.Icon(c, lucide.Zap()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(pillFg)
			components.Text(c, lbl).FontSize(float32(tok.TextMeta)).Bold().TextColor(pillFg).SingleLine()
		})
	})
}

func (s *Shell) renderMobileEdgeSwipeSensors(c *ui.Context, sheetMaxW float32) {
	leftEdge := ui.Box(c.Key("mb-edge-left")).Absolute().Left(0).Top(0).Bottom(0).Width(mobileEdgeSwipeZoneWidth)
	if dx, _, dragging := leftEdge.Dragged(); dragging && dx > 4 {
		s.MobileSwipeKind = "tree"
		p := dx / sheetMaxW
		if p > 1 {
			p = 1
		}
		s.MobileDrawerProgress = p
	} else if s.MobileSwipeKind == "tree" && s.MobileDrawer == "" {
		vx, _ := leftEdge.DragVelocity()
		if s.MobileDrawerProgress > 0.32 || vx > 320 {
			s.MobileDrawer = "tree"
			s.MobileDrawerProgress = 1
		} else {
			s.MobileDrawerProgress = 0
		}
		s.MobileSwipeKind = ""
	}

	rightEdge := ui.Box(c.Key("mb-edge-right")).Absolute().Right(0).Top(0).Bottom(0).Width(mobileEdgeSwipeZoneWidth)
	if dx, _, dragging := rightEdge.Dragged(); dragging && dx < -4 {
		s.MobileSwipeKind = "outline"
		p := (-dx) / sheetMaxW
		if p > 1 {
			p = 1
		}
		s.MobileDrawerProgress = p
	} else if s.MobileSwipeKind == "outline" && s.MobileDrawer == "" {
		vx, _ := rightEdge.DragVelocity()
		if s.MobileDrawerProgress > 0.32 || vx < -320 {
			s.MobileDrawer = "outline"
			s.MobileDrawerProgress = 1
		} else {
			s.MobileDrawerProgress = 0
		}
		s.MobileSwipeKind = ""
	}
}

func (s *Shell) renderMobileFloatingBottomBar(
	c *ui.Context,
	tok theme.Tokens,
	stage ui.Element,
	activePane *PaneNode,
	activeTab *WorkspaceTab,
) {
	editingBar := s.MobileEditToolbar &&
		activeTab != nil &&
		activeTab.Kind == TabFile &&
		activeTab.Mode == ModeEdit &&
		!s.MobileDockForceNav

	targetHide := float32(0)
	if s.MobileDockHidden && !editingBar {
		targetHide = 1
	}
	hideT := stage.AnimateTo("mb-dock-hide", targetHide, theme.MotionFluid)

	if hideT > 0.85 {
		pill := ui.ButtonBase(c.Key("mb-dock-reveal")).Absolute().
			Left(120).Right(120).Bottom(8).Height(18).Center().Label("显示导航栏")
		pill.Children(func() {
			ui.Box(c).Width(42).Height(4).Radius(2).Background(tok.TextMuted.Alpha(0.45))
		})
		if pill.Clicked() {
			s.MobileDockHidden = false
		}
		return
	}

	tabCount := 1
	if activePane != nil && len(activePane.Tabs) > 0 {
		tabCount = len(activePane.Tabs)
	}

	// Centered compact floating capsule (horizontally scrollable on narrow screens for edit toolbar!)
	vw, _ := c.Size()
	maxCapsuleW := float32(360)
	if vw > 48 {
		maxCapsuleW = vw - 32
	}
	ui.Row(c.Key("mb-dock-wrap")).Absolute().Left(16).Right(16).Bottom(14).Center().PassThrough().Children(func() {
		capsule := ui.Row(c.Key("mb-dock-capsule")).Height(48).MaxWidth(maxCapsuleW).Radius(24).
			Padding(0, 12).AlignItems(ui.Center).Gap(4).Clip().
			Translate(0, hideT*78).
			Opacity(1 - hideT*0.92)
		theme.ApplyCapsuleGlass(capsule, tok, false)
		capsule.Children(func() {
			if editingBar {
				ui.ScrollHorizontal(c.Key("mb-edit-tb-scroll")).MaxWidth(maxCapsuleW - 24).Height(40).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
						s.renderMobileEditToolbarItems(c, tok, activeTab)
					})
				})
				return
			}
			s.renderMobileNavDockItems(c, tok, activeTab, tabCount)
		})
	})
}

func (s *Shell) renderMobileNavDockItems(c *ui.Context, tok theme.Tokens, activeTab *WorkspaceTab, tabCount int) {
	navBtn := func(key, label string, enabled, active bool, custom func(fg ui.Color), onClick func()) {
		b := ui.ButtonBase(c.Key("mb-nav-"+key)).Size(40, 40).Radius(20).Center().
			Disabled(!enabled).Label(label)
		if b.Hovered() && enabled {
			b.Background(tok.Text.Alpha(0.06))
		} else if active {
			b.Background(tok.Text.Alpha(0.07))
		}
		theme.ApplyTactilePress(b)
		fg := tok.Text
		if !enabled {
			if tok.Dark {
				fg = tok.TextMuted.Alpha(0.45)
			} else {
				fg = tok.TextMuted.Alpha(0.35)
			}
		} else if active {
			fg = tok.Accent
		}
		b.Children(func() {
			custom(fg)
		})
		if enabled && b.Clicked() {
			onClick()
		}
	}

	// 1. Back (<)
	navBtn("back", "后退", s.CanBack(), false, func(fg ui.Color) {
		ui.Icon(c, lucide.ChevronLeft()).Size(20, 20).TextColor(fg)
	}, func() { s.GoBack() })

	// 2. Forward (>)
	navBtn("fwd", "前进", s.CanForward(), false, func(fg ui.Color) {
		ui.Icon(c, lucide.ChevronRight()).Size(20, 20).TextColor(fg)
	}, func() { s.GoForward() })

	// 3. Search / Quick Switcher (🔍)
	navBtn("search", "搜索", true, s.PaletteOpen, func(fg ui.Color) {
		ui.Icon(c, lucide.Search()).Size(19, 19).TextColor(fg)
	}, func() {
		s.MobileDrawer = ""
		s.MobileTabSwitcherOpen = false
		s.PaletteOpen = true
		s.PaletteCursor = 0
	})

	// 4. New Experiment (+)
	navBtn("new", "新建", true, false, func(fg ui.Color) {
		ui.Icon(c, lucide.Plus()).Size(21, 21).TextColor(fg)
	}, func() {
		s.MobileDrawer = ""
		s.MobileTabSwitcherOpen = false
		s.MobileDockForceNav = false
		s.triggerVaultNewFile("")
	})

	// 5. Tab Counter Box ([N]) - faithful to Obsidian iOS Tab Switcher glyph!
	navBtn("tabs", "标签", true, s.MobileTabSwitcherOpen, func(fg ui.Color) {
		ui.Box(c).Size(20, 20).Radius(5).Border(1.6, fg).Center().Children(func() {
			components.Text(c, fmt.Sprintf("%d", tabCount)).
				FontSize(11).Bold().TextColor(fg).FontFeatures("tnum").SingleLine()
		})
	}, func() {
		s.MobileDrawer = ""
		s.MobileTabSwitcherOpen = !s.MobileTabSwitcherOpen
	})

	// 6. Drawer / Menu (≡)
	navBtn("menu", "实验库与工具", true, s.MobileDrawer != "", func(fg ui.Color) {
		ui.Icon(c, lucide.Menu()).Size(20, 20).TextColor(fg)
	}, func() {
		s.MobileTabSwitcherOpen = false
		if s.MobileDrawer != "" {
			s.MobileDrawer = ""
		} else {
			s.MobileDrawer = "tree"
		}
	})
}

func (s *Shell) renderMobileEditToolbarItems(c *ui.Context, tok theme.Tokens, activeTab *WorkspaceTab) {
	triggerAction := func(act string) {
		if s.OnMobileDocToolbarAction != nil && activeTab != nil {
			s.OnMobileDocToolbarAction(*activeTab, act)
		} else if act == "undo" && s.OnUndo != nil {
			s.OnUndo()
		} else if act == "redo" && s.OnRedo != nil {
			s.OnRedo()
		}
	}
	toolIcon := func(key string, icon *ui.SVG, label string, accent bool, onClick func()) {
		b := ui.ButtonBase(c.Key("mb-ed-"+key)).Size(36, 36).Radius(18).Center().Label(label)
		if b.Hovered() {
			b.Background(tok.Text.Alpha(0.06))
		}
		theme.ApplyTactilePress(b)
		fg := tok.Text
		if accent {
			fg = tok.Accent
		}
		b.Children(func() {
			ui.Icon(c, icon).Size(18, 18).TextColor(fg)
		})
		if b.Clicked() {
			onClick()
		}
	}

	toolIcon("undo", lucide.Undo2(), "撤销", false, func() { triggerAction("undo") })
	toolIcon("redo", lucide.Redo2(), "重做", false, func() { triggerAction("redo") })
	ui.Box(c).Width(1).Height(18).Background(tok.BorderStrong.Alpha(0.55))
	toolIcon("step", lucide.ListPlus(), "+步骤", true, func() { triggerAction("add_step") })
	toolIcon("check", lucide.ListChecks(), "核对", false, func() { triggerAction("add_checklist") })
	toolIcon("param", lucide.SlidersHorizontal(), "参数", false, func() { triggerAction("add_param") })
	toolIcon("plate", lucide.Grid3x3(), "孔板", false, func() { triggerAction("add_plate") })
	toolIcon("timer", lucide.Timer(), "计时", false, func() { triggerAction("open_timer") })
	ui.Box(c).Width(1).Height(18).Background(tok.BorderStrong.Alpha(0.55))
	toolIcon("nav", lucide.LayoutGrid(), "切换底部导航栏", false, func() {
		s.MobileDockForceNav = true
	})
}

func (s *Shell) renderMobileSwipeDrawers(c *ui.Context, tok theme.Tokens, sheetMaxW float32) {
	activeKind := s.MobileDrawer
	if activeKind == "" && s.MobileSwipeKind != "" && s.MobileDrawerProgress > 0.01 {
		activeKind = s.MobileSwipeKind
	}
	if activeKind != "" {
		s.mobileLastDrawerKind = activeKind
	}
	targetP := float32(0)
	if s.MobileDrawer != "" {
		targetP = 1
	} else if s.MobileSwipeKind != "" {
		targetP = s.MobileDrawerProgress
	}

	root := ui.Box(c.Key("mb-drawer-host")).Absolute().Fill().PassThrough()
	pAnim := root.AnimateTo("mb-drawer-p", targetP, theme.MotionFluid)
	if s.MobileSwipeKind != "" {
		pAnim = targetP
	}
	if activeKind == "" && pAnim <= 0.01 {
		s.mobileLastDrawerKind = ""
		return
	}
	if activeKind == "" {
		activeKind = s.mobileLastDrawerKind
		if activeKind == "" {
			activeKind = "tree"
		}
	}

	root.Children(func() {
		maxScrim := float32(0.18)
		if tok.Dark {
			maxScrim = 0.48
		}
		scrimAlpha := maxScrim * pAnim
		backdrop := ui.Box(c.Key("mb-drawer-scrim")).Absolute().Fill().Background(tok.Text.Alpha(scrimAlpha))
		if s.MobileDrawer != "" && backdrop.Clicked() {
			s.MobileDrawer = ""
			s.MobileDrawerProgress = 0
		}

		isLeft := activeKind == "tree"
		sheetOffsetX := float32(0)
		if isLeft {
			sheetOffsetX = -(1 - pAnim) * sheetMaxW
		} else {
			sheetOffsetX = (1 - pAnim) * sheetMaxW
		}

		sheet := ui.Column(c.Key("mb-sheet-"+activeKind)).Absolute().Top(0).Bottom(0).Width(sheetMaxW).
			Shadow(0, 8, 32, 0, tok.Text.Alpha(0.16)).
			Translate(sheetOffsetX, 0)
		theme.ApplyPanelGlass(sheet, tok, 0.45)
		if isLeft {
			sheet.Left(0)
		} else {
			sheet.Right(0)
		}

		if s.MobileDrawer != "" {
			if dx, _, dragging := sheet.Dragged(); dragging {
				if isLeft && dx < -4 {
					p := 1 + dx/sheetMaxW
					if p < 0 {
						p = 0
					}
					sheet.Translate((1-p)*(-sheetMaxW), 0)
					s.MobileDrawerProgress = p
				} else if !isLeft && dx > 4 {
					p := 1 - dx/sheetMaxW
					if p < 0 {
						p = 0
					}
					sheet.Translate((1-p)*sheetMaxW, 0)
					s.MobileDrawerProgress = p
				}
			} else if s.MobileDrawerProgress > 0 && s.MobileDrawerProgress < 0.99 {
				vx, _ := sheet.DragVelocity()
				dismiss := s.MobileDrawerProgress < 0.65 || (isLeft && vx < -280) || (!isLeft && vx > 280)
				if dismiss {
					s.MobileDrawer = ""
				}
				s.MobileDrawerProgress = 0
			}
		}

		sheet.Children(func() {
			if isLeft {
				// Clean Obsidian iOS Drawer: Scrollable Tree on top + Floating 3-Tier Bottom Control Island
				ui.Box(c).FillWidth().Grow(1).MinHeight(0).Padding(float32(tok.Spacing12), float32(tok.Spacing8), 0, float32(tok.Spacing8)).Children(func() {
					s.renderFileTreeScrollBody(c, tok)
				})
				s.renderMobileDrawerBottomIsland(c, tok, "tree")
			} else {
				ui.Box(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
					s.rightDrawer(c)
				})
				s.renderMobileDrawerBottomIsland(c, tok, "outline")
			}
		})
	})
}

func (s *Shell) renderMobileDrawerBottomIsland(c *ui.Context, tok theme.Tokens, activeMode string) {
	expCnt, tplCnt := 0, 0
	for _, f := range s.VaultFiles {
		if f.IsTemplate {
			tplCnt++
		} else if !f.IsAsset {
			expCnt++
		}
	}
	if expCnt == 0 && len(s.Experiments) > 0 {
		expCnt = len(s.Experiments)
	}

	ui.Column(c.Key("mb-drawer-island")).FillWidth().
		Padding(float32(tok.Spacing8), float32(tok.Spacing16), float32(tok.Spacing16), float32(tok.Spacing16)).
		Gap(float32(tok.Spacing12)).Children(func() {
		// Tier 1: Quiet centered icon-only action toolbar (New, Folder, Calendar, Pantry, Collapse)
		ui.Row(c).FillWidth().Justify(ui.SpaceEvenly).AlignItems(ui.Center).Children(func() {
			miniAction := func(icon *ui.SVG, label string, onClick func()) {
				b := ui.ButtonBase(c).Size(36, 36).Radius(18).Center().Label(label)
				theme.ApplyTactilePress(b)
				b.Children(func() {
					ui.Icon(c, icon).Size(18, 18).TextColor(tok.Text)
				})
				if b.Clicked() {
					onClick()
				}
			}
			miniAction(lucide.SquarePen(), "新建实验", func() {
				s.MobileDrawer = ""
				s.triggerVaultNewFile("")
			})
			miniAction(lucide.FolderPlus(), "新建文件夹", func() {
				s.startNewFolderPrompt("")
			})
			miniAction(lucide.CalendarDays(), "日程主页", func() {
				s.MobileDrawer = ""
				s.OpenCalendarTab()
			})
			miniAction(lucide.Package(), "储藏室", func() {
				s.MobileDrawer = ""
				s.OpenPantryTab()
			})
			miniAction(lucide.ChevronsDownUp(), "折叠全部", func() {
				for k := range s.folderOpen {
					s.folderOpen[k] = false
				}
			})
		})

		// Tier 2: Pill mode switcher (📁 实验列表 ↕ / 📋 步骤与速算 ↕)
		modeIcon := lucide.Folder()
		modeText := "实验列表"
		if activeMode == "outline" {
			modeIcon = lucide.ListTree()
			modeText = "步骤与速算"
		}
		pillSwitch := ui.ButtonBase(c.Key("mb-drawer-mode-pill")).FillWidth().Height(40).
			Radius(20).Padding(0, float32(tok.Spacing16)).Background(tok.Elevated).Label(modeText)
		theme.ApplyTactilePress(pillSwitch)
		pillSwitch.Children(func() {
			ui.Row(c).Fill().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Icon(c, modeIcon).Size(17, 17).TextColor(tok.Text)
				components.Text(c, modeText).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text).Grow(1).MinWidth(0)
				ui.Icon(c, lucide.ChevronsUpDown()).Size(14, 14).TextColor(tok.TextMuted)
			})
		})
		if pillSwitch.Clicked() {
			if activeMode == "tree" {
				s.MobileDrawer = "outline"
			} else {
				s.MobileDrawer = "tree"
			}
		}

		// Tier 3: Vault title/stats on left + Standalone 40x40 Circular Glass Settings Gear on right!
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
					components.Text(c, "AsterLab").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
					ui.Icon(c, lucide.ChevronDown()).Size(14, 14).TextColor(tok.TextMuted)
				})
				components.Text(c, fmt.Sprintf("%d 份实验 · %d 模板", expCnt, tplCnt)).
					FontSize(float32(tok.TextMeta)*0.9).TextColor(tok.TextMuted).FontFeatures("tnum")
			})

			gearBtn := ui.ButtonBase(c.Key("mb-drawer-settings")).Size(40, 40).Radius(20).Center().Label("打开设置")
			theme.ApplyCapsuleGlass(gearBtn, tok, true)
			theme.ApplyTactilePress(gearBtn)
			gearBtn.Children(func() {
				ui.Icon(c, lucide.Settings()).Size(18, 18).TextColor(tok.Text)
			})
			if gearBtn.Clicked() {
				s.MobileDrawer = ""
				s.VaultSettingsOpen = true
			}
		})
	})
}

func (s *Shell) renderMobileTabSwitcherOverlay(c *ui.Context, tok theme.Tokens, activePane *PaneNode, activeTab *WorkspaceTab) {
	if activePane == nil {
		s.MobileTabSwitcherOpen = false
		return
	}
	overlay := ui.Column(c.Key("mb-tab-switcher")).Absolute().Fill().
		Padding(float32(tok.Spacing16), float32(tok.Spacing16)).
		Gap(float32(tok.Spacing12)).
		Transition(theme.FloatingBarTransition())
	theme.ApplyPanelGlass(overlay, tok, 0.35)
	overlay.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			components.Text(c, fmt.Sprintf("标签页概览 (%d)", len(activePane.Tabs))).
				FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text).Grow(1).MinWidth(0)
			if components.Button(c, "+ 新建实验", components.ButtonGhost).Clicked() {
				s.MobileTabSwitcherOpen = false
				s.triggerVaultNewFile("")
			}
			if components.Button(c, "完成", components.ButtonPrimary).Clicked() {
				s.MobileTabSwitcherOpen = false
			}
		})

		ui.Scroll(c).FillWidth().Grow(1).MinHeight(0).Children(func() {
			ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Padding(0, 0, float32(tok.Spacing24), 0).Children(func() {
				for idx := 0; idx < len(activePane.Tabs); idx += 2 {
					ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
						for j := 0; j < 2; j++ {
							pos := idx + j
							if pos >= len(activePane.Tabs) {
								ui.Box(c).Grow(1).MinWidth(0)
								continue
							}
							t := activePane.Tabs[pos]
							isActive := activeTab != nil && activeTab.ID == t.ID
							cardBg := tok.Elevated
							var borderW float32 = 0
							cardBorder := ui.Transparent
							if isActive {
								cardBg = tok.AccentSoftBg
								cardBorder = tok.Accent
								borderW = 1.5
							}

							card := ui.ButtonBase(c.Key("mb-ts-"+t.ID)).Grow(1).MinWidth(0).Height(96).
								Padding(float32(tok.Spacing12)).Radius(14).
								Background(cardBg).Border(borderW, cardBorder).
								Label(t.Title)
							theme.ApplyTactilePress(card)
							closedCard := false
							card.Children(func() {
								ui.Column(c).Fill().Gap(float32(tok.Spacing4)).Children(func() {
									ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
										ico := lucide.FlaskConical()
										switch t.Kind {
										case TabCalendar:
											ico = lucide.CalendarDays()
										case TabPantry:
											ico = lucide.Package()
										}
										ui.Icon(c, ico).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
										ui.Spacer(c)
										if isActive {
											components.Badge(c, "当前", components.BadgeOptions{Tone: components.BadgeAccent})
										}
										if !t.Pinned && len(activePane.Tabs) > 1 {
											xBtn := components.IconButton(c, lucide.X(), "关闭标签页", components.IconButtonCompact)
											if xBtn.Clicked() {
												closedCard = true
												s.CloseTabInPane(activePane.ID, t.ID)
											}
										}
									})
									components.Text(c, firstNonEmptyStr(t.Title, "未命名")).
										FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
									sub := "实验日程"
									switch t.Kind {
									case TabPantry:
										sub = "试剂耗材储藏室"
									case TabFile:
										modeLbl := "编辑态"
										if t.Mode == ModeRead {
											modeLbl = "阅读态"
										} else if t.Mode == ModeSource {
											modeLbl = "源码态"
										}
										dir := filepath.ToSlash(filepath.Dir(t.FilePath))
										if dir == "" || dir == "." {
											dir = "根目录"
										}
										sub = fmt.Sprintf("%s · %s", dir, modeLbl)
									}
									components.Text(c, sub).FontSize(float32(tok.TextMeta)*0.88).
										TextColor(tok.TextMuted).SingleLine().Ellipsis("…")
								})
							})
							if !closedCard && card.Clicked() {
								s.SwitchTabInPane(activePane.ID, t.ID, true)
								s.MobileTabSwitcherOpen = false
							}
						}
					})
				}
			})
		})
	})
}
