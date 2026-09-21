package app

import (
	"errors"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	modStrategyGroupAll          = "all"
	modStrategyGroupOff          = "off"
	modStrategyGroupSingle       = "single"
	modStrategyGroupSingleRandom = "single_random"
)

// ModStrategyGroup 是一组 Mod 与它们的启用策略。
// 与 FireAxe 不同，这里不隐式约束成员的开关：策略只在使用"应用"动作时生效。
type ModStrategyGroup struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Strategy    string `json:"strategy"`
	// Enforce 打开后，手动开关组内成员时会按策略自动联动其它成员。
	// 默认关闭：策略组默认只作为显式动作使用。
	Enforce   bool                     `json:"enforce"`
	Members   []ModStrategyGroupMember `json:"members"`
	CreatedAt string                   `json:"createdAt"`
	UpdatedAt string                   `json:"updatedAt"`
}

// ModStrategyGroupMember 记录单个成员：Key 用于匹配 addonlist 条目，
// Name 是写入 addonlist 时的显示名（保留原始大小写）。
type ModStrategyGroupMember struct {
	Key  string `json:"key"`
	Name string `json:"name"`
}

// ModStrategyGroupApplyOptions 允许单次应用覆盖策略与指定保留成员。
type ModStrategyGroupApplyOptions struct {
	Strategy string `json:"strategy"`
	PickKey  string `json:"pickKey"`
}

type ModStrategyGroupApplyResult struct {
	GroupID    string   `json:"groupId"`
	GroupName  string   `json:"groupName"`
	Strategy   string   `json:"strategy"`
	Enabled    []string `json:"enabled"`
	Disabled   []string `json:"disabled"`
	PickedName string   `json:"pickedName"`
}

type modStrategyGroupStore struct {
	Groups []ModStrategyGroup `json:"groups"`
}

func normalizeModStrategyGroupStrategy(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case modStrategyGroupAll:
		return modStrategyGroupAll, nil
	case modStrategyGroupOff:
		return modStrategyGroupOff, nil
	case modStrategyGroupSingle:
		return modStrategyGroupSingle, nil
	case modStrategyGroupSingleRandom:
		return modStrategyGroupSingleRandom, nil
	default:
		return "", fmt.Errorf("不支持的策略组类型: %s", value)
	}
}

func (a *App) ensureGroupsPath() string {
	a.ensureConfigPaths()
	if a.groupsPath == "" && a.configDir != "" {
		a.groupsPath = filepath.Join(a.configDir, "groups.json")
	}
	return a.groupsPath
}

func (a *App) readModStrategyGroupStore() (modStrategyGroupStore, error) {
	path := a.ensureGroupsPath()
	if path == "" {
		return modStrategyGroupStore{}, fmt.Errorf("未配置配置目录，无法读写策略组")
	}
	var store modStrategyGroupStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modStrategyGroupStore{}, nil
		}
		return modStrategyGroupStore{}, fmt.Errorf("无法读取策略组列表: %w", err)
	}
	return store, nil
}

func (a *App) writeModStrategyGroupStore(store modStrategyGroupStore) error {
	path := a.ensureGroupsPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存策略组")
	}
	if store.Groups == nil {
		store.Groups = []ModStrategyGroup{}
	}
	return writeJSONFile(a.configDir, path, store)
}

// ListModStrategyGroups 返回已保存的策略组，顺序与保存顺序一致。
func (a *App) ListModStrategyGroups() ([]ModStrategyGroup, error) {
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()

	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return nil, err
	}
	return append([]ModStrategyGroup(nil), store.Groups...), nil
}

