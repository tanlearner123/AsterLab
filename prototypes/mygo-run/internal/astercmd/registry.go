package astercmd

import (
	"fmt"
	"strings"

	"asterlab-mygo-prototype/internal/asterdoc"
)

// ExecMode distinguishes design/edit mode from anti-accidental-edit Lab Mode.
type ExecMode string

const (
	ModeEdit ExecMode = "edit"
	ModeLab  ExecMode = "lab"
)

// FieldSpec describes one typed argument of a Semantic Command.
type FieldSpec struct {
	Type        string `json:"type"` // "string" | "int" | "bool" | "string[]" | "object" | "any"
	Required    bool   `json:"required,omitempty"`
	Description string `json:"description"`
	Default     any    `json:"default,omitempty"`
}

// CommandDescriptor is the single source of truth shared by Slash menus, `+` menus,
// context menus, mobile bottom bars, and Agent CLI / SDK discovery (`cmd.list` / `cmd.describe`).
type CommandDescriptor struct {
	ID          string               `json:"id"`
	Label       string               `json:"label"`
	Category    string               `json:"category"` // "insert" | "step" | "relation" | "lab" | "edit"
	Slash       string               `json:"slash,omitempty"`
	Aliases     []string             `json:"aliases,omitempty"`
	Keywords    []string             `json:"keywords,omitempty"`
	Contexts    []string             `json:"contexts,omitempty"` // "top_level" | "step" | "any"
	AllowInLab  bool                 `json:"allowInLab"`
	Description string               `json:"description"`
	InputSchema map[string]FieldSpec `json:"inputSchema,omitempty"`
}

// StepPreset describes one lightweight step composition preset (blocks + title).
type StepPreset struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Description string   `json:"description"`
	BlockKinds  []string `json:"blockKinds"`
}

// StepPresetDefs returns the 1 blank + 4 lightweight step presets (never assay-specific kinds).
func StepPresetDefs() []StepPreset {
	return []StepPreset{
		{
			ID:          "blank",
			Label:       "空白步骤",
			Description: "仅含标题与空白内容区，可按需插入任意模块",
			BlockKinds:  nil,
		},
		{
			ID:          "standard",
			Label:       "常规操作",
			Description: "文本说明 + 参数表 + 检查清单",
			BlockKinds:  []string{"prose", "param_table", "checklist"},
		},
		{
			ID:          "timed",
			Label:       "计时 / 循环",
			Description: "计时器 + 参数表 + 检查清单",
			BlockKinds:  []string{"timer", "param_table", "checklist"},
		},
		{
			ID:          "capture",
			Label:       "测量采集",
			Description: "数据记录表 + 附件采图 + 参数表",
			BlockKinds:  []string{"data_table", "attachment", "param_table"},
		},
		{
			ID:          "analysis",
			Label:       "结果整理",
			Description: "数据记录表 + 计算器 + 文本说明",
			BlockKinds:  []string{"data_table", "calculator", "prose"},
		},
	}
}

// Registry holds all registered Semantic Commands.
type Registry struct {
	ordered []CommandDescriptor
	byID    map[string]CommandDescriptor
}

var defaultRegistry = buildDefaultRegistry()

// DefaultRegistry returns the canonical AsterLab Command Registry.
func DefaultRegistry() *Registry {
	return defaultRegistry
}

