package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"asterlab-mygo-prototype/internal/diag"
	"asterlab-mygo-prototype/internal/shell"
	"asterlab-mygo-prototype/internal/updater"
	"asterlab-mygo-prototype/modules/runs"
	mygo "github.com/egoist/mygo"
)

type onboardingState struct {
	Seen bool `json:"seen"`
}

func onboardingPath() string {
	dir := workspaceDataDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "onboarding.json")
}

func hasSeenOnboarding() bool {
	if strings.TrimSpace(os.Getenv("ASTERLAB_SKIP_WELCOME")) != "" {
		return true
	}
	path := onboardingPath()
	if path == "" {
		return true
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var st onboardingState
	if err := json.Unmarshal(raw, &st); err != nil {
		return false
	}
	return st.Seen
}

func markOnboardingSeen() {
	path := onboardingPath()
	if path == "" {
		return
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if raw, err := json.MarshalIndent(onboardingState{Seen: true}, "", "  "); err == nil {
		_ = os.WriteFile(path, raw, 0o644)
	}
}

func astermlFileArg(args []string) string {
	for _, arg := range args {
		tr := strings.TrimSpace(arg)
		if tr == "" || strings.HasPrefix(tr, "-") || strings.HasPrefix(tr, "asterlab://") {
			continue
		}
		if strings.EqualFold(filepath.Ext(tr), ".asterml") {
			if abs, err := filepath.Abs(tr); err == nil {
				return abs
			}
			return tr
		}
	}
	return ""
}

func (a *app) currentVaultStatsLabel() string {
	if a.shell == nil {
		return ""
	}
	expCnt, tplCnt, assetCnt := 0, 0, 0
	for _, f := range a.shell.VaultFiles {
		switch {
		case f.IsTemplate:
			tplCnt++
		case f.IsAsset:
			assetCnt++
		default:
			expCnt++
		}
	}
	return fmt.Sprintf("%d 实验 · %d 模板 · %d 附件", expCnt, tplCnt, assetCnt)
}

func (a *app) currentDiagnosticEnv() diag.EnvironmentMeta {
	return diag.EnvironmentMeta{
		AppVersion:  updater.Version,
		BuildCommit: updater.Commit,
		VaultStats:  a.currentVaultStatsLabel(),
		DataDir:     workspaceDataDir(),
	}
}

func (a *app) wireFeedbackAndWelcome(rollbackNotice string) {
	if a.shell == nil {
		return
	}
	a.shell.FeedbackIncludeLogs = true
	if a.shell.FeedbackCategory == "" {
		a.shell.FeedbackCategory = "程序异常"
	}

	if rollbackNotice != "" {
		a.shell.CrashNotice = rollbackNotice
	} else if crash := diag.PendingCrash(""); crash != nil {
		a.shell.CrashNotice = fmt.Sprintf("检测到上次运行发生意外异常（%s）：%s", crash.Tag, crash.Summary)
	}

	a.shell.OnDismissCrashNotice = func() {
		diag.DismissPendingCrash("")
	}

	a.shell.OnExportDiagBundle = func() {
		defaultName := fmt.Sprintf("AsterLab-Diagnostics-%s.zip", time.Now().Format("20060102-150405"))
		home, _ := os.UserHomeDir()
		defaultPath := filepath.Join(home, defaultName)
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      mygo.FocusedWindow(),
			Title:       "导出脱敏诊断包 (.zip)",
			DefaultPath: defaultPath,
			Filters:     []mygo.FileFilter{{Name: "ZIP 诊断压缩包", Extensions: []string{"zip"}}},
		})
		if err != nil {
			a.shell.FeedbackStatus = "导出取消或弹窗失败：" + err.Error()
			return
		}
		if strings.TrimSpace(path) == "" {
			return
		}
		if err := diag.ExportBundle(path, a.currentDiagnosticEnv()); err != nil {
			a.shell.FeedbackStatus = "导出诊断包失败：" + err.Error()
			return
		}
		a.shell.FeedbackStatus = "已导出脱敏诊断包：" + filepath.Base(path)
		go mygo.Shell.ShowItemInFolder(path)
	}

	a.shell.OnCopyFeedbackReport = func() {
		rep := diag.BuildReportMarkdown(a.shell.FeedbackCategory, a.shell.FeedbackText, a.currentDiagnosticEnv(), a.shell.FeedbackIncludeLogs)
		mygo.Clipboard.WriteText(rep)
		a.shell.FeedbackStatus = "已复制脱敏环境与诊断摘要到剪贴板"
	}

	a.shell.OnSubmitFeedback = func() {
		rep := diag.BuildReportMarkdown(a.shell.FeedbackCategory, a.shell.FeedbackText, a.currentDiagnosticEnv(), a.shell.FeedbackIncludeLogs)
		mygo.Clipboard.WriteText(rep)
		issueURL := diag.BuildGitHubIssueURL(updater.Repo, a.shell.FeedbackCategory, "", a.shell.FeedbackText, a.currentDiagnosticEnv(), a.shell.FeedbackIncludeLogs)
		if err := updater.OpenURL(issueURL); err != nil {
			a.shell.FeedbackStatus = "已复制报告，请手动打开 GitHub Issues 提交：" + err.Error()
			return
		}
		a.shell.FeedbackStatus = "已打开 GitHub 问题提交页面（同时已复制完整报告到剪贴板）"
	}

	a.shell.OnOpenUserGuide = func() {
		a.openOrSeedUserGuideDoc()
	}

	a.shell.OnOnboardingChoice = func(choice string) {
		markOnboardingSeen()
		a.shell.WelcomeOpen = false
		switch choice {
		case "guide":
			a.openOrSeedUserGuideDoc()
		case "tour":
			a.openOrSeedUserGuideDoc()
			if a.shell != nil {
				a.shell.StartTour()
			}
		case "demo":
			a.seedDemoExperimentAndPantry()
		case "new":
			a.openNewExperimentDialog("", a.shell.SelectedDate)
		case "existing":
			if a.shell.OnPickVaultPath != nil {
				if picked := a.shell.OnPickVaultPath(); picked != "" && a.shell.OnSwitchVaultPath != nil {
					a.shell.OnSwitchVaultPath(picked)
				}
			}
		}
	}

	if a.shouldShowWelcomeOnboarding() {
		a.shell.WelcomeOpen = true
	}
}

