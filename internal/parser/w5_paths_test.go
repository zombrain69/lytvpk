package parser

import (
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

// TestSpecialInfectedVoiceLayoutIsRecognized 覆盖 D5：本体真实布局是
// sound/player/<特感>/{voice,attack,hit,…}/…（不是 sound/player/infected/voice/<特感>/）。
func TestSpecialInfectedVoiceLayoutIsRecognized(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"sound/player/hunter/voice/warn/hunter_warn_01.wav", "Hunter"},
		{"sound/player/boomer/explode/boomer_explode_01.wav", "Boomer"},
		{"sound/player/tank/attack/hulk_punch_1.wav", "Tank"},
		{"sound/player/pz/voice/zombie_idle_1.wav", "Common Infected"},
		// 巫师走的是 npc 布局（本体真实路径：sound/npc/witch/voice/…）
		{"sound/npc/witch/voice/retreat/horrified_1.wav", "Witch"},
		// 幸存者布局保持不变
		{"sound/player/survivor/voice/coach/positive01.wav", "Coach"},
		// 历史假设的路径仍然兼容（作者可能自建）
		{"sound/player/infected/voice/hunter/warn01.wav", "Hunter"},
	}
	for _, tc := range cases {
		if got := detectVoiceCharacter(tc.path); got != tc.want {
			t.Fatalf("detectVoiceCharacter(%q) = %q，期望 %q", tc.path, got, tc.want)
		}
		if !isCharacterAssetPath(tc.path) {
			t.Fatalf("%q 应算角色资源", tc.path)
		}
	}

	// 不能误伤：脚步/环境音不是角色语音
	for _, path := range []string{
		"sound/player/footsteps/survivor/walk/step1.wav",
		"sound/player/items/pain_pills/pills_use_1.wav",
		"sound/player/water/water_ambient.wav",
	} {
		if got := detectVoiceCharacter(path); got != "" {
			t.Fatalf("%q 不应识别为角色语音，实际 %q", path, got)
		}
	}
}

// TestSpecialInfectedVoicePackGetsCharacterTags 验证语音包最终能拿到角色标签（D5 验收）。
func TestSpecialInfectedVoicePackGetsCharacterTags(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("sound/player/hunter/voice/warn", "hunter_warn_01", "wav"),
		vpkFile("sound/player/hunter/voice/attack", "hunter_attack_01", "wav"),
	)
	index := buildArchivePathIndex(archive)
	if got := determineVPKType(index); got != "人物" {
		t.Fatalf("特感语音包应判成人物，实际 %q", got)
	}

	tags := make(map[string]bool)
	ProcessCharacterVPK(index, &VPKFile{}, tags)
	for _, want := range []string{"hunter", "特殊感染者"} {
		if !tags[want] {
			t.Fatalf("缺少标签 %q，实际 %v", want, tags)
		}
	}
	if !index.contentTags["声音"] {
		t.Fatalf("语音包应带「声音」类别")
	}
}

// TestNewFileCategories 覆盖 D6（粒子）与 D10（resource 非 ui / VScript）。
func TestNewFileCategories(t *testing.T) {
	cases := []struct {
		path string
		want []string
	}{
		{"particles/xx2_c6_custom_rifle_ak47.pcf", []string{"粒子特效"}},
		{"particles/weapon_fx.pcf", []string{"粒子特效"}},
		{"resource/clientscheme.res", []string{"UI"}},
		{"resource/l4d360ui_schinese.txt", []string{"UI"}},
		{"scripts/vscripts/director_base_addon.nut", []string{"脚本", "VScript"}},
		{"materials/skybox/sky_urb01_up.vtf", []string{"天空"}},
	}
	for _, tc := range cases {
		tags := make(map[string]bool)
		collectContentTags(tc.path, tags)
		for _, want := range tc.want {
			if !tags[want] {
				t.Fatalf("路径 %q 缺少类别标签 %q，实际 %v", tc.path, want, tags)
			}
		}
	}
}

// TestMissionsOnlyPackTriggersMissionEvidence 覆盖 D9 的判定：
// 只要有 missions 文件就要提取战役证据（没有 BSP 也不例外）。
func TestMissionsOnlyPackTriggersMissionEvidence(t *testing.T) {
	empty := buildArchivePathIndex(testArchiveFiles(vpkFile("models/props_junk", "gnome", "mdl")))
	if shouldCollectMissionEvidence("其他", empty) {
		t.Fatalf("没有 mission 文件时不应提取战役证据")
	}

	withMissions := buildArchivePathIndex(testArchiveFiles(
		vpkFile("missions", "campaign1", "txt"),
	))
	if !shouldCollectMissionEvidence("其他", withMissions) {
		t.Fatalf("missions-only 包必须提取战役证据")
	}
	if len(withMissions.missionFiles) == 0 {
		t.Fatalf("mission 文件未被索引")
	}
	_ = vpk.File{}
}
