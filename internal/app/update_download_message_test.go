package app

import (
	"errors"
	"strings"
	"testing"
)

// TestFormatUpdateDownloadError 更新包下载失败时给出可行动的中文提示（并保留未知错误细节）。
func TestFormatUpdateDownloadError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   string
		absent string
	}{
		{
			name:   "英文超时",
			err:    errors.New(`Get "https://github.com/x.zip": read tcp 1.2.3.4:1->5.6.7.8:443: i/o timeout`),
			want:   "连接超时",
			absent: "i/o timeout",
		},
		{
			name:   "Windows 中文网络错误",
			err:    errors.New("read tcp 10.0.0.2:12969->20.205.243.166:443: wsarecv: 连接尝试失败"),
			want:   "连接超时",
			absent: "wsarecv",
		},
		{
			name: "Windows connectex 英文超时（真机实测）",
			err: errors.New(
				`dial tcp 20.205.243.166:443: connectex: A connection attempt failed because the connected party did not properly respond after a period of time, or established connection failed because connected host has failed to respond.`,
			),
			want:   "连接超时",
			absent: "connectex",
		},
		{
			name:   "连接被拒绝",
			err:    errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
			want:   "连接被拒绝",
			absent: "connection refused",
		},
		{
			name:   "域名解析失败",
			err:    errors.New("lookup github.com: no such host"),
			want:   "无法解析下载地址",
			absent: "no such host",
		},
		{
			name: "未知错误保留细节",
			err:  errors.New("unexpected zip layout"),
			want: "unexpected zip layout",
		},
	}

	for _, tc := range cases {
		got := formatUpdateDownloadError(tc.err)
		if !strings.Contains(got, tc.want) {
			t.Fatalf("%s：%q 缺少 %q", tc.name, got, tc.want)
		}
		if tc.absent != "" && strings.Contains(strings.ToLower(got), strings.ToLower(tc.absent)) {
			t.Fatalf("%s：%q 仍包含原始错误 %q", tc.name, got, tc.absent)
		}
	}

	if formatUpdateDownloadError(nil) != "" {
		t.Fatal("nil 错误应返回空字符串")
	}
}
