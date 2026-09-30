// Package rules 持有「声明式标签规则表」，并用游戏本体索引校验表中每一条前缀/关键词
// 是否真的能命中本体文件。
//
// 解决什么问题：标签识别的历史故障不是「规则写错一个字」，而是「规则永远不可能命中」——
//
//	烟花盒关键词写 `firework`，本体模型却是 models/props_junk/explosive_box001.mdl；
//	高尔夫球杆关键词写 `golf_club`，本体是 models/weapons/melee/w_golfclub.mdl。
//
// 这类规则在真机上只表现为「少标」，不报错、不留痕。本包把它变成显式报告：
// 前缀解析 0 命中 = 错误（规则永远不可能生效）；本体关键词 0 命中 = 警告（可能只匹配
// 作者命名空间，仍有价值，所以只提示不阻断）。
//
// 规则数据本身在 internal/ruletable（唯一事实源，解析器也直接读它）；
// 本包只负责"用游戏本体索引校验它"和产出报告。
package rules

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"vpk-manager/internal/gamedata"
	"vpk-manager/internal/gamedata/entities"
	"vpk-manager/internal/parser"
	"vpk-manager/internal/ruletable"
)

// 规则表类型与加载器都在 internal/ruletable；这里保留别名，调用方无需关心位置。
type (
	Table          = ruletable.Table
	CharacterRules = ruletable.CharacterRules
	VoiceDirRule   = ruletable.VoiceDirRule
	FileKind       = ruletable.FileKind
	ContentRule    = ruletable.ContentRule
	MatchRule      = ruletable.MatchRule
)

// SchemaVersion 是规则表格式版本。
const SchemaVersion = ruletable.SchemaVersion

// Load 读取唯一事实源里的规则表。
func Load() (*Table, error) { return ruletable.Load() }

// MustLoad 供包内/测试使用；解析失败直接 panic（内置资源损坏属于构建期错误）。
func MustLoad() *Table { return ruletable.MustLoad() }

// Resolution 是一条规则在本体索引里的解析结果。
type Resolution struct {
	Scope    string `json:"scope"` // fileKind | contentRule | weaponPathRule
	Tag      string `json:"tag"`
	Pattern  string `json:"pattern"`
	Kind     string `json:"kind"` // prefix | keyword
	Resolved int    `json:"resolved"`
	Dead     bool   `json:"dead,omitempty"`
}

// Report 是一次校验的完整结果。
type Report struct {
	GeneratedAt  string       `json:"generatedAt"`
	IndexPaths   int          `json:"indexPaths"`
	Checked      int          `json:"checked"`
	DeadTotal    int          `json:"deadTotal"`
	DeadPrefixes []Resolution `json:"deadPrefixes,omitempty"`
	DeadKeywords []Resolution `json:"deadKeywords,omitempty"`
	// AcceptedDead 是"本体里 0 命中、但有意保留"的规则（见 knownStockAbsent）。
	AcceptedDead []Resolution `json:"acceptedDead,omitempty"`
	// WeaponScoped 为真表示武器关键词是限定在武器目录里统计的。
	WeaponScoped bool `json:"weaponScoped"`
	// CharacterVoiceChecks 记录每张语音目录表的真实解析情况（信息性，不影响 OK）：
	// 代码当前假设的路径 vs 实测存在的另一种布局，用来把 D5 这类"路径假设错误"变成数据。
	CharacterVoiceChecks []VoiceDirCheck `json:"characterVoiceChecks,omitempty"`
	OK                   bool            `json:"ok"`
	Notes                string          `json:"notes,omitempty"`
}

// VoiceDirCheck 是单个语音目录段的解析结果。
type VoiceDirCheck struct {
	Scope           string `json:"scope"` // survivor | infected
	Dir             string `json:"dir"`
	Character       string `json:"character"`
	AssumedPattern  string `json:"assumedPattern"`
	AssumedHits     int    `json:"assumedHits"`
	FallbackPattern string `json:"fallbackPattern,omitempty"`
	FallbackHits    int    `json:"fallbackHits,omitempty"`
}

// knownStockAbsent 是「本体里不存在、但有意保留」的规则白名单。
//
// 为什么需要：有些规则本来就是给**作者自建路径**用的（社区命名习惯），本体里查不到不代表它没用;
// 但只要它出现在这里，就必须写清原因，并且只能解释"1 个前缀 0 命中"这种确定性判断——
// 没有登记的一律按错误处理，避免白名单变成垃圾桶。
//
// key 形如 "<scope>|<tag>|<pattern>"。
var knownStockAbsent = map[string]string{
	"contentRule|载入画面|materials/vgui/loadingscreen/": "本体载入画面是 materials/vgui/loadingscreen_* 平铺文件；" +
		"保留该前缀以兼容自建 loadingscreen/ 子目录的作者（对应的本体规则已由 loadingscreen 关键词覆盖）",
}

