package parser

import (
	"reflect"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

func TestXDRParserRecognizesCharacterModelAndSlot(t *testing.T) {
	archive := testArchiveFiles(
		vpk.File{Dir: "models/x dreanims/decompiled 0.71/zoey_slot_030_anims", Base: "heal_self_crouching", Ext: "smd"},
		vpk.File{Dir: "models/x dreanims/decompiled 0.71", Base: "zoey_slot_030", Ext: "mdl"},
		vpk.File{Dir: "models/x dreanims/decompiled 0.71", Base: "rochelle_slot_030", Ext: "ani"},
	)

	index := buildArchivePathIndex(archive)
	file := &VPKFile{Name: "3626294129.vpk", Title: "[xdR] R18 healing animation"}
	buildXDRInfo(index, file)

	if len(file.XDRSlots) != 2 {
		t.Fatalf("xdr slots = %#v, want two character slots", file.XDRSlots)
	}
	if got, want := file.XDRSlots[0].Character, "Rochelle"; got != want {
		t.Fatalf("first character = %q, want %q", got, want)
	}
	if got, want := file.XDRSlots[1].Character, "Zoey"; got != want {
		t.Fatalf("second character = %q, want %q", got, want)
	}
	zoey := file.XDRSlots[1]
	if zoey.Model != "zoey_slot_030" || zoey.Slot != 30 || zoey.SlotLabel != "030" {
		t.Fatalf("zoey slot info = %#v", zoey)
	}
	if !reflect.DeepEqual(zoey.Actions, []string{"治疗·蹲姿"}) {
		t.Fatalf("zoey actions = %v, want 治疗·蹲姿", zoey.Actions)
	}
	if !strings.HasPrefix(file.XDRSummary, "XDR：") {
		t.Fatalf("xdr summary = %q", file.XDRSummary)
	}
}

func TestXDRParserDoesNotPromoteUnrelatedSlotFilename(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "models/props_unique", Base: "slot_030", Ext: "mdl"},
	))
	file := &VPKFile{Name: "ordinary_prop.vpk", Title: "普通模型"}
	buildXDRInfo(index, file)
	if len(file.XDRSlots) != 0 || file.XDRSummary != "" {
		t.Fatalf("unrelated slot filename was classified as XDR: slots=%v summary=%q", file.XDRSlots, file.XDRSummary)
	}
}

func TestXDRParserMarksBaseOrGenericXDRWithoutInventingSlot(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpk.File{Dir: "scripts", Base: "xdr_gestures_disabler", Ext: "txt"},
	))
	file := &VPKFile{Name: "xdReanimsBase.vpk", Title: "xdReanimsBase"}
	buildXDRInfo(index, file)
	if len(file.XDRSlots) != 0 {
		t.Fatalf("generic XDR should not invent slots: %v", file.XDRSlots)
	}
	if file.XDRSummary != "XDR 动画相关：未发现具体角色/模型 slot 文件" {
		t.Fatalf("generic XDR summary = %q", file.XDRSummary)
	}
}
