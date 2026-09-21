package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// modEnableProfileFileExtension 是方案导出文件的默认后缀。
const modEnableProfileFileExtension = ".l4d2profile.json"

// ModEnableProfile 是一份可保存、可切换、可分享的启用方案。
// Entries 的顺序就是目标加载顺序，与 addonlist.txt 的条目顺序一致。
type ModEnableProfile struct {
	ID          string                  `json:"id"`
	Name        string                  `json:"name"`
	Description string                  `json:"description,omitempty"`
	CreatedAt   string                  `json:"createdAt"`
	UpdatedAt   string                  `json:"updatedAt"`
	Entries     []ModEnableProfileEntry `json:"entries"`
	// IncludesAutomation 为真时，下面的 Groups / Dependencies 会随方案一起恢复。
	IncludesAutomation bool                  `json:"includesAutomation,omitempty"`
	Groups             []ModStrategyGroup    `json:"groups,omitempty"`
	Dependencies       []ModDependencyRecord `json:"dependencies,omitempty"`
}

// ModEnableProfileEntry 记录单个 addonlist 条目的目标开关。
// Name 保留 addonlist.txt 中的写法：根目录 Mod 是文件名，
// workshop Mod 是 workshop\<id>.vpk。
type ModEnableProfileEntry struct {
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
}

// ModEnableProfileApplyResult 描述一次方案应用的影响范围。
type ModEnableProfileApplyResult struct {
	ProfileID    string `json:"profileId"`
	ProfileName  string `json:"profileName"`
	AppliedCount int    `json:"appliedCount"`
	AddedCount   int    `json:"addedCount"`
	KeptCount    int    `json:"keptCount"`
	BackupName   string `json:"backupName"`
	// RestoredGroups / RestoredDependencies 只在方案包含自动化快照时大于 0。
	RestoredGroups       int `json:"restoredGroups"`
	RestoredDependencies int `json:"restoredDependencies"`
}

type modEnableProfileStore struct {
	Profiles []ModEnableProfile `json:"profiles"`
}

func (a *App) ensureProfilesPath() string {
	a.ensureConfigPaths()
	if a.profilesPath == "" && a.configDir != "" {
		a.profilesPath = filepath.Join(a.configDir, "profiles.json")
	}
	return a.profilesPath
}

func (a *App) readModEnableProfileStore() (modEnableProfileStore, error) {
	path := a.ensureProfilesPath()
	if path == "" {
		return modEnableProfileStore{}, fmt.Errorf("未配置配置目录，无法读写方案")
	}
	var store modEnableProfileStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modEnableProfileStore{}, nil
		}
		return modEnableProfileStore{}, fmt.Errorf("无法读取方案列表: %w", err)
	}
	return store, nil
}

func (a *App) writeModEnableProfileStore(store modEnableProfileStore) error {
	path := a.ensureProfilesPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存方案")
	}
	if store.Profiles == nil {
		store.Profiles = []ModEnableProfile{}
	}
	return writeJSONFile(a.configDir, path, store)
}

// ListModEnableProfiles 返回已保存的方案，顺序与保存顺序一致。
func (a *App) ListModEnableProfiles() ([]ModEnableProfile, error) {
	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()

	store, err := a.readModEnableProfileStore()
	if err != nil {
		return nil, err
	}
	return append([]ModEnableProfile(nil), store.Profiles...), nil
}

// CaptureModEnableProfile 把当前 addonlist.txt 的开关与顺序保存为方案。
func (a *App) CaptureModEnableProfile(name string, description string) (ModEnableProfile, error) {
	return a.CaptureModEnableProfileWithAutomation(name, description, false)
}

