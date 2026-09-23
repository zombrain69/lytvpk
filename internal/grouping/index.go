package grouping

import "strings"

// Index 是一次遍历建立的倒排索引：所有信号只读它，避免每个信号重新扫描全部 Mod。
type Index struct {
	mods        []Mod
	byFolder    map[string][]Mod
	folderLabels map[string]string
	byTagPair   map[string][]Mod
	tagLabels   map[string]string
	byAuthor    map[string][]Mod
	byVoice     map[string][]Mod
	voiceLabels map[string]string
	bySubject   map[string][]Mod
	byPrefix    map[string][]Mod
	// prefixLabels 保存前缀的"原始大小写"展示名（`白银审判` 而不是 `白银审判`小写后的样子）。
	prefixLabels map[string]string
	byFileName  map[string][]Mod
}

func buildIndex(mods []Mod) *Index {
	index := &Index{
		mods:        mods,
		byFolder:    make(map[string][]Mod, 128),
		folderLabels: make(map[string]string, 128),
		byTagPair:   make(map[string][]Mod, 512),
		tagLabels:   make(map[string]string, 512),
		byAuthor:    make(map[string][]Mod, 256),
		byVoice:     make(map[string][]Mod, 32),
		voiceLabels: make(map[string]string, 32),
		bySubject:   make(map[string][]Mod, 256),
		byPrefix:    make(map[string][]Mod, 256),
		prefixLabels: make(map[string]string, 256),
		byFileName:  make(map[string][]Mod, 256),
	}

	for _, mod := range mods {
		indexIndexFolders(index, mod)
		indexTags(index, mod)
		indexAuthor(index, mod)
		indexVoices(index, mod)
		indexSubject(index, mod)
		indexNamePrefix(index, mod)
		indexFileName(index, mod)
	}
	return index
}

func indexIndexFolders(index *Index, mod Mod) {
	folder := strings.Trim(strings.TrimSpace(mod.Folder), `\/`)
	if folder == "" {
		return
	}
	parts := strings.FieldsFunc(folder, func(r rune) bool { return r == '\\' || r == '/' })
	if len(parts) == 0 || ReservedFolder(parts[0]) {
		return
	}
	// 每个祖先目录都收下这个 Mod：整套目录给出"整套"建议，
	// 每层下级目录再各自给出更精确的建议。
	for depth := 1; depth <= len(parts); depth++ {
		key := strings.ToLower(strings.Join(parts[:depth], `\`))
		index.byFolder[key] = append(index.byFolder[key], mod)
		if _, ok := index.folderLabels[key]; !ok {
			index.folderLabels[key] = strings.Join(parts[:depth], " / ")
		}
	}
}

func indexTags(index *Index, mod Mod) {
	if len(mod.SecondaryTags) < 2 {
		return
	}
	for i := 0; i < len(mod.SecondaryTags); i++ {
		for j := i + 1; j < len(mod.SecondaryTags); j++ {
			first := strings.TrimSpace(mod.SecondaryTags[i])
			second := strings.TrimSpace(mod.SecondaryTags[j])
			if first == "" || second == "" {
				continue
			}
			// 标签对按字典序归一化：同一个组合无论写在标签列表的第几位，
			// 都必须落进同一个桶（"任意两个标签相同"）。
			firstLower := strings.ToLower(first)
			secondLower := strings.ToLower(second)
			if firstLower > secondLower {
				firstLower, secondLower = secondLower, firstLower
			}
			key := firstLower + "\x00" + secondLower
			index.byTagPair[key] = append(index.byTagPair[key], mod)
			if _, ok := index.tagLabels[key]; !ok {
				index.tagLabels[key] = first + "、" + second
			}
		}
	}
}

func indexAuthor(index *Index, mod Mod) {
	author := NormalizeAuthor(mod.Author)
	if author == "" {
		return
	}
	key := strings.ToLower(author)
	index.byAuthor[key] = append(index.byAuthor[key], mod)
}

func indexVoices(index *Index, mod Mod) {
	for _, character := range mod.VoiceCharacters {
		name := strings.TrimSpace(character)
		if name == "" {
			continue
		}
		key := strings.ToLower(name)
		index.byVoice[key] = append(index.byVoice[key], mod)
		if _, ok := index.voiceLabels[key]; !ok {
			index.voiceLabels[key] = name
		}
	}
}

func indexSubject(index *Index, mod Mod) {
	// 解析器标注"低"置信度的主体只是兜底说明（例如"脚本资源（无法确认具体对象）"）。
	if strings.TrimSpace(mod.SubjectConfidence) == "低" {
		return
	}
	subject := NormalizeSubject(mod.Subject)
	if subject == "" {
		return
	}
	index.bySubject[subject] = append(index.bySubject[subject], mod)
}

func indexNamePrefix(index *Index, mod Mod) {
	for _, key := range NamePrefixKeys(mod.Name) {
		if key.Key == "" {
			continue
		}
		index.byPrefix[key.Key] = append(index.byPrefix[key.Key], mod)
		if _, ok := index.prefixLabels[key.Key]; !ok {
			index.prefixLabels[key.Key] = key.Label
		}
	}
}

func indexFileName(index *Index, mod Mod) {
	if strings.TrimSpace(mod.Folder) == "" {
		return
	}
	name := strings.ToLower(strings.TrimSpace(BaseName(mod.Key)))
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(mod.Name))
	}
	if name == "" {
		return
	}
	index.byFileName[name] = append(index.byFileName[name], mod)
}

// folderGroupKey 返回目录的"套件归属键"（最顶层目录，用于跨目录同名判定）。
func folderGroupKey(folder string) string {
	parts := strings.FieldsFunc(strings.Trim(strings.TrimSpace(folder), `\/`), func(r rune) bool {
		return r == '\\' || r == '/'
	})
	if len(parts) == 0 {
		return ""
	}
	top := strings.ToLower(parts[0])
	if ReservedFolder(top) {
		return ""
	}
	return top
}

// folderPaths 返回从顶层到当前的完整目录层（用于 folder 信号的展示名与去重）。
func folderPaths(folder string) []string {
	parts := strings.FieldsFunc(strings.Trim(strings.TrimSpace(folder), `\/`), func(r rune) bool {
		return r == '\\' || r == '/'
	})
	if len(parts) == 0 {
		return nil
	}
	labels := make([]string, 0, len(parts))
	for depth := 1; depth <= len(parts); depth++ {
		labels = append(labels, strings.Join(parts[:depth], " / "))
	}
	return labels
}

func modsFromBucket(keys []string) []Mod {
	result := make([]Mod, 0, len(keys))
	return result
}
