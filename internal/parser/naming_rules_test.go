// 命名规范化（路线图第 20 项）的纯函数测试：S1–S5 判定、上位名与待复核、
// 幂等（前缀匹配式剥旧前缀）、63B 预算（含 [标签]/隐藏前缀）、证据收集与文件级入口。
package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

func namingEvidence(total int, hasModel bool, rows map[string]map[string]int) NamingEvidence {
	evidence := NamingEvidence{Total: total, HasModelFile: hasModel, DomainRows: map[string]map[string]int{}}
	for domain, domainRows := range rows {
		for row, count := range domainRows {
			for i := 0; i < count; i++ {
				evidence.addRow(domain, row)
			}
		}
	}
	return evidence
}

func TestDeriveSuggestionMapStage(t *testing.T) {
	bsp := DeriveNameSuggestion("Unknown Pack", NamingEvidence{Total: 10, HasMapBSP: true}, 0)
	if bsp.Stem != "地图-Unknown Pack" || bsp.Prefix != "地图" || bsp.Stage != "S1" {
		t.Fatalf("bsp 包建议不对: %+v", bsp)
	}
	nav := DeriveNameSuggestion("Nav Only", NamingEvidence{Total: 10, HasMapNav: true}, 0)
	if nav.Stem != "地图-Nav Only" || nav.Stage != "S1" {
		t.Fatalf("nav 包建议不对: %+v", nav)
	}
}

func TestDeriveSuggestionSurvivorS2(t *testing.T) {
	evidence := namingEvidence(40, true, map[string]map[string]int{"生还者": {"Coach": 6}})
	suggestion := DeriveNameSuggestion("Airi", evidence, 0)
	if suggestion.Stem != "生还者-Coach-Airi" || suggestion.Stage != "S2" || suggestion.PendingReview {
		t.Fatalf("S2 建议不对: %+v", suggestion)
	}
	if !suggestion.Changed {
		t.Fatalf("应标记为有改动: %+v", suggestion)
	}
}

func TestDeriveSuggestionS3TextureOnlyPending(t *testing.T) {
	evidence := namingEvidence(30, false, map[string]map[string]int{"场景道具": {"梯子": 5}})
	suggestion := DeriveNameSuggestion("全部梯子-HD", evidence, 0)
	if suggestion.Stem != "场景道具-梯子-全部梯子-HD" || suggestion.Stage != "S3" || !suggestion.PendingReview {
		t.Fatalf("S3 建议不对: %+v", suggestion)
	}
}

func TestDeriveSuggestionSparseKeepsOriginal(t *testing.T) {
	evidence := namingEvidence(600, true, map[string]map[string]int{"生还者": {"Coach": 12}})
	suggestion := DeriveNameSuggestion("glubtastic5", evidence, 0)
	if suggestion.Stem != "glubtastic5" || suggestion.Changed || !suggestion.PendingReview {
		t.Fatalf("大而散应保留原名: %+v", suggestion)
	}
	if !strings.Contains(suggestion.Reason, "大而散") {
		t.Fatalf("原因应说明大而散: %q", suggestion.Reason)
	}
}

func TestDeriveSuggestionUnclassifiedKeepsOriginal(t *testing.T) {
	evidence := NamingEvidence{Total: 20, Unclassified: 11}
	suggestion := DeriveNameSuggestion("V社启动动画", evidence, 0)
	if suggestion.Stem != "V社启动动画" || suggestion.Changed || !suggestion.PendingReview {
		t.Fatalf("未归类过半应保留原名: %+v", suggestion)
	}
}

func TestDeriveSuggestionSpecialists(t *testing.T) {
	flashlight := NamingEvidence{Total: 100, FlashlightHits: 30}
	suggestion := DeriveNameSuggestion("Wide Beam", flashlight, 0)
	if suggestion.Stem != "手电筒-Wide Beam" || suggestion.Stage != "专项" {
		t.Fatalf("手电筒专项建议不对: %+v", suggestion)
	}

	belowThreshold := NamingEvidence{Total: 100, FlashlightHits: 20}
	kept := DeriveNameSuggestion("Wide Beam", belowThreshold, 0)
	if kept.Stem != "Wide Beam" || kept.Stage != "S5" {
		t.Fatalf("占比不达标不应给专项前缀: %+v", kept)
	}

	adminWins := NamingEvidence{Total: 100, AdminPluginHits: 30, FlashlightHits: 60}
	admin := DeriveNameSuggestion("Admin System", adminWins, 0)
	if admin.Stem != "管理员插件-Admin System" {
		t.Fatalf("管理员插件应优先于手电筒: %+v", admin)
	}
}

