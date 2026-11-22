package main

import (
	"asterlab-mygo-prototype/internal/assays"
	"asterlab-mygo-prototype/internal/assets"
	"asterlab-mygo-prototype/internal/asterdoc"
	"asterlab-mygo-prototype/internal/blocks"
	"asterlab-mygo-prototype/internal/blockui"
	"asterlab-mygo-prototype/internal/designer"
	"asterlab-mygo-prototype/internal/diag"
	"asterlab-mygo-prototype/internal/notebook"
	"asterlab-mygo-prototype/internal/notebook/record"
	"asterlab-mygo-prototype/internal/protocol"
	"asterlab-mygo-prototype/internal/resultgrid"
	"asterlab-mygo-prototype/internal/runui"
	"asterlab-mygo-prototype/internal/runworkspace"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/state"
	"asterlab-mygo-prototype/internal/theme"
	"asterlab-mygo-prototype/internal/tools"
	"asterlab-mygo-prototype/internal/updater"
	"asterlab-mygo-prototype/internal/vault"
	"asterlab-mygo-prototype/internal/workflow"
	"asterlab-mygo-prototype/internal/workflowtree"
	"asterlab-mygo-prototype/modules/runs"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	mygo "github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/richtext"
	"github.com/oneo-me/onevra/source/components"
)

type sample struct{ id, value string }

type stepPropDraft struct {
	Title      string
	Assay      string
	Timepoints string
	Tasks      []canvasTaskDraft
}

type pdfExportResult struct {
	path     string
	err      error
	canceled bool
}

type app struct {
	page    *string
	step    *string
	samples []sample

	runSvc                     runs.RunService
	designSvc                  runs.DesignService
	timeBudgetSvc              runs.TimeBudgetService
	timeBudgetErr              error
	snap                       runs.RunSnapshot
	syncErr                    string
	batchView                  string
	asterDoc                   *asterdoc.FrozenAsterDoc
	altHoverPreviewQuery       string
	asterErr                   string
	pdfExporting               bool
	pdfExportResults           chan pdfExportResult
	latestRelease              *updater.ReleaseInfo
	updateBusy                 bool
	showFrozenDoc              bool
	designDoc                  *runs.AsterDocSnapshot
	designView                 *asterdoc.View
	assetRoots                 map[string]string
	runAssets                  []runs.Asset
	runViewKinds               map[string]runworkspace.ViewKind
	taskGrids                  map[string]*resultgrid.Grid
	taskDrafts                 map[string]runui.Draft
	designDocErr               string
	designLoadErr              string
	designLoadID               string
	designDocLoadFailed        bool
	retryDesignDoc             func()
	pendingRunID               string
	projects                   []runs.ProjectSummary
	protocols                  []runs.ProtocolSummary
	readiness                  *runs.AsterDocAssessment
	readinessLoading           bool
	readinessErr               string
	preflightOpen              bool
	preflightRisk              bool
	newDesignOpen              bool
	newDesignTargetDate        string
	addModuleOpen              bool
	importOpen                 bool
	importPath                 string
	importTitle                string
	importErr                  string
	designPaths                map[string]string
	diskMirrorMtimes           map[string]time.Time
	externalSyncBusy           atomic.Bool
	designCache                map[string]runs.AsterDocSnapshot
	secondaryViews             map[string]*asterdoc.View
	runSummaries               []runs.RunSummary
	designHistory              []runs.AsterDocSnapshot
	designFuture               []runs.AsterDocSnapshot
	asterDraftUndo             [][]byte
	asterDraftRedo             [][]byte
	asterDraftLast             []byte
	asterDraftSuspend          bool
	asterDraftSelection        string
	designTab                  int
	designSplitW               float32
	designSrcSplitPct          float32
	asterSrcEditor             *editor.State
	asterSrcLoadedRev          uint64
	asterSrcLoadedSrc          string
	designCanvas               *workflow.Editor
	designCanvasSource         string
	designCanvasFitPending     bool
	designCanvasBoundsW        float32
	designCanvasBoundsH        float32
	designInspectorOpen        bool
	designInspectorUserToggled bool
	designLayouts              map[string]map[string]workflow.Position
	designHierarchy            *workflowtree.Tree
	designHierarchySource      string
	designHierarchyErr         string
	blockPreviewCache          map[string]workflowtree.BlockDefinition
	designPreviewExpanded      bool
	designSubflow              *workflow.Editor
	designSubflowParent        string
	designSubflowTitle         string
	designSubflowSelected      string
	designSubflowFitted        bool
	designSubflowBoundsW       float32
	designSubflowBoundsH       float32
	designSelected             string
	canvasTitle                string
	canvasAssay                string
	canvasPath                 string
	canvasTimepoints           string
	canvasSelLoaded            string
	canvasSyncing              bool
	confirmDeleteStep          string
	canvasTasks                []canvasTaskDraft
	canvasChecks               []canvasCheckDraft
	canvasUses                 []canvasUseDraft
	canvasUsePick              string
	stepDrafts                 map[string]stepPropDraft
	stepPropSaved              string
	noteDraft                  string
	noteRunID                  string
	noteDirty                  bool
	noteRecovered              bool
	noteDrafts                 map[string]notebookDraft
	noteState                  string
	noteErr                    string
	createRunErr               string
	linkErr                    string
	linkErrTask                string
	assetErr                   string
	assetErrTask               string
	assetErrRetry              func()
	linkDialogOpen             bool
	linkDialogTask             string
	linkDialogAssetID          string
	linkDialogWell             string
	linkDialogTimepoint        string
	linkDialogFOV              string
	linkDialogWells            []string
	linkDialogTimepoints       []string
	linkDialogErr              string
	// recKey/recProj/recTasks/recErr memoize Core's single read-only Record
	// projection: key is runID|rev|assets|fov|view, never the prose. The
	// Notebook board, the protocol archive and the Run history all read
	// from this memo, so the same Run can never disagree with itself.
	recKey   string
	recProj  *record.Projection
	recTasks []record.TaskStatus
	recErr   string
	recAt    time.Time
	// signConfirm opens the sign-off confirmation; deriveEv/deriveDesign
	// hold Core's derived draft for review before SaveDesign persists it.
	signConfirm   bool
	signErr       string
	deriveDesign  *runs.DerivedDesign
	deriveEv      *record.DerivedEvidence
	deriveErr     string
	deriveConfirm bool

	noteMode     int
	noteEditMode int
	noteDoc      *richtext.Document
	noteAster    *asterdoc.View
	shell               *shell.Shell
	ws                  *runui.Workspace
	runWS               *runworkspace.State
	runWSVM             *runworkspace.RunWorkspaceViewModel
	runWSKey            string
	designer            *designer.Designer
	pendingDesignID     string
	confirmDesignSwitch bool
	windowReady         bool
	guard               state.StaleGuard

	designSnap *runs.DesignSnapshot

	wf     *workflow.Editor
	blocks []blocks.Instance

	opCounter atomic.Uint64

	selectedTask string
	tools        *tools.Workspace
	// timersPoll throttles the global timer-strip refresh; see pollTimers.
	timersPoll time.Time
	// runLinkArg carries a startup-URL deep link (asterlab://…) handed to
	// the exe by the OS; the automation env hook stays as fallback.
	runLinkArg string

	// Obsidian-style local Vault + shared single-source LiveDocuments
	vault             *vault.Vault
	vaultIndex        *vault.VaultIndex
	docStore          *vault.DocStore
	liveViews         map[string]*asterdoc.View
	liveEditors       map[string]*editor.State
	liveEditorSynced  map[string]string
	liveTitleDrafts   map[string]*string
	liveDateDrafts    map[string]*string
	pendingFindQuery  map[string]string
	lastCompanionKey  string
	cachedAssays      map[string]cachedAssayResult
	scrollToStepID    string
	newExpOpen       bool
	newExpFolder     string
	newExpNewFolder  string
	newExpTitle      string
	newExpDate       string
	newExpTemplate   string
	newExpErr        string
	newExpNudgeSeq   int
}

func (a *app) nextOp() string {
	n := a.opCounter.Add(1)
	return fmt.Sprintf("op-%d-%d", time.Now().UnixNano(), n)
}

func stepModuleRefs(designData, stepID string) map[string]bool {
	refs := map[string]bool{}
	if stepID == "" {
		return refs
	}
	var design struct {
		MyGo struct {
			StepUses map[string][]struct {
				Ref  string `json:"ref"`
				Role string `json:"role"`
			} `json:"stepUses"`
		} `json:"mygo"`
	}
	if json.Unmarshal([]byte(designData), &design) != nil {
		return refs
	}
	for _, use := range design.MyGo.StepUses[stepID] {
		if use.Ref != "" {
			refs[use.Ref] = true
		}
	}
	return refs
}

func sdkModulesFromDesign(designData string) []map[string]any {
	var design struct {
		AsterLab struct {
			Modules []map[string]any `json:"modules"`
		} `json:"asterlab"`
	}
	if json.Unmarshal([]byte(designData), &design) != nil {
		return nil
	}
	return design.AsterLab.Modules
}

func frozenAssayEndpoints(designData string) map[string][2]string {
	out := map[string][2]string{}
	var design struct {
		Assays []struct {
			ID       string `json:"id"`
			Endpoint string `json:"endpoint"`
			Unit     string `json:"unit"`
		} `json:"assays"`
	}
	if json.Unmarshal([]byte(designData), &design) != nil {
		return out
	}
	for _, assay := range design.Assays {
		out[assay.ID] = [2]string{assay.Endpoint, assay.Unit}
	}
	return out
}

func toRunView(s runs.RunSnapshot) runui.RunView {
	endpoints := frozenAssayEndpoints(s.DesignData)
	v := runui.RunView{
		Title: s.Title, Status: s.Status, Revision: s.Revision,
		DesignRevision: s.DesignRevision,
		Measurements: make([]runui.Measurement, 0, len(s.Measurements)),
		Events:       make([]runui.Event, 0, len(s.Events)),
	}
	if s.AsterDoc != nil {
		v.AsterDocRevision = s.AsterDoc.Revision
	}
	for _, t := range s.Tasks {
		endpoint, unit := "", ""
		if pair, ok := endpoints[t.AssayID]; ok {
			endpoint, unit = pair[0], pair[1]
		}
		task := runui.Task{
			Key: t.Key, Title: t.Title, Timepoint: t.Timepoint, Status: t.Status,
			AssayID: t.AssayID, DesignStepID: t.DesignStepID,
			ProtocolVersionID: t.ProtocolVersionID, Instruction: t.Instruction,
			Endpoint: endpoint, Unit: unit,
		}
		var params []struct {
			Key string `json:"key"`
			Label string `json:"label"`
			Name string `json:"name"`
			Value any `json:"value"`
			Unit string `json:"unit"`
		}
		if json.Unmarshal([]byte(t.ParametersJSON), &params) == nil {
			for _, p := range params {
				label := p.Label
				if label == "" { label = p.Name }
				if label == "" { label = p.Key }
				value := ""
				if p.Value != nil { value = fmt.Sprint(p.Value) }
				task.Parameters = append(task.Parameters, runui.FrozenParameter{
					Key: p.Key, Label: label, Value: value, Unit: p.Unit,
				})
			}
		}
		for _, check := range t.Checks {
			task.Checks = append(task.Checks, runui.TaskCheck{
				CheckID: check.CheckID, Label: check.Label,
				Instruction: check.Instruction, Blocking: check.Blocking,
			})
		}
		v.Tasks = append(v.Tasks, task)
	}
	for _, m := range s.Measurements {
		v.Measurements = append(v.Measurements, runui.Measurement{SampleID: m.SampleID, Endpoint: m.Endpoint, Value: m.Value, Unit: m.Unit})
	}
	if audit := s.CreationAudit; audit != nil {
		v.Audit = &runui.AuditInfo{ActorID: audit.ActorID, RecordedAt: audit.RecordedAt,
			Decision: audit.Decision, Warnings: audit.Warnings}
	}
	for _, e := range s.Events {
		event := runui.Event{Kind: e.EventType, Message: e.Content,
			TaskKey: e.TaskKey, StepPath: e.ProtocolStepPath, AssayID: e.AssayID,
			Timepoint: e.Timepoint}
		for _, check := range e.Checks {
			event.Checks = append(event.Checks, runui.CheckAck{CheckID: check.CheckID, Status: check.Status})
		}
		v.Events = append(v.Events, event)
	}
	return v
}

func (a *app) applySnap(snap runs.RunSnapshot) {
	switchingRun := a.snap.RunID != "" && snap.RunID != "" && a.snap.RunID != snap.RunID
	if switchingRun {
		// StaleGuard tracks revisions within a single Run; switching Runs resets it so
		// returning to a lower-revision Run is not dropped as stale.
		a.guard = state.StaleGuard{}
	}
	if !a.guard.Accept(snap.Revision) {
		a.syncErr = fmt.Sprintf("忽略过期快照（收到 rev %d，当前已应用 rev %d）", snap.Revision, a.guard.Latest())
		return
	}
	var newSnap runs.RunSnapshot
	if switchingRun {
		newSnap = snap
		a.syncErr = ""
	} else {
		var err error
		newSnap, err = state.ApplySnapshot(a.snap, snap)
		if err != nil {
			a.syncErr = fmt.Sprintf("快照应用失败：%v", err)
			return
		}
		if a.snap.RunID != "" && a.snap.RunID == snap.RunID {
			if err := state.ConflictIfDifferent(a.snap, snap); err != nil {
				a.syncErr = fmt.Sprintf("检测到同修订快照冲突（rev %d）：%v", snap.Revision, err)
			} else {
				a.syncErr = ""
			}
		} else {
			a.syncErr = ""
		}
	}
	previousRunID := a.snap.RunID
	if previousRunID != "" && previousRunID != snap.RunID {
		a.clearTaskRecording()
		a.clearAssetError()
		a.linkErr = ""
		a.linkErrTask = ""
		a.selectedTask = ""
		if a.ws != nil {
			a.ws.Selected = ""
		}
	}
	a.snap = newSnap
	a.loadFrozenAsterDoc()
	if previousRunID != newSnap.RunID {
		a.loadRunAssets()
	}
	a.refreshRunWorkspace()
	a.rememberRun(newSnap)
	if a.ws != nil {
		view := toRunView(a.snap)
		a.ws.View = view
		a.ws.SyncChecks()
		a.ws.Notebook = notebook.Build(a.snap)
		a.ws.Draft = runui.NewDraft(view)
		if a.ws.Selected == "" {
			a.ws.Selected = currentRunTask(a.snap.Tasks).Key
		}
		a.selectedTask = a.ws.Selected
		if len(a.snap.Tasks) == 0 && len(a.snap.Measurements) == 0 {
			a.ws.State = runui.StateEmpty
		} else {
			a.ws.State = runui.StateReady
		}
		a.prepareGrid()
		a.selectTaskFromEnv()
	}
	if a.noteRunID != a.snap.RunID {
		a.loadNotebook()
	}
	a.syncRunEnvelopeMirror()
	a.syncShellExperiments()
}

func assetRootID(designID string) string {
	return "design-data-" + strings.TrimSpace(designID)
}

func localAssetFromRun(asset runs.Asset) assets.Asset {
	return assets.Asset{
		ID: asset.ID, RootID: asset.RootID, RelativePath: asset.RelativePath,
		MediaType: assets.MediaType(asset.MediaType), Size: asset.Size,
		ModTime: asset.ModTime, SHA256: asset.SHA256,
		Availability: assets.Availability(asset.Availability),
		Link:         assets.Link{RunID: asset.Link.RunID, TaskKey: asset.Link.TaskKey, SampleID: asset.Link.SampleID},
		LinkedAt:     asset.LinkedAt, ActorID: asset.ActorID,
	}
}

func runAssetFromLocal(asset assets.Asset) runs.Asset {
	return runs.Asset{
		ID: asset.ID, RootID: asset.RootID, RelativePath: asset.RelativePath,
		MediaType: string(asset.MediaType), Size: asset.Size, ModTime: asset.ModTime,
		SHA256: asset.SHA256, Availability: string(asset.Availability),
		Link:     runs.AssetLink{RunID: asset.Link.RunID, TaskKey: asset.Link.TaskKey, SampleID: asset.Link.SampleID},
		LinkedAt: asset.LinkedAt, ActorID: asset.ActorID,
	}
}

func localAssetFromView(asset runworkspace.LinkedAsset) assets.Asset {
	availability := assets.Unavailable
	if asset.Available {
		availability = assets.Available
	}
	return assets.Asset{
		ID: asset.AssetID, RootID: asset.RootID, RelativePath: asset.RelativePath,
		MediaType: assets.MediaType(asset.MediaType), Size: asset.Size,
		ModTime: asset.ModTime, SHA256: asset.SHA256, Availability: availability,
		Link: assets.Link{RunID: asset.LinkedTo.RunID, TaskKey: asset.LinkedTo.TaskKey, SampleID: asset.LinkedTo.SampleID},
	}
}

func (a *app) setAssetError(taskKey, msg string, retry func()) {
	a.assetErr = msg
	a.assetErrTask = taskKey
	a.assetErrRetry = retry
}

func (a *app) clearAssetError() {
	a.assetErr = ""
	a.assetErrTask = ""
	a.assetErrRetry = nil
}

func (a *app) loadRunAssets() {
	if a.snap.RunID == "" || a.runSvc == nil {
		a.runAssets = nil
		return
	}
	linked, err := a.runSvc.ListAssetLinks(context.Background(), a.snap.RunID)
	if err != nil {
		a.setAssetError("", "加载关联文件列表失败："+state.MapError(err).Error(), a.loadRunAssets)
		return
	}
	if a.assetErrTask == "" {
		a.clearAssetError()
	}
	a.runAssets = linked
}

func (a *app) runAssetsForTask(taskKey string) []runworkspace.LinkedAsset {
	out := []runworkspace.LinkedAsset{}
	roots := assets.Roots(a.assetRoots)
	for _, linked := range a.runAssets {
		if linked.Link.TaskKey != taskKey {
			continue
		}
		local := localAssetFromRun(linked)
		path, err := roots.Resolve(local)
		available := false
		if err == nil {
			available, _ = assets.Verify(local, path, false)
		}
		out = append(out, runworkspace.LinkedAsset{
			AssetID: linked.ID, RootID: linked.RootID, RelativePath: linked.RelativePath,
			MediaType: linked.MediaType, Size: linked.Size, ModTime: linked.ModTime,
			SHA256: linked.SHA256, Available: available,
			WellID: linked.Link.WellID, Timepoint: linked.Link.Timepoint, FOVIndex: linked.Link.FOVIndex,
			LinkedTo: runworkspace.AssetLink{
				RunID: linked.Link.RunID, TaskKey: linked.Link.TaskKey, SampleID: linked.Link.SampleID,
			},
		})
	}
	return out
}

func (a *app) openRunAsset(asset runworkspace.LinkedAsset) error {
	local := localAssetFromView(asset)
	path, err := assets.Roots(a.assetRoots).Resolve(local)
	if err != nil {
		return err
	}
	if ok, reason := assets.Verify(local, path, true); !ok {
		return fmt.Errorf("文件校验失败：%s", reason)
	}
	go mygo.Shell.OpenPath(path)
	return nil
}

func (a *app) tryOpenRunAsset(taskKey string, asset runworkspace.LinkedAsset) error {
	if err := a.openRunAsset(asset); err != nil {
		a.setAssetError(taskKey, "无法打开文件："+err.Error(), func() {
			_ = a.tryOpenRunAsset(taskKey, asset)
		})
		if a.shell != nil {
			a.shell.Invalidate()
		}
		return err
	}
	a.clearAssetError()
	if a.shell != nil {
		a.shell.Invalidate()
	}
	return nil
}

func (a *app) tryRelocateRunAsset(taskKey string, asset runworkspace.LinkedAsset) error {
	if err := a.relocateRunAsset(asset); err != nil {
		a.setAssetError(taskKey, "文件重新定位失败："+err.Error(), func() {
			_ = a.tryRelocateRunAsset(taskKey, asset)
		})
		if a.shell != nil {
			a.shell.Invalidate()
		}
		return err
	}
	a.clearAssetError()
	if a.shell != nil {
		a.shell.Invalidate()
	}
	return nil
}

func (a *app) relocateRunAsset(asset runworkspace.LinkedAsset) error {
	local := localAssetFromView(asset)
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.FocusedWindow(), Title: "重新定位本地数据根目录",
		DefaultPath: a.assetRoots[asset.RootID], Directory: true,
	})
	if err != nil || len(paths) == 0 {
		return err
	}
	roots := assets.Roots(a.assetRoots)
	roots[asset.RootID] = paths[0]
	path, err := roots.Resolve(local)
	if err != nil {
		return err
	}
	if ok, reason := assets.Verify(local, path, true); !ok {
		return fmt.Errorf("文件校验失败：%s", reason)
	}
	if a.assetRoots == nil {
		a.assetRoots = map[string]string{}
	}
	a.assetRoots[asset.RootID] = paths[0]
	a.saveSession()
	return nil
}

func validateAssetCoordinates(well, timepoint, fov string, wells, timepoints []string) (runs.AssetLink, error) {
	valid := func(value string, allowed []string) bool {
		for _, option := range allowed {
			if value == option {
				return true
			}
		}
		return false
	}
	if !valid(well, wells) {
		return runs.AssetLink{}, fmt.Errorf("未知孔位：%s", well)
	}
	if !valid(timepoint, timepoints) {
		return runs.AssetLink{}, fmt.Errorf("未知时间点：%s", timepoint)
	}
	index, err := strconv.ParseUint(strings.TrimSpace(fov), 10, 32)
	if err != nil || index < 1 {
		return runs.AssetLink{}, fmt.Errorf("FOV 必须是大于等于 1 的整数")
	}
	return runs.AssetLink{WellID: well, Timepoint: timepoint, FOVIndex: uint32(index)}, nil
}

