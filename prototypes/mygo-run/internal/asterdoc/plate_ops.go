package asterdoc

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
	"gopkg.in/yaml.v3"

	"asterlab-mygo-prototype/internal/theme"
	plateui "asterlab-mygo-prototype/internal/uikit/plate"
)

// PlateAnnotationResult describes the exact AsterML updates emitted by
// ApplyUnifiedPlateAnnotation so both direct-source hosts and Rust-Core
// command hosts can apply the mutation deterministically.
type PlateAnnotationResult struct {
	PlateProps            map[string]any
	NewGroupModule        map[string]any
	UpdateGroupTableID    string
	UpdateGroupTableStep  string
	UpdateGroupTableRows  []any
	PreferredColorBy      string
	PreferredFactorKey    string
	AppliedSummary        string
}

// ApplyUnifiedPlateAnnotation interprets a single researcher input string
// (plain treatment/sample/QC label, `样本:...`, `角色:...`, or orthogonal `维度:值` / gradient `浓度:100/2 uM`)
// over selectedWells and deterministically recomputes technical replicates.
func ApplyUnifiedPlateAnnotation(doc *Document, plate *Plate, wells []string, rawInput string) (PlateAnnotationResult, error) {
	if plate == nil {
		return PlateAnnotationResult{}, fmt.Errorf("未找到目标孔板模块")
	}
	trimmed := strings.TrimSpace(rawInput)
	if trimmed == "" {
		return PlateAnnotationResult{}, fmt.Errorf("请输入处理、基因或样本名称")
	}
	if len(wells) == 0 {
		return PlateAnnotationResult{}, fmt.Errorf("请先框选或点击要标注的孔位")
	}
	geometry, ok := geometryForPlate(plate.Format, plate.Topology, plate.Rows, plate.Columns)
	if !ok {
		return PlateAnnotationResult{}, fmt.Errorf("容器规格 %d 未注册", plate.Format)
	}
	topo := strings.TrimSpace(plate.Topology)
	if topo == "" {
		topo = "plate"
	}

	selected := make([]string, 0, len(wells))
	seenSel := map[string]bool{}
	for _, w := range wells {
		nw := strings.ToUpper(strings.TrimSpace(w))
		if nw == "" || seenSel[nw] {
			continue
		}
		if !ValidPlateWell(nw, topo, geometry.Rows, geometry.Columns) {
			return PlateAnnotationResult{}, fmt.Errorf("孔位 %q 超出 %dx%d 范围", nw, geometry.Rows, geometry.Columns)
		}
		seenSel[nw] = true
		selected = append(selected, nw)
	}
	sort.Slice(selected, func(i, j int) bool {
		return wellLess(geometry, selected[i], selected[j])
	})

	outGroups := copyWellGroupsMap(plate.Groups)
	outSamples := copyStringMap(plate.Samples)
	outRoles := copyStringMap(plate.Roles)
	outFactors := copyNestedFactorMap(plate.Factors)

	var res PlateAnnotationResult

	ensureAssignedGroup := func(defaultLabel, inferredRole string) {
		var needGroup []string
		wellOwner := invertGroupsMap(outGroups)
		for _, w := range selected {
			if wellOwner[w] == "" {
				needGroup = append(needGroup, w)
			}
		}
		if len(needGroup) == 0 {
			return
		}
		gid := firstExistingGroupID(plate, doc)
		if gid == "" {
			gid = createGroupInResult(doc, defaultLabel, inferredRole, &res)
		}
		outGroups[gid] = appendSortedUnique(outGroups[gid], needGroup...)
	}

	prefix, rest, hasColon := splitAnnotationPrefix(trimmed)
	if hasColon {
		pLow := strings.ToLower(prefix)
		switch {
		case pLow == "样本" || pLow == "sample":
			tokens := splitAnnotationTokens(rest)
			if len(tokens) == 0 {
				return PlateAnnotationResult{}, fmt.Errorf("请填写样本名称")
			}
			ensureAssignedGroup("常规样本", "sample")
			for i, w := range selected {
				sid := tokens[0]
				if len(tokens) > 1 {
					sid = tokens[i%len(tokens)]
				}
				outSamples[w] = sid
			}
			res.PreferredColorBy = "sample"
			res.AppliedSummary = fmt.Sprintf("样本已标至 %d 孔", len(selected))

		case pLow == "角色" || pLow == "role":
			roleCode := canonicalRoleAlias(rest)
			if roleCode == "" {
				return PlateAnnotationResult{}, fmt.Errorf("未知孔角色 %q（支持 空白/NTC/标准品/阳性对照/阴性对照/常规样本）", rest)
			}
			ensureAssignedGroup(plateui.RoleLabel(roleCode), inferredGroupRoleForWellRole(roleCode))
			for _, w := range selected {
				if roleCode == "sample" {
					delete(outRoles, w)
				} else {
					outRoles[w] = roleCode
				}
			}
			res.PreferredColorBy = "role"
			res.AppliedSummary = fmt.Sprintf("孔角色 %s 已标至 %d 孔", plateui.RoleLabel(roleCode), len(selected))

		default:
			dim := strings.TrimSpace(prefix)
			if dim == "" {
				return PlateAnnotationResult{}, fmt.Errorf("叠加维度名不能为空")
			}
			ensureAssignedGroup("实验组", "treatment")
			if outFactors[dim] == nil {
				outFactors[dim] = map[string]string{}
			}
			valSpec := strings.TrimSpace(rest)
			if valSpec == "" {
				for _, w := range selected {
					delete(outFactors[dim], w)
				}
				if len(outFactors[dim]) == 0 {
					delete(outFactors, dim)
				}
			} else if gradVals, ok := expandGradientTokens(valSpec, len(selected)); ok {
				for i, w := range selected {
					outFactors[dim][w] = gradVals[i]
				}
			} else {
				tokens := splitAnnotationTokens(valSpec)
				for i, w := range selected {
					val := valSpec
					if len(tokens) > 1 {
						val = tokens[i%len(tokens)]
					}
					outFactors[dim][w] = val
				}
			}
			res.PreferredColorBy = "factor"
			res.PreferredFactorKey = dim
			res.AppliedSummary = fmt.Sprintf("已叠加 %s 到 %d 孔", dim, len(selected))
		}
	} else if gradVals, ok := expandGradientTokens(trimmed, len(selected)); ok {
		gid := resolveGroupMatch(doc, plate, trimmed)
		if gid == "" {
			gid = createGroupInResult(doc, trimmed, "treatment", &res)
		}
		if outFactors["梯度浓度"] == nil {
			outFactors["梯度浓度"] = map[string]string{}
		}
		for g, ws := range outGroups {
			outGroups[g] = removeWellsFromList(ws, selected)
		}
		outGroups[gid] = appendSortedUnique(outGroups[gid], selected...)
		for i, w := range selected {
			outSamples[w] = gradVals[i]
			outFactors["梯度浓度"][w] = gradVals[i]
			delete(outRoles, w)
		}
		res.PreferredColorBy = "group"
		res.AppliedSummary = fmt.Sprintf("已为 %d 孔生成梯度稀释（%s）", len(selected), trimmed)
	} else {
		qcRole := canonicalRoleAlias(trimmed)
		if qcRole == "sample" {
			qcRole = ""
		}
		gid := resolveGroupMatch(doc, plate, trimmed)
		if gid == "" {
			gid = createGroupInResult(doc, trimmed, inferredGroupRoleForWellRole(qcRole), &res)
		}
		for g, ws := range outGroups {
			outGroups[g] = removeWellsFromList(ws, selected)
		}
		outGroups[gid] = appendSortedUnique(outGroups[gid], selected...)
		for _, w := range selected {
			outSamples[w] = trimmed
			if qcRole != "" {
				outRoles[w] = qcRole
			} else {
				delete(outRoles, w)
			}
		}
		res.PreferredColorBy = "group"
		res.AppliedSummary = fmt.Sprintf("已将 %d 孔标注为 %s（自动生成复孔）", len(selected), trimmed)
	}

	outReplicates := recomputePlateReplicates(geometry, outSamples, outFactors)
	res.PlateProps = map[string]any{
		"groups":     outGroups,
		"samples":    outSamples,
		"replicates": outReplicates,
		"roles":      outRoles,
		"factors":    outFactors,
	}
	return res, nil
}

