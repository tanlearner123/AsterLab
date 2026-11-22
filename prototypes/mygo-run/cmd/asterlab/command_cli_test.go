package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAgentCommandFullLoop(t *testing.T) {
	vaultDir := t.TempDir()
	expPath := filepath.Join(vaultDir, "huvec.asterml")
	initial := "---\ntitle: HUVEC 成管\n---\n\n自由记录：准备测试 VEGF 促成管效果。\n"
	if err := os.WriteFile(expPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	run := func(stdin string, args ...string) (int, map[string]any) {
		var out, errBuf bytes.Buffer
		full := append([]string{"--vault", vaultDir}, args...)
		code := runCLIWithStdin(full, strings.NewReader(stdin), &out, &errBuf)
		var parsed map[string]any
		if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
			t.Fatalf("CLI %v invalid JSON (%q): %v (stderr=%s)", args, out.String(), err, errBuf.String())
		}
		return code, parsed
	}

	// 1. Discover commands
	code, listRes := run("", "cmd.list")
	if code != exitOK || listRes["ok"] != true {
		t.Fatalf("cmd.list failed: code=%d res=%v", code, listRes)
	}

	// 2. Read initial objects & docHash
	code, objRes := run("", "asterdoc.objects", "huvec.asterml")
	if code != exitOK {
		t.Fatalf("asterdoc.objects failed: %v", objRes)
	}
	docHash, _ := objRes["docHash"].(string)
	if !strings.HasPrefix(docHash, "sha256:") {
		t.Fatalf("expected sha256 docHash, got %q", docHash)
	}

	// 3. Preview batch (must not mutate file on disk)
	batch := `{
		"expected_hash": "` + docHash + `",
		"idempotency_key": "agent-slice-1",
		"operations": [
			{"command_id": "group.create", "client_ref": "g_vegf", "args": {"name": "VEGF 组", "role": "treatment"}},
			{"command_id": "plate.create", "client_ref": "p12", "args": {"title": "12孔板", "format": 12}},
			{"command_id": "plate.assign_wells", "args": {"plate_id": "p12", "group_id": "g_vegf", "wells": ["A1", "A2"]}},
			{"command_id": "asset.embed", "args": {"asset": "./assets/a1_6h.png", "targets": ["@p12:A1"], "timepoint": "6h", "fov": 1, "role": "obs"}}
		]
	}`
	code, prevRes := run(batch, "asterdoc.preview", "huvec.asterml")
	if code != exitOK || prevRes["ok"] != true || prevRes["preview_only"] != true {
		t.Fatalf("asterdoc.preview failed: code=%d res=%v", code, prevRes)
	}
	afterPreview, _ := os.ReadFile(expPath)
	if string(afterPreview) != initial {
		t.Fatalf("preview mutated file on disk")
	}

	// 4. Execute batch atomically
	code, execRes := run(batch, "asterdoc.exec", "huvec.asterml")
	if code != exitOK || execRes["ok"] != true {
		t.Fatalf("asterdoc.exec failed: code=%d res=%v", code, execRes)
	}

	// 5. Retry with same idempotency_key -> replayed without duplicating objects
	code, retryRes := run(batch, "asterdoc.exec", "huvec.asterml")
	if code != exitOK || retryRes["replayed"] != true {
		t.Fatalf("expected idempotent replay, got code=%d res=%v", code, retryRes)
	}

	// 6. Query resolved relations (must resolve plate + group context cleanly)
	code, relRes := run("", "asterdoc.relations", "huvec.asterml")
	if code != exitOK || relRes["ok"] != true {
		t.Fatalf("asterdoc.relations failed: code=%d res=%v", code, relRes)
	}
	if int(relRes["count"].(float64)) != 1 {
		t.Fatalf("expected 1 relation, got %v", relRes)
	}

	// 7. Validate document
	code, valRes := run("", "asterdoc.validate", "huvec.asterml")
	if code != exitOK || valRes["ok"] != true {
		t.Fatalf("asterdoc.validate failed: code=%d res=%v", code, valRes)
	}
}
