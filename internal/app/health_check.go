package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	modHealthKindMissingAddonList = "missing_addonlist"
	modHealthKindMissingFile      = "missing_file"
	modHealthKindDisabledOnly     = "disabled_only"
	modHealthKindUnrecorded       = "unrecorded"
	modHealthKindDuplicateEntry   = "duplicate_entry"
	modHealthKindInvalidVPK       = "invalid_vpk"
	modHealthKindOrphanMeta       = "orphan_meta"
	modHealthKindMissingMeta      = "missing_meta"
)

// ModHealthCheckOptions 控制体检范围。DeepScan 会逐个解析 VPK 目录，
// 对大型 Mod 库明显更慢，因此默认关闭。
type ModHealthCheckOptions struct {
	DeepScan bool `json:"deepScan"`
}

// ModHealthIssue 是体检发现的一条问题。
type ModHealthIssue struct {
	Kind     string `json:"kind"`
	Severity string `json:"severity"`
	Name     string `json:"name"`
	Path     string `json:"path,omitempty"`
	Location string `json:"location,omitempty"`
	Message  string `json:"message"`
}

// ModHealthReport 是一次体检的完整结果。
type ModHealthReport struct {
	TotalIssues int              `json:"totalIssues"`
	Counts      map[string]int   `json:"counts"`
	Issues      []ModHealthIssue `json:"issues"`
	DeepScanned bool             `json:"deepScanned"`
}

func modHealthSeverityRank(severity string) int {
	switch severity {
	case "critical":
		return 3
	case "warning":
		return 2
	default:
		return 1
	}
}

func (report *ModHealthReport) addIssue(kind string, severity string, name string, path string, location string, message string) {
	report.Issues = append(report.Issues, ModHealthIssue{
		Kind:     kind,
		Severity: severity,
		Name:     name,
		Path:     path,
		Location: location,
		Message:  message,
	})
}

// modHealthEntryExists 判断 addonlist 条目对应的文件是否仍在磁盘上，
// 同时覆盖 disabled 目录中的副本（例如用户刚刚把它移到 disabled）。
func modHealthEntryExists(rootDir string, entryName string) bool {
	name := strings.ReplaceAll(strings.TrimSpace(entryName), "/", string(filepath.Separator))
	name = strings.ReplaceAll(name, "\\", string(filepath.Separator))
	if name == "" {
		return false
	}
	if info, err := os.Stat(filepath.Join(rootDir, name)); err == nil && !info.IsDir() {
		return true
	}
	if info, err := os.Stat(filepath.Join(rootDir, "disabled", name)); err == nil && !info.IsDir() {
		return true
	}
	return false
}

// checkWorkshopMetaFiles 检查工坊伴随文件：孤立 .meta 与缺失 .meta。
// 缺失提示只在开启“工坊信息存储”时给出，避免关闭该功能后满屏误报。
func (a *App) checkWorkshopMetaFiles(rootDir string, report *ModHealthReport) {
	for _, dir := range []string{rootDir, filepath.Join(rootDir, "workshop")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".meta") {
				continue
			}
			base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
			metaPath := filepath.Join(dir, entry.Name())
			if info, statErr := os.Stat(filepath.Join(dir, base+".vpk")); statErr == nil && !info.IsDir() {
				continue
			}
			report.addIssue(modHealthKindOrphanMeta, "info", entry.Name(), metaPath, a.getLocationFromPath(metaPath),
				fmt.Sprintf("%s 没有对应的 VPK 文件：工坊信息已经用不上了，可以安全删除", entry.Name()))
		}
	}

	a.mu.RLock()
	metaEnabled := a.workshopMetaEnabled
	a.mu.RUnlock()
	if !metaEnabled {
		return
	}

	workshopDir := filepath.Join(rootDir, "workshop")
	entries, err := os.ReadDir(workshopDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".vpk") {
			continue
		}
		base := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		metaPath := filepath.Join(workshopDir, base+".meta")
		if info, statErr := os.Stat(metaPath); statErr == nil && !info.IsDir() {
			continue
		}
		report.addIssue(modHealthKindMissingMeta, "info", entry.Name(), filepath.Join(workshopDir, entry.Name()), "workshop",
			fmt.Sprintf("%s 缺少同名 .meta 文件：工坊标题、标签与更新时间可能丢失，重新下载或更新一次即可补回", entry.Name()))
	}
}

