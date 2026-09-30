package app

import (
	"testing"
	"time"
)

// 没有 Wails 上下文时（后台任务、CLI、测试）下载进度不能调用 runtime.EventsEmit：
// Wails 对无效 ctx 是 log.Fatal，会把整个进程带走（仓库里其它 emit 都先判空，这里曾漏掉）。
// 断言：不崩、不退出，并且进度仍然照常累计。
func TestTaskWriteCounterWithoutWailsContextDoesNotAbort(t *testing.T) {
	task := &DownloadTask{ID: "unit-test", Filename: "demo.vpk", WorkshopID: "1"}
	counter := &TaskWriteCounter{
		Task:     task,
		Total:    10,
		LastTime: time.Now(),
	}

	if _, err := counter.Write([]byte("0123456789")); err != nil {
		t.Fatalf("进度计数写入失败: %v", err)
	}
	if counter.Current != 10 {
		t.Fatalf("计数器应累计到 10，实际 %d", counter.Current)
	}
	if task.DownloadedSize != 10 || task.Progress != 100 {
		t.Fatalf("没有 Wails 上下文时进度仍要更新：downloaded=%d progress=%d", task.DownloadedSize, task.Progress)
	}
}