// CaptureModEnableProfileWithAutomation 额外把策略组与依赖声明一起快照进方案。
func (a *App) CaptureModEnableProfileWithAutomation(name string, description string, includeAutomation bool) (ModEnableProfile, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ModEnableProfile{}, fmt.Errorf("方案名称不能为空")
	}

	a.addonListGuardMu.Lock()
	list, _, err := a.readAddonList()
	a.addonListGuardMu.Unlock()
	if err != nil {
		return ModEnableProfile{}, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	entries := make([]ModEnableProfileEntry, 0, len(list))
	for _, item := range list {
		entryName := strings.TrimSpace(item.Name)
		if entryName == "" {
			continue
		}
		entries = append(entries, ModEnableProfileEntry{
			Name:    entryName,
			Enabled: strings.TrimSpace(item.Value) == "1",
		})
	}
	if len(entries) == 0 {
		return ModEnableProfile{}, fmt.Errorf("addonlist.txt 中没有任何有效条目")
	}

	now := time.Now().Format(time.RFC3339)
	profile := ModEnableProfile{
		ID:          newLocalRecordID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		CreatedAt:   now,
		UpdatedAt:   now,
		Entries:     entries,
	}
	if includeAutomation {
		groups, groupErr := a.ListModStrategyGroups()
		if groupErr != nil {
			return ModEnableProfile{}, groupErr
		}
		dependencies, dependencyErr := a.ListModDependencies()
		if dependencyErr != nil {
			return ModEnableProfile{}, dependencyErr
		}
		profile.IncludesAutomation = true
		profile.Groups = groups
		profile.Dependencies = dependencies
	}

	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	store, err := a.readModEnableProfileStore()
	if err != nil {
		return ModEnableProfile{}, err
	}
	store.Profiles = append(store.Profiles, profile)
	if err := a.writeModEnableProfileStore(store); err != nil {
		return ModEnableProfile{}, fmt.Errorf("无法保存方案: %w", err)
	}
	return profile, nil
}

// DeleteModEnableProfile 从方案库中移除指定方案。
func (a *App) DeleteModEnableProfile(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("缺少方案 ID")
	}

	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	store, err := a.readModEnableProfileStore()
	if err != nil {
		return err
	}

	remaining := make([]ModEnableProfile, 0, len(store.Profiles))
	found := false
	for _, profile := range store.Profiles {
		if profile.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, profile)
	}
	if !found {
		return fmt.Errorf("方案不存在: %s", id)
	}
	store.Profiles = remaining
	return a.writeModEnableProfileStore(store)
}

func (a *App) findModEnableProfile(id string) (ModEnableProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModEnableProfile{}, fmt.Errorf("缺少方案 ID")
	}

	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	store, err := a.readModEnableProfileStore()
	if err != nil {
		return ModEnableProfile{}, err
	}
	for _, profile := range store.Profiles {
		if profile.ID == id {
			return profile, nil
		}
	}
	return ModEnableProfile{}, fmt.Errorf("方案不存在: %s", id)
}