func (report *ModHealthReport) finalize() ModHealthReport {
	sort.SliceStable(report.Issues, func(i, j int) bool {
		left, right := report.Issues[i], report.Issues[j]
		if rankLeft, rankRight := modHealthSeverityRank(left.Severity), modHealthSeverityRank(right.Severity); rankLeft != rankRight {
			return rankLeft > rankRight
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Name < right.Name
	})

	counts := make(map[string]int, 4)
	for _, issue := range report.Issues {
		counts[issue.Kind]++
	}
	report.Counts = counts
	report.TotalIssues = len(report.Issues)
	if report.Issues == nil {
		report.Issues = []ModHealthIssue{}
	}
	return *report
}

// modHealthDisabledCandidate 返回条目在 disabled 目录中的对应路径。
func modHealthDisabledCandidate(rootDir string, entryName string) string {
	name := strings.ReplaceAll(strings.TrimSpace(entryName), "/", string(filepath.Separator))
	name = strings.ReplaceAll(name, "\\", string(filepath.Separator))
	return filepath.Join(rootDir, "disabled", name)
}

// RunModHealthCheck 对 addonlist.txt 与磁盘文件做一致性体检。
// 只读：不会修改 addonlist.txt 或任何 VPK 文件。
func (a *App) RunModHealthCheck(options ModHealthCheckOptions) (ModHealthReport, error) {
	report := ModHealthReport{DeepScanned: options.DeepScan}

	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModHealthReport{}, fmt.Errorf("未选择L4D2目录")
	}

	addonListPath, err := a.addonListPath()
	if err != nil {
		return ModHealthReport{}, err
	}
	if !fileExists(addonListPath) {
		report.addIssue(modHealthKindMissingAddonList, "warning", "addonlist.txt", addonListPath, "",
			"还没有 addonlist.txt：游戏尚未写入过开关状态，暂时无法比较磁盘文件与开关记录")
		return report.finalize(), nil
	}

	a.addonListGuardMu.Lock()
	list, _, err := a.readAddonList()
	a.addonListGuardMu.Unlock()
	if err != nil {
		return ModHealthReport{}, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	recorded := make(map[string]struct{}, len(list))
	duplicateReported := make(map[string]struct{}, 4)
	for _, item := range list {
		name := strings.TrimSpace(item.Name)
		key := normalizeAddonListKey(name)
		if key == "" {
			continue
		}
		if _, duplicate := recorded[key]; duplicate {
			if _, alreadyReported := duplicateReported[key]; alreadyReported {
				continue
			}
			duplicateReported[key] = struct{}{}
			report.addIssue(modHealthKindDuplicateEntry, "warning", name, "", "",
				fmt.Sprintf("addonlist.txt 中重复记录了 %s：游戏只会使用其中一条，开关可能与预期不一致", name))
			continue
		}
		recorded[key] = struct{}{}
	}

	// 1) 已记录的条目：文件是否还在（含 disabled 目录提示）。
	for _, item := range list {
		name := strings.TrimSpace(item.Name)
		key := normalizeAddonListKey(name)
		if key == "" {
			continue
		}
		if _, duplicate := duplicateReported[key]; duplicate {
			continue
		}

		candidate := filepath.Join(rootDir, strings.ReplaceAll(strings.ReplaceAll(name, "/", string(filepath.Separator)), "\\", string(filepath.Separator)))
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			continue
		}

		disabledCandidate := modHealthDisabledCandidate(rootDir, name)
		if info, statErr := os.Stat(disabledCandidate); statErr == nil && !info.IsDir() {
			report.addIssue(modHealthKindDisabledOnly, "warning", name, disabledCandidate, "disabled",
				fmt.Sprintf("%s 只存在于 disabled 目录，但 addonlist.txt 仍记录着它：游戏不会加载该文件", name))
			continue
		}

		report.addIssue(modHealthKindMissingFile, "critical", name, candidate, "",
			fmt.Sprintf("addonlist.txt 记录了 %s，但磁盘上找不到对应文件：游戏会忽略该条目", name))
	}

	// 2) 磁盘上的文件：是否已写入 addonlist（disabled 目录本身不参与游戏加载，跳过）。
	for _, diskPath := range collectConflictFilesystemPaths(rootDir) {
		location := a.getLocationFromPath(diskPath)
		if location == "disabled" {
			continue
		}
		key, keyErr := addonListKeyForManagedVPKPathFromRoot(rootDir, diskPath)
		if keyErr != nil {
			continue
		}
		if _, ok := recorded[key]; ok {
			continue
		}
		display, displayErr := addonListDisplayKeyForVPKPathFromRoot(rootDir, diskPath)
		if displayErr != nil || display == "" {
			display = filepath.Base(diskPath)
		}
		report.addIssue(modHealthKindUnrecorded, "info", display, diskPath, location,
			fmt.Sprintf("%s 没有写入 addonlist.txt：游戏内开关状态未记录，管理器无法判断它是否启用", display))
	}

	// 3) 深度扫描：逐个解析 VPK 目录，找出损坏文件。
	if options.DeepScan {
		for _, diskPath := range collectConflictFilesystemPaths(rootDir) {
			if location := a.getLocationFromPath(diskPath); location == "disabled" {
				continue
			}
			if _, parseErr := a.getConflictFileList(diskPath); parseErr != nil {
				report.addIssue(modHealthKindInvalidVPK, "critical", filepath.Base(diskPath), diskPath, a.getLocationFromPath(diskPath),
					fmt.Sprintf("%s 无法解析：文件可能已损坏或被截断（%v）", filepath.Base(diskPath), parseErr))
			}
		}
	}

	// 4) 工坊伴随文件：孤立 .meta 与缺失 .meta。
	a.checkWorkshopMetaFiles(rootDir, &report)

	// 5) 用户声明的依赖：依赖被关闭或依赖文件缺失。
	a.checkModDependencies(rootDir, addonListStateMap(list), &report)

	return report.finalize(), nil
}

