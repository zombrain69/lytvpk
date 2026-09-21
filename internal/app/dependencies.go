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
	modHealthKindDependencyMissing  = "dependency_missing"
	modHealthKindDependencyDisabled = "dependency_disabled"
)

// ModDependencyRef 指向一个依赖项，按 addonlist 键匹配。
type ModDependencyRef struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ModDependencyRecord 记录某个 Mod 声明的依赖。依赖由用户声明，
// LytVPK 不会自动推断。
type ModDependencyRecord struct {
	Key          string             `json:"key"`
	Name         string             `json:"name"`
	Dependencies []ModDependencyRef `json:"dependencies"`
	UpdatedAt    string             `json:"updatedAt"`
}

// ModDependencyEnableResult 描述一次“启用依赖”的结果。
type ModDependencyEnableResult struct {
	MasterName     string   `json:"masterName"`
	MasterCount    int      `json:"masterCount"`
	Enabled        []string `json:"enabled"`
	AlreadyEnabled []string `json:"alreadyEnabled"`
	Missing        []string `json:"missing"`
}

type modDependencyStore struct {
	Records []ModDependencyRecord `json:"records"`
}

func (a *App) ensureDependenciesPath() string {
	a.ensureConfigPaths()
	if a.dependenciesPath == "" && a.configDir != "" {
		a.dependenciesPath = filepath.Join(a.configDir, "dependencies.json")
	}
	return a.dependenciesPath
}

func (a *App) readModDependencyStore() (modDependencyStore, error) {
	path := a.ensureDependenciesPath()
	if path == "" {
		return modDependencyStore{}, fmt.Errorf("未配置配置目录，无法读写依赖")
	}
	var store modDependencyStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modDependencyStore{}, nil
		}
		return modDependencyStore{}, fmt.Errorf("无法读取依赖列表: %w", err)
	}
	return store, nil
}

func (a *App) writeModDependencyStore(store modDependencyStore) error {
	path := a.ensureDependenciesPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存依赖")
	}
	if store.Records == nil {
		store.Records = []ModDependencyRecord{}
	}
	return writeJSONFile(a.configDir, path, store)
}

// ListModDependencies 返回已声明的依赖记录。
func (a *App) ListModDependencies() ([]ModDependencyRecord, error) {
	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()

	store, err := a.readModDependencyStore()
	if err != nil {
		return nil, err
	}
	return append([]ModDependencyRecord(nil), store.Records...), nil
}

// SetModDependencies 覆盖某个 Mod 的依赖声明；依赖为空表示删除该记录。
func (a *App) SetModDependencies(targetPath string, dependencyPaths []string) (ModDependencyRecord, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyRecord{}, fmt.Errorf("未选择L4D2目录")
	}
	target, err := modStrategyGroupMemberFromPath(rootDir, targetPath)
	if err != nil {
		return ModDependencyRecord{}, fmt.Errorf("无法识别主 Mod: %w", err)
	}

	dependencies := make([]ModDependencyRef, 0, len(dependencyPaths))
	seen := map[string]bool{target.Key: true}
	for _, rawPath := range dependencyPaths {
		if strings.TrimSpace(rawPath) == "" {
			continue
		}
		member, memberErr := modStrategyGroupMemberFromPath(rootDir, rawPath)
		if memberErr != nil {
			continue
		}
		if seen[member.Key] {
			continue
		}
		seen[member.Key] = true
		dependencies = append(dependencies, ModDependencyRef{Key: member.Key, Name: member.Name})
	}

	record := ModDependencyRecord{
		Key:          target.Key,
		Name:         target.Name,
		Dependencies: dependencies,
		UpdatedAt:    time.Now().Format(time.RFC3339),
	}

	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()
	store, err := a.readModDependencyStore()
	if err != nil {
		return ModDependencyRecord{}, err
	}

	next := make([]ModDependencyRecord, 0, len(store.Records)+1)
	replaced := false
	for _, existing := range store.Records {
		if existing.Key == record.Key {
			replaced = true
			continue
		}
		next = append(next, existing)
	}
	if len(dependencies) > 0 {
		next = append(next, record)
	}
	if !replaced && len(dependencies) == 0 {
		// 之前本来就没有记录：什么都不用写。
		return record, nil
	}
	store.Records = next
	if err := a.writeModDependencyStore(store); err != nil {
		return ModDependencyRecord{}, fmt.Errorf("无法保存依赖: %w", err)
	}
	return record, nil
}

