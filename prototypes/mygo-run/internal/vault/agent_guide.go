package vault

import (
	"os"
	"path/filepath"

	"asterlab-mygo-prototype/internal/asterdoc"
)

const AgentGuideFileName = "AGENTS.md"

// DefaultVaultAgentGuide is seeded once into new Vault roots (never overwriting
// existing user edits) so any zero-context AI Agent (Claude Code, Cursor, Codex,
// Antigravity, etc.) immediately knows how to read, author, and validate `.asterml`
// experiments and inspect Vault-local Pantry inventory.
const DefaultVaultAgentGuide = `# AsterLab Vault · AI Agent 协作指南

本目录是一个 **AsterLab 本地单库（Vault）**。所有实验文档均以明文 ` + "`.asterml`" + `（Markdown + YAML 结构化科学模块）存储，桌面端每 ~1.2 秒自动监听本目录变更——外部 Agent 新增、修改或重命名 ` + "`.asterml`" + ` 文件后，AsterLab 会像 Obsidian 一样自动刷新左侧文件树、日历与已打开的标签页。

## 1. 目录结构与数据格式

- ` + "`<项目文件夹>/*.asterml`" + `：实验设计与执行单源文档（可直接用任意编辑器或 Agent 读写）。
- ` + "`templates/*.asterml`" + `：无日期复用模板（不会出现在实验日程日历中）。
- ` + "`assets/`" + `：实验图片与附件（文档内使用相对路径引用）。
- ` + "`.asterlab/`" + `：本 Vault 独立配置与**储藏室试剂台账**（随 Vault 同步与隔离）：
  - ` + "`.asterlab/pantry.jsonl`" + `：单文件 JSONL 储藏室总账（每行一个 JSON 对象，按 ` + "`type + id`" + ` 排序，云盘/Git 同步友好）：
    - ` + "`{\"type\":\"schema\",\"version\":1}`" + `
    - ` + "`{\"type\":\"box\",\"id\":\"...\",\"name\":\"冻存盒 #1\",\"kind\":\"cryobox_9x9\",\"zoneKey\":\"-80\",\"rows\":9,\"cols\":9}`" + `
    - ` + "`{\"type\":\"reagent\",\"id\":\"...\",\"labelCode\":\"PN-001\",\"name\":\"Vancomycin\",\"location\":\"-20°C\",\"purchasedOn\":\"2026-11-23\",\"shelfLifeDays\":365,\"revision\":1,\"meta\":{\"catalogNo\":\"...\",\"lotNo\":\"...\",\"boxId\":\"...\",\"boxSlot\":\"A1\"}}`" + `
  - ` + "`.asterlab/pantry-manuals/`" + `：上传的试剂说明书 PDF / COA / SDS 与自定义贴纸图片。

## 2. AsterML v2.1 编写规范

每份 ` + "`.asterml`" + ` 文件由 **YAML Frontmatter** + **标准 Markdown 正文** + **结构化科学模块**组成。

### 2.1 Frontmatter（文件头）

` + "```yaml" + `
---
title: 实验标题
scheduled_date: 2026-11-24
stage: planned
---
` + "```" + `

- ` + "`stage`" + ` 合法枚举：` + "`planned`" + `（计划中）| ` + "`in_progress`" + `（进行中）| ` + "`completed`" + `（已完成）。
- 模板文件省略 ` + "`scheduled_date`" + `。

### 2.2 顶层实验元信息与分组表

` + "````markdown" + `
` + "```asterlab" + `
kind: experiment
id: exp-01
schema: 1
title: 实验标题
objective: 一句话描述科学目标与终点指标
experimental_unit: well
` + "```" + `

` + "```asterlab" + `
kind: group_table
id: gt-groups
schema: 1
title: 实验分组
rows:
  - id: g-ctrl
    name: 阴性对照组
    role: control
    condition: 空白缓冲液
    independent_n: 3
  - id: g-treat
    name: 处理组
    role: treatment
    condition: 10 ug/mL 样品
    independent_n: 3
` + "```" + `
` + "````" + `

- ` + "`experimental_unit`" + ` 常用值：` + "`well`" + ` | ` + "`tube`" + ` | ` + "`plate`" + ` | ` + "`dish`" + ` | ` + "`sample`" + ` | ` + "`animal`" + `。
- ` + "`group_table.rows[].role`" + ` 常用值：` + "`control`" + ` | ` + "`positive_control`" + ` | ` + "`treatment`" + ` | ` + "`blank`" + `。

### 2.3 步骤与 11 种标准模块（严禁自造新 ` + "`kind`" + `）

使用 ` + "`#@ 01 步骤标题`" + ` 或 ` + "```asterlab\nkind: step\n```" + ` 组织实验步骤。在 ` + "`step.blocks[]`" + ` 中可自由组合以下 **11 种通用模块**：

| ` + "`kind`" + ` | 用途 | 核心字段 |
|---|---|---|
| ` + "`prose`" + ` | 操作说明、注意事项、观察记录 | ` + "`body`" + ` |
| ` + "`group_table`" + ` | 分组设计（对照、处理、独立重复数） | ` + "`title, rows[{id, name, role, condition, independent_n}]`" + ` |
| ` + "`param_table`" + ` | 关键参数（温度、浓度、转速、体积等） | ` + "`title, rows[{key, label, value, unit}]`" + ` |
| ` + "`plate`" + ` | 孔板 / 平皿排布映射 | ` + "`title, format (6|12|24|48|96|384), groups: {group_id: [A1, A2]}`" + ` |
| ` + "`calculator`" + ` | 稀释、配液、MasterMix 计算 | ` + "`title, formula, inputs, outputs`" + ` |
| ` + "`timer`" + ` | 孵育/离心倒计时或正计时 | ` + "`title, mode (countdown|stopwatch), status (idle), duration: {value, unit}`" + ` |
| ` + "`checklist`" + ` | 关键质控核对清单 | ` + "`title, items[{id, label, checked, blocking}]`" + ` |
| ` + "`data_table`" + ` | 计划测量表与实测读数表 | ` + "`title, columns[{key, label, value_kind, unit}], rows[...]`" + ` |
| ` + "`figure`" + ` | 示意图 / 实拍图（仅作视觉补充，不存结构化数据） | ` + "`title, src, caption`" + ` |
| ` + "`attachment`" + ` | 原始数据/质检附件链接 | ` + "`title, path, mime`" + ` |
| ` + "`subflow`" + ` | 可复用子流程引用 | ` + "`title, ref`" + ` |

每个模块都必须包含 ` + "`kind`" + `、` + "`id`" + `（文档内唯一）和 ` + "`schema: 1`" + `。

### 2.4 关键硬约束（写前必查）

1. **UCUM 合法单位**：` + "`param_table`" + ` 与 ` + "`data_table`" + ` 的 ` + "`unit`" + ` 字段必须使用标准 UCUM 记法，例如：
   - 温度：` + "`Cel`" + `（不要写 ` + "`°C`" + `）
   - 浓度：` + "`ug/mL`" + `、` + "`mg/mL`" + `、` + "`ng/mL`" + `、` + "`mmol/L`" + `、` + "`uM`" + `、` + "`mM`" + `、` + "`%`" + `
   - 体积 / 质量：` + "`uL`" + `、` + "`mL`" + `、` + "`L`" + `、` + "`ug`" + `、` + "`mg`" + `、` + "`g`" + `
   - 时间 / 长度 / 微生物 / 吸光度：` + "`s`" + `、` + "`min`" + `、` + "`h`" + `、` + "`d`" + `、` + "`mm`" + `、` + "`cm`" + `、` + "`CFU/mL`" + `、` + "`AU`" + `、` + "`rpm`" + `
   - **无量纲比例 / 倍数 / 计数**：必须写 ` + "`\"1\"`" + ` 或留空 ` + "`\"\"`" + `，**严禁**写 ` + "`\"×\"`" + `、` + "`\"倍\"`" + `、` + "`\"次\"`" + `！
2. **严禁伪造实测数据**：编写实验计划时，` + "`data_table`" + ` 只预填样本/组别/计划浓度等自变量列，**所有待测读数列必须留空（` + "`null`" + ` 或省略）**，等实验执行时由实验人员录入。
3. **不修改历史冻结记录**：除非用户明确要求，不要篡改已完成实验的原始读数。

## 3. 校验命令（CLI）

若环境已安装 ` + "`asterlab`" + ` CLI，写入或修改 ` + "`.asterml`" + ` 后建议运行校验与就绪度检查：

` + "```bash" + `
# 校验 AsterML 语法、模块 Schema 与 UCUM 单位
asterlab asterdoc.validate ./项目目录/实验计划.asterml

# 评估实验方案就绪度（检查对照、重复数、缺失参数等）
asterlab asterdoc.assess ./项目目录/实验计划.asterml

# 查看全部可用命令与结构化参数说明
asterlab cmd.list
` + "```" + `
`

// SeedAgentGuide writes AGENTS.md into the vault root if absent so external
// AI Agents have self-contained instructions without prior conversation context.
func (v *Vault) SeedAgentGuide() error {
	if v == nil || v.Root == "" {
		return nil
	}
	target := filepath.Join(v.Root, AgentGuideFileName)
	if _, err := os.Stat(target); err == nil {
		return nil
	}
	return asterdoc.WriteFileAtomic(target, []byte(DefaultVaultAgentGuide))
}
