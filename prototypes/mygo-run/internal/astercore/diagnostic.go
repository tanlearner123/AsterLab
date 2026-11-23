// Package astercore implements AsterLab's Pure Go unified scientific core
// (ADR-0002): graded scientific diagnostics, ARRIVE 2.0 experimental unit checks,
// deterministic assay projection/analysis (CCK-8, RT-qPCR), and Vault-native validation.
package astercore

import (
	"fmt"
	"strings"

	"asterlab-mygo-prototype/internal/asterdoc"
)

// Severity matches CONTEXT.md ValidationIssue levels.
type Severity string

const (
	SeverityBlocking          Severity = "blocking"
	SeverityNeedsConfirmation Severity = "needs_confirmation"
	SeverityAdvisory          Severity = "advisory"
)

// Diagnostic is a machine-readable scientific/structural finding with stable code & path.
type Diagnostic struct {
	Code       string   `json:"code"`
	Severity   Severity `json:"severity"`
	Path       string   `json:"path"`
	Message    string   `json:"message"`
	NextAction string   `json:"next_action,omitempty"`
}

// AssessmentReport summarizes readiness & graded scientific issues for an AsterML document.
type AssessmentReport struct {
	Ready       bool         `json:"ready"`
	Blocking    int          `json:"blocking"`
	Confirm     int          `json:"needs_confirmation"`
	Advisory    int          `json:"advisory"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

// AssessDocument evaluates structural validity, ARRIVE 2.0 experimental unit vs
// replicate separation, control presence, concentration bounds, and pantry refs.
func AssessDocument(doc *asterdoc.Document, knownPantryCodes map[string]bool) AssessmentReport {
	var rep AssessmentReport
	add := func(code string, sev Severity, path, msg, next string) {
		rep.Diagnostics = append(rep.Diagnostics, Diagnostic{
			Code:       code,
			Severity:   sev,
			Path:       path,
			Message:    msg,
			NextAction: next,
		})
		switch sev {
		case SeverityBlocking:
			rep.Blocking++
		case SeverityNeedsConfirmation:
			rep.Confirm++
		case SeverityAdvisory:
			rep.Advisory++
		}
	}

	if doc == nil {
		add("DOC_MISSING", SeverityBlocking, "document", "文档未能正常加载，请检查实验内容格式。", "可从右上角菜单打开高级源码视图排查。")
		return rep
	}

	fm, _, _ := asterdoc.ScanFrontmatter(doc.Source)
	if strings.TrimSpace(fm.Title) == "" {
		add("MISSING_TITLE", SeverityNeedsConfirmation, "frontmatter.title", "缺少实验标题。", "在文档顶部输入实验名称。")
	}

	var hasControlGroup, hasAnyGroup, hasPlate, hasStep bool
	var plateTotalAssigned int
	if st := asterdoc.ParseStepTree(doc.Source, doc); st != nil && len(st.Roots) > 0 {
		hasStep = true
	}
	checkGroupTable := func(bid string, bm map[string]any) {
		hasAnyGroup = true
		rows, _ := bm["rows"].([]any)
		for rIdx, rRaw := range rows {
			rm, _ := rRaw.(map[string]any)
			if rm == nil {
				continue
			}
			role, _ := rm["role"].(string)
			if role == "control" || role == "reference" {
				hasControlGroup = true
			}
			indN := numericFromAny(rm["independent_n"])
			if indN <= 0 {
				name, _ := rm["name"].(string)
				add("ARRIVE_MISSING_N", SeverityNeedsConfirmation,
					fmt.Sprintf("%s.rows[%d]", bid, rIdx),
					fmt.Sprintf("分组「%s」未填写独立样本数 n（技术复孔数不能替代独立 n）。", firstNonEmpty(name, "未命名组")),
					"在分组表中补充独立样本数 n。")
			}
		}
	}
	checkCalcStock := func(bid string, bm map[string]any) {
		var c1, c2 float64
		hasC1, hasC2 := false, false
		if stockMap, ok := bm["stock"].(map[string]any); ok {
			c1 = numericFromAny(stockMap["value"])
			hasC1 = c1 > 0
		}
		if targetMap, ok := bm["target"].(map[string]any); ok {
			c2 = numericFromAny(targetMap["value"])
			hasC2 = c2 > 0
		}
		if rows, ok := bm["rows"].([]any); ok {
			for _, rr := range rows {
				rm, _ := rr.(map[string]any)
				if rm == nil {
					continue
				}
				k, _ := rm["key"].(string)
				if k == "c1" || k == "C_1" || k == "stock_concentration" {
					c1 = numericFromAny(rm["value"])
					hasC1 = c1 > 0
				}
				if k == "c2" || k == "C_2" || k == "target_concentration" {
					c2 = numericFromAny(rm["value"])
					hasC2 = c2 > 0
				}
			}
		}
		if hasC1 && hasC2 && c2 > c1 {
			add("TARGET_EXCEEDS_STOCK", SeverityBlocking, bid,
				fmt.Sprintf("计算表目标浓度 (%g) 高于母液浓度 (%g)，物理上无法稀释配制。", c2, c1),
				"降低目标浓度或更换更高浓度母液。")
		}
	}

	for _, m := range doc.Modules {
		switch m.Kind {
		case "step":
			hasStep = true
			if blks, ok := m.Data["blocks"].([]any); ok {
				for _, raw := range blks {
					bm, _ := raw.(map[string]any)
					if bm == nil {
						continue
					}
					bk, _ := bm["kind"].(string)
					bid, _ := bm["id"].(string)
					switch bk {
					case "group_table":
						checkGroupTable(bid, bm)
					case "plate":
						hasPlate = true
						if grps, ok := bm["groups"].(map[string]any); ok {
							for _, v := range grps {
								if arr, ok := v.([]any); ok {
									plateTotalAssigned += len(arr)
								}
							}
						}
					case "calculator":
						checkCalcStock(bid, bm)
					}
				}
			}
		case "group_table":
			checkGroupTable(m.ID, m.Data)
		case "calculator":
			checkCalcStock(m.ID, m.Data)
		case "group":
			hasAnyGroup = true
			if role, _ := m.Data["role"].(string); role == "control" || role == "reference" {
				hasControlGroup = true
			}
			if numericFromAny(m.Data["independent_n"]) <= 0 {
				add("ARRIVE_MISSING_N", SeverityNeedsConfirmation, m.ID, "该实验分组尚未填写独立样本数 n。", "在分组中补充独立样本数 n。")
			}
		case "plate":
			hasPlate = true
			if m.Plate != nil {
				plateTotalAssigned += len(m.Plate.WellGroups())
			}
		case "material":
			if pc, _ := m.Data["pantry_code"].(string); strings.TrimSpace(pc) != "" && len(knownPantryCodes) > 0 {
				if !knownPantryCodes[strings.TrimSpace(pc)] {
					add("PANTRY_CODE_UNRESOLVED", SeverityNeedsConfirmation, m.ID,
						fmt.Sprintf("试剂编号 %q 在当前储藏室台账中未找到。", pc),
						"核对储藏室货架编号或补充入库。")
				}
			}
		}
	}

	if !hasStep {
		add("NO_STEPS", SeverityNeedsConfirmation, "steps", "实验尚未包含任何操作步骤。", "点击 + 添加至少一个实验步骤。")
	}
	if !hasAnyGroup {
		add("NO_GROUPS", SeverityNeedsConfirmation, "groups", "实验尚未声明分组与对照关系。", "插入「实验分组表」明确对照组与处理组。")
	} else if !hasControlGroup {
		add("NO_CONTROL_GROUP", SeverityNeedsConfirmation, "groups", "当前分组中缺少对照组。", "将空白或溶剂组角色设为「对照组」。")
	}
	if hasPlate && plateTotalAssigned == 0 {
		add("EMPTY_PLATE", SeverityAdvisory, "plate", "已插入孔板但尚未分配任何孔位。", "框选孔位并点选分组即可分配。")
	}
	if asterdoc.NormalizeDateYMD(fm.ScheduledDate) == "" {
		add("UNSCHEDULED_DATE", SeverityAdvisory, "frontmatter.date", "实验尚未排期（未填写日期）。", "在顶部属性栏填写实验日期。")
	}

	rep.Ready = rep.Blocking == 0 && rep.Confirm == 0
	return rep
}

func numericFromAny(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case float64:
		return n
	case float32:
		return float64(n)
	default:
		return 0
	}
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return strings.TrimSpace(a)
	}
	return strings.TrimSpace(b)
}
