package parser

import "testing"

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
