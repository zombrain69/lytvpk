package parser

import (
	"strings"

	"vpk-manager/internal/ruletable"
)

var survivorVoiceDirectoryRules = buildVoiceDirRules(ruletable.MustLoad().CharacterRules.VoiceDirSurvivor)

var infectedVoiceDirectoryRules = buildVoiceDirRules(ruletable.MustLoad().CharacterRules.VoiceDirInfected)

// buildVoiceDirRules 把"语音目录段 → 角色"映射从规则表转成解析器内部形态。
func buildVoiceDirRules(rules []ruletable.VoiceDirRule) map[string]string {
	out := make(map[string]string, len(rules))
	for _, rule := range rules {
		if rule.Dir == "" || rule.Character == "" {
			continue
		}
		out[rule.Dir] = rule.Character
	}
	return out
}

// detectVoiceCharacter identifies the character slot from the standard Source
// voice directory, e.g. sound/player/survivor/voice/coach/*.wav.  The slot is
// authoritative for voice replacements; individual filenames often contain
// references to other survivors (youarewelcomeproducer01.wav, etc.) and must
// not be treated as evidence for another character.
func detectVoiceCharacter(name string) string {
	parts := strings.Split(strings.Trim(strings.ToLower(name), "/"), "/")
	if len(parts) < 4 || parts[0] != "sound" {
		return ""
	}
	// 语音有两套根：sound/player/**（玩家角色）与 sound/npc/**（如 Witch 的 npc 语音）。
	if parts[1] != "player" && parts[1] != "npc" {
		return ""
	}

	// 幸存者：sound/player/survivor/voice/<角色目录>/...
	if parts[2] == "survivor" && len(parts) >= 5 && parts[3] == "voice" {
		return survivorVoiceDirectoryRules[parts[4]]
	}
	// 历史假设（本体里并不存在，保留兼容作者自建目录）：sound/player/infected/voice/<特感>/...
	if parts[2] == "infected" && len(parts) >= 5 && parts[3] == "voice" {
		return infectedVoiceDirectoryRules[parts[4]]
	}

	// 本体真实布局（D5）：sound/player/<特感>/{voice,attack,hit,death,idle,miss,alert,...}/...
	// 实测目录：boomer charger hunter jockey smoker spitter tank pz footsteps survivor items water。
	if character := infectedVoiceDirectoryRules[parts[2]]; character != "" {
		return character
	}
	// 第三种布局（本体真实路径）：sound/npc/<class>/voice/…，例如 Witch 的鬼叫。
	if parts[1] == "npc" && len(parts) >= 4 {
		if character := infectedVoiceDirectoryRules[parts[2]]; character != "" {
			return character
		}
	}
	if parts[2] == "pz" {
		// pz = "player zombie"，普通感染者的语音目录。
		return "Common Infected"
	}
	return ""
}

func isInfectedVoiceCharacter(character string) bool {
	switch character {
	case "Boomer", "Charger", "Hunter", "Jockey", "Smoker", "Spitter", "Tank", "Witch", "Common Infected":
		return true
	default:
		return false
	}
}

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
	for character := range index.voiceCharacters {
		if character == "Common Infected" {
			secondaryTags["common"] = true
			secondaryTags["普通感染者"] = true
			index.evidence.record("普通感染者", "character:"+character, EvidenceLevelPattern, "")
			continue
		}
		if isInfectedVoiceCharacter(character) {
			secondaryTags[strings.ToLower(character)] = true
			secondaryTags["特殊感染者"] = true
			index.evidence.record(character, "character:"+character, EvidenceLevelPattern, "")
			index.evidence.record("特殊感染者", "character:"+character, EvidenceLevelPattern, "")
			continue
		}
		secondaryTags["幸存者"] = true
		secondaryTags[character] = true
		index.evidence.record(character, "character:"+character, EvidenceLevelPattern, "")
		index.evidence.record("幸存者", "character:"+character, EvidenceLevelPattern, "")
	}

	// 只处理建立目录索引时确认的角色资源，避免再次遍历整个 VPK。
	for _, entry := range index.characterFiles {
		filename := entry.name
		if detectVoiceCharacter(filename) != "" {
			continue
		}

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

var survivorVariantRules = buildCharacterMatchRules(ruletable.MustLoad().CharacterRules.SurvivorVariants)

var survivorRules = buildCharacterMatchRules(ruletable.MustLoad().CharacterRules.Survivors)

var specialInfectedRules = buildCharacterMatchRules(ruletable.MustLoad().CharacterRules.SpecialInfected)

var commonInfectedRules = buildCharacterMatchRules(ruletable.MustLoad().CharacterRules.CommonInfected)

// buildCharacterMatchRules 把角色关键词规则从规则表转成解析器内部形态（顺序即优先级）。
func buildCharacterMatchRules(rules []ruletable.MatchRule) []characterMatchRule {
	out := make([]characterMatchRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, characterMatchRule{keyword: rule.Keyword, tag: rule.Tag})
	}
	return out
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
