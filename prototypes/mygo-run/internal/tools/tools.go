package tools

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"asterlab-mygo-prototype/internal/assays"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/blocks"
	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/modules/runs"
	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

type Tool string

const (
	ToolPantry   Tool = "pantry"
	ToolSOP      Tool = "sop"
	ToolPlate    Tool = "plate"
	ToolDilution Tool = "dilution"
)

type Draft struct {
	Block *blocks.Instance
	Assay *assays.Design
}

type Callbacks struct {
	UseInDesigner       func(blocks.Instance)
	UseAsterPlate       func(source string, plateID string)
	InsertAsterBlock    func(block map[string]any)
	ListSOPs            func() []SOPEntry
	ExportSOPPDF        func(title, source string)
	CreateDesignFromSOP func(title, source string)
	ListPantry          func() (runs.PantryLedger, error)
	UpsertPantry        func(runs.PantryUpsert) (runs.PantryItem, error)
	DeletePantry        func(runs.PantryDelete) error
	ExportPantry        func() (runs.PantryLabels, error)
	LoadPantryMeta      func() (map[string]PantryItemMeta, error)
	SavePantryMeta      func(map[string]PantryItemMeta) error
	LoadPantryBoxes     func() ([]PantryBox, error)
	SavePantryBoxes     func([]PantryBox) error
	PickManualFiles     func() ([]string, error)
	PickStickerImage    func() (string, error)
	ImportPantryFile    func(itemKey, srcPath string) (storedRel, fileName string, sizeBytes int64, err error)
	ResolvePantryFile   func(storedRel string) string
	OpenLocalFile       func(absPath string) error
	OpenExternalURL     func(rawURL string) error
	LoadPlateHistory    func() ([]PlateHistoryEntry, error)
	SavePlateHistory    func([]PlateHistoryEntry) error
	SavePlateFile       func(defaultName, format string, data []byte) (string, error)
	CreateDraft         func(Draft)
}

// Workspace holds editable tool drafts until an explicit handoff action.
type Workspace struct {
	Active               Tool
	Plate                blocks.Plate
	PlateFormat          int
	PlateTopology        string
	ActivePlatePreset    string
	plateDoc             *asterdoc.Document
	plateView            *asterdoc.View
	PlateHistory         []PlateHistoryEntry
	currentHistoryID     string
	ActiveSOPID          string
	sopView              *asterdoc.View
	sopViewKey           string
	sopDocErr            string
	BenchCalc            BenchCalcState
	Group                string
	Stock, Final, Volume string
	Error                string
	Status               string
	NextDraftID          uint64
	Callbacks            Callbacks

	Pantry                    runs.PantryLedger
	PantryID                  string
	PantryName                string
	PantryLocation            string
	PantryPurchasedOn         string
	PantryOpenedOn            string
	PantryShelfLifeDays       string
	PantryOpenedShelfLifeDays *uint32
	PantryExhausted           bool
	PantryLoaded              bool
	PantryRetry               func()
	PantryExportJSON          string

	// Sticker shelf & reagent archive metadata
	PantryMeta         map[string]PantryItemMeta
	pantryMetaLoaded   bool
	pantrySearch       string
	pantryZoneFilter   string
	pantryViewTab      int
	PantryDialogOpen   bool
	pantryPickerOpen   bool
	pantryConfirmDel   bool
	pantryDialogErr    string
	pantryStatusIdx    int
	pantryDraftMeta    PantryItemMeta
	pantryCustomImages map[string]pantryImageCacheEntry

	// Level 2 Storage Boxes & Tube Racks (冻存盒 / EP 盒 / 离心管架 / PCR 盒)
	PantryBoxes         []PantryBox
	pantryBoxesLoaded   bool
	activeBoxID         string
	selectedBoxSlot     string
	boxMoveItemID       string
	boxMoveFromSlot     string
	PantryBoxDialogOpen bool
	boxDraft            PantryBox
	boxDraftRowsStr     string
	boxDraftColsStr     string
	boxDialogErr        string
	boxConfirmDel       bool
	boxEditingID        string
}

func New(cb Callbacks) *Workspace {
	w := &Workspace{
		Active:    ToolPantry,
		Plate:     blocks.NewPlate(),
		Stock:     "100",
		Final:     "10",
		Volume:    "100",
		BenchCalc: defaultBenchCalcState(),
		Callbacks: cb,
	}
	w.initPlateStudio()
	w.ReloadPantry()
	w.EnsurePantryMetaLoaded()
	w.EnsurePantryBoxesLoaded()
	return w
}

