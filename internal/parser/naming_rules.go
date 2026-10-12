// 命名规范化（路线图第 20 项）——词表映射（S1–S5）与替换对象提取层。
//
// 参考实现：sHartet/l4d2-addons 的 naming-rules v1.2 与 scripts/plan_v4.py（判定顺序、
// 占比阈值、上位名、幂等剥前缀）。评估文档：docs/development/mod-naming-normalization-eval.md；
// 63B 截断器在 naming_cap.go（本文件的组装直接复用它）。
//
// 本文件分三层（都可单测、无副作用）：
//   - CollectNamingEvidence —— 对一份 VPK 目录索引做单遍路径级证据收集：物件域/行（替换对象）
//     命中计数、资源类型计数、模型文件与 bsp/nav 标记、专项计数（手电筒 / 管理员插件）、
//     未归类计数。识别口径复用扫描管线的既有原语（collectContentTagsWithEvidence /
//     weaponPathTag / 角色规则），保证「建议依据」与界面上的标签识别一致。
//   - DeriveNameSuggestion —— 纯函数：证据 → 建议。判定顺序 S1 地图 → S5a 未归类过半 →
//     S2/S3 物件域（含 大而散 阈值）→ 专项 → S4 资源类型 → S5 保留原名；低置信一律
//     PendingReview 提示，绝不静默猜。
//   - SuggestNameForFile —— 只读入口：打开 VPK、收集证据、按「去掉 [标签]/隐藏前缀后」的
//     编辑体重命名段推导建议；外层修饰（[标签] / _ / .vpk）的字节量计入 63B 预算。
//
// 与参考实现的已知差异（v1，见评估文档「实施进展」）：
//   - 横切行（骨架动画/手势/残肢）不单独建行：LytVPK 的识别原语不产出这些行，落 S3/S4/S5
//     会保守处理，不会造成错误中段；
//   - 物件域行名用 LytVPK 自己的标签词汇（如 AK47 / 木喷），不做 20 前缀表的全名映射；
//   - 上位名只保留可机械判定的两条（副武器→手枪、主武器→霰弹枪）。
package parser

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"
)

// ---------------------------------------------------------------------------
// 证据收集
// ---------------------------------------------------------------------------

// NamingEvidence 是命名建议所需的路径级证据（一次收集，供纯函数消费）。
type NamingEvidence struct {
	Total           int                       // 计入统计的路径数（剔除 addoninfo / 预览图等噪声）
	Unclassified    int                       // 未落入任何已知规则/根目录的路径数（S5 判据）
	HasMapBSP       bool                      // 含 maps/*.bsp
	HasMapNav       bool                      // 含 maps/*.nav
	HasModelFile    bool                      // 含模型文件（.mdl/.vvd/.vtx/.phy/.ani）
	DomainRows      map[string]map[string]int // 物件域 → 行（替换对象） → 命中路径数
	TypeHits        map[string]int            // 资源类型（界面/音效/材质/模型/脚本/特效） → 命中数
	FlashlightHits  int                       // 专项·手电筒本体的命中数
	AdminPluginHits int                       // 专项·管理员插件的命中数
}

func (e *NamingEvidence) addRow(domain, row string) {
	if e.DomainRows == nil {
		e.DomainRows = make(map[string]map[string]int)
	}
	rows := e.DomainRows[domain]
	if rows == nil {
		rows = make(map[string]int)
		e.DomainRows[domain] = rows
	}
	rows[row]++
}

func (e *NamingEvidence) addType(kind string) {
	if e.TypeHits == nil {
		e.TypeHits = make(map[string]int)
	}
	e.TypeHits[kind]++
}

func (e *NamingEvidence) domainHits(domain string) int {
	total := 0
	for _, count := range e.DomainRows[domain] {
		total += count
	}
	return total
}

// namingModelFileExts 是「该包含模型文件」的判定扩展名（对齐参考规则的 MODEL_EXT）。
var namingModelFileExts = []string{".mdl", ".vvd", ".vtx", ".phy", ".ani"}