// AppendPlateAnnotation overlays an additional tag onto existing well labels
// (e.g. `WT` + `GAPDH` -> `WT · GAPDH`) without requiring `dim:val` colon
// syntax, updating structural groups, orthogonal factors and replicates cleanly.
func AppendPlateAnnotation(doc *Document, plate *Plate, wells []string, extraTag string) (PlateAnnotationResult, error) {
	if plate == nil {
		return PlateAnnotationResult{}, fmt.Errorf("未找到目标孔板模块")
	}
	extra := strings.TrimSpace(extraTag)
	if extra == "" {
		return PlateAnnotationResult{}, fmt.Errorf("请输入要追加叠加的标签名称")
	}
	if prefix, _, hasColon := splitAnnotationPrefix(extra); hasColon && prefix != "" {
		return ApplyUnifiedPlateAnnotation(doc, plate, wells, extra)
	}
	geometry, ok := geometryForPlate(plate.Format, plate.Topology, plate.Rows, plate.Columns)
	if !ok {
		return PlateAnnotationResult{}, fmt.Errorf("容器规格 %d 未注册", plate.Format)
	}
	topo := strings.TrimSpace(plate.Topology)
	if topo == "" {
		topo = "plate"
	}
	var selected []string
	seenSel := map[string]bool{}
	for _, w := range wells {
		nw := strings.ToUpper(strings.TrimSpace(w))
		if nw == "" || seenSel[nw] {
			continue
		}
		if !ValidPlateWell(nw, topo, geometry.Rows, geometry.Columns) {
			return PlateAnnotationResult{}, fmt.Errorf("孔位 %q 超出 %dx%d 范围", nw, geometry.Rows, geometry.Columns)
		}
		seenSel[nw] = true
		selected = append(selected, nw)
	}
	if len(selected) == 0 {
		return PlateAnnotationResult{}, fmt.Errorf("请先框选或点击要叠加标注的孔位")
	}
	sort.Slice(selected, func(i, j int) bool {
		return wellLess(geometry, selected[i], selected[j])
	})

	outGroups := copyWellGroupsMap(plate.Groups)
	outSamples := copyStringMap(plate.Samples)
	outRoles := copyStringMap(plate.Roles)
	outFactors := copyNestedFactorMap(plate.Factors)
	wellOwner := invertGroupsMap(outGroups)
	groupLabels := PlateGroupLabelMap(doc)

	gradVals, isGrad := expandGradientTokens(extra, len(selected))
	if outFactors["追加标签"] == nil {
		outFactors["追加标签"] = map[string]string{}
	}

	var res PlateAnnotationResult
	createdGroups := map[string]string{}
	for i, w := range selected {
		tag := extra
		if isGrad {
			tag = gradVals[i]
		}
		existing := strings.TrimSpace(outSamples[w])
		if existing == "" {
			if ownerGID := wellOwner[w]; ownerGID != "" {
				existing = firstNonEmpty(groupLabels[ownerGID], ownerGID)
			}
		}
		combined := tag
		if existing != "" {
			already := false
			for _, part := range strings.Split(existing, "·") {
				if strings.EqualFold(strings.TrimSpace(part), tag) {
					already = true
					break
				}
			}
			if already {
				combined = existing
			} else {
				combined = existing + " · " + tag
			}
		}
		outSamples[w] = combined
		if prev := strings.TrimSpace(outFactors["追加标签"][w]); prev == "" {
			outFactors["追加标签"][w] = tag
		} else if !strings.Contains(strings.ToLower(prev), strings.ToLower(tag)) {
			outFactors["追加标签"][w] = prev + " · " + tag
		}

		// Ensure each distinct combined label owns its own color group cleanly.
		gid := createdGroups[combined]
		if gid == "" {
			gid = resolveGroupMatch(doc, plate, combined)
		}
		if gid == "" {
			role := "treatment"
			if wellOwner[w] != "" && existing == "" {
				gid = wellOwner[w]
			} else {
				gid = createGroupInResultWithSeed(doc, &res, createdGroups, combined, role)
				createdGroups[combined] = gid
			}
		}
		for g, ws := range outGroups {
			outGroups[g] = removeWellsFromList(ws, []string{w})
		}
		outGroups[gid] = appendSortedUnique(outGroups[gid], w)
	}

	outReplicates := recomputePlateReplicates(geometry, outSamples, outFactors)
	res.PreferredColorBy = "group"
	res.AppliedSummary = fmt.Sprintf("已在 %d 孔追加标签「%s」", len(selected), extra)
	res.PlateProps = map[string]any{
		"groups":     outGroups,
		"samples":    outSamples,
		"replicates": outReplicates,
		"roles":      outRoles,
		"factors":    outFactors,
	}
	return res, nil
}

