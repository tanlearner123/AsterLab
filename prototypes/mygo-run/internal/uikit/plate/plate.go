package plate

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"

	"asterlab-mygo-prototype/internal/theme"
)

type WellState struct {
	Group     string
	SampleID  string
	Replicate int
	Timepoint string
	Role      string
	Factors   map[string]string
	Ambiguous bool
}

// InteractionState persists pointer drag-marquee and hover state across frames
// for interactive inline, overlay, and tool plates.
type InteractionState struct {
	pointerDown   bool
	pointerStart  ui.Point
	dragStart     ui.Point
	dragLast      ui.Point
	dragging      bool
	additive      bool
	suppressClick bool
	baseSelected  []string
	hoverWell     string
}

type Props struct {
	Wells       map[string]WellState
	SelectedIDs []string
	OnSelect    func(wellIDs []string)
	Interactive bool
	Rows        int
	Cols        int
	Topology    string
	Roles       map[string]string
	ColorBy     string // "group" | "sample" | "factor" | "replicate" | "role"
	FactorKey   string
	Blind       bool
	GroupIndex  map[string]int
	GroupLabels map[string]string
	ExcludedIDs map[string]bool
	HideFooter  bool
	Zoom        float32
	State       *InteractionState
}

const (
	defaultRows = 8
	defaultCols = 12
)

// supportedPlateSizes are the geometries AsterLab renders: 4, 6, 12, 24, 48, 96
// and 384 wells as rows×cols. Unsupported dimensions fall back to 96 wells.
var supportedPlateSizes = [...]struct{ Wells, Rows, Cols int }{
	{4, 2, 2},
	{6, 2, 3},
	{12, 3, 4},
	{24, 4, 6},
	{48, 6, 8},
	{96, 8, 12},
	{384, 16, 24},
}

func wellGrid(rows, cols int) bool {
	for _, size := range supportedPlateSizes {
		if size.Rows == rows && size.Cols == cols {
			return true
		}
	}
	return false
}

func rowLabel(row int) string {
	if row < 26 {
		return string(rune('A' + row))
	}
	return fmt.Sprintf("A%c", rune('A'+row-26))
}

func wellID(row, col int) string {
	return fmt.Sprintf("%s%d", rowLabel(row), col+1)
}

func wellPosition(id string, rows, cols int) (int, int, bool) {
	id = strings.ToUpper(strings.TrimSpace(id))
	row, i := 0, 0
	for i < len(id) && id[i] >= 'A' && id[i] <= 'Z' {
		row = row*26 + int(id[i]-'A'+1)
		i++
	}
	if i == 0 || i == len(id) {
		return 0, 0, false
	}
	row--
	col, err := strconv.Atoi(id[i:])
	if err != nil || col < 1 {
		return 0, 0, false
	}
	col--
	return row, col, row >= 0 && row < rows && col >= 0 && col < cols
}

func GroupColor(group string, dark bool) ui.Color {
	return GroupColorIndexed(group, nil, dark)
}

// GroupColorIndexed returns a deterministic semantic color for a group,
// preferring sequential CategoricalPalette indexing when groupIndex is supplied
// so adjacent groups never collide.
func GroupColorIndexed(group string, groupIndex map[string]int, dark bool) ui.Color {
	g := strings.ToLower(strings.TrimSpace(group))
	t := theme.GetTokens(theme.Palette(dark))
	switch {
	case g == "":
		return ui.Color{}
	case g == "c1":
		return t.Accent
	case g == "c2":
		return t.Success
	case g == "c3":
		return t.Warning
	case g == "c4":
		return t.Danger
	case g == "c5":
		return t.AccentHover
	}
	if groupIndex != nil {
		if idx, ok := groupIndex[strings.TrimSpace(group)]; ok && idx >= 0 {
			pal := theme.CategoricalPalette(dark)
			return pal[idx%len(pal)]
		}
	}
	switch {
	case strings.Contains(g, "nc"), strings.Contains(g, "空白"), strings.Contains(g, "blank"), strings.Contains(g, "对照"), strings.Contains(g, "control"):
		return t.BorderStrong
	case strings.Contains(g, "pc"), strings.Contains(g, "阳性"), strings.Contains(g, "positive"):
		return t.WarningBg
	default:
		return theme.CategoricalColor("group:"+g, dark)
	}
}

// AdaptiveMetrics returns responsive well/lane pixel dimensions and gap based
// on total well count, container topology, and zoom factor.
func AdaptiveMetrics(rows, cols int, topology string, zoom float32) (cellW, cellH, gap float32) {
	if zoom <= 0 {
		zoom = 1
	}
	gapScale := float32(1)
	if zoom < 1 {
		gapScale = zoom
	}
	scaleGap := func(base float32) float32 {
		g := base * gapScale
		if g < 2 {
			g = 2
		}
		return g
	}
	if topology == "rack_1d" {
		cellW, cellH, gap = 40*zoom, 58*zoom, scaleGap(6)
		if cols > 12 {
			cellW = (560*zoom - float32(cols-1)*gap) / float32(cols)
			if cellW < 18*zoom {
				cellW = 18 * zoom
			}
		}
		return cellW, cellH, gap
	}
	total := rows * cols
	switch {
	case total <= 4:
		cellW, cellH, gap = 56*zoom, 56*zoom, scaleGap(10)
	case total <= 6:
		cellW, cellH, gap = 48*zoom, 48*zoom, scaleGap(8)
	case total <= 12:
		cellW, cellH, gap = 42*zoom, 42*zoom, scaleGap(8)
	case total <= 24:
		cellW, cellH, gap = 36*zoom, 36*zoom, scaleGap(6)
	case total <= 48:
		cellW, cellH, gap = 28*zoom, 28*zoom, scaleGap(5)
	case total <= 96:
		cellW, cellH, gap = 26*zoom, 26*zoom, scaleGap(4)
	default:
		cellW, cellH, gap = 17*zoom, 17*zoom, scaleGap(3)
	}
	return cellW, cellH, gap
}

