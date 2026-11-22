package asterdoc

import (
	"bytes"
	"image/png"
	"path/filepath"
	"sync"
	"testing"
)

func TestWriteFileAtomicConcurrent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "concurrent.asterml")
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			payload := []byte("---\ntitle: test\n---\n# doc\n")
			if err := WriteFileAtomic(target, payload); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent WriteFileAtomic failed: %v", err)
	}
}

func TestRenderPlatePNG(t *testing.T) {
	p := &Plate{
		ID:            "p-96",
		Format:        96,
		Groups:        map[string][]string{"ctrl": {"A1", "A2"}, "treat": {"B1", "B2"}},
		ExcludedWells: []string{"B2"},
	}
	g, ok := geometryForPlate(96, "", 0, 0)
	if !ok {
		t.Fatalf("expected 96-well geometry")
	}
	raw, err := RenderPlatePNG(p, g, nil, "group", "", false)
	if err != nil {
		t.Fatalf("RenderPlatePNG failed: %v", err)
	}
	if len(raw) < 32 {
		t.Fatalf("expected non-empty PNG bytes, got %d", len(raw))
	}
	img, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("RenderPlatePNG output is not valid PNG: %v", err)
	}
	if img.Bounds().Dx() <= 0 || img.Bounds().Dy() <= 0 {
		t.Fatalf("unexpected decoded PNG bounds: %v", img.Bounds())
	}
}

func TestDefaultStarterBlockWithPrefs(t *testing.T) {
	plate := DefaultStarterBlockWithPrefs(nil, "step-1", "plate", 96, 15)
	if plate["format"] != 96 {
		t.Fatalf("expected plate format=96, got %v", plate["format"])
	}
	if plate["id"] == "" {
		t.Fatalf("expected generated block id on starter plate")
	}

	chk := DefaultStarterBlockWithPrefs(nil, "step-1", "checklist", 96, 15)
	items, ok := chk["items"].([]any)
	if !ok || len(items) == 0 {
		t.Fatalf("expected checklist items slice, got %T", chk["items"])
	}
	first, _ := items[0].(map[string]any)
	if first["label"] == nil || first["label"] == "" {
		t.Fatalf("expected checklist item to use 'label' schema key, got %+v", first)
	}

	calc := DefaultStarterBlockWithPrefs(nil, "step-1", "calculator", 96, 15)
	if ratio, _ := calc["excess_ratio"].(float64); ratio != 0.15 {
		t.Fatalf("expected calculator excess_ratio=0.15, got %v", calc["excess_ratio"])
	}
	if _, bad := calc["excess_pct"]; bad {
		t.Fatalf("calculator must not emit undeclared excess_pct key")
	}
}
