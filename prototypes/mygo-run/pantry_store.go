package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/tools"
	"asterlab-mygo-prototype/modules/runs"
)

const (
	pantryNearExpiryWindowDays uint32 = 30
	pantryJSONLFileName               = "pantry.jsonl"
	pantryJSONLSchemaVersion          = 1
)

type localPantryDisk struct {
	NextSeq int               `json:"nextSeq"`
	Items   []runs.PantryItem `json:"items"`
}

// pantryStore holds the unified Vault-local Pantry state persisted in a single
// canonical, line-oriented `<Vault>/.asterlab/pantry.jsonl` file.
type pantryStore struct {
	Items []runs.PantryItem
	Meta  map[string]tools.PantryItemMeta
	Boxes []tools.PantryBox
}

type pantrySchemaRecord struct {
	Type    string `json:"type"`
	Version int    `json:"version"`
}

type pantryBoxRecord struct {
	Type     string              `json:"type"`
	ID       string              `json:"id"`
	Name     string              `json:"name"`
	Kind     tools.PantryBoxKind `json:"kind"`
	ZoneKey  string              `json:"zoneKey"`
	ShelfPos string              `json:"shelfPos,omitempty"`
	Rows     int                 `json:"rows"`
	Cols     int                 `json:"cols"`
	Notes    string              `json:"notes,omitempty"`
}

// pantryReagentRecord stores only canonical persisted fields (never volatile
// date-derived fields like daysRemaining/status) so daily clock ticks do not
// cause spurious Git or cloud-sync diffs.
type pantryReagentRecord struct {
	Type                string                `json:"type"`
	ID                  string                `json:"id"`
	LabelCode           string                `json:"labelCode,omitempty"`
	ProjectID           string                `json:"projectId,omitempty"`
	Name                string                `json:"name"`
	Location            string                `json:"location,omitempty"`
	PurchasedOn         string                `json:"purchasedOn,omitempty"`
	OpenedOn            string                `json:"openedOn,omitempty"`
	ShelfLifeDays       *uint32               `json:"shelfLifeDays,omitempty"`
	OpenedShelfLifeDays *uint32               `json:"openedShelfLifeDays,omitempty"`
	Exhausted           bool                  `json:"exhausted,omitempty"`
	Revision            int64                 `json:"revision"`
	CreatedAt           string                `json:"createdAt,omitempty"`
	UpdatedAt           string                `json:"updatedAt,omitempty"`
	Meta                *tools.PantryItemMeta `json:"meta,omitempty"`
}

type pantryMetaOnlyRecord struct {
	Type string               `json:"type"`
	ID   string               `json:"id"`
	Meta tools.PantryItemMeta `json:"meta"`
}

type pantryJSONLRawLine struct {
	Type                string                `json:"type"`
	Version             int                   `json:"version"`
	ID                  string                `json:"id"`
	LabelCode           string                `json:"labelCode"`
	ProjectID           string                `json:"projectId"`
	Name                string                `json:"name"`
	Location            string                `json:"location"`
	PurchasedOn         string                `json:"purchasedOn"`
	OpenedOn            string                `json:"openedOn"`
	ShelfLifeDays       *uint32               `json:"shelfLifeDays"`
	OpenedShelfLifeDays *uint32               `json:"openedShelfLifeDays"`
	Exhausted           bool                  `json:"exhausted"`
	Revision            int64                 `json:"revision"`
	CreatedAt           string                `json:"createdAt"`
	UpdatedAt           string                `json:"updatedAt"`
	Meta                *tools.PantryItemMeta `json:"meta"`

	// Box fields
	Kind     tools.PantryBoxKind `json:"kind"`
	ZoneKey  string              `json:"zoneKey"`
	ShelfPos string              `json:"shelfPos"`
	Rows     int                 `json:"rows"`
	Cols     int                 `json:"cols"`
	Notes    string              `json:"notes"`
}

// pantryDataDir returns the storage directory for Pantry data and manuals.
// When a local Vault is active, Pantry data lives strictly inside `<Vault>/.asterlab/`
// so each Vault is self-contained, portable, and isolated from other Vaults or legacy global data.
func (a *app) pantryDataDir() string {
	if a.vault != nil && strings.TrimSpace(a.vault.Root) != "" {
		return filepath.Join(a.vault.Root, ".asterlab")
	}
	return workspaceDataDir()
}

