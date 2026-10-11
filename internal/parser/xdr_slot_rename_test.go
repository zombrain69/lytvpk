package parser

import (
	"reflect"
	"testing"
)

func TestPlanXDRSlotRenamesRewritesMatchingSlotOnly(t *testing.T) {
	entries := []string{
		"models/xdreanims/bill_slot_002.mdl",
		"models/xdreanims/bill_slot_002.ani",
		"models/xdreanims/zoey_slot_002.mdl",
		"models/xdreanims/bill_slot_030.ani",
		"addoninfo.txt",
	}
	got := PlanXDRSlotRenames(entries, "Bill", 2, 44)
	want := []XDRSlotRename{
		{From: "models/xdreanims/bill_slot_002.mdl", To: "models/xdreanims/bill_slot_044.mdl"},
		{From: "models/xdreanims/bill_slot_002.ani", To: "models/xdreanims/bill_slot_044.ani"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("改名规划不符：\n got=%#v\nwant=%#v", got, want)
	}
}

func TestPlanXDRSlotRenamesUsesAliasTokens(t *testing.T) {
	// 引擎模型名 token（namvet = Bill）也要命中。
	got := PlanXDRSlotRenames([]string{"models/xdreanims/namvet_slot_007.mdl"}, "Bill", 7, 20)
	if len(got) != 1 || got[0].To != "models/xdreanims/namvet_slot_020.mdl" {
		t.Fatalf("别名 token 应命中 Bill：%#v", got)
	}
}

func TestPlanXDRSlotRenamesKeepsDigitWidth(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
	}{
		{"三位数保持三位", "models/xdreanims/coach_slot_008.mdl", "models/xdreanims/coach_slot_047.mdl"},
		{"一位数不补零成三位", "models/xdreanims/coach_slot_8.mdl", "models/xdreanims/coach_slot_47.mdl"},
		{"两位数保持两位", "models/xdreanims/coach_slot_08.mdl", "models/xdreanims/coach_slot_47.mdl"},
		{"新槽号位数更多时不截断", "models/xdreanims/coach_slot_8.mdl", "models/xdreanims/coach_slot_40.mdl"},
	}
	for _, tc := range cases {
		got := PlanXDRSlotRenames([]string{tc.in}, "Coach", 8, 47)
		if tc.in == "models/xdreanims/coach_slot_8.mdl" && tc.name == "新槽号位数更多时不截断" {
			got = PlanXDRSlotRenames([]string{tc.in}, "Coach", 8, 40)
		}
		if len(got) != 1 || got[0].To != tc.out {
			t.Fatalf("%s：期望 %q，得到 %#v", tc.name, tc.out, got)
		}
	}
}

func TestPlanXDRSlotRenamesIgnoresDirectoryPattern(t *testing.T) {
	// 目录名里的 zoey_slot_030_anims 是源文件工作目录，不是资源路径：basename 不命中就不动。
	entries := []string{
		"models/xdreanims/decompiled 0.71/zoey_slot_030_anims/heal_self_standing.smd",
		"models/xdreanims/decompiled 0.71/zoey_slot_030.qc",
	}
	got := PlanXDRSlotRenames(entries, "Zoey", 30, 45)
	want := []XDRSlotRename{
		{From: "models/xdreanims/decompiled 0.71/zoey_slot_030.qc", To: "models/xdreanims/decompiled 0.71/zoey_slot_045.qc"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("目录模式不应命中、qc basename 应命中：%#v", got)
	}
}

func TestPlanXDRSlotRenamesPreservesCaseAndSpacingForms(t *testing.T) {
	got := PlanXDRSlotRenames([]string{"models/xdreanims/Bill Slot 007.MDL"}, "Bill", 7, 12)
	if len(got) != 1 || got[0].To != "models/xdreanims/Bill Slot 012.MDL" {
		t.Fatalf("大小写与空格写法的数字段应原位替换：%#v", got)
	}
}

func TestPlanXDRSlotRenamesSkipsUnknownCharacterAndInvalidArgs(t *testing.T) {
	entries := []string{"models/xdreanims/zzz_slot_002.mdl"}
	if got := PlanXDRSlotRenames(entries, "Bill", 2, 44); len(got) != 0 {
		t.Fatalf("未识别角色不应改名：%#v", got)
	}
	if got := PlanXDRSlotRenames(entries, "", 2, 44); got != nil {
		t.Fatalf("缺角色应返回 nil：%#v", got)
	}
	if got := PlanXDRSlotRenames(entries, "Bill", 0, 44); got != nil {
		t.Fatalf("非法槽号应返回 nil：%#v", got)
	}
	if got := PlanXDRSlotRenames(entries, "Bill", 2, 2); got != nil {
		t.Fatalf("同槽改名应返回 nil：%#v", got)
	}
}

func TestPlanXDRSlotRenamesPicksFirstMatchingPatternInBase(t *testing.T) {
	// basename 里出现多个槽位模式时，取第一个"角色+槽号都命中"的。
	got := PlanXDRSlotRenames([]string{"models/xdreanims/zzz_slot_002_bill_slot_007.mdl"}, "Bill", 7, 20)
	if len(got) != 1 || got[0].To != "models/xdreanims/zzz_slot_002_bill_slot_020.mdl" {
		t.Fatalf("应命中第二个模式：%#v", got)
	}
}
