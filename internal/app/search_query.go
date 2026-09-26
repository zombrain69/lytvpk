package app

import (
	"regexp"
	"strings"
)

// 搜索框语法（对齐 FireAxe `AddonNodeSearchUtils` / `AddonNodeSearchOptions`）：
//
//	FireAxe 的能力            →  本项目搜索框写法
//	IsRegex（正则匹配）          →  re:^ak\d+$
//	Tags + TagFilterMode.Or    →  tag:步枪|狙击
//	Tags + TagFilterMode.And   →  tag:武器 tag:步枪
//	Tags + TagFilterMode.Not   →  -tag:材质
//	IncludeName / 忽略大小写     →  默认行为（标题、文件名、标签、主体…都参与，大小写不敏感）
//
// 为什么不用"模式开关"而是逐 token 表达：搜索框里没有地方放模式下拉，
// 而 `tag:a|b`（或）/ `tag:a tag:b`（且）/ `-tag:a`（排除）比"先切模式再输入"更直接，
// 三种 FireAxe 模式都能表达。多个普通词之间是"且"，引号里的空格算一个词。
type modSearchTagCondition struct {
	// Terms 是"任一命中即可"的标签名（来自 `tag:a|b` 的 OR 组）。
	Terms []string
}

type modSearchQuery struct {
	Raw         string
	Terms       []string
	IncludeTags []modSearchTagCondition
	ExcludeTags []string
	Regex       *regexp.Regexp
	// RegexError 记录 `re:` 的编译错误：这种情况一个都不匹配（与 FireAxe 的"无效正则匹配不到"一致），
	// 但界面要把原因说清楚，而不是静默空列表。
	RegexError string
}

// tokenizeSearchQuery 按空白切词，支持双引号包住带空格的短语；引号内不切分。
func tokenizeSearchQuery(raw string) []string {
	tokens := make([]string, 0, 8)
	var builder strings.Builder
	inQuotes := false
	flush := func() {
		token := strings.TrimSpace(builder.String())
		builder.Reset()
		if token != "" {
			tokens = append(tokens, token)
		}
	}

	for _, r := range raw {
		switch {
		case r == '"':
			inQuotes = !inQuotes
		case (r == ' ' || r == '\t' || r == '\n' || r == '\r') && !inQuotes:
			flush()
		default:
			builder.WriteRune(r)
		}
	}
	flush()
	return tokens
}

// parseModSearchQuery 解析搜索框内容。解析永远成功（不像正则那样有硬错误），
// 只有 `re:` 会带出 RegexError。
func parseModSearchQuery(raw string) modSearchQuery {
	spec := modSearchQuery{Raw: strings.TrimSpace(raw)}
	for _, token := range tokenizeSearchQuery(spec.Raw) {
		switch {
		case strings.HasPrefix(token, "re:") || strings.HasPrefix(token, "正则:"):
			pattern := trimSearchPrefix(token, "re:", "正则:")
			if pattern == "" {
				continue
			}
			compiled, err := regexp.Compile("(?i)" + pattern)
			if err != nil {
				spec.RegexError = err.Error()
				continue
			}
			spec.Regex = compiled
		case strings.HasPrefix(token, "-tag:") || strings.HasPrefix(token, "-标签:"):
			name := trimSearchPrefix(token, "-tag:", "-标签:")
			if name == "" {
				continue
			}
			for _, part := range splitTagAlternatives(name) {
				spec.ExcludeTags = append(spec.ExcludeTags, part)
			}
		case strings.HasPrefix(token, "tag:") || strings.HasPrefix(token, "标签:"):
			name := trimSearchPrefix(token, "tag:", "标签:")
			if name == "" {
				continue
			}
			terms := splitTagAlternatives(name)
			if len(terms) > 0 {
				spec.IncludeTags = append(spec.IncludeTags, modSearchTagCondition{Terms: terms})
			}
		case strings.HasPrefix(token, "-") && len(token) > 1:
			// 普通词前的减号：命中该词的记录直接排除（与 `-tag:` 并列的写法）。
			spec.Terms = append(spec.Terms, "-"+strings.TrimPrefix(token, "-"))
		default:
			spec.Terms = append(spec.Terms, token)
		}
	}
	return spec
}

// trimSearchPrefix 去掉前缀（支持中英文两种写法）。
func trimSearchPrefix(token string, prefixes ...string) string {
	for _, prefix := range prefixes {
		if strings.HasPrefix(token, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(token, prefix))
		}
	}
	return strings.TrimSpace(token)
}

