package app

import (
	"errors"
	"strings"
	"testing"
)

// TestFormatServerQueryError 服务器查询失败时不再把原始英文网络错误直接展示给用户。
func TestFormatServerQueryError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   string
		absent string
	}{
		{
			name:   "超时",
			err:    errors.New("read udp 127.0.0.1:50211->127.0.0.1:1: i/o timeout"),
			want:   "服务器没有响应",
			absent: "i/o timeout",
		},
		{
			name:   "拒绝连接",
			err:    errors.New("dial udp 127.0.0.1:1: connect: connection refused"),
			want:   "服务器拒绝连接",
			absent: "connection refused",
		},
		{
			name:   "域名解析失败",
			err:    errors.New("lookup zz-not-exist.invalid: no such host"),
			want:   "无法解析服务器地址",
			absent: "no such host",
		},
		{
			name: "未知错误保留细节",
			err:  errors.New("unexpected parser state"),
			want: "unexpected parser state",
		},
	}

	for _, tc := range cases {
		got := formatServerQueryError("查询服务器失败（已重试 3 次）", tc.err)
		if got == nil {
			t.Fatalf("%s：不应返回 nil", tc.name)
		}
		text := got.Error()
		if !strings.Contains(text, tc.want) {
			t.Fatalf("%s：%q 缺少 %q", tc.name, text, tc.want)
		}
		if tc.absent != "" && strings.Contains(strings.ToLower(text), strings.ToLower(tc.absent)) {
			t.Fatalf("%s：%q 仍然包含原始英文 %q", tc.name, text, tc.absent)
		}
	}

	if formatServerQueryError("stage", nil) != nil {
		t.Fatal("nil 错误应返回 nil")
	}
}
