package app

import (
	"os"
	"time"
)

// modTaskTarget 是异步任务开始时捕获的"目标身份"。
//
// 对齐 FireAxe 的 ValidRef / ValidTaskCreator（`ValidRef.cs:14-38`、
// `ValidTaskCreator.cs:22-36`）与 `IValidityExtensions.RegisterInvalidHandler`
// （`IValidityExtensions.cs:8-42`）：上游把节点的"有效性"当成一等公民 ——
// 后台任务持有的是 ValidRef，回写前先复查，节点被删除或替换后任务直接放弃
// （`StartNew` 返回 true 表示"目标已失效"），而不是把结果写到一个已经换了归属的路径上。
//
// 本项目不维护全量对象缓存，但同样存在"检测 / 扫描还在飞行中，
// 用户把 Mod 移走或换掉"的时间窗，所以只需要这一个轻量守卫：
// 捕获时记录路径 + 大小 + 修改时间，回写前复查。
type modTaskTarget struct {
	Path    string
	Size    int64
	ModTime time.Time
}

// captureModTaskTarget 捕获目标身份。目标不存在、是目录或读不到元信息时返回 ok=false。
func captureModTaskTarget(path string) (modTaskTarget, bool) {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return modTaskTarget{}, false
	}
	return modTaskTarget{Path: path, Size: info.Size(), ModTime: info.ModTime()}, true
}

// stillValid 复查目标是否还在原处、还是同一个文件。
// 文件被删除、被移走、被同名目录占用，或者被另一个文件替换（大小 / 修改时间变化）都算失效。
func (t modTaskTarget) stillValid() bool {
	if t.Path == "" {
		return false
	}
	info, err := os.Stat(t.Path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() == t.Size && info.ModTime().Equal(t.ModTime)
}
