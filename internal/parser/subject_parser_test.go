package parser

import (
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

func TestSubjectInfoRecognizesStandardSurvivorVoice(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpkFile("sound/player/survivor/voice/coach", "pain", "wav"),
		vpkFile("sound/player/survivor/voice/coach", "revive", "wav"),
	))
	file := &VPKFile{}
	buildSubjectInfo(index, file)

	if len(file.ContentSubjects) != 1 || file.ContentSubjects[0] != "Coach 语音" {
		t.Fatalf("content subjects = %v, want Coach voice subject", file.ContentSubjects)
	}
	if file.SubjectSummary != "主体：Coach 语音" || file.SubjectConfidence != "高" {
		t.Fatalf("summary/confidence = %q/%q", file.SubjectSummary, file.SubjectConfidence)
	}
}

func TestSubjectInfoDoesNotTreatVoiceLineWordsAsSceneSubjects(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpkFile("sound/player/survivor/voice/coach", "alarm_jukebox_line", "wav"),
	))
	file := &VPKFile{}
	buildSubjectInfo(index, file)

	if file.SubjectSummary != "主体：Coach 语音" {
		t.Fatalf("voice line keywords leaked into subject summary: %q (%v)", file.SubjectSummary, file.ContentSubjects)
	}
}

func TestSubjectInfoRecognizesConcreteWeaponAndItem(t *testing.T) {
	tests := []struct {
		name string
		file vpk.File
		want string
	}{
		{name: "weapon", file: vpkFile("sound/weapons/rifle", "m16_fire", "wav"), want: "M16 武器"},
		{name: "medical", file: vpkFile("models/w_models/weapons", "eq_medkit", "mdl"), want: "医疗包"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := &VPKFile{}
			buildSubjectInfo(buildArchivePathIndex(testArchiveFiles(test.file)), file)
			if !containsTag(file.ContentSubjects, test.want) {
				t.Fatalf("content subjects = %v, want %q", file.ContentSubjects, test.want)
			}
			if file.SubjectConfidence != "高" {
				t.Fatalf("subject confidence = %q, want 高", file.SubjectConfidence)
			}
		})
	}
}

func TestSubjectInfoUsesCampaignForMap(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(vpkFile("maps", "c1m1_hotel", "bsp")))
	file := &VPKFile{Campaign: "Dead Center"}
	buildSubjectInfo(index, file)

	if file.SubjectSummary != "主体：战役地图：Dead Center" {
		t.Fatalf("map subject summary = %q", file.SubjectSummary)
	}
}

func TestSubjectInfoKeepsMapAsPrimarySubjectWhenMapHasExtras(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpkFile("maps", "c2m1_highway", "bsp"),
		vpkFile("models/props_unique", "truck", "mdl"),
	))
	file := &VPKFile{PrimaryTag: "地图", Campaign: "The Passing"}
	buildSubjectInfo(index, file)

	if !strings.HasPrefix(file.SubjectSummary, "主体：战役地图：The Passing") {
		t.Fatalf("map subject must stay primary, got %q", file.SubjectSummary)
	}
	if strings.Contains(file.SubjectSummary, "混合包") {
		t.Fatalf("map subject should not be described as mixed package: %q", file.SubjectSummary)
	}
}

func TestSubjectInfoMarksGenericAudioAsUncertain(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpkFile("sound/zzd", "cache_001", "wav"),
		vpkFile("sound", "sound", "cache"),
	))
	file := &VPKFile{}
	buildSubjectInfo(index, file)

	if len(file.ContentSubjects) != 1 || file.ContentSubjects[0] != "泛音频资源" {
		t.Fatalf("generic audio subjects = %v", file.ContentSubjects)
	}
	if !strings.Contains(file.SubjectSummary, "无法确认具体对象") || file.SubjectConfidence != "低" {
		t.Fatalf("generic audio summary/confidence = %q/%q", file.SubjectSummary, file.SubjectConfidence)
	}
}

func TestSubjectInfoKeepsMixedPackageSubjects(t *testing.T) {
	index := buildArchivePathIndex(testArchiveFiles(
		vpkFile("models/survivors", "survivor_coach", "mdl"),
		vpkFile("models/w_models/weapons", "w_rifle_ak47", "mdl"),
		vpkFile("materials/vgui/hud", "healthbar", "vtf"),
	))
	file := &VPKFile{}
	buildSubjectInfo(index, file)

	for _, want := range []string{"Coach 模型", "AK47 武器", "血条"} {
		if !containsTag(file.ContentSubjects, want) {
			t.Fatalf("mixed package subjects = %v, missing %q", file.ContentSubjects, want)
		}
	}
	if !strings.HasPrefix(file.SubjectSummary, "主体：混合包（") {
		t.Fatalf("mixed package summary = %q", file.SubjectSummary)
	}
}
