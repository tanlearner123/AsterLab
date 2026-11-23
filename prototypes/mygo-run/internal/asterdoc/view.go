package asterdoc

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"

	"asterlab-mygo-prototype/internal/protocol"
	"asterlab-mygo-prototype/internal/theme"
	plateui "asterlab-mygo-prototype/internal/uikit/plate"
)

// View renders an AsterML document with native modules. It never writes a
// record: a change replaces the module's fence body and reports the new source
// to OnChange, leaving validation and persistence to the caller.
type View struct {
	Doc              *Document
	Registry         *Registry
	Mode             Mode
	Density          Density
	RunID            string
	DocumentRevision string
	ReadOnly         bool
	SelectedGroup    string
	SelectedWells    []string
	// Allowed is the explicit business-command grant for this render. Mode and
	// ReadOnly never imply write access on their own.
	Allowed  map[string]bool
	OnChange func(source string)
	// OnAssignWells/OnClearWells are the Core command seam. When set, the
	// renderer reports the semantic command and never mutates the source
	// locally; the caller applies it and reloads a new revision.
	OnAssignWells func(moduleID, group string, wells []string)
	OnClearWells  func(moduleID string, wells []string)
	// OnUpdateModule reports a generic module edit intent. The renderer never
	// writes Core or the document; the host applies it and reloads a revision.
	OnUpdateModule func(moduleID string, props map[string]any)
	// Step-scoped block editing callbacks (Design mode).
	OnAddModule          func(module map[string]any)
	OnInsertStepBlock    func(stepID string, index int, block map[string]any)
	OnUpdateStepBlock    func(stepID, blockID string, props map[string]any)
	OnMoveStepBlock      func(stepID, blockID, direction string)
	OnMoveStepBlockIndex func(stepID, blockID string, toIndex int)
	OnRemoveStepBlock    func(stepID, blockID string)
	OnPickFigureFile     func(stepID, blockID string)
	StepID               string
	BaseDir              string
	currentModuleID     string
	currentModuleKind   string
	currentModuleSchema int
	currentModuleData   map[string]any
	isExportRender      bool
	titleEditID         string
	titleEditBuf       string
	// Run-mode interactive execution callbacks & state (nil-safe in read-only views).
	OnRecordActualParam  func(stepID, blockID, key, value, unit string)
	OnToggleCheck        func(stepID, checkID string, checked bool)
	OnRuleSkip           func(stepID, checkID, rule string)
	OnTimerAction        func(stepID, blockID, action string)
	OnSubmitTableReadout func(stepID, blockID string, payload map[string]any) error
	OnExcludeSample      func(sampleID, status string)
	OnLinkAsset          func(stepID, blockID, wellID string)
	OnOpenExternalAsset  func(asset AssetThumb)
	AsyncDecode          bool
	OnAsyncInvalidate    func()
	ActualParams         map[string]string
	CalcActuals          *CalcActualContext
	CheckedItems         map[string]bool
	SkippedItems         map[string]string
	TimerStates          map[string]TimerRuntimeState
	TableCells           map[string]map[string]string
	TableQualifiers      map[string]map[string]string
	TableSubmitStatus    map[string]string
	TableSubmitErrors    map[string]string
	SampleStatuses       map[string]string
	LinkedAssets         map[string][]AssetThumb
	FocusedTableCell     string
	ExpandedFigureID     string
	FigureZoom           float32
	figureOpen           bool
	ExpandedAsset        *AssetThumb
	AssetZoom            float32
	assetOpen            bool
	// UnifiedLive enables Obsidian Live-Preview single-source document behavior:
	// AltHoverPreviewQuery optionally forces the Alt+Hover NL->module preview popover on a matching line (visual showcase/testing).
	AltHoverPreviewQuery string
	// frameless prose/checklist, inline auto-save, hover block handles & `+` lines,
	// layer-based plate editing, table column popovers, and Ctrl+V figure paste.
	UnifiedLive        bool
	LabMode            bool // Anti-accidental-edit lab execution shield: locks existing prose/structure while opening execution slots & blank note areas
	MobileCompact      bool
	DocumentTitle      string
	SearchQuery        string
	DefaultPlateFormat int
	DefaultExcessPct   int
	ScrollToStepID     string
	SlashItems             []SlashCommandItem
	VaultCompletionTargets []string
	OnExecuteSlash         func(item SlashCommandItem, stepID string, insertOffset int)
	OnAttachPhotoToWell    func(plateID string, wells []string)
	OnDuplicateBlock   func(stepID, blockID string)
	OnImportAssetBytes func(data []byte, origName string) (string, error)
	OnPickAttachmentFile func(stepID, blockID string)
	OnSavePlateExport    func(defaultName, format string, data []byte) error
	OnCopyImage          func(pngBytes []byte) error
	OnLocatePantryCode   func(code string)
	OnSlashInsert      func(stepID string, afterIndex int, block map[string]any)
	PantryLocationMap  map[string]string // reagent code/name -> human box/slot location
	HideStepHeader     bool
	stepCollapsed       map[string]bool
	liveEditors         map[string]*liveEditorEntry
	nlHover             *nlHoverState
	lastFocusedLiveKey  string
	lastActiveTailKey   string
	pendingFocusLiveKey string
	pendingInjectSlash  bool
	labBlankNotes       map[string]bool
	widgetTextDrafts   map[string]*string
	widgetBoolDrafts   map[string]*bool
	anchorClock        map[string]time.Time // live single-document runtime timestamps for {#t0} etc.
	svgCache           map[string]*ui.SVG
	imgCache           map[string]ui.ImageSource
	imgLoading         map[string]bool
	imgErrCache        map[string]string
	calcScratch        map[string]map[string]*string
	// ExpandedPlate is the plate rendered in the full-screen overlay; it is
	// editor state, not document state.
	ExpandedPlate string
	// moduleDrafts stores editable text per module/field across frames.
	moduleDrafts        map[string]map[string]*string
	moduleDraftBool     map[string]map[string]*bool
	moduleLists         map[string]map[string][]string
	moduleOriginals     map[string]map[string]any
	ExpandedWells       []string
	PlateSampleID       string
	PlateReplicateStart string
	PlateBatchRepCount  int
	PlateFactorDim      string
	PlateFactorVal      string
	PlateRoleChoice     string
	PlateSampleError    string
	PlateZoom           float32
	plateColorBy        map[string]string
	plateFactorKey      map[string]string
	plateStates         map[string]*plateui.InteractionState
	plateSelections     map[string][]string
	plateInputDrafts    map[string]*string
	plateFillMode       map[string]int // 0=单值, 1=平铺列表, 2=等比序列
	plateSerialStart    map[string]*string
	plateSerialUnit     map[string]*string
	plateSerialFactor   map[string]*string
	plateSerialDir      map[string]*string // "row" | "col"
	plateAddLayerOpen   map[string]bool
	plateAddLayerName   map[string]*string
	overlayPlateState   plateui.InteractionState
	openColPopover      string // "<moduleID>:<colKey>"
	openFigSVGEdit      string
	gridPointerDown     bool
	gridPointerStart    ui.Point
	gridDragging        bool
	gridAdditive        bool
	gridSuppressClick   bool
	gridStart           ui.Point
	gridLast            ui.Point
	gridBase            []string
}

var (
	sharedBlockClipboard map[string]any
	sharedWellClipboard  []wellClipboardCell
)

type wellClipboardCell struct {
	dRow    int
	dCol    int
	group   string
	sample  string
	role    string
	factors map[string]string
}

// Can reports whether the host granted one business command.
func (v *View) Can(permission string) bool {
	return v.Allowed[permission]
}

// SetPlateSelection sets the selected well IDs for plateID in Design/Tools hosts.
func (v *View) SetPlateSelection(plateID string, wells []string) {
	if v.plateSelections == nil {
		v.plateSelections = map[string][]string{}
	}
	cp := append([]string(nil), wells...)
	sort.Strings(cp)
	v.plateSelections[plateID] = cp
}

// PlateSelection returns the currently selected well IDs for plateID.
func (v *View) PlateSelection(plateID string) []string {
	if v.plateSelections == nil {
		return nil
	}
	return append([]string(nil), v.plateSelections[plateID]...)
}

// PlateColorLayer returns the active ColorBy mode and optional FactorKey for plateID.
func (v *View) PlateColorLayer(plateID string) (colorBy, factorKey string) {
	if v.plateColorBy != nil {
		colorBy = v.plateColorBy[plateID]
	}
	if colorBy == "" {
		colorBy = "group"
	}
	if v.plateFactorKey != nil {
		factorKey = v.plateFactorKey[plateID]
	}
	return colorBy, factorKey
}

// Render draws the whole document, replacing every asterlab fence with the
// registered module renderer. In UnifiedLive mode, renders a true Obsidian-style
// Live Preview document stream interleaving editable Markdown segments, interactive
// image/relation embed cards, and native scientific modules.
func (v *View) Render(c *ui.Context) {
	if v.Doc == nil {
		return
	}
	if v.UnifiedLive {
		v.renderUnifiedLiveDocument(c)
		v.plateOverlay(c)
		return
	}
	protocol.RenderMarkdownWith(c, v.Doc.Source, v.codeBlock)
	v.plateOverlay(c)
}