// ApplyModEnableProfile 应用方案：先备份当前 addonlist.txt，再按方案的
// 顺序与开关重写，并同步受保护快照与缓存状态。
func (a *App) ApplyModEnableProfile(id string) (ModEnableProfileApplyResult, error) {
	profile, err := a.findModEnableProfile(id)
	if err != nil {
		return ModEnableProfileApplyResult{}, err
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()
	return a.applyModEnableProfileLocked(profile)
}

func (a *App) applyModEnableProfileLocked(profile ModEnableProfile) (ModEnableProfileApplyResult, error) {
	result := ModEnableProfileApplyResult{ProfileID: profile.ID, ProfileName: profile.Name}

	list, path, err := a.readAddonList()
	if err != nil {
		return result, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	currentByKey := make(map[string]AddonListItem, len(list))
	for _, item := range list {
		if key := normalizeAddonListKey(item.Name); key != "" {
			currentByKey[key] = item
		}
	}

	ordered := make([]AddonListItem, 0, len(profile.Entries)+len(list))
	seen := make(map[string]struct{}, len(profile.Entries)+len(list))
	for _, entry := range profile.Entries {
		key := normalizeAddonListKey(entry.Name)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		name := strings.TrimSpace(entry.Name)
		if existing, ok := currentByKey[key]; ok {
			// 保留磁盘上的原始拼写，避免改变游戏识别的条目名。
			name = existing.Name
			result.AppliedCount++
		} else {
			result.AddedCount++
		}
		value := "0"
		if entry.Enabled {
			value = "1"
		}
		ordered = append(ordered, AddonListItem{Name: name, Value: value})
	}
	for _, item := range list {
		key := normalizeAddonListKey(item.Name)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		// 方案保存后新增的 Mod：保留原开关，排在方案条目之后。
		ordered = append(ordered, item)
		result.KeptCount++
	}

	if content, readErr := os.ReadFile(path); readErr == nil {
		backup, backupErr := a.createAddonListBackupLocked("before-profile-apply", content)
		if backupErr != nil {
			return result, fmt.Errorf("无法建立应用前备份: %w", backupErr)
		}
		result.BackupName = backup.Name
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return result, fmt.Errorf("无法读取 addonlist.txt: %w", readErr)
	}

	if err := a.writeAddonList(path, ordered); err != nil {
		return result, fmt.Errorf("无法写入 addonlist.txt: %w", err)
	}
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return result, err
	}
	a.applyAddonListGameStates()

	if profile.IncludesAutomation {
		// 快照里的策略组与依赖整体替换本地记录；不合并，保证"应用方案"就是恢复那套状态。
		a.groupsMu.Lock()
		if err := a.writeModStrategyGroupStore(modStrategyGroupStore{Groups: profile.Groups}); err != nil {
			a.groupsMu.Unlock()
			return result, fmt.Errorf("无法恢复策略组: %w", err)
		}
		a.groupsMu.Unlock()

		a.dependenciesMu.Lock()
		if err := a.writeModDependencyStore(modDependencyStore{Records: profile.Dependencies}); err != nil {
			a.dependenciesMu.Unlock()
			return result, fmt.Errorf("无法恢复依赖声明: %w", err)
		}
		a.dependenciesMu.Unlock()

		result.RestoredGroups = len(profile.Groups)
		result.RestoredDependencies = len(profile.Dependencies)
	}
	return result, nil
}

// ExportModEnableProfileToFile 把方案写入指定路径；路径没有 .json 后缀时会补上。
func (a *App) ExportModEnableProfileToFile(id string, targetPath string) error {
	profile, err := a.findModEnableProfile(id)
	if err != nil {
		return err
	}

	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return fmt.Errorf("导出路径不能为空")
	}
	targetPath = filepath.Clean(targetPath)
	if !strings.HasSuffix(strings.ToLower(targetPath), ".json") {
		targetPath += modEnableProfileFileExtension
	}
	if err := writeJSONFile(filepath.Dir(targetPath), targetPath, profile); err != nil {
		return fmt.Errorf("无法导出方案: %w", err)
	}
	return nil
}

// ImportModEnableProfileFromFile 读取方案文件并保存为本地方案，始终分配新的 ID。
func (a *App) ImportModEnableProfileFromFile(sourcePath string) (ModEnableProfile, error) {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return ModEnableProfile{}, fmt.Errorf("请选择要导入的方案文件")
	}

	var profile ModEnableProfile
	if err := readJSONFile(filepath.Clean(sourcePath), &profile); err != nil {
		return ModEnableProfile{}, fmt.Errorf("无法读取方案文件: %w", err)
	}
	normalized, err := normalizeModEnableProfile(profile)
	if err != nil {
		return ModEnableProfile{}, err
	}
	normalized.ID = newLocalRecordID()
	now := time.Now().Format(time.RFC3339)
	normalized.CreatedAt = now
	normalized.UpdatedAt = now

	a.profilesMu.Lock()
	defer a.profilesMu.Unlock()
	store, err := a.readModEnableProfileStore()
	if err != nil {
		return ModEnableProfile{}, err
	}
	store.Profiles = append(store.Profiles, normalized)
	if err := a.writeModEnableProfileStore(store); err != nil {
		return ModEnableProfile{}, fmt.Errorf("无法保存导入的方案: %w", err)
	}
	return normalized, nil
}

