package app

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
)

// 外部改动自动发现：别的程序（资源管理器复制、Steam 更新工坊、游戏写文件）
// 往 Mod 目录里增删文件时，界面不该等用户手点刷新。
//
// 设计取舍（性能 / 体验）：
//   - 只监听界面**真的会列出来**的三处位置：根目录的 VPK、workshop/**、disabled/**。
//     用户自己的备用版本文件夹（addons/ 下其它子目录）不参与刷新——界面本来就不列它们，
//     为它们刷新只会白扫一遍几千个 Mod。
//   - 防抖：复制一批 Mod 会产生成百上千个事件，安静 600ms 后只通知一次。
//   - 最小间隔：持续写入（一边下载一边解压）时，两次自动刷新之间至少隔 1.5s。
//   - 应用自己发起的文件操作（移动 / 删除 / 打包）走 fileOpsMu，事件整批丢弃：
//     那些调用点自己会刷新列表，避免"一次批量操作刷两遍"。
//   - 只发事件不直接扫盘：扫描仍由前端的 refreshFilesKeepFilter 统一单飞与合并。
const (
	addonsWatchQuietPeriod = 600 * time.Millisecond
	addonsWatchMinInterval = 1500 * time.Millisecond
)

// addonsWatchExtensions 是被监听的文件类型：Mod 本体 + 同名封面图 / meta 侧车文件。
// 侧车文件也要监听：换一张封面图同样应该让卡片更新。
var addonsWatchExtensions = map[string]struct{}{
	".vpk":  {},
	".meta": {},
	".jpg":  {},
	".jpeg": {},
	".png":  {},
	".gif":  {},
}

// addonsWatchSubdirs 是除了根目录之外、界面会列表的子树。
var addonsWatchSubdirs = []string{"workshop", "disabled"}

// isWatchedAddonsPath 报告某个文件变化是否落在"界面会列出来的位置"：
//
//	<root>/xxx.vpk           根目录里的 Mod
//	<root>/workshop/**       创意工坊 Mod
//	<root>/disabled/**       已禁用 Mod
//
// 其余位置（用户自己的备用版本文件夹、其它子目录）一律返回 false。
func isWatchedAddonsPath(root, path string) bool {
	if root == "" || path == "" {
		return false
	}
	if _, ok := addonsWatchExtensions[strings.ToLower(filepath.Ext(path))]; !ok {
		return false
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) == 1 {
		return true
	}
	for _, name := range addonsWatchSubdirs {
		if strings.EqualFold(parts[0], name) {
			return true
		}
	}
	return false
}

// addonsWatchDirs 返回要挂监听的具体目录（不存在的跳过）。
func addonsWatchDirs(root string) []string {
	dirs := []string{root}
	for _, name := range addonsWatchSubdirs {
		dirs = append(dirs, filepath.Join(root, name))
	}
	existing := dirs[:0]
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			existing = append(existing, dir)
		}
	}
	return existing
}

// addonsWatcher 监听 Mod 目录的外部变化，防抖后通知前端做静默刷新。
type addonsWatcher struct {
	app     *App
	watcher *fsnotify.Watcher
	root    string

	marks    chan struct{}
	stopCh   chan struct{}
	doneCh   chan struct{}
	stopOnce sync.Once

	// 下面两项与通知回调只在测试里替换成毫秒级参数；生产值见包级常量。
	quietPeriod time.Duration
	minInterval time.Duration
	busy        func() bool
	notify      func(count int, overflow bool)

	mu       sync.Mutex
	changes  int
	overflow bool
}

// startAddonsWatcher 为 root 挂监听；root 为空或没有任何可监听目录时返回 nil。
func startAddonsWatcher(app *App, root string) *addonsWatcher {
	if app == nil || root == "" {
		return nil
	}
	dirs := addonsWatchDirs(root)
	if len(dirs) == 0 {
		return nil
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Mod 目录监听不可用（外部改动将需要手动刷新）: %v", err)
		return nil
	}
	added := 0
	for _, dir := range dirs {
		if err := watcher.Add(dir); err != nil {
			log.Printf("监听目录失败 %s: %v", dir, err)
			continue
		}
		added++
	}
	if added == 0 {
		_ = watcher.Close()
		return nil
	}

	aw := &addonsWatcher{
		app:         app,
		watcher:     watcher,
		root:        filepath.Clean(root),
		marks:       make(chan struct{}, 1),
		stopCh:      make(chan struct{}),
		doneCh:      make(chan struct{}),
		quietPeriod: addonsWatchQuietPeriod,
		minInterval: addonsWatchMinInterval,
		busy:        app.IsFileOperationBusy,
		notify:      app.notifyExternalAddonsChange,
	}
	go aw.consume()
	go aw.run()
	log.Printf("已开始监听 Mod 目录外部改动：%s", strings.Join(dirs, " | "))
	return aw
}

// stop 停止监听；可重复调用。
func (aw *addonsWatcher) stop() {
	if aw == nil {
		return
	}
	aw.stopOnce.Do(func() {
		close(aw.stopCh)
		if aw.watcher != nil {
			_ = aw.watcher.Close()
		}
		log.Printf("已停止监听 Mod 目录外部改动（目录句柄已释放）")
	})
	select {
	case <-aw.doneCh:
	case <-time.After(2 * time.Second):
	}
}

