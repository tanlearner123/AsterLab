# Pure-Go CLI 与深度链接契约

Agent 与外部自动化脚本直接通过 **Pure-Go Vault-Native CLI（`cmd/asterlab`）** 读写并校验本地 `.asterml` Vault，产品主运行时已彻底弃用 `Runs.dll` / SQLite 与 Rust CLI；`asterlab://` 深度链接支持直接定位实验文件、步骤与储藏室试剂。

## 1. Pure-Go Vault-Native CLI (`cmd/asterlab`)

源码位于 [cmd/asterlab/main.go](../../prototypes/mygo-run/cmd/asterlab/main.go)，通过 `--vault <dir>` 指定实验室目录（默认读取 `vault.DefaultVaultDir()` / `ASTERLAB_VAULT_DIR`）：

```powershell
# 列出可用工具清单 & 人类/Agent 共用的 Semantic Command 注册表
go run ./cmd/asterlab tools --json
go run ./cmd/asterlab cmd.list
go run ./cmd/asterlab cmd.describe plate.assign_wells

# 按状态/标签筛选 Vault 中的实验列表 & 全库全文搜索
go run ./cmd/asterlab --vault C:\LabVault vault.list --status in_progress --tag cck8
go run ./cmd/asterlab --vault C:\LabVault vault.search "OD450"

# 读取实验 Frontmatter、稳定对象 ID 列表、统一行内科学关联（含孔板/组别上下文）及校验诊断
go run ./cmd/asterlab --vault C:\LabVault asterdoc.read 2026/huvec.asterml
go run ./cmd/asterlab --vault C:\LabVault asterdoc.objects 2026/huvec.asterml
go run ./cmd/asterlab --vault C:\LabVault asterdoc.relations 2026/huvec.asterml
go run ./cmd/asterlab --vault C:\LabVault asterdoc.validate 2026/huvec.asterml

# 预览 & 原子执行批量 Semantic Commands（带 expected_hash 并发检查、client_ref 批内引用、idempotency_key 幂等重放）
go run ./cmd/asterlab --vault C:\LabVault asterdoc.preview 2026/huvec.asterml --ops '{"expected_hash":"sha256:...","operations":[...]}'
go run ./cmd/asterlab --vault C:\LabVault asterdoc.exec 2026/huvec.asterml --ops '{"expected_hash":"sha256:...","idempotency_key":"op-1","operations":[...]}'

# 运行 Pure-Go AsterCore 分级科学诊断（ARRIVE 2.0 / 对照组 / 浓度越界）与确定性统计分析
go run ./cmd/asterlab --vault C:\LabVault asterdoc.assess 2026/cck8.asterml
go run ./cmd/asterlab --vault C:\LabVault asterdoc.analyze 2026/cck8.asterml
```

- **统一读写闭环契约**：Agent 通过 `cmd.list` 发现命令、`asterdoc.objects` / `asterdoc.relations` 读取对象稳定 ID 与 `docHash`、`asterdoc.preview` 预演 diff、`asterdoc.exec` 原子提交（校验 `expected_hash` 防止覆盖人工编辑，且支持同一批内用 `client_ref` 连接新建分组、孔板、孔位与图片关联），全程无需手写 YAML 围栏。
- **单行 JSON stdout 契约**：所有子命令向标准输出打印单行结构化 JSON（包含 `ok` 及对应结果或 `code`/`error`）。
- **固定退出码语义**：
  - `0`：成功（对 `asterdoc.assess` 表示无 `blocking` 阻断问题）
  - `2`：参数或子命令用法错误
  - `3`：Vault 目录或目标 `.asterml` 文件不存在
  - `4`：阻断或并发冲突（`asterdoc.assess` 检测到 `SeverityBlocking`，或 `asterdoc.exec` 检测到 `CONFLICT` 哈希冲突）
  - `5`：`.asterml` 语法解析失败、关系校验失败或命令参数非法

## 2. `asterlab://` URI 深度链接路由

解析实现位于 [deeplink.go](../../prototypes/mygo-run/internal/state/deeplink.go) 的 `ParseAsterLabURI`，并在 [main.go](../../prototypes/mygo-run/main.go) 单实例与启动参数中统一调度：

| URI 路由格式 | 路由动作 |
|---|---|
| `asterlab://open?file=<relPath>&step=<stepID>` | 在活动窗格打开指定 `.asterml` 文件并自动滚动聚焦至 `<stepID>` |
| `asterlab://pantry?code=<labelCode>` | 切换至 `TabPantry` 储藏室货架并高亮定位对应试剂条码 |
| `asterlab://run/<runID>` · `asterlab://run/<runID>/well/<well>` | 打开对应历史 Run 并定位孔板指定孔位 |

## 3. 已弃用的历史 Rust 路径说明（Archived）

仓库历史阶段曾存在 `modules/runs`（`Runs.dll`）与 Rust `labworkbench`（`src/cli.rs`）SQLite 存储方案，现已全面弃用并由上方的 Pure-Go `vault` + `astercore` + `cmd/asterlab` 完全替代。新功能与自动化工具一律基于 `.asterml` 与 `cmd/asterlab` 开发，不引入任何 Rust 或 SQLite 依赖。
