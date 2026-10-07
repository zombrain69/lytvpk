package app

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"vpk-manager/internal/parser"
)

// 套件命名空间内的标签继承（W4 通道 5）。
//
// 现实场景：一个套件常拆成"本体 + 参数包 + 贴图包"。参数包落在
// `materials/models/<作者>/<套件>/…`，路径里看不出武器型号；但同命名空间下的本体
// 往往已经带上了「AK47」这类精确标签。把命名空间的共同标签补给缺标签的成员，
// 就能把整套模块绑到同一个实体上。
//
// 安全边界（对话硬约束"可以多标、不能少标"的直接体现）：
//   - **只加不删**：只给缺该标签的成员补，绝不修改或删除已有标签；
//   - **只看特征标签**：只有 ≥60% 成员都带的标签才算这个命名空间的特征，
//     避免同一命名空间里两个不同武器的包互相污染；
//   - **有上限**：单个成员最多补 suiteInheritanceTagCap 个继承标签，防止堆标签。
const (
	suiteInheritanceMinMembers = 3
	suiteInheritanceMinRatio   = 0.6
	suiteInheritanceTagCap     = 20
)

// suiteInheritanceStore 把"套件继承"的结果持久化（只增不减）。
//
// 为什么需要：继承是唯一的**跨 Mod 推断**通道，靠 60% 阈值算出来。用户经常装/删/改 Mod 时，
// 同套件成员一变，阈值就可能翻转，于是"参数包昨天有 AK47、今天没了"。落盘成
// `命名空间 → 标签集合` 之后，扫描时用**并集**：成员变化只会补，不会撤。
// 命名空间整体消失（套件卸载）才清理条目；要重置直接删
// `%APPDATA%\LytVPK\suite_inheritance.json`。
type suiteInheritanceStore struct {
	Version    int                 `json:"version"`
	UpdatedAt  string              `json:"updatedAt,omitempty"`
	Namespaces map[string][]string `json:"namespaces"`
}

const suiteInheritanceStoreVersion = 1

func (a *App) suiteInheritanceStorePath() string {
	a.mu.RLock()
	dir := a.configDir
	a.mu.RUnlock()
	if strings.TrimSpace(dir) == "" {
		return ""
	}
	return filepath.Join(dir, "suite_inheritance.json")
}

func (a *App) loadSuiteInheritanceStore() *suiteInheritanceStore {
	store := &suiteInheritanceStore{
		Version:    suiteInheritanceStoreVersion,
		Namespaces: make(map[string][]string),
	}
	path := a.suiteInheritanceStorePath()
	if path == "" {
		return store
	}
	// 首次运行 / 文件损坏：从空开始（并集语义只会"少继承"，不会少标，安全）。
	if err := readJSONFile(path, store); err != nil {
		store.Namespaces = make(map[string][]string)
		return store
	}
	if store.Namespaces == nil {
		store.Namespaces = make(map[string][]string)
	}
	return store
}

func (a *App) saveSuiteInheritanceStore(store *suiteInheritanceStore) {
	path := a.suiteInheritanceStorePath()
	if path == "" || store == nil {
		return
	}
	store.Version = suiteInheritanceStoreVersion
	store.UpdatedAt = time.Now().Format(time.RFC3339)
	if err := writeJSONFile(filepath.Dir(path), path, store); err != nil {
		log.Printf("保存套件继承快照失败: %v", err)
	}
}

// ResetSuiteInheritanceSnapshot 删除套件继承快照（设置页的"手动重置"入口）。
// 返回删除前快照里的命名空间数量；文件不存在时返回 0。删除后下一轮扫描会按当前
// 成员重新推导（并集语义：重置只会少继承，不会少标）。
func (a *App) ResetSuiteInheritanceSnapshot() (int, error) {
	path := a.suiteInheritanceStorePath()
	if path == "" {
		return 0, fmt.Errorf("配置目录不可用，无法重置套件继承快照")
	}
	store := a.loadSuiteInheritanceStore()
	count := len(store.Namespaces)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return 0, fmt.Errorf("删除套件继承快照失败: %w", err)
	}
	return count, nil
}

// mergeSuiteInheritanceTags 把当轮算出的特征标签并入已持久化的集合：并集、去重、按小写排序。
// 返回 (合并结果, 是否发生变化)。"只增不减"是"经常增删 Mod 时标签不抖"的关键。
func mergeSuiteInheritanceTags(stored []string, computed []string) ([]string, bool) {
	merged := make([]string, 0, len(stored)+len(computed))
	seen := make(map[string]struct{}, len(stored)+len(computed))
	for _, tag := range append(append([]string{}, stored...), computed...) {
		trimmed := strings.TrimSpace(tag)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		merged = append(merged, trimmed)
	}
	sort.Slice(merged, func(i, j int) bool {
		return strings.ToLower(merged[i]) < strings.ToLower(merged[j])
	})
	changed := len(merged) != len(stored)
	return merged, changed
}