// 专项·手电筒：只认「手电筒本体」的核心路径（参考规则 2026-10-03 收窄后的口径）。
// 故意不认 materials/particle/(beam_)?flashlight* —— 那是粒子包/菜单的共享光束与光晕资源。
var namingFlashlightCore = []*regexp.Regexp{
	regexp.MustCompile(`^materials/effects/flashlight`),
	regexp.MustCompile(`^models/.*flashlight`),
	regexp.MustCompile(`^scripts/vscripts/[^/]*flashlight`),
}

// 专项·管理员插件：VScript 管理入口 + SourceMod / MetaMod（不认 sm_* 通用库）。
var namingAdminPluginCore = []*regexp.Regexp{
	regexp.MustCompile(`^scripts/vscripts/[^/]*admin`),
	regexp.MustCompile(`^scripts/clientmenu\.txt$`),
	regexp.MustCompile(`^(addons/)?(sourcemod|metamod)/`),
	regexp.MustCompile(`^cfg/sourcemod/`),
	regexp.MustCompile(`\.smx$`),
}

// 未归类判定的已知资源根：落在这些根下的路径即使没命中具体规则也算「已归类」。
var namingKnownRoots = []string{
	"resource/", "materials/", "sound/", "scenes/", "scripts/", "models/",
	"particles/", "maps/", "missions/", "cfg/", "addons/",
}

// 文件类别（fileKindRules）→ 资源类型前缀。
var namingKindTypes = map[string]string{
	"UI": "界面", "声音": "音效", "脚本": "脚本", "VScript": "脚本",
	"模型": "模型", "贴图": "材质", "粒子特效": "特效",
}

// 内容标签（contentTagRules 命中产物）→ 资源类型。
var namingContentTagTypes = map[string]string{
	"主菜单": "界面", "HUD": "界面", "准星": "界面", "血条": "界面",
	"伤害指示器": "界面", "人物语音表": "界面", "过场画面": "界面",
	"载入画面": "界面", "动态箭头": "界面",
	"尸潮": "音效", "警报": "音效", "唱片机": "音效", "音乐": "音效", "语音包": "音效",
	"贴花":   "材质",
	"粒子特效": "特效",
}

// 内容标签 → 物件域·消耗品 / 投掷物（对应参考表的两个域）。
var namingItemTagDomains = map[string]string{
	"土制炸弹": "投掷物", "燃烧瓶": "投掷物", "胆汁": "投掷物",
	"医疗包": "消耗品", "电击器": "消耗品", "止痛药": "消耗品", "肾上腺": "消耗品",
	"一代子弹堆": "消耗品", "二代子弹堆": "消耗品", "燃烧弹盒": "消耗品",
	"高爆弹盒": "消耗品", "激光瞄准盒": "消耗品",
	"汽油桶": "消耗品", "煤气罐": "消耗品", "氧气罐": "消耗品", "烟花盒": "消耗品",
}

// 内容标签 → 场景道具域（手电筒除外——它走专项判定，不落物件域）。
var namingSceneTagDomains = map[string]string{
	"梯子": "场景道具", "天空": "场景道具", "侏儒": "场景道具", "直升机": "场景道具",
	"海报": "场景道具", "船": "场景道具", "售货机": "场景道具", "电视": "场景道具",
	"屏幕": "场景道具", "货车": "场景道具", "面包车": "场景道具", "雕像": "场景道具",
}

// 武器标签 → 物件域。固定机关枪归主武器（与应用的「枪械」分类树一致；参考表的
// 固定武器·场景在其实测库中为空集，无样本可依）。
var namingWeaponTagDomains = map[string]string{
	"小手枪": "副武器", "马格南": "副武器",
	"AK47": "主武器", "M16": "主武器", "三连发": "主武器", "sg552": "主武器",
	"大狙": "主武器", "猎枪": "主武器", "军狙": "主武器", "鸟狙": "主武器",
	"木喷": "主武器", "一代连喷": "主武器", "铁喷": "主武器", "二代连喷": "主武器",
	"乌兹": "主武器", "消音": "主武器", "MP5": "主武器",
	"榴弹": "主武器", "榴弹发射器": "主武器", "M60": "主武器",
	"固定机关枪": "主武器", "一代固定机枪": "主武器", "二代固定机枪": "主武器",
	"棒球棍": "近战", "板球拍": "近战", "吉他": "近战", "平底锅": "近战",
	"高尔夫球杆": "近战", "消防斧": "近战", "砍刀": "近战", "武士刀": "近战",
	"电锯": "近战", "撬棍": "近战", "草叉": "近战", "铁铲": "近战",
	"警棍": "近战", "防爆盾": "近战", "匕首": "近战",
}

