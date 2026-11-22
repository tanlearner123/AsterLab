# 桌面端工作台、平权分屏与储藏室

桌面端以「**三个主要表面 + 平权四向分屏 + 全库闭环辅助面板**」组织全部科研界面，去除多余顶栏工具堆砌与二级模式弹窗。

## 工作区四域布局

1. **左栏（40px Ribbon + Vault 文件树）**
   - 文件树依据显式 `depth` 绘制纵向缩进引导线，支持 `F2` 内联重命名（同步自动重写全库 `[[wikilink]]` 与 `repeat_of`）、文件右键菜单，以及直接将 `.asterml` 文件拖入日程改期或拖入编辑区边缘拆屏。
2. **主区（`PaneNode` 平权分屏树 + Obsidian Live Preview + 底部状态栏）**
   - 任意叶窗格均可平权托管 `实验日程`、`储藏室货架` 或 `.asterml` 文档（**`编辑 | 实验模式（防误触） | 源码`**），拖拽页签至窗格四周 25% 热区即可通过 `LiquidSplitTransition` 动态四向分屏。
   - **纯白纸起点与 Obsidian Live Preview**：新建实验打开即空白 Markdown 画布（零预置步骤）；基于 `mygo@v0.10.0` 原生 `editor` 实现光标行露源码、非光标行渲染标题/粗体/行内可点复选框/科学关联胶囊，并支持行首 `/` 命令、`{@` 目标补全、`[[` 双链补全与统一悬浮上下文卡。
   - **实验模式（`ModeLab`，`Ctrl+E` 一键切换）**：锁定既有方案正文与模块结构防止湿实验误触，仅开放预留执行槽位（勾选、计时、实测偏离、读数矩阵、孔板异常孔与拍图关联），并在文末常驻**实验空白补记区**允许随时自由补记。
   - 文档页采用简洁单行面包屑头 + 实验/编辑模式切换 + 页内查找（`Ctrl+F`）+ 自适应阅读列宽（`760px` 正文 / `940px` 宽孔板与数据表）。
   - 底部常驻 `24px` **Obsidian 风格状态栏**，实时展示保存状态（`实时已保存` / 语法提醒）、当前步骤与模块计数、可点击的**全库运行中计时器角标**、模式名称（`编辑模式` / `实验模式（防误触）` / `源码模式`）与全局命令快捷键提示。
3. **右伴侣抽屉（三页签，默认折叠）**
   - 收拢与实验直接相关的三类工具：**「步骤大纲」**、**「速算与计时（含跨 Vault 实时计时器与跳转）」**、**「关联与诊断（双向链接/复现谱系 + AsterCore 分级诊断）」**。
4. **全库全文搜索与命令面板**
   - `Ctrl+Shift+F` 打开 **Vault 全局全文搜索模态窗**，键盘上下键与回车直达对应 `.asterml` 文件并自动唤醒页内 `Ctrl+F` 高亮定位。
   - `Ctrl+K` / `Ctrl+O` 命令面板统一检索实验文件、储藏室试剂与工作区动作。

## 分屏、全文搜索、伴侣抽屉与科学分析实拍

| 桌面分屏 + 右伴侣抽屉（Light） | 桌面分屏 + 右伴侣抽屉（Dark） |
|---|---|
| ![桌面分屏 Light](../sdk/evidence/v2-visual-catalog/workbench_obsidian_split_drawer_light.png) | ![桌面分屏 Dark](../sdk/evidence/v2-visual-catalog/workbench_obsidian_split_drawer_dark.png) |

| `Ctrl+Shift+F` 全局全文搜索（Dark） | 右抽屉「关联与诊断」页签（Light） | AsterCore 科学分析汇总 + `Ctrl+F`（Light） |
|---|---|---|
| ![全局全文搜索 Dark](../sdk/evidence/v2-visual-catalog/workbench_global_search_dark.png) | ![右抽屉关联与诊断 Light](../sdk/evidence/v2-visual-catalog/companion_drawer_links_diagnostics_light.png) | ![科学分析汇总 Light](../sdk/evidence/v2-visual-catalog/unified_live_analysis_summary_light.png) |

| UnifiedLive「编辑」态 | UnifiedLive「实验模式（防误触）」 | UnifiedLive「源码」态 |
|---|---|---|
| ![UnifiedLive 编辑态](../sdk/evidence/v2-visual-catalog/unified_live_edit_light.png) | ![UnifiedLive 实验模式](../sdk/evidence/v2-visual-catalog/unified_live_read_light.png) | ![UnifiedLive 源码态](../sdk/evidence/v2-visual-catalog/unified_live_source_light.png) |

## Obsidian Live Preview、纯白纸起点、行内关联卡与实验模式实拍

| 纯白纸空白文档（零预置步骤） | Obsidian Live Preview 混排（Light） | Obsidian Live Preview 混排（Dark） |
|---|---|---|
| ![纯白纸文档](../sdk/evidence/v2-visual-catalog/unified_blank_whitepaper_light.png) | ![Live Preview Light](../sdk/evidence/v2-visual-catalog/unified_live_preview_huvec_light.png) | ![Live Preview Dark](../sdk/evidence/v2-visual-catalog/unified_live_preview_huvec_dark.png) |

