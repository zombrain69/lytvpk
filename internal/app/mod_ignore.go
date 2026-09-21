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

// 每 Mod 冲突忽略文件（对齐 FireAxe 的 ConflictIgnoringFiles，用 Go 重写）
//
// FireAxe 的判定是"内置忽略集合 ∪ 调用方传入的忽略集合 ∪ 该 Mod 自己的忽略集合"，
// 其中"自己的忽略集合"在扫描该 Mod 的 VPK 时生效：命中即视为该 Mod 不提供这个文件，
// 因此该 Mod 不会因为这条路径与别人重叠而被判定为冲突方。
//
// LytVPK 沿用同一语义，但把"该 Mod"落成 addonlist 归一化键（root 为文件名，
// 工坊为 workshop\<id>.vpk），并额外在结果里标注"因自身规则被跳过的重叠"，
// 让用户能解释"为什么刚才还在报冲突，现在不报了"。

// ModIgnoreRecord 是某个 Mod 自己的忽略清单。
type ModIgnoreRecord struct {
	Key       string   `json:"key"`
	Name      string   `json:"name,omitempty"`
	Files     []string `json:"files"`
	UpdatedAt string   `json:"updatedAt"`
}

type modIgnoreStore struct {
	Records []ModIgnoreRecord `json:"records"`
}

func (a *App) ensureIgnorePath() string {
	a.ensureConfigPaths()
	if a.ignorePath == "" && a.configDir != "" {
		a.ignorePath = filepath.Join(a.configDir, "ignore.json")
	}
	return a.ignorePath
}

func (a *App) readModIgnoreStore() (modIgnoreStore, error) {
	path := a.ensureIgnorePath()
	if path == "" {
		return modIgnoreStore{}, nil
	}
	var store modIgnoreStore
	if err := readJSONFile(path, &store); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return modIgnoreStore{}, nil
		}
		return modIgnoreStore{}, fmt.Errorf("无法读取单 Mod 忽略清单: %w", err)
	}
	return store, nil
}

func (a *App) writeModIgnoreStore(store modIgnoreStore) error {
	path := a.ensureIgnorePath()
	if path == "" {
		return fmt.Errorf("未配置配置目录，无法保存单 Mod 忽略清单")
	}
	if store.Records == nil {
		store.Records = []ModIgnoreRecord{}
	}
	a.backupLocalStoreFileIfNeeded(path)
	return writeJSONFile(a.configDir, path, store)
}

// normalizeModIgnoreRecord 归一化一条记录；键为空表示无效记录。
func normalizeModIgnoreRecord(record ModIgnoreRecord) (ModIgnoreRecord, bool) {
	key := normalizeAddonListKey(record.Key)
	if key == "" {
		return ModIgnoreRecord{}, false
	}
	record.Key = key
	record.Name = strings.TrimSpace(record.Name)
	record.Files = normalizeConflictIgnoreFileList(record.Files)
	return record, true
}

// ListModIgnoreRecords 返回全部单 Mod 忽略清单（按键升序）。
func (a *App) ListModIgnoreRecords() ([]ModIgnoreRecord, error) {
	a.ignoreMu.Lock()
	defer a.ignoreMu.Unlock()

	store, err := a.readModIgnoreStore()
	if err != nil {
		return nil, err
	}
	records := make([]ModIgnoreRecord, 0, len(store.Records))
	for _, record := range store.Records {
		normalized, ok := normalizeModIgnoreRecord(record)
		if !ok || len(normalized.Files) == 0 {
			continue
		}
		records = append(records, normalized)
	}
	sort.SliceStable(records, func(i, j int) bool { return records[i].Key < records[j].Key })
	return records, nil
}

