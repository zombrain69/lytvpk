package parser

import "strings"

// 标题/描述证据通道（W4 通道 4）。
//
// 过去只有"主类型=武器"的包才会读 addoninfo 标题（见 ProcessWeaponVPK），
// 于是 `ak47冰龙参数包紫.vpk`（主类型=其他）这类包明明标题写着型号，却拿不到标签（D7）。
// 本通道对**所有主类型**生效，且只加标签：
//
//  1. 武器型号关键词（ak47 / m16 / mac10 / riot shield / katana …）
//  2. 内容/物品规则的关键词（medkit / molotov / hud / flashlight …）
//  3. 本体实体标签名（标题直接写中文名：铁喷 / 可乐 / 烟花盒 / 高尔夫球杆 …）
//
// fileName 也要一起看：很多包的中文标题写在 addoninfo 里，但作者自制的包干脆
// 把型号写进文件名（`[m16] xxx.vpk`、`立华奏ingram_(mac10)切枪语言音频.vpk`）。
func applyTitleEvidence(title, desc, fileName string, tags map[string]bool) {
	applyTitleEvidenceWithRecorder(title, desc, fileName, tags, nil)
}

// applyTitleEvidenceWithRecorder 是带证据登记的版本（W6）。
func applyTitleEvidenceWithRecorder(title, desc, fileName string, tags map[string]bool, evidence *tagEvidenceRecorder) {
	text := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(title), strings.TrimSpace(desc), strings.TrimSpace(fileName),
	}, " "))
	if strings.TrimSpace(text) == "" {
		return
	}

	for _, rule := range weaponMetadataRules {
		if rule.pathOnly {
			continue
		}
		if metadataRuleMatches(text, rule) {
			addWeaponTag(rule.tag, tags)
			evidence.record(rule.tag, "title:"+rule.keyword, EvidenceLevelInferred, fileName)
		}
	}

	// 路径规则里也有一批"标题里常写"的型号（riot shield / katana / nightstick …），
	// 只挑足够独特的，避免 chrome / rifle 这类通用词在标题里误伤。
	for _, rule := range weaponPathRules {
		// pathOnly（minigun / 50cal 这类"模型名"）不做标题推断：
		// 标题写 "Minigun 替换 M60" 说的是模型长什么样，不代表改的是固定机枪。
		if rule.pathOnly {
			continue
		}
		if !isDistinctiveTokenKeyword(rule.keyword) {
			continue
		}
		if textContainsKeyword(text, rule.keyword) {
			addWeaponTag(rule.tag, tags)
			evidence.record(rule.tag, "title:"+rule.keyword, EvidenceLevelInferred, fileName)
		}
	}

	for _, rule := range contentTagRules {
		for _, keyword := range rule.keywords {
			if !textContainsKeyword(text, keyword) {
				continue
			}
			tags[rule.tag] = true
			evidence.record(rule.tag, "title:"+keyword, EvidenceLevelInferred, fileName)
			if rule.isItem {
				applyItemAggregateTags(rule.tag, tags)
			}
		}
	}

	if stockEntities == nil {
		return
	}
	for _, entity := range stockEntities.Entities {
		if entity.Tag == "" || entity.Character != "" {
			continue
		}
		if !textContainsKeyword(text, entity.Tag) {
			continue
		}
		if entity.IsItem {
			tags[entity.Tag] = true
			applyItemAggregateTags(entity.Tag, tags)
			evidence.record(entity.Tag, "title:"+entity.Tag, EvidenceLevelInferred, fileName)
			continue
		}
		addWeaponTag(entity.Tag, tags)
		evidence.record(entity.Tag, "title:"+entity.Tag, EvidenceLevelInferred, fileName)
	}
}

// metadataRuleMatches 复刻 DetectWeaponTypeFromMetadata 的匹配口径：
// token 规则与 scar 特例都要求整词，其余按子串。
func metadataRuleMatches(lowerText string, rule weaponMatchRule) bool {
	if rule.tokenMatch || rule.keyword == "scar" {
		return textContainsToken(lowerText, rule.keyword)
	}
	return strings.Contains(lowerText, rule.keyword)
}

// textContainsKeyword 对 ASCII 关键词用"整词"匹配（避免 oscar 命中 scar、
// atv 命中 tv 这类误伤），中文等非字母数字字符按子串匹配。
func textContainsKeyword(text, keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	if !isASCIIKeyword(keyword) {
		return strings.Contains(text, keyword)
	}
	if textContainsToken(text, keyword) {
		return true
	}
	// 分隔符归一：关键词 `riot_shield` 要能命中标题里的 "Riot Shield"，
	// 反之亦然。做法是把两边都切成字母数字词，再做连续词序列匹配。
	words := phraseWords(keyword)
	if len(words) == 0 {
		return false
	}
	if len(words) == 1 {
		return false
	}
	return textContainsPhrase(text, words)
}

// phraseWords 把短语切成纯字母数字词（`riot_shield` → [riot shield]）。
func phraseWords(phrase string) []string {
	fields := strings.FieldsFunc(strings.ToLower(phrase), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

// textContainsPhrase 在文本的词序列里查找连续出现的短语。
func textContainsPhrase(text string, words []string) bool {
	if len(words) == 0 {
		return false
	}
	tokens := phraseWords(text)
	if len(tokens) < len(words) {
		return false
	}
	for i := 0; i+len(words) <= len(tokens); i++ {
		matched := true
		for j, word := range words {
			if tokens[i+j] != word {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func isASCIIKeyword(keyword string) bool {
	for _, r := range keyword {
		if r > 127 {
			return false
		}
	}
	return true
}

// textContainsToken 要求 token 两侧不是 `[a-z0-9_-]`（大小写不敏感）。
func textContainsToken(text, token string) bool {
	token = strings.ToLower(token)
	if token == "" {
		return false
	}
	from := 0
	for {
		idx := strings.Index(text[from:], token)
		if idx < 0 {
			return false
		}
		start := from + idx
		end := start + len(token)
		leftOK := start == 0 || !isTokenRune(rune(text[start-1]))
		rightOK := end >= len(text) || !isTokenRune(rune(text[end]))
		if leftOK && rightOK {
			return true
		}
		from = start + 1
	}
}
