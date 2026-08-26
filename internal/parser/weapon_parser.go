package parser

import (
	"regexp"
	"strings"
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
}

var weaponPathRules = []weaponMatchRule{
	// 具体资源名优先。这些兼容规则来自本项目早期实现和 L4D2 的原版
	// pak01 目录；按切片顺序匹配，避免 Go map 遍历导致的误判。
	// 手枪：具体文件名必须先于宽泛的 pistol 规则。
	{"w_desert_eagle", "马格南"},
	{"pistol_magnum", "马格南"},
	{"desert_eagle", "马格南"},
	{"deserteagle", "马格南"},
	{"magnum", "马格南"},
	{"w_pistol_glock", "小手枪"},
	{"pistol_glock", "小手枪"},
	{"w_pistol_b", "小手枪"},
	{"glock", "小手枪"},
	{"p220", "小手枪"},
	{"pistol", "小手枪"},

	// 步枪。
	{"rifle_desert", "三连发"},
	{"desert_rifle", "三连发"},
	{"combat_rifle", "三连发"},
	{"scar", "三连发"},
	{"rifle_ak47", "AK47"},
	{"ak47", "AK47"},
	{"rifle_m16a2", "M16"},
	{"rifle_m16", "M16"},
	{"m16a2", "M16"},
	{"m16", "M16"},
	{"m4a1", "M16"},
	{"rifle_sg552", "sg552"},
	{"sg552", "sg552"},
	{"rifle_m60", "M60"},
	{"m60", "M60"},

	// 狙击枪。
	{"sniper_awp", "大狙"},
	{"w_sniper_mini14", "猎枪"},
	{"hunting_rifle", "猎枪"},
	{"sniper_military", "军狙"},
	{"msg90", "军狙"},
	{"g3sg1", "军狙"},
	{"sniper_a", "军狙"},
	{"sniper_scout", "鸟狙"},
	{"scout", "鸟狙"},
	{"awp", "大狙"},

	// 霰弹枪。
	{"shotgun_chrome", "铁喷"},
	{"w_shotgun_m1014", "一代连喷"},
	{"w_autoshot_m4super", "一代连喷"},
	{"autoshotgun", "一代连喷"},
	{"autoshot", "一代连喷"},
	{"m1014", "一代连喷"},
	{"shotgun_spas", "二代连喷"},
	{"spas", "二代连喷"},
	{"chrome", "铁喷"},
	{"shotgun_pump", "木喷"},
	{"pumpshotgun", "木喷"},
	{"w_shotgun", "木喷"},
	{"shotgun", "木喷"},

	// 冲锋枪。
	{"smg_silenced", "消音"},
	{"mac10", "消音"},
	{"mac_10", "消音"},
	{"w_smg_mp5", "MP5"},
	{"smg_mp5", "MP5"},
	{"smg_uzi", "乌兹"},
	{"w_smg_uzi", "乌兹"},
	{"smg_a", "消音"},
	{"mp5", "MP5"},
	{"uzi", "乌兹"},
	{"smg", "乌兹"},

	// 发射器。
	{"grenade_launcher", "榴弹发射器"},
	{"50cal", "固定机关枪"},
	{"minigun", "固定机关枪"},

	// 近战武器。
	{"baseball_bat", "棒球棍"},
	{"cricket_bat", "板球拍"},
	{"electric_guitar", "吉他"},
	{"frying_pan", "平底锅"},
	{"golf_club", "高尔夫球杆"},
	{"fireaxe", "消防斧"},
	{"machete", "砍刀"},
	{"katana", "武士刀"},
	{"chainsaw", "电锯"},
	{"crowbar", "撬棍"},
	{"pitchfork", "草叉"},
	{"shovel", "铁铲"},
	{"tonfa", "警棍"},
	{"nightstick", "警棍"},
	{"riot_shield", "防爆盾"},
	{"riotshield", "防爆盾"},
	{"melee_knife", "匕首"},
	{"w_knife_t", "匕首"},
	{"knife", "匕首"},
	{"w_chainsaw", "电锯"},
	{"w_crowbar", "撬棍"},
	{"w_fireaxe", "消防斧"},
	{"w_frying_pan", "平底锅"},
	{"w_guitar", "吉他"},
	{"w_bat", "棒球棍"},
}