// 未知武器标签的兜底：用武器类别（weaponCategoryTag）映射域，未来新增型号自动生效。
var namingWeaponCategoryDomains = map[string]string{
	"手枪": "副武器",
	"步枪": "主武器", "狙击枪": "主武器", "霰弹枪": "主武器", "冲锋枪": "主武器",
	"榴弹发射器": "主武器", "M60": "主武器", "固定机关枪": "主武器",
	"近战": "近战",
}

// 角色显示名（关键字规则产出的标签 → 规范英文名；特感标签是小写）。
var namingCharacterVocabulary = map[string]string{
	"Bill": "Bill", "Francis": "Francis", "Louis": "Louis", "Zoey": "Zoey",
	"Coach": "Coach", "Ellis": "Ellis", "Nick": "Nick", "Rochelle": "Rochelle",
	"boomer": "Boomer", "charger": "Charger", "hunter": "Hunter", "jockey": "Jockey",
	"spitter": "Spitter", "smoker": "Smoker", "tank": "Tank", "witch": "Witch",
}

func namingMatchesAny(patterns []*regexp.Regexp, name string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(name) {
			return true
		}
	}
	return false
}

func namingHasKnownRoot(name string) bool {
	for _, root := range namingKnownRoots {
		if strings.HasPrefix(name, root) {
			return true
		}
	}
	return false
}

// namingWeaponDomainOfTag 把武器标签映射到物件域（含类别兜底）。
func namingWeaponDomainOfTag(tag string) (string, bool) {
	tag = canonicalWeaponTag(tag)
	if tag == "" {
		return "", false
	}
	if domain, ok := namingWeaponTagDomains[tag]; ok {
		return domain, true
	}
	if category := weaponCategoryTag(tag); category != "" {
		if domain, ok := namingWeaponCategoryDomains[category]; ok {
			return domain, true
		}
	}
	return "", false
}

// namingContentTagDomain 把内容标签映射到物件域（物品/场景道具/武器三类表合并）。
func namingContentTagDomain(tag string) (string, bool) {
	if domain, ok := namingItemTagDomains[tag]; ok {
		return domain, true
	}
	if domain, ok := namingSceneTagDomains[tag]; ok {
		return domain, true
	}
	return namingWeaponDomainOfTag(tag)
}

// namingCharacterRow 从一条路径里提取「替换对象=角色名」的证据（三条通道）：
// 标准语音目录（最权威）→ 角色模型/材质目录 token → 关键字规则（股票目录名等）。
// 变体标签（BillDeathPose 这类横切内容）不建行，避免把残肢/姿势包命名成角色替换。
func namingCharacterRow(name string) (string, bool) {
	if character := detectVoiceCharacter(name); character != "" {
		return character, true
	}
	if character := detectCharacterSubject(name); character != "" {
		return character, true
	}
	if !isCharacterAssetPath(name) {
		return "", false
	}
	for _, rule := range survivorVariantRules {
		if strings.Contains(name, rule.keyword) {
			return "", false
		}
	}
	for _, rule := range survivorRules {
		if strings.Contains(name, rule.keyword) {
			if display, ok := namingCharacterVocabulary[rule.tag]; ok {
				return display, true
			}
			return "", false
		}
	}
	for _, rule := range specialInfectedRules {
		if strings.Contains(name, rule.keyword) {
			if display, ok := namingCharacterVocabulary[rule.tag]; ok {
				return display, true
			}
			return "", false
		}
	}
	return "", false
}