func (v *View) renderUnifiedLiveDocument(c *ui.Context) {
	tok := theme.GetTokens(c.Theme())
	registry := v.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	src := v.Doc.Source
	// Defer promoting the currently edited line into a standalone SegmentEmbed card while
	// the user is actively typing on it (so `![[img.png]]{@plate:A1 ...}` never drops focus after `]]`).
	activeEditOffset := -1
	if v.liveEditors != nil {
		for _, ent := range v.liveEditors {
			if ent != nil && ent.state != nil && !ent.state.ReadOnly && ent.state.Focused() {
				// Clamp cursor to syncedText length because st.Buffer may hold newly typed bytes
				// (e.g. `{` right after `]]`) not yet committed to v.Doc.Source in this pass.
				localCur := min(ent.state.Cursor(), len(ent.syncedText))
				activeEditOffset = ent.docOffset + localCur
				break
			}
		}
	}
	segs := SplitDocumentSegmentsExcludingLine(src, v.Doc, activeEditOffset)
	stepTree := ParseStepTree(src, v.Doc)
	resolved := ResolveRelations(v.Doc)
	resolvedBySpan := make(map[int]ResolvedRelation, len(resolved))
	for _, rr := range resolved {
		if rr.SourceKind == "inline" {
			resolvedBySpan[rr.SpanStart] = rr
		}
	}
	editableDesign := v.Mode == ModeDesign && !v.LabMode && !v.ReadOnly && v.Can(PermDocumentEdit)
	labWritable := v.LabMode && !v.ReadOnly && v.Can(PermDocumentEdit)

	// Partition Prose segments at step-header / step-exit boundaries ONLY when the document
	// uses heading steps, so each STEP and Sub-step renders inside a TRUE continuous nested
	// layout container with unbroken guide rails and strictly aligned indented components!
	visualSegs := expandSegmentsForStepContainers(src, segs, stepTree)

	// Tightened vertical rhythm: 8px base gap (heading->content = 8px, component<->prose/component = 12px)
	ui.Column(c).FillWidth().Gap(float32(tok.Spacing8)).Children(func() {
		renderedTrailingEditableProse := false
		prevAnchorID := "__start__"
		embedOrdinal := 0

		renderLeftBlockHandles := func(handleKey string, insertAfterOffset int, moduleID string) {
			if !editableDesign || v.MobileCompact {
				return
			}
			ui.Row(c.Key("gutter-h:" + handleKey)).AlignItems(ui.Center).Gap(float32(tok.Spacing4) * 0.25).Shrink(0).Children(func() {
				if len(v.SlashItems) > 0 && v.OnExecuteSlash != nil {
					plusBtn := components.IconButton(c.Key("gh-add:"+handleKey), lucide.Plus(), "在此块下方插入组件", components.IconButtonCompact)
					components.Tooltip(c, plusBtn, "在下方插入组件")
					components.MenuButton(c, plusBtn, func(m *components.Menu) {
						for _, si := range v.SlashItems {
							si := si
							if m.Item(si.Label + "  (" + si.Slash + ")").Chosen() {
								v.OnExecuteSlash(si, "", insertAfterOffset)
							}
						}
					})
				}
				if moduleID != "" {
					gripBtn := components.IconButton(c.Key("gh-grip:"+handleKey), lucide.GripVertical(), "块操作菜单", components.IconButtonCompact)
					components.Tooltip(c, gripBtn, "点击打开块操作菜单")
					components.MenuButton(c, gripBtn, func(menu *components.Menu) {
						if menu.Item("上移模块").Chosen() && v.OnChange != nil {
							if next, err := MoveTopLevelModuleInSource(v.Doc.Source, moduleID, -1); err == nil {
								v.OnChange(next)
							}
						}
						if menu.Item("下移模块").Chosen() && v.OnChange != nil {
							if next, err := MoveTopLevelModuleInSource(v.Doc.Source, moduleID, 1); err == nil {
								v.OnChange(next)
							}
						}
						if menu.Item("复制模块（新 ID）").Chosen() && v.OnChange != nil {
							if next, _, err := DuplicateTopLevelModuleInSource(v.Doc.Source, moduleID); err == nil {
								v.OnChange(next)
							}
						}
						menu.Separator()
						if menu.Item("删除模块").Chosen() && v.OnChange != nil {
							if next, err := RemoveTopLevelModuleInSource(v.Doc.Source, moduleID); err == nil {
								v.OnChange(next)
							}
						}
					})
				}
			})
		}

		renderSingleSegment := func(vs visualFlowSeg, isLastVisual bool) {
			seg := vs.seg
			switch seg.Kind {
			case SegmentProse:
				displayText := seg.Text
				displayStart := seg.Start
				if seg.Index == 0 && v.DocumentTitle != "" {
					if stripped := stripDuplicateLeadingH1(displayText, v.DocumentTitle); stripped != displayText {
						if idx := strings.Index(displayText, stripped); idx >= 0 {
							displayStart += idx
							displayText = stripped
						}
					}
				}
				editStart := displayStart
				editEnd := seg.End
				if vs.isStepHeader {
					// Step header line: trim leading & trailing newlines so it sits as a crisp 1-line header
					for strings.HasPrefix(displayText, "\r\n") {
						editStart += 2
						displayText = displayText[2:]
					}
					for strings.HasPrefix(displayText, "\n") {
						editStart++
						displayText = displayText[1:]
					}
					for strings.HasSuffix(displayText, "\r\n") {
						editEnd -= 2
						displayText = displayText[:len(displayText)-2]
					}
					for strings.HasSuffix(displayText, "\n") {
						editEnd--
						displayText = displayText[:len(displayText)-1]
					}
				} else {
					if strings.HasPrefix(displayText, "\r\n") {
						editStart += 2
						displayText = displayText[2:]
					} else if strings.HasPrefix(displayText, "\n") {
						editStart++
						displayText = displayText[1:]
					}
					if !isLastVisual {
						if strings.HasSuffix(displayText, "\r\n\r\n") {
							editEnd -= 4
							displayText = displayText[:len(displayText)-4]
						} else if strings.HasSuffix(displayText, "\n\n") {
							editEnd -= 2
							displayText = displayText[:len(displayText)-2]
						} else if strings.HasSuffix(displayText, "\r\n") {
							editEnd -= 2
							displayText = displayText[:len(displayText)-2]
						} else if strings.HasSuffix(displayText, "\n") {
							editEnd--
							displayText = displayText[:len(displayText)-1]
						}
					}
				}
				if editEnd < editStart {
					editStart = seg.End
					editEnd = seg.End
				}
				editText := src[editStart:editEnd]
				trimmed := strings.TrimSpace(editText)
				segKey := vs.stableKey
				if segKey == "" {
					segKey = "prose-after:" + prevAnchorID
				}
				if vs.isStepHeader && vs.stepNode != nil {
					prevAnchorID = "stephdr-" + vs.stepNode.ID
				}

				if v.labBlankNotes == nil {
					v.labBlankNotes = map[string]bool{}
				}
				isBlankDoc := len(visualSegs) == 1 && trimmed == ""
				if labWritable && isBlankDoc {
					v.labBlankNotes[segKey] = true
				}
				if trimmed == "" && !isBlankDoc {
					if !(isLastVisual && editableDesign) {
						return
					}
				}
				isReadOnly := v.ReadOnly
				if v.LabMode {
					if !v.labBlankNotes[segKey] {
						isReadOnly = true
					}
				} else if !editableDesign {
					isReadOnly = true
				}
				placeholder := ""
				if editableDesign && isBlankDoc {
					placeholder = "开始自由记录实验笔记，或在空行首输入 / 插入孔板、分组、参数表、步骤…"
				} else if editableDesign && isLastVisual && trimmed == "" {
					placeholder = "继续输入文字，或敲 / 插入科研组件…"
				} else if labWritable && v.labBlankNotes[segKey] && trimmed == "" {
					placeholder = "实验现场补记区：记录观察现象、偏离说明或直接粘贴照片…"
				}
				if isLastVisual && !isReadOnly {
					renderedTrailingEditableProse = true
				}
				v.renderLiveMarkdownSegmentFull(
					c,
					segKey,
					"科学笔记正文",
					editText,
					editStart,
					isReadOnly,
					!v.ReadOnly,
					placeholder,
					"",
					isLastVisual,
					func(nextText string) string {
						if v.Doc == nil || v.OnChange == nil {
							return ""
						}
						if nextSrc, err := ReplaceByteRange(v.Doc.Source, editStart, editEnd, nextText); err == nil {
							v.OnChange(nextSrc)
							return nextSrc
						}
						return ""
					},
					nil,
				)

			case SegmentEmbed:
				embedOrdinal++
				embKey := fmt.Sprintf("embed-%d", embedOrdinal)
				prevAnchorID = embKey
				renderedTrailingEditableProse = false
				embRow := ui.Box(c.Key("top-emb-row:"+embKey)).FillWidth().Padding(float32(tok.Spacing4), 0, 0, 0)
				embHover := embRow.Hovered()
				embRow.Children(func() {
					v.renderEmbedCard(c, seg, resolvedBySpan, editableDesign, !v.ReadOnly)
					if editableDesign && !v.MobileCompact {
						op := float32(0)
						if embHover {
							op = 1
						}
						hWrap := ui.Box(c.Key("emb-hwrap:" + embKey)).
							Absolute().Left(-float32(tok.Spacing48)-float32(tok.Spacing4)).Top(float32(tok.Spacing8))
						hWrap.Opacity(hWrap.AnimateTo("ehop", op, theme.MotionMicroFade)).Children(func() {
							renderLeftBlockHandles(embKey, seg.End, "")
						})
					}
				})

			case SegmentModule:
				if seg.Module == nil {
					return
				}
				m := seg.Module
				if m.Kind == "workflow" {
					return
				}
				prevAnchorID = m.ID
				renderedTrailingEditableProse = false
				modRow := ui.Box(c.Key("top-mod-row:"+m.ID)).FillWidth().Padding(float32(tok.Spacing4), 0, 0, 0)
				modHover := modRow.Hovered()
				modRow.Children(func() {
					modWrap := ui.Column(c.Key("top-mod:" + m.ID)).FillWidth().Gap(float32(tok.Spacing4))
					modWrap.Children(func() {
						v.renderRegisteredModule(c, registry, m.Kind, m.Schema, m.ID, m.Data)
					})
					if editableDesign && !v.MobileCompact {
						op := float32(0)
						if modHover {
							op = 1
						}
						hWrap := ui.Box(c.Key("mod-hwrap:" + m.ID)).
							Absolute().Left(-float32(tok.Spacing48)-float32(tok.Spacing4)).Top(float32(tok.Spacing8))
						hWrap.Opacity(hWrap.AnimateTo("mhop", op, theme.MotionMicroFade)).Children(func() {
							renderLeftBlockHandles(m.ID, seg.End, m.ID)
						})
					}
					if editableDesign && m.Kind != "workflow" {
						components.ContextMenu(c, modWrap, func(menu *components.Menu) {
							if menu.Item("上移模块").Chosen() && v.OnChange != nil {
								if next, err := MoveTopLevelModuleInSource(v.Doc.Source, m.ID, -1); err == nil {
									v.OnChange(next)
								}
							}
							if menu.Item("下移模块").Chosen() && v.OnChange != nil {
								if next, err := MoveTopLevelModuleInSource(v.Doc.Source, m.ID, 1); err == nil {
									v.OnChange(next)
								}
							}
							if menu.Item("复制模块（新 ID）").Chosen() && v.OnChange != nil {
								if next, _, err := DuplicateTopLevelModuleInSource(v.Doc.Source, m.ID); err == nil {
									v.OnChange(next)
								}
							}
							menu.Separator()
							if menu.Item("删除模块").Chosen() && v.OnChange != nil {
								if next, err := RemoveTopLevelModuleInSource(v.Doc.Source, m.ID); err == nil {
									v.OnChange(next)
								}
							}
						})
					}
				})
			}
		}

		// Render segments recursively into TRUE nested Step / Sub-step containers with
		// ONE unbroken vertical guide rail per step scope (zero broken dashes, components
		// strictly aligned with their owning step!).
		var renderRange func(startIdx, endIdx int)
		renderRange = func(startIdx, endIdx int) {
			for i := startIdx; i < endIdx; {
				vs := visualSegs[i]
				if vs.isStepHeader && vs.stepNode != nil {
					sn := vs.stepNode
					// Find all following visual segments that fall inside this step's [LineStart, ScopeEnd)
					j := i + 1
					for j < endIdx && visualSegs[j].seg.Start < sn.ScopeEnd {
						j++
					}
					topGap := float32(0)
					if i > 0 {
						if sn.Depth == 0 {
							topGap = float32(tok.Spacing12)
						} else {
							topGap = float32(tok.Spacing8)
						}
					}
					scopeCol := ui.Column(c.Key("step-scope:"+sn.ID)).FillWidth().
						Padding(topGap, 0, 0, 0).Gap(float32(tok.Spacing8))
					if v.ScrollToStepID != "" && v.ScrollToStepID == sn.ID {
						scopeCol.ScrollIntoView()
						v.ScrollToStepID = ""
					}
					scopeCol.Children(func() {
						// 1. Step header line (at current container's x-axis)
						renderSingleSegment(vs, i == len(visualSegs)-1)
						// 2. Step body inside ONE continuous vertical guide rail + clean indent!
						if j > i+1 {
							railColor := tok.BorderStrong.Alpha(0.60)
							railW := float32(1.5)
							railLeftPad := float32(tok.Spacing8)
							if sn.Depth == 0 {
								railColor = tok.Accent.Alpha(0.28)
								railW = 2
								railLeftPad = float32(tok.Spacing8)
							}
							ui.Row(c.Key("step-body:"+sn.ID)).FillWidth().
								Padding(0, 0, 0, railLeftPad).
								AlignItems(ui.Stretch).Gap(float32(tok.Spacing12)).Children(func() {
								ui.Box(c).Width(railW).Radius(1).Background(railColor).Shrink(0)
								ui.Column(c).Grow(1).MinWidth(0).Gap(float32(tok.Spacing8)).Children(func() {
									renderRange(i+1, j)
								})
							})
						}
					})
					i = j
					continue
				}
				renderSingleSegment(vs, i == len(visualSegs)-1)
				i++
			}
		}
		renderRange(0, len(visualSegs))

		// Always-ready Notion-style writable trailing Markdown line in Edit Mode:
		// Guarantees that after inserting ANY top-level plate/group/calc/step/embed,
		// the user can immediately keep writing Markdown or type `/` right below it.
		if editableDesign && !renderedTrailingEditableProse {
			tailKey := "prose-after:" + prevAnchorID
			v.renderLiveMarkdownSegmentFull(
				c,
				tailKey,
				"科学笔记正文",
				"",
				len(src),
				false,
				!v.ReadOnly,
				"继续输入文字，或敲 / 插入科研组件…",
				"",
				true,
				func(nextText string) string {
					if v.Doc == nil || v.OnChange == nil || nextText == "" {
						return ""
					}
					sep := "\n"
					if strings.HasSuffix(v.Doc.Source, "\n\n") {
						sep = ""
					} else if !strings.HasSuffix(v.Doc.Source, "\n") {
						sep = "\n\n"
					}
					nextSrc := v.Doc.Source + sep + nextText
					v.OnChange(nextSrc)
					return nextSrc
				},
				nil,
			)
		}

		// Always-ready Lab Mode blank writing slot at the end of the document
		if labWritable {
			lastSeg := segs[len(segs)-1]
			lastSegKey := "prose-after:" + prevAnchorID
			if lastSeg.Kind != SegmentProse || !v.labBlankNotes[lastSegKey] {
				labTailKey := "lab-tail-note"
				draft := v.ensureWidgetDraft(labTailKey, "")
				commitLabDraft := func(textToSave string) {
					if strings.TrimSpace(textToSave) == "" || v.OnChange == nil {
						return
					}
					sep := "\n\n"
					if strings.HasSuffix(src, "\n\n") {
						sep = ""
					} else if strings.HasSuffix(src, "\n") {
						sep = "\n"
					}
					nextSrc := src + sep + strings.TrimRight(textToSave, "\r\n") + "\n"
					*draft = ""
					if v.liveEditors != nil {
						delete(v.liveEditors, labTailKey)
					}
					v.OnChange(nextSrc)
				}
				ui.Column(c.Key("lab-tail-box")).FillWidth().
					Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
					Radius(float32(tok.ControlRadius)).
					Background(tok.Elevated.Alpha(0.45)).
					Border(1, tok.Border).Gap(float32(tok.Spacing4)).Children(func() {
					ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
						ui.Icon(c, lucide.PenLine()).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(tok.Accent)
						ui.Text(c, "实验空白补记区（原文已锁防误触，此处可自由记录与贴图关联）").
							FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					})
					v.renderLiveMarkdownSegmentFull(
						c,
						labTailKey,
						"实验空白补记区",
						*draft,
						len(src),
						false,
						true,
						"在此记录现场现象、偏离或临时备注（失焦或点击写入自动追加至文档末尾）…",
						"",
						true,
						func(next string) string {
							*draft = next
							return ""
						},
						func(curText string) {
							commitLabDraft(curText)
						},
					)
					if strings.TrimSpace(*draft) != "" {
						ui.Row(c).FillWidth().Justify(ui.End).Children(func() {
							if components.Button(c, "写入实验记录", components.ButtonSecondary).Clicked() {
								commitLabDraft(*draft)
							}
						})
					}
				})
			}
		}
	})
}