// ResolveDimensions normalizes plate/dish/rack row & column counts.
func ResolveDimensions(props Props) (rows, cols int) {
	rows, cols = props.Rows, props.Cols
	if rows == 0 {
		rows = defaultRows
	}
	if cols == 0 {
		cols = defaultCols
	}
	isCustomTopology := props.Topology != "" && props.Topology != "plate"
	if !isCustomTopology && !wellGrid(rows, cols) {
		rows, cols = defaultRows, defaultCols
	}
	if rows < 1 || rows > 16 || cols < 1 || cols > 24 {
		rows, cols = defaultRows, defaultCols
	}
	return rows, cols
}

// LegendItem describes one category swatch rendered below a plate grid or export.
type LegendItem struct {
	Label string
	Color ui.Color
	Count int
}

// ComputeLegend returns the sorted legend entries shared across interactive views,
// read-only summaries, and SVG/PNG exports. Numeric factor layers sort high-to-low
// with monotonic sequential ramp swatches instead of unordered string hashes.
func ComputeLegend(props Props, dark bool) []LegendItem {
	t := theme.GetTokens(theme.Palette(dark))
	counts := make(map[string]int)
	colors := make(map[string]ui.Color)
	order := make([]string, 0)
	scale := buildFactorColorScale(props, dark, t)
	for id, well := range props.Wells {
		if well.Ambiguous {
			continue
		}
		role := effectiveRole(well, id, props.Roles)
		lbl, col := wellLegendCategoryWithScale(props, well, role, dark, t, scale)
		if lbl == "" {
			continue
		}
		if _, seen := counts[lbl]; !seen {
			order = append(order, lbl)
			colors[lbl] = col
		}
		counts[lbl]++
	}
	if scale != nil && scale.isNumeric {
		sort.Slice(order, func(i, j int) bool {
			vi, oki := parseNumericFactorValue(order[i])
			vj, okj := parseNumericFactorValue(order[j])
			if oki && okj {
				if vi != vj {
					return vi > vj
				}
				return order[i] < order[j]
			}
			if oki != okj {
				return oki
			}
			return order[i] < order[j]
		})
	} else {
		sort.Strings(order)
	}
	out := make([]LegendItem, 0, len(order))
	for _, lbl := range order {
		out = append(out, LegendItem{Label: lbl, Color: colors[lbl], Count: counts[lbl]})
	}
	return out
}

type factorColorScale struct {
	isNumeric bool
	colors    map[string]ui.Color
}

func parseNumericFactorValue(raw string) (float64, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return 0, false
	}
	end := 0
	for end < len(s) {
		ch := s[end]
		if (ch >= '0' && ch <= '9') || ch == '.' || (end == 0 && (ch == '+' || ch == '-')) {
			end++
			continue
		}
		break
	}
	if end == 0 {
		return 0, false
	}
	v, err := strconv.ParseFloat(s[:end], 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	rest := strings.TrimSpace(s[end:])
	if rest != "" {
		for _, r := range rest {
			if unicode.IsDigit(r) && !strings.ContainsAny(rest, "uUmMnNpPkKgGLl%") {
				return 0, false
			}
		}
	}
	return v, true
}

func buildFactorColorScale(props Props, dark bool, t theme.Tokens) *factorColorScale {
	if !strings.EqualFold(strings.TrimSpace(props.ColorBy), "factor") || props.FactorKey == "" {
		return nil
	}
	seen := map[string]bool{}
	var vals []string
	var numVals []float64
	for _, well := range props.Wells {
		if well.Factors == nil {
			continue
		}
		v := strings.TrimSpace(well.Factors[props.FactorKey])
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		vals = append(vals, v)
		if num, ok := parseNumericFactorValue(v); ok && num > 0 {
			numVals = append(numVals, num)
		}
	}
	if len(vals) == 0 {
		return nil
	}
	colors := make(map[string]ui.Color, len(vals))
	isNumeric := len(numVals) >= 2
	if isNumeric {
		minV, maxV := numVals[0], numVals[0]
		for _, nv := range numVals[1:] {
			if nv < minV {
				minV = nv
			}
			if nv > maxV {
				maxV = nv
			}
		}
		useLog := minV > 0 && maxV/minV >= 8
		for _, v := range vals {
			nv, ok := parseNumericFactorValue(v)
			if !ok || nv <= 0 {
				colors[v] = t.BorderStrong
				continue
			}
			norm := 0.5
			if maxV > minV {
				if useLog {
					norm = (math.Log10(nv) - math.Log10(minV)) / (math.Log10(maxV) - math.Log10(minV))
				} else {
					norm = (nv - minV) / (maxV - minV)
				}
			}
			colors[v] = theme.GradientRampColor(norm, dark)
		}
		return &factorColorScale{isNumeric: true, colors: colors}
	}
	sort.Strings(vals)
	pal := theme.CategoricalPalette(dark)
	for i, v := range vals {
		if len(pal) > 0 {
			colors[v] = pal[i%len(pal)]
		} else {
			colors[v] = theme.CategoricalColor(props.FactorKey+":"+v, dark)
		}
	}
	return &factorColorScale{isNumeric: false, colors: colors}
}

