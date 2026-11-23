package tools

import (
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/uikit/labsticker"
	"asterlab-mygo-prototype/modules/runs"
)

// PantryManual stores a locally managed reagent datasheet, COA, or SDS attachment.
type PantryManual struct {
	ID        string `json:"id"`
	FileName  string `json:"fileName"`
	StoredRel string `json:"storedRel"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	AddedOn   string `json:"addedOn,omitempty"`
}

// PantryItemMeta persists rich UI & documentation metadata per PantryItem.ID
// alongside Core's authoritative PantryLedger without changing native FFI structs.
type PantryItemMeta struct {
	VesselID         labsticker.VesselID `json:"vesselId,omitempty"`
	CustomStickerRel string              `json:"customStickerRel,omitempty"`
	CatalogNo        string              `json:"catalogNo,omitempty"`
	LotNo            string              `json:"lotNo,omitempty"`
	VolumeNote       string              `json:"volumeNote,omitempty"`
	ProductURL       string              `json:"productUrl,omitempty"`
	Notes            string              `json:"notes,omitempty"`
	Manuals          []PantryManual      `json:"manuals,omitempty"`
	BoxID            string              `json:"boxId,omitempty"`
	BoxSlot          string              `json:"boxSlot,omitempty"`
	BoxSlots         []string            `json:"boxSlots,omitempty"`
}

type pantryImageCacheEntry struct {
	img image.Image
}

// EnsurePantryMetaLoaded lazily loads extended pantry metadata via Callbacks.LoadPantryMeta.
func (w *Workspace) EnsurePantryMetaLoaded() {
	if w.PantryMeta == nil {
		w.PantryMeta = map[string]PantryItemMeta{}
	}
	if w.pantryMetaLoaded {
		return
	}
	w.pantryMetaLoaded = true
	if w.Callbacks.LoadPantryMeta == nil {
		return
	}
	if meta, err := w.Callbacks.LoadPantryMeta(); err == nil && meta != nil {
		w.PantryMeta = meta
	}
}

func (w *Workspace) savePantryMeta() error {
	if w.PantryMeta == nil {
		w.PantryMeta = map[string]PantryItemMeta{}
	}
	if w.Callbacks.SavePantryMeta != nil {
		return w.Callbacks.SavePantryMeta(w.PantryMeta)
	}
	return nil
}

// MetaForPantryItem returns the metadata for item, auto-suggesting an appropriate BioRender
// vessel silhouette from its name and storage location when unset.
func (w *Workspace) MetaForPantryItem(item runs.PantryItem) PantryItemMeta {
	w.EnsurePantryMetaLoaded()
	m := w.PantryMeta[item.ID]
	if m.VesselID == "" {
		m.VesselID = labsticker.SuggestVesselID(item.Name, item.Location)
	}
	return m
}

func clonePantryMeta(m PantryItemMeta) PantryItemMeta {
	out := m
	if len(m.Manuals) > 0 {
		out.Manuals = append([]PantryManual(nil), m.Manuals...)
	}
	if len(m.BoxSlots) > 0 {
		out.BoxSlots = append([]string(nil), m.BoxSlots...)
	}
	return out
}

// OpenPantryDetail opens the reagent detail dialog pre-populated for an existing reagent.
func (w *Workspace) OpenPantryDetail(item runs.PantryItem) {
	w.EditPantry(item)
	w.pantryDraftMeta = clonePantryMeta(w.MetaForPantryItem(item))
	if w.pantryDraftMeta.BoxSlot != "" {
		w.pantryDraftMeta.BoxSlot = NormalizeBoxSlot(w.pantryDraftMeta.BoxSlot)
	}
	switch {
	case item.Exhausted || item.Status == "exhausted":
		w.pantryStatusIdx = 2
	case item.OpenedOn != "" || item.Status == "opened" || item.Status == "near_expiry" || item.Status == "expired":
		w.pantryStatusIdx = 1
	default:
		w.pantryStatusIdx = 0
	}
	w.pantryPickerOpen = false
	w.pantryConfirmDel = false
	w.pantryDialogErr = ""
	w.PantryDialogOpen = true
}

// OpenNewPantryDialog opens the reagent detail dialog in creation mode, optionally pre-filling location.
func (w *Workspace) OpenNewPantryDialog(presetLocation string) {
	w.clearPantryForm()
	w.PantryLocation = strings.TrimSpace(presetLocation)
	w.pantryDraftMeta = PantryItemMeta{
		VesselID: labsticker.SuggestVesselID("", w.PantryLocation),
	}
	w.pantryStatusIdx = 0
	w.pantryPickerOpen = true
	w.pantryConfirmDel = false
	w.pantryDialogErr = ""
	w.PantryDialogOpen = true
}

// SelectDraftVessel sets the BioRender vessel sticker in the currently open reagent dialog.
func (w *Workspace) SelectDraftVessel(v labsticker.VesselID) {
	w.pantryDraftMeta.VesselID = v
	w.pantryDraftMeta.CustomStickerRel = ""
}

// SetDraftCustomSticker imports an external PNG/JPG and binds it as the reagent's die-cut sticker.
func (w *Workspace) SetDraftCustomSticker(srcPath string) error {
	srcPath = strings.TrimSpace(srcPath)
	if srcPath == "" {
		return fmt.Errorf("未选择图片文件")
	}
	ext := strings.ToLower(filepath.Ext(srcPath))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return fmt.Errorf("贴纸图片仅支持 PNG 或 JPG 格式")
	}
	itemKey := w.PantryID
	if itemKey == "" {
		itemKey = fmt.Sprintf("draft-%d", time.Now().Unix()%100000)
	}
	storedRel := srcPath
	if w.Callbacks.ImportPantryFile != nil {
		rel, _, _, err := w.Callbacks.ImportPantryFile(itemKey, srcPath)
		if err != nil {
			return err
		}
		storedRel = rel
	}
	w.pantryDraftMeta.CustomStickerRel = storedRel
	if w.pantryCustomImages != nil {
		delete(w.pantryCustomImages, storedRel)
	}
	w.pantryDialogErr = ""
	return nil
}

// AttachManualFiles imports one or more local reagent manuals/datasheets (PDF, image, doc)
// into the current dialog draft and persists metadata immediately when editing an existing item.
func (w *Workspace) AttachManualFiles(paths []string) error {
	w.EnsurePantryMetaLoaded()
	itemKey := w.PantryID
	if itemKey == "" {
		itemKey = fmt.Sprintf("draft-%d", time.Now().Unix()%100000)
	}
	added := 0
	for _, src := range paths {
		src = strings.TrimSpace(src)
		if src == "" {
			continue
		}
		storedRel := src
		name := filepath.Base(src)
		var sizeBytes int64
		if st, err := os.Stat(src); err == nil {
			sizeBytes = st.Size()
		}
		if w.Callbacks.ImportPantryFile != nil {
			rel, importedName, sz, err := w.Callbacks.ImportPantryFile(itemKey, src)
			if err != nil {
				w.pantryDialogErr = fmt.Sprintf("导入说明书 %q 失败：%v", name, err)
				return err
			}
			storedRel = rel
			if importedName != "" {
				name = importedName
			}
			sizeBytes = sz
		}
		manual := PantryManual{
			ID:        fmt.Sprintf("man-%d-%d", time.Now().UnixNano()%1000000, len(w.pantryDraftMeta.Manuals)+1),
			FileName:  name,
			StoredRel: storedRel,
			SizeBytes: sizeBytes,
			AddedOn:   time.Now().Format("2006-01-02"),
		}
		w.pantryDraftMeta.Manuals = append(w.pantryDraftMeta.Manuals, manual)
		added++
	}
	if added > 0 && w.PantryID != "" {
		w.PantryMeta[w.PantryID] = clonePantryMeta(w.pantryDraftMeta)
		if err := w.savePantryMeta(); err != nil {
			w.pantryDialogErr = err.Error()
			return err
		}
	}
	w.pantryDialogErr = ""
	return nil
}

// RemoveManualFile removes a manual attachment from the current reagent draft.
func (w *Workspace) RemoveManualFile(manualID string) {
	kept := make([]PantryManual, 0, len(w.pantryDraftMeta.Manuals))
	for _, m := range w.pantryDraftMeta.Manuals {
		if m.ID != manualID {
			kept = append(kept, m)
		}
	}
	w.pantryDraftMeta.Manuals = kept
	if w.PantryID != "" {
		w.EnsurePantryMetaLoaded()
		w.PantryMeta[w.PantryID] = clonePantryMeta(w.pantryDraftMeta)
		_ = w.savePantryMeta()
	}
}

// OpenManualFile opens an attached reagent datasheet/manual in the system viewer.
func (w *Workspace) OpenManualFile(m PantryManual) error {
	abs := m.StoredRel
	if w.Callbacks.ResolvePantryFile != nil {
		if resolved := w.Callbacks.ResolvePantryFile(m.StoredRel); resolved != "" {
			abs = resolved
		}
	}
	if w.Callbacks.OpenLocalFile != nil {
		if err := w.Callbacks.OpenLocalFile(abs); err != nil {
			w.pantryDialogErr = "打开说明书失败：" + err.Error()
			return err
		}
	}
	w.pantryDialogErr = ""
	return nil
}

// ValidateProductURL normalizes and validates a user-entered reagent catalog/vendor URL.
func ValidateProductURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	candidate := trimmed
	if !strings.Contains(candidate, "://") && (strings.HasPrefix(candidate, "www.") || strings.Contains(candidate, ".")) && !strings.ContainsAny(candidate, " \t\n:") {
		candidate = "https://" + candidate
	}
	u, err := url.Parse(candidate)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("产品网址需以 http:// 或 https:// 开头（当前：%q）", trimmed)
	}
	return u.String(), nil
}

// ValidateAndOpenProductURL checks the URL scheme and opens it in the external browser.
func (w *Workspace) ValidateAndOpenProductURL(raw string) error {
	normalized, err := ValidateProductURL(raw)
	if err != nil {
		w.pantryDialogErr = err.Error()
		return err
	}
	if normalized == "" {
		w.pantryDialogErr = "请先填写产品主页或货号链接"
		return fmt.Errorf("%s", w.pantryDialogErr)
	}
	w.pantryDraftMeta.ProductURL = normalized
	if w.Callbacks.OpenExternalURL != nil {
		if err := w.Callbacks.OpenExternalURL(normalized); err != nil {
			w.pantryDialogErr = "打开产品网址失败：" + err.Error()
			return err
		}
	}
	w.pantryDialogErr = ""
	return nil
}

// SavePantryDetail validates and commits both Core PantryItem fields and extended metadata
// (vessel sticker, vendor URL, and attached manuals) from the detail dialog.
func (w *Workspace) SavePantryDetail() bool {
	w.pantryDialogErr = ""
	w.EnsurePantryMetaLoaded()

	name := strings.TrimSpace(w.PantryName)
	if name == "" {
		w.pantryDialogErr = "请输入试剂或耗材名称"
		return false
	}
	normalizedURL, err := ValidateProductURL(w.pantryDraftMeta.ProductURL)
	if err != nil {
		w.pantryDialogErr = err.Error()
		return false
	}
	w.pantryDraftMeta.ProductURL = normalizedURL

	var days *uint32
	if value := strings.TrimSpace(w.PantryShelfLifeDays); value != "" {
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil || n == 0 {
			w.pantryDialogErr = "保质期请输入正整数天数"
			return false
		}
		d := uint32(n)
		days = &d
	}

	switch w.pantryStatusIdx {
	case 0:
		w.PantryExhausted = false
	case 1:
		w.PantryExhausted = false
		if strings.TrimSpace(w.PantryOpenedOn) == "" {
			w.PantryOpenedOn = time.Now().Format("2006-01-02")
		}
	case 2:
		w.PantryExhausted = true
	}

	// Validate optional storage box & slot coordinate assignment before saving.
	w.EnsurePantryBoxesLoaded()
	locationOut := strings.TrimSpace(w.PantryLocation)
	meta := clonePantryMeta(w.pantryDraftMeta)
	if strings.TrimSpace(meta.BoxID) == "" {
		meta.BoxID = ""
		meta.BoxSlot = ""
		meta.BoxSlots = nil
	} else {
		box, ok := w.FindPantryBox(meta.BoxID)
		if !ok {
			w.pantryDialogErr = "所选收纳盒不存在，请重新选择"
			return false
		}
		normSlot := NormalizeBoxSlot(meta.BoxSlot)
		if normSlot == "" {
			normSlot = w.FirstEmptyBoxSlot(box)
		}
		if normSlot == "" {
			w.pantryDialogErr = fmt.Sprintf("收纳盒「%s」已满，请切换盒子或改放货架", box.Name)
			return false
		}
		if _, _, valid := ParseBoxSlot(normSlot, box.Rows, box.Cols); !valid {
			w.pantryDialogErr = fmt.Sprintf("孔位 %q 超出「%s」范围（最大 %c%d）", normSlot, box.Name, 'A'+box.Rows-1, box.Cols)
			return false
		}
		slotMap := w.BoxSlotMap(box)
		if existing, taken := slotMap[normSlot]; taken && existing.ID != w.PantryID {
			w.pantryDialogErr = fmt.Sprintf("孔位 %s 已被「%s」占用，请换其他空位", normSlot, existing.Name)
			return false
		}
		meta.BoxSlot = normSlot
		if len(meta.BoxSlots) > 1 {
			meta.BoxSlots[0] = normSlot
			meta.BoxSlots = ItemBoxSlots(meta)
			meta.BoxSlot = meta.BoxSlots[0]
		} else {
			meta.BoxSlots = nil
		}
		locationOut = FormatBoxLocation(box.ZoneKey, box.Name, FormatSlotList(ItemBoxSlots(meta)))
		w.PantryLocation = locationOut
	}

	if w.Callbacks.UpsertPantry == nil {
		w.pantryDialogErr = "储藏室保存回调未配置"
		return false
	}

	input := runs.PantryUpsert{
		ID:                  w.PantryID,
		Name:                name,
		Location:            locationOut,
		PurchasedOn:         strings.TrimSpace(w.PantryPurchasedOn),
		OpenedOn:            strings.TrimSpace(w.PantryOpenedOn),
		ShelfLifeDays:       days,
		OpenedShelfLifeDays: w.PantryOpenedShelfLifeDays,
		Exhausted:           w.PantryExhausted,
	}
	if w.PantryID != "" {
		for _, item := range w.Pantry.Items {
			if item.ID == w.PantryID {
				revision := item.Revision
				input.ExpectedRevision = &revision
				break
			}
		}
	}

	saved, err := w.Callbacks.UpsertPantry(input)
	if err != nil {
		w.pantryDialogErr = err.Error()
		return false
	}

	if meta.VesselID == "" {
		meta.VesselID = labsticker.SuggestVesselID(saved.Name, saved.Location)
	}
	meta.CatalogNo = strings.TrimSpace(meta.CatalogNo)
	meta.LotNo = strings.TrimSpace(meta.LotNo)
	meta.VolumeNote = strings.TrimSpace(meta.VolumeNote)
	meta.Notes = strings.TrimSpace(meta.Notes)
	w.PantryMeta[saved.ID] = meta
	if err := w.savePantryMeta(); err != nil {
		w.pantryDialogErr = "档案附加信息保存失败：" + err.Error()
		return false
	}

	if meta.BoxID != "" && w.activeBoxID == meta.BoxID {
		w.selectedBoxSlot = meta.BoxSlot
	}

	w.PantryDialogOpen = false
	w.pantryConfirmDel = false
	w.ReloadPantry()
	if w.Error == "" {
		w.Status = "已保存试剂档案：" + saved.Name
	}
	return true
}

func (w *Workspace) loadCustomStickerImage(rel string) image.Image {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return nil
	}
	if w.pantryCustomImages == nil {
		w.pantryCustomImages = map[string]pantryImageCacheEntry{}
	}
	if cached, ok := w.pantryCustomImages[rel]; ok {
		return cached.img
	}
	abs := rel
	if w.Callbacks.ResolvePantryFile != nil {
		if resolved := w.Callbacks.ResolvePantryFile(rel); resolved != "" {
			abs = resolved
		}
	}
	f, err := os.Open(abs)
	if err != nil {
		w.pantryCustomImages[rel] = pantryImageCacheEntry{}
		return nil
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	if err != nil {
		w.pantryCustomImages[rel] = pantryImageCacheEntry{}
		return nil
	}
	w.pantryCustomImages[rel] = pantryImageCacheEntry{img: img}
	return img
}

func pantryZoneCategory(location string) (key, title, subtitle string) {
	loc := strings.ToLower(strings.TrimSpace(location))
	switch {
	case strings.Contains(loc, "-80") || strings.Contains(loc, "超低温") || strings.Contains(loc, "液氮"):
		return "-80", "-80°C 超低温与细胞冻存架", "抗体原液 · 冻存盒与超低温样品管"
	case strings.Contains(loc, "-20") || strings.Contains(loc, "酶盒") || strings.Contains(loc, "冻存"):
		return "-20", "-20°C 酶与核酸冻存架", "分子生物学酶 · 预混液与引物母液"
	case strings.Contains(loc, "4°c") || strings.Contains(loc, "4℃") || strings.Contains(loc, "冷藏"):
		return "4C", "4°C 冷藏培养基与缓冲液架", "日常培养基 · 无菌缓冲液与前处理试剂"
	default:
		return "ambient", "常温与避光干燥试剂柜", "标准品 · 固定液与无水缓冲耗材"
	}
}

func pantryMatchesFilter(item runs.PantryItem, meta PantryItemMeta, query, zoneFilter string) bool {
	if zoneFilter != "" && zoneFilter != "all" {
		if zoneFilter == "near_expiry" {
			if item.Status != "near_expiry" && item.Status != "expired" {
				return false
			}
		} else {
			zk, _, _ := pantryZoneCategory(item.Location)
			if zk != zoneFilter {
				return false
			}
		}
	}
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return true
	}
	hay := strings.ToLower(strings.Join([]string{
		item.Name,
		item.Location,
		pantryItemStatus(item),
		meta.CatalogNo,
		meta.LotNo,
		meta.VolumeNote,
		meta.Notes,
	}, " "))
	return strings.Contains(hay, q)
}

func toLabStickerStatus(item runs.PantryItem) (labsticker.ItemStatus, string) {
	switch item.Status {
	case "near_expiry":
		return labsticker.StatusNearExpiry, "临期"
	case "expired":
		return labsticker.StatusExpired, "已过期"
	case "exhausted":
		return labsticker.StatusExhausted, "已用尽"
	case "opened":
		return labsticker.StatusOpened, "已开封"
	default:
		return labsticker.StatusOK, ""
	}
}

func (w *Workspace) buildShelfTiers(items []runs.PantryItem) ([]labsticker.ShelfTier, map[string]string) {
	w.EnsurePantryBoxesLoaded()
	boxMatchSlot := map[string]string{}
	zoneOrder := []string{"-80", "-20", "4C", "ambient"}
	zoneMeta := map[string]struct{ title, subtitle string }{
		"-80":     {"-80°C 超低温与细胞冻存架", "长期保藏 · 抗体冻存盒与超低温样品管"},
		"-20":     {"-20°C 酶与核酸冻存架", "常用分子酶 · EP 离心管盒与 PCR 孔盒"},
		"4C":      {"4°C 冷藏培养基与常用缓冲液架", "高频取用 · 培养基瓶与 15/50 mL 离心管架"},
		"ambient": {"常温与避光干燥试剂柜", "冻干标准品 · 固定液与避光试剂"},
	}
	buckets := map[string][]labsticker.ShelfItem{}

	// 1. Place storage boxes & racks first in their temperature zones.
	boxByID := map[string]PantryBox{}
	q := strings.ToLower(strings.TrimSpace(w.pantrySearch))
	for _, box := range w.PantryBoxes {
		boxByID[box.ID] = box
		occSlots, uniqueItems, alertCnt, matchSlots := w.BoxStats(box, w.pantrySearch)
		spec := LookupPantryBoxSpec(box.Kind)
		if w.pantryZoneFilter != "" && w.pantryZoneFilter != "all" {
			if w.pantryZoneFilter == "near_expiry" {
				if alertCnt == 0 {
					continue
				}
			} else if box.ZoneKey != w.pantryZoneFilter {
				continue
			}
		}
		boxHay := strings.ToLower(strings.Join([]string{box.Name, spec.Title, spec.ShortTitle, box.ShelfPos, box.Notes}, " "))
		boxSelfMatches := q != "" && strings.Contains(boxHay, q)
		tubeMatched := len(matchSlots) > 0
		if q != "" && !boxSelfMatches && !tubeMatched {
			continue
		}
		if len(matchSlots) > 0 {
			boxMatchSlot[box.ID] = matchSlots[0]
		}
		totalCap := max(1, box.Rows*box.Cols)
		occText := fmt.Sprintf("%d/%d 孔", occSlots, totalCap)
		badgeText := occText
		st := labsticker.StatusOK
		if alertCnt > 0 {
			st = labsticker.StatusNearExpiry
			badgeText = fmt.Sprintf("含 %d 临期", alertCnt)
		}
		metaLine := fmt.Sprintf("%s · %d 管", spec.ShortTitle, uniqueItems)
		if box.ShelfPos != "" {
			metaLine = box.ShelfPos + " · " + occText
		}
		tip := fmt.Sprintf("%s (%s %d×%d · 已放 %d 管/%s)", box.Name, spec.Title, box.Rows, box.Cols, uniqueItems, occText)
		if tubeMatched {
			tip += " · 搜索命中孔位 " + strings.Join(matchSlots, ", ")
		} else {
			tip += " · 点击打开盒内孔位图"
		}
		buckets[box.ZoneKey] = append(buckets[box.ZoneKey], labsticker.ShelfItem{
			ID:             "box:" + box.ID,
			Name:           box.Name,
			Location:       box.ShelfPos,
			Vessel:         spec.StickerID,
			Status:         st,
			MetaLine:       metaLine,
			BadgeText:      badgeText,
			TooltipText:    tip,
			IsBox:          true,
			OccupancyText:  occText,
			MatchHighlight: tubeMatched,
		})
	}

	// 2. Place standalone (unboxed) bottles & reagents alongside the boxes on their shelves.
	for _, item := range items {
		meta := w.MetaForPantryItem(item)
		if meta.BoxID != "" {
			if _, boxed := boxByID[meta.BoxID]; boxed {
				continue
			}
		}
		if !pantryMatchesFilter(item, meta, w.pantrySearch, w.pantryZoneFilter) {
			continue
		}
		zk, _, _ := pantryZoneCategory(item.Location)
		st, badge := toLabStickerStatus(item)
		metaLine := strings.TrimSpace(meta.VolumeNote)
		if metaLine == "" {
			if item.Location != "" {
				metaLine = item.Location
			} else if spec, ok := labsticker.Lookup(meta.VesselID); ok {
				metaLine = spec.VolumeLabel
			}
		}
		if item.ExpiresOn != "" && (item.Status == "near_expiry" || item.Status == "expired") && metaLine == "" {
			metaLine = "效期 " + item.ExpiresOn
		}
		tooltip := item.Name
		if item.Location != "" {
			tooltip += " · " + item.Location
		}
		if meta.CatalogNo != "" {
			tooltip += " · 货号 " + meta.CatalogNo
		}
		if len(meta.Manuals) > 0 {
			tooltip += fmt.Sprintf(" · 含 %d 份说明书", len(meta.Manuals))
		}
		if meta.ProductURL != "" {
			tooltip += " · 已绑定产品主页"
		}
		buckets[zk] = append(buckets[zk], labsticker.ShelfItem{
			ID:          item.ID,
			Name:        item.Name,
			Location:    item.Location,
			Vessel:      meta.VesselID,
			CustomImage: w.loadCustomStickerImage(meta.CustomStickerRel),
			Status:      st,
			MetaLine:    metaLine,
			BadgeText:   badge,
			TooltipText: tooltip,
			HasManual:   len(meta.Manuals) > 0,
			HasURL:      strings.TrimSpace(meta.ProductURL) != "",
		})
	}
	var tiers []labsticker.ShelfTier
	for _, zk := range zoneOrder {
		list := buckets[zk]
		if len(list) == 0 {
			continue
		}
		info := zoneMeta[zk]
		tiers = append(tiers, labsticker.ShelfTier{
			Title:    info.title,
			Subtitle: info.subtitle,
			ZoneKey:  zk,
			Items:    list,
		})
	}
	return tiers, boxMatchSlot
}

func findPantryItemByID(items []runs.PantryItem, id string) (runs.PantryItem, bool) {
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return runs.PantryItem{}, false
}

func pantryView(c *ui.Context, w *Workspace, tok theme.Tokens) {
	t := c.Theme()
	w.EnsurePantryMetaLoaded()
	w.EnsurePantryBoxesLoaded()

	// Level 2 Drill-Down: Spatial Storage Box / Tube Rack Interior View
	if w.activeBoxID != "" {
		if box, ok := w.FindPantryBox(w.activeBoxID); ok {
			renderPantryBoxInteriorView(c, w, tok, box)
			return
		}
		w.activeBoxID = ""
	}

	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(0)).Children(func() {
			ui.Text(c, "储藏室货架 · 试剂盒与贴纸台账").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			ui.Text(c, "点击收纳盒贴纸下钻查看盒内孔位坐标；点击瓶管贴纸查阅批号效期、说明书 (PDF) 与产品网址。").
				FontSize(float32(tok.TextData)).TextColor(tok.TextMuted)
		})
		components.Tabs(c, &w.pantryViewTab, components.TabsSegmented, "贴纸货架", "台账表格")
		if components.Button(c, "+ 新建收纳盒", components.ButtonSecondary).Clicked() {
			zone := w.pantryZoneFilter
			if zone == "" || zone == "all" || zone == "near_expiry" {
				zone = "-80"
			}
			w.OpenNewBoxDialog(zone)
		}
		if components.Button(c, "+ 新试剂上架", components.ButtonPrimary).Clicked() {
			w.OpenNewPantryDialog("")
		}
	})

	// Count zone stats & expiring alerts
	total := len(w.Pantry.Items)
	nearCount := 0
	zoneCounts := map[string]int{}
	boxByID := map[string]PantryBox{}
	for _, b := range w.PantryBoxes {
		boxByID[b.ID] = b
	}
	for _, it := range w.Pantry.Items {
		if it.Status == "near_expiry" || it.Status == "expired" {
			nearCount++
		}
		meta := w.MetaForPantryItem(it)
		zk, _, _ := pantryZoneCategory(it.Location)
		if b, ok := boxByID[meta.BoxID]; ok && b.ZoneKey != "" {
			zk = b.ZoneKey
		}
		zoneCounts[zk]++
	}

	// Top Search & Zone Filter Capsule Toolbar
	ui.Row(c).FillWidth().Wrap().Padding(t.Space(2), t.Space(3)).
		Radius(float32(tok.PanelRadius)).Background(tok.Elevated).Border(1, tok.Border).
		AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
		ui.Box(c).Width(float32(tok.Spacing48)*5.2).Children(func() {
			components.TextBox(c, &w.pantrySearch, components.TextBoxOptions{
				LeadingIcon: lucide.Search(),
			}).Placeholder("搜试剂、货号、盒子或孔位…")
		})
		ui.Box(c).Width(t.Space(0)+1).Height(t.Space(4)).Background(tok.BorderStrong)
		filters := []struct {
			key   string
			label string
		}{
			{"all", fmt.Sprintf("全部 (%d)", total)},
			{"-80", fmt.Sprintf("-80°C (%d)", zoneCounts["-80"])},
			{"-20", fmt.Sprintf("-20°C (%d)", zoneCounts["-20"])},
			{"4C", fmt.Sprintf("4°C (%d)", zoneCounts["4C"])},
			{"ambient", fmt.Sprintf("常温避光 (%d)", zoneCounts["ambient"])},
			{"near_expiry", fmt.Sprintf("临期提醒 (%d)", nearCount)},
		}
		for _, f := range filters {
			f := f
			active := w.pantryZoneFilter == f.key || (w.pantryZoneFilter == "" && f.key == "all")
			variant := components.ButtonGhost
			if active {
				variant = components.ButtonToolbar
			}
			if components.Button(c, f.label, variant).FontFeatures("tnum").Clicked() {
				w.pantryZoneFilter = f.key
			}
		}
		ui.Spacer(c)
		if components.Button(c, "导出贴标", components.ButtonGhost).Clicked() {
			w.ExportPantryLabels()
		}
		if components.Button(c, "刷新", components.ButtonGhost).Clicked() {
			w.ReloadPantry()
		}
	})

	if !w.PantryLoaded && len(w.Pantry.Items) == 0 && len(w.PantryBoxes) == 0 {
		ui.Text(c, "正在载入储藏室台账…").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		return
	}

	if len(w.Pantry.Items) == 0 && len(w.PantryBoxes) == 0 {
		ui.Column(c).FillWidth().Padding(t.Space(6)).Radius(float32(tok.PanelRadius)).
			Background(tok.Elevated).Border(1, tok.Border).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
			ui.Text(c, "储藏室货架暂无在库试剂与收纳盒，点击右上方「+ 新建收纳盒」或「+ 新试剂上架」。").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			ui.Row(c).Gap(t.Space(2)).Children(func() {
				if components.Button(c, "新建收纳盒", components.ButtonSecondary).Clicked() {
					w.OpenNewBoxDialog("-80")
				}
				if components.Button(c, "录入第一支试剂", components.ButtonSecondary).Clicked() {
					w.OpenNewPantryDialog("4°C 冷藏架")
				}
			})
		})
		return
	}

	if w.pantryViewTab == 0 {
		tiers, boxMatchSlot := w.buildShelfTiers(w.Pantry.Items)
		if len(tiers) == 0 {
			ui.Row(c).FillWidth().Padding(t.Space(4)).Radius(float32(tok.ControlRadius)).
				Background(tok.Elevated).Border(1, tok.Border).AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Text(c, "当前筛选条件下没有匹配的收纳盒或试剂。").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				ui.Spacer(c)
				if components.Button(c, "重置筛选", components.ButtonSecondary).Clicked() {
					w.pantrySearch = ""
					w.pantryZoneFilter = "all"
				}
			})
		} else {
			selectedID := ""
			if w.PantryDialogOpen {
				selectedID = w.PantryID
			}
			labsticker.RenderShelfGrid(c, tiers, labsticker.ShelfRenderOptions{
				SelectedID: selectedID,
				OnSelect: func(si labsticker.ShelfItem) {
					if si.IsBox && strings.HasPrefix(si.ID, "box:") {
						boxID := strings.TrimPrefix(si.ID, "box:")
						w.OpenPantryBox(boxID, boxMatchSlot[boxID])
						return
					}
					if item, ok := findPantryItemByID(w.Pantry.Items, si.ID); ok {
						w.OpenPantryDetail(item)
					}
				},
				OnAddBoxInTier: func(tier labsticker.ShelfTier) {
					w.OpenNewBoxDialog(tier.ZoneKey)
				},
				OnAddInTier: func(tier labsticker.ShelfTier) {
					preset := "常温避光柜"
					switch tier.ZoneKey {
					case "-80":
						preset = "-80°C 超低温冻存架"
					case "-20":
						preset = "-20°C 酶盒"
					case "4C":
						preset = "4°C 冷藏架"
					}
					w.OpenNewPantryDialog(preset)
				},
			})
		}
	} else {
		renderPantryTableView(c, w, tok)
	}

	if w.PantryExportJSON != "" {
		ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
			ui.Text(c, "贴标清单明细").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text)
			components.TextBox(c, &w.PantryExportJSON, components.TextBoxOptions{Multiline: true, ReadOnly: true})
		})
	}
}

func renderPantryTableView(c *ui.Context, w *Workspace, tok theme.Tokens) {
	t := c.Theme()
	var filtered []runs.PantryItem
	for _, it := range w.Pantry.Items {
		if pantryMatchesFilter(it, w.MetaForPantryItem(it), w.pantrySearch, w.pantryZoneFilter) {
			filtered = append(filtered, it)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].Status == "near_expiry" && filtered[j].Status != "near_expiry"
	})

	cols := []ui.TableColumn{
		{ID: "name", Title: "试剂 / 耗材名称", MinWidth: 168},
		{ID: "cat", Title: "货号 / 批号", Width: 136},
		{ID: "loc", Title: "存放位置", Width: 136},
		{ID: "status", Title: "状态", Width: 88},
		{ID: "exp", Title: "到期日", Width: 104, Align: ui.End},
		{ID: "docs", Title: "资料与链接", Width: 112},
		{ID: "act", Title: "操作", Width: 88, Fixed: true},
	}
	components.Table(c, nil, cols, func() {
		for _, item := range filtered {
			item := item
			meta := w.MetaForPantryItem(item)
			_, _ = toLabStickerStatus(item)
			components.TableRow(c, item.ID, item.Name, false, func(col int) {
				switch col {
				case 0:
					nameBtn := ui.ButtonBase(c.Key("pantry-row-open-"+item.ID)).Label("查看 "+item.Name+" 详情").
						FillWidth().Justify(ui.Start).OnClick(func() {
						w.OpenPantryDetail(item)
					})
					nameBtn.Children(func() {
						ui.Text(c, item.Name).FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text).
							SingleLine().Ellipsis("…").MinWidth(0)
					})
				case 1:
					label := firstNonEmptyStr(meta.CatalogNo, meta.LotNo, "—")
					if meta.CatalogNo != "" && meta.LotNo != "" {
						label = meta.CatalogNo + " · " + meta.LotNo
					}
					ui.Text(c, label).FontSize(float32(tok.TextMeta)).FontFeatures("tnum").TextColor(tok.TextMuted).
						SingleLine().Ellipsis("…").MinWidth(0)
				case 2:
					dispLoc := firstNonEmptyStr(w.FormatPantryDisplayLocation(item), "—")
					if box, ok := w.FindPantryBox(meta.BoxID); ok {
						slots := ItemBoxSlots(meta)
						firstSlot := "A1"
						if len(slots) > 0 {
							firstSlot = slots[0]
						}
						if components.Button(c, box.Name+" · "+firstNonEmptyStr(FormatSlotList(slots), firstSlot), components.ButtonGhost).Clicked() {
							w.OpenPantryBox(box.ID, firstSlot)
						}
					} else {
						ui.Text(c, dispLoc).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).
							SingleLine().Ellipsis("…").MinWidth(0)
					}
				case 3:
					tone := components.BadgeNeutral
					switch item.Status {
					case "near_expiry":
						tone = components.BadgeWarning
					case "expired", "exhausted":
						tone = components.BadgeDanger
					case "opened":
						tone = components.BadgeAccent
					}
					components.Badge(c, pantryItemStatus(item), components.BadgeOptions{Tone: tone})
				case 4:
					ui.Text(c, pantryDate(item.ExpiresOn)).FontSize(float32(tok.TextMeta)).FontFeatures("tnum").TextColor(tok.TextMuted)
				case 5:
					ui.Row(c).AlignItems(ui.Center).Gap(t.Space(1)).Children(func() {
						if len(meta.Manuals) > 0 {
							components.Badge(c, fmt.Sprintf("说明书×%d", len(meta.Manuals)), components.BadgeOptions{Tone: components.BadgeAccent})
						}
						if strings.TrimSpace(meta.ProductURL) != "" {
							ui.Icon(c, lucide.Globe()).Size(t.Space(4), t.Space(4)).TextColor(tok.Accent)
						}
						if len(meta.Manuals) == 0 && strings.TrimSpace(meta.ProductURL) == "" {
							ui.Text(c, "—").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
						}
					})
				case 6:
					if components.Button(c, "详情", components.ButtonGhost).Clicked() {
						w.OpenPantryDetail(item)
					}
				}
			})
		}
	})
}

func (w *Workspace) renderPantryDetailDialog(c *ui.Context) {
	if !w.PantryDialogOpen {
		return
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	title := "新增储藏室试剂"
	if w.PantryID != "" {
		title = "试剂档案 · " + firstNonEmptyStr(w.PantryName, "详情与说明书")
	}

	ui.Box(c.Key("pantry-detail-dlg")).Children(func() {
		components.Dialog(c, &w.PantryDialogOpen, components.DialogOptions{
			Label: title,
		}, func(panel ui.Element) {
		panel.Width(float32(tok.Spacing48)*15.5).Padding(t.Space(3)).Radius(float32(tok.DialogRadius)).
			Background(tok.Surface).Border(1, tok.BorderStrong)

		ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
			// Header strip
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				ui.Icon(c, lucide.FlaskConical()).Size(t.Space(4), t.Space(4)).TextColor(tok.Accent)
				ui.Text(c, title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text).
					Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
				if w.PantryID != "" {
					if curItem, ok := findPantryItemByID(w.Pantry.Items, w.PantryID); ok {
						components.Badge(c, pantryItemStatus(curItem), components.BadgeOptions{Tone: detailStatusTone(curItem.Status)})
					} else {
						components.Badge(c, "在库试剂", components.BadgeOptions{Tone: components.BadgeNeutral})
					}
				} else {
					components.Badge(c, "新试剂上架", components.BadgeOptions{Tone: components.BadgeAccent})
				}
			})

			// Scrollable body keeps dialog reachable even at 900x600 when vessel picker expands;
			ui.Scroll(c).FillWidth().MaxHeight(float32(tok.Spacing48)*13.8).Children(func() {
				ui.Column(c).FillWidth().Gap(t.Space(2)).Children(func() {
					// 1. Hero Continuity Card: large white-bordered sticker + identity inputs + segmented status
					ui.Row(c).FillWidth().Padding(t.Space(2)).Radius(float32(tok.PanelRadius)).
						Background(tok.Elevated).Border(1, tok.Border).Gap(t.Space(2)).AlignItems(ui.Start).Children(func() {
						// Left recessed sticker pedestal (Continuity from shelf sticker -> hero preview)
						pedestal := ui.Column(c).Width(float32(tok.Spacing48)*3.1).Padding(t.Space(2)).Radius(float32(tok.ControlRadius)).
							Border(1, tok.Border).
							LinearGradient(ui.LinearGradient{
								Angle: 180, Oklab: true,
								From: tok.Canvas.Mix(tok.BorderStrong, 0.22),
								To:   tok.Surface,
							}).
							InsetShadow(0, 2, 4, 0, tok.Text.Alpha(0.06)).
							AlignItems(ui.Center).Gap(t.Space(2))
						pedestal.Children(func() {
							customImg := w.loadCustomStickerImage(w.pantryDraftMeta.CustomStickerRel)
							labsticker.RenderSticker(c, w.pantryDraftMeta.VesselID, customImg, labsticker.StickerOptions{
								TargetHeightDIP: 84,
								BorderDIP:       4.5,
								Exhausted:       w.pantryStatusIdx == 2,
							})
							if spec, ok := labsticker.Lookup(w.pantryDraftMeta.VesselID); ok {
								ui.Text(c, spec.Title).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).
									SingleLine().Ellipsis("…")
							}
							btnLabel := "更换器皿贴纸"
							if w.pantryPickerOpen {
								btnLabel = "收起贴纸库"
							}
							if components.Button(c, btnLabel, components.ButtonSecondary).Clicked() {
								w.pantryPickerOpen = !w.pantryPickerOpen
							}
						})
						if files := pedestal.DroppedFiles(); len(files) > 0 {
							if err := w.SetDraftCustomSticker(files[0]); err != nil {
								w.pantryDialogErr = err.Error()
							}
						}

						// Right structured identity + stock fields
						ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(2)).Children(func() {
							ui.Row(c).FillWidth().Gap(t.Space(2)).AlignItems(ui.End).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "试剂 / 耗材名称 *").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									components.TextBox(c, &w.PantryName, components.TextBoxOptions{}).
										Placeholder("例如：SYBR Green Premix Ex Taq (2×)")
								})
								ui.Column(c).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "在库状态").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									components.Tabs(c, &w.pantryStatusIdx, components.TabsSegmented, "未开封", "已开封", "已用尽")
								})
							})
							ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "品牌与货号 (Catalog #)").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									components.TextBox(c, &w.pantryDraftMeta.CatalogNo, components.TextBoxOptions{}).
										Placeholder("如 Takara RR820A")
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "批次号 (Lot #)").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									components.TextBox(c, &w.pantryDraftMeta.LotNo, components.TextBoxOptions{}).
										Placeholder("如 AJF1042A")
								})
								ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "货架标签说明").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									components.TextBox(c, &w.pantryDraftMeta.VolumeNote, components.TextBoxOptions{}).
										Placeholder("如 2 mL · 余 40%")
								})
							})
						})
					})

					// 2. Collapsible 12-vessel BioRender sticker picker & custom image import
					if w.pantryPickerOpen {
						ui.Column(c).FillWidth().Padding(t.Space(3)).Radius(float32(tok.PanelRadius)).
							Background(tok.Elevated).Border(1, tok.Border).Gap(t.Space(2)).Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Text(c, "选择容器贴纸样式（12 款 BioRender 器皿）").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
								ui.Spacer(c)
								if components.Button(c, "上传自定义图片", components.ButtonSecondary).Clicked() && w.Callbacks.PickStickerImage != nil {
									if p, err := w.Callbacks.PickStickerImage(); err == nil && p != "" {
										if err := w.SetDraftCustomSticker(p); err != nil {
											w.pantryDialogErr = err.Error()
										}
									}
								}
							})
							specs := labsticker.AllVessels()
							const cols = 6
							for start := 0; start < len(specs); start += cols {
								end := min(start+cols, len(specs))
								ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
									for _, spec := range specs[start:end] {
										spec := spec
										chosen := w.pantryDraftMeta.CustomStickerRel == "" && w.pantryDraftMeta.VesselID == spec.ID
										bg := tok.Surface
										border := tok.Border
										if chosen {
											bg = tok.AccentSoftBg
											border = tok.Accent
										}
										tile := ui.ButtonBase(c.Key("pick-vessel-"+string(spec.ID))).Label("选用 " + spec.Title).
											Grow(1).Basis(0).MinWidth(0).Padding(t.Space(1)).
											Radius(float32(tok.ControlRadius)).Background(bg).Border(1, border).
											OnClick(func() {
												w.SelectDraftVessel(spec.ID)
											})
										tile.Children(func() {
											ui.Column(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(0)).Children(func() {
												labsticker.RenderSticker(c, spec.ID, nil, labsticker.StickerOptions{
													TargetHeightDIP: 42,
													BorderDIP:       3.0,
												})
												ui.Text(c, spec.Title).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).
													SingleLine().Ellipsis("…").MinWidth(0)
											})
										})
									}
								})
							}
						})
					}

					// 3. Storage Location & Dates
					ui.Column(c).FillWidth().Padding(t.Space(2)).Radius(float32(tok.PanelRadius)).
						Background(tok.Elevated).Border(1, tok.Border).Gap(t.Space(1)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Text(c, "存放位置与效期记录").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
							ui.Spacer(c)
							for _, preset := range []string{"-80°C", "-20°C 酶盒", "4°C 冷藏架", "常温避光"} {
								preset := preset
								variant := components.ButtonGhost
								if w.PantryLocation == preset {
									variant = components.ButtonSecondary
								}
								if components.Button(c, preset, variant).Clicked() {
									w.PantryLocation = preset
								}
							}
						})
						ui.Row(c).FillWidth().Gap(t.Space(2)).Children(func() {
							ui.Column(c).Grow(2).MinWidth(0).Gap(t.Space(1)).Children(func() {
								ui.Text(c, "存放柜层").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								components.TextBox(c, &w.PantryLocation, components.TextBoxOptions{}).
									Placeholder("如 -20°C 酶盒 A2")
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
								ui.Text(c, "购入日期").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								components.TextBox(c, &w.PantryPurchasedOn, components.TextBoxOptions{}).Placeholder("YYYY-MM-DD")
							})
							ui.Column(c).Grow(1).MinWidth(0).Gap(t.Space(1)).Children(func() {
								ui.Text(c, "开封日期").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								components.TextBox(c, &w.PantryOpenedOn, components.TextBoxOptions{}).Placeholder("YYYY-MM-DD")
							})
							ui.Column(c).Width(float32(tok.Spacing48)*2.0).Gap(t.Space(1)).Children(func() {
								ui.Text(c, "保质期(天)").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
								components.TextBox(c, &w.PantryShelfLifeDays, components.TextBoxOptions{}).Placeholder("365")
							})
						})

						// Optional nested Box / Rack & Slot placement row
						w.EnsurePantryBoxesLoaded()
						if len(w.PantryBoxes) > 0 {
							ui.Row(c).FillWidth().AlignItems(ui.End).Gap(t.Space(2)).Children(func() {
								ui.Column(c).Grow(2).MinWidth(0).Gap(t.Space(1)).Children(func() {
									ui.Text(c, "放入收纳盒 / 离心管架（可选）").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
									boxSel := components.Select(c, &w.pantryDraftMeta.BoxID)
									boxSel.Trigger.Children(func() {
										lbl := "独立摆放（不放入盒子）"
										if bx, ok := w.FindPantryBox(w.pantryDraftMeta.BoxID); ok {
											lbl = fmt.Sprintf("%s · %s (%d×%d)", ZoneDisplayPrefix(bx.ZoneKey), bx.Name, bx.Rows, bx.Cols)
										}
										ui.Text(c, lbl).FontSize(float32(tok.TextMeta)).TextColor(tok.Text).SingleLine().Ellipsis("…")
									})
									boxSel.Popup(func(panel ui.Element) {
										noneItem := boxSel.Item("")
										noneItem.Children(func() {
											ui.Text(c, "独立摆放（不放入盒子）").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
										})
										if noneItem.Clicked() {
											w.pantryDraftMeta.BoxID = ""
											w.pantryDraftMeta.BoxSlot = ""
											w.pantryDraftMeta.BoxSlots = nil
										}
										for _, bx := range w.PantryBoxes {
											bx := bx
											it := boxSel.Item(bx.ID)
											it.Children(func() {
												ui.Text(c, fmt.Sprintf("%s · %s (%d×%d)", ZoneDisplayPrefix(bx.ZoneKey), bx.Name, bx.Rows, bx.Cols)).
													FontSize(float32(tok.TextMeta)).TextColor(tok.Text)
											})
											if it.Clicked() {
												w.pantryDraftMeta.BoxID = bx.ID
												if w.pantryDraftMeta.BoxSlot == "" {
													w.pantryDraftMeta.BoxSlot = w.FirstEmptyBoxSlot(bx)
												}
												w.PantryLocation = FormatBoxLocation(bx.ZoneKey, bx.Name, w.pantryDraftMeta.BoxSlot)
											}
										}
									})
								})
								if bx, ok := w.FindPantryBox(w.pantryDraftMeta.BoxID); ok {
									ui.Column(c).Width(float32(tok.Spacing48)*2.0).Gap(t.Space(1)).Children(func() {
										ui.Text(c, "盒内孔位").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
										components.TextBox(c, &w.pantryDraftMeta.BoxSlot, components.TextBoxOptions{}).Placeholder("如 C2")
									})
									if components.Button(c, "选空位", components.ButtonSecondary).Clicked() {
										if free := w.FirstEmptyBoxSlot(bx); free != "" {
											w.pantryDraftMeta.BoxSlot = free
											w.PantryLocation = FormatBoxLocation(bx.ZoneKey, bx.Name, free)
										}
									}
								}
							})
						}
					})

					// 4. Manuals / Datasheets Upload + Vendor Product URL
					ui.Column(c).FillWidth().Padding(t.Space(2)).Radius(float32(tok.PanelRadius)).
						Background(tok.Elevated).Border(1, tok.Border).Gap(t.Space(1)).Children(func() {
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Icon(c, lucide.FileText()).Size(t.Space(4), t.Space(4)).TextColor(tok.Accent)
							ui.Text(c, "说明书与产品网址").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Text)
							ui.Spacer(c)
							if len(w.pantryDraftMeta.Manuals) > 0 {
								components.Badge(c, fmt.Sprintf("%d 份文档", len(w.pantryDraftMeta.Manuals)), components.BadgeOptions{Tone: components.BadgeAccent})
							}
						})

						// Vendor / Catalog URL row
						ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
							ui.Box(c).Grow(1).MinWidth(0).Children(func() {
								components.TextBox(c, &w.pantryDraftMeta.ProductURL, components.TextBoxOptions{
									LeadingIcon: lucide.Globe(),
								}).Placeholder("官网产品页 / 说明书链接（https://...）")
							})
							if host := extractURLHost(w.pantryDraftMeta.ProductURL); host != "" {
								components.Badge(c, host, components.BadgeOptions{Tone: components.BadgeNeutral})
							}
							if components.Button(c, "打开网页", components.ButtonSecondary).Clicked() {
								_ = w.ValidateAndOpenProductURL(w.pantryDraftMeta.ProductURL)
							}
						})

						// Manual / COA / SDS upload dropzone + file list
						dropBox := ui.Column(c).FillWidth().Padding(t.Space(2)).Radius(float32(tok.ControlRadius)).
							Background(tok.Surface).Border(1, tok.Border).Gap(t.Space(2))
						dropBox.Children(func() {
							ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
								ui.Icon(c, lucide.Upload()).Size(t.Space(4), t.Space(4)).TextColor(tok.TextMuted)
								ui.Text(c, "拖入或选择试剂说明书、COA 质检单、SDS (PDF / 图片 / Word)").
									FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).
									Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
								if components.Button(c, "上传说明书", components.ButtonSecondary).Clicked() && w.Callbacks.PickManualFiles != nil {
									if paths, err := w.Callbacks.PickManualFiles(); err == nil && len(paths) > 0 {
										_ = w.AttachManualFiles(paths)
									}
								}
							})
							for _, manual := range w.pantryDraftMeta.Manuals {
								manual := manual
								ui.Row(c).FillWidth().Padding(t.Space(1), t.Space(2)).
									Radius(float32(tok.ControlRadius)).Background(tok.Elevated).Border(1, tok.Border).
									AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
									components.FileIcon(c, manual.FileName)
									ui.Text(c, manual.FileName).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text).
										Grow(1).MinWidth(0).SingleLine().Ellipsis("…")
									if manual.SizeBytes > 0 {
										ui.Text(c, formatByteSize(manual.SizeBytes)).FontSize(float32(tok.TextMeta)).
											FontFeatures("tnum").TextColor(tok.TextMuted)
									}
									if components.Button(c, "查阅", components.ButtonGhost).Clicked() {
										_ = w.OpenManualFile(manual)
									}
									if components.IconButton(c, lucide.Trash(), "移除说明书", components.IconButtonCompact).Clicked() {
										w.RemoveManualFile(manual.ID)
									}
								})
							}
						})
						if dropped := dropBox.DroppedFiles(); len(dropped) > 0 {
							_ = w.AttachManualFiles(dropped)
						}

						// Notes
						ui.Column(c).FillWidth().Gap(t.Space(1)).Children(func() {
							ui.Text(c, "配制与存放备注").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
							components.TextBox(c, &w.pantryDraftMeta.Notes, components.TextBoxOptions{}).
								Placeholder("如分装体积、避免反复冻融、配制比例…")
						})
					})
				})
			})

			if w.pantryDialogErr != "" {
				ui.Row(c).FillWidth().Padding(t.Space(2), t.Space(3)).
					Radius(float32(tok.ControlRadius)).Background(tok.DangerBg).Border(1, tok.Danger).
					AlignItems(ui.Center).Children(func() {
					ui.Text(c, w.pantryDialogErr).FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText)
				})
			}

			// Footer action bar: 001 single Primary CTA + 009 inline delete Popconfirm
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(t.Space(2)).Children(func() {
				if w.PantryID != "" {
					if !w.pantryConfirmDel {
						if components.Button(c, "删除试剂", components.ButtonGhost).Clicked() {
							w.pantryConfirmDel = true
						}
					} else {
						ui.Text(c, "确认从台账移除？").FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText)
						if components.Button(c, "确认删除", components.ButtonSecondary).Clicked() {
							if it, ok := findPantryItemByID(w.Pantry.Items, w.PantryID); ok {
								w.DeletePantry(it)
							}
							w.PantryDialogOpen = false
							w.pantryConfirmDel = false
						}
						if components.Button(c, "取消", components.ButtonGhost).Clicked() {
							w.pantryConfirmDel = false
						}
					}
				}
				ui.Spacer(c)
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					w.PantryDialogOpen = false
					w.pantryConfirmDel = false
					w.pantryDialogErr = ""
				}
				if components.Button(c, "保存试剂档案", components.ButtonPrimary).Clicked() {
					_ = w.SavePantryDetail()
				}
			})
		})
	})
	})
}

func extractURLHost(raw string) string {
	norm, err := ValidateProductURL(raw)
	if err != nil || norm == "" {
		return ""
	}
	u, err := url.Parse(norm)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

func formatByteSize(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
