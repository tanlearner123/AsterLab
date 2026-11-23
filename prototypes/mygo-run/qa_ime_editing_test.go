package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/theme"
)

// TestQA2_IMEAndCJKLiveEditingRobustness verifies Layer 2 (CJK / IME-safe Live Editing):
// UTF-8 rune boundaries in Live Preview decorations, Slash trigger accuracy with CJK &
// scientific fractions (`1/1000`), live multi-byte typing, and Undo/Redo roundtrip.
func TestQA2_IMEAndCJKLiveEditingRobustness(t *testing.T) {
	// 1. Slash trigger precision: must NOT misfire inside fractions, units, or URLs;
	// MUST fire cleanly at line start (including after leading whitespace).
	slashCases := []struct {
		text   string
		cursor int
		wantOK bool
		wantQ  string
	}{
		{"/", 1, true, ""},
		{"  /plate", len("  /plate"), true, "plate"},
		{"第一行笔记\n/timer", len("第一行笔记\n/timer"), true, "timer"},
		{"稀释比例 1/1000 缓冲液", len("稀释比例 1/1000"), false, ""},
		{"浓度 50 ug/mL", len("浓度 50 ug/mL"), false, ""},
		{"参考 https://example.com/a/b", len("参考 https://example.com/a/b"), false, ""},
		{"中文句子末尾不触发/", len("中文句子末尾不触发/"), false, ""},
	}
	for _, tc := range slashCases {
		_, q, ok := asterdoc.ShouldTriggerSlashAt(tc.text, tc.cursor)
		if ok != tc.wantOK || (ok && q != tc.wantQ) {
			t.Fatalf("ShouldTriggerSlashAt(%q, %d) = (ok=%v, q=%q), want (ok=%v, q=%q)",
				tc.text, tc.cursor, ok, q, tc.wantOK, tc.wantQ)
		}
	}

	// 2. ComputeLivePreviewDecorations UTF-8 rune boundary audit across rich CJK + emoji + CRLF + relations.
	cjkSample := strings.Join([]string{
		"#@ 01 配制 **万古霉素** 与 *溶葡萄球菌素* 工作液",
		"##@ 1.1 梯度稀释（1/1000 ~ 1/100000）🧪",
		"- [ ] 确认 37°C 水浴与 `OD600 = 0.1` 菌液浓度",
		"- [x] 核对 ~~过期~~ 有效试剂与 [[Vancomycin档案|万古霉素]]",
		"1. 加入 100 μL 菌液至目标孔 {@pl-1:A1,A2}",
		"---",
		"正常中文段落，含全角标点：，。、；！？（）【】",
	}, "\n")
	doc, err := asterdoc.Parse("---\ntitle: CJK 装饰测试\n---\n\n" + cjkSample + "\n")
	if err != nil {
		t.Fatalf("asterdoc.Parse CJK sample: %v", err)
	}
	tok := theme.LightTokens()
	for _, activeLine := range []int{-1, 0, 1, 2, 3, 4, 5, 6} {
		_, repls, _ := asterdoc.ComputeLivePreviewDecorations(
			cjkSample, activeLine, activeLine >= 0, nil, tok, doc, 0, true, nil, nil,
		)
		for _, r := range repls {
			if r.Start < 0 || r.End > len(cjkSample) || r.Start > r.End {
				t.Fatalf("activeLine=%d invalid replacement bounds [%d,%d] for len=%d", activeLine, r.Start, r.End, len(cjkSample))
			}
			if !utf8.ValidString(cjkSample[r.Start:r.End]) {
				t.Fatalf("activeLine=%d replacement [%d,%d] sliced inside multi-byte UTF-8 rune: %q", activeLine, r.Start, r.End, cjkSample[r.Start:r.End])
			}
		}
	}

	// 3. End-to-end live CJK typing + Backspace + Undo/Redo inside a real blank `.asterml` document.
	vaultRoot := filepath.Join(t.TempDir(), "IMETestVault")
	if err := os.MkdirAll(filepath.Join(vaultRoot, "IME"), 0o755); err != nil {
		t.Fatal(err)
	}
	rel := "IME/中文输入测试.asterml"
	if err := os.WriteFile(filepath.Join(vaultRoot, filepath.FromSlash(rel)), []byte(releaseBlankTemplateDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	sh.OpenFileInPane(sh.ActivePaneID, rel, "中文输入测试", shell.ModeEdit, false)
	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1280, 800)
	assertBoundedFrame(t, st, "ime-open-blank")

	// Focus the live prose editor and commit multi-byte Chinese + scientific units + symbols.
	st.Click("科学笔记正文")
	assertBoundedFrame(t, st, "ime-focus-prose")
	st.Type("配制 100 μg/mL 万古霉素，稀释 1/1000")
	assertBoundedFrame(t, st, "ime-type-cjk")

	ld, err := a.docStore.Open(rel)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ld.Source, "配制 100 μg/mL 万古霉素，稀释 1/1000") {
		t.Fatalf("expected CJK typed text committed to LiveDocument, got: %q", ld.Source)
	}
	if !utf8.ValidString(ld.Source) {
		t.Fatalf("LiveDocument contains invalid UTF-8 after CJK typing")
	}

	// Exercise Backspace on multi-byte characters & verify valid UTF-8 on disk.
	st.Key(0, ui.KeyBackspace)
	st.Key(0, ui.KeyBackspace)
	assertBoundedFrame(t, st, "ime-backspace-cjk")
	if !utf8.ValidString(ld.Source) {
		t.Fatalf("LiveDocument contains invalid UTF-8 after Backspace")
	}

	// Undo & Redo roundtrip must stay valid UTF-8 and panic-free.
	a.undoActiveLiveDoc()
	assertBoundedFrame(t, st, "ime-undo")
	a.redoActiveLiveDoc()
	assertBoundedFrame(t, st, "ime-redo")
	if !utf8.ValidString(ld.Source) {
		t.Fatalf("LiveDocument invalid UTF-8 after Undo/Redo")
	}
}
