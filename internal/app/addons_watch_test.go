package app

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestIsWatchedAddonsPath(t *testing.T) {
	root := filepath.Join(`C:\`, "games", "left4dead2", "addons")
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"根目录 VPK", filepath.Join(root, "aaa.vpk"), true},
		{"根目录封面图", filepath.Join(root, "aaa.jpg"), true},
		{"根目录 meta", filepath.Join(root, "aaa.vpk.meta"), true},
		{"根目录大小写", filepath.Join(root, "AAA.VPK"), true},
		{"workshop 下 VPK", filepath.Join(root, "workshop", "123.vpk"), true},
		{"workshop 子目录 VPK", filepath.Join(root, "workshop", "group", "123.vpk"), true},
		{"disabled 下 VPK", filepath.Join(root, "disabled", "123.vpk"), true},
		{"WORKSHOP 大小写", filepath.Join(root, "WORKSHOP", "123.vpk"), true},
		{"用户自己的备用版本文件夹", filepath.Join(root, "!!医疗箱（矛版）", "aaa.vpk"), false},
		{"其它子目录", filepath.Join(root, "zz-staging", "aaa.vpk"), false},
		{"不关心的扩展名", filepath.Join(root, "aaa.zip"), false},
		{"根目录下的其它文件", filepath.Join(root, "readme.txt"), false},
		{"目录本身不算文件变化", root, false},
		{"root 之外", filepath.Join(`C:\`, "games", "left4dead2", "other", "aaa.vpk"), false},
		{"空路径", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isWatchedAddonsPath(root, tc.path); got != tc.want {
				t.Fatalf("isWatchedAddonsPath(%q) = %v, 期望 %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestIsWatchedAddonsPathEmptyRoot(t *testing.T) {
	if isWatchedAddonsPath("", filepath.Join("x", "aaa.vpk")) {
		t.Fatal("root 为空时必须返回 false")
	}
}

func TestIsAddonsWatchSubtreeDir(t *testing.T) {
	root := filepath.Join(`C:\`, "addons")
	cases := []struct {
		dir  string
		want bool
	}{
		{filepath.Join(root, "workshop"), true},
		{filepath.Join(root, "workshop", "group"), true},
		{filepath.Join(root, "disabled"), true},
		{filepath.Join(root, "disabled", "old", "older"), true},
		{filepath.Join(root, "!!医疗箱"), false},
		{root, false},
	}
	for _, tc := range cases {
		if got := isAddonsWatchSubtreeDir(root, tc.dir); got != tc.want {
			t.Fatalf("isAddonsWatchSubtreeDir(%q) = %v, 期望 %v", tc.dir, got, tc.want)
		}
	}
}

type watchNotify struct {
	count    int
	overflow bool
	at       time.Time
}

func newTestAddonsWatcher(quiet, minInterval time.Duration, busy func() bool) (*addonsWatcher, chan watchNotify) {
	events := make(chan watchNotify, 16)
	var mu sync.Mutex
	aw := &addonsWatcher{
		marks:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		quietPeriod: quiet,
		minInterval: minInterval,
		busy:        busy,
		notify: func(count int, overflow bool) {
			mu.Lock()
			defer mu.Unlock()
			select {
			case events <- watchNotify{count: count, overflow: overflow, at: time.Now()}:
			default:
			}
		},
	}
	go aw.run()
	return aw, events
}

func waitNotify(t *testing.T, events chan watchNotify, timeout time.Duration) (watchNotify, bool) {
	t.Helper()
	select {
	case ev := <-events:
		return ev, true
	case <-time.After(timeout):
		return watchNotify{}, false
	}
}

// 一次批量复制会产生成百上千个事件，必须合并成一次通知。
func TestAddonsWatcherCoalescesBurstIntoOneNotification(t *testing.T) {
	aw, events := newTestAddonsWatcher(40*time.Millisecond, 10*time.Millisecond, func() bool { return false })
	defer aw.stop()

	for i := 0; i < 200; i++ {
		aw.record("aaa.vpk", false)
	}

	first, ok := waitNotify(t, events, 2*time.Second)
	if !ok {
		t.Fatal("200 个连续事件没有产生通知")
	}
	if first.count != 200 {
		t.Fatalf("通知里的变化数 = %d，期望 200", first.count)
	}
	if extra, ok := waitNotify(t, events, 300*time.Millisecond); ok {
		t.Fatalf("同一个批次只该通知一次，却又收到 count=%d 的通知", extra.count)
	}
}

// 应用自己的文件操作自己会刷新列表：那一批事件必须被丢弃，避免刷两遍。
func TestAddonsWatcherDropsEventsDuringSelfOperation(t *testing.T) {
	var busyMu sync.Mutex
	busy := true
	aw, events := newTestAddonsWatcher(30*time.Millisecond, 10*time.Millisecond, func() bool {
		busyMu.Lock()
		defer busyMu.Unlock()
		return busy
	})
	defer aw.stop()

	for i := 0; i < 20; i++ {
		aw.record("aaa.vpk", false)
	}
	if ev, ok := waitNotify(t, events, 400*time.Millisecond); ok {
		t.Fatalf("文件操作期间的目录事件不该触发刷新，却收到 count=%d", ev.count)
	}

	busyMu.Lock()
	busy = false
	busyMu.Unlock()

	aw.record("bbb.vpk", false)
	ev, ok := waitNotify(t, events, 2*time.Second)
	if !ok {
		t.Fatal("文件操作结束后的事件应该触发一次刷新")
	}
	if ev.count != 1 {
		t.Fatalf("丢弃过的批次不该被算进来：count=%d，期望 1", ev.count)
	}
}

// 持续写入（一边下载一边解压）时，两次自动刷新之间要有最小间隔。
func TestAddonsWatcherEnforcesMinInterval(t *testing.T) {
	quiet := 30 * time.Millisecond
	minInterval := 250 * time.Millisecond
	aw, events := newTestAddonsWatcher(quiet, minInterval, func() bool { return false })
	defer aw.stop()

	aw.record("aaa.vpk", false)
	first, ok := waitNotify(t, events, 2*time.Second)
	if !ok {
		t.Fatal("第一批没有产生通知")
	}

	time.Sleep(quiet + 20*time.Millisecond)
	aw.record("bbb.vpk", false)
	second, ok := waitNotify(t, events, 2*time.Second)
	if !ok {
		t.Fatal("第二批没有产生通知")
	}
	if gap := second.at.Sub(first.at); gap < minInterval-20*time.Millisecond {
		t.Fatalf("两次通知间隔 %v，小于最小间隔 %v", gap, minInterval)
	}
}

// 监听异常（例如事件缓冲区溢出）必须保守地通知一次，不能静默丢掉整批改动。
func TestAddonsWatcherOverflowStillNotifies(t *testing.T) {
	aw, events := newTestAddonsWatcher(20*time.Millisecond, 10*time.Millisecond, func() bool { return false })
	defer aw.stop()

	aw.record("", true)
	ev, ok := waitNotify(t, events, 2*time.Second)
	if !ok {
		t.Fatal("溢出没有产生通知")
	}
	if !ev.overflow {
		t.Fatal("通知里应该带上 overflow 标记")
	}
}