// RemoveDuplicateAddonListEntries 清理 addonlist.txt 中重复的条目（保留第一条）。
// 返回被移除的重复条目数量；没有重复项时不写盘。
// SaveModHealthReport 把 Markdown 体检报告保存到配置目录的 reports 文件夹，
// 返回写入的完整路径（前端可以据此定位文件）。
func (a *App) SaveModHealthReport(markdown string) (string, error) {
	content := strings.TrimSpace(markdown)
	if content == "" {
		return "", fmt.Errorf("报告内容为空，无法保存")
	}

	a.ensureConfigPaths()
	if a.configDir == "" {
		return "", fmt.Errorf("未配置配置目录，无法保存报告")
	}
	reportDir := filepath.Join(a.configDir, "reports")
	if err := os.MkdirAll(reportDir, 0o755); err != nil {
		return "", fmt.Errorf("无法创建报告目录: %w", err)
	}

	stamp := time.Now().Format("20060102-150405")
	path := filepath.Join(reportDir, "mod-health-"+stamp+".md")
	// 同一秒内重复保存时追加序号，避免互相覆盖。
	for index := 1; fileExists(path); index++ {
		path = filepath.Join(reportDir, fmt.Sprintf("mod-health-%s-%d.md", stamp, index))
	}
	if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("无法写入报告: %w", err)
	}
	return path, nil
}

func (a *App) RemoveDuplicateAddonListEntries() (int, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	list, path, err := a.readAddonList()
	if err != nil {
		return 0, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	deduped := make([]AddonListItem, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	removed := 0
	for _, item := range list {
		key := normalizeAddonListKey(item.Name)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			removed++
			continue
		}
		seen[key] = struct{}{}
		deduped = append(deduped, item)
	}
	if removed == 0 {
		return 0, nil
	}

	if _, err := a.createAddonListFixBackupLocked(path); err != nil {
		return 0, err
	}
	if err := a.writeAddonList(path, deduped); err != nil {
		return 0, fmt.Errorf("无法写入 addonlist.txt: %w", err)
	}
	a.applyAddonListGameStates()
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return 0, err
	}
	return removed, nil
}

// createAddonListFixBackupLocked 在体检修复写盘前建立可恢复备份。
// 调用方必须持有 addonListGuardMu。
func (a *App) createAddonListFixBackupLocked(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("无法读取 addonlist.txt 以建立备份: %w", err)
	}
	backup, err := a.createAddonListBackupLocked("before-health-fix", content)
	if err != nil {
		return "", fmt.Errorf("无法建立修复前备份: %w", err)
	}
	return backup.Name, nil
}

// RemoveMissingFileAddonListEntries 删除 addonlist.txt 中已经找不到文件的条目。
// disabled 目录中的副本视为文件仍然存在；返回被移除的条目数量。
func (a *App) RemoveMissingFileAddonListEntries() (int, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return 0, fmt.Errorf("未选择L4D2目录")
	}

	list, path, err := a.readAddonList()
	if err != nil {
		return 0, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	kept := make([]AddonListItem, 0, len(list))
	removed := 0
	for _, item := range list {
		name := strings.TrimSpace(item.Name)
		if name != "" && !modHealthEntryExists(rootDir, name) {
			removed++
			continue
		}
		kept = append(kept, item)
	}
	if removed == 0 {
		return 0, nil
	}

	if _, err := a.createAddonListFixBackupLocked(path); err != nil {
		return 0, err
	}
	if err := a.writeAddonList(path, kept); err != nil {
		return 0, fmt.Errorf("无法写入 addonlist.txt: %w", err)
	}
	a.applyAddonListGameStates()
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return 0, err
	}
	return removed, nil
}