func linkPickedAssets(paths []string, coordinate runs.AssetLink, link func(string, runs.AssetLink) error) error {
	for _, path := range paths {
		if err := link(path, coordinate); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) openAssetLinkForm(taskKey string, prefill runworkspace.LinkedAsset) {
	a.linkDialogTask = taskKey
	a.linkDialogAssetID = strings.TrimSpace(prefill.AssetID)
	a.linkDialogWells, a.linkDialogTimepoints = a.assetCoordinateOptions(taskKey)
	a.linkDialogWell = strings.TrimSpace(prefill.WellID)
	a.linkDialogTimepoint = strings.TrimSpace(prefill.Timepoint)
	a.linkDialogFOV = strconv.FormatUint(uint64(prefill.FOVIndex), 10)
	if a.linkDialogWell == "" && len(a.linkDialogWells) > 0 {
		a.linkDialogWell = a.linkDialogWells[0]
	}
	if a.linkDialogTimepoint == "" {
		for _, task := range a.snap.Tasks {
			if task.Key == taskKey {
				a.linkDialogTimepoint = strings.TrimSpace(task.Timepoint)
				break
			}
		}
	}
	if a.linkDialogTimepoint == "" && len(a.linkDialogTimepoints) > 0 {
		a.linkDialogTimepoint = a.linkDialogTimepoints[0]
	}
	if a.linkDialogFOV == "0" || a.linkDialogFOV == "" {
		a.linkDialogFOV = "1"
	}
	a.linkDialogErr = ""
	a.linkDialogOpen = true
}

func designPlateWells(raw, assayID string) []string {
	var doc struct {
		MyGo struct {
			Instances map[string][]blocks.Instance `json:"instances"`
			Assays    map[string]struct {
				PlateInstanceID string `json:"plateInstanceId"`
			} `json:"assays"`
		} `json:"mygo"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return nil
	}
	plateID := doc.MyGo.Assays[assayID].PlateInstanceID
	keys := make([]string, 0, len(doc.MyGo.Instances))
	for key := range doc.MyGo.Instances {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var plate *blocks.Instance
	for _, key := range keys {
		for i := range doc.MyGo.Instances[key] {
			instance := &doc.MyGo.Instances[key][i]
			if instance.Type != blocks.TypePlate96 && instance.Type != blocks.TypePlate12 {
				continue
			}
			if plateID == "" || instance.ID == plateID {
				plate = instance
				break
			}
		}
		if plate != nil {
			break
		}
	}
	if plate == nil {
		return nil
	}
	var dimensions struct {
		Rows int `json:"rows"`
		Cols int `json:"cols"`
	}
	if json.Unmarshal(plate.RawProps, &dimensions) != nil {
		return nil
	}
	if dimensions.Rows == 0 || dimensions.Cols == 0 {
		if plate.Type == blocks.TypePlate12 {
			dimensions.Rows, dimensions.Cols = 3, 4
		} else {
			dimensions.Rows, dimensions.Cols = 8, 12
		}
	}
	wells := make([]string, 0, dimensions.Rows*dimensions.Cols)
	for row := 0; row < dimensions.Rows && row < 26; row++ {
		for column := 1; column <= dimensions.Cols; column++ {
			wells = append(wells, string(rune('A'+row))+strconv.Itoa(column))
		}
	}
	return wells
}

func frozenAssayTimepoints(raw, assayID string) []string {
	var doc struct {
		Assays []struct {
			ID         string   `json:"id"`
			Timepoints []string `json:"timepoints"`
		} `json:"assays"`
		MyGo struct {
			Assays map[string]assays.Design `json:"assays"`
		} `json:"mygo"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return nil
	}
	if tps := doc.MyGo.Assays[assayID].Timepoints; len(tps) > 0 {
		return tps
	}
	for _, assay := range doc.Assays {
		if assay.ID == assayID && len(assay.Timepoints) > 0 {
			return assay.Timepoints
		}
	}
	return nil
}

func (a *app) assetCoordinateOptions(taskKey string) ([]string, []string) {
	selectedAssay, stepID := "", ""
	for _, task := range a.snap.Tasks {
		if task.Key == taskKey {
			selectedAssay, stepID = task.AssayID, task.DesignStepID
			break
		}
	}

	timepoints := make([]string, 0, len(a.snap.Tasks))
	seenTimes := map[string]bool{}
	addTimepoint := func(raw string) {
		value := strings.TrimSpace(raw)
		if value != "" && !seenTimes[value] {
			seenTimes[value] = true
			timepoints = append(timepoints, value)
		}
	}
	for _, task := range a.snap.Tasks {
		if task.AssayID != selectedAssay {
			continue
		}
		addTimepoint(task.Timepoint)
	}

	// QA-HUVEC F2: capture slots come from the Run's frozen plan only. The
	// Design page live draft (a.designSnap) is never consulted: a design
	// edited after freezing must not move an executing Run's slots (UX-STATE).
	designData := a.snap.DesignData
	if selectedAssay != "" {
		for _, value := range frozenAssayTimepoints(designData, selectedAssay) {
			addTimepoint(value)
		}
	}
	wells := frozenPlannedWells(a.frozenAsterSource(), selectedAssay, stepID)
	if len(wells) == 0 {
		wells = designPlateWells(designData, selectedAssay)
	}
	if len(wells) == 0 && selectedAssay != "" {
		if schema, _, err := assays.BuildSchema([]byte(designData), selectedAssay, ""); err == nil &&
			(schema.Layout == "plate96" || schema.Layout == "plate12") {
			for _, row := range schema.RowLabels {
				for _, column := range schema.ColumnLabels {
					wells = append(wells, row+column)
				}
			}
		}
	}
	return wells, timepoints
}

// frozenAsterSource returns the exact AsterDoc text frozen on the open Run.
// Empty when the Run has no frozen document: callers then fall back to the
// frozen design JSON, never to the live draft.
func (a *app) frozenAsterSource() string {
	if a.asterDoc != nil {
		return a.asterDoc.Source
	}
	return ""
}

// frozenPlannedWells resolves capture slots in fill order from the Run's
// frozen AsterDoc plan, never the Design-page live mygo instance (QA-HUVEC
// F2, UX-STATE). Plates contribute their assigned group wells (planned,
// never full capacity); containers contribute sample_map keys (2D with a
// plan) or Lane<N> slots (1D, e.g. petri/tubes). Science is never inferred
// from filenames: unlisted wells simply have no slots. Wells from a second
// plate/container qualify as "sourceID:well" so the same A1 on two plates
// never collides; a single source keeps bare wells.
func frozenPlannedWells(asterSource, assayID, stepID string) []string {
	assayID, stepID = strings.TrimSpace(assayID), strings.TrimSpace(stepID)
	if strings.TrimSpace(asterSource) == "" || assayID == "" {
		return nil
	}
	doc, err := asterdoc.Parse(asterSource)
	if err != nil {
		return nil
	}
	uses := map[string]bool{}
	for _, m := range doc.Modules {
		if strings.TrimSpace(m.ID) != stepID {
			continue
		}
		for _, ref := range stepContainerRefs(m.Data) {
			uses[ref] = true
		}
	}
	type source struct {
		id    string
		wells []string
	}
	sources := []source{}
	singleContainer := 0
	for _, m := range doc.Modules {
		if m.Kind == "container" {
			singleContainer++
		}
	}
	for _, m := range doc.Modules {
		switch m.Kind {
		case "plate":
			if m.Plate == nil {
				continue
			}
			if assay := strings.TrimSpace(m.Plate.Assay); assay != "" && assay != assayID {
				continue
			}
			planned := map[string]bool{}
			for _, wells := range m.Plate.Groups {
				for _, well := range wells {
					if well = strings.ToUpper(strings.TrimSpace(well)); well != "" {
						planned[well] = true
					}
				}
			}
			if len(planned) == 0 {
				for well := range m.Plate.Samples {
					if well = strings.ToUpper(strings.TrimSpace(well)); well != "" {
						planned[well] = true
					}
				}
			}
			if len(planned) == 0 {
				continue
			}
			sources = append(sources, source{id: m.ID, wells: sortedWells(planned)})
		case "container":
			if !uses[m.ID] && len(containerSampleMap(m.Data)) == 0 && singleContainer != 1 {
				continue
			}
			wells := containerPlannedWells(m.Data)
			if len(wells) == 0 {
				continue
			}
			sources = append(sources, source{id: m.ID, wells: wells})
		}
	}
	if len(sources) == 0 {
		return nil
	}
	if len(sources) == 1 {
		return sources[0].wells
	}
	out := []string{}
	for _, s := range sources {
		for _, well := range s.wells {
			out = append(out, s.id+":"+well)
		}
	}
	return out
}

// stepContainerRefs reads the container refs a frozen step binds through
// uses (ref + role), block_refs, or inline container_grid attachment blocks.
func stepContainerRefs(data map[string]any) []string {
	refs := []string{}
	if uses, _ := data["uses"].([]any); len(uses) > 0 {
		for _, raw := range uses {
			item, _ := raw.(map[string]any)
			if ref, _ := item["ref"].(string); strings.TrimSpace(ref) != "" {
				refs = append(refs, strings.TrimSpace(ref))
			}
		}
	}
	if blockRefs, _ := data["block_refs"].([]any); len(blockRefs) > 0 {
		for _, raw := range blockRefs {
			if ref, _ := raw.(string); strings.TrimSpace(ref) != "" {
				refs = append(refs, strings.TrimSpace(ref))
			}
		}
	}
	if blocks, _ := data["blocks"].([]any); len(blocks) > 0 {
		for _, raw := range blocks {
			bm, _ := raw.(map[string]any)
			if cref, _ := bm["container_ref"].(string); strings.TrimSpace(cref) != "" {
				refs = append(refs, strings.TrimSpace(cref))
			}
		}
	}
	return refs
}

// containerSampleMap returns the explicit well→sample plan of a generic
// container, if it declares one. No map means no planned 2D slots: capacity
// is never used as a plan.
func containerSampleMap(data map[string]any) map[string]string {
	out := map[string]string{}
	raw, _ := data["sample_map"].(map[string]any)
	for well, sample := range raw {
		well = strings.ToUpper(strings.TrimSpace(well))
		if well == "" {
			continue
		}
		out[well] = strings.TrimSpace(firstString(sample))
	}
	return out
}

func firstString(v any) string {
	switch value := v.(type) {
	case string:
		return value
	case []any:
		if len(value) > 0 {
			if s, _ := value[0].(string); s != "" {
				return s
			}
		}
	}
	return ""
}

// containerPlannedWells resolves one generic container's planned slots:
// sample_map keys for 2D labware with a plan, Lane<N> for 1D labware
// (petri, tube, rack, strip). A 2D format without a map declares nothing.
func containerPlannedWells(data map[string]any) []string {
	if plan := containerSampleMap(data); len(plan) > 0 {
		planned := make([]string, 0, len(plan))
		for well := range plan {
			planned = append(planned, well)
		}
		return sortedWellsSlice(planned)
	}
	format, _ := data["format"].(string)
	if isTwoDimensionalFormat(format) {
		return nil
	}
	n := containerWellCount(data)
	if n <= 0 || n > 4096 {
		return nil
	}
	wells := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		wells = append(wells, "Lane"+strconv.Itoa(i))
	}
	return wells
}

func isTwoDimensionalFormat(format string) bool {
	lower := strings.ToLower(strings.TrimSpace(format))
	if lower == "" {
		return true
	}
	for _, token := range []string{"well", "plate", "孔板", "孔"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	if i := strings.IndexAny(lower, "0123456789"); i == 0 {
		return true
	}
	return false
}

func containerWellCount(data map[string]any) int {
	switch v := data["wells"].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return 0
}

func splitWell(well string) (string, int) {
	i := len(well)
	for i > 0 && well[i-1] >= '0' && well[i-1] <= '9' {
		i--
	}
	n, _ := strconv.Atoi(well[i:])
	return well[:i], n
}

func wellLess(a, b string) bool {
	rowA, numA := splitWell(a)
	rowB, numB := splitWell(b)
	if rowA != rowB {
		return rowA < rowB
	}
	if numA != numB {
		return numA < numB
	}
	return a < b
}

func sortedWells(set map[string]bool) []string {
	return sortedWellsSlice(keysOf(set))
}

func sortedWellsSlice(wells []string) []string {
	out := append([]string(nil), wells...)
	sort.Slice(out, func(i, j int) bool { return wellLess(out[i], out[j]) })
	return out
}

func keysOf(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for well := range set {
		out = append(out, well)
	}
	return out
}

func (a *app) linkRunAsset(taskKey string, coordinate runs.AssetLink) error {
	if a.runSvc == nil || a.snap.RunID == "" || taskKey == "" {
		return fmt.Errorf("当前没有可关联文件的 Run 任务")
	}
	if strings.TrimSpace(a.snap.DesignID) == "" {
		return fmt.Errorf("当前 Run 没有关联设计")
	}
	rootID := assetRootID(a.snap.DesignID)
	rootDir := a.assetRoots[rootID]
	info, statErr := os.Stat(rootDir)
	if rootDir == "" || statErr != nil || !info.IsDir() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: mygo.FocusedWindow(), Title: "选择实验数据根目录", Directory: true,
		})
		if err != nil {
			return err
		}
		if len(paths) == 0 {
			return nil
		}
		rootDir = paths[0]
		if a.assetRoots == nil {
			a.assetRoots = map[string]string{}
		}
		a.assetRoots[rootID] = rootDir
		a.saveSession()
	}
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent: mygo.FocusedWindow(), Title: "关联实验文件", DefaultPath: rootDir, Multiple: true,
	})
	if err != nil || len(paths) == 0 {
		return err
	}
	coordinate.RunID, coordinate.TaskKey = a.snap.RunID, taskKey
	return a.linkAssetPaths(rootID, rootDir, paths, coordinate)
}

func (a *app) upsertRunAsset(saved runs.Asset) {
	for i, old := range a.runAssets {
		if (saved.ID != "" && old.ID == saved.ID) ||
			(old.RootID == saved.RootID && old.RelativePath == saved.RelativePath &&
				old.Link.RunID == saved.Link.RunID && old.Link.TaskKey == saved.Link.TaskKey &&
				old.Link.SampleID == saved.Link.SampleID) {
			a.runAssets[i] = saved
			return
		}
	}
	a.runAssets = append(a.runAssets, saved)
}

func (a *app) relabelRunAsset(_ string, coordinate runs.AssetLink) error {
	if a.runSvc == nil || a.snap.RunID == "" || strings.TrimSpace(a.linkDialogAssetID) == "" {
		return fmt.Errorf("当前没有可重设坐标的 Run 资产")
	}
	updated, err := a.runSvc.UpdateAssetLinkCoordinates(
		context.Background(),
		a.snap.RunID,
		a.linkDialogAssetID,
		coordinate.WellID,
		coordinate.Timepoint,
		coordinate.FOVIndex,
	)
	if err != nil {
		return state.MapError(err)
	}
	a.upsertRunAsset(updated)
	a.syncAssetSurfaces()
	return nil
}

// unlinkRunAsset drops one asset link through Core, then syncs every
// surface (strip, gallery, history projection) from the same updated
// asset list. The file itself is never touched (UX-P5, UX-STATE).
func (a *app) unlinkRunAsset(assetID string) error {
	if a.runSvc == nil || a.snap.RunID == "" || strings.TrimSpace(assetID) == "" {
		return fmt.Errorf("当前没有可解除关联的 Run 资产")
	}
	removed, err := a.runSvc.UnlinkAssetLink(context.Background(), a.snap.RunID, strings.TrimSpace(assetID))
	if err != nil {
		return state.MapError(err)
	}
	kept := a.runAssets[:0]
	for _, existing := range a.runAssets {
		if existing.ID != removed.ID && existing.ID != strings.TrimSpace(assetID) {
			kept = append(kept, existing)
		}
	}
	a.runAssets = kept
	a.syncAssetSurfaces()
	return nil
}

// syncAssetSurfaces rebuilds the workspace view model from the current
// snapshot and drops the shared record projection memo so the strip, the
// gallery and the history matrix all read the same updated asset list.
// Link writes never bump the Run revision, so the memo key would otherwise
// keep serving the pre-write projection.
func (a *app) syncAssetSurfaces() {
	if a.runWS != nil {
		a.refreshRunWorkspace()
	}
	a.invalidateRecord()
}

// linkAssetPaths validates every selected path against rootDir before saving
// any link so an out-of-bounds or unreadable file leaves Core untouched.
func (a *app) linkAssetPaths(rootID, rootDir string, paths []string, coordinate runs.AssetLink) error {
	staged := make([]runs.Asset, 0, len(paths))
	for _, path := range paths {
		asset, err := assets.NewLink(rootID, rootDir, path, assets.Link{
			RunID: coordinate.RunID, TaskKey: coordinate.TaskKey,
		}, "local-user", true)
		if err != nil {
			return err
		}
		savedAsset := runAssetFromLocal(asset)
		savedAsset.Link.WellID = coordinate.WellID
		savedAsset.Link.Timepoint = coordinate.Timepoint
		savedAsset.Link.FOVIndex = coordinate.FOVIndex
		staged = append(staged, savedAsset)
	}
	for _, savedAsset := range staged {
		saved, err := a.runSvc.SaveAssetLink(context.Background(), runs.AssetLinkInput{Asset: savedAsset})
		if err != nil {
			return state.MapError(err)
		}
		a.upsertRunAsset(saved)
	}
	a.syncAssetSurfaces()
	return nil
}

func (a *app) plannedDocFOV() int {
	if a.asterDoc != nil {
		if n := runworkspace.FOVPerWellForDoc(a.asterDoc.Source); n > 0 {
			return n
		}
	}
	return 0
}

func (a *app) recordFOV() int {
	return a.plannedDocFOV()
}

// recordBlind reports whether the Run is viewed under blinding. The Run
// blind switch is the single mask source; the projection carries the mask.
func (a *app) recordBlind() bool {
	return a.runWS != nil && a.runWS.Bench != nil && a.runWS.Bench.Blind
}

// Notebook board, the protocol archive and the Run history all project from
// this memo. The timer watermark is the fetch instant; the memo moves only
// with the key, so frames never call Core.
func (a *app) fetchRecord() (runworkspace.RecordKey, record.Projection, error) {
	if a.runSvc == nil || a.snap.RunID == "" {
		return "", record.Projection{}, fmt.Errorf("运行服务不可用")
	}
	view := ""
	if a.recordBlind() {
		view = "blind"
	}
	var assetsFP strings.Builder
	for _, asset := range a.runAssets {
		link := asset.Link
		fmt.Fprintf(&assetsFP, "%s@%s@%s%s%d;", asset.ID, asset.RelativePath, link.WellID, link.Timepoint, link.FOVIndex)
	}
	key := runworkspace.RecordKey(fmt.Sprintf("%s|%d|%s|%d|%s", a.snap.RunID, a.snap.Revision, assetsFP.String(), a.recordFOV(), view))
	if a.recProj != nil && a.recKey == string(key) {
		return key, *a.recProj, nil
	}
	proj, err := record.Fetch(context.Background(), a.runSvc, record.FetchInput{
		RunID: a.snap.RunID, View: view, FOVPerWell: uint32(a.recordFOV()),
		NowUTC: time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		a.recErr = state.MapError(err).Error()
		return key, record.Projection{}, err
	}
	cp := proj
	a.recKey = string(key)
	a.recProj = &cp
	a.recTasks = record.SnapshotTasks(a.snap.Tasks)
	a.recErr = ""
	a.recAt = time.Now()
	return key, cp, nil
}

// recordArchived reports whether Core's persisted record lifecycle already
// locked the notes. Only persistent Core state is authority; the prose
// trailer is history and never a lock.
func (a *app) recordArchived() bool {
	if a.recProj == nil {
		return false
	}
	return a.recProj.Archive.Status == "signed" || a.recProj.Archive.Status == "archived"
}

// invalidateRecord drops the shared projection memo so the next frame
// refetches from Core. The host calls it after sign/archive, which flips
// the persisted archive state without bumping the Run revision.
func (a *app) invalidateRecord() {
	a.recKey = ""
	a.recProj = nil
	a.recTasks = nil
	a.recErr = ""
	if a.runWS != nil {
		a.runWS.InvalidateRecord()
	}
}

// buildRunWorkspaceVM asks Core for the frozen Run hierarchy. Go only decodes
// the contract and renders it; no containment or task identity is rebuilt here.
func (a *app) buildRunWorkspaceVM() *runworkspace.RunWorkspaceViewModel {
	if a.runSvc == nil || a.snap.RunID == "" {
		return runworkspace.BuildViewModel(a.snap, a.asterDoc, runworkspace.Projection{})
	}
	projection := runworkspace.LoadProjection(a.snap, func() (*workflowtree.Tree, error) {
		raw, err := a.runSvc.WorkflowTree(context.Background(), a.snap.RunID)
		if err != nil {
			return nil, state.MapError(err)
		}
		return workflowtree.ParseJSON(raw)
	})
	return runworkspace.BuildViewModel(a.snap, a.asterDoc, projection)
}

// loadFrozenAsterDoc loads the exact document revision frozen on the Run. The
// latest live document is never substituted for a missing or broken revision.
func (a *app) refreshRunWorkspace() {
	vm := a.buildRunWorkspaceVM()
	key := a.snap.RunID + "|" + fmt.Sprintf("%d", a.snap.Revision)
	if a.asterDoc != nil {
		key += "|" + a.asterDoc.SourceHash
	}
	a.runWSVM = vm
	a.runWSKey = key
	if a.runWS != nil {
		a.runWS.SyncFrom(vm)
	}
}

func (a *app) loadFrozenAsterDoc() {
	a.asterDoc = nil
	a.asterErr = ""
	if a.snap.AsterDoc == nil {
		return
	}
	if a.designSvc == nil {
		a.asterErr = "设计服务不可用，无法读取冻结文档"
		return
	}
	ref := a.snap.AsterDoc
	if ref.SourceHash == "" {
		a.asterErr = "Run 未记录冻结文档源码哈希"
		return
	}
	frozen, err := asterdoc.LoadFrozenAsterDocAt(
		context.Background(), a.designSvc, ref.DesignID, ref.Revision, ref.SourceHash,
	)
	if err != nil {
		a.asterErr = state.MapError(err).Error()
		return
	}
	a.asterDoc = &frozen
}

func (a *app) startPDFExport(c *ui.Context) {
	if a.pdfExporting {
		return
	}
	if a.snap.AsterDoc == nil || a.designSvc == nil {
		c.AddToast(ui.Toast{Title: "无法导出 PDF", Description: "当前实验运行未找到冻结方案快照。", Type: components.ToastError})
		return
	}
	ref := *a.snap.AsterDoc
	loader := a.designSvc
	runID, title := a.snap.RunID, a.snap.Title
	designData, designID, designRevision := a.snap.DesignData, a.snap.DesignID, a.snap.DesignRevision
	if ref.SourceHash == "" {
		c.AddToast(ui.Toast{Title: "无法导出 PDF", Description: "Run 未记录冻结文档源码哈希。", Type: components.ToastError})
		return
	}

	frozenAt := ""
	if audit := a.snap.CreationAudit; audit != nil {
		frozenAt = audit.RecordedAt
	}
	runRecords := renderRunRecords(a.runWSVM, a.snap.Revision, frozenAt)
	a.pdfExporting = true
	a.pdfExportResults = make(chan pdfExportResult, 1)
	c.AddToast(ui.Toast{
		ID: "asterlab-pdf-export", Title: "正在导出 PDF…", Timeout: -1,
		Data: components.ToastContent{Leading: func() { components.Spinner(c, components.SpinnerRing) }},
	})
	results := a.pdfExportResults
	go func() {
		defer func() {
			if a.shell != nil {
				a.shell.Invalidate()
			}
		}()
		frozen, err := asterdoc.LoadFrozenAsterDocAt(
			context.Background(), loader, ref.DesignID, ref.Revision, ref.SourceHash,
		)
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		plan := ""
		if designData != "" {
			doc, err := protocol.Build(designData, designID, designRevision)
			if err != nil {
				results <- pdfExportResult{err: fmt.Errorf("冻结实验方案无法生成: %w", err)}
				return
			}
			plan = doc.Markdown()
		}
		home, _ := os.UserHomeDir()
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: mygo.FocusedWindow(), Title: "导出 PDF",
			DefaultPath: filepath.Join(home, pdfFilename(title)), ButtonLabel: "保存",
			Filters: []mygo.FileFilter{{Name: "PDF", Extensions: []string{"pdf"}}},
		})
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		if path == "" {
			results <- pdfExportResult{canceled: true}
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			path += ".pdf"
		}
		dir := filepath.Dir(path)
		tmp, err := os.CreateTemp(dir, ".asterlab-export-*.pdf")
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		tmpPath := tmp.Name()
		err = asterdoc.ExportFrozenPDFWithRunRecords(tmp, title, frozen, runID, plan, runRecords)
		if err == nil {
			err = tmp.Sync()
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmpPath, path)
		}
		if err != nil {
			_ = os.Remove(tmpPath)
			results <- pdfExportResult{err: err}
			return
		}
		results <- pdfExportResult{path: path}
	}()
}

func (a *app) startDesignPDFExport(c *ui.Context, doc *asterdoc.Document) {
	if doc == nil {
		return
	}
	title := designExperimentTitle(doc)
	srcHash := ""
	if a.designDoc != nil {
		srcHash = a.designDoc.SourceHash
	}
	baseDir := a.designBaseDir()
	a.startDocumentPDFExport(c, title, "", srcHash, baseDir, doc)
}

func (a *app) startRunRecordPDFExport(c *ui.Context) {
	if a.pdfExporting {
		return
	}
	frozenSrc := ""
	srcHash := ""
	if a.asterDoc != nil {
		frozenSrc = a.asterDoc.Source
	}
	if a.snap.AsterDoc != nil {
		srcHash = a.snap.AsterDoc.SourceHash
	}
	if strings.TrimSpace(frozenSrc) == "" && (a.snap.AsterDoc == nil || a.designSvc == nil) {
		if c != nil {
			c.AddToast(ui.Toast{Title: "无法导出实验记录 PDF", Description: "当前实验运行未关联实验方案文档。", Type: components.ToastError})
		}
		return
	}
	proj, _, _ := a.recordProjection()
	hv := a.ensureHydratedNotebookAsterView(proj)
	recordedAt := ""
	if a.snap.CreationAudit != nil {
		recordedAt = a.snap.CreationAudit.RecordedAt
	}
	if a.snap.RunID != "" && len(a.snap.Events) > 0 {
		last := a.snap.Events[len(a.snap.Events)-1]
		if strings.TrimSpace(last.RecordedAt) != "" {
			recordedAt = last.RecordedAt
		}
	}
	statusWord := notebookStatusWord(a.snap.Status)
	if statusWord == "" {
		statusWord = "执行记录"
	}
	opts := asterdoc.DocumentPDFOptions{
		Title:            firstNonEmpty(a.snap.Title, "实验记录"),
		SourceHash:       srcHash,
		RunID:            a.snap.RunID,
		IsRunRecord:      true,
		StatusLabel:      statusWord,
		RecordedAt:       recordedAt,
		BaseDir:          a.designBaseDir(),
		NotebookMarkdown: a.noteSource(),
	}
	if hv != nil {
		if len(hv.ActualParams) > 0 {
			opts.ActualParams = make(map[string]string, len(hv.ActualParams))
			for k, v := range hv.ActualParams {
				opts.ActualParams[k] = v
			}
		}
		opts.CalcActuals = hv.CalcActuals
		if len(hv.CheckedItems) > 0 {
			opts.CheckedItems = make(map[string]bool, len(hv.CheckedItems))
			for k, v := range hv.CheckedItems {
				opts.CheckedItems[k] = v
			}
		}
		if len(hv.SkippedItems) > 0 {
			opts.SkippedItems = make(map[string]string, len(hv.SkippedItems))
			for k, v := range hv.SkippedItems {
				opts.SkippedItems[k] = v
			}
		}
		if len(hv.TableCells) > 0 {
			opts.TableCells = make(map[string]map[string]string, len(hv.TableCells))
			for rk, row := range hv.TableCells {
				rcp := make(map[string]string, len(row))
				for ck, cv := range row {
					rcp[ck] = cv
				}
				opts.TableCells[rk] = rcp
			}
		}
		if len(hv.LinkedAssets) > 0 {
			opts.LinkedAssets = make(map[string][]asterdoc.AssetThumb, len(hv.LinkedAssets))
			for tk, list := range hv.LinkedAssets {
				opts.LinkedAssets[tk] = append([]asterdoc.AssetThumb(nil), list...)
			}
		}
	}
	snapCopy := a.snap
	refCopy := a.snap.AsterDoc
	loader := a.designSvc
	rootsCopy := make(map[string]string, len(a.assetRoots))
	for k, v := range a.assetRoots {
		rootsCopy[k] = v
	}
	readoutsCopy := record.DecodeReadouts(proj.Readouts)

	a.pdfExporting = true
	a.pdfExportResults = make(chan pdfExportResult, 1)
	if c != nil {
		c.AddToast(ui.Toast{
			ID: "asterlab-pdf-export", Title: "正在渲染实验记录 PDF…", Timeout: -1,
			Data: components.ToastContent{Leading: func() { components.Spinner(c, components.SpinnerRing) }},
		})
	}
	results := a.pdfExportResults
	go func() {
		defer func() {
			if a.shell != nil {
				a.shell.Invalidate()
			}
		}()
		if strings.TrimSpace(frozenSrc) == "" && refCopy != nil && loader != nil {
			loaded, err := asterdoc.LoadFrozenAsterDocAt(
				context.Background(), loader, refCopy.DesignID, refCopy.Revision, refCopy.SourceHash,
			)
			if err != nil {
				results <- pdfExportResult{err: err}
				return
			}
			frozenSrc = loaded.Source
			if hv == nil {
				vm := runworkspace.BuildViewModel(snapCopy, &asterdoc.FrozenAsterDoc{
					DesignID: loaded.DesignID, Revision: loaded.Revision, Source: loaded.Source,
				})
				if vm != nil && vm.FrozenDoc != nil {
					resolve := func(asset runworkspace.LinkedAsset) string {
						if strings.TrimSpace(asset.RootID) == "" || strings.TrimSpace(asset.RelativePath) == "" {
							return ""
						}
						p, err := assets.Roots(rootsCopy).Resolve(assets.Asset{
							RootID: asset.RootID, RelativePath: asset.RelativePath,
						})
						if err != nil {
							return ""
						}
						return p
					}
					v2 := runworkspace.HydrateReadOnlyAsterDocView(vm, readoutsCopy, a.runAssetsForTask, resolve)
					if v2 != nil {
						opts.ActualParams = v2.ActualParams
						opts.CalcActuals = v2.CalcActuals
						opts.CheckedItems = v2.CheckedItems
						opts.SkippedItems = v2.SkippedItems
						opts.TableCells = v2.TableCells
						opts.LinkedAssets = v2.LinkedAssets
					}
				}
			}
		}
		doc, err := asterdoc.Parse(frozenSrc)
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		home, _ := os.UserHomeDir()
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: mygo.FocusedWindow(), Title: "导出实验记录 PDF",
			DefaultPath: filepath.Join(home, pdfFilename(opts.Title+"-实验记录")), ButtonLabel: "保存",
			Filters: []mygo.FileFilter{{Name: "PDF", Extensions: []string{"pdf"}}},
		})
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		if path == "" {
			results <- pdfExportResult{canceled: true}
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			path += ".pdf"
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".asterlab-record-*.pdf")
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		tmpPath := tmp.Name()
		err = asterdoc.ExportDocumentPDF(tmp, doc, opts)
		if err == nil {
			err = tmp.Sync()
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmpPath, path)
		}
		if err != nil {
			_ = os.Remove(tmpPath)
			results <- pdfExportResult{err: err}
			return
		}
		results <- pdfExportResult{path: path}
	}()
}

