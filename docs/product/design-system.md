# AsterLab 设计系统

本文是 AsterLab UI 的唯一视觉与语义规范。任何 UI 改动开工前必读，验收时必须能引用本文条目。

权威关系：通用颜色/间距/圆角/字号由 `Tokens` 定义；具备科学语义的颜色由本文登记的具名调色板提供。本文规定使用方式与允许的例外；实现与本文冲突时，以对应的 `Tokens` 或具名调色板函数为准，并修本文。

## 1. 单一参数源

所有通用视觉参数来自 `theme.GetTokens(c.Theme())`；唯一例外是本文件登记的具名科学语义颜色调色板。禁止在调用点写字面值。

| 类别 | 唯一来源 | 禁止写法 | 正确写法 |
|---|---|---|---|
| 颜色 | `Tokens`（`Canvas/Surface/Elevated/SelectedRow/Border/Text/TextMuted/Accent/Success/Warning/Danger` 及 `*Bg/*Text`）；具名科学调色板（见 §1 例外） | 在渲染/组件调用点写 `ui.Hex(...)`、`ui.RGBA(...)` | `tokens.Surface`、`tokens.DangerText`；调色板色复用具名函数 |
| 字号 | `TextPage/TextSection/TextBody/TextMeta/TextData`，或 `t.Rem(x)`（随主题缩放） | `FontSize(12)`、`FontSize(11)` | `FontSize(float32(tokens.TextMeta))`、`.FontSize(t.Rem(.85))` |
| 间距 | `Spacing4/8/12/16/24/32/48`，或 `t.Space(n)`（n 是单位倍数；`Theme.Spacing=4`，`t.Space(1/2/3/4/6/8/12)` 对应 `4/8/12/16/24/32/48`） | `Padding(3, 8)`、`Padding(6, 12)`、`Gap(5)` | `Padding(t.Space(2), t.Space(4))`、`Gap(t.Space(1))` |
| 图标 | `IconSm/IconMd/IconLg`（随 `EffectiveScale()` 与字号联动）；主导航/工具栏按钮用 `IconButtonPlain`，仅行内微操用 `IconButtonCompact` | 主导航栏滥用 `IconButtonCompact` 或写死 `Size(12, 12)` | `.Size(float32(tokens.IconMd), float32(tokens.IconMd))` |
| 圆角 | `ControlRadius`（控件）/`PanelRadius`（面板）/`DialogRadius`（对话框） | `Radius(3)`、`Radius(5)`、`Radius(7)` | `.Radius(float32(tokens.ControlRadius))` |
| 控件高度 | `ButtonHeight/InputHeight/RowHeight` | `Height(28)`、`MinHeight(33)` | `.Height(float32(tokens.ButtonHeight))` |

颜色字面值只允许出现在 `internal/theme/theme.go`（定义）与确有科学语义的具名调色板函数（如孔板分组色、多维分类色与孔角色色）中；已有同语义调色板时必须复用，不得散落在渲染代码里。

当前在 `internal/theme/theme.go` 登记的具名科学调色板函数：
- `theme.CategoricalPalette(dark)` / `theme.CategoricalColor(key, dark)`：8 色无障碍实验室分类调色板，用于多维孔板（`plate@1`）按样本、正交因子（如靶点基因、加药浓度）与技术复孔区分着色，并配适亮/暗色主题；
- `theme.GradientRampColor(tNorm, dark)`：孔板数值浓度梯度因子的连续单调色阶渐变（图例按浓度从高到低排序）；
- `theme.WellRoleColor(role, dark)`：孔位对照角色语义色（`sample` / `standard` / `ntc` / `blank` / `positive_control` / `negative_control` / `vehicle`）。

## 2. 间距、行距与正文/组件区分体系（强制设计语言）

- **4px 基准间距**：只用 `Spacing4 (4)`、`Spacing8 (8)`、`Spacing12 (12)`、`Spacing16 (16)`、`Spacing24 (24)`、`Spacing32 (32)`、`Spacing48 (48)`。
- **统一行间距标准（Typography Rhythm）**：
  - 自由 Markdown 正文（`LiveMarkdownEditor`）：`LineHeight = 1.65`（中文/英文混排舒适读感）；
  - Markdown 标题（`#` ~ `####`）：`LineHeight = 1.4`；
  - 表格/元数据行：单行 `SingleLine()` + `FontFeatures("tnum")` 数值对齐。
