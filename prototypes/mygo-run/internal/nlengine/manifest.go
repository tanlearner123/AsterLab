package nlengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// BundleSchemaVersion is the current manifest contract version supported by nlengine.
const BundleSchemaVersion = 1

// BundleManifest is the self-describing upgrade contract stored in <bundle>/manifest.json.
// Dropping a new compatible bundle into the user override directory upgrades the local
// semantic encoder and routing calibration with ZERO Go code changes or recompilation.
type BundleManifest struct {
	SchemaVersion int               `json:"schema_version"`
	ModelVersion  string            `json:"model_version"`
	Backbone      string            `json:"backbone"`
	ONNXFile      string            `json:"onnx_file"`
	ONNXSHA256    string            `json:"onnx_sha256,omitempty"`
	VocabFile     string            `json:"vocab_file"`
	SchemasFile   string            `json:"schemas_file,omitempty"`
	TokenizerType string            `json:"tokenizer_type"` // "wordpiece_bert"
	DoLowerCase   bool              `json:"do_lower_case"`
	MaxTextLen    int               `json:"max_text_len"`
	MaxDescLen    int               `json:"max_desc_len"`
	EmbedDim      int               `json:"embed_dim"`
	InputNames    []string          `json:"input_names"` // ["input_ids", "attention_mask"]
	OutputName    string            `json:"output_name"` // "embedding"
	Scale         float32           `json:"scale"`
	NoulBias      float32           `json:"noul_bias"`
	PosGate       float32           `json:"pos_gate"`
	TaskPrefixes  map[string]string `json:"task_prefixes,omitempty"` // reserved for multi-task "[ROUTE] ", "[RETRIEVE] ", etc.
}

// SchemaEmbeddingEntry stores one precomputed schema descriptor vector and its content hash
// so startup can detect new or edited spec/nl_schemas.json entries and auto-encode them Zero-Shot.
type SchemaEmbeddingEntry struct {
	ID        string    `json:"id"`
	DescHash  string    `json:"desc_hash"`
	Embedding []float32 `json:"embedding"`
}

// SchemasCacheFile is the portable JSON format written by scripts/nl-router/export_bundle.py.
type SchemasCacheFile struct {
	ModelVersion string                 `json:"model_version"`
	EmbedDim     int                    `json:"embed_dim"`
	Entries      []SchemaEmbeddingEntry `json:"entries"`
}

// LoadManifest reads and validates manifest.json inside bundleDir.
func LoadManifest(bundleDir string) (BundleManifest, error) {
	p := filepath.Join(bundleDir, "manifest.json")
	raw, err := os.ReadFile(p)
	if err != nil {
		return BundleManifest{}, err
	}
	var m BundleManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return BundleManifest{}, fmt.Errorf("parse manifest.json: %w", err)
	}
	if m.SchemaVersion <= 0 || m.SchemaVersion > BundleSchemaVersion {
		return BundleManifest{}, fmt.Errorf("unsupported bundle schema_version %d (supported <= %d)", m.SchemaVersion, BundleSchemaVersion)
	}
	if m.ONNXFile == "" || m.VocabFile == "" {
		return BundleManifest{}, fmt.Errorf("manifest missing onnx_file or vocab_file")
	}
	if m.TokenizerType == "" {
		m.TokenizerType = "wordpiece_bert"
	}
	if m.MaxTextLen <= 0 {
		m.MaxTextLen = 96
	}
	if m.MaxDescLen <= 0 {
		m.MaxDescLen = 128
	}
	if m.EmbedDim <= 0 {
		m.EmbedDim = 512
	}
	if len(m.InputNames) < 2 {
		m.InputNames = []string{"input_ids", "attention_mask"}
	}
	if m.OutputName == "" {
		m.OutputName = "embedding"
	}
	if m.Scale <= 0 {
		m.Scale = 16.0
	}
	if m.PosGate <= 0 || m.PosGate >= 1 {
		m.PosGate = 0.50
	}
	return m, nil
}

// LoadSchemasCache reads the optional precomputed schema embeddings file from bundleDir.
func LoadSchemasCache(bundleDir, schemasFile string) (*SchemasCacheFile, error) {
	if strings.TrimSpace(schemasFile) == "" {
		schemasFile = "schemas.json"
	}
	raw, err := os.ReadFile(filepath.Join(bundleDir, schemasFile))
	if err != nil {
		return nil, err
	}
	var sf SchemasCacheFile
	if err := json.Unmarshal(raw, &sf); err != nil {
		return nil, err
	}
	return &sf, nil
}

// VerifyFileSHA256 checks path against expectedHex (skipped if expectedHex is empty).
func VerifyFileSHA256(path, expectedHex string) error {
	expected := strings.ToLower(strings.TrimSpace(expectedHex))
	if expected == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	got := hex.EncodeToString(h.Sum(nil))
	if got != expected {
		return fmt.Errorf("sha256 mismatch for %s: got %s, want %s", filepath.Base(path), got, expected)
	}
	return nil
}

// HashDescriptor computes a short stable SHA256 digest of a schema descriptor string.
func HashDescriptor(desc string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(desc)))
	return hex.EncodeToString(sum[:8])
}

// CandidateBundleDirs returns model bundle directories ordered by upgrade precedence:
//  1. ASTERLAB_NL_ROUTER_DIR env var (explicit override / A-B testing)
//  2. User hot-upgrade dir (%LOCALAPPDATA%/AsterLab/models/nl-router)
//  3. App-adjacent release dir (<exe_dir>/models/nl-router)
//  4. Workspace dev dirs (walking up from cwd for models/nl-router)
func CandidateBundleDirs() []string {
	var dirs []string
	if env := strings.TrimSpace(os.Getenv("ASTERLAB_NL_ROUTER_DIR")); env != "" {
		dirs = append(dirs, env)
	}
	if localApp := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localApp != "" {
		dirs = append(dirs, filepath.Join(localApp, "AsterLab", "models", "nl-router"))
	} else if cfgDir, err := os.UserConfigDir(); err == nil && cfgDir != "" {
		dirs = append(dirs, filepath.Join(cfgDir, "AsterLab", "models", "nl-router"))
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, filepath.Join(filepath.Dir(exe), "models", "nl-router"))
	}
	if cwd, err := os.Getwd(); err == nil {
		cur := cwd
		for i := 0; i < 5; i++ {
			dirs = append(dirs, filepath.Join(cur, "models", "nl-router"))
			dirs = append(dirs, filepath.Join(cur, "prototypes", "mygo-run", "models", "nl-router"))
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
		}
	}
	// Deduplicate preserving order
	seen := map[string]bool{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		clean := filepath.Clean(d)
		if !seen[clean] {
			seen[clean] = true
			out = append(out, clean)
		}
	}
	return out
}

// ResolveBundleDir returns the highest-priority existing bundle directory containing manifest.json.
func ResolveBundleDir() (string, error) {
	for _, d := range CandidateBundleDirs() {
		if st, err := os.Stat(filepath.Join(d, "manifest.json")); err == nil && !st.IsDir() {
			return d, nil
		}
	}
	return "", fmt.Errorf("no nl-router model bundle found (checked ASTERLAB_NL_ROUTER_DIR, user dir, exe dir, workspace)")
}