func createGroupInResultWithSeed(doc *Document, res *PlateAnnotationResult, extraCreated map[string]string, name, role string) string {
	used := map[string]bool{}
	if doc != nil {
		for _, g := range doc.GroupList() {
			used[g.ID] = true
		}
		flat, _ := flattenModules(doc.Modules)
		for _, m := range flat {
			used[m.ID] = true
		}
	}
	for _, gid := range extraCreated {
		used[gid] = true
	}
	for _, r := range res.UpdateGroupTableRows {
		if rm, ok := r.(map[string]any); ok {
			if id, _ := rm["id"].(string); id != "" {
				used[id] = true
			}
		}
	}
	base := "g-" + sanitizeIDFrag(name)
	gid := base
	for n := 2; used[gid]; n++ {
		gid = fmt.Sprintf("%s-%d", base, n)
	}
	if doc != nil {
		for _, m := range doc.Modules {
			if m.Kind == "group_table" {
				var rows []any
				if res.UpdateGroupTableID == m.ID && len(res.UpdateGroupTableRows) > 0 {
					rows = append(rows, res.UpdateGroupTableRows...)
				} else {
					rows = append(rows, rawAnySlice(m.Data["rows"])...)
				}
				rows = append(rows, map[string]any{"id": gid, "name": name, "role": role})
				res.UpdateGroupTableID = m.ID
				res.UpdateGroupTableRows = rows
				return gid
			}
			if m.Kind == "step" {
				for _, bm := range mapSlice(m.Data["blocks"]) {
					if textValue(bm, "kind") == "group_table" {
						bid := textValue(bm, "id")
						var rows []any
						if res.UpdateGroupTableID == bid && len(res.UpdateGroupTableRows) > 0 {
							rows = append(rows, res.UpdateGroupTableRows...)
						} else {
							rows = append(rows, rawAnySlice(bm["rows"])...)
						}
						rows = append(rows, map[string]any{"id": gid, "name": name, "role": role})
						res.UpdateGroupTableID = bid
						res.UpdateGroupTableStep = m.ID
						res.UpdateGroupTableRows = rows
						return gid
					}
				}
			}
		}
	}
	res.NewGroupModule = map[string]any{"kind": "group", "id": gid, "schema": 1, "name": name, "role": role}
	return gid
}

func (v *View) applyUnifiedPlateInput(plate *Plate, ownerStepID string, wells []string, rawInput string) error {
	if plate == nil {
		return nil
	}
	res, err := ApplyUnifiedPlateAnnotation(v.Doc, plate, wells, rawInput)
	if err != nil {
		return err
	}
	return v.commitPlateAnnotationResult(plate, ownerStepID, res)
}

func (v *View) appendPlateInput(plate *Plate, ownerStepID string, wells []string, extraTag string) error {
	if plate == nil {
		return nil
	}
	res, err := AppendPlateAnnotation(v.Doc, plate, wells, extraTag)
	if err != nil {
		return err
	}
	return v.commitPlateAnnotationResult(plate, ownerStepID, res)
}

func (v *View) commitPlateAnnotationResult(plate *Plate, ownerStepID string, res PlateAnnotationResult) error {
	if v.plateColorBy == nil {
		v.plateColorBy = map[string]string{}
	}
	if v.plateFactorKey == nil {
		v.plateFactorKey = map[string]string{}
	}
	if res.PreferredColorBy != "" {
		v.plateColorBy[plate.ID] = res.PreferredColorBy
	}
	if res.PreferredFactorKey != "" {
		v.plateFactorKey[plate.ID] = res.PreferredFactorKey
	}

	// Host with Rust Core callbacks (Design UI)
	if v.OnChange == nil {
		if res.UpdateGroupTableID != "" {
			if res.UpdateGroupTableStep != "" && v.OnUpdateStepBlock != nil {
				v.OnUpdateStepBlock(res.UpdateGroupTableStep, res.UpdateGroupTableID, map[string]any{"rows": res.UpdateGroupTableRows})
			} else if v.OnUpdateModule != nil {
				v.OnUpdateModule(res.UpdateGroupTableID, map[string]any{"rows": res.UpdateGroupTableRows})
			}
		} else if res.NewGroupModule != nil && v.OnAddModule != nil {
			v.OnAddModule(res.NewGroupModule)
		}
		return v.applyPlateProps(plate.ID, ownerStepID, res.PlateProps)
	}

	// Direct-source host (tests, Notebook asterdoc editor)
	if v.Doc != nil && v.Can(PermDocumentEdit) {
		if res.UpdateGroupTableID != "" {
			if res.UpdateGroupTableStep != "" {
				if err := v.updateStepBlockInDoc(res.UpdateGroupTableStep, res.UpdateGroupTableID, map[string]any{"rows": res.UpdateGroupTableRows}); err != nil {
					return err
				}
			} else if _, err := v.Doc.UpdateModule(res.UpdateGroupTableID, map[string]any{"rows": res.UpdateGroupTableRows}); err != nil {
				return err
			}
		} else if res.NewGroupModule != nil {
			if err := insertModuleInDocBeforeSteps(v.Doc, res.NewGroupModule); err != nil {
				return err
			}
		}
		if err := v.applyPlateProps(plate.ID, ownerStepID, res.PlateProps); err != nil {
			return err
		}
		if v.OnChange != nil {
			v.OnChange(v.Doc.Source)
		}
	}
	return nil
}