// splitTagAlternatives 把 `a|b|c` 拆成 OR 组，去空、去重、保持顺序。
func splitTagAlternatives(value string) []string {
	parts := strings.Split(value, "|")
	result := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, name)
	}
	return result
}

// searchableModFields 汇总参与搜索的字段（与界面展示的字段一一对应）。
type searchableModFields struct {
	Title           string
	Name            string
	PrimaryTag      string
	SecondaryTags   []string
	SubjectSummary  string
	ContentSubjects []string
	VoiceCharacters []string
	XDRSummary      string
}

// tagsInHierarchyOrFlat 返回该 Mod 的全部标签（一级 + 二级），用于 tag: 条件判断。
func (fields searchableModFields) allTags() []string {
	tags := make([]string, 0, len(fields.SecondaryTags)+1)
	if tag := strings.TrimSpace(fields.PrimaryTag); tag != "" {
		tags = append(tags, tag)
	}
	for _, tag := range fields.SecondaryTags {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	return tags
}

// hasTag 判断标签集合里是否有某个标签（大小写不敏感，去掉首尾空白）。
func (fields searchableModFields) hasTag(name string) bool {
	needle := strings.ToLower(strings.TrimSpace(name))
	if needle == "" {
		return false
	}
	for _, tag := range fields.allTags() {
		if strings.ToLower(tag) == needle {
			return true
		}
	}
	return false
}

// textFields 返回参与"文本 / 正则"匹配的字段。
func (fields searchableModFields) textFields() []string {
	result := make([]string, 0, 4+len(fields.SecondaryTags)+len(fields.ContentSubjects)+len(fields.VoiceCharacters))
	result = append(result, fields.Title, fields.Name, fields.PrimaryTag, fields.SubjectSummary, fields.XDRSummary)
	result = append(result, fields.SecondaryTags...)
	result = append(result, fields.ContentSubjects...)
	result = append(result, fields.VoiceCharacters...)
	return result
}

// matches 判断一条记录是否满足整个查询。
func (spec modSearchQuery) matches(fields searchableModFields) bool {
	if spec.RegexError != "" {
		// 正则写错时一个都不匹配：宁可空列表 + 明确提示，也不要悄悄忽略用户的意图。
		return false
	}

	for _, term := range spec.Terms {
		if strings.HasPrefix(term, "-") && len(term) > 1 {
			// `-词`：**包含**该词的记录被排除。
			// 排除特意用子串而不是模糊：字段里有完整路径，模糊匹配会让 `-old`、`-hd`
			// 这类短词在长路径里"凑字母"命中，把不相干的记录一起排掉。
			needle := strings.TrimSpace(term[1:])
			if needle != "" && fieldsContainsText(fields, needle) {
				return false
			}
			continue
		}
		if !fieldsMatchesText(fields, term) {
			return false
		}
	}

	if spec.Regex != nil {
		matched := false
		for _, text := range fields.textFields() {
			if text != "" && spec.Regex.MatchString(text) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	for _, condition := range spec.IncludeTags {
		hit := false
		for _, name := range condition.Terms {
			if fields.hasTag(name) {
				hit = true
				break
			}
		}
		if !hit {
			return false
		}
	}

	for _, name := range spec.ExcludeTags {
		if name == "" {
			continue
		}
		if fields.hasTag(name) {
			return false
		}
	}

	return true
}

// fieldsContainsText 用一个词做**子串**匹配（大小写不敏感），专供 `-排除` 使用。
// 正向词仍然走模糊匹配：那是既有的"宽松好找"语义，用户熟悉。
func fieldsContainsText(fields searchableModFields, term string) bool {
	needle := strings.ToLower(strings.TrimSpace(term))
	if needle == "" {
		return false
	}
	for _, text := range fields.textFields() {
		if text == "" {
			continue
		}
		if strings.Contains(strings.ToLower(text), needle) {
			return true
		}
	}
	return false
}

// fieldsMatchesText 用一个词去匹配所有可搜索字段（模糊：字符按顺序出现即命中，
// 与既有 SearchVPKFiles 的行为一致）。
func fieldsMatchesText(fields searchableModFields, term string) bool {
	needle := strings.TrimSpace(term)
	if needle == "" {
		return true
	}
	for _, text := range fields.textFields() {
		if text == "" {
			continue
		}
		if fuzzyMatch(strings.ToLower(needle), strings.ToLower(text)) {
			return true
		}
	}
	return false
}
