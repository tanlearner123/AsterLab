package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
)

func (s *Shell) renderCrashOrWatchdogBanner(c *ui.Context, tok theme.Tokens) {
	if strings.TrimSpace(s.CrashNotice) == "" {
		return
	}
	ui.Row(c.Key("crash-watchdog-banner")).FillWidth().
		Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		Background(tok.Warning.Alpha(0.10)).
		AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
		ui.Icon(c, lucide.TriangleAlert()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Warning).Shrink(0)
		components.Text(c, s.CrashNotice).FontSize(float32(tok.TextData)).TextColor(tok.Text).Grow(1).MinWidth(0)
		if components.Button(c, "导出诊断包", components.ButtonSecondary).Clicked() && s.OnExportDiagBundle != nil {
			s.OnExportDiagBundle()
		}
		if components.Button(c, "反馈问题", components.ButtonPrimary).Clicked() {
			s.FeedbackCategory = "程序异常"
			s.FeedbackOpen = true
		}
		if components.Button(c, "忽略", components.ButtonGhost).Clicked() {
			s.CrashNotice = ""
			if s.OnDismissCrashNotice != nil {
				s.OnDismissCrashNotice()
			}
		}
	})
	ui.Box(c).FillWidth().Height(1).Background(tok.Warning.Alpha(0.25))
}

func (s *Shell) renderFeedbackDialog(c *ui.Context) {
	if !s.FeedbackOpen {
		ui.Box(c.Key("feedback-dlg")).Children(func() {
			components.Dialog(c, &s.FeedbackOpen, components.DialogOptions{Label: "反馈与诊断中心"}, nil)
		})
		return
	}
	tok := theme.GetTokens(c.Theme())
	if strings.TrimSpace(s.FeedbackCategory) == "" {
		s.FeedbackCategory = "程序异常"
	}
	open := s.FeedbackOpen
	ui.Box(c.Key("feedback-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "反馈与诊断中心"}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*11.5).Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing12))
			if tok.Dark {
				panel.Background(tok.Surface).Border(1, tok.BorderStrong)
			} else if tok.LiquidGlass {
				theme.ApplyPanelGlass(panel, tok, 0.36)
			}
			panel.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Icon(c, lucide.LifeBuoy()).Size(float32(tok.Spacing24), float32(tok.Spacing24)).TextColor(tok.Accent)
					ui.Text(c, "反馈与诊断中心").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
					ui.Spacer(c)
					ver := firstNonEmptyStr(s.AppVersion, "0.1.0-dev")
					components.Badge(c, "v"+strings.TrimPrefix(ver, "v"))
				})

				ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "反馈类别").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
					ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing8)).Children(func() {
						for _, cat := range []string{"程序异常", "安装与更新", "界面排版与显示", "功能建议"} {
							cat := cat
							variant := components.ButtonOutline
							if s.FeedbackCategory == cat {
								variant = components.ButtonPrimary
							}
							if components.Button(c, cat, variant).Clicked() {
								s.FeedbackCategory = cat
							}
						}
					})
				})

				ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "现象描述与复现步骤").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
					components.TextBox(c, &s.FeedbackText, components.TextBoxOptions{Multiline: true}).
						Placeholder("请简要描述您做了什么操作、遇到了什么报错或希望改进的体验…").
						Height(float32(tok.Spacing48) * 2.2)
				})

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					components.Switch(c, &s.FeedbackIncludeLogs, "附加最近脱敏运行日志与环境摘要", components.SwitchOptions{})
				})

				ui.Row(c).FillWidth().Padding(float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
					Background(tok.Elevated).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Icon(c, lucide.ShieldCheck()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Success).Shrink(0)
					ui.Text(c, "隐私保护：用户名、本机用户目录与令牌均自动脱敏为 <USER_HOME> / <REDACTED_TOKEN>，诊断包绝不包含您的 .asterml 实验内容与附件。").
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).Grow(1).MinWidth(0)
				})

				if s.FeedbackStatus != "" {
					ui.Text(c, s.FeedbackStatus).FontSize(float32(tok.TextData)).TextColor(tok.Accent)
				}

				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "导出脱敏诊断包 (.zip)", components.ButtonSecondary).Clicked() && s.OnExportDiagBundle != nil {
						s.OnExportDiagBundle()
					}
					if components.Button(c, "复制报告摘要", components.ButtonGhost).Clicked() && s.OnCopyFeedbackReport != nil {
						s.OnCopyFeedbackReport()
					}
					ui.Spacer(c)
					if components.Button(c, "关闭", components.ButtonGhost).Clicked() {
						open = false
					}
					if components.Button(c, "前往 GitHub 提交", components.ButtonPrimary).Clicked() && s.OnSubmitFeedback != nil {
						s.OnSubmitFeedback()
					}
				})
			})
		})
	})
	s.FeedbackOpen = open
}