func (a *app) startDocumentPDFExport(c *ui.Context, title, rev, srcHash, baseDir string, doc *asterdoc.Document) {
	if doc == nil || a.pdfExporting {
		return
	}
	a.pdfExporting = true
	a.pdfExportResults = make(chan pdfExportResult, 1)
	if c != nil {
		c.AddToast(ui.Toast{
			ID: "asterlab-pdf-export", Title: "正在渲染实验方案 PDF…", Timeout: -1,
			Data: components.ToastContent{Leading: func() { components.Spinner(c, components.SpinnerRing) }},
		})
	}
	results := a.pdfExportResults
	go func() {
		defer func() {
			if a.shell != nil {
				a.shell.Invalidate()
			}
		}()
		home, _ := os.UserHomeDir()
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: mygo.FocusedWindow(), Title: "导出实验方案 PDF",
			DefaultPath: filepath.Join(home, pdfFilename(title)), ButtonLabel: "保存",
			Filters: []mygo.FileFilter{{Name: "PDF", Extensions: []string{"pdf"}}},
		})
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		if path == "" {
			results <- pdfExportResult{canceled: true}
			return
		}
		if !strings.EqualFold(filepath.Ext(path), ".pdf") {
			path += ".pdf"
		}
		tmp, err := os.CreateTemp(filepath.Dir(path), ".asterlab-design-*.pdf")
		if err != nil {
			results <- pdfExportResult{err: err}
			return
		}
		tmpPath := tmp.Name()
		err = asterdoc.ExportDocumentPDF(tmp, doc, asterdoc.DocumentPDFOptions{
			Title:      title,
			Revision:   rev,
			SourceHash: srcHash,
			BaseDir:    baseDir,
		})
		if err == nil {
			err = tmp.Sync()
		}
		if closeErr := tmp.Close(); err == nil {
			err = closeErr
		}
		if err == nil {
			err = os.Rename(tmpPath, path)
		}
		if err != nil {
			_ = os.Remove(tmpPath)
			results <- pdfExportResult{err: err}
			return
		}
		results <- pdfExportResult{path: path}
	}()
}

func renderRunRecords(vm *runworkspace.RunWorkspaceViewModel, _ uint64, frozenAt string) string {
	var output strings.Builder
	if frozenAt == "" {
		output.WriteString("冻结时间：未记录\n")
	} else {
		fmt.Fprintf(&output, "冻结时间：%s\n", frozenAt)
	}
	output.WriteString("\n## Run 记录\n")
	if vm != nil {
		for _, task := range vm.Steps {
			appendRunTaskRecords(&output, task)
		}
		if len(vm.UnassociatedMeasurements) > 0 || len(vm.UnassociatedEvents) > 0 {
			output.WriteString("\n## 未关联记录\n这些记录缺少可确认的 task key 或 protocol step path，未归入任何步骤。\n")
			for _, measurement := range vm.UnassociatedMeasurements {
				sampleID := measurement.SampleID
				if sampleID == "" {
					sampleID = "未分配"
				}
				fmt.Fprintf(&output, "- %s · %s %s %s\n",
					sampleID, measurement.Field, measurement.Value, measurement.Unit)
			}
			for _, event := range vm.UnassociatedEvents {
				fmt.Fprintf(&output, "- %s · %s\n", runEventLabel(event.Type), event.Content)
			}
		}
	}
	return output.String()
}

func appendRunTaskRecords(output *strings.Builder, task *runworkspace.RunTaskViewModel) {
	if task == nil {
		return
	}
	if len(task.Children) > 0 {
		for _, child := range task.Children {
			appendRunTaskRecords(output, child)
		}
		return
	}
	fmt.Fprintf(output, "\n### %s", task.StepTitle)
	if task.Timepoint != "" {
		fmt.Fprintf(output, " · %s", task.Timepoint)
	}
	output.WriteByte('\n')
	if len(task.Parameters) > 0 {
		output.WriteString("计划与实际参数\n")
		for _, parameter := range task.Parameters {
			planned := parameter.Value
			if parameter.Unit != "" {
				planned += " " + parameter.Unit
			}
			fmt.Fprintf(output, "- %s: %s", parameter.Label, planned)
			if task.Actual != nil {
				if actual, ok := task.Actual.Parameters[parameter.Key]; ok && actual != planned {
					fmt.Fprintf(output, "；实际 %s", actual)
				}
			}
			output.WriteByte('\n')
		}
	}
	if task.Actual != nil && len(task.Actual.Measurements) > 0 {
		output.WriteString("实际读数\n")
		for _, measurement := range task.Actual.Measurements {
			sampleID := measurement.SampleID
			if sampleID == "" {
				sampleID = "未分配"
			}
			value := strings.TrimSpace(measurement.Field + " " + measurement.Value + " " + measurement.Unit)
			fmt.Fprintf(output, "- %s · %s\n", sampleID, value)
		}
	}
	for _, event := range task.Events {
		when := event.RecordedAt
		if when == "" {
			when = event.OccurredAt
		}
		if when == "" {
			fmt.Fprintf(output, "\n#### %s\n%s\n", runEventLabel(event.Type), event.Content)
		} else {
			fmt.Fprintf(output, "\n#### %s · %s\n%s\n", runEventLabel(event.Type), when, event.Content)
		}
		if event.BatchID != "" {
			fmt.Fprintf(output, "- Batch %s\n", event.BatchID)
		}
		if event.CheckAck {
			output.WriteString("- 检查确认\n")
		}
		if event.Deviation != "" {
			fmt.Fprintf(output, "- 偏差：%s\n", event.Deviation)
		}
		for _, check := range event.Checks {
			fmt.Fprintf(output, "- %s · %s\n", check.CheckID, check.Status)
		}
		for _, material := range event.Materials {
			fmt.Fprintf(output, "- %s %.3g %s\n", material.Material, material.Amount, material.Unit)
		}
		for _, parameter := range event.Parameters {
			value := parameter.Value
			if parameter.Unit != "" {
				value += " " + parameter.Unit
			}
			fmt.Fprintf(output, "- %s %s\n", parameter.Key, value)
		}
		for _, edge := range event.Lineage {
			fmt.Fprintf(output, "- %s:%s → %s:%s (%s)\n",
				edge.FromKind, edge.FromID, edge.ToKind, edge.ToID, edge.Relation)
		}
		for _, measurement := range event.Measurements {
			sampleID := measurement.SampleID
			if sampleID == "" {
				sampleID = "未分配"
			}
			fmt.Fprintf(output, "- %s · %s %s %s\n",
				sampleID, measurement.Field, measurement.Value, measurement.Unit)
		}
	}
}

func runEventLabel(kind string) string {
	switch kind {
	case "operation":
		return "操作"
	case "material_use":
		return "材料使用"
	case "measurement":
		return "测量"
	case "deviation":
		return "偏差"
	case "observation":
		return "观察"
	case "check":
		return "检查"
	default:
		return kind
	}
}

func pdfFilename(title string) string {
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`<>:"/\\|?*`, r) || r < 32 {
			return '_'
		}
		return r
	}, strings.TrimSpace(title))
	if name == "" || name == "." || name == ".." {
		name = "AsterLab"
	}
	return name + ".pdf"
}

func (a *app) pollPDFExport(c *ui.Context) {
	if a.pdfExportResults == nil {
		return
	}
	select {
	case result := <-a.pdfExportResults:
		a.pdfExportResults = nil
		a.pdfExporting = false
		c.CloseToast("asterlab-pdf-export")
		if result.canceled {
			return
		}
		if result.err != nil {
			c.AddToast(ui.Toast{Title: "PDF 导出失败", Description: result.err.Error(), Type: components.ToastError})
			return
		}
		c.AddToast(ui.Toast{
			Title: "PDF 已保存", Description: result.path, Type: components.ToastSuccess,
			Action: "打开 PDF", OnAction: func() { go mygo.Shell.OpenPath(result.path) },
		})
	default:
	}
}

// documentExportButton exports the complete Run record PDF using component-level rendering.
func (a *app) documentExportButton(c *ui.Context) {
	if a.snap.AsterDoc == nil {
		return
	}
	button := components.Button(c, "导出实验记录 PDF", components.ButtonSecondary)
	button.Disabled(a.pdfExporting || a.asterErr != "")
	if button.Clicked() && !a.pdfExporting && a.asterErr == "" {
		a.startRunRecordPDFExport(c)
	}
}

func (a *app) documentToolbar(c *ui.Context) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	ui.Row(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
		if a.snap.AsterDoc != nil {
			ui.Text(c, "方案快照已锁定").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		} else if a.snap.DesignRevision > 0 {
			ui.Text(c, "已关联实验方案").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		} else {
			ui.Text(c, "尚未关联实验方案").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		}
		if a.snap.AsterDoc != nil {
			label := "查看方案原文"
			if a.showFrozenDoc {
				label = "收起方案原文"
			}
			if components.Button(c, label, components.ButtonGhost).Clicked() {
				a.showFrozenDoc = !a.showFrozenDoc
			}
		}
		a.documentExportButton(c)
	})
}

func (a *app) renderFrozenAsterDoc(c *ui.Context, mode asterdoc.Mode) {
	t := c.Theme()
	if a.asterErr != "" {
		ui.Text(c, "冻结文档无法读取："+a.asterErr).FontSize(t.Rem(.85)).TextColor(t.Danger)
		return
	}
	if a.asterDoc == nil {
		ui.Text(c, "正在读取冻结文档…").FontSize(t.Rem(.85)).TextColor(t.TextMuted)
		return
	}
	doc, err := asterdoc.Parse(a.asterDoc.Source)
	if err != nil {
		ui.Text(c, "冻结文档语法无法解析："+err.Error()).FontSize(t.Rem(.85)).TextColor(t.Danger)
		return
	}
	view := &asterdoc.View{
		Doc: doc, Registry: asterdoc.BuiltinRegistry(), Mode: mode,
		ReadOnly: true,
	}
	view.RenderDocumentSheet(c)
}

//go:embed starter.asterml
var starterAsterDoc string

//go:embed blank.asterml
var blankAsterDoc string

//go:embed internal/asterdoc/testdata/inhibition-zone.asterml
var inhibitionZoneAsterDoc string

//go:embed internal/asterdoc/testdata/cfu.asterml
var cfuAsterDoc string

//go:embed internal/asterdoc/testdata/huvec-tube.asterml
var huvecAsterDoc string

//go:embed internal/asterdoc/testdata/rt-qpcr.asterml
var rtQpcrAsterDoc string

func (a *app) ensureDefaultProjectID() (string, error) {
	if len(a.projects) > 0 && strings.TrimSpace(a.projects[0].ProjectID) != "" {
		return a.projects[0].ProjectID, nil
	}
	if a.designSvc != nil {
		if items, err := a.designSvc.ListProjects(context.Background()); err == nil && len(items) > 0 {
			a.projects = items
			if a.shell != nil {
				names := make([]string, 0, len(items))
				for _, item := range items {
					names = append(names, item.Name)
				}
				a.shell.Projects = names
				if a.shell.Project == "" && len(names) > 0 {
					a.shell.Project = names[0]
				}
			}
			return items[0].ProjectID, nil
		}
	}
	if a.runSvc != nil {
		if _, err := a.runSvc.ExecuteCommand(context.Background(), runs.ExecuteCommandInput{
			Kind:           "project.create",
			IdempotencyKey: "asterlab-default-space",
			PayloadJSON:    `{"name":"默认实验空间"}`,
		}); err == nil && a.designSvc != nil {
			if items, listErr := a.designSvc.ListProjects(context.Background()); listErr == nil && len(items) > 0 {
				a.projects = items
				return items[0].ProjectID, nil
			}
		}
	}
	return "", fmt.Errorf("未找到可用实验工作空间")
}

func (a *app) designDiskPath(designID string) string {
	id := strings.TrimSpace(designID)
	if id == "" {
		return ""
	}
	if a.designPaths != nil {
		if p := strings.TrimSpace(a.designPaths[id]); p != "" {
			return p
		}
	}
	if os.Getenv("LABWORKBENCH_DATA_DIR") == "" && testing.Testing() {
		return ""
	}
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "experiments", id, "experiment.asterml")
}

func (a *app) designBaseDir() string {
	if a.designDoc != nil {
		if p := a.designDiskPath(a.designDoc.DesignID); p != "" {
			return filepath.Dir(p)
		}
	}
	if strings.TrimSpace(a.importPath) != "" {
		return filepath.Dir(a.importPath)
	}
	return ""
}

func (a *app) syncDesignDiskMirror(designID, source string) {
	p := a.designDiskPath(designID)
	if p == "" {
		return
	}
	if a.designPaths == nil {
		a.designPaths = map[string]string{}
	}
	a.designPaths[designID] = p
	dir := filepath.Dir(p)
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		return
	}

	designSource := source
	if envIn, err := asterdoc.ParseEnvelope(source); err == nil && strings.TrimSpace(envIn.DesignSource) != "" {
		designSource = envIn.DesignSource
	}
	meta := asterdoc.InspectExperiment(designSource, nil)
	stage := "draft"
	if meta.Complete {
		stage = "ready"
	}
	fm := asterdoc.EnvelopeFrontmatter{
		AsterML:       asterdoc.EnvelopeFormatVersion,
		ID:            designID,
		Title:         firstNonEmpty(meta.Title, "新实验"),
		ScheduledDate: meta.ScheduledDate,
		Stage:         stage,
	}
	var existingExec *asterdoc.ExecutionLedger
	var existingRec *asterdoc.RecordSection
	if prevBytes, err := os.ReadFile(p); err == nil {
		if prevEnv, pErr := asterdoc.ParseEnvelope(string(prevBytes)); pErr == nil {
			fm.RepeatOf = prevEnv.Frontmatter.RepeatOf
			if fm.ScheduledDate == "" {
				fm.ScheduledDate = prevEnv.Frontmatter.ScheduledDate
			}
			if prevEnv.Execution != nil {
				existingExec = prevEnv.Execution
				if prevEnv.Frontmatter.Stage == "in_progress" || prevEnv.Frontmatter.Stage == "completed" {
					fm.Stage = prevEnv.Frontmatter.Stage
				}
			}
			existingRec = prevEnv.Record
		}
	}
	if composed, err := asterdoc.ComposeEnvelope(fm, designSource, existingExec, existingRec); err != nil {
		_ = asterdoc.WriteFileAtomic(p, []byte(source))
	} else {
		_ = asterdoc.WriteFileAtomic(p, []byte(composed))
	}
	if info, statErr := os.Stat(p); statErr == nil {
		if a.diskMirrorMtimes == nil {
			a.diskMirrorMtimes = map[string]time.Time{}
		}
		a.diskMirrorMtimes[designID] = info.ModTime()
	}
}

func (a *app) startExternalSyncLoop() {
	if a.designSvc == nil {
		return
	}
	go func() {
		ticker := time.NewTicker(2 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			if a.externalSyncBusy.Swap(true) {
				continue
			}
			a.pollExternalDesignChanges()
			a.externalSyncBusy.Store(false)
		}
	}()
}

func (a *app) pollExternalDesignChanges() {
	if a.designSvc == nil || a.shell == nil {
		return
	}
	var activeID string
	var curRev uint64
	var curSrc string
	var diskPath string
	var lastMtime time.Time
	var knownDesignCount int
	var editorDirty bool

	doneSnap := make(chan struct{})
	mygo.RunOnMain(func() {
		defer close(doneSnap)
		knownDesignCount = len(a.shell.Designs)
		if a.designDoc != nil {
			activeID = a.designDoc.DesignID
			curRev = a.designDoc.Revision
			curSrc = a.designDoc.Source
			diskPath = a.designDiskPath(activeID)
			if a.diskMirrorMtimes != nil {
				lastMtime = a.diskMirrorMtimes[activeID]
			}
			if a.designTab == 2 && a.asterSrcEditor != nil && a.asterSrcEditor.Buffer != nil && strings.ReplaceAll(a.asterSrcEditor.Buffer.String(), "\r\n", "\n") != strings.ReplaceAll(a.asterSrcLoadedSrc, "\r\n", "\n") {
				editorDirty = true
			}
		}
	})
	<-doneSnap

	if items, err := a.fetchDesigns(); err == nil && len(items) != knownDesignCount {
		mygo.RunOnMain(func() {
			a.applyDesigns(items, nil)
			a.shell.Invalidate()
		})
	}
	if activeID == "" || editorDirty {
		return
	}

	if latest, err := a.designSvc.LoadAsterDoc(context.Background(), activeID); err == nil && latest.Revision > curRev {
		mygo.RunOnMain(func() {
			if a.designDoc == nil || a.designDoc.DesignID != activeID || latest.Revision <= a.designDoc.Revision {
				return
			}
			a.designDoc = &latest
			if a.designCache == nil {
				a.designCache = map[string]runs.AsterDocSnapshot{}
			}
			a.designCache[activeID] = latest
			a.syncDesignDiskMirror(activeID, latest.Source)
			a.designView = nil
			a.asterSrcLoadedRev = 0
			a.asterSrcLoadedSrc = ""
			a.refreshReadiness()
			a.syncShellExperiments()
			a.shell.Invalidate()
		})
		return
	}

	if diskPath != "" && !lastMtime.IsZero() {
		info, err := os.Stat(diskPath)
		if err != nil || !info.ModTime().After(lastMtime.Add(150*time.Millisecond)) {
			return
		}
		raw, err := os.ReadFile(diskPath)
		if err != nil || len(raw) == 0 {
			return
		}
		designSrc := string(raw)
		if env, eErr := asterdoc.ParseEnvelope(designSrc); eErr == nil && strings.TrimSpace(env.DesignSource) != "" {
			designSrc = env.DesignSource
		}
		if strings.TrimSpace(designSrc) == strings.TrimSpace(curSrc) {
			mygo.RunOnMain(func() {
				if a.diskMirrorMtimes == nil {
					a.diskMirrorMtimes = map[string]time.Time{}
				}
				a.diskMirrorMtimes[activeID] = info.ModTime()
			})
			return
		}
		if _, parseErr := asterdoc.Parse(designSrc); parseErr != nil {
			return
		}
		updated, saveErr := a.designSvc.SaveAsterDoc(context.Background(), runs.AsterDocSaveInput{
			DesignID: activeID, Source: designSrc, ExpectedRevision: curRev,
			OperationID: a.nextOp(), ActorID: "external-file",
		})
		if saveErr != nil {
			return
		}
		mygo.RunOnMain(func() {
			a.designDoc = &updated
			if a.designCache == nil {
				a.designCache = map[string]runs.AsterDocSnapshot{}
			}
			a.designCache[activeID] = updated
			if a.diskMirrorMtimes == nil {
				a.diskMirrorMtimes = map[string]time.Time{}
			}
			a.diskMirrorMtimes[activeID] = info.ModTime()
			a.designView = nil
			a.asterSrcLoadedRev = 0
			a.asterSrcLoadedSrc = ""
			a.refreshReadiness()
			a.syncShellExperiments()
			a.shell.Invalidate()
		})
	}
}

func (a *app) ensureEnvelopeSeedRaw(designID string) (string, string) {
	p := a.designDiskPath(designID)
	if p == "" {
		return "", ""
	}
	if existing, err := os.ReadFile(p); err == nil && len(existing) > 0 {
		return p, string(existing)
	}
	designSrc := ""
	if a.asterDoc != nil && a.asterDoc.DesignID == designID {
		designSrc = a.asterDoc.Source
	} else if cached, ok := a.designCache[designID]; ok {
		designSrc = cached.Source
	} else if a.designDoc != nil && a.designDoc.DesignID == designID {
		designSrc = a.designDoc.Source
	}
	if strings.TrimSpace(designSrc) == "" {
		return "", ""
	}
	meta := asterdoc.InspectExperiment(designSrc, nil)
	fm := asterdoc.EnvelopeFrontmatter{
		AsterML:       asterdoc.EnvelopeFormatVersion,
		ID:            designID,
		Title:         firstNonEmpty(meta.Title, a.snap.Title, "实验"),
		ScheduledDate: meta.ScheduledDate,
		Stage:         "ready",
	}
	composed, err := asterdoc.ComposeEnvelope(fm, designSrc, nil, nil)
	if err != nil {
		return "", ""
	}
	return p, composed
}

func (a *app) syncRunEnvelopeMirror() {
	if a.snap.RunID == "" || a.snap.DesignID == "" {
		return
	}
	p, raw := a.ensureEnvelopeSeedRaw(a.snap.DesignID)
	if p == "" || raw == "" {
		return
	}
	checks := map[string]string{}
	for _, tk := range a.snap.Tasks {
		for _, chk := range tk.Checks {
			if chk.Status != "" {
				checks[tk.Key+":"+chk.CheckID] = chk.Status
			}
		}
	}
	deviations := make([]map[string]any, 0)
	for _, ev := range a.snap.Events {
		if ev.EventType == "deviation" || ev.EventType == "observation" {
			if ev.Stage == "notebook" {
				continue
			}
			deviations = append(deviations, map[string]any{
				"id":          ev.ID,
				"type":        ev.EventType,
				"task_key":    ev.TaskKey,
				"step_path":   ev.ProtocolStepPath,
				"content":     ev.Content,
				"recorded_at": firstNonEmpty(ev.RecordedAt, ev.OccurredAt),
			})
		}
	}
	assetsMeta := make([]map[string]any, 0, len(a.runAssets))
	for _, ast := range a.runAssets {
		assetsMeta = append(assetsMeta, map[string]any{
			"id":            ast.ID,
			"relative_path": ast.RelativePath,
			"sha256":        ast.SHA256,
			"task_key":      ast.Link.TaskKey,
			"well":          ast.Link.WellID,
			"timepoint":     ast.Link.Timepoint,
			"fov":           ast.Link.FOVIndex,
		})
	}
	readouts := make([]map[string]any, 0, len(a.snap.Measurements))
	for _, m := range a.snap.Measurements {
		readouts = append(readouts, map[string]any{
			"sample":    m.SampleID,
			"endpoint":  m.Endpoint,
			"value":     m.Value,
			"unit":      m.Unit,
			"step_path": m.ProtocolStepPath,
		})
	}
	ledger := asterdoc.ExecutionLedger{
		Schema:     1,
		RunID:      a.snap.RunID,
		Status:     a.snap.Status,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
		Checks:     checks,
		Readouts:   readouts,
		Assets:     assetsMeta,
		Deviations: deviations,
	}
	next, err := asterdoc.PatchEnvelopeExecution(raw, ledger)
	if err != nil {
		return
	}
	_ = asterdoc.WriteFileAtomic(p, []byte(next))
}

func (a *app) syncNotebookEnvelopeMirror(markdown string) {
	if a.snap.RunID == "" || a.snap.DesignID == "" {
		return
	}
	p, raw := a.ensureEnvelopeSeedRaw(a.snap.DesignID)
	if p == "" || raw == "" {
		return
	}
	next, err := asterdoc.PatchEnvelopeRecord(raw, asterdoc.RecordSection{
		Schema:    1,
		RunID:     a.snap.RunID,
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Markdown:  markdown,
	})
	if err != nil {
		return
	}
	_ = asterdoc.WriteFileAtomic(p, []byte(next))
}

func (a *app) forkExperimentAsDraft(designID string) {
	if strings.TrimSpace(designID) == "" {
		return
	}
	raw := ""
	if p := a.designDiskPath(designID); p != "" {
		if b, err := os.ReadFile(p); err == nil {
			raw = string(b)
		}
	}
	if strings.TrimSpace(raw) == "" {
		if snap, ok := a.designCache[designID]; ok {
			raw = snap.Source
		} else if a.designDoc != nil && a.designDoc.DesignID == designID {
			raw = a.designDoc.Source
		}
	}
	if strings.TrimSpace(raw) == "" {
		return
	}
	targetDate := time.Now().Format("2006-01-02")
	if a.shell != nil && strings.TrimSpace(a.shell.TodayDate) != "" {
		targetDate = a.shell.TodayDate
	}
	forked, err := asterdoc.ForkEnvelopeAsDraft(raw, "", targetDate)
	if err != nil {
		return
	}
	env, err := asterdoc.ParseEnvelope(forked)
	if err != nil {
		return
	}
	title := firstNonEmpty(env.Frontmatter.Title, "新实验") + "（复刻）"
	a.createAsterDocDesign(env.DesignSource, title)
	if a.designDoc != nil && env.Frontmatter.RepeatOf != "" {
		if dp := a.designDiskPath(a.designDoc.DesignID); dp != "" {
			if curBytes, rErr := os.ReadFile(dp); rErr == nil {
				if curEnv, pErr := asterdoc.ParseEnvelope(string(curBytes)); pErr == nil {
					curEnv.Frontmatter.RepeatOf = env.Frontmatter.RepeatOf
					if next, cErr := asterdoc.ComposeEnvelope(curEnv.Frontmatter, a.designDoc.Source, nil, nil); cErr == nil {
						_ = asterdoc.WriteFileAtomic(dp, []byte(next))
					}
				}
			}
		}
	}
}