// stripDuplicateLeadingH1 removes a leading Markdown `# Title` line in UnifiedLive
// mode when it duplicates the rendered inline document title, preventing double H1s.
func stripDuplicateLeadingH1(body, docTitle string) string {
	lines := strings.Split(body, "\n")
	norm := func(s string) string {
		s = strings.TrimSpace(strings.ToLower(s))
		s = strings.TrimSuffix(s, "(asterml v2)")
		return strings.TrimSpace(s)
	}
	want := norm(docTitle)
	for i, ln := range lines {
		tr := strings.TrimSpace(ln)
		if tr == "" {
			continue
		}
		if strings.HasPrefix(tr, "# ") {
			h1 := strings.TrimSpace(strings.TrimPrefix(tr, "# "))
			if want == "" || norm(h1) == want || strings.Contains(norm(h1), want) || strings.Contains(want, norm(h1)) {
				return strings.Join(append(lines[:i], lines[i+1:]...), "\n")
			}
		}
		break
	}
	return body
}

// RenderPlateOverlay draws the zoomed plate overlay when ExpandedPlate is active.
// Hosts calling StepBlockComposer (Design/Run/Notebook) should call this at page root.
func (v *View) RenderPlateOverlay(c *ui.Context) {
	v.plateOverlay(c)
}

func (v *View) codeBlock(c *ui.Context, block *markdown.CodeBlockView) {
	if block.Language != "asterlab" {
		markdown.DefaultCodeBlock(c, block)
		return
	}
	registry := v.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	module, err := parseModuleWith(block.Code(), 0, 0, registry)
	if err != nil {
		v.moduleError(c, err)
		return
	}
	v.renderRegisteredModule(c, registry, module.Kind, module.Schema, module.ID, module.Data)
}

// renderRegisteredModule checks the surface declared for this mode and then
// renders the module through the registered implementation.
func (v *View) renderUnifiedExperimentPurpose(c *ui.Context, moduleID string, data map[string]any) {
	tok := theme.GetTokens(c.Theme())
	obj := strings.TrimSpace(textValue(data, "objective"))
	unit := strings.TrimSpace(textValue(data, "experimental_unit"))
	editable := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)
	if !editable && obj == "" && unit == "" {
		return
	}
	ui.Row(c.Key("live-exp-obj:"+moduleID)).FillWidth().Radius(float32(tok.ControlRadius)).
		Background(tok.Text.Alpha(0.025)).Clip().Children(func() {
		ui.Box(c).Width(2).FillHeight().Background(tok.Accent.Alpha(0.5))
		ui.Row(c).Grow(1).MinWidth(0).Wrap().Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
			AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
			ui.Text(c, "实验目的").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.TextMuted).SingleLine().Shrink(0)
			if editable {
				buf := v.ensureWidgetDraft("exp_obj:"+moduleID, obj)
				ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*3).Children(func() {
					ti := BorderlessTextInput(c, tok, buf, float32(tok.TextBody), false, "一句话说明本实验要验证的科学问题或目标…")
					if ti.Submitted() || (!ti.Focused() && strings.TrimSpace(*buf) != obj) {
						v.emitAnyModuleEdit(moduleID, map[string]any{"objective": strings.TrimSpace(*buf)})
					}
				})
			} else if obj != "" {
				ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*3).Children(func() {
					ui.Text(c, obj).FontSize(float32(tok.TextBody)).TextColor(tok.Text)
				})
			}
			if unit != "" {
				components.Badge(c, unit)
			}
		})
	})
}

func (v *View) renderUnifiedAssayBar(c *ui.Context, moduleID string, data map[string]any) {
	tok := theme.GetTokens(c.Theme())
	title := strings.TrimSpace(textValue(data, "title"))
	endpoint := strings.TrimSpace(textValue(data, "endpoint"))
	unit := strings.TrimSpace(textValue(data, "unit"))
	tps := stringList(data, "timepoints")
	editable := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)

	// Skip empty default assay boilerplate when no endpoint/unit/custom title is set.
	if title == "主检测流程" && endpoint == "" && unit == "" {
		return
	}

	ui.Row(c.Key("live-assay:"+moduleID)).FillWidth().Wrap().
		Padding(float32(tok.Spacing8), float32(tok.Spacing12)).
		Radius(float32(tok.ControlRadius)).Background(tok.Elevated).
		AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
		components.Badge(c, "检测指标")
		if editable {
			tBuf := v.ensureWidgetDraft("assay_title:"+moduleID, title)
			eBuf := v.ensureWidgetDraft("assay_ep:"+moduleID, endpoint)
			uBuf := v.ensureWidgetDraft("assay_unit:"+moduleID, unit)
			ui.Box(c).Grow(1).MinWidth(float32(tok.Spacing48)*3).Children(func() {
				ti := BorderlessTextInput(c, tok, tBuf, float32(tok.TextData), true, "检测名称（如 CCK-8 活力）…")
				if ti.Submitted() || (!ti.Focused() && strings.TrimSpace(*tBuf) != title) {
					v.emitAnyModuleEdit(moduleID, map[string]any{"title": strings.TrimSpace(*tBuf)})
				}
			})
			ui.Text(c, "终点:").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
			ui.Box(c).Width(float32(tok.Spacing48)*2.4).Children(func() {
				ei := BorderlessTextInput(c, tok, eBuf, float32(tok.TextData), false, "OD450")
				if ei.Submitted() || (!ei.Focused() && strings.TrimSpace(*eBuf) != endpoint) {
					v.emitAnyModuleEdit(moduleID, map[string]any{"endpoint": strings.TrimSpace(*eBuf)})
				}
			})
			ui.Text(c, "单位:").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).SingleLine()
			ui.Box(c).Width(float32(tok.Spacing48)*1.5).Children(func() {
				uiIn := BorderlessTextInput(c, tok, uBuf, float32(tok.TextData), false, "AU")
				if uiIn.Submitted() || (!uiIn.Focused() && strings.TrimSpace(*uBuf) != unit) {
					v.emitAnyModuleEdit(moduleID, map[string]any{"unit": strings.TrimSpace(*uBuf)})
				}
			})
		} else {
			summary := strings.Join(nonEmpty([]string{title, endpoint, unit}), " · ")
			if summary != "" {
				ui.Text(c, summary).FontSize(float32(tok.TextData)).Bold().TextColor(tok.Text)
			}
		}
		if len(tps) > 0 {
			components.Badge(c, "时间点 "+strings.Join(tps, " / "))
		}
	})
}

func (v *View) renderRegisteredModule(c *ui.Context, registry *Registry, kind string, schema int, moduleID string, data map[string]any) {
	if v.UnifiedLive {
		switch kind {
		case "workflow":
			return
		case "experiment":
			v.renderUnifiedExperimentPurpose(c, moduleID, data)
			return
		case "assay":
			v.renderUnifiedAssayBar(c, moduleID, data)
			return
		case "step":
			prevStep := v.StepID
			v.StepID = moduleID
			defer func() { v.StepID = prevStep }()
			stepMod := Module{Kind: "step", ID: moduleID, Schema: schema, Data: data}
			renderLiveStepComposer(v, c, &stepMod)
			return
		}
	}
	spec, ok := registry.Lookup(kind, schema)
	if !ok {
		v.rendererGap(c, kind, schema, "")
		return
	}
	key, declared := spec.Descriptor.RendererKey(v.Mode)
	if !declared {
		v.rendererGap(c, kind, schema, "")
		return
	}
	if v.UnifiedLive && key == "generic.editor" {
		if docKey, docOK := spec.Descriptor.RendererKey(ModeRun); docOK && registry.HasRenderer(docKey) && docKey != "generic.run" {
			key = docKey
		} else if nbKey, nbOK := spec.Descriptor.RendererKey(ModeNotebook); nbOK && registry.HasRenderer(nbKey) && nbKey != "generic.run" {
			key = nbKey
		}
	}
	if !registry.HasRenderer(key) {
		v.rendererGap(c, kind, schema, key)
		return
	}
	// Generic editor is a registered renderer capability; dispatch by key only.
	fn, _ := registry.Renderer(key)
	if fn == nil {
		v.rendererGap(c, kind, schema, key)
		return
	}
	prevID, prevKind, prevSchema, prevData := v.currentModuleID, v.currentModuleKind, v.currentModuleSchema, v.currentModuleData
	v.currentModuleID = moduleID
	v.currentModuleKind = kind
	v.currentModuleSchema = schema
	v.currentModuleData = data
	defer func() {
		v.currentModuleID, v.currentModuleKind, v.currentModuleSchema, v.currentModuleData = prevID, prevKind, prevSchema, prevData
	}()
	if !v.isExportRender && v.StepID == "" && kind != "plate" && kind != "step" && kind != "workflow" && kind != "experiment" {
		wrap := ui.Column(c).FillWidth()
		wrap.Children(func() {
			fn(v, c, data)
		})
		components.ContextMenu(c, wrap, func(m *components.Menu) {
			v.buildComponentShareMenu(c, m, kind, schema, data)
		})
		return
	}
	fn(v, c, data)
}

// rendererGap reports a declared-but-unavailable or undeclared renderer as a
// visible capability gap. The host must never draw a guess.
func (v *View) rendererGap(c *ui.Context, kind string, schema int, key string) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	reason := "该模块未声明此视图的渲染能力"
	if key != "" {
		reason = fmt.Sprintf("渲染能力 %q 未被实现", key)
	}
	ui.Row(c).Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Radius(float32(tok.PanelRadius)).
		Background(tok.Warning.Alpha(.08)).Border(1, tok.Warning).Children(func() {
		ui.Text(c, fmt.Sprintf("%s@%d · %s", kind, schema, reason)).
			FontSize(float32(tok.TextMeta)).TextColor(tok.Warning)
	})
}