// CaptureModStrategyGroup 用选中的 Mod 路径建立策略组。
func (a *App) CaptureModStrategyGroup(name string, description string, strategy string, memberPaths []string) (ModStrategyGroup, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return ModStrategyGroup{}, fmt.Errorf("策略组名称不能为空")
	}
	normalizedStrategy, err := normalizeModStrategyGroupStrategy(strategy)
	if err != nil {
		return ModStrategyGroup{}, err
	}

	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return ModStrategyGroup{}, fmt.Errorf("未选择L4D2目录")
	}

	members := make([]ModStrategyGroupMember, 0, len(memberPaths))
	seen := make(map[string]struct{}, len(memberPaths))
	for _, rawPath := range memberPaths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		member, memberErr := modStrategyGroupMemberFromPath(rootDir, path)
		if memberErr != nil {
			continue
		}
		if _, duplicate := seen[member.Key]; duplicate {
			continue
		}
		seen[member.Key] = struct{}{}
		members = append(members, member)
	}
	if len(members) == 0 {
		return ModStrategyGroup{}, fmt.Errorf("请至少选择一个有效的 Mod（只能选择当前 addons 目录内的 VPK）")
	}

	now := time.Now().Format(time.RFC3339)
	group := ModStrategyGroup{
		ID:          newLocalRecordID(),
		Name:        name,
		Description: strings.TrimSpace(description),
		Strategy:    normalizedStrategy,
		Members:     members,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	store.Groups = append(store.Groups, group)
	if err := a.writeModStrategyGroupStore(store); err != nil {
		return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
	}
	return group, nil
}

// DeleteModStrategyGroup 从策略组库中移除指定组。
func (a *App) DeleteModStrategyGroup(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return fmt.Errorf("缺少策略组 ID")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return err
	}

	remaining := make([]ModStrategyGroup, 0, len(store.Groups))
	found := false
	for _, group := range store.Groups {
		if group.ID == id {
			found = true
			continue
		}
		remaining = append(remaining, group)
	}
	if !found {
		return fmt.Errorf("策略组不存在: %s", id)
	}
	store.Groups = remaining
	return a.writeModStrategyGroupStore(store)
}

// SetModStrategyGroupEnforcement 打开/关闭某个策略组的自动联动。
func (a *App) SetModStrategyGroupEnforcement(id string, enforced bool) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for index := range store.Groups {
		if store.Groups[index].ID != id {
			continue
		}
		store.Groups[index].Enforce = enforced
		store.Groups[index].UpdatedAt = time.Now().Format(time.RFC3339)
		if err := a.writeModStrategyGroupStore(store); err != nil {
			return ModStrategyGroup{}, fmt.Errorf("无法保存策略组: %w", err)
		}
		return store.Groups[index], nil
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// listEnforcingStrategyGroups 返回开启了自动联动的策略组（按保存顺序）。
func (a *App) listEnforcingStrategyGroups() ([]ModStrategyGroup, error) {
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()

	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return nil, err
	}
	groups := make([]ModStrategyGroup, 0, len(store.Groups))
	for _, group := range store.Groups {
		if group.Enforce {
			groups = append(groups, group)
		}
	}
	return groups, nil
}

// strategyGroupEnforcementTargets 计算一次手动开关触发的联动目标。
// 第二个返回值为 false 表示这次操作不该联动（未开启联动、策略为 off，
// 或在单选组里关闭成员）。
func strategyGroupEnforcementTargets(group ModStrategyGroup, toggledKey string, enabled bool) (map[string]bool, bool) {
	if !group.Enforce || !modStrategyGroupHasMember(group, toggledKey) {
		return nil, false
	}

	targets := make(map[string]bool, len(group.Members))
	switch group.Strategy {
	case modStrategyGroupSingle, modStrategyGroupSingleRandom:
		if !enabled {
			// 单选组里关闭成员不影响其它成员。
			return nil, false
		}
		for _, member := range group.Members {
			targets[member.Key] = member.Key == toggledKey
		}
		return targets, true
	case modStrategyGroupAll:
		for _, member := range group.Members {
			targets[member.Key] = enabled
		}
		return targets, true
	default:
		return nil, false
	}
}

// strategyGroupMemberLoadable 判断成员文件是否位于 addons 根目录或 workshop 下。
// disabled 目录里的副本不会被游戏加载，因此联动时跳过。
func strategyGroupMemberLoadable(rootDir string, name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	relative := strings.ReplaceAll(strings.ReplaceAll(name, "/", string(filepath.Separator)), "\\", string(filepath.Separator))
	if info, err := os.Stat(filepath.Join(rootDir, relative)); err == nil && !info.IsDir() {
		return true
	}
	return false
}

