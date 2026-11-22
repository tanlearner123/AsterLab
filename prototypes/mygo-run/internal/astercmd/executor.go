package astercmd

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"asterlab-mygo-prototype/internal/asterdoc"
)

// Operation is one atomic semantic command invocation inside a BatchRequest.
type Operation struct {
	CommandID string         `json:"command_id"`
	Args      map[string]any `json:"args,omitempty"`
	ClientRef string         `json:"client_ref,omitempty"`
}

// BatchRequest executes one or more Operations atomically against an AsterML document.
type BatchRequest struct {
	ExpectedHash   string      `json:"expected_hash,omitempty"`
	IdempotencyKey string      `json:"idempotency_key,omitempty"`
	Mode           ExecMode    `json:"mode,omitempty"` // default ModeEdit
	PlateFormat    int         `json:"plate_format,omitempty"`
	ExcessPct      int         `json:"excess_pct,omitempty"`
	PreviewOnly    bool        `json:"preview_only,omitempty"`
	Operations     []Operation `json:"operations"`
}

// CommandError is a structured, machine-readable error for both UI banners and Agents.
type CommandError struct {
	Code       string `json:"code"` // "CONFLICT" | "MODE_BLOCKED" | "UNKNOWN_COMMAND" | "INVALID_ARGS" | "VALIDATION_FAILED" | "NOT_FOUND"
	Message    string `json:"message"`
	OpIndex    int    `json:"op_index,omitempty"`
	CommandID  string `json:"command_id,omitempty"`
	LatestHash string `json:"latest_hash,omitempty"`
}

