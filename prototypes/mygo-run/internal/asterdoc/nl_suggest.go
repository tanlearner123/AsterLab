package asterdoc

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"asterlab-mygo-prototype/internal/scimath"
)

//go:embed spec/nl_schemas.json
var nlSchemasJSON []byte

// NLSchemaRegistry is the authoritative schema document loaded from spec/nl_schemas.json.
type NLSchemaRegistry struct {
	Version                string                   `json:"version"`
	Description            string                   `json:"description"`
	AtomicSlotTypes        []string                 `json:"atomic_slot_types"`
	NonSuggestibleKinds    []string                 `json:"non_suggestible_kinds"`
	GlobalNegativeBoundary string                   `json:"global_negative_boundary"`
	Targets                []NLComponentInputSchema `json:"targets"`
}

var (
	nlRegistryOnce sync.Once
	nlRegistry     NLSchemaRegistry
	nlRegistryErr  error
)

// LoadNLSchemaRegistry returns the single-source-of-truth compact extraction schema registry
// embedded from spec/nl_schemas.json.
func LoadNLSchemaRegistry() (NLSchemaRegistry, error) {
	nlRegistryOnce.Do(func() {
		nlRegistryErr = json.Unmarshal(nlSchemasJSON, &nlRegistry)
	})
	return nlRegistry, nlRegistryErr
}

// NLParamKV is one extracted key-value parameter displayed in the Alt+Hover preview dropdown.
type NLParamKV struct {
	Label     string
	Value     string
	IsDefault bool // true when parameter was absent in natural language and filled via safe editable fallback
}

// NLSlotFieldSpec defines one atomic slot in the compact extraction schema shared by
// Jev routing, local GBNF slot-filling models, and the deterministic Go module expander.
type NLSlotFieldSpec struct {
	Key      string `json:"key"`
	Type     string `json:"type"` // "string" | "number" | "int" | "qty" | "phase_list" | "group_list"
	Required bool   `json:"required,omitempty"`
	Desc     string `json:"desc"`
}

// NLComponentInputSchema is the authoritative compact input schema for one convertible
// AsterLab module target loaded from spec/nl_schemas.json.
type NLComponentInputSchema struct {
	ID               string            `json:"id"`
	Kind             string            `json:"kind"`
	Mode             string            `json:"mode,omitempty"`
	Title            string            `json:"title"`
	WhenToSuggest    string            `json:"when_to_suggest"`
	WhenNotToSuggest string            `json:"when_not_to_suggest"`
	Fields           []NLSlotFieldSpec `json:"slots"`
	ExampleZH        string            `json:"example_zh"`
	ExampleEN        string            `json:"example_en"`
	ExampleJSON      string            `json:"example_json"`
}

// NLComponentSchemas returns the canonical registry of compact input schemas loaded from
// spec/nl_schemas.json.
func NLComponentSchemas() []NLComponentInputSchema {
	reg, err := LoadNLSchemaRegistry()
	if err != nil {
		return nil
	}
	return reg.Targets
}

// ExpandCompactSlot compiles a compact 20-35 token slot-filling JSON object (produced by
// a local small model, external LLM, or deterministic extractor) into 100% spec-validated
// AsterML v2 module map(s) with deterministic unit conversion & plate well allocation.
func ExpandCompactSlot(doc *Document, kind, mode string, slots map[string]any) (*NLSuggestion, error) {
	if slots == nil {
		return nil, fmt.Errorf("empty slots")
	}
	var sug *NLSuggestion
	switch kind {
	case "timer":
		id := nextUniqueBlockID(doc, "timer-nl")
		title := firstNonEmpty(textValue(slots, "title"), "实验计时")
		switch mode {
		case "cycle":
			rounds := int(numOrZero(slots["r"]))
			if rounds < 1 {
				rounds = 1
			}
			var phases []any
			var labels []string
			totSec := 0
			if rawP, ok := slots["p"].([]any); ok {
				for idx, item := range rawP {
					arr, ok := item.([]any)
					if !ok || len(arr) < 3 {
						continue
					}
					pn := firstNonEmpty(fmt.Sprint(arr[0]), fmt.Sprintf("阶段 %d", idx+1))
					pv, _ := numericValue(arr[1])
					pu := normDurationUnit(fmt.Sprint(arr[2]))
					phases = append(phases, map[string]any{"name": pn, "mode": "fixed", "duration": map[string]any{"value": pv, "unit": pu}})
					labels = append(labels, fmt.Sprintf("%s %s %s", pn, formatNumber(pv), pu))
					totSec += durationToSec(pv, pu)
				}
			}
			if len(phases) == 0 {
				return nil, fmt.Errorf("cycle timer requires phases")
			}
			totSec *= rounds
			mod := map[string]any{"kind": "timer", "id": id, "schema": 1, "title": title, "mode": "cycle", "rounds": rounds, "phases": phases}
			sug = &NLSuggestion{
				Kind: "timer", Mode: "cycle", KindLabel: fmt.Sprintf("循环计时器 · %d 轮", rounds),
				Summary: fmt.Sprintf("%s · 共 %s", strings.Join(labels, " ↔ "), formatClockHMS(totSec)),
				Params:  []NLParamKV{{Label: "循环轮数", Value: fmt.Sprintf("%d 轮", rounds)}, {Label: "总计预算", Value: formatClockHMS(totSec)}},
				Modules: []map[string]any{mod},
			}
		case "countdown":
			v, u := parseQtyTuple(slots["dur"], 10, "min")
			u = normDurationUnit(u)
			mod := map[string]any{"kind": "timer", "id": id, "schema": 1, "title": title, "mode": "countdown", "duration": map[string]any{"value": v, "unit": u}}
			sug = &NLSuggestion{
				Kind: "timer", Mode: "countdown", KindLabel: "倒计时器",
				Summary: fmt.Sprintf("%s · %s %s", title, formatNumber(v), u),
				Params:  []NLParamKV{{Label: "名称", Value: title}, {Label: "时长", Value: fmt.Sprintf("%s %s", formatNumber(v), u)}},
				Modules: []map[string]any{mod},
			}
		case "stopwatch":
			mod := map[string]any{"kind": "timer", "id": id, "schema": 1, "title": title, "mode": "stopwatch"}
			sug = &NLSuggestion{Kind: "timer", Mode: "stopwatch", KindLabel: "正计时秒表", Summary: title, Modules: []map[string]any{mod}}
		case "window":
			v, u := parseQtyTuple(slots["off"], 2, "h")
			u = normDurationUnit(u)
			anchor := firstNonEmpty(textValue(slots, "anchor"), "t0")
			win := firstNonEmpty(textValue(slots, "win"), fmt.Sprintf("+%s %s", formatNumber(v), u))
			mod := map[string]any{"kind": "timer", "id": id, "schema": 1, "title": title, "mode": "window", "anchor": anchor, "offset": map[string]any{"value": v, "unit": u}, "schedule_window": win}
			sug = &NLSuggestion{Kind: "timer", Mode: "window", KindLabel: "时间窗口提醒", Summary: fmt.Sprintf("%s · +%s %s", anchor, formatNumber(v), u), Modules: []map[string]any{mod}}
		}
	case "calculator":
		id := nextUniqueBlockID(doc, "calc-nl")
		switch mode {
		case "dilution":
			reagent := firstNonEmpty(textValue(slots, "name"), "母液")
			c1v, c1u := parseQtyTuple(slots["c1"], 100, "ug/mL")
			c2v, c2u := parseQtyTuple(slots["c2"], 50, "ng/mL")
			v2v, v2u := parseQtyTuple(slots["v2"], 5, "mL")
			s1, f1 := concFactorToBase(c1u)
			s2, f2 := concFactorToBase(c2u)
			if f1 == f2 && s1 > 0 {
				c2v = round4(c2v * s2 / s1)
				c2u = c1u
			}
			v2uL := v2v
			if strings.EqualFold(v2u, "mL") {
				v2uL = v2v * 1000
			} else if strings.EqualFold(v2u, "L") {
				v2uL = v2v * 1e6
			}
			mod := map[string]any{
				"kind": "calculator", "id": id, "schema": 1, "title": fmt.Sprintf("%s 稀释配制 (C₁V₁ = C₂V₂)", reagent), "mode": "formula",
				"rows": []any{
					map[string]any{"key": "C_1", "label": reagent + " 母液浓度", "kind": "input", "value": c1v, "unit": c1u},
					map[string]any{"key": "C_2", "label": "目标工作浓度", "kind": "input", "value": c2v, "unit": c2u},
					map[string]any{"key": "V_2", "label": "配制终体积", "kind": "input", "value": v2uL, "unit": "uL"},
					map[string]any{"key": "V_1", "label": "取母液体积", "kind": "formula", "formula": `\frac{C_{2} \times V_{2}}{C_{1}}`, "unit": "uL"},
					map[string]any{"key": "V_dil", "label": "补加稀释液", "kind": "formula", "formula": `V_{2} - \frac{C_{2} \times V_{2}}{C_{1}}`, "unit": "uL"},
				},
			}
			v1 := round4(c2v * v2uL / c1v)
			sug = &NLSuggestion{
				Kind: "calculator", Mode: "dilution", KindLabel: "稀释配液计算器",
				Summary: fmt.Sprintf("取 %s 母液 %s µL + 补稀释液 %s µL", reagent, formatNumber(v1), formatNumber(round4(v2uL-v1))),
				Modules: []map[string]any{mod},
			}
		case "weighing":
			cv, cu := parseQtyTuple(slots["c"], 5, "% w/v")
			vv, vu := parseQtyTuple(slots["v"], 2, "mL")
			if strings.EqualFold(vu, "uL") {
				vv /= 1000
			}
			mw := numOrZero(slots["mw"])
			if mw > 0 {
				mod := map[string]any{
					"kind": "calculator", "id": id, "schema": 1, "title": "摩尔称量配液计算", "mode": "formula",
					"rows": []any{
						map[string]any{"key": "C", "label": "目标摩尔浓度", "kind": "input", "value": cv, "unit": "mM"},
						map[string]any{"key": "V", "label": "配制终体积", "kind": "input", "value": vv, "unit": "mL"},
						map[string]any{"key": "MW", "label": "分子量", "kind": "input", "value": mw, "unit": "g/mol"},
						map[string]any{"key": "m", "label": "需称取溶质质量", "kind": "formula", "formula": `\frac{C \times V \times MW}{1000}`, "unit": "mg"},
					},
				}
				sug = &NLSuggestion{Kind: "calculator", Mode: "weighing", KindLabel: "摩尔称量计算器", Summary: fmt.Sprintf("称取 %s mg", formatNumber(round4(cv*vv*mw/1000))), Modules: []map[string]any{mod}}
			} else {
				_ = cu
				mod := map[string]any{
					"kind": "calculator", "id": id, "schema": 1, "title": "称量配液计算 (% w/v)", "mode": "formula",
					"rows": []any{
						map[string]any{"key": "C_w", "label": "目标浓度", "kind": "input", "value": cv, "unit": "% w/v"},
						map[string]any{"key": "V", "label": "配制终体积", "kind": "input", "value": vv, "unit": "mL"},
						map[string]any{"key": "m", "label": "需称取溶质质量", "kind": "formula", "formula": `C_{w} \times V \times 10`, "unit": "mg"},
					},
				}
				sug = &NLSuggestion{Kind: "calculator", Mode: "weighing", KindLabel: "% w/v 称量计算器", Summary: fmt.Sprintf("称取 %s mg", formatNumber(round4(cv*vv*10))), Modules: []map[string]any{mod}}
			}
		case "mastermix":
			n := numOrZero(slots["n"])
			if n < 1 {
				n = 24
			}
			v0, v0u := parseQtyTuple(slots["v0"], 10, "uL")
			r := numOrZero(slots["r"])
			mod := map[string]any{
				"kind": "calculator", "id": id, "schema": 1, "title": "孔板预混液配制计算", "mode": "formula",
				"rows": []any{
					map[string]any{"key": "N", "label": "反应孔数", "kind": "input", "value": n, "unit": "孔"},
					map[string]any{"key": "V_0", "label": "单孔加样体积", "kind": "input", "value": v0, "unit": v0u},
					map[string]any{"key": "r", "label": "富余比例", "kind": "input", "value": r, "unit": ""},
					map[string]any{"key": "V_tot", "label": "预混液总体积", "kind": "formula", "formula": `N \times V_{0} \times (1 + r)`, "unit": v0u},
				},
			}
			sug = &NLSuggestion{Kind: "calculator", Mode: "mastermix", KindLabel: "预混液计算器", Summary: fmt.Sprintf("总计 %s %s", formatNumber(round4(n*v0*(1+r))), v0u), Modules: []map[string]any{mod}}
		case "seeding":
			rho := numOrZero(slots["rho"])
			if rho <= 0 {
				rho = 500000
			}
			nw := numOrZero(slots["nw"])
			n := numOrZero(slots["n"])
			vw := numOrZero(slots["vw"])
			mod := map[string]any{
				"kind": "calculator", "id": id, "schema": 1, "title": "细胞接种稀释计算", "mode": "formula",
				"rows": []any{
					map[string]any{"key": "rho", "label": "实测悬液密度", "kind": "input", "value": rho, "unit": "cells/mL"},
					map[string]any{"key": "n_w", "label": "每孔目标细胞数", "kind": "input", "value": nw, "unit": "cells"},
					map[string]any{"key": "N", "label": "接种总孔数", "kind": "input", "value": n, "unit": "孔"},
					map[string]any{"key": "V_w", "label": "每孔接种体积", "kind": "input", "value": vw, "unit": "uL"},
					map[string]any{"key": "V_cell", "label": "取细胞悬液", "kind": "formula", "formula": `\frac{n_{w} \times N}{\rho}`, "unit": "mL"},
					map[string]any{"key": "V_med", "label": "补加培养基", "kind": "formula", "formula": `\frac{V_{w} \times N}{1000} - \frac{n_{w} \times N}{\rho}`, "unit": "mL"},
				},
			}
			sug = &NLSuggestion{Kind: "calculator", Mode: "seeding", KindLabel: "细胞接种计算器", Summary: fmt.Sprintf("取悬液 %s mL", formatNumber(round4(nw*n/rho))), Modules: []map[string]any{mod}}
		}
	case "plate_group":
		fmtN := int(numOrZero(slots["fmt"]))
		geom, ok := geometryForPlate(fmtN, "", 0, 0)
		if !ok {
			fmtN = 12
			geom, _ = geometryForPlate(12, "", 0, 0)
		}
		var gtRows []any
		plateGroups := map[string]any{}
		plateRoles := map[string]any{}
		rowCur := 0
		if rawG, ok := slots["grps"].([]any); ok {
			for i, item := range rawG {
				arr, ok := item.([]any)
				if !ok || len(arr) < 2 {
					continue
				}
				gname := fmt.Sprint(arr[0])
				repF, _ := numericValue(arr[1])
				rep := int(repF)
				if rep < 1 {
					rep = 1
				}
				role := "treatment"
				if len(arr) >= 3 && strings.TrimSpace(fmt.Sprint(arr[2])) != "" {
					role = strings.TrimSpace(fmt.Sprint(arr[2]))
				}
				gid := fmt.Sprintf("g-slot-%d", i+1)
				gtRole := role
				if gtRole == "blank" {
					gtRole = "control"
				}
				gtRows = append(gtRows, map[string]any{"id": gid, "name": gname, "role": gtRole, "technical_repeats": rep})
				var wells []any
				if rowCur < geom.Rows {
					for col := 1; col <= rep && col <= geom.Columns; col++ {
						w := fmt.Sprintf("%s%d", rowLabel(rowCur), col)
						wells = append(wells, w)
						if role == "blank" {
							plateRoles[w] = "blank"
						}
					}
					rowCur++
				}
				if len(wells) > 0 {
					plateGroups[gid] = wells
				}
			}
		}
		gtMod := map[string]any{"kind": "group_table", "id": nextUniqueBlockID(doc, "gt-nl"), "schema": 1, "title": "实验分组", "rows": gtRows}
		plMod := map[string]any{"kind": "plate", "id": nextUniqueBlockID(doc, "plate-nl"), "schema": 1, "title": fmt.Sprintf("%d 孔板布局", fmtN), "format": fmtN, "groups": plateGroups}
		if len(plateRoles) > 0 {
			plMod["roles"] = plateRoles
		}
		sug = &NLSuggestion{Kind: "plate_group", KindLabel: fmt.Sprintf("%d 孔板 + %d 组", fmtN, len(gtRows)), Modules: []map[string]any{gtMod, plMod}}
	case "attachment":
		slotsN := int(numOrZero(slots["slots"]))
		if slotsN < 1 {
			slotsN = 6
		}
		hint := firstNonEmpty(textValue(slots, "hint"), "孔板网格采图槽位")
		plateRef := ""
		if doc != nil {
			for _, m := range doc.Modules {
				if m.Kind == "plate" {
					plateRef = m.ID
					break
				}
			}
		}
		mod := map[string]any{"kind": "attachment", "id": nextUniqueBlockID(doc, "att-nl"), "schema": 1, "title": "孔板网格采图槽位", "scope": "step_evidence", "slots_per_cell": slotsN, "media_hint": hint}
		if plateRef != "" {
			mod["scope"] = "container_grid"
			mod["container_ref"] = plateRef
		}
		sug = &NLSuggestion{Kind: "attachment", KindLabel: fmt.Sprintf("采图网格 · %d 视野/孔", slotsN), Summary: hint, Modules: []map[string]any{mod}}
	}
	if sug == nil || len(sug.Modules) == 0 {
		return nil, fmt.Errorf("unsupported slot kind/mode %s/%s", kind, mode)
	}
	for _, m := range sug.Modules {
		if err := specValidator(textValue(m, "kind"))(m); err != nil {
			return nil, err
		}
	}
	return sug, nil
}