func (a *app) pantryStorePath() string {
	dir := a.pantryDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, pantryJSONLFileName)
}

func isEmptyPantryMeta(m tools.PantryItemMeta) bool {
	return m.VesselID == "" &&
		m.CustomStickerRel == "" &&
		m.CatalogNo == "" &&
		m.LotNo == "" &&
		m.VolumeNote == "" &&
		m.ProductURL == "" &&
		m.Notes == "" &&
		len(m.Manuals) == 0 &&
		m.BoxID == "" &&
		m.BoxSlot == "" &&
		len(m.BoxSlots) == 0
}

func parsePantryJSONL(data []byte) (pantryStore, error) {
	store := pantryStore{
		Items: []runs.PantryItem{},
		Meta:  map[string]tools.PantryItemMeta{},
		Boxes: []tools.PantryBox{},
	}
	itemsByID := map[string]runs.PantryItem{}
	boxesByID := map[string]tools.PantryBox{}

	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		var rec pantryJSONLRawLine
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			return pantryStore{}, fmt.Errorf("pantry.jsonl 第 %d 行解析失败：%w", lineNo, err)
		}
		kind := strings.ToLower(strings.TrimSpace(rec.Type))
		if kind == "" {
			switch {
			case rec.Rows > 0 && rec.Cols > 0:
				kind = "box"
			case strings.TrimSpace(rec.Name) != "":
				kind = "reagent"
			case rec.ID != "" && rec.Meta != nil:
				kind = "meta"
			}
		}
		switch kind {
		case "schema":
			continue
		case "box":
			id := strings.TrimSpace(rec.ID)
			if id == "" {
				id = fmt.Sprintf("box-line-%d", lineNo)
			}
			boxesByID[id] = tools.PantryBox{
				ID:       id,
				Name:     strings.TrimSpace(rec.Name),
				Kind:     rec.Kind,
				ZoneKey:  strings.TrimSpace(rec.ZoneKey),
				ShelfPos: strings.TrimSpace(rec.ShelfPos),
				Rows:     rec.Rows,
				Cols:     rec.Cols,
				Notes:    strings.TrimSpace(rec.Notes),
			}
		case "reagent", "item":
			id := strings.TrimSpace(rec.ID)
			if id == "" {
				id = fmt.Sprintf("pantry-line-%d", lineNo)
			}
			rev := rec.Revision
			if rev <= 0 {
				rev = 1
			}
			itemsByID[id] = runs.PantryItem{
				ID:                  id,
				LabelCode:           strings.TrimSpace(rec.LabelCode),
				ProjectID:           strings.TrimSpace(rec.ProjectID),
				Name:                strings.TrimSpace(rec.Name),
				Location:            strings.TrimSpace(rec.Location),
				PurchasedOn:         strings.TrimSpace(rec.PurchasedOn),
				OpenedOn:            strings.TrimSpace(rec.OpenedOn),
				ShelfLifeDays:       rec.ShelfLifeDays,
				OpenedShelfLifeDays: rec.OpenedShelfLifeDays,
				Exhausted:           rec.Exhausted,
				Revision:            rev,
				CreatedAt:           strings.TrimSpace(rec.CreatedAt),
				UpdatedAt:           strings.TrimSpace(rec.UpdatedAt),
			}
			if rec.Meta != nil && !isEmptyPantryMeta(*rec.Meta) {
				store.Meta[id] = *rec.Meta
			}
		case "meta":
			id := strings.TrimSpace(rec.ID)
			if id != "" && rec.Meta != nil && !isEmptyPantryMeta(*rec.Meta) {
				store.Meta[id] = *rec.Meta
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return pantryStore{}, err
	}

	boxIDs := make([]string, 0, len(boxesByID))
	for id := range boxesByID {
		boxIDs = append(boxIDs, id)
	}
	sort.Strings(boxIDs)
	for _, id := range boxIDs {
		store.Boxes = append(store.Boxes, boxesByID[id])
	}

	itemIDs := make([]string, 0, len(itemsByID))
	for id := range itemsByID {
		itemIDs = append(itemIDs, id)
	}
	sort.Strings(itemIDs)
	for _, id := range itemIDs {
		store.Items = append(store.Items, itemsByID[id])
	}
	return store, nil
}

