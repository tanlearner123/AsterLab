package asterdoc

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed spec/modules.json
var moduleSpecJSON []byte

//go:embed spec/foundation-v1.schema.json
var foundationSchemaJSON []byte

// FoundationSchema returns the machine-readable Foundation v1 schema shipped
// with the binary; it is a contract artifact, not the runtime validator.
func FoundationSchema() []byte { return append([]byte(nil), foundationSchemaJSON...) }

type specFile struct {
	SchemaVersion     int                 `json:"schemaVersion"`
	FoundationVersion int                 `json:"foundationVersion"`
	SDKVersion        string              `json:"sdkVersion"`
	RoleCapabilities  map[string][]string `json:"roleCapabilities"`
	Modules           []json.RawMessage   `json:"modules"`
}

type specModule struct {
	Descriptor
}

var (
	specDescriptors   []Descriptor
	specByKey         map[string]specModule
	specErr           error
	specFoundationVer int
	specSDKVersion    string
	roleCapabilities  map[string][]string
)

func init() { specErr = loadSpec() }

func loadSpec() error {
	var file specFile
	if err := json.Unmarshal(moduleSpecJSON, &file); err != nil {
		return fmt.Errorf("asterdoc module spec: %w", err)
	}
	if file.SchemaVersion != 1 || file.FoundationVersion != 1 {
		return fmt.Errorf("asterdoc module spec: unsupported schema/foundation version")
	}
	if strings.TrimSpace(file.SDKVersion) == "" {
		return fmt.Errorf("asterdoc module spec: sdkVersion is required")
	}
	specFoundationVer, specSDKVersion = file.FoundationVersion, file.SDKVersion
	roleCapabilities = file.RoleCapabilities
	if unitBySymbol == nil {
		if err := loadUnits(); err != nil {
			return err
		}
	}
	specByKey = map[string]specModule{}
	for _, raw := range file.Modules {
		var module specModule
		if err := json.Unmarshal(raw, &module); err != nil {
			return fmt.Errorf("asterdoc module spec: %w", err)
		}
		if err := validateSpecModule(module, file.SDKVersion); err != nil {
			return fmt.Errorf("asterdoc module spec: %w", err)
		}
		key := specKey(module.Kind, module.Schema)
		if _, exists := specByKey[key]; exists {
			return fmt.Errorf("asterdoc module spec repeats %s", key)
		}
		specByKey[key] = module
		specDescriptors = append(specDescriptors, module.Descriptor)
	}
	sort.Slice(specDescriptors, func(i, j int) bool {
		if specDescriptors[i].Kind != specDescriptors[j].Kind {
			return specDescriptors[i].Kind < specDescriptors[j].Kind
		}
		return specDescriptors[i].Schema < specDescriptors[j].Schema
	})
	return nil
}

func specKey(kind string, schema int) string { return fmt.Sprintf("%s@%d", kind, schema) }

var foundationPortTypes = map[string]bool{
	"string": true, "enum": true, "bool": true, "boolean": true,
	"int": true, "integer": true, "number": true, "decimal": true,
	"quantity": true, "reference": true, "list": true, "object": true,
	"table": true, "geometry": true, "artifact": true, "duration": true,
	"timepoint": true, "relative_time": true,
	// Legacy aliases remain valid for existing documents.
	"well-map": true, "string[]": true, "object-list": true,
}

var foundationCategories = map[string]bool{
	"workflow": true, "resource": true, "labware": true,
	"measurement": true, "document": true, "device": true,
}

// knownModuleRules lists every rule specValidatorFor implements. loadSpec
// rejects a manifest that declares anything else, so a manifest-only addition
// fails at startup instead of on a user's document (module-catalog-v1.md §4.1).
var knownModuleRules = map[string]bool{
	"plate-geometry-wells": true, "reference-ref": true,
	"group-fields": true, "assay-fields": true,
	"step-execution": true, "step-blocks": true, "workflow-fields": true,
	"instrument-profile-fields": true, "material-fields": true, "sample-fields": true,
	"prose-fields": true, "group_table-fields": true, "param_table-fields": true,
	"figure-fields": true, "data_table-fields": true, "checklist-fields": true,
	"calculator-fields": true, "timer-fields": true, "attachment-fields": true,
	"subflow-fields": true,
}

