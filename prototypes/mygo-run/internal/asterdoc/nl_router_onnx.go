package asterdoc

import (
	"fmt"
	"math"
	"strings"
	"sync"

	"asterlab-mygo-prototype/internal/nlengine"
)

// nlZhSchemaHints supplements spec/nl_schemas.json's English Trigger Descriptors with concise
// Chinese operational cues, matching scripts/nl-router/export_bundle.py so cached descriptor
// hashes match on startup and new components auto-encode cleanly.
var nlZhSchemaHints = map[string]string{
	"none":                 "普通湿实验文字记录或未支持操作，无需插入交互组件：常规PBS清洗、移液吹打混匀、吸弃上清、-20°C/-80°C冰箱保存、细胞代次汇合度观察、历史实验复盘、文献综述、SOP规范、耗材清单、仪器参数检查或未注册专业工具。",
	"timer.cycle":          "循环计时器：多阶段交替定时重复多轮操作，例如水浴与冰上交替3轮、反复冻融循环、热激与冰浴交替、超声开停循环。",
	"timer.countdown":      "倒计时器：单次固定时长的主动操作定时，包括孵育、离心(如300g 5min)、胰酶消化、PFA固定、封闭、透膜、裂解、酶切、摇床脱色或光交联倒计时。",
	"timer.stopwatch":      "正计时秒表：开放时长操作，手工开启秒表计时直至肉眼或物理终点（如完全固化、细胞变圆脱落、显色变蓝、团块散开）并记录实际耗时秒数。",
	"timer.window":         "时间锚点等待窗：最小静置/浸泡平衡/细胞贴壁时间窗（至少2小时、不少于4小时、4°C过夜孵育≥12h）或以t0为锚点的后续多时间点随访排程（如6h/12h/24h/48h）。",
	"calculator.dilution":  "溶液稀释计算器(C1V1=C2V2)：将已知母液浓度稀释至目标工作浓度和配制终体积（含100x稀释为1x及缺参稀释请求），计算需取母液体积和补液量。",
	"calculator.weighing":  "称量配液计算器：根据目标质量体积浓度(% w/v)或摩尔浓度(mM与分子量MW)及配制体积，计算需称取的固体干粉质量(mg/g)。",
	"calculator.mastermix": "孔板预混液计算器：按单孔试剂用量放大至多孔/多反应体系，并包含富余比例(如+10%/+15%余量或额外多算几孔)计算预混总液量。",
	"calculator.seeding":   "细胞接种计算器：根据实测细胞悬液密度(cells/mL)、每孔目标细胞数、接种孔数与每孔体积，计算需取细胞悬液和补加培养基体积。",
	"plate.layout":         "孔板与实验分组布局：定义6/12/24/48/96孔板规格、各实验处理组/对照组/空白组及每组技术复孔排布。",
	"attachment.grid":      "孔板采图网格：定义多孔板系统性显微拍照或荧光成像方案，指定采图孔位、每孔非重叠视野数(FOV)或荧光通道槽位。",
}

type nlTargetEmbedding struct {
	ID        string
	Kind      string
	Mode      string
	Embedding []float32
}

// NLOnnxRouter runs offline ~7ms pure-CPU semantic routing using the resident nlengine.Engine.
// It automatically synchronizes against embedded spec/nl_schemas.json on startup so newly added
// component schemas are encoded Zero-Shot without retraining or recompiling Go.
type NLOnnxRouter struct {
	mu          sync.RWMutex
	engine      *nlengine.Engine
	targets     []nlTargetEmbedding // index 0 is ALWAYS "none"
	singleVec   []float32           // optional meta.single_action probe (v1.3.0+)
	multiVec    []float32           // optional meta.multi_action probe (v1.3.0+)
	hasMultiRef bool
	cache       map[string]*JevRouteDecision
}

