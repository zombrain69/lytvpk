package grouping

import (
	"strings"
	"testing"
)

func containsKey(keys []string, wanted string) bool {
	for _, key := range keys {
		if key == wanted {
			return true
		}
	}
	return false
}

func hasSignal(signals []string, wanted string) bool {
	for _, signal := range signals {
		if signal == wanted {
			return true
		}
	}
	return false
}

// 套件命名空间信号：同一个 models/<作者>/<套件> 下的多个模块属于同一套件，
// 应该给出一个 all 组，并把"只替换官方目标的本体"一并纳入。
// 现实例子：l4n 的 `913limod/airi_evilfall`（24 个配件 + 8 个幸存者本体）、
// 武器侧的 `codm/ice`（贴图包 + 各枪参数包）。

func suiteTestMods() []Mod {
	return []Mod{
		{
			Key: "airi初代战斗员臂甲.vpk", Name: "airi初代战斗员臂甲.vpk",
			ResourceRoots: []string{"913limod/airi_evilfall"},
			SecondaryTags: []string{"贴图"},
		},
		{
			Key: "airi初代战斗员大衣.vpk", Name: "airi初代战斗员大衣.vpk",
			ResourceRoots: []string{"913limod/airi_evilfall"},
			SecondaryTags: []string{"贴图"},
		},
		{
			Key: "airi初代恶堕战斗员基础材质.vpk", Name: "airi初代恶堕战斗员基础材质.vpk",
			ResourceRoots: []string{"913limod/airi_evilfall"},
			SecondaryTags: []string{"贴图"},
		},
		// 本体：只替换官方目标，没有套件命名空间，但文件名共享 `airi初代` 前缀。
		{
			Key: "airi初代恶堕战斗员coach.vpk", Name: "airi初代恶堕战斗员coach.vpk",
			Subject: "Coach 模型",
		},
		{
			Key: "airi初代恶堕战斗员nick.vpk", Name: "airi初代恶堕战斗员nick.vpk",
			Subject: "Nick 模型",
		},
		// 无关 Mod：作者名里有 airi，但不是这个套件。
		{
			Key: "[airi]woolywinter_francis.vpk", Name: "[airi]woolwinter_francis.vpk",
			Subject: "Francis 模型",
		},
	}
}

func TestSuiteNamespaceProducesAllGroupAndAttachesBodies(t *testing.T) {
	suggestions, _ := Suggest(suiteTestMods(), Options{})
	var found *Suggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "913limod/airi_evilfall") {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("应产出套件组: %#v", suggestions)
	}
	if found.Strategy != "all" {
		t.Fatalf("套件组必须是 all（一起启用）: %#v", found)
	}
	if len(found.MemberKeys) != 5 {
		t.Fatalf("套件组应包含 3 个配件 + 2 个本体: %#v", found.MemberKeys)
	}
	for _, key := range []string{
		"airi初代战斗员臂甲.vpk", "airi初代战斗员大衣.vpk", "airi初代恶堕战斗员基础材质.vpk",
		"airi初代恶堕战斗员coach.vpk", "airi初代恶堕战斗员nick.vpk",
	} {
		if !containsKey(found.MemberKeys, key) {
			t.Fatalf("套件组缺少 %q: %#v", key, found.MemberKeys)
		}
	}
	// 只是作者名相同、套件不同的 Mod 不能被拉进来。
	if containsKey(found.MemberKeys, "[airi]woolywinter_francis.vpk") {
		t.Fatalf("不同套件的 Mod 不应被纳入: %#v", found.MemberKeys)
	}
	if !hasSignal(found.Signals, suiteSignalLabel) {
		t.Fatalf("应带套件信号: %#v", found.Signals)
	}
}

func TestSuiteNamespaceRequiresAtLeastThreeModules(t *testing.T) {
	mods := []Mod{
		{Key: "a.vpk", Name: "a.vpk", ResourceRoots: []string{"codm/ice"}},
		{Key: "b.vpk", Name: "b.vpk", ResourceRoots: []string{"codm/ice"}},
	}
	suggestions, _ := Suggest(mods, Options{})
	for _, item := range suggestions {
		if strings.Contains(item.Reason, "codm/ice") {
			t.Fatalf("只有 2 个模块时不应产出套件组: %#v", item)
		}
	}
}

func TestSuiteNamespaceIgnoresOfficialRootsOnlyMods(t *testing.T) {
	mods := []Mod{
		{Key: "a.vpk", Name: "a.vpk", Subject: "AK47 武器"},
		{Key: "b.vpk", Name: "b.vpk", Subject: "AK47 武器"},
		{Key: "c.vpk", Name: "c.vpk", Subject: "AK47 武器"},
	}
	suggestions, _ := Suggest(mods, Options{})
	for _, item := range suggestions {
		if strings.Contains(item.Reason, "套件") {
			t.Fatalf("没有套件命名空间时不应产出套件组: %#v", item)
		}
	}
}

