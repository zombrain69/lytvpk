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

// 统一优先级模型（对齐 FireAxe 的深层理解，用 Go 重写）
//
//	addonlist.txt 的条目顺序仍然是游戏侧唯一权威：本文件只维护"意图 + 判定 + 排序"层，
//	不会让分层变成独立于 addonlist.txt 的平行真相。
//
//	base(mod)      = 显式分层（priority.json 已记录）否则 = 该 Mod 在 addonlist.txt 中的顺序号（0 基）
//	effective(mod) = min(base(mod), min{ 组权重 | 该 Mod 所属且显式设置了 Tier 的策略组 })
//
// 组权重取 min 而不是像 FireAxe 那样沿父子链累加：LytVPK 的策略组是可重叠的集合，
// 累加会让"同时属于多个组"的 Mod 反复漂移；取 min 保证组权重只能抬高优先级、
// 结果唯一且可解释。
//
// 未设置任何分层时 effective 恒等于顺序号，因此冲突判定/排序结果与本模型引入前逐字节一致。

const (
	prioritySourceTier  = "tier"
	prioritySourceGroup = "group"
	prioritySourceOrder = "order"
)

// ModPriorityEntry 是某个 Mod 的显式分层记录。Tier 越小越先加载，可为负。
type ModPriorityEntry struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	Tier      int    `json:"tier"`
	UpdatedAt string `json:"updatedAt"`
}

// ModEffectivePriority 是某个 Mod 的有效分层明细，供前端展示
// "优先级 #顺序（分层 T）"以及解释分层来源。
type ModEffectivePriority struct {
	Key     string `json:"key"`
	Name    string `json:"name"`
	Order   int    `json:"order"` // addonlist 顺序号（1 基）；0 表示未记录
	Known   bool   `json:"known"`
	Enabled bool   `json:"enabled"`
	// Tier 是显式分层，nil 表示未设置（此时 base = 顺序号）。
	Tier *int `json:"tier,omitempty"`
	// GroupTier 是所属策略组权重的最小值，nil 表示没有任何设置了权重的组。
	GroupTier *int `json:"groupTier,omitempty"`
	// Effective 是判定与排序共用的有效分层（0 基，越小越先加载）。
	Effective int    `json:"effective"`
	Source    string `json:"source"`
}

type modPriorityStore struct {
	Entries []ModPriorityEntry `json:"entries"`
}

// modPriorityLayers 是判定/排序共用的查询表：键统一为 normalizeAddonListKey。
type modPriorityLayers struct {
	own   map[string]int
	group map[string]int
}

func (l modPriorityLayers) isEmpty() bool {
	return len(l.own) == 0 && len(l.group) == 0
}

func (l modPriorityLayers) tierFor(key string) *int {
	if value, ok := l.own[key]; ok {
		value := value
		return &value
	}
	return nil
}

func (l modPriorityLayers) groupTierFor(key string) *int {
	if value, ok := l.group[key]; ok {
		value := value
		return &value
	}
	return nil
}

func (l modPriorityLayers) effective(key string, orderIndex int) int {
	return computeEffectivePriority(orderIndex, l.tierFor(key), l.groupTierFor(key))
}

// computeEffectivePriority 是有效分层的唯一计算入口。
func computeEffectivePriority(orderIndex int, ownTier *int, groupTier *int) int {
	base := orderIndex
	if ownTier != nil {
		base = *ownTier
	}
	if groupTier != nil && *groupTier < base {
		return *groupTier
	}
	return base
}

// effectivePrioritySource 解释有效分层来自显式分层、组权重还是顺序号。
func effectivePrioritySource(orderIndex int, ownTier *int, groupTier *int) string {
	base := orderIndex
	if ownTier != nil {
		base = *ownTier
	}
	if groupTier != nil && *groupTier < base {
		return prioritySourceGroup
	}
	if ownTier != nil {
		return prioritySourceTier
	}
	return prioritySourceOrder
}

func (a *App) ensurePriorityPath() string {
	a.ensureConfigPaths()
	if a.priorityPath == "" && a.configDir != "" {
		a.priorityPath = filepath.Join(a.configDir, "priority.json")
	}
	return a.priorityPath
}

func (a *App) readModPriorityStore() (modPriorityStore, error) {
	path := a.ensurePriorityPath()
	if path == "" {
		return modPriorityStore{}, nil
	}
	var store modPriorityStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modPriorityStore{}, nil
		}
		return modPriorityStore{}, fmt.Errorf("无法读取优先级分层: %w", err)
	}
	return store, nil
}

func (a *App) writeModPriorityStore(store modPriorityStore) error {
	path := a.ensurePriorityPath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存优先级分层")
	}
	if store.Entries == nil {
		store.Entries = []ModPriorityEntry{}
	}
	a.backupLocalStoreFileIfNeeded(path)
	return writeJSONFile(a.configDir, path, store)
}

