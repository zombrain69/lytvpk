// Package gamedata 维护「游戏本体文件索引」（StockIndex）。
//
// 为什么需要它：Mod 标签识别过去靠「关键词 + 目录名」猜测，会在两类地方出错 ——
//  1. **猜错**：来源脚本里的模型名与直觉不一致。实测：`weapon_shotgun_chrome` 用的是
//     `models/w_models/weapons/w_pumpshotgun_A.mdl`，而 `weapon_pumpshotgun` 用的是
//     `models/w_models/weapons/w_shotgun.mdl` —— 按名字猜会把两者对调。
//  2. **漏掉**：本体里确实存在的实体，关键词表里根本没有。实测：`w_golfclub.mdl`、
//     `models/props_junk/explosive_box001.mdl`（烟花盒）、`w_cola.mdl`。
//
// StockIndex 把五个挂载点（left4dead2 / dlc1 / dlc2 / dlc3 / update）的 VPK 条目与
// 松散文件合成一份规范路径清单，作为「本体事实」供规则做精确判定与自检。
//
// 只读约束：本包**只读取**游戏目录（VPK 目录树 + 松散文件列表），绝不写、绝不改游戏文件；
// 索引缓存由调用方写到应用配置目录。
package gamedata

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"l4d2-manager-next/pkg/valve/vpk"

	"vpk-manager/internal/parser"
)

// SchemaVersion 是缓存文件格式版本；不兼容时调用方应重建而不是硬读。
const SchemaVersion = 1

// MountNames 按引擎搜索顺序（低优先级 → 高优先级）排列。
// 高优先级挂载点里的同名文件会覆盖低优先级，因此构建时后面的写入即可。
var MountNames = []string{
	"left4dead2",
	"left4dead2_dlc1",
	"left4dead2_dlc2",
	"left4dead2_dlc3",
	"update",
}

// looseRoots 是松散文件的扫描根。刻意不包含 addons（用户 Mod，不是本体），
// 也刻意把 sound / scenes 放进来 —— 实测语音只存在于松散目录，不在任何 pak01 里。
var looseRoots = []string{
	"models", "materials", "sound", "scenes", "scripts", "resource",
	"particles", "maps", "missions", "cfg", "expressions", "media", "modes",
}

// MountStat 记录单个挂载点的扫描结果，便于用户/维护者判断索引是否完整。
type MountStat struct {
	Name       string `json:"name"`
	VPKPath    string `json:"vpkPath,omitempty"`
	VPKEntries int    `json:"vpkEntries"`
	LooseFiles int    `json:"looseFiles"`
	Skipped    bool   `json:"skipped,omitempty"`
	Error      string `json:"error,omitempty"`
}

// StockIndex 是本体路径索引。构造后只读，可并发查询。
type StockIndex struct {
	SchemaVersion int         `json:"schemaVersion"`
	GeneratedAt   string      `json:"generatedAt"`
	GameRoot      string      `json:"gameRoot"`
	Mounts        []MountStat `json:"mounts"`
	PathCount     int         `json:"pathCount"`
	Paths         []string    `json:"paths"`

	set       map[string]struct{}
	globCache map[string][]string
	globMu    sync.Mutex
}

// ---------------------------------------------------------------------------
// 构建
// ---------------------------------------------------------------------------

// NormalizeGameRoot 接受三种常见输入，统一返回「安装根」（即包含 left4dead2/ 的那一层）：
//
//	<root>\left4dead2\addons  → <root>
//	<root>\left4dead2         → <root>
//	<root>                    → <root>
func NormalizeGameRoot(path string) (string, error) {
	candidate := filepath.Clean(strings.TrimSpace(path))
	if candidate == "" || candidate == "." {
		return "", fmt.Errorf("游戏目录为空")
	}

	mountVPK := func(dir string) string { return filepath.Join(dir, "pak01_dir.vpk") }
	// 已经是安装根。
	if fileExists(mountVPK(filepath.Join(candidate, "left4dead2"))) {
		return candidate, nil
	}
	// 传进来的是某个挂载点（最常见：left4dead2）。
	if fileExists(mountVPK(candidate)) {
		parent := filepath.Dir(candidate)
		if fileExists(mountVPK(filepath.Join(parent, "left4dead2"))) {
			return parent, nil
		}
		return parent, nil
	}
	// 传进来的是 addons 目录。
	if strings.EqualFold(filepath.Base(candidate), "addons") {
		parent := filepath.Dir(candidate) // ...\left4dead2
		root := filepath.Dir(parent)
		if fileExists(mountVPK(parent)) {
			return root, nil
		}
	}
	return "", fmt.Errorf("未找到游戏本体资源包（应为 <安装根>\\left4dead2\\pak01_dir.vpk）：%s", candidate)
}

