package astercmd

import (
	"path/filepath"
	"strings"
	"testing"

	"asterlab-mygo-prototype/internal/asterdoc"
)

func TestVerticalSliceBlankToGroupPlateWellsEmbedRelation(t *testing.T) {
	blank := "---\ntitle: HUVEC 成管实验\n---\n\n先写一段实验目的：评估 VEGF 对 HUVEC 成管的影响。\n"

	// 1. Atomic batch with client_ref: create group -> create plate -> assign A1,A2 -> embed photo -> relate to A1
	res := ExecuteBatch(blank, BatchRequest{
		ExpectedHash: DocumentHash(blank),
		Operations: []Operation{
			{
				CommandID: "group.create",
				ClientRef: "grp_vegf",
				Args:      map[string]any{"name": "VEGF 处理组", "role": "treatment"},
			},
			{
				CommandID: "plate.create",
				ClientRef: "plate_main",
				Args:      map[string]any{"title": "12 孔成管板", "format": 12},
			},
			{
				CommandID: "plate.assign_wells",
				Args: map[string]any{
					"plate_id": "plate_main",
					"group_id": "grp_vegf",
					"wells":    []string{"A1", "A2"},
				},
			},
			{
				CommandID: "asset.embed",
				Args: map[string]any{
					"asset":     "./assets/huvec-a1-6h.png",
					"targets":   []string{"@plate_main:A1"},
					"timepoint": "6h",
					"fov":       1,
					"role":      "obs",
				},
			},
		},
	})
	if !res.OK {
		t.Fatalf("vertical slice batch failed: %v", res.Error)
	}
	if !strings.Contains(res.NextSource, "先写一段实验目的：评估 VEGF 对 HUVEC 成管的影响。") {
		t.Fatalf("original prose was damaged")
	}
	doc, err := asterdoc.Parse(res.NextSource)
	if err != nil {
		t.Fatalf("Parse after batch failed: %v", err)
	}
	rels := asterdoc.ResolveRelations(doc)
	if len(rels) != 1 || len(rels[0].Contexts) != 1 || !rels[0].Contexts[0].Valid {
		t.Fatalf("expected 1 valid resolved relation, got %+v", rels)
	}
	if rels[0].Contexts[0].GroupName != "VEGF 处理组" || rels[0].Timepoint != "6h" || rels[0].FOV != 1 {
		t.Fatalf("resolved relation context mismatch: %+v", rels[0])
	}

	// 2. Optimistic concurrency conflict check
	conflict := ExecuteBatch(res.NextSource, BatchRequest{
		ExpectedHash: DocumentHash(blank), // stale hash
		Operations: []Operation{
			{CommandID: "doc.append_note", Args: map[string]any{"text": "不会写入"}},
		},
	})
	if conflict.OK || conflict.Error == nil || conflict.Error.Code != "CONFLICT" {
		t.Fatalf("expected CONFLICT on stale hash, got %+v", conflict)
	}

	// 3. Atomic rollback on invalid well assignment (H12 outside 12-well plate)
	bad := ExecuteBatch(res.NextSource, BatchRequest{
		Operations: []Operation{
			{CommandID: "group.create", ClientRef: "g_bad", Args: map[string]any{"name": "临时组"}},
			{CommandID: "plate.assign_wells", Args: map[string]any{
				"plate_id": res.CreatedIDs["plate_main"],
				"group_id": "g_bad",
				"wells":    []string{"H12"},
			}},
		},
	})
	if bad.OK || bad.NextSource != "" {
		t.Fatalf("expected atomic failure on invalid well H12, got %+v", bad)
	}

	// 4. LabMode guard: blocks structural edits, allows lab notes & asset embeds
	labBlocked := ExecuteBatch(res.NextSource, BatchRequest{
		Mode: ModeLab,
		Operations: []Operation{
			{CommandID: "group.create", Args: map[string]any{"name": "误触分组"}},
		},
	})
	if labBlocked.OK || labBlocked.Error == nil || labBlocked.Error.Code != "MODE_BLOCKED" {
		t.Fatalf("expected MODE_BLOCKED in LabMode, got %+v", labBlocked)
	}
	labOK := ExecuteBatch(res.NextSource, BatchRequest{
		Mode: ModeLab,
		Operations: []Operation{
			{CommandID: "doc.append_note", Args: map[string]any{"text": "14:20 观察到 A1 成管明显"}},
		},
	})
	if !labOK.OK {
		t.Fatalf("expected doc.append_note allowed in LabMode, got %v", labOK.Error)
	}
}

func TestExecuteBatchOnFileIdempotency(t *testing.T) {
	dir := t.TempDir()
	docPath := filepath.Join(dir, "exp.asterml")
	journalPath := filepath.Join(dir, "cmd-journal.json")
	if err := asterdoc.WriteFileAtomic(docPath, []byte("---\ntitle: 幂等测试\n---\n\n")); err != nil {
		t.Fatal(err)
	}
	req := BatchRequest{
		IdempotencyKey: "op-retry-001",
		Operations: []Operation{
			{CommandID: "step.create", Args: map[string]any{"title": "孵育", "preset": "timed"}},
		},
	}
	r1 := ExecuteBatchOnFile(docPath, journalPath, req)
	if !r1.OK || r1.Replayed {
		t.Fatalf("first run failed: %+v", r1)
	}
	r2 := ExecuteBatchOnFile(docPath, journalPath, req)
	if !r2.OK || !r2.Replayed || r2.DocumentHash != r1.DocumentHash {
		t.Fatalf("second run should replay idempotently: %+v", r2)
	}
}

func TestAllSlashInsertCommandsValidateCleanlyOnBlankDoc(t *testing.T) {
	blank := "---\nasterml: \"2.1\"\ntitle: 空白实验\n---\n\n"
	for _, entry := range DefaultRegistry().SlashEntries("top_level", ModeEdit) {
		if entry.CommandID == "asset.embed" {
			continue
		}
		args := map[string]any{}
		if entry.CommandID == "block.insert" {
			args["kind"] = entry.Kind
		}
		if entry.CommandID == "step.create" {
			args["preset"] = entry.Preset
		}
		if entry.CommandID == "doc.append_note" {
			args["text"] = "观察备注"
		}
		res := ExecuteBatch(blank, BatchRequest{
			Mode:        ModeEdit,
			PlateFormat: 12,
			ExcessPct:   10,
			Operations:  []Operation{{CommandID: entry.CommandID, Args: args}},
		})
		if !res.OK {
			t.Fatalf("slash %s (%s kind=%s preset=%s) failed on blank doc: %v",
				entry.Slash, entry.CommandID, entry.Kind, entry.Preset, res.Error)
		}
	}
}
