// Package grouping 是与 UI / 存储解耦的"策略组推导引擎"。
//
// 设计约束：
//   - 纯逻辑：只接收 []Mod，不读磁盘、不碰 App 状态，便于单测与基准测试；
//   - 标准化：信号（signal）集中登记在 Catalog 里，带稳定的 ID、展示名与可信度分级；
//   - 低耦合：工坊合集、外部建议文件等"来自外部的事实"通过 Injected 候选注入，
//     引擎本身不知道它们从哪来；
//   - 性能：一次遍历建立倒排索引，各信号只读索引，不做重复扫描。
package grouping

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Mod 是推导引擎的输入：只包含与分组有关的字段。
type Mod struct {
	Key               string
	Name              string
	Title             string
	Author            string
	PrimaryTag        string
	SecondaryTags     []string
	Subject           string
	SubjectConfidence string
	VoiceCharacters   []string
	Folder            string
	WorkshopID        string
	// ResourceRoots 是 VPK 内部路径里的"作者/套件命名空间"（models/<作者>/<套件>，
	// 例如 913limod/airi_evilfall、codm/ice）：同一个套件常拆成很多 VPK
	// （本体 + 配件 / 贴图包 / 参数包），它们共享这个目录，是"需要一起启用"的结构性证据。
	ResourceRoots []string
	// SourcePath 是调用方自己的寻址信息（物理路径）。
	// 推导引擎完全不使用它，只保证"同一个物理文件"能被调用方区分开：
	// addonlist 键在 root 与 disabled 同名时会重复，物理路径才唯一。
	SourcePath string
}

// Candidate 是一条候选分组：由某个信号产出，或由外部注入。
type Candidate struct {
	// Provider 是产出这条候选的信号 ID（见 Catalog）；外部注入用 Source 标记。
	Provider string
	Label    string
	Reason   string
	Score    int
	Signals  []string
	Keys     []string
	Names    []string
	// Source 为空表示内置推导；"external" 表示来自外部建议文件（模型 / 人工）。
	Source   string
	Strategy string
	// TagKey / TagScope / TagInSet / TagOutside 由 annotateTagCoverage 填充：
	// 描述"已有标签能否精确选出这一批 Mod"（见 tags.go）。
	TagKey     string
	TagScope   string
	TagInSet   int
	TagOutside int
	// Tag / TagReason / MemberTags 是"调用方（外部建议文件）给的标签提案"，
	// 引擎原样透传：有它时优先于按规则推导出来的标签。
	Tag        string
	TagReason  string
	MemberTags map[string][]string
}

// Suggestion 是引擎输出的建议（与前端 JSON 模型一致）。
type Suggestion struct {
	ID          string   `json:"id"`
	Label       string   `json:"label"`
	Reason      string   `json:"reason"`
	Confidence  string   `json:"confidence"`
	Score       int      `json:"score"`
	Signals     []string `json:"signals"`
	MemberKeys  []string `json:"memberKeys"`
	MemberNames []string `json:"memberNames"`
	// ExistingGroupID 由调用方在拿到结果后填充（引擎不关心策略组存储）。
	ExistingGroupID string `json:"existingGroupId,omitempty"`
	Source          string `json:"source,omitempty"`
	Strategy        string `json:"strategy,omitempty"`
	// TagKey / TagScope / TagInSet / TagOutside 描述"已有标签能多大程度代表这一组"：
	// scope=exact 表示该标签恰好只能筛出这一批 Mod —— 这时建组的增量价值很低
	// （直接用标签筛选即可），列表里会被降权，并在界面上标注。
	TagKey     string `json:"tagKey,omitempty"`
	TagScope   string `json:"tagScope,omitempty"`
	TagInSet   int    `json:"tagInSet,omitempty"`
	TagOutside int    `json:"tagOutside,omitempty"`
	// Tag / TagReason / MemberTags：来自外部建议文件的标签提案（可空）。
	Tag        string              `json:"tag,omitempty"`
	TagReason  string              `json:"tagReason,omitempty"`
	MemberTags map[string][]string `json:"memberTags,omitempty"`
}

