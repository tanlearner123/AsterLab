package vault

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"asterlab-mygo-prototype/internal/asterdoc"
)

const IndexCacheFile = "cache/index.json"

// IndexedDoc holds cached Obsidian-MetadataCache-equivalent metadata for one `.asterml` file.
type IndexedDoc struct {
	RelPath      string            `json:"relPath"`
	Title        string            `json:"title"`
	Folder       string            `json:"folder"`
	Date         string            `json:"date"`
	Status       string            `json:"status"`
	RepeatOf     string            `json:"repeatOf,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	IsTemplate   bool              `json:"isTemplate,omitempty"`
	Steps        []string          `json:"steps,omitempty"`
	PantryCodes  []string          `json:"pantryCodes,omitempty"`
	WikiLinks    []string          `json:"wikiLinks,omitempty"`
	Relations    []IndexedRelation `json:"relations,omitempty"`
	ActiveTimers []VaultTimerEntry `json:"activeTimers,omitempty"`
	GroupCount   int               `json:"groupCount,omitempty"`
	Lines        []string          `json:"lines,omitempty"`
	ModTimeUnix  int64             `json:"modTimeUnix"`
	Size         int64             `json:"size"`
}

// VaultTimerEntry represents a running/paused timer inside any `.asterml` file in the Vault.
type VaultTimerEntry struct {
	RelPath     string `json:"relPath"`
	DocTitle    string `json:"docTitle"`
	StepID      string `json:"stepId"`
	TimerID     string `json:"timerId"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Mode        string `json:"mode"`
	PlannedSec  int    `json:"plannedSec"`
	ElapsedSec  int    `json:"elapsedSec"`
	StartedAt   string `json:"startedAt,omitempty"`
}

// SearchHit describes one full-text match across the Vault.
type SearchHit struct {
	RelPath  string `json:"relPath"`
	Title    string `json:"title"`
	Folder   string `json:"folder"`
	LineNum  int    `json:"lineNum"`
	Snippet  string `json:"snippet"`
	Date     string `json:"date,omitempty"`
	Status   string `json:"status,omitempty"`
}

// LinkRef describes a backlink or repeat lineage link pointing to a target document.
type LinkRef struct {
	SourceRel   string `json:"sourceRel"`
	SourceTitle string `json:"sourceTitle"`
	SourceDate  string `json:"sourceDate,omitempty"`
	Kind        string `json:"kind"` // "wikilink" | "repeat_of"
}

// IndexedRelation caches one explicit inline or attachment scientific relation for fast bidirectional lookup.
type IndexedRelation struct {
	RelPath   string `json:"relPath"`
	Asset     string `json:"asset"`
	ObjectID  string `json:"objectId"`
	SubTarget string `json:"subTarget,omitempty"`
	Timepoint string `json:"timepoint,omitempty"`
	FOV       int    `json:"fov,omitempty"`
	Role      string `json:"role,omitempty"`
	Line      int    `json:"line,omitempty"`
	Valid     bool   `json:"valid"`
}

// BasesQuery defines structured filtering for cross-experiment LabBases queries.
type BasesQuery struct {
	FolderPrefix string `json:"folderPrefix,omitempty"`
	Status       string `json:"status,omitempty"`
	Tag          string `json:"tag,omitempty"`
	Keyword      string `json:"keyword,omitempty"`
}

// BasesRow is one normalized row returned by QueryBases.
type BasesRow struct {
	RelPath    string   `json:"relPath"`
	Title      string   `json:"title"`
	Folder     string   `json:"folder"`
	Date       string   `json:"date"`
	Status     string   `json:"status"`
	GroupCount int      `json:"groupCount"`
	StepCount  int      `json:"stepCount"`
	Tags       []string `json:"tags,omitempty"`
	RepeatOf   string   `json:"repeatOf,omitempty"`
}

// VaultIndex provides incremental caching, full-text search, backlinks, global timer
// aggregation, and cross-experiment LabBases queries over a local Vault.
type VaultIndex struct {
	mu    sync.RWMutex
	vault *Vault
	docs  map[string]IndexedDoc
}

// NewVaultIndex creates (and loads any cached `.asterlab/cache/index.json` for) v.
func NewVaultIndex(v *Vault) *VaultIndex {
	idx := &VaultIndex{
		vault: v,
		docs:  map[string]IndexedDoc{},
	}
	idx.loadCache()
	return idx
}

func (idx *VaultIndex) cachePath() string {
	return idx.vault.systemPath(filepath.FromSlash(IndexCacheFile))
}

