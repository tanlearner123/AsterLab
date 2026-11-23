// Package theme is AsterLab's visual system: Obsidian-inspired pure editorial
// surfaces, neutral Zinc option, or classic Warm paper, paired with restrained
// violet accents and tuned MyGo Liquid Glass capsule materials.
package theme

import (
	"math"
	"sync"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	onevra "github.com/oneo-me/onevra/source/theme"
)

// MixColor linearly interpolates between two registered theme colors in sRGB space.
func MixColor(c0, c1 ui.Color, frac float32) ui.Color {
	if frac < 0 {
		frac = 0
	} else if frac > 1 {
		frac = 1
	}
	mix := func(a, b uint8) uint8 {
		return uint8(math.Round(float64(a)*(1-float64(frac)) + float64(b)*float64(frac)))
	}
	return ui.RGBA(mix(c0.R, c1.R), mix(c0.G, c1.G), mix(c0.B, c1.B), 1.0)
}

const (
	PresetObsidian = "obsidian" // Pure white editorial paper + whisper-cool #f5f6f8 borderless cards
	PresetZinc     = "zinc"     // Achromatic cool silver-zinc #f4f5f7 canvas + pure #ffffff cards
	PresetWarm     = "warm"     // Classic warm laboratory paper (#f7f6f3 / #171614)
)

var (
	themeCfgMu            sync.RWMutex
	activePreset          = PresetObsidian
	activeLiquidGlass     = true
	activeTextScale       float32
	activeSystemTextScale float32 = 1.0
	activeReduceMotion    bool
)

// SetActiveLook configures the active surface preset ("obsidian" | "zinc" | "warm")
// and whether MyGo Liquid Glass refractive surfaces are active.
func SetActiveLook(preset string, liquidGlass bool) {
	p := normalizePreset(preset)
	themeCfgMu.Lock()
	activePreset = p
	activeLiquidGlass = liquidGlass
	themeCfgMu.Unlock()
}

// SetAccessibilityPrefs configures user-selected in-app text scale (0 = follow OS)
// and reduced motion preference, immediately syncing motion tokens.
func SetAccessibilityPrefs(textScale float32, reduceMotion bool) {
	if textScale < 0 {
		textScale = 0
	}
	themeCfgMu.Lock()
	activeTextScale = textScale
	activeReduceMotion = reduceMotion
	themeCfgMu.Unlock()
	applyMotionPreference(reduceMotion)
}

// AccessibilityPrefs returns the currently active in-app text scale and reduce-motion flag.
func AccessibilityPrefs() (textScale float32, reduceMotion bool) {
	themeCfgMu.RLock()
	s, rm := activeTextScale, activeReduceMotion
	themeCfgMu.RUnlock()
	return s, rm
}

// SetSystemTextScale records the OS accessibility text scale reported by MyGo
// (e.g. Windows "Make text bigger" slider) so GetTokens/TokensFor stay in lockstep
// with Onevra components without double-scaling.
func SetSystemTextScale(scale float32) {
	if scale <= 0 {
		scale = 1.0
	}
	if scale < 0.80 {
		scale = 0.80
	} else if scale > 2.0 {
		scale = 2.0
	}
	themeCfgMu.Lock()
	activeSystemTextScale = scale
	themeCfgMu.Unlock()
}

// EffectiveScale returns the single unified UI scale factor applied consistently
// across typography, icons, control heights, and Onevra theme metrics.
func EffectiveScale() float32 {
	themeCfgMu.RLock()
	userScale := activeTextScale
	sysScale := activeSystemTextScale
	themeCfgMu.RUnlock()
	if userScale > 0 {
		if userScale < 0.85 {
			return 0.85
		}
		if userScale > 1.50 {
			return 1.50
		}
		return userScale
	}
	if sysScale > 0 {
		return sysScale
	}
	return 1.0
}

// ActiveLook returns the currently active theme preset and Liquid Glass toggle.
func ActiveLook() (preset string, liquidGlass bool) {
	themeCfgMu.RLock()
	p, g := activePreset, activeLiquidGlass
	themeCfgMu.RUnlock()
	return p, g
}

func normalizePreset(preset string) string {
	switch preset {
	case PresetZinc, PresetWarm, PresetObsidian:
		return preset
	case "aurora": // migrate legacy config cleanly to default Obsidian
		return PresetObsidian
	default:
		return PresetObsidian
	}
}