// Stats 汇总一次推导的规模，便于诊断与前端展示（哪个信号贡献了多少）。
type Stats struct {
	Mods        int            `json:"mods"`
	Candidates  int            `json:"candidates"`
	Merged      int            `json:"merged"`
	Dropped     int            `json:"dropped"`
	ByProvider  map[string]int `json:"byProvider"`
	DroppedByRe map[string]int `json:"droppedByReason"`
}

// Options 是推导参数（全部有默认值，零值即可用）。
type Options struct {
	Limit              int
	MinScore           int
	MinMembers         int
	MaxMembers         int
	MaxCuratedMembers  int
	MaxAuthorMembers   int
	ComfortSize        int
	SizePenalty        int
	MaxPenalty         int
	MultiSignalBonus   int
	PerProviderLimit   int
	MaxCandidates      int
	Injected           []Candidate
	ExcludedProviders  []string
	DisableQuotaForIDs []string
}

// DefaultOptions 返回生产环境使用的默认参数。
func DefaultOptions() Options {
	return Options{
		Limit:             40,
		MinScore:          45,
		MinMembers:        2,
		MaxMembers:        60,
		MaxCuratedMembers: 300,
		MaxAuthorMembers:  12,
		ComfortSize:       8,
		SizePenalty:       4,
		MaxPenalty:        60,
		MultiSignalBonus:  5,
		PerProviderLimit:  20,
		MaxCandidates:     4000,
	}
}

func (o Options) withDefaults() Options {
	defaults := DefaultOptions()
	if o.Limit <= 0 {
		o.Limit = defaults.Limit
	}
	if o.MinScore <= 0 {
		o.MinScore = defaults.MinScore
	}
	if o.MinMembers <= 0 {
		o.MinMembers = defaults.MinMembers
	}
	if o.MaxMembers <= 0 {
		o.MaxMembers = defaults.MaxMembers
	}
	if o.MaxCuratedMembers <= 0 {
		o.MaxCuratedMembers = defaults.MaxCuratedMembers
	}
	if o.MaxAuthorMembers <= 0 {
		o.MaxAuthorMembers = defaults.MaxAuthorMembers
	}
	if o.ComfortSize <= 0 {
		o.ComfortSize = defaults.ComfortSize
	}
	if o.SizePenalty <= 0 {
		o.SizePenalty = defaults.SizePenalty
	}
	if o.MaxPenalty <= 0 {
		o.MaxPenalty = defaults.MaxPenalty
	}
	if o.MultiSignalBonus <= 0 {
		o.MultiSignalBonus = defaults.MultiSignalBonus
	}
	if o.PerProviderLimit <= 0 {
		o.PerProviderLimit = defaults.PerProviderLimit
	}
	if o.MaxCandidates <= 0 {
		o.MaxCandidates = defaults.MaxCandidates
	}
	return o
}

