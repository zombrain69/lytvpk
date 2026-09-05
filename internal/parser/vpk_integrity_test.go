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

func TestBuildRepairedAddonInfoUsesWorkshopFallbacksWithoutOverwritingFields(t *testing.T) {
	content := `"AddonInfo"
{
	addontitle "作者原始标题"
	addonDescription "作者原始描述"
}
`
	repaired, summary := BuildRepairedAddonInfoWithMetadata(content, "文件名标题", map[string]string{
		"addontitle":       "工坊标题",
		"addonauthor":      "工坊作者",
		"addonDescription": "工坊描述",
		"addonURL0":        "https://steamcommunity.com/sharedfiles/filedetails/?id=123",
	})
	for _, expected := range []string{
		`"addontitle"`, `"作者原始标题"`, `"作者原始描述"`,
		`"addonauthor"`, `"工坊作者"`, `"addonURL0"`, `"https://steamcommunity.com/sharedfiles/filedetails/?id=123"`,
	} {
		if !strings.Contains(repaired, expected) {
			t.Fatalf("repaired addoninfo missing %s:\n%s", expected, repaired)
		}
	}
	if strings.Contains(repaired, "工坊标题") || strings.Contains(repaired, "工坊描述") {
		t.Fatalf("workshop fallback overwrote existing fields:\n%s", repaired)
	}
	if len(summary.DerivedFields) == 0 {
		t.Fatalf("repair summary did not record derived fields: %+v", summary)
	}
}
