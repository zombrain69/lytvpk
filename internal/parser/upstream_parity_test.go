package parser

import (
	"sort"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

// 这一组用例对应上游 `fix(parser): 修正界面资源导致人物误判并稳定关键词优先级`。
// 我们的实现是**白名单**（只有角色内容根目录才算证据）+ 有序规则切片，
// 比上游的"黑名单排除界面目录"更严，这里把上游的具体场景钉成回归测试。

func TestUpstreamParity_UIOnlyArchivesAreNotCharacterMods(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		base string
		ext  string
	}{
		{"vgui/hud 血条", "materials/vgui/hud", "survivor_health", "vtf"},
		{"sprites 头像", "materials/sprites", "survivor_icon", "vtf"},
		{"materials/hud 目录段", "materials/hud", "survivor_bar", "vtf"},
		{"particles 粒子", "particles", "survivor_sparks", "pcf"},
		{"sound/ui 提示音", "sound/ui", "survivor_click", "wav"},
		{"scripts 脚本", "scripts", "survivor_helper", "nut"},
		{"resource/ui 界面定义", "resource/ui", "survivor_menu", "res"},
		{"resource 根", "resource", "survivor_menu", "res"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			index := buildArchivePathIndex(testArchive(tc.dir, tc.base, tc.ext))
			if got := determineVPKType(index); got == "人物" {
				t.Fatalf("%s 不应被判成人物 Mod（界面资源只是提到了角色名）", tc.dir)
			}
			tags := make(map[string]bool)
			ProcessCharacterVPK(index, &VPKFile{}, tags)
			if tags["幸存者"] || tags["特殊感染者"] {
				t.Fatalf("%s 不应产生角色标签，实际 %v", tc.dir, tags)
			}
		})
	}
}

func TestUpstreamParity_UncommonInfectedBeatsCommonKeyword(t *testing.T) {
	// `common_male_ceda` 同时包含 common 与 ceda：必须是"非普通感染者"，不能被 common 抢走。
	index := buildArchivePathIndex(testArchive("models/infected", "common_male_ceda", "mdl"))
	if got := determineVPKType(index); got != "人物" {
		t.Fatalf("expected 人物, got %q", got)
	}

	tags := make(map[string]bool)
	ProcessCharacterVPK(index, &VPKFile{}, tags)
	if !tags["uncommon_infected"] {
		t.Fatalf("expected uncommon_infected, got %v", tags)
	}
	if tags["common"] {
		t.Fatalf("common_male_ceda 不应同时命中普通感染者，实际 %v", tags)
	}
}

func TestUpstreamParity_KeywordPriorityIsDeterministic(t *testing.T) {
	// 上游用 map 存关键词表，Go 的 map 迭代顺序随机 → 同一个 VPK 每次可能给出不同标签。
	// 我们改成有序切片后，同一个输入必须永远得到同一组标签。
	const filename = "models/infected/common_male_riot.mdl"
	const runs = 50

	var first []string
	for i := 0; i < runs; i++ {
		tags := make(map[string]bool)
		DetectInfectedType(filename, tags)
		got := make([]string, 0, len(tags))
		for tag := range tags {
			got = append(got, tag)
		}
		sort.Strings(got)
		if i == 0 {
			first = got
			continue
		}
		if strings.Join(got, ",") != strings.Join(first, ",") {
			t.Fatalf("第 %d 次结果不一致：%v vs %v", i, got, first)
		}
	}

	if len(first) == 0 || first[0] != "uncommon_infected" {
		t.Fatalf("expected uncommon_infected 作为唯一标签，实际 %v", first)
	}
}

// 反向保险：真正的角色资源仍然要能被识别（避免把白名单收得过紧）。
func TestUpstreamParity_RealCharacterAssetsStillDetected(t *testing.T) {
	cases := []struct {
		dir  string
		base string
		ext  string
	}{
		{"models/survivors", "survivor_namvet", "mdl"},
		{"models/infected", "infected_hunter", "mdl"},
		{"materials/models/survivors", "survivor_coach", "vmt"},
		{"sound/player/survivor/voice/coach", "youarewelcomeproducer01", "wav"},
	}

	for _, tc := range cases {
		index := buildArchivePathIndex(testArchive(tc.dir, tc.base, tc.ext))
		if got := determineVPKType(index); got != "人物" {
			t.Fatalf("%s/%s 应判成人物，实际 %q", tc.dir, tc.base, got)
		}
	}

	_ = vpk.File{}
}