// Suggest 跑完整条推导流水线：建索引 → 各信号产出候选 → 合并去重 → 排序 → 截断。
func Suggest(mods []Mod, options Options) ([]Suggestion, Stats) {
	options = options.withDefaults()
	stats := Stats{
		Mods:        len(mods),
		ByProvider:  map[string]int{},
		DroppedByRe: map[string]int{},
	}
	index := buildIndex(mods)

	candidates := make([]Candidate, 0, 256)
	skipped := make(map[string]struct{}, len(options.ExcludedProviders))
	for _, id := range options.ExcludedProviders {
		skipped[id] = struct{}{}
	}
	for _, provider := range Providers() {
		if _, skip := skipped[provider.ID]; skip {
			continue
		}
		produced := provider.Run(index, options)
		stats.ByProvider[provider.ID] = len(produced)
		candidates = append(candidates, produced...)
		if len(candidates) >= options.MaxCandidates {
			candidates = candidates[:options.MaxCandidates]
			break
		}
	}
	if len(options.Injected) > 0 {
		stats.ByProvider["injected"] = len(options.Injected)
		candidates = append(candidates, options.Injected...)
	}
	stats.Candidates = len(candidates)

	merged := mergeCandidates(candidates, options, &stats)
	stats.Merged = len(merged)
	merged = applyProviderQuota(merged, options)
	// 贴上"标签覆盖"标注供调用方使用。这里**不改排序**：引擎的排序语义是
	// "这条建议有多像一个真实的组"，而"用户能不能用标签代替它"是展示层的事
	// （app 层会把标签已精确覆盖的建议单独下沉一层，见 sortSuggestionsForDisplay）。
	annotateTagCoverage(merged, index)

	suggestions := make([]Suggestion, 0, len(merged))
	for _, candidate := range merged {
		suggestions = append(suggestions, Suggestion{
			ID:          "sug-" + signature(candidate.Keys),
			Label:       candidate.Label,
			Reason:      candidate.Reason,
			Confidence:  ConfidenceForSignals(candidate.Signals),
			Score:       candidate.Score,
			Signals:     candidate.Signals,
			MemberKeys:  candidate.Keys,
			MemberNames: candidate.Names,
			Source:      candidate.Source,
			Strategy:    candidate.Strategy,
			TagKey:      candidate.TagKey,
			TagScope:    candidate.TagScope,
			TagInSet:    candidate.TagInSet,
			TagOutside:  candidate.TagOutside,
			Tag:         candidate.Tag,
			TagReason:   candidate.TagReason,
			MemberTags:  candidate.MemberTags,
		})
	}
	if len(suggestions) > options.Limit {
		suggestions = suggestions[:options.Limit]
	}
	return suggestions, stats
}

// mergeCandidates 合并"同一批成员"的候选：保留分数最高的理由，
// 合并信号标签，并按排序分数降序排列（已建组沉底由调用方负责）。
func mergeCandidates(candidates []Candidate, options Options, stats *Stats) []Candidate {
	bySignature := make(map[string]int, len(candidates))
	merged := make([]Candidate, 0, len(candidates))

	for _, candidate := range candidates {
		if len(candidate.Keys) < options.MinMembers {
			stats.Dropped++
			stats.DroppedByRe["members-too-few"]++
			continue
		}
		if len(candidate.Keys) > candidateMemberLimit(candidate, options) {
			stats.Dropped++
			stats.DroppedByRe["members-too-many"]++
			continue
		}
		if candidate.Score < options.MinScore {
			stats.Dropped++
			stats.DroppedByRe["score-too-low"]++
			continue
		}
		key := strings.Join(candidate.Keys, "|")
		position, exists := bySignature[key]
		if !exists {
			bySignature[key] = len(merged)
			merged = append(merged, candidate)
			continue
		}
		existing := &merged[position]
		existing.Signals = mergeSignalLabels(existing.Signals, candidate.Signals)
		if candidate.Score > existing.Score {
			existing.Label = candidate.Label
			existing.Reason = candidate.Reason
			existing.Score = candidate.Score
			existing.Provider = candidate.Provider
		}
		if candidate.Source != "" {
			existing.Source = candidate.Source
		}
		if candidate.Strategy != "" {
			existing.Strategy = candidate.Strategy
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		rankI := SortScore(merged[i].Score, merged[i].Signals, len(merged[i].Keys), merged[i].ConfidenceSignals(), options)
		rankJ := SortScore(merged[j].Score, merged[j].Signals, len(merged[j].Keys), merged[j].ConfidenceSignals(), options)
		if rankI != rankJ {
			return rankI > rankJ
		}
		if merged[i].Score != merged[j].Score {
			return merged[i].Score > merged[j].Score
		}
		if len(merged[i].Keys) != len(merged[j].Keys) {
			return len(merged[i].Keys) < len(merged[j].Keys)
		}
		return merged[i].Label < merged[j].Label
	})
	return merged
}