// OpenNLOnnxRouter opens a specific model bundle directory and synchronizes schema embeddings.
func OpenNLOnnxRouter(bundleDir string) (*NLOnnxRouter, error) {
	eng, err := nlengine.OpenBundle(bundleDir)
	if err != nil {
		return nil, err
	}
	return newNLOnnxRouter(eng)
}

// OpenDefaultNLOnnxRouter resolves and opens the highest-priority model bundle on disk.
func OpenDefaultNLOnnxRouter() (*NLOnnxRouter, error) {
	eng, err := nlengine.OpenDefault()
	if err != nil {
		return nil, err
	}
	return newNLOnnxRouter(eng)
}

func newNLOnnxRouter(eng *nlengine.Engine) (*NLOnnxRouter, error) {
	reg, err := LoadNLSchemaRegistry()
	if err != nil {
		_ = eng.Close()
		return nil, err
	}
	m := eng.Manifest()

	// Load optional precomputed embeddings cache from bundle
	cachedByID := map[string]nlengine.SchemaEmbeddingEntry{}
	if sf, err := nlengine.LoadSchemasCache(eng.BundleDir(), m.SchemasFile); err == nil && sf != nil {
		for _, ent := range sf.Entries {
			cachedByID[ent.ID] = ent
		}
	}

	resolveVec := func(id, desc string) ([]float32, error) {
		h := nlengine.HashDescriptor(desc)
		if ent, ok := cachedByID[id]; ok && ent.DescHash == h && len(ent.Embedding) == m.EmbedDim {
			vec := make([]float32, len(ent.Embedding))
			copy(vec, ent.Embedding)
			nlengine.L2NormalizeInPlace(vec)
			return vec, nil
		}
		// Zero-Shot auto-encode on startup for any new or edited schema in spec/nl_schemas.json!
		return eng.Encode("route_desc", desc)
	}

	noneDesc := CanonicalSchemaDescriptor(reg, "none", nil)
	noneVec, err := resolveVec("none", noneDesc)
	if err != nil {
		_ = eng.Close()
		return nil, fmt.Errorf("encode none anchor: %w", err)
	}
	targets := []nlTargetEmbedding{{ID: "none", Kind: "none", Embedding: noneVec}}

	for i := range reg.Targets {
		sc := &reg.Targets[i]
		desc := CanonicalSchemaDescriptor(reg, sc.ID, sc)
		vec, err := resolveVec(sc.ID, desc)
		if err != nil {
			_ = eng.Close()
			return nil, fmt.Errorf("encode schema %s: %w", sc.ID, err)
		}
		targets = append(targets, nlTargetEmbedding{
			ID:        sc.ID,
			Kind:      sc.Kind,
			Mode:      sc.Mode,
			Embedding: vec,
		})
	}

	var singleVec, multiVec []float32
	hasMultiRef := false
	if sEnt, ok1 := cachedByID["meta.single_action"]; ok1 && len(sEnt.Embedding) == m.EmbedDim {
		if mEnt, ok2 := cachedByID["meta.multi_action"]; ok2 && len(mEnt.Embedding) == m.EmbedDim {
			singleVec = make([]float32, len(sEnt.Embedding))
			copy(singleVec, sEnt.Embedding)
			nlengine.L2NormalizeInPlace(singleVec)
			multiVec = make([]float32, len(mEnt.Embedding))
			copy(multiVec, mEnt.Embedding)
			nlengine.L2NormalizeInPlace(multiVec)
			hasMultiRef = true
		}
	}

	return &NLOnnxRouter{
		engine:      eng,
		targets:     targets,
		singleVec:   singleVec,
		multiVec:    multiVec,
		hasMultiRef: hasMultiRef,
		cache:       make(map[string]*JevRouteDecision, 256),
	}, nil
}