func (s *Shell) renderWelcomeDialog(c *ui.Context) {
	if !s.WelcomeOpen {
		ui.Box(c.Key("welcome-dlg")).Children(func() {
			components.Dialog(c, &s.WelcomeOpen, components.DialogOptions{Label: "欢迎使用 AsterLab"}, nil)
		})
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := s.WelcomeOpen
	choiceHandled := false
	ui.Box(c.Key("welcome-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "欢迎使用 AsterLab"}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*12).Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing16))
			if tok.Dark {
				panel.Background(tok.Surface).Border(1, tok.BorderStrong)
			} else if tok.LiquidGlass {
				theme.ApplyPanelGlass(panel, tok, 0.36)
			}
			panel.Children(func() {
				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing12)).Children(func() {
					ui.Image(c, theme.LogoParsedTriStream(tok.Dark)).Width(float32(tok.Spacing48)).Height(float32(tok.Spacing48)*0.85)
					ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4)).Children(func() {
						ui.Text(c, "欢迎使用 AsterLab").FontSize(float32(tok.TextPage)).Bold().TextColor(tok.Text)
						ui.Text(c, "本地科研实验工作台 · 所有实验档案与照片安全保存在你自己的电脑中").
							FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
					})
				})

				ui.Column(c).FillWidth().Padding(float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
					Background(tok.Elevated).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Text(c, "3 步快速上手（无需懂任何代码语法）：").FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text)
					ui.Text(c, "1. 写方案：左侧管理实验档案与模板，画布像普通文档一样打字，点 + 或右键即可插入分组、孔板、计算器和计时器。").
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					ui.Text(c, "2. 文字变工具：写完稀释或孵育句子后，点行尾 ✦（或按住 Alt 悬停）即可一键转为交互组件。").
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					ui.Text(c, "3. 台面执行：走进实验室点右上角「实验」开启防误触，直接计时、勾待办、从 Excel 粘贴读数并自动汇总。").
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				})

				ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
					card := func(key, title, desc string, badge string, primary bool, choice string) {
						row := ui.Row(c.Key("welcome-opt-"+key)).FillWidth().Padding(float32(tok.Spacing12)).
							Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border).
							AlignItems(ui.Center).Gap(float32(tok.Spacing12))
						if primary {
							row.Border(1, tok.Accent.Alpha(0.55)).Background(tok.AccentSoftBg.Alpha(0.45))
						}
						theme.ApplyTactilePress(row)
						row.Children(func() {
							ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4)).Children(func() {
								ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
									ui.Text(c, title).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
									if badge != "" {
										components.Badge(c, badge)
									}
								})
								ui.Text(c, desc).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
							})
							btnStyle := components.ButtonSecondary
							if primary {
								btnStyle = components.ButtonPrimary
							}
							if components.Button(c, "开始", btnStyle).Clicked() {
								open = false
								choiceHandled = true
								if s.OnOnboardingChoice != nil {
									s.OnOnboardingChoice(choice)
								}
							}
						})
					}
					card("guide", "打开「新手入门指南」互动文档（推荐）", "边看边动手：内含可试玩的 12 孔板、配液计算器、计时器、快捷键与全部右键功能速查。", "5 分钟上手", true, "guide")
					card("new", "新建空白实验文档", "打开干净白纸，直接输入笔记，随时点击 + 插入步骤、孔板、计算器与数据表。", "", false, "new")
					card("demo", "体验「CCK-8 细胞活力」完整示范实验", "一键生成含分组、96 孔板、显色计时器与 OD450 汇总统计的真实示范实验。", "", false, "demo")
					card("existing", "打开电脑已有实验库文件夹", fmt.Sprintf("当前目录：%s", firstNonEmptyStr(s.VaultPathInput, "默认实验库目录")), "", false, "existing")
				})

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "1 分钟界面区域导览", components.ButtonOutline).Clicked() {
						open = false
						choiceHandled = true
						if s.OnOnboardingChoice != nil {
							s.OnOnboardingChoice("tour")
						}
					}
					ui.Spacer(c)
					if components.Button(c, "稍后探索", components.ButtonGhost).Clicked() {
						open = false
						choiceHandled = true
						if s.OnOnboardingChoice != nil {
							s.OnOnboardingChoice("skip")
						}
					}
				})
			})
		})
	})
	if !open && !choiceHandled && s.WelcomeOpen && s.OnOnboardingChoice != nil {
		s.OnOnboardingChoice("skip")
	}
	s.WelcomeOpen = open
}