// CollectNamingEvidence 对一份 VPK 目录索引做单遍路径级证据收集（只读目录名，不解压）。
// 识别口径与扫描管线一致：内容标签走 collectContentTagsWithEvidence（含本体锚点等通道），
// 武器走 weaponPathTag，角色走 detectVoiceCharacter / 角色规则。
func CollectNamingEvidence(archive *vpk.Archive) NamingEvidence {
	evidence := NamingEvidence{}
	if archive == nil {
		return evidence
	}
	pathTags := make(map[string]bool, 8)
	for i := range archive.Files {
		name := normalizeArchivePath(archive.Files[i].Name())
		if name == "" || isStructureNoisePath(name) {
			continue
		}
		evidence.Total++
		classified := false

		if strings.HasSuffix(name, ".bsp") {
			evidence.HasMapBSP = true
			classified = true
		}
		if strings.HasSuffix(name, ".nav") {
			evidence.HasMapNav = true
			classified = true
		}
		for _, ext := range namingModelFileExts {
			if strings.HasSuffix(name, ext) {
				evidence.HasModelFile = true
				classified = true
				break
			}
		}

		if namingMatchesAny(namingFlashlightCore, name) {
			evidence.FlashlightHits++
			classified = true
		}
		if namingMatchesAny(namingAdminPluginCore, name) {
			evidence.AdminPluginHits++
			classified = true
		}

		if character, ok := namingCharacterRow(name); ok {
			domain := "生还者"
			if character == "Common Infected" {
				domain = "普感"
			} else if isInfectedVoiceCharacter(character) {
				domain = "特感"
			}
			evidence.addRow(domain, character)
			classified = true
		}

		// 内容标签（与扫描同口径；临时集合按路径重置）。
		for tag := range pathTags {
			delete(pathTags, tag)
		}
		isItem := collectContentTagsWithEvidence(name, pathTags, nil)
		for tag := range pathTags {
			if domain, ok := namingContentTagDomain(tag); ok {
				evidence.addRow(domain, tag)
				classified = true
			}
			if kind, ok := namingContentTagTypes[tag]; ok {
				evidence.addType(kind)
				classified = true
			}
		}

		// 武器资源路径（与扫描同口径：先排除物品资源）。
		if isWeaponAssetPath(name, isItem) {
			if tag := weaponPathTag(name); tag != "" {
				if domain, ok := namingWeaponDomainOfTag(tag); ok {
					evidence.addRow(domain, tag)
					classified = true
				}
			}
		}

		// 资源类型：文件类别规则按序首个命中（避免 materials/vgui 同时计成 材质）。
		for _, kind := range fileKindRules {
			if kind.Tag == "" || len(kind.Prefixes) == 0 {
				continue
			}
			if hasPathPrefix(name, kind.Prefixes) {
				if kindType, ok := namingKindTypes[kind.Tag]; ok {
					evidence.addType(kindType)
					classified = true
				}
				break
			}
		}

		if !classified && !namingHasKnownRoot(name) {
			evidence.Unclassified++
		}
	}
	return evidence
}

// ---------------------------------------------------------------------------
// 建议推导（S1–S5）
// ---------------------------------------------------------------------------

// NamingSuggestion 是「按分类建议填充」的返回值（前端重命名弹窗消费）。
type NamingSuggestion struct {
	Stem          string `json:"stem"`          // 建议的编辑体（不含 [标签] / 隐藏前缀 / .vpk）
	Prefix        string `json:"prefix"`        // 分类前缀（如 生还者 / 界面 / 地图）
	Subject       string `json:"subject"`       // 替换对象（可为空）
	Stage         string `json:"stage"`         // S1 / S2 / S3 / S4 / S5 / 专项
	PendingReview bool   `json:"pendingReview"` // 低置信或同域多行：请人工复核
	Truncated     bool   `json:"truncated"`     // 标题段被 63B 截断
	Fits          bool   `json:"fits"`          // 前缀段本身放得进 63B（false 时无法压缩）
	Changed       bool   `json:"changed"`       // 建议与当前名不同
	Reason        string `json:"reason"`        // 依据说明（界面展示）
}

// 物件域优先级（参考规则：命中多域时取优先级最高者）。
var namingDomainOrder = []string{
	"生还者", "特感", "普感", "主武器", "副武器", "近战", "投掷物", "消耗品", "场景道具", "场景",
}

var namingDomainPrefix = map[string]string{
	"生还者": "生还者", "特感": "特感", "普感": "普感", "主武器": "主武器", "副武器": "副武器",
	"近战": "近战", "投掷物": "投掷物", "消耗品": "消耗品", "场景道具": "场景道具", "场景": "场景",
}