// Build 扫描游戏本体，返回索引。单个挂载点缺失或损坏只记录在 Mounts 里，不中断；
// 全部挂载点都读不到时返回错误（调用方据此降级为「无本体索引」）。
func Build(gameRoot string) (*StockIndex, error) {
	root, err := NormalizeGameRoot(gameRoot)
	if err != nil {
		return nil, err
	}

	index := &StockIndex{
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().Format(time.RFC3339),
		GameRoot:      root,
		Mounts:        make([]MountStat, 0, len(MountNames)),
	}
	paths := make(map[string]struct{}, 70000)

	for _, mount := range MountNames {
		stat := MountStat{Name: mount}
		dir := filepath.Join(root, mount)
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			stat.Skipped = true
			index.Mounts = append(index.Mounts, stat)
			continue
		}

		pakPath := filepath.Join(dir, "pak01_dir.vpk")
		if fileExists(pakPath) {
			stat.VPKPath = pakPath
			entries, vpkErr := readVPKEntries(pakPath)
			if vpkErr != nil {
				stat.Error = vpkErr.Error()
			} else {
				stat.VPKEntries = len(entries)
				for _, entry := range entries {
					paths[entry] = struct{}{}
				}
			}
		} else {
			stat.Skipped = true
		}

		loose, looseErr := collectLooseFiles(dir)
		if looseErr != nil {
			if stat.Error == "" {
				stat.Error = looseErr.Error()
			}
		} else {
			stat.LooseFiles = len(loose)
			for _, entry := range loose {
				paths[entry] = struct{}{}
			}
		}

		index.Mounts = append(index.Mounts, stat)
	}

	if len(paths) == 0 {
		return nil, fmt.Errorf("未能从 %s 读取任何本体文件", root)
	}

	index.Paths = make([]string, 0, len(paths))
	for path := range paths {
		index.Paths = append(index.Paths, path)
	}
	sort.Strings(index.Paths)
	index.PathCount = len(index.Paths)
	index.attach()
	return index, nil
}

// readVPKEntries 只读取 VPK 目录树（不解压任何文件），条目名做与解析器一致的解码。
func readVPKEntries(pakPath string) ([]string, error) {
	opener := vpk.Single(pakPath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return nil, err
	}

	entries := make([]string, 0, len(archive.Files))
	for i := range archive.Files {
		name := archive.Files[i].Name()
		if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
			name = decoded
		}
		if normalized := NormalizePath(name); normalized != "" {
			entries = append(entries, normalized)
		}
	}
	return entries, nil
}

// collectLooseFiles 只列目录条目，不读文件内容。
func collectLooseFiles(mountDir string) ([]string, error) {
	var out []string
	var firstErr error
	for _, root := range looseRoots {
		base := filepath.Join(mountDir, root)
		if info, err := os.Stat(base); err != nil || !info.IsDir() {
			continue
		}
		walkErr := filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				if firstErr == nil {
					firstErr = err
				}
				return nil // 单个子目录读不了就跳过，不影响其余文件
			}
			if entry.IsDir() {
				return nil
			}
			relative, relErr := filepath.Rel(mountDir, path)
			if relErr != nil {
				return nil
			}
			if normalized := NormalizePath(relative); normalized != "" {
				out = append(out, normalized)
			}
			return nil
		})
		if walkErr != nil && firstErr == nil {
			firstErr = walkErr
		}
	}
	return out, firstErr
}

