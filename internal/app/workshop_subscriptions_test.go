package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseSubscriptionVDF(t *testing.T) {
	content := "\"subscribedfiles\"\n" +
		"{\n" +
		"\t\"appid\"\t\t\"550\"\n" +
		"\t\"time_last_updated\"\t\t\"1791328600\"\n" +
		"\t\"0\"\n" +
		"\t{\n" +
		"\t\t\"publishedfileid\"\t\t\"3815004017\"\n" +
		"\t\t\"time_subscribed\"\t\t\"1791369027\"\n" +
		"\t\t\"disabled_locally\"\t\t\"0\"\n" +
		"\t}\n" +
		"\t\"1\"\n" +
		"\t{\n" +
		"\t\t\"publishedfileid\"\t\t\"3644332748\"\n" +
		"\t\t\"time_subscribed\"\t\t\"1791221653\"\n" +
		"\t\t\"disabled_locally\"\t\t\"1\"\n" +
		"\t}\n" +
		"}\n"

	updated, entries := parseSubscriptionVDF(content)
	if updated != 1791328600 {
		t.Fatalf("time_last_updated = %d", updated)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, 期望 2", len(entries))
	}
	if entries[0].PublishedFileID != "3815004017" || entries[0].TimeSubscribed != 1791369027 || entries[0].DisabledLocally {
		t.Fatalf("条目 0 = %#v", entries[0])
	}
	if entries[1].PublishedFileID != "3644332748" || !entries[1].DisabledLocally {
		t.Fatalf("条目 1 = %#v", entries[1])
	}

	// 空内容 / 垃圾内容：返回空结果不 panic。
	emptyUpdated, emptyEntries := parseSubscriptionVDF("")
	if emptyUpdated != 0 || len(emptyEntries) != 0 {
		t.Fatalf("空内容应为零值：%d %#v", emptyUpdated, emptyEntries)
	}
}

func TestFindBestSubscriptionArchive(t *testing.T) {
	older := "\"subscribedfiles\" { \"appid\" \"550\" \"time_last_updated\" \"100\" \"0\" { \"publishedfileid\" \"1\" \"time_subscribed\" \"1\" \"disabled_locally\" \"0\" } }"
	newer := "\"subscribedfiles\" { \"appid\" \"550\" \"time_last_updated\" \"200\" \"0\" { \"publishedfileid\" \"2\" \"time_subscribed\" \"2\" \"disabled_locally\" \"0\" } \"1\" { \"publishedfileid\" \"3\" \"time_subscribed\" \"3\" \"disabled_locally\" \"0\" } }"

	steamDir := filepath.FromSlash(`C:/steam`)
	listDirs := func(dir string) ([]string, error) {
		if dir == filepath.Join(steamDir, "userdata") {
			return []string{"111", "222", "333"}, nil
		}
		return nil, os.ErrNotExist
	}
	readFile := func(path string) ([]byte, error) {
		switch {
		case strings.Contains(path, "111"):
			return []byte(older), nil
		case strings.Contains(path, "222"):
			return []byte(newer), nil
		default:
			return nil, os.ErrNotExist
		}
	}

	archive, ok := findBestSubscriptionArchive(steamDir, listDirs, readFile)
	if !ok {
		t.Fatal("应找到订阅档")
	}
	if archive.AccountID != "222" || archive.Updated != 200 || len(archive.Entries) != 2 {
		t.Fatalf("应取最近更新的账号 222：%#v", archive)
	}

	// 没有任何 vdf：找不到。
	noVdf := func(string) ([]byte, error) { return nil, os.ErrNotExist }
	if _, ok := findBestSubscriptionArchive(steamDir, listDirs, noVdf); ok {
		t.Fatal("没有 vdf 时不应报找到")
	}
	// Steam 路径为空。
	if _, ok := findBestSubscriptionArchive("", listDirs, readFile); ok {
		t.Fatal("空 Steam 路径不应报找到")
	}
}

func TestApplySubscriptionLocalState(t *testing.T) {
	entries := []WorkshopSubscription{
		{PublishedFileID: "1"},
		{PublishedFileID: "2"},
		{PublishedFileID: "3"},
		{PublishedFileID: "4"},
	}
	root := filepath.FromSlash(`C:/addons`)
	existing := map[string]bool{
		filepath.FromSlash(`C:/addons/workshop/1.vpk`): true, // 1：已下载 + 根目录副本
		filepath.FromSlash(`C:/addons/1.vpk`):          true,
		filepath.FromSlash(`C:/addons/disabled/2.vpk`): true, // 2：只有禁用副本
		filepath.FromSlash(`C:/addons/workshop/4.vpk`): true, // 4：已下载 + 有标题
	}
	statFile := func(path string) bool { return existing[path] }
	titles := map[string]string{"4": "测试标题"}

	applySubscriptionLocalState(entries, root, statFile, titles)

	if !entries[0].HasWorkshopFile || !entries[0].HasRootCopy || entries[0].HasDisabledCopy {
		t.Fatalf("条目 1 状态 = %#v", entries[0])
	}
	if entries[1].HasWorkshopFile || entries[1].HasRootCopy || !entries[1].HasDisabledCopy {
		t.Fatalf("条目 2 状态 = %#v", entries[1])
	}
	if entries[2].HasWorkshopFile || entries[2].HasRootCopy || entries[2].HasDisabledCopy {
		t.Fatalf("条目 3 应为三处皆无：%#v", entries[2])
	}
	if entries[3].Title != "测试标题" || !entries[3].HasWorkshopFile {
		t.Fatalf("条目 4 状态 = %#v", entries[3])
	}

	downloaded, rootCopies, missing, disabled := countSubscriptionCategories(entries)
	if downloaded != 2 || rootCopies != 1 || missing != 1 || disabled != 0 {
		t.Fatalf("计数 = %d/%d/%d/%d，期望 2/1/1/0", downloaded, rootCopies, missing, disabled)
	}
}