func buildDefaultRegistry() *Registry {
	cmds := []CommandDescriptor{
		{
			ID:          "doc.insert_markdown",
			Label:       "插入自由正文",
			Category:    "insert",
			Slash:       "/text",
			Aliases:     []string{"text", "markdown", "prose", "正文", "文本", "zhengwen"},
			Contexts:    []string{"top_level"},
			AllowInLab:  false,
			Description: "在文档指定位置或末尾插入普通 Markdown 正文",
			InputSchema: map[string]FieldSpec{
				"markdown":  {Type: "string", Required: true, Description: "要插入的 Markdown 文本"},
				"at_offset": {Type: "int", Description: "插入字节偏移，省略则追加到末尾"},
			},
		},
		{
			ID:          "doc.append_note",
			Label:       "追加实验空白笔记",
			Category:    "lab",
			Slash:       "/note",
			Aliases:     []string{"note", "memo", "笔记", "备注", "biji", "beizhu"},
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "在文档末尾或空白处追加现场实验笔记/偏离记录（实验模式可用）",
			InputSchema: map[string]FieldSpec{
				"text": {Type: "string", Required: true, Description: "要追加的实验笔记文本"},
			},
		},
		{
			ID:          "group.create",
			Label:       "实验分组表 / 新增组别",
			Category:    "insert",
			Slash:       "/group",
			Aliases:     []string{"group", "fenzu", "分组", "对照组", "处理组"},
			Contexts:    []string{"top_level", "step"},
			AllowInLab:  false,
			Description: "新建共享分组表，或向已有分组表追加一个有稳定 ID 的组别（不预设假样本量 n）",
			InputSchema: map[string]FieldSpec{
				"name":              {Type: "string", Description: "组别名称；省略时创建空白分组表"},
				"role":              {Type: "string", Description: "角色：control | treatment | comparator | reference"},
				"condition":         {Type: "string", Description: "处理条件说明"},
				"independent_n":     {Type: "int", Description: "每组独立实验单位数 n（仅明确已知时填写）"},
				"technical_repeats": {Type: "int", Description: "技术重复数（可选）"},
				"group_table_id":    {Type: "string", Description: "可选：向指定分组表追加行"},
				"step_id":           {Type: "string", Description: "可选：插入到指定步骤内部"},
				"at_offset":         {Type: "int", Description: "可选：顶层插入字节偏移"},
			},
		},
		{
			ID:          "plate.create",
			Label:       "孔板 / 管架 / 培养皿",
			Category:    "insert",
			Slash:       "/plate",
			Aliases:     []string{"plate", "kongban", "孔板", "96孔板", "12孔板", "培养皿", "dish"},
			Contexts:    []string{"top_level", "step"},
			AllowInLab:  false,
			Description: "插入通用孔板/培养皿/管架（默认干净空板，不预设假 A1 对照孔）",
			InputSchema: map[string]FieldSpec{
				"title":     {Type: "string", Description: "孔板标题"},
				"format":    {Type: "int", Description: "板型：4 | 6 | 12 | 24 | 48 | 96 | 384", Default: 12},
				"topology":  {Type: "string", Description: "拓扑：plate | dish | rack_1d | cohort", Default: "plate"},
				"step_id":   {Type: "string", Description: "可选：插入到指定步骤内部"},
				"at_offset": {Type: "int", Description: "可选：顶层插入字节偏移"},
			},
		},
		{
			ID:          "plate.assign_wells",
			Label:       "分配孔板孔位",
			Category:    "edit",
			Aliases:     []string{"assign", "wells", "分孔", "排板"},
			Contexts:    []string{"any"},
			AllowInLab:  false,
			Description: "将指定孔位分配给已声明的稳定组别 ID，并可选附带孔角色与正交因子",
			InputSchema: map[string]FieldSpec{
				"plate_id":  {Type: "string", Required: true, Description: "目标孔板稳定 ID（支持批内 client_ref）"},
				"group_id":  {Type: "string", Required: true, Description: "目标组别稳定 ID（支持批内 client_ref）"},
				"wells":     {Type: "string[]", Required: true, Description: "孔位列表，例如 [\"A1\", \"A2\"]"},
				"role":      {Type: "string", Description: "可选孔角色：sample | blank | vehicle | positive_control | negative_control | standard | ntc"},
				"factors":   {Type: "object", Description: "可选正交因子键值对，例如 {\"浓度\": \"10 uM\"}"},
				"sample_id": {Type: "string", Description: "可选样本 ID"},
			},
		},
		{
			ID:          "step.create",
			Label:       "实验步骤",
			Category:    "step",
			Slash:       "/step",
			Aliases:     []string{"step", "buzhou", "步骤", "操作步骤", "template", "预设"},
			Contexts:    []string{"top_level"},
			AllowInLab:  false,
			Description: "插入一个实验步骤容器，支持空白或 4 种轻量预设（blank | standard | timed | capture | analysis）",
			InputSchema: map[string]FieldSpec{
				"title":     {Type: "string", Description: "步骤标题"},
				"preset":    {Type: "string", Description: "预设：blank | standard | timed | capture | analysis", Default: "blank"},
				"at_offset": {Type: "int", Description: "可选：顶层插入字节偏移"},
			},
		},
		{
			ID:          "block.insert",
			Label:       "插入通用科学模块",
			Category:    "insert",
			Slash:       "/block",
			Aliases:     []string{"param", "table", "calc", "timer", "check", "figure", "attachment", "subflow", "参数表", "数据表", "计算器", "计时器", "检查清单", "示意图", "附件", "子流程"},
			Contexts:    []string{"top_level", "step"},
			AllowInLab:  false,
			Description: "在顶层正文或指定步骤内插入任意通用内容模块（param_table / data_table / checklist / timer / calculator / figure / attachment / prose / subflow / group_table / plate）",
			InputSchema: map[string]FieldSpec{
				"kind":      {Type: "string", Required: true, Description: "模块类型 kind"},
				"step_id":   {Type: "string", Description: "目标步骤 ID；为空则作为顶层模块插入"},
				"index":     {Type: "int", Description: "步骤内插入索引，-1 追加到末尾", Default: -1},
				"at_offset": {Type: "int", Description: "顶层插入字节偏移"},
				"title":     {Type: "string", Description: "可选标题"},
				"props":     {Type: "object", Description: "可选模块字段覆盖"},
			},
		},
		{
			ID:          "block.update",
			Label:       "更新模块属性",
			Category:    "edit",
			Contexts:    []string{"any"},
			AllowInLab:  false,
			Description: "按稳定 ID 定点更新顶层模块或步骤内子块字段",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "目标模块或块 ID"},
				"step_id":   {Type: "string", Description: "若为步骤内子块，指定所属步骤 ID（省略时自动查找）"},
				"props":     {Type: "object", Required: true, Description: "要合并更新的属性键值"},
			},
		},
		{
			ID:          "block.move",
			Label:       "移动模块顺序",
			Category:    "edit",
			Contexts:    []string{"any"},
			AllowInLab:  false,
			Description: "调整顶层模块或步骤内子块的顺序",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "目标模块 ID"},
				"step_id":   {Type: "string", Description: "步骤内子块所属步骤 ID"},
				"to_index":  {Type: "int", Description: "步骤内目标索引"},
				"delta":     {Type: "int", Description: "顶层移动方向：-1 上移，+1 下移"},
			},
		},
		{
			ID:          "block.duplicate",
			Label:       "复制模块（新 ID）",
			Category:    "edit",
			Contexts:    []string{"any"},
			AllowInLab:  false,
			Description: "复制模块并自动分配全新稳定 ID，避免身份冲突",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "要复制的模块 ID"},
				"step_id":   {Type: "string", Description: "若为步骤内子块，指定所属步骤 ID"},
			},
		},
		{
			ID:          "block.remove",
			Label:       "删除模块",
			Category:    "edit",
			Contexts:    []string{"any"},
			AllowInLab:  false,
			Description: "按稳定 ID 删除顶层模块或步骤内子块，保留周边正文",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "要删除的模块 ID"},
				"step_id":   {Type: "string", Description: "若为步骤内子块，指定所属步骤 ID"},
			},
		},
		{
			ID:          "asset.embed",
			Label:       "嵌入图片 / 附件（可选关联）",
			Category:    "relation",
			Slash:       "/image",
			Aliases:     []string{"image", "photo", "file", "tupian", "zhaopian", "图片", "照片", "插图", "附件"},
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "以极简行内语法 `![[path]]{@...}` 插入图片或附件，并可选附带孔板/孔位/时间点/FOV 显式关联（实验模式可用）",
			InputSchema: map[string]FieldSpec{
				"asset":     {Type: "string", Required: true, Description: "Vault 相对路径，如 ./assets/huvec-a1.png"},
				"width":     {Type: "string", Description: "可选 Obsidian 显示宽度，如 400"},
				"targets":   {Type: "any", Description: "可选关联目标列表，如 [\"plate-1:A1\"] 或 [{\"objectId\":\"plate-1\",\"subTargets\":[\"A1\"]}]"},
				"timepoint": {Type: "string", Description: "可选时间点，如 6h"},
				"fov":       {Type: "int", Description: "可选视野编号，如 1"},
				"role":      {Type: "string", Description: "可选角色：obs | raw | reference"},
				"note":      {Type: "string", Description: "可选简短备注"},
				"at_offset": {Type: "int", Description: "可选插入字节偏移"},
			},
		},
		{
			ID:          "relation.set",
			Label:       "建立 / 更新语义关联",
			Category:    "relation",
			Slash:       "/link",
			Aliases:     []string{"link", "relation", "guanlian", "关联", "绑定孔位"},
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "为已有图片或附件链接写入/更新规范 `{@...}` 语义关联（实验模式可用）",
			InputSchema: map[string]FieldSpec{
				"asset":     {Type: "string", Description: "目标图片/附件路径（与 span_start/span_end 二选一）"},
				"span_start":{Type: "int", Description: "目标链接起始字节偏移"},
				"span_end":  {Type: "int", Description: "目标链接结束字节偏移"},
				"targets":   {Type: "any", Required: true, Description: "目标引用列表，如 [\"plate-1:A1\"]"},
				"timepoint": {Type: "string", Description: "可选时间点"},
				"fov":       {Type: "int", Description: "可选视野号"},
				"role":      {Type: "string", Description: "可选角色"},
				"note":      {Type: "string", Description: "可选备注"},
			},
		},
		{
			ID:          "relation.remove",
			Label:       "移除语义关联",
			Category:    "relation",
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "移除图片/附件上的 `{@...}` 科学关联，保留原图本身（实验模式可用）",
			InputSchema: map[string]FieldSpec{
				"asset":      {Type: "string", Description: "目标图片/附件路径"},
				"span_start": {Type: "int", Description: "可选起始字节偏移"},
				"span_end":   {Type: "int", Description: "可选结束字节偏移"},
			},
		},
		{
			ID:          "lab.check",
			Label:       "勾选检查项（实验记录）",
			Category:    "lab",
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "在实验模式或编辑模式下更新检查清单勾选状态与备注",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "checklist 模块 ID"},
				"item_id":   {Type: "string", Required: true, Description: "检查项 ID"},
				"done":      {Type: "bool", Required: true, Description: "是否完成"},
				"note":      {Type: "string", Description: "可选执行备注"},
			},
		},
		{
			ID:          "lab.param_actual",
			Label:       "记录参数实测 / 偏离值",
			Category:    "lab",
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "在不修改计划值的前提下记录 param_table 某行的实测值 actual 与备注",
			InputSchema: map[string]FieldSpec{
				"module_id": {Type: "string", Required: true, Description: "param_table 模块 ID"},
				"key":       {Type: "string", Required: true, Description: "参数行 key"},
				"actual":    {Type: "any", Required: true, Description: "实测数值或字符串"},
				"note":      {Type: "string", Description: "可选偏离说明"},
			},
		},
		{
			ID:          "lab.plate_exclude",
			Label:       "标记 / 取消孔板异常孔",
			Category:    "lab",
			Contexts:    []string{"any"},
			AllowInLab:  true,
			Description: "在不改变实验分组设计的前提下标记或取消异常孔位",
			InputSchema: map[string]FieldSpec{
				"plate_id": {Type: "string", Required: true, Description: "孔板 ID"},
				"wells":    {Type: "string[]", Required: true, Description: "孔位列表"},
				"excluded": {Type: "bool", Required: true, Description: "true 标记异常，false 恢复正常"},
			},
		},
	}

	reg := &Registry{
		ordered: cmds,
		byID:    make(map[string]CommandDescriptor, len(cmds)),
	}
	for _, c := range cmds {
		reg.byID[c.ID] = c
	}
	return reg
}