func (v *View) renderPlateFloatingBar(
	c *ui.Context,
	plate *Plate,
	ownerStepID string,
	selected []string,
	setSelected func([]string),
	inOverlay bool,
	zoom float32,
	closeOverlay func(),
) {
	tok := theme.GetTokens(c.Theme())
	canEdit := v.Can(PermDocumentEdit) && !v.ReadOnly
	groups := plate.GroupNames()
	groupLabels := make(map[string]string, len(groups))
	if v.Doc != nil {
		for _, g := range v.Doc.GroupList() {
			if g.Name != "" {
				groupLabels[g.ID] = g.Name
			}
		}
	}

	if v.plateInputDrafts == nil {
		v.plateInputDrafts = map[string]*string{}
	}
	draft := v.plateInputDrafts[plate.ID]
	if draft == nil {
		empty := ""
		draft = &empty
		v.plateInputDrafts[plate.ID] = draft
	}
	if v.SelectedGroup == "" && len(groups) > 0 {
		v.SelectedGroup = groups[0]
	}

	applyText := func() {
		if len(selected) == 0 {
			v.PlateSampleError = "请先框选或点击要标注的孔位"
			return
		}
		if err := v.applyUnifiedPlateInput(plate, ownerStepID, selected, *draft); err != nil {
			v.PlateSampleError = err.Error()
			return
		}
		v.PlateSampleError = ""
		*draft = ""
		setSelected(nil)
	}
	appendText := func() {
		if len(selected) == 0 {
			v.PlateSampleError = "请先选择要叠加标签的孔位"
			return
		}
		if err := v.appendPlateInput(plate, ownerStepID, selected, *draft); err != nil {
			v.PlateSampleError = err.Error()
			return
		}
		v.PlateSampleError = ""
		*draft = ""
		setSelected(nil)
	}
	assignQuickGroup := func(gid string) {
		if len(selected) == 0 || gid == "" {
			return
		}
		if v.OnAssignWells != nil && ownerStepID == "" {
			v.OnAssignWells(plate.ID, gid, append([]string(nil), selected...))
		} else {
			if err := v.applyUnifiedPlateInput(plate, ownerStepID, selected, gid); err != nil {
				v.PlateSampleError = err.Error()
				return
			}
		}
		v.PlateSampleError = ""
		setSelected(nil)
	}
	clearSelected := func() {
		if len(selected) == 0 {
			return
		}
		if v.OnClearWells != nil && ownerStepID == "" {
			v.OnClearWells(plate.ID, append([]string(nil), selected...))
		} else {
			_ = v.assign(selected, "")
		}
		v.PlateSampleError = ""
		setSelected(nil)
	}

	if v.UnifiedLive {
		v.renderUnifiedPlateFloatingBar(c, plate, ownerStepID, selected, setSelected, inOverlay, zoom, closeOverlay)
		return
	}

	bg := tok.Elevated
	borderCol := tok.BorderStrong
	if inOverlay {
		bg = tok.Surface
		borderCol = tok.Border
	}
	barRadius := float32(tok.DialogRadius)
	if !inOverlay {
		barRadius = float32(tok.PanelRadius)
	}
	box := ui.Column(c).FillWidth().Padding(float32(tok.Spacing12), float32(tok.Spacing16)).
		Radius(barRadius).Background(bg).Border(1, borderCol).Gap(float32(tok.Spacing8))
	if inOverlay {
		box.Shadow(0, float32(tok.Spacing8), float32(tok.Spacing24), 0, tok.Text.Alpha(.22))
	}
	box.Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			if len(selected) > 0 {
				components.Badge(c, fmt.Sprintf("已选 %d 孔", len(selected)), components.BadgeOptions{Tone: components.BadgeAccent})
				ui.Text(c, summarizeWellRange(selected)).FontSize(float32(tok.TextMeta)).Bold().
					TextColor(tok.TextMuted).FontFeatures("tnum")
			} else {
				components.Badge(c, "框选孔位后标注")
			}

			if canEdit {
				ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*4).Children(func() {
					tb := components.TextBox(c, draft, components.TextBoxOptions{}).
						Placeholder("输入处理、样本、引物或梯度（如 WT、GAPDH、100/2 uM、NTC）")
					tb.OnTextKey(func(k ui.TextKey) bool {
						if k.Key == ui.KeyEnter && k.Mods == 0 && len(selected) > 0 && strings.TrimSpace(*draft) != "" {
							applyText()
							return true
						}
						return false
					})
				})
				hasDraft := strings.TrimSpace(*draft) != ""
				applyBtn := components.Button(c, "标注", components.ButtonPrimary)
				applyBtn.Disabled(len(selected) == 0 || !hasDraft)
				if applyBtn.Clicked() {
					applyText()
				}
				appendBtn := components.Button(c, "+ 追加标签", components.ButtonSecondary)
				components.Tooltip(c, appendBtn, "在所选孔已有标签后叠加新标签（例如先标 WT，再选列追加 GAPDH）")
				appendBtn.Disabled(len(selected) == 0 || !hasDraft)
				if appendBtn.Clicked() {
					appendText()
				}
				clearBtn := components.Button(c, "清空", components.ButtonGhost)
				clearBtn.Disabled(len(selected) == 0)
				if clearBtn.Clicked() {
					clearSelected()
				}
			}

			if !inOverlay && canEdit && len(selected) > 0 {
				if components.IconButton(c, lucide.X(), "取消选区", components.IconButtonCompact).Clicked() {
					setSelected(nil)
				}
			}

			if inOverlay {
				ui.Box(c).Width(1).Height(float32(tok.Spacing16)).Background(tok.Border)
				if components.IconButton(c, lucide.ZoomOut(), "缩小孔板", components.IconButtonCompact).Clicked() && zoom > 0.6 {
					v.PlateZoom = zoom - 0.1
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", zoom*100)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
				if components.IconButton(c, lucide.ZoomIn(), "放大", components.IconButtonCompact).Clicked() && zoom < 1.6 {
					v.PlateZoom = zoom + 0.1
				}
				if components.IconButton(c, lucide.RotateCcw(), "重置缩放", components.IconButtonCompact).Clicked() {
					v.PlateZoom = 1
				}
				if components.IconButton(c, lucide.X(), "关闭放大孔板", components.IconButtonCompact).Clicked() && closeOverlay != nil {
					closeOverlay()
				}
			}
		})

		if canEdit && len(groups) > 0 {
			idxMap := plateGroupPaletteMap(v.Doc, plate)
			dark := c.Theme() != nil && c.Theme().Dark
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				ui.Text(c, "已有分组快选：").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				for _, gid := range groups {
					gid := gid
					lbl := firstNonEmpty(groupLabels[gid], gid)
					chip := ui.ButtonBase(c).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
						Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border).
						Label("归入分组 " + lbl)
					chip.Children(func() {
						ui.Row(c).Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
							ui.Box(c).Size(float32(tok.Spacing8), float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
								Background(plateui.GroupColorIndexed(gid, idxMap, dark))
							ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
						})
					})
					if chip.Clicked() {
						v.SelectedGroup = gid
						if len(selected) > 0 {
							assignQuickGroup(gid)
						}
					}
				}
			})
		}

		if v.PlateSampleError != "" {
			ui.Text(c, v.PlateSampleError).FontSize(float32(tok.TextMeta)).TextColor(tok.Danger)
		}
	})
}

