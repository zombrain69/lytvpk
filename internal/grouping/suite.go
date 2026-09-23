package grouping

import (
	"fmt"
	"sort"
	"strings"
)

// 套件命名空间信号（suite-namespace）。
//
// 现实里同一个套件经常拆成几十个 VPK：本体（替换官方目标，例如
// `survivors/survivor_coach`）+ 配件 / 服装 / 材质 / 开关件（替换套件自己的目录）。
// 它们在 VPK 内部路径里共享同一个 `models/<作者>/<套件>` 命名空间，
// 例如 l4n 的 `913limod/airi_evilfall`、武器侧的 `codm/ice`（贴图包 + 各枪参数包）。
//
// 这类关系是"要一起启用"（all），而不是"二选一"（single）—— 所以单独成一条信号：
//   - 同一命名空间下 ≥3 个模块 → 一个候选；
//   - 再把"本体"附着进来：本体只替换官方目标、没有该命名空间，
//     但它们与配件共享很长的文件名前缀（例如 `airi初代`），据此关联；
//   - 官方目录（weapons / survivors / props…）在解析阶段就已被排除，
//     不会把"同一把枪的多个替换"误判成套件。

const (
	// 一个套件至少要有 3 个模块，避免把零散的两三个文件凑成"套件"。
	suiteMinMembers = 3
	// 用文件名公共前缀把本体关联进来时，前缀至少要这么长（太短容易误伤）。
	// 3 个中文字符足够有区分度（如 `死库水`），再短就不可靠了。
	suiteMinNamePrefixRunes = 3
	// 用"套件名关键词"匹配本体文件名时的最小长度（`shinano` 7、`sikushui` 8、`ice` 3 太短）。
	suiteMinKeywordRunes = 6
	// 一个套件最多挂多少个本体，防止"前缀过短"时把半个库拉进来。
	suiteMaxMembers = 120
)

// genericSuiteSegments 是"通用目录名"：它们出现在 models/<作者>/<这一段> 时不是套件名
// （典型误报：`models/xxx/weapons/…` 会把半个武器库混成一组）。
var genericSuiteSegments = map[string]struct{}{
	"weapons": {}, "weapon": {}, "characters": {}, "character": {},
	"survivors": {}, "survivor": {}, "props": {}, "prop": {},
	"models": {}, "model": {}, "materials": {}, "material": {},
	"sounds": {}, "sound": {}, "maps": {}, "map": {},
	"vehicles": {}, "vehicle": {}, "cars": {}, "car": {},
	"vehiculos": {}, "vehiculo": {}, "personajes": {}, "armas": {},
	"modelos": {}, "texturas": {}, "coches": {}, "autos": {},
	"player": {}, "players": {}, "zombie": {}, "infected": {}, "humans": {}, "human": {},
	"tools": {}, "tool": {}, "ui": {}, "gui": {}, "scripts": {},
	"particles": {}, "effects": {}, "misc": {}, "other": {}, "others": {},
	"test": {}, "temp": {}, "tmp": {}, "new": {}, "backup": {}, "files": {}, "file": {},
	"data": {}, "assets": {}, "asset": {}, "textures": {}, "texture": {}, "shared": {},
}

// suiteSegmentUsable 判断命名空间的第二段能不能当"套件名"：
// 通用目录名、纯数字、太短（1 个字符）都不行。
func suiteSegmentUsable(segment string) bool {
	trimmed := strings.TrimSpace(segment)
	if len([]rune(trimmed)) < 2 {
		return false
	}
	if _, generic := genericSuiteSegments[strings.ToLower(trimmed)]; generic {
		return false
	}
	for _, r := range trimmed {
		if r < '0' || r > '9' {
			return true
		}
	}
	return false // 纯数字
}

// looksNumericOnly 判断一段文本是否只由数字/空白/标点组成（工坊 ID 之类）。
func looksNumericOnly(value string) bool {
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
		case r == ' ' || r == '-' || r == '_' || r == '.':
		default:
			return false
		}
	}
	return true
}