// Validate 用本体索引校验整张规则表。
//
// index 为 nil 时（没有游戏目录）返回 OK=false 且 Notes 说明"无法校验"，
// 调用方应把它当作"未验证"而不是"规则坏了"。
//
// entityTable 是「本体实体表」（可为 nil，表示本轮不校验实体锚点）：
// 传 nil 不会影响规则表自身的校验结论。
func Validate(table *Table, index *gamedata.StockIndex, entityTable *entities.Table) Report {
	report := Report{GeneratedAt: time.Now().Format(time.RFC3339)}
	if table == nil {
		report.Notes = "规则表为空"
		return report
	}
	if index == nil {
		report.Notes = "本体索引不可用，无法校验规则；这不是规则错误，请先构建本体索引。"
		return report
	}
	report.IndexPaths = index.PathCount

	countPrefix := func(prefix string) int {
		return len(index.PathsWithPrefix(prefix))
	}
	countKeyword := func(keyword string, scopes []string) int {
		keyword = strings.ToLower(strings.TrimSpace(keyword))
		if keyword == "" {
			return 0
		}
		total := 0
		if len(scopes) == 0 {
			for _, path := range index.Paths {
				if strings.Contains(path, keyword) {
					total++
				}
			}
			return total
		}
		seen := make(map[string]struct{})
		for _, scope := range scopes {
			for _, path := range index.PathsWithPrefix(scope) {
				if _, dup := seen[path]; dup {
					continue
				}
				seen[path] = struct{}{}
				if strings.Contains(path, keyword) {
					total++
				}
			}
		}
		return total
	}

	// 1) 文件类别：前缀必须能命中本体（否则整类标签永远不会出现）。
	for _, kind := range table.FileKinds {
		for _, prefix := range kind.Prefixes {
			count := countPrefix(prefix)
			report.Checked++
			resolution := Resolution{
				Scope: "fileKind", Tag: kind.Tag, Pattern: prefix, Kind: "prefix",
				Resolved: count, Dead: count == 0,
			}
			if resolution.Dead {
				if _, accepted := knownStockAbsent[resolutionKey(resolution)]; accepted {
					report.AcceptedDead = append(report.AcceptedDead, resolution)
				} else {
					report.DeadPrefixes = append(report.DeadPrefixes, resolution)
				}
			}
		}
	}

	// 2) 内容规则：前缀必须能命中；关键词按"前缀限定后"统计（与匹配语义一致）。
	for _, rule := range table.ContentRules {
		for _, prefix := range rule.Prefixes {
			count := countPrefix(prefix)
			report.Checked++
			resolution := Resolution{
				Scope: "contentRule", Tag: rule.Tag, Pattern: prefix, Kind: "prefix",
				Resolved: count, Dead: count == 0,
			}
			if resolution.Dead {
				if _, accepted := knownStockAbsent[resolutionKey(resolution)]; accepted {
					report.AcceptedDead = append(report.AcceptedDead, resolution)
				} else {
					report.DeadPrefixes = append(report.DeadPrefixes, resolution)
				}
			}
		}
		for _, keyword := range rule.Keywords {
			count := countKeyword(keyword, rule.Prefixes)
			report.Checked++
			resolution := Resolution{
				Scope: "contentRule", Tag: rule.Tag, Pattern: keyword, Kind: "keyword",
				Resolved: count, Dead: count == 0,
			}
			if resolution.Dead {
				report.DeadKeywords = append(report.DeadKeywords, resolution)
			}
		}
	}

	// 3) 武器路径规则：限定在武器目录里统计（与 isWeaponAssetPath 的口径一致）。
	weaponScopes := weaponScopesForIndex(index)
	report.WeaponScoped = len(weaponScopes) > 0
	for _, rule := range table.WeaponPathRules {
		count := countKeyword(rule.Keyword, weaponScopes)
		report.Checked++
		resolution := Resolution{
			Scope: "weaponPathRule", Tag: rule.Tag, Pattern: rule.Keyword, Kind: "keyword",
			Resolved: count, Dead: count == 0,
		}
		if resolution.Dead {
			report.DeadKeywords = append(report.DeadKeywords, resolution)
		}
	}

	sortResolutions(report.DeadPrefixes)
	sortResolutions(report.DeadKeywords)
	sortResolutions(report.AcceptedDead)

	// 4) 角色语音目录：信息性统计（不影响 OK）。
	//    解析器目前假设 sound/player/{survivor,infected}/voice/<dir>/；
	//    实测特感语音其实在 sound/player/<dir>/（见 left4dead2/sound/player 目录），
	//    所以这里把"假设路径"和"另一布局"一起报出来，供 W5 修 D5 时对照。
	for _, rule := range table.CharacterRules.VoiceDirSurvivor {
		assumed := "sound/player/survivor/voice/" + rule.Dir + "/"
		report.CharacterVoiceChecks = append(report.CharacterVoiceChecks, VoiceDirCheck{
			Scope: "survivor", Dir: rule.Dir, Character: rule.Character,
			AssumedPattern: assumed, AssumedHits: countPrefix(assumed),
		})
	}
	for _, rule := range table.CharacterRules.VoiceDirInfected {
		assumed := "sound/player/infected/voice/" + rule.Dir + "/"
		fallback := "sound/player/" + rule.Dir + "/"
		report.CharacterVoiceChecks = append(report.CharacterVoiceChecks, VoiceDirCheck{
			Scope: "infected", Dir: rule.Dir, Character: rule.Character,
			AssumedPattern: assumed, AssumedHits: countPrefix(assumed),
			FallbackPattern: fallback, FallbackHits: countPrefix(fallback),
		})
	}

	// 5) 本体实体表锚点：每条锚点都必须能在本体索引里解析到文件，
	//    否则这条"精确证据"永远不可能命中（表漂移 / 换游戏版本后的静默失效）。
	if entityTable != nil {
		validateEntityAnchors(&report, entityTable, countPrefix)
	}
	report.DeadTotal = len(report.DeadPrefixes) + len(report.DeadKeywords)
	report.OK = len(report.DeadPrefixes) == 0
	report.Notes = validateNotes(report)
	return report
}

