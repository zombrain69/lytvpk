package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeBackupFixture(t *testing.T, path string, content string, moment time.Time) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// 备份文件的"时间"来自文件名，这里只保证内容可比较。
	_ = moment
}

func TestRotateLocalStoreBackupSkipsInsideInterval(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.Local)
	writeBackupFixture(t, filepath.Join(dir, localStoreBackupFileName("priority", now.Add(-time.Minute))), "old", now)

	created, _, overflow, err := rotateLocalStoreBackup(dir, "priority", []byte("new"), now, 15*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("最短间隔内不应创建新备份")
	}
	if len(overflow) != 0 {
		t.Fatalf("不应产生溢出: %#v", overflow)
	}
}

func TestRotateLocalStoreBackupSkipsIdenticalContent(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.Local)
	writeBackupFixture(t, filepath.Join(dir, localStoreBackupFileName("profiles", now.Add(-2*time.Hour))), "same", now)

	created, _, _, err := rotateLocalStoreBackup(dir, "profiles", []byte("same"), now, 15*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("内容与最近一份相同则不应创建备份")
	}
}

func TestRotateLocalStoreBackupCreatesAndReportsOverflow(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.Local)
	for index := 3; index >= 1; index-- {
		moment := now.Add(-time.Duration(index) * time.Hour)
		writeBackupFixture(t, filepath.Join(dir, localStoreBackupFileName("groups", moment)), "v"+moment.Format("15"), moment)
	}

	created, path, overflow, err := rotateLocalStoreBackup(dir, "groups", []byte("newest"), now, 15*time.Minute, 3)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("超过间隔且内容不同时应该创建备份")
	}
	if got := filepath.Base(path); got != localStoreBackupFileName("groups", now) {
		t.Fatalf("备份文件名 = %q", got)
	}
	if len(overflow) != 1 {
		t.Fatalf("超出上限时应报告 1 个溢出备份: %#v", overflow)
	}
	// 溢出的应是最旧的一份（now-3h）。
	if got := filepath.Base(overflow[0].Path); got != localStoreBackupFileName("groups", now.Add(-3*time.Hour)) {
		t.Fatalf("溢出备份 = %q", got)
	}
}

func TestRotateLocalStoreBackupIgnoresOtherStoresAndFutureFiles(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 9, 22, 10, 0, 0, 0, time.Local)
	writeBackupFixture(t, filepath.Join(dir, localStoreBackupFileName("profiles", now.Add(-time.Hour))), "other", now)
	writeBackupFixture(t, filepath.Join(dir, localStoreBackupFileName("priority", now.Add(time.Hour))), "future", now)
	writeBackupFixture(t, filepath.Join(dir, "priority.json"), "not-a-backup", now)

	created, _, _, err := rotateLocalStoreBackup(dir, "priority", []byte("new"), now, 15*time.Minute, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !created {
		t.Fatal("另一个记录的备份与未来备份都不应参与间隔判断")
	}
}

// TestLocalStoreWritesCreateRotatingBackups 覆盖各 store 写入路径的接入：
// 首次写入不备份，内容变化且超过间隔后产生备份，并调用注入的删除函数处理溢出。
func TestLocalStoreWritesCreateRotatingBackups(t *testing.T) {
	a, _ := newPriorityTestApp(t)
	moment := time.Date(2026, 9, 22, 10, 0, 0, 0, time.Local)
	a.localStoreBackupClock = func() time.Time { return moment }
	a.localStoreBackupInterval = 15 * time.Minute
	a.localStoreBackupMaxFiles = 2
	removed := make([]string, 0)
	a.localStoreBackupRemove = func(path string) error {
		removed = append(removed, path)
		// 真实实现走回收站；测试里直接删除，保证上限约束可观测。
		return os.Remove(path)
	}

	if _, err := a.SetModPriority("a.vpk", "a.vpk", 1); err != nil {
		t.Fatal(err)
	}
	backups, err := a.ListLocalStoreBackups("priority.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 0 {
		t.Fatalf("首次写入不应产生备份: %#v", backups)
	}

	// 内容变化 + 超过间隔 → 产生备份。
	moment = moment.Add(time.Hour)
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 2); err != nil {
		t.Fatal(err)
	}
	backups, err = a.ListLocalStoreBackups("priority.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 {
		t.Fatalf("内容变化后应产生 1 份备份: %#v", backups)
	}

	// 连续三次变化，上限为 2 → 溢出触发删除注入。
	for index := 0; index < 3; index++ {
		moment = moment.Add(time.Hour)
		if _, err := a.SetModPriority("a.vpk", "a.vpk", 10+index); err != nil {
			t.Fatal(err)
		}
	}
	backups, err = a.ListLocalStoreBackups("priority.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) > 2 {
		t.Fatalf("备份数量应受上限约束，实际 %d: %#v", len(backups), backups)
	}
	if len(removed) == 0 {
		t.Fatal("超过上限时应调用溢出删除（回收站）")
	}

	// 内容未变化时不应该继续产生备份。
	moment = moment.Add(time.Hour)
	if _, err := a.SetModPriority("a.vpk", "a.vpk", 10+2); err != nil {
		t.Fatal(err)
	}
	after, err := a.ListLocalStoreBackups("priority.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(backups) {
		t.Fatalf("内容未变化时不应新增备份: %#v → %#v", backups, after)
	}
}
