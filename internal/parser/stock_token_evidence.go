package parser

import (
	"strings"
	"unicode"
)

// 本体基名 token 通道（W4 通道 2）。
//
// 作者常把"参数包 / 特效包 / 贴图包"放在自己的命名空间里，路径长这样：
//
//	materials/models/codm/ice/jc/rifle_ak47/ads.vmt
//	particles/codm_krig6icedrake-ak47.pcf
//	particles/xx2_c6_custom_rifle_ak47.pcf
//
// 这些路径不在武器目录白名单里，关键词表也就不会跑；但它们的文件名里带着**本体模型名**
// （rifle_ak47 / ak47）。本通道把这些 token 当证据：命中就加标签，纯增量。
//
// 匹配口径：把路径切成 `[a-z0-9_-]` 连续段，段内去掉 `_`/`-` 后查表，
// 于是 `w_rifle_ak47`、`rifle_ak47`、`rifle-ak47` 会归一到同一个 key。

type stockTokenHit struct {
	Tag       string
	IsItem    bool
	Character string
}

var stockTokenIndex = buildStockTokenIndex()

func buildStockTokenIndex() map[string][]stockTokenHit {
	index := make(map[string][]stockTokenHit)
	add := func(key string, hit stockTokenHit) {
		key = strings.TrimSpace(key)
		if key == "" {
			return
		}
		for _, existing := range index[key] {
			if existing == hit {
				return
			}
		}
		index[key] = append(index[key], hit)
	}

	if stockEntities != nil {
		for _, entity := range stockEntities.Entities {
			hit := stockTokenHit{Tag: entity.Tag, IsItem: entity.IsItem, Character: entity.Character}
			for _, anchor := range entity.Anchors {
				base := anchorBaseName(anchor)
				if base == "" {
					continue
				}
				add(normalizeToken(base), hit)
				// `w_rifle_ak47` / `v_rifle_ak47` → `rifle_ak47`：
				// 作者命名空间里通常只保留后半段。
				if stripped := stripModelPrefix(base); stripped != base {
					if _, generic := genericStrippedTokens[strings.ToLower(stripped)]; !generic {
						add(normalizeToken(stripped), hit)
					}
				}
			}
		}
	}

	for _, rule := range weaponPathRules {
		if !isDistinctiveTokenKeyword(rule.keyword) {
			continue
		}
		add(normalizeToken(rule.keyword), stockTokenHit{Tag: rule.tag})
	}
	return index
}

// isDistinctiveTokenKeyword 判断一个关键词是否足够"独特"，可以在任意目录 / 标题里当证据。
//
// 判据（从松到紧）：
//   - 带数字的型号（ak47 / m16 / m60 / mp5 / p220 / g3sg1 / 50cal …）
//   - 少量公认无歧义的短代号（awp / uzi / spas）
//   - 多词短语（riot shield / baseball bat / grenade launcher …）
//   - 长度 ≥5 且不在通用词黑名单里
func isDistinctiveTokenKeyword(keyword string) bool {
	keyword = strings.ToLower(strings.TrimSpace(keyword))
	if keyword == "" {
		return false
	}
	if _, generic := genericStrippedTokens[keyword]; generic {
		return false
	}
	if containsDigit(keyword) {
		return true
	}
	if _, short := distinctiveShortTokens[keyword]; short {
		return true
	}
	if strings.ContainsAny(keyword, " -") {
		return true
	}
	return len([]rune(keyword)) >= 5
}

// genericStrippedTokens 是「去掉 w_/v_ 前缀后过于通用、放到任意目录会误伤」的 token。
//
// 典型反例（D1）：Chrome 连喷的世界模型叫 `w_pumpshotgun_A.mdl`。若把 `pumpshotgun`
// 当作通用 token，Chrome 包又会被判成木喷 —— 正是 W3 修掉的错误标签。
// 这些词只在"本体精确路径"或"武器目录白名单 + 关键词表"里生效，不来这里掺和。
var genericStrippedTokens = map[string]struct{}{
	"shotgun": {}, "pumpshotgun": {}, "rifle": {}, "smg": {}, "pistol": {}, "pistola": {},
	"sniper": {}, "knife": {}, "melee": {}, "weapon": {}, "guns": {},
	// "chrome" 是材质/颜色词，放进标题通道会误伤（"chrome heart AK47" 之类）。
	"chrome": {},
}