| 行内图片关联卡 `![[...]]{@plate:A1 6h fov=1}` | 实验模式（锁原文 + 开放预留槽位 + 空白补记区） |
|---|---|
| ![行内图片关联卡](../sdk/evidence/v2-visual-catalog/unified_embed_relation_card_light.png) | ![实验模式防误触](../sdk/evidence/v2-visual-catalog/unified_lab_mode_huvec_light.png) |

| 方案灯箱放大 (`figure@1`) | Notebook ELN 双栏对照 |
|---|---|
| ![方案灯箱放大](../sdk/evidence/v2-visual-catalog/design_iz_figure_zoom_light.png) | ![Notebook ELN 双栏对照](../sdk/evidence/v2-visual-catalog/notebook_eln_split_light.png) |

## 实验日程、命令面板、设置中心与快照恢复

- **实验日程（Home）**：默认提供月视图与周时间网格，支持直接从左栏拖拽实验改期、点击空白格快速建实验，以及右键变更状态。
- **设置中心（`Ctrl+,`）**：支持全文关键字过滤、可绑定热键映射（含 `Ctrl+Shift+F`）、`TextScale` 字号缩放、`ReduceMotion` 动效降级、回收站一键还原与 **File Recovery 历史版本快照一键恢复**，并在 Dark 模式下使用高对比度实底卡片消除透射重影。

| 实验日程月视图（Light） | 实验日程月视图（Dark） | 实验日程周网格（Light） |
|---|---|---|
| ![日程月视图 Light](../sdk/evidence/v2-visual-catalog/home_calendar_month_light.png) | ![日程月视图 Dark](../sdk/evidence/v2-visual-catalog/home_calendar_month_dark.png) | ![日程周视图 Light](../sdk/evidence/v2-visual-catalog/home_calendar_week_light.png) |

| 从模板新建实验 | `Ctrl+K` 全局命令面板 | 设置中心 — 外观（Light） | 设置中心 — 快照恢复（Dark） |
|---|---|---|---|
| ![新建实验对话框](../sdk/evidence/v2-visual-catalog/vault_new_exp_dialog_light.png) | ![Ctrl+K 命令面板](../sdk/evidence/v2-visual-catalog/workbench_command_palette_light.png) | ![设置中心外观](../sdk/evidence/v2-visual-catalog/vault_settings_appearance_light.png) | ![设置中心快照恢复 Dark](../sdk/evidence/v2-visual-catalog/vault_settings_recovery_dark.png) |

## 储藏室货架、容器贴纸与独立工具

储藏室通过 `internal/uikit/labsticker` 生成基于 NIH BioArt 矢量化与 EDT 物理白边的试剂耗材图标，并与实验文档内 `material@1` 的 `pantry_code` 双向定位；配套提供 9×9 冻存盒、96 孔 PCR 盒、离心管架孔位盘点及独立孔板 Studio / 速算台。

| 储藏室货架主页（Light） | 储藏室货架（Dark） | 实验器材矢量贴纸全表 |
|---|---|---|
| ![储藏室货架](../sdk/evidence/v2-visual-catalog/tools_pantry_standalone_light.png) | ![储藏室暗黑模式](../sdk/evidence/v2-visual-catalog/comp_16c_labsticker_shelf_dark_v3.png) | ![实验器材贴纸表](../sdk/evidence/v2-visual-catalog/comp_16_labsticker_sheet_v3.png) |

| 9×9 冻存盒（Light） | 9×9 冻存盒（Dark） | 96 孔 PCR 管盒 | 15/50mL 离心管架 |
|---|---|---|---|
| ![9x9 冻存盒 Light](../sdk/evidence/v2-visual-catalog/tools_pantry_box_cryo_9x9_light.png) | ![9x9 冻存盒 Dark](../sdk/evidence/v2-visual-catalog/tools_pantry_box_cryo_9x9_dark.png) | ![96 孔 PCR 管盒](../sdk/evidence/v2-visual-catalog/tools_pantry_box_pcr_96_light.png) | ![离心管架](../sdk/evidence/v2-visual-catalog/tools_pantry_box_falcon_rack_light.png) |

| 试剂详单与消耗追溯 | 独立孔板 Studio | 独立实验速算台 |
|---|---|---|
| ![试剂详单](../sdk/evidence/v2-visual-catalog/comp_16d_pantry_reagent_detail_v3.png) | ![孔板 Studio](../sdk/evidence/v2-visual-catalog/tools_plate_studio_v3.png) | ![独立速算台](../sdk/evidence/v2-visual-catalog/tools_bench_calc_v1.png) |

## 上下文右键菜单 (`ContextMenu`)

低频管理操作与批处理全部下沉至上下文右键菜单，不在卡片表面常态常驻一排文字按钮：

| 文件树右键 | 窗格标签页右键 | 孔板选区右键 | 步骤卡片右键 | 日程事件右键 |
|---|---|---|---|---|
| ![文件树右键](../sdk/evidence/v2-visual-catalog/context_menu_file_tree_light.png) | ![页签右键](../sdk/evidence/v2-visual-catalog/context_menu_pane_tab_light.png) | ![孔板右键](../sdk/evidence/v2-visual-catalog/context_menu_plate_light.png) | ![步骤卡片右键](../sdk/evidence/v2-visual-catalog/context_menu_step_block_light.png) | ![日程事件右键](../sdk/evidence/v2-visual-catalog/context_menu_calendar_light.png) |