func TestSuiteNamespaceHandlesWeaponTextureAndModelSplit(t *testing.T) {
	// 武器侧的真实形态：一个贴图包 + 多个"参数包/模型包"，共享 models/codm/ice。
	mods := []Mod{
		{Key: "codm冰霜巨龙贴图包.vpk", Name: "codm冰霜巨龙贴图包.vpk", ResourceRoots: []string{"codm/ice"}, Subject: "泛材质资源"},
		{Key: "ak47冰龙参数包蓝.vpk", Name: "ak47冰龙参数包蓝.vpk", ResourceRoots: []string{"codm/ice"}, Subject: "AK47 武器"},
		{Key: "m16冰龙参数包紫.vpk", Name: "m16冰龙参数包紫.vpk", ResourceRoots: []string{"codm/ice"}, Subject: "M16 武器"},
	}
	suggestions, _ := Suggest(mods, Options{})
	var found *Suggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "codm/ice") {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("武器贴图包 + 参数包 应被识别为同一套件: %#v", suggestions)
	}
	if found.Strategy != "all" || len(found.MemberKeys) != 3 {
		t.Fatalf("武器套件组应为 all 且含 3 个成员: %#v", found)
	}
}

func TestSuiteNamespaceRejectsGenericAndNumericSuiteNames(t *testing.T) {
	mods := []Mod{
		// models/<作者>/weapons 这种"通用目录名"不是套件名
		{Key: "a.vpk", Name: "a.vpk", ResourceRoots: []string{"someauthor/weapons"}},
		{Key: "b.vpk", Name: "b.vpk", ResourceRoots: []string{"someauthor/weapons"}},
		{Key: "c.vpk", Name: "c.vpk", ResourceRoots: []string{"someauthor/weapons"}},
		// 纯数字套件段（工坊 ID 之类的路径残留）同样不算
		{Key: "d.vpk", Name: "d.vpk", ResourceRoots: []string{"someauthor/12345"}},
		{Key: "e.vpk", Name: "e.vpk", ResourceRoots: []string{"someauthor/12345"}},
		{Key: "f.vpk", Name: "f.vpk", ResourceRoots: []string{"someauthor/12345"}},
	}
	suggestions, stats := Suggest(mods, Options{})
	for _, item := range suggestions {
		if strings.Contains(item.Reason, "套装") || strings.Contains(item.Reason, "someauthor/") {
			t.Fatalf("通用目录名 / 纯数字套件段不应产出套件组: %#v", item)
		}
	}
	if stats.ByProvider["suite-namespace"] != 0 {
		t.Fatalf("suite-namespace 不应产出候选: %#v", stats.ByProvider)
	}
}

func TestSuiteNamespaceIgnoresNumericOnlyNamePrefix(t *testing.T) {
	// 工坊 ID 文件名共享的"数字前缀"不能拿来当组名，应回退到套件段。
	mods := []Mod{
		{Key: "3788635187.vpk", Name: "3788635187.vpk", ResourceRoots: []string{"author/suiteseven"}},
		{Key: "3788639080.vpk", Name: "3788639080.vpk", ResourceRoots: []string{"author/suiteseven"}},
		{Key: "3788734957.vpk", Name: "3788734957.vpk", ResourceRoots: []string{"author/suiteseven"}},
	}
	suggestions, _ := Suggest(mods, Options{})
	var found *Suggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "author/suiteseven") {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("应产出套件组: %#v", suggestions)
	}
	if strings.HasPrefix(found.Label, "3788") {
		t.Fatalf("纯数字文件名前缀不能当组名: %#v", found.Label)
	}
	if found.Label != "suiteseven 套装" {
		t.Fatalf("应回退到套件段命名: %#v", found.Label)
	}
}

