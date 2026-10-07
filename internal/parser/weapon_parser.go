package parser

import (
	"strings"

	"vpk-manager/internal/ruletable"
)

// ProcessWeaponVPK 处理武器类型VPK
func ProcessWeaponVPK(index archivePathIndex, vpkFile *VPKFile, secondaryTags map[string]bool) {
	vpkFile.PrimaryTag = "武器"

	// addoninfo 是补充证据；文件路径的具体武器名仍会继续收集，使武器包可显示多个标签。
	if strings.TrimSpace(vpkFile.Title) != "" || strings.TrimSpace(vpkFile.Desc) != "" {
		DetectWeaponTypeFromMetadata(vpkFile.Title+" "+vpkFile.Desc, secondaryTags)
	}

	collectWeaponPathTags(index, secondaryTags)
}

// collectWeaponPathTags records concrete weapon evidence without changing the
// VPK's primary type.  It is deliberately based on recognized L4D2 resource
// names instead of merely seeing a materials/models/weapons directory: generic
// texture names in that directory otherwise create false labels for mixed mods.
func collectWeaponPathTags(index archivePathIndex, secondaryTags map[string]bool) {
	foundConcreteWeapon := false

	// 仅处理建索引时确认的武器资源，不再扫描整个 archive.Files。
	for _, entry := range index.weaponFiles {
		if tag := weaponPathTag(entry.name); tag != "" {
			addWeaponTag(tag, secondaryTags)
			index.evidence.record(tag, "weapon:"+tag, EvidenceLevelInferred, entry.name)
			foundConcreteWeapon = true
		}
	}

	if foundConcreteWeapon {
		secondaryTags["武器"] = true
	}
}

type weaponMatchRule struct {
	keyword string
	tag     string
	// tokenMatch：关键词两侧必须是词边界（见 ruletable.MatchRule.Match）。
	tokenMatch bool
	// pathOnly：只参与"武器资源路径"通道，不做标题/描述推断（见 ruletable.MatchRule.Scope）。
	pathOnly bool
}

var weaponPathRules = buildWeaponMatchRules(ruletable.MustLoad().WeaponPathRules)

// weaponMetadataRules 是标题/描述侧的武器关键词表（切片顺序即优先级）。
// 提到包级是为了让 W4 的跨类型标题通道复用同一份数据，而不是在函数里私藏一份。
var weaponMetadataRules = buildWeaponMatchRules(ruletable.MustLoad().WeaponMetadataRules)

// buildWeaponMatchRules 把规则表（JSON）转成解析器内部使用的形态。
func buildWeaponMatchRules(rules []ruletable.MatchRule) []weaponMatchRule {
	out := make([]weaponMatchRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, weaponMatchRule{
			keyword:    rule.Keyword,
			tag:        rule.Tag,
			tokenMatch: strings.EqualFold(strings.TrimSpace(rule.Match), "token"),
			pathOnly:   strings.EqualFold(strings.TrimSpace(rule.Scope), "path"),
		})
	}
	return out
}

// DetectWeaponTypeFromMetadata 根据addoninfo的文本检测武器类型
func DetectWeaponTypeFromMetadata(text string, secondaryTags map[string]bool) {
	lowerText := strings.ToLower(text)

	for _, rule := range weaponMetadataRules {
		if rule.pathOnly {
			continue
		}
		if metadataRuleMatchesWeaponRule(lowerText, rule) {
			// D7：不再「命中即 return」——标题里写了多个型号时，每个型号都要出标签。
			addWeaponTag(rule.tag, secondaryTags)
		}
	}
}

// metadataRuleMatchesWeaponRule 按规则自己的匹配口径判断标题/描述命中。
// token 规则的边界口径与解析器其它通道保持一致（`\b` 在 PowerShell 之外不可移植，
// 这里直接复用 textContainsToken）。
func metadataRuleMatchesWeaponRule(lowerText string, rule weaponMatchRule) bool {
	if rule.tokenMatch {
		return textContainsToken(lowerText, rule.keyword)
	}
	if rule.keyword == "scar" {
		// 历史特例：scar 必须整词，否则 oscar 会命中。
		return textContainsToken(lowerText, rule.keyword)
	}
	return strings.Contains(lowerText, rule.keyword)
}

// DetectWeaponType 检测武器类型
func DetectWeaponType(filename string, secondaryTags map[string]bool) {
	if tag := weaponPathTag(filename); tag != "" {
		addWeaponTag(tag, secondaryTags)
		return
	}
	// 关键词表没有覆盖时，用本体脚本声明的精确锚点兜底：
	// 例如 models/weapons/melee/w_golfclub.mdl、models/w_models/weapons/w_pumpshotgun_A.mdl
	// 这些路径按"命名直觉"猜不出来，但脚本里写得清清楚楚。
	for _, hit := range stockEntities.Lookup(filename) {
		if hit.Character != "" || hit.IsItem || hit.Tag == "" {
			continue
		}
		addWeaponTag(hit.Tag, secondaryTags)
	}
}

func addWeaponTag(tag string, secondaryTags map[string]bool) {
	tag = canonicalWeaponTag(tag)
	if tag == "" {
		return
	}
	secondaryTags[tag] = true
	if category := weaponCategoryTag(tag); category != "" {
		secondaryTags[category] = true
		if category == "近战" && isOfficialMeleeTag(tag) {
			secondaryTags["官方近战"] = true
			secondaryTags["所有官方近战"] = true
		}
		if isFirearmCategory(category) {
			secondaryTags["所有枪械"] = true
		}
	}
}

