package asterdoc

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"asterlab-mygo-prototype/internal/scimath"
)

// ValidateFormulaExpr validates arithmetic or LaTeX math syntax (+ - * /, \frac, \sqrt, powers, variables).
func ValidateFormulaExpr(expr string) error {
	return scimath.Validate(expr)
}

// EvalFormula evaluates an arithmetic or LaTeX math formula with provided scalar variables.
func EvalFormula(expr string, vars map[string]float64) (float64, error) {
	return scimath.Eval(expr, vars)
}

// CalcSampleInput supplies one upstream measured sample concentration (or scalar sample fact).
type CalcSampleInput struct {
	SampleID string
	Value    float64
	Unit     string
	Endpoint string
}

// CalcActualContext feeds Run-stage measured parameters and upstream sample measurements into calculator@1.
type CalcActualContext struct {
	Params     map[string]float64
	ParamUnits map[string]string
	Samples    []CalcSampleInput
}

// CalcRow is one computed line item from EvalCalculator.
type CalcRow struct {
	Label  string
	Amount float64
	Unit   string
	Note   string
}

// CalcResult holds the deterministic preview of a calculator@1 module.
type CalcResult struct {
	Mode        string
	Summary     string
	Rows        []CalcRow
	UsedActuals []string
	Err         error
}

// EvalCalculator computes deterministic preview quantities for calculator@1 across all 5 modes.
func EvalCalculator(doc *Document, data map[string]any) CalcResult {
	return EvalCalculatorWithActuals(doc, data, nil)
}

func docPlannedParamFloat(doc *Document, key string) (float64, string, bool) {
	if doc == nil || strings.TrimSpace(key) == "" {
		return 0, "", false
	}
	flat, _ := flattenModules(doc.Modules)
	for _, m := range flat {
		if m.Kind != "param_table" {
			continue
		}
		rows, _ := m.Data["rows"].([]any)
		for _, raw := range rows {
			rm, _ := raw.(map[string]any)
			if strings.EqualFold(textValue(rm, "key"), key) || strings.EqualFold(textValue(rm, "label"), key) {
				if n, ok := numericValue(rm["value"]); ok {
					return n, textValue(rm, "unit"), true
				}
			}
		}
	}
	return 0, "", false
}

func massToNg(val float64, unit string) (float64, bool) {
	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "", "ng":
		return val, true
	case "ug", "µg", "μg":
		return val * 1000.0, true
	case "mg":
		return val * 1e6, true
	default:
		return 0, false
	}
}

func massConcToNgPerUL(val float64, unit string) (float64, bool) {
	u := strings.ReplaceAll(strings.TrimSpace(unit), "μ", "µ")
	switch strings.ToLower(u) {
	case "", "ng/ul", "ng/µl", "ug/ml", "µg/ml":
		return val, true
	case "mg/ml":
		return val * 1000.0, true
	case "pg/ul", "pg/µl", "ng/ml":
		return val * 0.001, true
	default:
		return 0, false
	}
}

func volumeToUL(val float64, unit string) (float64, string, bool) {
	u := strings.ReplaceAll(strings.TrimSpace(unit), "μ", "µ")
	switch strings.ToLower(u) {
	case "", "ul", "µl":
		return val, "uL", true
	case "ml":
		return val * 1000.0, "uL", true
	case "l":
		return val * 1e6, "uL", true
	default:
		return 0, unit, false
	}
}

// UnitDimension classifies a scientific unit via internal/scimath.
func UnitDimension(unit string) (dim string, scale float64, known bool) {
	d, s, k := scimath.DimensionOf(unit)
	return string(d), s, k
}

// ConvertScientificUnit converts val between compatible physical units via internal/scimath.
func ConvertScientificUnit(val float64, fromUnit, toUnit string) (out float64, ok bool, sameDim bool) {
	return scimath.Convert(val, fromUnit, toUnit)
}

