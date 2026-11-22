package asterdoc

import (
	"strings"
	"testing"
)

func TestSplitDocumentSegmentsLosslessAndTopLevelOps(t *testing.T) {
	src := `---
title: 自由白纸混排
---

# 实验背景

这是一段自由文字，含 **加粗** 和列表：
- 第一条

![[assets/huvec-a1.png]]{@plate-1:A1 6h}

` + "```asterlab" + `
kind: group_table
id: gt-1
schema: 1
rows:
  - {id: g1, name: 对照组, role: control}
` + "```" + `

中间继续写普通正文，不能被吞掉。

` + "```asterlab" + `
kind: plate
id: plate-1
schema: 1
format: 12
groups:
  g1: [A1]
` + "```" + `

尾注文字。
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	segs := SplitDocumentSegments(src, doc)

	// Reconstruct byte-for-byte
	_, fmEnd, _ := ScanFrontmatter(src)
	var rebuilt strings.Builder
	rebuilt.WriteString(src[:fmEnd])
	var embedCount, modCount, proseCount int
	for _, s := range segs {
		rebuilt.WriteString(s.Text)
		switch s.Kind {
		case SegmentEmbed:
			embedCount++
		case SegmentModule:
			modCount++
		case SegmentProse:
			proseCount++
		}
	}
	if rebuilt.String() != src {
		t.Fatalf("SplitDocumentSegments is not byte-for-byte lossless!\ngot:\n%q\nwant:\n%q", rebuilt.String(), src)
	}
	if embedCount != 1 || modCount != 2 || proseCount < 3 {
		t.Fatalf("unexpected segment counts: embed=%d mod=%d prose=%d", embedCount, modCount, proseCount)
	}

	// Insert top-level param_table at end and verify prose preserved
	next, createdID, err := InsertTopLevelModuleInSource(src, -1, map[string]any{
		"kind":  "param_table",
		"title": "培养条件",
		"rows":  []any{},
	})
	if err != nil {
		t.Fatalf("InsertTopLevelModuleInSource failed: %v", err)
	}
	if createdID == "" || !strings.Contains(next, "中间继续写普通正文，不能被吞掉。") || !strings.Contains(next, "尾注文字。") {
		t.Fatalf("prose damaged after insert: id=%q", createdID)
	}

	// Duplicate and remove top-level module
	dupSrc, dupID, err := DuplicateTopLevelModuleInSource(next, createdID)
	if err != nil {
		t.Fatalf("DuplicateTopLevelModuleInSource failed: %v", err)
	}
	if dupID == createdID {
		t.Fatalf("expected fresh ID on duplicate, got %q", dupID)
	}
	rmSrc, err := RemoveTopLevelModuleInSource(dupSrc, dupID)
	if err != nil {
		t.Fatalf("RemoveTopLevelModuleInSource failed: %v", err)
	}
	if !strings.Contains(rmSrc, "尾注文字。") {
		t.Fatalf("prose lost after remove")
	}
}