// GetModIgnoreFiles 读取某个 Mod 自己的忽略清单；没有记录时返回空列表。
func (a *App) GetModIgnoreFiles(key string) ([]string, error) {
	normalizedKey := normalizeAddonListKey(key)
	if normalizedKey == "" {
		return nil, fmt.Errorf("缺少 Mod 标识，无法读取忽略清单")
	}
	a.ignoreMu.Lock()
	defer a.ignoreMu.Unlock()

	store, err := a.readModIgnoreStore()
	if err != nil {
		return nil, err
	}
	for _, record := range store.Records {
		normalized, ok := normalizeModIgnoreRecord(record)
		if !ok || normalized.Key != normalizedKey {
			continue
		}
		return append([]string(nil), normalized.Files...), nil
	}
	return []string{}, nil
}

// SetModIgnoreFiles 覆盖写入某个 Mod 自己的忽略清单；空清单等价于删除该记录。
func (a *App) SetModIgnoreFiles(key string, name string, paths []string) (ModIgnoreRecord, error) {
	normalizedKey := normalizeAddonListKey(key)
	if normalizedKey == "" {
		return ModIgnoreRecord{}, fmt.Errorf("缺少 Mod 标识，无法保存忽略清单")
	}
	files := normalizeConflictIgnoreFileList(paths)

	record := ModIgnoreRecord{
		Key:       normalizedKey,
		Name:      strings.TrimSpace(name),
		Files:     files,
		UpdatedAt: time.Now().Format(time.RFC3339),
	}

	a.ignoreMu.Lock()
	defer a.ignoreMu.Unlock()

	store, err := a.readModIgnoreStore()
	if err != nil {
		return ModIgnoreRecord{}, err
	}
	records := make([]ModIgnoreRecord, 0, len(store.Records)+1)
	for _, existing := range store.Records {
		normalized, ok := normalizeModIgnoreRecord(existing)
		if !ok {
			continue
		}
		if normalized.Key == normalizedKey {
			continue
		}
		records = append(records, normalized)
	}
	if len(files) > 0 {
		records = append(records, record)
	}
	if err := a.writeModIgnoreStore(modIgnoreStore{Records: records}); err != nil {
		return ModIgnoreRecord{}, fmt.Errorf("无法保存单 Mod 忽略清单: %w", err)
	}
	return record, nil
}

// DeleteModIgnoreFiles 删除某个 Mod 自己的忽略清单。
func (a *App) DeleteModIgnoreFiles(key string) error {
	normalizedKey := normalizeAddonListKey(key)
	if normalizedKey == "" {
		return fmt.Errorf("缺少 Mod 标识，无法删除忽略清单")
	}
	a.ignoreMu.Lock()
	defer a.ignoreMu.Unlock()

	store, err := a.readModIgnoreStore()
	if err != nil {
		return err
	}
	records := make([]ModIgnoreRecord, 0, len(store.Records))
	for _, record := range store.Records {
		normalized, ok := normalizeModIgnoreRecord(record)
		if !ok || normalized.Key == normalizedKey {
			continue
		}
		records = append(records, normalized)
	}
	return a.writeModIgnoreStore(modIgnoreStore{Records: records})
}

// modIgnoreSetsByPath 为一次冲突检测构建"VPK 完整路径 -> 该 Mod 自己的忽略集合"。
// 键为空或记录为空的 Mod 会被跳过；读取失败时降级为空表（不阻断既有分析）。
func (a *App) modIgnoreSetsByPath(rootDir string, vpkPaths []string) map[string]conflictIgnoreSet {
	if rootDir == "" || len(vpkPaths) == 0 {
		return nil
	}
	records, err := a.ListModIgnoreRecords()
	if err != nil || len(records) == 0 {
		return nil
	}
	byKey := make(map[string]conflictIgnoreSet, len(records))
	for _, record := range records {
		if len(record.Files) == 0 {
			continue
		}
		byKey[record.Key] = newConflictIgnoreSet(record.Files)
	}
	if len(byKey) == 0 {
		return nil
	}

	result := make(map[string]conflictIgnoreSet)
	for _, path := range vpkPaths {
		key, err := addonListKeyForManagedVPKPathFromRoot(rootDir, path)
		if err != nil {
			continue
		}
		if set, ok := byKey[key]; ok {
			result[filepath.Clean(path)] = set
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}