func (a *app) shouldShowWelcomeOnboarding() bool {
	if hasSeenOnboarding() || a.vault == nil || a.shell == nil {
		return false
	}
	for _, f := range a.shell.VaultFiles {
		if !f.IsTemplate && !f.IsAsset {
			return false
		}
	}
	return true
}

func (a *app) seedDemoExperimentAndPantry() {
	if a.vault == nil {
		return
	}
	demoSrc := "---\ntitle: L929 药物梯度 CCK-8 细胞活力测定（示范）\nscheduled_date: " + time.Now().Format("2006-01-02") + "\nstage: in_progress\n---\n\n" +
		"```asterlab\nkind: experiment\nid: exp-demo\nschema: 1\ntitle: L929 药物梯度 CCK-8 细胞活力测定（示范）\nobjective: 对比不同处理浓度下 L929 成纤维细胞培养 24h 的相对增殖活力（OD450）。\nexperimental_unit: well\n```\n\n" +
		"```asterlab\nkind: group_table\nid: gt-demo\nschema: 1\ntitle: 实验分组\nrows:\n  - id: g-ctrl\n    name: 溶剂对照组 (DMSO)\n    role: control\n    condition: 0.1% DMSO\n    independent_n: 3\n  - id: g-low\n    name: 低剂量处理组\n    role: treatment\n    condition: 10 uM\n    independent_n: 3\n  - id: g-high\n    name: 高剂量处理组\n    role: treatment\n    condition: 50 uM\n    independent_n: 3\n```\n\n" +
		"```asterlab\nkind: step\nid: step-01-seed\nschema: 1\ntitle: 01 细胞接种与梯度加药\nblocks:\n  - kind: prose\n    id: p-demo-1\n    schema: 1\n    body: 每孔接种 100 uL 细胞悬液（5000 cells/well），37°C 5% CO2 贴壁 12h 后按梯度加入受试物。\n  - kind: param_table\n    id: pt-demo-1\n    schema: 1\n    title: 关键培养参数\n    rows:\n      - key: temp\n        label: 培养温度\n        value: 37\n        unit: Cel\n      - key: co2\n        label: CO2 浓度\n        value: 5\n        unit: '%'\n  - kind: plate\n    id: pl-demo-1\n    schema: 1\n    title: 96 孔分组与读数排布\n    format: 96\n    groups:\n      g-ctrl: [A1, A2, A3]\n      g-low: [B1, B2, B3]\n      g-high: [C1, C2, C3]\n```\n\n" +
		"```asterlab\nkind: step\nid: step-02-read\nschema: 1\ntitle: 02 CCK-8 显色与 OD450 读板\nblocks:\n  - kind: timer\n    id: tm-demo-1\n    schema: 1\n    title: CCK-8 显色孵育\n    mode: countdown\n    status: idle\n    duration:\n      value: 60\n      unit: min\n  - kind: data_table\n    id: dt-demo-1\n    schema: 1\n    title: 酶标仪 OD450 读数\n    columns:\n      - key: well\n        label: 孔位\n        value_kind: text\n      - key: group\n        label: 分组\n        value_kind: text\n      - key: value\n        label: OD450\n        value_kind: number\n        unit: AU\n    rows:\n      - well: A1\n        group: 溶剂对照组 (DMSO)\n        value: 1.46\n      - well: A2\n        group: 溶剂对照组 (DMSO)\n        value: 1.52\n      - well: B1\n        group: 低剂量处理组\n        value: 1.08\n      - well: B2\n        group: 低剂量处理组\n        value: 1.12\n      - well: C1\n        group: 高剂量处理组\n        value: 0.44\n      - well: C2\n        group: 高剂量处理组\n        value: 0.48\n```\n"
	folder, err := a.vault.CreateFolder("", "示范实验项目")
	if err != nil {
		folder = ""
	}
	entry, cErr := a.vault.CreateExperiment(folder, "L929 CCK-8 细胞活力测定（示范）", time.Now().Format("2006-01-02"), demoSrc)
	if cErr != nil {
		log.Printf("seed demo experiment failed: %v", cErr)
		return
	}
	if ledger, err := a.localListPantry(); err == nil && len(ledger.Items) == 0 {
		s1, s2 := uint32(365), uint32(180)
		_, _ = a.localUpsertPantry(runs.PantryUpsert{
			Name: "CCK-8 细胞增殖检测试剂盒", Location: "4°C B1", PurchasedOn: time.Now().Format("2006-01-02"), ShelfLifeDays: &s1,
		})
		_, _ = a.localUpsertPantry(runs.PantryUpsert{
			Name: "DMSO 细胞级溶剂", Location: "常温避光 C2", PurchasedOn: time.Now().Format("2006-01-02"), ShelfLifeDays: &s2,
		})
		if a.tools != nil {
			a.tools.ReloadPantry()
		}
	}
	a.refreshVaultTreeAndCalendar()
	a.openVaultFileInActivePane(entry.RelPath, entry.Title)
	if a.shell != nil {
		a.shell.SetActiveTabMode(shell.ModeEdit)
	}
}

