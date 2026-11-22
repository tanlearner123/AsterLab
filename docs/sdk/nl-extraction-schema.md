# AsterLab 自然语言 → 组件抽取 Schema 与未来扩展兼容规范

本文档定义 AsterLab 端侧语义路由（`Jev` / `GLiClass` / `GLiNER2`）与确定性 Go 展开器（`ExpandCompactSlot`）之间的唯一契约。权威机器可读定义位于 [`prototypes/mygo-run/internal/asterdoc/spec/nl_schemas.json`](../../prototypes/mygo-run/internal/asterdoc/spec/nl_schemas.json)。

## 1. 四条不可妥协的兼容铁律

1. **铁律 1 — 双层 Schema 严格解耦（AI 槽位层 ≠ 存储契约层）**
   - 底层存储契约 [`spec/modules.json`](../../prototypes/mygo-run/internal/asterdoc/spec/modules.json) 面向运行态、审计与跨端校验（完整包含 `id`、`schema`、`groups` 孔位坐标矩阵、LaTeX 公式等）。
   - AI 抽取层只面向 `spec/nl_schemas.json` 中 20–35 token 的**极简扁平原子槽位**；所有算术、单位换算（`ug/mL ↔ ng/mL`）、孔板坐标排布（`A1..C4`）和 `specValidator` 强校验全部由 Go 的 `ExpandCompactSlot()` 确定性完成。
   - **兼容性收益**：底层 `modules.json` 演进存储字段时，只要原子槽位不变，**已微调的端侧模型零重训**。

2. **铁律 2 — 原子槽位原语封闭集（仅允许 5 种）**
   - 现有与未来所有组件的 `slots[]` **只能**使用以下 5 种原子原语：
     - `text`：短文本名称/提示（如试剂名、阶段名、采图说明）
     - `quantity`：`[数值, 单位]`（如 `[5, "min"]`、`[100, "ug/mL"]`）
     - `count`：正整数计数（如循环轮数 `3`、复孔数 `3`、视野数 `6`）
     - `enum`：有限离散选项（如孔板规格 `4|6|12|24|48|96|384`、组角色 `treatment|control|blank`）
     - `repeated_tuple`：最多两层的短元组数组（如阶段列表 `[[名称, 数值, 单位], ...]`、分组列表 `[[组名, 复孔数, 角色], ...]`）
   - **兼容性收益**：禁止任意深层嵌套对象，确保 `GLiNER2` 边界抽取、GBNF 约束解码与 Go 解析器永远共享统一语法，新增组件不会破坏模型能力。

3. **铁律 3 — Trigger Descriptor 三件套 + 点号层级 ID（零重训接入新组件）**
   - 所有可推荐目标采用扁平点号命名空间 **`<kind>.<mode>`**（如 `timer.cycle`、`calculator.dilution`、`plate.layout`、`attachment.grid`），原生兼容 `GLiClass` 层级标签、`GLiNER2` 动态 Schema 与 `Jev` `Choice(criteria=...)`。
   - 每个目标必须同时声明：
     - `when_to_suggest`：何时产生**即时台面工具回报**；
     - `when_not_to_suggest`：明确**禁止误触发的反例红线**；
     - `slots[]` + `example_zh` / `example_en` / `example_json`。
   - 模型在训练/蒸馏时必须开启 **Schema-Conditioned + 选项随机打乱（Option Shuffling）**，学习“对照描述做判断”的元能力，而非死背固定类别编号。

4. **铁律 4 — 推荐空间克制红线（`non_suggestible_kinds`）**
   - 仅当转换为活组件能**当场帮科研人算量、盯时间、排孔板或建采图网格**时，才允许进入 `nl_schemas.json` 推荐空间。
   - `prose`、`figure`、`subflow`、`data_table`、`checklist`、`step`、`workflow` 显式列入 `non_suggestible_kinds`，避免未来组件增多后稀释路由精度或制造误弹窗噪音。

## 2. 未来新增组件接入清单（无需重训即可上线）

未来若通过 SDK 新增第 N 个具备即时台面工具回报的组件（例如 `calculator.serial_dilution` 梯度稀释）：
1. 在 `spec/nl_schemas.json` 的 `targets[]` 新增一条记录（写齐 `id`、`when_to_suggest`、`when_not_to_suggest`、仅用 5 种原子原语的 `slots[]` 与示例）；
2. 在 Go `ExpandCompactSlot()` 增加对应的纯函数展开分支（保证输出通过 `specValidator`）；
3. 端侧 `GLiClass` / `GLiNER2` / `Jev` 自动读取最新 `nl_schemas.json` 动态触发描述即可 **Zero-Shot 生效**；仅当需要把新组件的准确率从 ~80% 刷到 96%+ 时，再补充 ~300 条样本做 5 分钟增量蒸馏导出新 `.onnx`。
