package app

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// 自身崩溃上报（与已有的 mdmp 查看器互补）
//
// 分工：
//   - mdmp 查看器：读取 Windows 原生崩溃转储（游戏/系统级），偏"事后解剖"；
//   - 这里：本应用自己的 panic / 未处理异常 → 本地**结构化报告**（版本、堆栈、日志尾部），
//     让用户不必找开发者就能说清"崩在哪一步"。
//
// 三条约束：
//  1. 只写本地配置目录的 `crashes/`，不联网、不上传；
//  2. 报告必须包含版本、Go 版本、系统与日志尾部，避免"只有一行 panic"；
//  3. 上报本身失败也不能影响主流程（全部 best-effort）。

const (
	crashReportDirName   = "crashes"
	crashLogTailLines    = 80
	crashReportMaxKeep   = 30
	crashReportFileGlob  = "crash-*.json"
	crashReportKindPanic = "panic"
	crashReportKindUI    = "frontend"
)

// CrashReport 是一份本地崩溃报告。
type CrashReport struct {
	FileName  string   `json:"fileName"`
	Path      string   `json:"path"`
	Kind      string   `json:"kind"` // panic | frontend | runtime
	Source    string   `json:"source,omitempty"`
	Reason    string   `json:"reason"`
	Stack     string   `json:"stack,omitempty"`
	Version   string   `json:"version"`
	GoVersion string   `json:"goVersion"`
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	CreatedAt string   `json:"createdAt"`
	LogTail   []string `json:"logTail,omitempty"`
	Extra     string   `json:"extra,omitempty"`
}

// logRingBuffer 保存最近若干行日志，供崩溃报告附带"日志尾部"。
type logRingBuffer struct {
	mu    sync.Mutex
	limit int
	lines []string
}

func newLogRingBuffer(limit int) *logRingBuffer {
	if limit <= 0 {
		limit = crashLogTailLines
	}
	return &logRingBuffer{limit: limit}
}

func (b *logRingBuffer) Write(data []byte) (int, error) {
	text := strings.TrimRight(string(data), "\r\n")
	if text == "" {
		return len(data), nil
	}
	b.mu.Lock()
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		b.lines = append(b.lines, line)
	}
	if len(b.lines) > b.limit {
		b.lines = append([]string(nil), b.lines[len(b.lines)-b.limit:]...)
	}
	b.mu.Unlock()
	return len(data), nil
}

func (b *logRingBuffer) Tail() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.lines...)
}

type crashReporter struct {
	mu          sync.Mutex
	dir         string
	buffer      *logRingBuffer
	installOnce sync.Once
	crashFile   *os.File
}

// crashDir 返回崩溃报告目录（配置目录下的 crashes/）。
func (a *App) crashDir() string {
	a.ensureConfigPaths()
	if a.configDir == "" {
		return ""
	}
	return filepath.Join(a.configDir, crashReportDirName)
}

// InstallCrashReporter 安装日志环形缓冲与 Go 运行时崩溃输出。
// 幂等：重复调用不会重复接管日志输出。
func (a *App) InstallCrashReporter() {
	a.crashReporter.installOnce.Do(func() {
		buffer := a.crashReporter.buffer
		if buffer == nil {
			buffer = newLogRingBuffer(crashLogTailLines)
			a.crashReporter.buffer = buffer
		}
		// 保留原有输出（控制台/调试），同时喂给环形缓冲。
		log.SetOutput(io.MultiWriter(os.Stderr, buffer))

		dir := a.crashDir()
		a.crashReporter.mu.Lock()
		a.crashReporter.dir = dir
		a.crashReporter.mu.Unlock()
		if dir == "" {
			return
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			log.Printf("无法创建崩溃报告目录: %v", err)
			return
		}
		// Go 运行时崩溃（例如并发 map 写）会走这里落盘，配合 Windows mdmp 使用。
		if file, err := os.Create(filepath.Join(dir, "runtime-crash.log")); err == nil {
			if err := debug.SetCrashOutput(file, debug.CrashOptions{}); err == nil {
				a.crashReporter.mu.Lock()
				a.crashReporter.crashFile = file
				a.crashReporter.mu.Unlock()
			} else {
				_ = file.Close()
			}
		}
	})
}

// CloseCrashReporter 释放运行时崩溃输出文件句柄（退出前调用，避免文件被占用）。
func (a *App) CloseCrashReporter() {
	a.crashReporter.mu.Lock()
	file := a.crashReporter.crashFile
	a.crashReporter.crashFile = nil
	a.crashReporter.mu.Unlock()
	if file == nil {
		return
	}
	_ = debug.SetCrashOutput(os.Stderr, debug.CrashOptions{})
	_ = file.Close()
}

// runGuarded 执行一段可能 panic 的代码，并把 panic 记成崩溃报告而不是让进程退出。
// 返回 true 表示捕获到了 panic。
func (a *App) runGuarded(source string, fn func()) (recovered bool) {
	defer func() {
		if r := recover(); r != nil {
			recovered = true
			report := CrashReport{
				Kind:      crashReportKindPanic,
				Source:    source,
				Reason:    fmt.Sprintf("%v", r),
				Stack:     string(debug.Stack()),
				Version:   AppVersion,
				GoVersion: runtime.Version(),
				OS:        runtime.GOOS,
				Arch:      runtime.GOARCH,
				CreatedAt: time.Now().Format(time.RFC3339),
			}
			if _, err := a.writeCrashReport(report); err != nil {
				log.Printf("写入崩溃报告失败: %v", err)
			}
			log.Printf("已捕获 panic（%s）: %v；报告已写入本地 crashes 目录", source, r)
		}
	}()
	fn()
	return false
}