// List returns all registered commands in stable order.
func (r *Registry) List() []CommandDescriptor {
	out := make([]CommandDescriptor, len(r.ordered))
	copy(out, r.ordered)
	return out
}

// Describe looks up one command by exact ID.
func (r *Registry) Describe(id string) (CommandDescriptor, bool) {
	c, ok := r.byID[strings.TrimSpace(id)]
	return c, ok
}

// SlashEntry represents one actionable item shown in Slash menus, `+` menus, or mobile toolbars.
type SlashEntry struct {
	CommandID   string
	Label       string
	Detail      string
	Slash       string
	Kind        string // for block.insert / group.create / plate.create
	Preset      string // for step.create
	SearchTerms string
}

// SlashEntries returns the unified insertable Slash / `+` menu entries derived from the Registry.
func (r *Registry) SlashEntries(contextName string, mode ExecMode) []SlashEntry {
	var entries []SlashEntry
	add := func(e SlashEntry, allowInLab bool) {
		if mode == ModeLab && !allowInLab {
			return
		}
		entries = append(entries, e)
	}

	if contextName != "step" {
		add(SlashEntry{CommandID: "step.create", Label: "主步骤", Detail: "新建实验阶段主步骤（快捷键 #@ ）", Slash: "/step", Preset: "blank", SearchTerms: "step buzhou 步骤 主步骤 空白步骤 #@"}, false)
		add(SlashEntry{CommandID: "step.create", Label: "子步骤", Detail: "在当前主步骤下新增子步骤（快捷键 ##@ ）", Slash: "/substep", Preset: "substep", SearchTerms: "substep zibuzhou 子步骤 分支步骤 ##@"}, false)
		for _, p := range StepPresetDefs()[1:] {
			add(SlashEntry{
				CommandID:   "step.create",
				Label:       "步骤片段 · " + p.Label,
				Detail:      p.Description + "（扁平展开）",
				Slash:       "/step-" + p.ID,
				Preset:      p.ID,
				SearchTerms: "step template yushe 预设 " + p.ID + " " + p.Label + " " + p.Description,
			}, false)
		}
	}

	add(SlashEntry{CommandID: "group.create", Label: "实验分组表", Detail: "定义共享对照组与处理组", Slash: "/group", Kind: "group_table", SearchTerms: "group fenzu 分组表 对照 处理"}, false)
	add(SlashEntry{CommandID: "plate.create", Label: "孔板 / 培养皿 / 管架", Detail: "通用容器与多图层排板", Slash: "/plate", Kind: "plate", SearchTerms: "plate kongban 孔板 96 12 培养皿 dish"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "参数表", Detail: "计划条件与实测偏离记录", Slash: "/param", Kind: "param_table", SearchTerms: "param canshu 参数表 条件"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "数据记录表", Detail: "二维读数矩阵 / CSV 粘贴", Slash: "/table", Kind: "data_table", SearchTerms: "table data shuju 数据表 读数"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "定量计算器", Detail: "稀释 / 预混液 / 公式联动", Slash: "/calc", Kind: "calculator", SearchTerms: "calc jisuan 计算器 稀释 配液"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "计时器", Detail: "倒计时 / 正计时 / 多阶段循环", Slash: "/timer", Kind: "timer", SearchTerms: "timer jishi 计时器 孵育"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "检查清单", Detail: "带必检标记的操作核对项", Slash: "/check", Kind: "checklist", SearchTerms: "check checklist qingdan 检查清单 核对"}, false)
	add(SlashEntry{CommandID: "asset.embed", Label: "插入图片并关联", Detail: "![[图]]{@孔板:孔位} 极简语法", Slash: "/image", Kind: "image", SearchTerms: "image photo tupian zhaopian 图片 照片 显微 关联"}, true)
	add(SlashEntry{CommandID: "block.insert", Label: "示意图 / SVG", Detail: "矢量示意图或本地图注", Slash: "/figure", Kind: "figure", SearchTerms: "figure svg shiyitu 示意图"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "附件与采图", Detail: "原始数据文件或孔位采图槽", Slash: "/file", Kind: "attachment", SearchTerms: "file attachment fujian 附件 采图"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "子流程", Detail: "嵌套多步细分操作", Slash: "/subflow", Kind: "subflow", SearchTerms: "subflow ziliucheng 子流程"}, false)
	add(SlashEntry{CommandID: "block.insert", Label: "文本 / 警示说明", Detail: "带提示/警示样式的说明块", Slash: "/prose", Kind: "prose", SearchTerms: "prose text shuoming 文本 说明 警示"}, false)
	add(SlashEntry{CommandID: "doc.append_note", Label: "实验现场备注", Detail: "在空白处追加实验观察笔记", Slash: "/note", Kind: "note", SearchTerms: "note memo biji 笔记 备注 观察"}, true)
	return entries
}