// annotateTagCoverage 给每条合并后的建议贴上"标签覆盖率"，供排序与界面展示使用。
func annotateTagCoverage(merged []Candidate, index *Index) {
	if index == nil {
		return
	}
	for i := range merged {
		coverage, ok := TagCoverageFor(index.mods, merged[i].Keys)
		if !ok {
			continue
		}
		merged[i].TagKey = coverage.Tag
		merged[i].TagScope = coverage.Scope
		merged[i].TagInSet = coverage.InSet
		merged[i].TagOutside = coverage.Outside
	}
}

// applyProviderQuota 限制单个信号占用的条目数：任何一个信号都不应该刷屏，
// 这样"文件夹 / 合集 / 前缀 / 作者"等其它信号也有机会出现在前 40 条里。
// applyProviderQuota 限制单个信号占用的条目数，并返回裁剪后的切片。
// 必须返回新切片：早期版本只改本地长度，导致被删候选在尾部残留（同一批成员重复几百次）。
func applyProviderQuota(candidates []Candidate, options Options) []Candidate {
	if options.PerProviderLimit <= 0 {
		return candidates
	}
	exempt := make(map[string]struct{}, len(options.DisableQuotaForIDs))
	for _, id := range options.DisableQuotaForIDs {
		exempt[id] = struct{}{}
	}
	counts := make(map[string]int, len(candidates))
	kept := make([]Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		provider := candidate.Provider
		if _, ok := exempt[provider]; ok {
			kept = append(kept, candidate)
			continue
		}
		counts[provider]++
		if counts[provider] > options.PerProviderLimit {
			continue
		}
		kept = append(kept, candidate)
	}
	return kept
}

// ConfidenceSignals 返回用于判定的信号集合（目前与 Signals 相同，
// 保留这个方法是为了以后把展示信号与证据信号分开）。
func (c Candidate) ConfidenceSignals() []string { return c.Signals }

type providerFunc struct {
	ID      string
	Signals []string
	Run     func(*Index, Options) []Candidate
}

type signalSpec struct {
	ID         string
	Label      string
	Confidence string // high | medium | low
	Curated    bool   // 用户显式维护的结构（合集 / 文件夹）
}

// 各信号的基准分（集中在这里，避免分数散落在多个 provider 里）。
const (
	CollectionScore   = 80
	FolderScore       = 78
	SameFileNameScore = 60
	VoiceScore        = 55
	SubjectScore      = 50
	PrefixScore       = 45
	TagScore          = 45
	AuthorScore       = 45
	ExternalScore     = 90
	// SuiteNamespaceScore 是"同一套件命名空间下的多个模块"（models/<作者>/<套件>）的基准分：
	// 结构证据很硬，但低于用户显式维护的合集 / 文件夹。
	SuiteNamespaceScore = 74
)

// suiteSignalLabel 是套件信号的展示名（提示词与文档里用同一个词）。
const suiteSignalLabel = "套装资源目录"

var signalCatalog = []signalSpec{
	{ID: "folder", Label: "同一文件夹", Confidence: "high", Curated: true},
	{ID: "same-filename", Label: "同名文件（不同目录）", Confidence: "medium"},
	{ID: "filename-prefix", Label: "文件名前缀", Confidence: "low"},
	{ID: "shared-tags", Label: "共同标签", Confidence: "medium"},
	{ID: "same-author", Label: "同一作者", Confidence: "low"},
	{ID: "voice-character", Label: "语音角色", Confidence: "medium"},
	{ID: "subject", Label: "主体识别", Confidence: "medium"},
	// 结构证据属于"内容证据"档（与主体识别同级）：比前缀/作者硬，但不是用户显式维护的结构。
	{ID: "suite-namespace", Label: suiteSignalLabel, Confidence: "medium"},
	{ID: "collection", Label: "工坊合集成员", Confidence: "high", Curated: true},
	{ID: "external", Label: "外部建议", Confidence: "high", Curated: true},
}