func (idx *VaultIndex) loadCache() {
	if idx == nil || idx.vault == nil {
		return
	}
	raw, err := os.ReadFile(idx.cachePath())
	if err != nil {
		return
	}
	var cached map[string]IndexedDoc
	if json.Unmarshal(raw, &cached) == nil && cached != nil {
		idx.mu.Lock()
		idx.docs = cached
		idx.mu.Unlock()
	}
}

func (idx *VaultIndex) saveCacheLocked() {
	if idx == nil || idx.vault == nil {
		return
	}
	data, err := json.Marshal(idx.docs)
	if err != nil {
		return
	}
	_ = asterdoc.WriteFileAtomic(idx.cachePath(), data)
}

// Refresh incrementally re-indexes only `.asterml` files whose ModTime or Size changed.
func (idx *VaultIndex) Refresh() error {
	if idx == nil || idx.vault == nil {
		return nil
	}
	_, files, err := idx.vault.ScanTree()
	if err != nil {
		return err
	}
	present := make(map[string]bool, len(files))
	idx.mu.Lock()
	defer idx.mu.Unlock()

	dirty := false
	for _, f := range files {
		if f.IsAsset {
			continue
		}
		present[f.RelPath] = true
		mtime := f.ModTime.UnixNano()
		if existing, ok := idx.docs[f.RelPath]; ok && existing.ModTimeUnix == mtime && existing.Size == f.Size {
			continue
		}
		rawBytes, rErr := os.ReadFile(f.AbsPath)
		if rErr != nil {
			continue
		}
		idx.docs[f.RelPath] = indexOneSource(f, string(rawBytes), mtime)
		dirty = true
	}
	for k := range idx.docs {
		if !present[k] {
			delete(idx.docs, k)
			dirty = true
		}
	}
	if dirty {
		idx.saveCacheLocked()
	}
	return nil
}

// UpsertSource updates the in-memory & cached index entry for one file immediately after edit.
func (idx *VaultIndex) UpsertSource(relPath, source string) {
	if idx == nil || idx.vault == nil || relPath == "" {
		return
	}
	abs := idx.vault.AbsPath(relPath)
	var mtime, size int64
	if st, err := os.Stat(abs); err == nil {
		mtime = st.ModTime().UnixNano()
		size = st.Size()
	} else {
		mtime = time.Now().UnixNano()
		size = int64(len(source))
	}
	folder := filepath.ToSlash(filepath.Dir(relPath))
	if folder == "." {
		folder = ""
	}
	fe := FileEntry{
		RelPath:    relPath,
		AbsPath:    abs,
		Name:       strings.TrimSuffix(filepath.Base(relPath), filepath.Ext(relPath)),
		Folder:     folder,
		IsTemplate: folder == TemplatesDirName || strings.HasPrefix(folder, TemplatesDirName+"/"),
		Size:       size,
	}
	rec := indexOneSource(fe, source, mtime)
	idx.mu.Lock()
	idx.docs[relPath] = rec
	idx.saveCacheLocked()
	idx.mu.Unlock()
}

