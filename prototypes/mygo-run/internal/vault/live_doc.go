package vault

import (
	"os"
	"strings"
	"sync"
	"time"

	"asterlab-mygo-prototype/internal/asterdoc"
)

const (
	maxUndoStack            = 100
	sourceUndoCoalesceDelay = 800 * time.Millisecond
)

// LiveDocument represents one in-memory `.asterml` file shared across any number
// of open split panes/tabs. All visual edits and raw source edits mutate this single
// source of truth and participate in the unified 100-step Undo/Redo ring.
type LiveDocument struct {
	mu       sync.Mutex
	RelPath  string
	AbsPath  string
	Source   string
	Parsed   *asterdoc.Document
	ParseErr error
	LastSave time.Time
	DiskMtime time.Time
	Dirty    bool

	undoStack      []string
	redoStack      []string
	lastSourceEdit time.Time
}

// DocStore caches shared *LiveDocument instances keyed by normalized RelPath.
type DocStore struct {
	mu    sync.Mutex
	Vault *Vault
	Docs  map[string]*LiveDocument
}

// NewDocStore creates a shared document store backed by v.
func NewDocStore(v *Vault) *DocStore {
	return &DocStore{
		Vault: v,
		Docs:  map[string]*LiveDocument{},
	}
}

// Open loads (or returns the already-open shared instance for) relPath.
func (s *DocStore) Open(relPath string) (*LiveDocument, error) {
	if s == nil || s.Vault == nil {
		return nil, os.ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if doc, ok := s.Docs[relPath]; ok {
		return doc, nil
	}
	abs := s.Vault.AbsPath(relPath)
	rawBytes, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	raw := string(rawBytes)
	// Transparently normalize legacy 3-part envelopes on load without discarding data.
	if env, eErr := asterdoc.ParseEnvelope(raw); eErr == nil && (env.Execution != nil || env.Record != nil) {
		if flat, fErr := asterdoc.FlattenEnvelopeToSingleSource(raw); fErr == nil {
			raw = flat
		}
	}
	var mtime time.Time
	if info, stErr := os.Stat(abs); stErr == nil {
		mtime = info.ModTime()
	}
	parsed, pErr := asterdoc.Parse(raw)
	ld := &LiveDocument{
		RelPath:   relPath,
		AbsPath:   abs,
		Source:    raw,
		Parsed:    parsed,
		ParseErr:  pErr,
		DiskMtime: mtime,
	}
	s.Docs[relPath] = ld
	return ld, nil
}

// RenameKey updates a cached LiveDocument's path key after rename or move.
func (s *DocStore) RenameKey(oldRel, newRel string) {
	if s == nil || s.Vault == nil || oldRel == newRel {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ld, ok := s.Docs[oldRel]; ok {
		delete(s.Docs, oldRel)
		ld.mu.Lock()
		ld.RelPath = newRel
		ld.AbsPath = s.Vault.AbsPath(newRel)
		ld.mu.Unlock()
		s.Docs[newRel] = ld
	}
}

// Evict removes a document from cache (e.g., on delete).
func (s *DocStore) Evict(relPath string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.Docs, relPath)
	s.mu.Unlock()
}

// ReloadAllExternalChanged checks every cached LiveDocument against disk and
// reloads any file modified externally (e.g., by an AI Agent or external editor).
func (s *DocStore) ReloadAllExternalChanged() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	snapshot := make([]*LiveDocument, 0, len(s.Docs))
	for _, ld := range s.Docs {
		snapshot = append(snapshot, ld)
	}
	s.mu.Unlock()

	var changedPaths []string
	for _, ld := range snapshot {
		if changed, _ := ld.ReloadIfExternalChanged(); changed {
			changedPaths = append(changedPaths, ld.RelPath)
		}
	}
	return changedPaths
}

// ApplyEdit replaces Source (pushing previous Source onto the Undo stack),
// re-parses the document, and writes atomically to disk.
func (ld *LiveDocument) ApplyEdit(nextSource string) error {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	if nextSource == ld.Source {
		return nil
	}
	ld.pushUndoLocked(ld.Source)
	ld.Source = nextSource
	ld.Parsed, ld.ParseErr = asterdoc.Parse(nextSource)
	ld.Dirty = true
	return ld.writeLocked()
}

