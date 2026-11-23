package main

import (
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/modules/runs"
)

func decodePNGImage(t *testing.T, path string) image.Image {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open rendered PNG %s: %v", path, err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode PNG %s: %v", path, err)
	}
	return img
}

func assertNonBlankImage(t *testing.T, img image.Image, name string, wantW, wantH int) {
	t.Helper()
	b := img.Bounds()
	if b.Dx() < wantW || b.Dy() < wantH {
		t.Fatalf("VRT %s dimensions %dx%d smaller than expected %dx%d", name, b.Dx(), b.Dy(), wantW, wantH)
	}
	// Sample a 64x64 grid across the image to confirm visible luminance variance (not blank/all-white/all-black).
	stepX := max(1, b.Dx()/64)
	stepY := max(1, b.Dy()/64)
	var sum, sumSq float64
	var count float64
	for y := b.Min.Y; y < b.Max.Y; y += stepY {
		for x := b.Min.X; x < b.Max.X; x += stepX {
			r, g, bl, _ := img.At(x, y).RGBA()
			lum := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(bl)) / 65535.0
			sum += lum
			sumSq += lum * lum
			count++
		}
	}
	mean := sum / count
	variance := math.Max(0, sumSq/count-mean*mean)
	if variance < 0.0005 {
		t.Fatalf("VRT %s appears blank or flat-colored (luminance mean=%.4f var=%.6f)", name, mean, variance)
	}
}

func pixelDiffRatio(a, b image.Image) float64 {
	ba, bb := a.Bounds(), b.Bounds()
	if ba.Dx() != bb.Dx() || ba.Dy() != bb.Dy() {
		return 1.0
	}
	total := ba.Dx() * ba.Dy()
	if total == 0 {
		return 0
	}
	diff := 0
	for y := 0; y < ba.Dy(); y++ {
		for x := 0; x < ba.Dx(); x++ {
			ra, ga, bla, aa := a.At(ba.Min.X+x, ba.Min.Y+y).RGBA()
			rb, gb, blb, ab := b.At(bb.Min.X+x, bb.Min.Y+y).RGBA()
			dr := int(ra>>8) - int(rb>>8)
			dg := int(ga>>8) - int(gb>>8)
			db := int(bla>>8) - int(blb>>8)
			da := int(aa>>8) - int(ab>>8)
			if dr < 0 {
				dr = -dr
			}
			if dg < 0 {
				dg = -dg
			}
			if db < 0 {
				db = -db
			}
			if da < 0 {
				da = -da
			}
			if dr > 8 || dg > 8 || db > 8 || da > 8 {
				diff++
			}
		}
	}
	return float64(diff) / float64(total)
}

