package parser

import (
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vdf"
)

func TestBuildRepairedAddonInfoPreservesCompleteMetadata(t *testing.T) {
	content := `"AddonInfo"
{
	addonSteamAppID "550"
	addontitle "原始标题"
	addonversion "2.3"
	addontagline "简短说明"
	addonauthor "作者"
	addonAuthorSteamID "123"
	addonURL0 "https://example.invalid/mod"
	addonContent_weapon "1"
	customBlock
	{
		customValue "保留"
	}
}
`

	repaired := BuildRepairedAddonInfo(content, "文件名标题")
	for _, expected := range []string{
		`"addontitle"`, `"原始标题"`, `"addontagline"`, `"简短说明"`,
		`"addonAuthorSteamID"`, `"123"`, `"addonURL0"`, `"addonContent_weapon"`,
		`"customBlock"`, `"customValue"`, `"保留"`,
	} {
		if !strings.Contains(repaired, expected) {
			t.Fatalf("repaired addoninfo missing %s:\n%s", expected, repaired)
		}
	}
	var parsed vdf.KeyValues
	if _, err := parsed.ReadFrom(strings.NewReader(repaired)); err != nil {
		t.Fatalf("repaired addoninfo is not valid KeyValues: %v\n%s", err, repaired)
	}
}

func TestBuildRepairedAddonInfoRecoversTruncatedDescription(t *testing.T) {
	content := `"AddonInfo"
{
	addonSteamAppID "550"
	addontitle "三角行动-信条"
	addonversion "1.0"
	addonauthor "幻木"
	addonDescription "
替换:砍刀
未经允许不得移植
}
`

	repaired := BuildRepairedAddonInfo(content, "fallback")
	if !strings.Contains(repaired, `"addontitle"`) || !strings.Contains(repaired, `三角行动-信条`) {
		t.Fatalf("title was not recovered:\n%s", repaired)
	}
	if !strings.Contains(repaired, `替换:砍刀\n未经允许不得移植`) {
		t.Fatalf("truncated description was not recovered:\n%s", repaired)
	}
	if strings.Contains(repaired, `不得移植\n}`) {
		t.Fatalf("structural brace leaked into description:\n%s", repaired)
	}
	var parsed vdf.KeyValues
	if _, err := parsed.ReadFrom(strings.NewReader(repaired)); err != nil {
		t.Fatalf("recovered addoninfo is not valid KeyValues: %v\n%s", err, repaired)
	}
}

func TestBuildRepairedAddonInfoUsesFilenameWhenMetadataMissing(t *testing.T) {
	repaired := BuildRepairedAddonInfo("", "3790397261")
	if !strings.Contains(repaired, `"addontitle"`) || !strings.Contains(repaired, `"3790397261"`) {
		t.Fatalf("filename title was not added:\n%s", repaired)
	}
	if !strings.Contains(repaired, `"addonDescription"`) || !strings.Contains(repaired, `文件名：3790397261`) {
		t.Fatalf("filename description was not added:\n%s", repaired)
	}
}
