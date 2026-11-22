package asterdoc

import (
	"encoding/json"
	"testing"
)

func TestAnalyzeLabSentenceExtractsSchemaValidModules(t *testing.T) {
	cases := []struct {
		text     string
		wantKind string
		wantMode string
		wantMods int
	}{
		{"80°C 水浴 5 min 与冰上 10 min 交替 3 轮至澄清", "timer", "cycle", 1},
		{"37°C 培养箱孵育 30 min 使其充分聚合成胶", "timer", "countdown", 1},
		{"405 nm 蓝光照射直至水凝胶完全固化，记录实际固化秒数", "timer", "stopwatch", 1},
		{"4°C 完全培养基浸泡平衡至少 2 小时（可并行处理其他实验）", "timer", "window", 1},
		{"将 100 ug/mL VEGF 母液稀释至 50 ng/mL，共配制 5 mL 工作液", "calculator", "dilution", 1},
		{"配制 5% w/v GelMA 前驱液 2 mL，计算需称取的冻干粉质量", "calculator", "weighing", 1},
		{"配制 10 mM 药物溶液 5 mL（分子量 MW = 543.5 g/mol），需称量多少毫克", "calculator", "weighing", 1},
		{"24 孔板预混液（多配 15% 余量）：每孔 2x Mix 10 uL、引物 1.6 uL", "calculator", "mastermix", 1},
		{"实测 HUVEC 悬液密度 5e5 cells/mL，每孔接种 5e4 细胞（500 uL/孔），共 14 孔", "calculator", "seeding", 1},
		{"12 孔板，3% / 5% / 7% GelMA 各 3 个技术复孔，第 4 列加无菌 PBS 作空白对照", "plate_group", "", 2},
		{"对 A1-C3 共 9 个孔进行 10× 明场采图，每孔随机拍 6 个非重叠视野（共 54 张）", "attachment", "", 1},
	}
	for _, tc := range cases {
		sug := AnalyzeLabSentence(nil, tc.text, nil)
		if sug == nil {
			t.Fatalf("expected suggestion for %q, got nil", tc.text)
		}
		if sug.Kind != tc.wantKind || sug.Mode != tc.wantMode || len(sug.Modules) != tc.wantMods {
			t.Fatalf("for %q: got kind=%q mode=%q mods=%d, want %q/%q/%d",
				tc.text, sug.Kind, sug.Mode, len(sug.Modules), tc.wantKind, tc.wantMode, tc.wantMods)
		}
	}

	// Hard negatives must return nil
	negs := []string{
		"用无菌 PBS 轻柔清洗细胞 2 遍后吸弃上清",
		"用 1 mL 移液枪轻柔吹打混匀 3 次，注意避免产生气泡",
		"剩余 GelMA 冻干粉避光密封保存于 -20°C 冰箱",
		"评估 GelMA 浓度梯度对 HUVEC 体外成管的影响",
		"本批次细胞传代至 P5，汇合度约 85%，状态良好",
		"复核上次实验中 5% GelMA 组管腔长度偏高的原因",
	}
	for _, n := range negs {
		if sug := AnalyzeLabSentence(nil, n, nil); sug != nil {
			t.Fatalf("expected nil for hard negative %q, got %+v", n, sug)
		}
	}
}

func TestExpandCompactSlotSchemasRoundTrip(t *testing.T) {
	reg, err := LoadNLSchemaRegistry()
	if err != nil {
		t.Fatalf("LoadNLSchemaRegistry failed: %v", err)
	}
	allowedAtoms := map[string]bool{}
	for _, a := range reg.AtomicSlotTypes {
		allowedAtoms[a] = true
	}
	if len(allowedAtoms) != 5 {
		t.Fatalf("expected exactly 5 closed atomic slot primitives, got %v", reg.AtomicSlotTypes)
	}
	schemas := NLComponentSchemas()
	if len(schemas) != 10 {
		t.Fatalf("expected 10 compact component input schemas, got %d", len(schemas))
	}
	// Verify ALL 10 targets obey the 4 Ironclad Principles & round-trip cleanly into valid AsterML v2 modules
	for _, sc := range schemas {
		if sc.ID == "" || sc.WhenToSuggest == "" || sc.WhenNotToSuggest == "" {
			t.Fatalf("schema %+v missing required Trigger Descriptor fields", sc)
		}
		for _, f := range sc.Fields {
			if !allowedAtoms[f.Type] {
				t.Fatalf("schema %s field %s uses non-atomic type %q (allowed: %v)", sc.ID, f.Key, f.Type, reg.AtomicSlotTypes)
			}
		}
		var slots map[string]any
		if err := json.Unmarshal([]byte(sc.ExampleJSON), &slots); err != nil {
			t.Fatalf("schema %s example JSON invalid: %v", sc.ID, err)
		}
		sug, err := ExpandCompactSlot(nil, sc.Kind, sc.Mode, slots)
		if err != nil {
			t.Fatalf("ExpandCompactSlot(%s) failed: %v", sc.ID, err)
		}
		if len(sug.Modules) == 0 {
			t.Fatalf("ExpandCompactSlot(%s) produced 0 modules", sc.ID)
		}
	}
}

func TestMissingParameterFallback(t *testing.T) {
	// 1. Dilution missing stock C1 and final volume V2
	sug := AnalyzeLabSentence(nil, "将 VEGF 母液稀释至 50 ng/mL 工作液", nil)
	if sug == nil || sug.Kind != "calculator" || sug.Mode != "dilution" {
		t.Fatalf("expected dilution fallback suggestion, got %+v", sug)
	}
	if len(sug.MissingSlots) != 2 {
		t.Fatalf("expected 2 missing slots (C1, V2), got %v", sug.MissingSlots)
	}
	if !sug.Params[0].IsDefault || sug.Params[1].IsDefault || !sug.Params[2].IsDefault {
		t.Fatalf("expected C1 & V2 marked IsDefault=true and C2=false, got %+v", sug.Params)
	}

	// 2. Weighing missing target volume V
	sugW := AnalyzeLabSentence(nil, "打算配制 2% w/v GelMA 冻干粉溶液，算称粉量", nil)
	if sugW == nil || sugW.Kind != "calculator" || sugW.Mode != "weighing" {
		t.Fatalf("expected weighing fallback suggestion, got %+v", sugW)
	}
	if len(sugW.MissingSlots) == 0 {
		t.Fatalf("expected missing volume slot recorded for %q", sugW.SourceText)
	}

	// 3. Cycle timer missing phase durations
	sugC := AnalyzeLabSentence(nil, "水浴与冰上交替循环 4 轮加速溶解", nil)
	if sugC == nil || sugC.Kind != "timer" || sugC.Mode != "cycle" {
		t.Fatalf("expected cycle timer fallback suggestion, got %+v", sugC)
	}
	if len(sugC.MissingSlots) == 0 {
		t.Fatalf("expected missing phase durations recorded, got %v", sugC.MissingSlots)
	}
}