func TestDeriveSuggestionResourceTypes(t *testing.T) {
	interfaceType := NamingEvidence{Total: 20, TypeHits: map[string]int{"界面": 9, "材质": 4}}
	suggestion := DeriveNameSuggestion("HUD Cleanup", interfaceType, 0)
	if suggestion.Stem != "界面-HUD Cleanup" || suggestion.Stage != "S4" {
		t.Fatalf("S4 界面建议不对: %+v", suggestion)
	}

	modelHeavy := NamingEvidence{Total: 100, HasModelFile: true, TypeHits: map[string]int{"模型": 25, "材质": 40}}
	model := DeriveNameSuggestion("Pack", modelHeavy, 0)
	if model.Stem != "模型-Pack" || !strings.Contains(model.Reason, "占比达标") {
		t.Fatalf("模型占比达标应给模型前缀: %+v", model)
	}

	modelFallback := NamingEvidence{Total: 100, TypeHits: map[string]int{"模型": 7}}
	fallback := DeriveNameSuggestion("Pack", modelFallback, 0)
	if fallback.Stem != "模型-Pack" {
		t.Fatalf("无其它类型时应兜底到模型: %+v", fallback)
	}
}

func TestDeriveSuggestionWeaponSuperNames(t *testing.T) {
	pistols := namingEvidence(30, true, map[string]map[string]int{"副武器": {"小手枪": 3, "马格南": 2}})
	suggestion := DeriveNameSuggestion("John Wick", pistols, 0)
	if suggestion.Subject != "手枪" || suggestion.PendingReview {
		t.Fatalf("双持手枪应合并为上位名且不待复核: %+v", suggestion)
	}

	shotguns := namingEvidence(30, true, map[string]map[string]int{"主武器": {"木喷": 2, "铁喷": 3}})
	shotgun := DeriveNameSuggestion("Chrome Pack", shotguns, 0)
	if shotgun.Subject != "霰弹枪" || shotgun.PendingReview {
		t.Fatalf("多把霰弹枪应合并为上位名: %+v", shotgun)
	}

	multiRow := namingEvidence(30, true, map[string]map[string]int{"主武器": {"AK47": 5, "M16": 2}})
	review := DeriveNameSuggestion("Gun Pack", multiRow, 0)
	if review.Subject != "AK47" || !review.PendingReview {
		t.Fatalf("同域多行应取占比最高并待复核: %+v", review)
	}
}

func TestComposeIdempotentKeepsNormalized(t *testing.T) {
	evidence := NamingEvidence{Total: 10, HasMapBSP: true}
	kept := DeriveNameSuggestion("地图-Cold Front (Part 1)", evidence, 0)
	if kept.Stem != "地图-Cold Front (Part 1)" || kept.Changed {
		t.Fatalf("已规范名应原样保留: %+v", kept)
	}
	if !strings.Contains(kept.Reason, "已符合规范") {
		t.Fatalf("原因应说明已符合规范: %q", kept.Reason)
	}

	survivor := namingEvidence(40, true, map[string]map[string]int{"生还者": {"Coach": 6}})
	again := DeriveNameSuggestion("生还者-Coach-Airi", survivor, 0)
	if again.Stem != "生还者-Coach-Airi" || again.Changed {
		t.Fatalf("已规范名应幂等: %+v", again)
	}
}

