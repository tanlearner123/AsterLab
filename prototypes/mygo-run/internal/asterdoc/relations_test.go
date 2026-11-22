package asterdoc

import (
	"strings"
	"testing"
)

func TestInlineRelationsGrammarAndMathSafety(t *testing.T) {
	src := `---
title: HUVEC 成管实验
---

实验背景：浓度公式 $C_1 V_1 = C_2 V_2$，同时记录两张图：
![[assets/huvec-a1-6h-1.png|400]]{@plate-12:A1 6h fov=1 role=obs} 与 ![[assets/huvec-a1-6h-2.png]]{@plate-12:A1,A2 t=6h fov=2 note="中心视野"}

普通示意图（无关联）：
![[assets/scheme.png]]

标准 Markdown 图片与原始附件：
![对照孔](./assets/ctrl.jpg){@plate-12:B1 24h}
[[assets/qpcr_raw.csv]]{@step-01 role=raw}

行内代码里不应解析：` + "`![[fake.png]]{@plate-12:A1}`" + `

` + "```asterlab" + `
kind: group_table
id: gt-1
schema: 1
rows:
  - {id: g-ctrl, name: 对照组, role: control}
  - {id: g-vegf, name: VEGF 处理组, role: treatment}
` + "```" + `

` + "```asterlab" + `
kind: plate
id: plate-12
schema: 1
title: 12 孔成管板
format: 12
groups:
  g-ctrl: [A1, A2]
  g-vegf: [B1]
` + "```" + `

` + "```asterlab" + `
kind: step
id: step-01
schema: 1
title: 显微成像
blocks: []
` + "```" + `
`

	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	rels := ResolveRelations(doc)
	if len(rels) != 4 {
		t.Fatalf("expected 4 annotated relations (ignoring plain scheme & inline code), got %d: %+v", len(rels), rels)
	}

	// 1st: obsidian embed with |400 width + @plate-12:A1 6h fov=1 role=obs
	r0 := rels[0]
	if r0.Asset != "assets/huvec-a1-6h-1.png" || r0.Width != "400" {
		t.Fatalf("r0 asset/width mismatch: %+v", r0)
	}
	if r0.Timepoint != "6h" || r0.FOV != 1 || r0.Role != "obs" {
		t.Fatalf("r0 metadata mismatch: %+v", r0)
	}
	if len(r0.Contexts) != 1 || !r0.Contexts[0].Valid || r0.Contexts[0].GroupName != "对照组" {
		t.Fatalf("r0 resolved context mismatch: %+v", r0.Contexts)
	}

	// 2nd: multi-well + quoted note
	r1 := rels[1]
	if len(r1.Contexts) != 2 || r1.Note != "中心视野" || r1.FOV != 2 {
		t.Fatalf("r1 multi-well/note mismatch: %+v", r1)
	}

	// 3rd: standard markdown image
	r2 := rels[2]
	if r2.SyntaxKind != "md_image" || r2.Timepoint != "24h" || len(r2.Contexts) != 1 || r2.Contexts[0].GroupName != "VEGF 处理组" {
		t.Fatalf("r2 md_image mismatch: %+v", r2)
	}

	// 4th: wiki link attachment -> step
	r3 := rels[3]
	if r3.SyntaxKind != "wiki_link" || r3.Role != "raw" || len(r3.Contexts) != 1 || r3.Contexts[0].ObjectKind != "step" {
		t.Fatalf("r3 wiki_link mismatch: %+v", r3)
	}

	// Reverse lookup: A1 should return 2 images (FOV1 and FOV2)
	a1Rels := RelationsForTarget(rels, "plate-12", "A1")
	if len(a1Rels) != 2 {
		t.Fatalf("expected 2 images linked to plate-12:A1, got %d", len(a1Rels))
	}

	// Round-trip canonical formatting
	formatted := FormatRelationAttr(r0.InlineRelation)
	if formatted != "{@plate-12:A1 6h fov=1 role=obs}" {
		t.Fatalf("unexpected canonical format: %q", formatted)
	}
}

func TestInlineRelationDiagnosticsAndPatch(t *testing.T) {
	src := `---
title: 诊断测试
---

![[assets/bad.png]]{@plate-6:H12 @missing-obj}

![[assets/plain.png]]

` + "```asterlab" + `
kind: group_table
id: gt-1
schema: 1
rows:
  - {id: g1, name: G1, role: control}
` + "```" + `

` + "```asterlab" + `
kind: plate
id: plate-6
schema: 1
format: 6
groups:
  g1: [A1]
` + "```" + `
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	rels := ResolveRelations(doc)
	if len(rels) != 1 {
		t.Fatalf("expected 1 annotated relation, got %d", len(rels))
	}
	if len(rels[0].Issues) != 2 {
		t.Fatalf("expected 2 diagnostics (out-of-bounds well + missing target), got %v", rels[0].Issues)
	}

	// Attach relation to previously plain image without touching surrounding text
	next, err := SetInlineRelationByAsset(src, "assets/plain.png", InlineRelation{
		Targets:   []RelationTarget{{ObjectID: "plate-6", SubTargets: []string{"A1"}}},
		Timepoint: "12h",
		FOV:       1,
	})
	if err != nil {
		t.Fatalf("SetInlineRelationByAsset failed: %v", err)
	}
	if !strings.Contains(next, "![[assets/plain.png]]{@plate-6:A1 12h fov=1}") {
		t.Fatalf("expected canonical attr injected, got:\n%s", next)
	}
	// Remove relation cleanly
	next2, err := SetInlineRelationByAsset(next, "assets/plain.png", InlineRelation{})
	if err != nil {
		t.Fatalf("remove relation failed: %v", err)
	}
	if next2 != src {
		t.Fatalf("expected lossless round-trip after removing relation")
	}
}
