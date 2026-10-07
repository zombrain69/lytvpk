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

// 地图专属目录（道具/场景资源）前缀：地图包里出现这些路径只是为了搭场景，
// 不代表这个地图 mod "替换了某个物品"。主类型=地图 时，这些路径上的精确锚点也不再当标签。
var mapOnlyPathMarkers = []string{
	"models/props",
	"materials/models/props",
	"materials/props",
	"/props/",
	"props_",
}

// isMapOnlyEvidencePath 判断一条证据路径是不是"地图专属目录"里的资源。
func isMapOnlyEvidencePath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	if lower == "" || strings.HasSuffix(lower, ".vpk") {
		return false
	}
	for _, marker := range mapOnlyPathMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// isLocalizationTokenTag 判断一个标签是不是游戏本地化 token（不是给人看的标签）：
// 地图任务文件里的 `"DisplayTitle" "#L4D360UI_CampaignName_C1"` 会被当成战役名带进标签集，
// 于是筛选条里出现上百个 `#L4D360UI_*` 的"子标签"——用户口径：这类不是标签。
func isLocalizationTokenTag(tag string) bool {
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return true
	}
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "L4D360UI_") || strings.HasPrefix(upper, "L4D_")
}

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

// resourceAnchorEvidencePrefixes 是"这个 Mod 真的替换了本体某个资源"的精确锚点通道。
// 只有这条通道的证据才允许被"地图专属目录"降级；内容/类别（content: / category:）、
// 标题（title:）、角色（character:）等证据描述的是"包里有什么"，不是"替换了本体什么"，
// 一旦拿路径去降级就会误删「模型 / 贴图」这类正当内容标签。
var resourceAnchorEvidencePrefixes = []string{"entity:"}

func isResourceAnchorEvidenceRule(rule string) bool {
	for _, prefix := range resourceAnchorEvidencePrefixes {
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
		switch {
		case isCoincidenceEvidenceRule(item.Rule):
			coincidenceOnly[tag] = true
		case isResourceAnchorEvidenceRule(item.Rule) && allSourcesMapOnly(item.Source):
			// 地图专属目录里的"精确锚点"同样是搭场景：广西-南宁地图自带
			// models/props_junk/explosive_box001 道具 → 不该给整张地图挂「烟花盒」。
			coincidenceOnly[tag] = true
		default:
			supported[tag] = true
		}
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

// allSourcesMapOnly 判断某个标签的全部证据路径是否都在"地图专属目录"里。
func allSourcesMapOnly(sources []string) bool {
	if len(sources) == 0 {
		return false
	}
	for _, source := range sources {
		if !isMapOnlyEvidencePath(source) {
			return false
		}
	}
	return true
}

// dropLocalizationTokenTags 把本地化 token 从标签集合里清掉（对所有主类型生效）：
// 任务文件的 `DisplayTitle "#L4D360UI_CampaignName_C1"` 不是标签，筛选条里上百个
// `#L4D360UI_*` 也没有任何可用性。
func dropLocalizationTokenTags(tags map[string]bool) int {
	dropped := 0
	for tag := range tags {
		if isLocalizationTokenTag(tag) {
			delete(tags, tag)
			dropped++
		}
	}
	return dropped
}
