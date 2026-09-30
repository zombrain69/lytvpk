package parser

import "testing"

// TestTitleEvidenceAppliesToAllPrimaryTypes 覆盖 W4 通道 4（D7）：
// 标题/描述证据不再只对"主类型=武器"生效，且命中多个型号时每个都要出标签。
func TestTitleEvidenceAppliesToAllPrimaryTypes(t *testing.T) {
	cases := []struct {
		name   string
		title  string
		desc   string
		file   string
		want   []string
		reject []string
	}{
		{
			name:  "参数包标题里的 ak47",
			title: "ak47冰龙参数包紫",
			want:  []string{"AK47", "步枪", "所有枪械"},
		},
		{
			name:  "标题同时写两个型号（D7 多命中）",
			title: "AK47 + M16 替换合集",
			want:  []string{"AK47", "M16"},
		},
		{
			name:  "标题直接写中文标签名",
			title: "糖糖铁喷",
			want:  []string{"铁喷", "霰弹枪"},
		},
		{
			name:  "标题写物品中文名",
			title: "可乐替换",
			want:  []string{"可乐", "物品"},
		},
		{
			name:  "英文物品名",
			title: "Medkit replacement pack",
			want:  []string{"医疗包", "物品", "所有医疗物品"},
		},
		{
			name:   "scar 必须整词匹配（oscar 不算）",
			title:  "Oscar statue texture",
			reject: []string{"三连发", "步枪"},
		},
		{
			name:   "空标题不产生任何标签",
			title:  "",
			desc:   "",
			reject: []string{"AK47", "铁喷"},
		},
		{
			name:  "文件名里的型号也算证据（作者把型号写进文件名）",
			title: "",
			// 真机案例：立华奏ingram_(mac10)切枪语言音频.vpk
			file: "立华奏ingram_(mac10)切枪语言音频.vpk",
			want: []string{"消音"},
		},
		{
			name:  "文件名里的 Riot Shield（真机案例 1827847412.vpk）",
			title: "Scripts: Riot Shield Unlocker",
			want:  []string{"防爆盾"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags := make(map[string]bool)
			applyTitleEvidence(tc.title, tc.desc, tc.file, tags)
			for _, want := range tc.want {
				if !tags[want] {
					t.Fatalf("标题 %q 缺少标签 %q，实际 %v", tc.title, want, tags)
				}
			}
			for _, reject := range tc.reject {
				if tags[reject] {
					t.Fatalf("标题 %q 不应产生标签 %q，实际 %v", tc.title, reject, tags)
				}
			}
		})
	}
}

// TestStockTokenEvidenceMatchesAuthorNamespaces 覆盖 W4 通道 2：
// 作者命名空间里带着本体模型名的"参数包/特效包"要能识别。
func TestStockTokenEvidenceMatchesAuthorNamespaces(t *testing.T) {
	cases := []struct {
		name string
		path string
		want []string
	}{
		{
			name: "材质目录名带 rifle_ak47",
			path: "materials/models/codm/ice/jc/rifle_ak47/ads.vmt",
			want: []string{"AK47", "步枪"},
		},
		{
			name: "粒子文件名带 ak47",
			path: "particles/codm_krig6icedrake-ak47.pcf",
			want: []string{"AK47"},
		},
		{
			name: "粒子文件名用下划线拼接",
			path: "particles/xx2_c6_custom_rifle_ak47.pcf",
			want: []string{"AK47"},
		},
		{
			name: "可乐的自定义命名空间",
			path: "materials/models/necola/cola/custom.vmt",
			want: []string{"可乐", "物品"},
		},
		{
			name: "特感手臂模型（作者目录）",
			path: "models/weapons/arms/v_arms_coach_new.mdl",
			want: []string{"Coach", "幸存者"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tags := make(map[string]bool)
			applyStockTokenEvidence(tc.path, tags, nil)
			for _, want := range tc.want {
				if !tags[want] {
					t.Fatalf("路径 %q 缺少标签 %q，实际 %v", tc.path, want, tags)
				}
			}
		})
	}

	// 反例：普通 HUD 贴图不应因为 token 通道获得武器标签。
	tags := make(map[string]bool)
	applyStockTokenEvidence("materials/vgui/hud/screen_scope.vtf", tags, nil)
	for _, unwanted := range []string{"AK47", "M16", "铁喷", "可乐"} {
		if tags[unwanted] {
			t.Fatalf("HUD 贴图不应获得 %q，实际 %v", unwanted, tags)
		}
	}
}

// TestMetadataRulesCollectAllMatches 锁定 D7：多个型号都要命中，而不是只取第一个。
func TestMetadataRulesCollectAllMatches(t *testing.T) {
	tags := make(map[string]bool)
	DetectWeaponTypeFromMetadata("AK47 and M16 and MAC-10 replacement set", tags)
	for _, want := range []string{"AK47", "M16", "消音"} {
		if !tags[want] {
			t.Fatalf("多型号标题缺少 %q，实际 %v", want, tags)
		}
	}
}
