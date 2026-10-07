package parser

import "strings"

// 地图包的"资源名巧合"标签过滤（2026-10-07 用户口径）。
//
// 背景：本体 token 通道与新的"独占资源槽位"通道（`stock:<entityID>`）都是**名字证据**。
// 地图包会自带一堆道具 / 素材：广西-南宁地图里有 `models/nanningcity/props/50cal_easter.vvd`、
// `.../props/crowbar.vvd`，名称里的 `50cal` / `crowbar` 会让整张地图挂上「二代固定机枪」「撬棍」
// ——用户看着就是错标。
//
// 口径：主类型是「地图」时，**只砍掉"证据完全来自 token / 槽位通道"的标签**；本体精确锚点
// （真的替换了本体文件）、标题/描述证据、文件类别与内容标签（贴图 / 模型 / 汽油桶…）全部保留。
// 聚合标签（近战 / 所有枪械 / 物品…）是派生的：先删再按剩下的具体标签重算，避免留下孤儿聚合。

// coincidenceEvidencePrefixes 是"资源名巧合"通道的规则前缀。
var coincidenceEvidencePrefixes = []string{"token:", "stock:"}

func isCoincidenceEvidenceRule(rule string) bool {
	for _, prefix := range coincidenceEvidencePrefixes {
		if strings.HasPrefix(rule, prefix) {
			return true
		}
	}
	return false
}

// derivedAggregateTags 是 addWeaponTag / applyItemAggregateTags 派生出来的聚合标签。
// 它们本身没有证据条目，过滤时必须先删再按剩余的具体标签重算。
var derivedAggregateTags = []string{
	"手枪", "步枪", "狙击枪", "霰弹枪", "冲锋枪", "榴弹发射器", "M60", "固定机关枪", "近战",
	"官方近战", "所有官方近战", "所有枪械",
	"物品", "投掷物", "所有投掷物品", "医疗物品", "所有医疗物品", "弹药堆", "盒子",
	"燃烧弹", "高爆弹", "镭射",
}

// itemConcreteTags 是"物品类具体标签"（内容规则里标了 item 的 + 本体实体表里的物品实体）。
// 用来在重算聚合时补齐「物品 / 医疗物品 / 投掷物…」。
func itemConcreteTags() map[string]bool {
	out := make(map[string]bool)
	for _, rule := range contentTagRules {
		if rule.isItem {
			out[rule.tag] = true
		}
	}
	if stockEntities != nil {
		for _, entity := range stockEntities.Entities {
			if entity.IsItem && entity.Tag != "" {
				out[entity.Tag] = true
			}
		}
	}
	return out
}

// dropCoincidenceOnlyTags 删掉"证据只有 token / 槽位通道"的标签，并按剩下的具体标签重算聚合。
// 返回删除的标签数。tags 就是最终二级标签集合（调用点在地图分支）。
func dropCoincidenceOnlyTags(tags map[string]bool, evidence *tagEvidenceRecorder) int {
	if len(tags) == 0 || evidence == nil {
		return 0
	}
	coincidenceOnly := make(map[string]bool)
	supported := make(map[string]bool)
	for _, tag := range evidence.order {
		item := evidence.items[tag]
		if item == nil {
			continue
		}
		if isCoincidenceEvidenceRule(item.Rule) {
			coincidenceOnly[tag] = true
			continue
		}
		supported[tag] = true
	}

	dropped := 0
	for tag := range tags {
		if coincidenceOnly[tag] && !supported[tag] {
			delete(tags, tag)
			dropped++
		}
	}
	if dropped == 0 {
		return 0
	}

	// 聚合标签重算：先删派生聚合，再按剩下的具体标签补回（武器聚合走 addWeaponTag，
	// 物品聚合走 applyItemAggregateTags）。
	for _, tag := range derivedAggregateTags {
		delete(tags, tag)
	}
	items := itemConcreteTags()
	for tag := range tags {
		addWeaponTag(tag, tags)
	}
	for tag := range tags {
		if items[tag] {
			applyItemAggregateTags(tag, tags)
		}
	}
	return dropped
}