func (a *app) createAsterDocDesign(source, title string) {
	if a.designSvc == nil && a.vault != nil {
		cleanSource := source
		if env, envErr := asterdoc.ParseEnvelope(source); envErr == nil && strings.TrimSpace(env.DesignSource) != "" {
			cleanSource = env.DesignSource
		}
		meta := asterdoc.InspectExperiment(cleanSource, nil)
		expTitle := firstNonEmpty(strings.TrimSpace(title), meta.Title, "新实验")
		dateYMD := strings.TrimSpace(meta.ScheduledDate)
		if dateYMD == "" && a.shell != nil {
			dateYMD = a.shell.SelectedDate
		}
		folder := ""
		switch a.vault.Config.DefaultProjectDir {
		case "/", "":
			if a.vault.Config.DefaultProjectDir == "" {
				folder = a.vault.Config.LastProjectDir
			}
		default:
			folder = a.vault.Config.DefaultProjectDir
		}
		entry, err := a.vault.CreateExperiment(folder, expTitle, dateYMD, cleanSource)
		if err != nil {
			a.syncErr = fmt.Sprintf("创建实验失败：%v", err)
			return
		}
		a.syncErr = ""
		a.refreshVaultTreeAndCalendar()
		a.openVaultFileInActivePane(entry.RelPath, entry.Title)
		return
	}
	if a.designSvc == nil {
		msg := "设计服务不可用"
		a.syncErr = msg
		if a.shell != nil {
			a.shell.Error = msg
		}
		return
	}
	projectID, err := a.ensureDefaultProjectID()
	if err != nil {
		msg := err.Error()
		a.syncErr = msg
		if a.shell != nil {
			a.shell.Error = msg
		}
		return
	}
	cleanSource := source
	if env, envErr := asterdoc.ParseEnvelope(source); envErr == nil && strings.TrimSpace(env.DesignSource) != "" {
		cleanSource = env.DesignSource
	}
	doc, err := a.designSvc.CreateAsterDocDesign(context.Background(), runs.AsterDocCreateInput{
		ProjectID: projectID, Title: title, Source: cleanSource,
		OperationID: a.nextOp(), ActorID: os.Getenv("USERNAME"),
	})
	if err != nil {
		msg := state.MapError(err).Error()
		a.syncErr = msg
		if a.shell != nil {
			a.shell.Error = msg
		}
		return
	}
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	a.designCache[doc.DesignID] = doc
	a.syncDesignDiskMirror(doc.DesignID, doc.Source)
	meta := asterdoc.InspectExperiment(doc.Source, nil)
	stageLabel := "设计中"
	stageEnum := shell.StageDraft
	if meta.Complete {
		stageLabel = "方案就绪"
		stageEnum = shell.StageReady
	}
	expTitle := firstNonEmpty(meta.Title, title, "新实验")
	a.syncErr = ""
	a.designTab = 0
	a.loadDesign(doc.DesignID)
	if a.shell != nil {
		a.shell.Designs = append([]shell.DesignItem{{
			ID: doc.DesignID, Title: expTitle, Meta: stageLabel,
		}}, a.shell.Designs...)
		a.shell.Selected = doc.DesignID
		a.shell.Page = shell.Design
		a.shell.Error = ""
		a.shell.OpenExperimentTab(shell.ExperimentItem{
			ID:       doc.DesignID,
			DesignID: doc.DesignID,
			Title:    expTitle,
			Date:     firstNonEmpty(meta.ScheduledDate, a.shell.TodayDate),
			Stage:    stageEnum,
		}, shell.Design)
	}
	a.syncShellExperiments()
	a.saveSession()
}

func (a *app) selectRun(runID string) {
	if runID == "" {
		return
	}
	a.pendingRunID = runID
	if a.runSvc == nil {
		err := fmt.Errorf("Rust RunService 不可用")
		a.applyRun(nil, err)
		if a.shell != nil {
			a.shell.Error = err.Error()
		}
		return
	}
	snap, err := a.runSvc.LoadRun(context.Background(), runID)
	if err != nil {
		mapped := state.MapError(err)
		a.applyRun(nil, mapped)
		if a.shell != nil {
			a.shell.Error = mapped.Error()
		}
		return
	}
	a.applyRun(&snap, nil)
	if a.shell != nil {
		a.shell.Error = ""
		a.shell.Page = shell.Run
		a.shell.Selected = snap.DesignID
		a.saveSession()
	}
}

func (a *app) retryLoadRun() {
	runID := a.pendingRunID
	if runID == "" {
		runID = os.Getenv("ASTERLAB_RUN_ID")
	}
	if runID == "" {
		runID = a.snap.RunID
	}
	if runID == "" {
		snap, err := a.fetchRun()
		a.applyRun(snap, err)
		return
	}
	a.selectRun(runID)
}

type workspaceSession struct {
	Project       string                                  `json:"project"`
	AssetRoots    map[string]string                       `json:"asset_roots,omitempty"`
	DesignPaths   map[string]string                       `json:"design_paths,omitempty"`
	CanvasLayouts map[string]map[string]workflow.Position `json:"canvas_layouts,omitempty"`
	DesignID      string                                  `json:"design_id"`
	RunID         string                                  `json:"run_id"`
	TaskKey       string                                  `json:"task_key,omitempty"`
	TaskPath      string                                  `json:"task_path,omitempty"`
	DesignFocus   string                                  `json:"design_focus,omitempty"`
	DesignPreview bool                                    `json:"design_preview,omitempty"`
	DesignSubflow string                                  `json:"design_subflow,omitempty"`
	Collapsed     map[string]bool                         `json:"collapsed,omitempty"`
	Page          string                                  `json:"page"`
	DesignTab     int                                     `json:"design_tab"`
	UpdatedAt     string                                  `json:"updated_at"`
}

func workspaceDataDir() string {
	if dir := os.Getenv("LABWORKBENCH_DATA_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
			return filepath.Join(local, "AsterLab", "data")
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "AsterLab", "data")
	}
	return ""
}

func (a *app) sessionPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "workspace-session.json")
}

func (a *app) saveSession() {
	if a.shell == nil || a.designDoc == nil {
		if a.shell == nil {
			return
		}
	}
	path := a.sessionPath()
	if path == "" {
		return
	}
	session := workspaceSession{
		Project: a.shell.Project, DesignID: a.shell.Selected,
		Page: a.shell.PageName(), DesignTab: a.designTab,
		CanvasLayouts: a.designLayouts,
		AssetRoots:    a.assetRoots,
		DesignPaths:   a.designPaths,
		UpdatedAt:     time.Now().Format(time.RFC3339),
	}
	if a.snap.RunID != "" {
		// The selected Run belongs to the workspace, not only to the Run page.
		// Keeping it lets Design/Notebook/Tools restore with their run context.
		session.RunID = a.snap.RunID
	}
	if a.designDoc != nil {
		session.DesignFocus = a.designSelected
		session.DesignPreview = a.designPreviewExpanded
		if a.designSubflow != nil {
			session.DesignSubflow = a.designSubflowParent
		}
	}
	if a.runWS != nil {
		collapsed := a.runWS.TreeCollapsed()
		if len(collapsed) > 0 {
			session.Collapsed = collapsed
		}
		if a.runWS.Selected != "" {
			for _, task := range a.snap.Tasks {
				if task.Key == a.runWS.Selected {
					session.TaskKey = task.Key
					session.TaskPath = taskStepPath(task)
					break
				}
			}
		}
	}
	raw, err := json.Marshal(session)
	if err != nil {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	_ = os.WriteFile(path, raw, 0o644)
}

func (a *app) restoreSession() {
	path := a.sessionPath()
	if path == "" {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var session workspaceSession
	if json.Unmarshal(raw, &session) != nil {
		return
	}
	if session.AssetRoots != nil {
		a.assetRoots = session.AssetRoots
	}
	if session.DesignPaths != nil {
		a.designPaths = session.DesignPaths
	}
	if session.CanvasLayouts != nil {
		a.designLayouts = session.CanvasLayouts
	}
	if session.Project != "" {
		a.shell.Project = session.Project
	}
	loadedRun := false
	if session.RunID != "" {
		a.selectRun(session.RunID)
		loadedRun = a.snap.RunID != ""
		if loadedRun && a.runWS != nil {
			if session.Collapsed != nil {
				a.runWS.Collapsed = session.Collapsed
				a.runWS.SetTreeCollapsed(session.Collapsed)
			}
			if key := restoreTaskKey(a.snap.Tasks, session.TaskKey, session.TaskPath); key != "" {
				a.ws.Selected = key
				a.runWS.Selected = key
				a.prepareGrid()
			}
		}
	}
	if session.Page == "design" && session.DesignID != "" {
		a.selectDesign(session.DesignID)
		a.shell.Page = shell.Design
		a.designTab = session.DesignTab
		if a.designDoc != nil {
			a.designSelected = session.DesignFocus
			a.designPreviewExpanded = session.DesignPreview
			if session.DesignSubflow != "" {
				a.enterDesignSubflow(session.DesignSubflow)
			}
		}
		a.saveSession()
		return
	}
	switch session.Page {
	case "project":
		a.shell.Page = shell.Project
	case "tools":
		a.shell.Page = shell.Tools
	case "notebook":
		a.shell.Page = shell.Notebook
	case "run":
		if loadedRun {
			a.shell.Page = shell.Run
		}
	default:
		if loadedRun {
			a.shell.Page = shell.Run
		}
	}
	a.saveSession()
}

func (a *app) rememberRun(snap runs.RunSnapshot) {
	if a.shell == nil || snap.RunID == "" {
		return
	}
	meta := notebookStatusWord(snap.Status)
	if meta == "" {
		meta = "进行中"
	}
	next := []shell.RecentRun{{ID: snap.RunID, Title: snap.Title, Meta: meta}}
	for _, run := range a.shell.RecentRuns {
		if run.ID != snap.RunID {
			next = append(next, run)
		}
	}
	if len(next) > 5 {
		next = next[:5]
	}
	a.shell.RecentRuns = next
}

// selectTaskFromEnv applies an explicit automation deep link without changing
// normal task selection: ASTERLAB_TASK_PATH names a frozen step path.
func (a *app) selectTaskFromEnv() {
	path := os.Getenv("ASTERLAB_TASK_PATH")
	if path == "" || a.ws == nil {
		return
	}
	for _, task := range a.snap.Tasks {
		if task.Path == path || (task.Path == "" && task.Title == path) {
			a.selectRunTask(task.Key)
			return
		}
	}
}

// deepLinkArg returns the first startup argument that is an AsterLab deep
// link. Non-link arguments are ignored.
func deepLinkArg(args []string) string {
	const scheme = "asterlab://"
	for _, arg := range args {
		if len(arg) >= len(scheme) && strings.EqualFold(arg[:len(scheme)], scheme) {
			return arg
		}
	}
	return ""
}

// openRunLinkFromEnv applies an explicit automation deep link from
// ASTERLAB_RUN_LINK, alongside ASTERLAB_TASK_PATH. Invalid links become a
// visible shell error, never a panic. UX-P5, UX-IA-3, UX-DESIGN.
func (a *app) openRunLinkFromEnv() {
	raw := os.Getenv("ASTERLAB_RUN_LINK")
	if raw == "" {
		return
	}
	a.openRunWellLink(raw)
}

// openRunWellLink resolves all asterlab:// deep-link routes (`run`, `open`, `pantry`)
// into in-process navigation intents. Invalid links surface a visible shell error.
func (a *app) openRunWellLink(raw string) {
	intent, err := state.ParseAsterLabURI(raw)
	if err != nil {
		if a.shell != nil {
			a.shell.Error = err.Error()
		}
		return
	}
	if a.shell != nil {
		a.shell.Error = ""
	}
	switch intent.Kind {
	case "open":
		if intent.StepID != "" {
			a.scrollToStepID = intent.StepID
			a.designSelected = intent.StepID
		}
		a.openVaultFileInActivePane(intent.FilePath, "")
		return
	case "pantry":
		if a.tools == nil {
			a.tools = a.newToolsWorkspace()
		}
		a.tools.LocatePantryItem(intent.PantryCode)
		if a.shell != nil {
			a.shell.OpenPantryTab()
		}
		return
	}
	a.selectRun(intent.RunID)
	if intent.Well == "" {
		return
	}
	if a.shell == nil || a.shell.Page != shell.Run {
		return
	}
	if a.ws == nil || a.ws.Grid == nil {
		a.shell.Error = "实验已打开，但当前任务没有可定位的孔板"
		return
	}
	schema := a.ws.Grid.Schema
	row, col, ok := state.FindWellCell(intent.Well, schema.RowLabels, schema.ColumnLabels)
	if !ok {
		a.shell.Error = "孔位 " + intent.Well + " 不在当前任务的孔板上"
		return
	}
	a.ws.Grid.SetSelection(resultgrid.Selection{StartRow: row, StartColumn: col, EndRow: row, EndColumn: col})
}

// pollTimers refreshes the global timer strip at most every five seconds, so
// countdowns stay live without a Core call on every frame. Only successful
// reads replace the strip; a failed poll keeps the last known timers, and a
// failing service must never crash the frame loop. UX-P5, UX-DESIGN.
func (a *app) pollTimers() {
	if a.runSvc == nil || a.shell == nil {
		return
	}
	now := time.Now()
	if !a.timersPoll.IsZero() && now.Sub(a.timersPoll) < 5*time.Second {
		return
	}
	a.timersPoll = now
	timers, err := listActiveTimers(a.runSvc, now)
	if err != nil {
		return
	}
	if !equalTimers(a.shell.ActiveTimers, timers) {
		a.shell.ActiveTimers = timers
		a.shell.Invalidate()
	}
}

// listActiveTimers calls the service with a panic guard: the poller runs on
// the frame loop, so a torn implementation must degrade to stale timers.
func listActiveTimers(svc runs.RunService, now time.Time) (timers []runs.ActiveTimer, err error) {
	defer func() {
		if recover() != nil {
			timers, err = nil, fmt.Errorf("计时服务不可用")
		}
	}()
	return svc.ListActiveTimers(context.Background(), now.UTC().Format(time.RFC3339))
}

func equalTimers(a, b []runs.ActiveTimer) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].TimerID != b[i].TimerID || a[i].RunID != b[i].RunID ||
			a[i].ElapsedS != b[i].ElapsedS || remainingS(a[i]) != remainingS(b[i]) {
			return false
		}
	}
	return true
}

func remainingS(t runs.ActiveTimer) uint64 {
	if t.RemainingS == nil {
		return 0
	}
	return *t.RemainingS
}

func taskStepPath(task runs.RunTask) string {
	if task.Path != "" {
		return task.Path
	}
	return task.Title
}

// restoreTaskKey prefers the complete TaskKey. A path is only a fallback when
// exactly one frozen task carries it; duplicate paths across assays or
// timepoints are ambiguous and must not select an arbitrary occurrence.
func restoreTaskKey(tasks []runs.RunTask, taskKey, taskPath string) string {
	if taskKey != "" {
		for _, task := range tasks {
			if task.Key == taskKey {
				return task.Key
			}
		}
	}
	if taskPath == "" {
		return ""
	}
	key := ""
	for _, task := range tasks {
		if taskStepPath(task) != taskPath {
			continue
		}
		if key != "" {
			return ""
		}
		key = task.Key
	}
	return key
}

func currentRunTask(tasks []runs.RunTask) runs.RunTask {
	for _, task := range tasks {
		if task.Status != "completed" {
			return task
		}
	}
	if len(tasks) > 0 {
		return tasks[len(tasks)-1]
	}
	return runs.RunTask{}
}

func runTaskCacheKey(runID, taskKey string) string { return runID + "\x00" + taskKey }

func (a *app) clearTaskRecording() {
	a.taskGrids = nil
	a.taskDrafts = nil
}

func (a *app) clearTaskRecordingFor(runID, taskKey string) {
	key := runTaskCacheKey(runID, taskKey)
	delete(a.taskGrids, key)
	delete(a.taskDrafts, key)
}

// selectRunTask keeps the current task's in-memory edits before changing the
// selection. UX-P5: task navigation must never discard an unsaved draft.
func (a *app) selectRunTask(taskKey string) {
	if a.ws == nil {
		return
	}
	if a.selectedTask != "" && a.selectedTask != taskKey {
		key := runTaskCacheKey(a.snap.RunID, a.selectedTask)
		if a.taskGrids == nil {
			a.taskGrids = map[string]*resultgrid.Grid{}
		}
		if a.taskDrafts == nil {
			a.taskDrafts = map[string]runui.Draft{}
		}
		if a.ws.Grid != nil {
			a.taskGrids[key] = a.ws.Grid
		}
		a.taskDrafts[key] = a.ws.Draft
	}
	a.selectedTask = taskKey
	a.ws.Selected = taskKey
	key := runTaskCacheKey(a.snap.RunID, taskKey)
	if draft, ok := a.taskDrafts[key]; ok {
		a.ws.Draft = draft
	} else {
		a.ws.Draft = runui.NewDraft(a.ws.View)
	}
	if grid, ok := a.taskGrids[key]; ok {
		a.ws.Grid = grid
	} else {
		a.prepareGrid()
	}
	if a.runWS != nil {
		a.runWS.Selected = taskKey
		a.runWS.SetRecordingDirty(taskKey, a.ws.Grid != nil && a.ws.Grid.State != resultgrid.StateSaved)
	}
}

func (a *app) prepareGrid() {
	if a.ws == nil {
		return
	}
	if grid, ok := a.taskGrids[runTaskCacheKey(a.snap.RunID, a.ws.Selected)]; ok {
		a.ws.Grid = grid
		return
	}
	var task *runs.RunTask
	for i := range a.snap.Tasks {
		if a.snap.Tasks[i].Key == a.ws.Selected {
			task = &a.snap.Tasks[i]
			break
		}
	}
	if task == nil || task.AssayID == "" {
		a.ws.Grid = nil
		return
	}
	// The grid decodes the Run's frozen design only. A live Design-page
	// draft is never substituted: unfrozen content stays an explicit
	// "missing frozen design" error with retry (UX-P5, UX-STATE).
	designData := a.snap.DesignData
	if designData == "" {
		a.ws.Grid = nil
		a.ws.Error = "运行缺少冻结设计数据"
		return
	}
	schema, cells, err := assays.BuildSchema([]byte(designData), task.AssayID, task.Timepoint)
	if err != nil {
		a.ws.Grid = nil
		a.ws.Error = state.MapError(err).Error()
		return
	}
	a.ws.Error = ""
	key := runTaskCacheKey(a.snap.RunID, task.Key)
	grid := resultgrid.New(schema, cells)
	// Batch events are scoped to one frozen task. Unscoped legacy batches
	// cannot be assigned safely when a Run has multiple tasks.
	hasSavedValues := false
	for _, event := range a.snap.Events {
		if event.TaskKey != task.Key {
			continue
		}
		for _, c := range event.ResultCells {
			cell, ok := grid.CellAt(c.Row, c.Column)
			if !ok {
				continue
			}
			if c.SampleID != "" {
				cell.SampleID = c.SampleID
			}
			if c.Group != "" {
				cell.Group = c.Group
			}
			if c.Replicate != 0 {
				cell.Replicate = c.Replicate
			}
			cell.Field, cell.Value, cell.Status = c.Field, c.Value, c.Status
			_ = grid.SetCell(cell)
			hasSavedValues = true
		}
	}
	// Old measurements do not carry task/timepoint IDs, so only replay them
	// when the Run has one task and the target is unambiguous.
	if len(a.snap.Tasks) == 1 {
		for _, measurement := range a.snap.Measurements {
			for _, cell := range grid.Cells {
				if cell.SampleID == measurement.SampleID && strings.EqualFold(cell.Field, measurement.Endpoint) {
					value := measurement.Value
					cell.Value, cell.Status = &value, "raw"
					_ = grid.SetCell(cell)
					hasSavedValues = true
					break
				}
			}
		}
	}
	if hasSavedValues {
		grid.State = resultgrid.StateSaved
	}
	if a.taskGrids != nil {
		a.taskGrids[key] = grid
	}
	a.ws.Grid = grid
}

func (a *app) loadDesign(id string) {
	if id != "" {
		a.designLoadID = id
	}
	if a.designSvc == nil {
		a.designLoadErr = "设计服务不可用"
		return
	}
	snap, err := a.designSvc.LoadDesign(context.Background(), id)
	if err != nil {
		a.designLoadErr = state.MapError(err).Error()
		return
	}
	a.designLoadErr = ""
	a.designer = designer.New(id, "", 0, a.designSvc)
	a.designer.TimeBudgetSvc = a.timeBudgetSvc
	if a.timeBudgetErr != nil {
		a.designer.TimeBudgetError = a.timeBudgetErr.Error()
	}
	a.designer.OnCreateRun = func(snap runs.RunSnapshot) {
		// The new run replaces the previous run's revision/task state.
		a.snap = snap
		a.loadRunAssets()
		a.guard = state.StaleGuard{}
		if a.ws != nil {
			view := toRunView(snap)
			a.ws.View = view
			a.ws.Notebook = notebook.Build(snap)
			a.ws.Draft = runui.NewDraft(view)
			a.ws.Selected = ""
			a.ws.Grid = nil
			if len(snap.Tasks) == 0 && len(snap.Measurements) == 0 {
				a.ws.State = runui.StateEmpty
			} else {
				a.ws.State = runui.StateReady
			}
		}
		a.prepareGrid()
		a.shell.Page = shell.Run
	}
	a.designer.Load(context.Background(), id)
	prevDoc := a.designDoc
	a.designSnap = &snap
	a.designDoc = nil
	a.designView = nil
	a.designDocErr = ""
	a.designDocLoadFailed = false
	a.retryDesignDoc = nil
	a.createRunErr = ""
	a.readiness = nil
	a.readinessErr = ""
	a.readinessLoading = false
	a.designHistory = nil
	a.designFuture = nil
	a.resetAsterDraftHistory()
	a.designCanvas = nil
	a.designCanvasSource = ""
	a.designCanvasFitPending = false
	a.designCanvasBoundsW = 0
	a.designCanvasBoundsH = 0
	a.designInspectorOpen = false
	a.designInspectorUserToggled = false
	a.designHierarchy = nil
	a.designHierarchySource = ""
	a.designHierarchyErr = ""
	a.designSubflow = nil
	a.designSubflowParent = ""
	a.designSubflowTitle = ""
	a.designSubflowSelected = ""
	// Keep per-design layouts loaded from the session; selecting a design uses
	// its own saved coordinates. (no-op)
	a.designSelected = ""
	a.canvasSelLoaded = ""
	a.stepPropSaved = ""
	if prevDoc == nil || prevDoc.DesignID != id {
		a.stepDrafts = nil
	}
	if doc, docErr := a.designSvc.LoadAsterDoc(context.Background(), id); docErr != nil {
		a.designDocErr = state.MapError(docErr).Error()
		a.designDocLoadFailed = true
		if prevDoc != nil && prevDoc.DesignID == id {
			a.designDoc = prevDoc
		}
	} else if doc.Revision > 0 {
		a.designDoc = &doc
		if a.designCache == nil {
			a.designCache = map[string]runs.AsterDocSnapshot{}
		}
		a.designCache[id] = doc
		a.syncDesignDiskMirror(id, doc.Source)
		a.stepDrafts = nil
		a.refreshReadiness()
		a.syncShellExperiments()
	}
	a.preflightOpen = false
	a.preflightRisk = false
}

func (a *app) retryDesignID() string {
	if a.designLoadID != "" {
		return a.designLoadID
	}
	return a.currentDesignID()
}

func (a *app) currentDesignID() string {
	if a.designDoc != nil && a.designDoc.DesignID != "" {
		return a.designDoc.DesignID
	}
	if a.designer != nil && a.designer.DesignID != "" {
		return a.designer.DesignID
	}
	if a.shell != nil {
		return a.shell.Selected
	}
	return ""
}

// preload fetches the design list and the Run named by ASTERLAB_RUN_ID before
// the first meaningful frame. Native services must not be called on the UI
// thread, so the fetch runs on its own goroutine and the result is applied on
// the main thread.
func (a *app) preload() {
	go func() {
		items, designErr := a.fetchDesigns()
		projects, projectErr := a.fetchProjects()
		protocols, protocolErr := a.fetchProtocols()
		runItems, runListErr := a.fetchRuns()
		snap, runErr := a.fetchRun()
		mygo.RunOnMain(func() {
			a.applyDesigns(items, designErr)
			a.applyProjects(projects, projectErr)
			a.applyProtocols(protocols, protocolErr)
			a.applyRuns(runItems, runListErr)
			a.applyRun(snap, runErr)
			if os.Getenv("ASTERLAB_RUN_ID") == "" && os.Getenv("ASTERLAB_DESIGN_ID") == "" {
				a.restoreSession()
			}
			// An explicit deep link wins over the restored session. A startup
			// URL argument takes precedence over the automation env hook.
			// Selection stays on the Run's current recording task; a well
			// outside that grid is a visible error, never a silent success.
			if a.runLinkArg != "" {
				a.openRunWellLink(a.runLinkArg)
			} else {
				a.openRunLinkFromEnv()
			}
			a.shell.Invalidate()
		})
	}()
}

func (a *app) fetchProjects() ([]runs.ProjectSummary, error) {
	if a.designSvc == nil {
		return nil, fmt.Errorf("设计服务不可用")
	}
	items, err := a.designSvc.ListProjects(context.Background())
	if err != nil {
		return nil, state.MapError(err)
	}
	return items, nil
}

func (a *app) applyProjects(items []runs.ProjectSummary, err error) {
	if err != nil || a.shell == nil {
		return
	}
	names := make([]string, 0, len(items))
	for _, project := range items {
		names = append(names, project.Name)
	}
	a.projects = items
	a.shell.Projects = names
	if a.shell.Project == "" && len(names) > 0 {
		a.shell.Project = names[0]
	}
}

func (a *app) fetchProtocols() ([]runs.ProtocolSummary, error) {
	if a.designSvc == nil {
		return nil, fmt.Errorf("设计服务不可用")
	}
	items, err := a.designSvc.ListProtocols(context.Background())
	if err != nil {
		return nil, state.MapError(err)
	}
	return items, nil
}