// normalizeModPriorityEntry 把一条分层记录归一化；键为空表示记录无效。
func normalizeModPriorityEntry(entry ModPriorityEntry) (ModPriorityEntry, bool) {
	key := normalizeAddonListKey(entry.Key)
	if key == "" {
		return ModPriorityEntry{}, false
	}
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		name = key
	}
	entry.Key = key
	entry.Name = name
	return entry, true
}

// loadModPriorityLayers 构建判定/排序共用的分层查询表。
// 调用方要区分"记录损坏"与"尚未设置"时使用严格版本；冲突检测等只读路径使用 best-effort 版本。
func (a *App) loadModPriorityLayers() (modPriorityLayers, error) {
	layers := modPriorityLayers{}

	store, err := a.readModPriorityStore()
	if err != nil {
		return layers, err
	}
	if len(store.Entries) > 0 {
		layers.own = make(map[string]int, len(store.Entries))
		for _, entry := range store.Entries {
			normalized, ok := normalizeModPriorityEntry(entry)
			if !ok {
				continue
			}
			layers.own[normalized.Key] = normalized.Tier
		}
	}

	groups, err := a.readModStrategyGroupStore()
	if err != nil {
		return layers, err
	}
	for _, group := range groups.Groups {
		if group.Tier == nil {
			continue
		}
		if layers.group == nil {
			layers.group = make(map[string]int)
		}
		for _, member := range group.Members {
			key := normalizeAddonListKey(member.Key)
			if key == "" {
				continue
			}
			if current, exists := layers.group[key]; !exists || *group.Tier < current {
				layers.group[key] = *group.Tier
			}
		}
	}
	return layers, nil
}

// loadModPriorityLayersBestEffort 供冲突检测等只读路径使用：
// 分层记录损坏时退化为"未设置分层"，绝不阻断既有分析流程。
func (a *App) loadModPriorityLayersBestEffort() modPriorityLayers {
	layers, err := a.loadModPriorityLayers()
	if err != nil {
		return modPriorityLayers{}
	}
	return layers
}

// ListModPriorities 返回显式分层记录（按分层升序、键升序）。
func (a *App) ListModPriorities() ([]ModPriorityEntry, error) {
	a.priorityMu.Lock()
	defer a.priorityMu.Unlock()

	store, err := a.readModPriorityStore()
	if err != nil {
		return nil, err
	}
	entries := make([]ModPriorityEntry, 0, len(store.Entries))
	for _, entry := range store.Entries {
		normalized, ok := normalizeModPriorityEntry(entry)
		if !ok {
			continue
		}
		entries = append(entries, normalized)
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Tier != entries[j].Tier {
			return entries[i].Tier < entries[j].Tier
		}
		return entries[i].Key < entries[j].Key
	})
	return entries, nil
}

// SetModPriority 设置（或覆盖）某个 Mod 的显式分层。只写 priority.json，
// 不会改动 addonlist.txt —— 重排只发生在显式的 ApplyModPriorityLayers。
func (a *App) SetModPriority(key string, name string, tier int) (ModPriorityEntry, error) {
	normalizedKey := normalizeAddonListKey(key)
	if normalizedKey == "" {
		return ModPriorityEntry{}, fmt.Errorf("缺少 Mod 标识，无法设置分层")
	}
	entry := ModPriorityEntry{
		Key:       normalizedKey,
		Name:      strings.TrimSpace(name),
		Tier:      tier,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}
	if entry.Name == "" {
		entry.Name = normalizedKey
	}

	a.priorityMu.Lock()
	defer a.priorityMu.Unlock()

	store, err := a.readModPriorityStore()
	if err != nil {
		return ModPriorityEntry{}, err
	}
	replaced := false
	entries := make([]ModPriorityEntry, 0, len(store.Entries)+1)
	for _, existing := range store.Entries {
		current, ok := normalizeModPriorityEntry(existing)
		if !ok {
			continue
		}
		if current.Key == normalizedKey {
			entries = append(entries, entry)
			replaced = true
			continue
		}
		entries = append(entries, current)
	}
	if !replaced {
		entries = append(entries, entry)
	}
	if err := a.writeModPriorityStore(modPriorityStore{Entries: entries}); err != nil {
		return ModPriorityEntry{}, fmt.Errorf("无法保存优先级分层: %w", err)
	}
	return entry, nil
}

