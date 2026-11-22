# AsterML 语法与双端 Parity

AsterML v2 是面向人类科研书写与机器严格校验的声明式实验文档格式（`sdkVersion: "2.0.0"`），规范清单位于 [modules.json](../../prototypes/mygo-run/internal/asterdoc/spec/modules.json) 与 [units.json](../../prototypes/mygo-run/internal/asterdoc/spec/units.json)。

## 文档基本结构

每个 `.asterml` 文件由三类自由混排的内容组成（新建空白文档仅含最小 Frontmatter，不强制预置步骤）：

1. **YAML Frontmatter**：声明 `title`、`date`、`time`、`experiment_id`、`project`、`objective`、`culture_condition`、`status`、`tags` 等文档级元数据。
2. **自由 Markdown 正文、扁平步骤标题与行内关联语法**：
   - **轻量化嵌套步骤语法（`##@ 标题`）**：行首 `#{1,6}@ <步骤标题>`（可选尾缀 `{#anchor}` 或 `{optional}`）声明一个可执行实验步骤，井号数表嵌套层级（如 `##@ 配液与交联` 下挂 `###@ 热循环溶解` 与 `###@ 405 nm 光交联 {#t0}`）。由 [steps.go](../../prototypes/mygo-run/internal/asterdoc/steps.go) 单调栈解析器自动划定作用域（遇同级或更高级普通标题如 `## 实验讨论` 自动闭合，不误吸讨论区待办），向上汇总 `- [ ]` 核对进度与计时状态，驱动 Live Preview 步骤徽标、右侧嵌套大纲、`{@t0}` 关联解析与 Agent CLI；与旧文档中的 `step@1` YAML 模块无损共存。
   - **行内科学关联语法（`{...}`）**：任意图片或附件紧跟零冲突 `{...}` 属性块（Pandoc / Markdown 通用惯例），例如：
     - 普通插图（无科学关联）：`![[assets/scheme.png]]`
     - 孔位显微图：`![[assets/huvec-a1-6h.png|400]]{@plate-12:A1 6h fov=1 role=obs}`
     - 关联步骤锚点 / 一图多孔 / 原始附件：`![[assets/cure.png]]{@t0 role=obs}`、`![A1](./assets/a1.jpg){@plate-12:A1,A2 24h}`、`[[qpcr_raw.csv]]{@step-04 role=raw}`
     - 该语法由 [relations.go](../../prototypes/mygo-run/internal/asterdoc/relations.go) 统一解析，驱动 Live Preview 交互胶囊、悬浮上下文卡、孔板双向反查与 Agent CLI 理解，与 LaTeX `$...$` 及 Obsidian `|400` 零冲突。
3. **`asterlab` 结构化模块围栏**：可置于文档顶层（共享分组、共享孔板等）或 `step@1.blocks[]` 内，每个块以稳定 `id` 与 `kind` 标识。

## 三条架构铁律

所有模块开发、界面迭代与 Agent 编排必须无条件遵守三条铁律：

1. **禁止实验专用模块与专用面板**：凡能由通用内容模块与第二层科学原语组合表达的场景（如 CCK-8、RT-qPCR、Western Blot、抑菌圈、流式或动物队列），严禁新增专用 `kind` 或专用 UI 面板。
2. **`figure@1` 仅作视觉补充表达**：琼脂平皿分区、WB 泳道示意图、显微镜视野等图像仅用于直观辅助；所有分组、加药浓度、时间点与数值读数必须存储在 `group_table@1`、`param_table@1`、`plate@1` 与 `data_table@1` 中，严禁反向解析图像文本。
3. **步骤自由组合与自包含编译**：`step@1.blocks[]` 支持任意顺序内联组合内容模块或 `block_refs`；文档可直接编译并冻结为 Run，无需预先在外部注册 `ProtocolBlockVersion` UUID。

## 模块分类总表（21 个注册模块）

| 层级 | 模块列表 | 说明 |
|---|---|---|
| **容器 (`labware`)** | `plate@1` | 覆盖 `plate`（4/6/12/24/48/96/384 孔板）、`rack_1d` 管架/泳道、`dish` 培养皿与 `cohort` 分组队列 |
| **第一层通用内容块** | `prose@1`、`group_table@1`、`param_table@1`、`figure@1`、`data_table@1`、`checklist@1`、`calculator@1`、`timer@1`、`attachment@1`、`subflow@1` | 承载具体书写、参数偏离、计算求解、步骤检查、计时、二维读数与照片取证 |
| **第二层科学原语** | `experiment@1`、`group@1`、`assay@1`、`workflow@1`、`step@1`、`instrument_profile@1`、`material@1`、`sample@1`、`reference@1`、`readout@1` | 声明科学关系图、资源档案、实验步骤树与结构化结果卡片 |

## 单位校验与契约门禁

- **UCUM 受控单位集**：数值参数只接受 [units.json](../../prototypes/mygo-run/internal/asterdoc/spec/units.json) 注册的规范单位（如 `Cel`、`uM`、`ng/uL`、`cells/mL`、`mW/cm2`、`xg`），未知单位禁止隐式换算。
- **引文与跨块引用**：支持 `@groups`、`@plate.<id>`、`@param.<key>`、`@actual.<key>` 显式跨块联动，确保实验条件更改或实测偏离后下游计算自动重算。
- **Schema 与核心一致性门禁**：通过 `python scripts/sdk/check.py` 校验 `modules.json` 清单与 Go 实现（`internal/asterdoc` / `internal/astercore`）在 21 个模块、20 条规则、21 种端口类型与 6 个分类上的完全一致（仓库历史阶段遗留的 Rust crate 镜像仅作只读归档参考）。
