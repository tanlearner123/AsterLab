package asterdoc

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/theme"
)

func TestShouldTriggerSlashAtSafety(t *testing.T) {
	cases := []struct {
		text   string
		cursor int
		wantOK bool
		wantQ  string
	}{
		{"/", 1, true, ""},
		{"/group", 6, true, "group"},
		{"  /plate", 8, true, "plate"},
		{"第一行\n/step", len("第一行\n/step"), true, "step"},
		{"稀释比 1/1000", len("稀释比 1/1000"), false, ""},
		{"路径 ./assets/fig.png", len("路径 ./assets/fig.png"), false, ""},
		{"https://example.com", len("https://example.com"), false, ""},
		{"/group extra", len("/group extra"), false, ""},
	}
	for _, tc := range cases {
		_, q, ok := ShouldTriggerSlashAt(tc.text, tc.cursor)
		if ok != tc.wantOK || q != tc.wantQ {
			t.Errorf("ShouldTriggerSlashAt(%q, %d) = (ok=%v, q=%q), want (ok=%v, q=%q)", tc.text, tc.cursor, ok, q, tc.wantOK, tc.wantQ)
		}
	}
}

func TestComputeLivePreviewDecorationsCursorRevealAndRelations(t *testing.T) {
	src := `---
title: Live Preview 测试
---

# 实验背景
这是一段 **加粗** 文本，附带任务：
- [ ] 配制培养基
关联图：![[assets/a1.png]]{@plate-1:A1 6h} 与断链 ![[assets/bad.png]]{@plate-1:H12}

` + "```asterlab" + `
kind: group_table
id: gt-1
schema: 1
rows:
  - {id: g1, name: 对照组, role: control}
` + "```" + `

` + "```asterlab" + `
kind: plate
id: plate-1
schema: 1
format: 12
groups:
  g1: [A1]
` + "```" + `
`
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	segs := SplitDocumentSegments(src, doc)
	proseSeg := segs[0]
	th := ui.LightTheme()
	tok := theme.GetTokens(th)

	// Cursor away (unfocused): heading style applied, `# ` & `**` hidden, checkbox interactive, 2 relation chips, 1 warning diagnostic for H12
	styles, repls, diags := ComputeLivePreviewDecorations(proseSeg.Text, 0, false, th, tok, doc, proseSeg.Start, true, nil, nil)
	if len(styles) == 0 {
		t.Fatalf("expected heading LineStyle")
	}
	if len(repls) < 5 {
		t.Fatalf("expected live preview replacements for heading/bold/task/relations, got %d", len(repls))
	}
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic for out-of-bounds well H12, got %+v", diags)
	}

	// Cursor focused on the `# 实验背景` line (line 0 or 1 depending on leading newline):
	// hiding replacements on that active line must be suppressed so raw syntax is revealed!
	activeLine := 1 // line 0 is leading `\n`, line 1 is `# 实验背景`
	_, replsFocused, _ := ComputeLivePreviewDecorations(proseSeg.Text, activeLine, true, th, tok, doc, proseSeg.Start, true, nil, nil)
	if len(replsFocused) >= len(repls) {
		t.Fatalf("expected fewer replacements when active heading line reveals raw syntax (unfocused=%d, focused=%d)", len(repls), len(replsFocused))
	}
}
