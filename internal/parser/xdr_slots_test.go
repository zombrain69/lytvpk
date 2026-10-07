package parser

import (
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

// 官方槽位表来自基础包作者的 "Slots designations"，抽查几个代表值 + 越界值。
func TestXDRSlotDesignationTable(t *testing.T) {
	cases := []struct {
		slot  int
		name  string
		group string
	}{
		{1, "noise", "闲置层"},
		{5, "Shotgun", "武器"},
		{21, "walk", "站立/移动"},
		{30, "Reload", "战斗"},
		{32, "Heal_self", "救援/治疗"},
		{39, "gestures", "手势"},
		{44, "其它（44–47）", "其它"},
		{48, "animfixes", "动画修复"},
	}
	for _, tc := range cases {
		got := XDRSlotDesignationFor(tc.slot)
		if got.Name != tc.name || got.Group != tc.group {
			t.Fatalf("slot %d => %#v，期望 %s / %s", tc.slot, got, tc.name, tc.group)
		}
	}
	for _, slot := range []int{-1, 0, 49, 999} {
		if got := XDRSlotDesignationFor(slot); got.Name != "" {
			t.Fatalf("越界槽位 %d 不该有建议用途：%#v", slot, got)
		}
	}
}

// slot 证据要带上"官方建议用途"，摘要里还要写清两条冲突规则。
func TestXDRInfoCarriesOfficialDesignation(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "models/xdreanims", Base: "zoey_slot_032", Ext: "mdl"},
	))
	file := &VPKFile{Name: "heal.vpk", Title: "[xdR] heal"}
	buildXDRInfo(index, file)

	if len(file.XDRSlots) != 1 {
		t.Fatalf("slots = %#v", file.XDRSlots)
	}
	slot := file.XDRSlots[0]
	if slot.SlotName != "Heal_self" || slot.SlotGroup != "救援/治疗" {
		t.Fatalf("slot designation = %q / %q", slot.SlotName, slot.SlotGroup)
	}
	if !strings.Contains(file.XDRSummary, "官方建议 Heal_self") {
		t.Fatalf("摘要没有带上官方建议用途：%q", file.XDRSummary)
	}
	// 同槽随机、跨槽小号优先 —— 这是基础包作者的原话，不能被改成别的说法。
	if !strings.Contains(file.XDRSummary, "同角色同 slot 只会随机生效一个") ||
		!strings.Contains(file.XDRSummary, "slot 数字小的优先") {
		t.Fatalf("摘要缺少冲突规则：%q", file.XDRSummary)
	}
}

// 筛选预设「动作（XDR）」靠标签实现：根级聚合标签 XDR动画 由 parser.go 在 XDRSummary 非空时写入，
// 这里补两个细分标签：槽位动作与基础包。
func TestXDRTagsFeedFilterPreset(t *testing.T) {
	slotIndex := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "models/xdreanims", Base: "zoey_slot_031", Ext: "mdl"},
	))
	slotFile := &VPKFile{Name: "3626294129.vpk", Title: "[xdR] healing animation"}
	buildXDRInfo(slotIndex, slotFile)
	if !xdrHasTag(slotFile.SecondaryTags, "XDR槽位动作") {
		t.Fatalf("槽位动作应带细分标签：%v", slotFile.SecondaryTags)
	}
	if xdrHasTag(slotFile.SecondaryTags, "XDR基础包") {
		t.Fatalf("普通动作不该被当成基础包：%v", slotFile.SecondaryTags)
	}
	if slotFile.XDRSummary == "" {
		t.Fatal("槽位动作的摘要不该为空（parser.go 靠它写 XDR动画 聚合标签）")
	}

	baseIndex := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "models/survivors", Base: "anim_biker", Ext: "mdl"},
	))
	baseFile := &VPKFile{Name: "2121557118.vpk", Title: "xdReanimsBase (L4D2) Anim Mods base"}
	buildXDRInfo(baseIndex, baseFile)
	if !xdrHasTag(baseFile.SecondaryTags, "XDR基础包") {
		t.Fatalf("基础包应带 XDR基础包 标签：%v", baseFile.SecondaryTags)
	}

	markerIndex := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "scripts", Base: "xdr_gestures_disabler", Ext: "txt"},
	))
	markerFile := &VPKFile{Name: "2776331224.vpk", Title: "xdr gestures disabler"}
	buildXDRInfo(markerIndex, markerFile)
	if markerFile.XDRSummary == "" {
		t.Fatal("只带 xdr 标记的包也要有摘要（聚合标签靠它）")
	}

	plainFile := &VPKFile{Name: "ordinary.vpk", Title: "普通模型"}
	buildXDRInfo(buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "models/props", Base: "chair", Ext: "mdl"},
	)), plainFile)
	for _, tag := range plainFile.SecondaryTags {
		if strings.HasPrefix(tag, "XDR") {
			t.Fatalf("普通 Mod 不该带 XDR 标签：%v", plainFile.SecondaryTags)
		}
	}
}

func xdrHasTag(tags []string, want string) bool {
	for _, tag := range tags {
		if tag == want {
			return true
		}
	}
	return false
}