- **统一块间距标准（Vertical Block Rhythm）**：
  - 顶层文档流（正文 ↔ 组件、组件 ↔ 组件）：统一 `Spacing16 (16px)`；
  - 组件内部骨架（标题头 ↔ 分割线 ↔ 内容体）：统一 `Spacing12 (12px)`；
  - 组件内同级控件/表格行间距：统一 `Spacing8 (8px)`；图标与文字间距：`Spacing8 (8px)` 或 `Spacing4 (4px)`；
  - 页面级分隔：`Spacing32 / Spacing48`（900×600 下压至 `Spacing24 / Spacing32`，绝不低于 `Spacing4`）。
- **普通 Markdown 正文 vs. 结构化科学组件的视觉区分（一眼可辨）**：
  - **普通 Markdown 正文（纸面层 Paper Layer）**：直接铺在 `tok.Canvas` 纸面上，**零卡片底色、零外边框、零标题头栏**，呈现无干扰的 Obsidian 纸面书写体验。
  - **结构化科学组件（模块卡片层 Module Surface Layer）**：所有顶层科学组件（`group_table`、`plate`、`param_table`、`data_table`、`calculator`、`figure`、`attachment`、`subflow`、`embed_card`）统一由 `renderUnifiedModuleShell` 承载：
    - 表面：`Background(tok.Surface)` + `Border(1, tok.Border)` + `Radius(tok.PanelRadius)` + `Padding(tok.Spacing12, tok.Spacing16)`；
    - **统一头部骨架（所有组件必须一致，严禁有的有图标有的没图标）**：
      `[16px 语义图标 (tok.Accent)]` + `[可编辑标题 (TextSection Bold)]` + `[Spacer]` + `[右对齐计数/状态摘要 (TextMeta tnum)]` + `[可选紧凑图标动作，如放大/源码]` → 下接 `1px` 发丝线 `tok.Border.Alpha(0.55)` → 组件详情区；
    - 嵌套于 `step@1` 步骤卡片内的子块自动省略外层重边框，仅保留统一的 `[图标 + 标题 + 摘要] + 发丝线 + 内容区`，严禁卡片套重卡片。
- **语义嵌套**：容器内边距 ≥ 子元素间距；同一层嵌套的上下左右取同一档；相邻同级元素均分剩余空间。

## 3. 圆角

三档，不做第四种：控件 `ControlRadius`(6)、面板 `PanelRadius`(10)、对话框 `DialogRadius`(12)。同类容器全项目一致。嵌套圆角保持内小外大，不要出现内外同值或内大外小。连续圆角在性能允许时优先；性能不允许时在本文记录例外，不在代码里各写各的。

## 4. 组件语义

先选组件，再套样式；不要给错误的组件换皮。来源：`github.com/oneo-me/onevra/source/components`（本地 `~/go/pkg/mod/github.com/oneo-me/onevra/source@*/components/`）、mygo `ui.*` 原语、仓库 `internal/uikit` 与各 `internal/<domain>` 包。

| 需求 | 唯一组件 | Do | Don't |
|---|---|---|---|
| 主/次操作 | `components.Button`（Primary/Secondary/Ghost 三档） | 一个表面只有一个 Primary | 用 `ui.ButtonBase` 手搓按钮外观 |
| 图标操作 | `components.IconButton` + `lucide.*` | 提供 tooltip 文案 | 用文字按钮做图标动作 |
| 分段切换 | `components.TabsSegmented` | 视图切换（流程/孔板/配液单） | 自己写一排可点 Box |
| 状态标签 | `components.Badge` | 用 §3 状态词 | 自造彩色圆点标签 |
| 表格 | `components.Table` / `TableVirtualized` | 记录、参数表 | 用 Column+Row 拼表头 |
| 键值对 | 标签 `TextMeta` + 值 `TextData/TextBody`，值加 `tnum` | 数值对齐 | 标语式大字号 |
| 空态 | 一行 `TextMeta` + 一个下一步动作 | 说清下一步 | 插画 + 营销文案 |
| 错误/警告 | `Badge` + 行内横幅（`*Bg`/`*Text` token） | 原因 + 重试 | 只弹 Toast 就消失 |
| 对话框 | `components.Dialog`（`DialogRadius`） | 破坏性动作二次确认 | 自建浮层 |
| 侧栏/导航 | `components.Sidebar` / `components.Tabs` | 沿用既有 shell | 每页新写一套导航 |
| 进度/预算 | `components.Progress` 或既有 `budgetBar` | 数值来自单一来源并标注来源 | 同一指标两套组件 |
| 骨架屏 | `components.Skeleton` | 首屏加载 | 长时间显示「加载中…」而无结构 |

