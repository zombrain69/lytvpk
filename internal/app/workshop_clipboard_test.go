package app

import "testing"

// TestParseWorkshopClipboardLink 覆盖"剪贴板里可能是工坊链接"的各种写法。
// 语义对齐 FireAxe 的 PublishedFileUtils.TryParsePublishedFileId / TryParsePublishedFileIdLink：
// 纯数字 ID 与 filedetails 链接都认，别的内容一律不认。
func TestParseWorkshopClipboardLink(t *testing.T) {
	const canonical = "https://steamcommunity.com/sharedfiles/filedetails/?id="
	cases := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{
			name:  "官方链接 sharedfiles",
			input: "https://steamcommunity.com/sharedfiles/filedetails/?id=2302720558",
			want:  canonical + "2302720558",
			ok:    true,
		},
		{
			name:  "workshop 路径同样认",
			input: "https://steamcommunity.com/workshop/filedetails/?id=12345",
			want:  canonical + "12345",
			ok:    true,
		},
		{
			name:  "前后带文字与空白",
			input: "看看这个 https://steamcommunity.com/sharedfiles/filedetails/?id=777 很不错",
			want:  canonical + "777",
			ok:    true,
		},
		{
			name:  "id 不是第一个参数",
			input: "https://steamcommunity.com/sharedfiles/filedetails/?l=schinese&id=42&searchtext=",
			want:  canonical + "42",
			ok:    true,
		},
		{
			name:  "纯数字 ID（带首尾空白）",
			input: " 2302720558 ",
			want:  canonical + "2302720558",
			ok:    true,
		},
		{
			name:  "短数字也算（工坊 ID 长度不固定）",
			input: "100",
			want:  canonical + "100",
			ok:    true,
		},
		{
			name:  "非工坊链接不认",
			input: "https://example.com/?id=5",
			want:  "",
			ok:    false,
		},
		{
			name:  "数字里混字母不认",
			input: "2302720558abc",
			want:  "",
			ok:    false,
		},
		{
			name:  "空字符串不认",
			input: "   ",
			want:  "",
			ok:    false,
		},
		{
			name:  "链接里没有 id 不认",
			input: "https://steamcommunity.com/sharedfiles/filedetails/?l=schinese",
			want:  "",
			ok:    false,
		},
		{
			name:  "普通文本不认",
			input: "这是一段普通文本，不是链接",
			want:  "",
			ok:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseWorkshopClipboardLink(tc.input)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v（输入 %q）", ok, tc.ok, tc.input)
			}
			if got != tc.want {
				t.Fatalf("link = %q, want %q", got, tc.want)
			}
		})
	}
}
