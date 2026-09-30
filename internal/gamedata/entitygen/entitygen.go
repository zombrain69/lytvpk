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
	return table, nil
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