// WellsInRectCells returns every well/lane ID overlapping rect in row-major order.
func WellsInRectCells(rows, cols int, cellW, cellH, gap float32, rect ui.Rect) []string {
	var selected []string
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			cellRect := ui.Rect{
				X: float32(col) * (cellW + gap),
				Y: float32(row) * (cellH + gap),
				W: cellW,
				H: cellH,
			}
			if cellRect.X <= rect.X+rect.W && cellRect.X+cellRect.W >= rect.X &&
				cellRect.Y <= rect.Y+rect.H && cellRect.Y+cellRect.H >= rect.Y {
				selected = append(selected, wellID(row, col))
			}
		}
	}
	return selected
}

func selectedSet(props Props) map[string]bool {
	out := make(map[string]bool, len(props.SelectedIDs))
	for _, id := range props.SelectedIDs {
		out[id] = true
	}
	return out
}

func nextSelection(selected map[string]bool, id string, additive bool) map[string]bool {
	next := make(map[string]bool, len(selected)+1)
	if additive {
		for selectedID := range selected {
			next[selectedID] = true
		}
	} else if len(selected) == 1 && selected[id] {
		return next
	}
	if additive && next[id] {
		delete(next, id)
	} else {
		next[id] = true
	}
	return next
}

// View renders the plate. Drag-rectangle selection is not implemented:
// MyGo exposes no pointer-coordinate API for a marquee without a separate model.
// Click selects one well; clicking it again clears it. Shift+click toggles a well in the selection.