func (v *View) moduleError(c *ui.Context, err error) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	ui.Row(c).Padding(float32(tok.Spacing8), float32(tok.Spacing12)).Radius(float32(tok.PanelRadius)).
		Background(tok.Danger.Alpha(.08)).Border(1, tok.Danger).Children(func() {
		ui.Text(c, "asterlab 模块无法解析："+err.Error()).FontSize(float32(tok.TextMeta)).
			TextColor(tok.Danger)
	})
}

// RenderModule renders one already-parsed module through the same surface
// checks used for fenced AsterML blocks.
func (v *View) RenderModule(c *ui.Context, kind string, schema int, data map[string]any) {
	registry := v.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	moduleID, _ := data["id"].(string)
	v.renderRegisteredModule(c, registry, kind, schema, moduleID, data)
}

func (v *View) plate(c *ui.Context, p *Plate) {
	t := c.Theme()
	tok := theme.GetTokens(t)
	dark := t != nil && t.Dark
	factorDims := p.FactorDimensions()
	layerSet := map[string]bool{}
	var layerNames []string
	addLayerName := func(n string) {
		n = strings.TrimSpace(n)
		if n != "" && !layerSet[n] {
			layerSet[n] = true
			layerNames = append(layerNames, n)
		}
	}
	for _, d := range factorDims {
		addLayerName(d)
	}
	for d := range p.Layers {
		addLayerName(d)
	}
	sort.Strings(layerNames)

	excludedSet := map[string]bool{}
	for _, ew := range p.ExcludedWells {
		excludedSet[strings.ToUpper(strings.TrimSpace(ew))] = true
	}

	geometry, _ := geometryForPlate(p.Format, p.Topology, p.Rows, p.Columns)
	wells := map[string]plateui.WellState{}
	for well, group := range p.WellGroups() {
		fMap := copyStringMap(p.WellFactors(well))
		for lName, lVals := range p.Layers {
			if lv := strings.TrimSpace(lVals[well]); lv != "" {
				fMap[lName] = lv
			}
		}
		roleVal := p.Roles[well]
		if excludedSet[strings.ToUpper(well)] && roleVal == "" {
			roleVal = "blank"
		}
		wells[well] = plateui.WellState{
			Group:     group,
			SampleID:  p.Samples[well],
			Replicate: p.Replicates[well],
			Role:      roleVal,
			Factors:   fMap,
		}
	}
	// Include layer-only wells that don't yet belong to a group
	for lName, lVals := range p.Layers {
		for well, lv := range lVals {
			if strings.TrimSpace(lv) == "" {
				continue
			}
			ws := wells[well]
			if ws.Factors == nil {
				ws.Factors = map[string]string{}
			}
			ws.Factors[lName] = lv
			wells[well] = ws
		}
	}
	if v.plateColorBy == nil {
		v.plateColorBy = map[string]string{}
	}
	if v.plateFactorKey == nil {
		v.plateFactorKey = map[string]string{}
	}
	if v.plateStates == nil {
		v.plateStates = map[string]*plateui.InteractionState{}
	}
	if v.plateSelections == nil {
		v.plateSelections = map[string][]string{}
	}
	st := v.plateStates[p.ID]
	if st == nil {
		st = &plateui.InteractionState{}
		v.plateStates[p.ID] = st
	}
	colorBy := v.plateColorBy[p.ID]
	if colorBy == "" {
		colorBy = "group"
	}
	factorKey := v.plateFactorKey[p.ID]
	if factorKey == "" && len(layerNames) > 0 {
		factorKey = layerNames[0]
		v.plateFactorKey[p.ID] = factorKey
	}

	title := p.Title
	if strings.TrimSpace(title) == "" {
		if p.Topology == "dish" {
			title = fmt.Sprintf("培养皿分区（%d 区）", geometry.Rows*geometry.Columns)
		} else if p.Topology == "rack_1d" {
			title = fmt.Sprintf("%d 位样本架 / 泳道", geometry.Rows*geometry.Columns)
		} else {
			title = fmt.Sprintf("%d 孔板", p.Format)
		}
	}
	editable := v.Mode == ModeDesign && !v.ReadOnly && v.Can(PermDocumentEdit)
	ownerStepID := v.ownerStepForPlate(p.ID)
	selected := v.plateSelections[p.ID]
	idxMap := plateGroupPaletteMap(v.Doc, p)
	lblMap := PlateGroupLabelMap(v.Doc)

	plateIcon := lucide.Grid2x2()
	meta := fmt.Sprintf("%d 孔板 · 已排 %d 孔", p.Format, len(p.WellGroups()))
	switch p.Topology {
	case "rack_1d":
		plateIcon = lucide.Rows3()
		meta = fmt.Sprintf("%d 联管 / 泳道 · 已排 %d 位", geometry.Rows*geometry.Columns, len(p.WellGroups()))
	case "dish":
		plateIcon = lucide.CircleDot()
		meta = fmt.Sprintf("%d 区培养皿 · 已排 %d 区", geometry.Rows*geometry.Columns, len(p.WellGroups()))
	}
	if p.Timepoint != "" {
		meta += " · " + p.Timepoint
	}

	renderPlateBody := func() {
			if v.UnifiedLive {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, "图层").FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
					renderLayerPill := func(key, label string, active bool, onClick func()) {
						fgCol := tok.TextMuted
						if active {
							fgCol = tok.Accent
						}
						btn := ui.ButtonBase(c.Key("pl-lpill:" + p.ID + ":" + key)).
							Padding(float32(tok.Spacing4)*0.6, float32(tok.Spacing8)).Radius(float32(tok.ControlRadius)).
							Label(label)
						if active {
							btn.Background(tok.AccentSoftBg).Border(1, tok.Accent)
						} else {
							hoverBg := ui.Transparent
							if btn.Hovered() {
								hoverBg = tok.Text.Alpha(0.04)
							}
							btn.Background(btn.AnimateColor("lpbg", hoverBg, theme.MotionMicroFade)).
								Border(1, tok.Border)
						}
						fg := btn.AnimateColor("lpfg", fgCol, theme.MotionSnappy)
						theme.ApplyTactilePress(btn)
						btn.Children(func() {
							ui.Text(c, label).FontSize(float32(tok.TextMeta)).Bold().TextColor(fg).SingleLine()
						})
						if btn.Clicked() {
							onClick()
						}
					}
					renderLayerPill("__group__", "分组", colorBy == "group", func() {
						v.plateColorBy[p.ID] = "group"
					})
					for _, lName := range layerNames {
						lName := lName
						renderLayerPill(lName, lName, colorBy == "factor" && factorKey == lName, func() {
							v.plateColorBy[p.ID] = "factor"
							v.plateFactorKey[p.ID] = lName
						})
					}
					if editable {
						addLayerBtn := components.IconButton(c, lucide.Plus(), "新建孔板图层", components.IconButtonCompact)
						newLayerOpen := v.ensureWidgetBool("pl_new_layer_open:" + p.ID)
						if addLayerBtn.Clicked() {
							*newLayerOpen = !*newLayerOpen
						}
						components.Popover(c.Key("pl-new-layer:"+p.ID), addLayerBtn, newLayerOpen, func(panel ui.Element) {
							panel.Width(float32(tok.Spacing48)*5).Padding(float32(tok.Spacing12)).Gap(float32(tok.Spacing8)).Children(func() {
								ui.Text(c, "新建维度图层").FontSize(float32(tok.TextMeta)).Bold().TextColor(tok.Text)
								lBuf := v.ensureWidgetDraft("pl_new_layer_name:"+p.ID, "")
								tb := components.TextBox(c, lBuf, components.TextBoxOptions{}).Placeholder("图层名（如 药物浓度、时间点）")
								commitLayer := func() {
									ln := strings.TrimSpace(*lBuf)
									if ln != "" {
										nextLayers := copyNestedFactorMap(p.Layers)
										if nextLayers[ln] == nil {
											nextLayers[ln] = map[string]string{}
										}
										v.emitAnyModuleEdit(p.ID, map[string]any{"layers": nextLayers})
										v.plateColorBy[p.ID] = "factor"
										v.plateFactorKey[p.ID] = ln
										*lBuf = ""
										*newLayerOpen = false
									}
								}
								if tb.Submitted() {
									commitLayer()
								}
								if components.Button(c, "创建图层", components.ButtonPrimary).Clicked() {
									commitLayer()
								}
							})
						})
					}
				})
			}

			plateGridBox := ui.Column(c.Key("plate-grid:" + p.ID)).FillWidth()
			if v.ExpandedPlate != p.ID {
				plateGridBox.Transition(theme.HeroFlyTransition(plateHeroMatch{PlateID: p.ID}))
			}
			plateGridBox.Children(func() {
				plateui.View(c, plateui.Props{
					Wells: wells, Rows: geometry.Rows, Cols: geometry.Columns,
					Topology: p.Topology, Roles: p.Roles, Interactive: editable || (v.LabMode && !v.ReadOnly),
					SelectedIDs: selected,
					OnSelect: func(ids []string) {
						v.plateSelections[p.ID] = ids
					},
					ColorBy: colorBy, FactorKey: factorKey, GroupIndex: idxMap, GroupLabels: lblMap, ExcludedIDs: excludedSet, Zoom: v.PlateZoom, State: st,
				})
			})
			components.ContextMenu(c, plateGridBox, func(m *components.Menu) {
				if editable {
					if m.Item("清空选区标注").Disabled(len(selected) == 0).Chosen() {
						_ = v.assign(selected, "")
						v.plateSelections[p.ID] = nil
					}
					if m.Item("复制选区标注").Disabled(len(selected) == 0).Chosen() {
						v.copyPlateWellAnnotations(p, selected)
					}
					if m.Item("粘贴标注到选区").Disabled(len(selected) == 0 || len(sharedWellClipboard) == 0).Chosen() {
						v.pastePlateWellAnnotations(p, ownerStepID, selected)
					}
					m.Separator()
					if m.Item("反选孔位").Chosen() {
						v.plateSelections[p.ID] = invertPlateSelection(geometry, selected)
					}
					if m.Item("选中最外圈边缘孔").Chosen() {
						v.plateSelections[p.ID] = edgePlateWells(geometry)
					}
					if m.Item("标记 / 取消异常孔").Disabled(len(selected) == 0).Chosen() {
						v.togglePlateExcludedWells(p, selected)
					}
					m.Separator()
					// Deterministic sequential plate layout actions (row-major / col-major / optional edge-blank)
					hasEdgeBlank := false
					for _, rv := range p.Roles {
						if strings.EqualFold(rv, "blank") {
							hasEdgeBlank = true
							break
						}
					}
					applyRelayout := func(dir PlateLayoutDirection, edgeBlank bool) {
						nextGroups, nextRoles, _, relErr := RelayoutExistingPlate(v.Doc, p, dir, edgeBlank)
						if relErr != nil {
							v.PlateSampleError = relErr.Error()
							return
						}
						v.PlateSampleError = ""
						gMap := map[string]any{}
						for gid, ws := range nextGroups {
							arr := make([]any, len(ws))
							for i, w := range ws {
								arr[i] = w
							}
							gMap[gid] = arr
						}
						rMap := map[string]any{}
						for w, rv := range nextRoles {
							rMap[w] = rv
						}
						v.emitAnyModuleEdit(p.ID, map[string]any{"groups": gMap, "roles": rMap})
					}
					if m.Item("自动顺排：按行横排").Disabled(len(p.Groups) == 0).Chosen() {
						applyRelayout(LayoutRowMajor, hasEdgeBlank)
					}
					if m.Item("自动顺排：按列竖排 (8 道排枪)").Disabled(len(p.Groups) == 0).Chosen() {
						applyRelayout(LayoutColMajor, hasEdgeBlank)
					}
					edgeItemLbl := "自动排版：外周留空防蒸发 (PBS)"
					if hasEdgeBlank {
						edgeItemLbl = "自动排版：取消外周留空 (紧凑满排)"
					}
					if m.Item(edgeItemLbl).Disabled(len(p.Groups) == 0).Chosen() {
						applyRelayout(LayoutRowMajor, !hasEdgeBlank)
					}
					m.Separator()
				} else if v.LabMode && !v.ReadOnly {
					if m.Item("标记 / 取消异常孔").Disabled(len(selected) == 0).Chosen() {
						v.togglePlateExcludedWells(p, selected)
					}
					if v.OnAttachPhotoToWell != nil && m.Item("为所选孔关联照片…").Disabled(len(selected) == 0).Chosen() {
						v.OnAttachPhotoToWell(p.ID, append([]string(nil), selected...))
					}
					m.Separator()
				}
				v.buildComponentShareMenu(c, m, "plate", 1, map[string]any{"id": p.ID, "title": p.Title})
			})

			if editable {
				if len(selected) > 0 {
					v.renderPlateFloatingBar(c, p, ownerStepID, selected, func(next []string) {
						v.plateSelections[p.ID] = next
					}, false, 1, nil)
				} else {
					ui.Text(c, "拖拽框选或点击行/列号标注孔位，右键可快速排版。").
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted)
				}
			} else if v.LabMode && !v.ReadOnly && len(selected) > 0 {
				ui.Row(c).FillWidth().Wrap().AlignItems(ui.Center).Gap(float32(tok.Spacing8)).Children(func() {
					ui.Text(c, fmt.Sprintf("实验模式已选 %d 孔（设计布局已锁）", len(selected))).
						FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
					if components.Button(c, "标记 / 取消异常孔", components.ButtonOutline).Clicked() {
						v.togglePlateExcludedWells(p, selected)
					}
					ui.Spacer(c)
					if components.Button(c, "清除选择", components.ButtonGhost).Clicked() {
						v.plateSelections[p.ID] = nil
					}
				})
			}
			// Bidirectional relation strip: show images linked to selected well(s) in both Edit & Lab modes
			if len(selected) > 0 {
				v.renderPlateWellRelationsStrip(c, tok, p, selected, !v.ReadOnly)
			}
			_ = dark
	}
	renderUnifiedModuleShell(v, c, plateIcon, p.ID, title, meta, editable, func() {
		if editable && (p.Topology == "" || p.Topology == "grid_2d") {
			fmtStr := strconv.Itoa(p.Format)
			fSel := components.Select(c.Key("plate-fmt:"+p.ID), &fmtStr)
			StyleSelectQuiet(c, tok, fSel)
			fSel.Trigger.Children(func() {
				ui.Text(c, fmt.Sprintf("%d 孔", p.Format)).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
			})
			fSel.Popup(func(panel ui.Element) {
				for _, opt := range []int{4, 6, 12, 24, 48, 96, 384} {
					opt := opt
					optStr := strconv.Itoa(opt)
					fSel.Item(optStr).Children(func() {
						ui.Text(c, fmt.Sprintf("%d 孔板", opt)).FontSize(float32(tok.TextMeta)).TextColor(tok.Text).FontFeatures("tnum")
					})
				}
			})
			if fSel.Trigger.Changed() {
				if nextFmt, err := strconv.Atoi(fmtStr); err == nil && nextFmt != p.Format {
					patch := map[string]any{"format": nextFmt}
					// Refresh auto-generated default title like "96 孔板布局" -> "12 孔板布局"
					if p.Title == fmt.Sprintf("%d 孔板布局", p.Format) || p.Title == fmt.Sprintf("%d 孔板", p.Format) {
						patch["title"] = fmt.Sprintf("%d 孔板布局", nextFmt)
					}
					v.plateSelections[p.ID] = nil
					v.emitAnyModuleEdit(p.ID, patch)
				}
			}
		}
		if v.ReadOnly {
			components.Badge(c, "只读视图")
		}
		if components.IconButton(c, lucide.Maximize2(), "放大孔板", components.IconButtonCompact).Clicked() {
			v.ExpandedPlate = p.ID
			v.ExpandedWells = append([]string(nil), selected...)
			v.PlateZoom = 1
		}
	}, renderPlateBody)
}

