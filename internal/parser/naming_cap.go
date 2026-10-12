// 命名规范化（路线图第 20 项）——引擎定长名字缓冲的计量与标题截断器（cap_name 等价物）。
//
// 背景与决策：`docs/development/mod-naming-normalization-eval.md`。参考实现
// sHartet/l4d2-addons 的 naming-rules v1.2（scripts/plan_v4.py 的 cap_name 与幂等判定）。
//
// 硬约束：引擎给 addon 主干名（不含 .vpk）的定长缓冲只有 63 个 ANSI 字节 —— 超长会
// 「静默不挂载」（与体检 filename_too_long 同一约束，见 internal/app/health_check.go）。
// 规范化名字必须压进 63B：**只截断标题段**（分类前缀与替换对象永不缩短），
// 先按空格断词、再按字符断，截完去掉尾部 ` - .` 连接符。
//
// 本文件只放与内容识别无关的纯字符串工具（无副作用、可直接单测）：
//   - ANSIByteLength —— 与引擎定长缓冲同口径的字节计量（ASCII 1 字节、GBK 汉字 2 字节）；
//   - CapAddonStemToANSIBytes —— 对一段文本（标题段）的截断（断词 → 断字 → 尾部清理；
//     幂等：结果自身重跑不再变化）；
//   - ComposeAddonStem —— 组装「前缀-替换对象-标题」并保证前缀段逐字节不动（只动标题段）；
//     前缀段本身就超限时原样返回并报告放不下，交由上层提示「无法压缩」。
//
// 词表映射与替换对象提取（S1–S5）在后续批次落地。
package parser

import (
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

// AddonStemMaxANSIBytes 是引擎给 addon 文件名（主干，不含 .vpk）的定长缓冲上限：
// 超过 63 个 ANSI 字节的包引擎根本不挂载（附加内容列表里看不到、内容全不加载、无报错）。
// 依据：sHartet/l4d2-addons 真机实测（82B / 88B 完全无效、41B 立即生效、58B / 61B 正常）。
const AddonStemMaxANSIBytes = 63

// ANSIByteLength 返回字符串按 Windows ANSI（中文环境即 GBK/CP936）编码后的字节数，
// 与引擎定长名字缓冲的计量口径一致：ASCII 1 字节、汉字 2 字节。
// GBK 表示不了的字符（emoji 等）按 1 字节近似 —— 引擎把这些字转成 '?'，同样只占 1 字节。
func ANSIByteLength(value string) int {
	if encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(value)); err == nil {
		return len(encoded)
	}
	total := 0
	for _, r := range value {
		if encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(string(r))); err == nil {
			total += len(encoded)
			continue
		}
		total++ // 引擎侧会退化成 '?' 这类单字节占位
	}
	return total
}

// CapAddonStemToANSIBytes 把文本截断到 ≤ maxBytes 个 ANSI 字节（命名规范化的标题段截断器）。
//
// 规则（对齐参考体系 cap_name）：
//  1. 预算内能放下就原样返回；
//  2. 先按空格断词：断点落在词中间时回退到最近的空格，丢掉整个残词（不切半词）；
//  3. 没有可用的空格断点再按字符断（中文按单个字断）；
//  4. 截完去掉尾部残留的 ` - .` 连接符；
//  5. 幂等：结果 ≤ maxBytes，重跑必然原样返回。
//
// maxBytes ≤ 0 视为预算为零（返回空串）。返回 (截断后的文本, 是否发生截断)。
func CapAddonStemToANSIBytes(text string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", text != ""
	}
	if ANSIByteLength(text) <= maxBytes {
		return text, false
	}

	runes := []rune(text)
	// byteAt[i] = 前 i 个 rune 的字节长度；用来在预算内找最大可用 rune 数。
	byteAt := make([]int, len(runes)+1)
	for i, r := range runes {
		byteAt[i+1] = byteAt[i] + ANSIByteLength(string(r))
	}
	maxRunes := 0
	for i := len(runes) - 1; i >= 0; i-- {
		if byteAt[i] <= maxBytes {
			maxRunes = i
			break
		}
	}

	cut := maxRunes
	if cut < len(runes) && runes[cut] != ' ' {
		// 断点落在词中间（截断处下一个字符不是空格）→ 回退到最近的空格丢整词。
		for i := cut - 1; i > 0; i-- {
			if runes[i] == ' ' {
				cut = i
				break
			}
		}
	}
	result := strings.TrimRight(string(runes[:cut]), " -.")
	if result == "" {
		// 断词回退把内容清空（全连接符 / 极小预算）——退回按字符断的切口。
		result = strings.TrimRight(string(runes[:maxRunes]), " -.")
	}
	return result, result != text
}

// ComposeAddonStem 把「前缀段 + 原标题」组装为规范化主干并压进 maxBytes 个 ANSI 字节。
//
// head 是「分类-替换对象-」这类前缀（由命名规则层推得；**任何情况下不做缩短**），
// title 是原标题。约束与纪律（见评估文档）：
//   - 前缀段逐字节不动，只截断标题段（CapAddonStemToANSIBytes）；
//   - 前缀段本身就超过 maxBytes 时不做任何压缩，原样返回并报告 fits=false
//     （上层据此提示「前缀+替换对象已达上限，无法压进 63B」）。
//
// 返回 (组装结果, 标题是否被截断, 是否放得下)。
func ComposeAddonStem(head, title string, maxBytes int) (string, bool, bool) {
	if maxBytes <= 0 {
		return head + title, false, false
	}
	if ANSIByteLength(head) > maxBytes {
		return head + title, false, false
	}
	budget := maxBytes - ANSIByteLength(head)
	capped, truncated := CapAddonStemToANSIBytes(title, budget)
	return head + capped, truncated, true
}