// Catalog 返回信号目录（标准化：ID / 展示名 / 可信度 / 是否属于用户维护结构）。
func Catalog() []signalSpec {
	result := make([]signalSpec, len(signalCatalog))
	copy(result, signalCatalog)
	return result
}

// SignalLabel 把信号 ID 翻译成展示名。
func SignalLabel(id string) string {
	for _, spec := range signalCatalog {
		if spec.ID == id {
			return spec.Label
		}
	}
	return id
}

// SignalID 把展示名翻译回 ID（外部文件可能直接写中文信号名）。
func SignalID(label string) string {
	value := strings.TrimSpace(label)
	for _, spec := range signalCatalog {
		if spec.ID == value || spec.Label == value {
			return spec.ID
		}
	}
	return ""
}

// ConfidenceForSignals 按"证据类型"判定可信度：用户维护的结构最高，
// 内容证据次之，弱启发式最低。
func ConfidenceForSignals(signals []string) string {
	level := "low"
	for _, signal := range signals {
		id := SignalID(signal)
		spec, ok := lookupSignal(id)
		if !ok {
			continue
		}
		if spec.Curated {
			return "high"
		}
		if spec.Confidence == "medium" {
			level = "medium"
		}
	}
	return level
}

func lookupSignal(id string) (signalSpec, bool) {
	for _, spec := range signalCatalog {
		if spec.ID == id {
			return spec, true
		}
	}
	return signalSpec{}, false
}

func candidateIsCurated(candidate Candidate) bool {
	for _, signal := range candidate.Signals {
		if spec, ok := lookupSignal(SignalID(signal)); ok && spec.Curated {
			return true
		}
	}
	return candidate.Source == "external"
}

func candidateMemberLimit(candidate Candidate, options Options) int {
	if candidateIsCurated(candidate) {
		return options.MaxCuratedMembers
	}
	return options.MaxMembers
}

// SortScore 是排序配方：可信度加权 + 规模调整后的原始分 + 多信号加权。
// 规模惩罚让"3 个成员、证据明确"的小组排在"几十个成员、只共享通用标签"的粗糙聚类之前。
func SortScore(score int, signals []string, memberCount int, _ []string, options Options) int {
	options = options.withDefaults()
	total := ConfidenceBonus(ConfidenceForSignals(signals)) +
		sizeAdjustedScore(score, memberCount, signals, options)
	if len(signals) >= 2 {
		total += options.MultiSignalBonus
	}
	return total
}

// ConfidenceBonus 暴露可信度加权（App 层测试与文档引用同一口径）。
func ConfidenceBonus(level string) int {
	switch level {
	case "high":
		return 40
	case "medium":
		return 20
	default:
		return 0
	}
}

func sizeAdjustedScore(score int, memberCount int, signals []string, options Options) int {
	for _, signal := range signals {
		if spec, ok := lookupSignal(SignalID(signal)); ok && spec.Curated {
			return score
		}
	}
	if memberCount <= options.ComfortSize {
		return score
	}
	penalty := (memberCount - options.ComfortSize) * options.SizePenalty
	if penalty > options.MaxPenalty {
		penalty = options.MaxPenalty
	}
	return score - penalty
}

func mergeSignalLabels(existing []string, extra []string) []string {
	seen := make(map[string]struct{}, len(existing)+len(extra))
	merged := make([]string, 0, len(existing)+len(extra))
	for _, group := range [][]string{existing, extra} {
		for _, signal := range group {
			value := strings.TrimSpace(signal)
			if value == "" {
				continue
			}
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			merged = append(merged, value)
		}
	}
	return merged
}

// signature 生成稳定 ID（同一批成员无论顺序都得到同一个 ID）。
// Signature 生成稳定的成员集合签名（排序后取 SHA1 前 12 位）。
func Signature(keys []string) string { return signature(keys) }