func collectCalcContextVars(doc *Document, actuals *CalcActualContext) (map[string]float64, []string) {
	vars, _, used := collectCalcContextVarsWithUnits(doc, actuals)
	return vars, used
}

func collectCalcContextVarsWithUnits(doc *Document, actuals *CalcActualContext) (map[string]float64, map[string]string, []string) {
	vars := map[string]float64{}
	varUnits := map[string]string{}
	var usedActuals []string
	if doc != nil {
		flat, _ := flattenModules(doc.Modules)
		totalWells := 0
		groupWells := map[string]int{}
		for _, m := range flat {
			switch m.Kind {
			case "param_table":
				for _, rm := range mapSlice(m.Data["rows"]) {
					k := strings.TrimSpace(textValue(rm, "key"))
					if k == "" {
						continue
					}
					u := strings.TrimSpace(textValue(rm, "unit"))
					if n, ok := numericValue(rm["value"]); ok {
						vars[k] = n
						vars["param_"+k] = n
						if u != "" {
							varUnits[k] = u
							varUnits["param_"+k] = u
						}
					}
					if act, ok := numericValue(rm["actual"]); ok {
						vars[k] = act
						vars["param_"+k] = act
						if u != "" {
							varUnits[k] = u
							varUnits["param_"+k] = u
						}
					}
				}
			case "plate":
				excluded := map[string]bool{}
				for _, w := range calcStringSlice(m.Data["excluded_wells"]) {
					excluded[strings.ToUpper(strings.TrimSpace(w))] = true
				}
				if gm, ok := m.Data["groups"].(map[string]any); ok {
					for gKey, raw := range gm {
						wells := calcStringSlice(raw)
						cnt := 0
						for _, w := range wells {
							if !excluded[strings.ToUpper(strings.TrimSpace(w))] {
								cnt++
							}
						}
						totalWells += cnt
						groupWells[gKey] += cnt
					}
				}
			}
		}
		if totalWells > 0 {
			vars["plate_wells"] = float64(totalWells)
			vars["n_wells"] = float64(totalWells)
			varUnits["plate_wells"] = "孔"
			varUnits["n_wells"] = "孔"
		}
		if g := BuildSciGraph(doc); g != nil {
			if pl := g.PrimaryPlate(); pl != nil && pl.Assigned {
				if pl.SampleWells > 0 {
					vars["plate_sample_wells"] = float64(pl.SampleWells)
					varUnits["plate_sample_wells"] = "孔"
				}
				vars["plate_blank_wells"] = float64(pl.BlankWells)
				varUnits["plate_blank_wells"] = "孔"
				if totalWells == 0 && pl.TotalWells > 0 {
					vars["plate_wells"] = float64(pl.TotalWells)
					vars["n_wells"] = float64(pl.TotalWells)
				}
			}
			if allSW := g.AllSampleWells(); allSW > 0 {
				vars["plate_all_sample_wells"] = float64(allSW)
				varUnits["plate_all_sample_wells"] = "孔"
			}
			for _, c := range g.Containers {
				if !c.Assigned {
					continue
				}
				cid := sanitizeCalcVar(c.ID)
				vars["plate_"+cid+"_sample_wells"] = float64(c.SampleWells)
				vars["plate_"+cid+"_wells"] = float64(c.TotalWells)
				vars["plate_"+cid+"_blank_wells"] = float64(c.BlankWells)
				varUnits["plate_"+cid+"_sample_wells"] = "孔"
				varUnits["plate_"+cid+"_wells"] = "孔"
				varUnits["plate_"+cid+"_blank_wells"] = "孔"
			}
			// Inject upstream single-document calculator outputs (`@calc.<id>.<key>`) WITH their physical units!
			for _, co := range g.CalcOutputs {
				varKey := "calc_" + sanitizeCalcVar(co.ModuleID) + "_" + sanitizeCalcVar(co.Key)
				vars[varKey] = co.Amount
				if co.Unit != "" {
					varUnits[varKey] = co.Unit
				}
			}
		}
		for gk, cnt := range groupWells {
			vars["group_"+sanitizeCalcVar(gk)+"_wells"] = float64(cnt)
			varUnits["group_"+sanitizeCalcVar(gk)+"_wells"] = "孔"
		}
	}
	if actuals != nil {
		for k, v := range actuals.Params {
			vars[k] = v
			vars["param_"+k] = v
			usedActuals = append(usedActuals, fmt.Sprintf("%s=%s", k, formatNumber(v)))
		}
	}
	sort.Strings(usedActuals)
	return vars, varUnits, usedActuals
}