// validateEntityAnchors 校验本体实体表的每条锚点是否能在本体索引里解析到文件。
func validateEntityAnchors(report *Report, entityTable *entities.Table, countPrefix func(string) int) {
	for _, entity := range entityTable.Entities {
		label := entity.Tag
		if label == "" {
			label = entity.ID
		}
		for _, anchor := range entity.Anchors {
			count := countPrefix(anchor)
			report.Checked++
			resolution := Resolution{
				Scope: "entity", Tag: label, Pattern: anchor, Kind: "anchor",
				Resolved: count, Dead: count == 0,
			}
			if resolution.Dead {
				report.DeadPrefixes = append(report.DeadPrefixes, resolution)
			}
		}
	}
}

// validateNotes 汇总人类可读的结论。
func validateNotes(report Report) string {
	switch {
	case !report.OK:
		return "存在解析 0 命中的前缀/锚点：这些规则永远不可能生效（属于「少标」根因）。"
	case len(report.DeadKeywords) > 0:
		return fmt.Sprintf("前缀与锚点全部有效；有 %d 条关键词在本体里 0 命中——"+
			"它们可能只匹配作者命名空间（仍有用），也可能已经失效，逐条核对即可。"+
			"（另有 %d 条规则已在 knownStockAbsent 登记为有意保留）", len(report.DeadKeywords), len(report.AcceptedDead))
	default:
		return fmt.Sprintf("全部规则都能在本体索引里解析到文件（%d 条登记为有意保留）。", len(report.AcceptedDead))
	}
}

func sortResolutions(items []Resolution) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Scope != items[j].Scope {
			return items[i].Scope < items[j].Scope
		}
		if items[i].Tag != items[j].Tag {
			return items[i].Tag < items[j].Tag
		}
		return items[i].Pattern < items[j].Pattern
	})
}

func resolutionKey(item Resolution) string {
	return item.Scope + "|" + item.Tag + "|" + item.Pattern
}

// weaponScopesForIndex 返回武器关键词的统计范围。
// 口径与 parser.isWeaponAssetPath 保持一致：直接复用 parser.WeaponAssetRoots()，
// 避免这里再抄一份目录清单造成漂移（空目录自然贡献 0 命中，不影响统计）。
func weaponScopesForIndex(index *gamedata.StockIndex) []string {
	if index == nil {
		return nil
	}
	return parser.WeaponAssetRoots()
}
