// Package entitygen 从游戏自带脚本生成「本体实体表」。
//
// 数据来源（全部是 Valve 自己写的权威声明，不是命名猜测）：
//   - `scripts/weapon_*.txt`：`playermodel` / `viewmodel` / `CharacterViewmodelAddon`
//   - `scripts/melee/*.txt`：近战武器的 `viewmodel` / `playermodel`
//
// 实测反直觉的例子（README of coffeegrind123/l4d2-mod-manager 也点名过）：
//
//	weapon_pumpshotgun      → models/w_models/weapons/w_shotgun.mdl      （木喷）
//	weapon_shotgun_chrome   → models/w_models/weapons/w_pumpshotgun_A.mdl（铁喷）
//
// 所以锚点一律取自脚本，中文名只在下面的 scriptSpecs 里维护（那部分是展示层，无法从游戏推导）。
package entitygen

import (
	"bytes"
	"fmt"
	"sort"
	"strings"
	"time"

	"l4d2-manager-next/pkg/valve/vdf"

	"vpk-manager/internal/gamedata"
	"vpk-manager/internal/gamedata/entities"
)

// spec 是"脚本 → 展示标签"的人工维护部分。留空 Tag 表示只产出角色归属。
type spec struct {
	Tag       string
	Category  string
	Group     string
	IsItem    bool
	Official  bool
	Character string
	Skip      bool
}

