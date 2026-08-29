package parser

import (
	"sort"
	"strings"
)

// subjectEvidence is deliberately kept internal.  It is the path-level
// evidence used to build the concise subject fields exposed by VPKFile.
// score counts corroborating files, while confidence prevents a large generic
// audio/materials folder from outweighing one standard, concrete resource path.
type subjectEvidence struct {
	label      string
	category   string
	score      int
	files      int
	confidence int // 2 = high, 1 = medium, 0 = low
}

var specificSubjectTags = map[string]struct {
	label    string
	category string
	weight   int
}{
	"主菜单":   {"主菜单界面", "界面", 5},
	"HUD":   {"HUD", "界面", 5},
	"准星":    {"准星", "界面", 5},
	"血条":    {"血条", "界面", 5},
	"伤害指示器": {"伤害指示器", "界面", 5},
	"人物语音表": {"语音菜单", "界面", 5},
	"过场画面":  {"过场画面", "界面", 5},
	"载入画面":  {"载入画面", "界面", 5},
	"动态箭头":  {"动态箭头", "界面", 5},
	"手电筒":   {"手电筒", "场景物件", 5},
	"梯子":    {"梯子", "场景物件", 5},
	"天空":    {"天空盒", "场景物件", 5},
	"侏儒":    {"侏儒", "场景物件", 5},
	"直升机":   {"直升机", "场景物件", 5},
	"海报":    {"海报", "场景物件", 5},
	"船":     {"船", "场景物件", 5},
	"售货机":   {"售货机", "场景物件", 5},
	"电视":    {"电视", "场景物件", 5},
	"屏幕":    {"屏幕", "场景物件", 5},
	"货车":    {"货车", "场景物件", 5},
	"面包车":   {"面包车", "场景物件", 5},
	"雕像":    {"雕像", "场景物件", 5},
	"尸潮":    {"尸潮音效", "声音", 5},
	"警报":    {"警报音效", "声音", 5},
	"唱片机":   {"唱片机音效", "声音", 5},
}

// collectSubjectEvidence runs during the same archive.Files pass as the
// existing type/tag index.  Rules are intentionally ordered by authority:
// standard slot directories and concrete stock resource names first, then
// specific UI/scene rules, and finally generic resource families.
func collectSubjectEvidence(name string, isItem bool, index *archivePathIndex) {
	lower := strings.ToLower(name)

	if strings.HasSuffix(lower, ".bsp") {
		recordSubject(index, "地图资源", "地图", 10, 2)
	}

	if character := detectVoiceCharacter(lower); character != "" {
		recordSubject(index, character+" 语音", "角色", 8, 2)
	}

	if character := detectCharacterSubject(lower); character != "" {
		recordSubject(index, character+" 模型", "角色", 7, 2)
	}

	if isItem {
		for _, tag := range detectItemSubjectTags(lower) {
			recordSubject(index, tag, "物品", 8, 2)
		}
	}

	weaponEvidence := isWeaponAssetPath(lower, isItem)
	if !isItem && weaponEvidence {
		if weapon := weaponPathTag(lower); weapon != "" {
			recordSubject(index, weapon+" 武器", "武器", 7, 2)
		}
	}

	for tag, rule := range specificSubjectTags {
		if contentRuleMatchesTag(lower, tag) {
			recordSubject(index, rule.label, rule.category, rule.weight, 2)
		}
	}

	// A sound/player entry without a recognized slot is still useful to the
	// player, but it must not be presented as a specific survivor voice pack.
	if (strings.HasPrefix(lower, "sound/") || strings.HasPrefix(lower, "scenes/")) &&
		detectVoiceCharacter(lower) == "" && !weaponEvidence {
		recordSubject(index, "泛音频资源", "通用", 1, 0)
	}
	if strings.HasPrefix(lower, "models/") && detectCharacterSubject(lower) == "" && !weaponEvidence && !isItem {
		recordSubject(index, "泛模型资源", "通用", 1, 0)
	}
	if strings.HasPrefix(lower, "materials/") && detectCharacterSubject(lower) == "" && !weaponEvidence && !isItem {
		recordSubject(index, "泛材质资源", "通用", 1, 0)
	}
	if strings.HasPrefix(lower, "scripts/") && !isMetadataScriptPath(lower) && !weaponEvidence && !isItem {
		recordSubject(index, "脚本资源", "通用", 1, 0)
	}
}

func isMetadataScriptPath(name string) bool {
	base := name
	if slash := strings.LastIndexByte(base, '/'); slash >= 0 {
		base = base[slash+1:]
	}
	return strings.HasPrefix(base, "game_sounds") ||
		strings.HasPrefix(base, "soundscapes") ||
		strings.HasPrefix(base, "soundscript")
}

func recordSubject(index *archivePathIndex, label, category string, weight, confidence int) {
	label = strings.TrimSpace(label)
	if label == "" {
		return
	}
	if index.subjectEvidence == nil {
		index.subjectEvidence = make(map[string]subjectEvidence)
	}
	evidence := index.subjectEvidence[label]
	evidence.label = label
	evidence.category = category
	evidence.score += weight
	evidence.files++
	if confidence > evidence.confidence {
		evidence.confidence = confidence
	}
	index.subjectEvidence[label] = evidence
}