func validateSpecModule(module specModule, sdkVersion string) error {
	if strings.TrimSpace(module.Kind) == "" || module.Schema <= 0 {
		return fmt.Errorf("module needs kind and schema")
	}
	if module.SDK != "" && module.SDK != sdkVersion {
		return fmt.Errorf("%s@%d declares sdk %q, want %q", module.Kind, module.Schema, module.SDK, sdkVersion)
	}
	for _, rule := range module.Rules {
		if !knownModuleRules[rule] {
			return fmt.Errorf("%s@%d declares unregistered rule %q", module.Kind, module.Schema, rule)
		}
	}
	if module.Category != "" && !foundationCategories[module.Category] {
		return fmt.Errorf("%s@%d has unknown category %q", module.Kind, module.Schema, module.Category)
	}
	permissions := map[string]bool{}
	for _, permission := range module.Permissions {
		permissions[permission] = true
	}
	for _, action := range module.Actions {
		if action.Permission == "" || !permissions[action.Permission] {
			return fmt.Errorf("%s@%d action %q names undeclared permission %q",
				module.Kind, module.Schema, action.ID, action.Permission)
		}
	}
	for _, port := range append(append([]Port{}, module.Inputs...), module.Outputs...) {
		if !foundationPortTypes[port.Type] {
			return fmt.Errorf("%s@%d port %q has unknown type %q", module.Kind, module.Schema, port.Name, port.Type)
		}
		if port.ItemType != "" && !foundationPortTypes[port.ItemType] {
			return fmt.Errorf("%s@%d port %q has unknown item_type %q", module.Kind, module.Schema, port.Name, port.ItemType)
		}
		if port.Type == "reference" || port.ItemType == "reference" {
			if strings.TrimSpace(port.Target) == "" {
				return fmt.Errorf("%s@%d port %q is a reference without target", module.Kind, module.Schema, port.Name)
			}
		}
		if port.Type == "quantity" && port.Dimension != "" {
			found := false
			for _, unit := range unitBySymbol {
				if unit.Dimension == port.Dimension {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s@%d port %q has unknown dimension %q", module.Kind, module.Schema, port.Name, port.Dimension)
			}
		}
		if port.Origin == "derived" && port.Calculator == "" {
			return fmt.Errorf("%s@%d output %q is derived without calculator", module.Kind, module.Schema, port.Name)
		}
	}
	if module.Lifecycle != nil {
		for _, state := range module.Lifecycle.States {
			if state == "" {
				return fmt.Errorf("%s@%d has an empty lifecycle state", module.Kind, module.Schema)
			}
		}
	}
	if module.Provenance != nil && module.Provenance.Origin == "recorded" {
		return fmt.Errorf("%s@%d cannot store recorded facts in the document", module.Kind, module.Schema)
	}
	return nil
}

func builtinDescriptor(kind string) Descriptor {
	return descriptorFor(kind, 1)
}

func descriptorFor(kind string, schema int) Descriptor {
	module, ok := specByKey[specKey(kind, schema)]
	if !ok {
		if specErr != nil {
			panic("asterdoc: load module spec: " + specErr.Error())
		}
		panic(fmt.Sprintf("asterdoc: missing module spec %s@%d", kind, schema))
	}
	return module.Descriptor
}

// specValidator implements the declarative field and rule checks shared with
// Rust Core. Renderers still keep their typed parse, but the module contract is
// defined once in spec/modules.json.
func specValidator(kind string) func(map[string]any) error {
	return specValidatorFor(kind, 1)
}

func specValidatorFor(kind string, schema int) func(map[string]any) error {
	module, ok := specByKey[specKey(kind, schema)]
	if !ok {
		if specErr != nil {
			panic("asterdoc: load module spec: " + specErr.Error())
		}
		panic("asterdoc: missing module spec " + kind)
	}
	return func(data map[string]any) error {
		allowed := make(map[string]bool, len(module.Inputs))
		allowedNames := make([]string, 0, len(module.Inputs))
		for _, input := range module.Inputs {
			allowed[input.Name] = true
			allowedNames = append(allowedNames, input.Name)
		}
		var unknown []string
		for k := range data {
			if !allowed[k] {
				unknown = append(unknown, k)
			}
		}
		if len(unknown) > 0 {
			sort.Strings(unknown)
			return fmt.Errorf("unknown field %q (allowed: %s)", unknown[0], strings.Join(allowedNames, ", "))
		}
		for _, input := range module.Inputs {
			value, present := data[input.Name]
			if !present || value == nil {
				if input.Required {
					return fmt.Errorf("missing required field %q", input.Name)
				}
				continue
			}
			if err := validatePortValue(input, value); err != nil {
				return err
			}
		}
		for _, rule := range module.Rules {
			switch rule {
			case "plate-geometry-wells":
				if _, err := plateFromData(data); err != nil {
					return err
				}
			case "reference-ref":
				ref, _ := data["ref"].(string)
				if strings.TrimSpace(ref) == "" {
					return fmt.Errorf("reference needs a ref")
				}
			case "group-fields":
				if name, _ := data["name"].(string); strings.TrimSpace(name) == "" {
					return fmt.Errorf("group needs a name")
				}
			case "assay-fields":
				if refs, ok := data["group_refs"].([]any); ok && len(refs) == 0 {
					return fmt.Errorf("assay needs at least one group_ref")
				}
			case "step-execution":
				exec := textValue(data, "execution")
				if exec == "" {
					exec = "executable"
				}
				switch exec {
				case "executable":
				case "organizational":
					for _, field := range []string{"assay", "timepoints", "path", "checks", "tasks", "uses", "blocks", "block_refs"} {
						value, ok := data[field]
						if !ok || value == nil {
							continue
						}
						switch value := value.(type) {
						case string:
							if strings.TrimSpace(value) != "" {
								return fmt.Errorf("organizational step cannot declare %s", field)
							}
						case []any:
							if len(value) > 0 {
								return fmt.Errorf("organizational step cannot declare %s", field)
							}
						default:
							return fmt.Errorf("organizational step cannot declare %s", field)
						}
					}
				default:
					return fmt.Errorf("step execution must be executable or organizational")
				}
			case "step-blocks":
				if rawBlocks, present := data["blocks"]; present && rawBlocks != nil {
					items, ok := rawBlocks.([]any)
					if !ok {
						return fmt.Errorf("step.blocks must be a list")
					}
					for _, rawItem := range items {
						itemMap, ok := rawItem.(map[string]any)
						if !ok {
							return fmt.Errorf("step.blocks item must be a mapping")
						}
						ik := textValue(itemMap, "kind")
						switch ik {
						case "prose", "group_table", "param_table", "figure", "data_table",
							"checklist", "calculator", "timer", "attachment", "subflow", "plate":
						default:
							return fmt.Errorf("step.blocks cannot embed module kind %q", ik)
						}
						isch, hasSchema := numericValue(itemMap["schema"])
						if !hasSchema || isch <= 0 {
							return fmt.Errorf("step.blocks item %q needs positive schema", ik)
						}
						if _, specExists := specByKey[specKey(ik, int(isch))]; !specExists {
							return fmt.Errorf("step.blocks embeds unsupported module %s@%d", ik, int(isch))
						}
						if err := specValidatorFor(ik, int(isch))(itemMap); err != nil {
							return err
						}
					}
				}
			case "workflow-fields":
				steps, _ := data["steps"].([]any)
				if len(steps) == 0 {
					return fmt.Errorf("workflow needs at least one step")
				}
			case "instrument-profile-fields":
				if strings.TrimSpace(textValue(data, "class")) == "" {
					return fmt.Errorf("instrument profile needs a class")
				}
				capabilities, _ := data["capabilities"].([]any)
				if len(capabilities) == 0 {
					return fmt.Errorf("instrument profile needs at least one capability")
				}
				for _, raw := range capabilities {
					item, _ := raw.(map[string]any)
					if name, _ := item["name"].(string); strings.TrimSpace(name) == "" {
						return fmt.Errorf("instrument capability needs a name")
					}
				}
			case "material-fields":
				if name, _ := data["name"].(string); strings.TrimSpace(name) == "" {
					return fmt.Errorf("material needs a name")
				}
				if role, _ := data["role"].(string); strings.TrimSpace(role) == "" {
					return fmt.Errorf("material needs a role")
				}
				if src, _ := data["source"].(string); strings.TrimSpace(src) == "" {
					return fmt.Errorf("material needs a source")
				}
			case "sample-fields":
				if id, _ := data["sample_id"].(string); strings.TrimSpace(id) == "" {
					return fmt.Errorf("sample needs a sample_id")
				}
			case "prose-fields":
				if _, ok := data["body"].(string); !ok {
					return fmt.Errorf("prose needs body string")
				}
			case "group_table-fields":
				rows, ok := data["rows"].([]any)
				if !ok {
					return fmt.Errorf("group_table needs rows")
				}
				seenIDs := map[string]bool{}
				for _, raw := range rows {
					row, _ := raw.(map[string]any)
					rid := textValue(row, "id")
					rname := textValue(row, "name")
					if rid == "" || rname == "" {
						return fmt.Errorf("group_table row needs non-empty id and name")
					}
					if seenIDs[rid] {
						return fmt.Errorf("group_table repeats id %q", rid)
					}
					seenIDs[rid] = true
				}
			case "param_table-fields":
				rows, ok := data["rows"].([]any)
				if !ok {
					return fmt.Errorf("param_table needs rows")
				}
				for _, raw := range rows {
					row, _ := raw.(map[string]any)
					key := textValue(row, "key")
					label := textValue(row, "label")
					if key == "" && label == "" {
						return fmt.Errorf("param_table row needs non-empty key or label")
					}
					if row["value"] == nil {
						return fmt.Errorf("param_table row needs value")
					}
					if u := textValue(row, "unit"); u != "" {
						if _, ok := unitFor(u); !ok {
							return fmt.Errorf("param_table unit %q is not in UCUM subset", u)
						}
					}
				}
			case "figure-fields":
				src := textValue(data, "src")
				svg := textValue(data, "svg")
				if src == "" && svg == "" {
					return fmt.Errorf("figure needs src or svg")
				}
				if svg != "" && !strings.Contains(svg, "<svg") {
					return fmt.Errorf("figure svg must contain <svg")
				}
				if strings.HasPrefix(strings.ToLower(src), "data:") && len(src) > 65536 {
					return fmt.Errorf("figure data URI exceeds 65536 bytes")
				}
				if h, hasH := numericValue(data["max_height"]); hasH && h < 40 {
					return fmt.Errorf("figure max_height must be >= 40")
				}
			case "data_table-fields":
				cols, ok := data["columns"].([]any)
				if !ok || len(cols) == 0 {
					return fmt.Errorf("data_table needs at least one column")
				}
				seenKeys := map[string]bool{}
				for _, raw := range cols {
					col, _ := raw.(map[string]any)
					ckey := textValue(col, "key")
					clabel := textValue(col, "label")
					vkind := textValue(col, "value_kind")
					if ckey == "" || clabel == "" {
						return fmt.Errorf("data_table column needs key and label")
					}
					if seenKeys[ckey] {
						return fmt.Errorf("data_table repeats column %q", ckey)
					}
					seenKeys[ckey] = true
					switch vkind {
					case "text", "number", "categorical", "formula":
					default:
						return fmt.Errorf("data_table column %q has invalid value_kind %q", ckey, vkind)
					}
					if u := textValue(col, "unit"); u != "" {
						if _, ok := unitFor(u); !ok {
							return fmt.Errorf("data_table column %q unit %q is not in UCUM subset", ckey, u)
						}
					}
					minVal, hasMin := numericValue(col["min"])
					maxVal, hasMax := numericValue(col["max"])
					if hasMin && hasMax && minVal > maxVal {
						return fmt.Errorf("data_table column %q min exceeds max", ckey)
					}
					if vkind == "formula" {
						if err := ValidateFormulaExpr(textValue(col, "formula")); err != nil {
							return fmt.Errorf("data_table column %q %w", ckey, err)
						}
					}
				}
			case "checklist-fields":
				items, ok := data["items"].([]any)
				if !ok || len(items) == 0 {
					return fmt.Errorf("checklist needs at least one item")
				}
				seenIDs := map[string]bool{}
				for _, raw := range items {
					item, _ := raw.(map[string]any)
					cid := textValue(item, "id")
					label := textValue(item, "label")
					if cid == "" || label == "" {
						return fmt.Errorf("checklist item needs non-empty id and label")
					}
					if seenIDs[cid] {
						return fmt.Errorf("checklist repeats item id %q", cid)
					}
					seenIDs[cid] = true
				}
			case "calculator-fields":
				switch textValue(data, "mode") {
				case "dilution":
					res := EvalCalculator(nil, data)
					if res.Err != nil {
						return res.Err
					}
				case "group_mastermix":
					vpuVal, vpuUnit, err := quantityParts(data["volume_per_unit"])
					if err != nil {
						return fmt.Errorf("calculator group_mastermix volume_per_unit: %w", err)
					}
					spec, ok := unitFor(vpuUnit)
					if !ok || spec.Dimension != "volume" || vpuVal <= 0 {
						return fmt.Errorf("calculator group_mastermix volume_per_unit must be positive volume")
					}
					if ex, hasEx := numericValue(data["excess_ratio"]); hasEx && (!isFiniteNum(ex) || ex < 0) {
						return fmt.Errorf("calculator excess_ratio must be non-negative")
					}
					comps, _ := data["components"].([]any)
					if len(comps) == 0 && textValue(data, "plate_ref") == "" && textValue(data, "container_ref") == "" {
						return fmt.Errorf("calculator group_mastermix needs components or plate_ref")
					}
					for _, raw := range comps {
						cm, _ := raw.(map[string]any)
						if textValue(cm, "name") == "" {
							return fmt.Errorf("calculator component needs name")
						}
					}
				case "serial_dilution":
					df, ok1 := numericValue(data["dilution_factor"])
					steps, ok2 := numericValue(data["steps_count"])
					if !ok1 || !isFiniteNum(df) || df <= 1 {
						return fmt.Errorf("calculator serial_dilution dilution_factor must be > 1")
					}
					if !ok2 || steps < 1 {
						return fmt.Errorf("calculator serial_dilution steps_count must be >= 1")
					}
				case "sample_normalize":
					sc, _ := numericValue(data["sample_count"])
					vpuVal, vpuUnit, verr := quantityParts(data["volume_per_unit"])
					spec, uok := unitFor(vpuUnit)
					if sc < 1 && (verr != nil || !uok || spec.Dimension != "volume" || vpuVal <= 0) {
						return fmt.Errorf("calculator sample_normalize needs positive sample_count or volume_per_unit")
					}
				case "formula":
					if rows, hasRows := data["rows"].([]any); hasRows && len(rows) > 0 {
						for _, raw := range rows {
							rm, _ := raw.(map[string]any)
							if textValue(rm, "label") == "" && textValue(rm, "key") == "" {
								return fmt.Errorf("calculator row needs key or label")
							}
							if textValue(rm, "kind") == "formula" {
								if err := ValidateFormulaExpr(textValue(rm, "formula")); err != nil {
									return err
								}
							}
						}
					} else if err := ValidateFormulaExpr(textValue(data, "formula")); err != nil {
						return err
					}
				default:
					return fmt.Errorf("unsupported calculator mode %q", textValue(data, "mode"))
				}
			case "timer-fields":
				switch textValue(data, "mode") {
				case "countdown":
					dv, du, err := quantityParts(data["duration"])
					if err != nil {
						return fmt.Errorf("countdown timer needs duration: %w", err)
					}
					spec, ok := unitFor(du)
					if !ok || spec.Dimension != "time" || dv <= 0 {
						return fmt.Errorf("countdown timer duration must be positive time")
					}
				case "stopwatch":
					if dur, ok := data["duration"]; ok && dur != nil {
						if err := validateQuantity(dur, "time"); err != nil {
							return err
						}
					}
				case "cycle":
					phases, ok := data["phases"].([]any)
					if !ok || len(phases) == 0 {
						return fmt.Errorf("cycle timer needs at least one phase")
					}
					if rounds, hasR := numericValue(data["rounds"]); hasR && rounds < 1 {
						return fmt.Errorf("cycle timer rounds must be >= 1")
					}
					for _, raw := range phases {
						ph, _ := raw.(map[string]any)
						pname := textValue(ph, "name")
						if pname == "" {
							return fmt.Errorf("cycle timer phase needs name")
						}
						if textValue(ph, "mode") != "open" || ph["duration"] != nil {
							dv, du, err := quantityParts(ph["duration"])
							if err != nil {
								return fmt.Errorf("cycle timer phase %q needs duration: %w", pname, err)
							}
							spec, uok := unitFor(du)
							if !uok || spec.Dimension != "time" || dv <= 0 {
								return fmt.Errorf("cycle timer phase %q duration must be positive time", pname)
							}
						}
					}
				case "window":
					if textValue(data, "anchor") == "" && textValue(data, "schedule_window") == "" {
						return fmt.Errorf("window timer needs anchor or schedule_window")
					}
					if off, ok := data["offset"]; ok && off != nil {
						ov, _, err := quantityParts(off)
						if err != nil || validateQuantity(off, "time") != nil || ov < 0 {
							return fmt.Errorf("window timer offset must be non-negative time")
						}
					}
				default:
					return fmt.Errorf("unsupported timer mode %q", textValue(data, "mode"))
				}
			case "attachment-fields":
				if slots, hasS := numericValue(data["slots_per_cell"]); hasS && slots < 1 {
					return fmt.Errorf("attachment slots_per_cell must be >= 1")
				}
				if textValue(data, "scope") == "container_grid" && textValue(data, "container_ref") == "" {
					return fmt.Errorf("container_grid attachment needs container_ref")
				}
			case "subflow-fields":
				if textValue(data, "title") == "" {
					return fmt.Errorf("subflow needs title")
				}
				items, ok := data["items"].([]any)
				if !ok {
					return fmt.Errorf("subflow needs items")
				}
				for _, raw := range items {
					it, _ := raw.(map[string]any)
					if textValue(it, "id") == "" || textValue(it, "title") == "" {
						return fmt.Errorf("subflow item needs non-empty id and title")
					}
				}
			default:
				return fmt.Errorf("unknown module rule %q", rule)
			}
		}
		return nil
	}
}

func validatePortValue(input Port, value any) error {
	switch input.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("field %q must be a string", input.Name)
		}
	case "enum":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("field %q must be an enum string", input.Name)
		}
		if len(input.Values) > 0 {
			allowed := false
			for _, candidate := range input.Values {
				if text == candidate {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("field %q must be one of %s", input.Name, strings.Join(input.Values, ", "))
			}
		}
	case "int", "integer":
		if !isInt(value) {
			return fmt.Errorf("field %q must be an integer", input.Name)
		}
	case "number", "decimal":
		if !isNumber(value) {
			return fmt.Errorf("field %q must be a number", input.Name)
		}
	case "bool", "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("field %q must be a boolean", input.Name)
		}
	case "well-map", "object", "geometry":
		if _, ok := value.(map[string]any); !ok {
			return fmt.Errorf("field %q must be a mapping", input.Name)
		}
	case "string[]":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("field %q must be a list of strings", input.Name)
		}
		for _, item := range items {
			if _, ok := item.(string); !ok {
				return fmt.Errorf("field %q must be a list of strings", input.Name)
			}
		}
	case "object-list", "table":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("field %q must be a list of objects", input.Name)
		}
		for _, item := range items {
			if _, ok := item.(map[string]any); !ok {
				return fmt.Errorf("field %q must be a list of objects", input.Name)
			}
		}
	case "list":
		items, ok := value.([]any)
		if !ok {
			return fmt.Errorf("field %q must be a list", input.Name)
		}
		for _, item := range items {
			if err := validateListItem(input, item); err != nil {
				return err
			}
		}
	case "quantity":
		if err := validateQuantity(value, input.Dimension); err != nil {
			return fmt.Errorf("field %q: %w", input.Name, err)
		}
	case "duration":
		if err := validateQuantity(value, "time"); err != nil {
			return fmt.Errorf("field %q: %w", input.Name, err)
		}
	case "reference":
		text, ok := value.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("field %q must be a non-empty reference", input.Name)
		}
	case "artifact":
		if err := validateArtifact(value); err != nil {
			return fmt.Errorf("field %q: %w", input.Name, err)
		}
	case "timepoint":
		if _, ok := value.(string); !ok {
			if mapping, mappingOK := value.(map[string]any); mappingOK {
				if text, _ := mapping["label"].(string); strings.TrimSpace(text) == "" {
					return fmt.Errorf("field %q timepoint needs a label", input.Name)
				}
				return nil
			}
			return fmt.Errorf("field %q must be a timepoint string or label mapping", input.Name)
		}
	case "relative_time":
		mapping, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("field %q must be a relative-time mapping", input.Name)
		}
		if text, _ := mapping["anchor"].(string); strings.TrimSpace(text) == "" {
			return fmt.Errorf("field %q relative time needs an anchor", input.Name)
		}
		if err := validateQuantity(mapping["offset"], "time"); err != nil {
			return fmt.Errorf("field %q offset: %w", input.Name, err)
		}
	}
	return nil
}

