package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机复现：夹具里放一个纯垃圾的 zz-garbage.vpk，
// 列表扫描把它记成"不是有效的 VPK 文件（文件头 …）"（中文），
// 但模型统计窗口显示的是解码库原文「vpk: invalid magic: 96ba173e」。
func TestScanModelStatsTargetDescribesBadVPKInChinese(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "garbage.vpk")
	garbage := make([]byte, 512)
	for index := range garbage {
		garbage[index] = byte(index % 251)
	}
	if err := os.WriteFile(path, garbage, 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: dir}
	item := app.scanModelStatsTarget(modelStatsScanTarget{
		Name:     "garbage.vpk",
		Path:     path,
		Location: "root",
	})

	if item.Message == "" {
		t.Fatal("坏文件应给出原因")
	}
	if !strings.Contains(item.Message, "不是有效的 VPK 文件") {
		t.Fatalf("应复用列表扫描的中文说明，实际 %q", item.Message)
	}
	if strings.HasPrefix(item.Message, "vpk:") {
		t.Fatalf("不该把解码库的英文原文放在开头：%q", item.Message)
	}
	// 原始错误仍要留在括号里，方便排查。
	if !strings.Contains(item.Message, "invalid magic") {
		t.Fatalf("原始错误应保留在括号里，实际 %q", item.Message)
	}
}

// ZIP 伪装成 .vpk 时给出的是"其实是 ZIP 压缩包"这条更具体的说明。
func TestScanModelStatsTargetExplainsZipRenamedToVPK(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zip-in-disguise.vpk")
	if err := os.WriteFile(path, []byte("PK\x03\x04"+strings.Repeat("x", 200)), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: dir}
	item := app.scanModelStatsTarget(modelStatsScanTarget{
		Name:     "zip-in-disguise.vpk",
		Path:     path,
		Location: "root",
	})

	if !strings.Contains(item.Message, "实际是 ZIP 压缩包") {
		t.Fatalf("应指出它其实是 ZIP，实际 %q", item.Message)
	}
}
