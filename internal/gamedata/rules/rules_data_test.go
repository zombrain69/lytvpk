package rules

import "testing"

// 规则表迁移收尾（W2）之后，`internal/ruletable/rules.json` 是唯一事实源：
// 不再有"Go 表 ↔ JSON 逐条比对"的对偶关系，改为**关键条目 + 规模下限**的数据完整性检查。
//
// 真正保证"没有少标"的是真机护栏：`--check-tag-regression` 拿旧基线逐 Mod diff，
// 任何标签消失都必须显式登记（见 tools/tag-regression-allowlist.json）。
func TestRuleTableHasEssentialEntries(t *testing.T) {
	table := MustLoad()

	contentTags := make(map[string]bool, len(table.ContentRules))
	for _, rule := range table.ContentRules {
		contentTags[rule.Tag] = true
	}
	for _, want := range []string{
		"医疗包", "电击器", "止痛药", "肾上腺", "土制炸弹", "燃烧瓶", "胆汁",
		"汽油桶", "煤气罐", "氧气罐", "烟花盒", "一代子弹堆", "二代子弹堆",
		"燃烧弹盒", "高爆弹盒", "激光瞄准盒", "侏儒", "载入画面", "天空", "主菜单",
	} {
		if !contentTags[want] {
			t.Fatalf("内容规则缺少关键标签 %q", want)
		}
	}

	weaponTags := make(map[string]bool, len(table.WeaponPathRules))
	for _, rule := range table.WeaponPathRules {
		weaponTags[rule.Tag] = true
	}
	for _, want := range []string{
		"AK47", "M16", "sg552", "M60", "大狙", "猎枪", "军狙", "鸟狙",
		"木喷", "一代连喷", "铁喷", "二代连喷", "乌兹", "消音", "MP5",
		"榴弹发射器", "固定机关枪", "棒球棍", "板球拍", "吉他", "平底锅",
		"高尔夫球杆", "消防斧", "砍刀", "武士刀", "电锯", "撬棍", "草叉",
		"铁铲", "警棍", "匕首",
	} {
		if !weaponTags[want] {
			t.Fatalf("武器路径规则缺少关键标签 %q", want)
		}
	}

	survivors := make(map[string]bool, len(table.CharacterRules.Survivors))
	for _, rule := range table.CharacterRules.Survivors {
		survivors[rule.Tag] = true
	}
	for _, want := range []string{"Bill", "Francis", "Louis", "Zoey", "Coach", "Ellis", "Nick", "Rochelle"} {
		if !survivors[want] {
			t.Fatalf("角色表缺少幸存者 %q", want)
		}
	}
	infected := make(map[string]bool, len(table.CharacterRules.SpecialInfected))
	for _, rule := range table.CharacterRules.SpecialInfected {
		infected[rule.Tag] = true
	}
	for _, want := range []string{"boomer", "charger", "hunter", "jockey", "smoker", "spitter", "tank", "witch"} {
		if !infected[want] {
			t.Fatalf("角色表缺少特感 %q", want)
		}
	}

	fileKinds := make(map[string]bool, len(table.FileKinds))
	for _, kind := range table.FileKinds {
		fileKinds[kind.Tag] = true
	}
	for _, want := range []string{"UI", "声音", "脚本", "VScript", "模型", "贴图", "粒子特效"} {
		if !fileKinds[want] {
			t.Fatalf("文件类别缺少 %q", want)
		}
	}

	// 规模下限：防止误删大段规则（真正的"没少标"由真机护栏兜底）。
	if len(table.ContentRules) < 40 || len(table.WeaponPathRules) < 80 ||
		len(table.WeaponMetadataRules) < 40 || len(table.FileKinds) < 7 {
		t.Fatalf("规则表规模异常：content=%d weaponPath=%d weaponMetadata=%d fileKinds=%d",
			len(table.ContentRules), len(table.WeaponPathRules),
			len(table.WeaponMetadataRules), len(table.FileKinds))
	}
	if len(table.CharacterRules.VoiceDirSurvivor) < 8 || len(table.CharacterRules.VoiceDirInfected) < 7 {
		t.Fatalf("语音目录表规模异常：survivor=%d infected=%d",
			len(table.CharacterRules.VoiceDirSurvivor), len(table.CharacterRules.VoiceDirInfected))
	}
}