func (w *Workspace) ReloadPantry() {
	if w.Callbacks.ListPantry == nil {
		return
	}
	ledger, err := w.Callbacks.ListPantry()
	if err != nil {
		w.Error, w.Status = err.Error(), ""
		w.PantryRetry = w.ReloadPantry
		return
	}
	w.Pantry, w.PantryLoaded = ledger, true
	w.Error, w.Status, w.PantryRetry = "", "", nil
}

// ResetPantryState clears all cached pantry ledger, metadata, box, and dialog state
// and reloads fresh data from the currently active Vault/callbacks.
func (w *Workspace) ResetPantryState() {
	w.Pantry = runs.PantryLedger{}
	w.PantryLoaded = false
	w.PantryMeta = nil
	w.pantryMetaLoaded = false
	w.PantryBoxes = nil
	w.pantryBoxesLoaded = false
	w.pantryCustomImages = nil
	w.activeBoxID = ""
	w.selectedBoxSlot = ""
	w.boxMoveItemID = ""
	w.boxMoveFromSlot = ""
	w.PantryDialogOpen = false
	w.PantryBoxDialogOpen = false
	w.pantryPickerOpen = false
	w.pantryConfirmDel = false
	w.boxConfirmDel = false
	w.pantryDialogErr = ""
	w.boxDialogErr = ""
	w.PantryExportJSON = ""
	w.Error = ""
	w.Status = ""
	w.ReloadPantry()
	w.EnsurePantryMetaLoaded()
	w.EnsurePantryBoxesLoaded()
}

func (w *Workspace) SavePantry() {
	w.PantryRetry = nil
	if w.Callbacks.UpsertPantry == nil {
		return
	}
	name := strings.TrimSpace(w.PantryName)
	if name == "" {
		w.Error, w.Status = "请输入名称", ""
		return
	}
	var days *uint32
	if value := strings.TrimSpace(w.PantryShelfLifeDays); value != "" {
		n, err := strconv.ParseUint(value, 10, 32)
		if err != nil || n == 0 {
			w.Error, w.Status = "保质期请输入正整数天数", ""
			return
		}
		d := uint32(n)
		days = &d
	}
	input := runs.PantryUpsert{
		ID: w.PantryID, Name: name, Location: strings.TrimSpace(w.PantryLocation),
		PurchasedOn: strings.TrimSpace(w.PantryPurchasedOn), OpenedOn: strings.TrimSpace(w.PantryOpenedOn),
		ShelfLifeDays: days, OpenedShelfLifeDays: w.PantryOpenedShelfLifeDays,
		Exhausted: w.PantryExhausted,
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
	if _, err := w.Callbacks.UpsertPantry(input); err != nil {
		w.Error, w.Status = err.Error(), ""
		w.PantryRetry = w.SavePantry
		return
	}
	w.clearPantryForm()
	w.ReloadPantry()
	if w.Error == "" {
		w.Status = "已保存"
	}
}

func (w *Workspace) EditPantry(item runs.PantryItem) {
	w.PantryID, w.PantryName, w.PantryLocation = item.ID, item.Name, item.Location
	w.PantryPurchasedOn, w.PantryOpenedOn, w.PantryExhausted = item.PurchasedOn, item.OpenedOn, item.Exhausted
	w.PantryOpenedShelfLifeDays = item.OpenedShelfLifeDays
	w.PantryShelfLifeDays = ""
	if item.ShelfLifeDays != nil {
		w.PantryShelfLifeDays = strconv.FormatUint(uint64(*item.ShelfLifeDays), 10)
	}
	w.Error, w.Status, w.PantryRetry = "", "", nil
}

func (w *Workspace) DeletePantry(item runs.PantryItem) {
	w.PantryRetry = nil
	if w.Callbacks.DeletePantry == nil {
		return
	}
	if err := w.Callbacks.DeletePantry(runs.PantryDelete{ID: item.ID, ExpectedRevision: &item.Revision}); err != nil {
		w.Error, w.Status = err.Error(), ""
		w.PantryRetry = func() { w.DeletePantry(item) }
		return
	}
	if w.PantryID == item.ID {
		w.clearPantryForm()
	}
	w.ReloadPantry()
}

func (w *Workspace) clearPantryForm() {
	w.PantryID, w.PantryName, w.PantryLocation = "", "", ""
	w.PantryPurchasedOn, w.PantryOpenedOn, w.PantryShelfLifeDays = "", "", ""
	w.PantryOpenedShelfLifeDays = nil
	w.PantryExhausted = false

}
func (w *Workspace) ExportPantryLabels() {
	w.PantryRetry, w.PantryExportJSON = nil, ""
	w.Error, w.Status = "", ""
	if w.Callbacks.ExportPantry == nil {
		return
	}
	labels, err := w.Callbacks.ExportPantry()
	if err != nil {
		w.Error, w.Status = err.Error(), ""
		w.PantryRetry = w.ExportPantryLabels
		return
	}
	data, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		w.Error, w.Status = err.Error(), ""
		w.PantryRetry = w.ExportPantryLabels
		return
	}
	w.PantryExportJSON = string(data)
	w.Error, w.Status, w.PantryRetry = "", fmt.Sprintf("贴标清单已就绪 · %d 项", len(labels.Labels)), nil
}