// distinctiveShortTokens 是不带数字、但公认无歧义的短代号。
var distinctiveShortTokens = map[string]struct{}{
	"awp": {}, "uzi": {}, "spas": {},
}

// applyStockTokenEvidence 在任意目录下按"本体基名 token"补标签，返回是否命中物品实体。
func applyStockTokenEvidence(name string, tags map[string]bool, evidence *tagEvidenceRecorder) bool {
	return applyStockTokenLookup(stockPathTokens(name), tags, evidence, "token:", name)
}

// applyStockTokenLookup 是通道 2（路径 token）与通道 3（.mdl 材质表 token）共用的查表逻辑。
// rulePrefix 区分证据来源：`token:` / `mdl:`。
func applyStockTokenLookup(tokens []string, tags map[string]bool, evidence *tagEvidenceRecorder, rulePrefix, source string) bool {
	if len(stockTokenIndex) == 0 {
		return false
	}
	isItem := false
	for _, token := range tokens {
		// 先试整段（`rifle_ak47` → `rifleak47`），再试按 `_`/`-` 拆开的子片段
		// （`codm_krig6icedrake-ak47` → `codm` / `krig6icedrake` / `ak47`）。
		for _, candidate := range tokenCandidates(token) {
			hits, ok := stockTokenIndex[candidate]
			if !ok {
				continue
			}
			for _, hit := range hits {
				switch {
				case hit.Character != "":
					applyStockCharacterAnchors(hit.Character, tags)
					evidence.record(hit.Character, rulePrefix+candidate, EvidenceLevelPattern, source)
				case hit.IsItem:
					isItem = true
					tags[hit.Tag] = true
					applyItemAggregateTags(hit.Tag, tags)
					evidence.record(hit.Tag, rulePrefix+candidate, EvidenceLevelPattern, source)
				case hit.Tag != "":
					addWeaponTag(hit.Tag, tags)
					evidence.record(hit.Tag, rulePrefix+candidate, EvidenceLevelPattern, source)
				}
			}
		}
	}
	return isItem
}

// tokenCandidates 返回一个路径段的所有查表候选：整段与按分隔符拆开的子片段。
func tokenCandidates(token string) []string {
	candidates := make([]string, 0, 4)
	seen := make(map[string]struct{}, 4)
	appendCandidate := func(value string) {
		value = normalizeToken(value)
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		candidates = append(candidates, value)
	}

	appendCandidate(token)
	start := 0
	for i, r := range token {
		if r == '_' || r == '-' {
			appendCandidate(token[start:i])
			start = i + 1
		}
	}
	appendCandidate(token[start:])
	return candidates
}

func anchorBaseName(anchor string) string {
	anchor = strings.TrimSpace(anchor)
	if anchor == "" {
		return ""
	}
	if slash := strings.LastIndexByte(anchor, '/'); slash >= 0 {
		anchor = anchor[slash+1:]
	}
	if dot := strings.LastIndexByte(anchor, '.'); dot > 0 {
		anchor = anchor[:dot]
	}
	return anchor
}

func stripModelPrefix(base string) string {
	lower := strings.ToLower(base)
	for _, prefix := range []string{"w_", "v_"} {
		if strings.HasPrefix(lower, prefix) && len(lower) > len(prefix)+2 {
			return base[len(prefix):]
		}
	}
	return base
}

// stockPathTokens 把路径切成 `[a-z0-9_-]` 连续段（大小写不敏感）。
// 与 subject_parser 的 pathTokens（按非字母数字切分）口径不同，故单独命名。
func stockPathTokens(name string) []string {
	lower := strings.ToLower(name)
	tokens := make([]string, 0, 12)
	start := -1
	for i, r := range lower {
		if isTokenRune(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			tokens = append(tokens, lower[start:i])
			start = -1
		}
	}
	if start >= 0 {
		tokens = append(tokens, lower[start:])
	}
	return tokens
}

func isTokenRune(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' || r == '-'
}

// normalizeToken 去掉分隔符，让 w_rifle_ak47 / rifle-ak47 / rifleak47 归一。
func normalizeToken(token string) string {
	var builder strings.Builder
	builder.Grow(len(token))
	for _, r := range strings.ToLower(token) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func containsDigit(value string) bool {
	for _, r := range value {
		if unicode.IsDigit(r) {
			return true
		}
	}
	return false
}