规则：同类控件全项目只有一种用法。需要变体时，在既有组件上加参数，不新写第二个。

## 5. 文案

- 先定页面定位，再写字。一个页面只有一个标题。
- 只保留核心标题、关键键值对与必要的动作标签；说明性文字能删就删。
- 书面语，禁止营销腔、感叹号、emoji、口语（「搞定」「一键起飞」）。
- 低频说明折叠进 tooltip / alert / 悬停详情 / 可展开行，不常驻。
- 状态词只能取 `ux-contract.md` §3 的词表，不自造同义词。
- 单位与数值：数值右对齐 + `tnum`；单位与数值之间留一个空格。

## 6. 反模式基线（2026-11-16，`ad8ed02`）

以下为当前代码里真实存在的违规计数（`prototypes/mygo-run`，排除 `_test.go`）。清理按这张清单另开卡，不要在功能卡里顺手重构。

| 反模式 | 总数 | 集中位置 |
|---|---|---|
| `ui.Hex(...)` 颜色字面值 | 3 行（定义处 `theme.go` 另有 11 行，允许） | `internal/asterdoc/view.go:735,736,739`（`groupColor` 本地调色板）；`uikit` 已收进 `plate.GroupColor`，无字面值 |
| `FontSize(数字)` | 175 | `internal/runui/runui.go` 80、根目录 40（`main.go` 34、`design_page.go` 4、`design_canvas.go` 2）、`internal/protocol` 25、`internal/runworkspace` 22、`internal/tools` 7、`internal/resultgrid` 1 |
| `Padding(数字)` | 97 | `internal/runworkspace` 31、`internal/runui` 23、根目录 16（`main.go` 6、`design_canvas.go` 5、`design_page.go` 2、`notebook_page.go` 3）、`internal/asterdoc` 10、`internal/resultgrid` 7、`internal/designer` 4、`internal/protocol` 3，另 `internal/shell`/`blockui`/`blocks` 各 1 |
| `Radius(数字)` | 58 | `internal/runworkspace` 30、`internal/runui` 19、`internal/resultgrid` 5、`internal/asterdoc` 3、`main.go` 1 |
| 单参数 `Padding(6)`（6 不是 4 的倍数） | 3 | `internal/resultgrid/view.go:54,60,81`（全项目仅有的三处单参数 `Padding(6)`） |
| 不在 6/10/12 档的 `Radius` | 30 | `runui` 7（`runui.go:630,725,730,755,760,1095` 为 8；`:1139` 为 5）、`runworkspace` 15（`recording.go:138,633,720`；`view.go:536,561,628,692,731,1013,1029,1096,1278,1296,1453,1481`）、`resultgrid` 4（`plate.go:124,474` 为 5；`:162` 为 4；`:417` 为 8）、`asterdoc` 3（`view.go:198,371` 为 4.5；`:567` 为 3）、`main.go:3140`（4） |

已合规的范式：`internal/uikit/plate/plate.go:277` 以 `t.ControlRadius` 设控件圆角；`internal/runworkspace/view.go` 以 `tokens.PanelRadius` 设面板圆角。间距用 `t.Space(n)`，字号缩放用 `t.Rem(x)`。
`internal/designer/step_list.go:528,558` 当前用 `Radius(t.Space(...))`；这属于现存的不合规圆角用法，不应复制为范例。代码清理不在本次文档卡范围内。

## 7. 验收方式

UI 卡的 PR 必须带 `## 设计一致性` 小节，写明：

1. 用到的 token（列出实际引用，例如 `tokens.Spacing16`、`tokens.TextMeta`）；
2. 复用的既有组件（列名 + 文件）；
3. 新增组件（若有）：名字 + 为何仓库与 onevra 都没有等价实现；
4. 若引入本文未列的新做法，先改本文并说明理由。

可机器检查的部分（后续可加进本地门禁）：