// Tokens defines the shared AsterLab design system.
type Tokens struct {
	Preset                                                                                       string
	LiquidGlass                                                                                  bool
	Dark                                                                                         bool
	Canvas, Surface, Elevated, SelectedRow                                                       ui.Color
	Border, BorderStrong                                                                         ui.Color
	Text, TextMuted                                                                              ui.Color
	Accent, AccentHover, AccentText                                                              ui.Color
	Success, Warning, Danger                                                                     ui.Color
	RailBackground, SidebarBackground                                                            ui.Color
	CardBackground, CardBorder                                                                   ui.Color
	SuccessBg, SuccessText, WarningBg, WarningText, DangerBg, DangerText, NeutralBg, NeutralText ui.Color
	AccentSoftBg, InfoBg, InfoText, CodeBg                                                       ui.Color
	Spacing4, Spacing8, Spacing12, Spacing16, Spacing24, Spacing32, Spacing48                    int
	TextPage, TextSection, TextBody, TextMeta, TextData                                          int
	IconSm, IconMd, IconLg                                                                       int
	ButtonHeight, InputHeight, RowHeight                                                         int
	ControlRadius, PanelRadius, DialogRadius                                                     int
}

func scaleToken(base int, scale float32, minVal int) int {
	if scale <= 0 || scale == 1 {
		return base
	}
	v := int(float32(base)*scale + 0.5)
	if v < minVal {
		v = minVal
	}
	return v
}

// TokensFor returns the full design token set for an explicit preset, dark flag,
// and Liquid Glass toggle. Typography, icons, and control heights scale together
// from EffectiveScale() so fonts and icons always stay proportional across monitors.
func TokensFor(preset string, dark, liquidGlass bool) Tokens {
	p := normalizePreset(preset)
	scale := EffectiveScale()
	t := Tokens{
		Preset:        p,
		LiquidGlass:   liquidGlass,
		Dark:          dark,
		Spacing4:      4,
		Spacing8:      8,
		Spacing12:     scaleToken(12, scale, 10),
		Spacing16:     scaleToken(16, scale, 14),
		Spacing24:     scaleToken(24, scale, 20),
		Spacing32:     scaleToken(32, scale, 28),
		Spacing48:     scaleToken(48, scale, 42),
		TextPage:      scaleToken(26, scale, 20),
		TextSection:   scaleToken(18, scale, 15),
		TextBody:      scaleToken(15, scale, 13),
		TextData:      scaleToken(14, scale, 12),
		TextMeta:      scaleToken(13, scale, 11),
		IconSm:        scaleToken(15, scale, 13),
		IconMd:        scaleToken(17, scale, 15),
		IconLg:        scaleToken(20, scale, 18),
		ButtonHeight:  scaleToken(32, scale, 28),
		InputHeight:   scaleToken(32, scale, 28),
		RowHeight:     scaleToken(34, scale, 30),
		ControlRadius: 6,
		PanelRadius:   12,
		DialogRadius:  14,
	}

	switch p {
	case PresetWarm:
		if dark {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#171614"), ui.Hex("#1e1d1a"), ui.Hex("#242320")
			t.SelectedRow = ui.Hex("#2c2a25")
			t.Border, t.BorderStrong = ui.Hex("#37352f"), ui.Hex("#4a473f")
			t.Text, t.TextMuted = ui.Hex("#f2efe9"), ui.Hex("#a9a49a")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#a996ff"), ui.Hex("#bcabff"), ui.Hex("#1b1533")
			t.Success, t.Warning, t.Danger = ui.Hex("#84c9a4"), ui.Hex("#d9b878"), ui.Hex("#e79a9c")
			t.RailBackground, t.SidebarBackground = t.Canvas, t.Canvas
		} else {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#f7f6f3"), ui.Hex("#ffffff"), ui.Hex("#faf9f7")
			t.SelectedRow = ui.Hex("#edeae3")
			t.Border, t.BorderStrong = ui.Hex("#e7e3db"), ui.Hex("#d6d0c5")
			t.Text, t.TextMuted = ui.Hex("#1c1b18"), ui.Hex("#5f5a52")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#5b45c9"), ui.Hex("#4b37ae"), ui.Hex("#ffffff")
			t.Success, t.Warning, t.Danger = ui.Hex("#2f7a4f"), ui.Hex("#7a6233"), ui.Hex("#a3373a")
			t.RailBackground, t.SidebarBackground = ui.Hex("#f1efea"), ui.Hex("#f1efea")
		}
	case PresetZinc:
		if dark {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#0f1115"), ui.Hex("#16181e"), ui.Hex("#1e2129")
			t.SelectedRow = ui.Hex("#262a34")
			t.Border, t.BorderStrong = ui.Hex("#262a35"), ui.Hex("#383e4d")
			t.Text, t.TextMuted = ui.Hex("#f3f4f6"), ui.Hex("#9ca3af")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#a397ff"), ui.Hex("#b8aeff"), ui.Hex("#131026")
			t.Success, t.Warning, t.Danger = ui.Hex("#74cfa3"), ui.Hex("#e2b96f"), ui.Hex("#ed939b")
			t.RailBackground, t.SidebarBackground = ui.Hex("#0c0d10"), ui.Hex("#12141a")
		} else {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#f4f5f7"), ui.Hex("#ffffff"), ui.Hex("#eceef2")
			t.SelectedRow = ui.Hex("#e5e7ec")
			t.Border, t.BorderStrong = ui.Hex("#e2e5ec"), ui.Hex("#cbd0da")
			t.Text, t.TextMuted = ui.Hex("#111827"), ui.Hex("#5c6370")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#635bff"), ui.Hex("#5046e5"), ui.Hex("#ffffff")
			t.Success, t.Warning, t.Danger = ui.Hex("#18794e"), ui.Hex("#8c6115"), ui.Hex("#c03641")
			t.RailBackground, t.SidebarBackground = ui.Hex("#eff1f4"), ui.Hex("#f1f2f5")
		}
	default: // PresetObsidian: pure crisp white paper (#ffffff) + whisper-cool #f5f6f8 borderless cards
		if dark {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#111215"), ui.Hex("#16181d"), ui.Hex("#1e2027")
			t.SelectedRow = ui.Hex("#272a33")
			t.Border, t.BorderStrong = ui.Hex("#272b36"), ui.Hex("#3b4152")
			t.Text, t.TextMuted = ui.Hex("#f5f5f7"), ui.Hex("#a6abb8")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#a596ff"), ui.Hex("#bab0ff"), ui.Hex("#120f24")
			t.Success, t.Warning, t.Danger = ui.Hex("#6fd3a4"), ui.Hex("#e6bc6a"), ui.Hex("#f08e96")
			t.RailBackground, t.SidebarBackground = ui.Hex("#0d0e11"), ui.Hex("#13151a")
		} else {
			t.Canvas, t.Surface, t.Elevated = ui.Hex("#ffffff"), ui.Hex("#ffffff"), ui.Hex("#f5f6f8")
			t.SelectedRow = ui.Hex("#ededf0")
			t.Border, t.BorderStrong = ui.Hex("#e7e9ee"), ui.Hex("#d2d6df")
			t.Text, t.TextMuted = ui.Hex("#18181b"), ui.Hex("#71717a")
			t.Accent, t.AccentHover, t.AccentText = ui.Hex("#6c5ce7"), ui.Hex("#5949d4"), ui.Hex("#ffffff")
			t.Success, t.Warning, t.Danger = ui.Hex("#168053"), ui.Hex("#9a6715"), ui.Hex("#c23643")
			t.RailBackground, t.SidebarBackground = ui.Hex("#f7f8fa"), ui.Hex("#f8f9fb")
		}
	}
	return aliases(t)
}

