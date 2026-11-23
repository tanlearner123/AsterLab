package main

import (
	"log"
	"strings"
	"time"

	"asterlab-mygo-prototype/internal/shell"
)

// openOrSeedUserGuideDoc opens (or creates once) an Obsidian-style interactive
// user guide inside the user's Vault so wet-lab scientists can read concrete
// examples and immediately click, drag, right-click, and paste on live widgets.
func (a *app) openOrSeedUserGuideDoc() {
	if a.vault == nil {
		return
	}
	if a.shell != nil {
		for _, f := range a.shell.VaultFiles {
			if strings.Contains(f.Title, "新手入门指南") || strings.Contains(f.RelPath, "新手入门指南") {
				a.openVaultFileInActivePane(f.RelPath, f.Title)
				a.shell.SetActiveTabMode(shell.ModeEdit)
				return
			}
		}
	}

	today := time.Now().Format("2006-01-02")
	guideSrc := "---\n" +
		"title: 新手入门指南（可动手试玩）\n" +
		"scheduled_date: " + today + "\n" +
		"stage: in_progress\n" +
		"tags:\n  - 新手指南\n" +
		"---\n\n" +
		"欢迎来到 **AsterLab**！这份文档既是说明手册，也是你的「试验田」——你可以放心在下面拖拽孔板、修改数值、右键试菜单、甚至粘贴 Excel 数据，随时可删可改。\n\n" +
		"## 1. 常用快捷输入示例（不会也没关系，点 `+` 全都有）\n\n" +
		"- 输入 `/`：弹出工具插入菜单（插入步骤、分组表、孔板、计算器、计时器、数据表）\n" +
		"- 输入 `#@ 步骤名`：自动生成带编号与导轨的主步骤；`##@ 子步骤` 生成缩进子步骤\n" +
		"- 输入 `- [ ] 内容`：生成可勾选的实验待办框（例如下面这两条，现在就可以直接点击打勾）：\n" +
		"- [ ] 试着点一下我前面的复选框\n" +
		"- [ ] 试着点右上角「实验」切到防误触模式，再切回「编辑」\n" +
		"- 输入 `**重点**` 加粗文字，输入 `[[另一份实验]]` 链接到你库里的其他实验档案\n\n" +
		"## 2. 顶部共享蓝图：实验分组 + 12 孔板（动手试试）\n\n" +
		"- 本示例试剂清单：VEGF 母液 100 ug/mL（-80°C 保存）\n\n" +
		"```asterlab\n" +
		"kind: group_table\n" +
		"id: gt-guide\n" +
		"schema: 1\n" +
		"title: 示例实验分组\n" +
		"rows:\n" +
		"  - id: g_ctrl\n" +
		"    name: 对照组\n" +
		"    role: control\n" +
		"    condition: 无处理\n" +
		"    independent_n: 3\n" +
		"  - id: g_low\n" +
		"    name: 低剂量组\n" +
		"    role: treatment\n" +
		"    condition: 10 ng/mL\n" +
		"    independent_n: 3\n" +
		"  - id: g_high\n" +
		"    name: 高剂量组\n" +
		"    role: treatment\n" +
		"    condition: 50 ng/mL\n" +
		"    independent_n: 3\n" +
		"  - id: g_pbs\n" +
		"    name: PBS 空白\n" +
		"    role: control\n" +
		"    condition: 防蒸发\n" +
		"    independent_n: 3\n" +
		"```\n\n" +
		"试试下面这块 12 孔板：**拖选几个孔**后直接点下方分组色块或「空白(PBS)」「异常孔」；再试试**右键点击孔板**，体验「按行横排」「按列竖排（8道排枪）」「外周留空防蒸发」与导出图片：\n\n" +
		"```asterlab\n" +
		"kind: plate\n" +
		"id: plate-guide\n" +
		"schema: 1\n" +
		"title: 12 孔板（试试拖选和右键菜单）\n" +
		"format: 12\n" +
		"groups:\n" +
		"  g_ctrl: [A1, A2, A3]\n" +
		"  g_low: [B1, B2, B3]\n" +
		"  g_high: [C1, C2, C3]\n" +
		"  g_pbs: [A4, B4, C4]\n" +
		"roles:\n" +
		"  A4: blank\n" +
		"  B4: blank\n" +
		"  C4: blank\n" +
		"```\n\n" +
		"#@ 3. 动手体验联动计算器与计时器 {#t0}\n\n" +
		"下面这个预混液计算器已经自动联动上方孔板（9 个有效实验孔，已自动剔除第 4 列 PBS 空白孔）。试着改一下「单孔体积」，或在孔板上把某个孔标成「异常孔」，观察这里的孔数与总体积实时同步变化：\n\n" +
		"```asterlab\n" +
		"kind: calculator\n" +
		"id: calc-guide-mm\n" +
		"schema: 1\n" +
		"title: 孔板预混液计算（已自动联动孔板有效孔）\n" +
		"mode: formula\n" +
		"rows:\n" +
		"  - key: N\n" +
		"    label: 有效实验孔数\n" +
		"    kind: input\n" +
		"    value: \"@plate.sample_wells\"\n" +
		"    unit: 孔\n" +
		"  - key: V_0\n" +
		"    label: 单孔体积\n" +
		"    kind: input\n" +
		"    value: 500\n" +
		"    unit: uL\n" +
		"  - key: r\n" +
		"    label: 富余比例 (20%)\n" +
		"    kind: input\n" +
		"    value: 0.2\n" +
		"    unit: \"\"\n" +
		"  - key: V_tot\n" +
		"    label: 预混液总体积\n" +
		"    kind: formula\n" +
		"    formula: 'N \\times V_{0} \\times (1 + r)'\n" +
		"    unit: uL\n" +
		"```\n\n" +
		"```asterlab\n" +
		"kind: timer\n" +
		"id: timer-guide-1\n" +
		"schema: 1\n" +
		"title: 示例孵育倒计时（试试点击启动）\n" +
		"mode: countdown\n" +
		"status: idle\n" +
		"duration:\n" +
		"  value: 15\n" +
		"  unit: min\n" +
		"```\n\n" +
		"#@ 4. 试试把普通实验文字一键变成组件\n\n" +
		"把鼠标点到下面任意一行，然后**点击行尾微亮的 `✦` 图标**，或者**按住 `Alt` 键把鼠标悬停在句子上**（也可以按 `Alt + Enter`）：\n" +
		"- 将 VEGF 母液稀释至 50 ng/mL 工作液，终体积 5 mL\n" +
		"- 80°C 水浴 5 min 与冰上 10 min 交替 3 轮至完全澄清\n" +
		"- 37°C 避光孵育 30 min\n\n" +
		"#@ 5. 试试从 Excel / 酶标仪直接粘贴读数\n\n" +
		"点进下方表格任意单元格，直接按 `Ctrl + V` 就能粘贴多行多列数据；点右上角 `···` 还能切换成「孔板矩阵视图」：\n\n" +
		"```asterlab\n" +
		"kind: data_table\n" +
		"id: dt-guide\n" +
		"schema: 1\n" +
		"title: 示例读数表（支持直接 Ctrl+V 粘贴）\n" +
		"container_ref: plate-guide\n" +
		"columns:\n" +
		"  - key: well\n" +
		"    label: 孔位\n" +
		"    value_kind: text\n" +
		"  - key: value\n" +
		"    label: 测定读数\n" +
		"    value_kind: number\n" +
		"rows:\n" +
		"  - well: A1\n" +
		"    value: 1.02\n" +
		"  - well: A2\n" +
		"    value: 0.98\n" +
		"  - well: B1\n" +
		"    value: 0.65\n" +
		"  - well: B2\n" +
		"    value: 0.68\n" +
		"  - well: A4\n" +
		"    value: 0.05\n" +
		"```\n\n" +
		"## 6. 各组件右键菜单 & `···` 功能速查\n\n" +
		"- **孔板右键**：按行横排、按列竖排（8 道排枪）、外周留空防蒸发（PBS）、复制/粘贴选区、标记异常孔、关联照片、导出 PNG/CSV\n" +
		"- **计算器**：顶栏切换 9 套高频配液公式库、`+ 引入孔板有效孔`、`+ 引用上游计算`（支持自动换算 µL↔mL），点击公式可展开编辑或复制\n" +
		"- **数据表**：右键任意行插入/删除行；右上角 `···` 切换普通列表/孔板矩阵视图、从剪贴板粘贴、绑定分组或孔板；点列标题改单位或设公式列\n" +
		"- **图片卡片**：点击放大查看；点「关联」绑定具体孔位、时间点、视野编号与类型（观察图 / 原始数据 / 参考图）\n" +
		"- **文档右上角 `···`**：导出 PDF 执行单、另存为模板、创建副本\n\n" +
		"## 7. 常用快捷键\n\n" +
		"- `Ctrl + K`：快速跳转实验或功能\n" +
		"- `Ctrl + Shift + F`：全实验库搜索关键词\n" +
		"- `Ctrl + E`：切换「编辑」与「实验（防误触）」模式\n" +
		"- `Alt + 悬停` / `Alt + Enter`：把操作句一键转为交互组件\n" +
		"- `Ctrl + V`：在数据表直接粘贴 Excel / 酶标仪矩阵；在正文直接粘贴截图\n" +
		"- `Ctrl + Z` / `Ctrl + Y`：撤销 / 重做；`F2` 重命名文件\n"

	entry, err := a.vault.CreateExperiment("", "新手入门指南（可动手试玩）", today, guideSrc)
	if err != nil {
		log.Printf("seed user guide failed: %v", err)
		return
	}
	a.refreshVaultTreeAndCalendar()
	a.openVaultFileInActivePane(entry.RelPath, entry.Title)
	if a.shell != nil {
		a.shell.SetActiveTabMode(shell.ModeEdit)
	}
}
