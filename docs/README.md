# AsterLab 文档地图（按模块导航）

本文件解决“哪份文档当前具有权威”以及“按模块去哪里找规范”。研究笔记不是规范；被权威文档引用后才进入当前设计。

## 1. 两套手册边界（公开用户指南 vs 仓库工程手册）

| 手册体系 | 面向读者与发布位置 | 入口文档 |
|---|---|---|
| **用户帮助手册（`docs/guide/`）** | **面向科研工作者 · 部署上线至 `docs.asterlab.party`**（Obsidian Help 风格，零开发/设计语言术语，专注举例、快捷键与右键菜单） | [`guide/index.md`](./guide/index.md) · [`guide/quickstart.md`](./guide/quickstart.md) · [`guide/menus-and-faq.md`](./guide/menus-and-faq.md) |
| **工程与架构手册（`docs/HANDBOOK.md` + `docs/handbook/`）** | **面向项目维护者与开发者 · 保留在仓库内部查阅**（涵盖架构包、SciGraph/scimath/ONNX/updater 源码细节、AsterML SDK 与设计系统） | [`HANDBOOK.md`](./HANDBOOK.md) · [`handbook/install-and-update.md`](./handbook/install-and-update.md) · [`handbook/domain-model.md`](./handbook/domain-model.md) |

## 2. 架构、存储与科学语义引擎（Core & AI）

| 文档 | 覆盖范围 |
|---|---|
| [`handbook/architecture.md`](./handbook/architecture.md) | Pure-Go Vault + `astercmd` + `astercore` + `nlengine` + `updater` 全景分层与代码包索引 |
| [`handbook/scigraph-and-ai.md`](./handbook/scigraph-and-ai.md) | **SciGraph 科学网络、离线 AI 路由与孔板排版引擎**：`SciUnit` 全链消费、跨计算器物理量纲换算、未分配空板与已录数据锁、确定性孔板引擎与心智分工 |
| [`sdk/scigraph-nl-architecture.md`](./sdk/scigraph-nl-architecture.md) | **ADR：SciGraph 与端侧轻模型协同架构决策**（决策 1–9 详细工程依据） |
| [`sdk/nl-router-upgrade.md`](./sdk/nl-router-upgrade.md) | **NL-Router 模型包契约与热升级指南**（Bundle v1、Zero-Shot 边界与本地重训导出命令） |
| [`adr/0002-asterml-vault-pure-go-core.md`](./adr/0002-asterml-vault-pure-go-core.md) | **ADR-0002 架构决策**：`.asterml` 本地 Vault 单真源与 Pure-Go 统一核心 |
| [`handbook/vault-storage.md`](./handbook/vault-storage.md) · [`handbook/mobile-and-sync.md`](./handbook/mobile-and-sync.md) | Vault 增量索引、快照恢复、双端同步与移动端边界 |
| [`architecture.md`](./architecture.md) | 仓库级模块依赖与历史演进对照（Slint 节为历史） |

## 3. AsterML SDK、领域契约与 Agent 接口

| 文档 | 覆盖范围 |
|---|---|
| [`asterdoc-contract.md`](./asterdoc-contract.md) · [`sdk/README.md`](./sdk/README.md) | AsterML v2 统一模块契约（21 个通用模块组合铁律）与 SDK 校验 |
| [`handbook/asterml-spec.md`](./handbook/asterml-spec.md) · [`handbook/modules-content.md`](./handbook/modules-content.md) · [`handbook/modules-labware-and-pdf.md`](./handbook/modules-labware-and-pdf.md) | AsterML 语法、11 种通用内容模块、容器孔板/原语与 PDF 执行单 |
| [`../CONTEXT.md`](../CONTEXT.md) | 领域术语、对象边界、读模型/执行身份、工具/仪器边界 |
| [`../AGENT_API.md`](../AGENT_API.md) · [`handbook/ffi-and-cli.md`](./handbook/ffi-and-cli.md) | Pure-Go Vault CLI（`cmd/asterlab`）命令、退出码与 `asterlab://` 深度链接 |
| [`contracts/core-v1.md`](./contracts/core-v1.md) | 核心查询/命令/身份契约（同目录含 `operation-spec-v1/v2.md`、`readout-schema-v1.md`、`record-closure.md`、`time-plan-closure.md`） |

## 4. UI / UX 设计规范与工作台

| 文档 | 覆盖范围 |
|---|---|
| [`product/ux-contract.md`](./product/ux-contract.md) | 信息架构、UX 规则与布局契约 |
| [`product/design-system.md`](./product/design-system.md) · [`../DESIGN.md`](../DESIGN.md) | 唯一 Token 源、4 倍数间距、组件对照表与科学语义色盘 |
| [`handbook/design-system.md`](./handbook/design-system.md) · [`handbook/liquid-glass.md`](./handbook/liquid-glass.md) | 主题色盘、Liquid Glass 材质与物理动效 |
| [`handbook/desktop-workbench.md`](./handbook/desktop-workbench.md) · [`handbook/mobile-workspace.md`](./handbook/mobile-workspace.md) | 桌面端分屏工作台、实验模式（ModeLab）与移动端单手底岛 |

## 5. 工程治理、测试门禁与发布

| 文档 | 覆盖范围 |
|---|---|
| [`release/beta-scope.md`](./release/beta-scope.md) · [`release/doc-authority.md`](./release/doc-authority.md) | Beta 交付范围、发布 DoD 与文档权威登记 |
| [`product/ownership.md`](./product/ownership.md) | 文件所有权与并发开发泳道约束 |
| [`handbook/verification-and-backlog.md`](./handbook/verification-and-backlog.md) | 自动化门禁、视觉回归与已关闭优化热点 |
| [`engineering/cross-platform-builds.md`](./engineering/cross-platform-builds.md) | 跨平台构建与 Windows Gate 执行手册 |

## 归档与研究分组

- 早期 Slint / Rune / Web 时代规划与历史根目录文档已统一归档至 [`archive/README.md`](archive/README.md)。
- 历史研究笔记保留在 [`research/README.md`](research/README.md)，未被权威文档引用前不进入当前设计。

## 状态标记

研究文档应在标题下保留其状态（研究笔记 / 候选 / 已实现 / 已取代）。没有状态的历史文档，默认按“历史研究”读取，不当作当前规范。