func (e *CommandError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// BatchResult reports the outcome of a Preview or Execute call.
type BatchResult struct {
	OK           bool              `json:"ok"`
	PreviewOnly  bool              `json:"preview_only,omitempty"`
	Replayed     bool              `json:"replayed,omitempty"`
	PreviousHash string            `json:"previous_hash"`
	DocumentHash string            `json:"document_hash"`
	CreatedIDs   map[string]string `json:"created_ids,omitempty"`
	AffectedIDs  []string          `json:"affected_ids,omitempty"`
	DiffSummary  []string          `json:"diff_summary,omitempty"`
	Warnings     []string          `json:"warnings,omitempty"`
	NextSource   string            `json:"-"`
	Error        *CommandError     `json:"error,omitempty"`
}

// DocumentHash computes a deterministic SHA-256 ETag over normalized `\n` document text.
func DocumentHash(source string) string {
	norm := strings.ReplaceAll(source, "\r\n", "\n")
	sum := sha256.Sum256([]byte(norm))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ExecuteBatch runs req atomically on source in memory and returns BatchResult.
// If any operation fails, NextSource is left empty and the original source is untouched.
func ExecuteBatch(source string, req BatchRequest) BatchResult {
	reg := DefaultRegistry()
	prevHash := DocumentHash(source)
	mode := req.Mode
	if mode == "" {
		mode = ModeEdit
	}
	res := BatchResult{
		PreviewOnly:  req.PreviewOnly,
		PreviousHash: prevHash,
		DocumentHash: prevHash,
		CreatedIDs:   map[string]string{},
	}
	if exp := strings.TrimSpace(req.ExpectedHash); exp != "" && exp != prevHash {
		res.Error = &CommandError{
			Code:       "CONFLICT",
			Message:    fmt.Sprintf("文档已被外部修改（预期 %s，当前 %s），请重新读取后再提交", exp, prevHash),
			LatestHash: prevHash,
		}
		return res
	}
	if len(req.Operations) == 0 {
		res.Error = &CommandError{Code: "INVALID_ARGS", Message: "operations 不能为空"}
		return res
	}

	working := source
	refs := map[string]string{}
	affectedSeen := map[string]bool{}
	addAffected := func(id string) {
		if id != "" && !affectedSeen[id] {
			affectedSeen[id] = true
			res.AffectedIDs = append(res.AffectedIDs, id)
		}
	}

	for idx, op := range req.Operations {
		desc, ok := reg.Describe(op.CommandID)
		if !ok {
			res.Error = &CommandError{
				Code:      "UNKNOWN_COMMAND",
				Message:   fmt.Sprintf("未知命令 %q", op.CommandID),
				OpIndex:   idx,
				CommandID: op.CommandID,
			}
			return res
		}
		if mode == ModeLab && !desc.AllowInLab {
			res.Error = &CommandError{
				Code:      "MODE_BLOCKED",
				Message:   fmt.Sprintf("实验模式下禁止执行结构修改命令 %q，请切回编辑模式", op.CommandID),
				OpIndex:   idx,
				CommandID: op.CommandID,
			}
			return res
		}

		args := resolveClientRefsMap(op.Args, refs)
		next, createdID, affectedID, summary, cmdErr := executeSingleOp(working, op.CommandID, args, req.PlateFormat, req.ExcessPct)
		if cmdErr != nil {
			cmdErr.OpIndex = idx
			cmdErr.CommandID = op.CommandID
			res.Error = cmdErr
			return res
		}
		working = next
		if createdID != "" {
			if ref := strings.TrimSpace(op.ClientRef); ref != "" {
				refs[ref] = createdID
				res.CreatedIDs[ref] = createdID
			} else {
				res.CreatedIDs[fmt.Sprintf("op_%d", idx)] = createdID
			}
			addAffected(createdID)
		}
		addAffected(affectedID)
		if summary != "" {
			res.DiffSummary = append(res.DiffSummary, summary)
		}
	}

	// Final document validation + relation diagnostics as non-blocking warnings
	finalDoc, err := asterdoc.Parse(working)
	if err != nil {
		res.Error = &CommandError{
			Code:    "VALIDATION_FAILED",
			Message: "最终文档校验失败: " + err.Error(),
		}
		return res
	}
	for _, rel := range asterdoc.ResolveRelations(finalDoc) {
		res.Warnings = append(res.Warnings, rel.Issues...)
	}

	res.OK = true
	res.DocumentHash = DocumentHash(working)
	res.NextSource = working
	return res
}

func executeSingleOp(source, cmdID string, args map[string]any, plateFormat, excessPct int) (nextSource, createdID, affectedID, summary string, cmdErr *CommandError) {
	switch cmdID {
	case "doc.insert_markdown":
		md := strArg(args, "markdown")
		if md == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "markdown 不能为空"}
		}
		pos := intArg(args, "at_offset", -1)
		next := insertProseAt(source, pos, md)
		return next, "", "", "插入自由 Markdown 正文", nil

	case "doc.append_note":
		txt := strings.TrimSpace(strArg(args, "text"))
		if txt == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "text 不能为空"}
		}
		next := insertProseAt(source, -1, txt)
		return next, "", "", "追加实验空白笔记", nil

	case "group.create":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		name := strings.TrimSpace(strArg(args, "name"))
		role := strings.TrimSpace(strArg(args, "role"))
		cond := strings.TrimSpace(strArg(args, "condition"))
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		gtID := strings.TrimSpace(strArg(args, "group_table_id"))
		atOffset := intArg(args, "at_offset", -1)

		// If caller specified name and an existing group_table exists (or explicit group_table_id), append a row to it.
		if name != "" {
			targetGT, ownerStep := findGroupTable(doc, gtID)
			if targetGT != nil {
				gid := nextUniqueGroupID(doc, name)
				row := map[string]any{"id": gid, "name": name}
				if role != "" {
					row["role"] = role
				}
				if cond != "" {
					row["condition"] = cond
				}
				if n := intArg(args, "independent_n", 0); n > 0 {
					row["independent_n"] = n
				}
				if r := intArg(args, "technical_repeats", 0); r > 0 {
					row["technical_repeats"] = r
				}
				rows := anySliceCopy(targetGT.Data["rows"])
				rows = append(rows, row)
				var next string
				var uErr error
				if ownerStep != "" {
					next, uErr = asterdoc.UpdateStepBlockInSource(source, ownerStep, targetGT.ID, map[string]any{"rows": rows})
				} else {
					next, uErr = doc.UpdateModule(targetGT.ID, map[string]any{"rows": rows})
				}
				if uErr != nil {
					return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
				}
				return next, gid, targetGT.ID, fmt.Sprintf("向分组表 %s 添加组别 %s (%s)", targetGT.ID, name, gid), nil
			}
		}

		// Otherwise create a new group_table module (with 0 rows if blank, or 1 row if name provided).
		blk := CleanStarterBlock(doc, firstNonEmptyStr(stepID, "grp"), "group_table", plateFormat, excessPct)
		if title := strings.TrimSpace(strArg(args, "title")); title != "" {
			blk["title"] = title
		}
		var createdGroupID string
		if name != "" {
			createdGroupID = nextUniqueGroupID(doc, name)
			row := map[string]any{"id": createdGroupID, "name": name}
			if role != "" {
				row["role"] = role
			}
			if cond != "" {
				row["condition"] = cond
			}
			if n := intArg(args, "independent_n", 0); n > 0 {
				row["independent_n"] = n
			}
			if r := intArg(args, "technical_repeats", 0); r > 0 {
				row["technical_repeats"] = r
			}
			blk["rows"] = []any{row}
		}
		if stepID != "" {
			next, err := asterdoc.InsertStepBlockInSource(source, stepID, intArg(args, "index", -1), blk)
			if err != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
			}
			retID := firstNonEmptyStr(createdGroupID, fmt.Sprint(blk["id"]))
			return next, retID, fmt.Sprint(blk["id"]), fmt.Sprintf("在步骤 %s 中创建分组表", stepID), nil
		}
		next, modID, err := asterdoc.InsertTopLevelModuleInSource(source, atOffset, blk)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		retID := firstNonEmptyStr(createdGroupID, modID)
		return next, retID, modID, fmt.Sprintf("创建顶层分组表 %s", modID), nil

	case "plate.create":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		fmtVal := intArg(args, "format", plateFormat)
		blk := CleanStarterBlock(doc, firstNonEmptyStr(stepID, "plate"), "plate", fmtVal, excessPct)
		if title := strings.TrimSpace(strArg(args, "title")); title != "" {
			blk["title"] = title
		}
		if topo := strings.TrimSpace(strArg(args, "topology")); topo != "" {
			blk["topology"] = topo
		}
		if stepID != "" {
			next, err := asterdoc.InsertStepBlockInSource(source, stepID, intArg(args, "index", -1), blk)
			if err != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
			}
			pid := fmt.Sprint(blk["id"])
			return next, pid, pid, fmt.Sprintf("在步骤 %s 中创建孔板 %s", stepID, pid), nil
		}
		next, pid, err := asterdoc.InsertTopLevelModuleInSource(source, intArg(args, "at_offset", -1), blk)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		return next, pid, pid, fmt.Sprintf("创建顶层孔板 %s", pid), nil

	case "plate.assign_wells":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		plateID := strings.TrimSpace(strArg(args, "plate_id"))
		groupID := strings.TrimSpace(strArg(args, "group_id"))
		if groupID == "" {
			groupID = strings.TrimSpace(strArg(args, "group_ref"))
		}
		wells := strSliceArg(args, "wells")
		if plateID == "" || groupID == "" || len(wells) == 0 {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "plate_id、group_id 和 wells 均为必填"}
		}
		plateMod, ownerStep := findModuleByID(doc, plateID)
		if plateMod == nil || plateMod.Kind != "plate" {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("未找到孔板 %q", plateID)}
		}
		groupsMap := map[string][]string{}
		if rawG, ok := plateMod.Data["groups"].(map[string]any); ok {
			for g, rawW := range rawG {
				for _, w := range toStringSlice(rawW) {
					groupsMap[g] = append(groupsMap[g], strings.ToUpper(strings.TrimSpace(w)))
				}
			}
		}
		targetWells := make(map[string]bool, len(wells))
		normWells := make([]string, 0, len(wells))
		for _, w := range wells {
			nw := strings.ToUpper(strings.TrimSpace(w))
			if nw != "" && !targetWells[nw] {
				targetWells[nw] = true
				normWells = append(normWells, nw)
			}
		}
		// Remove target wells from any previous group to avoid duplicate assignment error.
		for g, ws := range groupsMap {
			kept := ws[:0]
			for _, w := range ws {
				if !targetWells[w] {
					kept = append(kept, w)
				}
			}
			if len(kept) == 0 {
				delete(groupsMap, g)
			} else {
				groupsMap[g] = kept
			}
		}
		groupsMap[groupID] = append(groupsMap[groupID], normWells...)

		props := map[string]any{"groups": groupsMap}
		if role := strings.TrimSpace(strArg(args, "role")); role != "" {
			roles := copyStringAnyMap(plateMod.Data["roles"])
			for _, w := range normWells {
				roles[w] = role
			}
			props["roles"] = roles
		}
		if sid := strings.TrimSpace(strArg(args, "sample_id")); sid != "" {
			samples := copyStringAnyMap(plateMod.Data["samples"])
			for _, w := range normWells {
				samples[w] = sid
			}
			props["samples"] = samples
		}
		if facRaw, ok := args["factors"].(map[string]any); ok && len(facRaw) > 0 {
			factors := copyNestedStringAnyMap(plateMod.Data["factors"])
			for dim, val := range facRaw {
				if factors[dim] == nil {
					factors[dim] = map[string]string{}
				}
				for _, w := range normWells {
					factors[dim][w] = strings.TrimSpace(fmt.Sprint(val))
				}
			}
			props["factors"] = factors
		}

		var next string
		var uErr error
		if ownerStep != "" {
			next, uErr = asterdoc.UpdateStepBlockInSource(source, ownerStep, plateID, props)
		} else {
			next, uErr = doc.UpdateModule(plateID, props)
		}
		if uErr != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
		}
		return next, "", plateID, fmt.Sprintf("为孔板 %s 分配 %d 个孔位到组 %s", plateID, len(normWells), groupID), nil

	case "step.create":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		presetID := strings.TrimSpace(strArg(args, "preset"))
		if presetID == "" {
			presetID = "blank"
		}
		isSubstep := presetID == "substep"
		lookupPresetID := presetID
		if isSubstep {
			lookupPresetID = "blank"
		}
		var preset *StepPreset
		for _, p := range StepPresetDefs() {
			if p.ID == lookupPresetID {
				cp := p
				preset = &cp
				break
			}
		}
		if preset == nil {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: fmt.Sprintf("未知步骤预设 %q", presetID)}
		}
		tree := asterdoc.ParseStepTree(source, doc)
		stepCount := len(tree.Roots)
		title := strings.TrimSpace(strArg(args, "title"))
		if title == "" {
			if isSubstep {
				title = "新子步骤"
			} else if preset.ID == "blank" {
				title = fmt.Sprintf("步骤 %d", stepCount+1)
			} else {
				title = fmt.Sprintf("步骤 %d · %s", stepCount+1, preset.Label)
			}
		}
		style := strings.TrimSpace(strArg(args, "style"))
		legacyContainer, _ := args["legacy_container"].(bool)
		if style == "module" || legacyContainer {
			stepID := asterdoc.EnsureUniqueBlockID(doc, fmt.Sprintf("step-%02d", stepCount+1))
			stepID = strings.TrimPrefix(stepID, "blk-")
			blocks := make([]any, 0, len(preset.BlockKinds))
			for bi, bk := range preset.BlockKinds {
				blk := CleanStarterBlock(doc, fmt.Sprintf("%s-%d", stepID, bi+1), bk, plateFormat, excessPct)
				blocks = append(blocks, blk)
			}
			mod := map[string]any{
				"kind":      "step",
				"id":        stepID,
				"schema":    1,
				"execution": "executable",
				"title":     title,
				"blocks":    blocks,
			}
			next, created, err := asterdoc.InsertTopLevelModuleInSource(source, intArg(args, "at_offset", -1), mod)
			if err != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
			}
			return next, created, created, fmt.Sprintf("创建步骤 %s (%s)", title, preset.Label), nil
		}

		// Default flat AsterML heading-step flow: `##@ 标题` + flat top-level modules & Markdown tasks
		level := intArg(args, "level", 1)
		if presetID == "substep" {
			level = 2
			if title == "新步骤" {
				title = "新子步骤"
			}
		}
		anchor := strings.TrimSpace(strArg(args, "anchor"))
		headerLine := asterdoc.FormatHeadingStep(level, title, anchor)
		atOff := intArg(args, "at_offset", -1)
		working := insertProseAt(source, atOff, headerLine)
		insertCursor := -1
		if atOff >= 0 {
			if idx := strings.Index(working[min(atOff, len(working)):], headerLine); idx >= 0 {
				insertCursor = min(atOff, len(working)) + idx + len(headerLine)
			}
		}
		for _, bk := range preset.BlockKinds {
			if bk == "checklist" {
				working = insertProseAt(working, insertCursor, "- [ ] 待填写核对项")
				if insertCursor >= 0 {
					insertCursor = min(len(working), insertCursor+len("\n\n- [ ] 待填写核对项"))
				}
				continue
			}
			curDoc, pErr := asterdoc.Parse(working)
			if pErr != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: pErr.Error()}
			}
			blk := CleanStarterBlock(curDoc, bk, bk, plateFormat, excessPct)
			nextW, _, insErr := asterdoc.InsertTopLevelModuleInSource(working, insertCursor, blk)
			if insErr != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: insErr.Error()}
			}
			if insertCursor >= 0 {
				insertCursor += len(nextW) - len(working)
			}
			working = nextW
		}
		finalDoc, _ := asterdoc.Parse(working)
		finalTree := asterdoc.ParseStepTree(working, finalDoc)
		createdID := fmt.Sprintf("step-%d", stepCount+1)
		if anchor != "" {
			createdID = anchor
		} else if len(finalTree.Flat) > 0 {
			createdID = finalTree.Flat[len(finalTree.Flat)-1].ID
		}
		return working, createdID, createdID, fmt.Sprintf("创建扁平步骤 %s (%s)", title, preset.Label), nil

	case "block.insert":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		kind := strings.TrimSpace(strArg(args, "kind"))
		if kind == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "kind 为必填"}
		}
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		blk := CleanStarterBlock(doc, firstNonEmptyStr(stepID, kind), kind, plateFormat, excessPct)
		if title := strings.TrimSpace(strArg(args, "title")); title != "" {
			blk["title"] = title
		}
		if props, ok := args["props"].(map[string]any); ok {
			for k, v := range props {
				if k != "kind" && k != "schema" {
					blk[k] = v
				}
			}
		}
		if stepID != "" {
			next, err := asterdoc.InsertStepBlockInSource(source, stepID, intArg(args, "index", -1), blk)
			if err != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
			}
			bid := fmt.Sprint(blk["id"])
			return next, bid, bid, fmt.Sprintf("在步骤 %s 中插入 %s (%s)", stepID, kind, bid), nil
		}
		next, mid, err := asterdoc.InsertTopLevelModuleInSource(source, intArg(args, "at_offset", -1), blk)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		return next, mid, mid, fmt.Sprintf("在顶层插入 %s (%s)", kind, mid), nil

	case "block.update":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		modID := strings.TrimSpace(strArg(args, "module_id"))
		props, _ := args["props"].(map[string]any)
		if modID == "" || len(props) == 0 {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "module_id 和 props 为必填"}
		}
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		if stepID == "" {
			_, owner := findModuleByID(doc, modID)
			stepID = owner
		}
		var next string
		var uErr error
		if stepID != "" {
			next, uErr = asterdoc.UpdateStepBlockInSource(source, stepID, modID, props)
		} else {
			next, uErr = doc.UpdateModule(modID, props)
		}
		if uErr != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
		}
		return next, "", modID, fmt.Sprintf("更新模块 %s", modID), nil

	case "block.move":
		modID := strings.TrimSpace(strArg(args, "module_id"))
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		if modID == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "module_id 为必填"}
		}
		var next string
		var err error
		if stepID != "" {
			next, err = asterdoc.MoveStepBlockToIndex(source, stepID, modID, intArg(args, "to_index", 0))
		} else {
			next, err = asterdoc.MoveTopLevelModuleInSource(source, modID, intArg(args, "delta", 0))
		}
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		return next, "", modID, fmt.Sprintf("移动模块 %s", modID), nil

	case "block.duplicate":
		modID := strings.TrimSpace(strArg(args, "module_id"))
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		if modID == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "module_id 为必填"}
		}
		if stepID != "" {
			next, err := asterdoc.DuplicateStepBlockInSource(source, stepID, modID)
			if err != nil {
				return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
			}
			return next, "", modID, fmt.Sprintf("复制步骤内模块 %s", modID), nil
		}
		next, created, err := asterdoc.DuplicateTopLevelModuleInSource(source, modID)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		return next, created, created, fmt.Sprintf("复制顶层模块 %s → %s", modID, created), nil

	case "block.remove":
		modID := strings.TrimSpace(strArg(args, "module_id"))
		stepID := strings.TrimSpace(strArg(args, "step_id"))
		if modID == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "module_id 为必填"}
		}
		if stepID == "" {
			if doc, err := asterdoc.Parse(source); err == nil {
				_, stepID = findModuleByID(doc, modID)
			}
		}
		var next string
		var err error
		if stepID != "" {
			next, err = asterdoc.RemoveStepBlockInSource(source, stepID, modID)
		} else {
			next, err = asterdoc.RemoveTopLevelModuleInSource(source, modID)
		}
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		return next, "", modID, fmt.Sprintf("删除模块 %s", modID), nil

	case "asset.embed":
		asset := strings.TrimSpace(strArg(args, "asset"))
		if asset == "" {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "asset 路径不能为空"}
		}
		if strings.Contains(asset, "..") {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "禁止路径越界 (..)"}
		}
		rel := buildInlineRelationFromArgs(args)
		line := asterdoc.FormatAnnotatedEmbed(asset, strArg(args, "width"), rel)
		next := insertProseAt(source, intArg(args, "at_offset", -1), line)
		return next, "", asset, fmt.Sprintf("嵌入资产 %s", line), nil

	case "relation.set":
		rel := buildInlineRelationFromArgs(args)
		if !rel.HasRelation() {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "至少需要一个关联目标或属性"}
		}
		spanStart := intArg(args, "span_start", -1)
		spanEnd := intArg(args, "span_end", -1)
		var next string
		var err error
		if spanStart >= 0 && spanEnd > spanStart {
			next, err = asterdoc.SetInlineRelationAtSpan(source, spanStart, spanEnd, rel)
		} else {
			asset := strings.TrimSpace(strArg(args, "asset"))
			if asset == "" {
				return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "需要提供 asset 或 span_start/span_end"}
			}
			next, err = asterdoc.SetInlineRelationByAsset(source, asset, rel)
		}
		if err != nil {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: err.Error()}
		}
		return next, "", strArg(args, "asset"), fmt.Sprintf("设置行内语义关联 %s", asterdoc.FormatRelationAttr(rel)), nil

	case "relation.remove":
		spanStart := intArg(args, "span_start", -1)
		spanEnd := intArg(args, "span_end", -1)
		var next string
		var err error
		if spanStart >= 0 && spanEnd > spanStart {
			next, err = asterdoc.SetInlineRelationAtSpan(source, spanStart, spanEnd, asterdoc.InlineRelation{})
		} else {
			asset := strings.TrimSpace(strArg(args, "asset"))
			if asset == "" {
				return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "需要提供 asset 或 span_start/span_end"}
			}
			next, err = asterdoc.SetInlineRelationByAsset(source, asset, asterdoc.InlineRelation{})
		}
		if err != nil {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: err.Error()}
		}
		return next, "", strArg(args, "asset"), "移除行内语义关联", nil

	case "lab.check":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		modID := strings.TrimSpace(strArg(args, "module_id"))
		itemID := strings.TrimSpace(strArg(args, "item_id"))
		done, _ := args["done"].(bool)
		mod, ownerStep := findModuleByID(doc, modID)
		if mod == nil || mod.Kind != "checklist" {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("未找到检查清单 %q", modID)}
		}
		items := anySliceCopy(mod.Data["items"])
		found := false
		for i, raw := range items {
			im, _ := raw.(map[string]any)
			if im != nil && strings.TrimSpace(fmt.Sprint(im["id"])) == itemID {
				im["done"] = done
				if note, hasNote := args["note"].(string); hasNote {
					im["note"] = strings.TrimSpace(note)
				}
				items[i] = im
				found = true
				break
			}
		}
		if !found {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("未找到检查项 %q", itemID)}
		}
		var next string
		var uErr error
		if ownerStep != "" {
			next, uErr = asterdoc.UpdateStepBlockInSource(source, ownerStep, modID, map[string]any{"items": items})
		} else {
			next, uErr = doc.UpdateModule(modID, map[string]any{"items": items})
		}
		if uErr != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
		}
		return next, "", modID, fmt.Sprintf("更新检查项 %s → %v", itemID, done), nil

	case "lab.param_actual":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		modID := strings.TrimSpace(strArg(args, "module_id"))
		key := strings.TrimSpace(strArg(args, "key"))
		actual, hasActual := args["actual"]
		if modID == "" || key == "" || !hasActual {
			return "", "", "", "", &CommandError{Code: "INVALID_ARGS", Message: "module_id、key 和 actual 为必填"}
		}
		mod, ownerStep := findModuleByID(doc, modID)
		if mod == nil || mod.Kind != "param_table" {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("未找到参数表 %q", modID)}
		}
		rows := anySliceCopy(mod.Data["rows"])
		found := false
		for i, raw := range rows {
			rm, _ := raw.(map[string]any)
			if rm != nil && strings.TrimSpace(fmt.Sprint(rm["key"])) == key {
				rm["actual"] = actual
				if note, ok := args["note"].(string); ok {
					rm["note"] = strings.TrimSpace(note)
				}
				rows[i] = rm
				found = true
				break
			}
		}
		if !found {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("参数表中未找到 key=%q", key)}
		}
		var next string
		var uErr error
		if ownerStep != "" {
			next, uErr = asterdoc.UpdateStepBlockInSource(source, ownerStep, modID, map[string]any{"rows": rows})
		} else {
			next, uErr = doc.UpdateModule(modID, map[string]any{"rows": rows})
		}
		if uErr != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
		}
		return next, "", modID, fmt.Sprintf("记录参数 %s 实测值 %v", key, actual), nil

	case "lab.plate_exclude":
		doc, err := asterdoc.Parse(source)
		if err != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: err.Error()}
		}
		plateID := strings.TrimSpace(strArg(args, "plate_id"))
		wells := strSliceArg(args, "wells")
		excluded, _ := args["excluded"].(bool)
		mod, ownerStep := findModuleByID(doc, plateID)
		if mod == nil || mod.Kind != "plate" {
			return "", "", "", "", &CommandError{Code: "NOT_FOUND", Message: fmt.Sprintf("未找到孔板 %q", plateID)}
		}
		cur := map[string]bool{}
		for _, w := range toStringSlice(mod.Data["excluded_wells"]) {
			cur[strings.ToUpper(strings.TrimSpace(w))] = true
		}
		for _, w := range wells {
			nw := strings.ToUpper(strings.TrimSpace(w))
			if nw == "" {
				continue
			}
			if excluded {
				cur[nw] = true
			} else {
				delete(cur, nw)
			}
		}
		nextList := make([]string, 0, len(cur))
		for w := range cur {
			nextList = append(nextList, w)
		}
		var next string
		var uErr error
		if ownerStep != "" {
			next, uErr = asterdoc.UpdateStepBlockInSource(source, ownerStep, plateID, map[string]any{"excluded_wells": nextList})
		} else {
			next, uErr = doc.UpdateModule(plateID, map[string]any{"excluded_wells": nextList})
		}
		if uErr != nil {
			return "", "", "", "", &CommandError{Code: "VALIDATION_FAILED", Message: uErr.Error()}
		}
		return next, "", plateID, fmt.Sprintf("更新孔板 %s 异常孔标记", plateID), nil
	}

	return "", "", "", "", &CommandError{Code: "UNKNOWN_COMMAND", Message: fmt.Sprintf("未实现的命令 %q", cmdID)}
}

