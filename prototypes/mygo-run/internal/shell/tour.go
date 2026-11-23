package shell

import (
	"fmt"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
)

type tourStepSpec struct {
	Badge   string
	Title   string
	Summary string
	Tips    []string
	Region  string // "left" | "top" | "center" | "ai" | "right"
}

var workbenchTourSteps = []tourStepSpec{
	{
		Badge:   "1 / 5 · 实验库与目录",
		Title:   "左侧：你的全部实验文档、模板与储藏室",
		Summary: "所有实验都保存在本机文件夹里，互不干扰。上方标签可切换文件目录、实验日程和试剂储藏室。",
		Tips: []string{
			"右键文件夹：新建实验、新建分组目录、重命名或移入回收站",
			"把常用流程「另存为模板」，下次开新实验一键复用",
		},
		Region: "left",
	},
	{
		Badge:   "2 / 5 · 编辑与实验模式",
		Title:   "顶栏：写方案用「编辑」，进实验室切「实验」",
		Summary: "「编辑」模式自由排版与设计分组；做实验时点「实验」（或按 Ctrl+E）开启防误触保护。",
		Tips: []string{
			"实验模式下会锁住步骤原文与孔板布局，避免手套误碰改乱方案",
			"依然可以直接启动计时、勾待办、录入读数、标记异常孔和关联照片",
		},
		Region: "top",
	},
	{
		Badge:   "3 / 5 · 交互组件与右键菜单",
		Title:   "画布：孔板、计算器、数据表自动联动",
		Summary: "点选或右键即可完成绝大多数操作，不需要记任何代码语法。",
		Tips: []string{
			"孔板：拖选孔位直接点分组涂色；右键孔板可「横排 / 8道排枪竖排 / 外周留空」",
			"数据表：点进任意单元格直接 Ctrl+V 粘贴 Excel 或酶标仪读数；右上角 ··· 可切孔板矩阵",
			"计算器：点「+ 引入孔板有效孔」或「+ 引用上游计算」，修改上游自动重算",
		},
		Region: "center",
	},
	{
		Badge:   "4 / 5 · 文字自动变工具",
		Title:   "像平时一样写步骤，一键生成计算器或计时器",
		Summary: "写下“将 VEGF 稀释至 50 ng/mL，终体积 5 mL”或“37°C 孵育 30 min”后，无需手动配表。",
		Tips: []string{
			"光标停在操作句时，点击行尾微亮的 ✦ 图标即可预览并一键插入组件",
			"或者按住 Alt 键将鼠标悬停在句子上（快捷键 Alt+Enter 也可锁定弹窗）",
		},
		Region: "ai",
	},
	{
		Badge:   "5 / 5 · 大纲、计时与质检",
		Title:   "右侧抽屉：步骤目录、全局计时与对照检查",
		Summary: "长方案随时点步骤跳转，同时自动检查对照组、样本量 n 和照片关联关系。",
		Tips: []string{
			"随时从左下角/状态栏「指南」重新打开本导览或「新手入门指南」互动文档",
			"按 Ctrl+K 快速切文档，按 Ctrl+Shift+F 搜索全部实验记录",
		},
		Region: "right",
	},
}

// StartTour opens the interactive 5-step region-highlighting onboarding tour from step 0.
func (s *Shell) StartTour() {
	s.TourOpen = true
	s.TourStep = 0
	s.syncTourRightDrawer()
}

// syncTourRightDrawer ensures the Right Companion Drawer is actually open during
// Step 5 ("right") so the highlight box frames the real drawer instead of empty canvas,
// and cleanly restores its prior collapsed state when stepping away or closing.
func (s *Shell) syncTourRightDrawer() {
	needRight := s.TourOpen && s.TourStep >= 0 && s.TourStep < len(workbenchTourSteps) &&
		workbenchTourSteps[s.TourStep].Region == "right"
	if needRight {
		if !s.RightOpen {
			s.tourAutoOpenedRight = true
			s.SetRightDrawer(true, 0)
		}
	} else if s.tourAutoOpenedRight {
		s.tourAutoOpenedRight = false
		s.SetRightDrawer(false, s.RightTab)
	}
}

