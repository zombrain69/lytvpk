package parser

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
	"l4d2-manager-next/pkg/vpkmission"
)

// 本地化 token 不是战役名/章节名（真机案例 2239816060.vpk：14 个 mission 文件的
// DisplayTitle 都是 #L4D360UI_CampaignName_C*，筛选条里多出 14 个"子标签"）。
func TestConvertMissionCampaignDropsLocalizationTokens(t *testing.T) {
	mission := &vpkmission.Campaign{
		Title: "#L4D360UI_CampaignName_C9",
		Chapters: []*vpkmission.Chapter{
			{Code: "c9m1_city", Title: "#L4D360UI_ChapterName_C9M1", Modes: []string{"coop"}},
			{Code: "c9m2_town", Title: "主街", Modes: []string{"coop"}},
		},
	}

	campaign := convertMissionCampaign(mission)
	if campaign == nil {
		t.Fatal("expected campaign")
	}
	if campaign.Title != "" {
		t.Fatalf("token 战役名应被清空（留给调用方用文件名兜底），实际 %q", campaign.Title)
	}
	if len(campaign.Chapters) != 2 {
		t.Fatalf("expected two chapters, got %d", len(campaign.Chapters))
	}
	if campaign.Chapters[0].Title != "c9m1_city" {
		t.Fatalf("token 章节名应退回章节代码，实际 %q", campaign.Chapters[0].Title)
	}
	if campaign.Chapters[1].Title != "主街" {
		t.Fatalf("正常章节名必须保留，实际 %q", campaign.Chapters[1].Title)
	}
}

func TestMissionFileStem(t *testing.T) {
	cases := map[string]string{
		"missions/campaign9.txt":            "campaign9",
		"missions\\sub\\nanningcity_m1.txt": "nanningcity_m1",
		"campaign14.txt":                    "campaign14",
		"":                                  "",
	}
	for input, want := range cases {
		if got := missionFileStem(input); got != want {
			t.Fatalf("missionFileStem(%q) = %q，期望 %q", input, got, want)
		}
	}
}

func TestParseMissionContentHandlesInlineModeBrace(t *testing.T) {
	mission := `"mission"
{
	"DisplayTitle" "City Escape"
	"modes"
	{
		"coop" {
			"1"
			{
				"Map" "c1m1_hotel"
				"DisplayName" "The Hotel"
			}
		}
	}
}`

	campaign := ParseMissionContent(strings.NewReader(mission))
	if campaign == nil {
		t.Fatalf("expected campaign")
	}
	if campaign.Title != "City Escape" {
		t.Fatalf("expected title City Escape, got %q", campaign.Title)
	}
	if len(campaign.Chapters) != 1 {
		t.Fatalf("expected one chapter, got %d", len(campaign.Chapters))
	}

	chapter := campaign.Chapters[0]
	if chapter.Code != "c1m1_hotel" {
		t.Fatalf("expected chapter code c1m1_hotel, got %q", chapter.Code)
	}
	if chapter.Title != "The Hotel" {
		t.Fatalf("expected chapter title The Hotel, got %q", chapter.Title)
	}
	if len(chapter.Modes) != 1 || chapter.Modes[0] != "战役模式" {
		t.Fatalf("expected translated coop mode, got %#v", chapter.Modes)
	}
}

func TestParseMissionContentDecodesGBK(t *testing.T) {
	mission := `"mission"
{
	"DisplayTitle" "中文战役"
	"modes"
	{
		"coop" { "1" { "Map" "c1m1_test" "DisplayName" "第一关" } }
	}
}`
	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(mission))
	if err != nil {
		t.Fatalf("encode mission fixture: %v", err)
	}
	campaign := ParseMissionContent(strings.NewReader(string(encoded)))
	if campaign == nil {
		t.Fatal("expected GBK mission campaign")
	}
	if campaign.Title != "中文战役" || len(campaign.Chapters) != 1 || campaign.Chapters[0].Title != "第一关" {
		t.Fatalf("GBK mission parsed as %#v", campaign)
	}
}

func TestParseMissionContentDecodesWindows1252(t *testing.T) {
	mission := `"mission"
{
	"DisplayTitle" "Café – Ê"
	"modes"
	{
		"coop" { "1" { "Map" "c1m1_test" "DisplayName" "The Café" } }
	}
}`
	encoded, _, err := transform.Bytes(charmap.Windows1252.NewEncoder(), []byte(mission))
	if err != nil {
		t.Fatalf("encode Windows-1252 mission fixture: %v", err)
	}
	campaign := ParseMissionContent(strings.NewReader(string(encoded)))
	if campaign == nil {
		t.Fatal("expected Windows-1252 mission campaign")
	}
	if campaign.Title != "Café – Ê" || len(campaign.Chapters) != 1 || campaign.Chapters[0].Title != "The Café" {
		t.Fatalf("Windows-1252 mission parsed as %#v", campaign)
	}
}