func View(c *ui.Context, props Props) {
	th := c.Theme()
	t := theme.GetTokens(th)
	dark := th != nil && th.Dark
	var lastHoverWell string

	rows, cols := ResolveDimensions(props)
	cellW, cellH, gap := AdaptiveMetrics(rows, cols, props.Topology, props.Zoom)
	mini := !props.Interactive && props.Zoom > 0 && props.Zoom < 0.75
	labelSize := float32(t.TextMeta)
	leftLabel := float32(t.Spacing24)
	topLabel := float32(t.Spacing16)
	dishPad := float32(0)
	footerPad := float32(t.Spacing32)
	if props.Topology == "dish" {
		dishPad = float32(t.Spacing24)
	}
	if mini {
		if labelSize = labelSize * (0.7 + 0.3*props.Zoom); labelSize < 9 {
			labelSize = 9
		}
		if leftLabel = float32(t.Spacing24) * (0.65 + 0.35*props.Zoom); leftLabel < 15 {
			leftLabel = 15
		}
		if topLabel = float32(t.Spacing16) * (0.65 + 0.35*props.Zoom); topLabel < 11 {
			topLabel = 11
		}
		if props.Topology == "dish" {
			dishPad = float32(t.Spacing24) * (0.55 + 0.45*props.Zoom)
			if dishPad < 20 {
				dishPad = 20
			}
		}
		footerPad = float32(t.Spacing8)
	}
	gridW := float32(cols)*cellW + float32(cols-1)*gap
	gridH := float32(rows)*cellH + float32(rows-1)*gap
	width := leftLabel + gridW + dishPad*2 + float32(t.Spacing4)
	height := topLabel + gridH + dishPad*2 + footerPad
	selected := selectedSet(props)
	legend := ComputeLegend(props, dark)
	legendCounts := make(map[string]int, len(legend))
	legendColors := make(map[string]ui.Color, len(legend))
	legendOrder := make([]string, 0, len(legend))
	for _, item := range legend {
		legendOrder = append(legendOrder, item.Label)
		legendColors[item.Label] = item.Color
		legendCounts[item.Label] = item.Count
	}

	wellRadius := func() float32 {
		if props.Topology == "rack_1d" {
			return float32(t.ControlRadius) * 2
		}
		if props.Topology == "cohort" {
			return float32(t.ControlRadius)
		}
		return cellW / 2
	}

	drawDishRim := func(p *ui.Painter, gx0, gy0 float32) {
		if props.Topology != "dish" {
			return
		}
		cx := gx0 + gridW/2
		cy := gy0 + gridH/2
		rOuter := gridW
		if gridH > rOuter {
			rOuter = gridH
		}
		rimScale := float32(0.72)
		rimPad := float32(t.Spacing8)
		if mini {
			rimScale = 0.65
			rimPad = float32(t.Spacing4)
		}
		rOuter = rOuter*rimScale + rimPad
		outerRect := ui.Rect{X: cx - rOuter, Y: cy - rOuter, W: rOuter * 2, H: rOuter * 2}
		p.Fill(outerRect, t.Elevated.Alpha(.65), rOuter)
		p.Stroke(outerRect, t.BorderStrong, rOuter, 1.5)
		innerR := rOuter - rimPad
		if innerR > 8 {
			innerRect := ui.Rect{X: cx - innerR, Y: cy - innerR, W: innerR * 2, H: innerR * 2}
			p.Stroke(innerRect, t.Border, innerR, 1)
		}
		if rows == 2 && cols == 2 {
			p.Line(cx-rOuter+float32(t.Spacing8), cy, cx+rOuter-float32(t.Spacing8), cy, 1, t.BorderStrong.Alpha(.55))
			p.Line(cx, cy-rOuter+float32(t.Spacing8), cx, cy+rOuter-float32(t.Spacing8), 1, t.BorderStrong.Alpha(.55))
		}
	}

	ui.Column(c).Gap(float32(t.Spacing8)).AlignItems(ui.Center).Children(func() {
		if props.Interactive {
			st := props.State
			if st == nil {
				st = &InteractionState{}
			}
			suppressGridClick := st.suppressClick
			st.suppressClick = false
			st.hoverWell = ""

			notify := func(ids []string) {
				sort.Strings(ids)
				if props.OnSelect != nil {
					props.OnSelect(ids)
				}
			}
			mergeSelect := func(ids []string, additive bool) {
				seen := map[string]bool{}
				out := make([]string, 0, len(props.SelectedIDs)+len(ids))
				if additive {
					for _, id := range props.SelectedIDs {
						if !seen[id] {
							seen[id] = true
							out = append(out, id)
						}
					}
				}
				for _, id := range ids {
					if !seen[id] {
						seen[id] = true
						out = append(out, id)
					}
				}
				notify(out)
			}
			applyMarquee := func() {
				x0, y0 := st.dragStart.X, st.dragStart.Y
				x1, y1 := st.dragLast.X, st.dragLast.Y
				if x0 > x1 {
					x0, x1 = x1, x0
				}
				if y0 > y1 {
					y0, y1 = y1, y0
				}
				hit := WellsInRectCells(rows, cols, cellW, cellH, gap, ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0})
				seen := map[string]bool{}
				out := make([]string, 0, len(st.baseSelected)+len(hit))
				if st.additive {
					for _, id := range st.baseSelected {
						if !seen[id] {
							seen[id] = true
							out = append(out, id)
						}
					}
				}
				for _, id := range hit {
					if !seen[id] {
						seen[id] = true
						out = append(out, id)
					}
				}
				notify(out)
			}

			gridBox := ui.Column(c).AlignItems(ui.Center).Padding(dishPad).Children(func() {
				ui.Column(c).Gap(gap).AlignItems(ui.Center).Children(func() {
					ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(leftLabel, topLabel)
						for col := 0; col < cols; col++ {
							cIdx := col
							hdrBtn := ui.ButtonBase(c).Width(cellW).Height(topLabel).
								Label(fmt.Sprintf("选择第 %d 列", cIdx+1))
							hdrBtn.Children(func() {
								ui.Box(c).Fill().Center().Children(func() {
									ui.Text(c, fmt.Sprintf("%d", cIdx+1)).FontSize(labelSize).TextColor(t.TextMuted).SingleLine()
								})
							})
							if hdrBtn.Clicked() && !st.dragging && !suppressGridClick {
								ids := make([]string, 0, rows)
								for r := 0; r < rows; r++ {
									ids = append(ids, wellID(r, cIdx))
								}
								mergeSelect(ids, hdrBtn.ClickModifiers()&(ui.Shift|ui.Cmd) != 0)
							}
						}
					})
					for row := 0; row < rows; row++ {
						rIdx := row
						ui.Row(c).Gap(gap).AlignItems(ui.Center).Children(func() {
							rBtn := ui.ButtonBase(c).Width(leftLabel).Height(cellH).
								Label("选择第 " + rowLabel(rIdx) + " 行")
							rBtn.Children(func() {
								ui.Box(c).Fill().Center().Children(func() {
									ui.Text(c, rowLabel(rIdx)).FontSize(labelSize).TextColor(t.TextMuted).SingleLine()
								})
							})
							if rBtn.Clicked() && !st.dragging && !suppressGridClick {
								ids := make([]string, 0, cols)
								for cc := 0; cc < cols; cc++ {
									ids = append(ids, wellID(rIdx, cc))
								}
								mergeSelect(ids, rBtn.ClickModifiers()&(ui.Shift|ui.Cmd) != 0)
							}
							for col := 0; col < cols; col++ {
								cIdx := col
								id := wellID(rIdx, cIdx)
								well, assigned := props.Wells[id]
								role := effectiveRole(well, id, props.Roles)
								fill := WellFillColor(props, well, assigned, role, dark, t)
								strokeCol := t.Border
								var strokeW float32 = 1
								if assigned && (len(well.Factors) > 0 || well.SampleID != "") && !props.Blind {
									strokeCol = fill.Mix(t.Text, 0.28)
								}
								if selected[id] {
									strokeCol, strokeW = t.Accent, 2
								}
								ink := contrastInkForFill(fill, assigned, t)
								badge := ""
								if !props.ExcludedIDs[id] {
									if well.Ambiguous {
										badge = "!"
									} else if !props.Blind && assigned {
										badge = wellGlyphAdaptive(props, well, role, cellW)
									} else if rb := roleShort(role); rb != "" {
										badge = rb
									}
								}
								btn := ui.ButtonBase(c).Key(id).Size(cellW, cellH).Radius(wellRadius()).
									Label("孔 " + id)
								animFill := btn.AnimateColor("wfill", fill, theme.MotionSnappy)
								animStroke := btn.AnimateColor("wstroke", strokeCol, theme.MotionSnappy)
								btn.Background(animFill).Border(strokeW, animStroke)
								targetScale := float32(1.0)
								if btn.Pressed() {
									targetScale = 0.93
								} else if selected[id] && st.hoverWell == id {
									targetScale = 1.04
								}
								if sc := btn.AnimateTo("wscale", targetScale, theme.MotionSnappy); sc > 0 && sc != 1.0 {
									btn.Scale(sc, sc)
								}
								if badge != "" || (selected[id] && cellW >= 32) {
									fontPx := float32(t.TextMeta) * 0.72
									if cellW >= 40 {
										fontPx = float32(t.TextMeta) * 0.86
									} else if len([]rune(badge)) == 1 {
										fontPx = float32(t.TextMeta) * 0.84
									}
									btn.Children(func() {
										ui.Column(c).Fill().Center().Gap(1).Children(func() {
											if badge != "" {
												ui.Text(c, badge).FontSize(fontPx).Bold().TextColor(ink).SingleLine()
											}
											if !props.Blind && assigned && well.Replicate > 0 && cellW >= 38 && props.ColorBy != "replicate" {
												ui.Text(c, fmt.Sprintf("#%d", well.Replicate)).FontSize(float32(t.TextMeta)*0.64).TextColor(ink.Alpha(.82)).SingleLine()
											}
										})
									})
								}
								if props.ExcludedIDs[id] {
									btn.DrawOver(func(p *ui.Painter, r ui.Rect) {
										pad := r.W * 0.28
										cross := &ui.Path{}
										cross.MoveTo(r.X+pad, r.Y+pad)
										cross.LineTo(r.X+r.W-pad, r.Y+r.H-pad)
										cross.MoveTo(r.X+r.W-pad, r.Y+pad)
										cross.LineTo(r.X+pad, r.Y+r.H-pad)
										p.StrokePath(cross, 2.2, t.Danger)
									})
								}
								if btn.Hovered() {
									st.hoverWell = id
								}
								cellPt := func(x, y float32) ui.Point {
									return ui.Point{
										X: float32(cIdx)*(cellW+gap) + x,
										Y: float32(rIdx)*(cellH+gap) + y,
									}
								}
								btn = btn.HandleInput(func(event ui.InputEvent) bool {
									if (event.Kind == ui.InputPointerDown || event.Kind == ui.InputPointerUp) && event.Button != 0 {
										return false
									}
									switch event.Kind {
									case ui.InputPointerDown:
										pt := cellPt(event.X, event.Y)
										st.pointerDown = true
										st.pointerStart = ui.Point{X: event.X, Y: event.Y}
										st.dragStart, st.dragLast = pt, pt
										st.dragging = false
										st.additive = event.Mods&(ui.Shift|ui.Cmd) != 0
										st.baseSelected = append(st.baseSelected[:0], props.SelectedIDs...)
										return false
									case ui.InputPointerMove:
										if !st.pointerDown {
											return false
										}
										dx, dy := event.X-st.pointerStart.X, event.Y-st.pointerStart.Y
										if !st.dragging {
											if dx > -3 && dx < 3 && dy > -3 && dy < 3 {
												return false
											}
											st.dragging = true
										}
										st.dragLast = cellPt(event.X, event.Y)
										applyMarquee()
										return true
									case ui.InputPointerUp:
										if !st.pointerDown {
											return false
										}
										st.pointerDown = false
										if !st.dragging {
											st.baseSelected = nil
											return false
										}
										if event.Canceled {
											notify(append([]string(nil), st.baseSelected...))
										} else {
											st.dragLast = cellPt(event.X, event.Y)
											applyMarquee()
										}
										st.dragging = false
										st.suppressClick = true
										st.baseSelected = nil
										return true
									}
									return false
								})
								if btn.Clicked() && !st.dragging && !suppressGridClick {
									next := nextSelection(selected, id, btn.ClickModifiers()&(ui.Shift|ui.Cmd) != 0)
									out := make([]string, 0, len(next))
									for k := range next {
										out = append(out, k)
									}
									notify(out)
								}
							}
						})
					}
				})
			})
			if props.Topology == "dish" {
				gridBox.Draw(func(p *ui.Painter, r ui.Rect) {
					drawDishRim(p, r.X+dishPad+leftLabel+gap, r.Y+dishPad+topLabel+gap)
				})
			}
			gridBox.DrawOver(func(p *ui.Painter, r ui.Rect) {
				if !st.dragging {
					return
				}
				ox := r.X + dishPad + leftLabel + gap
				oy := r.Y + dishPad + topLabel + gap
				x0, y0 := ox+st.dragStart.X, oy+st.dragStart.Y
				x1, y1 := ox+st.dragLast.X, oy+st.dragLast.Y
				if x0 > x1 {
					x0, x1 = x1, x0
				}
				if y0 > y1 {
					y0, y1 = y1, y0
				}
				mr := ui.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
				p.Fill(mr, t.Accent.Alpha(.14), 2)
				p.Stroke(mr, t.Accent, 2, 1)
			})

			hoverText, hoverColor := "悬停或拖拽孔位查看分组、样本与复孔", t.TextMuted
			if st.hoverWell != "" {
				hoverText, hoverColor = wellHoverSummary(props, st.hoverWell), t.Text
			} else if len(props.Wells) == 0 {
				hoverText = "尚无计划孔位分配"
			}
			ui.Text(c, hoverText).FontSize(labelSize).TextColor(hoverColor).SingleLine()
			if len(legendOrder) > 0 {
				ui.Row(c).Wrap().Gap(float32(t.Spacing8)).AlignItems(ui.Center).Children(func() {
					for _, label := range legendOrder {
						ui.Row(c).Key("leg-"+label).Gap(float32(t.Spacing4)).AlignItems(ui.Center).
							Transition(theme.AccordionTransition()).Children(func() {
							sw := ui.Box(c).Size(float32(t.Spacing8), float32(t.Spacing8)).Radius(float32(t.ControlRadius))
							sw.Background(sw.AnimateColor("lcol", legendColors[label], theme.MotionSnappy))
							ui.Text(c, fmt.Sprintf("%s %d", label, legendCounts[label])).FontSize(labelSize).
								TextColor(t.TextMuted).FontFeatures("tnum").ContentTransition(theme.NumericRoll(false))
						})
					}
				})
			}
			ui.Text(c, fmt.Sprintf("%d / %d 孔已分配 · 拖拽框选或点行列号快速选孔", len(props.Wells), rows*cols)).
				FontSize(labelSize).TextColor(t.TextMuted).ContentTransition(theme.NumericRoll(false))
			return
		}

		canvas := ui.Box(c).Size(width, height).Draw(func(p *ui.Painter, r ui.Rect) {
			gx0 := r.X + leftLabel + dishPad
			gy0 := r.Y + topLabel + dishPad
			drawDishRim(p, gx0, gy0)
			for col := 0; col < cols; col++ {
				cx := gx0 + float32(col)*(cellW+gap) + cellW/2 - float32(t.Spacing4)*0.8
				p.Text(cx, r.Y+dishPad+float32(t.Spacing4)*0.4, fmt.Sprintf("%d", col+1), labelSize, t.TextMuted)
			}
			for row := 0; row < rows; row++ {
				ry := gy0 + float32(row)*(cellH+gap) + (cellH-labelSize)*0.42
				p.Text(r.X+dishPad+float32(t.Spacing4), ry, rowLabel(row), labelSize, t.TextMuted)
				for col := 0; col < cols; col++ {
					id := wellID(row, col)
					x := gx0 + float32(col)*(cellW+gap)
					y := gy0 + float32(row)*(cellH+gap)
					wr := ui.Rect{X: x, Y: y, W: cellW, H: cellH}
					well, assigned := props.Wells[id]
					role := effectiveRole(well, id, props.Roles)
					fill := WellFillColor(props, well, assigned, role, dark, t)
					radius := wellRadius()
					p.Fill(wr, fill, radius)
					strokeCol := t.Border
					if assigned && (len(well.Factors) > 0 || well.SampleID != "") && !props.Blind {
						strokeCol = fill.Mix(t.Text, 0.28)
					}
					p.Stroke(wr, strokeCol, radius, 1)
					ink := contrastInkForFill(fill, assigned, t)
					if !props.Blind && assigned && well.SampleID != "" && len(well.Factors) > 0 && well.Replicate > 0 && cellW >= 20 {
						inner := ui.Rect{X: x + 2.5, Y: y + 2.5, W: cellW - 5, H: cellH - 5}
						innerR := radius - 2.5
						if innerR < 2 {
							innerR = 2
						}
						p.Stroke(inner, ink.Alpha(.38), innerR, 1)
					}
					if props.ExcludedIDs[id] {
						pad := cellW * 0.28
						cross := &ui.Path{}
						cross.MoveTo(x+pad, y+pad)
						cross.LineTo(x+cellW-pad, y+cellH-pad)
						cross.MoveTo(x+cellW-pad, y+pad)
						cross.LineTo(x+pad, y+cellH-pad)
						p.StrokePath(cross, 2.2, t.Danger)
					} else if well.Ambiguous {
						fontPx := labelSize * 0.9
						p.Text(x+(cellW-fontPx*0.4)/2, y+(cellH-fontPx)*0.44, "!", fontPx, ink)
					} else if !props.Blind && assigned {
						badge := wellGlyphAdaptive(props, well, role, cellW)
						if badge != "" {
							fontPx := labelSize * 0.72
							if cellW >= 40 {
								fontPx = labelSize * 0.88
							} else if len([]rune(badge)) == 1 {
								fontPx = labelSize * 0.84
							}
							glyphW := estimatedBadgeWidth(badge, fontPx)
							if maxW := cellW - 6; glyphW > maxW && maxW > 4 {
								fontPx *= maxW / glyphW
								if fontPx < 7.5 {
									fontPx = 7.5
								}
								glyphW = estimatedBadgeWidth(badge, fontPx)
							}
							tx := x + (cellW-glyphW)/2
							ty := y + (cellH-fontPx)*0.42
							if tx < x+2 {
								tx = x + 2
							}
							p.Text(tx, ty, badge, fontPx, ink)
						}
					} else if roleBadge := roleShort(role); roleBadge != "" {
						fontPx := labelSize * 0.78
						p.Text(x+(cellW-fontPx*0.6)/2, y+(cellH-fontPx)*0.44, roleBadge, fontPx, t.Text)
					}
					if selected[id] {
						p.Stroke(wr, t.Accent, radius, 2)
					}
				}
			}
			if !mini {
				hudY := gy0 + gridH + dishPad + float32(t.Spacing8)
				if lastHoverWell != "" {
					p.Text(r.X+leftLabel, hudY+float32(t.Spacing12), wellHoverSummary(props, lastHoverWell), labelSize, t.Text)
				} else if len(props.Wells) == 0 {
					p.Text(r.X+leftLabel, hudY+float32(t.Spacing12), "尚无计划孔位分配", labelSize, t.TextMuted)
				}
			}
		})
		if !mini {
			canvas.HitTest(func(x, y float32, r ui.Rect) bool {
				lastHoverWell = ""
				gx := x - (r.X + leftLabel + dishPad)
				gy := y - (r.Y + topLabel + dishPad)
				if gx < 0 || gy < 0 {
					return false
				}
				col := int(gx / (cellW + gap))
				row := int(gy / (cellH + gap))
				if col < 0 || col >= cols || row < 0 || row >= rows {
					return false
				}
				cx := gx - float32(col)*(cellW+gap)
				cy := gy - float32(row)*(cellH+gap)
				if cx > cellW || cy > cellH {
					return false
				}
				lastHoverWell = wellID(row, col)
				return true
			})
		}

		if len(legendOrder) > 0 {
			ui.Row(c).Wrap().Gap(float32(t.Spacing8)).AlignItems(ui.Center).Children(func() {
				for _, label := range legendOrder {
					ui.Row(c).Gap(float32(t.Spacing4)).AlignItems(ui.Center).Children(func() {
						ui.Box(c).Size(float32(t.Spacing8), float32(t.Spacing8)).Radius(float32(t.ControlRadius)).Background(legendColors[label])
						ui.Text(c, fmt.Sprintf("%s %d", label, legendCounts[label])).FontSize(labelSize).TextColor(t.TextMuted).FontFeatures("tnum")
					})
				}
			})
		}
		if !mini && !props.HideFooter {
			ui.Text(c, fmt.Sprintf("%d / %d 孔已分配 · 悬停查看分组与样本明细", len(props.Wells), rows*cols)).FontSize(labelSize).TextColor(t.TextMuted)
		}
	})
}

