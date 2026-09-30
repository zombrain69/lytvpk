package app

import "strings"

// 网络失败的三种可行动原因。更新包下载与工坊下载共用同一套关键词判定，
// 避免两边各写一份后慢慢走样；真机实测过的 Windows 文案（connectex、
// wsarecv「连接尝试失败」）都在这里认。
const (
	networkFailureTimeout = "timeout"
	networkFailureRefused = "refused"
	networkFailureDNS     = "dns"
)

// classifyNetworkFailure 返回 networkFailure* 之一；认不出来时返回空串。
func classifyNetworkFailure(err error) string {
	if err == nil {
		return ""
	}
	lower := strings.ToLower(err.Error())
	text := err.Error()
	switch {
	case strings.Contains(lower, "i/o timeout"),
		strings.Contains(lower, "deadline exceeded"),
		strings.Contains(lower, "connectex"),
		strings.Contains(lower, "connection attempt failed"),
		strings.Contains(lower, "did not properly respond"),
		strings.Contains(lower, "failed to respond"),
		strings.Contains(text, "连接尝试失败"),
		strings.Contains(text, "连接超时"),
		strings.Contains(text, "没有正确答复"):
		return networkFailureTimeout
	case strings.Contains(lower, "connection refused"), strings.Contains(text, "拒绝"):
		return networkFailureRefused
	case strings.Contains(lower, "no such host"), strings.Contains(lower, "lookup"):
		return networkFailureDNS
	}
	return ""
}