func parseQtyTuple(raw any, defVal float64, defUnit string) (float64, string) {
	if arr, ok := raw.([]any); ok && len(arr) >= 2 {
		v, ok := numericValue(arr[0])
		if ok && v > 0 {
			return v, strings.TrimSpace(fmt.Sprint(arr[1]))
		}
	}
	return defVal, defUnit
}

// NLSuggestion represents a Jev-routed + Go-slot-expanded interactive scientific module
// suggested for a single natural-language lab note sentence.
type NLSuggestion struct {
	SourceText       string
	Kind             string // "timer" | "calculator" | "plate_group" | "attachment"
	Mode             string // "cycle" | "countdown" | "stopwatch" | "window" | "dilution" | "weighing" | "mastermix" | "seeding" | "ratio_dilution" | "serial_dilution" | "rcf_rpm" | "custom" | ""
	Confidence       float64
	Noul             float64
	Source           string // "Jev" | "本地语义" | "本地速判"
	KindLabel        string // e.g. "循环计时器" | "稀释计算器" | "12 孔板 + 分组" | "显微采图网格"
	Summary          string
	InheritedFrom    string // e.g. "@plate-huvec" or "@calc-vegf" when upstream SciGraph facts were auto-inherited
	FormulaUncertain bool   // true when calculator intent is clear but formula needs human selection from Formula Library
	Params           []NLParamKV
	MissingSlots     []string // Human-readable names of parameters absent from prose that used editable placeholder defaults
	Modules          []map[string]any
}

// JevRouteDecision caches one routing result (cloud Jev or local INT8 ONNX) for a sentence.
type JevRouteDecision struct {
	Kind       string
	Mode       string
	Confidence float64
	Noul       float64
	IsMulti    bool
	MultiProb  float64
	Source     string // "Jev" | "本地语义" | "本地速判"
	Err        error
}

var (
	jevCacheMu sync.Mutex
	jevCache   = map[string]*JevRouteDecision{}
	jevInFly   = map[string]bool{}
)

// JevAPIKeyFromEnv returns the configured TypeSafe Jev API key from the environment (if any).
func JevAPIKeyFromEnv() string {
	return strings.TrimSpace(os.Getenv("TYPESAFE_API_KEY"))
}

// QueryJevDecisionAsync triggers a non-blocking single-pass TypeSafe Jev API classification
// for lineText (when TYPESAFE_API_KEY is set), invoking onReady once cached.
// Returns the cached decision immediately if already available.
func QueryJevDecisionAsync(lineText string, onReady func()) (*JevRouteDecision, bool) {
	key := strings.TrimSpace(lineText)
	if key == "" {
		return nil, false
	}
	jevCacheMu.Lock()
	if dec, ok := jevCache[key]; ok {
		jevCacheMu.Unlock()
		return dec, true
	}
	apiKey := JevAPIKeyFromEnv()
	if apiKey == "" || jevInFly[key] {
		jevCacheMu.Unlock()
		return nil, false
	}
	jevInFly[key] = true
	jevCacheMu.Unlock()

	go func() {
		dec := callTypeSafeJevAPI(apiKey, key)
		jevCacheMu.Lock()
		jevCache[key] = dec
		delete(jevInFly, key)
		jevCacheMu.Unlock()
		if onReady != nil {
			onReady()
		}
	}()
	return nil, false
}