func TestComposeStripsOldPrefixAndSubject(t *testing.T) {
	interfaceType := NamingEvidence{Total: 20, TypeHits: map[string]int{"界面": 9}}
	suggestion := DeriveNameSuggestion("材质-ESC菜单 Kokomi自用版", interfaceType, 0)
	if suggestion.Stem != "界面-ESC菜单 Kokomi自用版" {
		t.Fatalf("剥旧前缀应只换分类段: %+v", suggestion)
	}

	// 参考实现里的真实案例：特感-Boomer-Snorlax 命中「爆炸残肢」更多，但正确中段是 Boomer。
	hunter := namingEvidence(30, true, map[string]map[string]int{"特感": {"Hunter": 4}})
	swapped := DeriveNameSuggestion("特感-Boomer-Snorlax (Boomer)", hunter, 0)
	if swapped.Stem != "特感-Hunter-Snorlax (Boomer)" {
		t.Fatalf("换替换对象应剥掉旧段且保住标题: %+v", swapped)
	}
}

func TestComposeKeepsTitleDashesWhenFresh(t *testing.T) {
	evidence := namingEvidence(10, true, map[string]map[string]int{"生还者": {"Ellis": 2}})
	suggestion := DeriveNameSuggestion("Airi-第2版", evidence, 0)
	if suggestion.Stem != "生还者-Ellis-Airi-第2版" {
		t.Fatalf("原标题里的短横不能被吃掉: %+v", suggestion)
	}
}

func TestComposeKeepsTitleDashAfterPrefixStrip(t *testing.T) {
	effect := NamingEvidence{Total: 20, TypeHits: map[string]int{"特效": 9}}
	suggestion := DeriveNameSuggestion("界面-大厅背景-碧蓝档案", effect, 0)
	if suggestion.Stem != "特效-大厅背景-碧蓝档案" {
		t.Fatalf("换分类时标题短横要保住: %+v", suggestion)
	}
}

func TestSuggestionRespectsTagAndHiddenBudget(t *testing.T) {
	evidence := namingEvidence(10, true, map[string]map[string]int{"生还者": {"Ellis": 2}})
	longTitle := strings.Repeat("很长的标题", 12) // 60 字 ≈ 120 GBK 字节
	suggestion := DeriveNameSuggestion(longTitle, evidence, 0)
	if !suggestion.Truncated || !suggestion.Fits {
		t.Fatalf("超长标题应截断且可放置: %+v", suggestion)
	}
	if ANSIByteLength(suggestion.Stem) > AddonStemMaxANSIBytes {
		t.Fatalf("建议主干超过 63B: %d", ANSIByteLength(suggestion.Stem))
	}
	if !strings.HasPrefix(suggestion.Stem, "生还者-Ellis-") {
		t.Fatalf("前缀段不应被截断: %q", suggestion.Stem)
	}

	// 外层 [标签] 占 20 字节 → 只剩 43 字节预算。
	tagged := DeriveNameSuggestion(longTitle, evidence, 20)
	if ANSIByteLength(tagged.Stem) > AddonStemMaxANSIBytes-20 {
		t.Fatalf("含 [标签] 预算时主干超限: %d", ANSIByteLength(tagged.Stem))
	}

	// 外层修饰吃满预算 → 前缀段放不下，报告 fits=false。
	squeezed := DeriveNameSuggestion("X", evidence, AddonStemMaxANSIBytes)
	if squeezed.Fits {
		t.Fatalf("预算不足时应报 fits=false: %+v", squeezed)
	}
}

func TestComposeNormalizesIllegalFilenameChars(t *testing.T) {
	evidence := namingEvidence(10, true, map[string]map[string]int{"生还者": {"Ellis": 2}})
	suggestion := DeriveNameSuggestion(`Airi: v2? <best>|draft.`, evidence, 0)
	if suggestion.Stem != "生还者-Ellis-Airi： v2？ ＜best＞｜draft" {
		t.Fatalf("非法字符应全角化、尾句点应清掉: %q", suggestion.Stem)
	}

	// 保名分支同样过清洗（幂等输入里混入非法字符时仍给出可保存的建议）。
	mapEvidence := NamingEvidence{Total: 5, HasMapBSP: true}
	kept := DeriveNameSuggestion("地图-x?y", mapEvidence, 0)
	if kept.Stem != "地图-x？y" || !kept.Changed {
		t.Fatalf("保名分支应清洗非法字符: %+v", kept)
	}
}