```text
grep -rn "ui\.Hex(" prototypes/mygo-run --include=*.go | grep -v internal/theme/    # 应为 0（或仅具名调色板函数内）
grep -rn "FontSize([0-9]" prototypes/mygo-run --include=*.go                      # 应为 0
grep -rnE "Padding\(([0-9]+, )?[0-9]+" prototypes/mygo-run --include=*.go         # 应为 0
grep -rn "Radius([0-9]" prototypes/mygo-run --include=*.go                        # 应为 0
```

## 8. Do / Don't 范例

每条取自本仓库真实代码或真实违规点，可直接对照。

**D1 颜色**
- Do: `components.Text(c, title).TextColor(tokens.Text)`；分组色复用 `plate.GroupColor`（`internal/uikit/plate/plate.go:86`，内部已用 token，无字面值）。
- Don't: 在视图里自建调色板（`internal/asterdoc/view.go:734-739` `groupColor`，7 个 `ui.Hex` 字面值）；应复用 `plate.GroupColor` 或收进 `theme`。

**D2 字号**
- Do: `.FontSize(t.Rem(.85))` 或 `.FontSize(float32(tokens.TextMeta))`。
- Don't: `.FontSize(12)`（`internal/runui/runui.go` 有 80 处；主题字号变化时它们不跟随）。

**D3 间距**
- Do: `.Padding(t.Space(2), t.Space(4))`。
- Don't: `.Padding(6, 12)`（`design_page.go:37`、`design_canvas.go:111`）——6 不是 4 的倍数，且绕过了 `t.Space` 的缩放。

**D4 圆角**
- Do: `.Radius(float32(tokens.ControlRadius))`。
- Don't: `.Radius(8)`（`internal/runui/runui.go:630,725,730,755,760,1095`；另 `:1139` 为 `Radius(5)`）——不在 6/10/12 三档内，同类容器随后各写各的。

**D5 按钮**
- Do: `components.Button(c, "创建 Run", components.ButtonPrimary)`，一屏只有一个 Primary。
- Don't: `ui.ButtonBase(c).Radius(8).Padding(6, 12)` 手搓按钮（`internal/runui/runui.go:630`）。

**D6 分段控件**
- Do: `components.Tabs(c, &index, components.TabsSegmented, ...)`。
- Don't: 自己写一排可点 `ui.Box` 冒充分段控件。

**D7 状态标签**
- Do: `components.Badge` + `ux-contract.md` §3 的词（`草稿`/`未保存`/`校验中`/`未解析`）。
- Don't: 自造色点 + 新词（如把 `校验中` 写成「检查中」）。

**D8 错误呈现**
- Do: 行内横幅（`tokens.DangerBg`/`DangerText`）+ 原因 + `重试`。
- Don't: 只 `c.AddToast(...)` 让原因一闪而过。

**D9 表格与数值**
- Do: `components.Table`；数值列 `.FontFeatures("tnum")` 右对齐。
- Don't: 用 `Column`+`Row` 手拼表头，或数值左对齐导致列抖动。

**D10 空态**
- Do: 一行 `TextMeta` 说明 + 一个明确的下一步动作。
- Don't: 插画 + 营销文案，或只有「暂无数据」而不给出口。

**D11 文案**
- Do: 标题「实验设计」、值「9/12 孔」、动作「开始实验」。
- Don't: 「一键搞定你的实验！」这类营销腔，或常驻的三段说明文字（应折叠进 tooltip）。

**D12 新增组件**
- Do: 先在 onevra `components/` 与仓库既有包里找等价实现，复用或加参数。
- Don't: 每个页面新写一套视觉词汇（P3 直接失败）。

---

## 9. AsterML v2 通用模块与 Obsidian 单库工作台组件映射（2026-11-20）

`prototypes/mygo-run` 采用 **Obsidian 级单库本地工作区 + 单真源 `.asterml` 文件工作台**：左侧采用 `40px` 极简图标 `Ribbon`（`实验日历` / `储藏室货架` / `新建实验` / `新建项目文件夹` / `库路径与设置`）与透明本地目录文件树（`.asterlab/` 系统态、`assets/` 附件资源、`templates/` 无日期模板、`项目文件夹/*.asterml`；支持 `F2` 重命名、拖拽移动/分屏/排期及完整右键菜单）；中心为全等分屏树 `PaneNode`（支持 `编辑 | 阅读 | 源码` 切换、多窗格共享同一 `*vault.LiveDocument` 与 100 步 `Ctrl+Z/Y` 撤销栈、边缘拖拽四向分屏与中键关闭）；右侧伴侣抽屉默认折叠（只留 `步骤大纲` 与 `速算与计时`）。全部通用控件严格遵循统一 `Tokens` 与社区组件规范：