func normalizeModEnableProfile(profile ModEnableProfile) (ModEnableProfile, error) {
	name := strings.TrimSpace(profile.Name)
	if name == "" {
		return ModEnableProfile{}, fmt.Errorf("方案文件缺少名称")
	}

	entries := make([]ModEnableProfileEntry, 0, len(profile.Entries))
	seen := make(map[string]struct{}, len(profile.Entries))
	for _, entry := range profile.Entries {
		key := normalizeAddonListKey(entry.Name)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		entries = append(entries, ModEnableProfileEntry{
			Name:    strings.TrimSpace(entry.Name),
			Enabled: entry.Enabled,
		})
	}
	if len(entries) == 0 {
		return ModEnableProfile{}, fmt.Errorf("方案文件没有任何有效条目")
	}

	profile.Name = name
	profile.Description = strings.TrimSpace(profile.Description)
	profile.Entries = entries

	// 自动化快照做一次轻量校验：策略非法的组、缺成员的组、没有依赖项的记录都丢弃。
	if profile.IncludesAutomation {
		groups := make([]ModStrategyGroup, 0, len(profile.Groups))
		for _, group := range profile.Groups {
			strategy, strategyErr := normalizeModStrategyGroupStrategy(group.Strategy)
			if strategyErr != nil || strings.TrimSpace(group.Name) == "" || len(group.Members) == 0 {
				continue
			}
			group.Strategy = strategy
			group.ID = newLocalRecordID()
			groups = append(groups, group)
		}
		dependencies := make([]ModDependencyRecord, 0, len(profile.Dependencies))
		for _, record := range profile.Dependencies {
			if strings.TrimSpace(record.Key) == "" || len(record.Dependencies) == 0 {
				continue
			}
			dependencies = append(dependencies, record)
		}
		profile.Groups = groups
		profile.Dependencies = dependencies
	}
	return profile, nil
}

// ExportModEnableProfile 弹出保存对话框并导出方案；用户取消时返回空路径。
func (a *App) ExportModEnableProfile(id string) (string, error) {
	profile, err := a.findModEnableProfile(id)
	if err != nil {
		return "", err
	}
	if a.ctx == nil {
		return "", fmt.Errorf("应用尚未就绪，无法打开保存对话框")
	}

	targetPath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "导出 Mod 启用方案",
		DefaultFilename: sanitizeModEnableProfileFileName(profile.Name) + modEnableProfileFileExtension,
		Filters: []runtime.FileFilter{
			{DisplayName: "LytVPK 启用方案 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(targetPath) == "" {
		return "", nil
	}
	if err := a.ExportModEnableProfileToFile(id, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// ImportModEnableProfile 弹出文件对话框并导入方案；用户取消时返回零值方案
// （ID 为空），前端据此判断取消。
func (a *App) ImportModEnableProfile() (ModEnableProfile, error) {
	if a.ctx == nil {
		return ModEnableProfile{}, fmt.Errorf("应用尚未就绪，无法打开文件对话框")
	}

	sourcePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "导入 Mod 启用方案",
		Filters: []runtime.FileFilter{
			{DisplayName: "LytVPK 启用方案 (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil {
		return ModEnableProfile{}, err
	}
	if strings.TrimSpace(sourcePath) == "" {
		return ModEnableProfile{}, nil
	}
	return a.ImportModEnableProfileFromFile(sourcePath)
}

func sanitizeModEnableProfileFileName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "mod-profile"
	}
	replacer := strings.NewReplacer(
		"\\", "_", "/", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_",
	)
	name = strings.TrimSpace(replacer.Replace(name))
	if name == "" {
		return "mod-profile"
	}
	return name
}
