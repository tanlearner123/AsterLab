//go:build soak

package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/modules/runs"
)

// TestQA5_Soak_LongRunningMemoryAndGoroutineStability implements Layer 5 (Soak Testing),
// isolated behind `//go:build soak` so regular CI stays fast. Run on demand:
//
//	go test -tags soak -v -timeout 30m -run TestQA5_Soak .
//	ASTERLAB_SOAK_STEPS=3000 go test -tags soak -v -timeout 30m -run TestQA5_Soak .
func TestQA5_Soak_LongRunningMemoryAndGoroutineStability(t *testing.T) {
	steps := 600
	if raw := os.Getenv("ASTERLAB_SOAK_STEPS"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			steps = n
		}
	}

	vaultRoot := filepath.Join(t.TempDir(), "SoakVault")
	if err := os.MkdirAll(filepath.Join(vaultRoot, "Proj"), 0o755); err != nil {
		t.Fatal(err)
	}
	docs := []string{"Proj/Doc1.asterml", "Proj/Doc2.asterml", "Proj/Doc3.asterml"}
	for i, rel := range docs {
		src := fsmTestDocAlpha
		if i%2 == 1 {
			src = fsmTestDocBeta
		}
		if err := os.WriteFile(filepath.Join(vaultRoot, filepath.FromSlash(rel)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	a, sh := newWiredFSMTestApp(t, vaultRoot)
	st := ui.NewTester(func(c *ui.Context) {
		sh.RenderSplitView(c)
	}, 1280, 800)

	// Warmup before taking baseline goroutine & heap measurements.
	for i := 0; i < 20; i++ {
		sh.OpenFileInPane(sh.ActivePaneID, docs[i%len(docs)], "", shell.ModeEdit, false)
		st.Frame()
	}
	runtime.GC()
	var baseMem runtime.MemStats
	runtime.ReadMemStats(&baseMem)
	baseGoroutines := runtime.NumGoroutine()

	rng := rand.New(rand.NewSource(99887766))
	frameTimes := make([]time.Duration, 0, steps)

	for step := 0; step < steps; step++ {
		switch rng.Intn(8) {
		case 0:
			sh.OpenFileInPane(sh.ActivePaneID, docs[rng.Intn(len(docs))], "", shell.ModeEdit, false)
		case 1:
			modes := []shell.ViewMode{shell.ModeEdit, shell.ModeLab, shell.ModeSource}
			sh.SetActiveTabMode(modes[rng.Intn(len(modes))])
		case 2:
			sh.OpenPantryTab()
		case 3:
			sh.OpenCalendarTab()
		case 4:
			sh.SetRightDrawer(rng.Intn(2) == 0, rng.Intn(3))
		case 5:
			if t0 := sh.ActiveTab(); t0 != nil && !t0.Pinned {
				sh.CloseTabInPane(sh.ActivePaneID, t0.ID)
			}
		case 6:
			_, _ = a.localUpsertPantry(runs.PantryUpsert{
				Name:     fmt.Sprintf("SoakReagent-%d", step%12),
				Location: "-20°C",
			})
			a.tools.ReloadPantry()
			a.syncPantrySnapshotToShell()
		case 7:
			p := filepath.Join(vaultRoot, filepath.FromSlash(docs[rng.Intn(len(docs))]))
			future := time.Now().Add(time.Duration(step+1) * time.Second)
			_ = os.Chtimes(p, future, future)
			a.syncVaultDiskChangesNow()
		}

		assertGlobalShellInvariants(t, sh, fmt.Sprintf("soak-step-%d", step))
		f0 := time.Now()
		st.Frame()
		frameTimes = append(frameTimes, time.Since(f0))
	}

	runtime.GC()
	var endMem runtime.MemStats
	runtime.ReadMemStats(&endMem)
	endGoroutines := runtime.NumGoroutine()

	sort.Slice(frameTimes, func(i, j int) bool { return frameTimes[i] < frameTimes[j] })
	p50 := frameTimes[len(frameTimes)/2]
	p95 := frameTimes[(len(frameTimes)*95)/100]
	maxFrame := frameTimes[len(frameTimes)-1]
	heapDeltaMB := (float64(endMem.HeapAlloc) - float64(baseMem.HeapAlloc)) / (1024 * 1024)
	gDelta := endGoroutines - baseGoroutines

	t.Logf("SOAK (%d steps): goroutines %d -> %d (delta %+d) | heap %.1f MB -> %.1f MB (delta %+.1f MB) | frame p50=%v p95=%v max=%v",
		steps, baseGoroutines, endGoroutines, gDelta,
		float64(baseMem.HeapAlloc)/(1024*1024), float64(endMem.HeapAlloc)/(1024*1024), heapDeltaMB,
		p50, p95, maxFrame)

	if gDelta > 15 {
		t.Fatalf("suspected goroutine leak during soak: %d -> %d (delta +%d)", baseGoroutines, endGoroutines, gDelta)
	}
	if heapDeltaMB > 256 {
		t.Fatalf("suspected unbounded heap growth during soak: delta +%.1f MB", heapDeltaMB)
	}
	if p95 > 500*time.Millisecond {
		t.Fatalf("soak p95 frame latency exceeded 500ms budget: %v", p95)
	}
}
