package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"asterlab-mygo-prototype/internal/astercmd"
	"asterlab-mygo-prototype/internal/astercore"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/vault"
)

const (
	exitOK       = 0
	exitUsage    = 2
	exitNotFound = 3
	exitBlocked  = 4
	exitInvalid  = 5
)

func main() {
	os.Exit(runCLIWithStdin(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	return runCLIWithStdin(args, strings.NewReader(""), stdout, stderr)
}

func emitJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

func emitError(w io.Writer, code int, msg string) int {
	emitJSON(w, map[string]any{
		"ok":    false,
		"code":  code,
		"error": msg,
	})
	return code
}

func runCLIWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("asterlab", flag.ContinueOnError)
	fs.SetOutput(stderr)
	vaultDir := fs.String("vault", "", "AsterLab Vault directory (defaults to local Vault)")
	if err := fs.Parse(args); err != nil {
		return emitError(stdout, exitUsage, err.Error())
	}
	rest := fs.Args()
	if len(rest) == 0 {
		return emitError(stdout, exitUsage, "missing subcommand (try: tools | cmd.list | cmd.describe | vault.list | vault.search | asterdoc.read | asterdoc.objects | asterdoc.relations | asterdoc.validate | asterdoc.preview | asterdoc.exec | asterdoc.assess | asterdoc.analyze)")
	}
	cmd := rest[0]
	cmdArgs := rest[1:]

	resolvedVault := strings.TrimSpace(*vaultDir)
	if resolvedVault == "" {
		resolvedVault = vault.DefaultVaultDir()
	}

	if cmd == "tools" {
		emitJSON(stdout, map[string]any{
			"ok":    true,
			"vault": resolvedVault,
			"tools": []map[string]any{
				{"name": "tools", "description": "List available CLI tools as JSON"},
				{"name": "cmd.list", "description": "List all registered Semantic Commands (shared by UI Slash & Agent)"},
				{"name": "cmd.describe", "description": "Describe input schema and permissions for one Semantic Command"},
				{"name": "vault.list", "description": "List .asterml experiments with optional --status / --tag filters"},
				{"name": "vault.search", "description": "Full-text search across all .asterml files"},
				{"name": "asterdoc.read", "description": "Parse and return AsterML frontmatter, docHash, steps & modules"},
				{"name": "asterdoc.objects", "description": "List all stable-ID scientific objects (groups, plates, steps, blocks) with docHash"},
				{"name": "asterdoc.relations", "description": "Extract and resolve all inline ![[...]]{@...} and attachment scientific relations"},
				{"name": "asterdoc.validate", "description": "Validate AsterML syntax, references, and inline relation diagnostics"},
				{"name": "asterdoc.preview", "description": "Dry-run a JSON BatchRequest of Semantic Commands and return diff summary & warnings"},
				{"name": "asterdoc.exec", "description": "Atomically execute a JSON BatchRequest of Semantic Commands with expected_hash & idempotency"},
				{"name": "asterdoc.assess", "description": "Run Pure Go ARRIVE/control/dilution scientific diagnostics"},
				{"name": "asterdoc.analyze", "description": "Run deterministic CCK-8 / RT-qPCR / descriptive analysis"},
			},
		})
		return exitOK
	}

	if cmd == "cmd.list" {
		reg := astercmd.DefaultRegistry()
		emitJSON(stdout, map[string]any{
			"ok":          true,
			"commands":    reg.List(),
			"stepPresets": astercmd.StepPresetDefs(),
		})
		return exitOK
	}

	if cmd == "cmd.describe" {
		if len(cmdArgs) == 0 || strings.TrimSpace(cmdArgs[0]) == "" {
			return emitError(stdout, exitUsage, "cmd.describe requires a command_id")
		}
		desc, ok := astercmd.DefaultRegistry().Describe(cmdArgs[0])
		if !ok {
			return emitError(stdout, exitNotFound, fmt.Sprintf("unknown command %q", cmdArgs[0]))
		}
		emitJSON(stdout, map[string]any{
			"ok":      true,
			"command": desc,
		})
		return exitOK
	}

	v, err := vault.Open(resolvedVault)
	if err != nil {
		return emitError(stdout, exitNotFound, fmt.Sprintf("open vault %q: %v", resolvedVault, err))
	}
	idx := vault.NewVaultIndex(v)

	switch cmd {
	case "vault.list":
		sub := flag.NewFlagSet("vault.list", flag.ContinueOnError)
		sub.SetOutput(stderr)
		status := sub.String("status", "", "Filter by status (planned|in_progress|completed)")
		tag := sub.String("tag", "", "Filter by tag")
		folder := sub.String("folder", "", "Filter by folder prefix")
		if err := sub.Parse(cmdArgs); err != nil {
			return emitError(stdout, exitUsage, err.Error())
		}
		if err := idx.Refresh(); err != nil {
			return emitError(stdout, exitNotFound, err.Error())
		}
		rows := idx.QueryBases(vault.BasesQuery{
			FolderPrefix: *folder,
			Status:       *status,
			Tag:          *tag,
		})
		emitJSON(stdout, map[string]any{
			"ok":    true,
			"vault": v.Root,
			"count": len(rows),
			"files": rows,
		})
		return exitOK

	case "vault.search":
		if len(cmdArgs) == 0 || strings.TrimSpace(strings.Join(cmdArgs, " ")) == "" {
			return emitError(stdout, exitUsage, "vault.search requires a query string")
		}
		if err := idx.Refresh(); err != nil {
			return emitError(stdout, exitNotFound, err.Error())
		}
		q := strings.TrimSpace(strings.Join(cmdArgs, " "))
		hits := idx.SearchFullText(q, 50)
		emitJSON(stdout, map[string]any{
			"ok":    true,
			"query": q,
			"count": len(hits),
			"hits":  hits,
		})
		return exitOK

	case "asterdoc.read", "asterdoc.objects", "asterdoc.relations", "asterdoc.validate", "asterdoc.assess", "asterdoc.analyze":
		if len(cmdArgs) == 0 || strings.TrimSpace(cmdArgs[0]) == "" {
			return emitError(stdout, exitUsage, cmd+" requires a relative .asterml file path")
		}
		rel := cmdArgs[0]
		abs := v.AbsPath(rel)
		rawBytes, rErr := os.ReadFile(abs)
		if rErr != nil {
			return emitError(stdout, exitNotFound, fmt.Sprintf("read %q: %v", rel, rErr))
		}
		src := string(rawBytes)
		docHash := astercmd.DocumentHash(src)
		fm, _, _ := asterdoc.ScanFrontmatter(src)
		doc, pErr := asterdoc.Parse(src)
		if pErr != nil {
			if cmd == "asterdoc.validate" {
				emitJSON(stdout, map[string]any{
					"ok":       false,
					"relPath":  rel,
					"docHash":  docHash,
					"parseErr": pErr.Error(),
				})
				return exitInvalid
			}
			return emitError(stdout, exitInvalid, fmt.Sprintf("parse %q: %v", rel, pErr))
		}
		switch cmd {
		case "asterdoc.read":
			stepTree := asterdoc.ParseStepTree(src, doc)
			rels := asterdoc.ResolveRelations(doc)
			emitJSON(stdout, map[string]any{
				"ok":            true,
				"relPath":       rel,
				"docHash":       docHash,
				"frontmatter":   fm,
				"steps":         stepTree.Flat,
				"stepTree":      stepTree.Roots,
				"moduleCount":   len(doc.Modules),
				"relationCount": len(rels),
			})
			return exitOK

		case "asterdoc.objects":
			stepTree := asterdoc.ParseStepTree(src, doc)
			objs := make([]map[string]any, 0, len(doc.Modules)+len(stepTree.Flat))
			for _, sn := range stepTree.Flat {
				if sn.SourceKind == "heading" {
					objs = append(objs, map[string]any{
						"id":       sn.ID,
						"kind":     "step",
						"scope":    "heading",
						"number":   sn.Number,
						"depth":    sn.Depth,
						"parentId": sn.ParentID,
						"title":    sn.Title,
					})
				}
			}
			for _, m := range doc.Modules {
				entry := map[string]any{
					"id":    m.ID,
					"kind":  m.Kind,
					"scope": "top_level",
					"title": m.Data["title"],
				}
				objs = append(objs, entry)
				if m.Kind == "step" {
					if blks, ok := m.Data["blocks"].([]any); ok {
						for bi, rawBlk := range blks {
							if bm, ok := rawBlk.(map[string]any); ok {
								objs = append(objs, map[string]any{
									"id":      bm["id"],
									"kind":    bm["kind"],
									"scope":   "step",
									"stepId":  m.ID,
									"index":   bi,
									"title":   bm["title"],
								})
							}
						}
					}
				}
			}
			emitJSON(stdout, map[string]any{
				"ok":      true,
				"relPath": rel,
				"docHash": docHash,
				"groups":  doc.GroupList(),
				"objects": objs,
			})
			return exitOK

		case "asterdoc.relations":
			rels := asterdoc.ResolveRelations(doc)
			var issues []string
			for _, r := range rels {
				issues = append(issues, r.Issues...)
			}
			emitJSON(stdout, map[string]any{
				"ok":        len(issues) == 0,
				"relPath":   rel,
				"docHash":   docHash,
				"count":     len(rels),
				"relations": rels,
				"issues":    issues,
			})
			return exitOK

		case "asterdoc.validate":
			rels := asterdoc.ResolveRelations(doc)
			var issues []string
			for _, r := range rels {
				issues = append(issues, r.Issues...)
			}
			emitJSON(stdout, map[string]any{
				"ok":             len(issues) == 0,
				"relPath":        rel,
				"docHash":        docHash,
				"moduleCount":    len(doc.Modules),
				"relationCount":  len(rels),
				"relationIssues": issues,
			})
			if len(issues) > 0 {
				return exitInvalid
			}
			return exitOK

		case "asterdoc.assess":
			rep := astercore.AssessDocument(doc, nil)
			emitJSON(stdout, map[string]any{
				"ok":         rep.Blocking == 0,
				"relPath":    rel,
				"docHash":    docHash,
				"assessment": rep,
			})
			if rep.Blocking > 0 {
				return exitBlocked
			}
			return exitOK

		default: // asterdoc.analyze
			sums := astercore.AnalyzeDocument(doc, rel)
			emitJSON(stdout, map[string]any{
				"ok":        true,
				"relPath":   rel,
				"docHash":   docHash,
				"count":     len(sums),
				"summaries": sums,
			})
			return exitOK
		}

	case "asterdoc.preview", "asterdoc.exec":
		sub := flag.NewFlagSet(cmd, flag.ContinueOnError)
		sub.SetOutput(stderr)
		opsJSON := sub.String("ops", "", "JSON BatchRequest payload (or pipe via stdin)")
		if err := sub.Parse(cmdArgs); err != nil {
			return emitError(stdout, exitUsage, err.Error())
		}
		posArgs := sub.Args()
		if len(posArgs) == 0 || strings.TrimSpace(posArgs[0]) == "" {
			return emitError(stdout, exitUsage, cmd+" requires a relative .asterml file path")
		}
		rel := posArgs[0]
		abs := v.AbsPath(rel)
		payload := strings.TrimSpace(*opsJSON)
		if payload == "" && stdin != nil {
			b, _ := io.ReadAll(stdin)
			payload = strings.TrimSpace(string(b))
		}
		if payload == "" {
			return emitError(stdout, exitUsage, cmd+" requires a JSON BatchRequest via --ops or stdin")
		}
		var req astercmd.BatchRequest
		if err := json.Unmarshal([]byte(payload), &req); err != nil {
			return emitError(stdout, exitUsage, "invalid JSON BatchRequest: "+err.Error())
		}
		if cmd == "asterdoc.preview" {
			req.PreviewOnly = true
		}
		journalPath := filepath.Join(v.Root, vault.SystemDirName, "cache", "cmd-journal.json")
		res := astercmd.ExecuteBatchOnFile(abs, journalPath, req)
		if res.OK && !req.PreviewOnly {
			if raw, rErr := os.ReadFile(abs); rErr == nil {
				idx.UpsertSource(rel, string(raw))
			}
		}
		emitJSON(stdout, res)
		if !res.OK {
			if res.Error != nil && res.Error.Code == "CONFLICT" {
				return exitBlocked
			}
			return exitInvalid
		}
		return exitOK

	default:
		return emitError(stdout, exitUsage, fmt.Sprintf("unknown subcommand %q", cmd))
	}
}