// 参考实现的阈值（plan_v4.py）：大而散（总量下域占比过低）保留原名；专项与模型类型
// 需要占比门槛，防止「顺带覆盖」抢走身份。
const (
	namingSparseMinTotal       = 500
	namingSparseMinRatio       = 0.05
	namingSpecialistMinRatio   = 0.25
	namingModelTypeMinRatio    = 0.20
	namingUnclassifiedMaxRatio = 0.5
)

// namingKnownPrefixes 是幂等剥前缀的识别集合（采用本体系的 19 个分类前缀 + 兜底）。
var namingKnownPrefixes = []string{
	"生还者", "特感", "普感", "主武器", "副武器", "近战", "投掷物", "消耗品", "场景道具", "场景",
	"模型", "地图", "音效", "界面", "特效", "脚本", "材质", "手电筒", "管理员插件", "其它",
}

func namingIsKnownPrefix(segment string) bool {
	for _, prefix := range namingKnownPrefixes {
		if segment == prefix {
			return true
		}
	}
	return false
}

// namingIsKnownSubject 判断一个「旧替换对象」段是否可安全剥掉（只认已知词汇，
// 避免把原标题里的词吃掉）。
func namingIsKnownSubject(segment string) bool {
	if _, ok := namingWeaponTagDomains[segment]; ok {
		return true
	}
	if _, ok := namingItemTagDomains[segment]; ok {
		return true
	}
	if _, ok := namingSceneTagDomains[segment]; ok {
		return true
	}
	for _, display := range namingCharacterVocabulary {
		if segment == display {
			return true
		}
	}
	switch segment {
	case "手枪", "霰弹枪", "Common Infected", "手电筒", "管理员插件":
		return true
	}
	return false
}

// namingCandidateDomains 按优先级返回有命中的物件域。
func namingCandidateDomains(evidence NamingEvidence) []string {
	candidates := make([]string, 0, len(namingDomainOrder))
	for _, domain := range namingDomainOrder {
		if evidence.domainHits(domain) > 0 {
			candidates = append(candidates, domain)
		}
	}
	return candidates
}

// namingPickSubject 在同一域的多行里挑中段：单一命中直接用；两条可机械判定的上位名
// （副武器手枪 / 主武器霰弹枪）合并；其余取占比最高行并标记待复核。
func namingPickSubject(domain string, rows map[string]int) (string, bool) {
	if len(rows) == 0 {
		return "", false
	}
	if len(rows) == 1 {
		for row := range rows {
			return row, false
		}
	}
	if domain == "副武器" {
		if _, a := rows["小手枪"]; a {
			if _, b := rows["马格南"]; b {
				if len(rows) == 2 {
					return "手枪", false
				}
			}
		}
	}
	if domain == "主武器" {
		shotgunRows := []string{"木喷", "一代连喷", "铁喷", "二代连喷"}
		all := len(rows) >= 2
		for row := range rows {
			match := false
			for _, candidate := range shotgunRows {
				if row == candidate {
					match = true
					break
				}
			}
			if !match {
				all = false
				break
			}
		}
		if all {
			return "霰弹枪", false
		}
	}

	best, bestCount := "", -1
	for row, count := range rows {
		if count > bestCount || (count == bestCount && row < best) {
			best, bestCount = row, count
		}
	}
	return best, true
}

// namingTopResourceType 取占比最高的资源类型（不含 模型——模型有单独门槛与兜底）。
func namingTopResourceType(hits map[string]int) string {
	best, bestCount := "", 0
	for _, kind := range []string{"界面", "音效", "材质", "脚本", "特效"} {
		if hits[kind] > bestCount {
			best, bestCount = kind, hits[kind]
		}
	}
	return best
}