| 场景 / 通用模块 | 复用组件与包 | 关键规范 |
|---|---|---|
| Obsidian 级全等分屏壳、文件树、历史栈与 `Ctrl+K/O` 命令面板 | `onevra/shells/sidebar`（原生 `VibrancyMica`/`VibrancySidebar`）+ `glass.Glass` + `components.{SplitPercent,Dialog,ContextMenu}` + `internal/shell/{tabs,file_tree,right_drawer,command_palette}.go` + `internal/vault/{vault,live_doc}.go` | 左侧 `Ribbon` 与右抽屉透传系统 `Vibrancy`（无多余不透明背景与边框）；**文件树按显式 `depth` 绘制竖向层级引导线与 `16px` 折叠占位槽，子文件严格缩进于父文件夹图标内侧**；中心平权分屏去外框与圆角，由发丝线分割、焦点页顶部 `2px` `Accent` 指示；标签栏采用 `glass.Glass` 半透明磨砂；`Ctrl+K/O` 命令面板 |
| 高度自由的 `实验日程`（`月` / `周`）与正交 `项目 × 日期` | `internal/shell/calendar.go` + `onevra/components.{Popover,Tabs,Button,ContextMenu}` + `theme.Motion*` | 项目归属由实验所在文件夹决定，实验时间由 frontmatter `date` 决定；日历与文件树正交联动——可从左树拖拽实验到某天排期、点击/右键日程格新建（可选项目与模板）或跨天拖拽改期 |
| Obsidian Live-Preview 单真源文档画布（`UnifiedLive`） | `experiment_page.go` + `internal/asterdoc/{inputs,view,composer,modules,widget_prose,widget_ops,widget_tables,widget_figure,plate_ops}.go` | **无框正文哲学（Anti-XP-Box）**：标题、日期、项目、状态、步骤 H2、检查项与参数均采用 `asterdoc.BorderlessTextInput`（基于 `ui.TextInputBase`，静态无框无底色，仅悬停/聚焦柔和高亮）与 `StyleSelectQuiet`，禁止白底圆角文本框套娃；外层模块与表格全面改为纸面发丝线网格与弹性列宽（禁止固定总宽溢出），动作统一收敛为 `IconButton` / `ButtonGhost` |
| 实操型内联模块（清单 / 计时 / 参数 / 计算 / 数据表） | `asterdoc.View{UnifiedLive: true}` + `onevra/components.{Checkbox,Popover,MenuButton,ContextMenu}` | **唯一真源内联记录**：`checklist@1` 与 `timer@1`（`▶/⏸/↺`）在**编辑**和**阅读**态均直接可点并写入 `.asterml`；`param_table@1` 悬浮行触发 `记偏离` 展开实测输入框；`calculator@1` 支持 `@plate.wells` 与实测联动；`data_table@1` 支持内联编辑、TSV/CSV 粘贴、方向键导航与表头属性配置 |
| 示意图 / 拍照图片 `figure@1` 与附件 `attachment@1` | `widget_figure.go` / `widget_ops.go` + `components.Dialog` + `mygo/transfer.{PNG,FileList}` + `vault.ImportAssetBytes` | 支持 `Ctrl+V` 粘贴剪贴板截图与文件拖入，自动落库 `<Vault>/assets/` 并引用 `./assets/<name>`；图片与实拍附件弹窗使用 `ui.Box(c.Key(...)).Children(...)` 包裹 `components.Dialog` 防止多浮层 Key 冲突，支持 `HeroFlyTransition` 一镜到底缩放 |
| 图层化通用孔板 `plate@1` | `internal/uikit/plate` + `view.go` + `plate_ops.go` | 顶部图层胶囊条（`[分组]` + 各因子/浓度图层 + `[+]` 新建图层）；框选后底部托出 `单值 \| 平铺列表 \| 等比序列`（`÷2/÷5/÷10` + 按行/列）填充条；支持完整右键菜单（清空、复制/粘贴标注、反选、选中边缘孔、标记 `excluded_wells`、导出 CSV/PNG） |
| Obsidian 风格设置中心、多色盘主题引擎与 Liquid Glass 移动端 | `onevra/components.Settings` + `glass.Glass` + `internal/shell/{settings,mobile}.go` + `internal/theme/theme.go` + `internal/vault/vault.go` | **多色盘与材质切换**：支持 `obsidian`（纯白 `#ffffff` + `#f5f6f8` 无边框卡片 / 暗色 `#111215`）、`zinc`（冷银极简 `#f4f5f7`）与 `warm`（经典暖纸 `#f7f6f3`）三档预设及 `LiquidGlass` 开关，持久化写入 `.asterlab/config.json`。**Light/Dark Liquid Glass 分级**：浅色悬浮胶囊用 `glass.Regular`（76% 奶白雾透 + 亮边）；暗色严禁多层玻璃叠加（桌面左右侧栏保持纯正深空底色，禁止 Glass-on-Glass），移动端暗色胶囊采用 `glass.Clear` + `84%` 深烟熏曜石遮罩（避免 `paneDarkLow`/`2.2x saturation` 导致背景穿刺发灰）与亮银高光边。**移动端 iOS 26 布局**：`< 680dp` 或移动平台收敛为左上独立 `40dp` 圆形玻璃钮 + 右上双图标融合药丸 + 居中 `48dp` 纯图标底浮岛（`< > 🔍 + [N] ≡`）及编辑态纯图标条，抽屉配备三层底部控制岛与边缘滑出跟手物理反馈 |
| 步骤内模块拖拽排序与单页 PDF 导出 | `composer.go`（`.Drag/.DragOver/.Drop`）+ `sheet_view.go` + `pdf_sheet.go` | 悬浮手风琴显隐与手柄拖拽排序；`ExportDocumentPDF` 与纸面同源输出完整方案、实测参数、勾选状态、数据表及 `./assets/` 附图 |

