package shell

import (
	"fmt"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/vault"
)

func (s *Shell) refreshGlobalSearch() {
	q := strings.TrimSpace(s.GlobalSearchQuery)
	s.GlobalSearchLastQuery = s.GlobalSearchQuery
	s.GlobalSearchCursor = 0
	if q == "" {
		s.GlobalSearchHits = nil
		return
	}
	if s.OnGlobalSearch != nil {
		s.GlobalSearchHits = s.OnGlobalSearch(q)
		return
	}
	lq := strings.ToLower(q)
	var hits []vault.SearchHit
	for _, exp := range s.resolvedExperiments() {
		if strings.Contains(strings.ToLower(exp.Title+" "+exp.Summary+" "+exp.FilePath), lq) {
			hits = append(hits, vault.SearchHit{
				RelPath: exp.FilePath,
				Title:   exp.Title,
				Folder:  exp.Folder,
				Snippet: firstNonEmptyStr(exp.Summary, string(exp.Stage)),
				Date:    exp.Date,
				Status:  string(exp.Stage),
			})
		}
	}
	s.GlobalSearchHits = hits
}

func (s *Shell) openGlobalSearchHit(hit vault.SearchHit) {
	q := strings.TrimSpace(s.GlobalSearchQuery)
	if s.OnOpenSearchHit != nil {
		s.OnOpenSearchHit(hit.RelPath, q)
		return
	}
	for _, exp := range s.resolvedExperiments() {
		if exp.FilePath == hit.RelPath {
			s.openExperiment(exp)
			return
		}
	}
}

func (s *Shell) renderGlobalSearchModal(c *ui.Context) {
	if !s.GlobalSearchOpen {
		ui.Box(c.Key("global-search-dlg")).Children(func() {
			components.Dialog(c, &s.GlobalSearchOpen, components.DialogOptions{Label: "Vault 全局全文搜索"}, nil)
		})
		return
	}
	if s.GlobalSearchQuery != s.GlobalSearchLastQuery {
		s.refreshGlobalSearch()
	}
	tok := theme.GetTokens(c.Theme())
	open := s.GlobalSearchOpen
	hits := s.GlobalSearchHits
	if len(hits) == 0 {
		s.GlobalSearchCursor = 0
	} else {
		if s.GlobalSearchCursor < 0 {
			s.GlobalSearchCursor = 0
		}
		if s.GlobalSearchCursor >= len(hits) {
			s.GlobalSearchCursor = len(hits) - 1
		}
	}

	ui.Box(c.Key("global-search-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "Vault 全局全文搜索"}, func(panel ui.Element) {
			panel.Width(float32(tok.Spacing48)*12.5).Padding(float32(tok.Spacing16)).Gap(float32(tok.Spacing8)).
				MaxHeight(float32(tok.Spacing48) * 12.0)
			if tok.Dark {
				panel.Background(tok.Surface).Border(1, tok.BorderStrong)
			} else if tok.LiquidGlass {
				theme.ApplyPanelGlass(panel, tok, 0.36)
			}
			panel.Children(func() {
				if len(hits) > 0 {
					if panel.Shortcut(0, ui.KeyDown) {
						s.GlobalSearchCursor = (s.GlobalSearchCursor + 1) % len(hits)
					}
					if panel.Shortcut(0, ui.KeyUp) {
						s.GlobalSearchCursor = (s.GlobalSearchCursor - 1 + len(hits)) % len(hits)
					}
					if panel.Shortcut(0, ui.KeyEnter) {
						s.openGlobalSearchHit(hits[s.GlobalSearchCursor])
						open = false
						return
					}
				}
				if panel.Shortcut(0, ui.KeyEscape) {
					open = false
					return
				}

				ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Icon(c, lucide.FileSearch()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
					inp := ui.TextInputBase(c, &s.GlobalSearchQuery).FillWidth().FontSize(float32(tok.TextBody)).TextColor(tok.Text).
						Padding(float32(tok.Spacing4)*0.5, float32(tok.Spacing4)).
						Placeholder("全文检索所有 .asterml 实验步骤、分组、试剂编号或笔记片段…").
						Grow(1).MinWidth(0)
					inp.AutoFocus()
					if strings.TrimSpace(s.GlobalSearchQuery) != "" {
						components.Text(c, fmt.Sprintf("%d 条命中", len(hits))).
							FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
					}
				})

				ui.Divider(c)

				if strings.TrimSpace(s.GlobalSearchQuery) == "" {
					ui.Column(c).FillWidth().Padding(float32(tok.Spacing24)).Center().Gap(float32(tok.Spacing4)).Children(func() {
						components.Text(c, "输入关键词跨 Vault 全文检索").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.TextMuted)
						components.Text(c, "支持搜索步骤标题、孔板配方、Pantry 编号与实验笔记；选中后直接打开并高亮匹配行。").
							FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					})
					return
				}

				if len(hits) == 0 {
					ui.Column(c).FillWidth().Padding(float32(tok.Spacing24)).Center().Gap(float32(tok.Spacing4)).Children(func() {
						components.Text(c, "未找到包含该关键词的实验文档").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.TextMuted)
						components.Text(c, "尝试更换试剂名、细胞系名称或实验日期。").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					})
					return
				}

				ui.Scroll(c).FillWidth().MaxHeight(float32(tok.Spacing48) * 9.0).Children(func() {
					ui.Column(c).FillWidth().Gap(float32(tok.Spacing4)).Children(func() {
						for i, hit := range hits {
							hit := hit
							selected := i == s.GlobalSearchCursor
							targetBg := ui.Transparent
							if selected {
								targetBg = tok.AccentSoftBg
							}
							row := ui.ButtonBase(c.Key(fmt.Sprintf("gsearch-%d-%d-%s", i, hit.LineNum, hit.RelPath))).
								Label(firstNonEmptyStr(hit.Title, hit.RelPath)).
								FillWidth().Justify(ui.Start).
								Padding(float32(tok.Spacing8), float32(tok.Spacing8)).
								Radius(float32(tok.ControlRadius))
							if row.Hovered() && !selected {
								targetBg = tok.Elevated
							}
							row.Background(row.AnimateColor("gs-bg", targetBg, theme.MotionMicroFade))
							theme.ApplyTactilePress(row)
							row.Children(func() {
								ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
									ui.Icon(c, lucide.FileText()).Size(float32(tok.Spacing16), float32(tok.Spacing16)).TextColor(tok.Accent)
									ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing4) * 0.5).Children(func() {
										ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
											components.Text(c, firstNonEmptyStr(hit.Title, hit.RelPath)).
												FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text).SingleLine().Ellipsis("…")
											if hit.Date != "" {
												components.Text(c, hit.Date).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
											}
										})
										snip := hit.Snippet
										if hit.LineNum > 0 {
											snip = fmt.Sprintf("L%d: %s", hit.LineNum, snip)
										}
										components.Text(c, snip).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine().Ellipsis("…")
									})
								})
							})
							if row.Clicked() {
								s.openGlobalSearchHit(hit)
								open = false
							}
						}
					})
				})
			})
		})
	})
	s.GlobalSearchOpen = open
}
