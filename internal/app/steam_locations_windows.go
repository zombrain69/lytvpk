//go:build windows

package app

import (
	"strings"

	"golang.org/x/sys/windows/registry"
)

// readSteamInstallPathFromRegistry 读取 Steam 安装目录。
// 与 FireAxe `GamePathUtils.TryFind`（`GamePathUtils.cs:39-45`）一致：
// 先看 32 位视图 `SOFTWARE\WOW6432Node\Valve\Steam`，再退回 `SOFTWARE\Valve\Steam`。
func readSteamInstallPathFromRegistry() string {
	probes := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Valve\Steam`},
		{registry.LOCAL_MACHINE, `SOFTWARE\Valve\Steam`},
	}
	for _, probe := range probes {
		key, err := registry.OpenKey(probe.root, probe.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, _, valueErr := key.GetStringValue("InstallPath")
		key.Close()
		if valueErr != nil {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		return value
	}
	return ""
}