func marshalPantryJSONL(store pantryStore) ([]byte, error) {
	var buf bytes.Buffer
	writeLine := func(v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
		return nil
	}

	if err := writeLine(pantrySchemaRecord{Type: "schema", Version: pantryJSONLSchemaVersion}); err != nil {
		return nil, err
	}

	boxes := append([]tools.PantryBox(nil), store.Boxes...)
	sort.Slice(boxes, func(i, j int) bool { return boxes[i].ID < boxes[j].ID })
	for _, bx := range boxes {
		if err := writeLine(pantryBoxRecord{
			Type:     "box",
			ID:       bx.ID,
			Name:     bx.Name,
			Kind:     bx.Kind,
			ZoneKey:  bx.ZoneKey,
			ShelfPos: bx.ShelfPos,
			Rows:     bx.Rows,
			Cols:     bx.Cols,
			Notes:    bx.Notes,
		}); err != nil {
			return nil, err
		}
	}

	items := append([]runs.PantryItem(nil), store.Items...)
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	emittedMeta := map[string]bool{}
	for _, it := range items {
		var metaPtr *tools.PantryItemMeta
		if store.Meta != nil {
			if m, ok := store.Meta[it.ID]; ok && !isEmptyPantryMeta(m) {
				mCopy := m
				metaPtr = &mCopy
				emittedMeta[it.ID] = true
			}
		}
		if err := writeLine(pantryReagentRecord{
			Type:                "reagent",
			ID:                  it.ID,
			LabelCode:           it.LabelCode,
			ProjectID:           it.ProjectID,
			Name:                it.Name,
			Location:            it.Location,
			PurchasedOn:         it.PurchasedOn,
			OpenedOn:            it.OpenedOn,
			ShelfLifeDays:       it.ShelfLifeDays,
			OpenedShelfLifeDays: it.OpenedShelfLifeDays,
			Exhausted:           it.Exhausted,
			Revision:            it.Revision,
			CreatedAt:           it.CreatedAt,
			UpdatedAt:           it.UpdatedAt,
			Meta:                metaPtr,
		}); err != nil {
			return nil, err
		}
	}

	if len(store.Meta) > 0 {
		orphanIDs := make([]string, 0)
		for id, m := range store.Meta {
			if !emittedMeta[id] && !isEmptyPantryMeta(m) {
				orphanIDs = append(orphanIDs, id)
			}
		}
		sort.Strings(orphanIDs)
		for _, id := range orphanIDs {
			if err := writeLine(pantryMetaOnlyRecord{
				Type: "meta",
				ID:   id,
				Meta: store.Meta[id],
			}); err != nil {
				return nil, err
			}
		}
	}

	return buf.Bytes(), nil
}

func loadLegacyPantryFiles(dir string) (pantryStore, bool, error) {
	store := pantryStore{
		Items: []runs.PantryItem{},
		Meta:  map[string]tools.PantryItemMeta{},
		Boxes: []tools.PantryBox{},
	}
	foundAny := false

	if raw, err := os.ReadFile(filepath.Join(dir, "pantry-ledger.json")); err == nil {
		foundAny = true
		var disk localPantryDisk
		if err := json.Unmarshal(raw, &disk); err != nil {
			return pantryStore{}, true, err
		}
		if disk.Items != nil {
			store.Items = disk.Items
		}
	} else if !os.IsNotExist(err) {
		return pantryStore{}, false, err
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "pantry-meta.json")); err == nil {
		foundAny = true
		var meta map[string]tools.PantryItemMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return pantryStore{}, true, err
		}
		for k, v := range meta {
			if !isEmptyPantryMeta(v) {
				store.Meta[k] = v
			}
		}
	} else if !os.IsNotExist(err) {
		return pantryStore{}, false, err
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "pantry-boxes.json")); err == nil {
		foundAny = true
		var boxes []tools.PantryBox
		if err := json.Unmarshal(raw, &boxes); err != nil {
			return pantryStore{}, true, err
		}
		if boxes != nil {
			store.Boxes = boxes
		}
	} else if !os.IsNotExist(err) {
		return pantryStore{}, false, err
	}

	return store, foundAny, nil
}