func detectItemSubjectTags(name string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, 2)
	for _, rule := range contentTagRules {
		if !rule.isItem || !rule.matches(name) || seen[rule.tag] {
			continue
		}
		seen[rule.tag] = true
		result = append(result, rule.tag)
	}
	return result
}

func contentRuleMatchesTag(name, tag string) bool {
	// Character voice packs often keep line names such as alarm/jukebox in the
	// standard sound/player tree.  Those words describe the line text, not the
	// replaced object, so never promote them to scene/audio subjects here.
	if strings.HasPrefix(name, "sound/player/") || strings.HasPrefix(name, "scenes/") {
		switch tag {
		case "尸潮", "警报", "唱片机", "手电筒", "梯子", "天空":
			return false
		}
	}
	for _, rule := range contentTagRules {
		if rule.tag == tag && rule.matches(name) {
			return true
		}
	}
	return false
}

func detectCharacterSubject(name string) string {
	if !isCharacterSubjectPath(name) {
		return ""
	}
	for _, token := range pathTokens(name) {
		if character := survivorVoiceDirectoryRules[token]; character != "" {
			return character
		}
		if character := infectedVoiceDirectoryRules[token]; character != "" {
			return character
		}
	}
	return ""
}

func isCharacterSubjectPath(name string) bool {
	for _, root := range []string{
		"models/survivors/", "materials/models/survivors/", "materials/survivors/",
		"models/infected/", "materials/models/infected/", "materials/infected/",
		"models/zombie/", "materials/models/zombie/", "materials/zombie/",
	} {
		if strings.HasPrefix(name, root) {
			return true
		}
	}
	return false
}

func pathTokens(name string) []string {
	return strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
}

// buildSubjectInfo converts path evidence into the stable JSON fields shown by
// the UI.  Generic families are used only when no concrete subject survives
// the confidence threshold, making uncertainty explicit instead of guessing.
func buildSubjectInfo(index archivePathIndex, vpkFile *VPKFile) {
	evidence := make(map[string]subjectEvidence, len(index.subjectEvidence)+1)
	for label, item := range index.subjectEvidence {
		evidence[label] = item
	}

	mapSubject := ""
	if index.hasMap {
		delete(evidence, "地图资源")
		if campaign := strings.TrimSpace(vpkFile.Campaign); campaign != "" {
			mapSubject = "战役地图：" + campaign
		} else if len(index.missionFiles) > 0 {
			mapSubject = "地图资源（BSP + 战役配置）"
		} else {
			mapSubject = "地图资源（BSP）"
		}
		evidence[mapSubject] = subjectEvidence{label: mapSubject, category: "地图", score: 12, files: 1, confidence: 2}
	}

	concrete := make([]subjectEvidence, 0, len(evidence))
	for _, item := range evidence {
		if item.confidence >= 2 && item.score >= 5 {
			concrete = append(concrete, item)
		}
	}
	if len(concrete) == 0 {
		for _, item := range evidence {
			if item.confidence >= 1 && item.score >= 3 {
				concrete = append(concrete, item)
			}
		}
	}
	if len(concrete) == 0 {
		for _, item := range evidence {
			if item.confidence == 0 {
				concrete = append(concrete, item)
			}
		}
	}

	sort.Slice(concrete, func(i, j int) bool {
		if concrete[i].score != concrete[j].score {
			return concrete[i].score > concrete[j].score
		}
		return concrete[i].label < concrete[j].label
	})
	if len(concrete) > 6 {
		concrete = concrete[:6]
	}

	vpkFile.ContentSubjects = make([]string, 0, len(concrete))
	maxConfidence := 0
	for _, item := range concrete {
		vpkFile.ContentSubjects = append(vpkFile.ContentSubjects, item.label)
		if item.confidence > maxConfidence {
			maxConfidence = item.confidence
		}
	}

	switch {
	case len(concrete) == 0:
		vpkFile.SubjectConfidence = "低"
		vpkFile.SubjectSummary = "主体：未识别（没有足够的资源路径证据）"
	case maxConfidence >= 2:
		vpkFile.SubjectConfidence = "高"
	default:
		vpkFile.SubjectConfidence = "低"
	}

	if vpkFile.PrimaryTag == "地图" && mapSubject != "" {
		extras := make([]string, 0, len(vpkFile.ContentSubjects))
		for _, subject := range vpkFile.ContentSubjects {
			if subject == mapSubject {
				continue
			}
			for _, item := range concrete {
				if item.label == subject && item.confidence > 0 {
					extras = append(extras, subject)
					break
				}
			}
		}
		vpkFile.SubjectSummary = "主体：" + mapSubject
		if len(extras) > 0 {
			vpkFile.SubjectSummary += "；附加内容：" + strings.Join(extras, "、")
		}
	} else if len(vpkFile.ContentSubjects) == 1 {
		vpkFile.SubjectSummary = "主体：" + vpkFile.ContentSubjects[0]
	} else if len(vpkFile.ContentSubjects) > 1 {
		vpkFile.SubjectSummary = "主体：混合包（" + strings.Join(vpkFile.ContentSubjects, "、") + "）"
	}
	if maxConfidence == 0 && len(vpkFile.ContentSubjects) > 0 {
		vpkFile.SubjectSummary += "（无法确认具体对象）"
	}
}