// --- Idempotency Journal (stored in `.asterlab/cache/cmd-journal.json`, never in `.asterml`) ---

type journalEntry struct {
	Key    string      `json:"key"`
	Result BatchResult `json:"result"`
}

// ExecuteBatchOnFile runs req atomically on absPath with optional bounded idempotency journal in journalPath.
func ExecuteBatchOnFile(absPath, journalPath string, req BatchRequest) BatchResult {
	raw, err := os.ReadFile(absPath)
	if err != nil {
		return BatchResult{Error: &CommandError{Code: "NOT_FOUND", Message: err.Error()}}
	}
	src := string(raw)
	cacheKey := ""
	if k := strings.TrimSpace(req.IdempotencyKey); k != "" && journalPath != "" && !req.PreviewOnly {
		cacheKey = filepath.Clean(absPath) + "::" + k
		if cached, ok := lookupJournal(journalPath, cacheKey); ok {
			cached.Replayed = true
			return cached
		}
	}
	res := ExecuteBatch(src, req)
	if !res.OK || req.PreviewOnly {
		return res
	}
	if err := asterdoc.WriteFileAtomic(absPath, []byte(res.NextSource)); err != nil {
		res.OK = false
		res.Error = &CommandError{Code: "IO_ERROR", Message: err.Error()}
		return res
	}
	if cacheKey != "" {
		saveJournal(journalPath, cacheKey, res)
	}
	return res
}