// TestQA4_VisualRegression_MultiThemeAndViewportMatrix verifies Layer 4 (Visual
// Regression Testing) across Light/Dark themes and 900x600 compact / 1440x900 desktop
// viewports on all 4 core AsterLab surfaces, with optional golden-PNG pixel-diff mode
// via ASTERLAB_VRT_DIR (+ ASTERLAB_UPDATE_VRT=1 to refresh baselines).
func TestQA4_VisualRegression_MultiThemeAndViewportMatrix(t *testing.T) {
	vaultRoot := filepath.Join(t.TempDir(), "VRTVault")
	if err := os.MkdirAll(filepath.Join(vaultRoot, "Proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "Alpha.asterml"), []byte(fsmTestDocAlpha), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vaultRoot, "Proj", "Beta.asterml"), []byte(fsmTestDocBeta), 0o644); err != nil {
		t.Fatal(err)
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	shelf := uint32(180)
	if _, err := a.localUpsertPantry(runs.PantryUpsert{
		Name: "Vancomycin Stock", Location: "-20°C", PurchasedOn: "2026-11-20", ShelfLifeDays: &shelf,
	}); err != nil {
		t.Fatal(err)
	}
	a.tools.ResetPantryState()
	a.syncPantrySnapshotToShell()

	outDir := t.TempDir()
	goldenDir := os.Getenv("ASTERLAB_VRT_DIR")
	updateGolden := os.Getenv("ASTERLAB_UPDATE_VRT") == "1"

	type matrixCase struct {
		name      string
		w, h      int
		dark      bool
		setup     func()
		wantTexts []string
	}
	cases := []matrixCase{
		{
			name: "calendar_1440x900_light",
			w:    1440, h: 900, dark: false,
			setup: func() {
				sh.SetRightDrawer(false, 0)
				sh.OpenCalendarTab()
			},
			wantTexts: []string{"实验日程", "实验库"},
		},
		{
			name: "doc_edit_drawer_900x600_dark",
			w:    900, h: 600, dark: true,
			setup: func() {
				sh.OpenFileInPane(sh.ActivePaneID, "Proj/Alpha.asterml", "Alpha", shell.ModeEdit, false)
				sh.SetRightDrawer(true, 0)
			},
			wantTexts: []string{"Van-Lys 液体共培养涂板计数计划", "共培养孵育计时"},
		},
		{
			name: "pantry_detail_1440x900_light",
			w:    1440, h: 900, dark: false,
			setup: func() {
				sh.SetRightDrawer(false, 0)
				sh.OpenPantryTab()
				if len(a.tools.Pantry.Items) > 0 {
					a.tools.OpenPantryDetail(a.tools.Pantry.Items[0])
				}
			},
			wantTexts: []string{"储藏室货架", "保存试剂档案", "说明书与产品网址"},
		},
		{
			name: "split_two_docs_1440x900_dark",
			w:    1440, h: 900, dark: true,
			setup: func() {
				a.tools.PantryDialogOpen = false
				sh.ResetWorkspaceForVault()
				sh.OpenFileInPane(sh.ActivePaneID, "Proj/Alpha.asterml", "Alpha", shell.ModeEdit, false)
				sh.SplitPaneWithTab(sh.ActivePaneID, shell.WorkspaceTab{
					Kind: shell.TabFile, Title: "Beta", FilePath: "Proj/Beta.asterml", Mode: shell.ModeEdit,
				}, "right")
			},
			wantTexts: []string{"Van-Lys 液体共培养涂板计数计划", "Van-Lys 滤纸片扩散抑菌圈计划"},
		},
		{
			name: "monitor_27inch_2560x1440_light",
			w:    2560, h: 1440, dark: false,
			setup: func() {
				sh.TextScale = 0
				sh.ResetWorkspaceForVault()
				sh.OpenFileInPane(sh.ActivePaneID, "Proj/Alpha.asterml", "Alpha", shell.ModeEdit, false)
				sh.SetRightDrawer(true, 0)
			},
			wantTexts: []string{"Van-Lys 液体共培养涂板计数计划", "共培养孵育计时", "实验库"},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			tc.setup()
			fname := tc.name + ".png"
			renderVisualPNGSteps(t, outDir, fname, tc.w, tc.h, tc.dark, func(c *ui.Context) {
				sh.RenderSplitView(c)
			}, func(st *ui.Tester) {
				st.Frame()
				for _, want := range tc.wantTexts {
					if !st.HasText(want) {
						t.Fatalf("VRT %s missing critical visible text %q", tc.name, want)
					}
				}
			})

			renderedPath := filepath.Join(outDir, fname)
			img := decodePNGImage(t, renderedPath)
			assertNonBlankImage(t, img, tc.name, tc.w, tc.h)

			if goldenDir != "" {
				goldenPath := filepath.Join(goldenDir, fname)
				if updateGolden {
					if err := os.MkdirAll(goldenDir, 0o755); err != nil {
						t.Fatal(err)
					}
					raw, err := os.ReadFile(renderedPath)
					if err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(goldenPath, raw, 0o644); err != nil {
						t.Fatal(err)
					}
					return
				}
				if _, err := os.Stat(goldenPath); err == nil {
					goldenImg := decodePNGImage(t, goldenPath)
					diff := pixelDiffRatio(img, goldenImg)
					if diff > 0.015 {
						t.Fatalf("VRT %s pixel diff %.2f%% exceeded 1.50%% threshold against %s", tc.name, diff*100, goldenPath)
					}
					t.Logf("VRT %s pixel diff=%.3f%% (ok)", tc.name, diff*100)
				} else {
					t.Logf("VRT %s baseline not found at %s (run with ASTERLAB_UPDATE_VRT=1 to seed)", tc.name, goldenPath)
				}
			}
			_ = fmt.Sprint(tc.name)
		})
	}

	// Verify Ctrl+= / Ctrl+- / Ctrl+0 unified zoom scaling changes TextScale cleanly.
	sh.TextScale = 0
	sh.StepUIScale(nil, 0.15)
	if sh.TextScale < 1.14 || sh.TextScale > 1.16 {
		t.Fatalf("expected StepUIScale(+0.15) from 1.0 -> 1.15, got %.2f", sh.TextScale)
	}
	sh.StepUIScale(nil, -0.35)
	if sh.TextScale < 0.84 || sh.TextScale > 0.86 {
		t.Fatalf("expected StepUIScale clamp to 0.85, got %.2f", sh.TextScale)
	}
	sh.TextScale = 0
	sh.SyncThemeLook(nil)
}
