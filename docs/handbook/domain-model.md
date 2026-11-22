# 领域模型与科学红线

AsterLab 在概念层区分「日常书写与快速实验记录（UnifiedLive）」与「受控冻存与不可变审计（Frozen Core）」两层工作方式，但二者共用同一套科学对象边界。

## 三层科学对象模型

所有严谨实验流程由以下三个不可混为一谈的核心对象构成：

1. **Design（`DesignVersion`）**
   - 回答「**比较什么、为什么这样做**」。
   - 承载研究目的、实验因子（因素与水平）、分组对照角色（`control` / `treatment` / `comparator` / `reference`）、独立样本单位声明与待检指标。
2. **Protocol（`ProtocolVersion` 与 `ProtocolBlockVersion`）**
   - 回答「**按什么步骤与操作参数执行**」。
   - 由可独立复用的步骤块与操作模块构成（无环引用或自包含内联块）。
3. **Run（不可变执行记录）**
   - 回答「**某一次真实执行中发生了什么**」。
   - Run 在创建时锁定具体的 `DesignVersion` 与 `ProtocolVersion`，内部记录 `PlannedTask`、`StepOccurrence`（同一定义多次发生不改写原始定义）及追加式 `ExecutionEvent`（操作完成、试剂扣减、实测偏离、读数写入）。

## 不可逾越的科学红线

下列约束同时在文档 Schema、Go 解析器（`internal/asterdoc`）与 `internal/astercore` 纯 Go 诊断核心中强制执行：

- **Run 数量不等于独立样本量 `n`**：同一方案重复运行 3 次，除非显式声明独立的生物重复单位，否则不能自动汇总为 `n = 3`。
- **技术重复孔数不推导样本数**：96 孔板中每组铺设 6 个复孔仅代表技术重复（`replicates`），必须与独立的 `sample_id` 显式解耦。
- **只读投影不得假造缺失数据**：`CurrentDesignMap`、`ProtocolPreview`、`RunWorkspace` 在字段缺失时只能返回明确占位或诊断，禁止通过默认值静默填补科研参数。
- **模板与实验实例分离**：保存为模板（`templates/*.asterml`）时必须执行 `ClearExecutionDataForTemplate`，剥离日期、实测值与勾选态；从模板新建实验时再注入具体日程与实验编号。

## UnifiedLive 单文件工作流

在日常研发中，研究人员直接编辑本地 Vault 目录下的 `.asterml` 文件，无需重型文档冻结锁：

- **白纸起点与自由混排**：新建实验直接进入纯白纸 Obsidian Live Preview，可自由记录实验思路，按需用 `/` 或 `+` 插入顶层共享分组/孔板或 `step@1` 执行步骤（提供 1 空白 + 4 轻量预设：`常规操作`、`计时/循环`、`测量采集`、`结果整理`）。
- **三态防误触切换（`Ctrl/Cmd+E`）**：
  - **编辑模式（`ModeEdit`）**：Obsidian 式 Live Preview（光标行露源码、非光标行渲染标题/粗体/行内复选框/关联胶囊），自由增删模块与调整设计。
  - **实验模式（`ModeLab`）**：湿实验台边防误触执行盾——**锁定既有正文与设计结构**，仅开放预留执行槽位（`checklist`、行内 `- [ ]`、`timer`、`param_table.actual`、`data_table` 读数、孔板异常孔标记与拍图关联），并在文末开放**实验空白补记区**支持现场自由记录与贴图关联。
  - **源码模式（`ModeSource`）**：完整原始 `.asterml` 代码视图。
- **显式关联才成事实**：纯 `![[img.png]]` 只是自由插图；只有用户或 Agent 显式写入 `{@plate-12:A1 6h fov=1}` 后才建立双向可校验科学关系；不从文件名或正文猜测真值，新插入块亦不默认填充假 `n=3`、`100 uL` 或 `A1`。
- **向后兼容扁平化**：加载历史三段式包络（Design / Execution / Record）时，`live_doc.go` 会自动调用 `FlattenEnvelopeToSingleSource` 透明拍平为单源正文且不丢失数据。