func (v *View) renderUnifiedPlateFloatingBar(
	c *ui.Context,
	plate *Plate,
	ownerStepID string,
	selected []string,
	setSelected func([]string),
	inOverlay bool,
	zoom float32,
	closeOverlay func(),
) {
	tok := theme.GetTokens(c.Theme())
	canEdit := v.Can(PermDocumentEdit) && !v.ReadOnly
	colorBy := v.plateColorBy[plate.ID]
	if colorBy == "" {
		colorBy = "group"
	}
	activeLayer := "分组"
	isLayerMode := colorBy == "factor" && v.plateFactorKey[plate.ID] != ""
	if isLayerMode {
		activeLayer = v.plateFactorKey[plate.ID]
	}

	if v.plateFillMode == nil {
		v.plateFillMode = map[string]int{}
	}
	fillMode := v.plateFillMode[plate.ID]

	if v.plateInputDrafts == nil {
		v.plateInputDrafts = map[string]*string{}
	}
	draft := v.plateInputDrafts[plate.ID]
	if draft == nil {
		empty := ""
		draft = &empty
		v.plateInputDrafts[plate.ID] = draft
	}
	startDraft := v.ensureWidgetDraft("pl_s:"+plate.ID, "100")
	unitDraft := v.ensureWidgetDraft("pl_u:"+plate.ID, "uM")
	factorDraft := v.ensureWidgetDraft("pl_f:"+plate.ID, "2")
	opDraft := v.ensureWidgetDraft("pl_op:"+plate.ID, "/")
	dirDraft := v.ensureWidgetDraft("pl_dir:"+plate.ID, "row")

	writeLayerValues := func(layerName string, values map[string]string) {
		nextLayers := copyNestedFactorMap(plate.Layers)
		if nextLayers[layerName] == nil {
			nextLayers[layerName] = map[string]string{}
		}
		for w, val := range values {
			if strings.TrimSpace(val) == "" {
				delete(nextLayers[layerName], w)
			} else {
				nextLayers[layerName][w] = val
			}
		}
		nextFactors := copyNestedFactorMap(plate.Factors)
		if nextFactors[layerName] == nil {
			nextFactors[layerName] = map[string]string{}
		}
		for w, val := range values {
			if strings.TrimSpace(val) == "" {
				delete(nextFactors[layerName], w)
			} else {
				nextFactors[layerName][w] = val
			}
		}
		v.emitAnyModuleEdit(plate.ID, map[string]any{"layers": nextLayers, "factors": nextFactors})
		v.plateColorBy[plate.ID] = "factor"
		v.plateFactorKey[plate.ID] = layerName
	}

	applySingleOrList := func() {
		if len(selected) == 0 || strings.TrimSpace(*draft) == "" {
			return
		}
		if fillMode == 1 {
			tokens := splitAnnotationTokens(*draft)
			if len(tokens) == 0 {
				return
			}
			geom, _ := geometryForPlate(plate.Format, plate.Topology, plate.Rows, plate.Columns)
			sorted := sortWellsByDir(geom, selected, *dirDraft)
			if isLayerMode {
				vals := map[string]string{}
				for i, w := range sorted {
					vals[w] = tokens[i%len(tokens)]
				}
				writeLayerValues(activeLayer, vals)
			} else {
				for i, w := range sorted {
					_ = v.applyUnifiedPlateInput(plate, ownerStepID, []string{w}, tokens[i%len(tokens)])
				}
			}
			v.PlateSampleError = ""
			*draft = ""
			setSelected(nil)
			return
		}
		if isLayerMode {
			vals := map[string]string{}
			for _, w := range selected {
				vals[w] = strings.TrimSpace(*draft)
			}
			writeLayerValues(activeLayer, vals)
			v.PlateSampleError = ""
			*draft = ""
			setSelected(nil)
			return
		}
		if err := v.applyUnifiedPlateInput(plate, ownerStepID, selected, *draft); err != nil {
			v.PlateSampleError = err.Error()
			return
		}
		v.PlateSampleError = ""
		*draft = ""
		setSelected(nil)
	}

	nudgeSeq := ui.Local(c.Root(), "pl-nudge:"+plate.ID, func() int { return 0 })

	applySerial := func() {
		if len(selected) == 0 {
			return
		}
		geom, _ := geometryForPlate(plate.Format, plate.Topology, plate.Rows, plate.Columns)
		sorted := sortWellsByDir(geom, selected, *dirDraft)
		startVal, err1 := strconv.ParseFloat(strings.TrimSpace(*startDraft), 64)
		facVal, err2 := strconv.ParseFloat(strings.TrimSpace(*factorDraft), 64)
		if err1 != nil || err2 != nil || startVal <= 0 || facVal <= 0 {
			*nudgeSeq++
			v.PlateSampleError = "起始值与倍比需为正数"
			return
		}
		layerTarget := activeLayer
		if !isLayerMode {
			layerTarget = "浓度梯度"
		}
		vals := map[string]string{}
		cur := startVal
		for _, w := range sorted {
			lbl := formatNumber(round4(cur))
			if strings.TrimSpace(*unitDraft) != "" {
				lbl += " " + strings.TrimSpace(*unitDraft)
			}
			vals[w] = lbl
			if *opDraft == "*" {
				cur *= facVal
			} else {
				cur /= facVal
			}
		}
		writeLayerValues(layerTarget, vals)
		v.PlateSampleError = ""
		setSelected(nil)
	}

	// Note: omit FloatingBarTransition on pl-float-bar because it hosts components.Tabs
	// (TabsSegmented), whose slideBehind indicator reads previous-pass element bounds and
	// panics during exit-transition snapshots when the selection clears.
	box := ui.Column(c.Key("pl-float-bar:"+plate.ID)).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		Radius(float32(tok.PanelRadius)).Background(tok.Elevated).Border(1, tok.BorderStrong).Gap(float32(tok.Spacing8))
	theme.ApplyNudge(box, *nudgeSeq)
	if inOverlay {
		box.Background(tok.Surface).Shadow(0, float32(tok.Spacing8), float32(tok.Spacing24), 0, tok.Text.Alpha(.22))
	}
	box.Children(func() {
		ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			components.Badge(c, fmt.Sprintf("已选 %d 孔 · 图层「%s」", len(selected), activeLayer), components.BadgeOptions{Tone: components.BadgeAccent})
			fmIdx := fillMode
			components.Tabs(c, &fmIdx, components.TabsSegmented, "单值", "平铺列表", "等比序列")
			if fmIdx != fillMode {
				v.plateFillMode[plate.ID] = fmIdx
			}
			ui.Spacer(c)
			if inOverlay {
				if components.IconButton(c, lucide.ZoomOut(), "缩小", components.IconButtonCompact).Clicked() && zoom > 0.6 {
					v.PlateZoom = zoom - 0.1
				}
				ui.Text(c, fmt.Sprintf("%.0f%%", zoom*100)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
				if components.IconButton(c, lucide.ZoomIn(), "放大", components.IconButtonCompact).Clicked() && zoom < 1.6 {
					v.PlateZoom = zoom + 0.1
				}
				if components.IconButton(c, lucide.X(), "关闭放大", components.IconButtonCompact).Clicked() && closeOverlay != nil {
					closeOverlay()
				}
			} else {
				if components.IconButton(c, lucide.X(), "取消选区", components.IconButtonCompact).Clicked() {
					setSelected(nil)
				}
			}
		})

		if canEdit {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
				if fillMode == 2 {
					ui.Text(c, "起始").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					ui.Box(c).Width(float32(tok.Spacing48)*1.5).Children(func() {
						components.TextBox(c, startDraft, components.TextBoxOptions{})
					})
					ui.Box(c).Width(float32(tok.Spacing48)*1.2).Children(func() {
						components.TextBox(c, unitDraft, components.TextBoxOptions{}).Placeholder("单位")
					})
					for _, preset := range []struct {
						lbl string
						op  string
						fac string
					}{{"÷2", "/", "2"}, {"÷5", "/", "5"}, {"÷10", "/", "10"}, {"×2", "*", "2"}} {
						preset := preset
						stBtn := components.ButtonOutline
						if *opDraft == preset.op && *factorDraft == preset.fac {
							stBtn = components.ButtonSecondary
						}
						if components.Button(c, preset.lbl, stBtn).Clicked() {
							*opDraft = preset.op
							*factorDraft = preset.fac
						}
					}
					dirIdx := 0
					if *dirDraft == "col" {
						dirIdx = 1
					}
					components.Tabs(c, &dirIdx, components.TabsSegmented, "按行", "按列")
					if dirIdx == 1 {
						*dirDraft = "col"
					} else {
						*dirDraft = "row"
					}
					if components.Button(c, "生成梯度", components.ButtonPrimary).Clicked() {
						applySerial()
					}
				} else {
					ph := "输入值（如 WT、Vehicle、10uM），按 Enter 应用"
					if fillMode == 1 {
						ph = "输入逗号分隔列表（如 DMSO, 1uM, 10uM, 100uM）依次铺入选中孔"
					}
					ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*4).Children(func() {
						tb := components.TextBox(c, draft, components.TextBoxOptions{}).Placeholder(ph)
						tb.OnTextKey(func(k ui.TextKey) bool {
							if k.Key == ui.KeyEnter && k.Mods == 0 && strings.TrimSpace(*draft) != "" {
								applySingleOrList()
								return true
							}
							return false
						})
					})
					if fillMode == 1 {
						dirIdx := 0
						if *dirDraft == "col" {
							dirIdx = 1
						}
						components.Tabs(c, &dirIdx, components.TabsSegmented, "按行", "按列")
						if dirIdx == 1 {
							*dirDraft = "col"
						} else {
							*dirDraft = "row"
						}
					}
					if components.Button(c, "应用", components.ButtonPrimary).Clicked() && strings.TrimSpace(*draft) != "" {
						applySingleOrList()
					}
				}
			})
		}

		if canEdit && !isLayerMode {
			var groups []string
			seenGrp := map[string]bool{}
			if v.Doc != nil {
				for _, g := range v.Doc.GroupList() {
					if g.ID != "" && !seenGrp[g.ID] {
						seenGrp[g.ID] = true
						groups = append(groups, g.ID)
					}
				}
			}
			for _, gid := range plate.GroupNames() {
				if gid != "" && !seenGrp[gid] {
					seenGrp[gid] = true
					groups = append(groups, gid)
				}
			}
			idxMap := plateGroupPaletteMap(v.Doc, plate)
			lblMap := PlateGroupLabelMap(v.Doc)
			dark := c.Theme() != nil && c.Theme().Dark
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing4)).Children(func() {
				if len(groups) > 0 {
					ui.Text(c, "分组：").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					for _, gid := range groups {
						gid := gid
						lbl := firstNonEmpty(lblMap[gid], gid)
						chip := ui.ButtonBase(c).Padding(float32(tok.Spacing4), float32(tok.Spacing8)).
							Radius(float32(tok.ControlRadius)).Background(tok.Surface).Border(1, tok.Border)
						chip.Children(func() {
							ui.Row(c).Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
								ui.Box(c).Size(float32(tok.Spacing8), float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
									Background(plateui.GroupColorIndexed(gid, idxMap, dark))
								ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
							})
						})
						if chip.Clicked() {
							_ = v.applyUnifiedPlateInput(plate, ownerStepID, selected, gid)
							setSelected(nil)
						}
					}
				}
				// Compact scientific well role pills directly on selection bar (L1 progressive disclosure)
				if components.Button(c, "空白(PBS)", components.ButtonGhost).Clicked() {
					_ = v.applyUnifiedPlateInput(plate, ownerStepID, selected, "空白")
					setSelected(nil)
				}
				if components.Button(c, "异常孔", components.ButtonGhost).Clicked() {
					v.togglePlateExcludedWells(plate, selected)
					setSelected(nil)
				}
			})
		}

		if v.PlateSampleError != "" {
			ui.Text(c, v.PlateSampleError).FontSize(float32(tok.TextMeta)).TextColor(tok.Danger)
		}
	})
}

