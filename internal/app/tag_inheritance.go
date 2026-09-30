package app

import (
	"sort"
	"strings"

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
		if len(members) < suiteInheritanceMinMembers {
			continue
		}
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
			tag := display[key]
			if tag == "" {
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