// applySuiteTagInheritance 在扫描结果上做一次命名空间标签继承，返回补了多少个标签。
func (a *App) applySuiteTagInheritance() int {
	a.mu.RLock()
	rootDir := a.rootDir
	a.mu.RUnlock()
	if strings.TrimSpace(rootDir) == "" {
		return 0
	}

	type member struct {
		key  string
		file *parser.VPKFile
	}
	groups := make(map[string][]member)
	a.vpkCache.Range(func(_ any, value any) bool {
		cache, ok := value.(*VPKFileCache)
		if !ok || cache == nil {
			return true
		}
		for _, root := range cache.File.StructureResourceRoots {
			normalized := strings.ToLower(strings.TrimSpace(root))
			if normalized == "" {
				continue
			}
			groups[normalized] = append(groups[normalized], member{key: cache.File.Path, file: &cache.File})
		}
		return true
	})

	// 命名空间按名字排序：map 迭代顺序随机，直接遍历会让"成员上限"这种截断产生
	// 不确定结果（真机表现为两次导出的标签数不一样）。
	roots := make([]string, 0, len(groups))
	for root := range groups {
		roots = append(roots, root)
	}
	sort.Strings(roots)

	// 套件继承快照：命名空间 → 已继承过的标签（只增不减）。
	store := a.loadSuiteInheritanceStore()
	storeDirty := false
	for root := range store.Namespaces {
		if _, alive := groups[root]; !alive {
			// 套件整体卸载才清理；成员暂时变少（<3）时保留，等成员回来标签直接生效。
			delete(store.Namespaces, root)
			storeDirty = true
		}
	}

	// 先用**扫描结果的快照**算频次：继承不能在命名空间之间级联，
	// 否则处理顺序会改变最终标签集合。
	type assignment struct {
		member *parser.VPKFile
		root   string
		tag    string
	}
	assignments := make([]assignment, 0, 32)
	for _, root := range roots {
		members := groups[root]
		computed := make([]string, 0, 16)
		if len(members) >= suiteInheritanceMinMembers {
			counts := make(map[string]int, 16)
			display := make(map[string]string, 16)
			for _, item := range members {
				seen := make(map[string]struct{}, len(item.file.SecondaryTags))
				for _, tag := range item.file.SecondaryTags {
					key := strings.ToLower(strings.TrimSpace(tag))
					if key == "" {
						continue
					}
					if _, dup := seen[key]; dup {
						continue
					}
					seen[key] = struct{}{}
					counts[key]++
					if _, exists := display[key]; !exists {
						display[key] = strings.TrimSpace(tag)
					}
				}
			}

			threshold := int(float64(len(members))*suiteInheritanceMinRatio + 0.999)
			if threshold < 2 {
				threshold = 2
			}
			keys := make([]string, 0, len(counts))
			for key := range counts {
				if counts[key] >= threshold {
					keys = append(keys, key)
				}
			}
			sort.Strings(keys)
			for _, key := range keys {
				if tag := display[key]; tag != "" {
					computed = append(computed, tag)
				}
			}
		}

		// 并集：当轮算出的特征标签 ∪ 已持久化的标签。成员增删只会让集合变大。
		merged, changed := mergeSuiteInheritanceTags(store.Namespaces[root], computed)
		if changed {
			store.Namespaces[root] = merged
			storeDirty = true
		}

		for _, tag := range merged {
			key := strings.ToLower(strings.TrimSpace(tag))
			if key == "" {
				continue
			}
			for _, item := range members {
				if containsTagFold(item.file.SecondaryTags, key) || strings.EqualFold(item.file.PrimaryTag, key) {
					continue
				}
				assignments = append(assignments, assignment{member: item.file, root: root, tag: tag})
			}
		}
	}

	inherited := 0
	for _, item := range assignments {
		file := item.member
		if file == nil || containsTagFold(file.SecondaryTags, strings.ToLower(item.tag)) {
			continue
		}
		if len(file.SecondaryTags) >= suiteInheritanceTagCap {
			continue
		}
		file.SecondaryTags = append(file.SecondaryTags, item.tag)
		sort.SliceStable(file.SecondaryTags, func(i, j int) bool {
			return strings.ToLower(file.SecondaryTags[i]) < strings.ToLower(file.SecondaryTags[j])
		})
		// 记录证据：让 UI/清单能说清"这个标签是从同套件的其他模块继承来的"。
		file.TagEvidence = append(file.TagEvidence, parser.TagEvidence{
			Tag:   item.tag,
			Rule:  "suite:" + item.root,
			Level: parser.EvidenceLevelPattern,
		})
		inherited++
	}
	if storeDirty {
		a.saveSuiteInheritanceStore(store)
	}
	return inherited
}

func containsTagFold(tags []string, key string) bool {
	for _, tag := range tags {
		if strings.EqualFold(strings.TrimSpace(tag), key) {
			return true
		}
	}
	return false
}