func lookupJournal(path, key string) (BatchResult, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return BatchResult{}, false
	}
	var entries []journalEntry
	if json.Unmarshal(b, &entries) != nil {
		return BatchResult{}, false
	}
	for _, e := range entries {
		if e.Key == key {
			return e.Result, true
		}
	}
	return BatchResult{}, false
}

func saveJournal(path, key string, res BatchResult) {
	var entries []journalEntry
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &entries)
	}
	res.NextSource = ""
	entries = append(entries, journalEntry{Key: key, Result: res})
	if len(entries) > 200 {
		entries = entries[len(entries)-200:]
	}
	if data, err := json.Marshal(entries); err == nil {
		_ = asterdoc.WriteFileAtomic(path, data)
	}
}

// --- Helpers ---

func buildInlineRelationFromArgs(args map[string]any) asterdoc.InlineRelation {
	rel := asterdoc.InlineRelation{
		Timepoint: strings.TrimSpace(strArg(args, "timepoint")),
		FOV:       intArg(args, "fov", 0),
		Role:      strings.TrimSpace(strArg(args, "role")),
		Note:      strings.TrimSpace(strArg(args, "note")),
	}
	switch tv := args["targets"].(type) {
	case []any:
		for _, item := range tv {
			switch it := item.(type) {
			case string:
				parsed := asterdoc.ParseRelationAttr("{" + ensureAtPrefix(it) + "}")
				rel.Targets = append(rel.Targets, parsed.Targets...)
			case map[string]any:
				oid := strings.TrimSpace(strArg(it, "objectId"))
				if oid == "" {
					oid = strings.TrimSpace(strArg(it, "object_id"))
				}
				subs := strSliceArg(it, "subTargets")
				if len(subs) == 0 {
					subs = strSliceArg(it, "sub_targets")
				}
				if oid != "" {
					rel.Targets = append(rel.Targets, asterdoc.RelationTarget{ObjectID: oid, SubTargets: subs})
				}
			}
		}
	case []string:
		for _, s := range tv {
			parsed := asterdoc.ParseRelationAttr("{" + ensureAtPrefix(s) + "}")
			rel.Targets = append(rel.Targets, parsed.Targets...)
		}
	case string:
		if strings.TrimSpace(tv) != "" {
			parsed := asterdoc.ParseRelationAttr("{" + ensureAtPrefix(tv) + "}")
			rel.Targets = append(rel.Targets, parsed.Targets...)
		}
	}
	return rel
}

