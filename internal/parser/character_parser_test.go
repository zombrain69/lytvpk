package parser

import (
	"reflect"
	"testing"
)

func TestCharacterPresetTagsRemainDetectable(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		detect   func(string, map[string]bool)
		wantTags []string
	}{
		{
			name:     "一代幸存者 Bill",
			path:     "models/survivors/survivor_namvet.mdl",
			detect:   DetectSurvivorType,
			wantTags: []string{"Bill"},
		},
		{
			name:     "二代幸存者 Rochelle",
			path:     "models/survivors/survivor_producer.mdl",
			detect:   DetectSurvivorType,
			wantTags: []string{"Rochelle"},
		},
		{
			name:     "特殊感染者 Tank",
			path:     "models/infected/hulk.mdl",
			detect:   DetectInfectedType,
			wantTags: []string{"tank", "特殊感染者"},
		},
		{
			name:     "特殊感染者 Spitter",
			path:     "models/infected/spitter.mdl",
			detect:   DetectInfectedType,
			wantTags: []string{"spitter", "特殊感染者"},
		},
		{
			name:     "普通感染者",
			path:     "models/infected/common_male.mdl",
			detect:   DetectInfectedType,
			wantTags: []string{"common", "普通感染者"},
		},
		{
			name:     "罕见感染者",
			path:     "models/infected/uncommon_mudman.mdl",
			detect:   DetectInfectedType,
			wantTags: []string{"uncommon_infected", "普通感染者"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := make(map[string]bool)
			tt.detect(tt.path, tags)
			for _, tag := range tt.wantTags {
				if !tags[tag] {
					t.Fatalf("%q 未产生预设需要的标签 %q，实际标签：%v", tt.path, tag, tags)
				}
			}
		})
	}
}

func TestVoiceDirectoryIdentifiesReplacementCharacter(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("sound/player/survivor/voice/coach", "youarewelcomeproducer01", "wav"),
		vpkFile("sound/player/survivor/voice/teengirl", "worldgenericfrancis01", "wav"),
		vpkFile("sound/player/survivor/voice/mechanic", "youarewelcomegambler01", "wav"),
	)
	index := buildArchivePathIndex(archive)

	if got, want := sortedTagSet(index.voiceCharacters), []string{"Coach", "Ellis", "Zoey"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("voice characters = %v, want %v", got, want)
	}

	tags := make(map[string]bool)
	ProcessCharacterVPK(index, &VPKFile{}, tags)
	for _, want := range []string{"Coach", "Ellis", "Zoey", "幸存者"} {
		if !tags[want] {
			t.Fatalf("expected precise voice tag %q in %v", want, tags)
		}
	}
	for _, unexpected := range []string{"Bill", "Francis", "Nick", "Rochelle"} {
		if tags[unexpected] {
			t.Fatalf("filename-only voice mention incorrectly produced %q in %v", unexpected, tags)
		}
	}
}

func TestVoiceDirectoryDoesNotUseFilenameCharacterMentions(t *testing.T) {
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "sound/player/survivor/voice/producer/youarewelcomeellis01.wav", want: "Rochelle"},
		{path: "sound/player/survivor/voice/gambler/youarewelcomeproducer01.wav", want: "Nick"},
		{path: "sound/player/infected/voice/boomer/youarewelcometank01.wav", want: "Boomer"},
	} {
		t.Run(test.path, func(t *testing.T) {
			if got := detectVoiceCharacter(test.path); got != test.want {
				t.Fatalf("detectVoiceCharacter(%q) = %q, want %q", test.path, got, test.want)
			}
		})
	}
}