// scriptSpecs 以脚本路径为键（最明确，避免 weapon_knife 与 melee/knife 之类混淆）。
var scriptSpecs = map[string]spec{
	// 枪械
	"scripts/weapon_pistol.txt":           {Tag: "小手枪", Category: "手枪", Group: "手枪", Official: true},
	"scripts/weapon_pistol_magnum.txt":    {Tag: "马格南", Category: "手枪", Group: "手枪", Official: true},
	"scripts/weapon_smg.txt":              {Tag: "乌兹", Category: "冲锋枪", Group: "冲锋枪", Official: true},
	"scripts/weapon_smg_silenced.txt":     {Tag: "消音", Category: "冲锋枪", Group: "冲锋枪", Official: true},
	"scripts/weapon_smg_mp5.txt":          {Tag: "MP5", Category: "冲锋枪", Group: "冲锋枪", Official: true},
	"scripts/weapon_pumpshotgun.txt":      {Tag: "木喷", Category: "霰弹枪", Group: "霰弹枪", Official: true},
	"scripts/weapon_shotgun_chrome.txt":   {Tag: "铁喷", Category: "霰弹枪", Group: "霰弹枪", Official: true},
	"scripts/weapon_autoshotgun.txt":      {Tag: "一代连喷", Category: "霰弹枪", Group: "霰弹枪", Official: true},
	"scripts/weapon_shotgun_spas.txt":     {Tag: "二代连喷", Category: "霰弹枪", Group: "霰弹枪", Official: true},
	"scripts/weapon_rifle.txt":            {Tag: "M16", Category: "步枪", Group: "步枪", Official: true},
	"scripts/weapon_rifle_ak47.txt":       {Tag: "AK47", Category: "步枪", Group: "步枪", Official: true},
	"scripts/weapon_rifle_desert.txt":     {Tag: "三连发", Category: "步枪", Group: "步枪", Official: true},
	"scripts/weapon_rifle_sg552.txt":      {Tag: "sg552", Category: "步枪", Group: "步枪", Official: true},
	"scripts/weapon_rifle_m60.txt":        {Tag: "M60", Category: "M60", Group: "M60", Official: true},
	"scripts/weapon_hunting_rifle.txt":    {Tag: "猎枪", Category: "狙击枪", Group: "狙击枪", Official: true},
	"scripts/weapon_sniper_military.txt":  {Tag: "军狙", Category: "狙击枪", Group: "狙击枪", Official: true},
	"scripts/weapon_sniper_scout.txt":     {Tag: "鸟狙", Category: "狙击枪", Group: "狙击枪", Official: true},
	"scripts/weapon_sniper_awp.txt":       {Tag: "大狙", Category: "狙击枪", Group: "狙击枪", Official: true},
	"scripts/weapon_grenade_launcher.txt": {Tag: "榴弹发射器", Category: "榴弹发射器", Group: "榴弹发射器", Official: true},

	// 物品与可携带物
	"scripts/weapon_first_aid_kit.txt": {Tag: "医疗包", Category: "物品", Group: "医疗物品", IsItem: true},
	"scripts/weapon_pain_pills.txt":    {Tag: "止痛药", Category: "物品", Group: "医疗物品", IsItem: true},
	"scripts/weapon_adrenaline.txt":    {Tag: "肾上腺", Category: "物品", Group: "医疗物品", IsItem: true},
	"scripts/weapon_defibrillator.txt": {Tag: "电击器", Category: "物品", Group: "医疗物品", IsItem: true},
	"scripts/weapon_pipe_bomb.txt":     {Tag: "土制炸弹", Category: "物品", Group: "投掷物", IsItem: true},
	"scripts/weapon_molotov.txt":       {Tag: "燃烧瓶", Category: "物品", Group: "投掷物", IsItem: true},
	"scripts/weapon_vomitjar.txt":      {Tag: "胆汁", Category: "物品", Group: "投掷物", IsItem: true},
	"scripts/weapon_gascan.txt":        {Tag: "汽油桶", Category: "物品", IsItem: true},
	"scripts/weapon_propanetank.txt":   {Tag: "煤气罐", Category: "物品", IsItem: true},
	"scripts/weapon_oxygentank.txt":    {Tag: "氧气罐", Category: "物品", IsItem: true},
	// 本体里"烟花盒"的模型叫 explosive_box001.mdl —— 关键词表写 firework 永远匹配不到（D2）。
	"scripts/weapon_fireworkcrate.txt": {Tag: "烟花盒", Category: "物品", IsItem: true},
	// 可乐（D3）：本体存在但过去完全没有对应标签。
	"scripts/weapon_cola_bottles.txt":           {Tag: "可乐", Category: "物品", IsItem: true},
	"scripts/weapon_gnome.txt":                  {Tag: "侏儒", Category: "物品", IsItem: true},
	"scripts/weapon_ammo_pack.txt":              {Tag: "高爆弹盒", Category: "物品", IsItem: true},
	"scripts/weapon_upgradepack_explosive.txt":  {Tag: "高爆弹盒", Category: "物品", IsItem: true},
	"scripts/weapon_upgradepack_incendiary.txt": {Tag: "燃烧弹盒", Category: "物品", IsItem: true},

	// 特感爪子（D4）：本体 playermodel 是共享占位（w_pistol_a.mdl），只能用 viewmodel 当锚点。
	"scripts/weapon_boomer_claw.txt":  {Character: "Boomer"},
	"scripts/weapon_charger_claw.txt": {Character: "Charger"},
	"scripts/weapon_hunter_claw.txt":  {Character: "Hunter"},
	"scripts/weapon_jockey_claw.txt":  {Character: "Jockey"},
	"scripts/weapon_smoker_claw.txt":  {Character: "Smoker"},
	"scripts/weapon_spitter_claw.txt": {Character: "Spitter"},
	"scripts/weapon_tank_claw.txt":    {Character: "Tank"},

	// 这两个是基类/清单，playermodel 是占位模型，必须跳过（否则会把 gascan/claw 当成它们的锚点）。
	"scripts/weapon_melee.txt":    {Skip: true},
	"scripts/weapon_manifest.txt": {Skip: true},

	// 近战
	"scripts/melee/baseball_bat.txt":    {Tag: "棒球棍", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/cricket_bat.txt":     {Tag: "板球拍", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/crowbar.txt":         {Tag: "撬棍", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/electric_guitar.txt": {Tag: "吉他", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/fireaxe.txt":         {Tag: "消防斧", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/frying_pan.txt":      {Tag: "平底锅", Category: "近战", Group: "近战", Official: true},
	// 本体路径是 w_golfclub.mdl（没有下划线）——关键词表写 golf_club 永远匹配不到（D8）。
	"scripts/melee/golfclub.txt":       {Tag: "高尔夫球杆", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/katana.txt":         {Tag: "武士刀", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/knife.txt":          {Tag: "匕首", Category: "近战", Group: "近战"},
	"scripts/melee/machete.txt":        {Tag: "砍刀", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/pitchfork.txt":      {Tag: "草叉", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/shovel.txt":         {Tag: "铁铲", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/tonfa.txt":          {Tag: "警棍", Category: "近战", Group: "近战", Official: true},
	"scripts/melee/melee_manifest.txt": {Skip: true},
	"scripts/weapon_chainsaw.txt":      {Tag: "电锯", Category: "近战", Group: "近战", Official: true},
}

// armsCharacters 把 CharacterViewmodelAddon 里的内部名映射到玩家认识的角色名。
var armsCharacters = map[string]string{
	"coach":     "Coach",
	"mechanic":  "Ellis",
	"producer":  "Rochelle",
	"gambler":   "Nick",
	"namvet":    "Bill",
	"biker":     "Francis",
	"manager":   "Louis",
	"teenangst": "Zoey",
	"teengirl":  "Zoey",
}

// Generate 读取游戏脚本并生成实体表。只读游戏目录。
func Generate(gameRoot string) (*entities.Table, error) {
	files, err := gamedata.ReadGameFiles(gameRoot, func(path string) bool {
		if !strings.HasSuffix(path, ".txt") {
			return false
		}
		return strings.HasPrefix(path, "scripts/weapon_") || strings.HasPrefix(path, "scripts/melee/")
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("未在 %s 找到任何 scripts/weapon_*.txt 或 scripts/melee/*.txt", gameRoot)
	}

	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	// 锚点 → 实体：同一路径可能被多个脚本声明（例如 ammo_pack 与 upgradepack_explosive），
	// 用 map 归并，最后统一排序输出，保证生成结果可复现。
	type acc struct {
		entity entities.Entity
	}
	byID := make(map[string]*acc)
	armsByPath := make(map[string]string) // 手臂模型路径 → 角色
	unmapped := make([]string, 0)

	for _, path := range paths {
		item, known := scriptSpecs[path]
		if !known {
			unmapped = append(unmapped, path)
			continue
		}
		if item.Skip {
			continue
		}

		parsed, err := parseScript(files[path])
		if err != nil {
			return nil, fmt.Errorf("解析 %s 失败: %w", path, err)
		}

		id := entityIDFor(path)
		anchors := make([]string, 0, 2)
		if item.Character != "" {
			// 角色资产：playermodel 多为共享占位，只取 viewmodel。
			if parsed.viewmodel != "" {
				anchors = append(anchors, parsed.viewmodel)
			}
		} else {
			for _, candidate := range []string{parsed.playermodel, parsed.viewmodel} {
				if candidate == "" {
					continue
				}
				anchors = append(anchors, candidate)
			}
		}
		if len(anchors) == 0 {
			continue
		}

		if existing, ok := byID[id]; ok {
			existing.entity.Anchors = appendUnique(existing.entity.Anchors, anchors...)
		} else {
			byID[id] = &acc{entity: entities.Entity{
				ID:        id,
				Tag:       item.Tag,
				Category:  item.Category,
				Group:     item.Group,
				Official:  item.Official,
				IsItem:    item.IsItem,
				Character: item.Character,
				Script:    path,
				Anchors:   appendUnique(nil, anchors...),
			}}
		}

		for key, model := range parsed.arms {
			if character := armsCharacters[strings.ToLower(key)]; character != "" && model != "" {
				armsByPath[entities.NormalizePath(model)] = character
			}
		}
	}

	// 幸存者手臂（v_arms_*.mdl）：过去这些路径"两头都不占"——既不在武器关键词表，也不在角色白名单。
	armsPaths := make([]string, 0, len(armsByPath))
	for path := range armsByPath {
		armsPaths = append(armsPaths, path)
	}
	sort.Strings(armsPaths)
	for _, path := range armsPaths {
		character := armsByPath[path]
		id := "arms." + strings.ToLower(character)
		if existing, ok := byID[id]; ok {
			existing.entity.Anchors = appendUnique(existing.entity.Anchors, path)
			continue
		}
		byID[id] = &acc{entity: entities.Entity{
			ID:        id,
			Character: character,
			Official:  true,
			Anchors:   []string{path},
			Note:      "幸存者第一人称手臂（来自武器脚本的 CharacterViewmodelAddon）",
		}}
	}

	table := &entities.Table{
		SchemaVersion: entities.SchemaVersion,
		GeneratedAt:   time.Now().Format(time.RFC3339),
		Source:        "scripts/weapon_*.txt + scripts/melee/*.txt（游戏本体脚本）",
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		entity := byID[id].entity
		sort.Strings(entity.Anchors)
		table.Entities = append(table.Entities, entity)
	}

	// 未在 scriptSpecs 里登记的脚本要显式暴露：游戏更新新增武器时，维护者需要补中文名。
	if len(unmapped) > 0 {
		sort.Strings(unmapped)
		table.Source += "；未登记脚本：" + strings.Join(unmapped, ", ")
	}
	prefixSummary, err := attachOwnerPrefixes(table, gameRoot)
	if err != nil {
		return nil, err
	}
	table.Source += prefixSummary
	return table, nil
}

// attachOwnerPrefixes 用游戏本体索引给实体补"独占资源前缀"（开发文档 §4.2 的槽位落地）。
//
// 做法：把本体目录与文件 stem 当作候选前缀，凡是**只被一个实体**的归属词命中的，
// 就记到该实体名下。于是：
//
//	sound/weapons/knife/**            → 匕首（词 knife）
//	materials/**/knife_t/**           → 匕首（锚点 w_knife_t / v_knife_t 的词）
//	materials/vgui/hud/icon_knife.*   → 匕首（文件 stem 的词）
//	sound/weapons/rifle/**            → 不归属（rifle 被 5 把步枪共用）
//
// 作者命名空间（materials/weapons/oe/knife_annihil/**）不在本体索引里，因此永远不会被
// 这条通道命中 —— 这正是"规则锚定本体真实路径"的做法，替代手写关键词。
// 本体索引不可用（游戏目录缺失）时只跳过并标注，不阻断生成。
func attachOwnerPrefixes(table *entities.Table, gameRoot string) (string, error) {
	index, err := gamedata.Build(gameRoot)
	if err != nil || index == nil || len(index.Paths) == 0 {
		return "；本体索引不可用，未产出资源槽位", nil
	}

	// 1) 归属词：只保留"全表唯一属于一个实体"的词，避免 rifle/shotgun/weapon 这类泛词。
	wordOwner := make(map[string]string)
	ambiguous := make(map[string]bool)
	for i := range table.Entities {
		entity := &table.Entities[i]
		if entity.Tag == "" {
			continue
		}
		for _, word := range ownerWords(entityOwnerText(*entity), true) {
			if ambiguous[word] {
				continue
			}
			if _, generic := genericOwnerWords[word]; generic {
				continue
			}
			if existing, ok := wordOwner[word]; ok {
				if existing == entity.ID {
					continue
				}
				delete(wordOwner, word)
				ambiguous[word] = true
				continue
			}
			wordOwner[word] = entity.ID
		}
	}
	if len(wordOwner) == 0 {
		return "；本体索引里没有可归属的实体词", nil
	}

	// 2) 认领前缀：一个前缀命中两个不同实体就不归属。
	owner := make(map[string]string)
	hits := make(map[string]int)
	matchOwner := func(text string) string {
		matched := ""
		for _, word := range ownerWords(text, false) {
			id, ok := wordOwner[word]
			if !ok {
				continue
			}
			if matched == "" {
				matched = id
				continue
			}
			if matched != id {
				return ""
			}
		}
		return matched
	}
	claim := func(prefix string) {
		prefix = strings.ToLower(strings.TrimSpace(prefix))
		if len(prefix) < 6 {
			return
		}
		// 前缀必须落在"武器资源根"或 UI 根里：`materials/models/infected/common/military_national_guard/`
		// （军狙的词 military）、`sound/npc/05_military/` 这类角色/场景资源不是武器证据。
		if !ownerPrefixRootAllowed(prefix) {
			return
		}
		matched := matchOwner(prefix)
		if matched == "" {
			return
		}
		// 往上收：只要祖先目录仍然唯一属于同一实体，就用更短的祖先前缀
		// （sound/weapons/magnum/gunfire/ → sound/weapons/magnum/）。
		best := prefix
		for {
			trimmed := strings.TrimSuffix(best, "/")
			slash := strings.LastIndex(trimmed, "/")
			if slash < 0 {
				break
			}
			parent := trimmed[:slash+1]
			if matchOwner(parent) != matched {
				break
			}
			best = parent
		}
		if existing, ok := owner[best]; ok && existing != matched {
			delete(owner, best)
			return
		}
		owner[best] = matched
		hits[best]++
	}
	for _, path := range index.Paths {
		if slash := strings.LastIndex(path, "/"); slash >= 0 {
			claim(path[:slash+1])
		}
		// 文件 stem 只信 UI/图标目录：`materials/vgui/hud/icon_knife.vmt` 这种"图标名里带武器名"
		// 是可靠归属；而 `sound/weapons/rifle/gunfire/rifle_fire_1_incendiary.wav`、
		// `models/infected/common_military_male01.vvd` 这类 stem 里的通用词会把整类 Mod
		// 误归到某个实体（真机实测：217 个包被一条 incendiary 音效 stem 标成「燃烧弹盒」）。
		if dot := strings.LastIndex(path, "."); dot > strings.LastIndex(path, "/") {
			stem := path[:dot]
			if strings.HasPrefix(stem, "materials/vgui/") {
				claim(stem)
			}
		}
	}

	// 3) 只保留最短的祖先前缀：更长的子前缀已经包含在它里面。
	prefixes := make([]string, 0, len(owner))
	for prefix := range owner {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	kept := make(map[string]struct{}, len(owner))
	for _, prefix := range prefixes {
		redundant := false
		for _, shorter := range prefixes {
			if shorter == prefix || !strings.HasPrefix(prefix, shorter) {
				continue
			}
			if _, ok := kept[shorter]; ok && owner[shorter] == owner[prefix] {
				redundant = true
				break
			}
		}
		if !redundant {
			kept[prefix] = struct{}{}
		}
	}

	byID := make(map[string]int, len(table.Entities))
	for i := range table.Entities {
		byID[table.Entities[i].ID] = i
	}
	total, entitiesTouched := 0, make(map[string]struct{})
	zeroHit := make([]string, 0)
	for prefix := range kept {
		indexOf, ok := byID[owner[prefix]]
		if !ok {
			continue
		}
		count := hits[prefix]
		if count <= 0 {
			// 显式槽位表要"0 命中 = 大声失败"：命中数直接从本体索引数出来，
			// 出现 0 说明索引里有路径被删/改，表已经不可信。
			zeroHit = append(zeroHit, prefix)
			continue
		}
		table.Entities[indexOf].Slots = append(table.Entities[indexOf].Slots,
			entities.Slot{Kind: slotKindForPrefix(prefix), Prefix: prefix, Hits: count})
		total++
		entitiesTouched[owner[prefix]] = struct{}{}
	}
	if len(zeroHit) > 0 {
		sort.Strings(zeroHit)
		return "", fmt.Errorf("有 %d 条槽位在本体索引里 0 命中（游戏更新后请重跑生成器）：%s",
			len(zeroHit), strings.Join(zeroHit, ", "))
	}
	for i := range table.Entities {
		sort.Slice(table.Entities[i].Slots, func(a, b int) bool {
			return table.Entities[i].Slots[a].Prefix < table.Entities[i].Slots[b].Prefix
		})
	}
	return fmt.Sprintf("；资源槽位 %d 条（覆盖 %d 个实体）", total, len(entitiesTouched)), nil
}

// slotKindForPrefix 按本体根目录给槽位分类（开发文档 §4.2 的 kind 词表）。
func slotKindForPrefix(prefix string) string {
	switch {
	case strings.HasPrefix(prefix, "sound/"):
		return "sound"
	case strings.HasPrefix(prefix, "scripts/"):
		return "script"
	case strings.HasPrefix(prefix, "models/"):
		return "model"
	case strings.HasPrefix(prefix, "materials/vgui/"):
		return "ui"
	case strings.HasPrefix(prefix, "materials/"):
		return "material"
	default:
		return "other"
	}
}

// entityOwnerText 把实体的溯源信息拼成一段文本，用于拆归属词。
func entityOwnerText(entity entities.Entity) string {
	parts := make([]string, 0, len(entity.Anchors)+2)
	parts = append(parts, entity.Script, entity.ID)
	parts = append(parts, entity.Anchors...)
	return strings.Join(parts, " ")
}

// ownerPrefixRootAllowed 限制"独占资源前缀"只能落在武器资源根或 UI 根里。
// 与 parser 的 weaponAssetRoots 同口径（这里不 import parser，避免 gamedata ↔ parser 反向依赖）。
var ownerPrefixRoots = []string{
	"models/weapons/",
	"models/v_models/weapons/",
	"models/w_models/weapons/",
	"materials/models/weapons/",
	"materials/models/v_models/weapons/",
	"materials/weapons/",
	"scripts/weapons/",
	"scripts/melee/",
	"sound/weapons/",
	"materials/vgui/",
}

func ownerPrefixRootAllowed(prefix string) bool {
	for _, root := range ownerPrefixRoots {
		if strings.HasPrefix(prefix, root) {
			return true
		}
	}
	return false
}

// genericOwnerWords 是与"武器种类"绑定的泛词，不能作为归属词。
// 真机教训（D1）：`sound/weapons/shotgun/**` 是泵动 / Chrome / 连喷共用的音效目录，
// 木喷的世界模型叫 `w_shotgun.mdl`，于是 "shotgun" 成了木喷的独占词 —— 结果
// Chrome 铁喷包（`sound/weapons/shotgun/gunother/shotgun_pump_1.wav`）被标成木喷。
// 这些词在 parser 的 token 通道里同样是黑名单（genericStrippedTokens）。
var genericOwnerWords = map[string]struct{}{
	"shotgun": {}, "pumpshotgun": {}, "rifle": {}, "smg": {}, "pistol": {},
	"sniper": {}, "guns": {}, "weapon": {}, "weapons": {}, "melee": {},
}

// ownerWords 把文本拆成"词"：非字母数字都当分隔符，只保留长度 ≥3 的小写词。
// tailOnly 只保留每段路径 token 的**最后一个词**，用于实体归属词：脚本名
// `weapon_pain_pills` 只取 `pills`（`pain` 这种中间词在资源路径里太泛，真机实测把
// sound/npc/witch/voice/pain/** 误归成「止痛药」）。
func ownerWords(text string, tailOnly bool) []string {
	fields := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
	})
	// 先按 `/` 之外的已切分 token 序列处理：tailOnly 时每个 token 取最后一个词。
	tokens := strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return r == '/' || r == '\\' || r == '.' || r == ' ' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	add := func(field string) {
		if len(field) < 3 {
			return
		}
		if _, dup := seen[field]; dup {
			return
		}
		seen[field] = struct{}{}
		out = append(out, field)
	}
	if tailOnly {
		for _, token := range tokens {
			words := strings.FieldsFunc(token, func(r rune) bool {
				return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'))
			})
			if len(words) == 0 {
				continue
			}
			add(words[len(words)-1])
		}
		return out
	}
	for _, field := range fields {
		add(field)
	}
	return out
}

func entityIDFor(scriptPath string) string {
	base := strings.TrimSuffix(strings.TrimPrefix(scriptPath, "scripts/"), ".txt")
	base = strings.ReplaceAll(base, "/", ".")
	return strings.ToLower(base)
}

func appendUnique(target []string, values ...string) []string {
	seen := make(map[string]struct{}, len(target))
	for _, item := range target {
		seen[item] = struct{}{}
	}
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		target = append(target, value)
	}
	return target
}

type parsedScript struct {
	printName   string
	playermodel string
	viewmodel   string
	arms        map[string]string
}

// parseScript 用模块里现成的 Valve KeyValues 解析器读取脚本（不自己写临时解析）。
func parseScript(data []byte) (parsedScript, error) {
	root, err := vdf.ReadTolerant(bytes.NewReader(data))
	if err != nil {
		return parsedScript{}, err
	}
	result := parsedScript{arms: map[string]string{}}
	if key := root.FindKey("printname"); key != nil {
		result.printName = key.Value
	}
	if key := root.FindKey("playermodel"); key != nil {
		result.playermodel = entities.NormalizePath(key.Value)
	}
	if key := root.FindKey("viewmodel"); key != nil {
		result.viewmodel = entities.NormalizePath(key.Value)
	}
	if block := root.FindKey("CharacterViewmodelAddon"); block != nil {
		for child := block.FirstSubKey(); child != nil; child = child.NextSubKey() {
			result.arms[child.Key] = entities.NormalizePath(child.Value)
		}
	}
	return result, nil
}