func estimatedBadgeWidth(badge string, fontPx float32) float32 {
	var units float32
	for _, r := range badge {
		if r > 0x7f {
			units += 1.0
		} else {
			units += 0.56
		}
	}
	return units * fontPx
}

func wellHoverSummary(props Props, id string) string {
	well, assigned := props.Wells[id]
	role := effectiveRole(well, id, props.Roles)
	parts := []string{id}
	switch {
	case !assigned:
		parts = append(parts, "未分配")
	case well.Ambiguous:
		parts = append(parts, "分配冲突", well.Timepoint)
	case props.Blind:
		parts = append(parts, "盲态编号")
		if well.Replicate > 0 {
			parts = append(parts, fmt.Sprintf("复孔 #%d", well.Replicate))
		}
		if well.Timepoint != "" {
			parts = append(parts, well.Timepoint)
		}
	default:
		if well.Group != "" {
			if gl := strings.TrimSpace(props.GroupLabels[well.Group]); gl != "" {
				parts = append(parts, gl)
			} else {
				parts = append(parts, well.Group)
			}
		}
		if well.SampleID != "" {
			parts = append(parts, "样本 "+well.SampleID)
		}
		if len(well.Factors) > 0 {
			fdims := make([]string, 0, len(well.Factors))
			for dk := range well.Factors {
				fdims = append(fdims, dk)
			}
			sort.Strings(fdims)
			for _, dk := range fdims {
				parts = append(parts, dk+"="+well.Factors[dk])
			}
		}
		if well.Replicate > 0 {
			parts = append(parts, fmt.Sprintf("复孔 #%d", well.Replicate))
		}
		if rl := RoleLabel(role); rl != "" && role != "" && role != "sample" {
			parts = append(parts, rl)
		}
		if well.Timepoint != "" {
			parts = append(parts, well.Timepoint)
		}
	}
	return strings.Join(parts, " · ")
}

