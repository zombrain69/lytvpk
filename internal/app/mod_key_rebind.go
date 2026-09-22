package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

// mod_key_rebind.go：Mod 改名的"键迁移"。
//
// LytVPK 里所有本地记录（策略组 / 优先级分层 / 依赖 / 冲突忽略清单）都以
// addonlist 键为身份。文件一旦改名，键就变了——如果不跟着改绑，记录会变成
// 指向旧键的"悬空成员"：策略组里少一个成员、分层与忽略清单静默失效。
// 这里在重命名成功后把旧键统一迁移到新键。

// rebindModKeyOnRename 把本地记录里所有旧的 addonlist 键迁移到新键。
// 任何一个存储写入失败都不会影响其它存储，也不会回滚已经完成的重命名。
func (a *App) rebindModKeyOnRename(oldKey, newKey, displayName string) {
	oldKey = normalizeAddonListKey(oldKey)
	newKey = normalizeAddonListKey(newKey)
	if oldKey == "" || newKey == "" || oldKey == newKey {
		return
	}

	a.rebindStrategyGroupMembers(oldKey, newKey, displayName)
	a.rebindPriorityEntries(oldKey, newKey, displayName)
	a.rebindDependencyRecords(oldKey, newKey, displayName)
	a.rebindIgnoreRecords(oldKey, newKey, displayName)
}

func (a *App) rebindStrategyGroupMembers(oldKey, newKey, displayName string) {
	a.groupsMu.Lock()
	defer a.groupsMu.Unlock()

	store, err := a.readModStrategyGroupStore()
	if err != nil || len(store.Groups) == 0 {
		return
	}
	changed := false
	for groupIndex := range store.Groups {
		for memberIndex := range store.Groups[groupIndex].Members {
			member := &store.Groups[groupIndex].Members[memberIndex]
			if normalizeAddonListKey(member.Key) != oldKey {
				continue
			}
			member.Key = newKey
			if strings.TrimSpace(displayName) != "" {
				member.Name = displayName
			}
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := a.writeModStrategyGroupStore(store); err != nil {
		log.Printf("改名后同步策略组失败: %v", err)
	}
}

func (a *App) rebindPriorityEntries(oldKey, newKey, displayName string) {
	store, err := a.readModPriorityStore()
	if err != nil || len(store.Entries) == 0 {
		return
	}
	changed := false
	for index := range store.Entries {
		entry := &store.Entries[index]
		if normalizeAddonListKey(entry.Key) != oldKey {
			continue
		}
		entry.Key = newKey
		if strings.TrimSpace(displayName) != "" {
			entry.Name = displayName
		}
		changed = true
	}
	if !changed {
		return
	}
	if err := a.writeModPriorityStore(store); err != nil {
		log.Printf("改名后同步优先级分层失败: %v", err)
	}
}

func (a *App) rebindDependencyRecords(oldKey, newKey, displayName string) {
	store, err := a.readModDependencyStore()
	if err != nil || len(store.Records) == 0 {
		return
	}
	changed := false
	for index := range store.Records {
		record := &store.Records[index]
		if normalizeAddonListKey(record.Key) == oldKey {
			record.Key = newKey
			if strings.TrimSpace(displayName) != "" {
				record.Name = displayName
			}
			changed = true
		}
		for refIndex := range record.Dependencies {
			ref := &record.Dependencies[refIndex]
			if normalizeAddonListKey(ref.Key) != oldKey {
				continue
			}
			ref.Key = newKey
			if strings.TrimSpace(displayName) != "" {
				ref.Name = displayName
			}
			changed = true
		}
	}
	if !changed {
		return
	}
	if err := a.writeModDependencyStore(store); err != nil {
		log.Printf("改名后同步依赖记录失败: %v", err)
	}
}

func (a *App) rebindIgnoreRecords(oldKey, newKey, displayName string) {
	store, err := a.readModIgnoreStore()
	if err != nil || len(store.Records) == 0 {
		return
	}
	changed := false
	for index := range store.Records {
		record := &store.Records[index]
		if normalizeAddonListKey(record.Key) != oldKey {
			continue
		}
		record.Key = newKey
		if strings.TrimSpace(displayName) != "" {
			record.Name = displayName
		}
		changed = true
	}
	if !changed {
		return
	}
	if err := a.writeModIgnoreStore(store); err != nil {
		log.Printf("改名后同步冲突忽略清单失败: %v", err)
	}
}

// addonListKeyForPath 计算某个物理路径对应的 addonlist 键（失败返回空串）。
func (a *App) addonListKeyForPath(path string) string {
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" || strings.TrimSpace(path) == "" {
		return ""
	}
	key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, path)
	if err != nil {
		return ""
	}
	return key
}

// modKeyExistsInVault 判断某个 addonlist 键当前是否还能在已扫描列表里找到文件。
// 组应用前用它过滤"文件已被删除/移走"的成员，避免往 addonlist 写幽灵条目。
func (a *App) modKeyExistsInVault(key string) bool {
	target := normalizeAddonListKey(key)
	if target == "" {
		return false
	}
	rootDir := a.rootDirectorySnapshot()
	if rootDir == "" {
		return true // 没有根目录信息时不拦截，保持旧行为
	}
	found := false
	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		current, err := addonListKeyForManagedVPKPathFromRoot(rootDir, cache.File.Path)
		if err != nil {
			return true
		}
		if normalizeAddonListKey(current) == target {
			found = true
			return false
		}
		return true
	})
	if found {
		return true
	}
	// 缓存可能还没扫描到（例如测试夹具或刚移动完的文件），
	// 再用物理路径兜底：根目录、disabled、workshop 三个受管位置。
	for _, candidate := range []string{
		filepath.Join(rootDir, target),
		filepath.Join(rootDir, "disabled", target),
		filepath.Join(rootDir, "workshop", target),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}
