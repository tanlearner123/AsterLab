package theme

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDesignTokens(t *testing.T) {
	SetActiveLook(PresetObsidian, true)
	light, dark := LightTokens(), DarkTokens()
	if light.Canvas != ui.Hex("#ffffff") || light.Elevated != ui.Hex("#f5f6f8") {
		t.Fatalf("obsidian light surfaces unexpected: canvas=%v elevated=%v", light.Canvas, light.Elevated)
	}
	if light.Accent != ui.Hex("#6c5ce7") || light.Text != ui.Hex("#18181b") {
		t.Fatalf("obsidian light ink/accent unexpected: accent=%v text=%v", light.Accent, light.Text)
	}
	if dark.Canvas != ui.Hex("#111215") || dark.Accent != ui.Hex("#a596ff") || dark.Text != ui.Hex("#f5f5f7") {
		t.Fatalf("obsidian dark palette unexpected: %+v", dark)
	}
	if light.Accent == light.Success || light.Accent == light.Warning {
		t.Fatal("accent must stay distinct from semantic success/warning")
	}

	zinc := TokensFor(PresetZinc, false, true)
	if zinc.Canvas != ui.Hex("#f4f5f7") || zinc.Accent != ui.Hex("#635bff") {
		t.Fatalf("zinc preset mismatch: %+v", zinc)
	}

	warmL := TokensFor(PresetWarm, false, false)
	warmD := TokensFor(PresetWarm, true, false)
	if warmL.Canvas != ui.Hex("#f7f6f3") || warmD.Canvas != ui.Hex("#171614") {
		t.Fatalf("warm preset compatibility broken: light=%v dark=%v", warmL.Canvas, warmD.Canvas)
	}

	lt, dt := LightTheme(), DarkTheme()
	if lt.Dark || !dt.Dark {
		t.Fatal("theme selection mismatch")
	}
	if lt.Background != light.Canvas || lt.Accent != light.Accent || lt.Radius != float32(light.ControlRadius) || lt.Spacing != 4 {
		t.Fatalf("onevra palette not layered with asterlab tokens: %+v", lt)
	}
	if GetTokens(dt).Danger != dark.Danger {
		t.Fatal("painter tokens do not follow the dark palette")
	}

	if !(light.TextPage >= light.TextSection && light.TextSection > light.TextBody && light.TextBody == 15 && light.TextData == 14 && light.TextMeta >= 13) {
		t.Fatalf("typography hierarchy mismatch with Obsidian-grade desktop 15px base: %+v", light)
	}
	if light.IconSm < 14 || light.IconMd < 16 || light.IconLg < 19 {
		t.Fatalf("icon scale tokens too small for 27-inch desktop readability: %+v", light)
	}
	SetAccessibilityPrefs(1.2, false)
	defer SetAccessibilityPrefs(0, false)
	scaled := LightTokens()
	if scaled.TextBody <= light.TextBody || scaled.TextMeta <= light.TextMeta || scaled.IconMd <= light.IconMd {
		t.Fatalf("accessibility font+icon scale did not apply cleanly: base=%+v scaled=%+v", light, scaled)
	}
	// Verify OS TextScale is respected when user scale is 0 (follow OS) without double-scaling.
	SetAccessibilityPrefs(0, false)
	SetSystemTextScale(1.25)
	defer SetSystemTextScale(1.0)
	osScaled := LightTokens()
	osTheme := LightTheme()
	if osScaled.TextBody <= light.TextBody || osTheme.FontSize != float32(osScaled.TextBody) {
		t.Fatalf("OS TextScale must drive both Tokens and Onevra FontSize without double-scaling: tok=%d theme=%.1f", osScaled.TextBody, osTheme.FontSize)
	}
}
