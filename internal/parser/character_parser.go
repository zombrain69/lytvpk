package parser

import (
	"strings"
)

// ProcessCharacterVPK 处理人物类型VPK
func ProcessCharacterVPK(index archivePathIndex, vpkFile *VPKFile, secondaryTags map[string]bool) {
	vpkFile.PrimaryTag = "人物"
	collectCharacterTags(index, secondaryTags)
}

// collectCharacterTags adds character evidence without changing the VPK's
// primary type.  This allows a map or weapon pack that also replaces a
// survivor/infected model to stay in its primary category while remaining
// discoverable through Workshop-style character filters.
func collectCharacterTags(index archivePathIndex, secondaryTags map[string]bool) {
	// 只处理建立目录索引时确认的角色资源，避免再次遍历整个 VPK。
	for _, entry := range index.characterFiles {
		filename := entry.name

		// 幸存者检测
		if strings.Contains(filename, "survivor") {
			secondaryTags["幸存者"] = true
			DetectSurvivorType(filename, secondaryTags)
		}

		// 感染者检测
		if strings.Contains(filename, "infected") || strings.Contains(filename, "zombie") {
			DetectInfectedType(filename, secondaryTags)
		}
	}
}

type characterMatchRule struct {
	keyword string
	tag     string
}

var survivorVariantRules = []characterMatchRule{
	{"bill_death", "BillDeathPose"},
	{"billdeath", "BillDeathPose"},
	{"bill_corpse", "BillDeathPose"},
	{"billcorpse", "BillDeathPose"},
	{"francis_flashlight", "FrancisLight"},
	{"francisflashlight", "FrancisLight"},
	{"francis_light", "FrancisLight"},
	{"francislight", "FrancisLight"},
	{"zoey_flashlight", "ZoeyLight"},
	{"zoeyflashlight", "ZoeyLight"},
	{"zoey_light", "ZoeyLight"},
	{"zoeylight", "ZoeyLight"},
}

var survivorRules = []characterMatchRule{
	{"namvet", "Bill"},
	{"bill", "Bill"},
	{"biker", "Francis"},
	{"francis", "Francis"},
	{"manager", "Louis"},
	{"louis", "Louis"},
	{"teenangst", "Zoey"},
	{"zoey", "Zoey"},
	{"coach", "Coach"},
	{"mechanic", "Ellis"},
	{"ellis", "Ellis"},
	{"gambler", "Nick"},
	{"nick", "Nick"},
	{"producer", "Rochelle"},
	{"rochelle", "Rochelle"},
}

var specialInfectedRules = []characterMatchRule{
	{"charger", "charger"},
	{"jockey", "jockey"},
	{"spitter", "spitter"},
	{"smoker", "smoker"},
	{"boomer", "boomer"},
	{"hunter", "hunter"},
	{"witch", "witch"},
	{"hulk", "tank"},
	{"tank", "tank"},
}

var commonInfectedRules = []characterMatchRule{
	{"uncommon", "uncommon_infected"},
	{"roadcrew", "uncommon_infected"},
	{"fallen", "uncommon_infected"},
	{"ceda", "uncommon_infected"},
	{"clown", "uncommon_infected"},
	{"jimmy", "uncommon_infected"},
	{"riot", "uncommon_infected"},
	{"mud", "uncommon_infected"},
	{"common", "common"},
	{"zombie", "common"},
	{"infected", "common"},
}

// DetectSurvivorType 检测幸存者类型 - 基于NekoVpk识别模式
func DetectSurvivorType(filename string, secondaryTags map[string]bool) {
	lowerFilename := strings.ToLower(filename)

	for _, rule := range survivorVariantRules {
		if strings.Contains(lowerFilename, rule.keyword) {
			secondaryTags[rule.tag] = true
			return
		}
	}

	for _, rule := range survivorRules {
		if strings.Contains(lowerFilename, rule.keyword) {
			secondaryTags[rule.tag] = true
			return
		}
	}
}

// DetectInfectedType 检测感染者类型 - 基于NekoVpk模式
func DetectInfectedType(filename string, secondaryTags map[string]bool) {
	lowerFilename := strings.ToLower(filename)

	for _, rule := range specialInfectedRules {
		if strings.Contains(lowerFilename, rule.keyword) {
			secondaryTags[rule.tag] = true
			secondaryTags["特殊感染者"] = true
			return
		}
	}

	for _, rule := range commonInfectedRules {
		if strings.Contains(lowerFilename, rule.keyword) {
			secondaryTags[rule.tag] = true
			secondaryTags["普通感染者"] = true
			return
		}
	}
}