// LightTokens returns Tokens for the active preset in light mode.
func LightTokens() Tokens {
	p, g := ActiveLook()
	return TokensFor(p, false, g)
}

// DarkTokens returns Tokens for the active preset in dark mode.
func DarkTokens() Tokens {
	p, g := ActiveLook()
	return TokensFor(p, true, g)
}

func aliases(t Tokens) Tokens {
	t.CardBackground, t.CardBorder = t.Elevated, t.Border
	t.SuccessBg, t.SuccessText = t.Surface, t.Success
	t.WarningBg, t.WarningText = t.Surface, t.Warning
	t.DangerBg, t.DangerText = t.Surface, t.Danger
	t.NeutralBg, t.NeutralText = t.Surface, t.TextMuted
	t.AccentSoftBg = t.Accent.Alpha(.14)
	if t.Dark {
		t.AccentSoftBg = ui.Hex("#2a2645")
	} else if t.Preset == PresetObsidian {
		t.AccentSoftBg = ui.Hex("#eeebff")
	}
	t.InfoBg, t.InfoText = t.Elevated, t.Accent
	t.CodeBg = t.Elevated
	return t
}

// PaintCanvasBackground paints a clean, crisp neutral editorial canvas without distracting tints.
func PaintCanvasBackground(el ui.Element, tok Tokens) ui.Element {
	return el.Background(tok.Canvas)
}

