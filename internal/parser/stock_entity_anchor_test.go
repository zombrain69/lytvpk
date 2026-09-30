package parser

import "testing"

// TestStockEntityAnchorsAddPreciseTags 用**本体真实路径**验证精确锚点通道（D1/D2/D3/D4/D8）。
// 每一条路径都来自游戏自带脚本（playermodel/viewmodel），不是编造的样例。
func TestStockEntityAnchorsAddPreciseTags(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "Chrome 连喷的世界模型不算木喷（D1）",
			path: "models/w_models/weapons/w_pumpshotgun_A.mdl",
			want: []string{"铁喷", "霰弹枪", "所有枪械"},
		},
		{
			name: "木喷自己的世界模型",
			path: "models/w_models/weapons/w_shotgun.mdl",
			want: []string{"木喷", "霰弹枪"},
		},
		{
			name: "高尔夫球杆本体路径带下划线变体（D8）",
			path: "models/weapons/melee/w_golfclub.mdl",
			want: []string{"高尔夫球杆", "近战", "官方近战"},
		},
		{
			name: "烟花盒的真实模型名（D2）",
			path: "models/props_junk/explosive_box001.mdl",
			want: []string{"烟花盒", "物品"},
		},
		{
			name: "可乐（D3）",
			path: "models/w_models/weapons/w_cola.mdl",
			want: []string{"可乐", "物品"},
		},
		{
			name: "特感爪子归属角色（D4）",
			path: "models/v_models/weapons/v_claw_hunter.mdl",
			want: []string{"hunter", "特殊感染者", "人物"},
		},
		{
			name: "幸存者手臂归属角色",
			path: "models/weapons/arms/v_arms_coach_new.mdl",
			want: []string{"Coach", "幸存者", "人物"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags := make(map[string]bool)
			collectContentTags(tc.path, tags)
			for _, want := range tc.want {
				if !tags[want] {
					t.Fatalf("路径 %q 缺少标签 %q，实际 %v", tc.path, want, tags)
				}
			}
		})
	}
}

// TestRealWorldSilencedSMGModTagged 复刻真机案例 3582222262.vpk：
// 该包只替换了 `models/w_models/weapons/w_smg_a.mdl`（消音冲锋枪的世界模型）
// 与 `sound/weapons/smg_silenced/**`、`models/v_models/v_silenced_smg.mdl`，
// 主分类应是武器且必须带上「消音」。这条覆盖"锚点 + 关键词"两路都不能漏。
func TestRealWorldSilencedSMGModTagged(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("models/w_models/weapons", "w_smg_a", "mdl"),
		vpkFile("models/v_models", "v_silenced_smg", "mdl"),
		vpkFile("sound/weapons/smg_silenced/gunother", "smg_slideforward_1", "wav"),
		vpkFile("materials/models/boki/ppsh41", "huan", "vmt"),
		vpkFile("materials/vgui/hud", "reticle_ui_stencil", "vmt"),
	)
	index := buildArchivePathIndex(archive)
	if got := determineVPKType(index); got != "武器" {
		t.Fatalf("主分类应为武器，实际 %q", got)
	}

	tags := make(map[string]bool)
	ProcessWeaponVPK(index, &VPKFile{}, tags)
	collectSupplementaryTypeTags(index, "武器", tags)
	mergeTagSet(tags, index.contentTags)

	for _, want := range []string{"消音", "冲锋枪", "所有枪械"} {
		if !tags[want] {
			t.Fatalf("缺少标签 %q，实际 %v", want, tags)
		}
	}
}

// TestChromeShotgunIsNotPumpShotgun 锁定 D1 的关键词反例：
// TestSharedShotgunPathsDoNotImplyPumpShotgun 锁定共享霰弹枪目录的精度：
// 本体里 pump/chrome/连喷共用这些音效与素材目录，出现泛词 shotgun 不足以断定是木喷。
func TestSharedShotgunPathsDoNotImplyPumpShotgun(t *testing.T) {
	for _, path := range []string{
		"sound/weapons/shotgun/gunother/shotgun_pump_1.wav",
		"materials/models/v_models/weapons/shotgun/qx.vmt",
	} {
		if tag := weaponPathTag(path); tag == "木喷" {
			t.Fatalf("共享路径 %q 不应判成木喷（实际 %q）", path, tag)
		}
	}
	// 真正的木喷世界模型仍然命中。
	if tag := weaponPathTag("models/w_models/weapons/w_shotgun.mdl"); tag != "木喷" {
		t.Fatalf("木喷本体模型应命中木喷，实际 %q", tag)
	}
}

// TestChromeShotgunIsNotPumpShotgun 锁定 D1 的关键词反例：
// 本体 Chrome 连喷的世界模型名字里有 "pumpshotgun"，但不能被「木喷」关键词吃掉。
func TestChromeShotgunIsNotPumpShotgun(t *testing.T) {
	if tag := weaponPathTag("models/w_models/weapons/w_pumpshotgun_A.mdl"); tag == "木喷" {
		t.Fatalf("Chrome 连喷的世界模型不应命中「木喷」关键词（实际 tag=%q）", tag)
	}
	// 作者自建路径里的 pumpshotgun 仍然算木喷（保留旧召回，不误伤社区命名）。
	if tag := weaponPathTag("materials/models/pumpshotgun/custom_skin.vmt"); tag != "木喷" {
		t.Fatalf("作者命名空间下的 pumpshotgun 应仍是木喷，实际 %q", tag)
	}
}

// TestEntityTableShape 保证内置实体表非空且覆盖到关键实体（防止生成/嵌入出问题）。
func TestEntityTableShape(t *testing.T) {
	if stockEntities == nil {
		t.Fatal("实体表未加载")
	}
	if len(stockEntities.Entities) < 50 {
		t.Fatalf("实体数过少：%d", len(stockEntities.Entities))
	}
	if stockEntities.AnchorCount() < 90 {
		t.Fatalf("锚点过少：%d", stockEntities.AnchorCount())
	}
	seen := map[string]bool{}
	for _, entity := range stockEntities.Entities {
		seen[entity.ID] = true
	}
	for _, want := range []string{
		"weapon_shotgun_chrome", "weapon_pumpshotgun", "melee.golfclub",
		"weapon_cola_bottles", "weapon_fireworkcrate", "weapon_hunter_claw",
		"arms.coach",
	} {
		if !seen[want] {
			t.Fatalf("实体表缺少 %q", want)
		}
	}
}
