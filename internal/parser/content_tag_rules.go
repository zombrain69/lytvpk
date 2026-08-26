package parser

import "strings"

// contentTagRule maps Source resource-path evidence to a Chinese secondary tag.
// Directory and file names inside a VPK are normally English; the tag is the
// localized label shown to the player.  Rules deliberately require a Source
// content root so a word in a workshop title or unrelated UI asset cannot
// classify the whole VPK.
type contentTagRule struct {
	tag      string
	prefixes []string
	keywords []string
	isItem   bool
}

var contentTagRules = []contentTagRule{
	// Medical, throwables and supplies. These are checked before broad weapon
	// directory evidence, because Valve stores several equipment world/view
	// models below models/*_models/weapons/.
	{"医疗包", []string{"models/", "materials/models/", "scripts/"}, []string{"eq_medkit", "medkit", "firstaid", "first_aid"}, true},
	{"电击器", []string{"models/", "materials/models/", "scripts/"}, []string{"defibrillator", "defib"}, true},
	{"止痛药", []string{"models/", "materials/models/", "scripts/"}, []string{"painpills", "pain_pills", "painpill"}, true},
	{"肾上腺", []string{"models/", "materials/models/", "scripts/"}, []string{"adrenaline", "adrenal_shot"}, true},
	{"土制炸弹", []string{"models/", "materials/models/", "scripts/"}, []string{"pipebomb", "pipe_bomb"}, true},
	{"燃烧瓶", []string{"models/", "materials/models/", "scripts/"}, []string{"molotov"}, true},
	{"胆汁", []string{"models/", "materials/models/", "scripts/"}, []string{"vomitjar", "bile_bomb", "boomer_bile", "bile_flask"}, true},
	{"汽油桶", []string{"models/", "materials/models/", "scripts/"}, []string{"gascan", "gas_can"}, true},
	{"煤气罐", []string{"models/", "materials/models/", "scripts/"}, []string{"propane"}, true},
	{"氧气罐", []string{"models/", "materials/models/", "scripts/"}, []string{"oxygen"}, true},
	{"烟花盒", []string{"models/", "materials/models/", "scripts/"}, []string{"firework"}, true},
	{"一代子弹堆", []string{"models/", "materials/models/", "scripts/"}, []string{"ammo_can", "ammocan", "ammo_can_03", "small_cabinet_ammo"}, true},
	{"二代子弹堆", []string{"models/", "materials/models/", "scripts/"}, []string{"ammo_pile", "ammopile", "ammo_stack", "ammostack", "coffeeammo", "ammo_crate", "ammopack", "eq_ammopack"}, true},
	{"燃烧弹盒", []string{"models/", "materials/models/", "scripts/"}, []string{"incendiary_ammo", "incendiary_ammopack", "eq_incendiary_ammopack", "weapon_upgradepack_incendiary"}, true},
	{"高爆弹盒", []string{"models/", "materials/models/", "scripts/"}, []string{"explosive_ammo", "explosive_ammopack", "eq_explosive_ammopack", "exploding_ammo", "weapon_upgradepack_explosive"}, true},
	{"激光瞄准盒", []string{"models/", "materials/models/", "scripts/"}, []string{"laser_sight", "lasersight", "laser_sights", "eq_laser_sights"}, true},

	// UI and player-facing presentation.
	{"主菜单", []string{"materials/vgui/", "resource/ui/"}, []string{"mainmenu", "main_menu", "menu_background"}, false},
	{"HUD", []string{"materials/vgui/hud/", "resource/ui/"}, []string{"hud"}, false},
	{"准星", []string{"materials/vgui/", "resource/ui/"}, []string{"crosshair"}, false},
	{"血条", []string{"materials/vgui/", "resource/ui/"}, []string{"healthbar", "health_bar"}, false},
	{"伤害指示器", []string{"materials/vgui/", "resource/ui/"}, []string{"damage_indicator", "damageindicator", "indicator"}, false},
	{"人物语音表", []string{"resource/ui/", "scripts/", "scenes/"}, []string{"vocalizer", "radial"}, false},
	{"语音包", []string{"sound/player/"}, nil, false},
	{"语音包", []string{"sound/", "scenes/"}, []string{"voice", "voicepack", "voice_pack"}, false},
	{"手电筒", []string{"models/", "materials/", "sound/", "scripts/"}, []string{"flashlight"}, false},
	{"梯子", []string{"models/", "materials/models/"}, []string{"ladder"}, false},
	{"天空", []string{"materials/skybox/", "models/props_skybox/"}, nil, false},
	{"过场画面", []string{"materials/vgui/", "resource/"}, []string{"transition"}, false},
	{"载入画面", []string{"materials/vgui/loadingscreen/"}, nil, false},
	{"尸潮", []string{"sound/", "scripts/"}, []string{"horde"}, false},
	{"动态箭头", []string{"materials/vgui/", "resource/overviews/"}, []string{"arrow"}, false},
	{"警报", []string{"sound/", "models/", "scripts/"}, []string{"alarm"}, false},
	{"唱片机", []string{"sound/", "models/", "materials/models/"}, []string{"jukebox"}, false},

	// World props and set dressing.
	{"侏儒", []string{"models/", "materials/models/"}, []string{"gnome"}, false},
	{"直升机", []string{"models/", "materials/models/"}, []string{"helicopter"}, false},
	{"海报", []string{"models/", "materials/", "resource/"}, []string{"poster"}, false},
	{"船", []string{"models/", "materials/models/"}, []string{"ship", "boat"}, false},
	{"售货机", []string{"models/", "materials/models/"}, []string{"vending"}, false},
	{"电视", []string{"models/", "materials/models/"}, []string{"television", "tv_", "/tv"}, false},
	{"屏幕", []string{"models/", "materials/models/"}, []string{"screen"}, false},
	{"货车", []string{"models/", "materials/models/"}, []string{"truck"}, false},
	{"面包车", []string{"models/", "materials/models/"}, []string{"van_", "/van"}, false},
	{"雕像", []string{"models/", "materials/models/"}, []string{"statue"}, false},
}