// applyStrategyGroupEnforcementLocked 把与本次手动开关同组的成员一并改写。
// 只应用第一个命中的联动组（按保存顺序），不做跨组级联，避免相互冲突与死循环。
// 返回真正被改动的"其它成员"数量。调用方必须持有 addonListGuardMu。
func (a *App) applyStrategyGroupEnforcementLocked(content *string, toggledKey string, enabled bool) (int, error) {
	groups, err := a.listEnforcingStrategyGroups()
	if err != nil {
		log.Printf("读取策略组失败，跳过自动联动: %v", err)
		return 0, nil
	}
	if len(groups) == 0 {
		return 0, nil
	}
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return 0, nil
	}

	var targets map[string]bool
	var matched ModStrategyGroup
	for _, group := range groups {
		if candidate, applies := strategyGroupEnforcementTargets(group, toggledKey, enabled); applies {
			targets = candidate
			matched = group
			break
		}
	}
	if targets == nil {
		return 0, nil
	}

	states := addonListStateMap(parseAddonListItems(*content))
	placement := normalizeAddonListUnrecordedPlacement(a.unrecordedModLoadOrderPlacement)
	changed := 0
	for _, member := range matched.Members {
		if member.Key == toggledKey {
			continue
		}
		target, ok := targets[member.Key]
		if !ok || !strategyGroupMemberLoadable(rootDir, member.Name) {
			continue
		}
		if current, known := states[member.Key]; known && current == target {
			continue
		}

		value := "0"
		if target {
			value = "1"
		}
		displayName := strings.TrimSpace(member.Name)
		if displayName == "" {
			displayName = member.Key
		}

		var updated string
		if target {
			updated, _, err = replaceAddonListValueWithPlacementAndName(*content, member.Key, displayName, value, placement)
		} else {
			updated, _, err = replaceAddonListValueWithName(*content, member.Key, displayName, value)
		}
		if err != nil {
			return changed, fmt.Errorf("无法更新 addonlist.txt 条目 %s: %w", member.Key, err)
		}
		*content = updated
		changed++
	}
	return changed, nil
}

func (a *App) findModStrategyGroup(id string) (ModStrategyGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return ModStrategyGroup{}, fmt.Errorf("缺少策略组 ID")
	}

	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()
	store, err := a.readModStrategyGroupStore()
	if err != nil {
		return ModStrategyGroup{}, err
	}
	for _, group := range store.Groups {
		if group.ID == id {
			return group, nil
		}
	}
	return ModStrategyGroup{}, fmt.Errorf("策略组不存在: %s", id)
}

// ApplyModStrategyGroup 按策略改写组内成员的开关，不改动其它 Mod 也不重排顺序。
func (a *App) ApplyModStrategyGroup(id string, options ModStrategyGroupApplyOptions) (ModStrategyGroupApplyResult, error) {
	group, err := a.findModStrategyGroup(id)
	if err != nil {
		return ModStrategyGroupApplyResult{}, err
	}

	strategy := group.Strategy
	if override := strings.TrimSpace(options.Strategy); override != "" {
		normalized, normalizeErr := normalizeModStrategyGroupStrategy(override)
		if normalizeErr != nil {
			return ModStrategyGroupApplyResult{}, normalizeErr
		}
		strategy = normalized
	}

	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	doc, err := a.readAddonListDocument()
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return ModStrategyGroupApplyResult{}, fmt.Errorf("无法读取 addonlist.txt: %w", err)
		}
		path, pathErr := a.addonListPath()
		if pathErr != nil {
			return ModStrategyGroupApplyResult{}, pathErr
		}
		doc = addonListDocument{path: path, content: "\"AddonList\"\n{\n}\n", encoding: addonListEncodingUTF8}
	}

	targets, pickedKey, err := decideModStrategyGroupTargets(group, strategy, options.PickKey, doc.content)
	if err != nil {
		return ModStrategyGroupApplyResult{}, err
	}

	result := ModStrategyGroupApplyResult{GroupID: group.ID, GroupName: group.Name, Strategy: strategy}
	updated := doc.content
	changed := false
	placement := normalizeAddonListUnrecordedPlacement(a.unrecordedModLoadOrderPlacement)
	for _, member := range group.Members {
		enabled := targets[member.Key]
		value := "0"
		if enabled {
			value = "1"
		}
		displayName := strings.TrimSpace(member.Name)
		if displayName == "" {
			displayName = member.Key
		}

		var replaced bool
		if enabled {
			// 与 SetVPKGameEnabled 一致：开启未记录条目时按配置的插入位置补写。
			updated, replaced, err = replaceAddonListValueWithPlacementAndName(updated, member.Key, displayName, value, placement)
		} else {
			updated, replaced, err = replaceAddonListValueWithName(updated, member.Key, displayName, value)
		}
		if err != nil {
			return ModStrategyGroupApplyResult{}, fmt.Errorf("无法更新 addonlist.txt 条目 %s: %w", member.Key, err)
		}
		changed = changed || replaced

		if enabled {
			result.Enabled = append(result.Enabled, displayName)
		} else {
			result.Disabled = append(result.Disabled, displayName)
		}
		if pickedKey != "" && member.Key == pickedKey {
			result.PickedName = displayName
		}
	}

	if !changed {
		return result, nil
	}
	if err := a.writeAddonListDocument(doc, updated); err != nil {
		return ModStrategyGroupApplyResult{}, fmt.Errorf("无法写入 addonlist.txt: %w", err)
	}
	a.applyAddonListGameStates()
	if err := a.syncManagedAddonListSnapshotLocked(doc.path); err != nil {
		return ModStrategyGroupApplyResult{}, err
	}
	return result, nil
}