func indexOneSource(f FileEntry, src string, mtime int64) IndexedDoc {
	fm, _, _ := asterdoc.ScanFrontmatter(src)
	title := strings.TrimSpace(fm.Title)
	if title == "" {
		title = f.Name
	}
	status := normalizeStatus(fm.Stage)
	if status == "" {
		status = f.Status
	}
	date := asterdoc.NormalizeDateYMD(fm.ScheduledDate)
	if date == "" {
		date = f.Date
	}

	var steps []string
	var pantryCodes []string
	var timers []VaultTimerEntry
	var relations []IndexedRelation
	groupCount := 0
	pantrySeen := map[string]bool{}
	addPantry := func(c string) {
		c = strings.TrimSpace(c)
		if c != "" && !pantrySeen[c] {
			pantrySeen[c] = true
			pantryCodes = append(pantryCodes, c)
		}
	}

	if parsed, err := asterdoc.Parse(src); err == nil && parsed != nil {
		for _, m := range parsed.Modules {
			switch m.Kind {
			case "step":
				stTitle, _ := m.Data["title"].(string)
				steps = append(steps, firstNonEmptyStr(stTitle, m.ID))
				if blks, ok := m.Data["blocks"].([]any); ok {
					for _, raw := range blks {
						bm, _ := raw.(map[string]any)
						if bm == nil {
							continue
						}
						bk, _ := bm["kind"].(string)
						if bk == "group_table" {
							if rows, ok := bm["rows"].([]any); ok {
								groupCount += len(rows)
							}
						} else if bk == "timer" {
							if st, _ := bm["status"].(string); st == "running" || st == "paused" {
								tid, _ := bm["id"].(string)
								tt, _ := bm["title"].(string)
								tmode, _ := bm["mode"].(string)
								sa, _ := bm["started_at"].(string)
								el := intFromAny(bm["elapsed_sec"])
								pl := durationSecondsFromMap(bm["duration"])
								timers = append(timers, VaultTimerEntry{
									RelPath:    f.RelPath,
									DocTitle:   title,
									StepID:     m.ID,
									TimerID:    tid,
									Title:      firstNonEmptyStr(tt, "计时器"),
									Status:     st,
									Mode:       firstNonEmptyStr(tmode, "countdown"),
									PlannedSec: pl,
									ElapsedSec: el,
									StartedAt:  sa,
								})
							}
						}
					}
				}
			case "material":
				if pc, _ := m.Data["pantry_code"].(string); pc != "" {
					addPantry(pc)
				}
			case "group":
				groupCount++
			}
		}
		for _, rr := range asterdoc.ResolveRelations(parsed) {
			for _, tc := range rr.Contexts {
				relations = append(relations, IndexedRelation{
					RelPath:   f.RelPath,
					Asset:     rr.Asset,
					ObjectID:  tc.ObjectID,
					SubTarget: tc.SubTarget,
					Timepoint: rr.Timepoint,
					FOV:       rr.FOV,
					Role:      rr.Role,
					Line:      rr.Line,
					Valid:     tc.Valid,
				})
			}
		}
	}

	rawLines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")
	lines := make([]string, 0, len(rawLines))
	for _, ln := range rawLines {
		tr := strings.TrimSpace(ln)
		if tr != "" {
			lines = append(lines, tr)
		}
	}

	return IndexedDoc{
		RelPath:      f.RelPath,
		Title:        title,
		Folder:       f.Folder,
		Date:         date,
		Status:       status,
		RepeatOf:     strings.TrimSpace(fm.RepeatOf),
		Tags:         append([]string(nil), fm.Tags...),
		IsTemplate:   f.IsTemplate,
		Steps:        steps,
		PantryCodes:  pantryCodes,
		WikiLinks:    ExtractWikiLinks(src),
		Relations:    relations,
		ActiveTimers: timers,
		GroupCount:   groupCount,
		Lines:        lines,
		ModTimeUnix:  mtime,
		Size:         f.Size,
	}
}

// SearchFullText searches titles, steps, metadata, and every line of non-template `.asterml` files.
func (idx *VaultIndex) SearchFullText(query string, limit int) []SearchHit {
	q := strings.ToLower(strings.TrimSpace(query))
	if idx == nil || q == "" {
		return nil
	}
	if limit <= 0 {
		limit = 50
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()

	var hits []SearchHit
	keys := make([]string, 0, len(idx.docs))
	for k := range idx.docs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		d := idx.docs[k]
		if d.IsTemplate {
			continue
		}
		if strings.Contains(strings.ToLower(d.Title+" "+d.Folder+" "+strings.Join(d.PantryCodes, " ")), q) {
			hits = append(hits, SearchHit{
				RelPath: d.RelPath,
				Title:   d.Title,
				Folder:  d.Folder,
				LineNum: 0,
				Snippet: fmt.Sprintf("%s · %s", firstNonEmptyStr(d.Folder, "根目录"), firstNonEmptyStr(d.Date, "未排期")),
				Date:    d.Date,
				Status:  d.Status,
			})
			if len(hits) >= limit {
				return hits
			}
			continue
		}
		for li, ln := range d.Lines {
			if strings.Contains(strings.ToLower(ln), q) {
				snip := ln
				if len([]rune(snip)) > 110 {
					snip = string([]rune(snip)[:110]) + "…"
				}
				hits = append(hits, SearchHit{
					RelPath: d.RelPath,
					Title:   d.Title,
					Folder:  d.Folder,
					LineNum: li + 1,
					Snippet: snip,
					Date:    d.Date,
					Status:  d.Status,
				})
				break
			}
		}
		if len(hits) >= limit {
			return hits
		}
	}
	return hits
}

