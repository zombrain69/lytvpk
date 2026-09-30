package parser

import (
	"sort"
	"strings"
)

// TagEvidence 记录「某个标签是被什么证据打上的」，供 UI 展示与外部清单使用（W6）。
//
// 设计约束：
//   - 只增不减：证据只是"解释"，不参与取舍，标签集合不会因为证据缺失而变化；
//   - 体量受控：每个标签最多 3 条来源路径、整个文件最多 40 条证据，
//     因为清单要被大模型读进上下文（见 catalog 的 notes）。
type TagEvidence struct {
	Tag    string   `json:"tag"`
	Rule   string   `json:"rule"`
	Level  string   `json:"level"`            // exact | pattern | inferred
	Source []string `json:"source,omitempty"` // 命中的真实路径（≤3 条）
}

// 证据强度：exact（本体精确路径 / .mdl 材质表 / 官方脚本）> pattern（本体基名 token、目录规则）
// > inferred（关键词、标题、命名空间）。
const (
	EvidenceLevelExact    = "exact"
	EvidenceLevelPattern  = "pattern"
	EvidenceLevelInferred = "inferred"

	tagEvidenceSourceLimit = 3
	tagEvidenceLimit       = 40
)

func evidenceLevelRank(level string) int {
	switch level {
	case EvidenceLevelExact:
		return 3
	case EvidenceLevelPattern:
		return 2
	case EvidenceLevelInferred:
		return 1
	default:
		return 0
	}
}

type tagEvidenceRecorder struct {
	order []string
	items map[string]*TagEvidence
}

func newTagEvidenceRecorder() *tagEvidenceRecorder {
	return &tagEvidenceRecorder{items: make(map[string]*TagEvidence)}
}

// record 登记一次命中。rule 建议写清来源（`entity:weapon_shotgun_chrome`、`token:ak47`、
// `content:粒子特效`、`weapon:铁喷`、`character:hunter`、`title:ak47`）。
func (recorder *tagEvidenceRecorder) record(tag, rule, level, source string) {
	if recorder == nil {
		return
	}
	tag = CanonicalTag(tag)
	rule = strings.TrimSpace(rule)
	if tag == "" || rule == "" {
		return
	}
	item, exists := recorder.items[tag]
	if !exists {
		item = &TagEvidence{Tag: tag, Rule: rule, Level: level}
		recorder.items[tag] = item
		recorder.order = append(recorder.order, tag)
	}
	if evidenceLevelRank(level) > evidenceLevelRank(item.Level) {
		// 强度升级时换规则，同时清空旧来源：否则会出现「rule=本体锚点、source=关键词命中的路径」
		// 这种自相矛盾的证据（真机案例：消音的 rule 是 entity:weapon_smg_silenced，
		// source 却是 sound/weapons/… 的音频文件）。
		item.Level = level
		item.Rule = rule
		item.Source = nil
	}
	source = strings.TrimSpace(source)
	if source == "" || len(item.Source) >= tagEvidenceSourceLimit {
		return
	}
	for _, existing := range item.Source {
		if existing == source {
			return
		}
	}
	item.Source = append(item.Source, source)
}

// list 输出稳定排序（按标签名）的证据列表，并限制总条数。
func (recorder *tagEvidenceRecorder) list() []TagEvidence {
	if recorder == nil || len(recorder.order) == 0 {
		return nil
	}
	out := make([]TagEvidence, 0, len(recorder.order))
	for _, tag := range recorder.order {
		item := recorder.items[tag]
		if item == nil {
			continue
		}
		copied := *item
		out = append(out, copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Tag < out[j].Tag })
	if len(out) > tagEvidenceLimit {
		out = out[:tagEvidenceLimit]
	}
	return out
}

// evidenceForTags 只保留最终确实存在的标签的证据（标签可能被自定义一级标签排除掉）。
func evidenceForTags(recorder *tagEvidenceRecorder, secondaryTags []string, primaryTag string) []TagEvidence {
	items := recorder.list()
	if len(items) == 0 {
		return nil
	}
	keep := make(map[string]struct{}, len(secondaryTags)+1)
	for _, tag := range secondaryTags {
		keep[strings.ToLower(CanonicalTag(tag))] = struct{}{}
	}
	if primary := strings.ToLower(CanonicalTag(primaryTag)); primary != "" {
		keep[primary] = struct{}{}
	}
	out := make([]TagEvidence, 0, len(items))
	for _, item := range items {
		if _, ok := keep[strings.ToLower(item.Tag)]; !ok {
			continue
		}
		out = append(out, item)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