func callTypeSafeJevAPI(apiKey, sentence string) *JevRouteDecision {
	payload := map[string]any{
		"model": "jev-latest",
		"state": map[string]any{
			"lab_sentence": sentence,
			"domain":       "wet-lab biological protocol note in AsterLab scientific notebook",
		},
		"questions": map[string]any{
			"should_suggest": map[string]any{
				"type": "noul",
				"instructions": "True ONLY if converting this lab note sentence into an interactive scientific tool widget " +
					"(timer, dilution/weighing/mastermix/seeding calculator, multi-well plate+group layout, or microscopy capture grid) " +
					"provides immediate practical bench utility. False for routine washing, pipetting, storage, objectives, or observations.",
			},
			"component_kind": map[string]any{
				"type":         "choice",
				"instructions": "Select the single best AsterLab interactive module for this sentence, or 'none' if it should remain plain text.",
				"criteria": map[string]string{
					"none":        "Plain text note, routine washing/pipetting/storage/discussion/observation; does NOT warrant a widget.",
					"timer":       "Active incubation, centrifugation, cyclic hot/cold rounds, timed curing stopwatch, or waiting/imaging window.",
					"calculator":  "Solution dilution (C1V1=C2V2), powder weighing (% w/v or MW), multi-well mastermix, or cell seeding math.",
					"plate_group": "Defines experimental groups/replicates and multi-well plate layout (6/12/24/96-well).",
					"attachment":  "Systematic multi-well microscopy/imaging capture grid with wells and multiple FOVs or channels per well.",
				},
			},
			"timer_mode": map[string]any{
				"type":         "choice",
				"instructions": "If component_kind is timer, choose the exact timer mode; otherwise choose na.",
				"criteria": map[string]string{
					"na":        "Not a timer sentence.",
					"countdown": "Single fixed-duration incubation, spin, or waiting period.",
					"cycle":     "Alternating multi-phase cycle repeated for multiple rounds.",
					"stopwatch": "Open-ended duration measured manually until endpoint is reached.",
					"window":    "Minimum waiting window (>= 2h) or post-anchor follow-up schedule (6h/12h/24h).",
				},
			},
			"calc_mode": map[string]any{
				"type":         "choice",
				"instructions": "If component_kind is calculator, choose the exact calculation mode; otherwise choose na.",
				"criteria": map[string]string{
					"na":        "Not a calculator sentence.",
					"dilution":  "Diluting stock concentration to target concentration and final volume.",
					"weighing":  "Weighing solid powder mass from % w/v or molar concentration + MW + volume.",
					"mastermix": "Scaling per-well reagent volumes across N wells with overage percentage.",
					"seeding":   "Calculating cell suspension volume from density, cells/well, and well count.",
				},
			},
		},
	}
	rawBody, err := json.Marshal(payload)
	if err != nil {
		return &JevRouteDecision{Err: err}
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.typesafe.ai/v1/systemone", bytes.NewReader(rawBody))
	if err != nil {
		return &JevRouteDecision{Err: err}
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &JevRouteDecision{Err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return &JevRouteDecision{Err: fmt.Errorf("jev http %d", resp.StatusCode)}
	}
	var parsed struct {
		Answers map[string]struct {
			Choice     string  `json:"choice"`
			Confidence float64 `json:"confidence"`
			Noul       float64 `json:"noul"`
		} `json:"answers"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return &JevRouteDecision{Err: err}
	}
	noul := parsed.Answers["should_suggest"].Noul
	kind := parsed.Answers["component_kind"].Choice
	conf := parsed.Answers["component_kind"].Confidence
	mode := ""
	if kind == "timer" {
		mode = parsed.Answers["timer_mode"].Choice
	} else if kind == "calculator" {
		mode = parsed.Answers["calc_mode"].Choice
	}
	if mode == "na" {
		mode = ""
	}
	if noul < 0.30 {
		kind = "none"
	}
	return &JevRouteDecision{
		Kind:       kind,
		Mode:       mode,
		Confidence: conf,
		Noul:       noul,
	}
}

var (
	reNLListPrefix  = regexp.MustCompile(`^[ \t]*(?:[-*+][ \t]+(?:\[[ xX]\][ \t]+)?|\d+[.)][ \t]+)`)
	reNLDuration    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(h|hr|hrs|小时|min|分钟|分|s|sec|秒)`)
	reNLRounds      = regexp.MustCompile(`(\d+)\s*(?:轮|个?循环|次循环|cycles?|rounds?)`)
	reNLConc        = regexp.MustCompile(`(\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)\s*(mg/mL|ug/mL|µg/mL|μg/mL|ng/mL|mg/ml|ug/ml|µg/ml|μg/ml|ng/ml|mM|uM|µM|μM|nM|M)(?:[^a-zA-Z/]|$)`)
	reNLPercentWV   = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*(?:\(?\s*w/v\s*\)?)?`)
	reNLVolume      = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(mL|uL|µL|μL|L)(?:\b|[^/a-zA-Z])`)
	reNLMW          = regexp.MustCompile(`(?i)(?:MW|分子量)\s*[=:：]?\s*(\d+(?:\.\d+)?)`)
	reNLPlateFmt    = regexp.MustCompile(`(4|6|12|24|48|96|384)\s*孔板?`)
	reNLReplicates  = regexp.MustCompile(`(?:各|每组)\s*(\d+)\s*(?:个?技术?复孔|复孔|孔)`)
	reNLFOV         = regexp.MustCompile(`(\d+)\s*个?(?:非重叠)?(?:[\d×xX]+\s*)?(?:明场|荧光)?视野`)
	reNLOverage     = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*%`)
	reNLWellCount   = regexp.MustCompile(`(?:共|总计?)\s*(\d+)\s*(?:个)?孔`)
	reNLDensity     = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?(?:e\d+|×10[⁴⁵⁶⁷⁸\^4-8]+)?)\s*(?:cells?/mL|个/mL)`)
	reNLCellsPerW   = regexp.MustCompile(`(?i)每孔[^0-9]*(\d+(?:\.\d+)?(?:e\d+|×10[³⁴⁵⁶\^3-6]+)?)\s*(?:cells?|个?细胞)?`)
	reNLRatio       = regexp.MustCompile(`(?i)(?:1\s*[:/：]\s*(\d+(?:\.\d+)?))`)
	reNLSerialFold  = regexp.MustCompile(`(\d+(?:\.\d+)?)\s*倍(?:梯度|倍比|连续)?稀释`)
	reNLMass        = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(mg|g|ug|µg|μg)`)
	reNLRPM         = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:rpm|转/分)`)
	reNLRadius      = regexp.MustCompile(`(?i)(?:半径|r)\s*[=:：]?\s*(\d+(?:\.\d+)?)\s*cm`)
	reNLWashTimes   = regexp.MustCompile(`(\d+)\s*次`)
	reNLClauseSplit = regexp.MustCompile(`(?:[；;。]|，\s*(?:再|然后|随后|接着|并)|然后|随后)`)
	reNLNegative    = regexp.MustCompile(`(清洗|吹打|冰箱|保存|存放|传代|汇合度|状态良好|评估.*影响|复核.*原因|观察结论)`)
)

// CleanNLSentence strips leading Markdown list/checkbox/step prefixes for semantic parsing.
func CleanNLSentence(raw string) string {
	s := strings.TrimSpace(raw)
	if hs, ok := ParseHeadingStepLine(s); ok {
		return strings.TrimSpace(hs.Title)
	}
	s = reNLListPrefix.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

func splitNaturalClauses(text string) []string {
	parts := reNLClauseSplit.Split(text, -1)
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(strings.Trim(p, "，,、；;。 "))
		if len([]rune(p)) >= 4 {
			out = append(out, p)
		}
		if len(out) >= 3 {
			break
		}
	}
	return out
}

// AnalyzeLabSentence routes (using optional Jev decision or local offline ONNX router) and
// deterministically extracts schema-valid AsterML v2 modules from a natural-language sentence.
func AnalyzeLabSentence(doc *Document, rawLine string, jev *JevRouteDecision) *NLSuggestion {
	sugs := AnalyzeLabSentenceWithContext(doc, rawLine, -1, jev)
	if len(sugs) == 0 {
		return nil
	}
	return sugs[0]
}

// AnalyzeLabSentenceWithContext executes the full SciGraph-aware NL->Module pipeline:
//  1. Deterministic AST structural zone gating (blocks heading/metadata/materials/review).
//  2. Model-driven continuous p_pos gating + is_multi semantic probe (no brittle past-tense regexes).
//  3. Multi-action clause split & aggregation (up to 3 clauses, ~2ms each on local INT8 ONNX).
//  4. Upstream SciGraph fact inheritance (@plate.sample_wells, upstream stock concentrations, anchors).
func AnalyzeLabSentenceWithContext(doc *Document, rawLine string, globalOffset int, jev *JevRouteDecision) []*NLSuggestion {
	rawTrim := strings.TrimSpace(rawLine)
	if rawTrim == "" || strings.HasPrefix(rawTrim, "#") || strings.HasPrefix(rawTrim, "```") || reThematicBreakLine.MatchString(rawTrim) {
		return nil
	}

	var ctx ScientificLineContext
	hasCtx := false
	if doc != nil && globalOffset >= 0 {
		if g := BuildSciGraph(doc); g != nil {
			ctx = g.ContextAt(globalOffset)
			hasCtx = true
			if !ctx.AllowedForNL() {
				return nil
			}
		}
	} else if doc != nil {
		// Even without a byte offset (unit tests), populate ActivePlate/Reagents from document graph
		if g := BuildSciGraph(doc); g != nil {
			ctx.Zone = ZoneStepBody
			ctx.ActivePlate = g.PrimaryPlate()
			ctx.UpstreamGroups = g.Groups
			ctx.UpstreamReagents = g.Reagents
			ctx.UpstreamAnchors = g.Anchors
			hasCtx = true
		}
	}

	text := CleanNLSentence(rawLine)
	if len([]rune(text)) < 4 {
		return nil
	}

	routeOne := func(sent string, useMicro bool, explicitJev *JevRouteDecision) (kind, mode string, conf, noul float64, isMulti bool, source string, hasMultiProbe bool) {
		if explicitJev != nil && explicitJev.Err == nil && explicitJev.Kind != "" {
			return explicitJev.Kind, explicitJev.Mode, explicitJev.Confidence, explicitJev.Noul, explicitJev.IsMulti, firstNonEmpty(explicitJev.Source, "Jev"), false
		}
		if onnxRouter := DefaultNLOnnxRouter(); onnxRouter != nil {
			q := sent
			if useMicro && hasCtx {
				q = ctx.MicroContextText(sent)
			}
			dec, ok := onnxRouter.Route(q)
			if !ok || dec == nil {
				return "none", "", 0, 0, false, "本地语义", onnxRouter.HasMultiActionProbe()
			}
			return dec.Kind, dec.Mode, dec.Confidence, dec.Noul, dec.IsMulti, firstNonEmpty(dec.Source, "本地语义"), onnxRouter.HasMultiActionProbe()
		}
		if reNLNegative.MatchString(sent) && !strings.Contains(sent, "稀释") && !strings.Contains(sent, "交替") && !strings.Contains(sent, "孔板") && !strings.Contains(sent, "视野") {
			return "none", "", 0, 0, false, "本地速判", false
		}
		k, m, c, n := localRouteSentence(sent)
		return k, m, c, n, false, "本地速判", false
	}

	kind, mode, conf, noul, isMulti, source, hasMultiProbe := routeOne(text, true, jev)
	// Model-driven whole-sentence p_pos gate (never split negative/past-tense/discussion sentences!)
	if noul < 0.25 && (kind == "" || kind == "none") {
		return nil
	}

	// Multi-action clause split & aggregation:
	// - When v1.3.0+ semantic multi-action probe is present, trust `isMulti` directly.
	// - Before v1.3.0 loads, gracefully probe natural delimiters only if whole-sentence p_pos >= 0.40
	//   and >= 2 clauses independently route cleanly to distinct actionable widgets.
	clauses := splitNaturalClauses(text)
	shouldFanOut := false
	if len(clauses) >= 2 && jev == nil {
		if hasMultiProbe {
			shouldFanOut = isMulti && noul >= 0.30
		} else if noul >= 0.40 && reNLClauseSplit.MatchString(text) {
			shouldFanOut = true
		}
	}
	if shouldFanOut {
		var multiSugs []*NLSuggestion
		seen := map[string]bool{}
		for _, cl := range clauses {
			ck, cm, cc, cn, _, cs, _ := routeOne(cl, false, nil)
			if ck == "" || ck == "none" || cn < 0.50 {
				continue
			}
			if hasCtx && !ctx.AllowsKind(ck) {
				continue
			}
			s := buildValidatedSuggestion(doc, ctx, hasCtx, cl, ck, cm, cc, cn, cs)
			if s == nil {
				continue
			}
			sig := s.Kind + ":" + s.Mode + ":" + s.Summary
			if !seen[sig] {
				seen[sig] = true
				multiSugs = append(multiSugs, s)
			}
		}
		if len(multiSugs) >= 2 {
			return multiSugs
		}
	}

	if kind == "" || kind == "none" {
		return nil
	}
	if hasCtx && !ctx.AllowsKind(kind) {
		return nil
	}
	single := buildValidatedSuggestion(doc, ctx, hasCtx, text, kind, mode, conf, noul, source)
	if single == nil {
		return nil
	}
	return []*NLSuggestion{single}
}