func loadPantryStore(dir string) (pantryStore, error) {
	empty := pantryStore{
		Items: []runs.PantryItem{},
		Meta:  map[string]tools.PantryItemMeta{},
		Boxes: []tools.PantryBox{},
	}
	if strings.TrimSpace(dir) == "" {
		return empty, nil
	}
	jsonlPath := filepath.Join(dir, pantryJSONLFileName)
	raw, err := os.ReadFile(jsonlPath)
	if err == nil {
		return parsePantryJSONL(raw)
	}
	if !os.IsNotExist(err) {
		return empty, err
	}
	// Seamless one-time migration if legacy 3-file JSON exists in this directory.
	legacy, found, lErr := loadLegacyPantryFiles(dir)
	if lErr != nil {
		return empty, lErr
	}
	if found {
		_ = savePantryStore(dir, legacy)
		return legacy, nil
	}
	return empty, nil
}

func savePantryStore(dir string, store pantryStore) error {
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := marshalPantryJSONL(store)
	if err != nil {
		return err
	}
	return asterdoc.WriteFileAtomic(filepath.Join(dir, pantryJSONLFileName), raw)
}

func nextPantrySeq(items []runs.PantryItem) int {
	maxSeq := 0
	for _, it := range items {
		code := strings.ToUpper(strings.TrimSpace(it.LabelCode))
		if strings.HasPrefix(code, "PN-") {
			if n, err := strconv.Atoi(strings.TrimPrefix(code, "PN-")); err == nil && n > maxSeq {
				maxSeq = n
			}
		}
	}
	if maxSeq < len(items) {
		maxSeq = len(items)
	}
	return maxSeq + 1
}

func computePantryDerived(it runs.PantryItem, today time.Time) runs.PantryItem {
	it.ExpiresOn = ""
	it.DaysRemaining = nil
	it.ExpiryBasis = ""

	var anchor string
	var shelf uint32
	if strings.TrimSpace(it.OpenedOn) != "" && it.OpenedShelfLifeDays != nil && *it.OpenedShelfLifeDays > 0 {
		anchor = strings.TrimSpace(it.OpenedOn)
		shelf = *it.OpenedShelfLifeDays
		it.ExpiryBasis = "opened_shelf_life"
	} else if it.ShelfLifeDays != nil && *it.ShelfLifeDays > 0 {
		anchor = firstNonEmpty(strings.TrimSpace(it.PurchasedOn), strings.TrimSpace(it.OpenedOn))
		shelf = *it.ShelfLifeDays
		if anchor != "" {
			it.ExpiryBasis = "shelf_life"
		}
	}

	if anchor != "" && shelf > 0 {
		if baseDate, err := time.Parse("2006-01-02", anchor); err == nil {
			expDate := baseDate.AddDate(0, 0, int(shelf))
			it.ExpiresOn = expDate.Format("2006-01-02")
			rem := int64(expDate.Sub(today).Hours() / 24)
			it.DaysRemaining = &rem
		}
	}

	switch {
	case it.Exhausted:
		it.Status = "exhausted"
	case it.DaysRemaining != nil && *it.DaysRemaining < 0:
		it.Status = "expired"
	case it.DaysRemaining != nil && *it.DaysRemaining <= int64(pantryNearExpiryWindowDays):
		it.Status = "near_expiry"
	case strings.TrimSpace(it.OpenedOn) != "":
		it.Status = "opened"
	default:
		it.Status = "unopened"
	}
	return it
}

func (a *app) localListPantry() (runs.PantryLedger, error) {
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	store, err := loadPantryStore(a.pantryDataDir())
	if err != nil {
		return runs.PantryLedger{}, err
	}
	items := make([]runs.PantryItem, 0, len(store.Items))
	for _, it := range store.Items {
		items = append(items, computePantryDerived(it, today))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].UpdatedAt != items[j].UpdatedAt {
			return items[i].UpdatedAt > items[j].UpdatedAt
		}
		return items[i].Name < items[j].Name
	})
	return runs.PantryLedger{
		AsOf:           today.Format("2006-01-02"),
		NearExpiryDays: pantryNearExpiryWindowDays,
		Items:          items,
	}, nil
}