func ensureAtPrefix(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasPrefix(s, "@") {
		return s
	}
	return "@" + s
}

func insertProseAt(source string, byteOffset int, text string) string {
	_, bodyStart, _ := asterdoc.ScanFrontmatter(source)
	pos := byteOffset
	if pos < 0 || pos > len(source) {
		pos = len(source)
	}
	if pos < bodyStart {
		pos = bodyStart
	}
	prefix := source[:pos]
	suffix := source[pos:]
	var b strings.Builder
	b.WriteString(prefix)
	if len(prefix) > 0 && !strings.HasSuffix(prefix, "\n\n") {
		if strings.HasSuffix(prefix, "\n") {
			b.WriteString("\n")
		} else {
			b.WriteString("\n\n")
		}
	}
	b.WriteString(strings.TrimRight(text, "\r\n"))
	b.WriteString("\n")
	if len(suffix) > 0 && !strings.HasPrefix(suffix, "\n") {
		b.WriteString("\n")
	}
	b.WriteString(suffix)
	return b.String()
}

func resolveClientRefsMap(args map[string]any, refs map[string]string) map[string]any {
	if args == nil || len(refs) == 0 {
		return args
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = resolveClientRefValue(v, refs)
	}
	return out
}

func resolveClientRefValue(v any, refs map[string]string) any {
	switch tv := v.(type) {
	case string:
		if strings.HasPrefix(tv, "$ref:") {
			if id, ok := refs[strings.TrimPrefix(tv, "$ref:")]; ok {
				return id
			}
		}
		if id, ok := refs[tv]; ok {
			return id
		}
		// Also substitute `@<client_ref>:A1` inside target strings
		for refName, realID := range refs {
			if strings.HasPrefix(tv, "@"+refName+":") {
				return "@" + realID + tv[1+len(refName):]
			}
			if strings.HasPrefix(tv, refName+":") {
				return realID + tv[len(refName):]
			}
		}
		return tv
	case []any:
		cp := make([]any, len(tv))
		for i, item := range tv {
			cp[i] = resolveClientRefValue(item, refs)
		}
		return cp
	case []string:
		cp := make([]string, len(tv))
		for i, item := range tv {
			if s, ok := resolveClientRefValue(item, refs).(string); ok {
				cp[i] = s
			} else {
				cp[i] = item
			}
		}
		return cp
	case map[string]any:
		return resolveClientRefsMap(tv, refs)
	default:
		return v
	}
}