func signature(keys []string) string {
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	sum := sha1.Sum([]byte(strings.Join(sorted, "|")))
	return hex.EncodeToString(sum[:])[:12]
}

// KnownPrimaryCategories 是解析器规定的分类；真实数据里有脏值，
// 只比较这些已知值，避免把同套件 Mod 误判成跨分类。
var KnownPrimaryCategories = map[string]struct{}{
	"地图": {},
	"人物": {},
	"武器": {},
	"其他": {},
}

// SharesPrimaryTag 判断一批 Mod 的主分类是否一致（未知值与缺失值不参与判断）。
func SharesPrimaryTag(mods []Mod) bool {
	primary := ""
	for _, mod := range mods {
		value := strings.TrimSpace(mod.PrimaryTag)
		if value == "" {
			continue
		}
		if _, known := KnownPrimaryCategories[value]; !known {
			continue
		}
		if primary == "" {
			primary = value
			continue
		}
		if !strings.EqualFold(primary, value) {
			return false
		}
	}
	return true
}

// NormalizeAuthor 过滤 addonauthor 里的占位符与多人联名：
// 实测数据里常见 `AUTHOR_NAME`、`Animal33/zmg/momo`、`A  +  B`、`甲，乙，丙`。
func NormalizeAuthor(raw string) string {
	author := strings.TrimSpace(raw)
	if author == "" {
		return ""
	}
	switch strings.ToLower(author) {
	case "author_name", "author", "unknown", "n/a", "none", "null", "-", "无", "作者":
		return ""
	}
	if strings.ContainsAny(author, `/\,|、，；;&＋`) || strings.Contains(author, " + ") || strings.Contains(author, "作者") {
		return ""
	}
	if len([]rune(author)) < 2 {
		return ""
	}
	return author
}

// NormalizeSubject 去掉解析器统一加上的"主体："前缀，并丢弃"混合包"这类内容混杂主体。
func NormalizeSubject(raw string) string {
	subject := strings.TrimSpace(raw)
	if subject == "" {
		return ""
	}
	for _, prefix := range []string{"主体：", "主体:"} {
		subject = strings.TrimSpace(strings.TrimPrefix(subject, prefix))
	}
	if subject == "" || strings.Contains(subject, "混合包") {
		return ""
	}
	return subject
}

// NamePrefix 取文件名的"系列前缀"：首个词太短时用前两个词，中文名不分词时返回空。
func NamePrefix(name string) string {
	value := strings.TrimSpace(strings.ToLower(name))
	if value == "" {
		return ""
	}
	if index := strings.LastIndex(value, "."); index > 0 {
		value = value[:index]
	}
	if index := strings.Index(value, "_addon"); index > 0 {
		value = value[:index]
	}
	value = strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(value)
	words := strings.Fields(value)
	if len(words) < 2 {
		return ""
	}
	prefix := words[0]
	if len([]rune(prefix)) < 4 {
		if len(words) < 3 {
			return ""
		}
		prefix = words[0] + " " + words[1]
		if len([]rune(prefix)) < 6 {
			return ""
		}
	}
	return prefix
}

// PrefixKey 是"系列前缀"索引键：Key 小写用于比较，Label 保留原始大小写用于当组名。
type PrefixKey struct {
	Key   string
	Label string
}