func TestCollectNamingEvidenceCountsRowsAndTypes(t *testing.T) {
	archive := testArchiveFiles(
		vpkTestFile("models/survivors/survivor_coach/coach.mdl"),
		vpkTestFile("sound/player/survivor/voice/coach/line01.wav"),
		vpkTestFile("materials/models/survivors/survivor_coach/skin.vtf"),
		vpkTestFile("materials/effects/flashlight/beam.vtf"),
		vpkTestFile("media/intro.bik"),
		vpkTestFile("addoninfo.txt"),
	)
	evidence := CollectNamingEvidence(archive)

	if evidence.Total != 5 {
		t.Fatalf("Total 应剔除 addoninfo: %d", evidence.Total)
	}
	if !evidence.HasModelFile {
		t.Fatalf("应识别出模型文件")
	}
	if evidence.domainHits("生还者") < 3 {
		t.Fatalf("Coach 命中应 ≥3: %+v", evidence.DomainRows)
	}
	if evidence.FlashlightHits != 1 {
		t.Fatalf("手电筒本体应命中 1: %d", evidence.FlashlightHits)
	}
	if evidence.TypeHits["模型"] == 0 || evidence.TypeHits["音效"] == 0 || evidence.TypeHits["材质"] == 0 {
		t.Fatalf("资源类型计数缺失: %+v", evidence.TypeHits)
	}
	if evidence.Unclassified != 1 {
		t.Fatalf("media/intro.bik 应计入未归类: %d", evidence.Unclassified)
	}
}

func TestSuggestNameForFileEndToEnd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Airi.vpk")
	buildSingleFileVPK(t, path, []testVPKEntry{
		{name: "addoninfo.txt", content: []byte("\"AddonInfo\"\n{\n\t\"addontitle\" \"Airi\"\n}\n")},
		{name: "models/survivors/survivor_coach/coach.mdl", content: []byte("mdl")},
		{name: "sound/player/survivor/voice/coach/line01.wav", content: []byte("wav")},
		{name: "materials/models/survivors/survivor_coach/skin.vtf", content: []byte("vtf")},
	})

	suggestion, err := SuggestNameForFile(path)
	if err != nil {
		t.Fatalf("SuggestNameForFile: %v", err)
	}
	if suggestion.Stem != "生还者-Coach-Airi" || suggestion.Prefix != "生还者" || suggestion.Subject != "Coach" {
		t.Fatalf("端到端建议不对: %+v", suggestion)
	}

	// 带 [标签] 的文件：编辑体不含标签，外层字节计入预算。
	tagged := filepath.Join(dir, "[人物+coach]生还者-Coach-Airi.vpk")
	buildSingleFileVPK(t, tagged, []testVPKEntry{
		{name: "models/survivors/survivor_coach/coach.mdl", content: []byte("mdl")},
	})
	kept, err := SuggestNameForFile(tagged)
	if err != nil {
		t.Fatalf("SuggestNameForFile(tagged): %v", err)
	}
	if kept.Stem != "生还者-Coach-Airi" || kept.Changed || !kept.Fits {
		t.Fatalf("带标签的已规范名应原样保留: %+v", kept)
	}
}

// vpkTestFile 是构造内存归档条目的便捷函数（目录/基名/扩展名从路径拆出）。
func vpkTestFile(name string) vpk.File {
	dir, base, ext := splitVPKEntryName(name)
	return vpk.File{Dir: dir, Base: base, Ext: ext}
}

func TestComposeNormalizedFilenamePreservesTagsAndHidden(t *testing.T) {
	cases := []struct {
		oldName string
		stem    string
		want    string
	}{
		{"Airi.vpk", "生还者-Coach-Airi", "生还者-Coach-Airi.vpk"},
		{"[人物]Airi.vpk", "生还者-Coach-Airi", "[人物]生还者-Coach-Airi.vpk"},
		{"_[人物+coach]Airi.vpk", "生还者-Coach-Airi", "_[人物+coach]生还者-Coach-Airi.vpk"},
		{"_Airi.vpk", "生还者-Coach-Airi", "_生还者-Coach-Airi.vpk"},
		{"Airi.vpk", "", "Airi.vpk"},
	}
	for _, tc := range cases {
		if got := ComposeNormalizedFilename(tc.oldName, tc.stem); got != tc.want {
			t.Fatalf("ComposeNormalizedFilename(%q, %q) = %q, want %q", tc.oldName, tc.stem, got, tc.want)
		}
	}
}

