package nlengine

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

// Engine is AsterLab's general-purpose offline ONNX scientific text encoder.
// It exposes task-prefixed Encode(task, text) -> L2-normalized embedding so a single
// 23 MB resident model bundle can power component routing today and semantic retrieval,
// K2 operation alignment, or deviation clustering tomorrow without duplicating runtime code.
type Engine struct {
	bundleDir string
	manifest  BundleManifest
	tokenizer *WordPieceTokenizer
	session   *ORTSession
}

// OpenBundle loads, SHA256-verifies, and initializes a versioned model bundle from bundleDir.
func OpenBundle(bundleDir string) (*Engine, error) {
	m, err := LoadManifest(bundleDir)
	if err != nil {
		return nil, err
	}
	if m.TokenizerType != "wordpiece_bert" {
		return nil, fmt.Errorf("unsupported tokenizer_type %q", m.TokenizerType)
	}
	onnxPath := filepath.Join(bundleDir, m.ONNXFile)
	if err := VerifyFileSHA256(onnxPath, m.ONNXSHA256); err != nil {
		return nil, err
	}
	vocabPath := filepath.Join(bundleDir, m.VocabFile)
	tok, err := LoadWordPieceVocab(vocabPath, m.DoLowerCase)
	if err != nil {
		return nil, fmt.Errorf("load vocab: %w", err)
	}
	sess, err := NewORTSession(bundleDir, onnxPath, m.InputNames, m.OutputName, m.EmbedDim)
	if err != nil {
		return nil, err
	}
	return &Engine{
		bundleDir: bundleDir,
		manifest:  m,
		tokenizer: tok,
		session:   sess,
	}, nil
}

// OpenDefault discovers the highest-priority valid model bundle on disk and opens it.
func OpenDefault() (*Engine, error) {
	dir, err := ResolveBundleDir()
	if err != nil {
		return nil, err
	}
	return OpenBundle(dir)
}

// Manifest returns the loaded bundle's self-describing manifest.
func (e *Engine) Manifest() BundleManifest { return e.manifest }

// BundleDir returns the resolved filesystem directory of the active bundle.
func (e *Engine) BundleDir() string { return e.bundleDir }

// ORTVersion returns the loaded ONNX Runtime library version string.
func (e *Engine) ORTVersion() string { return e.session.ORTVersion() }

// Encode produces an L2-normalized embedding for text under the given task name
// (e.g., "route", "route_desc", or future "retrieve" / "k2_align").
func (e *Engine) Encode(task, text string) ([]float32, error) {
	maxLen := e.manifest.MaxTextLen
	if strings.HasSuffix(task, "_desc") {
		maxLen = e.manifest.MaxDescLen
	}
	if prefix := e.manifest.TaskPrefixes[task]; prefix != "" {
		text = prefix + text
	}
	ids, mask := e.tokenizer.Encode(text, maxLen)
	vec, err := e.session.EncodeIDs(ids, mask)
	if err != nil {
		return nil, err
	}
	L2NormalizeInPlace(vec)
	return vec, nil
}

// Close releases underlying ONNX Runtime resources.
func (e *Engine) Close() error {
	if e == nil || e.session == nil {
		return nil
	}
	return e.session.Close()
}

// L2NormalizeInPlace normalizes v to unit L2 norm in place.
func L2NormalizeInPlace(v []float32) {
	var sum float64
	for _, x := range v {
		sum += float64(x) * float64(x)
	}
	if sum <= 1e-12 {
		return
	}
	inv := float32(1.0 / math.Sqrt(sum))
	for i := range v {
		v[i] *= inv
	}
}

// CosineSim computes the dot product of two L2-normalized vectors.
func CosineSim(a, b []float32) float32 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	var dot float32
	for i := 0; i < n; i++ {
		dot += a[i] * b[i]
	}
	return dot
}