// ApplySourceDraft updates Source from raw text editor without failing if intermediate
// syntax is temporarily incomplete; only persists when valid or explicitly flushed.
// Rapid keystrokes within sourceUndoCoalesceDelay are coalesced into a single undo
// snapshot so continuous typing never exhausts the 100-step undo history.
func (ld *LiveDocument) ApplySourceDraft(nextSource string) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	if nextSource == ld.Source {
		return
	}
	now := time.Now()
	if ld.lastSourceEdit.IsZero() || now.Sub(ld.lastSourceEdit) > sourceUndoCoalesceDelay {
		ld.pushUndoLocked(ld.Source)
	}
	ld.lastSourceEdit = now
	ld.Source = nextSource
	ld.Parsed, ld.ParseErr = asterdoc.Parse(nextSource)
	ld.Dirty = true
	if ld.ParseErr == nil {
		_ = ld.writeLocked()
	}
}

// CanUndo reports whether undo snapshots exist.
func (ld *LiveDocument) CanUndo() bool {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	return len(ld.undoStack) > 0
}

// CanRedo reports whether redo snapshots exist.
func (ld *LiveDocument) CanRedo() bool {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	return len(ld.redoStack) > 0
}

// Undo restores the previous source snapshot and writes to disk.
func (ld *LiveDocument) Undo() bool {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	if len(ld.undoStack) == 0 {
		return false
	}
	prev := ld.undoStack[len(ld.undoStack)-1]
	ld.undoStack = ld.undoStack[:len(ld.undoStack)-1]
	ld.redoStack = append(ld.redoStack, ld.Source)
	ld.Source = prev
	ld.Parsed, ld.ParseErr = asterdoc.Parse(prev)
	ld.Dirty = true
	_ = ld.writeLocked()
	return true
}

// Redo reapplies a previously undone snapshot and writes to disk.
func (ld *LiveDocument) Redo() bool {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	if len(ld.redoStack) == 0 {
		return false
	}
	next := ld.redoStack[len(ld.redoStack)-1]
	ld.redoStack = ld.redoStack[:len(ld.redoStack)-1]
	ld.undoStack = append(ld.undoStack, ld.Source)
	ld.Source = next
	ld.Parsed, ld.ParseErr = asterdoc.Parse(next)
	ld.Dirty = true
	_ = ld.writeLocked()
	return true
}

// ReloadIfExternalChanged reloads disk content if another tool (Agent/editor) modified the file.
func (ld *LiveDocument) ReloadIfExternalChanged() (bool, error) {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	info, err := os.Stat(ld.AbsPath)
	if err != nil {
		return false, err
	}
	if !info.ModTime().After(ld.DiskMtime) {
		return false, nil
	}
	rawBytes, err := os.ReadFile(ld.AbsPath)
	if err != nil {
		return false, err
	}
	raw := string(rawBytes)
	if strings.ReplaceAll(raw, "\r\n", "\n") == strings.ReplaceAll(ld.Source, "\r\n", "\n") {
		ld.DiskMtime = info.ModTime()
		return false, nil
	}
	ld.pushUndoLocked(ld.Source)
	ld.Source = raw
	ld.Parsed, ld.ParseErr = asterdoc.Parse(raw)
	ld.DiskMtime = info.ModTime()
	ld.Dirty = false
	return true, nil
}

// Flush writes any pending source changes to disk immediately.
func (ld *LiveDocument) Flush() error {
	ld.mu.Lock()
	defer ld.mu.Unlock()
	if !ld.Dirty {
		return nil
	}
	return ld.writeLocked()
}

func (ld *LiveDocument) pushUndoLocked(snapshot string) {
	if len(ld.undoStack) > 0 && ld.undoStack[len(ld.undoStack)-1] == snapshot {
		return
	}
	ld.undoStack = append(ld.undoStack, snapshot)
	if len(ld.undoStack) > maxUndoStack {
		ld.undoStack = ld.undoStack[len(ld.undoStack)-maxUndoStack:]
	}
	ld.redoStack = nil
}

func (ld *LiveDocument) writeLocked() error {
	if ld.AbsPath == "" {
		return nil
	}
	if err := asterdoc.WriteFileAtomic(ld.AbsPath, []byte(ld.Source)); err != nil {
		return err
	}
	if info, err := os.Stat(ld.AbsPath); err == nil {
		ld.DiskMtime = info.ModTime()
	}
	ld.LastSave = time.Now()
	ld.Dirty = false
	return nil
}

// WriteWithVaultSnapshot persists via writeLocked and optionally records a throttled
// recovery snapshot plus Vault EventBus notification.
func (ld *LiveDocument) NotifySaved(v *Vault, forceSnapshot bool) {
	if ld == nil || v == nil {
		return
	}
	_ = v.RecordSnapshot(ld.RelPath, ld.Source, forceSnapshot)
	if v.Events != nil {
		v.Events.Emit(VaultEvent{Kind: EventModify, RelPath: ld.RelPath})
	}
}
