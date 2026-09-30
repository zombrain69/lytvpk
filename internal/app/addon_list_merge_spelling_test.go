package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 真机复现：融合外部 addonlist 时，新增条目被写成规范化（小写）的键 ——
// 来源里写的 "ZZTest_Unrecorded.VPK" 到了当前文件里变成 "zztest_unrecorded.vpk"。
// 项目规则是 addonlist 条目要保留磁盘上的真实拼写，规范化键只用于匹配。
func TestApplyAddonListMergeKeepsSourceSpellingForNewEntries(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "existing.vpk", Value: "1"},
	})

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "addonlist.txt")
	if err := os.WriteFile(sourcePath, []byte(
		"\"AddonList\"\n{\n\t\"ZZTest_Unrecorded.VPK\"\t\t\"1\"\n}\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir, configDir: filepath.Join(root, "config")}
	if err := app.ApplyAddonListMerge(sourcePath, nil); err != nil {
		t.Fatalf("融合失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"ZZTest_Unrecorded.VPK"`) {
		t.Fatalf("新增条目应保留来源拼写，实际内容：\n%s", content)
	}
	if strings.Contains(content, `"zztest_unrecorded.vpk"`) {
		t.Fatalf("不该把条目改写成规范化的小写键：\n%s", content)
	}
	// 原有条目仍在。
	if !strings.Contains(content, `"existing.vpk"`) {
		t.Fatalf("原有条目丢了：\n%s", content)
	}
}

// 冲突项（两边都有）只改值，不改写法。
func TestApplyAddonListMergeKeepsExistingSpellingOnConflict(t *testing.T) {
	root := t.TempDir()
	gameDir := filepath.Join(root, "left4dead2")
	addonsDir := filepath.Join(gameDir, "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeConflictTestAddonList(t, gameDir, []conflictTestAddonListEntry{
		{Name: "Mixed.Case.VPK", Value: "0"},
	})

	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "addonlist.txt")
	if err := os.WriteFile(sourcePath, []byte(
		"\"AddonList\"\n{\n\t\"mixed.case.vpk\"\t\t\"1\"\n}\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir, configDir: filepath.Join(root, "config")}
	if err := app.ApplyAddonListMerge(sourcePath, []string{"mixed.case.vpk"}); err != nil {
		t.Fatalf("融合失败: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(gameDir, "addonlist.txt"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !strings.Contains(content, `"Mixed.Case.VPK"		"1"`) {
		t.Fatalf("冲突项应就地改值、保留原写法，实际内容：\n%s", content)
	}
}