func (a *app) applyProtocols(items []runs.ProtocolSummary, err error) {
	if err != nil {
		return
	}
	a.protocols = items
}

func (a *app) fetchRuns() ([]runs.RunSummary, error) {
	if a.runSvc == nil {
		return nil, fmt.Errorf("运行服务不可用")
	}
	items, err := a.runSvc.ListRuns(context.Background())
	if err != nil {
		return nil, state.MapError(err)
	}
	return items, nil
}

func (a *app) applyRuns(items []runs.RunSummary, err error) {
	if err != nil || a.shell == nil {
		return
	}
	a.runSummaries = append([]runs.RunSummary(nil), items...)
	recent := make([]shell.RecentRun, 0, len(items))
	for _, run := range items {
		meta := notebookStatusWord(run.Status)
		if meta == "" {
			meta = run.UpdatedAt
		}
		recent = append(recent, shell.RecentRun{ID: run.RunID, Title: run.Title, Meta: meta})
	}
	a.shell.RecentRuns = recent
	a.syncShellExperiments()
}

func (a *app) fetchDesigns() ([]shell.DesignItem, error) {
	if a.designSvc == nil {
		return nil, fmt.Errorf("设计服务不可用")
	}
	ds, err := a.designSvc.ListDesigns(context.Background())
	if err != nil {
		return nil, state.MapError(err)
	}
	items := make([]shell.DesignItem, 0, len(ds))
	for _, d := range ds {
		items = append(items, shell.DesignItem{ID: d.DesignID, Title: d.Title, Meta: "实验方案"})
	}
	return items, nil
}

func (a *app) applyDesigns(items []shell.DesignItem, err error) {
	if err != nil {
		a.shell.Error = err.Error()
		return
	}
	a.shell.Designs = items
	a.syncShellExperiments()
	if id := os.Getenv("ASTERLAB_DESIGN_ID"); id != "" {
		for _, item := range items {
			if item.ID == id {
				a.selectDesign(id)
				break
			}
		}
	}
}

func (a *app) syncShellExperiments() {
	if a.shell == nil {
		return
	}
	today := time.Now().Format("2006-01-02")
	if strings.TrimSpace(a.shell.TodayDate) == "" {
		a.shell.TodayDate = today
	}
	if strings.TrimSpace(a.shell.SelectedDate) == "" {
		a.shell.SelectedDate = a.shell.TodayDate
	}
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	if a.designDoc != nil && a.designDoc.DesignID != "" {
		a.designCache[a.designDoc.DesignID] = *a.designDoc
	}
	runByDesign := map[string]runs.RunSummary{}
	for _, r := range a.runSummaries {
		if r.DesignID == "" {
			continue
		}
		if prev, ok := runByDesign[r.DesignID]; !ok || r.UpdatedAt >= prev.UpdatedAt {
			runByDesign[r.DesignID] = r
		}
	}
	if a.snap.RunID != "" && a.snap.DesignID != "" {
		runByDesign[a.snap.DesignID] = runs.RunSummary{
			RunID:     a.snap.RunID,
			DesignID:  a.snap.DesignID,
			Title:     a.snap.Title,
			Status:    a.snap.Status,
			UpdatedAt: time.Now().Format("2006-01-02"),
		}
	}
	seenDesigns := map[string]bool{}
	seenRuns := map[string]bool{}
	exps := make([]shell.ExperimentItem, 0, len(a.shell.Designs)+len(a.runSummaries))
	for i := range a.shell.Designs {
		d := &a.shell.Designs[i]
		seenDesigns[d.ID] = true
		docSnap, ok := a.designCache[d.ID]
		if !ok && a.designSvc != nil {
			if loaded, err := a.designSvc.LoadAsterDoc(context.Background(), d.ID); err == nil && loaded.Source != "" {
				docSnap = loaded
				a.designCache[d.ID] = loaded
			}
		}
		var meta asterdoc.ExperimentMeta
		if strings.TrimSpace(docSnap.Source) != "" {
			meta = asterdoc.InspectExperiment(docSnap.Source, nil)
		} else {
			meta = asterdoc.ExperimentMeta{
				Title:            d.Title,
				IncompleteReason: "待补充实验步骤与模块",
				Summary:          "尚未编辑实验步骤",
			}
		}
		stage := shell.StageDraft
		if meta.Complete {
			stage = shell.StageReady
			d.Meta = "方案就绪"
		} else {
			d.Meta = "设计中"
		}
		activeRunID := ""
		date := meta.ScheduledDate
		if r, hasRun := runByDesign[d.ID]; hasRun {
			seenRuns[r.RunID] = true
			activeRunID = r.RunID
			if strings.EqualFold(r.Status, "completed") {
				stage = shell.StageCompleted
				d.Meta = "已完成"
			} else {
				stage = shell.StageInProgress
				d.Meta = "进行中"
			}
			if date == "" {
				date = asterdoc.NormalizeDateYMD(r.UpdatedAt)
			}
		}
		if date == "" {
			date = a.shell.TodayDate
		}
		title := firstNonEmpty(meta.Title, d.Title, "新实验")
		summary := meta.Summary
		if stage == shell.StageInProgress {
			summary = "实验执行中 · 点击继续记录"
		} else if stage == shell.StageCompleted {
			summary = "实验已完成 · 点击查看实验记录与 PDF"
		}
		gap := ""
		if stage == shell.StageDraft {
			gap = meta.IncompleteReason
		}
		exps = append(exps, shell.ExperimentItem{
			ID:          d.ID,
			DesignID:    d.ID,
			ActiveRunID: activeRunID,
			Title:       title,
			Date:        date,
			Stage:       stage,
			Summary:     summary,
			GapHint:     gap,
			DurationMin: meta.Budget.TotalSpanSec / 60,
			FileName:    filepath.Base(a.designDiskPath(d.ID)),
		})
	}
	for _, r := range a.runSummaries {
		if seenRuns[r.RunID] || (r.DesignID != "" && seenDesigns[r.DesignID]) {
			continue
		}
		seenRuns[r.RunID] = true
		stage := shell.StageInProgress
		summary := "实验执行中 · 点击继续记录"
		if strings.EqualFold(r.Status, "completed") {
			stage = shell.StageCompleted
			summary = "实验已完成 · 点击查看实验记录与 PDF"
		}
		date := asterdoc.NormalizeDateYMD(r.UpdatedAt)
		if date == "" {
			date = a.shell.TodayDate
		}
		exps = append(exps, shell.ExperimentItem{
			ID:          r.RunID,
			DesignID:    r.DesignID,
			ActiveRunID: r.RunID,
			Title:       firstNonEmpty(r.Title, "实验运行"),
			Date:        date,
			Stage:       stage,
			Summary:     summary,
		})
	}
	if a.snap.RunID != "" && !seenRuns[a.snap.RunID] && (a.snap.DesignID == "" || !seenDesigns[a.snap.DesignID]) {
		stage := shell.StageInProgress
		summary := "实验执行中 · 点击继续记录"
		if strings.EqualFold(a.snap.Status, "completed") {
			stage = shell.StageCompleted
			summary = "实验已完成 · 点击查看实验记录与 PDF"
		}
		exps = append(exps, shell.ExperimentItem{
			ID:          a.snap.RunID,
			DesignID:    a.snap.DesignID,
			ActiveRunID: a.snap.RunID,
			Title:       firstNonEmpty(a.snap.Title, "实验运行"),
			Date:        a.shell.TodayDate,
			Stage:       stage,
			Summary:     summary,
		})
	}
	a.shell.Experiments = exps
	a.syncShellOutlineAndPantry()
	if a.vault != nil {
		a.refreshVaultTreeAndCalendar()
	}
}

func (a *app) syncShellOutlineAndPantry() {
	if a.shell == nil {
		return
	}
	var outline []shell.OutlineItem
	if a.designDoc != nil && strings.TrimSpace(a.designDoc.Source) != "" {
		if parsed, err := asterdoc.Parse(a.designDoc.Source); err == nil {
			for _, m := range parsed.Modules {
				if m.Kind != "step" {
					continue
				}
				blocksCount := 0
				if rawBlocks, ok := m.Data["blocks"].([]any); ok {
					blocksCount = len(rawBlocks)
				}
				meta := ""
				if blocksCount > 0 {
					meta = fmt.Sprintf("%d 板块", blocksCount)
				}
				outline = append(outline, shell.OutlineItem{
					ID:     m.ID,
					Title:  firstNonEmpty(moduleString(m.Data, "title"), m.ID),
					Meta:   meta,
					Active: m.ID == a.designSelected,
				})
			}
		}
	}
	a.shell.OutlineSteps = outline

	var pantrySource []runs.PantryItem
	if a.tools != nil && len(a.tools.Pantry.Items) > 0 {
		pantrySource = a.tools.Pantry.Items
	} else if a.runSvc != nil && !testing.Testing() && len(a.shell.PantryItems) == 0 {
		if ledger, err := a.runSvc.ListPantry(context.Background(), runs.PantryQuery{}); err == nil {
			pantrySource = ledger.Items
		}
	}
	if len(pantrySource) > 0 {
		qItems := make([]shell.QuickPantryItem, 0, len(pantrySource))
		for _, p := range pantrySource {
			loc := p.Location
			if a.tools != nil {
				if formatted := a.tools.FormatPantryDisplayLocation(p); formatted != "" {
					loc = formatted
				}
			}
			qItems = append(qItems, shell.QuickPantryItem{
				ID:        p.ID,
				Name:      p.Name,
				Location:  loc,
				Status:    p.Status,
				ExpiresOn: p.ExpiresOn,
				LotNumber: p.LabelCode,
			})
		}
		a.shell.PantryItems = qItems
	}
}

func (a *app) selectDesign(id string) {
	if id == "" {
		return
	}
	if a.designer != nil && id == a.designer.DesignID && a.designLoadErr == "" && !a.designDocLoadFailed {
		a.shell.Selected = id
		return
	}
	if a.designDoc == nil && a.designer != nil && a.designer.Dirty {
		a.pendingDesignID = id
		a.confirmDesignSwitch = true
		a.shell.Selected = a.designer.DesignID
		return
	}
	a.shell.Selected = id
	a.loadDesign(id)
	if a.designLoadErr != "" && a.designer != nil && a.designer.DesignID != "" {
		a.shell.Selected = a.designer.DesignID
	}
	a.saveSession()
}

func (a *app) renderDesignSwitchConfirmation(c *ui.Context) {
	if !a.confirmDesignSwitch {
		return
	}
	t := c.Theme()
	title := "所选设计"
	for _, item := range a.shell.Designs {
		if item.ID == a.pendingDesignID {
			title = item.Title
			break
		}
	}
	components.Dialog(c, &a.confirmDesignSwitch, components.DialogOptions{Label: "切换设计"}, func(panel ui.Element) {
		panel.Width(460).Children(func() {
			components.Text(c, "当前设计有未保存修改").FontSize(t.Rem(1.1)).FontWeight(600)
			components.Text(c, fmt.Sprintf("切换到“%s”前，请先保存、明确放弃修改，或继续编辑。", title)).
				TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(2)).Children(func() {
				if components.Button(c, "继续编辑", components.ButtonSecondary).Clicked() {
					a.confirmDesignSwitch = false
					a.pendingDesignID = ""
					a.shell.Selected = a.designer.DesignID
				}
				if components.Button(c, "放弃并切换", components.ButtonOutline).Clicked() {
					id := a.pendingDesignID
					a.confirmDesignSwitch = false
					a.pendingDesignID = ""
					a.shell.Selected = id
					a.loadDesign(id)
				}
				if components.Button(c, "保存并切换", components.ButtonPrimary).Clicked() {
					id := a.pendingDesignID
					a.designer.Save(context.Background())
					if a.designer.SaveState == designer.SaveSaved {
						a.confirmDesignSwitch = false
						a.pendingDesignID = ""
						a.shell.Selected = id
						a.loadDesign(id)
					}
				}
			})
		})
	})
	if !a.confirmDesignSwitch {
		a.pendingDesignID = ""
	}
}

func (a *app) fetchRun() (*runs.RunSnapshot, error) {
	runID := os.Getenv("ASTERLAB_RUN_ID")
	if runID == "" {
		return nil, nil
	}
	if a.runSvc == nil {
		return nil, fmt.Errorf("Rust RunService 不可用")
	}
	snap, err := a.runSvc.LoadRun(context.Background(), runID)
	if err != nil {
		return nil, state.MapError(err)
	}
	return &snap, nil
}

func (a *app) applyRun(snap *runs.RunSnapshot, err error) {
	if err != nil {
		if a.ws != nil {
			a.ws.State = runui.StateBlocked
			a.ws.Error = err.Error()
		}
		if a.snap.RunID == "" || a.snap.Status == "loading" {
			a.snap = runs.RunSnapshot{Title: "实验运行加载失败", Status: "error"}
			a.runWSVM = &runworkspace.RunWorkspaceViewModel{
				Title:  "实验运行加载失败",
				Status: "error",
			}
			a.runWSKey = ""
			if a.runWS != nil {
				a.runWS.SyncFrom(a.runWSVM)
			}
		}
		if a.runWS != nil {
			a.runWS.LoadError = err.Error()
		}
		return
	}
	a.syncErr = ""
	if a.shell != nil {
		a.shell.Error = ""
	}
	if snap == nil {
		if a.noteRunID != "" && (a.noteDirty || a.noteErr != "") {
			a.persistNotebookDraft()
		}
		a.snap = runs.RunSnapshot{}
		if a.ws != nil {
			a.ws.View = toRunView(a.snap)
			a.ws.Draft = runui.NewDraft(a.ws.View)
			a.ws.Selected = ""
			a.ws.Grid = nil
			a.ws.State = runui.StateEmpty
			a.ws.Error = ""
		}
		a.runAssets = nil
		a.runWSVM = a.buildRunWorkspaceVM()
		if a.runWS != nil {
			a.runWS.SyncFrom(a.runWSVM)
		}
		return
	}
	a.applySnap(*snap)
	if a.ws != nil && a.ws.State == runui.StateLoading {
		if len(a.snap.Tasks) == 0 && len(a.snap.Measurements) == 0 {
			a.ws.State = runui.StateEmpty
		} else {
			a.ws.State = runui.StateReady
		}
	}
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// applyDesignCommand routes a professional editor action through Core, then
// reloads the new immutable revision.
func (a *app) applyDesignCommand(kind, moduleID, group string, wells []string) {
	command := map[string]any{"type": kind, "moduleId": moduleID, "wells": wells}
	if group != "" {
		command["group"] = group
	}
	_ = a.applyDesignRaw(command)
}

// applyDesignRaw sends one semantic command and reloads the document on
// success. It is the only write seam for professional editors.
func (a *app) applyDesignRaw(command map[string]any) bool {
	if a.designSvc == nil {
		a.designDocErr = "设计服务不可用，无法保存修改"
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { _ = a.applyDesignRaw(command) }
		return false
	}
	if a.designDoc == nil {
		a.designDocErr = "尚未加载实验方案，暂时无法保存修改"
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { _ = a.applyDesignRaw(command) }
		return false
	}
	previous := *a.designDoc
	raw, err := json.Marshal(command)
	if err != nil {
		a.designDocErr = err.Error()
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { _ = a.applyDesignRaw(command) }
		return false
	}
	updated, err := a.designSvc.ApplyAsterDocCommand(context.Background(), runs.AsterDocCommandInput{
		DesignID: a.designDoc.DesignID, ExpectedRevision: a.designDoc.Revision,
		OperationID: a.nextOp(), ActorID: os.Getenv("USERNAME"), Command: string(raw),
	})
	if err != nil && isConflictErr(err) {
		if latest, loadErr := a.designSvc.LoadAsterDoc(context.Background(), a.designDoc.DesignID); loadErr == nil {
			a.designDoc = &latest
			updated, err = a.designSvc.ApplyAsterDocCommand(context.Background(), runs.AsterDocCommandInput{
				DesignID: latest.DesignID, ExpectedRevision: latest.Revision,
				OperationID: a.nextOp(), ActorID: os.Getenv("USERNAME"), Command: string(raw),
			})
		}
	}
	if err != nil {
		a.designDocErr = state.MapError(err).Error()
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { _ = a.applyDesignRaw(command) }
		return false
	}
	a.designDocErr = ""
	a.designDocLoadFailed = false
	a.retryDesignDoc = nil
	a.designHistory = append(a.designHistory, previous)
	a.designFuture = nil
	a.resetAsterDraftHistory()
	a.designDoc = &updated
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	a.designCache[updated.DesignID] = updated
	a.syncDesignDiskMirror(updated.DesignID, updated.Source)
	a.designView = nil
	a.designCanvas = nil
	a.designCanvasSource = ""
	a.designCanvasFitPending = false
	a.designCanvasBoundsW = 0
	a.designCanvasBoundsH = 0
	a.designInspectorOpen = false
	a.designInspectorUserToggled = false
	a.designHierarchy = nil
	a.designHierarchySource = ""
	a.designSubflow = nil
	a.designSubflowParent = ""
	a.refreshReadiness()
	a.syncShellExperiments()
	return true
}

func (a *app) designUndo() {
	if a.asterDraftUndoStep() {
		return
	}
	if a.designSvc == nil || a.designDoc == nil || len(a.designHistory) == 0 {
		return
	}
	target := a.designHistory[len(a.designHistory)-1]
	current := *a.designDoc
	updated, err := a.designSvc.SaveAsterDoc(context.Background(), runs.AsterDocSaveInput{
		DesignID: a.designDoc.DesignID, Source: target.Source,
		ExpectedRevision: a.designDoc.Revision, OperationID: a.nextOp(),
		ActorID: os.Getenv("USERNAME"),
	})
	if err != nil && isConflictErr(err) {
		if latest, loadErr := a.designSvc.LoadAsterDoc(context.Background(), a.designDoc.DesignID); loadErr == nil {
			updated, err = a.designSvc.SaveAsterDoc(context.Background(), runs.AsterDocSaveInput{
				DesignID: latest.DesignID, Source: target.Source,
				ExpectedRevision: latest.Revision, OperationID: a.nextOp(),
				ActorID: os.Getenv("USERNAME"),
			})
		}
	}
	if err != nil {
		a.designDocErr = state.MapError(err).Error()
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { a.designUndo() }
		return
	}
	a.designHistory = a.designHistory[:len(a.designHistory)-1]
	a.designFuture = append(a.designFuture, current)
	a.designDocErr = ""
	a.designDocLoadFailed = false
	a.retryDesignDoc = nil
	a.designDoc = &updated
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	a.designCache[updated.DesignID] = updated
	a.syncDesignDiskMirror(updated.DesignID, updated.Source)
	a.designView = nil
	a.designCanvas = nil
	a.designCanvasSource = ""
	a.designCanvasFitPending = false
	a.designCanvasBoundsW = 0
	a.designCanvasBoundsH = 0
	a.designInspectorOpen = false
	a.designInspectorUserToggled = false
	a.designHierarchy = nil
	a.designHierarchySource = ""
	a.designSubflow = nil
	a.designSubflowParent = ""
	a.refreshReadiness()
	a.syncShellExperiments()
}

func (a *app) designRedo() {
	if a.asterDraftRedoStep() {
		return
	}
	if a.designSvc == nil || a.designDoc == nil || len(a.designFuture) == 0 {
		return
	}
	target := a.designFuture[len(a.designFuture)-1]
	current := *a.designDoc
	updated, err := a.designSvc.SaveAsterDoc(context.Background(), runs.AsterDocSaveInput{
		DesignID: a.designDoc.DesignID, Source: target.Source,
		ExpectedRevision: a.designDoc.Revision, OperationID: a.nextOp(),
		ActorID: os.Getenv("USERNAME"),
	})
	if err != nil && isConflictErr(err) {
		if latest, loadErr := a.designSvc.LoadAsterDoc(context.Background(), a.designDoc.DesignID); loadErr == nil {
			updated, err = a.designSvc.SaveAsterDoc(context.Background(), runs.AsterDocSaveInput{
				DesignID: latest.DesignID, Source: target.Source,
				ExpectedRevision: latest.Revision, OperationID: a.nextOp(),
				ActorID: os.Getenv("USERNAME"),
			})
		}
	}
	if err != nil {
		a.designDocErr = state.MapError(err).Error()
		a.designDocLoadFailed = false
		a.retryDesignDoc = func() { a.designRedo() }
		return
	}
	a.designFuture = a.designFuture[:len(a.designFuture)-1]
	a.designHistory = append(a.designHistory, current)
	a.designDocErr = ""
	a.designDocLoadFailed = false
	a.retryDesignDoc = nil
	a.designDoc = &updated
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	a.designCache[updated.DesignID] = updated
	a.syncDesignDiskMirror(updated.DesignID, updated.Source)
	a.designView = nil
	a.designCanvas = nil
	a.designCanvasSource = ""
	a.designCanvasFitPending = false
	a.designCanvasBoundsW = 0
	a.designCanvasBoundsH = 0
	a.designInspectorOpen = false
	a.designInspectorUserToggled = false
	a.designHierarchy = nil
	a.designHierarchySource = ""
	a.designSubflow = nil
	a.designSubflowParent = ""
	a.refreshReadiness()
	a.syncShellExperiments()
}

func (a *app) createRunFromDesignDoc() {
	if a.designSvc == nil {
		a.createRunErr = "设计服务不可用，无法创建 Run"
		return
	}
	if a.designDoc == nil {
		a.createRunErr = "尚未加载实验方案，暂时无法启动实验运行"
		return
	}
	a.createRunErr = ""
	acknowledged := []string{}
	if a.preflightRisk && a.readiness != nil {
		acknowledged = append(acknowledged, a.readiness.Warnings...)
	}
	run, err := a.designSvc.CreateRunFromAsterDoc(context.Background(), runs.AsterDocRunInput{
		DesignID: a.designDoc.DesignID, DocumentRevision: a.designDoc.Revision,
		OperationID: a.nextOp(), AcknowledgedWarnings: acknowledged,
	})
	if err != nil {
		a.createRunErr = state.MapError(err).Error()
		return
	}
	a.createRunErr = ""
	a.snap = run
	a.loadRunAssets()
	a.guard = state.StaleGuard{}
	if a.ws != nil {
		view := toRunView(run)
		a.ws.View = view
		a.ws.Notebook = notebook.Build(run)
		a.ws.Draft = runui.NewDraft(view)
		a.ws.Selected = ""
		a.ws.Grid = nil
		a.ws.State = runui.StateReady
	}
	a.loadFrozenAsterDoc()
	a.refreshRunWorkspace()
	a.rememberRun(run)
	a.syncRunEnvelopeMirror()
	a.syncShellExperiments()
	a.prepareGrid()
	a.shell.Page = shell.Run
	if a.shell != nil {
		a.shell.OpenExperimentTab(shell.ExperimentItem{
			ID:          firstNonEmpty(run.DesignID, run.RunID),
			DesignID:    run.DesignID,
			ActiveRunID: run.RunID,
			Title:       firstNonEmpty(run.Title, "实验运行"),
			Stage:       shell.StageInProgress,
		}, shell.Run)
	}
	a.saveSession()
}

func findTargetDesignStepID(doc *asterdoc.Document, selectedID string) string {
	if doc == nil {
		return ""
	}
	for _, m := range doc.Modules {
		if m.Kind == "step" && m.ID == selectedID && moduleString(m.Data, "execution") != "organizational" {
			return m.ID
		}
	}
	for _, m := range doc.Modules {
		if m.Kind == "step" && moduleString(m.Data, "execution") != "organizational" {
			return m.ID
		}
	}
	return ""
}

func (a *app) insertAsterBlockToDesign(blk map[string]any) {
	if len(blk) == 0 || a.designDoc == nil {
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
		return
	}
	doc, err := asterdoc.Parse(a.designDoc.Source)
	if err != nil {
		return
	}
	targetStepID := findTargetDesignStepID(doc, a.designSelected)
	if targetStepID == "" {
		return
	}
	if a.applyDesignRaw(map[string]any{
		"type": "insert_step_block", "stepId": targetStepID, "index": -1, "block": blk,
	}) {
		a.designSelected = targetStepID
		a.designTab = 0
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
	}
}

func (a *app) applyToolInstanceToDesign(inst blocks.Instance) {
	if a.designDoc == nil {
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
		return
	}
	doc, err := asterdoc.Parse(a.designDoc.Source)
	if err != nil {
		return
	}
	targetStepID := findTargetDesignStepID(doc, a.designSelected)
	if targetStepID == "" {
		return
	}
	blk := map[string]any{}
	idBase := fmt.Sprintf("tool-%d", time.Now().Unix()%100000)
	switch inst.Type {
	case blocks.TypeDilutionCalculator:
		var dp blocks.DilutionProps
		_ = inst.DecodeProps(&dp)
		blk = map[string]any{
			"kind": "calculator", "id": idBase + "-calc", "schema": 1, "title": "稀释配液计算", "mode": "dilution",
			"stock":        map[string]any{"value": dp.Input.StockConc, "unit": "mg/mL"},
			"target":       map[string]any{"value": dp.Input.FinalConc, "unit": "mg/mL"},
			"final_volume": map[string]any{"value": dp.Input.FinalVol, "unit": "uL"},
		}
	case blocks.TypeParameterTable:
		var pp blocks.ParameterTableProps
		_ = inst.DecodeProps(&pp)
		rows := make([]any, 0, len(pp.Rows))
		for i, r := range pp.Rows {
			var v any = r.Value
			if f, err := strconv.ParseFloat(strings.TrimSpace(r.Value), 64); err == nil {
				v = f
			}
			row := map[string]any{"key": firstNonEmpty(r.Key, fmt.Sprintf("param-%d", i+1)), "label": firstNonEmpty(r.Key, fmt.Sprintf("参数 %d", i+1)), "value": v, "record_actual": true}
			if r.Unit != "" {
				row["unit"] = r.Unit
			}
			rows = append(rows, row)
		}
		if len(rows) > 0 {
			blk = map[string]any{"kind": "param_table", "id": idBase + "-params", "schema": 1, "title": "实验参数表", "rows": rows}
		}
	case blocks.TypeChecklist:
		var cp blocks.ChecklistProps
		_ = inst.DecodeProps(&cp)
		items := make([]any, 0, len(cp.Items))
		for i, it := range cp.Items {
			items = append(items, map[string]any{"id": firstNonEmpty(it.ID, fmt.Sprintf("%s-c%d", idBase, i+1)), "label": firstNonEmpty(it.Text, "检查项"), "required": true})
		}
		if len(items) == 0 {
			items = append(items, map[string]any{"id": idBase + "-c1", "label": "确认试剂与耗材准备就绪", "required": true})
		}
		blk = map[string]any{"kind": "checklist", "id": idBase + "-chk", "schema": 1, "title": "操作检查清单", "items": items}
	case blocks.TypeMeasurementTable:
		blk = asterdoc.DefaultStarterBlock(doc, targetStepID, "data_table")
	case blocks.TypeNote:
		var np blocks.NoteProps
		_ = inst.DecodeProps(&np)
		blk = map[string]any{"kind": "prose", "id": idBase + "-note", "schema": 1, "title": firstNonEmpty(np.Title, "操作说明"), "tone": "tip", "body": firstNonEmpty(np.Text, "操作说明与质控要点")}
	case blocks.TypePlate96, blocks.TypePlate12:
		var pp blocks.Plate96Props
		_ = inst.DecodeProps(&pp)
		cols := pp.Cols
		if cols <= 0 {
			cols = 12
		}
		firstGroup := ""
		if gl := doc.GroupList(); len(gl) > 0 {
			firstGroup = gl[0].ID
		}
		if firstGroup == "" {
			blk = asterdoc.DefaultStarterBlock(doc, targetStepID, "prose")
			break
		}
		wells := []any{}
		for _, idx := range pp.Selected {
			wells = append(wells, fmt.Sprintf("%c%d", 'A'+(idx/cols), (idx%cols)+1))
		}
		if len(wells) == 0 {
			wells = []any{"A1"}
		}
		blk = map[string]any{"kind": "plate", "id": idBase + "-plate", "schema": 1, "title": "孔板排布", "format": 96, "groups": map[string]any{firstGroup: wells}}
	default:
		blk = asterdoc.DefaultStarterBlock(doc, targetStepID, "prose")
	}
	if len(blk) > 0 {
		_ = a.applyDesignRaw(map[string]any{
			"type": "insert_step_block", "stepId": targetStepID, "index": -1, "block": blk,
		})
		a.designSelected = targetStepID
		a.designTab = 0
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
	}
}

func (a *app) plateHistoryPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "plate-history.json")
}