func (v *View) ownerStepForPlate(plateID string) string {
	if v.Doc == nil || plateID == "" {
		return v.StepID
	}
	for i := range v.Doc.Modules {
		if v.Doc.Modules[i].Kind == "step" {
			if blocks, ok := v.Doc.Modules[i].Data["blocks"].([]any); ok {
				for _, raw := range blocks {
					bm, _ := raw.(map[string]any)
					if bm != nil && textValue(bm, "kind") == "plate" && textValue(bm, "id") == plateID {
						return v.Doc.Modules[i].ID
					}
				}
			}
		}
	}
	return ""
}

const (
	plateGridLabelWidth   = float32(24)
	plateGridHeaderHeight = float32(18)
	plateGridGap          = float32(4)
	plateGridWellSize     = float32(18)
)

// ponytail: the canvas is sized for the largest supported 384-well plate; grow it if larger formats are added.
func plateGridLayout(geometry Geometry, zoom float32) (viewportW, viewportH, cell float32) {
	maxGeometry, _ := geometryFor(384)
	viewportW = plateGridLabelWidth + plateGridGap +
		float32(maxGeometry.Columns)*plateGridWellSize*zoom +
		float32(maxGeometry.Columns-1)*plateGridGap
	viewportH = plateGridHeaderHeight + plateGridGap +
		float32(maxGeometry.Rows)*plateGridWellSize*zoom +
		float32(maxGeometry.Rows-1)*plateGridGap

	cellW := (viewportW - plateGridLabelWidth - plateGridGap -
		float32(geometry.Columns-1)*plateGridGap) / float32(geometry.Columns)
	cellH := (viewportH - plateGridHeaderHeight - plateGridGap -
		float32(geometry.Rows-1)*plateGridGap) / float32(geometry.Rows)
	cell = cellW
	if cellH < cell {
		cell = cellH
	}
	return viewportW, viewportH, cell
}

func (v *View) findPlate(plateID string) (plate *Plate, ownerStepID string) {
	if v.Doc == nil || plateID == "" {
		return nil, ""
	}
	for i := range v.Doc.Modules {
		if v.Doc.Modules[i].ID == plateID && v.Doc.Modules[i].Plate != nil {
			return v.Doc.Modules[i].Plate, ""
		}
		if v.Doc.Modules[i].Kind == "step" {
			if blocks, ok := v.Doc.Modules[i].Data["blocks"].([]any); ok {
				for _, raw := range blocks {
					bm, _ := raw.(map[string]any)
					if bm != nil && textValue(bm, "kind") == "plate" && textValue(bm, "id") == plateID {
						if p, err := plateFromData(bm); err == nil {
							return &p, v.Doc.Modules[i].ID
						}
					}
				}
			}
		}
	}
	return nil, ""
}

func (v *View) applyPlateProps(plateID, ownerStepID string, props map[string]any) error {
	if ownerStepID != "" {
		if v.OnUpdateStepBlock != nil {
			v.OnUpdateStepBlock(ownerStepID, plateID, props)
			return nil
		}
		if v.Doc != nil && v.Can(PermDocumentEdit) {
			if err := v.updateStepBlockInDoc(ownerStepID, plateID, props); err != nil {
				return err
			}
			if v.OnChange != nil {
				v.OnChange(v.Doc.Source)
			}
		}
		return nil
	}
	if v.OnUpdateModule != nil {
		v.OnUpdateModule(plateID, props)
		return nil
	}
	if v.Doc != nil && v.Can(PermDocumentEdit) {
		if _, err := v.Doc.UpdateModule(plateID, props); err != nil {
			return err
		}
		if v.OnChange != nil {
			v.OnChange(v.Doc.Source)
		}
	}
	return nil
}

// ClearTextDrafts resets cached inline input buffers so Undo/Redo or external
// document reloads immediately reflect the updated source values in editable fields.
func (v *View) ClearTextDrafts() {
	if v == nil {
		return
	}
	v.liveEditors = nil
	v.labBlankNotes = nil
	v.widgetTextDrafts = nil
	v.widgetBoolDrafts = nil
	v.moduleDrafts = nil
	v.moduleDraftBool = nil
	v.calcScratch = nil
	v.titleEditID = ""
	v.titleEditBuf = ""
}

type plateHeroMatch struct{ PlateID string }