// ClearModPriority 删除某个 Mod 的显式分层，使其回到"顺序号 + 组权重"。
func (a *App) ClearModPriority(key string) error {
	normalizedKey := normalizeAddonListKey(key)
	if normalizedKey == "" {
		return fmt.Errorf("缺少 Mod 标识，无法清除分层")
	}

	a.priorityMu.Lock()
	defer a.priorityMu.Unlock()

	store, err := a.readModPriorityStore()
	if err != nil {
		return err
	}
	entries := make([]ModPriorityEntry, 0, len(store.Entries))
	found := false
	for _, existing := range store.Entries {
		current, ok := normalizeModPriorityEntry(existing)
		if !ok {
			continue
		}
		if current.Key == normalizedKey {
			found = true
			continue
		}
		entries = append(entries, current)
	}
	if !found {
		return nil
	}
	return a.writeModPriorityStore(modPriorityStore{Entries: entries})
}

// GetModPriorityPlan 返回每个 Mod 的有效分层明细：
// addonlist.txt 中的条目按顺序给出，另有显式分层但当前未记录的条目排在最后。
func (a *App) GetModPriorityPlan() ([]ModEffectivePriority, error) {
	a.addonListGuardMu.Lock()
	list, _, err := a.readAddonList()
	a.addonListGuardMu.Unlock()
	if err != nil {
		return nil, fmt.Errorf("无法读取 addonlist.txt: %w", err)
	}

	layers, err := a.loadModPriorityLayers()
	if err != nil {
		return nil, err
	}

	plan := make([]ModEffectivePriority, 0, len(list))
	seen := make(map[string]struct{}, len(list))
	for index, item := range list {
		key := normalizeAddonListKey(item.Name)
		if key == "" {
			continue
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}

		ownTier := layers.tierFor(key)
		groupTier := layers.groupTierFor(key)
		plan = append(plan, ModEffectivePriority{
			Key:       key,
			Name:      strings.TrimSpace(item.Name),
			Order:     index + 1,
			Known:     true,
			Enabled:   strings.TrimSpace(item.Value) == "1",
			Tier:      ownTier,
			GroupTier: groupTier,
			Effective: computeEffectivePriority(index, ownTier, groupTier),
			Source:    effectivePrioritySource(index, ownTier, groupTier),
		})
	}

	for key, tier := range layers.own {
		if _, exists := seen[key]; exists {
			continue
		}
		tierValue := tier
		groupTier := layers.groupTierFor(key)
		plan = append(plan, ModEffectivePriority{
			Key:       key,
			Name:      key,
			Order:     0,
			Known:     false,
			Enabled:   false,
			Tier:      &tierValue,
			GroupTier: groupTier,
			Effective: computeEffectivePriority(0, &tierValue, groupTier),
			Source:    effectivePrioritySource(0, &tierValue, groupTier),
		})
	}
	return plan, nil
}

// ApplyModPriorityLayers 是唯一允许按分层重排 addonlist.txt 的入口：
// 按有效分层升序稳定排序（同层内保持既有相对顺序）写回，并走既有编码保真 + 原子写 + 快照同步。
// 顺序没有变化时不写盘，保证"未设置分层时逐字节一致"。
func (a *App) ApplyModPriorityLayers() (AddonListLoadOrderPreview, error) {
	a.addonListGuardMu.Lock()
	defer a.addonListGuardMu.Unlock()

	list, path, err := a.readAddonList()
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	uniqueList, err := deduplicateAddonListItemsForLoadOrder(list)
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	layers, err := a.loadModPriorityLayers()
	if err != nil {
		return AddonListLoadOrderPreview{}, err
	}

	ordered := sortAddonListItemsByEffectiveLayer(uniqueList, layers)
	if addonListItemsEqualOrder(uniqueList, ordered) {
		return AddonListLoadOrderPreview{Entries: makeAddonListLoadOrderEntries(uniqueList)}, nil
	}
	if err := a.writeAddonList(path, ordered); err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	if err := a.syncManagedAddonListSnapshotLocked(path); err != nil {
		return AddonListLoadOrderPreview{}, err
	}
	return AddonListLoadOrderPreview{Entries: makeAddonListLoadOrderEntries(ordered)}, nil
}

func addonListItemsEqualOrder(left []AddonListItem, right []AddonListItem) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// sortAddonListItemsByEffectiveLayer 按有效分层升序稳定排序；同层内保持原有相对顺序。
func sortAddonListItemsByEffectiveLayer(list []AddonListItem, layers modPriorityLayers) []AddonListItem {
	type rankedItem struct {
		item      AddonListItem
		effective int
	}
	ranked := make([]rankedItem, 0, len(list))
	for index, item := range list {
		key := normalizeAddonListKey(item.Name)
		ranked = append(ranked, rankedItem{
			item:      item,
			effective: layers.effective(key, index),
		})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].effective < ranked[j].effective
	})
	ordered := make([]AddonListItem, 0, len(ranked))
	for _, entry := range ranked {
		ordered = append(ordered, entry.item)
	}
	return ordered
}