func sortWellsByDir(geom Geometry, wells []string, dir string) []string {
	out := append([]string(nil), wells...)
	sort.Slice(out, func(i, j int) bool {
		ri, ci, ok1 := wellRowColParsed(out[i])
		rj, cj, ok2 := wellRowColParsed(out[j])
		if !ok1 || !ok2 {
			return out[i] < out[j]
		}
		if dir == "col" {
			if ci != cj {
				return ci < cj
			}
			return ri < rj
		}
		if ri != rj {
			return ri < rj
		}
		return ci < cj
	})
	_ = geom
	return out
}

func wellRowColParsed(well string) (int, int, bool) {
	w := strings.ToUpper(strings.TrimSpace(well))
	if len(w) < 2 {
		return 0, 0, false
	}
	row := int(w[0] - 'A')
	col, err := strconv.Atoi(w[1:])
	if err != nil {
		return 0, 0, false
	}
	return row, col - 1, true
}

func invertPlateSelection(geom Geometry, selected []string) []string {
	selMap := map[string]bool{}
	for _, w := range selected {
		selMap[strings.ToUpper(w)] = true
	}
	var out []string
	for r := 0; r < geom.Rows; r++ {
		for col := 0; col < geom.Columns; col++ {
			id := fmt.Sprintf("%c%d", 'A'+r, col+1)
			if !selMap[id] {
				out = append(out, id)
			}
		}
	}
	return out
}

func edgePlateWells(geom Geometry) []string {
	var out []string
	for r := 0; r < geom.Rows; r++ {
		for col := 0; col < geom.Columns; col++ {
			if r == 0 || r == geom.Rows-1 || col == 0 || col == geom.Columns-1 {
				out = append(out, fmt.Sprintf("%c%d", 'A'+r, col+1))
			}
		}
	}
	return out
}

func (v *View) copyPlateWellAnnotations(p *Plate, selected []string) {
	sharedWellClipboard = nil
	if len(selected) == 0 {
		return
	}
	wellOwner := invertGroupsMap(p.Groups)
	activeLayer := v.plateFactorKey[p.ID]
	minR, minC := 1<<30, 1<<30
	type parsedWell struct {
		w string
		r int
		c int
	}
	var parsed []parsedWell
	for _, w := range selected {
		r, col, ok := wellRowColParsed(w)
		if !ok {
			continue
		}
		if r < minR {
			minR = r
		}
		if col < minC {
			minC = col
		}
		parsed = append(parsed, parsedWell{w: w, r: r, c: col})
	}
	for _, pw := range parsed {
		cell := wellClipboardCell{
			dRow:    pw.r - minR,
			dCol:    pw.c - minC,
			group:   wellOwner[pw.w],
			sample:  p.Samples[pw.w],
			role:    p.Roles[pw.w],
			factors: copyStringMap(p.WellFactors(pw.w)),
		}
		if v.plateColorBy[p.ID] == "factor" && activeLayer != "" {
			if lv := p.Layers[activeLayer][pw.w]; lv != "" {
				if cell.factors == nil {
					cell.factors = map[string]string{}
				}
				cell.factors[activeLayer] = lv
			}
		}
		sharedWellClipboard = append(sharedWellClipboard, cell)
	}
}