// plateOverlay floats the enlarged plate over a dimmed canvas using the exact
// same unified plateui.View component and floating single-input annotation bar.
func (v *View) plateOverlay(c *ui.Context) {
	if v.ExpandedPlate == "" {
		return
	}
	plate, ownerStepID := v.findPlate(v.ExpandedPlate)
	if plate == nil {
		v.ExpandedPlate = ""
		return
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	geometry, _ := geometryForPlate(plate.Format, plate.Topology, plate.Rows, plate.Columns)
	factorDims := plate.FactorDimensions()
	if v.plateColorBy == nil {
		v.plateColorBy = map[string]string{}
	}
	if v.plateFactorKey == nil {
		v.plateFactorKey = map[string]string{}
	}
	colorBy := v.plateColorBy[plate.ID]
	if colorBy == "" {
		colorBy = "group"
	}
	factorKey := v.plateFactorKey[plate.ID]
	if factorKey == "" && len(factorDims) > 0 {
		factorKey = factorDims[0]
	}
	canEdit := v.Can(PermDocumentEdit) && !v.ReadOnly
	zoom := v.PlateZoom
	if zoom <= 0 {
		zoom = 1
	}
	closeOverlay := func() {
		v.ExpandedPlate = ""
		v.ExpandedWells = nil
	}
	wells := map[string]plateui.WellState{}
	for well, group := range plate.WellGroups() {
		wells[well] = plateui.WellState{
			Group:     group,
			SampleID:  plate.Samples[well],
			Replicate: plate.Replicates[well],
			Role:      plate.Roles[well],
			Factors:   plate.WellFactors(well),
		}
	}
	idxMap := plateGroupPaletteMap(v.Doc, plate)
	lblMap := PlateGroupLabelMap(v.Doc)

	ui.Overlay(c, func() {
		scrimCol := theme.DarkTokens().Canvas.Alpha(0.62)
		backdrop := ui.Box(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Background(scrimCol).Modal()
		if backdrop.Clicked() {
			closeOverlay()
		}
		layer := ui.Column(c).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			AlignItems(ui.Center).Justify(ui.Center).Padding(float32(tok.Spacing24)).Modal()
		if layer.OverlayShortcut(0, ui.KeyEscape) {
			closeOverlay()
		}
		layer.Children(func() {
			ui.Column(c.Key("plate-modal-col:"+plate.ID)).MaxWidth(float32(tok.Spacing48)*20).Gap(float32(tok.Spacing8)).
				AlignItems(ui.Center).Transition(theme.HeroFlyTransition(plateHeroMatch{PlateID: plate.ID})).Children(func() {
				ui.Box(c).Background(tok.Surface).Border(1, tok.Border).
					Radius(float32(tok.DialogRadius)).Shadow(0, float32(tok.Spacing12), float32(tok.Spacing32), 0, theme.DarkTokens().Canvas.Alpha(0.45)).
					Padding(float32(tok.Spacing16), float32(tok.Spacing16)).Children(func() {
					ui.Column(c).Gap(float32(tok.Spacing8)).AlignItems(ui.Center).Children(func() {
						plateTitle := plate.Title
						if strings.TrimSpace(plateTitle) == "" {
							if plate.Topology == "dish" {
								plateTitle = fmt.Sprintf("培养皿分区（%d 区）", geometry.Rows*geometry.Columns)
							} else {
								plateTitle = fmt.Sprintf("%d 孔板", plate.Format)
							}
						}
						ui.Row(c).FillWidth().AlignItems(ui.Center).Wrap().Gap(float32(tok.Spacing8)).Children(func() {
							ui.Column(c).Gap(float32(tok.Spacing4) * 0.5).Children(func() {
								ui.Text(c, plateTitle).FontSize(float32(tok.TextSection)).Bold().TextColor(tok.Text)
								meta := fmt.Sprintf("%d 孔板 · 已排 %d 孔", plate.Format, len(plate.WellGroups()))
								if plate.Topology == "rack_1d" {
									meta = fmt.Sprintf("%d 联管 / 泳道 · 已排 %d 位", geometry.Rows*geometry.Columns, len(plate.WellGroups()))
								}
								if len(v.ExpandedWells) > 0 {
									meta += fmt.Sprintf(" · 当前已选 %d 孔", len(v.ExpandedWells))
								}
								ui.Text(c, meta).FontSize(float32(tok.TextMeta)).TextColor(tok.TextMuted).FontFeatures("tnum")
							})
							ui.Spacer(c)
						})
						overlayZoom := zoom * 1.2
						if geometry.Rows*geometry.Columns > 96 {
							overlayZoom = zoom
						}
						ui.ScrollBoth(c).MaxWidth(float32(tok.Spacing48)*19).MaxHeight(580).Children(func() {
							plateui.View(c, plateui.Props{
								Wells: wells, Rows: geometry.Rows, Cols: geometry.Columns,
								Topology: plate.Topology, Roles: plate.Roles,
								Interactive: canEdit,
								SelectedIDs: v.ExpandedWells,
								OnSelect: func(ids []string) {
									if canEdit {
										v.ExpandedWells = ids
									}
								},
								ColorBy: colorBy, FactorKey: factorKey, GroupIndex: idxMap, GroupLabels: lblMap,
								Zoom: overlayZoom, State: &v.overlayPlateState,
							})
						})
					})
				})

				if canEdit {
					v.renderPlateFloatingBar(c, plate, ownerStepID, v.ExpandedWells, func(next []string) {
						v.ExpandedWells = next
					}, true, zoom, closeOverlay)
				} else {
					ui.Row(c).Justify(ui.Center).Children(func() {
						if components.Button(c, "关闭放大视图", components.ButtonSecondary).Clicked() {
							closeOverlay()
						}
					})
				}
			})
		})
	})
}

func overlayWellTag(props plateui.Props, ws plateui.WellState, role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "ntc":
		return "NTC"
	case "blank":
		return "BLK"
	case "standard":
		return "STD"
	case "positive_control":
		return "+"
	case "negative_control":
		return "-"
	case "vehicle":
		return "V"
	}
	if props.ColorBy == "replicate" && ws.Replicate > 0 {
		return fmt.Sprintf("#%d", ws.Replicate)
	}
	if props.ColorBy == "factor" && props.FactorKey != "" && ws.Factors[props.FactorKey] != "" {
		rs := []rune(ws.Factors[props.FactorKey])
		if len(rs) > 3 {
			return string(rs[:3])
		}
		return ws.Factors[props.FactorKey]
	}
	if ws.Replicate > 0 {
		return fmt.Sprintf("%d", ws.Replicate)
	}
	return ""
}

func overlayContrastInk(bg ui.Color, tok theme.Tokens) ui.Color {
	if bg.A == 0 {
		return tok.Text
	}
	lum := (0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B)) / 255.0
	if lum > 0.54 {
		return theme.LightTokens().Text
	}
	return theme.DarkTokens().Text
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// ApplyPlateSamples maps one sample ID onto the selected wells. A positive
// startReplicate assigns successive replicate numbers in deterministic
// well order; zero leaves replicate labels absent. Existing mappings for
// other wells are preserved.
func ApplyPlateSamples(geometry Geometry, groups map[string][]string, samples map[string]string, replicates map[string]int, wells []string, sampleID string, startReplicate int) (map[string]string, map[string]int, error) {
	return ApplyPlateSamplesWithFactors(geometry, groups, samples, replicates, nil, wells, sampleID, startReplicate)
}

// ApplyPlateSamplesWithFactors maps one sample ID onto selected wells while
// scoping technical replicate uniqueness by (sampleID, factorSignature, replicate).
func ApplyPlateSamplesWithFactors(geometry Geometry, groups map[string][]string, samples map[string]string, replicates map[string]int, factors map[string]map[string]string, wells []string, sampleID string, startReplicate int) (map[string]string, map[string]int, error) {
	sampleID = strings.TrimSpace(sampleID)
	if sampleID == "" {
		return nil, nil, fmt.Errorf("sample ID cannot be blank")
	}
	if startReplicate < 0 {
		return nil, nil, fmt.Errorf("replicate start must be positive")
	}
	groupByWell := map[string]string{}
	for group, assigned := range groups {
		for _, well := range assigned {
			groupByWell[strings.ToUpper(strings.TrimSpace(well))] = strings.TrimSpace(group)
		}
	}
	outSamples := make(map[string]string, len(samples)+len(wells))
	for well, id := range samples {
		outSamples[strings.ToUpper(strings.TrimSpace(well))] = strings.TrimSpace(id)
	}
	outReplicates := make(map[string]int, len(replicates)+len(wells))
	for well, n := range replicates {
		outReplicates[strings.ToUpper(strings.TrimSpace(well))] = n
	}
	selected := make([]string, 0, len(wells))
	seen := map[string]bool{}
	for _, well := range wells {
		well = strings.ToUpper(strings.TrimSpace(well))
		if well == "" {
			return nil, nil, fmt.Errorf("well ID cannot be blank")
		}
		if seen[well] {
			continue
		}
		seen[well] = true
		selected = append(selected, well)
	}
	sort.Slice(selected, func(i, j int) bool {
		return wellLess(geometry, selected[i], selected[j])
	})
	if startReplicate > 0 && len(selected) > 0 {
		maxInt := int(^uint(0) >> 1)
		if startReplicate > maxInt-(len(selected)-1) {
			return nil, nil, fmt.Errorf("replicate range overflows integer")
		}
	}
	selectedSet := make(map[string]bool, len(selected))
	for _, well := range selected {
		selectedSet[well] = true
	}
	factorSigs := factorSignatureMap(factors)
	replicateOwner := map[string]string{}
	for well, replicate := range outReplicates {
		if selectedSet[well] || replicate < 1 || outSamples[well] == "" {
			continue
		}
		replicateOwner[outSamples[well]+"\x00"+factorSigs[well]+"\x00"+strconv.Itoa(replicate)] = well
	}
	for i, well := range selected {
		if !validWell(well, geometry) {
			return nil, nil, fmt.Errorf("invalid well %q", well)
		}
		if _, ok := groupByWell[well]; !ok {
			return nil, nil, fmt.Errorf("well %s has no group assignment", well)
		}
		outSamples[well] = sampleID
		if startReplicate > 0 {
			replicate := startReplicate + i
			key := sampleID + "\x00" + factorSigs[well] + "\x00" + strconv.Itoa(replicate)
			if previous, exists := replicateOwner[key]; exists {
				return nil, nil, fmt.Errorf("sample %s replicate %d is already assigned to %s", sampleID, replicate, previous)
			}
			replicateOwner[key] = well
			outReplicates[well] = replicate
		} else {
			delete(outReplicates, well)
		}
	}
	return outSamples, outReplicates, nil
}

// ApplyPlateBatchSamples splits comma/newline-separated sample IDs evenly across selected wells
// with automatic per-sample replicate numbering (#1..#repCount).
func ApplyPlateBatchSamples(geometry Geometry, groups map[string][]string, samples map[string]string, replicates map[string]int, factors map[string]map[string]string, wells []string, sampleSpec string, repCount int) (map[string]string, map[string]int, error) {
	if repCount < 1 {
		repCount = 1
	}
	rawTokens := strings.FieldsFunc(sampleSpec, func(r rune) bool {
		return r == ',' || r == '，' || r == '\n' || r == '\r' || r == ';' || r == '；'
	})
	var sampleList []string
	for _, tok := range rawTokens {
		if t := strings.TrimSpace(tok); t != "" {
			sampleList = append(sampleList, t)
		}
	}
	if len(sampleList) == 0 {
		return nil, nil, fmt.Errorf("请填写至少一个样本 ID")
	}
	groupByWell := map[string]string{}
	for group, assigned := range groups {
		for _, well := range assigned {
			groupByWell[strings.ToUpper(strings.TrimSpace(well))] = strings.TrimSpace(group)
		}
	}
	outSamples := make(map[string]string, len(samples)+len(wells))
	for well, id := range samples {
		outSamples[strings.ToUpper(strings.TrimSpace(well))] = strings.TrimSpace(id)
	}
	outReplicates := make(map[string]int, len(replicates)+len(wells))
	for well, n := range replicates {
		outReplicates[strings.ToUpper(strings.TrimSpace(well))] = n
	}
	selected := make([]string, 0, len(wells))
	seen := map[string]bool{}
	for _, well := range wells {
		w := strings.ToUpper(strings.TrimSpace(well))
		if w == "" || seen[w] {
			continue
		}
		if !validWell(w, geometry) {
			return nil, nil, fmt.Errorf("无效孔位 %q", w)
		}
		if _, ok := groupByWell[w]; !ok {
			return nil, nil, fmt.Errorf("孔位 %s 尚未分配实验分组", w)
		}
		seen[w] = true
		selected = append(selected, w)
	}
	sort.Slice(selected, func(i, j int) bool {
		return wellLess(geometry, selected[i], selected[j])
	})
	selectedSet := map[string]bool{}
	for _, w := range selected {
		selectedSet[w] = true
	}
	factorSigs := factorSignatureMap(factors)
	replicateOwner := map[string]string{}
	for well, rep := range outReplicates {
		if selectedSet[well] || rep < 1 || outSamples[well] == "" {
			continue
		}
		replicateOwner[outSamples[well]+"\x00"+factorSigs[well]+"\x00"+strconv.Itoa(rep)] = well
	}
	for idx, w := range selected {
		sIdx := (idx / repCount) % len(sampleList)
		sid := sampleList[sIdx]
		rep := (idx % repCount) + 1
		// Advance rep if same sample wraps or repeats in a non-orthogonal block
		for {
			key := sid + "\x00" + factorSigs[w] + "\x00" + strconv.Itoa(rep)
			if _, taken := replicateOwner[key]; !taken {
				replicateOwner[key] = w
				break
			}
			rep++
		}
		outSamples[w] = sid
		outReplicates[w] = rep
	}
	return outSamples, outReplicates, nil
}

// ApplyPlateFactor assigns (or clears when factorVal=="") an orthogonal factor value across selected wells.
func ApplyPlateFactor(geometry Geometry, groups map[string][]string, factors map[string]map[string]string, wells []string, factorDim, factorVal string) (map[string]map[string]string, error) {
	dim := strings.TrimSpace(factorDim)
	if dim == "" {
		return nil, fmt.Errorf("请填写因子维度名称（如 靶点基因、加药浓度）")
	}
	val := strings.TrimSpace(factorVal)
	groupByWell := map[string]string{}
	for group, assigned := range groups {
		for _, well := range assigned {
			groupByWell[strings.ToUpper(strings.TrimSpace(well))] = strings.TrimSpace(group)
		}
	}
	out := make(map[string]map[string]string, len(factors)+1)
	for d, m := range factors {
		sub := make(map[string]string, len(m))
		for k, v := range m {
			sub[k] = v
		}
		out[d] = sub
	}
	if out[dim] == nil {
		out[dim] = map[string]string{}
	}
	for _, well := range wells {
		w := strings.ToUpper(strings.TrimSpace(well))
		if !validWell(w, geometry) {
			return nil, fmt.Errorf("无效孔位 %q", w)
		}
		if _, ok := groupByWell[w]; !ok {
			return nil, fmt.Errorf("孔位 %s 尚未分配实验分组", w)
		}
		if val == "" {
			delete(out[dim], w)
		} else {
			out[dim][w] = val
		}
	}
	if len(out[dim]) == 0 {
		delete(out, dim)
	}
	return out, nil
}

// ApplyPlateRole assigns a control/sample role across selected wells.
func ApplyPlateRole(geometry Geometry, roles map[string]string, wells []string, role string) (map[string]string, error) {
	r := strings.TrimSpace(role)
	out := make(map[string]string, len(roles)+len(wells))
	for w, v := range roles {
		out[strings.ToUpper(strings.TrimSpace(w))] = v
	}
	for _, well := range wells {
		w := strings.ToUpper(strings.TrimSpace(well))
		if !validWell(w, geometry) {
			return nil, fmt.Errorf("无效孔位 %q", w)
		}
		if r == "" {
			delete(out, w)
		} else {
			out[w] = r
		}
	}
	return out, nil
}

func wellLess(geometry Geometry, a, b string) bool {
	ra, ca := wellRowCol(a)
	rb, cb := wellRowCol(b)
	if ra != rb {
		return ra < rb
	}
	return ca < cb
}

func wellRowCol(well string) (int, int) {
	if len(well) < 2 {
		return 0, 0
	}
	row := int(well[0] - 'A')
	col := 0
	for _, r := range well[1:] {
		col = col*10 + int(r-'0')
	}
	return row, col
}

func removeString(values []string, target string) []string {
	out := values[:0]
	for _, value := range values {
		if value != target {
			out = append(out, value)
		}
	}
	return out
}

func groupColorForCtx(c *ui.Context, group string) ui.Color {
	t := c.Theme()
	return plateui.GroupColor(group, t != nil && t.Dark)
}

// Assign moves wells into group in the current document and reports the new
// source. An empty group removes the wells. It is exported for tests and for a
// future source editor to call outside a render pass.
func (v *View) Assign(wells []string, group string) error {
	return v.assign(wells, group)
}

func (v *View) assign(wells []string, group string) error {
	if !v.Can(PermDocumentEdit) {
		return fmt.Errorf("permission %s is not granted", PermDocumentEdit)
	}
	if v.Doc == nil {
		return fmt.Errorf("document has no plate module")
	}
	var module *Module
	for i := range v.Doc.Modules {
		candidate := &v.Doc.Modules[i]
		if candidate.Plate == nil || (v.ExpandedPlate != "" && candidate.ID != v.ExpandedPlate) {
			continue
		}
		module = candidate
		break
	}
	if module == nil {
		return fmt.Errorf("document has no plate module")
	}
	groups := map[string][]string{}
	for name, existing := range module.Plate.Groups {
		groups[name] = append([]string(nil), existing...)
	}
	target := strings.TrimSpace(group)
	for _, well := range wells {
		well = strings.ToUpper(strings.TrimSpace(well))
		for name, existing := range groups {
			kept := existing[:0]
			for _, candidate := range existing {
				if candidate != well {
					kept = append(kept, candidate)
				}
			}
			groups[name] = kept
		}
		if target != "" {
			groups[target] = append(groups[target], well)
		}
	}
	// Deterministic order inside each group.
	for name := range groups {
		sort.Strings(groups[name])
		groups[name] = dedupe(groups[name])
	}
	source, err := v.Doc.UpdatePlate(module.ID, groups)
	if err != nil {
		return err
	}
	if v.OnChange != nil {
		v.OnChange(source)
	}
	return nil
}

func dedupe(values []string) []string {
	if len(values) < 2 {
		return values
	}
	out := values[:1]
	for _, value := range values[1:] {
		if value != out[len(out)-1] {
			out = append(out, value)
		}
	}
	return out
}

func (v *View) ensureDraft(moduleID string, data map[string]any) (map[string]*string, map[string]*bool) {
	if v.moduleDrafts == nil {
		v.moduleDrafts = map[string]map[string]*string{}
	}
	if v.moduleDraftBool == nil {
		v.moduleDraftBool = map[string]map[string]*bool{}
	}
	if v.moduleOriginals == nil {
		v.moduleOriginals = map[string]map[string]any{}
	}
	draft, ok := v.moduleDrafts[moduleID]
	orig := v.moduleOriginals[moduleID]
	changed := !ok
	if ok {
		// reset if original data changed
		for k, val := range data {
			if !sameValue(orig[k], val) {
				changed = true
				break
			}
		}
	}
	if changed {
		draft = map[string]*string{}
		bools := map[string]*bool{}
		lists := map[string][]string{}
		for key, val := range data {
			if key == "kind" || key == "id" || key == "schema" {
				continue
			}
			if items, ok := val.([]any); ok {
				values := make([]string, 0, len(items))
				allStrings := true
				for _, item := range items {
					text, isText := item.(string)
					if !isText {
						allStrings = false
						break
					}
					values = append(values, text)
				}
				if allStrings {
					lists[key] = values
					continue
				}
			}
			switch typed := val.(type) {
			case bool:
				b := typed
				bools[key] = &b
				draft[key] = nil
			case map[string]any:
				if _, ok := typed["value"]; ok {
					vs := fmt.Sprint(typed["value"])
					draft[key] = &vs
					if u, ok := typed["unit"].(string); ok {
						us := u
						draft[key+"__unit"] = &us
					}
				} else {
					vs := fmt.Sprint(typed)
					draft[key] = &vs
				}
			default:
				vs := fmt.Sprint(val)
				draft[key] = &vs
			}
		}
		v.moduleDrafts[moduleID] = draft
		v.moduleDraftBool[moduleID] = bools
		if v.moduleLists == nil {
			v.moduleLists = map[string]map[string][]string{}
		}
		v.moduleLists[moduleID] = lists
		clone := map[string]any{}
		for k, val := range data {
			clone[k] = val
		}
		v.moduleOriginals[moduleID] = clone
	}
	if v.moduleDraftBool[moduleID] == nil {
		v.moduleDraftBool[moduleID] = map[string]*bool{}
	}
	return v.moduleDrafts[moduleID], v.moduleDraftBool[moduleID]
}

func sameValue(a, b any) bool {
	return fmt.Sprint(a) == fmt.Sprint(b)
}

func (v *View) genericModuleEditor(c *ui.Context, moduleID string, data map[string]any) {
	v.GenericModuleEditor(c, moduleID, data)
}

func (v *View) GenericModuleEditor(c *ui.Context, moduleID string, data map[string]any) {
	t := c.Theme()
	kind, _ := data["kind"].(string)
	schema := 1
	switch value := data["schema"].(type) {
	case int:
		schema = value
	case int64:
		schema = int(value)
	case float64:
		schema = int(value)
	}
	registry := v.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	spec, ok := registry.Lookup(kind, schema)
	if !ok {
		v.moduleError(c, fmt.Errorf("no editor for %s@%d", kind, schema))
		return
	}
	draft, bools := v.ensureDraft(moduleID, data)
	canEdit := v.Can(PermDocumentEdit) && !v.ReadOnly
	ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(t.Space(3)).Radius(t.Space(1.5)).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		tok := theme.GetTokens(t)
		ui.Text(c, spec.Descriptor.Title).FontSize(float32(tok.TextSection)).Bold().TextColor(t.Text)
		for _, input := range spec.Descriptor.Inputs {
			if input.Name == "kind" || input.Name == "id" || input.Name == "schema" {
				continue
			}
			// Composite ports are never edited through the scalar draft. Giving
			// them an empty string draft made a plain save write "" over the
			// stored value (a plate's groups, a step's tasks).
			switch input.Type {
			case "string[]", "list", "object", "table", "object-list", "well-map", "geometry", "artifact":
			default:
				if draft[input.Name] == nil {
					empty := ""
					draft[input.Name] = &empty
				}
			}
			if input.Type == "quantity" && draft[input.Name+"__unit"] == nil {
				empty := ""
				draft[input.Name+"__unit"] = &empty
			}
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, genericPortLabel(input.Name)).FontSize(float32(tok.TextData)).Bold().TextColor(t.TextMuted).Width(172)
				switch input.Type {
				case "string", "reference", "artifact":
					if input.Type == "artifact" {
						ui.Text(c, "结构化列表（在步骤工作台或源码中编辑）").FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
					} else {
						ui.Box(c).Grow(1).MinWidth(0).Children(func() {
							ui.TextInput(c, draft[input.Name]).Label(input.Name).FillWidth()
						})
					}
				case "enum":
					ui.Select(c, draft[input.Name], input.Values)
				case "bool", "boolean":
					if bools[input.Name] == nil {
						b := false
						bools[input.Name] = &b
					}
					ui.Checkbox(c, bools[input.Name], "")
				case "int", "integer", "number", "decimal":
					ui.Box(c).Grow(1).MinWidth(0).Children(func() {
						ui.DecimalInput(c, draft[input.Name], "", "", "1")
					})
				case "quantity":
					ui.DecimalInput(c, draft[input.Name], "", "", "1").Label(input.Name)
					ui.Box(c).Width(96).Children(func() {
						ui.TextInput(c, draft[input.Name+"__unit"]).Label(input.Name + " unit").FillWidth()
					})
				case "string[]":
					lists := v.moduleLists[moduleID]
					if lists == nil {
						lists = map[string][]string{}
						v.moduleLists[moduleID] = lists
					}
					items := lists[input.Name]
					ui.Column(c).Gap(4).Grow(1).Children(func() {
						remove := -1
						for i := range items {
							i := i
							ui.Row(c).FillWidth().Gap(4).AlignItems(ui.Center).Children(func() {
								ui.TextInput(c, &items[i]).Label(input.Name)
								if components.IconButton(c, lucide.Trash(), "删除 "+input.Name, components.IconButtonCompact).Clicked() {
									remove = i
								}
							})
						}
						if remove >= 0 {
							items = append(items[:remove], items[remove+1:]...)
						}
						if components.Button(c, "+ 添加", components.ButtonGhost).Clicked() {
							items = append(items, "")
						}
						lists[input.Name] = items
					})
				case "list", "object", "table", "object-list", "well-map", "geometry":
					ui.Text(c, "结构化列表（在步骤工作台或源码中编辑）").FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
				default:
					ui.TextInput(c, draft[input.Name])
				}
				if input.Description != "" {
					ui.Text(c, "· "+input.Description).FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
				}
			})
		}
		if canEdit {
			btn := components.Button(c, "保存", components.ButtonSecondary)
			if btn.Clicked() && v.OnUpdateModule != nil {
				props := map[string]any{}
				orig := v.moduleOriginals[moduleID]
				for key, valptr := range draft {
					if strings.HasSuffix(key, "__unit") {
						continue
					}
					if bools[key] != nil {
						continue
					}
					val := ""
					if valptr != nil {
						val = *valptr
					}
					origVal, origExists := orig[key]
					if !origExists && val == "" {
						continue
					}
					var originalStr string
					qm, isQ := origVal.(map[string]any)
					if isQ {
						originalStr = fmt.Sprint(qm["value"])
					} else {
						originalStr = fmt.Sprint(origVal)
					}
					unitChanged := false
					if isQ {
						u, _ := qm["unit"].(string)
						if draft[key+"__unit"] != nil && *draft[key+"__unit"] != u {
							unitChanged = true
						}
					}
					if val == originalStr && !unitChanged {
						continue
					}
					if isQ {
						num := parseNumber(val)
						unit := ""
						if draft[key+"__unit"] != nil {
							unit = *draft[key+"__unit"]
						}
						props[key] = map[string]any{"value": num, "unit": unit}
					} else if bptr := bools[key]; bptr != nil {
						props[key] = *bptr
					} else {
						if inputTypeIsInt(spec, key) {
							props[key] = parseInt(val)
						} else {
							props[key] = val
						}
					}
				}
				// bool changes: bools not in draft text; handle separately
				for key, ptr := range bools {
					if ptr == nil {
						continue
					}
					origVal, origExists := orig[key].(bool)
					if !origExists && !*ptr {
						continue
					}
					if *ptr != origVal {
						props[key] = *ptr
					}
				}
				if lists := v.moduleLists[moduleID]; lists != nil {
					for name, items := range lists {
						if sameStringList(items, orig[name]) {
							continue
						}
						out := make([]any, 0, len(items))
						for _, item := range items {
							if strings.TrimSpace(item) == "" {
								continue
							}
							out = append(out, item)
						}
						props[name] = out
					}
				}
				v.OnUpdateModule(moduleID, props)
			}
		}
	})
}

