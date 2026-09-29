package app

import (
	"strings"
	"testing"
)

// TestParseWorkshopIDErrorsAreActionable 锁定"无效工坊链接"的报错可读性。
//
// 之前这里返回英文的 invalid URL / could not find valid workshop ID in URL，
// 而下载页会原样显示"解析失败: could not find valid workshop ID in URL"。
// 现在要求：中文说明 + 告诉用户该粘贴什么。
func TestParseWorkshopIDErrorsAreActionable(t *testing.T) {
	app := &App{}
	cases := map[string]string{
		"这不是链接":                 "工坊 ID",
		"https://example.com/123": "工坊 ID",
		"https://steamcommunity.com/sharedfiles/filedetails/?id=abc": "工坊 ID",
	}
	for input, want := range cases {
		_, err := app.ParseWorkshopID(input)
		if err == nil {
			t.Fatalf("%q 应该解析失败", input)
		}
		message := err.Error()
		if !strings.Contains(message, want) {
			t.Fatalf("%q 的提示应包含 %q，实际: %s", input, want, message)
		}
		for _, ascii := range []string{"invalid URL", "could not find valid workshop ID"} {
			if strings.Contains(message, ascii) {
				t.Fatalf("%q 的提示不应再出现英文原文 %q：%s", input, ascii, message)
			}
		}
	}

	// 正常输入仍然要能解析出 ID。
	id, err := app.ParseWorkshopID("https://steamcommunity.com/sharedfiles/filedetails/?id=1234567890")
	if err != nil || id != "1234567890" {
		t.Fatalf("正常链接解析失败: id=%q err=%v", id, err)
	}
}
