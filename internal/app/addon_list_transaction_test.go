package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// push 的"事务化"：写盘 / 派生步骤 / 快照同步包成一个事务，
// 快照同步失败必须把 addonlist.txt 回滚成写前内容，并让派生状态按旧内容重跑。
func TestRunAddonListTransactionRollsBackWhenSnapshotSyncFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addonlist.txt")
	if err := os.WriteFile(path, []byte("original-content"), 0o644); err != nil {
		t.Fatal(err)
	}

	derivedRuns := 0
	syncErr := errors.New("snapshot sync failed")
	err := runAddonListTransaction(
		path,
		func() error { return os.WriteFile(path, []byte("new-content"), 0o644) },
		func() { derivedRuns++ },
		func() error { return syncErr },
	)
	if !errors.Is(err, syncErr) {
		t.Fatalf("应把快照同步失败原样返回: %v", err)
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(content) != "original-content" {
		t.Fatalf("文件应回滚到写前内容: %q", content)
	}
	if derivedRuns != 2 {
		t.Fatalf("派生步骤应在写入后跑一次、回滚后再跑一次，实际 %d", derivedRuns)
	}
}

func TestRunAddonListTransactionKeepsContentOnSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addonlist.txt")
	if err := os.WriteFile(path, []byte("original-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	derivedRuns := 0
	if err := runAddonListTransaction(
		path,
		func() error { return os.WriteFile(path, []byte("new-content"), 0o644) },
		func() { derivedRuns++ },
		func() error { return nil },
	); err != nil {
		t.Fatalf("成功路径不应报错: %v", err)
	}
	content, _ := os.ReadFile(path)
	if string(content) != "new-content" {
		t.Fatalf("成功时内容应保持新值: %q", content)
	}
	if derivedRuns != 1 {
		t.Fatalf("派生步骤只应跑一次，实际 %d", derivedRuns)
	}
}

func TestRunAddonListTransactionSkipsWriteErrorsAndMissingOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "addonlist.txt")
	writeErr := errors.New("disk full")
	if err := runAddonListTransaction(
		path,
		func() error { return writeErr },
		func() { t.Fatal("写失败时不应执行派生步骤") },
		func() error { t.Fatal("写失败时不应执行快照同步"); return nil },
	); !errors.Is(err, writeErr) {
		t.Fatalf("写失败应原样返回: %v", err)
	}

	// 文件本来不存在（首次创建 addonlist.txt）：快照同步失败时不回滚（没有旧内容可回滚），
	// 但仍要把错误报出去。
	syncErr := errors.New("snapshot sync failed")
	if err := runAddonListTransaction(
		path,
		func() error { return os.WriteFile(path, []byte("first-content"), 0o644) },
		nil,
		func() error { return syncErr },
	); !errors.Is(err, syncErr) {
		t.Fatalf("应返回快照同步失败: %v", err)
	}
}