func validateListItem(input Port, item any) error {
	switch input.ItemType {
	case "", "object":
		if _, ok := item.(map[string]any); !ok {
			return fmt.Errorf("field %q must be a list of objects", input.Name)
		}
	case "string":
		if _, ok := item.(string); !ok {
			return fmt.Errorf("field %q must be a list of strings", input.Name)
		}
	case "reference":
		text, ok := item.(string)
		if !ok || strings.TrimSpace(text) == "" {
			return fmt.Errorf("field %q must be a list of non-empty references", input.Name)
		}
	case "quantity":
		if err := validateQuantity(item, input.Dimension); err != nil {
			return fmt.Errorf("field %q item: %w", input.Name, err)
		}
	case "artifact":
		if err := validateArtifact(item); err != nil {
			return fmt.Errorf("field %q item: %w", input.Name, err)
		}
	}
	return nil
}

func validateArtifact(value any) error {
	mapping, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("artifact must be a mapping")
	}
	uri, _ := mapping["uri"].(string)
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return fmt.Errorf("artifact needs a uri")
	}
	if strings.HasPrefix(strings.ToLower(uri), "data:") {
		return fmt.Errorf("artifact must reference external bytes, not inline data")
	}
	if hash, ok := mapping["sha256"].(string); ok && hash != "" {
		if len(hash) != 64 {
			return fmt.Errorf("artifact sha256 must be 64 hex characters")
		}
		for _, r := range hash {
			if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
				return fmt.Errorf("artifact sha256 must be hex")
			}
		}
	}
	return nil
}

func isNumber(value any) bool {
	switch value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return true
	}
	return false
}

func isInt(value any) bool {
	switch n := value.(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return true
	case float64:
		return n == float64(int64(n))
	}
	return false
}