// HasMultiActionProbe reports whether the loaded model bundle provides semantic
// meta.single_action / meta.multi_action probe vectors (v1.3.0+).
func (r *NLOnnxRouter) HasMultiActionProbe() bool {
	if r == nil {
		return false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.hasMultiRef
}

// CanonicalSchemaDescriptor builds the canonical bilingual descriptor text for a schema ID.
func CanonicalSchemaDescriptor(reg NLSchemaRegistry, id string, sc *NLComponentInputSchema) string {
	if id == "none" || sc == nil {
		return fmt.Sprintf("无需组件 (none) | %s | %s", nlZhSchemaHints["none"], reg.GlobalNegativeBoundary)
	}
	zh := nlZhSchemaHints[id]
	if zh == "" {
		zh = sc.Title
	}
	return fmt.Sprintf("%s (%s) | %s | %s | Not: %s", sc.Title, id, zh, sc.WhenToSuggest, sc.WhenNotToSuggest)
}

// Engine returns the underlying general-purpose nlengine.Engine (for future semantic retrieval,
// K2 operation alignment, or cross-block reference matching without reloading ONNX).
func (r *NLOnnxRouter) Engine() *nlengine.Engine {
	return r.engine
}

// RegisterZeroShotSchema dynamically appends a brand-new candidate component schema at runtime
// by encoding its natural-language description through the shared ONNX encoder.
func (r *NLOnnxRouter) RegisterZeroShotSchema(id, kind, mode, description string) error {
	vec, err := r.engine.Encode("route_desc", description)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.targets {
		if r.targets[i].ID == id {
			r.targets[i].Kind = kind
			r.targets[i].Mode = mode
			r.targets[i].Embedding = vec
			r.cache = make(map[string]*JevRouteDecision, 256)
			return nil
		}
	}
	r.targets = append(r.targets, nlTargetEmbedding{
		ID:        id,
		Kind:      kind,
		Mode:      mode,
		Embedding: vec,
	})
	r.cache = make(map[string]*JevRouteDecision, 256)
	return nil
}

// RouteResult holds both the routing decision and raw target ID for tests/diagnostics.
type RouteResult struct {
	TargetID   string
	Kind       string
	Mode       string
	PosProb    float64 // 1 - P(none)
	Confidence float64 // P(chosen target)
	IsMulti    bool    // true when v1.3.0+ semantic multi-action probe fires on the same 512-d vector
	MultiProb  float64 // P(multi_action) in [0, 1]
}

// RouteDetail classifies sentence offline via the resident INT8 ONNX encoder with Hierarchical Gate decoding.
func (r *NLOnnxRouter) RouteDetail(sentence string) (RouteResult, error) {
	vec, err := r.engine.Encode("route", sentence)
	if err != nil {
		return RouteResult{}, err
	}
	m := r.engine.Manifest()

	r.mu.RLock()
	targets := r.targets
	singleVec := r.singleVec
	multiVec := r.multiVec
	hasMultiRef := r.hasMultiRef
	r.mu.RUnlock()

	isMulti := false
	multiProb := 0.0
	if hasMultiRef {
		diff := float64((nlengine.CosineSim(vec, multiVec) - nlengine.CosineSim(vec, singleVec)) * m.Scale)
		multiProb = 1.0 / (1.0 + math.Exp(-diff))
		isMulti = multiProb >= 0.50
	}

	n := len(targets)
	logits := make([]float64, n)
	maxLogit := math.Inf(-1)
	for i, tgt := range targets {
		l := float64(nlengine.CosineSim(vec, tgt.Embedding) * m.Scale)
		if i == 0 {
			l += float64(m.NoulBias)
		}
		logits[i] = l
		if l > maxLogit {
			maxLogit = l
		}
	}

	probs := make([]float64, n)
	var sumExp float64
	for i, l := range logits {
		e := math.Exp(l - maxLogit)
		probs[i] = e
		sumExp += e
	}
	for i := range probs {
		probs[i] /= sumExp
	}

	pPos := 1.0 - probs[0]
	if pPos < float64(m.PosGate) || n <= 1 {
		return RouteResult{
			TargetID:   "none",
			Kind:       "none",
			PosProb:    pPos,
			Confidence: probs[0],
			IsMulti:    isMulti,
			MultiProb:  multiProb,
		}, nil
	}

	bestIdx := 1
	for i := 2; i < n; i++ {
		if probs[i] > probs[bestIdx] {
			bestIdx = i
		}
	}
	// Calibrated low-confidence split guard: suppress scattered uncertainty (< 0.35 top class prob
	// when overall positivity isn't strong) to prevent noisy false triggers on ambiguous mixed prose.
	if probs[bestIdx] < 0.35 && pPos < 0.85 {
		return RouteResult{
			TargetID:   "none",
			Kind:       "none",
			PosProb:    pPos,
			Confidence: probs[0],
			IsMulti:    isMulti,
			MultiProb:  multiProb,
		}, nil
	}
	best := targets[bestIdx]
	return RouteResult{
		TargetID:   best.ID,
		Kind:       best.Kind,
		Mode:       best.Mode,
		PosProb:    pPos,
		Confidence: probs[bestIdx],
		IsMulti:    isMulti,
		MultiProb:  multiProb,
	}, nil
}

// Route returns a cached JevRouteDecision for sentence using the local INT8 ONNX router.
func (r *NLOnnxRouter) Route(sentence string) (*JevRouteDecision, bool) {
	key := strings.TrimSpace(sentence)
	if key == "" {
		return nil, false
	}
	r.mu.RLock()
	if dec, ok := r.cache[key]; ok {
		r.mu.RUnlock()
		return dec, true
	}
	r.mu.RUnlock()

	res, err := r.RouteDetail(key)
	if err != nil {
		return nil, false
	}
	dec := &JevRouteDecision{
		Kind:       res.Kind,
		Mode:       res.Mode,
		Confidence: res.Confidence,
		Noul:       res.PosProb,
		IsMulti:    res.IsMulti,
		MultiProb:  res.MultiProb,
		Source:     "本地语义",
	}
	r.mu.Lock()
	if len(r.cache) > 1024 {
		r.cache = make(map[string]*JevRouteDecision, 256)
	}
	r.cache[key] = dec
	r.mu.Unlock()
	return dec, true
}

// Close releases the underlying ONNX Runtime session.
func (r *NLOnnxRouter) Close() error {
	if r == nil || r.engine == nil {
		return nil
	}
	return r.engine.Close()
}

var (
	defaultRouterMu     sync.RWMutex
	defaultRouterInit   bool
	defaultRouter       *NLOnnxRouter
	defaultRouterErr    error
)

// DefaultNLOnnxRouter lazily initializes and returns the singleton offline ONNX router (or nil if unavailable).
func DefaultNLOnnxRouter() *NLOnnxRouter {
	defaultRouterMu.RLock()
	if defaultRouterInit {
		r := defaultRouter
		defaultRouterMu.RUnlock()
		return r
	}
	defaultRouterMu.RUnlock()

	defaultRouterMu.Lock()
	defer defaultRouterMu.Unlock()
	if defaultRouterInit {
		return defaultRouter
	}
	defaultRouterInit = true
	r, err := OpenDefaultNLOnnxRouter()
	defaultRouter = r
	defaultRouterErr = err
	return defaultRouter
}

// ReloadDefaultNLRouter hot-reloads the model bundle from disk (e.g., after a user drops a
// newer model bundle into %LOCALAPPDATA%/AsterLab/models/nl-router/) without restarting the app.
func ReloadDefaultNLRouter() (*NLOnnxRouter, error) {
	next, err := OpenDefaultNLOnnxRouter()
	if err != nil {
		return nil, err
	}
	defaultRouterMu.Lock()
	old := defaultRouter
	defaultRouter = next
	defaultRouterErr = nil
	defaultRouterInit = true
	defaultRouterMu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return next, nil
}