// ApplyChromeGlass equips top bars with crisp frosted surface without glass-on-glass washout.
func ApplyChromeGlass(el ui.Element, tok Tokens, interactive bool) ui.Element {
	if tok.LiquidGlass {
		if tok.Dark {
			// In dark mode, Avoid glass.Regular lifting dark pixels to gray; use deep smoked Clear glass.
			return el.Material(glass.Glass{
				Style:       glass.Clear,
				Tint:        ui.RGBA(20, 22, 28, 0.86),
				Interactive: interactive,
			})
		}
		return el.Material(glass.Glass{
			Style:       glass.Regular,
			Tint:        tok.Surface.Alpha(0.76),
			Interactive: interactive,
		})
	}
	return el.Background(tok.Surface.Alpha(0.95))
}

// ApplyCapsuleGlass gives Obsidian iOS 26 floating pills optimal Light/Dark Liquid Glass contrast:
// Light uses luminous milky frost (76%); Dark uses deep smoked Clear-bezel glass + crisp specular rim
// so bright elements underneath never bleach or blotch icons.
func ApplyCapsuleGlass(el ui.Element, tok Tokens, interactive bool) ui.Element {
	if tok.Dark {
		el.Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.56)).
			Border(1, ui.RGBA(255, 255, 255, 0.14))
		if tok.LiquidGlass {
			return el.Material(glass.Glass{
				Style:       glass.Clear,
				Tint:        ui.RGBA(24, 26, 33, 0.84),
				Interactive: interactive,
			})
		}
		return el.Background(ui.Hex("#1e2028"))
	}
	el.Shadow(0, 8, 28, 0, ui.RGBA(0, 0, 0, 0.11)).
		Border(1, tok.Surface.Alpha(0.95))
	if tok.LiquidGlass {
		return el.Material(glass.Glass{
			Style:       glass.Regular,
			Tint:        tok.Surface.Alpha(0.76),
			Interactive: interactive,
		})
	}
	return el.Background(tok.Surface.Alpha(0.96))
}

// ApplyPanelGlass applies clean sheet/drawer surfaces (deep smoked glass in dark mode to preserve full text legibility).
func ApplyPanelGlass(el ui.Element, tok Tokens, clarity float32) ui.Element {
	if tok.LiquidGlass {
		if tok.Dark {
			return el.Material(glass.Glass{
				Style:   glass.Clear,
				Clarity: clarity,
				Tint:    ui.RGBA(20, 22, 28, 0.92),
			})
		}
		return el.Material(glass.Glass{
			Style:   glass.Regular,
			Clarity: clarity,
			Tint:    tok.Surface.Alpha(0.86),
		})
	}
	return el.Background(tok.Surface)
}

// ApplyAccentGlass gives primary CTA buttons a clean tinted finish.
func ApplyAccentGlass(el ui.Element, tok Tokens, interactive bool) ui.Element {
	if tok.LiquidGlass {
		return el.Material(glass.Glass{
			Style:       glass.Regular,
			Tint:        tok.Accent.Alpha(0.88),
			Interactive: interactive,
		})
	}
	return el.Background(tok.Accent)
}

// GetTokens returns the painter tokens matching the frame's appearance.
func GetTokens(t *ui.Theme) Tokens {
	if t != nil && t.Dark {
		return DarkTokens()
	}
	return LightTokens()
}

// LightPalette is the AsterLab light theme for the active look.
func LightPalette() *ui.Theme {
	p, g := ActiveLook()
	return PaletteFor(p, false, g)
}

// DarkPalette is the AsterLab dark theme for the active look.
func DarkPalette() *ui.Theme {
	p, g := ActiveLook()
	return PaletteFor(p, true, g)
}

// Palette returns the AsterLab palette for the given appearance.
func Palette(dark bool) *ui.Theme {
	if dark {
		return DarkPalette()
	}
	return LightPalette()
}

// PaletteFor constructs a *ui.Theme for the specified preset, dark mode, and glass toggle.
func PaletteFor(preset string, dark, liquidGlass bool) *ui.Theme {
	return applyTokens(TokensFor(preset, dark, liquidGlass))
}

// Window is the theme function Onevra's shells and Apply take: it follows the
// desktop's appearance, OS text scale, in-app zoom, and higher-contrast preference
// with a single unified scale factor (zero double-scaling).
func Window(c *ui.Context) *ui.Theme {
	p := c.Preferences()
	if p.TextScale > 0 {
		SetSystemTextScale(p.TextScale)
	} else {
		SetSystemTextScale(1.0)
	}
	t := Palette(c.Theme().Dark)
	if p.HighContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
		t.Scrollbar = t.Scrollbar.Alpha(1.6)
	}
	return t
}

