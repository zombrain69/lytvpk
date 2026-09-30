//go:build !cua

package app

import "context"

// maybeStartCuaBridge 是默认构建（含发布构建）里的空实现。
//
// CUA 调试桥的真相在 cua_bridge_cua.go，只有 `-tags cua` 构建才会编进来 ——
// 这样桥可以长期留在仓库里随时可用，而发布产物里既没有这个本地 HTTP 端点，
// 也不需要每轮"用完就删"。发布门禁见 scripts/verify-release.ps1 的残留扫描。
func maybeStartCuaBridge(context.Context) {}
