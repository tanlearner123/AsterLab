# Token、色盘与动效规范

AsterLab 所有界面的颜色、间距、圆角、字号与物理动效均收口于 `internal/theme` 包。开发任何 UI 视图时严禁在业务调用点手写十六进制色值或魔法间距。

## 单一 Token 源与三套色盘预设

主题参数由 [theme.go](../../prototypes/mygo-run/internal/theme/theme.go) 中的 `TokensFor(preset, dark, liquidGlass)` 统一生成，并支持在 **设置 → 外观** 与 **`Ctrl+K` 命令面板**中实时热切换：

| 预设 ID | 浅色模式 (`Dark: false`) | 深色模式 (`Dark: true`) | 设计定位 |
|---|---|---|---|
| **`obsidian`（默认）** | `Canvas #ffffff` · `Elevated #f5f6f8` · `SelectedRow #ededf0` · `Accent #6c5ce7`（软高亮 `#eeebff`） | `Canvas #111215` · `Surface #16181d` · `Elevated #1e2027` · `Accent #a596ff`（软高亮 `#2a2645`） | 纯净白纸与深空曜石底色，零偏色，无框信息卡层次最干净 |
| **`zinc`** | `Canvas #f4f5f7` · `Surface #ffffff` · `Elevated #eceef2` · `Accent #635bff` | `Canvas #0f1115` · `Surface #16181e` · `Elevated #1e2129` · `Accent #a397ff` | 中性冷银底衬纯白卡片，边界分明适合高密度工作台 |
| **`warm`** | `Canvas #f7f6f3` · `Surface #ffffff` · `Accent #5b45c9` | `Canvas #171614` · `Surface #1e1d1a` · `Accent #a996ff` | 经典暖纸备份色盘 |

## 科学语义调色板例外规则

通用界面禁止使用 `ui.Hex(...)`；唯有孔板分组、样本标签与对照角色色允许且必须调用具名科学调色板函数：

- **`theme.CategoricalPalette(dark)` / `CategoricalColor(key, dark)`**：提供 Light/Dark 双套校准的 8 色序列（Cobalt、Emerald、Amber、Violet、Coral、Teal、Indigo、Terracotta）。
- **`theme.GradientRampColor(tNorm, dark)`**：用于 `plate@1` 数值浓度梯度的单调连续色阶渐变（图例自动按浓度从高到低排列）。
- **`theme.WellRoleColor(role, dark)`**：统一映射 `sample`、`standard`、`positive_control`、`negative_control`、`vehicle`、`ntc` 与 `blank`。

## 间距、字号、圆角、正文/组件区分与统一骨架

- **4px 基准间距与块间距节奏**：仅使用 `Spacing4 (4)`、`Spacing8 (8)`、`Spacing12 (12)`、`Spacing16 (16)`、`Spacing24 (24)`、`Spacing32 (32)`、`Spacing48 (48)`。文档顶层块间距统一 `Spacing16`，组件内头部↔内容间距统一 `Spacing12`，组件内行间距统一 `Spacing8`；Markdown 正文行高统一 `1.65`，标题行高统一 `1.4`。
- **普通 Markdown vs. 科学组件一目了然**：普通 Markdown 正文无框无底色直接铺在 `tok.Canvas` 纸面上；所有顶层结构化科学组件（分组表、孔板、参数表、数据表、计算器、图片关联卡等）统一使用 `renderUnifiedModuleShell`（`tok.Surface` + `1px tok.Border` + `PanelRadius`），并强制保持一致的头部骨架：**`[16px Accent 语义图标] [可编辑标题] [Spacer] [右侧 tnum 摘要/操作图标]` + `1px` 发丝分割线 + 组件详情区**（嵌套于步骤卡片内时自动去掉外框避免套娃）。
- **字号层级与全库缩放**：`TextPage (22)`、`TextSection (15)`、`TextBody (13)`、`TextMeta (12)`、`TextData (12)`；控件基准高度 `ButtonHeight / InputHeight = 30`、`RowHeight = 32`，全部通过 `SetAccessibilityPrefs(textScale, reduceMotion)` 动态响应设置中心的字体缩放滑块。
- **三档圆角统一**：`ControlRadius (6)`（全局基准 `t.Radius = float32(ControlRadius)`）、`PanelRadius (12)`、`DialogRadius (14)`；悬浮胶囊与圆形图标按钮使用 `height / 2`。
- **Anti-XP-Box（去重框）与实底 Dark 弹窗**：正文与属性内联输入禁止白底圆角框套娃，统一使用 `asterdoc.BorderlessTextInput`、`StyleSelectQuiet` 与科学显示单位映射 `DisplayUnit`（`°C`、`μL`、`μm`、`μM`、`×g`）；模态弹窗在 Dark 模式下强制铺设高对比度实底 `tok.Surface` + `tok.BorderStrong` 防止底层文本透射。

## 四档 Oklab 物理动效系统

实现位于 [motion.go](../../prototypes/mygo-run/internal/theme/motion.go)，所有曲线在 `Oklab` 色彩空间插值并自动响应系统 `ReduceMotion`：

| Token / 辅助器 | 曲线参数 | 典型使用场景 |
|---|---|---|
| `MotionMicro` | `100ms EaseOut` + `Oklab` | 按压微缩 `0.97`（`ApplyTactilePress`）、悬浮工具渐显 |
| `MotionSnappy` | 临界阻尼弹簧 `160ms, Bounce: 0` | 磁吸共享滑块（`SharedPillTransition`）、孔位染色、选区浮条托出（`FloatingBarTransition`） |
| `MotionFluid` | 柔和弹簧 `220ms, Bounce: 0.04` | 手风琴折叠（`AccordionTransition`）、四向分屏预览（`LiquidSplitTransition`）、移动端底浮岛显隐 |
| `MotionHero` | 空间弹簧 `260ms, Bounce: 0.03` | `HeroFlyTransition` 图表与孔板灯箱一镜到底放大 |
| `ApplyNudge` | 欠阻尼横向脉冲 `±5px` | 空提交或非法参数输入轻晃提示 |