// NamePrefixKeys 生成一个文件名的全部"系列前缀"键。
//
// 为什么需要多个键（真实缺陷）：`[Milfy]白银审判_Rochelle.vpk` 与
// `[Milfy]白银审判里内衣关.vpk` 明明是同一套，但名字里只有 `_` 一处分隔，
// 中文名整体是一个"词"，旧实现要求"至少两个词"于是直接判空，一条建议都出不来。
//
// 规则：
//  1. 旧语义保留：完整的第一段（含 `[标签]`）作为一个精确键，`series_a/b/c` 这类仍然照旧；
//  2. 新增：去掉 `[标签]` / `【标签】` 之后的系列名，按 4~12 字生成前缀键，
//     这样 `白银审判`、`白银审判里内衣关` 会落进同一个桶；
//  3. 纯数字前缀（工坊 ID）不产出键，避免出现 `3788635187` 这种组名。
func NamePrefixKeys(name string) []PrefixKey {
	value := strings.TrimSpace(name)
	if value == "" {
		return nil
	}
	if index := strings.LastIndex(value, "."); index > 0 {
		value = value[:index]
	}
	if index := strings.Index(strings.ToLower(value), "_addon"); index > 0 {
		value = value[:index]
	}
	words := strings.Fields(strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(value))
	if len(words) == 0 {
		return nil
	}

	keys := make([]PrefixKey, 0, 6)
	seen := make(map[string]struct{}, 6)
	push := func(key, label string) {
		key = strings.ToLower(strings.TrimSpace(key))
		if len([]rune(key)) < 4 || numericOnly(key) {
			return
		}
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		keys = append(keys, PrefixKey{Key: key, Label: strings.TrimSpace(label)})
	}

	// 1) 旧语义整条保留：`NamePrefix` 的"首个词≥4 字，否则前两词≥6 字"。
	//    真实回归：漏掉这条以后 `Hit Marker Overhaul*`（首词 hit 只有 3 字，靠前两词成键）
	//    与 `fm_alpha_*` 都不再有前缀建议。
	if legacy := NamePrefix(value); legacy != "" {
		push(legacy, legacy)
	}

	// 2) 完整的第一段（含 `[标签]`）。
	raw := words[0]
	push(raw, raw)

	// 3) 新语义：去掉标签后的系列名，按 4~12 字生成前缀。
	series := stripBracketTags(raw)
	seriesRunes := []rune(series)
	if len(seriesRunes) >= 4 {
		limit := len(seriesRunes)
		if limit > 12 {
			limit = 12
		}
		for length := 4; length <= limit; length++ {
			push(string(seriesRunes[:length]), string(seriesRunes[:length]))
		}
	}
	return keys
}

// stripBracketTags 反复去掉名字开头的 `[标签]` / `【标签】` / `（标签）` / `(标签)` / `《标签》`。
func stripBracketTags(value string) string {
	trimmed := strings.TrimSpace(value)
	pairs := [][2]string{{"[", "]"}, {"【", "】"}, {"（", "）"}, {"(", ")"}, {"《", "》"}}
	for changed := true; changed; {
		changed = false
		for _, pair := range pairs {
			if !strings.HasPrefix(trimmed, pair[0]) {
				continue
			}
			if end := strings.Index(trimmed, pair[1]); end > 0 {
				trimmed = strings.TrimSpace(trimmed[end+len(pair[1]):])
				changed = true
			}
		}
	}
	return trimmed
}

// numericOnly 判断前缀是不是纯数字（工坊 ID / 时间戳），这种不能当系列名。
func numericOnly(value string) bool {
	hasDigit := false
	for _, r := range value {
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		if r == ' ' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return false
	}
	return hasDigit
}

// FolderDisplay 把 "Airi包\开关" 变成 "Airi包 / 开关"。
func FolderDisplay(folder string) string {
	parts := strings.FieldsFunc(folder, func(r rune) bool { return r == '\\' || r == '/' })
	return strings.Join(parts, " / ")
}

// ReservedFolder 判断是否是"存放位置"而不是用户整理的套件文件夹。
func ReservedFolder(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "workshop", "disabled":
		return true
	default:
		return false
	}
}

// BaseName 是文件名（不含目录）。
func BaseName(key string) string {
	return filepath.Base(strings.ReplaceAll(key, "\\", "/"))
}

// DescribeStats 生成一行可读的诊断信息（用于日志 / 前端提示）。
func (s Stats) DescribeStats() string {
	return fmt.Sprintf("mods=%d candidates=%d merged=%d dropped=%d", s.Mods, s.Candidates, s.Merged, s.Dropped)
}