func (v *View) pastePlateWellAnnotations(p *Plate, ownerStepID string, target []string) {
	if len(sharedWellClipboard) == 0 || len(target) == 0 {
		return
	}
	activeLayer := v.plateFactorKey[p.ID]
	if v.plateColorBy[p.ID] == "factor" && activeLayer != "" {
		nextLayers := copyNestedFactorMap(p.Layers)
		if nextLayers[activeLayer] == nil {
			nextLayers[activeLayer] = map[string]string{}
		}
		for i, w := range target {
			cell := sharedWellClipboard[i%len(sharedWellClipboard)]
			if val := cell.factors[activeLayer]; val != "" {
				nextLayers[activeLayer][w] = val
			}
		}
		v.emitAnyModuleEdit(p.ID, map[string]any{"layers": nextLayers})
		return
	}
	for i, w := range target {
		cell := sharedWellClipboard[i%len(sharedWellClipboard)]
		if cell.group != "" {
			_ = v.applyUnifiedPlateInput(p, ownerStepID, []string{w}, cell.group)
		} else if cell.sample != "" {
			_ = v.applyUnifiedPlateInput(p, ownerStepID, []string{w}, cell.sample)
		}
	}
}

func (v *View) togglePlateExcludedWells(p *Plate, selected []string) {
	if len(selected) == 0 {
		return
	}
	cur := map[string]bool{}
	for _, ew := range p.ExcludedWells {
		cur[strings.ToUpper(ew)] = true
	}
	allExcluded := true
	for _, w := range selected {
		if !cur[strings.ToUpper(w)] {
			allExcluded = false
			break
		}
	}
	for _, w := range selected {
		uw := strings.ToUpper(w)
		if allExcluded {
			delete(cur, uw)
		} else {
			cur[uw] = true
		}
	}
	var out []string
	for w := range cur {
		out = append(out, w)
	}
	sort.Strings(out)
	v.emitAnyModuleEdit(p.ID, map[string]any{"excluded_wells": out})
}

func formatPlateAsCSV(p *Plate, geom Geometry) string {
	wellGroups := p.WellGroups()
	var b strings.Builder
	b.WriteString("Well,Group,Sample,Role\n")
	for r := 0; r < geom.Rows; r++ {
		for col := 0; col < geom.Columns; col++ {
			w := fmt.Sprintf("%c%d", 'A'+r, col+1)
			g := wellGroups[w]
			s := p.Samples[w]
			rl := p.Roles[w]
			if g != "" || s != "" || rl != "" {
				b.WriteString(fmt.Sprintf("%s,%s,%s,%s\n", w, g, s, rl))
			}
		}
	}
	return b.String()
}

func summarizeWellRange(wells []string) string {
	if len(wells) == 0 {
		return ""
	}
	if len(wells) == 1 {
		return wells[0]
	}
	if len(wells) <= 3 {
		return strings.Join(wells, ", ")
	}
	return fmt.Sprintf("%s … %s", wells[0], wells[len(wells)-1])
}

// PlateGroupPaletteMap returns deterministic sequential CategoricalPalette
// indices for every group declared in doc or assigned in plate.
func PlateGroupPaletteMap(doc *Document, plate *Plate) map[string]int {
	return plateGroupPaletteMap(doc, plate)
}

// PlateGroupLabelMap maps canonical group IDs declared in doc to their display names.
func PlateGroupLabelMap(doc *Document) map[string]string {
	out := map[string]string{}
	if doc == nil {
		return out
	}
	for _, g := range doc.GroupList() {
		if g.ID != "" && strings.TrimSpace(g.Name) != "" {
			out[g.ID] = strings.TrimSpace(g.Name)
		}
	}
	return out
}

func plateGroupPaletteMap(doc *Document, plate *Plate) map[string]int {
	out := map[string]int{}
	idx := 0
	if doc != nil {
		for _, g := range doc.GroupList() {
			if g.ID != "" {
				if _, seen := out[g.ID]; !seen {
					out[g.ID] = idx
					idx++
				}
			}
		}
	}
	if plate != nil {
		for _, g := range plate.GroupNames() {
			if g != "" {
				if _, seen := out[g]; !seen {
					out[g] = idx
					idx++
				}
			}
		}
	}
	return out
}

func splitAnnotationPrefix(s string) (prefix, rest string, ok bool) {
	idxASCII := strings.IndexByte(s, ':')
	idxWide := strings.IndexRune(s, '：')
	idx := -1
	sepLen := 1
	switch {
	case idxASCII >= 0 && (idxWide < 0 || idxASCII < idxWide):
		idx = idxASCII
	case idxWide >= 0:
		idx = idxWide
		sepLen = len("：")
	}
	if idx <= 0 {
		return "", s, false
	}
	p := strings.TrimSpace(s[:idx])
	r := strings.TrimSpace(s[idx+sepLen:])
	if p == "" {
		return "", s, false
	}
	return p, r, true
}

func splitAnnotationTokens(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\r'
	})
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func canonicalRoleAlias(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "sample", "常规", "常规样本":
		return "sample"
	case "blank", "空白", "空白对照":
		return "blank"
	case "ntc", "无模板阴参", "无模板", "无模板对照":
		return "ntc"
	case "std", "standard", "标准品":
		return "standard"
	case "positive_control", "positive", "pc", "阳性", "阳性对照", "阳参":
		return "positive_control"
	case "negative_control", "negative", "nc", "阴性", "阴性对照", "阴参":
		return "negative_control"
	case "vehicle", "溶剂", "溶剂对照":
		return "vehicle"
	default:
		return ""
	}
}

func inferredGroupRoleForWellRole(wellRole string) string {
	switch wellRole {
	case "blank", "ntc", "negative_control", "vehicle":
		return "control"
	case "positive_control", "standard":
		return "comparator"
	default:
		return "treatment"
	}
}

func expandGradientTokens(spec string, count int) ([]string, bool) {
	if count <= 1 {
		return nil, false
	}
	s := strings.TrimSpace(spec)
	opIdx := strings.IndexAny(s, "/*")
	if opIdx <= 0 {
		return nil, false
	}
	op := s[opIdx]
	left := strings.TrimSpace(s[:opIdx])
	rightFields := strings.Fields(strings.TrimSpace(s[opIdx+1:]))
	if len(rightFields) == 0 {
		return nil, false
	}
	startVal, err1 := strconv.ParseFloat(left, 64)
	factorVal, err2 := strconv.ParseFloat(rightFields[0], 64)
	if err1 != nil || err2 != nil || startVal <= 0 || factorVal <= 0 || math.IsNaN(startVal) || math.IsInf(startVal, 0) {
		return nil, false
	}
	unit := strings.Join(rightFields[1:], " ")
	out := make([]string, count)
	cur := startVal
	for i := 0; i < count; i++ {
		txt := formatNumber(round4(cur))
		if unit != "" {
			txt += " " + unit
		}
		out[i] = txt
		if op == '/' {
			cur /= factorVal
		} else {
			cur *= factorVal
		}
	}
	return out, true
}