// DeriveNameSuggestion 把证据推演成命名建议。currentStem 是去掉 [标签]/隐藏前缀/.vpk
// 后的编辑体；outerBytes 是外层修饰（[标签] 与 _）占掉的 ANSI 字节数（计入 63B 预算）。
func DeriveNameSuggestion(currentStem string, evidence NamingEvidence, outerBytes int) NamingSuggestion {
	budget := AddonStemMaxANSIBytes - outerBytes
	if budget < 1 {
		budget = 1
	}
	total := evidence.Total
	if total < 1 {
		total = 1
	}

	keepOriginal := func(reason string, pending bool) NamingSuggestion {
		stem, truncated := CapAddonStemToANSIBytes(currentStem, budget)
		return NamingSuggestion{
			Stem:          stem,
			Stage:         "S5",
			PendingReview: pending,
			Truncated:     truncated,
			Fits:          true,
			Changed:       stem != currentStem,
			Reason:        reason,
		}
	}

	// S1 · 地图【高置信】
	if evidence.HasMapBSP || evidence.HasMapNav {
		return namingComposeSuggestion(currentStem, "地图", "", "S1", false, "S1 含 maps/*.bsp 或 maps/*.nav（地图包）", budget)
	}

	// S5a · 未归类过半（标签体系没覆盖到，判定不可靠）
	if ratio := float64(evidence.Unclassified) / float64(total); ratio > namingUnclassifiedMaxRatio {
		return keepOriginal(fmt.Sprintf("S5 未归类占比 %.0f%%，保留原名", ratio*100), true)
	}

	// S2/S3 · 物件域【高/中置信】
	if candidates := namingCandidateDomains(evidence); len(candidates) > 0 {
		best := candidates[0]
		ratio := float64(evidence.domainHits(best)) / float64(total)
		stage, pending := "S2", false
		if !evidence.HasModelFile {
			// S3 · 无模型证据（贴图/音效类物件）→ 仍用物件域前缀，列入待复核
			stage, pending = "S3", true
		} else if total >= namingSparseMinTotal && ratio < namingSparseMinRatio {
			return keepOriginal(fmt.Sprintf("S5 大而散（总 %d 条路径、域占比 %.1f%%），保留原名", total, ratio*100), true)
		}
		subject, multiRow := namingPickSubject(best, evidence.DomainRows[best])
		if multiRow {
			pending = true
		}
		reason := fmt.Sprintf("%s 物件域=%s（占比 %.0f%%）", stage, best, ratio*100)
		if multiRow {
			reason += "；同域多行，中段取占比最高行"
		}
		return namingComposeSuggestion(currentStem, namingDomainPrefix[best], subject, stage, pending, reason, budget)
	}

	// 专项（放在物件域之后、资源类型之前）
	if float64(evidence.AdminPluginHits)/float64(total) >= namingSpecialistMinRatio {
		return namingComposeSuggestion(currentStem, "管理员插件", "", "专项", false, "专项：管理员插件入口资源占比达标", budget)
	}
	if float64(evidence.FlashlightHits)/float64(total) >= namingSpecialistMinRatio {
		return namingComposeSuggestion(currentStem, "手电筒", "", "专项", false, "专项：手电筒本体资源占比达标", budget)
	}

	// S4 · 资源类型【高置信】（模型需 20% 门槛，防「顺带带模型」）
	if evidence.HasModelFile && float64(evidence.TypeHits["模型"])/float64(total) >= namingModelTypeMinRatio {
		return namingComposeSuggestion(currentStem, "模型", "", "S4", false, "S4 资源类型=模型（含模型文件且占比达标）", budget)
	}
	if kind := namingTopResourceType(evidence.TypeHits); kind != "" {
		return namingComposeSuggestion(currentStem, kind, "", "S4", false, "S4 资源类型="+kind, budget)
	}
	if evidence.TypeHits["模型"] > 0 {
		return namingComposeSuggestion(currentStem, "模型", "", "S4", false, "S4 资源类型=模型", budget)
	}

	// S5 · 保留原前缀【低置信】
	return keepOriginal("S5 无可用标签，保留原名", true)
}

// namingNormalizeTitleSegment 是参考规则的「硬性改写」之一：Windows 非法字符全角化
// （保证建议可直接保存 —— 后端重命名校验会拒绝半角非法字符），并清掉换行/制表符/
// 控制字符与结尾句点/空格。原标题的其它字节一律不动。
func namingNormalizeTitleSegment(text string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch r {
		case '\\':
			return '＼'
		case '/':
			return '／'
		case ':':
			return '：'
		case '*':
			return '＊'
		case '?':
			return '？'
		case '"':
			return '＂'
		case '<':
			return '＜'
		case '>':
			return '＞'
		case '|':
			return '｜'
		}
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, text)
	return strings.TrimRight(cleaned, ". ")
}