func effectiveRole(well WellState, id string, roles map[string]string) string {
	if well.Role != "" {
		return strings.ToLower(strings.TrimSpace(well.Role))
	}
	if roles != nil {
		return strings.ToLower(strings.TrimSpace(roles[id]))
	}
	return ""
}

// RoleLabel returns the researcher-facing Chinese label for a well role.
func RoleLabel(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "sample":
		return "常规样本"
	case "blank":
		return "空白对照 Blank"
	case "ntc":
		return "无模板阴参 NTC"
	case "standard":
		return "标准品 Std"
	case "positive_control":
		return "阳性对照"
	case "negative_control":
		return "阴性对照"
	case "vehicle":
		return "溶剂对照"
	default:
		return role
	}
}

// WellFillColor computes the circle fill color according to active ColorBy mode.
func WellFillColor(props Props, well WellState, assigned bool, role string, dark bool, t theme.Tokens) ui.Color {
	if !assigned {
		return t.Canvas
	}
	if well.Ambiguous {
		return t.Warning
	}
	if props.Blind {
		return t.BorderStrong
	}
	scale := buildFactorColorScale(props, dark, t)
	switch strings.ToLower(strings.TrimSpace(props.ColorBy)) {
	case "sample":
		if strings.TrimSpace(well.SampleID) != "" {
			return theme.CategoricalColor("sample:"+well.SampleID, dark)
		}
		return t.Elevated
	case "factor":
		if props.FactorKey != "" && well.Factors != nil {
			if val := strings.TrimSpace(well.Factors[props.FactorKey]); val != "" {
				if scale != nil {
					if c, ok := scale.colors[val]; ok {
						return c
					}
				}
				return theme.CategoricalColor(props.FactorKey+":"+val, dark)
			}
		}
		return t.Elevated
	case "replicate":
		if well.Replicate > 0 {
			palette := theme.CategoricalPalette(dark)
			return palette[(well.Replicate-1)%len(palette)]
		}
		return t.Elevated
	case "role":
		r := role
		if r == "" {
			r = "sample"
		}
		return theme.WellRoleColor(r, dark)
	default:
		return GroupColorIndexed(well.Group, props.GroupIndex, dark)
	}
}

