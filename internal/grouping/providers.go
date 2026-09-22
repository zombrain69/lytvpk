package grouping

import (
	"fmt"
	"sort"
	"strings"
)

// Providers 返回全部内置信号，顺序固定：同类成员命中多个信号时，
// 先出现的信号保留自己的名字，因此顺序本身就是"谁更适合当组名"的排序。
func Providers() []providerFunc {
	return []providerFunc{
		{ID: "folder", Signals: []string{"同一文件夹"}, Run: runFolderSignal},
		{ID: "same-filename", Signals: []string{"同名文件（不同目录）"}, Run: runSameFileNameSignal},
		{ID: "filename-prefix", Signals: []string{"文件名前缀"}, Run: runFilenamePrefixSignal},
		{ID: "shared-tags", Signals: []string{"共同标签"}, Run: runSharedTagsSignal},
		{ID: "same-author", Signals: []string{"同一作者"}, Run: runSameAuthorSignal},
		{ID: "voice-character", Signals: []string{"语音角色"}, Run: runVoiceSignal},
		{ID: "subject", Signals: []string{"主体识别"}, Run: runSubjectSignal},
	}
}

func runFolderSignal(index *Index, options Options) []Candidate {
	if len(index.byFolder) == 0 {
		return nil
	}
	keys := sortedKeys(index.byFolder)
	candidates := make([]Candidate, 0, len(keys))
	for _, key := range keys {
		bucket := index.byFolder[key]
		if len(bucket) < options.MinMembers || len(bucket) > options.MaxCuratedMembers {
			continue
		}
		label := index.folderLabels[key]
		if label == "" {
			label = FolderDisplay(key)
		}
		candidates = append(candidates, newCandidate("folder", "同一文件夹", label,
			fmt.Sprintf("都在同一个文件夹：%s", label), FolderScore, bucket))
	}
	return candidates
}

func runSameFileNameSignal(index *Index, options Options) []Candidate {
	if len(index.byFileName) == 0 {
		return nil
	}
	keys := sortedKeys(index.byFileName)
	candidates := make([]Candidate, 0, len(keys))
	for _, name := range keys {
		bucket := index.byFileName[name]
		if len(bucket) < options.MinMembers {
			continue
		}
		groups := make(map[string]struct{}, len(bucket))
		for _, mod := range bucket {
			group := folderGroupKey(mod.Folder)
			if group == "" {
				continue
			}
			groups[group] = struct{}{}
		}
		// 必须限制在同一个套件内：不同套件各自有一个"目镜.vpk"只是重名。
		if len(groups) != 1 {
			continue
		}
		label := strings.TrimSpace(bucket[0].Title)
		if label == "" {
			label = BaseName(bucket[0].Key)
		}
		candidates = append(candidates, newCandidate("same-filename", "同名文件（不同目录）", label,
			fmt.Sprintf("文件名相同但放在不同目录：%s", BaseName(bucket[0].Key)), SameFileNameScore, bucket))
	}
	return candidates
}

func runFilenamePrefixSignal(index *Index, options Options) []Candidate {
	if len(index.byPrefix) == 0 {
		return nil
	}
	keys := sortedKeys(index.byPrefix)
	candidates := make([]Candidate, 0, len(keys))
	for _, prefix := range keys {
		bucket := index.byPrefix[prefix]
		if len(bucket) < options.MinMembers || !SharesPrimaryTag(bucket) {
			continue
		}
		candidates = append(candidates, newCandidate("filename-prefix", "文件名前缀", prefix,
			fmt.Sprintf("文件名前缀相同：%s", prefix), PrefixScore, bucket))
	}
	return candidates
}

func runSharedTagsSignal(index *Index, options Options) []Candidate {
	if len(index.byTagPair) == 0 {
		return nil
	}
	keys := sortedKeys(index.byTagPair)
	candidates := make([]Candidate, 0, len(keys))
	for _, pair := range keys {
		bucket := index.byTagPair[pair]
		// 至少要 3 个成员，"两个人刚好同标签"噪声太大。
		if len(bucket) < 3 || !SharesPrimaryTag(bucket) {
			continue
		}
		label := index.tagLabels[pair]
		candidates = append(candidates, newCandidate("shared-tags", "共同标签", label,
			fmt.Sprintf("共同标签：%s", label), TagScore, bucket))
	}
	return candidates
}

func runSameAuthorSignal(index *Index, options Options) []Candidate {
	if len(index.byAuthor) == 0 {
		return nil
	}
	keys := sortedKeys(index.byAuthor)
	candidates := make([]Candidate, 0, len(keys))
	for _, author := range keys {
		bucket := index.byAuthor[author]
		// 高产作者（超过阈值）的作品之间往往没有实际关系。
		if len(bucket) < options.MinMembers || len(bucket) > options.MaxAuthorMembers {
			continue
		}
		if !SharesPrimaryTag(bucket) {
			continue
		}
		label := NormalizeAuthor(bucket[0].Author)
		candidates = append(candidates, newCandidate("same-author", "同一作者", label,
			fmt.Sprintf("同一作者：%s", label), AuthorScore, bucket))
	}
	return candidates
}

func runVoiceSignal(index *Index, options Options) []Candidate {
	if len(index.byVoice) == 0 {
		return nil
	}
	keys := sortedKeys(index.byVoice)
	candidates := make([]Candidate, 0, len(keys))
	for _, character := range keys {
		bucket := index.byVoice[character]
		if len(bucket) < options.MinMembers || !SharesPrimaryTag(bucket) {
			continue
		}
		display := index.voiceLabels[character]
		candidates = append(candidates, newCandidate("voice-character", "语音角色", display+" 语音",
			fmt.Sprintf("替换了同一个角色的语音：%s", display), VoiceScore, bucket))
	}
	return candidates
}

func runSubjectSignal(index *Index, options Options) []Candidate {
	if len(index.bySubject) == 0 {
		return nil
	}
	keys := sortedKeys(index.bySubject)
	candidates := make([]Candidate, 0, len(keys))
	for _, subject := range keys {
		bucket := index.bySubject[subject]
		if len(bucket) < options.MinMembers {
			continue
		}
		candidates = append(candidates, newCandidate("subject", "主体识别", subject,
			fmt.Sprintf("主体相同：%s", subject), SubjectScore, bucket))
	}
	return candidates
}

// newCandidate 从一批 Mod 生成候选：成员按 Key 排序保证结果稳定可复现。
func newCandidate(provider, signal, label, reason string, score int, bucket []Mod) Candidate {
	sorted := append([]Mod(nil), bucket...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	keys := make([]string, 0, len(sorted))
	names := make([]string, 0, len(sorted))
	for _, mod := range sorted {
		keys = append(keys, mod.Key)
		name := strings.TrimSpace(mod.Title)
		if name == "" {
			name = strings.TrimSpace(mod.Name)
		}
		if name == "" {
			name = BaseName(mod.Key)
		}
		names = append(names, name)
	}
	return Candidate{
		Provider: provider,
		Label:    label,
		Reason:   reason,
		Score:    score,
		Signals:  []string{signal},
		Keys:     keys,
		Names:    names,
	}
}

func sortedKeys(buckets map[string][]Mod) []string {
	keys := make([]string, 0, len(buckets))
	for key := range buckets {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