// BacklinksFor returns all experiments referencing targetRel via [[wikilinks]] or `repeat_of`.
func (idx *VaultIndex) BacklinksFor(targetRel string) []LinkRef {
	if idx == nil || strings.TrimSpace(targetRel) == "" {
		return nil
	}
	norm := strings.TrimSuffix(filepath.ToSlash(targetRel), ".asterml")
	base := strings.TrimSuffix(filepath.Base(targetRel), filepath.Ext(targetRel))
	idx.mu.RLock()
	targetTitle := base
	if td, ok := idx.docs[targetRel]; ok && td.Title != "" {
		targetTitle = td.Title
	}
	var out []LinkRef
	for rel, d := range idx.docs {
		if rel == targetRel {
			continue
		}
		if d.RepeatOf != "" && (d.RepeatOf == targetRel || d.RepeatOf == norm || strings.EqualFold(d.RepeatOf, base) || strings.EqualFold(d.RepeatOf, targetTitle)) {
			out = append(out, LinkRef{SourceRel: rel, SourceTitle: d.Title, SourceDate: d.Date, Kind: "repeat_of"})
			continue
		}
		for _, wl := range d.WikiLinks {
			wNorm := strings.TrimSuffix(filepath.ToSlash(wl), ".asterml")
			if wNorm == norm || strings.EqualFold(wNorm, base) || strings.EqualFold(wNorm, targetTitle) {
				out = append(out, LinkRef{SourceRel: rel, SourceTitle: d.Title, SourceDate: d.Date, Kind: "wikilink"})
				break
			}
		}
	}
	idx.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].SourceTitle < out[j].SourceTitle })
	return out
}

// RelationsFor returns cached bidirectional relations targeting (objectID, subTarget).
// When relPath is empty, searches across all Vault documents.
func (idx *VaultIndex) RelationsFor(relPath, objectID, subTarget string) []IndexedRelation {
	if idx == nil || strings.TrimSpace(objectID) == "" {
		return nil
	}
	objectID = strings.TrimSpace(objectID)
	subTarget = strings.ToUpper(strings.TrimSpace(subTarget))
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	var out []IndexedRelation
	for p, d := range idx.docs {
		if relPath != "" && p != relPath {
			continue
		}
		for _, r := range d.Relations {
			if r.ObjectID != objectID {
				continue
			}
			if subTarget != "" && !strings.EqualFold(r.SubTarget, subTarget) {
				continue
			}
			out = append(out, r)
		}
	}
	return out
}

// ActiveTimers aggregates running/paused timers across all Vault `.asterml` files.
func (idx *VaultIndex) ActiveTimers() []VaultTimerEntry {
	if idx == nil {
		return nil
	}
	idx.mu.RLock()
	var out []VaultTimerEntry
	for _, d := range idx.docs {
		if d.IsTemplate {
			continue
		}
		out = append(out, d.ActiveTimers...)
	}
	idx.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status == "running"
		}
		return out[i].DocTitle < out[j].DocTitle
	})
	return out
}

// QueryBases executes a structured cross-experiment query over indexed metadata (LabBases foundation).
func (idx *VaultIndex) QueryBases(q BasesQuery) []BasesRow {
	if idx == nil {
		return nil
	}
	kw := strings.ToLower(strings.TrimSpace(q.Keyword))
	idx.mu.RLock()
	var rows []BasesRow
	for _, d := range idx.docs {
		if d.IsTemplate {
			continue
		}
		if q.FolderPrefix != "" && !strings.HasPrefix(d.Folder, q.FolderPrefix) {
			continue
		}
		if q.Status != "" && d.Status != q.Status {
			continue
		}
		if q.Tag != "" {
			hasTag := false
			for _, tg := range d.Tags {
				if strings.EqualFold(tg, q.Tag) {
					hasTag = true
					break
				}
			}
			if !hasTag {
				continue
			}
		}
		if kw != "" && !strings.Contains(strings.ToLower(d.Title+" "+d.Folder+" "+strings.Join(d.Steps, " ")), kw) {
			continue
		}
		rows = append(rows, BasesRow{
			RelPath:    d.RelPath,
			Title:      d.Title,
			Folder:     d.Folder,
			Date:       d.Date,
			Status:     d.Status,
			GroupCount: d.GroupCount,
			StepCount:  len(d.Steps),
			Tags:       append([]string(nil), d.Tags...),
			RepeatOf:   d.RepeatOf,
		})
	}
	idx.mu.RUnlock()
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Date != rows[j].Date {
			return rows[i].Date > rows[j].Date
		}
		return rows[i].Title < rows[j].Title
	})
	return rows
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func intFromAny(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int64:
		return int(n)
	case float64:
		return int(n)
	default:
		return 0
	}
}

func durationSecondsFromMap(raw any) int {
	m, _ := raw.(map[string]any)
	if m == nil {
		return 0
	}
	var v float64
	switch n := m["value"].(type) {
	case int:
		v = float64(n)
	case float64:
		v = n
	}
	u, _ := m["unit"].(string)
	switch strings.TrimSpace(u) {
	case "h":
		return int(v * 3600)
	case "min":
		return int(v * 60)
	case "d":
		return int(v * 86400)
	default:
		return int(v)
	}
}