// CleanStarterBlock constructs a scientifically clean starter block without silently
// fabricating `independent_n: 3`, `100 uL`, `30 min`, or fake `A1` control assignments.
func CleanStarterBlock(doc *asterdoc.Document, scopePrefix, kind string, plateFormat, excessPct int) map[string]any {
	prefix := strings.TrimSpace(scopePrefix)
	if prefix == "" {
		prefix = "blk"
	}
	base := kind
	if prefix != "" && prefix != "blk" && prefix != kind {
		base = prefix + "-" + kind
	}
	id := asterdoc.EnsureUniqueBlockID(doc, base)
	switch kind {
	case "prose":
		return map[string]any{"kind": "prose", "id": id, "schema": 1, "tone": "plain", "body": ""}
	case "group_table":
		return map[string]any{
			"kind":   "group_table",
			"id":     id,
			"schema": 1,
			"title":  "实验分组",
			"rows":   []any{},
		}
	case "param_table":
		return map[string]any{
			"kind":   "param_table",
			"id":     id,
			"schema": 1,
			"title":  "实验条件",
			"rows":   []any{},
		}
	case "figure":
		return map[string]any{
			"kind":    "figure",
			"id":      id,
			"schema":  1,
			"title":   "示意图 / 图片",
			"caption": "",
			"svg":     `<svg viewBox="0 0 360 160"></svg>`,
		}
	case "data_table":
		return map[string]any{
			"kind":   "data_table",
			"id":     id,
			"schema": 1,
			"title":  "数据记录表",
			"columns": []any{
				map[string]any{"key": "sample", "label": "样本", "value_kind": "text"},
				map[string]any{"key": "reading", "label": "测定值", "value_kind": "number"},
			},
		}
	case "checklist":
		return map[string]any{
			"kind":   "checklist",
			"id":     id,
			"schema": 1,
			"items": []any{
				map[string]any{"id": id + "-c1", "label": "待填写核对项"},
			},
		}
	case "calculator":
		blk := asterdoc.DefaultCalculatorPreset(doc, prefix, "blank")
		blk["id"] = id
		if excessPct > 0 {
			blk["excess_ratio"] = float64(excessPct) / 100.0
		}
		return blk
	case "timer":
		return map[string]any{
			"kind":   "timer",
			"id":     id,
			"schema": 1,
			"title":  "实验计时",
			"mode":   "stopwatch",
		}
	case "attachment":
		return map[string]any{
			"kind":       "attachment",
			"id":         id,
			"schema":     1,
			"title":      "实验照片与附件",
			"scope":      "step_evidence",
			"media_hint": "拖入或选择实验照片/原始文件",
		}
	case "subflow":
		return map[string]any{
			"kind":   "subflow",
			"id":     id,
			"schema": 1,
			"title":  "子流程",
			"items":  []any{},
		}
	case "plate":
		fmtVal := plateFormat
		switch fmtVal {
		case 4, 6, 12, 24, 48, 96, 384:
		default:
			fmtVal = 12
		}
		return map[string]any{
			"kind":   "plate",
			"id":     id,
			"schema": 1,
			"title":  fmt.Sprintf("%d 孔板布局", fmtVal),
			"format": fmtVal,
			"groups": map[string]any{},
		}
	default:
		return map[string]any{"kind": "prose", "id": id, "schema": 1, "body": ""}
	}
}
