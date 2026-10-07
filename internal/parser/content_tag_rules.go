package parser

import (
	"strings"

	"vpk-manager/internal/gamedata/entities"
	"vpk-manager/internal/ruletable"
)

// contentTagRule maps Source resource-path evidence to a Chinese secondary tag.
// Directory and file names inside a VPK are normally English; the tag is the
// localized label shown to the player.  Rules deliberately require a Source
// content root so a word in a workshop title or unrelated UI asset cannot
// classify the whole VPK.
type contentTagRule struct {
	tag      string
	prefixes []string
	keywords []string
	isItem   bool
}

var contentTagRules = buildContentTagRules(ruletable.MustLoad().ContentRules)

// buildContentTagRules 把规则表（JSON）转成解析器内部使用的形态。
// 规则表是唯一事实源：改规则改 JSON，不再有 Go 里私藏的一份。
func buildContentTagRules(rules []ruletable.ContentRule) []contentTagRule {
	out := make([]contentTagRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, contentTagRule{
			tag:      rule.Tag,
			prefixes: rule.Prefixes,
			keywords: rule.Keywords,
			isItem:   rule.Item,
		})
	}
	return out
}

// collectContentTags returns whether the path is a known non-weapon item.
// This lets the type detector avoid classifying eq_medkit and similar assets as
// "武器" merely because Source keeps their models under a weapons directory.
func collectContentTags(name string, tags map[string]bool) bool {
	return collectContentTagsWithEvidence(name, tags, nil)
}

// collectContentTagsWithEvidence 是带证据登记的版本（W6）：多通道命中时记录
// "这个标签由哪条规则、哪条真实路径得出"，供 Mod 详情与外部清单展示。
func collectContentTagsWithEvidence(name string, tags map[string]bool, evidence *tagEvidenceRecorder) bool {
	collectWorkshopContentCategories(name, tags)
	recordContentCategoryEvidence(name, tags, evidence)

	// 本体实体锚点：mod 里出现与本体完全相同的路径 → 明确替换了该实体。
	// 纯新增通道（命中就加标签），不参与任何"排除"判断。
	isItem := applyStockEntityAnchors(name, tags, evidence)
	// 本体基名 token：作者命名空间里的"参数包/特效包"（materials/models/<作者>/ak47/…）。
	if applyStockTokenEvidence(name, tags, evidence) {
		isItem = true
	}
	// 本体独占资源前缀：实体表里由本体索引推导的槽位（音效 / 贴图 / 图标目录）。
	// 只认"全表唯一属于一个实体"的前缀，作者命名空间不会命中。
	for _, hit := range stockOwnerPrefixHits(name) {
		if hit.IsItem {
			tags[hit.Tag] = true
			applyItemAggregateTags(hit.Tag, tags)
			isItem = true
		} else {
			addWeaponTag(hit.Tag, tags)
		}
		evidence.record(hit.Tag, "stock:"+hit.EntityID, EvidenceLevelPattern, name)
	}
	for _, rule := range contentTagRules {
		if !rule.matches(name) {
			continue
		}
		evidence.record(rule.tag, "content:"+rule.tag, EvidenceLevelPattern, name)
		if rule.isItem {
			tags[rule.tag] = true
			applyItemAggregateTags(rule.tag, tags)
		} else {
			// 非物品规则也可能是武器标签（匕首 / 防爆盾 …）：走 addWeaponTag 才会补
			// 「近战 / 所有枪械」这类聚合标签；普通内容标签在这里等价于直接置位。
			addWeaponTag(rule.tag, tags)
		}
		isItem = isItem || rule.isItem
	}
	applyMediaSceneAggregateTags(tags)
	return isItem
}

// applyMediaSceneAggregateTags 把"音画与场景"这一族标签汇总成一个聚合标签，
// 让筛选预设的「查看全部」只靠一个标签就能一次列全（音乐 / 贴花 / 载具 / 粒子特效）。
func applyMediaSceneAggregateTags(tags map[string]bool) {
	for _, member := range []string{"音乐", "贴花", "载具", "粒子特效"} {
		if tags[member] {
			tags["音画场景"] = true
			return
		}
	}
}

// recordContentCategoryEvidence 记录文件类别标签的来源（UI / 声音 / 粒子特效 …）。
func recordContentCategoryEvidence(name string, tags map[string]bool, evidence *tagEvidenceRecorder) {
	for _, kind := range fileKindRules {
		if kind.Tag != "" && tags[kind.Tag] {
			evidence.record(kind.Tag, "category:"+kind.Tag, EvidenceLevelPattern, name)
		}
	}
}

// fileKindRules 是「文件类别」规则（UI / 声音 / 脚本 / VScript / 模型 / 贴图 / 粒子特效），
// 与其它规则一样来自唯一事实源 rules.json。
var fileKindRules = ruletable.MustLoad().FileKinds

