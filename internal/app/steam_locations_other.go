//go:build !windows

package app

// readSteamInstallPathFromRegistry 在非 Windows 平台没有注册表可读，
// 自动发现会退化为"盘符 × 常见相对路径"扫描。
func readSteamInstallPathFromRegistry() string {
	return ""
}
