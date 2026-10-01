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

// ZIP 伪装成 .vpk 时给出的是"压缩包类条目"的类别说明，而不是"读不出来的 Mod"。
//
// 这类包是工坊作者特意做的插件/工具/教程包：游戏不加载、也不进 addonlist.txt，
// 所以消息要说清"它是什么类型"，而不是让用户以为自己的 Mod 坏了。
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

	if !strings.Contains(item.Message, "压缩包") {
		t.Fatalf("应指出它其实是压缩包，实际 %q", item.Message)
	}
	if !strings.Contains(item.Message, "不参与 addonlist.txt") {
		t.Fatalf("应说明这类包不进 addonlist，实际 %q", item.Message)
	}
	if strings.Contains(item.Message, "vpk:") {
		t.Fatalf("不该把解码库的英文原文摆出来：%q", item.Message)
	}
}