func buildValidatedSuggestion(doc *Document, ctx ScientificLineContext, hasCtx bool, text, kind, mode string, conf, noul float64, source string) *NLSuggestion {
	var sug *NLSuggestion
	switch kind {
	case "timer":
		sug = extractTimerSuggestionCtx(doc, ctx, hasCtx, text, mode)
	case "calculator":
		sug = extractCalculatorSuggestionCtx(doc, ctx, hasCtx, text, mode)
	case "plate_group":
		sug = extractPlateGroupSuggestion(doc, text)
	case "attachment":
		sug = extractAttachmentSuggestionCtx(doc, ctx, hasCtx, text)
	}
	if sug == nil || len(sug.Modules) == 0 {
		return nil
	}
	for _, m := range sug.Modules {
		k := textValue(m, "kind")
		if err := specValidator(k)(m); err != nil {
			return nil
		}
	}
	sug.SourceText = text
	sug.Confidence = conf
	sug.Noul = noul
	sug.Source = source
	return sug
}

func localRouteSentence(text string) (kind, mode string, conf, noul float64) {
	hasDur := reNLDuration.MatchString(text)
	hasRounds := reNLRounds.MatchString(text)
	hasPlate := reNLPlateFmt.MatchString(text)
	hasFOV := reNLFOV.MatchString(text)
	concs := reNLConc.FindAllString(text, -1)

	// 1. Multi-well microscopy capture grid
	if (hasFOV && (strings.Contains(text, "采图") || strings.Contains(text, "拍照") || strings.Contains(text, "成像") || strings.Contains(text, "视野"))) ||
		(hasPlate && (strings.Contains(text, "采图") || strings.Contains(text, "荧光拍照") || strings.Contains(text, "共聚焦"))) {
		return "attachment", "", 0.94, 0.82
	}
	// 2. Calculators (dilution / weighing / mastermix / seeding) — allow partial-parameter sentences for fallback UI
	if strings.Contains(text, "稀释") && (len(concs) >= 1 || reNLRatio.MatchString(text) || strings.Contains(text, "倍比") || strings.Contains(text, "母液") || strings.Contains(text, "工作液")) {
		return "calculator", "dilution", 0.96, 0.86
	}
	if (strings.Contains(text, "悬液密度") || strings.Contains(text, "cells/mL")) && (strings.Contains(text, "接种") || strings.Contains(text, "每孔") || strings.Contains(text, "铺")) {
		return "calculator", "seeding", 0.92, 0.80
	}
	if strings.Contains(text, "预混") || (strings.Contains(text, "余量") && strings.Contains(text, "每孔") && strings.Contains(text, "uL")) {
		return "calculator", "mastermix", 0.92, 0.84
	}
	if (strings.Contains(text, "称") || strings.Contains(text, "配制") || strings.Contains(text, "配")) && (reNLMW.MatchString(text) || strings.Contains(strings.ToLower(text), "w/v") || strings.Contains(text, "冻干粉") || strings.Contains(text, "干粉")) {
		return "calculator", "weighing", 0.94, 0.82
	}
	// 3. Plate + experimental groups layout
	if hasPlate && (strings.Contains(text, "复孔") || strings.Contains(text, "对照") || strings.Contains(text, "组")) {
		return "plate_group", "", 0.95, 0.78
	}
	// 4. Timers (cycle / stopwatch / window / countdown)
	if (hasDur && (hasRounds || strings.Contains(text, "交替") || strings.Contains(text, "循环"))) ||
		(hasRounds && (strings.Contains(text, "交替") || strings.Contains(text, "循环"))) {
		return "timer", "cycle", 0.96, 0.74
	}
	if strings.Contains(text, "直至") && (strings.Contains(text, "记录") || strings.Contains(text, "秒数") || strings.Contains(text, "固化")) {
		return "timer", "stopwatch", 0.90, 0.65
	}
	if strings.Contains(text, "至少") || strings.Contains(text, "≥") || (strings.Contains(text, "t0") && len(reNLDuration.FindAllString(text, -1)) >= 2) {
		return "timer", "window", 0.88, 0.58
	}
	if hasDur && (strings.Contains(text, "孵育") || strings.Contains(text, "离心") || strings.Contains(text, "水浴") || strings.Contains(text, "照射") || strings.Contains(text, "固化") || strings.Contains(text, "浸泡") || strings.Contains(text, "静置") || strings.Contains(text, "染核") || strings.Contains(text, "封闭") || strings.Contains(text, "透膜") || strings.Contains(text, "固定") || strings.Contains(text, "裂解") || strings.Contains(text, "灭活") || strings.Contains(text, "变性")) {
		return "timer", "countdown", 0.90, 0.55
	}
	return "none", "", 0, 0
}

func normDurationUnit(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "h", "hr", "hrs", "小时":
		return "h"
	case "min", "分钟", "分":
		return "min"
	default:
		return "s"
	}
}