func resolveGroupMatch(doc *Document, plate *Plate, label string) string {
	target := strings.TrimSpace(label)
	if target == "" {
		return ""
	}
	if doc != nil {
		for _, g := range doc.GroupList() {
			if strings.EqualFold(g.ID, target) || strings.EqualFold(g.Name, target) {
				return g.ID
			}
		}
	}
	if plate != nil {
		for _, gid := range plate.GroupNames() {
			if strings.EqualFold(gid, target) {
				return gid
			}
		}
	}
	return ""
}

func firstExistingGroupID(plate *Plate, doc *Document) string {
	if plate != nil {
		if names := plate.GroupNames(); len(names) > 0 {
			return names[0]
		}
	}
	if doc != nil {
		if list := doc.GroupList(); len(list) > 0 {
			return list[0].ID
		}
	}
	return ""
}

func createGroupInResult(doc *Document, name, role string, res *PlateAnnotationResult) string {
	if role == "" {
		role = "treatment"
	}
	used := map[string]bool{}
	if doc != nil {
		for _, g := range doc.GroupList() {
			used[g.ID] = true
		}
		flat, _ := flattenModules(doc.Modules)
		for _, m := range flat {
			used[m.ID] = true
		}
	}
	base := "g-" + sanitizeIDFrag(name)
	gid := base
	for n := 2; used[gid]; n++ {
		gid = fmt.Sprintf("%s-%d", base, n)
	}
	// Prefer appending into an existing group_table@1 when present
	if doc != nil {
		for _, m := range doc.Modules {
			if m.Kind == "group_table" {
				rows := append([]any{}, rawAnySlice(m.Data["rows"])...)
				rows = append(rows, map[string]any{"id": gid, "name": name, "role": role})
				res.UpdateGroupTableID = m.ID
				res.UpdateGroupTableRows = rows
				return gid
			}
			if m.Kind == "step" {
				for _, bm := range mapSlice(m.Data["blocks"]) {
					if textValue(bm, "kind") == "group_table" {
						rows := append([]any{}, rawAnySlice(bm["rows"])...)
						rows = append(rows, map[string]any{"id": gid, "name": name, "role": role})
						res.UpdateGroupTableID = textValue(bm, "id")
						res.UpdateGroupTableStep = m.ID
						res.UpdateGroupTableRows = rows
						return gid
					}
				}
			}
		}
	}
	res.NewGroupModule = map[string]any{
		"kind": "group", "id": gid, "schema": 1, "name": name, "role": role,
	}
	return gid
}

func rawAnySlice(v any) []any {
	s, _ := v.([]any)
	return s
}

func recomputePlateReplicates(geometry Geometry, samples map[string]string, factors map[string]map[string]string) map[string]int {
	wells := make([]string, 0, len(samples))
	for w, sid := range samples {
		if strings.TrimSpace(sid) != "" {
			wells = append(wells, strings.ToUpper(strings.TrimSpace(w)))
		}
	}
	sort.Slice(wells, func(i, j int) bool {
		return wellLess(geometry, wells[i], wells[j])
	})
	factorSigs := factorSignatureMap(factors)
	counts := map[string]int{}
	out := make(map[string]int, len(wells))
	for _, w := range wells {
		key := strings.TrimSpace(samples[w]) + "\x00" + factorSigs[w]
		counts[key]++
		out[w] = counts[key]
	}
	return out
}

func copyWellGroupsMap(in map[string][]string) map[string][]string {
	out := make(map[string][]string, len(in))
	for k, ws := range in {
		out[k] = append([]string(nil), ws...)
	}
	return out
}

func copyStringMap(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func copyNestedFactorMap(in map[string]map[string]string) map[string]map[string]string {
	out := make(map[string]map[string]string, len(in))
	for dim, m := range in {
		sub := make(map[string]string, len(m))
		for k, v := range m {
			sub[k] = v
		}
		out[dim] = sub
	}
	return out
}

func invertGroupsMap(groups map[string][]string) map[string]string {
	out := map[string]string{}
	for g, ws := range groups {
		for _, w := range ws {
			out[strings.ToUpper(strings.TrimSpace(w))] = g
		}
	}
	return out
}

func removeWellsFromList(list, drop []string) []string {
	dropSet := make(map[string]bool, len(drop))
	for _, d := range drop {
		dropSet[d] = true
	}
	out := make([]string, 0, len(list))
	for _, w := range list {
		if !dropSet[strings.ToUpper(strings.TrimSpace(w))] {
			out = append(out, w)
		}
	}
	return out
}

func appendSortedUnique(list []string, add ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list)+len(add))
	for _, w := range list {
		nw := strings.ToUpper(strings.TrimSpace(w))
		if nw != "" && !seen[nw] {
			seen[nw] = true
			out = append(out, nw)
		}
	}
	for _, w := range add {
		nw := strings.ToUpper(strings.TrimSpace(w))
		if nw != "" && !seen[nw] {
			seen[nw] = true
			out = append(out, nw)
		}
	}
	sort.Strings(out)
	return out
}

func (v *View) updateStepBlockInDoc(stepID, blockID string, props map[string]any) error {
	if v.Doc == nil {
		return fmt.Errorf("missing document")
	}
	for i := range v.Doc.Modules {
		m := v.Doc.Modules[i]
		if m.Kind != "step" || m.ID != stepID {
			continue
		}
		var data map[string]any
		if err := yaml.Unmarshal([]byte(m.Body), &data); err != nil {
			return err
		}
		seq, _ := data["blocks"].([]any)
		for bi := range seq {
			bm, _ := seq[bi].(map[string]any)
			if textValue(bm, "id") == blockID {
				for k, pv := range props {
					if k != "id" && k != "kind" && k != "schema" {
						bm[k] = pv
					}
				}
				seq[bi] = bm
				break
			}
		}
		data["blocks"] = seq
		raw, err := yaml.Marshal(data)
		if err != nil {
			return err
		}
		nextSrc := v.Doc.Source[:m.Start] + string(raw) + v.Doc.Source[m.End:]
		nextDoc, err := Parse(nextSrc)
		if err != nil {
			return err
		}
		v.Doc.Source = nextSrc
		v.Doc.Modules = nextDoc.Modules
		return nil
	}
	return fmt.Errorf("step %q not found", stepID)
}

func insertModuleInDocBeforeSteps(doc *Document, mod map[string]any) error {
	raw, err := yaml.Marshal(mod)
	if err != nil {
		return err
	}
	fence := "\n```asterlab\n" + string(raw) + "```\n"
	insertPos := len(doc.Source)
	for _, m := range doc.Modules {
		if m.Kind == "step" || m.Kind == "workflow" {
			idx := strings.LastIndex(doc.Source[:m.Start], "```asterlab")
			if idx >= 0 {
				insertPos = idx
				break
			}
		}
	}
	nextSrc := doc.Source[:insertPos] + fence + doc.Source[insertPos:]
	nextDoc, err := Parse(nextSrc)
	if err != nil {
		return err
	}
	doc.Source = nextSrc
	doc.Modules = nextDoc.Modules
	return nil
}