// modStrategyGroupStates 返回当前 addonlist 的开关映射（键为归一化条目名）。
func (a *App) modStrategyGroupStates() (map[string]bool, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	list, _, err := a.readAddonList()
	if err != nil {
		return nil, err
	}
	return addonListStateMap(list), nil
}

func modStrategyGroupMemberFromPath(rootDir string, filePath string) (ModStrategyGroupMember, error) {
	clean := filepath.Clean(filePath)
	key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, clean)
	if err != nil {
		return ModStrategyGroupMember{}, err
	}
	display, err := addonListDisplayKeyForVPKPathFromRoot(rootDir, clean)
	if err != nil {
		return ModStrategyGroupMember{}, err
	}
	if strings.HasPrefix(strings.ToLower(display), "disabled\\") {
		display = display[len("disabled\\"):]
	}
	if !strings.HasSuffix(strings.ToLower(display), ".vpk") {
		return ModStrategyGroupMember{}, fmt.Errorf("不是 VPK 文件: %s", filePath)
	}
	return ModStrategyGroupMember{Key: key, Name: display}, nil
}

func modStrategyGroupHasMember(group ModStrategyGroup, key string) bool {
	for _, member := range group.Members {
		if member.Key == key {
			return true
		}
	}
	return false
}

func decideModStrategyGroupTargets(group ModStrategyGroup, strategy string, pickKey string, content string) (map[string]bool, string, error) {
	if len(group.Members) == 0 {
		return nil, "", fmt.Errorf("策略组没有任何成员")
	}

	targets := make(map[string]bool, len(group.Members))
	switch strategy {
	case modStrategyGroupAll:
		for _, member := range group.Members {
			targets[member.Key] = true
		}
		return targets, "", nil
	case modStrategyGroupOff:
		for _, member := range group.Members {
			targets[member.Key] = false
		}
		return targets, "", nil
	}

	pickedKey := ""
	if strategy == modStrategyGroupSingle {
		pickedKey = normalizeAddonListKey(pickKey)
		if pickedKey != "" && !modStrategyGroupHasMember(group, pickedKey) {
			return nil, "", fmt.Errorf("指定保留的成员不在策略组中: %s", strings.TrimSpace(pickKey))
		}
	}
	if pickedKey == "" {
		if strategy == modStrategyGroupSingleRandom {
			pickedKey = group.Members[rand.IntN(len(group.Members))].Key
		} else {
			// single：优先保留当前已启用的成员，其次取第一个成员。
			states := addonListStateMap(parseAddonListItems(content))
			for _, member := range group.Members {
				if enabled, ok := states[member.Key]; ok && enabled {
					pickedKey = member.Key
					break
				}
			}
			if pickedKey == "" {
				pickedKey = group.Members[0].Key
			}
		}
	}

	for _, member := range group.Members {
		targets[member.Key] = member.Key == pickedKey
	}
	return targets, pickedKey, nil
}