func findModuleByID(doc *asterdoc.Document, id string) (*asterdoc.Module, string) {
	if doc == nil || id == "" {
		return nil, ""
	}
	for i := range doc.Modules {
		if doc.Modules[i].ID == id {
			return &doc.Modules[i], ""
		}
		if doc.Modules[i].Kind == "step" {
			if blks, ok := doc.Modules[i].Data["blocks"].([]any); ok {
				for _, raw := range blks {
					bm, _ := raw.(map[string]any)
					if bm != nil && fmt.Sprint(bm["id"]) == id {
						m := asterdoc.Module{
							Kind: fmt.Sprint(bm["kind"]),
							ID:   id,
							Data: bm,
						}
						return &m, doc.Modules[i].ID
					}
				}
			}
		}
	}
	return nil, ""
}

func findGroupTable(doc *asterdoc.Document, explicitID string) (*asterdoc.Module, string) {
	if explicitID != "" {
		m, owner := findModuleByID(doc, explicitID)
		if m != nil && m.Kind == "group_table" {
			return m, owner
		}
		return nil, ""
	}
	for i := range doc.Modules {
		if doc.Modules[i].Kind == "group_table" {
			return &doc.Modules[i], ""
		}
	}
	for i := range doc.Modules {
		if doc.Modules[i].Kind == "step" {
			if blks, ok := doc.Modules[i].Data["blocks"].([]any); ok {
				for _, raw := range blks {
					bm, _ := raw.(map[string]any)
					if bm != nil && fmt.Sprint(bm["kind"]) == "group_table" {
						m := asterdoc.Module{Kind: "group_table", ID: fmt.Sprint(bm["id"]), Data: bm}
						return &m, doc.Modules[i].ID
					}
				}
			}
		}
	}
	return nil, ""
}