// sameStringList compares an editable string list with the stored value, which
// arrives as []any from YAML/JSON.
func sameStringList(items []string, original any) bool {
	stored, ok := original.([]any)
	if !ok {
		return len(items) == 0 && original == nil
	}
	trimmed := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item) == "" {
			continue
		}
		trimmed = append(trimmed, item)
	}
	if len(trimmed) != len(stored) {
		return false
	}
	for i := range trimmed {
		if fmt.Sprint(stored[i]) != trimmed[i] {
			return false
		}
	}
	return true
}

// GenericModuleSummary is the shared, read-only renderer for Run surfaces.
// Hosts can request compact, inspector, or full density without changing data.
func (v *View) GenericModuleSummary(c *ui.Context, data map[string]any) {
	kind, _ := data["kind"].(string)
	schema := 1
	switch value := data["schema"].(type) {
	case int:
		schema = value
	case int64:
		schema = int(value)
	case float64:
		schema = int(value)
	}
	registry := v.Registry
	if registry == nil {
		registry = BuiltinRegistry()
	}
	spec, ok := registry.Lookup(kind, schema)
	if !ok {
		v.moduleError(c, fmt.Errorf("no Run summary for %s@%d", kind, schema))
		return
	}
	density := v.Density
	if density == "" {
		density = DensityCompact
	}
	fields := make([]Port, 0, len(spec.Inputs))
	for _, input := range spec.Inputs {
		if input.Name == "kind" || input.Name == "id" || input.Name == "schema" {
			continue
		}
		if value, exists := data[input.Name]; exists && value != nil {
			fields = append(fields, input)
		}
	}
	t := c.Theme()
	tok := theme.GetTokens(t)
	ui.Column(c).FillWidth().Gap(t.Space(1)).Padding(t.Space(2)).Radius(t.Space(1)).
		Background(t.Surface).Border(1, t.Border).Children(func() {
		ui.Text(c, spec.Title).FontSize(float32(tok.TextData)).Bold().TextColor(t.Text)
		if len(fields) == 0 {
			ui.Text(c, "暂无可预览的参数").FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
			return
		}
		limit := len(fields)
		if density == DensityCompact && limit > 3 {
			limit = 3
		}
		for _, input := range fields[:limit] {
			value := summaryFieldValue(data[input.Name])
			if value == "" {
				continue
			}
			ui.Row(c).FillWidth().Gap(t.Space(1)).AlignItems(ui.Center).Children(func() {
				ui.Text(c, input.Name).FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted).Width(112)
				ui.Text(c, value).FontSize(float32(tok.TextData)).TextColor(t.Text)
			})
			if density == DensityFull && input.Description != "" {
				ui.Text(c, input.Description).FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
			}
		}
		if limit < len(fields) {
			ui.Text(c, fmt.Sprintf("另有 %d 项参数", len(fields)-limit)).
				FontSize(float32(tok.TextMeta)).TextColor(t.TextMuted)
		}
	})
}