---

## 10. 动效系统与微交互规范（Motion System & Micro-interactions）

桌面科研效率工作台的动效遵循 **「精密科学仪器 × 温润纸质实验本」** 原则：高频键盘/输入操作零延迟，中频空间转换使用高刚度可打断物理弹簧维持空间连续性，低频达成态使用克制微交互提供确认感。**调用点严禁手写任意毫秒数字面值或散落自定义弹簧，必须统一复用 `internal/theme/motion.go`：**

| 动效 Token / 构造器 | 曲线与物理特征 | 强制适用边界 |
|---|---|---|
| `theme.MotionMicro` | `100ms` `EaseOut` + `Oklab` | 按钮/孔位按压微缩（`ApplyTactilePress` `0.97`）、悬浮 `⋮⋮` 手柄与 `+` 插入线显隐 |
| `theme.MotionSnappy` | 临界阻尼弹簧 (`160ms, Bounce: 0`) + `Oklab` | 共享几何磁吸滑块（`SharedPillTransition`：标签栏、`Ctrl+K` 列表高亮、孔板图层胶囊、填充模式切换）、孔板 Oklab 染色过渡、选孔底条托出（`FloatingBarTransition`，退场 `110ms`） |
| `theme.MotionFluid` | 柔和弹簧 (`220ms, Bounce: 0.04`) + `Oklab` | 文件夹/步骤块/检查项/实测偏离框手风琴生长收拢（`AccordionTransition`）、分屏四向 25% 预览框液态变形（`LiquidSplitTransition`）、按位数字滚轮（`NumericRoll`：计时器跳秒、计算器结果、选孔计数） |
| `theme.MotionHero` | 空间过渡弹簧 (`260ms, Bounce: 0.03`) + `Oklab` | `figure@1` 与 `plate@1` 缩略图至放大灯箱的一镜到底过渡（`HeroFlyTransition`）、日历月/周网格切换 |
| `theme.ApplyNudge` | 欠阻尼速度脉冲 (`k=580, c=16, v0=240`) | 空输入或非法倍比提交时的 `±5px` 海绵轻晃提醒（替代刺眼弹窗报错） |

