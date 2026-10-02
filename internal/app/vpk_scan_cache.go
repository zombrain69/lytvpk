package app

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"vpk-manager/internal/parser"
)

// 持久化扫描缓存：把"每个 VPK 解析出来的元数据"存到配置目录，冷启动时先校验再用，
// 省掉 2904 个 VPK 的全量解析（真机 1–1.5s）。
//
// 失效判据刻意**完全复用内存缓存那一套**（size + mtime + 同名封面图 mtime + .meta mtime），
// 不发明新规则：命中就是"和刚才这份解析结果一致"，命中失败就老老实实重新解析。
// 解析结果还依赖二进制里内嵌的规则表（rules.json / entities.json 都是 go:embed），
// 所以再加一层 AppVersion + schemaVersion：换版本、换缓存格式一律整份作废。
//
// 降级：文件不存在 / JSON 坏掉 / 版本不符 / 写入失败 —— 全部静默退回"全量解析"，
// 绝不让缓存问题变成标签结果问题。
const (
	vpkScanCacheSchemaVersion = 1
	vpkScanCacheFileName      = "vpk_scan_cache.json"
)

// vpkScanCacheEntry 是单个 VPK 的缓存记录。时间统一存 UnixNano，0 表示零值时间。
type vpkScanCacheEntry struct {
	Path         string         `json:"path"`
	Size         int64          `json:"size"`
	ModTime      int64          `json:"modTime"`
	ImageModTime int64          `json:"imageModTime"`
	MetaModTime  int64          `json:"metaModTime"`
	File         parser.VPKFile `json:"file"`
}

type vpkScanCachePayload struct {
	SchemaVersion int                 `json:"schemaVersion"`
	AppVersion    string              `json:"appVersion"`
	SavedAt       string              `json:"savedAt,omitempty"`
	Entries       []vpkScanCacheEntry `json:"entries"`
}

func (a *App) vpkScanCachePath() string {
	if a == nil || a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, vpkScanCacheFileName)
}

func unixNanoOrZero(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UnixNano()
}