// Apply installs the AsterLab palette for the current frame. Call it at the
// start of a root content builder that is not inside an Onevra shell.
func Apply(c *ui.Context) { c.SetTheme(Window(c)) }

// CategoricalPalette returns the 8-color accessible scientific categorical palette
// for distinguishing samples, replicates, orthogonal plate factors, and groups.
func CategoricalPalette(dark bool) []ui.Color {
	if dark {
		return []ui.Color{
			ui.Hex("#6b9cff"), // Cobalt
			ui.Hex("#72c997"), // Emerald
			ui.Hex("#e3b354"), // Amber
			ui.Hex("#b28dff"), // Violet
			ui.Hex("#f07c8b"), // Coral
			ui.Hex("#57c5c2"), // Teal
			ui.Hex("#8c9ae8"), // Indigo
			ui.Hex("#d6986a"), // Terracotta
		}
	}
	return []ui.Color{
		ui.Hex("#3b6fd8"), // Cobalt
		ui.Hex("#2f7a4f"), // Emerald
		ui.Hex("#c47f17"), // Amber
		ui.Hex("#7a49c4"), // Violet
		ui.Hex("#c94a5a"), // Coral
		ui.Hex("#1b8a88"), // Teal
		ui.Hex("#5d6bb8"), // Indigo
		ui.Hex("#9e6235"), // Terracotta
	}
}

// CategoricalColor maps an arbitrary label deterministically onto CategoricalPalette.
func CategoricalColor(key string, dark bool) ui.Color {
	palette := CategoricalPalette(dark)
	if len(palette) == 0 {
		return GetTokens(Palette(dark)).Accent
	}
	idx := 0
	for _, r := range key {
		idx = (idx*31 + int(r)) % len(palette)
	}
	if idx < 0 {
		idx = -idx
	}
	return palette[idx]
}

// WellRoleColor returns the semantic scientific color for well control roles.
func WellRoleColor(role string, dark bool) ui.Color {
	tok := GetTokens(Palette(dark))
	palette := CategoricalPalette(dark)
	switch role {
	case "sample":
		return palette[0]
	case "standard":
		return palette[2]
	case "positive_control":
		return tok.Success
	case "negative_control":
		return palette[5]
	case "vehicle":
		return palette[6]
	case "ntc":
		return tok.Danger
	case "blank":
		return tok.BorderStrong
	default:
		return tok.Accent
	}
}

// GradientRampColor returns a monotonic light-to-dark scientific sequential color
// along [0, 1] (used by plate concentration / numeric factor layers).
func GradientRampColor(tNorm float64, dark bool) ui.Color {
	if tNorm < 0 {
		tNorm = 0
	}
	if tNorm > 1 {
		tNorm = 1
	}
	// Low -> High concentration: pale misty violet/indigo to deep cobalt-violet.
	if dark {
		lo := ui.Hex("#312e59")
		hi := ui.Hex("#8b7cff")
		return lo.Mix(hi, float32(tNorm))
	}
	lo := ui.Hex("#ddd6fe")
	hi := ui.Hex("#4c35b5")
	return lo.Mix(hi, float32(tNorm))
}

// LightTheme and DarkTheme install a fixed appearance, for isolated studios.
func LightTheme() *ui.Theme { return LightPalette() }

func DarkTheme() *ui.Theme { return DarkPalette() }

func applyTokens(c Tokens) *ui.Theme {
	t := onevra.Palette(c.Dark)
	t.Background, t.Surface, t.SurfaceHover, t.SurfacePressed = c.Canvas, c.Surface, c.Elevated, c.SelectedRow
	t.Border, t.Text, t.TextMuted = c.Border, c.Text, c.TextMuted
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = c.Accent, c.AccentHover, c.AccentHover, c.AccentText
	t.Danger, t.Warning, t.Success = c.Danger, c.Warning, c.Success
	t.Selection = c.Accent.Alpha(.18)
	t.Focus = c.Accent.Alpha(.55)
	t.Scrollbar = c.Text.Alpha(.28)
	t.FocusRingWidth, t.FocusRingOffset = 2, 1
	t.Radius = float32(c.ControlRadius)
	if c.TextBody > 0 {
		t.FontSize = float32(c.TextBody)
		t.Spacing = max(3.5, min(6.0, 4.0*EffectiveScale()))
	} else {
		t.Spacing = 4
	}
	return t
}