func canonicalWeaponTag(tag string) string {
	switch strings.ToLower(strings.TrimSpace(tag)) {
	case "榴弹", "榴弹发射器":
		return "榴弹发射器"
	default:
		return strings.TrimSpace(tag)
	}
}

// isOfficialMeleeTag separates the stock L4D2 melee set from hidden/custom
// melee names.  Knife and Riot Shield are intentionally kept as ordinary
// "近战" tags, but are not included in the official-melee aggregate filter.
func isOfficialMeleeTag(tag string) bool {
	switch tag {
	case "棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "消防斧", "砍刀", "武士刀", "电锯", "撬棍", "草叉", "铁铲", "警棍":
		return true
	default:
		return false
	}
}

func isFirearmCategory(category string) bool {
	switch category {
	case "手枪", "步枪", "狙击枪", "霰弹枪", "冲锋枪", "榴弹发射器", "M60", "固定机关枪":
		return true
	default:
		return false
	}
}

func weaponCategoryTag(tag string) string {
	switch tag {
	case "AK47", "M16", "三连发", "sg552":
		return "步枪"
	case "大狙", "猎枪", "军狙", "鸟狙":
		return "狙击枪"
	case "木喷", "一代连喷", "铁喷", "二代连喷":
		return "霰弹枪"
	case "乌兹", "消音", "MP5":
		return "冲锋枪"
	case "小手枪", "马格南":
		return "手枪"
	case "榴弹", "榴弹发射器":
		return "榴弹发射器"
	case "M60":
		return "M60"
	case "固定机关枪", "一代固定机枪", "二代固定机枪":
		// 固定机枪按本体模型细分成一代（Minigun，L4D1）/ 二代（Heavy MG，L4D2），
		// 「固定机关枪」保留为聚合标签：筛"固定机关枪"仍然两代都能看到。
		return "固定机关枪"
	case "棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "消防斧", "砍刀", "武士刀", "电锯", "撬棍", "草叉", "铁铲", "警棍", "防爆盾", "匕首":
		return "近战"
	default:
		return ""
	}
}

func weaponPathTag(filename string) string {
	lowerFilename := strings.ToLower(filename)
	// 本体反例（D1）：models/w_models/weapons/w_pumpshotgun_A.mdl 是 Chrome 连喷的世界模型
	// （见 scripts/weapon_shotgun_chrome.txt 的 playermodel）。名字里的 "pumpshotgun"/"shotgun"
	// 是历史命名巧合，任何把它判成「木喷」的关键词都要跳过；木喷自己的路径 w_shotgun.mdl 不受影响。
	chromeWorldModel := strings.Contains(lowerFilename, "w_pumpshotgun_a")
	// 本体共享路径：sound/weapons/shotgun/** 与 materials/**/weapons/shotgun/** 是泵动 /
	// Chrome / 连喷等多把霰弹枪共用的音效与素材目录，出现泛词 "shotgun" 不足以断定是木喷
	// （真机残留：2 个 Chrome 包靠 shotgun_pump_1.wav / .../shotgun/qx.vmt 被误标木喷）。
	sharedShotgunPath := strings.Contains(lowerFilename, "sound/weapons/shotgun/") ||
		strings.Contains(lowerFilename, "materials/models/v_models/weapons/shotgun/")
	for _, rule := range weaponPathRules {
		if chromeWorldModel && rule.tag == "木喷" {
			continue
		}
		// 共享目录对**整个木喷标签**生效：引擎把 sound/weapons/shotgun/** 等目录
		// 用于多把霰弹枪（泵动/Chrome/连喷），任何把它判成木喷的关键词都只是猜测。
		if sharedShotgunPath && rule.tag == "木喷" {
			continue
		}
		if weaponRuleMatchesWithMode(lowerFilename, rule) {
			return rule.tag
		}
	}
	return ""
}

func weaponRuleMatches(path string, keyword string) bool {
	return weaponRuleMatchesWithMode(path, weaponMatchRule{keyword: keyword})
}

// weaponRuleMatchesWithMode 是带匹配口径的版本：token 规则要求关键词两侧是
// 词边界，避免 `m60` 命中 `m600v`、`knife` 命中作者命名空间里的 `knife_annihil`。
func weaponRuleMatchesWithMode(path string, rule weaponMatchRule) bool {
	if !rule.tokenMatch && rule.keyword != "scar" {
		return strings.Contains(path, rule.keyword)
	}
	return pathContainsToken(path, rule.keyword)
}

// pathContainsToken 要求 token 两侧不是 [a-z0-9]（`_` / `.` / `/` / `-` 都算边界，
// 于是 w_m60.mdl、m60_box5.wav 命中，而 m600v.vmt 不命中）。
func pathContainsToken(path, token string) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	from := 0
	for {
		index := strings.Index(path[from:], token)
		if index < 0 {
			return false
		}
		start := from + index
		end := start + len(token)
		leftOK := start == 0 || !isTokenBoundaryRune(rune(path[start-1]))
		rightOK := end >= len(path) || !isTokenBoundaryRune(rune(path[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
}

func isTokenBoundaryRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}