func (a *app) loadPlateHistory() ([]tools.PlateHistoryEntry, error) {
	path := a.plateHistoryPath()
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var entries []tools.PlateHistoryEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func (a *app) savePlateHistory(entries []tools.PlateHistoryEntry) error {
	path := a.plateHistoryPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func (a *app) saveToolPlateExport(defaultName, format string, data []byte) (string, error) {
	home, _ := os.UserHomeDir()
	filterName := "PNG 图像"
	if strings.EqualFold(format, "svg") {
		filterName = "SVG 矢量图"
	}
	path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
		Parent:      mygo.FocusedWindow(),
		Title:       "导出孔板图",
		DefaultPath: filepath.Join(home, defaultName),
		Filters:     []mygo.FileFilter{{Name: filterName, Extensions: []string{strings.ToLower(format)}}},
	})
	if err != nil || path == "" {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
}

func (a *app) applyAsterPlateToDesign(source, plateID string) {
	if a.designDoc == nil {
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
		return
	}
	srcDoc, err := asterdoc.Parse(source)
	if err != nil {
		return
	}
	var srcPlate *asterdoc.Plate
	for i := range srcDoc.Modules {
		if srcDoc.Modules[i].Plate != nil && (plateID == "" || srcDoc.Modules[i].ID == plateID) {
			srcPlate = srcDoc.Modules[i].Plate
			break
		}
	}
	if srcPlate == nil {
		return
	}
	dstDoc, err := asterdoc.Parse(a.designDoc.Source)
	if err != nil {
		return
	}
	targetStepID := ""
	for _, m := range dstDoc.Modules {
		if m.Kind == "step" && m.ID == a.designSelected && moduleString(m.Data, "execution") != "organizational" {
			targetStepID = m.ID
			break
		}
	}
	if targetStepID == "" {
		for _, m := range dstDoc.Modules {
			if m.Kind == "step" && moduleString(m.Data, "execution") != "organizational" {
				targetStepID = m.ID
				break
			}
		}
	}
	if targetStepID == "" {
		return
	}

	// Sync any groups introduced inside 孔板速排 into the target Design.
	existingGroups := map[string]bool{}
	for _, g := range dstDoc.GroupList() {
		if g.ID != "" {
			existingGroups[g.ID] = true
		}
	}
	srcGroupMeta := map[string]asterdoc.GroupInfo{}
	for _, g := range srcDoc.GroupList() {
		srcGroupMeta[g.ID] = g
	}
	var groupTableID string
	var groupTableRows []any
	for _, m := range dstDoc.Modules {
		if m.Kind == "group_table" {
			groupTableID = m.ID
			if rawRows, ok := m.Data["rows"].([]any); ok {
				groupTableRows = append(groupTableRows, rawRows...)
			}
			break
		}
	}
	addedToTable := false
	for _, gid := range srcPlate.GroupNames() {
		if existingGroups[gid] {
			continue
		}
		existingGroups[gid] = true
		meta := srcGroupMeta[gid]
		gName := firstNonEmpty(meta.Name, gid)
		gRole := firstNonEmpty(meta.Role, "treatment")
		if groupTableID != "" {
			groupTableRows = append(groupTableRows, map[string]any{
				"id":   gid,
				"name": gName,
				"role": gRole,
			})
			addedToTable = true
		} else {
			_ = a.applyDesignRaw(map[string]any{
				"type": "add_module",
				"module": map[string]any{
					"kind":   "group",
					"id":     gid,
					"schema": 1,
					"name":   gName,
					"role":   gRole,
				},
			})
		}
	}
	if addedToTable && groupTableID != "" {
		_ = a.applyDesignRaw(map[string]any{
			"type":     "update_module",
			"moduleId": groupTableID,
			"props":    map[string]any{"rows": groupTableRows},
		})
	}

	idBase := fmt.Sprintf("tool-%d", time.Now().Unix()%100000)
	blk := map[string]any{
		"kind":   "plate",
		"id":     idBase + "-plate",
		"schema": 1,
		"title":  firstNonEmpty(srcPlate.Title, "孔板排布"),
		"format": srcPlate.Format,
		"groups": srcPlate.Groups,
	}
	if srcPlate.Topology != "" && srcPlate.Topology != "plate" {
		blk["topology"] = srcPlate.Topology
	}
	if srcPlate.Rows > 0 {
		blk["rows"] = srcPlate.Rows
	}
	if srcPlate.Columns > 0 {
		blk["columns"] = srcPlate.Columns
	}
	if len(srcPlate.Samples) > 0 {
		blk["samples"] = srcPlate.Samples
	}
	if len(srcPlate.Replicates) > 0 {
		blk["replicates"] = srcPlate.Replicates
	}
	if len(srcPlate.Roles) > 0 {
		blk["roles"] = srcPlate.Roles
	}
	if len(srcPlate.Factors) > 0 {
		blk["factors"] = srcPlate.Factors
	}

	if a.applyDesignRaw(map[string]any{
		"type": "insert_step_block", "stepId": targetStepID, "index": -1, "block": blk,
	}) {
		a.designSelected = targetStepID
		a.designTab = 0
		if a.shell != nil {
			a.shell.Page = shell.Design
		}
	}
}

func (a *app) pantryMetaPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "pantry-meta.json")
}

func (a *app) pantryManualsDir() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "pantry-manuals")
}

func (a *app) pantryBoxesPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "pantry-boxes.json")
}

func (a *app) loadPantryBoxes() ([]tools.PantryBox, error) {
	path := a.pantryBoxesPath()
	if path == "" {
		return []tools.PantryBox{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []tools.PantryBox{}, nil
		}
		return nil, err
	}
	var out []tools.PantryBox
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = []tools.PantryBox{}
	}
	return out, nil
}

func (a *app) savePantryBoxes(boxes []tools.PantryBox) error {
	path := a.pantryBoxesPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(boxes, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, raw, 0644); err != nil {
		return err
	}
	a.syncShellOutlineAndPantry()
	return nil
}

func (a *app) loadPantryMeta() (map[string]tools.PantryItemMeta, error) {
	path := a.pantryMetaPath()
	if path == "" {
		return map[string]tools.PantryItemMeta{}, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]tools.PantryItemMeta{}, nil
		}
		return nil, err
	}
	var out map[string]tools.PantryItemMeta
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]tools.PantryItemMeta{}
	}
	return out, nil
}

func (a *app) savePantryMeta(meta map[string]tools.PantryItemMeta) error {
	path := a.pantryMetaPath()
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0644)
}

func sanitizePantryToken(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "item"
	}
	return out
}

func (a *app) pickPantryManualFiles() ([]string, error) {
	home, _ := os.UserHomeDir()
	return mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mygo.FocusedWindow(),
		Title:       "上传试剂说明书 / COA / SDS",
		DefaultPath: home,
		Multiple:    true,
		Filters: []mygo.FileFilter{
			{Name: "试剂说明书与文档", Extensions: []string{"pdf", "png", "jpg", "jpeg", "webp", "doc", "docx", "xls", "xlsx", "csv", "txt", "md"}},
		},
	})
}

func (a *app) pickPantryStickerImage() (string, error) {
	home, _ := os.UserHomeDir()
	paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
		Parent:      mygo.FocusedWindow(),
		Title:       "选择自定义容器贴纸图片",
		DefaultPath: home,
		Filters: []mygo.FileFilter{
			{Name: "图像文件", Extensions: []string{"png", "jpg", "jpeg"}},
		},
	})
	if err != nil || len(paths) == 0 {
		return "", err
	}
	return paths[0], nil
}

func (a *app) importPantryFile(itemKey, srcPath string) (storedRel, fileName string, sizeBytes int64, err error) {
	baseDir := a.pantryManualsDir()
	if baseDir == "" {
		return srcPath, filepath.Base(srcPath), 0, nil
	}
	folder := sanitizePantryToken(itemKey)
	dstDir := filepath.Join(baseDir, folder)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return "", "", 0, err
	}
	fileName = filepath.Base(srcPath)
	ext := filepath.Ext(fileName)
	stem := strings.TrimSuffix(fileName, ext)
	candidate := fileName
	for idx := 1; ; idx++ {
		if _, statErr := os.Stat(filepath.Join(dstDir, candidate)); os.IsNotExist(statErr) {
			break
		}
		candidate = fmt.Sprintf("%s_%d%s", stem, idx, ext)
	}
	dstPath := filepath.Join(dstDir, candidate)
	in, err := os.Open(srcPath)
	if err != nil {
		return "", "", 0, err
	}
	defer in.Close()
	out, err := os.Create(dstPath)
	if err != nil {
		return "", "", 0, err
	}
	written, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return "", "", 0, copyErr
	}
	if closeErr != nil {
		return "", "", 0, closeErr
	}
	return filepath.ToSlash(filepath.Join(folder, candidate)), fileName, written, nil
}

func (a *app) resolvePantryFile(storedRel string) string {
	storedRel = strings.TrimSpace(storedRel)
	if storedRel == "" {
		return ""
	}
	if filepath.IsAbs(storedRel) {
		return storedRel
	}
	baseDir := a.pantryManualsDir()
	if baseDir == "" {
		return storedRel
	}
	cleaned := filepath.Clean(filepath.FromSlash(storedRel))
	if strings.HasPrefix(cleaned, "..") {
		return ""
	}
	return filepath.Join(baseDir, cleaned)
}

func (a *app) openPantryLocalFile(absPath string) error {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return fmt.Errorf("未找到说明书文件路径")
	}
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("说明书文件不存在：%s", filepath.Base(absPath))
	}
	return mygo.Shell.OpenPath(absPath)
}

func (a *app) openPantryExternalURL(rawURL string) error {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("仅支持 http:// 或 https:// 协议链接")
	}
	return mygo.Shell.OpenExternal(u.String())
}

func (a *app) newToolsWorkspace() *tools.Workspace {
	return tools.New(tools.Callbacks{
		UseInDesigner: func(inst blocks.Instance) {
			a.applyToolInstanceToDesign(inst)
		},
		UseAsterPlate: func(source, plateID string) {
			a.applyAsterPlateToDesign(source, plateID)
		},
		InsertAsterBlock: func(blk map[string]any) {
			a.insertAsterBlockToDesign(blk)
		},
		ListSOPs: func() []tools.SOPEntry {
			return []tools.SOPEntry{
				{
					ID: "sop-rt-qpcr", Title: "RT-qPCR 多靶点相对定量", Category: "分子生物学",
					Summary: "96 孔三复孔 × 靶基因 + SYBR MasterMix 自动配平", Source: rtQpcrAsterDoc,
				},
				{
					ID: "sop-huvec", Title: "HUVEC 水凝胶血管成管测定", Category: "细胞与组织工程",
					Summary: "12 孔水凝胶涂覆 + VEGF 梯度稀释与双时刻计数", Source: huvecAsterDoc,
				},
				{
					ID: "sop-cfu", Title: "梯度稀释涂板菌落计数 (CFU)", Category: "微生物学",
					Summary: "连续 10 倍稀释、6 孔涂布计数与自动 CFU/mL 公式", Source: cfuAsterDoc,
				},
				{
					ID: "sop-iz", Title: "90 mm 琼脂平皿四象限抑菌圈测定", Category: "微生物与抗菌",
					Summary: "CLSI 标准物理比例 SVG 四象限排布 + 游标卡尺双径均值", Source: inhibitionZoneAsterDoc,
				},
			}
		},
		ExportSOPPDF: func(title, source string) {
			if doc, err := asterdoc.Parse(source); err == nil {
				a.startDocumentPDFExport(nil, title, "", "", "", doc)
			}
		},
		CreateDesignFromSOP: func(title, source string) {
			a.createAsterDocDesign(source, title)
		},
		LoadPlateHistory: func() ([]tools.PlateHistoryEntry, error) {
			return a.loadPlateHistory()
		},
		SavePlateHistory: func(entries []tools.PlateHistoryEntry) error {
			return a.savePlateHistory(entries)
		},
		SavePlateFile: func(defaultName, format string, data []byte) (string, error) {
			return a.saveToolPlateExport(defaultName, format, data)
		},
		ListPantry: func() (runs.PantryLedger, error) {
			if a.runSvc != nil {
				return a.runSvc.ListPantry(context.Background(), runs.PantryQuery{})
			}
			return a.localListPantry()
		},
		UpsertPantry: func(input runs.PantryUpsert) (runs.PantryItem, error) {
			if a.runSvc != nil {
				return a.runSvc.UpsertPantry(context.Background(), input)
			}
			item, err := a.localUpsertPantry(input)
			if err == nil {
				a.syncPantrySnapshotToShell()
			}
			return item, err
		},
		DeletePantry: func(input runs.PantryDelete) error {
			if a.runSvc != nil {
				_, err := a.runSvc.DeletePantry(context.Background(), input)
				return err
			}
			err := a.localDeletePantry(input)
			if err == nil {
				a.syncPantrySnapshotToShell()
			}
			return err
		},
		ExportPantry: func() (runs.PantryLabels, error) {
			if a.runSvc != nil {
				return a.runSvc.ExportPantryLabels(context.Background(), runs.PantryQuery{})
			}
			return a.localExportPantryLabels()
		},
		LoadPantryMeta: func() (map[string]tools.PantryItemMeta, error) {
			return a.loadPantryMeta()
		},
		SavePantryMeta: func(meta map[string]tools.PantryItemMeta) error {
			err := a.savePantryMeta(meta)
			if err == nil {
				a.syncShellOutlineAndPantry()
			}
			return err
		},
		LoadPantryBoxes: func() ([]tools.PantryBox, error) {
			return a.loadPantryBoxes()
		},
		SavePantryBoxes: func(boxes []tools.PantryBox) error {
			return a.savePantryBoxes(boxes)
		},
		PickManualFiles: func() ([]string, error) {
			return a.pickPantryManualFiles()
		},
		PickStickerImage: func() (string, error) {
			return a.pickPantryStickerImage()
		},
		ImportPantryFile: func(itemKey, srcPath string) (string, string, int64, error) {
			return a.importPantryFile(itemKey, srcPath)
		},
		ResolvePantryFile: func(storedRel string) string {
			return a.resolvePantryFile(storedRel)
		},
		OpenLocalFile: func(absPath string) error {
			return a.openPantryLocalFile(absPath)
		},
		OpenExternalURL: func(rawURL string) error {
			return a.openPantryExternalURL(rawURL)
		},
	})
}

func toolsPage(c *ui.Context, a *app) {
	ui.Column(c).Fill().Children(func() {
		if a.tools == nil {
			a.tools = a.newToolsWorkspace()
		}
		tools.View(c, a.tools)
	})
}

