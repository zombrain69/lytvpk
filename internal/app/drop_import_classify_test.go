package app

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func writeHeadBytes(t *testing.T, path string, head []byte) string {
	t.Helper()
	if err := os.WriteFile(path, head, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// 真机 + 真实库证据：workshop\3558049615.vpk 其实是一个 ZIP（作者上传的是压缩包，
// Steam 按工坊约定把它存成 <id>.vpk）。只按扩展名分类会把它当 VPK 复制进 addons
// 并提示"VPK 安装完成"——用户以为装好了，游戏却读不了这个文件。
func TestClassifyDropImportPathSniffsArchiveContentBehindVPKExtension(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name string
		head []byte
	}{
		{"zip-local-header", []byte{'P', 'K', 0x03, 0x04}},
		{"zip-empty-archive", []byte{'P', 'K', 0x05, 0x06}},
		{"rar4", []byte{'R', 'a', 'r', '!', 0x1A, 0x07, 0x00}},
		{"7z", []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}},
	}
	for _, tc := range cases {
		path := writeHeadBytes(t, filepath.Join(dir, "3558049615-"+tc.name+".vpk"), tc.head)
		kind, err := classifyDropImportPath(path)
		if err != nil {
			t.Fatalf("%s: 分类失败: %v", tc.name, err)
		}
		if kind != dropImportKindArchive {
			t.Fatalf("%s: .vpk 里其实是压缩包，应按压缩包处理，实际=%q", tc.name, kind)
		}
	}
}

func TestClassifyDropImportPathKeepsRealVPKAsVPK(t *testing.T) {
	dir := t.TempDir()
	realVPK := filepath.Join(dir, "真正的地图.vpk")
	writeTestVPK(t, realVPK, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddontitle \"Real\"\n}\n"),
	})
	kind, err := classifyDropImportPath(realVPK)
	if err != nil {
		t.Fatal(err)
	}
	if kind != dropImportKindVPK {
		t.Fatalf("真 VPK 应分类为 vpk，实际=%q", kind)
	}
}

// 反向也要稳：把 VPK 改名成 .zip 的（用户手工改名，或用压缩软件"打包"过一次）
// 不该被当成压缩包去解，否则会解出一堆乱码。
func TestClassifyDropImportPathSniffsVPKContentBehindArchiveExtension(t *testing.T) {
	dir := t.TempDir()
	renamed := filepath.Join(dir, "misnamed.zip")
	writeTestVPK(t, renamed, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddontitle \"Real\"\n}\n"),
	})
	kind, err := classifyDropImportPath(renamed)
	if err != nil {
		t.Fatal(err)
	}
	if kind != dropImportKindVPK {
		t.Fatalf(".zip 里其实是 VPK，应按 VPK 处理，实际=%q", kind)
	}
}

func TestClassifyDropImportPathKeepsDumpAndUnknownExtensions(t *testing.T) {
	dir := t.TempDir()
	dump := writeHeadBytes(t, filepath.Join(dir, "crash.mdmp"), []byte("MDMP"))
	if kind, err := classifyDropImportPath(dump); err != nil || kind != dropImportKindDump {
		t.Fatalf(".mdmp 应保持 dump，实际=%q err=%v", kind, err)
	}

	// 未知扩展名（哪怕里面有 ZIP 魔数）依旧不接受：不扩大入口面。
	unknown := writeHeadBytes(t, filepath.Join(dir, "notes.txt"), []byte{'P', 'K', 0x03, 0x04})
	if kind, err := classifyDropImportPath(unknown); err != nil || kind != dropImportKindUnsupported {
		t.Fatalf("未知扩展名应保持 unsupported，实际=%q err=%v", kind, err)
	}

	// 空文件不能把分类搞崩。
	empty := writeHeadBytes(t, filepath.Join(dir, "empty.vpk"), nil)
	if kind, err := classifyDropImportPath(empty); err != nil || kind != dropImportKindVPK {
		t.Fatalf("空 .vpk 仍按扩展名分类，实际=%q err=%v", kind, err)
	}
}

// 分类对了还不够：解包分发也必须按内容走，否则 .vpk 里其实是 zip 的条目会卡在
// "不支持的压缩格式: .vpk"，用户拿不到解包结果。
func TestExtractVPKFromArchiveDispatchesByContentForMisnamedArchive(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	dir := t.TempDir()

	archivePath := filepath.Join(dir, "3558049615.vpk")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create("inner.vpk")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write([]byte("not-a-real-vpk-but-the-name-is-what-matters")); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(dir, "out")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		t.Fatal(err)
	}
	noop := func(int, string, []string) {}
	if err := a.extractVPKFromArchiveWithProgress(archivePath, destDir, noop); err != nil {
		t.Fatalf(".vpk 里其实是 zip，应按内容分发解包，实际报错: %v", err)
	}
	if _, err := os.Stat(filepath.Join(destDir, "inner.vpk")); err != nil {
		t.Fatalf("解包结果里应有 inner.vpk: %v", err)
	}
}