// collectWorkshopContentCategories maps the broad content groups exposed by
// the Left 4 Dead 2 Workshop to localized filter tags.  A VPK may legitimately
// carry several groups, so these tags are additive evidence rather than a
// replacement for the primary type.
func collectWorkshopContentCategories(name string, tags map[string]bool) {
	for _, kind := range fileKindRules {
		if kind.Tag == "" {
			continue
		}
		for _, prefix := range kind.Prefixes {
			if prefix != "" && strings.HasPrefix(name, prefix) {
				tags[kind.Tag] = true
				break
			}
		}
	}
}

// applyItemAggregateTags 由具体物品标签推导聚合标签（物品 / 投掷物 / 医疗物品 / 弹药堆 / 盒子 …）。
// 抽出来是为了让「内容规则」与「本体实体锚点」两条通道共用同一套聚合语义。
func applyItemAggregateTags(tag string, tags map[string]bool) {
	tag = CanonicalTag(tag)
	if tag == "" {
		return
	}
	tags["物品"] = true
	if isThrowableItemTag(tag) {
		tags["投掷物"] = true
		tags["所有投掷物品"] = true
	}
	if isMedicalItemTag(tag) {
		tags["医疗物品"] = true
		tags["所有医疗物品"] = true
	}
	if isAmmoItemTag(tag) {
		tags["弹药堆"] = true
		tags["盒子"] = true
	}
	switch tag {
	case "燃烧弹盒":
		tags["燃烧弹"] = true
		tags["盒子"] = true
	case "高爆弹盒":
		tags["高爆弹"] = true
		tags["盒子"] = true
	case "激光瞄准盒":
		tags["镭射"] = true
		tags["盒子"] = true
	}
}

// stockEntities 是内置的「本体实体表」，由游戏自带脚本生成
// （生成器见 internal/gamedata/entitygen，命令：--generate-entity-table）。
var stockEntities = entities.MustLoad()

// applyStockEntityAnchors 用「精确本体路径」判定实体，返回是否命中物品实体。
//
// 这是精确证据通道：models/w_models/weapons/w_pumpshotgun_A.mdl 是 Chrome 连喷的世界模型
// （不是木喷），models/weapons/melee/w_golfclub.mdl 才是高尔夫球杆——都由游戏脚本声明，
// 不靠命名猜测。命中只加标签，不参与排除。
func applyStockEntityAnchors(name string, tags map[string]bool, evidence *tagEvidenceRecorder) bool {
	if stockEntities == nil {
		return false
	}
	hits := stockEntities.Lookup(name)
	if len(hits) == 0 {
		return false
	}
	isItem := false
	for _, hit := range hits {
		switch {
		case hit.Character != "":
			applyStockCharacterAnchors(hit.Character, tags)
			evidence.record(hit.Character, "entity:"+hit.EntityID, EvidenceLevelExact, name)
		case hit.IsItem:
			isItem = true
			tags[hit.Tag] = true
			applyItemAggregateTags(hit.Tag, tags)
			evidence.record(hit.Tag, "entity:"+hit.EntityID, EvidenceLevelExact, name)
		default:
			addWeaponTag(hit.Tag, tags)
			evidence.record(hit.Tag, "entity:"+hit.EntityID, EvidenceLevelExact, name)
		}
	}
	return isItem
}

// applyStockCharacterAnchors 给角色资产（特感爪子 / 幸存者手臂）补角色标签，
// 口径与 collectCharacterTags 一致：特感用小写名 + 特殊感染者，幸存者用姓名 + 幸存者。
func applyStockCharacterAnchors(character string, tags map[string]bool) {
	character = strings.TrimSpace(character)
	if character == "" {
		return
	}
	switch {
	case character == "Common Infected":
		tags["common"] = true
		tags["普通感染者"] = true
	case isInfectedVoiceCharacter(character):
		tags[strings.ToLower(character)] = true
		tags["特殊感染者"] = true
	default:
		tags[character] = true
		tags["幸存者"] = true
	}
	// 角色资产同属"人物内容"（与 collectSupplementaryTypeTags 语义一致）；
	// 若主分类本来就是人物，最后会被 delete(secondaryTags, PrimaryTag) 去掉。
	tags["人物"] = true
}

func isThrowableItemTag(tag string) bool {
	switch tag {
	case "土制炸弹", "燃烧瓶", "胆汁":
		return true
	default:
		return false
	}
}

func isMedicalItemTag(tag string) bool {
	switch tag {
	case "医疗包", "电击器", "止痛药", "肾上腺":
		return true
	default:
		return false
	}
}

func isAmmoItemTag(tag string) bool {
	switch tag {
	case "一代子弹堆", "二代子弹堆":
		return true
	default:
		return false
	}
}

func (rule contentTagRule) matches(name string) bool {
	if !hasPathPrefix(name, rule.prefixes) {
		return false
	}
	if len(rule.keywords) == 0 {
		return true
	}
	for _, keyword := range rule.keywords {
		if strings.Contains(name, keyword) {
			return true
		}
	}
	return false
}

func hasPathPrefix(name string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