// DetectWeaponTypeFromMetadata 根据addoninfo的文本检测武器类型
func DetectWeaponTypeFromMetadata(text string, secondaryTags map[string]bool) {
	lowerText := strings.ToLower(text)

	// 使用切片来保证匹配顺序
	type matchRule struct {
		keyword string
		tag     string
	}

	rules := []matchRule{
		// 步枪
		{"ak47", "AK47"},
		{"ak-47", "AK47"},
		{"m4a1", "M16"},
		{"m16", "M16"},
		{"sg552", "sg552"},
		{"scar", "三连发"},
		{"combat rifle", "三连发"},
		{"combat-rifle", "三连发"},
		{"desert rifle", "三连发"},
		{"desert-rifle", "三连发"},
		{"m60", "M60"},

		// 冲锋枪
		{"uzi", "乌兹"},
		{"silenced smg", "消音"},
		{"silenced-smg", "消音"},
		{"mac 10", "消音"},
		{"mac-10", "消音"},
		{"mac10", "消音"},
		{"mp5", "MP5"},

		// 狙击枪
		{"hunting rifle", "猎枪"},
		{"hunting-rifle", "猎枪"},
		{"mini14", "猎枪"},
		{"military sniper", "军狙"},
		{"military-sniper", "军狙"},
		{"scout", "鸟狙"},
		{"awp", "大狙"},

		// 霰弹枪
		{"m1014", "一代连喷"},
		{"chrome", "铁喷"},
		{"pump shotgun", "木喷"},
		{"pump-shotgun", "木喷"},
		{"auto shotgun", "一代连喷"},
		{"auto-shotgun", "一代连喷"},
		{"autoshotgun", "一代连喷"},
		{"spas", "二代连喷"},

		// 手枪
		{"magnum", "马格南"},
		{"desert eagle", "马格南"},
		{"desert-eagle", "马格南"},
		{"glock", "小手枪"},
		{"p220", "小手枪"},
		{"pistol", "小手枪"},

		// 发射器
		{"grenade launcher", "榴弹发射器"},
		{"grenade-launcher", "榴弹发射器"},

		// 近战武器
		{"machete", "砍刀"},
		{"katana", "武士刀"},
		{"baseball bat", "棒球棍"},
		{"knife", "匕首"},
		{"chainsaw", "电锯"},
		{"crowbar", "撬棍"},
		{"fireaxe", "消防斧"},
		{"frying pan", "平底锅"},
		{"guitar", "吉他"},
		{"cricket bat", "板球拍"},
		{"tonfa", "警棍"},
		{"nightstick", "警棍"},
		{"golf club", "高尔夫球杆"},
		{"shovel", "铁铲"},
		{"pitchfork", "草叉"},
	}

	for _, rule := range rules {
		isMatch := false
		if rule.keyword == "scar" {
			// 特殊处理 scar，防止匹配到 oscar 等词
			isMatch, _ = regexp.MatchString(`\bscar\b`, lowerText)
		} else {
			isMatch = strings.Contains(lowerText, rule.keyword)
		}

		if isMatch {
			addWeaponTag(rule.tag, secondaryTags)
			return
		}
	}
}

// DetectWeaponType 检测武器类型
func DetectWeaponType(filename string, secondaryTags map[string]bool) {
	if tag := weaponPathTag(filename); tag != "" {
		addWeaponTag(tag, secondaryTags)
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
	case "固定机关枪":
		return "固定机关枪"
	case "棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "消防斧", "砍刀", "武士刀", "电锯", "撬棍", "草叉", "铁铲", "警棍", "防爆盾", "匕首":
		return "近战"
	default:
		return ""
	}
}

func weaponPathTag(filename string) string {
	lowerFilename := strings.ToLower(filename)
	for _, rule := range weaponPathRules {
		if weaponRuleMatches(lowerFilename, rule.keyword) {
			return rule.tag
		}
	}
	return ""
}

func weaponRuleMatches(path string, keyword string) bool {
	if keyword != "scar" {
		return strings.Contains(path, keyword)
	}
	for _, part := range strings.FieldsFunc(path, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9')
	}) {
		if part == "scar" {
			return true
		}
	}
	return false
}