func main() {
	exePath, _ := os.Executable()
	exeDir := filepath.Dir(exePath)
	rollbackNotice := updater.EvaluateStartupWatchdog(workspaceDataDir(), updater.Version, exeDir)
	if dir := workspaceDataDir(); dir != "" {
		_ = diag.Init(filepath.Join(dir, "logs"))
		defer diag.Close()
	}
	defer diag.RecoverAndLog("main")

	// A startup-URL deep link (mygo-run.exe asterlab://run/…/well/…) takes
	// precedence over the ASTERLAB_RUN_LINK automation hook. UX-IA-3.
	runLink := deepLinkArg(os.Args[1:])
	startupAsterml := astermlFileArg(os.Args[1:])
	if len(os.Args) > 1 && os.Args[1] == "--inspect-tree" {
		if err := runInspectTree(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// One running instance owns the window. A second launch — e.g. an
	// external asterlab:// click while the app runs — forwards its
	// arguments to the first instance and exits; the first instance jumps
	// to the linked Run and focuses its window. No shell is ever invoked:
	// the OS hands the URL to the exe, the exe parses it in-process.
	// UX-IA-3, UX-P5.
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}
	if strings.EqualFold(os.Getenv("ASTERLAB_THEME"), "dark") {
		mygo.Theme.SetSource(mygo.ThemeDark)
	}
	page, step := "project", "design-l929"
	_ = step

	runSvc, runErr := runs.NewRunService()
	designSvc, designErr := runs.NewDesignService()
	timeBudgetSvc, timeBudgetErr := runs.NewTimeBudgetService()

	a := &app{
		page:       &page,
		step:       &step,
		runSvc:     runSvc,
		designSvc:  designSvc,
		timeBudgetSvc: timeBudgetSvc,
		timeBudgetErr: timeBudgetErr,
		snap:       runs.RunSnapshot{Title: "加载中", Status: "loading"},
		assetRoots: map[string]string{}, runViewKinds: map[string]runworkspace.ViewKind{},
		runLinkArg: runLink,
	}
	// Per ADR-0002, `.asterml` local Vault is the single source of truth and runs
	// natively in pure Go without requiring the optional legacy Runs.dll native bridge.
	// Log missing native services quietly instead of showing a blocking error banner.
	if designErr != nil || runErr != nil || designSvc == nil {
		log.Printf("running in pure-Go local Vault mode (optional legacy native bridge unavailable: run=%v design=%v)", runErr, designErr)
	}
	log.Printf("native services: run=%v(%v) design=%v(%v) time-budget=%v(%v)", runSvc != nil, runErr, designSvc != nil, designErr, timeBudgetSvc != nil, timeBudgetErr)
	today := time.Now().Format("2006-01-02")
	a.shell = &shell.Shell{
		Project:      "",
		Page:         shell.Project,
		TodayDate:    today,
		SelectedDate: today,
		OnSelect:     func(id string) { a.selectDesign(id) },
		OnRunSelect:  func(id string) { a.selectRun(id) },
		OnTimerSelect: func(id string) {
			a.selectRun(id)
		},
		OnCreate: func() {
			targetDate := ""
			if a.shell != nil {
				targetDate = a.shell.SelectedDate
			}
			a.openNewExperimentDialog("", targetDate)
		},
		OnCreateForDate: func(dateYMD string) {
			a.openNewExperimentDialog("", dateYMD)
		},
		OnOpenTemplatePicker: func(dateYMD string) {
			folder := ""
			if a.shell != nil {
				folder = a.shell.CreateTargetFolder
				a.shell.CreateTargetFolder = ""
				if strings.TrimSpace(a.shell.CreateTargetTemplate) != "" {
					a.newExpTemplate = a.shell.CreateTargetTemplate
					a.shell.CreateTargetTemplate = ""
				}
			}
			a.openNewExperimentDialog(folder, dateYMD)
		},
		OnVaultCreateFile: func(folderRel, dateYMD string) {
			a.openNewExperimentDialog(folderRel, dateYMD)
		},
		OnVaultCreateFolder: func(parentRel, name string) {
			if a.vault != nil {
				if _, err := a.vault.CreateFolder(parentRel, name); err != nil {
					a.syncErr = fmt.Sprintf("新建项目文件夹失败：%v", err)
				} else {
					a.refreshVaultTreeAndCalendar()
				}
			}
		},
		OnVaultRename: func(relPath, newName string) {
			if a.vault == nil {
				return
			}
			newRel, err := a.vault.Rename(relPath, newName)
			if err != nil {
				a.syncErr = fmt.Sprintf("重命名失败：%v", err)
				return
			}
			if a.docStore != nil {
				a.docStore.RenameKey(relPath, newRel)
			}
			if a.shell != nil {
				a.shell.SyncFileRename(relPath, newRel, newName)
			}
			a.refreshVaultTreeAndCalendar()
			a.saveVaultWorkspace()
		},
		OnVaultMove: func(srcRel, destFolderRel string) {
			a.moveVaultFile(srcRel, destFolderRel)
		},
		OnVaultDuplicate: func(relPath, targetDate string) {
			if a.vault == nil {
				return
			}
			if dup, err := a.vault.DuplicateFile(relPath, targetDate); err != nil {
				a.syncErr = fmt.Sprintf("创建副本失败：%v", err)
			} else {
				a.refreshVaultTreeAndCalendar()
				a.openVaultFileInActivePane(dup.RelPath, dup.Title)
			}
		},
		OnVaultSaveTemplate: func(relPath string) {
			if a.vault == nil {
				return
			}
			stem := strings.TrimSuffix(filepath.Base(relPath), ".asterml")
			if _, err := a.vault.SaveAsTemplate(relPath, stem); err != nil {
				a.syncErr = fmt.Sprintf("保存为模板失败：%v", err)
			} else {
				a.refreshVaultTreeAndCalendar()
			}
		},
		OnVaultDelete: func(relPath string) {
			a.deleteVaultFile(relPath)
		},
		OnVaultRestoreTrash: func(trashName string) {
			a.restoreVaultTrash(trashName)
		},
		OnGlobalSearch: func(query string) []vault.SearchHit {
			if a.vaultIndex == nil {
				return nil
			}
			return a.vaultIndex.SearchFullText(query, 50)
		},
		OnOpenSearchHit: func(relPath, query string) {
			if strings.TrimSpace(query) != "" {
				if a.pendingFindQuery == nil {
					a.pendingFindQuery = map[string]string{}
				}
				a.pendingFindQuery[relPath] = strings.TrimSpace(query)
			}
			a.openVaultFileInActivePane(relPath, "")
		},
		OnRestoreSnapshot: func(relPath, snapshotID string) {
			a.restoreSnapshotFromHistory(relPath, snapshotID)
		},
		OnSetExperimentDate: func(relPath, dateYMD string) {
			if a.vault == nil || a.docStore == nil {
				return
			}
			if ld, err := a.docStore.Open(relPath); err == nil {
				next := asterdoc.SetFrontmatterMetadata(ld.Source, "", dateYMD, "")
				_ = ld.ApplyEdit(next)
				a.refreshVaultTreeAndCalendar()
			}
		},
		OnRevealInExplorer: func(relPath string) {
			a.revealPathInOS(relPath)
		},
		OnPickVaultPath: func() string {
			startDir := ""
			if a.vault != nil {
				startDir = a.vault.Root
			}
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
				Parent:      mygo.FocusedWindow(),
				Title:       "选择本地实验库根目录",
				DefaultPath: startDir,
				Directory:   true,
			})
			if err != nil || len(paths) == 0 {
				return ""
			}
			return paths[0]
		},
		OnSwitchVaultPath: func(newPath string) {
			a.switchVaultRoot(newPath)
		},
		OnToggleHideAssets: func(hide bool) {
			if a.vault != nil {
				a.vault.Config.HideAssetsInTree = hide
				_ = a.vault.SaveConfig()
			}
		},
		OnSettingsChanged: func() {
			a.saveVaultSettingsFromShell()
		},
		OnUndo: func() { a.undoActiveLiveDoc() },
		OnRedo: func() { a.redoActiveLiveDoc() },
		OnOpenExperiment: func(item shell.ExperimentItem) {
			if item.FilePath != "" {
				a.openVaultFileInActivePane(item.FilePath, item.Title)
				return
			}
			if item.DesignID != "" {
				a.selectDesign(item.DesignID)
			}
		},
		OnActivateTab: func(tab shell.WorkspaceTab) {
			a.saveVaultWorkspace()
		},
		OnSelectOutlineStep: func(stepID string) {
			a.designSelected = stepID
			a.scrollToStepID = stepID
		},
		OnLocatePantry: func(itemID string) {
			if a.tools == nil {
				a.tools = a.newToolsWorkspace()
			}
			a.tools.LocatePantryItem(itemID)
			if a.shell != nil {
				a.shell.OpenPantryTab()
			}
		},
		RenderTabContent: func(c *ui.Context, pane *shell.PaneNode, tab shell.WorkspaceTab) {
			a.renderVaultTabContent(c, pane, tab)
		},
		RenderPantryContent: func(c *ui.Context) {
			a.renderStandalonePantry(c)
		},
		RenderOverlays: func(c *ui.Context) {
			a.pollPDFExport(c)
			a.pollTimers()
			a.renderSyncErrorBanner(c)
			a.renderVaultNewExperimentDialog(c)
			a.renderImportDialog(c)
		},
		PaletteCommands: []shell.PaletteCommand{
			{
				ID:       "export-pdf",
				Title:    "导出当前实验 PDF",
				Subtitle: "按组件级 A4 版式导出当前聚焦的实验文档",
				OnRun: func() {
					if a.shell == nil || a.docStore == nil || a.vault == nil {
						return
					}
					tab := a.shell.ActiveTab()
					if tab == nil || tab.Kind != shell.TabFile || tab.FilePath == "" {
						a.syncErr = "请先打开一份 .asterml 实验文档，再导出 PDF"
						return
					}
					if ld, err := a.docStore.Open(tab.FilePath); err == nil && ld.Parsed != nil {
						a.startDocumentPDFExport(nil, firstNonEmpty(tab.Title, "实验方案"), "", "", a.vault.Root, ld.Parsed)
					} else if err != nil {
						a.syncErr = fmt.Sprintf("导出 PDF 失败：%v", err)
					} else {
						a.syncErr = "当前文档语法尚未闭合，请先修复后再导出 PDF"
					}
				},
			},
			{
				ID:       "open-calc",
				Title:    "打开右侧速算与计时面板",
				Subtitle: "摩尔配液 · C1·V1=C2·V2 稀释 · 计时器",
				OnRun: func() {
					if a.shell != nil {
						a.shell.SetRightDrawer(true, 1)
					}
				},
			},
		},
		OnImport: func() { a.importOpen = true },
		OnPage:   func(p shell.Page) { a.shell.Page = p; a.saveVaultWorkspace() },
		Content:  func(c *ui.Context) { contentPage(c, a) },
	}

	// runui workspace.
	cb := runui.Callbacks{
		SaveReadout: func(ms []runui.Measurement) {
			if a.ws == nil {
				return
			}
			rMs := make([]runs.Measurement, 0, len(ms))
			for _, m := range ms {
				rMs = append(rMs, runs.Measurement{SampleID: m.SampleID, Endpoint: m.Endpoint, Value: m.Value, Unit: m.Unit})
			}
			taskKey := a.ws.Selected
			timepoint, assayID, stepPath, protocolVersion := "", "", "", ""
			for _, t := range a.snap.Tasks {
				if t.Key == taskKey {
					timepoint, assayID, stepPath, protocolVersion = t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
				}
			}
			if taskKey == "" && len(a.snap.Tasks) > 0 {
				t := a.snap.Tasks[0]
				taskKey, timepoint, assayID, stepPath, protocolVersion = t.Key, t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
			}
			opID := a.nextOp()
			snap, err := a.runSvc.SaveReadout(context.Background(), runs.ReadoutInput{
				RunID:             a.snap.RunID,
				TaskKey:           taskKey,
				Timepoint:         timepoint,
				AssayID:           assayID,
				ProtocolStepPath:  stepPath,
				ProtocolVersionID: protocolVersion,
				Measurements:      rMs,
				OperationID:       opID,
			})
			if err != nil {
				a.ws.State = runui.StateBlocked
				a.ws.Error = state.MapError(err).Error()
				a.ws.Draft.Error = state.MapError(err).Error()
				return
			}
			a.clearTaskRecordingFor(a.snap.RunID, taskKey)
			a.applySnap(snap)
			if a.ws != nil {
				a.ws.State = runui.StateSaved
				if a.ws.Grid != nil {
					a.ws.Grid.State = resultgrid.StateSaved
				}
			}
			if a.runWS != nil {
				a.runWS.SetRecordingDirty(taskKey, false)
			}
		},
		SaveBatch: func(schema resultgrid.Schema, cells []resultgrid.Cell) {
			a.saveBatch(schema, cells)
		},
		SaveChecks: func(taskKey string, checks []runui.CheckAck) {
			if a.ws == nil {
				return
			}
			timepoint, assayID, stepPath, protocolVersion := "", "", "", ""
			for _, t := range a.snap.Tasks {
				if t.Key == taskKey {
					timepoint, assayID, stepPath, protocolVersion = t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
				}
			}
			acks := make([]runs.CheckAck, 0, len(checks))
			for _, check := range checks {
				acks = append(acks, runs.CheckAck{CheckID: check.CheckID, Status: check.Status})
			}
			snap, err := a.runSvc.SaveTaskChecks(context.Background(), runs.TaskCheckInput{
				RunID: a.snap.RunID, TaskKey: taskKey, OperationID: a.nextOp(),
				AssayID: assayID, Timepoint: timepoint,
				ProtocolStepPath: stepPath, ProtocolVersionID: protocolVersion,
				Checks: acks,
			})
			if err != nil {
				log.Printf("save task checks: %v", err)
				a.ws.CheckSaving = false
				a.ws.SyncChecks()
				a.ws.State = runui.StateError
				a.ws.Error = state.MapError(err).Error()
				return
			}
			a.applySnap(snap)
		},
		CompleteTask: func(taskKey string, checks []runui.CheckAck) {
			if a.ws == nil {
				return
			}
			if taskKey == "" {
				taskKey = a.ws.Selected
			}
			opID := a.nextOp()
			timepoint, assayID, stepPath, protocolVersion := "", "", "", ""
			for _, t := range a.snap.Tasks {
				if t.Key == taskKey {
					timepoint, assayID, stepPath, protocolVersion = t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
				}
			}
			acks := make([]runs.CheckAck, 0, len(checks))
			for _, check := range checks {
				acks = append(acks, runs.CheckAck{CheckID: check.CheckID, Status: check.Status})
			}
			snap, err := a.runSvc.CompleteTask(context.Background(), runs.CompleteTaskInput{
				RunID:             a.snap.RunID,
				TaskKey:           taskKey,
				OperationID:       opID,
				AssayID:           assayID,
				Timepoint:         timepoint,
				ProtocolStepPath:  stepPath,
				ProtocolVersionID: protocolVersion,
				Checks:            acks,
			})
			if err != nil {
				a.ws.State = runui.StateError
				a.ws.Error = state.MapError(err).Error()
				return
			}
			a.applySnap(snap)
			if a.ws != nil {
				a.ws.State = runui.StateReady
			}
		},
		SelectTask: func(taskKey string) {
			a.selectRunTask(taskKey)
		},
		OpenBatch: func(batchID string) {
			a.batchView = batchID
		},
	}

	// runworkspace callbacks reuse the same Core seams as legacy runui.
	rwcb := runworkspace.Callbacks{
		SelectTask: func(taskKey string) {
			a.selectRunTask(taskKey)
			a.saveSession()
		},
		ToggleTree: func() {
			a.saveSession()
		},
		SaveChecks: func(taskKey string, checks []runworkspace.CheckAckView) error {
			return a.saveRunChecks(taskKey, checks)
		},
		RetryLoad: func() {
			a.retryLoadRun()
		},
		Complete: func(taskKey string, checks []runworkspace.CheckAckView) error {
			if taskKey == "" && a.runWS != nil {
				taskKey = a.runWS.Selected
			}
			opID := a.nextOp()
			timepoint, assayID, stepPath, protocolVersion := "", "", "", ""
			for _, t := range a.snap.Tasks {
				if t.Key == taskKey {
					timepoint, assayID, stepPath, protocolVersion = t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
				}
			}
			acks := make([]runs.CheckAck, 0, len(checks))
			for _, check := range checks {
				acks = append(acks, runs.CheckAck{CheckID: check.CheckID, Status: check.Status})
			}
			snap, err := a.runSvc.CompleteTask(context.Background(), runs.CompleteTaskInput{
				RunID:             a.snap.RunID,
				TaskKey:           taskKey,
				OperationID:       opID,
				AssayID:           assayID,
				Timepoint:         timepoint,
				ProtocolStepPath:  stepPath,
				ProtocolVersionID: protocolVersion,
				Checks:            acks,
			})
			if err != nil {
				mappedErr := state.MapError(err)
				return mappedErr
			}
			a.applySnap(snap)
			return nil
		},
		OpenBatch: func(batchID string) {
			a.batchView = batchID
		},
		Recording: func(c *ui.Context, task *runworkspace.RunTaskViewModel) {
			key := runTaskCacheKey(a.snap.RunID, task.TaskKey)
			grid := (*resultgrid.Grid)(nil)
			if a.ws != nil {
				if a.ws.Selected == task.TaskKey {
					grid = a.ws.Grid
				} else {
					grid = a.taskGrids[key]
				}
			}
			if a.runWS != nil {
				a.runWS.SetRecordingDirty(task.TaskKey, grid != nil && grid.State != resultgrid.StateSaved)
			}
			linkedAssets := a.runAssetsForTask(task.TaskKey)
			kindKey := a.snap.RunID + "\x00" + task.TaskKey
			if a.runViewKinds == nil {
				a.runViewKinds = map[string]runworkspace.ViewKind{}
			}
			kind, ok := a.runViewKinds[kindKey]
			if !ok {
				kind = runworkspace.SelectKind(task, grid, linkedAssets)
			}
			linkErr := ""
			if a.linkErr != "" && (a.linkErrTask == "" || a.linkErrTask == task.TaskKey) {
				linkErr = a.linkErr
			}
			assetErr := ""
			var onRetryAsset func()
			if a.assetErr != "" && (a.assetErrTask == "" || a.assetErrTask == task.TaskKey) {
				assetErr = a.assetErr
				onRetryAsset = a.assetErrRetry
			}
			// The gallery renders the same frozen plan as the bench strip:
			// whole unphotographed wells stay visible as gaps (UX-P4).
			planWells, planFOV := a.captureWellsForTask(task.TaskKey)
			_, planTimes := a.assetCoordinateOptions(task.TaskKey)
			rv := runworkspace.RecordingView{
				RunID: a.snap.RunID, Kind: kind, Grid: grid, Assets: linkedAssets, Task: task,
				PlannedWells: planWells, PlannedTimepoints: planTimes, PlannedFOV: planFOV,
				// The Run blind switch is the single mask source; the table and
				// plate renderers hide group identity from this flag.
				Blind:           a.runWS.Bench != nil && a.runWS.Bench.Blind,
				LinkErr:         linkErr,
				AssetErr:        assetErr,
				OnRetryAssetErr: onRetryAsset,
				OnKindChange: func(kind runworkspace.ViewKind) {
					a.runViewKinds[kindKey] = kind
				},
				OnTimeline: func() {
					a.runWS.WorkspaceTab = 2
					a.runWS.SelectedTimelineTask = task.TaskKey
				},
				OnOpenAsset: func(asset runworkspace.LinkedAsset) {
					if err := a.tryOpenRunAsset(task.TaskKey, asset); err != nil {
						c.AddToast(ui.Toast{Title: "无法打开文件", Description: err.Error(), Type: components.ToastError})
					}
				},
				OnRelocate: func(asset runworkspace.LinkedAsset) {
					if err := a.tryRelocateRunAsset(task.TaskKey, asset); err != nil {
						c.AddToast(ui.Toast{Title: "文件重新定位失败", Description: err.Error(), Type: components.ToastError})
					}
				},
				OnRelabelAsset: func(asset runworkspace.LinkedAsset) {
					a.openAssetLinkForm(task.TaskKey, asset)
					if a.shell != nil {
						a.shell.Invalidate()
					}
				},
				OnUnlinkAsset: func(asset runworkspace.LinkedAsset) {
					if err := a.unlinkRunAsset(asset.AssetID); err != nil {
						a.setAssetError(task.TaskKey, "解除关联失败："+err.Error(), func() {
							_ = a.unlinkRunAsset(asset.AssetID)
						})
					} else {
						a.clearAssetError()
					}
					if a.shell != nil {
						a.shell.Invalidate()
					}
				},
				OnLinkFile: func(prefill runworkspace.LinkedAsset) {
					a.openAssetLinkForm(task.TaskKey, prefill)
					if a.shell != nil {
						a.shell.Invalidate()
					}
				},
				OnSaveBatch: func(cells []resultgrid.Cell) {
					if grid == nil || grid.State == resultgrid.StateSaving {
						return
					}
					a.startSaveBatch(grid, task.TaskKey, cells)
				},
			}
			rv.Render(c)
		},
		FetchRecord: func() (runworkspace.RecordKey, record.Projection, error) {
			return a.fetchRecord()
		},
	}
	a.tools = a.newToolsWorkspace()
	a.ws = runui.NewWorkspace(toRunView(a.snap), cb)
	a.selectedTask = a.ws.Selected
	a.runWSVM = a.buildRunWorkspaceVM()

	a.runWS = runworkspace.NewState(a.runWSVM, rwcb)
	// The Run surface renders the frozen design's modules through the same
	// blockui the Designer uses: Run for open tasks, Review once completed.
	a.ws.Modules = func(c *ui.Context, task runui.Task) {
		t := c.Theme()
		if a.snap.DesignData == "" {
			ui.Text(c, "没有冻结设计数据").FontSize(12).TextColor(t.TextMuted)
			return
		}
		design, err := designer.DecodeBlocks(a.snap.DesignData)
		if err != nil {
			ui.Text(c, "冻结设计无法读取："+err.Error()).FontSize(12).TextColor(t.Danger)
			return
		}
		insts := design.Instances[task.DesignStepID]
		if len(insts) == 0 {
			ui.Text(c, "该步骤在设计里没有模块").FontSize(12).TextColor(t.TextMuted)
		}
		mode := blockui.Run
		if task.Status == "completed" {
			mode = blockui.Review
		}
		for i := range insts {
			if blockui.View(c, &insts[i], mode) {
				// The frozen design is read-only here by contract; blockui only
				// reports edits in Design mode.
			}
		}
		allModules := sdkModulesFromDesign(a.snap.DesignData)
		refs := stepModuleRefs(a.snap.DesignData, task.DesignStepID)
		modules := make([]map[string]any, 0, len(allModules))
		for _, module := range allModules {
			if id, _ := module["id"].(string); refs[id] {
				modules = append(modules, module)
			}
		}
		if len(modules) == 0 {
			if len(allModules) > 0 {
				ui.Text(c, "该任务未绑定 SDK 模块").FontSize(t.Rem(.75)).TextColor(t.TextMuted)
			}
			return
		}
		ui.Text(c, "SDK 模块 · 本任务绑定").FontSize(t.Rem(.8)).Bold().TextColor(t.TextMuted)
		frozen := &asterdoc.View{Mode: asterdoc.ModeRun, ReadOnly: true}
		for _, module := range modules {
			kind, _ := module["kind"].(string)
			schema := 1
			switch value := module["schema"].(type) {
			case float64:
				schema = int(value)
			case int:
				schema = value
			}
			frozen.RenderModule(c, kind, schema, module)
		}
	}
	a.ws.State = runui.StateLoading
	a.ws.Width = 1200

	// A forwarded second-instance launch jumps the running app: resolve the
	// link on the main thread (the socket handler runs off-thread) and
	// focus the window. Malformed links surface the same visible shell
	// error as a startup link. UX-IA-3, UX-P5.
	mygo.App.OnSecondInstance(func(args []string, _ string) {
		link := deepLinkArg(args)
		extFile := astermlFileArg(args)
		mygo.RunOnMain(func() {
			if link != "" {
				a.openRunWellLink(link)
			} else if extFile != "" {
				a.openExternalAstermlFile(extFile)
			}
			if a.shell != nil {
				a.shell.Invalidate()
			}
			if w := mygo.FocusedWindow(); w != nil {
				w.Focus()
			}
		})
	})
	mygo.App.WhenReady(func() {
		if pngIcon := theme.AppIconPNG(); len(pngIcon) > 0 {
			_ = mygo.App.Dock.SetIcon(pngIcon)
		}
		if err := a.initVaultWorkspace(""); err != nil {
			a.syncErr = err.Error()
		}
		win := a.shell.NewWindow()
		if win != nil && win.Window() != nil {
			if pngIcon := theme.AppIconPNG(); len(pngIcon) > 0 {
				_ = win.Window().SetIcon(pngIcon)
			}
		}
		a.windowReady = true
		a.wireUpdaterCallbacks()
		a.wireFeedbackAndWelcome(rollbackNotice)
		if startupAsterml != "" {
			a.openExternalAstermlFile(startupAsterml)
		}
		a.shell.Quit()
		a.preload()
		a.startExternalSyncLoop()
		a.startAutoUpdateCheck()
		go func() {
			defer diag.RecoverAndLog("boot-watchdog")
			time.Sleep(10 * time.Second)
			updater.MarkBootHealthy(workspaceDataDir(), exeDir)
		}()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
func (a *app) saveRunChecks(taskKey string, checks []runworkspace.CheckAckView) error {
	if a.runWS == nil {
		return nil
	}
	if a.runSvc == nil {
		return fmt.Errorf("运行服务不可用")
	}
	timepoint, assayID, stepPath, protocolVersion := "", "", "", ""
	for _, t := range a.snap.Tasks {
		if t.Key == taskKey {
			timepoint, assayID, stepPath, protocolVersion = t.Timepoint, t.AssayID, taskStepPath(t), t.ProtocolVersionID
		}
	}
	acks := make([]runs.CheckAck, 0, len(checks))
	for _, check := range checks {
		acks = append(acks, runs.CheckAck{CheckID: check.CheckID, Status: check.Status})
	}
	snap, err := a.runSvc.SaveTaskChecks(context.Background(), runs.TaskCheckInput{
		RunID: a.snap.RunID, TaskKey: taskKey, OperationID: a.nextOp(),
		AssayID: assayID, Timepoint: timepoint,
		ProtocolStepPath: stepPath, ProtocolVersionID: protocolVersion,
		Checks: acks,
	})
	if err != nil {
		log.Printf("save task checks: %v", err)
		return state.MapError(err)
	}
	a.applySnap(snap)
	return nil
}

type batchSaveRequest struct {
	grid       *resultgrid.Grid
	runID      string
	taskKey    string
	input      runs.BatchReadoutInput
	totalCells int
}

type batchSaveOutcome struct {
	result  runs.BatchReadoutResult
	err     error
	snap    runs.RunSnapshot
	loadErr error
}

func (a *app) prepareBatchSaveForTask(grid *resultgrid.Grid, taskKey string, schema resultgrid.Schema, cells []resultgrid.Cell) (batchSaveRequest, bool) {
	if grid == nil && a.ws != nil {
		grid = a.ws.Grid
	}
	if a.runSvc == nil {
		msg := "运行服务不可用"
		if grid != nil {
			grid.Error = msg
			grid.State = resultgrid.StateError
		}
		if a.ws != nil {
			a.ws.State = runui.StateError
			a.ws.Error = msg
		}
		return batchSaveRequest{}, false
	}
	if grid == nil {
		return batchSaveRequest{}, false
	}
	if taskKey == "" && a.ws != nil {
		taskKey = a.ws.Selected
	}
	task := currentRunTask(a.snap.Tasks)
	for _, candidate := range a.snap.Tasks {
		if candidate.Key == taskKey {
			task = candidate
			break
		}
	}
	batchCells := make([]runs.ResultCell, 0, len(cells))
	for _, cell := range cells {
		batchCells = append(batchCells, runs.ResultCell{
			Row: cell.Row, Column: cell.Column, SampleID: cell.SampleID,
			Group: cell.Group, Replicate: cell.Replicate, Field: cell.Field,
			Value: cell.Value, Status: cell.Status,
		})
	}
	grid.State = resultgrid.StateSaving
	grid.Error = ""
	if a.ws != nil {
		a.ws.State = runui.StateSaving
		a.ws.Error = ""
	}
	cellsDigest := grid.CellsDigest()
	if grid.PendingOpID == "" || grid.PendingOpHash != cellsDigest {
		grid.PendingOpID = a.nextOp()
		grid.PendingOpHash = cellsDigest
	}
	_ = runworkspace.SaveGridDraft(a.snap.RunID, task.Key, grid)
	return batchSaveRequest{
		grid:       grid,
		runID:      a.snap.RunID,
		taskKey:    task.Key,
		totalCells: len(batchCells),
		input: runs.BatchReadoutInput{
			RunID: a.snap.RunID, OperationID: grid.PendingOpID, TaskKey: task.Key,
			AssayID: task.AssayID, Timepoint: task.Timepoint,
			ProtocolStepPath: taskStepPath(task), ProtocolVersionID: task.ProtocolVersionID,
			SchemaID: schema.ID, SchemaVersion: schema.Version, Cells: batchCells,
			ExpectedRevision: a.snap.Revision,
		},
	}, true
}

func executeBatchSave(svc runs.RunService, req batchSaveRequest) batchSaveOutcome {
	batchResult, err := svc.SaveReadoutsBatch(context.Background(), req.input)
	if err != nil {
		return batchSaveOutcome{err: err}
	}
	if batchResult.Accepted != req.totalCells {
		return batchSaveOutcome{result: batchResult}
	}
	snap, loadErr := svc.LoadRun(context.Background(), req.runID)
	return batchSaveOutcome{result: batchResult, snap: snap, loadErr: loadErr}
}

func (a *app) applyBatchSaveOutcome(req batchSaveRequest, out batchSaveOutcome) {
	setErr := func(message string) {
		if req.grid != nil {
			req.grid.Error = message
			req.grid.State = resultgrid.StateError
		}
		if a.ws != nil {
			a.ws.State = runui.StateError
			a.ws.Error = message
		}
	}
	if out.err != nil {
		setErr(state.MapError(out.err).Error())
		return
	}
	if out.result.Accepted != req.totalCells {
		setErr(fmt.Sprintf("批量保存未接受全部单元格：%d/%d", out.result.Accepted, req.totalCells))
		return
	}
	if out.loadErr != nil {
		if out.result.Revision > a.snap.Revision {
			a.snap.Revision = out.result.Revision
		}
		setErr("实验数据已保存，但界面同步未完成：" + state.MapError(out.loadErr).Error())
		return
	}
	savedGrid := req.grid
	savedGrid.PendingOpID = ""
	savedGrid.PendingOpHash = ""
	savedGrid.Error = ""
	savedGrid.State = resultgrid.StateSaved
	runworkspace.ClearGridDraft(req.runID, req.taskKey)
	a.clearTaskRecordingFor(req.runID, req.taskKey)
	a.applySnap(out.snap)
	if a.ws != nil {
		a.ws.State = runui.StateSaved
		a.ws.Error = ""
		if a.ws.Selected == req.taskKey || a.ws.Selected == "" {
			a.ws.Grid = savedGrid
		}
	}
	if a.taskGrids == nil {
		a.taskGrids = map[string]*resultgrid.Grid{}
	}
	a.taskGrids[runTaskCacheKey(req.runID, req.taskKey)] = savedGrid
	if a.runWS != nil {
		a.runWS.SetRecordingDirty(req.taskKey, false)
	}
}

func (a *app) saveBatch(schema resultgrid.Schema, cells []resultgrid.Cell) {
	if a.ws == nil {
		return
	}
	req, ok := a.prepareBatchSaveForTask(a.ws.Grid, a.ws.Selected, schema, cells)
	if !ok {
		return
	}
	out := executeBatchSave(a.runSvc, req)
	a.applyBatchSaveOutcome(req, out)
}

func (a *app) startSaveBatch(grid *resultgrid.Grid, taskKey string, cells []resultgrid.Cell) {
	if grid == nil {
		return
	}
	req, ok := a.prepareBatchSaveForTask(grid, taskKey, grid.Schema, cells)
	if !ok {
		return
	}
	if !a.windowReady {
		out := executeBatchSave(a.runSvc, req)
		a.applyBatchSaveOutcome(req, out)
		return
	}
	if a.shell != nil {
		a.shell.Invalidate()
	}
	svc := a.runSvc
	go func() {
		out := executeBatchSave(svc, req)
		mygo.RunOnMain(func() {
			a.applyBatchSaveOutcome(req, out)
			if a.shell != nil {
				a.shell.Invalidate()
			}
		})
	}()
}

func (a *app) renderSyncErrorBanner(c *ui.Context) {
	if a.syncErr == "" {
		return
	}
	t := c.Theme()
	tokens := theme.GetTokens(t)
	ui.Row(c).FillWidth().Padding(t.Space(1), t.Space(2)).
		Background(tokens.DangerBg).Border(1, tokens.DangerText).
		AlignItems(ui.Center).Gap(t.Space(1.5)).Children(func() {
		ui.Box(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, a.syncErr).FontSize(float32(tokens.TextData)).TextColor(tokens.DangerText)
		})
		if a.runSvc != nil && (a.pendingRunID != "" || a.snap.RunID != "") {
			if components.Button(c, "重新加载 Run", components.ButtonSecondary).Clicked() {
				a.retryLoadRun()
			}
		}
		if components.Button(c, "忽略", components.ButtonGhost).Clicked() {
			a.syncErr = ""
		}
	})
}

func (a *app) renderSecondaryTabContent(c *ui.Context, tab shell.WorkspaceTab) {
	tok := theme.GetTokens(c.Theme())
	if tab.Kind == shell.TabTools {
		toolsPage(c, a)
		return
	}
	src := ""
	rev := uint64(0)
	if a.designDoc != nil && a.designDoc.DesignID == tab.DesignID {
		src = a.designDoc.Source
		rev = a.designDoc.Revision
	} else if cached, ok := a.designCache[tab.DesignID]; ok {
		src = cached.Source
		rev = cached.Revision
	} else if a.designSvc != nil && tab.DesignID != "" {
		if loaded, err := a.designSvc.LoadAsterDoc(context.Background(), tab.DesignID); err == nil {
			if a.designCache == nil {
				a.designCache = map[string]runs.AsterDocSnapshot{}
			}
			a.designCache[tab.DesignID] = loaded
			src = loaded.Source
			rev = loaded.Revision
		}
	}
	if strings.TrimSpace(src) == "" {
		ui.Column(c).Fill().Center().Padding(float32(tok.Spacing16)).Gap(float32(tok.Spacing8)).Children(func() {
			components.Text(c, tab.Title).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			components.Text(c, "暂无可对照的设计内容，点击左侧工作区可继续编辑。").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
		})
		return
	}
	cacheKey := fmt.Sprintf("%s|%d", tab.DesignID, rev)
	if a.secondaryViews == nil {
		a.secondaryViews = map[string]*asterdoc.View{}
	}
	v := a.secondaryViews[cacheKey]
	if v == nil {
		doc, err := asterdoc.Parse(src)
		if err != nil {
			ui.Column(c).Fill().Center().Padding(float32(tok.Spacing16)).Children(func() {
				ui.Text(c, "方案源码存在语法待修复："+err.Error()).FontSize(float32(tok.TextMeta)).TextColor(tok.DangerText)
			})
			return
		}
		baseDir := ""
		if p := a.designDiskPath(tab.DesignID); p != "" {
			baseDir = filepath.Dir(p)
		}
		v = &asterdoc.View{
			Registry:    asterdoc.BuiltinRegistry(),
			Doc:         doc,
			Mode:        asterdoc.ModeProtocol,
			ReadOnly:    true,
			BaseDir:     baseDir,
			AsyncDecode: a.windowReady,
			OnAsyncInvalidate: func() {
				if a.shell != nil {
					a.shell.Invalidate()
				}
			},
		}
		a.secondaryViews[cacheKey] = v
	}
	ui.Scroll(c).Fill().Children(func() {
		v.RenderDocumentSheet(c)
	})
	v.RenderPlateOverlay(c)
}

