package parser

import (
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type xdrSlotEvidence struct {
	character string
	model     string
	scope     string
	slot      int
	slotLabel string
	actions   map[string]bool
	evidence  map[string]bool
}

var xdrSlotPattern = regexp.MustCompile(`(?i)([a-z][a-z0-9]*)[\s_-]*slot[\s_-]*(\d{1,3})`)

var xdrCharacterAliases = map[string]struct {
	character string
	scope     string
}{
	"namvet":    {"Bill", "幸存者"},
	"bill":      {"Bill", "幸存者"},
	"biker":     {"Francis", "幸存者"},
	"francis":   {"Francis", "幸存者"},
	"manager":   {"Louis", "幸存者"},
	"louis":     {"Louis", "幸存者"},
	"teenangst": {"Zoey", "幸存者"},
	"teengirl":  {"Zoey", "幸存者"},
	"zoey":      {"Zoey", "幸存者"},
	"coach":     {"Coach", "幸存者"},
	"mechanic":  {"Ellis", "幸存者"},
	"ellis":     {"Ellis", "幸存者"},
	"gambler":   {"Nick", "幸存者"},
	"nick":      {"Nick", "幸存者"},
	"producer":  {"Rochelle", "幸存者"},
	"rochelle":  {"Rochelle", "幸存者"},
	"boomer":    {"Boomer", "特殊感染者"},
	"charger":   {"Charger", "特殊感染者"},
	"hunter":    {"Hunter", "特殊感染者"},
	"jockey":    {"Jockey", "特殊感染者"},
	"smoker":    {"Smoker", "特殊感染者"},
	"spitter":   {"Spitter", "特殊感染者"},
	"tank":      {"Tank", "特殊感染者"},
	"hulk":      {"Tank", "特殊感染者"},
	"witch":     {"Witch", "特殊感染者"},
	"common":    {"Common Infected", "普通感染者"},
	"infected":  {"Infected", "感染者"},
	"survivor":  {"Survivor", "幸存者"},
}

func collectXDRSlotEvidence(name string, index *archivePathIndex) {
	lower := strings.ToLower(name)
	if strings.Contains(lower, "xdr") || strings.Contains(lower, "dreanims") || strings.Contains(lower, "reanims") {
		index.hasXDRMarker = true
	}

	matches := xdrSlotPattern.FindStringSubmatch(lower)
	if len(matches) != 3 || !isXDRAnimationEvidencePath(lower) {
		return
	}
	slot, err := strconv.Atoi(matches[2])
	if err != nil || slot < 0 || slot > 48 {
		return
	}

	character, scope := detectXDRCharacter(matches[1], lower)
	if character == "" && !isXDRMarkerPath(lower) {
		return
	}
	model := fmt.Sprintf("%s_slot_%03d", strings.ToLower(matches[1]), slot)
	if character == "" {
		character, scope = "未指定角色/模型", "未知"
	}
	if character == "Survivor" || character == "Infected" {
		// The generic token is useful as a scope fallback, but a concrete path
		// token elsewhere is preferred when one exists.
		if concrete, concreteScope := detectXDRCharacterFromPath(lower); concrete != "" && concrete != character {
			character, scope = concrete, concreteScope
		}
	}

	key := fmt.Sprintf("%s\x00%s\x00%d", strings.ToLower(character), model, slot)
	evidence := index.xdrSlots[key]
	if evidence.actions == nil {
		evidence.actions = make(map[string]bool)
	}
	if evidence.evidence == nil {
		evidence.evidence = make(map[string]bool)
	}
	evidence.character = character
	evidence.model = model
	evidence.scope = scope
	evidence.slot = slot
	evidence.slotLabel = fmt.Sprintf("%03d", slot)
	evidence.evidence[lower] = true
	if action := xdrActionLabel(lower); action != "" {
		evidence.actions[action] = true
	}
	index.xdrSlots[key] = evidence
}

func isXDRMarkerPath(name string) bool {
	lower := strings.ToLower(name)
	return strings.Contains(lower, "xdr") || strings.Contains(lower, "dreanims") || strings.Contains(lower, "reanims")
}

func isXDRAnimationEvidencePath(name string) bool {
	ext := path.Ext(name)
	return ext == ".mdl" || ext == ".ani" || ext == ".qc" || ext == ".smd" ||
		ext == ".dmx" || ext == ".vta"
}

func detectXDRCharacter(modelToken, fullPath string) (string, string) {
	if meta, ok := xdrCharacterAliases[strings.ToLower(modelToken)]; ok {
		return meta.character, meta.scope
	}
	return detectXDRCharacterFromPath(fullPath)
}

func detectXDRCharacterFromPath(name string) (string, string) {
	parts := pathTokens(name)
	for _, token := range parts {
		if meta, ok := xdrCharacterAliases[token]; ok && meta.character != "Survivor" && meta.character != "Infected" {
			return meta.character, meta.scope
		}
	}
	for _, token := range parts {
		if meta, ok := xdrCharacterAliases[token]; ok {
			return meta.character, meta.scope
		}
	}
	return "", ""
}

func xdrActionLabel(name string) string {
	base := strings.TrimSuffix(path.Base(name), path.Ext(name))
	lower := strings.ToLower(base)
	if lower == "" || strings.Contains(lower, "_slot_") || strings.HasSuffix(lower, "_slot") {
		return ""
	}
	parts := make([]string, 0, 3)
	if strings.Contains(lower, "heal") {
		parts = append(parts, "治疗")
	}
	if strings.Contains(lower, "reload") {
		parts = append(parts, "换弹")
	}
	if strings.Contains(lower, "melee") || strings.Contains(lower, "swing") {
		parts = append(parts, "近战")
	}
	if strings.Contains(lower, "revive") {
		parts = append(parts, "救援")
	}
	if strings.Contains(lower, "shove") || strings.Contains(lower, "push") {
		parts = append(parts, "推击")
	}
	if strings.Contains(lower, "kick") {
		parts = append(parts, "踢击")
	}
	if strings.Contains(lower, "crouch") {
		parts = append(parts, "蹲姿")
	}
	if strings.Contains(lower, "stand") {
		parts = append(parts, "站姿")
	}
	if strings.Contains(lower, "idle") {
		parts = append(parts, "待机")
	}
	if len(parts) == 0 {
		return base
	}
	return strings.Join(uniqueStrings(parts), "·")
}

func buildXDRInfo(index archivePathIndex, vpkFile *VPKFile) {
	entries := make([]XDRSlotInfo, 0, len(index.xdrSlots))
	for _, evidence := range index.xdrSlots {
		actions := make([]string, 0, len(evidence.actions))
		for action := range evidence.actions {
			actions = append(actions, action)
		}
		sort.Strings(actions)
		if len(actions) > 8 {
			actions = actions[:8]
		}
		evidenceFiles := make([]string, 0, len(evidence.evidence))
		for file := range evidence.evidence {
			evidenceFiles = append(evidenceFiles, file)
		}
		sort.Strings(evidenceFiles)
		if len(evidenceFiles) > 6 {
			evidenceFiles = evidenceFiles[:6]
		}
		confidence := "高"
		if evidence.character == "未指定角色/模型" || evidence.scope == "未知" {
			confidence = "中"
		}
		entries = append(entries, XDRSlotInfo{
			Character:  evidence.character,
			Model:      evidence.model,
			Scope:      evidence.scope,
			Slot:       evidence.slot,
			SlotLabel:  evidence.slotLabel,
			Actions:    actions,
			Evidence:   evidenceFiles,
			Confidence: confidence,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Character != entries[j].Character {
			return entries[i].Character < entries[j].Character
		}
		if entries[i].Slot != entries[j].Slot {
			return entries[i].Slot < entries[j].Slot
		}
		return entries[i].Model < entries[j].Model
	})
	vpkFile.XDRSlots = entries

	marker := index.hasXDRMarker || strings.Contains(strings.ToLower(vpkFile.Name), "xdr") ||
		strings.Contains(strings.ToLower(vpkFile.Title), "xdr") || strings.Contains(strings.ToLower(vpkFile.Title), "reanims")
	if !marker {
		vpkFile.XDRSummary = ""
		return
	}
	if len(entries) == 0 {
		vpkFile.XDRSummary = "XDR 动画相关：未发现具体角色/模型 slot 文件"
		return
	}
	// XDR 路径本身通常不落在 models/survivors 或 models/infected 下，旧的
	// 通用主体规则可能只会给出“泛模型资源”。有了明确角色和 slot 证据后，
	// 清掉这个泛化结论，避免用户同时看到互相矛盾的主体标签。
	filteredSubjects := make([]string, 0, len(vpkFile.ContentSubjects))
	for _, subject := range vpkFile.ContentSubjects {
		if subject == "泛模型资源" || subject == "泛材质资源" {
			continue
		}
		filteredSubjects = append(filteredSubjects, subject)
	}
	vpkFile.ContentSubjects = filteredSubjects
	vpkFile.SubjectSummary = "主体：XDR 动画（具体角色/模型与 slot 见下方）"
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		part := fmt.Sprintf("%s（%s）· slot %s", entry.Character, entry.Model, entry.SlotLabel)
		if len(entry.Actions) > 0 {
			part += " · " + strings.Join(entry.Actions, "、")
		}
		parts = append(parts, part)
	}
	vpkFile.XDRSummary = "XDR：" + strings.Join(parts, "；") + "（同一动作冲突时 slot 数字越小优先级越高）"
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}