func nextUniqueGroupID(doc *asterdoc.Document, name string) string {
	used := map[string]bool{}
	for _, g := range doc.GroupList() {
		used[g.ID] = true
	}
	var slug strings.Builder
	for _, r := range strings.ToLower(name) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			slug.WriteRune(r)
		}
	}
	base := "g-" + slug.String()
	if base == "g-" {
		base = fmt.Sprintf("g-%d", len(used)+1)
	}
	cand := base
	for n := 2; used[cand]; n++ {
		cand = fmt.Sprintf("%s-%d", base, n)
	}
	return cand
}

func strArg(args map[string]any, key string) string {
	if args == nil {
		return ""
	}
	if s, ok := args[key].(string); ok {
		return s
	}
	return ""
}

func intArg(args map[string]any, key string, fallback int) int {
	if args == nil {
		return fallback
	}
	switch n := args[key].(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	}
	return fallback
}

func strSliceArg(args map[string]any, key string) []string {
	if args == nil {
		return nil
	}
	return toStringSlice(args[key])
}

func toStringSlice(raw any) []string {
	switch tv := raw.(type) {
	case []string:
		return append([]string(nil), tv...)
	case []any:
		out := make([]string, 0, len(tv))
		for _, it := range tv {
			if s := strings.TrimSpace(fmt.Sprint(it)); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func anySliceCopy(raw any) []any {
	src, _ := raw.([]any)
	out := make([]any, len(src))
	copy(out, src)
	return out
}

func copyStringAnyMap(raw any) map[string]string {
	out := map[string]string{}
	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

func copyNestedStringAnyMap(raw any) map[string]map[string]string {
	out := map[string]map[string]string{}
	if m, ok := raw.(map[string]any); ok {
		for dim, inner := range m {
			out[dim] = copyStringAnyMap(inner)
		}
	}
	return out
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