func blockInstance(kind blocks.BlockType, props any) (blocks.Instance, error) {
	return blocks.NewInstance("tool-draft", blocks.Definition{ID: "tool." + string(kind), Version: 1, Type: kind, Props: props})
}

func (w *Workspace) plateDraft() (blocks.Instance, error) {
	return blockInstance(blocks.TypePlate96, w.Plate.Props)
}
func (w *Workspace) dilutionDraft() (blocks.Instance, error) {
	stock, e1 := strconv.ParseFloat(strings.TrimSpace(w.Stock), 64)
	final, e2 := strconv.ParseFloat(strings.TrimSpace(w.Final), 64)
	volume, e3 := strconv.ParseFloat(strings.TrimSpace(w.Volume), 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return blocks.Instance{}, fmt.Errorf("enter numeric concentrations and volume")
	}
	p := blocks.DilutionPropsFromInput(blocks.DilutionInput{StockConc: stock, FinalConc: final, FinalVol: volume})
	if !p.Valid {
		return blocks.Instance{}, fmt.Errorf("%s", p.Error)
	}
	return blockInstance(blocks.TypeDilutionCalculator, p)
}
func (w *Workspace) handoff() {
	if w.Active == ToolPlate && w.Callbacks.UseAsterPlate != nil && (w.activePlateAssignedCount() > 0 || len(w.Plate.Props.Groups) == 0) && w.plateDoc != nil {
		if w.activePlateAssignedCount() > 0 {
			_, _ = w.SaveCurrentPlateToHistory()
		}
		w.Error, w.Status = "", "已应用孔板到当前设计"
		w.Callbacks.UseAsterPlate(w.plateDoc.Source, studioPlateID)
		return
	}
	var inst blocks.Instance
	var err error
	switch w.Active {
	case ToolPlate:
		inst, err = w.plateDraft()
	case ToolDilution:
		inst, err = w.dilutionDraft()
	default:
		w.Error = "请先选择一个有效草稿再导入"
		return
	}
	if err != nil {
		w.Error = err.Error()
		return
	}
	w.NextDraftID++
	inst.ID = fmt.Sprintf("tool-draft-%d", w.NextDraftID)
	w.Error, w.Status = "", "草稿已就绪"
	if w.Callbacks.UseInDesigner != nil {
		w.Callbacks.UseInDesigner(inst)
	}
	if w.Callbacks.CreateDraft != nil {
		w.Callbacks.CreateDraft(Draft{Block: &inst})
	}
}

// ViewPantry renders the standalone Pantry reagent shelf and box-slot view
// without the legacy 4-tool switcher chrome.
func ViewPantry(c *ui.Context, w *Workspace) {
	if w == nil {
		return
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	ui.Scroll(c).Fill().Background(tok.Canvas).Children(func() {
		ui.Column(c).FillWidth().Padding(float32(tok.Spacing16)).Gap(float32(tok.Spacing12)).Children(func() {
			pantryView(c, w, tok)
			if w.Error != "" {
				ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
					Radius(float32(tok.ControlRadius)).Background(tok.DangerBg).Border(1, tok.Danger).
					AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, w.Error).FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText).Grow(1).MinWidth(0)
					if w.PantryRetry != nil && components.Button(c, "重试", components.ButtonSecondary).Clicked() {
						w.PantryRetry()
					}
				})
			} else if w.Status != "" {
				ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
					Radius(float32(tok.ControlRadius)).Background(tok.SuccessBg).Border(1, tok.Success.Alpha(.4)).
					AlignItems(ui.Center).Children(func() {
					ui.Text(c, w.Status).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.SuccessText)
				})
			}
		})
	})
	w.renderPantryDetailDialog(c)
	w.renderPantryBoxDialog(c)
}