// consume 把 fsnotify 的原始事件过滤成"值得刷新"的标记。
func (aw *addonsWatcher) consume() {
	for {
		select {
		case <-aw.stopCh:
			return
		case event, ok := <-aw.watcher.Events:
			if !ok {
				return
			}
			aw.handleEvent(event)
		case err, ok := <-aw.watcher.Errors:
			if !ok {
				return
			}
			// 事件缓冲区溢出等异常：宁可多刷一次，也不要漏掉整批改动。
			log.Printf("Mod 目录监听异常（将保守刷新一次）: %v", err)
			aw.record("", true)
		}
	}
}

func (aw *addonsWatcher) handleEvent(event fsnotify.Event) {
	if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
		return
	}
	// 新建目录：非 Windows 后端不递归，得把新子树挂上（workshop/disabled 下的分组文件夹）。
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
			if isAddonsWatchSubtreeDir(aw.root, event.Name) {
				if err := aw.watcher.Add(event.Name); err != nil {
					log.Printf("监听新目录失败 %s: %v", event.Name, err)
				}
			}
			return
		}
	}
	if !isWatchedAddonsPath(aw.root, event.Name) {
		return
	}
	aw.record(event.Name, false)
}

// isAddonsWatchSubtreeDir 判断目录是否是 workshop / disabled 本身或它们的子目录。
func isAddonsWatchSubtreeDir(root, dir string) bool {
	for _, name := range addonsWatchSubdirs {
		base := filepath.Join(root, name)
		if strings.EqualFold(dir, base) {
			return true
		}
		rel, err := filepath.Rel(base, dir)
		if err != nil {
			continue
		}
		rel = filepath.Clean(rel)
		if rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// record 记一次待通知的变化。
func (aw *addonsWatcher) record(path string, overflow bool) {
	aw.mu.Lock()
	aw.changes++
	aw.overflow = aw.overflow || overflow
	// 一个防抖批次只打一行日志：批量复制 1000 个文件不该刷 1000 行。
	first := aw.changes == 1
	aw.mu.Unlock()

	if first && path != "" {
		log.Printf("检测到 Mod 目录外部改动（本批首个：%s）", filepath.Base(path))
	}
	select {
	case aw.marks <- struct{}{}:
	default:
	}
}

func (aw *addonsWatcher) takeBatch() (int, bool) {
	aw.mu.Lock()
	defer aw.mu.Unlock()
	count, overflow := aw.changes, aw.overflow
	aw.changes, aw.overflow = 0, false
	return count, overflow
}

// run 是"事件 → 防抖 → 通知"的主循环。
func (aw *addonsWatcher) run() {
	defer close(aw.doneCh)
	lastNotify := time.Time{}
	for {
		select {
		case <-aw.stopCh:
			return
		case <-aw.marks:
		}
		if !aw.waitQuiet() {
			return
		}

		// 应用自己的文件操作（移动 / 删除 / 打包）落盘的事件：整批丢弃。
		// 那些调用点结束时会自己刷新列表，不丢状态；这样一次批量操作只刷一遍。
		if aw.busy != nil && aw.busy() {
			count, _ := aw.takeBatch()
			log.Printf("忽略 %d 次自有文件操作产生的目录事件（该操作自行刷新列表）", count)
			continue
		}

		if !lastNotify.IsZero() {
			if wait := aw.minInterval - time.Since(lastNotify); wait > 0 {
				select {
				case <-aw.stopCh:
					return
				case <-time.After(wait):
				}
			}
		}

		count, overflow := aw.takeBatch()
		if count == 0 && !overflow {
			continue
		}
		lastNotify = time.Now()
		log.Printf("外部改动合并为一次自动刷新：%d 个变化（溢出=%v）", count, overflow)
		if aw.notify != nil {
			aw.notify(count, overflow)
		}
	}
}

// waitQuiet 等待事件安静下来；返回 false 表示收到停止信号。
func (aw *addonsWatcher) waitQuiet() bool {
	quiet := aw.quietPeriod
	if quiet <= 0 {
		quiet = addonsWatchQuietPeriod
	}
	timer := time.NewTimer(quiet)
	defer timer.Stop()
	for {
		select {
		case <-aw.stopCh:
			return false
		case <-aw.marks:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quiet)
		case <-timer.C:
			return true
		}
	}
}

// notifyExternalAddonsChange 通知界面"Mod 目录有外部改动，静默刷新一次"。
func (a *App) notifyExternalAddonsChange(count int, overflow bool) {
	a.emitEvent("addons_changed_external", map[string]any{
		"count":    count,
		"overflow": overflow,
	})
}

// restartAddonsWatcher 在切换 Mod 目录 / 启动完成时重建监听。
func (a *App) restartAddonsWatcher() {
	a.stopAddonsWatcher()
	a.mu.RLock()
	root := a.rootDir
	a.mu.RUnlock()
	if root == "" {
		return
	}
	watcher := startAddonsWatcher(a, root)
	if watcher == nil {
		return
	}
	a.addonsWatcherMu.Lock()
	a.addonsWatcher = watcher
	a.addonsWatcherMu.Unlock()
}

// stopAddonsWatcher 停止监听（关闭程序、切换目录前）。
func (a *App) stopAddonsWatcher() {
	a.addonsWatcherMu.Lock()
	watcher := a.addonsWatcher
	a.addonsWatcher = nil
	a.addonsWatcherMu.Unlock()
	if watcher != nil {
		watcher.stop()
	}
}
