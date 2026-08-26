package parser

import "testing"

func TestTranslateGameModeKeepsPresetTagsStable(t *testing.T) {
	tests := map[string]string{
		"coop":     "战役模式",
		"versus":   "对抗模式",
		"survival": "生存模式",
		"scavenge": "清道夫模式",
		"realism":  "写实模式",
		"halftank": "突变模式",
		"brawler":  "突变模式",
	}

	for mode, want := range tests {
		if got := TranslateGameMode(mode); got != want {
			t.Errorf("TranslateGameMode(%q) = %q, want %q", mode, got, want)
		}
	}
}