func (a *app) renderExperimentMissingRunPrompt(c *ui.Context, tab shell.WorkspaceTab) {
	tok := theme.GetTokens(c.Theme())
	ui.Column(c).Fill().Center().Padding(float32(tok.Spacing24)).Gap(float32(tok.Spacing12)).Children(func() {
		ui.Text(c, fmt.Sprintf("「%s」尚未开始实验运行", tab.Title)).
			FontSize(float32(tok.TextPage)).Bold().TextColor(tok.Text)
		ui.Text(c, "当前实验尚无冻结的 Run 实例；你可以直接冻结当前方案启动运行，或先返回「设计」完善步骤。").
			FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
		ui.Row(c).AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			if components.Button(c, "开始实验", components.ButtonPrimary).Clicked() {
				if tab.DesignID != "" && (a.designDoc == nil || a.designDoc.DesignID != tab.DesignID) {
					a.selectDesign(tab.DesignID)
				}
				a.createRunFromDesignDoc()
			}
			if components.Button(c, "返回设计", components.ButtonSecondary).Clicked() {
				a.shell.SetActiveExperimentStage(shell.Design)
			}
		})
	})
}

func contentPage(c *ui.Context, a *app) {
	a.pollPDFExport(c)
	a.pollTimers()
	ui.Column(c).Fill().Children(func() {
		a.renderSyncErrorBanner(c)
		ui.Box(c).Grow(1).MinHeight(0).FillWidth().Children(func() {
			var activeTab *shell.WorkspaceTab
			if a.shell != nil {
				activeTab = a.shell.ActiveTab()
			}
			missingRun := (a.shell.Page == shell.Run || a.shell.Page == shell.Notebook) &&
				activeTab != nil && activeTab.Kind == shell.TabExperiment && activeTab.DesignID != "" &&
				((activeTab.RunID == "" && a.snap.DesignID != activeTab.DesignID) ||
					(activeTab.RunID != "" && a.snap.RunID != activeTab.RunID))
			if missingRun {
				a.renderExperimentMissingRunPrompt(c, *activeTab)
				return
			}
			switch a.shell.Page {
			case shell.Design:
				designPage(c, a)
			case shell.Run:
				runPage(c, a)
			case shell.Protocol:
				if a.tools == nil {
					a.tools = a.newToolsWorkspace()
				}
				a.tools.Active = tools.ToolSOP
				tools.View(c, a.tools)
			case shell.Notebook:
				notebookPage(c, a)
			case shell.Tools:
				toolsPage(c, a)
			}
		})
	})
	a.renderNewDesignDialog(c)
	a.renderImportDialog(c)
	a.renderDesignSwitchConfirmation(c)
	a.renderAssetLinkDialog(c)
}

func (a *app) renderAssetLinkDialog(c *ui.Context) {
	if !a.linkDialogOpen {
		return
	}
	t := c.Theme()
	tokens := theme.GetTokens(t)
	open := a.linkDialogOpen
	relabelMode := a.linkDialogAssetID != ""
	title := "关联本地文件"
	subtitle := "先标注孔位、时间点和视野；所选坐标应用于本次选择的每个文件。"
	submitLabel := "选择文件"
	if relabelMode {
		title = "重设资产坐标"
		subtitle = "为未标注资产补齐孔位、时间点和视野坐标。"
		submitLabel = "保存坐标"
	}
	components.Dialog(c, &open, components.DialogOptions{Label: title}, func(panel ui.Element) {
		panel.Width(520).Children(func() {
			ui.Text(c, title).FontSize(float32(tokens.TextBody)).Bold().TextColor(t.Text)
			ui.Text(c, subtitle).
				FontSize(float32(tokens.TextMeta)).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "孔位").FontSize(float32(tokens.TextMeta)).TextColor(t.TextMuted).Width(68)
				ui.Select(c, &a.linkDialogWell, a.linkDialogWells).FillWidth()
			})
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "时间点").FontSize(float32(tokens.TextMeta)).TextColor(t.TextMuted).Width(68)
				ui.Select(c, &a.linkDialogTimepoint, a.linkDialogTimepoints).FillWidth()
			})
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "FOV").FontSize(float32(tokens.TextMeta)).TextColor(t.TextMuted).Width(68)
				ui.TextInput(c, &a.linkDialogFOV).Placeholder("≥ 1").FillWidth()
			})
			if a.linkDialogErr != "" {
				ui.Text(c, a.linkDialogErr).FontSize(float32(tokens.TextMeta)).TextColor(t.Danger)
			}
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(t.Space(1)).Children(func() {
				if components.Button(c, "取消", components.ButtonSecondary).Clicked() {
					a.linkDialogOpen = false
					a.linkDialogAssetID = ""
				}
				submit := components.Button(c, submitLabel, components.ButtonPrimary)
				submit.Disabled(len(a.linkDialogWells) == 0 || len(a.linkDialogTimepoints) == 0)
				if submit.Clicked() {
					if relabelMode {
						a.submitAssetLinkForm(a.relabelRunAsset)
					} else {
						a.submitAssetLinkForm(a.linkRunAsset)
					}
				}
			})
		})
	})
	if !open {
		a.linkDialogOpen = false
		a.linkDialogAssetID = ""
	}
}

func (a *app) submitAssetLinkForm(linker func(string, runs.AssetLink) error) {
	coordinate, err := validateAssetCoordinates(
		a.linkDialogWell, a.linkDialogTimepoint, a.linkDialogFOV,
		a.linkDialogWells, a.linkDialogTimepoints,
	)
	if err != nil {
		a.linkDialogErr = err.Error()
		return
	}
	a.linkDialogErr = ""
	a.linkErr = ""
	a.linkErrTask = ""
	if err := linker(a.linkDialogTask, coordinate); err != nil {
		a.linkDialogErr = err.Error()
		a.linkErr = err.Error()
		a.linkErrTask = a.linkDialogTask
		if a.shell != nil {
			a.shell.Invalidate()
		}
		return
	}
	a.linkDialogOpen = false
	a.linkDialogAssetID = ""
	if a.shell != nil {
		a.shell.Invalidate()
	}
}

// batchPage shows one stored readout batch as a read-only plate. Values come
// from the execution event; nothing here can edit a cell.
func batchPage(c *ui.Context, a *app) {
	t := c.Theme()
	tokens := theme.GetTokens(t)
	var event *runs.RunEvent
	for i := range a.snap.Events {
		if a.snap.Events[i].BatchID == a.batchView {
			event = &a.snap.Events[i]
			break
		}
	}
	header := func() {
		ui.Row(c).FillWidth().Padding(12, 16).AlignItems(ui.Center).Gap(12).
			Background(tokens.CardBackground).Border(1, tokens.CardBorder).Children(func() {
			if components.Button(c, "← 返回实验记录", components.ButtonGhost).Clicked() {
				a.batchView = ""
			}
			ui.Text(c, "原始读数").FontSize(18).Bold().TextColor(t.Text)
			if event != nil {
				ui.Text(c, event.BatchID).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Spacer(c)
			ui.Text(c, "只读 · 来自保存时的执行事件").FontSize(11).TextColor(t.TextMuted)
		})
	}
	if event == nil {
		header()
		ui.Text(c, "找不到这条批次记录；它可能已失效。").Padding(t.Space(6)).TextColor(t.Danger)
		return
	}
	schema, plan, err := assays.BuildSchema([]byte(a.snap.DesignData), event.AssayID, event.Timepoint)
	if err != nil || schema.Layout != "plate96" {
		rows := []string{"A", "B", "C", "D", "E", "F", "G", "H"}
		cols := make([]string, 12)
		for i := range cols {
			cols[i] = fmt.Sprint(i + 1)
		}
		schema = resultgrid.Schema{ID: "plate.matrix.v1", Version: 1,
			Layout: "plate96", RowLabels: rows, ColumnLabels: cols,
			Fields: []resultgrid.Field{{Key: "value", Label: "测定值", Unit: "", DataType: "number"}}}
		plan = nil
	}
	_ = plan
	grid := resultgrid.New(schema, batchGridCells(event.ResultCells))
	recorded, missing, excluded := 0, 0, 0
	for _, cell := range grid.Cells {
		switch {
		case cell.Status == "excluded":
			excluded++
		case cell.Value == nil:
			missing++
		default:
			recorded++
		}
	}
	header()
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Column(c).FillWidth().Padding(16).Gap(12).Children(func() {
			ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
				meta := func(label, value string) {
					ui.Row(c).Gap(4).Children(func() {
						ui.Text(c, label).FontSize(11).TextColor(t.TextMuted)
						ui.Text(c, value).FontSize(12).TextColor(t.Text)
					})
				}
				meta("检测", event.AssayID)
				if event.Timepoint != "" {
					meta("时间点", event.Timepoint)
				}
				if event.RecordedAt != "" {
					meta("记录时间", event.RecordedAt)
				}
				meta("有效读数", fmt.Sprintf("%d", recorded))
				meta("缺失", fmt.Sprintf("%d", missing))
				if excluded > 0 {
					meta("排除", fmt.Sprintf("%d", excluded))
				}
			})
			ui.Grid(c).Columns(len(schema.ColumnLabels)).Gap(4).Children(func() {
				for _, col := range schema.ColumnLabels {
					ui.Text(c, col).FontSize(10).TextColor(t.TextMuted).Center()
				}
				for row := range schema.RowLabels {
					for col := range schema.ColumnLabels {
						cell := grid.Cells[row*len(schema.ColumnLabels)+col]
						value := "—"
						color := t.TextMuted
						if cell.Value != nil {
							value = fmt.Sprintf("%.4g", *cell.Value)
							color = t.Text
						}
						if cell.Status == "excluded" {
							value = "排除"
							color = t.Warning
						}
						ui.Column(c).Padding(4).Gap(1).Radius(4).
							Background(tokens.CardBackground).Border(1, tokens.CardBorder).Children(func() {
							ui.Text(c, schema.RowLabels[row]+schema.ColumnLabels[col]).
								FontSize(9).TextColor(t.TextMuted)
							ui.Text(c, value).FontSize(12).FontFeatures("tnum").TextColor(color)
							sample := cell.SampleID
							if sample == "" {
								sample = "未分配"
							}
							ui.Text(c, sample).FontSize(9).TextColor(t.TextMuted).
								SingleLine().Ellipsis("…")
						})
					}
				}
			})
			ui.Text(c, "孔位、样本与数值均读取自该批次事件；此页不能修改原始读数。").
				FontSize(11).TextColor(t.TextMuted)
		})
	})
}

func batchGridCells(cells []runs.ResultCell) []resultgrid.Cell {
	out := make([]resultgrid.Cell, len(cells))
	for i, cell := range cells {
		out[i] = resultgrid.Cell{Row: cell.Row, Column: cell.Column, SampleID: cell.SampleID,
			Group: cell.Group, Replicate: cell.Replicate, Field: cell.Field,
			Value: cell.Value, Status: cell.Status}
	}
	return out
}

func (a *app) refreshReadiness() {
	a.readinessErr = ""
	if a.designDoc == nil {
		a.readinessLoading = false
		a.readiness = nil
		return
	}
	if a.designSvc == nil {
		a.readinessLoading = false
		a.readiness = nil
		a.readinessErr = "实验方案核查服务暂不可用"
		return
	}
	a.readinessLoading = true
	assessment, err := a.designSvc.AssessAsterDoc(context.Background(), a.designDoc.DesignID, a.designDoc.Revision)
	a.readinessLoading = false
	if err != nil {
		a.readiness = nil
		a.readinessErr = state.MapError(err).Error()
		return
	}
	a.readiness = &assessment
}

func (a *app) renderPreflight(c *ui.Context) {
	if !a.preflightOpen || a.designDoc == nil {
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := a.preflightOpen
	components.Dialog(c, &open, components.DialogOptions{Label: "创建运行前检查"}, func(panel ui.Element) {
		panel.Width(560).Children(func() {
			ui.Text(c, "创建运行前检查").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			ui.Text(c, "开始实验前完整性核查").
				FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			if a.readinessLoading {
				ui.Text(c, "校验中 · 正在核查分组、参数与步骤完整性…").FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
				ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "返回修改", components.ButtonSecondary).Clicked() {
						a.preflightOpen = false
					}
					create := components.Button(c, "开始实验", components.ButtonPrimary)
					create.Disabled(true)
				})
				return
			}
			assessment := a.readiness
			if assessment == nil {
				ui.Text(c, "未解析 · 暂时无法完成实验方案核查").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Danger)
				if a.readinessErr != "" {
					ui.Box(c).FillWidth().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
						Background(tok.Danger.Alpha(.06)).Children(func() {
						ui.Text(c, a.readinessErr).FontSize(float32(tok.TextBody)).TextColor(tok.Danger)
					})
				}
				ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "返回修改", components.ButtonSecondary).Clicked() {
						a.preflightOpen = false
					}
					if components.Button(c, "重试", components.ButtonPrimary).Clicked() {
						a.refreshReadiness()
					}
				})
				return
			}
			if len(assessment.Errors) > 0 {
				ui.Text(c, "必须修复").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Danger)
				for _, issue := range assessment.Errors {
					ui.Row(c).Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
						Background(tok.Danger.Alpha(.06)).Children(func() {
						ui.Text(c, issue).FontSize(float32(tok.TextBody)).TextColor(tok.Danger)
					})
				}
			}
			if len(assessment.Warnings) > 0 {
				ui.Text(c, "可以确认的风险").FontSize(float32(tok.TextBody)).Bold().TextColor(tok.Warning)
				for _, warning := range assessment.Warnings {
					ui.Text(c, warning).FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
				}
				components.Checkbox(c, &a.preflightRisk, "我已了解并确认这些风险")
			}
			canCreate := len(assessment.Errors) == 0 && (len(assessment.Warnings) == 0 || a.preflightRisk)
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
				if components.Button(c, "返回修改", components.ButtonSecondary).Clicked() {
					a.preflightOpen = false
				}
				create := components.Button(c, "开始实验", components.ButtonPrimary)
				create.Disabled(!canCreate)
				if create.Clicked() && canCreate {
					a.preflightOpen = false
					a.createRunFromDesignDoc()
				}
			})
		})
	})
	if !open {
		a.preflightOpen = false
	}
}

func (a *app) renderNewDesignDialog(c *ui.Context) {
	if !a.newDesignOpen {
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := a.newDesignOpen
	createTemplate := func(src, title string) {
		if strings.TrimSpace(a.newDesignTargetDate) != "" {
			src = asterdoc.StampScheduledDate(src, a.newDesignTargetDate)
			a.newDesignTargetDate = ""
		}
		a.newDesignOpen = false
		a.createAsterDocDesign(src, title)
	}
	components.Dialog(c, &open, components.DialogOptions{Label: "加入新实验"}, func(panel ui.Element) {
		panel.Width(580).Gap(float32(tok.Spacing12)).Children(func() {
			ui.Text(c, "加入新实验").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			ui.Text(c, "从空白实验开始自由组合步骤模块，或选用通用模块示范方案。").
				FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
			ui.Box(c).FillWidth().Padding(float32(tok.Spacing12)).Radius(float32(tok.ControlRadius)).
				Background(tok.Elevated).Border(1, tok.Border).Gap(float32(tok.Spacing8)).Children(func() {
				ui.Text(c, "典型通用模块方案").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted)
				ui.Row(c).FillWidth().Wrap().Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "抑菌圈实验（四象限SVG）", components.ButtonSecondary).Clicked() {
						createTemplate(inhibitionZoneAsterDoc, "抑菌圈定量实验")
					}
					if components.Button(c, "涂板菌落计数（CFU）", components.ButtonSecondary).Clicked() {
						createTemplate(cfuAsterDoc, "梯度稀释涂板 CFU")
					}
					if components.Button(c, "HUVEC 水凝胶成管（12孔板）", components.ButtonSecondary).Clicked() {
						createTemplate(huvecAsterDoc, "HUVEC 水凝胶成管")
					}
					if components.Button(c, "RT-qPCR 多靶点复孔（96孔板）", components.ButtonSecondary).Clicked() {
						createTemplate(rtQpcrAsterDoc, "RT-qPCR 多靶点定量实验")
					}
					if components.Button(c, "CCK-8 骨架", components.ButtonSecondary).Clicked() {
						createTemplate(starterAsterDoc, "CCK-8 实验")
					}
				})
			})
			ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
				if components.Button(c, "取消", components.ButtonGhost).Clicked() {
					a.newDesignOpen = false
					a.newDesignTargetDate = ""
				}
				if components.Button(c, "空白实验", components.ButtonPrimary).Clicked() {
					createTemplate(releaseBlankTemplateDoc, "新实验")
				}
			})
		})
	})
	if !open {
		a.newDesignOpen = false
		a.newDesignTargetDate = ""
	}
}

func (a *app) renderAddModuleDialog(c *ui.Context, doc *asterdoc.Document) {
	if !a.addModuleOpen {
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := a.addModuleOpen
	groups := []string{}
	assays := []string{}
	for _, module := range doc.Modules {
		if module.Kind == "group" {
			groups = append(groups, module.ID)
		}
		if module.Kind == "assay" {
			assays = append(assays, module.ID)
		}
	}
	components.Dialog(c, &open, components.DialogOptions{Label: "添加模块"}, func(panel ui.Element) {
		panel.Width(420).Gap(float32(tok.Spacing12)).Children(func() {
			ui.Text(c, "添加模块").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
			ui.Row(c).Wrap().Gap(float32(tok.Spacing8)).Children(func() {
				if components.Button(c, "分组", components.ButtonSecondary).Clicked() {
					id := fmt.Sprintf("group-%d", len(groups)+1)
					if a.applyDesignRaw(map[string]any{"type": "add_module", "module": map[string]any{
						"kind": "group", "id": id, "schema": 1, "name": id, "role": "treatment",
					}}) {
						a.addModuleOpen = false
					}
				}
				assayBtn := components.Button(c, "检测", components.ButtonSecondary)
				assayBtn.Disabled(len(groups) == 0)
				if assayBtn.Clicked() && len(groups) > 0 {
					id := fmt.Sprintf("assay-%d", len(assays)+1)
					if a.applyDesignRaw(map[string]any{"type": "add_module", "module": map[string]any{
						"kind": "assay", "id": id, "schema": 1, "title": "新检测",
						"group_refs": groups, "timepoints": []any{},
					}}) {
						a.addModuleOpen = false
					}
				}
				plateBtn := components.Button(c, "孔板", components.ButtonSecondary)
				plateBtn.Disabled(len(assays) == 0 || len(groups) == 0)
				if plateBtn.Clicked() && len(assays) > 0 && len(groups) > 0 {
					groupMap := map[string]any{}
					for _, group := range groups {
						groupMap[group] = []any{}
					}
					id := fmt.Sprintf("plate-%d", len(doc.Modules)+1)
					if a.applyDesignRaw(map[string]any{"type": "add_module", "module": map[string]any{
						"kind": "plate", "id": id, "schema": 1, "format": 96,
						"assay": assays[0], "groups": groupMap,
					}}) {
						a.addModuleOpen = false
					}
				}
			})
			if len(groups) == 0 {
				ui.Text(c, "先添加一个分组，才能添加检测。").
					FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
			}
		})
	})
	if !open {
		a.addModuleOpen = false
	}
}

func (a *app) renderImportDialog(c *ui.Context) {
	if !a.importOpen {
		return
	}
	tok := theme.GetTokens(c.Theme())
	open := a.importOpen
	ui.Box(c.Key("vault-import-dlg")).Children(func() {
		components.Dialog(c, &open, components.DialogOptions{Label: "导入 AsterML"}, func(panel ui.Element) {
			panel.Width(560).Gap(float32(tok.Spacing12)).Children(func() {
				ui.Text(c, "导入 AsterML 实验文档").FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
				ui.Text(c, "选择本地 .asterml 或 .md 实验方案文件，导入当前实验库后即可在工作台实时编辑。").
					FontSize(float32(tok.TextBody)).TextColor(tok.TextMuted)
				ui.Text(c, "文件路径").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
				ui.Row(c).FillWidth().Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
					ui.Box(c).Grow(1).MinWidth(0).Children(func() {
						components.TextBox(c, &a.importPath, components.TextBoxOptions{}).Placeholder("D:\\...\\experiment.asterml")
					})
					if components.Button(c, "浏览文件…", components.ButtonSecondary).Clicked() {
						home, _ := os.UserHomeDir()
						paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
							Parent:      mygo.FocusedWindow(),
							Title:       "选择要导入的实验文件",
							DefaultPath: home,
							Filters:     []mygo.FileFilter{{Name: "AsterML / Markdown", Extensions: []string{"asterml", "md"}}},
						})
						if err == nil && len(paths) > 0 {
							a.importPath = paths[0]
							a.importErr = ""
							if strings.TrimSpace(a.importTitle) == "" {
								a.importTitle = strings.TrimSuffix(filepath.Base(paths[0]), filepath.Ext(paths[0]))
							}
						}
					}
				})
				ui.Text(c, "实验名称（可选，留空自动读取文件标题）").FontSize(float32(tok.TextData)).Bold().TextColor(tok.TextMuted)
				components.TextBox(c, &a.importTitle, components.TextBoxOptions{}).Placeholder("例如 HUVEC 成管实验")
				if a.importErr != "" {
					ui.Text(c, a.importErr).FontSize(float32(tok.TextData)).TextColor(tok.Danger)
				}
				ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(float32(tok.Spacing8)).Children(func() {
					if components.Button(c, "取消", components.ButtonGhost).Clicked() {
						a.importOpen = false
						a.importErr = ""
					}
					if components.Button(c, "导入并打开", components.ButtonPrimary).Clicked() {
						a.importAsterDocFile()
					}
				})
			})
		})
	})
	if !open {
		a.importOpen = false
		a.importErr = ""
	}
}

func (a *app) importAsterDocFile() {
	path := strings.TrimSpace(a.importPath)
	if path == "" {
		a.importErr = "请填写或选择文件路径"
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		a.importErr = "无法读取文件：" + err.Error()
		return
	}
	rawStr := string(raw)
	designSource := rawStr
	if env, envErr := asterdoc.ParseEnvelope(rawStr); envErr == nil && strings.TrimSpace(env.DesignSource) != "" {
		designSource = env.DesignSource
	}
	meta := asterdoc.InspectExperiment(designSource, nil)
	title := strings.TrimSpace(a.importTitle)
	if title == "" {
		title = firstNonEmpty(meta.Title, strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), "导入实验")
	}
	if a.designSvc == nil && a.vault != nil {
		dateYMD := strings.TrimSpace(meta.ScheduledDate)
		if dateYMD == "" && a.shell != nil {
			dateYMD = a.shell.SelectedDate
		}
		folder := ""
		switch a.vault.Config.DefaultProjectDir {
		case "/", "":
			if a.vault.Config.DefaultProjectDir == "" {
				folder = a.vault.Config.LastProjectDir
			}
		default:
			folder = a.vault.Config.DefaultProjectDir
		}
		entry, cErr := a.vault.CreateExperiment(folder, title, dateYMD, designSource)
		if cErr != nil {
			a.importErr = "导入到实验库失败：" + cErr.Error()
			return
		}
		a.importOpen = false
		a.importPath = ""
		a.importTitle = ""
		a.importErr = ""
		a.refreshVaultTreeAndCalendar()
		a.openVaultFileInActivePane(entry.RelPath, entry.Title)
		return
	}
	if a.designSvc == nil {
		a.importErr = "设计服务不可用"
		return
	}
	projectID, err := a.ensureDefaultProjectID()
	if err != nil {
		a.importErr = err.Error()
		return
	}
	doc, err := a.designSvc.CreateAsterDocDesign(context.Background(), runs.AsterDocCreateInput{
		ProjectID: projectID, Title: title, Source: designSource,
		OperationID: a.nextOp(), ActorID: os.Getenv("USERNAME"),
	})
	if err != nil {
		a.importErr = state.MapError(err).Error()
		return
	}
	if a.designPaths == nil {
		a.designPaths = map[string]string{}
	}
	a.designPaths[doc.DesignID] = path
	if a.designCache == nil {
		a.designCache = map[string]runs.AsterDocSnapshot{}
	}
	a.designCache[doc.DesignID] = doc
	meta = asterdoc.InspectExperiment(doc.Source, nil)
	stageLabel := "设计中"
	stageEnum := shell.StageDraft
	if meta.Complete {
		stageLabel = "方案就绪"
		stageEnum = shell.StageReady
	}
	expTitle := firstNonEmpty(meta.Title, title)
	a.importOpen = false
	a.importErr = ""
	a.designTab = 0
	a.loadDesign(doc.DesignID)
	if a.shell != nil {
		a.shell.Designs = append([]shell.DesignItem{{
			ID: doc.DesignID, Title: expTitle, Meta: stageLabel,
		}}, a.shell.Designs...)
		a.shell.Selected = doc.DesignID
		a.shell.Page = shell.Design
		a.shell.OpenExperimentTab(shell.ExperimentItem{
			ID:       doc.DesignID,
			DesignID: doc.DesignID,
			Title:    expTitle,
			Date:     firstNonEmpty(meta.ScheduledDate, a.shell.TodayDate),
			Stage:    stageEnum,
		}, shell.Design)
	}
	a.syncShellExperiments()
	a.saveSession()
}