func runSuiteNamespaceSignal(index *Index, options Options) []Candidate {
	if index == nil || len(index.mods) == 0 {
		return nil
	}
	byRoot := make(map[string][]Mod, 32)
	rootOrder := make([]string, 0, 32)
	for _, mod := range index.mods {
		for _, root := range mod.ResourceRoots {
			key := strings.ToLower(strings.TrimSpace(root))
			if key == "" {
				continue
			}
			if _, exists := byRoot[key]; !exists {
				rootOrder = append(rootOrder, key)
			}
			byRoot[key] = append(byRoot[key], mod)
		}
	}
	sort.Strings(rootOrder)

	candidates := make([]Candidate, 0, len(rootOrder))
	for _, root := range rootOrder {
		members := dedupeMods(byRoot[root])
		if len(members) < suiteMinMembers {
			continue
		}
		segments := strings.Split(root, "/")
		if !suiteSegmentUsable(segments[len(segments)-1]) {
			continue
		}
		keys := make([]string, 0, len(members))
		names := make([]string, 0, len(members))
		namePool := make([]string, 0, len(members))
		for _, mod := range members {
			keys = append(keys, mod.Key)
			names = append(names, mod.Name)
			namePool = append(namePool, mod.Name)
		}

		// 本体附着：用簇内文件名的公共前缀把"只替换官方目标"的本体拉进来。
		prefix := longestCommonPrefix(namePool)
		prefix = strings.TrimRight(prefix, " _-·【】[]()（）")
		if looksNumericOnly(prefix) {
			// 工坊 ID 前缀（3788xxx）不是套件名。
			prefix = ""
		}
		// 单段候选（来自 materials 分支，如 `sikushui`）没有"作者/套件"两段的强结构，
		// 必须再有一条独立证据才成立：簇内文件名要有可用的公共前缀（例如 `死库水…`）。
		// 否则会把 `materials/<各种零散目录>/…` 误当成套件（实测会把 26 组涨到 101 组）。
		if !strings.Contains(root, "/") && len([]rune(prefix)) < suiteMinNamePrefixRunes {
			continue
		}
		attached := 0
		// 本体附着的两种线索：
		//   ① 文件名公共前缀（airi 的 `airi初代…`、死库水的 `死库水…`）；
		//   ② 套件名关键词（shinano 的本体叫 `shinano维纳斯bill.vpk`，
		//      而配件叫 `neko.vpk`/`油光渲染.vpk`，公共前缀为空）。
		attachAnchors := make([]string, 0, 2)
		if len([]rune(prefix)) >= suiteMinNamePrefixRunes {
			attachAnchors = append(attachAnchors, prefix)
		}
		suiteSegment := segments[len(segments)-1]
		if len([]rune(suiteSegment)) >= suiteMinKeywordRunes && !strings.EqualFold(suiteSegment, prefix) {
			attachAnchors = append(attachAnchors, suiteSegment)
		}
		for _, anchor := range attachAnchors {
			folded := strings.ToLower(anchor)
			for _, mod := range index.mods {
				if len(keys) >= suiteMaxMembers {
					break
				}
				if len(mod.ResourceRoots) > 0 {
					continue // 已有自己命名空间的模块不靠名字吸附
				}
				if !strings.HasPrefix(strings.ToLower(mod.Name), folded) {
					continue
				}
				if containsStringValue(keys, mod.Key) {
					continue
				}
				keys = append(keys, mod.Key)
				names = append(names, mod.Name)
				attached++
			}
		}
		sort.Strings(keys)
		sort.SliceStable(names, func(i, j int) bool { return names[i] < names[j] })

		label := suiteLabel(root, prefix)
		reason := fmt.Sprintf("同一套件目录 models/%s（%d 个模块，通常需要一起启用）",
			root, len(keys))
		if attached > 0 {
			reason += fmt.Sprintf("；另含 %d 个只替换官方目标的本体（靠文件名前缀关联）", attached)
		}
		candidates = append(candidates, Candidate{
			Provider: "suite-namespace",
			Label:    label,
			Reason:   reason,
			Score:    SuiteNamespaceScore,
			Signals:  []string{suiteSignalLabel},
			Keys:     keys,
			Names:    names,
			Strategy: "all",
		})
	}
	return candidates
}

func dedupeMods(mods []Mod) []Mod {
	result := make([]Mod, 0, len(mods))
	seen := make(map[string]struct{}, len(mods))
	for _, mod := range mods {
		key := strings.ToLower(strings.TrimSpace(mod.Key))
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, mod)
	}
	return result
}

// suiteLabel 给套件组起名：优先用文件名公共前缀（更贴近用户看到的系列名），
// 否则用命名空间的套件段。
func suiteLabel(root string, prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	if len([]rune(trimmed)) >= suiteMinNamePrefixRunes {
		return trimmed + " 套装"
	}
	segments := strings.Split(root, "/")
	suite := segments[len(segments)-1]
	if suite == "" {
		suite = root
	}
	return suite + " 套装"
}

// containsStringValue 判断切片里是否已有该值。
func containsStringValue(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// longestCommonPrefix 返回一组名字的公共前缀（按 rune 比较）。
func longestCommonPrefix(values []string) string {
	if len(values) == 0 {
		return ""
	}
	prefix := []rune(values[0])
	for _, value := range values[1:] {
		runes := []rune(value)
		limit := len(prefix)
		if len(runes) < limit {
			limit = len(runes)
		}
		index := 0
		for index < limit && strings.EqualFold(string(prefix[index]), string(runes[index])) {
			index++
		}
		prefix = prefix[:index]
		if len(prefix) == 0 {
			return ""
		}
	}
	return string(prefix)
}