**动效红线**：
1. 禁止从 `Scale: 0` 缩放弹出（最小起始缩放 `0.95 ~ 0.98`）；退场时长必须短于进场时长（`100ms ~ 120ms`）；初始常驻标签禁止挂 `Enter: {Collapse: true}` 以免首帧被零宽裁切。
2. 色彩插值统一启用 `Oklab: true`；所有位移与弹簧自动遵循系统 `Preferences.ReduceMotion` 无障碍降级；`components.Dialog` 外层须独立用 keyed `ui.Box` 包裹（不在参数内直接传 `c.Key(...)`）。

**前端科研文案规范（强制）**：所有面向研究者的界面文案、状态胶囊、提示与报错严禁暴露 `Core`、`Rust Core`、`后端`、`schema`、`JSON`、`readout.submit`、`AsterDoc`、`rev`、内部数据库版本号（如 `"第 N 版"` / `"方案第 N 版"`）、英文原词（如 `insufficient`、`missing variable`）、开发术语（如旧版层名或 `Notebook`）或原始数据库 ID（如 `blk-*`、`assay-*`、`inhibition_zone_mm`）；统一使用自然科研表达（如「数据已记录」「本步实验操作与记录」「方案耗时测算」「实验测定数据表」「计划中」「进行中」「已完成」「实时保存」「开始实验」「记偏离」「+ 追加标签」），底层标识仅在「步骤高级选项与技术标识」「技术属性（只读）」等二级折叠区按 `UX-P2` 按需展示。

---

## 11. 全局光学对齐、精简顶栏与编辑器排版铁律（对标 Obsidian × TopInspo）

1. **全局行内光学对齐铁律（禁止 `AlignSelf(ui.Start)` 破坏居中）**：
   - `onevra` 的 `components.Badge` 内部默认带 `AlignSelf(ui.Start)`，放入横排卡片底栏/标题栏/计时器行时会导致胶囊上浮错位。
   - **强制规范**：所有水平横排条（文档头、图片关联底栏、计时器行、计算器标题栏）必须使用 `AlignItems(ui.Center)`，且所有行内胶囊与徽标必须通过 `inlineBadge(...)`（显式 `.AlignSelf(ui.Center)`）及统一 `24dp` (`Spacing24`) 高度渲染，确保芯片、徽标、幽灵按钮处于同一条水平中线。
2. **顶部双栏极简框架（对标 Obsidian）**：
   - 桌面端顶部仅允许 **2 条**结构栏：① **统一顶栏标签条**（左紧凑 `← →` 历史导航 + 标签页 + `+`，右紧凑 `🔍` 快速打开与 `◫` 右抽屉开关）；② **单窗格视图头**（面包屑 + 实验模式/查找/复制/更多）。禁止再叠第三条低密度空白顶条。
3. **顶部纯图标悬浮工具栏 + 单 `+` 直连 Slash 命令面板**：
   - 取消文档底部常驻文字按钮栏；顶部居中仅保留极轻 6 图标胶囊（`H / B / I / 无序列表 / 待办清单 | +`），纯图标无常驻文字（仅 hover tooltip）。
   - 单 `+` 组件入口与行首 `/` 共享同一套 TopInspo 风格命令面板（`24×24` 图标块 + 粗体中文名 + 单行说明 + 右侧等宽 `/cmd` 胶囊，支持 `↑↓ / Enter / 单次 Esc 撤回关闭`）。
   - **实验模式 (`ModeLab`) 原样渲染锁定**：工具栏位置、宽度、高度原样保留，仅置灰锁定禁用，零布局跳动。
4. **Live Preview 排版与复选框规范**：
   - 中文友好正文行高 `1.75`；标题采用清晰排版跨度与呼吸感（`H1 22px / LH 1.65`、`H2 18px / LH 1.65`、`H3 15.5px / LH 1.6`）。
   - 行内待办清单 `- [ ]` / `- [x]` 使用 `14×14px` 光学居中圆角复选框，不直接使用 `16×16px` 表单大框挤压行距。
5. **计算器自解释表格解剖规范**：
   - 顶部必须提供清晰表头（`项目名称 | 公式代号 | 数值 / 公式表达式 | 单位 | 计算结果`）及一键预设模板菜单。
   - 公式代号以等宽浅底代码胶囊呈现（保留 `_` 合法变量字符），修改代号时自动词边界联动重写同表公式；输入行右侧静默标示「输入项」，公式行左侧显 `=` 徽标且右侧以 Accent 粗体突出计算输出。