type modDependencyPlan struct {
	results map[string]*ModDependencyEnableResult
	enable  []ModDependencyRef
}

// buildModDependencyPlan 计算需要开启的依赖。
// onlyMasterKey 非空时只处理该主 Mod；requireMasterEnabled 为真时跳过已关闭的主 Mod。
func buildModDependencyPlan(records []ModDependencyRecord, states map[string]bool, rootDir string, onlyMasterKey string, requireMasterEnabled bool) modDependencyPlan {
	plan := modDependencyPlan{results: make(map[string]*ModDependencyEnableResult, len(records))}
	if len(records) == 0 {
		return plan
	}
	queued := make(map[string]bool, 8)

	for _, record := range records {
		if onlyMasterKey != "" && record.Key != onlyMasterKey {
			continue
		}
		if requireMasterEnabled {
			if enabled, known := states[record.Key]; known && !enabled {
				continue
			}
		}

		result := &ModDependencyEnableResult{MasterName: record.Name, MasterCount: 1}
		for _, dependency := range record.Dependencies {
			if !modHealthEntryExists(rootDir, dependency.Name) {
				result.Missing = append(result.Missing, dependency.Name)
				continue
			}
			if enabled, known := states[dependency.Key]; known && enabled {
				result.AlreadyEnabled = append(result.AlreadyEnabled, dependency.Name)
				continue
			}
			result.Enabled = append(result.Enabled, dependency.Name)
			if !queued[dependency.Key] {
				queued[dependency.Key] = true
				plan.enable = append(plan.enable, dependency)
			}
		}
		plan.results[record.Key] = result
	}
	return plan
}

// applyModDependencyEnablement 把计划中的依赖写回 addonlist.txt（未记录的条目按插入位置补写）。
func (a *App) applyModDependencyEnablement(plan modDependencyPlan) error {
	if len(plan.enable) == 0 {
		return nil
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	doc, err := a.readAddonListDocument()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("无法读取 addonlist.txt: %w", err)
		}
		path, pathErr := a.addonListPath()
		if pathErr != nil {
			return pathErr
		}
		doc = addonListDocument{path: path, content: "\"AddonList\"\n{\n}\n", encoding: addonListEncodingUTF8}
	}

	placement := normalizeAddonListUnrecordedPlacement(a.unrecordedModLoadOrderPlacement)
	updated := doc.content
	changed := false
	for _, ref := range plan.enable {
		var replaced bool
		updated, replaced, err = replaceAddonListValueWithPlacementAndName(updated, ref.Key, ref.Name, "1", placement)
		if err != nil {
			return fmt.Errorf("无法更新 addonlist.txt 条目 %s: %w", ref.Key, err)
		}
		changed = changed || replaced
	}
	if !changed {
		return nil
	}
	if err := a.writeAddonListDocument(doc, updated); err != nil {
		return fmt.Errorf("无法写入 addonlist.txt: %w", err)
	}
	a.applyAddonListGameStates()
	if err := a.syncManagedAddonListSnapshotLocked(doc.path); err != nil {
		return err
	}
	return nil
}

// EnableModDependencies 开启某个主 Mod 的全部依赖（文件缺失的依赖会被跳过并报告）。
// target 可以是 VPK 路径（来自选择）或 addonlist 键（来自依赖列表）。
func (a *App) EnableModDependencies(targetPath string) (ModDependencyEnableResult, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyEnableResult{}, fmt.Errorf("未选择L4D2目录")
	}
	target, err := resolveModDependencyTarget(rootDir, targetPath)
	if err != nil {
		return ModDependencyEnableResult{}, fmt.Errorf("无法识别主 Mod: %w", err)
	}

	records, err := a.ListModDependencies()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}

	plan := buildModDependencyPlan(records, states, rootDir, target.Key, false)
	result, ok := plan.results[target.Key]
	if !ok {
		return ModDependencyEnableResult{}, fmt.Errorf("%s 还没有声明依赖", target.Name)
	}
	if err := a.applyModDependencyEnablement(plan); err != nil {
		return ModDependencyEnableResult{}, err
	}
	return *result, nil
}