func extractTimerSuggestionCtx(doc *Document, ctx ScientificLineContext, hasCtx bool, text, mode string) *NLSuggestion {
	durs := reNLDuration.FindAllStringSubmatchIndex(text, -1)
	id := nextUniqueBlockID(doc, "timer-nl")

	switch mode {
	case "cycle":
		rounds := 3
		roundsMissing := true
		if rm := reNLRounds.FindStringSubmatch(text); len(rm) >= 2 {
			if n, err := strconv.Atoi(rm[1]); err == nil && n >= 1 {
				rounds = n
				roundsMissing = false
			}
		} else if wm := reNLWashTimes.FindStringSubmatch(text); len(wm) >= 2 {
			if n, err := strconv.Atoi(wm[1]); err == nil && n >= 2 {
				rounds = n
				roundsMissing = false
			}
		}
		var phases []any
		var phaseLabels []string
		var missing []string
		if roundsMissing {
			missing = append(missing, "循环轮数")
		}
		totalSec := 0
		prevEnd := 0
		for i, idx := range durs {
			if i >= 4 {
				break
			}
			val, _ := strconv.ParseFloat(text[idx[2]:idx[3]], 64)
			unit := normDurationUnit(text[idx[4]:idx[5]])
			segName := strings.TrimSpace(text[prevEnd:idx[0]])
			segName = strings.TrimLeft(segName, "与和再先，,、 ")
			segName = strings.TrimSpace(segName)
			if segName == "" {
				segName = fmt.Sprintf("阶段 %d", i+1)
			}
			if len([]rune(segName)) > 14 {
				r := []rune(segName)
				segName = string(r[len(r)-14:])
			}
			phases = append(phases, map[string]any{
				"name":     segName,
				"mode":     "fixed",
				"duration": map[string]any{"value": val, "unit": unit},
			})
			phaseLabels = append(phaseLabels, fmt.Sprintf("%s %s %s", segName, formatNumber(val), unit))
			totalSec += durationToSec(val, unit)
			prevEnd = idx[1]
		}
		phasesMissing := false
		if len(phases) == 0 {
			phasesMissing = true
			missing = append(missing, "各阶段时长")
			phases = append(phases,
				map[string]any{"name": "阶段 1", "mode": "fixed", "duration": map[string]any{"value": 5, "unit": "min"}},
				map[string]any{"name": "阶段 2", "mode": "fixed", "duration": map[string]any{"value": 5, "unit": "min"}},
			)
			phaseLabels = []string{"阶段 1 5 min", "阶段 2 5 min"}
			totalSec = 600
		}
		totalSec *= rounds
		title := "多阶段循环计时"
		if strings.Contains(text, "水浴") || strings.Contains(text, "冰") {
			title = "冷热循环溶解计时"
		}
		mod := map[string]any{
			"kind":   "timer",
			"id":     id,
			"schema": 1,
			"title":  title,
			"mode":   "cycle",
			"rounds": rounds,
			"phases": phases,
		}
		return &NLSuggestion{
			Kind:      "timer",
			Mode:      "cycle",
			KindLabel: fmt.Sprintf("循环计时器 · %d 轮", rounds),
			Summary:   fmt.Sprintf("%s · 共 %s", strings.Join(phaseLabels, " ↔ "), formatClockHMS(totalSec)),
			Params: []NLParamKV{
				{Label: "循环轮数", Value: fmt.Sprintf("%d 轮", rounds), IsDefault: roundsMissing},
				{Label: "单轮阶段", Value: fmt.Sprintf("%d 阶段", len(phases)), IsDefault: phasesMissing},
				{Label: "总计预算", Value: formatClockHMS(totalSec), IsDefault: roundsMissing || phasesMissing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}

	case "stopwatch":
		title := "正计时秒表"
		if strings.Contains(text, "光") || strings.Contains(text, "固化") {
			title = "光固化正计时"
		}
		mod := map[string]any{
			"kind":   "timer",
			"id":     id,
			"schema": 1,
			"title":  title,
			"mode":   "stopwatch",
		}
		return &NLSuggestion{
			Kind:      "timer",
			Mode:      "stopwatch",
			KindLabel: "正计时秒表",
			Summary:   "点击启动记录真实操作耗时（秒表留痕）",
			Params:    []NLParamKV{{Label: "模式", Value: "手动起停正计时"}, {Label: "名称", Value: title}},
			Modules:   []map[string]any{mod},
		}

	case "window":
		val, unit := 2.0, "h"
		durMissing := true
		var missing []string
		if len(durs) > 0 {
			val, _ = strconv.ParseFloat(text[durs[0][2]:durs[0][3]], 64)
			unit = normDurationUnit(text[durs[0][4]:durs[0][5]])
			durMissing = false
		} else {
			missing = append(missing, "等待时长")
		}
		anchor := "t0"
		inheritedFrom := ""
		if hasCtx && len(ctx.UpstreamAnchors) > 0 && !strings.Contains(strings.ToLower(text), "t0") {
			lastAnc := ctx.UpstreamAnchors[len(ctx.UpstreamAnchors)-1]
			if lastAnc.ID != "" {
				anchor = lastAnc.ID
				inheritedFrom = "#" + lastAnc.ID
			}
		}
		winDesc := text
		if len([]rune(winDesc)) > 26 {
			winDesc = string([]rune(winDesc)[:26]) + "…"
		}
		mod := map[string]any{
			"kind":            "timer",
			"id":              id,
			"schema":          1,
			"title":           "时间锚点等待窗",
			"mode":            "window",
			"anchor":          anchor,
			"offset":          map[string]any{"value": val, "unit": unit},
			"schedule_window": winDesc,
		}
		return &NLSuggestion{
			Kind:          "timer",
			Mode:          "window",
			KindLabel:     "时间窗口提醒",
			Summary:       fmt.Sprintf("相对锚点 %s · +%s %s", anchor, formatNumber(val), unit),
			InheritedFrom: inheritedFrom,
			Params: []NLParamKV{
				{Label: "锚点", Value: anchor},
				{Label: "偏移时长", Value: fmt.Sprintf("%s %s", formatNumber(val), unit), IsDefault: durMissing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}

	default: // countdown
		val, unit := 10.0, "min"
		durMissing := true
		var missing []string
		if len(durs) > 0 {
			val, _ = strconv.ParseFloat(text[durs[0][2]:durs[0][3]], 64)
			unit = normDurationUnit(text[durs[0][4]:durs[0][5]])
			durMissing = false
		} else {
			missing = append(missing, "定时时长")
		}
		title := "实验倒计时"
		switch {
		case strings.Contains(text, "离心"):
			title = "离心倒计时"
		case strings.Contains(text, "二抗"):
			title = "二抗孵育倒计时"
		case strings.Contains(text, "一抗"):
			title = "一抗孵育倒计时"
		case strings.Contains(text, "DAPI") || strings.Contains(text, "染核"):
			title = "DAPI 染核倒计时"
		case strings.Contains(text, "孵育"):
			title = "孵育倒计时"
		case strings.Contains(text, "封闭"):
			title = "封闭倒计时"
		case strings.Contains(text, "固定"):
			title = "固定倒计时"
		case strings.Contains(text, "洗膜") || strings.Contains(text, "漂洗"):
			title = "摇床洗膜倒计时"
		case strings.Contains(text, "光") || strings.Contains(text, "固化"):
			title = "光交联倒计时"
		}
		mod := map[string]any{
			"kind":     "timer",
			"id":       id,
			"schema":   1,
			"title":    title,
			"mode":     "countdown",
			"duration": map[string]any{"value": val, "unit": unit},
		}
		return &NLSuggestion{
			Kind:      "timer",
			Mode:      "countdown",
			KindLabel: "倒计时器",
			Summary:   fmt.Sprintf("%s · %s %s", title, formatNumber(val), unit),
			Params: []NLParamKV{
				{Label: "名称", Value: title},
				{Label: "定时时长", Value: fmt.Sprintf("%s %s", formatNumber(val), unit), IsDefault: durMissing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}
	}
}

func durationToSec(v float64, unit string) int {
	switch unit {
	case "h":
		return int(math.Round(v * 3600))
	case "min":
		return int(math.Round(v * 60))
	default:
		return int(math.Round(v))
	}
}

func concFactorToBase(unit string) (scale float64, family string) {
	if s, f := scimath.ConcFactorToBase(unit); f != "" {
		return s, f
	}
	return 1, scimath.NormalizeUnit(unit)
}

func extractCalculatorSuggestionCtx(doc *Document, ctx ScientificLineContext, hasCtx bool, text, mode string) *NLSuggestion {
	id := nextUniqueBlockID(doc, "calc-nl")
	lowText := strings.ToLower(text)

	// Direct RCF/RPM conversion detection when user writes centrifuge speed math
	if mode == "rcf_rpm" || (strings.Contains(lowText, "rcf") || strings.Contains(text, "相对离心力") || (reNLRPM.MatchString(text) && reNLRadius.MatchString(text))) {
		rpmVal := 1500.0
		radVal := 10.0
		rpmMissing, radMissing := true, true
		var missing []string
		if rm := reNLRPM.FindStringSubmatch(text); len(rm) >= 2 {
			rpmVal, _ = strconv.ParseFloat(rm[1], 64)
			rpmMissing = false
		} else {
			missing = append(missing, "转速 RPM")
		}
		if rdm := reNLRadius.FindStringSubmatch(text); len(rdm) >= 2 {
			radVal, _ = strconv.ParseFloat(rdm[1], 64)
			radMissing = false
		} else {
			missing = append(missing, "转子半径 r")
		}
		rcfVal := round4(0.00001118 * radVal * rpmVal * rpmVal)
		mod := map[string]any{
			"kind": "calculator", "id": id, "schema": 1, "title": "离心力换算 (RCF ↔ RPM)", "mode": "formula",
			"rows": []any{
				map[string]any{"key": "r", "label": "转子有效半径", "kind": "input", "value": radVal, "unit": "cm"},
				map[string]any{"key": "RPM", "label": "设定转速", "kind": "input", "value": rpmVal, "unit": "rpm"},
				map[string]any{"key": "RCF", "label": "相对离心力", "kind": "formula", "formula": `0.00001118 \times r \times RPM^{2}`, "unit": "×g"},
			},
		}
		return &NLSuggestion{
			Kind: "calculator", Mode: "rcf_rpm", KindLabel: "离心力换算计算器",
			Summary: fmt.Sprintf("RCF ≈ %s ×g", formatNumber(rcfVal)),
			Params: []NLParamKV{
				{Label: "半径 r", Value: fmt.Sprintf("%s cm", formatNumber(radVal)), IsDefault: radMissing},
				{Label: "转速 RPM", Value: fmt.Sprintf("%s rpm", formatNumber(rpmVal)), IsDefault: rpmMissing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}
	}

	switch mode {
	case "dilution":
		concs := reNLConc.FindAllStringSubmatch(text, -1)
		vols := reNLVolume.FindAllStringSubmatch(text, -1)
		// 1a. Serial dilution detection ("倍比稀释" / "梯度稀释")
		if strings.Contains(text, "倍比") || strings.Contains(text, "梯度稀释") || reNLSerialFold.MatchString(text) {
			fold := 2.0
			if sm := reNLSerialFold.FindStringSubmatch(text); len(sm) >= 2 {
				if fv, err := strconv.ParseFloat(sm[1], 64); err == nil && fv > 1 {
					fold = fv
				}
			}
			vStep := 100.0
			vMissing := true
			var missing []string
			if len(vols) >= 1 {
				vv, _ := strconv.ParseFloat(vols[0][1], 64)
				vu := strings.ReplaceAll(strings.ReplaceAll(vols[0][2], "µ", "u"), "μ", "u")
				if strings.EqualFold(vu, "mL") {
					vStep = vv * 1000
				} else {
					vStep = vv
				}
				vMissing = false
			} else {
				missing = append(missing, "每级留存体积")
			}
			vTr := round4(vStep / (fold - 1))
			mod := map[string]any{
				"kind": "calculator", "id": id, "schema": 1, "title": "连续倍比稀释", "mode": "formula",
				"rows": []any{
					map[string]any{"key": "D", "label": "单级稀释倍数", "kind": "input", "value": fold, "unit": "倍"},
					map[string]any{"key": "V_step", "label": "每级终留存体积", "kind": "input", "value": vStep, "unit": "uL"},
					map[string]any{"key": "V_tr", "label": "逐级转移体积", "kind": "formula", "formula": `\frac{V_{step}}{D - 1}`, "unit": "uL"},
				},
			}
			return &NLSuggestion{
				Kind: "calculator", Mode: "serial_dilution", KindLabel: "连续倍比稀释计算器",
				Summary: fmt.Sprintf("每级预加 %s µL 稀释液，逐级转移 %s µL (%s倍)", formatNumber(vStep), formatNumber(vTr), formatNumber(fold)),
				Params: []NLParamKV{
					{Label: "倍比 D", Value: fmt.Sprintf("%s 倍", formatNumber(fold))},
					{Label: "每级体积", Value: fmt.Sprintf("%s µL", formatNumber(vStep)), IsDefault: vMissing},
				},
				MissingSlots: missing,
				Modules:      []map[string]any{mod},
			}
		}

		// 1b. Ratio dilution detection (e.g. `1:1000`, `1/1000`, `1:500` without explicit C1/C2 concentrations)
		if len(concs) == 0 {
			if rm := reNLRatio.FindStringSubmatch(text); len(rm) >= 2 {
				ratioN, _ := strconv.ParseFloat(rm[1], 64)
				if ratioN > 1 {
					v2uL := 1000.0
					v2Disp := "1 mL"
					v2Missing := true
					var missing []string
					if len(vols) >= 1 {
						vv, _ := strconv.ParseFloat(vols[len(vols)-1][1], 64)
						vu := strings.ReplaceAll(strings.ReplaceAll(vols[len(vols)-1][2], "µ", "u"), "μ", "u")
						v2Disp = fmt.Sprintf("%s %s", formatNumber(vv), vu)
						switch vu {
						case "mL":
							v2uL = vv * 1000
						case "L":
							v2uL = vv * 1e6
						default:
							v2uL = vv
						}
						v2Missing = false
					} else {
						missing = append(missing, "终体积 V₂")
					}
					v1 := round4(v2uL / ratioN)
					vdil := round4(v2uL - v1)
					mod := map[string]any{
						"kind": "calculator", "id": id, "schema": 1, "title": fmt.Sprintf("比例稀释配制 (1:%s)", formatNumber(ratioN)), "mode": "formula",
						"rows": []any{
							map[string]any{"key": "N", "label": "稀释倍数 (1:N)", "kind": "input", "value": ratioN, "unit": "倍"},
							map[string]any{"key": "V_2", "label": "配制终体积", "kind": "input", "value": v2uL, "unit": "uL"},
							map[string]any{"key": "V_1", "label": "取母液体积", "kind": "formula", "formula": `\frac{V_{2}}{N}`, "unit": "uL"},
							map[string]any{"key": "V_dil", "label": "补加稀释液", "kind": "formula", "formula": `V_{2} - \frac{V_{2}}{N}`, "unit": "uL"},
						},
					}
					return &NLSuggestion{
						Kind: "calculator", Mode: "ratio_dilution", KindLabel: "比例稀释计算器 (1:N)",
						Summary: fmt.Sprintf("取原液 %s µL + 补稀释液 %s µL", formatNumber(v1), formatNumber(vdil)),
						Params: []NLParamKV{
							{Label: "稀释比", Value: fmt.Sprintf("1:%s", formatNumber(ratioN))},
							{Label: "终体积 V₂", Value: v2Disp, IsDefault: v2Missing},
						},
						MissingSlots: missing,
						Modules:      []map[string]any{mod},
					}
				}
			}
		}

		reagent := "母液"
		if m := extractReagentName(text); m != "" {
			reagent = m
		}
		c1v, c1u := 100.0, "ug/mL"
		c2v, c2u := 0.05, "ug/mL"
		c1Missing, c2Missing, v2Missing := true, true, true
		inheritedFrom := ""
		var missing []string
		if len(concs) >= 2 {
			v1, _ := strconv.ParseFloat(concs[0][1], 64)
			u1 := strings.ReplaceAll(strings.ReplaceAll(concs[0][2], "µ", "u"), "μ", "u")
			v2, _ := strconv.ParseFloat(concs[1][1], 64)
			u2 := strings.ReplaceAll(strings.ReplaceAll(concs[1][2], "µ", "u"), "μ", "u")
			s1, f1 := concFactorToBase(u1)
			s2, f2 := concFactorToBase(u2)
			c1v, c1u = v1, u1
			if f1 == f2 && s1 > 0 {
				c2v = round4(v2 * s2 / s1)
				c2u = u1
			} else {
				c2v, c2u = v2, u2
				if (f1 == "mass_conc" && f2 == "molar") || (f1 == "molar" && f2 == "mass_conc") {
					missing = append(missing, "分子量 MW（跨量纲换算）")
				}
			}
			c1Missing, c2Missing = false, false
		} else if len(concs) == 1 {
			// Only one concentration mentioned — typically target concentration C2 ("稀释至 50 ng/mL")
			vOnly, _ := strconv.ParseFloat(concs[0][1], 64)
			uOnly := strings.ReplaceAll(strings.ReplaceAll(concs[0][2], "µ", "u"), "μ", "u")
			if strings.Contains(text, "稀释至") || strings.Contains(text, "稀释到") || strings.Contains(text, "稀释成") {
				c2v, c2u = vOnly, uOnly
				c2Missing = false
				// Try inheriting upstream stock concentration from SciGraph with physical C1 > C2 + dimension guard!
				if hasCtx && reagent != "母液" {
					if up := ctx.LookupReagentForDilution(reagent, c2v, c2u); up != nil && up.StockVal > 0 && up.StockUnit != "" {
						c1v, c1u = up.StockVal, up.StockUnit
						s1, f1 := concFactorToBase(c1u)
						s2, f2 := concFactorToBase(c2u)
						if f1 == f2 && s1 > 0 {
							c2v = round4(c2v * s2 / s1)
							c2u = c1u
						} else if f1 == "mass_conc" && f2 == "molar" && up.MW > 0 && s2 > 0 {
							// Convert mass-conc stock to target molar unit via MW: nM = (ng/mL) * 1000 / MW
							c1v = round4(((c1v * s1) * 1000.0 / up.MW) / s2)
							c1u = c2u
						} else if f1 == "molar" && f2 == "mass_conc" && up.MW > 0 && s2 > 0 {
							// Convert molar stock to target mass-conc unit via MW: ng/mL = nM * MW / 1000
							c1v = round4(((c1v * s1) * up.MW / 1000.0) / s2)
							c1u = c2u
						}
						c1Missing = false
						if up.SourceModuleID == "prose" {
							inheritedFrom = "文档试剂定义"
						} else {
							inheritedFrom = "@" + up.SourceModuleID
						}
					}
				}
				if c1Missing {
					c1v, c1u = round4(vOnly*1000), uOnly
					needMW := false
					if hasCtx && reagent != "母液" {
						if rawUp := ctx.LookupReagent(reagent); rawUp != nil && rawUp.StockVal > 0 && rawUp.StockUnit != "" && rawUp.MW <= 0 {
							_, fUp := concFactorToBase(rawUp.StockUnit)
							_, fTgt := concFactorToBase(c2u)
							if (fUp == "mass_conc" && fTgt == "molar") || (fUp == "molar" && fTgt == "mass_conc") {
								needMW = true
							}
						}
					}
					if needMW {
						missing = append(missing, "分子量 MW（跨量纲换算）")
					} else {
						missing = append(missing, "母液浓度 C₁")
					}
				}
			} else {
				c1v, c1u = vOnly, uOnly
				c1Missing = false
				c2v, c2u = round4(vOnly/100), uOnly
				missing = append(missing, "目标浓度 C₂")
			}
		} else {
			missing = append(missing, "母液浓度 C₁", "目标浓度 C₂")
		}
		v2uL := 5000.0
		v2Disp := "5 mL"
		if len(vols) >= 1 {
			vv, _ := strconv.ParseFloat(vols[len(vols)-1][1], 64)
			vu := strings.ReplaceAll(strings.ReplaceAll(vols[len(vols)-1][2], "µ", "u"), "μ", "u")
			v2Disp = fmt.Sprintf("%s %s", formatNumber(vv), vu)
			switch vu {
			case "mL":
				v2uL = vv * 1000
			case "L":
				v2uL = vv * 1e6
			default:
				v2uL = vv
			}
			v2Missing = false
		} else {
			missing = append(missing, "终体积 V₂")
		}
		mod := map[string]any{
			"kind":   "calculator",
			"id":     id,
			"schema": 1,
			"title":  fmt.Sprintf("%s 稀释配制 (C₁V₁ = C₂V₂)", reagent),
			"mode":   "formula",
			"rows": []any{
				map[string]any{"key": "C_1", "label": reagent + " 母液浓度", "kind": "input", "value": c1v, "unit": c1u},
				map[string]any{"key": "C_2", "label": "目标工作浓度", "kind": "input", "value": c2v, "unit": c2u},
				map[string]any{"key": "V_2", "label": "配制终体积", "kind": "input", "value": v2uL, "unit": "uL"},
				map[string]any{"key": "V_1", "label": "取母液体积", "kind": "formula", "formula": `\frac{C_{2} \times V_{2}}{C_{1}}`, "unit": "uL"},
				map[string]any{"key": "V_dil", "label": "补加稀释液", "kind": "formula", "formula": `V_{2} - \frac{C_{2} \times V_{2}}{C_{1}}`, "unit": "uL"},
			},
		}
		stockUL := round4(c2v * v2uL / c1v)
		dilUL := round4(v2uL - stockUL)
		return &NLSuggestion{
			Kind:          "calculator",
			Mode:          "dilution",
			KindLabel:     "稀释配液计算器",
			Summary:       fmt.Sprintf("取 %s 母液 %s µL + 补稀释液 %s µL", reagent, formatNumber(stockUL), formatNumber(dilUL)),
			InheritedFrom: inheritedFrom,
			Params: []NLParamKV{
				{Label: "母液 C₁", Value: fmt.Sprintf("%s %s", formatNumber(c1v), DisplayUnit(c1u)), IsDefault: c1Missing},
				{Label: "目标 C₂", Value: fmt.Sprintf("%s %s", formatNumber(c2v), DisplayUnit(c2u)), IsDefault: c2Missing},
				{Label: "终体积 V₂", Value: v2Disp, IsDefault: v2Missing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}

	case "weighing":
		vols := reNLVolume.FindAllStringSubmatch(text, -1)
		volML := 2.0
		volMissing := true
		var missing []string
		if len(vols) >= 1 {
			vv, _ := strconv.ParseFloat(vols[0][1], 64)
			vu := strings.ReplaceAll(strings.ReplaceAll(vols[0][2], "µ", "u"), "μ", "u")
			if vu == "uL" {
				volML = vv / 1000
			} else {
				volML = vv
			}
			volMissing = false
		} else {
			missing = append(missing, "配制体积 V")
		}
		if mwM := reNLMW.FindStringSubmatch(text); len(mwM) >= 2 {
			mw, _ := strconv.ParseFloat(mwM[1], 64)
			concMM := 10.0
			concMissing := true
			if cs := reNLConc.FindAllStringSubmatch(text, -1); len(cs) >= 1 {
				concMM, _ = strconv.ParseFloat(cs[0][1], 64)
				concMissing = false
			} else {
				missing = append(missing, "目标浓度 C")
			}
			massMG := round4(concMM * volML * mw / 1000)
			mod := map[string]any{
				"kind":   "calculator",
				"id":     id,
				"schema": 1,
				"title":  "摩尔称量配液计算",
				"mode":   "formula",
				"rows": []any{
					map[string]any{"key": "C", "label": "目标摩尔浓度", "kind": "input", "value": concMM, "unit": "mM"},
					map[string]any{"key": "V", "label": "配制终体积", "kind": "input", "value": volML, "unit": "mL"},
					map[string]any{"key": "MW", "label": "分子量", "kind": "input", "value": mw, "unit": "g/mol"},
					map[string]any{"key": "m", "label": "需称取溶质质量", "kind": "formula", "formula": `\frac{C \times V \times MW}{1000}`, "unit": "mg"},
				},
			}
			return &NLSuggestion{
				Kind:      "calculator",
				Mode:      "weighing",
				KindLabel: "摩尔称量计算器",
				Summary:   fmt.Sprintf("需称取溶质 %s mg，定容至 %s mL", formatNumber(massMG), formatNumber(volML)),
				Params: []NLParamKV{
					{Label: "浓度 C", Value: fmt.Sprintf("%s mM", formatNumber(concMM)), IsDefault: concMissing},
					{Label: "体积 V", Value: fmt.Sprintf("%s mL", formatNumber(volML)), IsDefault: volMissing},
					{Label: "分子量 MW", Value: fmt.Sprintf("%s g/mol", formatNumber(mw))},
				},
				MissingSlots: missing,
				Modules:      []map[string]any{mod},
			}
		}
		// % w/v weighing (guard against accidentally grabbing solvent % like "0.25% LAP" when solute mass is explicitly written!)
		pct := 5.0
		pctMissing := true
		pctIdxs := reNLPercentWV.FindAllStringSubmatchIndex(text, -1)
		for _, idx := range pctIdxs {
			after := ""
			if idx[1] < len(text) {
				end := min(len(text), idx[1]+16)
				after = strings.ToUpper(text[idx[1]:end])
			}
			// Skip solvent/buffer photoinitiator % (e.g. "0.25% LAP 溶液") if another % exists or pre-computed solid masses exist
			isSolvent := strings.Contains(after, "LAP") || strings.Contains(after, "FBS") || strings.Contains(after, "DMSO")
			if isSolvent && (len(pctIdxs) > 1 || len(reNLMass.FindAllString(text, -1)) >= 1) {
				continue
			}
			pct, _ = strconv.ParseFloat(text[idx[2]:idx[3]], 64)
			pctMissing = false
			break
		}
		if pctMissing {
			// If sentence gives explicit solid mass (e.g. "60/100/140 mg 干粉溶于 2 mL"), back-calculate % w/v or mark missing cleanly
			if ms := reNLMass.FindAllStringSubmatch(text, -1); len(ms) >= 1 && !volMissing && volML > 0 {
				mv, _ := strconv.ParseFloat(ms[0][1], 64)
				if strings.EqualFold(ms[0][2], "g") {
					mv *= 1000
				}
				pct = round4(mv / (volML * 10))
				pctMissing = false
			} else {
				missing = append(missing, "目标浓度 (% w/v)")
			}
		}
		massMG := round4(pct * volML * 10)
		mod := map[string]any{
			"kind":   "calculator",
			"id":     id,
			"schema": 1,
			"title":  "称量配液计算 (% w/v)",
			"mode":   "formula",
			"rows": []any{
				map[string]any{"key": "C_w", "label": "目标质量体积浓度", "kind": "input", "value": pct, "unit": "% w/v"},
				map[string]any{"key": "V", "label": "配制终体积", "kind": "input", "value": volML, "unit": "mL"},
				map[string]any{"key": "m", "label": "需称取冻干粉", "kind": "formula", "formula": `C_{w} \times V \times 10`, "unit": "mg"},
			},
		}
		return &NLSuggestion{
			Kind:      "calculator",
			Mode:      "weighing",
			KindLabel: "% w/v 称量计算器",
			Summary:   fmt.Sprintf("需称取固体 %s mg，定容至 %s mL", formatNumber(massMG), formatNumber(volML)),
			Params: []NLParamKV{
				{Label: "目标浓度", Value: fmt.Sprintf("%s%% w/v", formatNumber(pct)), IsDefault: pctMissing},
				{Label: "配制体积", Value: fmt.Sprintf("%s mL", formatNumber(volML)), IsDefault: volMissing},
				{Label: "称量质量", Value: fmt.Sprintf("%s mg", formatNumber(massMG)), IsDefault: pctMissing || volMissing},
			},
			MissingSlots: missing,
			Modules:      []map[string]any{mod},
		}

	case "mastermix":
		wells := 24.0
		var wellsRowVal any = 24.0
		wellsLabel := "反应孔数"
		inheritedFrom := ""
		wellsMissing := true
		if pm := reNLPlateFmt.FindStringSubmatch(text); len(pm) >= 2 {
			wells, _ = strconv.ParseFloat(pm[1], 64)
			wellsRowVal = wells
			wellsMissing = false
		} else if wm := reNLWellCount.FindStringSubmatch(text); len(wm) >= 2 {
			wells, _ = strconv.ParseFloat(wm[1], 64)
			wellsRowVal = wells
			wellsMissing = false
		} else if hasCtx && ctx.ActivePlate != nil && ctx.ActivePlate.SampleWells > 0 {
			wells = float64(ctx.ActivePlate.SampleWells)
			multiPlate := doc != nil && len(BuildSciGraph(doc).Containers) >= 2
			if multiPlate {
				wellsRowVal = "@plate." + ctx.ActivePlate.ID + ".sample_wells"
				wellsLabel = "有效实验孔数"
			} else if ctx.ActivePlate.BlankWells > 0 {
				wellsRowVal = "@plate.sample_wells"
				wellsLabel = "有效实验孔数"
			} else {
				wellsRowVal = "@plate.wells"
			}
			inheritedFrom = "@" + ctx.ActivePlate.ID
			wellsMissing = false
		}
		overage := 0.15
		overageMissing := true
		if om := reNLOverage.FindStringSubmatch(text); len(om) >= 2 {
			if p, err := strconv.ParseFloat(om[1], 64); err == nil {
				overage = round4(p / 100.0)
				overageMissing = false
			}
		}
		perWell := 10.0
		perWellMissing := true
		if vols := reNLVolume.FindAllStringSubmatch(text, -1); len(vols) >= 1 {
			perWell, _ = strconv.ParseFloat(vols[0][1], 64)
			perWellMissing = false
		}
		var mmMissing []string
		if wellsMissing {
			mmMissing = append(mmMissing, "反应孔数 N")
		}
		if perWellMissing {
			mmMissing = append(mmMissing, "单孔体积 V₀")
		}
		tot := round4(wells * perWell * (1 + overage))
		mod := map[string]any{
			"kind":   "calculator",
			"id":     id,
			"schema": 1,
			"title":  "孔板预混液配制计算",
			"mode":   "formula",
			"rows": []any{
				map[string]any{"key": "N", "label": wellsLabel, "kind": "input", "value": wellsRowVal, "unit": "孔"},
				map[string]any{"key": "V_0", "label": "单孔加样体积", "kind": "input", "value": perWell, "unit": "uL"},
				map[string]any{"key": "r", "label": "富余系数", "kind": "input", "value": overage, "unit": ""},
				map[string]any{"key": "V_tot", "label": "预混液总体积", "kind": "formula", "formula": `N \times V_{0} \times (1 + r)`, "unit": "uL"},
			},
		}
		return &NLSuggestion{
			Kind:          "calculator",
			Mode:          "mastermix",
			KindLabel:     "预混液计算器",
			Summary:       fmt.Sprintf("%s 孔 × %s µL × (1+%s%%) = %s µL", formatNumber(wells), formatNumber(perWell), formatNumber(overage*100), formatNumber(tot)),
			InheritedFrom: inheritedFrom,
			MissingSlots:  mmMissing,
			Params: []NLParamKV{
				{Label: wellsLabel, Value: fmt.Sprintf("%s 孔", formatNumber(wells)), IsDefault: wellsMissing},
				{Label: "单孔体积", Value: fmt.Sprintf("%s µL", formatNumber(perWell)), IsDefault: perWellMissing},
				{Label: "富余比例", Value: fmt.Sprintf("+%s%%", formatNumber(overage*100)), IsDefault: overageMissing},
			},
			Modules: []map[string]any{mod},
		}

	case "seeding":
		density := 500000.0
		densityMissing := true
		if dm := reNLDensity.FindStringSubmatch(text); len(dm) >= 2 {
			if v, ok := parseSciNum(dm[1]); ok {
				density = v
				densityMissing = false
			}
		}
		perWell := 50000.0
		perWellMissing := true
		if cm := reNLCellsPerW.FindStringSubmatch(text); len(cm) >= 2 {
			if v, ok := parseSciNum(cm[1]); ok {
				perWell = v
				perWellMissing = false
			}
		}
		wells := 14.0
		var wellsRowVal any = 14.0
		wellsMissing := true
		inheritedFrom := ""
		if wm := reNLWellCount.FindStringSubmatch(text); len(wm) >= 2 {
			if v, err := strconv.ParseFloat(wm[1], 64); err == nil {
				wells = v
				wellsRowVal = v
				wellsMissing = false
			}
		} else if hasCtx && ctx.ActivePlate != nil && ctx.ActivePlate.SampleWells > 0 {
			wells = float64(ctx.ActivePlate.SampleWells)
			multiPlate := doc != nil && len(BuildSciGraph(doc).Containers) >= 2
			if multiPlate {
				wellsRowVal = "@plate." + ctx.ActivePlate.ID + ".sample_wells"
			} else if ctx.ActivePlate.BlankWells > 0 {
				wellsRowVal = "@plate.sample_wells"
			} else {
				wellsRowVal = "@plate.wells"
			}
			inheritedFrom = "@" + ctx.ActivePlate.ID
			wellsMissing = false
		}
		volUL := 500.0
		volMissing := true
		if vols := reNLVolume.FindAllStringSubmatch(text, -1); len(vols) >= 1 {
			volUL, _ = strconv.ParseFloat(vols[0][1], 64)
			volMissing = false
		}
		var seedMissing []string
		if densityMissing {
			seedMissing = append(seedMissing, "悬液密度")
		}
		if perWellMissing {
			seedMissing = append(seedMissing, "每孔细胞数")
		}
		if wellsMissing {
			seedMissing = append(seedMissing, "接种孔数")
		}
		if volMissing {
			seedMissing = append(seedMissing, "每孔体积")
		}
		suspML := round4(perWell * wells / density)
		medML := round4(volUL*wells/1000 - suspML)
		mod := map[string]any{
			"kind":   "calculator",
			"id":     id,
			"schema": 1,
			"title":  "细胞接种稀释计算",
			"mode":   "formula",
			"rows": []any{
				map[string]any{"key": "rho", "label": "实测悬液密度", "kind": "input", "value": density, "unit": "cells/mL"},
				map[string]any{"key": "n_w", "label": "每孔目标细胞数", "kind": "input", "value": perWell, "unit": "cells"},
				map[string]any{"key": "N", "label": "接种总孔数", "kind": "input", "value": wellsRowVal, "unit": "孔"},
				map[string]any{"key": "V_w", "label": "每孔接种体积", "kind": "input", "value": volUL, "unit": "uL"},
				map[string]any{"key": "V_cell", "label": "取细胞悬液", "kind": "formula", "formula": `\frac{n_{w} \times N}{\rho}`, "unit": "mL"},
				map[string]any{"key": "V_med", "label": "补加培养基", "kind": "formula", "formula": `\frac{V_{w} \times N}{1000} - \frac{n_{w} \times N}{\rho}`, "unit": "mL"},
			},
		}
		return &NLSuggestion{
			Kind:          "calculator",
			Mode:          "seeding",
			KindLabel:     "细胞接种计算器",
			Summary:       fmt.Sprintf("取细胞悬液 %s mL + 补培养基 %s mL", formatNumber(suspML), formatNumber(medML)),
			InheritedFrom: inheritedFrom,
			MissingSlots:  seedMissing,
			Params: []NLParamKV{
				{Label: "悬液密度", Value: fmt.Sprintf("%s cells/mL", formatNumber(density)), IsDefault: densityMissing},
				{Label: "每孔细胞", Value: formatNumber(perWell), IsDefault: perWellMissing},
				{Label: "接种孔数", Value: fmt.Sprintf("%s 孔", formatNumber(wells)), IsDefault: wellsMissing},
			},
			Modules: []map[string]any{mod},
		}

	default: // Formula-Uncertain Human-in-the-Loop Pre-fill: extract recognized scientific quantities, let user pick formula!
		var rows []any
		var params []NLParamKV
		var keys []string
		addVar := func(k, lbl string, val float64, unit string) {
			rows = append(rows, map[string]any{"key": k, "label": lbl, "kind": "input", "value": val, "unit": unit})
			params = append(params, NLParamKV{Label: lbl, Value: strings.TrimSpace(fmt.Sprintf("%s %s", formatNumber(val), DisplayUnit(unit)))})
			keys = append(keys, k)
		}
		for i, cm := range reNLConc.FindAllStringSubmatch(text, 2) {
			v, _ := strconv.ParseFloat(cm[1], 64)
			u := strings.ReplaceAll(strings.ReplaceAll(cm[2], "µ", "u"), "μ", "u")
			addVar(fmt.Sprintf("C_%d", i+1), fmt.Sprintf("浓度 %d", i+1), v, u)
		}
		for i, vm := range reNLVolume.FindAllStringSubmatch(text, 2) {
			v, _ := strconv.ParseFloat(vm[1], 64)
			u := strings.ReplaceAll(strings.ReplaceAll(vm[2], "µ", "u"), "μ", "u")
			addVar(fmt.Sprintf("V_%d", i+1), fmt.Sprintf("体积 %d", i+1), v, u)
		}
		for i, mm := range reNLMass.FindAllStringSubmatch(text, 2) {
			v, _ := strconv.ParseFloat(mm[1], 64)
			u := strings.ReplaceAll(strings.ReplaceAll(mm[2], "µ", "u"), "μ", "u")
			addVar(fmt.Sprintf("m_%d", i+1), fmt.Sprintf("质量 %d", i+1), v, u)
		}
		if len(rows) == 0 {
			addVar("x_1", "参数 1", 1, "")
			addVar("x_2", "参数 2", 1, "")
		}
		formulaExpr := keys[0]
		if len(keys) >= 2 {
			formulaExpr = keys[0] + ` \times ` + keys[1]
		}
		rows = append(rows, map[string]any{"key": "y", "label": "计算结果（可在公式库切换）", "kind": "formula", "formula": formulaExpr, "unit": ""})
		mod := map[string]any{
			"kind": "calculator", "id": id, "schema": 1, "title": "自定义公式计算器（已提取参数）", "mode": "formula", "rows": rows,
		}
		return &NLSuggestion{
			Kind:             "calculator",
			Mode:             "custom",
			KindLabel:        "自定义计算器 · 已提取变量",
			Summary:          fmt.Sprintf("已提取 %d 个科学变量，插入后可在「公式库」一键选择公式", len(keys)),
			FormulaUncertain: true,
			Params:           params,
			Modules:          []map[string]any{mod},
		}
	}
}

func parseSciNum(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if v, err := strconv.ParseFloat(s, 64); err == nil {
		return v, true
	}
	// Handle `5×10⁵` / `5×10^5`
	s = strings.ReplaceAll(s, "⁴", "^4")
	s = strings.ReplaceAll(s, "⁵", "^5")
	s = strings.ReplaceAll(s, "⁶", "^6")
	s = strings.ReplaceAll(s, "³", "^3")
	if idx := strings.Index(s, "×10^"); idx > 0 {
		base, err1 := strconv.ParseFloat(strings.TrimSpace(s[:idx]), 64)
		exp, err2 := strconv.ParseFloat(strings.TrimSpace(s[idx+len("×10^"):]), 64)
		if err1 == nil && err2 == nil {
			return base * math.Pow(10, exp), true
		}
	}
	return 0, false
}

func extractPlateGroupSuggestion(doc *Document, text string) *NLSuggestion {
	format := 12
	if pm := reNLPlateFmt.FindStringSubmatch(text); len(pm) >= 2 {
		if n, err := strconv.Atoi(pm[1]); err == nil {
			format = n
		}
	}
	repCount := 3
	if rm := reNLReplicates.FindStringSubmatch(text); len(rm) >= 2 {
		if n, err := strconv.Atoi(rm[1]); err == nil && n >= 1 {
			repCount = n
		}
	}

	// Extract group names cleanly (e.g., `3% / 5% / 7% GelMA` or `对照组、低剂量、中剂量、高剂量`)
	type parsedGrp struct {
		id   string
		name string
		role string
	}
	var groups []parsedGrp
	pctMatches := reNLPercentWV.FindAllStringSubmatch(text, -1)
	if len(pctMatches) >= 2 {
		baseMat := "处理组"
		for _, m := range []string{"GelMA", "FBS", "VEGF", "PEG", "Matrigel"} {
			if strings.Contains(strings.ToUpper(text), strings.ToUpper(m)) {
				baseMat = m
				break
			}
		}
		for i, pm := range pctMatches {
			groups = append(groups, parsedGrp{
				id:   fmt.Sprintf("g-nl-%d", i+1),
				name: fmt.Sprintf("%s%% %s", pm[1], baseMat),
				role: "treatment",
			})
		}
	} else {
		// Check named groups
		known := []struct{ kw, name, role string }{
			{"对照组", "对照组", "control"},
			{"阴性对照", "阴性对照", "control"},
			{"处理组", "处理组", "treatment"},
			{"低剂量", "低剂量", "treatment"},
			{"中剂量", "中剂量", "treatment"},
			{"高剂量", "高剂量", "treatment"},
		}
		for _, k := range known {
			if strings.Contains(text, k.kw) {
				groups = append(groups, parsedGrp{
					id:   fmt.Sprintf("g-nl-%d", len(groups)+1),
					name: k.name,
					role: k.role,
				})
			}
		}
	}
	if len(groups) == 0 {
		groups = []parsedGrp{
			{id: "g-nl-1", name: "对照组", role: "control"},
			{id: "g-nl-2", name: "实验组", role: "treatment"},
		}
	}
	hasPBSBlank := strings.Contains(strings.ToUpper(text), "PBS") || strings.Contains(text, "空白")
	if hasPBSBlank {
		groups = append(groups, parsedGrp{id: "g-nl-blank", name: "空白 / PBS", role: "control"})
	}

	geom, ok := geometryForPlate(format, "", 0, 0)
	if !ok {
		format = 12
		geom, _ = geometryForPlate(12, "", 0, 0)
	}

	// Delegate physical well coordinate assignment 100% to our deterministic Plate Layout Engine!
	// Model/NL extracts ONLY logical groups & replicate counts; zero LLM coordinate hallucinations.
	wantEdgeBlank := strings.Contains(text, "防蒸发") || strings.Contains(text, "边缘") ||
		strings.Contains(text, "外周") || strings.Contains(text, "四周") || strings.Contains(text, "第 4 列")
	dir := LayoutRowMajor
	if strings.Contains(text, "竖排") || strings.Contains(text, "按列") || strings.Contains(text, "8道") || strings.Contains(text, "8 道") {
		dir = LayoutColMajor
	}

	var layoutSpecs []PlateLayoutGroupSpec
	gtRows := make([]any, 0, len(groups))
	blankGroupID := ""
	for _, g := range groups {
		gtRows = append(gtRows, map[string]any{
			"id":                g.id,
			"name":              g.name,
			"role":              g.role,
			"technical_repeats": repCount,
		})
		lRole := g.role
		if g.id == "g-nl-blank" {
			lRole = "blank"
			blankGroupID = g.id
		}
		layoutSpecs = append(layoutSpecs, PlateLayoutGroupSpec{
			ID:         g.id,
			Name:       g.name,
			Role:       lRole,
			Replicates: repCount,
		})
	}

	plan := ComputePlateLayout(PlateLayoutSpec{
		Format:       format,
		Groups:       layoutSpecs,
		Direction:    dir,
		EdgeBlank:    wantEdgeBlank,
		BlankGroupID: blankGroupID,
	})

	gtID := nextUniqueBlockID(doc, "gt-nl")
	gtMod := map[string]any{
		"kind":   "group_table",
		"id":     gtID,
		"schema": 1,
		"title":  "实验分组",
		"rows":   gtRows,
	}
	mods := []map[string]any{gtMod}
	usedWells := 0
	for pIdx, pr := range plan.Plates {
		plID := nextUniqueBlockID(doc, fmt.Sprintf("plate-nl-%d", pIdx+1))
		if len(plan.Plates) == 1 {
			plID = nextUniqueBlockID(doc, "plate-nl")
		}
		plTitle := fmt.Sprintf("%d 孔板布局", pr.Format)
		if len(plan.Plates) > 1 {
			plTitle = fmt.Sprintf("%d 孔板 #%d（共 %d 块）", pr.Format, pIdx+1, len(plan.Plates))
		}
		plateGroups := map[string]any{}
		for gid, ws := range pr.Groups {
			arr := make([]any, len(ws))
			for i, w := range ws {
				arr[i] = w
			}
			plateGroups[gid] = arr
			usedWells += len(ws)
		}
		plMod := map[string]any{
			"kind":   "plate",
			"id":     plID,
			"schema": 1,
			"title":  plTitle,
			"format": pr.Format,
			"groups": plateGroups,
		}
		if len(pr.Roles) > 0 {
			plateRoles := map[string]any{}
			for w, rv := range pr.Roles {
				plateRoles[w] = rv
			}
			plMod["roles"] = plateRoles
		}
		mods = append(mods, plMod)
	}

	names := make([]string, 0, len(groups))
	for _, g := range groups {
		names = append(names, g.name)
	}
	kindLabel := fmt.Sprintf("%d 孔板 + %d 组", format, len(groups))
	if len(plan.Plates) > 1 {
		kindLabel = fmt.Sprintf("%d 块 %d 孔板 + %d 组", len(plan.Plates), format, len(groups))
	}
	summary := fmt.Sprintf("已自动顺排 %d 孔：%s", usedWells, strings.Join(names, " · "))
	if plan.EdgeBlankUsed {
		summary += "（已含外周/边缘 PBS 留空）"
	}
	if len(plan.Plates) > 1 {
		summary += fmt.Sprintf(" · 自动拆分 %d 块板", len(plan.Plates))
	}
	return &NLSuggestion{
		Kind:      "plate_group",
		KindLabel: kindLabel,
		Summary:   summary,
		Params: []NLParamKV{
			{Label: "容器规格", Value: fmt.Sprintf("%d 孔板 (%d×%d)", format, geom.Rows, geom.Columns)},
			{Label: "组别数", Value: fmt.Sprintf("%d 组", len(groups))},
			{Label: "每组复孔", Value: fmt.Sprintf("%d 复孔", repCount)},
		},
		Modules: mods,
	}
}

func extractAttachmentSuggestionCtx(doc *Document, ctx ScientificLineContext, hasCtx bool, text string) *NLSuggestion {
	slots := 6
	if fm := reNLFOV.FindStringSubmatch(text); len(fm) >= 2 {
		if n, err := strconv.Atoi(fm[1]); err == nil && n >= 1 {
			slots = n
		}
	}
	plateRef := ""
	wellCount := 9
	inheritedFrom := ""
	if hasCtx && ctx.ActivePlate != nil {
		plateRef = ctx.ActivePlate.ID
		if ctx.ActivePlate.SampleWells > 0 {
			wellCount = ctx.ActivePlate.SampleWells
		} else if ctx.ActivePlate.TotalWells > 0 {
			wellCount = ctx.ActivePlate.TotalWells
		}
		inheritedFrom = "@" + ctx.ActivePlate.ID
	} else if doc != nil {
		for _, m := range doc.Modules {
			if m.Kind == "plate" {
				plateRef = m.ID
				if m.Plate != nil && len(m.Plate.WellGroups()) > 0 {
					wellCount = len(m.Plate.WellGroups())
				}
				break
			}
		}
	}
	if wm := reNLWellCount.FindStringSubmatch(text); len(wm) >= 2 {
		if n, err := strconv.Atoi(wm[1]); err == nil && n >= 1 {
			wellCount = n
		}
	}
	id := nextUniqueBlockID(doc, "att-nl")
	mod := map[string]any{
		"kind":           "attachment",
		"id":             id,
		"schema":         1,
		"title":          "孔板网格采图槽位",
		"scope":          "step_evidence",
		"slots_per_cell": slots,
		"media_hint":     text,
	}
	if plateRef != "" {
		mod["scope"] = "container_grid"
		mod["container_ref"] = plateRef
	}
	return &NLSuggestion{
		Kind:          "attachment",
		KindLabel:     fmt.Sprintf("采图网格 · %d 视野/孔", slots),
		Summary:       fmt.Sprintf("%d 有效实验孔 × %d 视野 = 共 %d 个采图槽位", wellCount, slots, wellCount*slots),
		InheritedFrom: inheritedFrom,
		Params: []NLParamKV{
			{Label: "有效实验孔", Value: fmt.Sprintf("%d 孔", wellCount)},
			{Label: "每孔视野", Value: fmt.Sprintf("%d FOV", slots)},
			{Label: "预计槽位", Value: fmt.Sprintf("%d 张", wellCount*slots)},
		},
		Modules: []map[string]any{mod},
	}
}