func summaryFieldValue(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case map[string]any:
		if quantity, ok := typed["value"]; ok {
			unit, _ := typed["unit"].(string)
			return strings.TrimSpace(fmt.Sprint(quantity) + " " + unit)
		}
		return "结构化数据"
	case []any:
		return fmt.Sprintf("%d 项", len(typed))
	default:
		return fmt.Sprint(value)
	}
}

func (v *View) GenericEditor(c *ui.Context, moduleID string, data map[string]any) {
	v.GenericModuleEditor(c, moduleID, data)
}
func (v *View) RenderGenericEditor(c *ui.Context, moduleID string, data map[string]any) {
	v.GenericModuleEditor(c, moduleID, data)
}

func inputTypeIsInt(spec ModuleSpec, key string) bool {
	for _, in := range spec.Descriptor.Inputs {
		if in.Name == key && (in.Type == "int" || in.Type == "integer") {
			return true
		}
	}
	return false
}

func parseNumber(s string) any {
	// preserve integer-like? use float64
	var f float64
	fmt.Sscan(s, &f)
	return f
}
func parseInt(s string) int {
	var n int
	fmt.Sscan(s, &n)
	return n
}

func (v *View) ensureWidgetDraft(key, initial string) *string {
	if v.widgetTextDrafts == nil {
		v.widgetTextDrafts = map[string]*string{}
	}
	if ptr, ok := v.widgetTextDrafts[key]; ok {
		return ptr
	}
	val := initial
	v.widgetTextDrafts[key] = &val
	return v.widgetTextDrafts[key]
}

func (v *View) ensureWidgetBool(key string) *bool {
	return v.ensureWidgetBoolInit(key, false)
}

func (v *View) ensureWidgetBoolInit(key string, initial bool) *bool {
	if v.widgetBoolDrafts == nil {
		v.widgetBoolDrafts = map[string]*bool{}
	}
	if ptr, ok := v.widgetBoolDrafts[key]; ok {
		return ptr
	}
	val := initial
	v.widgetBoolDrafts[key] = &val
	return v.widgetBoolDrafts[key]
}

func (v *View) emitAnyModuleEdit(moduleID string, props map[string]any) {
	if v.StepID != "" && v.OnUpdateStepBlock != nil {
		v.OnUpdateStepBlock(v.StepID, moduleID, props)
		return
	}
	if v.OnUpdateModule != nil {
		v.OnUpdateModule(moduleID, props)
		return
	}
	if v.Doc == nil || !v.Can(PermDocumentEdit) {
		return
	}
	source, err := v.Doc.UpdateModule(moduleID, props)
	if err != nil {
		return
	}
	if v.OnChange != nil {
		v.OnChange(source)
	}
}

func iconButton(c *ui.Context, icon *ui.SVG, label string, variant components.ButtonVariant) ui.Element {
	tok := theme.GetTokens(c.Theme())
	btn := components.Button(c, "", variant).Label(label)
	fg := tok.Text
	if variant == components.ButtonPrimary {
		fg = c.Theme().AccentText
	}
	btn.Children(func() {
		ui.Row(c).Gap(float32(tok.Spacing4)).AlignItems(ui.Center).Children(func() {
			if icon != nil {
				ui.Icon(c, icon).Size(float32(tok.Spacing12), float32(tok.Spacing12)).TextColor(fg)
			}
			components.Text(c, label).SingleLine().TextColor(fg)
		})
	})
	return btn
}

var genericPortLabelMap = map[string]string{
	"title":               "标题",
	"name":                "名称",
	"objective":           "实验目的",
	"role":                "组别角色",
	"condition":           "处理条件",
	"independent_n":       "生物学重复 n",
	"technical_repeats":   "技术重复",
	"schema_id":           "结构标识",
	"endpoint":            "观测终点",
	"value_kind":          "数据类型",
	"unit":                "单位",
	"plate_format":        "容器规格",
	"timepoints":          "采样时间点",
	"format":              "孔数规格",
	"topology":            "容器拓扑",
	"stock_concentration": "母液浓度",
	"storage":             "储存条件",
	"lot":                 "批号",
	"catalog":             "货号",
	"cas":                 "CAS 号",
	"mode":                "模式",
	"groups":              "孔位分组",
	"samples":             "样本映射",
	"replicates":          "复孔编号",
	"roles":               "孔位角色",
	"factors":             "多维因子",
	"rows":                "明细行",
	"columns":             "数据列",
	"layout":              "视图布局",
	"container_ref":       "关联容器",
	"plate_ref":           "关联孔板",
	"body":                "正文内容",
	"src":                 "图片路径",
	"svg":                 "矢量源码",
	"caption":             "图注",
	"duration":            "持续时长",
	"offset":              "时间窗偏移",
	"rounds":              "循环轮数",
	"phases":              "循环阶段",
	"items":               "条目列表",
	"media_hint":          "采集说明",
	"scope":               "附件范围",
	"slots_per_cell":      "每孔采图槽数",
	"target_mass":         "目标上样量",
	"volume_per_unit":     "单份体积",
	"excess_ratio":        "余量比例",
	"dilution_factor":     "稀释倍数",
	"steps_count":         "梯度级数",
	"stock":               "母液浓度",
	"target":              "目标浓度",
	"final_volume":        "终体积",
	"formula":             "计算公式",
	"timepoint":           "时间点",
	"missing_reasons":     "缺失原因枚举",
	"null_allowed":        "允许空值",
	"source_kinds":        "数据来源类型",
	"table_columns":       "表格列定义",
	"method":              "检测方法",
	"group_refs":          "关联分组",
	"protocol_version_id": "方案版本标识",
	"controls":            "对照设置",
	"timepoint_relation":  "时间点关联",
	"technical_replicates":"技术重复数",
	"control_group_ref":   "对照分组引用",
	"sample_id":           "样本编号",
	"group_ref":           "所属分组",
	"material_refs":       "关联试剂/材料",
	"amount":              "用量/体积",
	"class":               "仪器类别",
	"model":               "型号",
	"capabilities":        "能力清单",
	"constraints":         "使用约束",
	"source":              "来源渠道",
	"pantry_code":         "储藏室短码",
	"steps":               "步骤序列",
	"edges":               "依赖连线",
	"business_order":      "业务顺序",
	"execution":           "执行属性",
	"assay":               "关联检测",
	"path":                "步骤路径",
	"order":               "排序序号",
	"block_refs":          "共享模块引用",
	"blocks":              "步骤内模块",
	"checks":              "检查项",
	"tasks":               "任务列表",
	"uses":                "关联资源",
	"experimental_unit":   "实验单元",
	"primary_endpoint":    "主要终点",
	"endpoint_method":     "测定方法",
	"endpoint_unit":       "终点单位",
}

func genericPortLabel(name string) string {
	if lbl, ok := genericPortLabelMap[name]; ok {
		return lbl
	}
	return name
}

type visualFlowSeg struct {
	seg          DocSegment
	isStepHeader bool
	stepNode     *StepNode
	stableKey    string
}

// expandSegmentsForStepContainers splits SegmentProse slices at heading-step headers and
// step-exit boundaries (`## 讨论` / `---`) purely for visual container grouping so every
// STEP and Sub-step renders inside ONE continuous indented layout container with unbroken
// guide rails across mixed prose + embedded components.
func expandSegmentsForStepContainers(source string, segs []DocSegment, stepTree *StepTree) []visualFlowSeg {
	hasHeadingSteps := false
	if stepTree != nil {
		for _, sn := range stepTree.Flat {
			if sn.SourceKind == "heading" {
				hasHeadingSteps = true
				break
			}
		}
	}
	if !hasHeadingSteps {
		out := make([]visualFlowSeg, len(segs))
		for i, s := range segs {
			out[i] = visualFlowSeg{seg: s}
		}
		return out
	}

	var out []visualFlowSeg
	chunkCounts := map[string]int{}
	nextProseKey := func(startOff int) string {
		ownerKey := "root"
		if ow := StepOwnerAtOffset(stepTree, startOff); ow != nil {
			ownerKey = ow.ID
		}
		chunkCounts[ownerKey]++
		return fmt.Sprintf("prose-flow:%s:%d", ownerKey, chunkCounts[ownerKey])
	}

	for _, seg := range segs {
		if seg.Kind != SegmentProse {
			out = append(out, visualFlowSeg{seg: seg})
			continue
		}
		lines := strings.SplitAfter(seg.Text, "\n")
		curStart := seg.Start
		off := seg.Start
		inFence := false
		fenceTicks := 0

		flushProse := func(end int) {
			if end <= curStart {
				return
			}
			txt := source[curStart:end]
			if strings.TrimSpace(txt) == "" && curStart != seg.Start {
				return
			}
			out = append(out, visualFlowSeg{
				seg: DocSegment{
					Kind:  SegmentProse,
					Index: seg.Index,
					Start: curStart,
					End:   end,
					Text:  txt,
				},
				stableKey: nextProseKey(curStart),
			})
		}

		for _, rawLine := range lines {
			lineEnd := off + len(rawLine)
			lineText := strings.TrimRight(rawLine, "\r\n")
			trimmed := strings.TrimSpace(lineText)

			if !inFence {
				if ticks := countLeadingBackticks(trimmed); ticks >= 3 {
					inFence = true
					fenceTicks = ticks
					off = lineEnd
					continue
				}
				if sn := stepTree.ByLineStart[off]; sn != nil && sn.SourceKind == "heading" {
					flushProse(off)
					out = append(out, visualFlowSeg{
						seg: DocSegment{
							Kind:  SegmentProse,
							Index: seg.Index,
							Start: off,
							End:   lineEnd,
							Text:  rawLine,
						},
						isStepHeader: true,
						stepNode:     sn,
						stableKey:    "step-hdr:" + sn.ID,
					})
					curStart = lineEnd
					off = lineEnd
					continue
				}
				if (rePlainHeadingLine.MatchString(lineText) || reThematicBreakLine.MatchString(lineText)) && off > curStart {
					// Exit any open step scope cleanly BEFORE the plain heading or `---` divider
					if StepOwnerAtOffset(stepTree, curStart) != nil {
						flushProse(off)
						curStart = off
					}
				}
			} else {
				if isClosingBacktickFence(trimmed, fenceTicks) {
					inFence = false
					fenceTicks = 0
				}
			}
			off = lineEnd
		}
		flushProse(seg.End)
	}
	return out
}

