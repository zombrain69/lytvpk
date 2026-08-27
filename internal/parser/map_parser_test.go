package parser

import (
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

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