func wellLegendCategory(props Props, well WellState, role string, dark bool, t theme.Tokens) (string, ui.Color) {
	return wellLegendCategoryWithScale(props, well, role, dark, t, buildFactorColorScale(props, dark, t))
}

func wellLegendCategoryWithScale(props Props, well WellState, role string, dark bool, t theme.Tokens, scale *factorColorScale) (string, ui.Color) {
	if props.Blind {
		return "盲态孔位", t.BorderStrong
	}
	switch strings.ToLower(strings.TrimSpace(props.ColorBy)) {
	case "sample":
		if s := strings.TrimSpace(well.SampleID); s != "" {
			return s, theme.CategoricalColor("sample:"+s, dark)
		}
		return "未标样本", t.Elevated
	case "factor":
		if props.FactorKey != "" && well.Factors != nil {
			if v := strings.TrimSpace(well.Factors[props.FactorKey]); v != "" {
				if scale != nil {
					if c, ok := scale.colors[v]; ok {
						return v, c
					}
				}
				return v, theme.CategoricalColor(props.FactorKey+":"+v, dark)
			}
		}
		return "未标因子", t.Elevated
	case "replicate":
		if well.Replicate > 0 {
			palette := theme.CategoricalPalette(dark)
			return fmt.Sprintf("复孔 #%d", well.Replicate), palette[(well.Replicate-1)%len(palette)]
		}
		return "未标复孔", t.Elevated
	case "role":
		r := role
		if r == "" {
			r = "sample"
		}
		return RoleLabel(r), theme.WellRoleColor(r, dark)
	default:
		if well.Group == "" {
			return "", ui.Color{}
		}
		display := well.Group
		if gl := strings.TrimSpace(props.GroupLabels[well.Group]); gl != "" {
			display = gl
		}
		return display, GroupColorIndexed(well.Group, props.GroupIndex, dark)
	}
}