// namingComposeSuggestion 组装「分类-替换对象-原标题」并处理幂等（前缀匹配式剥旧前缀）。
func namingComposeSuggestion(currentStem, prefix, subject, stage string, pending bool, reason string, budget int) NamingSuggestion {
	want := prefix
	if subject != "" {
		want = prefix + "-" + subject
	}
	result := NamingSuggestion{
		Prefix:        prefix,
		Subject:       subject,
		Stage:         stage,
		PendingReview: pending,
		Fits:          true,
		Reason:        reason,
	}

	// 幂等：当前名已经以目标「前缀-替换对象」开头 → 原样保留（规范库重跑 0 改动）。
	if currentStem == want || strings.HasPrefix(currentStem, want+"-") {
		stem, truncated := CapAddonStemToANSIBytes(namingNormalizeTitleSegment(currentStem), budget)
		result.Stem = stem
		result.Truncated = truncated
		result.Changed = stem != currentStem
		result.Reason = reason + "；已符合规范"
		return result
	}

	// 剥旧前缀：只认已知分类段（防止把标题里的短横吃掉）；旧替换对象段只在它是
	// 已知词汇时才随之下剥（替换对象自带短横，如 AK-47 —— 不能按短横切分猜）。
	rest := currentStem
	strippedPrefix := false
	if segment, tail, ok := namingSplitFirstSegment(currentStem); ok && namingIsKnownPrefix(segment) {
		rest = tail
		strippedPrefix = true
	}
	if strippedPrefix && subject != "" && rest != "" {
		if segment, tail, ok := namingSplitFirstSegment(rest); ok && namingIsKnownSubject(segment) {
			rest = tail
		}
	}

	head := prefix + "-"
	if subject != "" {
		head += subject + "-"
	}
	rest = namingNormalizeTitleSegment(rest)
	if rest == "" {
		// 整名就是旧前缀（如「材质」）或旧替换对象：退化为纯前缀。
		result.Stem = strings.TrimSuffix(head, "-")
		result.Changed = result.Stem != currentStem
		return result
	}
	stem, truncated, fits := ComposeAddonStem(head, rest, budget)
	result.Stem = stem
	result.Truncated = truncated
	result.Fits = fits
	result.Changed = stem != currentStem
	return result
}

// namingSplitFirstSegment 按第一个短横把字符串切成两段。
func namingSplitFirstSegment(value string) (segment, tail string, ok bool) {
	index := strings.Index(value, "-")
	if index < 0 {
		return "", "", false
	}
	return value[:index], value[index+1:], true
}

// ---------------------------------------------------------------------------
// 只读入口
// ---------------------------------------------------------------------------

// SuggestNameForFile 为单个 VPK 文件生成规范化命名建议（只读：只解目录索引，不写盘）。
func SuggestNameForFile(filePath string) (NamingSuggestion, error) {
	opener := vpk.Single(filePath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return NamingSuggestion{}, err
	}
	base := filepath.Base(filePath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	editStem, outerBytes := namingEditContext(stem)
	evidence := CollectNamingEvidence(archive)
	return DeriveNameSuggestion(editStem, evidence, outerBytes), nil
}

// namingEditContext 返回重命名弹窗编辑体与「外层修饰」的 ANSI 字节数：
// [标签] 前缀与隐藏 `_`（编辑时隐藏、保存时还原——63B 预算必须把它们算进去，
// 与体检 filename_too_long 的「整段主干」口径一致）。
func namingEditContext(stem string) (string, int) {
	outerBytes := 0
	if strings.HasPrefix(stem, "_") {
		outerBytes++
	}
	primary, secondary, withoutTags, ok := ParseFilenameTags(stem)
	if !ok {
		return strings.TrimPrefix(stem, "_"), outerBytes
	}
	tags := make([]string, 0, 1+len(secondary))
	if tag := strings.TrimSpace(primary); tag != "" {
		tags = append(tags, tag)
	}
	for _, tag := range secondary {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			tags = append(tags, trimmed)
		}
	}
	if len(tags) > 0 {
		outerBytes += ANSIByteLength("[" + strings.Join(tags, "+") + "]")
	}
	return strings.TrimPrefix(withoutTags, "_"), outerBytes
}
