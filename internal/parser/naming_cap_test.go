// 命名规范化 P1 首块（路线图第 20 项）：ANSI 计量 / 标题截断器 / 前缀段保护 的单测。
package parser

import (
	"strings"
	"testing"
)

func TestANSIByteLengthCountsGBKBytes(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"abc", 3},
		{"中文", 4},
		{"a中b", 4},
		{"【撬棍】", 8},
		{"a🙂", 2}, // GBK 表示不了的表情按 1 字节近似（引擎侧退化成 '?'）
		{"主武器-AK47-", 12},
	}
	for _, tc := range cases {
		if got := ANSIByteLength(tc.input); got != tc.want {
			t.Errorf("ANSIByteLength(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestCapAddonStemToANSIBytes(t *testing.T) {
	cases := []struct {
		name     string
		input    string
		maxBytes int
		want     string
		wantCut  bool
	}{
		{"预算内原样", "abcd", 4, "abcd", false},
		{"ASCII 按字符断", "abcdef", 4, "abcd", true},
		{"断点落在词尾直接保留", "ab cd ef", 5, "ab cd", true},
		{"断点落在词中间丢整词", "hello world", 9, "hello", true},
		{"中文按字断到预算内", strings.Repeat("测", 40), 63, strings.Repeat("测", 31), true},
		{"63 字节整边界不动", strings.Repeat("a", 63), 63, strings.Repeat("a", 63), false},
		{"64 字节截到 63", strings.Repeat("a", 64), 63, strings.Repeat("a", 63), true},
		{"尾部 - . 清理", "ab.- ", 4, "ab", true},
		{"全连接符：清尾后为空（词界回退与字符断都只剩连接符）", "- - ", 2, "", true},
		{"预算为零返回空", "abc", 0, "", true},
	}
	for _, tc := range cases {
		got, cut := CapAddonStemToANSIBytes(tc.input, tc.maxBytes)
		if got != tc.want || cut != tc.wantCut {
			t.Errorf("%s: CapAddonStemToANSIBytes(%q, %d) = (%q, %v), want (%q, %v)",
				tc.name, tc.input, tc.maxBytes, got, cut, tc.want, tc.wantCut)
			continue
		}
		if ANSIByteLength(got) > tc.maxBytes && tc.maxBytes > 0 {
			t.Errorf("%s: 结果 %q 超过预算 %d 字节", tc.name, got, tc.maxBytes)
		}
		if again, cutAgain := CapAddonStemToANSIBytes(got, tc.maxBytes); again != got || cutAgain {
			t.Errorf("%s: 幂等失败——重跑得 (%q, %v)", tc.name, again, cutAgain)
		}
	}
}

func TestComposeAddonStemKeepsHeadVerbatim(t *testing.T) {
	head := "主武器-AK47-"
	title := "A Very Long Title That Exceeds The Budget And More Again" // 56B，放不下 51B 的标题预算
	got, truncated, fits := ComposeAddonStem(head, title, AddonStemMaxANSIBytes)
	if !fits || !truncated {
		t.Fatalf("want fits=true truncated=true, got fits=%v truncated=%v", fits, truncated)
	}
	if !strings.HasPrefix(got, head) {
		t.Fatalf("前缀段被改动: %q", got)
	}
	if ANSIByteLength(got) > AddonStemMaxANSIBytes {
		t.Fatalf("结果 %q 超过 63 字节", got)
	}
	wantTitle := "A Very Long Title That Exceeds The Budget And More"
	if got != head+wantTitle {
		t.Fatalf("ComposeAddonStem = %q, want %q", got, head+wantTitle)
	}
	// 幂等：把已截断的标题再走一遍组装，不再变化。
	again, truncatedAgain, fitsAgain := ComposeAddonStem(head, wantTitle, AddonStemMaxANSIBytes)
	if again != got || truncatedAgain || !fitsAgain {
		t.Fatalf("二次组装应不变: (%q, %v, %v)", again, truncatedAgain, fitsAgain)
	}
}

func TestComposeAddonStemReportsUnfittableHead(t *testing.T) {
	head := strings.Repeat("中", 32) // 64B：前缀段本身就超过 63B
	title := "标题"
	got, truncated, fits := ComposeAddonStem(head, title, AddonStemMaxANSIBytes)
	if fits || truncated {
		t.Fatalf("want fits=false truncated=false, got fits=%v truncated=%v", fits, truncated)
	}
	if got != head+title {
		t.Fatalf("放不下时应原样返回: %q", got)
	}
}

func TestComposeAddonStemZeroBudgetDropsTitle(t *testing.T) {
	head := strings.Repeat("a", AddonStemMaxANSIBytes) // 恰好 63B
	got, truncated, fits := ComposeAddonStem(head, "x", AddonStemMaxANSIBytes)
	if got != head || !truncated || !fits {
		t.Fatalf("want (%q, true, true), got (%q, %v, %v)", head, got, truncated, fits)
	}
}

func TestComposeAddonStemShortTitleUnchanged(t *testing.T) {
	head := "地图-"
	title := "de_dust2 复刻"
	got, truncated, fits := ComposeAddonStem(head, title, AddonStemMaxANSIBytes)
	if got != head+title || truncated || !fits {
		t.Fatalf("want (%q, false, true), got (%q, %v, %v)", head+title, got, truncated, fits)
	}
}