// DeleteModDependencies 删除某个主 Mod 的依赖声明。
func (a *App) DeleteModDependencies(target string) error {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return fmt.Errorf("未选择L4D2目录")
	}
	resolved, err := resolveModDependencyTarget(rootDir, target)
	if err != nil {
		return err
	}

	a.dependenciesMu.Lock()
	defer a.dependenciesMu.Unlock()
	store, err := a.readModDependencyStore()
	if err != nil {
		return err
	}
	next := make([]ModDependencyRecord, 0, len(store.Records))
	found := false
	for _, record := range store.Records {
		if record.Key == resolved.Key {
			found = true
			continue
		}
		next = append(next, record)
	}
	if !found {
		return fmt.Errorf("%s 还没有声明依赖", resolved.Name)
	}
	store.Records = next
	return a.writeModDependencyStore(store)
}

// resolveModDependencyTarget 接受路径或 addonlist 键，统一解析成主 Mod 标识。
func resolveModDependencyTarget(rootDir string, input string) (ModStrategyGroupMember, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return ModStrategyGroupMember{}, fmt.Errorf("缺少主 Mod 标识")
	}
	if member, err := modStrategyGroupMemberFromPath(rootDir, input); err == nil {
		return member, nil
	}
	key := normalizeAddonListKey(input)
	if key == "" {
		return ModStrategyGroupMember{}, fmt.Errorf("无法识别主 Mod: %s", input)
	}
	return ModStrategyGroupMember{Key: key, Name: input}, nil
}

// EnableAllMissingModDependencies 为所有“处于开启状态”的主 Mod 补齐依赖。
func (a *App) EnableAllMissingModDependencies() (ModDependencyEnableResult, error) {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModDependencyEnableResult{}, fmt.Errorf("未选择L4D2目录")
	}

	records, err := a.ListModDependencies()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}
	states, err := a.modStrategyGroupStates()
	if err != nil {
		return ModDependencyEnableResult{}, err
	}

	plan := buildModDependencyPlan(records, states, rootDir, "", true)
	combined := ModDependencyEnableResult{}
	masterKeys := make([]string, 0, len(plan.results))
	for key := range plan.results {
		masterKeys = append(masterKeys, key)
	}
	sort.Strings(masterKeys)
	for _, key := range masterKeys {
		result := plan.results[key]
		if len(result.Enabled) == 0 {
			continue
		}
		combined.MasterCount++
		combined.Enabled = append(combined.Enabled, result.Enabled...)
		combined.AlreadyEnabled = append(combined.AlreadyEnabled, result.AlreadyEnabled...)
		combined.Missing = append(combined.Missing, result.Missing...)
	}
	if err := a.applyModDependencyEnablement(plan); err != nil {
		return ModDependencyEnableResult{}, err
	}
	return combined, nil
}

// checkModDependencies 在体检里提示“依赖被关闭 / 依赖文件缺失”。
// 只检查处于开启状态（或状态未记录）的主 Mod。
func (a *App) checkModDependencies(rootDir string, states map[string]bool, report *ModHealthReport) {
	records, err := a.ListModDependencies()
	if err != nil || len(records) == 0 {
		return
	}
	for _, record := range records {
		if enabled, known := states[record.Key]; known && !enabled {
			continue
		}
		for _, dependency := range record.Dependencies {
			if !modHealthEntryExists(rootDir, dependency.Name) {
				report.addIssue(modHealthKindDependencyMissing, "warning", record.Name, "", "",
					fmt.Sprintf("%s 依赖的 %s 在磁盘上找不到：请重新下载该依赖，或移除这条依赖声明", record.Name, dependency.Name))
				continue
			}
			if enabled, known := states[dependency.Key]; known && !enabled {
				report.addIssue(modHealthKindDependencyDisabled, "warning", record.Name, "", "",
					fmt.Sprintf("%s 已开启，但依赖 %s 处于关闭状态：可以在体检里一键启用依赖", record.Name, dependency.Name))
			}
		}
	}
}
