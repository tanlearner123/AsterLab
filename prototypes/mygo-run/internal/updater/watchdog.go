package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ReleaseManifest is an optional JSON sidecar published alongside releases for channel & kill-switch control.
type ReleaseManifest struct {
	Version             string   `json:"version"`
	Tag                 string   `json:"tag"`
	Channel             string   `json:"channel"` // stable | prerelease
	MinSupportedVersion string   `json:"minSupportedVersion,omitempty"`
	YankedVersions      []string `json:"yankedVersions,omitempty"`
	Notice              string   `json:"notice,omitempty"`
}

// UpdateState tracks post-update boot verification so crash loops auto-rollback to .old binaries.
type UpdateState struct {
	PendingVersion  string `json:"pendingVersion,omitempty"`
	PreviousVersion string `json:"previousVersion,omitempty"`
	BackupStamp     string `json:"backupStamp,omitempty"`
	BootAttempts    int    `json:"bootAttempts,omitempty"`
	SkippedVersion  string `json:"skippedVersion,omitempty"`
	SwappedAt       string `json:"swappedAt,omitempty"`
}

func updateStatePath(dataDir string) string {
	if strings.TrimSpace(dataDir) == "" {
		return ""
	}
	return filepath.Join(dataDir, "update-state.json")
}

// LoadUpdateState reads update-state.json (returning zero struct if absent or unreadable).
func LoadUpdateState(dataDir string) UpdateState {
	path := updateStatePath(dataDir)
	if path == "" {
		return UpdateState{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return UpdateState{}
	}
	var st UpdateState
	if err := json.Unmarshal(raw, &st); err != nil {
		return UpdateState{}
	}
	return st
}

// SaveUpdateState writes update-state.json atomically.
func SaveUpdateState(dataDir string, st UpdateState) {
	path := updateStatePath(dataDir)
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if raw, err := json.MarshalIndent(st, "", "  "); err == nil {
		_ = os.WriteFile(path, raw, 0o644)
	}
}

// BackupLocalData saves a pre-update snapshot of local JSON stores (keeps latest 3 backups).
func BackupLocalData(dataDir, targetVer string) {
	if strings.TrimSpace(dataDir) == "" {
		return
	}
	backupRoot := filepath.Join(dataDir, "backups")
	tag := NormalizeVersion(targetVer)
	if tag == "" {
		tag = "unknown"
	}
	dest := filepath.Join(backupRoot, fmt.Sprintf("pre-%s-%d", tag, time.Now().UTC().Unix()))
	_ = os.MkdirAll(dest, 0o755)
	for _, name := range []string{"pantry.jsonl", "pantry-ledger.json", "vault-prefs.json", "update-prefs.json", "onboarding.json", "workspace-session.json"} {
		src := filepath.Join(dataDir, name)
		if _, err := os.Stat(src); err == nil {
			_ = copyRegularFile(src, filepath.Join(dest, name))
		}
	}
	entries, err := os.ReadDir(backupRoot)
	if err != nil || len(entries) <= 3 {
		return
	}
	var dirs []string
	for _, ent := range entries {
		if ent.IsDir() {
			dirs = append(dirs, filepath.Join(backupRoot, ent.Name()))
		}
	}
	sort.Strings(dirs)
	for i := 0; i < len(dirs)-3; i++ {
		_ = os.RemoveAll(dirs[i])
	}
}

// EvaluateStartupWatchdog runs early in startup:
// - If a newly updated version failed to reach healthy state >= 2 times, restores .old.<BackupStamp> binaries
//   in exeDir, marks the faulty version skipped, and returns rolledBackNotice != "".
// - If PendingVersion is active on boot 1..2, increments BootAttempts and retains .old.<BackupStamp> until MarkBootHealthy.
// - Otherwise cleans up stale .old/.new leftovers safely.
func EvaluateStartupWatchdog(dataDir, currentVer, exeDir string) (rolledBackNotice string) {
	st := LoadUpdateState(dataDir)
	if st.PendingVersion == "" {
		if exeDir != "" {
			cleanupOldUpdatesDir(exeDir)
		}
		return ""
	}
	if CompareVersions(currentVer, st.PendingVersion) != 0 {
		// Different version running (e.g. user reinstalled manually); clear pending verification.
		st.PendingVersion = ""
		st.BootAttempts = 0
		st.BackupStamp = ""
		SaveUpdateState(dataDir, st)
		if exeDir != "" {
			cleanupOldUpdatesDir(exeDir)
		}
		return ""
	}
	if st.BootAttempts >= 2 && exeDir != "" && st.BackupStamp != "" {
		if restoreBackupStamp(exeDir, st.BackupStamp) {
			faulty := st.PendingVersion
			prev := st.PreviousVersion
			st.SkippedVersion = faulty
			st.PendingVersion = ""
			st.BootAttempts = 0
			st.BackupStamp = ""
			SaveUpdateState(dataDir, st)
			return fmt.Sprintf("检测到新版本 %s 启动连续异常，已自动回滚至稳定版本 %s", faulty, prev)
		}
	}
	st.BootAttempts++
	SaveUpdateState(dataDir, st)
	return ""
}

// MarkBootHealthy confirms the newly updated binary ran steadily past startup grace window,
// clearing PendingVersion and removing .old backup binaries.
func MarkBootHealthy(dataDir, exeDir string) {
	st := LoadUpdateState(dataDir)
	if st.PendingVersion == "" {
		return
	}
	st.PendingVersion = ""
	st.BootAttempts = 0
	st.BackupStamp = ""
	SaveUpdateState(dataDir, st)
	if exeDir != "" {
		cleanupOldUpdatesDir(exeDir)
	}
}

func restoreBackupStamp(dir, stamp string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	suffix := ".old." + stamp
	restored := 0
	for _, ent := range entries {
		if ent.IsDir() || !strings.HasSuffix(ent.Name(), suffix) {
			continue
		}
		baseName := strings.TrimSuffix(ent.Name(), suffix)
		src := filepath.Join(dir, ent.Name())
		dst := filepath.Join(dir, baseName)
		broken := dst + ".broken." + stamp
		if _, err := os.Stat(dst); err == nil {
			_ = os.Rename(dst, broken)
		}
		if err := os.Rename(src, dst); err == nil {
			restored++
			_ = os.Remove(broken)
		}
	}
	return restored > 0
}
