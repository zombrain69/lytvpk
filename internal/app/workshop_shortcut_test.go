package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 工坊快捷方式（.url）生成的单元测试。
//
// 覆盖：解析链三层（.meta → workshop 纯数字文件名 → addonURL0）与优先级、
// 幂等（同 ID 不动原文件）、改写（异 ID）、跳过（无法确定 ID 的各种输入）、
// 受管目录之外与非法扩展名的拒绝，以及重命名时 .url 随行。

func newWorkshopShortcutTestApp(t *testing.T) (*App, string) {
	t.Helper()
	gameDir := filepath.Join(t.TempDir(), "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{rootDir: addonsDir, configDir: filepath.Join(gameDir, "config")}, addonsDir
}

func writeWorkshopShortcutTestVPK(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("vpk"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func wantWorkshopShortcutContent(id string) string {
	return "[InternetShortcut]\r\n" +
		"URL=https://steamcommunity.com/sharedfiles/filedetails/?id=" + id + "\r\n" +
		"IconIndex=0\r\n"
}

// workshop\ 下的纯数字文件名是最常见来源：Steam 官方投递的原样命名。
func TestGenerateWorkshopShortcutsCreatesURLForNumericWorkshopFile(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Requested != 1 || result.Created != 1 || result.Updated != 0 || result.Unchanged != 0 {
		t.Fatalf("统计不符: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "workshop", "123456789.url"))
	if err != nil {
		t.Fatalf("快捷方式没有生成: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("123456789") {
		t.Fatalf("内容不符:\n got: %q\nwant: %q", got, wantWorkshopShortcutContent("123456789"))
	}
}

// 重命名/搬运后 ID 线索只剩 .meta 里的 workshop_id。
func TestGenerateWorkshopShortcutsUsesMetaIDForRenamedFile(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "自定名.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{WorkshopID: "987654321"}); err != nil {
		t.Fatal(err)
	}

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("应从 .meta 解析出 ID 并生成: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "自定名.url"))
	if err != nil {
		t.Fatalf("快捷方式没有生成: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("987654321") {
		t.Fatalf("内容不符: %q", got)
	}
}

// 第三层来源：addoninfo.txt 的 addonURL0（走扫描缓存）。
func TestGenerateWorkshopShortcutsUsesAddonURL0FromCache(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "其他来源.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	app.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Path: vpkPath, Name: "其他来源.vpk",
		AddonURL0: "https://steamcommunity.com/sharedfiles/filedetails/?id=555666777",
	}})

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("应从 addonURL0 解析出 ID: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "其他来源.url"))
	if err != nil {
		t.Fatalf("快捷方式没有生成: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("555666777") {
		t.Fatalf("内容不符: %q", got)
	}
}

// 解析链优先级：.meta 优先于纯数字文件名（与前端 resolveWorkshopID 同序）。
func TestGenerateWorkshopShortcutsPrefersMetaOverFilename(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{WorkshopID: "987654321"}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.GenerateWorkshopShortcuts([]string{vpkPath}); err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "workshop", "123456789.url"))
	if err != nil {
		t.Fatalf("快捷方式没有生成: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("987654321") {
		t.Fatalf("应取 .meta 里的 ID: %q", got)
	}
}

// 隐藏（_ 前缀）的工坊文件：文件名里的 ID 仍然有效（隐藏只是本程序的前缀约定）。
func TestGenerateWorkshopShortcutsResolvesHiddenWorkshopFile(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "_123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Created != 1 {
		t.Fatalf("统计不符: %#v", result)
	}
	raw, err := os.ReadFile(filepath.Join(root, "workshop", "_123456789.url"))
	if err != nil {
		t.Fatalf("快捷方式没有生成: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("123456789") {
		t.Fatalf("内容不符: %q", got)
	}
}

// 无效的 .meta ID（direct- 前缀 / 非数字）不阻塞后续候选。
func TestGenerateWorkshopShortcutsFallsThroughInvalidMetaID(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{WorkshopID: "direct-1"}); err != nil {
		t.Fatal(err)
	}

	if _, err := app.GenerateWorkshopShortcuts([]string{vpkPath}); err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(root, "workshop", "123456789.url"))
	if err != nil {
		t.Fatalf("应回退到纯数字文件名: %v", err)
	}
	if got := string(raw); got != wantWorkshopShortcutContent("123456789") {
		t.Fatalf("内容不符: %q", got)
	}
}

// 既有 .url 已指向同一 ID：原样保留（包括浏览器拖拽生成的完整格式）。
func TestGenerateWorkshopShortcutsKeepsMatchingURLUntouched(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	existing := "[{000214A0-0000-0000-C000-000000000046}]\r\nProp3=19,11\r\n" +
		"[InternetShortcut]\r\nIDList=\r\n" +
		"URL=https://steamcommunity.com/sharedfiles/filedetails/?id=123456789\r\n" +
		"IconIndex=0\r\n"
	urlPath := filepath.Join(root, "workshop", "123456789.url")
	if err := os.WriteFile(urlPath, []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Unchanged != 1 || result.Created != 0 || result.Updated != 0 {
		t.Fatalf("统计不符: %#v", result)
	}
	raw, err := os.ReadFile(urlPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != existing {
		t.Fatalf("既有的同 ID 快捷方式不应被改写:\n%q", string(raw))
	}
}

// 既有 .url 指向别的 ID：改写并计入 Updated。
func TestGenerateWorkshopShortcutsRewritesMismatchedURL(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)
	urlPath := filepath.Join(root, "workshop", "123456789.url")
	if err := os.WriteFile(urlPath, []byte(wantWorkshopShortcutContent("111111111")), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Updated != 1 {
		t.Fatalf("统计不符: %#v", result)
	}
	raw, _ := os.ReadFile(urlPath)
	if got := string(raw); got != wantWorkshopShortcutContent("123456789") {
		t.Fatalf("改写后的内容不符: %q", got)
	}
}

// 重复执行幂等：第二次全部 Unchanged。
func TestGenerateWorkshopShortcutsSecondRunIsUnchanged(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	vpkPath := filepath.Join(root, "workshop", "123456789.vpk")
	writeWorkshopShortcutTestVPK(t, vpkPath)

	if _, err := app.GenerateWorkshopShortcuts([]string{vpkPath}); err != nil {
		t.Fatal(err)
	}
	result, err := app.GenerateWorkshopShortcuts([]string{vpkPath})
	if err != nil {
		t.Fatal(err)
	}
	if result.Created != 0 || result.Updated != 0 || result.Unchanged != 1 {
		t.Fatalf("第二次执行应为 Unchanged: %#v", result)
	}
}

// 无法确定 ID / 非 vpk / 受管目录外：逐项跳过、不产生任何文件、绝不猜测。
func TestGenerateWorkshopShortcutsSkipsUnresolvableInputs(t *testing.T) {
	app, root := newWorkshopShortcutTestApp(t)
	handmade := filepath.Join(root, "handmade.vpk")
	writeWorkshopShortcutTestVPK(t, handmade)
	txt := filepath.Join(root, "readme.txt")
	if err := os.WriteFile(txt, []byte("note"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.vpk")
	writeWorkshopShortcutTestVPK(t, outside)

	result, err := app.GenerateWorkshopShortcuts([]string{handmade, txt, outside})
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	if result.Requested != 3 || result.Created != 0 || result.Updated != 0 || result.Unchanged != 0 {
		t.Fatalf("统计不符: %#v", result)
	}
	if len(result.Skipped) != 3 {
		t.Fatalf("应有 3 条跳过原因: %#v", result.Skipped)
	}
	reasons := map[string]string{}
	for _, skip := range result.Skipped {
		reasons[skip.Path] = skip.Reason
	}
	if !strings.Contains(reasons[handmade], "无法确定工坊 ID") {
		t.Fatalf("handmade 跳过原因不符: %q", reasons[handmade])
	}
	if !strings.Contains(reasons[txt], "不是 .vpk") {
		t.Fatalf("txt 跳过原因不符: %q", reasons[txt])
	}
	if !strings.Contains(reasons[outside], "不在受管") {
		t.Fatalf("outside 跳过原因不符: %q", reasons[outside])
	}
	if _, err := os.Stat(filepath.Join(root, "handmade.url")); !os.IsNotExist(err) {
		t.Fatalf("跳过的文件不应产生 .url")
	}
}

// 重命名时 .url 作为旁车文件随行（保持"快捷方式跟着 Mod 走"）。
func TestHandleSidecarMoveCarriesWorkshopShortcutURL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "999000111.vpk")
	dst := filepath.Join(dir, "999000111_备用.vpk")
	writeWorkshopShortcutTestVPK(t, src)
	urlContent := wantWorkshopShortcutContent("999000111")
	if err := os.WriteFile(filepath.Join(dir, "999000111.url"), []byte(urlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	app.handleSidecarFile(src, dst, "move")

	if _, err := os.Stat(filepath.Join(dir, "999000111.url")); !os.IsNotExist(err) {
		t.Fatalf("旧 .url 应随重命名消失")
	}
	raw, err := os.ReadFile(filepath.Join(dir, "999000111_备用.url"))
	if err != nil {
		t.Fatalf(".url 没有随重命名移动: %v", err)
	}
	if string(raw) != urlContent {
		t.Fatalf(".url 内容变了: %q", string(raw))
	}
}

// copySidecarFiles 复制旁车文件时也带上 .url。
func TestCopySidecarFilesIncludesWorkshopShortcutURL(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.vpk")
	dst := filepath.Join(dir, "b.vpk")
	urlContent := wantWorkshopShortcutContent("999000111")
	if err := os.WriteFile(filepath.Join(dir, "a.url"), []byte(urlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copySidecarFiles(src, dst); err != nil {
		t.Fatalf("复制旁车文件失败: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "b.url"))
	if err != nil {
		t.Fatalf(".url 没有被复制: %v", err)
	}
	if string(raw) != urlContent {
		t.Fatalf(".url 内容变了: %q", string(raw))
	}
}