// TestSuggestionReasonKeysAreStableAndLocalizable 钉住每个判定分支都带 i18n 依据 key：
// key 一律 ASCII、落在 modals.naming.reason.*；参数为纯数据（domain / kind 原文词 + 已格式化数值）。
func TestSuggestionReasonKeysAreStableAndLocalizable(t *testing.T) {
	assertReason := func(label string, suggestion NamingSuggestion, wantKey string) {
		t.Helper()
		if suggestion.ReasonKey != wantKey {
			t.Fatalf("%s reasonKey = %q, want %q（%+v）", label, suggestion.ReasonKey, wantKey, suggestion)
		}
		if ANSIByteLength(suggestion.ReasonKey) != len(suggestion.ReasonKey) {
			t.Fatalf("%s reasonKey 不是纯 ASCII: %q", label, suggestion.ReasonKey)
		}
		if strings.TrimSpace(suggestion.Reason) == "" {
			t.Fatalf("%s 缺少中文兜底 reason", label)
		}
	}

	assertReason("地图", DeriveNameSuggestion("X", NamingEvidence{Total: 5, HasMapBSP: true}, 0), namingReasonMap)

	domain := DeriveNameSuggestion("Airi", namingEvidence(40, true, map[string]map[string]int{"生还者": {"Coach": 6}}), 0)
	assertReason("物件域", domain, namingReasonDomain)
	if domain.ReasonArgs["domain"] != "生还者" || domain.ReasonArgs["percent"] != "15" {
		t.Fatalf("物件域参数不对: %+v", domain.ReasonArgs)
	}

	multi := DeriveNameSuggestion("Pack", namingEvidence(30, true, map[string]map[string]int{"主武器": {"AK47": 5, "M16": 2}}), 0)
	assertReason("同域多行", multi, namingReasonDomainMulti)

	sparse := DeriveNameSuggestion("glubtastic5", namingEvidence(600, true, map[string]map[string]int{"生还者": {"Coach": 12}}), 0)
	assertReason("大而散", sparse, namingReasonSparse)
	if sparse.ReasonArgs["total"] != "600" || sparse.ReasonArgs["percent"] != "2.0" {
		t.Fatalf("大而散参数不对: %+v", sparse.ReasonArgs)
	}

	unclassified := DeriveNameSuggestion("V社启动动画", NamingEvidence{Total: 20, Unclassified: 11}, 0)
	assertReason("未归类", unclassified, namingReasonUnclassified)
	if unclassified.ReasonArgs["percent"] != "55" {
		t.Fatalf("未归类参数不对: %+v", unclassified.ReasonArgs)
	}

	flashlight := DeriveNameSuggestion("Wide Beam", NamingEvidence{Total: 100, FlashlightHits: 30}, 0)
	assertReason("手电筒", flashlight, namingReasonFlashlight)

	admin := DeriveNameSuggestion("Admin System", NamingEvidence{Total: 100, AdminPluginHits: 30}, 0)
	assertReason("管理员插件", admin, namingReasonAdminPlugin)

	model := DeriveNameSuggestion("Pack", NamingEvidence{Total: 100, HasModelFile: true, TypeHits: map[string]int{"模型": 25, "材质": 40}}, 0)
	assertReason("模型", model, namingReasonModel)

	resource := DeriveNameSuggestion("HUD Cleanup", NamingEvidence{Total: 20, TypeHits: map[string]int{"界面": 9, "材质": 4}}, 0)
	assertReason("资源类型", resource, namingReasonResource)
	if resource.ReasonArgs["kind"] != "界面" {
		t.Fatalf("资源类型参数不对: %+v", resource.ReasonArgs)
	}

	none := DeriveNameSuggestion("X", NamingEvidence{Total: 5}, 0)
	assertReason("保留原名", none, namingReasonNone)

	normalized := DeriveNameSuggestion("地图-Cold Front (Part 1)", NamingEvidence{Total: 10, HasMapBSP: true}, 0)
	assertReason("已符合规范", normalized, namingReasonMap)
	if !normalized.AlreadyNormalized {
		t.Fatalf("已规范名应置 AlreadyNormalized: %+v", normalized)
	}
}
