package app

import (
	"errors"
	"os"
	"strings"
	"testing"
)

// 工坊下载失败时，任务行里显示的是 task.Error —— 以前这里会直接把
// Go / Windows 的网络原始英文甩出来（真机抓到的 connectex / wsarecv 文案）。
func TestFormatWorkshopDownloadError(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		want   string
		absent string
	}{
		{
			name:   "Windows connectex 超时",
			err:    errors.New(`dial tcp 23.59.72.42:443: connectex: A connection attempt failed because the connected party did not properly respond after a period of time`),
			want:   "连接超时",
			absent: "connectex",
		},
		{
			name:   "中文网络错误",
			err:    errors.New("read tcp 10.0.0.2:1->2.3.4.5:443: wsarecv: 连接尝试失败"),
			want:   "连接超时",
			absent: "wsarecv",
		},
		{
			name:   "连接被拒绝",
			err:    errors.New("dial tcp 127.0.0.1:1: connect: connection refused"),
			want:   "连接被拒绝",
			absent: "connection refused",
		},
		{
			name:   "域名解析失败",
			err:    errors.New("lookup steamcdn-a.akamaihd.net: no such host"),
			want:   "无法解析下载地址",
			absent: "no such host",
		},
		{
			name: "已经是中文提示的不再叠字",
			err:  errors.New("下载失败：服务器返回 HTTP 404（稍后重试，或换镜像/优选线路）"),
			want: "HTTP 404",
		},
		{
			name: "未知错误保留细节",
			err:  errors.New("unexpected eof while reading body"),
			want: "unexpected eof while reading body",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := formatWorkshopDownloadError(testCase.err)
			if !strings.Contains(got, testCase.want) {
				t.Fatalf("%q 缺少 %q", got, testCase.want)
			}
			if strings.Count(got, "下载失败：") > 1 {
				t.Fatalf("%q 出现了重复的「下载失败」前缀", got)
			}
			if testCase.absent != "" && strings.Contains(strings.ToLower(got), strings.ToLower(testCase.absent)) {
				t.Fatalf("%q 仍包含原始错误 %q", got, testCase.absent)
			}
		})
	}

	if formatWorkshopDownloadError(nil) != "" {
		t.Fatal("nil 错误应返回空字符串")
	}

	// 磁盘写失败这类本地错误也要给出原因，而不是 os 的原始英文。
	diskFull := &os.PathError{Op: "write", Path: "mod.vpk", Err: errors.New("no space left on device")}
	if got := formatWorkshopDownloadError(diskFull); !strings.Contains(got, "下载失败") {
		t.Fatalf("本地写失败也要有中文前缀，实际 %q", got)
	}
}
