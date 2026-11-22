# AsterLab Handbook

AsterLab 是一个面向生物与湿实验研发的**本地单真源（Local-First Single Source of Truth）**实验工作台。它将科研数据的可追溯性与无摩擦的连续书写体验统一在同一份 `.asterml` 文件之上，同时服务于桌面端复杂编排、实验台平板/手机快速记录以及 Agent 结构化协作。

## 核心设计理念

- **单文件即工作区（UnifiedLive + Obsidian Live Preview）**：实验不再被拆散在孤立的「设计库」与「记录表单」中。一份 `.asterml` 文档打开即纯白纸，在「编辑（Obsidian 式 Live Preview + `/` 命令）」、「实验模式（防误触执行盾：锁原文与结构、开放预留槽位与空白补记）」、「源码」三种视图间共享同一底层事实。
- **极简行内科学关联（四方同源）**：普通图片保持 `![[img.png]]` 零负担；需要科学归属时使用零冲突行内语法 `![[assets/img.png]]{@plate-12:A1 6h fov=1 role=obs}`，由同一套 AST 驱动 Live Preview 胶囊、悬浮上下文卡、孔板反向查找与 Agent 读写理解。
- **通用原语与统一命令系统（`astercmd`）**：无论 CCK-8、RT-qPCR、HUVEC 还是抑菌圈，全部由 11 个通用内容模块、1 个通用容器 `plate@1` 与 10 个第二层科学原语组合表达；人类（`/`、`+`、右键、移动端）与 Agent（`cmd/asterlab`）共用同一套 `internal/astercmd` 语义命令总线。
- **纯 Go Vault 统一核心**：桌面端、移动端与 `cmd/asterlab` CLI 共用纯 Go 引擎（`internal/vault` + `internal/asterdoc` + `internal/astercmd` + `internal/astercore`），直接以 `.asterml` 文件系统为单一事实源，零 `Runs.dll` / SQLite 依赖即可运行读写命令、ARRIVE 诊断、CCK-8 / RT-qPCR 统计与全库双向检索；原 Rust Core（`crates/` / `modules/runs`）已正式退场，不再参与构建与运行。

## 手册导航

| 模块 | 阅读入口 | 核心覆盖内容 |
|---|---|---|
| **Start** | [领域模型与科学红线](./handbook/domain-model.md) | `Design` / `Protocol` / `Run` 三对象边界、`UnifiedLive` 与科学统计学红线 |
| **Architecture** | [分层架构与代码包](./handbook/architecture.md) · [Vault 存储](./handbook/vault-storage.md) · [移动端与双端同步](./handbook/mobile-and-sync.md) | `.asterml` 单真源与 `astercore` 架构决策、增量索引缓存、事件总线、快照恢复与双端同步 |
| **AsterML SDK** | [AsterML 语法与 Parity](./handbook/asterml-spec.md) · [内容模块 (11)](./handbook/modules-content.md) · [孔板、原语与 PDF](./handbook/modules-labware-and-pdf.md) | 21 个注册模块规范、UCUM 单位显示映射、A4 执行单 PDF 导出与全组件实图 |
| **UI & UX** | [Token、色盘与动效](./handbook/design-system.md) · [Liquid Glass 材质](./handbook/liquid-glass.md) · [桌面端工作台](./handbook/desktop-workbench.md) · [移动端单手工作台](./handbook/mobile-workspace.md) | 三套色盘、Oklab 动效、全局全文搜索、`关联与诊断` 抽屉、AsterCore 汇总卡片与移动端底岛 |
| **Reference** | [Pure-Go CLI 与深度链接](./handbook/ffi-and-cli.md) · [校验门禁与优化热点](./handbook/verification-and-backlog.md) | `cmd/asterlab` Vault CLI 退出码、`asterlab://` 路由、自动化巡检命令与视觉回归实图 |

> [!NOTE]
> 本手册各章节直接以当前可编译源码与自动视觉回归截图为唯一依据，剔除了历史阶段记录与过期审计日志。任何 UI 视觉修改与接口演进，均应先在本手册定位对应规范后再改动代码。
