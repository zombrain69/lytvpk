package app

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestNormalizeCompletedBlocks(t *testing.T) {
	// 100 字节 / 每块 30 字节 ⇒ 4 块（0..3）。
	cases := []struct {
		in   []int
		want []int
	}{
		{nil, []int{}},
		{[]int{2, 0, 2, -1, 9}, []int{0, 2}},
		{[]int{3, 1}, []int{1, 3}},
	}
	for _, tc := range cases {
		got := normalizeCompletedBlocks(tc.in, 100, 30)
		if !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("normalizeCompletedBlocks(%v) = %v, 期望 %v", tc.in, got, tc.want)
		}
	}
}

func TestValidateBlockCheckpointRejectsUnsafeResume(t *testing.T) {
	base := &downloadBlockCheckpoint{
		SchemaVersion: downloadBlockCheckpointSchemaVersion,
		TaskID:        "task-1",
		TotalSize:     100,
		BlockSize:     30,
		Completed:     []int{0},
	}

	if got := validateBlockCheckpoint(base, "task-1", 100, 30); len(got) != 1 {
		t.Fatalf("正常检查点应可用，实际 %v", got)
	}

	mutate := func(fn func(*downloadBlockCheckpoint)) *downloadBlockCheckpoint {
		copyValue := *base
		fn(&copyValue)
		return &copyValue
	}
	rejected := map[string]*downloadBlockCheckpoint{
		"版本不对":  mutate(func(c *downloadBlockCheckpoint) { c.SchemaVersion = 99 }),
		"任务换了":  mutate(func(c *downloadBlockCheckpoint) { c.TaskID = "task-2" }),
		"体积变了":  mutate(func(c *downloadBlockCheckpoint) { c.TotalSize = 200 }),
		"块大小变了": mutate(func(c *downloadBlockCheckpoint) { c.BlockSize = 64 }),
		"没有已完成": mutate(func(c *downloadBlockCheckpoint) { c.Completed = nil }),
		"下标越界":  mutate(func(c *downloadBlockCheckpoint) { c.Completed = []int{7} }),
		"全部块都标记完成": mutate(func(c *downloadBlockCheckpoint) { c.Completed = []int{0, 1, 2, 3} }),
	}
	for name, checkpoint := range rejected {
		if got := validateBlockCheckpoint(checkpoint, "task-1", 100, 30); len(got) != 0 {
			t.Fatalf("%s 时不应允许续传，实际 %v", name, got)
		}
	}
}

func TestSaveAndLoadBlockCheckpointRoundTrip(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "task_final")
	path := downloadBlockCheckpointPath(finalPath)

	if err := saveBlockCheckpoint(path, "task-9", 100, 30, []int{1, 1, 0}); err != nil {
		t.Fatalf("写入失败: %v", err)
	}
	loaded := loadBlockCheckpoint(path, "task-9", 100, 30)
	if loaded == nil {
		t.Fatal("应能读回检查点")
	}
	if !reflect.DeepEqual(loaded.Completed, []int{0, 1}) {
		t.Fatalf("读回的区块 = %v", loaded.Completed)
	}
	if loaded.UpdatedAt == "" {
		t.Fatal("应记录写入时间")
	}

	// 换任务读同一份检查点：不可用（避免把别的任务的数据当自己的）。
	if loadBlockCheckpoint(path, "task-other", 100, 30) != nil {
		t.Fatal("任务 ID 不匹配时不应续传")
	}
}

func TestReusablePartialDownloadRequiresMatchingFileSize(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "task_final")
	if err := os.WriteFile(finalPath, make([]byte, 100), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveBlockCheckpoint(downloadBlockCheckpointPath(finalPath), "task-1", 100, 30, []int{0}); err != nil {
		t.Fatal(err)
	}

	if got := reusablePartialDownload(finalPath, "task-1", 100, 30); got == nil {
		t.Fatal("大小一致且有检查点时应可续传")
	}
	// 临时文件被截断（比如磁盘出问题）：不能续传，必须从头下。
	if err := os.WriteFile(finalPath, make([]byte, 50), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := reusablePartialDownload(finalPath, "task-1", 100, 30); got != nil {
		t.Fatal("临时文件大小不对时不应续传")
	}
	// 文件不存在同理。
	if err := os.Remove(finalPath); err != nil {
		t.Fatal(err)
	}
	if got := reusablePartialDownload(finalPath, "task-1", 100, 30); got != nil {
		t.Fatal("临时文件不存在时不应续传")
	}
}

func TestRemoveDownloadCheckpointFiles(t *testing.T) {
	dir := t.TempDir()
	finalPath := filepath.Join(dir, "task_final")
	if err := os.WriteFile(finalPath, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := saveBlockCheckpoint(downloadBlockCheckpointPath(finalPath), "task-1", 100, 30, []int{0}); err != nil {
		t.Fatal(err)
	}
	removeDownloadCheckpointFiles(finalPath)
	if _, err := os.Stat(finalPath); !os.IsNotExist(err) {
		t.Fatal("临时文件应被删除")
	}
	if _, err := os.Stat(downloadBlockCheckpointPath(finalPath)); !os.IsNotExist(err) {
		t.Fatal("检查点应被删除")
	}
}
