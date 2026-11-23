package shell

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/theme"
)

func TestHotkeySpecParser(t *testing.T) {
	cases := []struct {
		spec string
		want []parsedHotkey
	}{
		{"Ctrl+K / Ctrl+O", []parsedHotkey{{ui.Cmd, ui.KeyK}, {ui.Cmd, ui.KeyO}}},
		{"Ctrl+Shift+R", []parsedHotkey{{ui.Cmd | ui.Shift, ui.KeyR}}},
		{"Alt+←", []parsedHotkey{{ui.Alt, ui.KeyLeft}}},
		{"Alt+→", []parsedHotkey{{ui.Alt, ui.KeyRight}}},
		{"Ctrl+\\", []parsedHotkey{{ui.Cmd, ui.KeyBackslash}}},
		{"Ctrl+,", []parsedHotkey{{ui.Cmd, ui.KeyComma}}},
		{"F2", []parsedHotkey{{0, ui.KeyF2}}},
		{"Del", []parsedHotkey{{0, ui.KeyDelete}}},
	}
	for _, tc := range cases {
		got := parseHotkeySpec(tc.spec)
		if len(got) != len(tc.want) {
			t.Fatalf("parseHotkeySpec(%q) len=%d, want %d (%+v)", tc.spec, len(got), len(tc.want), got)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("parseHotkeySpec(%q)[%d]=%+v, want %+v", tc.spec, i, got[i], tc.want[i])
			}
		}
	}
}

func TestInitialBoolPreferencesNotOverwritten(t *testing.T) {
	s := &Shell{}
	s.SetInitialThemeLook(theme.PresetZinc, false)
	s.SyncThemeLook(nil)
	if s.LiquidGlass {
		t.Fatalf("expected explicitly saved LiquidGlass=false to survive SyncThemeLook")
	}
	if s.ThemePreset != theme.PresetZinc {
		t.Fatalf("expected ThemePreset=%q, got %q", theme.PresetZinc, s.ThemePreset)
	}

	s.SetInitialMobilePrefs("calc", false, false)
	s.EnsureMobileDefaults()
	if s.MobileAutoHideDock {
		t.Fatalf("expected explicitly saved MobileAutoHideDock=false to survive EnsureMobileDefaults")
	}
	if s.MobileEditToolbar {
		t.Fatalf("expected explicitly saved MobileEditToolbar=false to survive EnsureMobileDefaults")
	}
	if s.MobilePullAction != "calc" {
		t.Fatalf("expected MobilePullAction=calc, got %q", s.MobilePullAction)
	}
}

func TestAccessibilityPrefsScaleAndReduceMotion(t *testing.T) {
	defer theme.SetAccessibilityPrefs(0, false)

	s := &Shell{TextScale: 1.25, ReduceMotion: true}
	s.SetInitialThemeLook(theme.PresetObsidian, true)
	tok := s.SyncThemeLook(nil)
	if tok.TextBody <= 15 {
		t.Fatalf("expected TextScale=1.25 to enlarge TextBody (>15), got %d", tok.TextBody)
	}
	if !theme.ReducedMotionEnabled() {
		t.Fatalf("expected ReducedMotionEnabled() to be true")
	}
	if theme.MotionSnappy.Spring.Duration != 0 {
		t.Fatalf("expected MotionSnappy spring to collapse under ReduceMotion, got %+v", theme.MotionSnappy)
	}

	s.TextScale = 0
	s.ReduceMotion = false
	tok2 := s.SyncThemeLook(nil)
	if tok2.TextBody != 15 {
		t.Fatalf("expected default TextBody=15 after reset, got %d", tok2.TextBody)
	}
	if theme.ReducedMotionEnabled() {
		t.Fatalf("expected ReducedMotionEnabled() to reset to false")
	}
	if theme.MotionSnappy.Spring.Duration == 0 {
		t.Fatalf("expected MotionSnappy spring restored after ReduceMotion=false")
	}
}