// renderSpotlightTour draws a calm region-highlighted overlay teaching the 5 workbench zones.
func (s *Shell) renderSpotlightTour(c *ui.Context, tok theme.Tokens) {
	if !s.TourOpen {
		return
	}
	if s.TourStep < 0 {
		s.TourStep = 0
	}
	if s.TourStep >= len(workbenchTourSteps) {
		s.TourStep = len(workbenchTourSteps) - 1
	}
	step := workbenchTourSteps[s.TourStep]

	// Full-window calm scrim overlay
	ui.Box(c.Key("spotlight-tour-scrim")).Absolute().Fill().
		Background(tok.Text.Alpha(0.32)).Children(func() {
		// Visual spotlight frame highlighting the target region of the current step
		hl := ui.Box(c.Key("tour-hl-"+step.Region)).Absolute().
			Radius(float32(tok.PanelRadius)).
			Background(tok.Accent.Alpha(0.08)).
			Border(2, tok.Accent)
		switch step.Region {
		case "left":
			hl.Top(8).Bottom(34).Left(8).Width(252)
		case "top":
			hl.Top(4).Left(272).Right(14).Height(88)
		case "center":
			hl.Top(62).Bottom(44).Left(292).Right(48)
		case "ai":
			hl.Top(160).Left(320).Right(80).Height(110)
		case "right":
			hl.Top(4).Bottom(4).Right(4).Width(float32(tok.Spacing48)*6.2 + 2)
		}

		// Centered floating guide card with crisp step navigation
		cardWrap := ui.Box(c.Key("tour-card-wrap")).Absolute().Fill().Center()
		cardWrap.Children(func() {
			card := ui.Column(c.Key(fmt.Sprintf("tour-card-%d", s.TourStep))).
				Width(float32(tok.Spacing48)*10.5).
				Padding(float32(tok.Spacing24)).
				Radius(float32(tok.PanelRadius)).
				Background(tok.Surface).Border(1, tok.BorderStrong).
				Gap(float32(tok.Spacing12))
			card.Children(func() {
				// Top header: step badge + close
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					components.Badge(c, step.Badge, components.BadgeOptions{Tone: components.BadgeAccent})
					ui.Spacer(c)
					closeBtn := components.IconButton(c, lucide.X(), "关闭导览", components.IconButtonCompact)
					if closeBtn.Clicked() {
						s.TourOpen = false
						s.syncTourRightDrawer()
					}
				})

				ui.Text(c, step.Title).FontSize(float32(tok.TextSection)+1).Bold().TextColor(tok.Text)
				ui.Text(c, step.Summary).FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)

				if len(step.Tips) > 0 {
					ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).
						Radius(float32(tok.ControlRadius)).Background(tok.Elevated).
						Gap(float32(tok.Spacing8)).Children(func() {
						for idx, tip := range step.Tips {
							ui.Row(c.Key(fmt.Sprintf("tour-tip-%d-%d", s.TourStep, idx))).FillWidth().
								AlignItems(ui.Start).Gap(float32(tok.Spacing8)).Children(func() {
								ui.Box(c).Size(6, 6).Radius(3).Background(tok.Accent).Margin(6, 0, 0, 0).Shrink(0)
								ui.Text(c, tip).FontSize(float32(tok.TextData)).TextColor(tok.Text).Grow(1).MinWidth(0)
							})
						}
					})
				}

				// Progress dots + actions
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
						for i := range workbenchTourSteps {
							dotCol := tok.BorderStrong
							dotW := float32(6)
							if i == s.TourStep {
								dotCol = tok.Accent
								dotW = 18
							}
							ui.Box(c).Width(dotW).Height(6).Radius(3).Background(dotCol)
						}
					})
					ui.Spacer(c)
					if s.OnOpenUserGuide != nil {
						if components.Button(c, "打开新手指南文档", components.ButtonGhost).Clicked() {
							s.TourOpen = false
							s.syncTourRightDrawer()
							s.OnOpenUserGuide()
						}
					}
					if s.TourStep > 0 {
						if components.Button(c, "上一步", components.ButtonOutline).Clicked() {
							s.TourStep--
							s.syncTourRightDrawer()
						}
					}
					if s.TourStep < len(workbenchTourSteps)-1 {
						if components.Button(c, "下一步", components.ButtonPrimary).Clicked() {
							s.TourStep++
							s.syncTourRightDrawer()
						}
					} else {
						if components.Button(c, "开始使用", components.ButtonPrimary).Clicked() {
							s.TourOpen = false
							s.syncTourRightDrawer()
						}
					}
				})
			})
		})
	})
}