// NormalizePath 统一归档内路径："/" 分隔、小写、去前导 "./"。
// 空串表示不是有效条目（调用方应忽略）。
func NormalizePath(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	name = strings.ToLower(name)
	return strings.TrimSpace(name)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ---------------------------------------------------------------------------
// 查询
// ---------------------------------------------------------------------------

func (index *StockIndex) attach() {
	if index.set == nil {
		index.set = make(map[string]struct{}, len(index.Paths))
		for _, path := range index.Paths {
			index.set[path] = struct{}{}
		}
	}
	if index.globCache == nil {
		index.globCache = make(map[string][]string)
	}
}

// Exists 判断路径是否属于本体。
func (index *StockIndex) Exists(path string) bool {
	if index == nil {
		return false
	}
	index.attach()
	_, ok := index.set[NormalizePath(path)]
	return ok
}

// Category 返回路径的顶层类别（models / materials / sound / maps ...）。
func (index *StockIndex) Category(path string) string {
	normalized := NormalizePath(path)
	if slash := strings.IndexByte(normalized, '/'); slash > 0 {
		return normalized[:slash]
	}
	return ""
}

// PathsWithPrefix 返回以 prefix 开头的本体路径（用于规则自检与实体枚举）。
func (index *StockIndex) PathsWithPrefix(prefix string) []string {
	if index == nil {
		return nil
	}
	prefix = NormalizePath(prefix)
	start := sort.SearchStrings(index.Paths, prefix)
	var out []string
	for i := start; i < len(index.Paths); i++ {
		if !strings.HasPrefix(index.Paths[i], prefix) {
			break
		}
		out = append(out, index.Paths[i])
	}
	return out
}

// Glob 返回匹配 glob 的本体路径。支持 `**`（跨目录）、`*`（不跨目录）、`?`（单字符）。
// 结果带缓存；同一 pattern 只编译一次。
func (index *StockIndex) Glob(pattern string) []string {
	if index == nil {
		return nil
	}
	pattern = NormalizePath(pattern)
	if pattern == "" {
		return nil
	}
	index.attach()

	index.globMu.Lock()
	if cached, ok := index.globCache[pattern]; ok {
		index.globMu.Unlock()
		return cached
	}
	index.globMu.Unlock()

	matcher, err := compileGlob(pattern)
	if err != nil {
		return nil
	}
	prefix := literalPrefix(pattern)

	var matched []string
	if prefix == "" {
		for _, path := range index.Paths {
			if matcher.MatchString(path) {
				matched = append(matched, path)
			}
		}
	} else {
		// 利用已排序的 Paths 做前缀跳转，避免每条规则全表扫描。
		start := sort.SearchStrings(index.Paths, prefix)
		for i := start; i < len(index.Paths); i++ {
			path := index.Paths[i]
			if !strings.HasPrefix(path, prefix) {
				break
			}
			if matcher.MatchString(path) {
				matched = append(matched, path)
			}
		}
	}

	index.globMu.Lock()
	index.globCache[pattern] = matched
	index.globMu.Unlock()
	return matched
}

// literalPrefix 取第一个通配符之前的字面量前缀，并退回到最后一个 "/"，
// 保证前缀跳转不会漏掉匹配项（`a/b*c` 的跳转前缀是 `a/`）。
func literalPrefix(pattern string) string {
	wildcard := strings.IndexAny(pattern, "*?")
	if wildcard < 0 {
		return pattern
	}
	head := pattern[:wildcard]
	if slash := strings.LastIndexByte(head, '/'); slash >= 0 {
		return head[:slash+1]
	}
	return ""
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	var builder strings.Builder
	builder.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		char := pattern[i]
		switch char {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				// `**` 跨目录；吞掉紧随其后的 "/" 以便 `a/**/b` 也能匹配 `a/b`。
				i++
				if i+1 < len(pattern) && pattern[i+1] == '/' {
					i++
					builder.WriteString("(?:.*/)?")
					continue
				}
				builder.WriteString(".*")
				continue
			}
			builder.WriteString("[^/]*")
		case '?':
			builder.WriteString("[^/]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(char)))
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

// ---------------------------------------------------------------------------
// 缓存
// ---------------------------------------------------------------------------

// NewFromPaths 用已有路径清单构造索引（读缓存、测试夹具用）。
// 传指针是为了避免复制内含互斥锁的结构体；paths 会在内部排序，
// 因为 Glob / PathsWithPrefix 依赖有序切片做前缀跳转。
func NewFromPaths(meta *StockIndex, paths []string) *StockIndex {
	sorted := make([]string, 0, len(paths))
	for _, path := range paths {
		if normalized := NormalizePath(path); normalized != "" {
			sorted = append(sorted, normalized)
		}
	}
	sort.Strings(sorted)

	index := &StockIndex{
		SchemaVersion: SchemaVersion,
		Paths:         sorted,
		PathCount:     len(sorted),
	}
	if meta != nil {
		index.SchemaVersion = meta.SchemaVersion
		index.GeneratedAt = meta.GeneratedAt
		index.GameRoot = meta.GameRoot
		index.Mounts = meta.Mounts
	}
	index.attach()
	return index
}

// Save 写缓存（调用方负责目录存在与权限）。
func (index *StockIndex) Save(path string) error {
	if index == nil {
		return fmt.Errorf("索引为空")
	}
	data, err := json.Marshal(index)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Load 读缓存。版本不匹配或内容为空时返回错误，由调用方决定是否重建。
func Load(path string) (*StockIndex, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var index StockIndex
	if err := json.Unmarshal(raw, &index); err != nil {
		return nil, err
	}
	if index.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("本体索引版本不匹配：缓存 %d，当前 %d", index.SchemaVersion, SchemaVersion)
	}
	if len(index.Paths) == 0 {
		return nil, fmt.Errorf("本体索引为空")
	}
	sort.Strings(index.Paths)
	index.PathCount = len(index.Paths)
	index.attach()
	return &index, nil
}

// LoadOrBuild 优先读缓存；缓存不存在、过期或损坏时重建并写回。
//
// maxAge <= 0 表示缓存永不过期（游戏更新后由用户显式重建）。
func LoadOrBuild(cachePath, gameRoot string, maxAge time.Duration) (*StockIndex, error) {
	if cachePath != "" {
		if info, err := os.Stat(cachePath); err == nil {
			fresh := maxAge <= 0 || time.Since(info.ModTime()) <= maxAge
			if fresh {
				if index, loadErr := Load(cachePath); loadErr == nil {
					return index, nil
				}
			}
		}
	}

	index, err := Build(gameRoot)
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		if mkErr := os.MkdirAll(filepath.Dir(cachePath), 0o755); mkErr == nil {
			_ = index.Save(cachePath) // 缓存写失败不影响本次使用
		}
	}
	return index, nil
}
