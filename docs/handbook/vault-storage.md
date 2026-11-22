# Vault 存储、索引缓存与快照恢复

AsterLab 的本地存储完全基于普通操作系统目录（Vault），不依赖专有二进制数据库锁定，也不在 `.asterml` 文件格式内绑定软件写锁，从而保持与 Git、网盘、Obsidian 式文本工具及外部 AI Agent 的完整纯文本互操作性。

## 目录结构契约

```text
<VaultRoot>/
├── .asterlab/
│   ├── config.json         # 可跨端同步的用户偏好：主题模式/预设/LiquidGlass、热键映射、移动端手势、默认板型与余量
│   ├── local.json          # 本机私有状态（不随云盘同步）：本机 VaultPath、最近打开项目文件夹 LastProjectDir
│   ├── workspace.json      # 分屏布局、已打开页签与当前焦点面板状态
│   ├── cache/
│   │   ├── index.json      # 一次性增量缓存（全文行索引 / Backlinks / 行内科学关联 / Vault Timers / Tags）；删除可随时重建
│   │   └── cmd-journal.json # 有界命令幂等回执日志（最近 200 条），防止 Agent 重试重复创建对象；不污染 .asterml 正文
│   ├── snapshots/          # File Recovery 离线版本快照（2 分钟防抖、每文件最多保留 20 份）
│   ├── trash_meta.json     # 软删除原路径与时间戳元数据
│   └── trash/              # 回收站文件备份目录
├── assets/                 # ./assets/<file> 相对路径图片与附件（Ctrl+V / 拖拽自动存盘）
├── templates/              # 空白方案模板（由 ClearExecutionDataForTemplate 去执行态生成）
└── <ProjectFolder>/
    └── *.asterml           # 带 YAML Frontmatter 与 asterlab 围栏的标准实验文档（唯一事实源）
```

## `vault.Vault`、`EventBus` 与双链重写

实现位于 [vault.go](../../prototypes/mygo-run/internal/vault/vault.go) 与 [events.go](../../prototypes/mygo-run/internal/vault/events.go)：

- **目录扫描 (`ScanTree`)**：扫描真实目录树并按文件类型与层级排序，识别 `scheduled_date` / `stage` / `tags` 以驱动日程与索引。
- **模板实例化重映射 (`CreateExperiment`)**：从模板新建实验时调用 `asterdoc.InstantiateTemplate` 自动生成全新 `id`，避免连续新建导致模块 ID 冲突。
- **事件总线 (`EventBus`)**：对 `create` / `modify` / `rename` / `delete` 四类生命周期广播订阅事件。
- **双链与复现引用重写 (`RewriteWikiAndRepeatLinks`)**：调用 `Rename` 或 `Move` 改名/移目录时，自动全库扫描并重写所有受影响的 `[[wikilink]]` 与 Frontmatter `repeat_of` 引用。
- **软删除与 File Recovery 快照**：
  - `Trash` / `RestoreFromTrash` / `ListTrash` 管理 `.asterlab/trash/` 并在设置中心直接列出可还原项。
  - `RecordSnapshot` / `ListSnapshots` / `ReadSnapshot` 基于 SHA-256 去重与 `defaultSnapshotMinGap = 2 * time.Minute` 防抖保留离线历史快照，恢复前亦会再留存一份当前状态。

## `VaultIndex` 增量缓存核心

实现位于 [index.go](../../prototypes/mygo-run/internal/vault/index.go)：

- **增量刷新 (`Refresh`) & 即时更新 (`UpsertSource`)**：基于文件 `ModTimeUnix` 与 `Size` 做增量解析并落盘 `.asterlab/cache/index.json`；前端每次编辑保存即时刷新内存索引，无冷启动卡顿。
- **全文检索 (`SearchFullText`)**：匹配标题、目录、试剂编号 `pantry_code` 与正文行，服务桌面端 `Ctrl+Shift+F` 与 CLI `vault.search`。
- **反向链接与复现谱系 (`BacklinksFor`)**：汇总全部通过 `[[...]]` 与 `repeat_of` 指向当前实验的上下游实验。
- **行内科学关联倒排 (`RelationsFor`)**：索引所有 `![[...]]{@plate:well ...}` / `[[...]]{@...}` 与块内采图关系，支持按 `(relPath, objectID, subTarget)` 瞬时双向反查。
- **全库运行中计时器聚合 (`ActiveTimers`)**：跨所有非模板 `.asterml` 文件收集 `status: running | paused` 的 `timer@1` 块，联动底部状态栏计数与右侧伴随抽屉跳转。
- **跨实验结构化查询 (`QueryBases`)**：按目录前缀、状态、标签与关键词筛选返回标准化 `BasesRow`（`LabBases` 数据基座）。

## `DocStore` 与 `LiveDocument` 单例共享

实现位于 [live_doc.go](../../prototypes/mygo-run/internal/vault/live_doc.go)：

- **多窗格单例缓存**：`DocStore.Open(relPath)` 以规范化相对路径作为键缓存唯一的 `*LiveDocument`，同一文件在多个分屏窗格中共享内存状态。
- **统一 100 步撤销栈与输入防抖**：`ApplyEdit` 与 `ApplySourceDraft` 共享同一 `undoStack / redoStack` 环（`maxUndoStack = 100`）；源码连续打字经 `sourceUndoCoalesceDelay = 800ms` 自动合并入栈，避免每敲一字塞满撤销历史。
- **Windows 安全原子写盘 (`WriteFileAtomic`)**：同目录 `os.CreateTemp` 写入并刷盘，随后执行最多 4 次指数退避 `os.Rename` 覆盖，应对 Windows 杀毒/云盘短时文件占用。
- **保存通知钩子 (`NotifySaved`)**：稳定保存后同步触发 Vault 文件修改事件与防抖快照录制。