func wellGlyph(props Props, well WellState, role string) string {
	return wellGlyphAdaptive(props, well, role, 26)
}

func wellGlyphAdaptive(props Props, well WellState, role string, cellW float32) string {
	if rs := roleShort(role); rs != "" {
		return rs
	}
	maxRunes := 3
	if cellW >= 48 {
		maxRunes = 7
	} else if cellW >= 38 {
		maxRunes = 6
	}
	switch strings.ToLower(strings.TrimSpace(props.ColorBy)) {
	case "replicate":
		if well.Replicate > 0 {
			return fmt.Sprintf("#%d", well.Replicate)
		}
	case "sample":
		if s := strings.TrimSpace(well.SampleID); s != "" {
			return compactTag(s, maxRunes)
		}
	case "factor":
		if props.FactorKey != "" && well.Factors != nil {
			if v := strings.TrimSpace(well.Factors[props.FactorKey]); v != "" {
				return compactTag(v, maxRunes)
			}
		}
	default:
		if cellW >= 38 {
			if s := strings.TrimSpace(well.SampleID); s != "" {
				return compactTag(s, maxRunes)
			}
			if well.Group != "" {
				lbl := well.Group
				if gl := strings.TrimSpace(props.GroupLabels[well.Group]); gl != "" {
					lbl = gl
				}
				return compactTag(strings.TrimPrefix(lbl, "g-"), maxRunes)
			}
		}
		if well.Replicate > 0 {
			return fmt.Sprintf("%d", well.Replicate)
		}
	}
	return ""
}

func compactTag(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{"cDNA-", "sample-", "Std-"} {
		if strings.HasPrefix(s, prefix) && len(s) > len(prefix) {
			s = strings.TrimPrefix(s, prefix)
			break
		}
	}
	rs := []rune(s)
	if len(rs) <= maxRunes {
		return s
	}
	// Preserve trailing identifier (e.g. "Donor-1" -> "Don-1" instead of "Donor-")
	if maxRunes >= 4 {
		last := rs[len(rs)-1]
		if unicode.IsDigit(last) {
			// Find contiguous trailing digits (and optional short separator like '-').
			tailStart := len(rs) - 1
			for tailStart > 0 && unicode.IsDigit(rs[tailStart-1]) {
				tailStart--
			}
			tail := rs[tailStart:]
			if len(tail) <= 2 && maxRunes-len(tail)-1 >= 2 {
				return string(rs[:maxRunes-len(tail)-1]) + "·" + string(tail)
			}
		}
	}
	return string(rs[:maxRunes])
}

func contrastInkForFill(fill ui.Color, assigned bool, t theme.Tokens) ui.Color {
	if !assigned || fill.A == 0 {
		return t.Text
	}
	lum := (0.299*float64(fill.R) + 0.587*float64(fill.G) + 0.114*float64(fill.B)) / 255.0
	if lum > 0.54 {
		return theme.LightTokens().Text
	}
	return theme.DarkTokens().Text
}

func roleShort(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "blank":
		return "B"
	case "positive_control":
		return "+"
	case "negative_control":
		return "-"
	case "standard":
		return "S"
	case "vehicle":
		return "V"
	case "ntc":
		return "N"
	default:
		return ""
	}
}