// collectContentTags returns whether the path is a known non-weapon item.
// This lets the type detector avoid classifying eq_medkit and similar assets as
// "武器" merely because Source keeps their models under a weapons directory.
func collectContentTags(name string, tags map[string]bool) bool {
	collectWorkshopContentCategories(name, tags)

	isItem := false
	for _, rule := range contentTagRules {
		if !rule.matches(name) {
			continue
		}
		tags[rule.tag] = true
		if rule.isItem {
			tags["物品"] = true
			if isThrowableItemTag(rule.tag) {
				tags["投掷物"] = true
				tags["所有投掷物品"] = true
			}
			if isMedicalItemTag(rule.tag) {
				tags["医疗物品"] = true
				tags["所有医疗物品"] = true
			}
			if isAmmoItemTag(rule.tag) {
				tags["弹药堆"] = true
				tags["盒子"] = true
			}
			if rule.tag == "燃烧弹盒" {
				tags["燃烧弹"] = true
				tags["盒子"] = true
			}
			if rule.tag == "高爆弹盒" {
				tags["高爆弹"] = true
				tags["盒子"] = true
			}
			if rule.tag == "激光瞄准盒" {
				tags["镭射"] = true
				tags["盒子"] = true
			}
		}
		isItem = isItem || rule.isItem
	}
	return isItem
}

// collectWorkshopContentCategories maps the broad content groups exposed by
// the Left 4 Dead 2 Workshop to localized filter tags.  A VPK may legitimately
// carry several groups, so these tags are additive evidence rather than a
// replacement for the primary type.
func collectWorkshopContentCategories(name string, tags map[string]bool) {
	if strings.HasPrefix(name, "resource/ui/") || strings.HasPrefix(name, "materials/vgui/") {
		tags["UI"] = true
	}
	if strings.HasPrefix(name, "sound/") || strings.HasPrefix(name, "scenes/") {
		tags["声音"] = true
	}
	if strings.HasPrefix(name, "scripts/") {
		tags["脚本"] = true
	}
	if strings.HasPrefix(name, "models/") {
		tags["模型"] = true
	}
	if strings.HasPrefix(name, "materials/") {
		tags["贴图"] = true
	}
}

func isThrowableItemTag(tag string) bool {
	switch tag {
	case "土制炸弹", "燃烧瓶", "胆汁":
		return true
	default:
		return false
	}
}

func isMedicalItemTag(tag string) bool {
	switch tag {
	case "医疗包", "电击器", "止痛药", "肾上腺":
		return true
	default:
		return false
	}
}

func isAmmoItemTag(tag string) bool {
	switch tag {
	case "一代子弹堆", "二代子弹堆":
		return true
	default:
		return false
	}
}

func (rule contentTagRule) matches(name string) bool {
	if !hasPathPrefix(name, rule.prefixes) {
		return false
	}
	if len(rule.keywords) == 0 {
		return true
	}
	for _, keyword := range rule.keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

func hasPathPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