func (a *app) localUpsertPantry(input runs.PantryUpsert) (runs.PantryItem, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return runs.PantryItem{}, fmt.Errorf("请输入试剂或耗材名称")
	}
	dir := a.pantryDataDir()
	store, err := loadPantryStore(dir)
	if err != nil {
		return runs.PantryItem{}, err
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	nowRFC := now.UTC().Format(time.RFC3339)

	idx := -1
	if strings.TrimSpace(input.ID) != "" {
		for i := range store.Items {
			if store.Items[i].ID == input.ID {
				idx = i
				break
			}
		}
		if idx < 0 {
			return runs.PantryItem{}, fmt.Errorf("未找到试剂条目：%s", input.ID)
		}
		if input.ExpectedRevision != nil && store.Items[idx].Revision != *input.ExpectedRevision {
			return runs.PantryItem{}, fmt.Errorf("CONFLICT: 试剂条目已被其他操作更新，请刷新后重试")
		}
	}

	var saved runs.PantryItem
	if idx >= 0 {
		prev := store.Items[idx]
		saved = runs.PantryItem{
			ID:                  prev.ID,
			LabelCode:           prev.LabelCode,
			ProjectID:           firstNonEmpty(input.ProjectID, prev.ProjectID),
			Name:                name,
			Location:            strings.TrimSpace(input.Location),
			PurchasedOn:         strings.TrimSpace(input.PurchasedOn),
			OpenedOn:            strings.TrimSpace(input.OpenedOn),
			ShelfLifeDays:       input.ShelfLifeDays,
			OpenedShelfLifeDays: input.OpenedShelfLifeDays,
			Exhausted:           input.Exhausted,
			Revision:            prev.Revision + 1,
			CreatedAt:           prev.CreatedAt,
			UpdatedAt:           nowRFC,
		}
		saved = computePantryDerived(saved, today)
		store.Items[idx] = saved
	} else {
		seq := nextPantrySeq(store.Items)
		saved = runs.PantryItem{
			ID:                  fmt.Sprintf("pantry-%d-%d", now.UnixMilli(), seq),
			LabelCode:           fmt.Sprintf("PN-%03d", seq),
			ProjectID:           input.ProjectID,
			Name:                name,
			Location:            strings.TrimSpace(input.Location),
			PurchasedOn:         strings.TrimSpace(input.PurchasedOn),
			OpenedOn:            strings.TrimSpace(input.OpenedOn),
			ShelfLifeDays:       input.ShelfLifeDays,
			OpenedShelfLifeDays: input.OpenedShelfLifeDays,
			Exhausted:           input.Exhausted,
			Revision:            1,
			CreatedAt:           nowRFC,
			UpdatedAt:           nowRFC,
		}
		saved = computePantryDerived(saved, today)
		store.Items = append(store.Items, saved)
	}
	if err := savePantryStore(dir, store); err != nil {
		return runs.PantryItem{}, err
	}
	return saved, nil
}

func (a *app) localDeletePantry(input runs.PantryDelete) error {
	if strings.TrimSpace(input.ID) == "" {
		return fmt.Errorf("缺少待删除试剂 ID")
	}
	dir := a.pantryDataDir()
	store, err := loadPantryStore(dir)
	if err != nil {
		return err
	}
	idx := -1
	for i := range store.Items {
		if store.Items[i].ID == input.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("未找到试剂条目：%s", input.ID)
	}
	if input.ExpectedRevision != nil && store.Items[idx].Revision != *input.ExpectedRevision {
		return fmt.Errorf("CONFLICT: 试剂条目已被修改，请刷新后重试")
	}
	store.Items = append(store.Items[:idx], store.Items[idx+1:]...)
	delete(store.Meta, input.ID)
	return savePantryStore(dir, store)
}

func (a *app) localExportPantryLabels() (runs.PantryLabels, error) {
	ledger, err := a.localListPantry()
	if err != nil {
		return runs.PantryLabels{}, err
	}
	labels := make([]runs.PantryLabel, 0, len(ledger.Items))
	for _, it := range ledger.Items {
		labels = append(labels, runs.PantryLabel{
			LabelCode:     it.LabelCode,
			PantryItemID:  it.ID,
			Name:          it.Name,
			Location:      it.Location,
			Status:        it.Status,
			ExpiresOn:     it.ExpiresOn,
			DaysRemaining: it.DaysRemaining,
			Revision:      it.Revision,
		})
	}
	return runs.PantryLabels{
		SchemaVersion:  1,
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		AsOf:           ledger.AsOf,
		NearExpiryDays: ledger.NearExpiryDays,
		Labels:         labels,
	}, nil
}