func (a *app) openExternalAstermlFile(filePath string) {
	absPath, err := filepath.Abs(strings.TrimSpace(filePath))
	if err != nil || absPath == "" {
		return
	}
	if _, err := os.Stat(absPath); err != nil {
		return
	}
	parent := filepath.Dir(absPath)
	vaultRoot := parent
	// If the file sits inside an existing subfolder of a Vault root (.asterlab marker), keep that root.
	cur := parent
	for i := 0; i < 4; i++ {
		if _, err := os.Stat(filepath.Join(cur, ".asterlab")); err == nil {
			vaultRoot = cur
			break
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	if a.vault == nil || !strings.EqualFold(filepath.Clean(a.vault.Root), filepath.Clean(vaultRoot)) {
		if err := a.initVaultWorkspace(vaultRoot); err != nil {
			log.Printf("open external .asterml initVaultWorkspace(%s): %v", vaultRoot, err)
			return
		}
		if a.vault != nil {
			saveLastVaultRoot(a.vault.Root)
		}
	}
	rel, err := filepath.Rel(a.vault.Root, absPath)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	rel = filepath.ToSlash(rel)
	title := strings.TrimSuffix(filepath.Base(absPath), filepath.Ext(absPath))
	a.refreshVaultTreeAndCalendar()
	a.openVaultFileInActivePane(rel, title)
}
