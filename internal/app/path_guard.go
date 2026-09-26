package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

// 本文件对齐 FireAxe 的路径守卫：
//   - `FileSystemUtils.IsValidPath` / `ThrowIfPathInvalid`（`FileSystemUtils.cs:90-113`）
//     负责"这个路径本身是不是能当路径用"；
//   - `FileOutOfAddonRootException`（`AddonNode.cs:405-413`：`value.StartsWith("..") ||
//     Path.IsPathRooted(value)` 直接抛异常）负责"这个路径必须在受管根目录内"。
//
// 本项目不是每个节点各自持有相对路径，而是用绝对路径直接管理真实文件，
// 所以守卫放在**会改动受管文件的操作入口**：删除 / 移动（源）/ 隐藏改名。
// 一个过期路径（用户中途换过 addons 目录、或前端状态陈旧）在旧实现里会照做，
// 现在会被明确拒绝并说明原因。
//
// 注意：**目标目录不设限**。"移动到…"允许把 Mod 移到用户自己选的任意目录
// （例如备份盘），这是既有能力，守卫只约束源文件。

// managedRootDirectories 返回受管根目录：addons 根、workshop、disabled。
// 与扫描范围保持一致（`ScanVPKFiles` 只扫这三处）。
func managedRootDirectories(rootDir string) []string {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil
	}
	return []string{
		filepath.Clean(rootDir),
		filepath.Join(rootDir, "workshop"),
		filepath.Join(rootDir, "disabled"),
	}
}

// pathWithinBase 判断 target 是否落在 base 之内（含 base 本身）。
// 纯文本比较 + 大小写不敏感：Windows 路径不区分大小写，
// 而 `filepath.Rel` 只对卷名做同一性判断，会给 `C:\Addons` 与 `C:\addons` 算出 `..` 前缀。
func pathWithinBase(base string, target string) bool {
	base = filepath.Clean(base)
	target = filepath.Clean(target)
	if strings.EqualFold(base, target) {
		return true
	}
	prefix := base + string(filepath.Separator)
	if len(target) <= len(prefix) {
		return false
	}
	return strings.EqualFold(target[:len(prefix)], prefix)
}

// managedFilePathProblem 返回"这个路径为什么不能当作受管文件来操作"；
// 返回空字符串表示可以操作。调用方直接把返回值当错误信息用。
func managedFilePathProblem(rootDir string, path string) string {
	if strings.TrimSpace(rootDir) == "" {
		return "还没有选择 addons 目录，无法确认这个文件是否受管"
	}
	target := strings.TrimSpace(path)
	if target == "" {
		return "文件路径为空"
	}
	if !filepath.IsAbs(target) {
		return fmt.Sprintf("只接受完整路径，收到的是相对路径: %s", target)
	}

	cleanTarget := filepath.Clean(target)
	// 先排除"目标就是某个受管目录本身"：否则 <addons>\workshop 会被父目录的包含判断先放行，
	// 而删除 / 改名一个目录会连带影响里面所有 Mod。
	for _, dir := range managedRootDirectories(rootDir) {
		if strings.EqualFold(filepath.Clean(dir), cleanTarget) {
			return fmt.Sprintf("%s 是一个受管目录本身，不是可以对单个 Mod 操作的目标", cleanTarget)
		}
	}
	for _, dir := range managedRootDirectories(rootDir) {
		if !pathWithinBase(dir, cleanTarget) {
			continue
		}
		return ""
	}
	return fmt.Sprintf("这个文件不在当前受管的 addons / workshop / disabled 目录里：%s", cleanTarget)
}

// managedRootDirectoryProblem 判断"这个目录能不能当受管根用"（目前只用于诊断与测试）。
func managedRootDirectoryProblem(rootDir string, dir string) string {
	target := strings.TrimSpace(dir)
	if target == "" {
		return "目录为空"
	}
	cleanTarget := filepath.Clean(target)
	for _, candidate := range managedRootDirectories(rootDir) {
		if strings.EqualFold(candidate, cleanTarget) {
			return ""
		}
	}
	return fmt.Sprintf("这个目录不是受管根之一：%s", cleanTarget)
}