func View(c *ui.Context, w *Workspace) {
	if w == nil {
		return
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	ui.Column(c).Fill().Background(tok.Canvas).Padding(float32(tok.Spacing16)).Gap(float32(tok.Spacing12)).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Text(c, "资料与工具").FontSize(float32(tok.TextPage)).Bold().TextColor(tok.Text)
		})
		// UX-P1/UX-LAYOUT: bounded scrollable main pane so 900x600 stays reachable.
		ui.Row(c).Grow(1).MinHeight(0).Gap(float32(tok.Spacing12)).AlignItems(ui.Stretch).Children(func() {
			ui.Column(c).Width(float32(tok.Spacing48)*4.4).FillHeight().Padding(float32(tok.Spacing8)).Radius(float32(tok.PanelRadius)).
				Background(tok.Surface).Border(1, tok.Border).Gap(float32(tok.Spacing4)).Children(func() {
				components.Sidebar(c).Children(func() {
					components.SidebarSection(c, "资料与工具", func() {
						navItems := []struct {
							tool  Tool
							label string
							icon  *ui.SVG
						}{
							{ToolPantry, "储藏室货架", lucide.FlaskConical()},
							{ToolSOP, "标准方案库", lucide.BookOpen()},
							{ToolPlate, "孔板速排", lucide.Grid2x2()},
							{ToolDilution, "实验台速算", lucide.Calculator()},
						}
						for _, item := range navItems {
							item := item
							if components.SidebarItem(c, item.label, item.icon, w.Active == item.tool).Clicked() {
								w.Active = item.tool
								if w.PantryRetry == nil {
									w.Error, w.Status = "", ""
								}
							}
						}
					})
				})
			})
			ui.Scroll(c).Grow(1).FillWidth().MinWidth(0).Children(func() {
				ui.Column(c).FillWidth().Padding(float32(tok.Spacing16)).Radius(float32(tok.PanelRadius)).
					Gap(float32(tok.Spacing12)).Background(tok.Surface).Border(1, tok.Border).Children(func() {
					switch w.Active {
					case ToolPantry:
						pantryView(c, w, tok)
					case ToolSOP:
						sopLibraryView(c, w, tok)
					case ToolPlate:
						plateView(c, w, tok)
					case ToolDilution:
						dilutionView(c, w, tok)
					}
					if w.Error != "" {
						ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
							Radius(float32(tok.ControlRadius)).Background(tok.DangerBg).Border(1, tok.Danger).
							AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
							ui.Text(c, w.Error).FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText).Grow(1).MinWidth(0)
							if w.PantryRetry != nil && components.Button(c, "重试", components.ButtonSecondary).Clicked() {
								w.PantryRetry()
							}
						})
					} else if w.Status != "" {
						ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
							Radius(float32(tok.ControlRadius)).Background(tok.SuccessBg).Border(1, tok.Success.Alpha(.4)).
							AlignItems(ui.Center).Children(func() {
							ui.Text(c, w.Status).FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.SuccessText)
						})
					}
				})
			})
		})
	})
	if w.Active == ToolPlate && w.plateView != nil {
		w.plateView.RenderPlateOverlay(c)
	}
	if w.Active == ToolSOP && w.sopView != nil {
		w.sopView.RenderPlateOverlay(c)
	}
	if w.Active == ToolPantry {
		w.renderPantryDetailDialog(c)
		w.renderPantryBoxDialog(c)
	}
}

func pantryDate(value string) string {
	if value == "" {
		return "—"
	}
	return value
}

func pantryDays(days *uint32) string {
	if days == nil {
		return "—"
	}
	return strconv.FormatUint(uint64(*days), 10) + "天"
}

func groupPantryItems(items []runs.PantryItem) (near, other []runs.PantryItem) {
	for _, item := range items {
		if item.Status == "near_expiry" {
			near = append(near, item)
		} else {
			other = append(other, item)
		}
	}
	return near, other
}

func pantryStatusText(exhausted, opened bool) string {
	if exhausted {
		return "已用尽"
	}
	if opened {
		return "已开封"
	}
	return "未开封"
}

func pantryItemStatus(item runs.PantryItem) string {
	switch item.Status {
	case "near_expiry":
		return "临期"
	case "expired":
		return "已过期"
	case "exhausted":
		return "已用尽"
	case "opened":
		return "已开封"
	default:
		return "未开封"
	}
}