func TestSuiteNamespaceAttachesBodiesBySuiteKeyword(t *testing.T) {
	// 真实形态（shinano）：配件叫 `neko.vpk` / `油光渲染.vpk`（文件名里没有套件名），
	// 本体叫 `shinano维纳斯bill.vpk`（文件名以套件名 shinano 开头，但只替换官方目标）。
	mods := []Mod{
		{Key: "neko.vpk", Name: "neko.vpk", ResourceRoots: []string{"limod/shinano"}},
		{Key: "油光渲染.vpk", Name: "油光渲染.vpk", ResourceRoots: []string{"limod/shinano"}},
		{Key: "neko-低透明.vpk", Name: "neko-低透明.vpk", ResourceRoots: []string{"limod/shinano"}},
		{Key: "shinano维纳斯bill.vpk", Name: "shinano维纳斯bill.vpk", Subject: "Bill 模型"},
		{Key: "shinano维纳斯nick.vpk", Name: "shinano维纳斯nick.vpk", Subject: "Nick 模型"},
		// 不相关：名字里没有套件关键词
		{Key: "死库水透明.vpk", Name: "死库水透明.vpk", Subject: "泛材质资源"},
	}
	suggestions, _ := Suggest(mods, Options{})
	var found *Suggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "limod/shinano") {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("应产出 shinano 套件组: %#v", suggestions)
	}
	if len(found.MemberKeys) != 5 {
		t.Fatalf("套件组应含 3 个配件 + 2 个本体: %#v", found.MemberKeys)
	}
	if !containsKey(found.MemberKeys, "shinano维纳斯bill.vpk") ||
		!containsKey(found.MemberKeys, "shinano维纳斯nick.vpk") {
		t.Fatalf("本体应按套件关键词附着: %#v", found.MemberKeys)
	}
	if containsKey(found.MemberKeys, "死库水透明.vpk") {
		t.Fatalf("无关 Mod 不应被附着: %#v", found.MemberKeys)
	}
}

func TestSuiteNamespaceFromMaterialsBranch(t *testing.T) {
	// 真实形态（死库水）：内部路径不统一，且都在 materials 下、没有 models 段：
	//   materials/sikushui/mo/white_2.vtf
	//   materials/qkl/mo/sikushui/waitao.vtf
	// 解析器会把它们都归一成候选片段 `sikushui`（见 parser 侧测试），引擎据此成包，
	// 再用文件名前缀 `死库水` 把本体（替换官方目标）并进来。
	mods := []Mod{
		{Key: "死库水校服基础材质.vpk", Name: "死库水校服基础材质.vpk", ResourceRoots: []string{"sikushui"}},
		{Key: "死库水校服外套透明.vpk", Name: "死库水校服外套透明.vpk", ResourceRoots: []string{"sikushui"}},
		{Key: "死库水透明.vpk", Name: "死库水透明.vpk", ResourceRoots: []string{"sikushui"}},
		{Key: "死库水校服-教练.vpk", Name: "死库水校服-教练.vpk", Subject: "Coach 模型"},
	}
	suggestions, _ := Suggest(mods, Options{})
	var found *Suggestion
	for index := range suggestions {
		if strings.Contains(suggestions[index].Reason, "sikushui") {
			found = &suggestions[index]
			break
		}
	}
	if found == nil {
		t.Fatalf("materials 分支的套件应能成组: %#v", suggestions)
	}
	if found.Strategy != "all" || len(found.MemberKeys) != 4 {
		t.Fatalf("死库水套件应为 all 且含 3 个材质 + 1 个本体: %#v", found)
	}
	if !containsKey(found.MemberKeys, "死库水校服-教练.vpk") {
		t.Fatalf("本体应按文件名前缀并入: %#v", found.MemberKeys)
	}
	if found.Label != "死库水 套装" {
		t.Fatalf("组名应来自文件名公共前缀: %#v", found.Label)
	}
}

func TestSuiteNamespaceSingleSegmentRequiresNamePrefix(t *testing.T) {
	// 单段候选（materials 分支）必须同时有文件名公共前缀，否则就是零散目录拼凑：
	// 真实误报：`codm-m13神话-黑耀星辰贴图包` / `codm冰霜巨龙贴图包` / `mg42永别（原版速度）`
	// 共享某个 materials 片段，但文件名毫无共同前缀。
	mods := []Mod{
		{Key: "codm-m13神话-黑耀星辰贴图包.vpk", Name: "codm-m13神话-黑耀星辰贴图包.vpk", ResourceRoots: []string{"xx_p_codm"}},
		{Key: "codm冰霜巨龙贴图包.vpk", Name: "codm冰霜巨龙贴图包.vpk", ResourceRoots: []string{"xx_p_codm"}},
		{Key: "mg42永别（原版速度）.vpk", Name: "mg42永别（原版速度）.vpk", ResourceRoots: []string{"xx_p_codm"}},
	}
	suggestions, stats := Suggest(mods, Options{})
	for _, item := range suggestions {
		if strings.Contains(item.Reason, "xx_p_codm") {
			t.Fatalf("没有文件名公共前缀的单段候选不应成组: %#v", item)
		}
	}
	if stats.ByProvider["suite-namespace"] != 0 {
		t.Fatalf("suite-namespace 不应产出候选: %#v", stats.ByProvider)
	}
}