// writeCrashReport 落盘一份报告并做数量轮转；返回写入后的报告（含文件名与路径）。
func (a *App) writeCrashReport(report CrashReport) (CrashReport, error) {
	dir := a.crashDir()
	if dir == "" {
		return report, fmt.Errorf("未配置配置目录，无法写入崩溃报告")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return report, err
	}
	if report.CreatedAt == "" {
		report.CreatedAt = time.Now().Format(time.RFC3339)
	}
	if report.Version == "" {
		report.Version = AppVersion
	}
	if report.GoVersion == "" {
		report.GoVersion = runtime.Version()
	}
	if report.OS == "" {
		report.OS = runtime.GOOS
	}
	if report.Arch == "" {
		report.Arch = runtime.GOARCH
	}
	if report.LogTail == nil {
		report.LogTail = a.crashLogTail()
	}

	fileName := fmt.Sprintf(
		"crash-%s-%s.json",
		time.Now().Format("20060102-150405"),
		report.Kind,
	)
	report.FileName = fileName
	report.Path = filepath.Join(dir, fileName)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return report, err
	}
	if err := os.WriteFile(report.Path, data, 0o644); err != nil {
		return report, err
	}
	a.pruneCrashReports(dir)
	return report, nil
}

func (a *App) crashLogTail() []string {
	if a.crashReporter.buffer != nil {
		if tail := a.crashReporter.buffer.Tail(); len(tail) > 0 {
			return tail
		}
	}
	return nil
}

// pruneCrashReports 只保留最近 crashReportMaxKeep 份报告（多的直接删除，
// 崩溃报告本身是诊断产物，体积小且可再生，不必进回收站）。
func (a *App) pruneCrashReports(dir string) {
	paths := listCrashReportFiles(dir)
	if len(paths) <= crashReportMaxKeep {
		return
	}
	for _, path := range paths[:len(paths)-crashReportMaxKeep] {
		if err := os.Remove(path); err != nil {
			log.Printf("清理旧崩溃报告失败: %s: %v", path, err)
		}
	}
}

// listCrashReportFiles 返回按文件名（即时间）升序排列的报告文件。
func listCrashReportFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, "crash-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		paths = append(paths, filepath.Join(dir, name))
	}
	sort.Strings(paths)
	return paths
}

// ReportFrontendError 记录来自前端的未处理异常（window.onerror / unhandledrejection）。
func (a *App) ReportFrontendError(message string, stack string) (string, error) {
	reason := strings.TrimSpace(message)
	if reason == "" {
		reason = "前端未命名错误"
	}
	report, err := a.writeCrashReport(CrashReport{
		Kind:   crashReportKindUI,
		Source: "frontend",
		Reason: reason,
		Stack:  strings.TrimSpace(stack),
	})
	if err != nil {
		return "", err
	}
	return report.FileName, nil
}

// ListCrashReports 返回全部本地崩溃报告（按时间倒序，最新的在前）。
func (a *App) ListCrashReports() ([]CrashReport, error) {
	dir := a.crashDir()
	if dir == "" {
		return []CrashReport{}, nil
	}
	paths := listCrashReportFiles(dir)
	reports := make([]CrashReport, 0, len(paths))
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var report CrashReport
		if err := json.Unmarshal(data, &report); err != nil {
			// 非本格式的文件（例如 runtime-crash.log 之外的第三方文件）跳过。
			continue
		}
		report.FileName = filepath.Base(path)
		report.Path = path
		reports = append(reports, report)
	}
	sort.SliceStable(reports, func(i, j int) bool { return reports[i].CreatedAt > reports[j].CreatedAt })
	return reports, nil
}

// GetCrashReportDirectory 返回崩溃报告目录，供"打开所在文件夹"使用。
func (a *App) GetCrashReportDirectory() string {
	return a.crashDir()
}

// ReadCrashReport 读取一份报告的完整内容。
func (a *App) ReadCrashReport(fileName string) (CrashReport, error) {
	dir := a.crashDir()
	if dir == "" {
		return CrashReport{}, fmt.Errorf("未配置配置目录，无法读取崩溃报告")
	}
	name := filepath.Base(strings.TrimSpace(fileName))
	if !strings.HasPrefix(name, "crash-") || !strings.HasSuffix(name, ".json") {
		return CrashReport{}, fmt.Errorf("无效的崩溃报告文件名: %s", fileName)
	}
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		return CrashReport{}, err
	}
	var report CrashReport
	if err := json.Unmarshal(data, &report); err != nil {
		return CrashReport{}, err
	}
	report.FileName = name
	report.Path = path
	return report, nil
}

// DeleteCrashReport 删除一份报告；只允许删除 crashes 目录下的 crash-*.json。
func (a *App) DeleteCrashReport(fileName string) error {
	dir := a.crashDir()
	if dir == "" {
		return fmt.Errorf("未配置配置目录，无法删除崩溃报告")
	}
	name := filepath.Base(strings.TrimSpace(fileName))
	if !strings.HasPrefix(name, "crash-") || !strings.HasSuffix(name, ".json") {
		return fmt.Errorf("无效的崩溃报告文件名: %s", fileName)
	}
	path := filepath.Join(dir, name)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return nil
}