func calcStringSlice(raw any) []string {
	var out []string
	switch v := raw.(type) {
	case []string:
		return append(out, v...)
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}

func sanitizeCalcVar(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	return b.String()
}

func expandCalcFormulaRefs(expr string) string {
	out := strings.ReplaceAll(expr, "@plate.all_sample_wells", "plate_all_sample_wells")
	out = strings.ReplaceAll(out, "@plate.sample_wells", "plate_sample_wells")
	out = strings.ReplaceAll(out, "@plate.blank_wells", "plate_blank_wells")
	out = strings.ReplaceAll(out, "@plate.wells", "plate_wells")
	// Scoped multi-plate references: @plate.<id>.sample_wells / @plate.<id>.wells / @plate.<id>.blank_wells
	for {
		idx := strings.Index(out, "@plate.")
		if idx < 0 {
			break
		}
		rest := out[idx+len("@plate."):]
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 {
			break
		}
		pid := sanitizeCalcVar(rest[:dot])
		after := rest[dot+1:]
		matched := false
		for _, suffix := range []string{"sample_wells", "blank_wells", "wells"} {
			if strings.HasPrefix(after, suffix) {
				out = out[:idx] + "plate_" + pid + "_" + suffix + after[len(suffix):]
				matched = true
				break
			}
		}
		if !matched {
			break
		}
	}
	for {
		idx := strings.Index(out, "@group.")
		if idx < 0 {
			break
		}
		rest := out[idx+len("@group."):]
		end := strings.Index(rest, ".wells")
		if end < 0 {
			break
		}
		gid := sanitizeCalcVar(rest[:end])
		out = out[:idx] + "group_" + gid + "_wells" + rest[end+len(".wells"):]
	}
	for {
		idx := strings.Index(out, "@calc.")
		if idx < 0 {
			break
		}
		rest := out[idx+len("@calc."):]
		dot := strings.IndexByte(rest, '.')
		if dot <= 0 {
			break
		}
		modID := sanitizeCalcVar(rest[:dot])
		after := rest[dot+1:]
		end := 0
		for end < len(after) && (unicode.IsLetter(rune(after[end])) || unicode.IsDigit(rune(after[end])) || after[end] == '_') {
			end++
		}
		if end == 0 {
			break
		}
		rowKey := sanitizeCalcVar(after[:end])
		out = out[:idx] + "calc_" + modID + "_" + rowKey + after[end:]
	}
	for {
		idx := strings.Index(out, "@param.")
		if idx < 0 {
			break
		}
		rest := out[idx+len("@param."):]
		end := 0
		for end < len(rest) && (unicode.IsLetter(rune(rest[end])) || unicode.IsDigit(rune(rest[end])) || rest[end] == '_') {
			end++
		}
		if end == 0 {
			break
		}
		out = out[:idx] + "param_" + rest[:end] + rest[end:]
	}
	return out
}

func evalCalculatorRows(doc *Document, data map[string]any, actuals *CalcActualContext) (CalcResult, bool) {
	rows := mapSlice(data["rows"])
	if len(rows) == 0 {
		return CalcResult{}, false
	}
	res := CalcResult{Mode: firstNonEmpty(textValue(data, "mode"), "formula")}
	vars, varUnits, usedActuals := collectCalcContextVarsWithUnits(doc, actuals)
	res.UsedActuals = usedActuals

	var lastSummary string
	for _, rm := range rows {
		key := strings.TrimSpace(textValue(rm, "key"))
		label := firstNonEmpty(textValue(rm, "label"), key, "条目")
		unit := textValue(rm, "unit")
		note := textValue(rm, "note")
		formula := strings.TrimSpace(textValue(rm, "formula"))
		if formula == "" {
			valStr := strings.TrimSpace(fmt.Sprintf("%v", rm["value"]))
			var val float64
			var ok bool
			rowNote := firstNonEmpty(note, "参数")
			if strings.HasPrefix(valStr, "@") {
				expanded := expandCalcFormulaRefs(valStr)
				if v, exists := vars[expanded]; exists {
					fromU := varUnits[expanded]
					if fromU != "" && unit != "" {
						conv, convOK, sameDim := ConvertScientificUnit(v, fromU, unit)
						if !convOK || !sameDim {
							res.Err = fmt.Errorf("量纲不兼容：%s 引用 %s（%s），无法直接换算到目标单位 %s", label, valStr, fromU, unit)
							return res, true
						}
						if !strings.EqualFold(strings.ReplaceAll(strings.ReplaceAll(fromU, "µ", "u"), "μ", "u"),
							strings.ReplaceAll(strings.ReplaceAll(unit, "µ", "u"), "μ", "u")) {
							rowNote = fmt.Sprintf("联动 %s (%s %s → %s %s)", valStr, formatNumber(round4(v)), fromU, formatNumber(round4(conv)), unit)
						}
						v = conv
					}
					val, ok = v, true
				} else if strings.HasPrefix(valStr, "@plate.") {
					res.Err = fmt.Errorf("%s：孔板尚未分配任何实验孔位，请先在孔板中框选分组", label)
					return res, true
				}
			}
			if !ok {
				val, ok = numericValue(rm["value"])
			}
			if ok && key != "" {
				vars[key] = val
			}
			if ok {
				res.Rows = append(res.Rows, CalcRow{
					Label:  label,
					Amount: round4(val),
					Unit:   unit,
					Note:   rowNote,
				})
			}
			continue
		}
		expanded := expandCalcFormulaRefs(formula)
		val, err := EvalFormula(expanded, vars)
		if err != nil {
			res.Err = fmt.Errorf("%s: %w", label, err)
			return res, true
		}
		if key != "" {
			vars[key] = val
		}
		res.Rows = append(res.Rows, CalcRow{
			Label:  label,
			Amount: round4(val),
			Unit:   unit,
			Note:   firstNonEmpty(note, formula),
		})
		lastSummary = fmt.Sprintf("%s: %s", label, formatNumber(round4(val)))
		if unit != "" {
			lastSummary += " " + unit
		}
	}
	res.Summary = lastSummary
	return res, true
}

// EvalCalculatorWithActuals computes calculator@1 quantities, substituting live Run
// measured parameters and upstream sample concentrations when supplied via actuals.
func EvalCalculatorWithActuals(doc *Document, data map[string]any, actuals *CalcActualContext) CalcResult {
	mode := textValue(data, "mode")
	res := CalcResult{Mode: mode}
	if rowRes, used := evalCalculatorRows(doc, data, actuals); used {
		return rowRes
	}
	switch mode {
	case "dilution":
		sv, su, err := quantityParts(data["stock"])
		if err != nil {
			res.Err = err
			return res
		}
		if stockRef := textValue(data, "stock_ref"); stockRef != "" {
			stockKey := strings.TrimPrefix(strings.TrimPrefix(stockRef, "@actual."), "@param.")
			if actuals != nil {
				if av, ok := actuals.Params[stockKey]; ok && av > 0 {
					sv = av
					if au := actuals.ParamUnits[stockKey]; au != "" {
						su = au
					}
					res.UsedActuals = append(res.UsedActuals, fmt.Sprintf("%s=%s %s", stockKey, formatNumber(sv), su))
				}
			} else if pv, pu, ok := docPlannedParamFloat(doc, stockKey); ok && pv > 0 {
				sv = pv
				if pu != "" {
					su = pu
				}
			}
		}
		tv, tu, err := quantityParts(data["target"])
		if err != nil {
			res.Err = err
			return res
		}
		fv, fu, err := quantityParts(data["final_volume"])
		if err != nil {
			res.Err = err
			return res
		}
		ss, ok1 := unitFor(su)
		ts, ok2 := unitFor(tu)
		_, ok3 := unitFor(fu)
		if !ok1 || !ok2 || !ok3 || ss.Dimension != ts.Dimension {
			res.Err = fmt.Errorf("dilution quantities have incompatible units")
			return res
		}
		if sv <= 0 || tv <= 0 || fv <= 0 {
			res.Err = fmt.Errorf("dilution quantities must be positive")
			return res
		}
		if tv*ts.Factor > sv*ss.Factor {
			res.Err = fmt.Errorf("target concentration exceeds stock concentration")
			return res
		}
		stockVol := (tv * ts.Factor / (sv * ss.Factor)) * fv
		diluentVol := fv - stockVol
		res.Summary = fmt.Sprintf("%s %s 母液 + %s %s 稀释液", formatNumber(round4(stockVol)), fu, formatNumber(round4(diluentVol)), fu)
		res.Rows = []CalcRow{
			{Label: "母液体积 (V1)", Amount: round4(stockVol), Unit: fu, Note: fmt.Sprintf("从 %s %s 稀释至 %s %s", formatNumber(sv), su, formatNumber(tv), tu)},
			{Label: "稀释液体积", Amount: round4(diluentVol), Unit: fu, Note: "补足至目标终体积"},
			{Label: "目标总体积 (V2)", Amount: round4(fv), Unit: fu, Note: "C1·V1 = C2·V2"},
		}
		return res

	case "group_mastermix":
		vpuVal, vpuUnit, err := quantityParts(data["volume_per_unit"])
		if err != nil || vpuVal <= 0 {
			res.Err = fmt.Errorf("group_mastermix needs positive volume_per_unit")
			return res
		}
		vpuSpec, ok := unitFor(vpuUnit)
		if !ok || vpuSpec.Dimension != "volume" {
			res.Err = fmt.Errorf("volume_per_unit must use volume unit")
			return res
		}
		excess, _ := numericValue(data["excess_ratio"])
		if excess < 0 {
			excess = 0
		}
		groupCounts := map[string]int{}
		groupOrder := []string{}
		if ref := firstNonEmpty(textValue(data, "plate_ref"), textValue(data, "container_ref")); ref != "" && doc != nil {
			if p, found := doc.PlateByID(ref); found {
				for _, g := range p.GroupNames() {
					n := len(p.Groups[g])
					if n > 0 {
						groupCounts[g] = n
						groupOrder = append(groupOrder, g)
					}
				}
			}
		}
		if len(groupOrder) == 0 {
			sc, okSC := numericValue(data["sample_count"])
			if !okSC || sc < 1 {
				sc = 1
			}
			groupOrder = []string{"总体系"}
			groupCounts["总体系"] = int(sc)
		}
		totalUnits := 0
		lblMap := PlateGroupLabelMap(doc)
		for _, g := range groupOrder {
			totalUnits += groupCounts[g]
			groupVol := float64(groupCounts[g]) * vpuVal * (1 + excess)
			gLabel := strings.TrimSpace(lblMap[g])
			if gLabel == "" {
				gLabel = g
			}
			res.Rows = append(res.Rows, CalcRow{
				Label:  fmt.Sprintf("%s 体系 (%d 孔/份)", gLabel, groupCounts[g]),
				Amount: round4(groupVol),
				Unit:   vpuUnit,
				Note:   fmt.Sprintf("含 %d%% 余量", int(math.Round(excess*100))),
			})
		}
		totalVol := float64(totalUnits) * vpuVal * (1 + excess)
		if comps, ok := data["components"].([]any); ok {
			for _, raw := range comps {
				cm, _ := raw.(map[string]any)
				cname := textValue(cm, "name")
				if cname == "" {
					continue
				}
				if ratio, hasR := numericValue(cm["ratio"]); hasR && ratio > 0 {
					res.Rows = append(res.Rows, CalcRow{
						Label:  cname,
						Amount: round4(totalVol * ratio),
						Unit:   vpuUnit,
						Note:   fmt.Sprintf("配比系数 %s", formatNumber(ratio)),
					})
				} else if targetPct, hasTP := numericValue(cm["target_percent"]); hasTP {
					stockPct, _ := numericValue(cm["stock_percent"])
					if stockPct > 0 {
						vol := totalVol * (targetPct / stockPct)
						res.Rows = append(res.Rows, CalcRow{
							Label:  cname,
							Amount: round4(vol),
							Unit:   vpuUnit,
							Note:   fmt.Sprintf("%s%% → %s%%", formatNumber(stockPct), formatNumber(targetPct)),
						})
					} else {
						massMg := (totalVol * vpuSpec.Factor * 1000.0) * (targetPct * 10.0) // mg/mL = 10*% w/v
						res.Rows = append(res.Rows, CalcRow{
							Label:  cname,
							Amount: round4(massMg),
							Unit:   "mg",
							Note:   fmt.Sprintf("终浓度 %s%% w/v", formatNumber(targetPct)),
						})
					}
				} else if qv, qu, qerr := quantityParts(cm["volume_per_unit"]); qerr == nil && qv > 0 {
					res.Rows = append(res.Rows, CalcRow{
						Label:  cname,
						Amount: round4(qv * float64(totalUnits) * (1 + excess)),
						Unit:   qu,
						Note:   fmt.Sprintf("单份 %s %s", formatNumber(qv), qu),
					})
				} else {
					note := textValue(cm, "note")
					if note == "" {
						note = "按分组浓度配置"
					}
					res.Rows = append(res.Rows, CalcRow{
						Label:  cname,
						Amount: round4(totalVol),
						Unit:   vpuUnit,
						Note:   note,
					})
				}
			}
		}
		res.Summary = fmt.Sprintf("共 %d 份 · 含 %d%% 余量 · 总配液 %s %s", totalUnits, int(math.Round(excess*100)), formatNumber(round4(totalVol)), vpuUnit)
		return res

	case "serial_dilution":
		df, ok1 := numericValue(data["dilution_factor"])
		steps, ok2 := numericValue(data["steps_count"])
		if !ok1 || df <= 1 || !ok2 || steps < 1 {
			res.Err = fmt.Errorf("serial_dilution needs dilution_factor > 1 and steps_count >= 1")
			return res
		}
		transferVal, transferUnit, qerr := quantityParts(data["volume_per_unit"])
		if qerr != nil || transferVal <= 0 {
			transferVal, transferUnit = 100, "uL"
		}
		diluentVal := transferVal * (df - 1)
		factor := 1.0
		for step := 1; step <= int(steps); step++ {
			factor *= df
			res.Rows = append(res.Rows, CalcRow{
				Label:  fmt.Sprintf("第 %d 级 (1:%s)", step, formatNumber(factor)),
				Amount: round4(transferVal),
				Unit:   transferUnit,
				Note:   fmt.Sprintf("吸取 %s %s + 稀释液 %s %s", formatNumber(round4(transferVal)), transferUnit, formatNumber(round4(diluentVal)), transferUnit),
			})
		}
		res.Summary = fmt.Sprintf("%d 级 %s 倍梯度稀释（总稀释至 1:%s）", int(steps), formatNumber(df), formatNumber(factor))
		return res

	case "sample_normalize":
		rxnVal, rxnUnit, rxnErr := quantityParts(data["volume_per_unit"])
		if rxnErr != nil || rxnVal <= 0 {
			rxnVal, rxnUnit, rxnErr = quantityParts(data["final_volume"])
		}
		var targetMassNg float64
		hasTargetMass := false
		if tmVal, tmUnit, tmErr := quantityParts(data["target_mass"]); tmErr == nil && tmVal > 0 {
			if ng, ok := massToNg(tmVal, tmUnit); ok {
				targetMassNg = ng
				hasTargetMass = true
			}
		} else if tmNum, ok := numericValue(data["target_mass_ng"]); ok && tmNum > 0 {
			targetMassNg = tmNum
			hasTargetMass = true
		}

		// Collect live upstream sample measurements first, otherwise fall back to authored sample rows.
		var samples []CalcSampleInput
		fromActual := false
		wantedEndpoint := strings.TrimSpace(textValue(data, "upstream_endpoint"))
		if actuals != nil && len(actuals.Samples) > 0 {
			for _, s := range actuals.Samples {
				if wantedEndpoint != "" && s.Endpoint != "" && !strings.EqualFold(s.Endpoint, wantedEndpoint) {
					continue
				}
				if s.Value > 0 {
					samples = append(samples, s)
				}
			}
			if len(samples) > 0 {
				fromActual = true
			}
		}
		if len(samples) == 0 {
			for _, rm := range mapSlice(data["samples"]) {
				sid := firstNonEmpty(textValue(rm, "sample_id"), textValue(rm, "name"))
				if sid == "" {
					continue
				}
				val, ok := numericValue(rm["concentration"])
				unit := textValue(rm, "unit")
				if !ok {
					if qv, qu, qerr := quantityParts(rm["concentration"]); qerr == nil {
						val, unit, ok = qv, qu, true
					}
				}
				if ok && val > 0 {
					samples = append(samples, CalcSampleInput{SampleID: sid, Value: val, Unit: unit})
				}
			}
		}
		if len(samples) > 0 && rxnErr == nil && rxnVal > 0 {
			rxnUL, normUnit, okV := volumeToUL(rxnVal, rxnUnit)
			if !okV {
				res.Err = fmt.Errorf("sample_normalize volume unit %q is invalid", rxnUnit)
				return res
			}
			if !hasTargetMass {
				targetMassNg = 500
			}
			insufficientCount := 0
			for _, s := range samples {
				concNgPerUL, okC := massConcToNgPerUL(s.Value, s.Unit)
				if !okC || concNgPerUL <= 0 {
					res.Rows = append(res.Rows, CalcRow{
						Label: s.SampleID, Amount: 0, Unit: normUnit,
						Note: fmt.Sprintf("实测 %s %s 单位暂不支持换算", formatNumber(s.Value), s.Unit),
					})
					continue
				}
				reqUL := targetMassNg / concNgPerUL
				sampleUL := reqUL
				diluentUL := rxnUL - sampleUL
				note := fmt.Sprintf("实测 %s %s · 补稀释液 %s %s", formatNumber(s.Value), firstNonEmpty(s.Unit, "ng/uL"), formatNumber(round4(diluentUL)), normUnit)
				if reqUL > rxnUL {
					insufficientCount++
					sampleUL = rxnUL
					note = fmt.Sprintf("实测 %s %s · 需 %s %s（浓度偏低，超出单孔反应体积上限）",
						formatNumber(s.Value), firstNonEmpty(s.Unit, "ng/uL"), formatNumber(round4(reqUL)), normUnit)
				}
				res.Rows = append(res.Rows, CalcRow{
					Label: s.SampleID + " 加样量", Amount: round4(sampleUL), Unit: normUnit, Note: note,
				})
				if fromActual {
					res.UsedActuals = append(res.UsedActuals, fmt.Sprintf("%s=%s %s", s.SampleID, formatNumber(s.Value), firstNonEmpty(s.Unit, "ng/uL")))
				}
			}
			res.Summary = fmt.Sprintf("%d 个样本归一化至 %s ng / %s %s（偏低 %d 项）",
				len(samples), formatNumber(round4(targetMassNg)), formatNumber(round4(rxnUL)), normUnit, insufficientCount)
			return res
		}

		count, _ := numericValue(data["sample_count"])
		if count < 1 {
			count = 1
		}
		excess, _ := numericValue(data["excess_ratio"])
		if excess < 0 {
			excess = 0
		}
		if rxnErr == nil && rxnVal > 0 {
			tot := count * rxnVal * (1 + excess)
			res.Summary = fmt.Sprintf("%d 个样本 · 单样 %s %s · 总配液 %s %s", int(count), formatNumber(rxnVal), rxnUnit, formatNumber(round4(tot)), rxnUnit)
			res.Rows = []CalcRow{
				{Label: "单样反应体积", Amount: round4(rxnVal), Unit: rxnUnit, Note: fmt.Sprintf("样本数 %d", int(count))},
				{Label: "含余量总体系", Amount: round4(tot), Unit: rxnUnit, Note: fmt.Sprintf("含 %d%% 余量", int(math.Round(excess*100)))},
			}
			return res
		}
		res.Summary = fmt.Sprintf("归一化 %d 个样本至目标浓度", int(count))
		return res

	case "formula":
		expr := textValue(data, "formula")
		if err := ValidateFormulaExpr(expr); err != nil {
			res.Err = err
			return res
		}
		vars := map[string]float64{}
		keys := []string{}
		if rawIn, ok := data["inputs"].(map[string]any); ok {
			for k, v := range rawIn {
				switch tv := v.(type) {
				case string:
					refKey := strings.TrimSpace(tv)
					if strings.HasPrefix(refKey, "@actual.") || strings.HasPrefix(refKey, "@param.") {
						refKey = strings.TrimPrefix(strings.TrimPrefix(refKey, "@actual."), "@param.")
						if actuals != nil {
							if av, found := actuals.Params[refKey]; found {
								vars[k] = av
								keys = append(keys, k)
								res.UsedActuals = append(res.UsedActuals, fmt.Sprintf("%s=%s", k, formatNumber(av)))
								continue
							}
						}
						if pv, _, found := docPlannedParamFloat(doc, refKey); found {
							vars[k] = pv
							keys = append(keys, k)
						}
					}
				default:
					if num, numOK := numericValue(v); numOK {
						vars[k] = num
						keys = append(keys, k)
					} else if qv, _, qerr := quantityParts(v); qerr == nil {
						vars[k] = qv
						keys = append(keys, k)
					} else if m, ok := v.(map[string]any); ok {
						if num, numOK := numericValue(m["value"]); numOK {
							vars[k] = num
							keys = append(keys, k)
						}
					}
				}
			}
		}
		if actuals != nil {
			for _, varName := range scimath.ExtractVars(expr) {
				if _, already := vars[varName]; already {
					continue
				}
				if av, ok := actuals.Params[varName]; ok {
					vars[varName] = av
					keys = append(keys, varName)
					res.UsedActuals = append(res.UsedActuals, fmt.Sprintf("%s=%s", varName, formatNumber(av)))
				}
			}
		}
		sort.Strings(keys)
		sort.Strings(res.UsedActuals)
		if len(vars) == 0 {
			res.Summary = fmt.Sprintf("公式：%s（Run 时代入实测值）", expr)
			return res
		}
		val, err := EvalFormula(expr, vars)
		if err != nil {
			res.Err = err
			return res
		}
		resUnit := textValue(data, "unit")
		res.Summary = fmt.Sprintf("%s = %s", expr, formatNumber(round4(val)))
		if resUnit != "" {
			res.Summary += " " + resUnit
		}
		res.Rows = []CalcRow{{Label: "计算结果", Amount: round4(val), Unit: resUnit, Note: expr}}
		return res
	}
	res.Err = fmt.Errorf("unsupported calculator mode %q", mode)
	return res
}

func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

func isFiniteNum(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}
