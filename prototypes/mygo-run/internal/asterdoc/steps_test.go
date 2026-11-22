package asterdoc

import "testing"

func TestParseStepTreeOrthogonalRulesAndMixedFlow(t *testing.T) {
	src := `---
asterml: "2.1"
title: HUVEC 成管
---

## 1.1 实验目的
评估 GelMA 浓度影响。
- [ ] 这条不是步骤里的待办

### 1.3 实验步骤（哪怕外层是三级标题，步骤仍用 #@ / ##@）

#@ GelMA 配液与交联
##@ 热循环溶解
- 称量 GelMA 60 / 100 / 140 mg
- 加入 0.25% LAP 缓冲液

` + "```asterlab\nid: timer-cycle\nkind: timer\nschema: 1\ntitle: 热循环\nmode: stopwatch\nstatus: running\n```" + `

- 观察至完全澄清

##@ 405 nm 光交联 {#t0}
1. 每孔加入 200 uL 前体液
2. 蓝光固化 120 s

#@ 细胞接种与培养
- 4°C 浸泡清洗
- 接种 HUVEC

---
这是一段分割线后的普通总结文字，不应被吞进步骤 2。
- 不是步骤 2 的列表项

## 2. 实验讨论与复盘
- [ ] 明天补查一篇文献
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	tree := ParseStepTree(src, doc)
	if len(tree.Roots) != 2 {
		t.Fatalf("expected 2 root steps, got %d", len(tree.Roots))
	}
	if len(tree.Flat) != 4 {
		t.Fatalf("expected 4 total steps (2 roots + 2 substeps), got %d", len(tree.Flat))
	}

	s1 := tree.Roots[0]
	if s1.Title != "GelMA 配液与交联" || s1.Number != "1" || s1.Depth != 0 || len(s1.Children) != 2 {
		t.Fatalf("unexpected root step 1: %+v", s1)
	}
	if s1.RunningTimers != 1 {
		t.Fatalf("expected 1 rolled-up running timer on step 1, got %d", s1.RunningTimers)
	}

	// Substep 1.1 has 2 list items + 1 timer module + 1 list item after timer = 4 continuous flow items!
	s11 := s1.Children[0]
	if s11.Title != "热循环溶解" || s11.Number != "1.1" || s11.Depth != 1 || len(s11.OwnedModuleIDs) != 1 || s11.OwnedModuleIDs[0] != "timer-cycle" {
		t.Fatalf("unexpected substep 1.1: %+v", s11)
	}
	if len(s11.Items) != 4 || s11.Items[0].Kind != "action" || s11.Items[1].Kind != "action" || s11.Items[2].Kind != "module" || s11.Items[3].Kind != "action" {
		t.Fatalf("expected 4 ordered mixed-flow items (action, action, module, action), got %+v", s11.Items)
	}

	s12 := s1.Children[1]
	if s12.ID != "t0" || s12.Number != "1.2" || len(s12.Items) != 2 || !s12.Items[0].Ordered {
		t.Fatalf("unexpected substep 1.2: %+v", s12)
	}

	// Thematic break `---` MUST cleanly close step 2 so trailing list & discussion are excluded!
	s2 := tree.Roots[1]
	if s2.Title != "细胞接种与培养" || s2.Number != "2" || len(s2.Items) != 2 {
		t.Fatalf("expected step 2 to have exactly 2 items (post-`---` items excluded), got %+v", s2.Items)
	}

	// Verify unified Document Outline: plain chapters + nested steps in exact document order
	outline := ParseDocumentOutline(src, doc)
	if len(outline) != 7 {
		t.Fatalf("expected 7 unified outline items (3 headings + 4 steps), got %d: %+v", len(outline), outline)
	}
	// 0: 1.1 实验目的 (depth 0, heading)
	// 1: 1.3 实验步骤 (depth 1, heading under ## 1.1)
	// 2: STEP 1 GelMA (depth 2, step, StepDepth 0)
	// 3: 1.1 热循环溶解 (depth 3, step, StepDepth 1)
	// 4: 1.2 405 nm 光交联 (depth 3, step, StepDepth 1)
	// 5: STEP 2 细胞接种 (depth 2, step, StepDepth 0)
	// 6: 2. 实验讨论与复盘 (depth 0, heading)
	if outline[0].IsStep || outline[0].Depth != 0 {
		t.Fatalf("unexpected outline[0]: %+v", outline[0])
	}
	if !outline[2].IsStep || outline[2].Number != "1" || outline[2].StepDepth != 0 || outline[2].Depth != outline[1].Depth+1 {
		t.Fatalf("expected STEP 1 nested directly under 1.3 实验步骤, got %+v", outline[2])
	}
	if !outline[3].IsStep || outline[3].Number != "1.1" || outline[3].StepDepth != 1 || outline[3].Depth != outline[2].Depth+1 {
		t.Fatalf("expected 1.1 nested under STEP 1, got %+v", outline[3])
	}
	if outline[6].IsStep || outline[6].Depth != 0 || outline[6].Title != "2. 实验讨论与复盘" {
		t.Fatalf("expected 2. 实验讨论与复盘 back at depth 0, got %+v", outline[6])
	}
}

func TestParseStepTreeBackwardCompatDoubleHash(t *testing.T) {
	src := "##@ 步骤甲\n###@ 子步骤乙\n- [x] 已完成\n- [ ] 待完成\n## 讨论\n- [ ] 非步骤待办\n"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	tree := ParseStepTree(src, doc)
	if len(tree.Roots) != 1 || len(tree.Flat) != 2 {
		t.Fatalf("expected 1 root + 1 substep, got roots=%d flat=%d", len(tree.Roots), len(tree.Flat))
	}
	if tree.Roots[0].Number != "1" || tree.Roots[0].Children[0].Number != "1.1" {
		t.Fatalf("unexpected numbering: root=%s child=%s", tree.Roots[0].Number, tree.Roots[0].Children[0].Number)
	}
	if tree.Roots[0].ChecksDone != 1 || tree.Roots[0].ChecksTotal != 2 {
		t.Fatalf("expected 1/2 checks rolled up, got %d/%d", tree.Roots[0].ChecksDone, tree.Roots[0].ChecksTotal)
	}
}

func TestParseHeadingStepLineRejectsNonSteps(t *testing.T) {
	cases := []string{
		"## 普通标题",
		"#tag",
		"##@无空格不算",
		"email@example.com",
		"![[img.png]]{@plate-1:A1}",
		"##@   ",
		"---",
	}
	for _, tc := range cases {
		if hs, ok := ParseHeadingStepLine(tc); ok {
			t.Fatalf("expected %q NOT to parse as heading step, got %+v", tc, hs)
		}
	}
	hs, ok := ParseHeadingStepLine("#@ 配液与交联")
	if !ok || hs.Level != 1 || hs.Title != "配液与交联" {
		t.Fatalf("unexpected parsed main step: ok=%v %+v", ok, hs)
	}
	hs2, ok := ParseHeadingStepLine("##@ 光交联 {#t0 optional}")
	if !ok || hs2.Level != 2 || hs2.Title != "光交联" || hs2.Anchor != "t0" || !hs2.Optional {
		t.Fatalf("unexpected parsed substep: ok=%v %+v", ok, hs2)
	}
}