func timeFromUnixNano(value int64) time.Time {
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

/**
 * loadVPKScanCache 读取并校验缓存头。任何异常都返回 (nil,false)，调用方退回全量解析。
 * 返回的 map 以 VPK 绝对路径为键，方便逐文件 O(1) 取用。
 */
func (a *App) loadVPKScanCache() (map[string]vpkScanCacheEntry, bool) {
	path := a.vpkScanCachePath()
	if path == "" {
		return nil, false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var payload vpkScanCachePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		log.Printf("扫描缓存无法解析（将重新全量解析）: %v", err)
		return nil, false
	}
	if payload.SchemaVersion != vpkScanCacheSchemaVersion {
		log.Printf("扫描缓存格式版本不符（%d ≠ %d），重新全量解析", payload.SchemaVersion, vpkScanCacheSchemaVersion)
		return nil, false
	}
	if payload.AppVersion != AppVersion {
		log.Printf("扫描缓存来自其它版本（%s ≠ %s），重新全量解析", payload.AppVersion, AppVersion)
		return nil, false
	}
	index := make(map[string]vpkScanCacheEntry, len(payload.Entries))
	for _, entry := range payload.Entries {
		if entry.Path == "" {
			continue
		}
		index[entry.Path] = entry
	}
	return index, true
}

/** vpkScanCacheEntryMatches 与 processVPKFileWithCache 里的内存缓存判据逐字一致。 */
func vpkScanCacheEntryMatches(entry vpkScanCacheEntry, size int64, modTime, imageModTime, metaModTime time.Time) bool {
	return entry.Size == size &&
		entry.ModTime == unixNanoOrZero(modTime) &&
		entry.ImageModTime == unixNanoOrZero(imageModTime) &&
		entry.MetaModTime == unixNanoOrZero(metaModTime)
}

var vpkScanCacheSaveMu sync.Mutex

// saveVPKScanCacheNow 串行化写盘：小库走同步写、大库走后台写，两条路径共用同一个
// `.tmp` 文件名，不串行化会出现「两个 writer 同时写临时文件 / 互相 rename」的竞态
// （轻则 rename 失败退回全量解析，重则把半截 JSON 改名成正式缓存）。
func (a *App) saveVPKScanCacheNow() error {
	vpkScanCacheSaveMu.Lock()
	defer vpkScanCacheSaveMu.Unlock()
	return a.writeVPKScanCacheLocked()
}

// pathWithinRoot 判断路径是否在给定根目录下（Windows 路径大小写不敏感）。
func pathWithinRoot(rootDir, path string) bool {
	root := filepath.Clean(strings.TrimSpace(rootDir))
	target := filepath.Clean(strings.TrimSpace(path))
	if root == "" || target == "" {
		return false
	}
	root = strings.ToLower(root)
	target = strings.ToLower(target)
	return target == root || strings.HasPrefix(target, root+string(filepath.Separator))
}

// writeVPKScanCacheLocked 把当前内存缓存整体写成 JSON（先写临时文件再改名）。
// 调用方必须持有 vpkScanCacheSaveMu。
//
// 只覆盖**当前根目录**的条目：其它库（用户切过目录）的记录原样保留，
// 免得在两个库之间来回切换时每次都全量重解析。当前库删掉的条目会被真正丢弃
// （用户常在开着程序时删/移 Mod，缓存里不能留已删文件的"幽灵记录"）。
func (a *App) writeVPKScanCacheLocked() error {
	path := a.vpkScanCachePath()
	if path == "" {
		return nil
	}
	rootDir := a.rootDirectorySnapshot()
	entries := make([]vpkScanCacheEntry, 0, 4096)
	replaced := make(map[string]struct{}, 4096)
	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil || cache.File.Path == "" {
			return true
		}
		file := cache.File
		// 列表不返回预览图数据；缓存里也没必要存(单张几 MB)，详情/卡片会按需重新读。
		file.PreviewImage = ""
		entries = append(entries, vpkScanCacheEntry{
			Path:         cache.File.Path,
			Size:         cache.Size,
			ModTime:      unixNanoOrZero(cache.ModTime),
			ImageModTime: unixNanoOrZero(cache.ImageModTime),
			MetaModTime:  unixNanoOrZero(cache.MetaModTime),
			File:         file,
		})
		replaced[cache.File.Path] = struct{}{}
		return true
	})
	// 保留其它根目录下的旧条目（多库切换不互相冲掉）。
	if payload, ok := a.loadVPKScanCache(); ok {
		for entryPath, entry := range payload {
			if _, exists := replaced[entryPath]; exists {
				continue
			}
			if pathWithinRoot(rootDir, entryPath) {
				continue // 当前库里已经不存在的文件：不要留幽灵记录
			}
			entries = append(entries, entry)
		}
	}
	payload := vpkScanCachePayload{
		SchemaVersion: vpkScanCacheSchemaVersion,
		AppVersion:    AppVersion,
		SavedAt:       time.Now().Format(time.RFC3339),
		Entries:       entries,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0644); err != nil {
		return err
	}
	// Windows 上 os.Rename 不能覆盖已存在的文件：先删旧再改名。
	// 这一步失败也不影响正确性 —— 下次扫描会重新全量解析。
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(temp)
		return err
	}
	return os.Rename(temp, path)
}

// vpkScanCacheSyncWriteLimit 是"同步写"的条目上限。
//
// 大库（真机 2904 条 / 6MB）写盘要 100–150ms，放在"点禁用/删除/切目录"这条同步链路上
// 就是白白多等一次；小库（测试夹具、几十个 Mod）写盘是微秒级，直接同步写反而更简单、
// 也不会和调用方的临时目录清理抢文件。所以按条目数分档，而不是一律异步。
const vpkScanCacheSyncWriteLimit = 200

// ScanStats 是最近一次扫描的结果统计（诊断 / 测试 / 真机验收用）。
type ScanStats struct {
	Total          int
	Unchanged      int
	Reparsed       int
	CacheLoaded    bool
	CacheEntrySize int
}

func (a *App) setScanStats(snapshot ScanStats) {
	a.scanStatsMu.Lock()
	a.scanStats = snapshot
	a.scanStatsMu.Unlock()
}

/** GetScanStats 返回最近一次扫描的统计（只读快照）。 */
func (a *App) GetScanStats() ScanStats {
	a.scanStatsMu.Lock()
	defer a.scanStatsMu.Unlock()
	return a.scanStats
}

// saveVPKScanCacheAsync 写缓存：大库后台写，小库同步写。
func (a *App) saveVPKScanCacheAsync() {
	if a.vpkCacheEntryCount() <= vpkScanCacheSyncWriteLimit {
		if err := a.saveVPKScanCacheNow(); err != nil {
			log.Printf("写入扫描缓存失败（下次仍会全量解析）: %v", err)
		}
		return
	}
	go func() {
		if err := a.saveVPKScanCacheNow(); err != nil {
			log.Printf("写入扫描缓存失败（下次仍会全量解析）: %v", err)
		}
	}()
}

func (a *App) vpkCacheEntryCount() int {
	count := 0
	a.vpkCache.Range(func(_ any, _ any) bool {
		count++
		return true
	})
	return count
}
