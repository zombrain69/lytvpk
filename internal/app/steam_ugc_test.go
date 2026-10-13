package app

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- 测试替身

// fakeSteamUGCSession 是 steamUGCSession 的假实现：只动内存集合，不碰真实 Steam。
type fakeSteamUGCSession struct {
	subscribed map[uint64]bool
	unsubCalls []uint64
	subCalls   []uint64
	failIDs    map[uint64]error
	closed     bool
}

func newFakeSteamUGCSession(subscribed ...uint64) *fakeSteamUGCSession {
	set := make(map[uint64]bool, len(subscribed))
	for _, id := range subscribed {
		set[id] = true
	}
	return &fakeSteamUGCSession{subscribed: set, failIDs: map[uint64]error{}}
}

func (f *fakeSteamUGCSession) SubscribedItems() ([]uint64, error) {
	out := make([]uint64, 0, len(f.subscribed))
	for id, ok := range f.subscribed {
		if ok {
			out = append(out, id)
		}
	}
	return out, nil
}

func (f *fakeSteamUGCSession) SetSubscribed(id uint64, want bool) error {
	if err, ok := f.failIDs[id]; ok {
		return err
	}
	if want {
		f.subCalls = append(f.subCalls, id)
		f.subscribed[id] = true
	} else {
		f.unsubCalls = append(f.unsubCalls, id)
		delete(f.subscribed, id)
	}
	return nil
}

func (f *fakeSteamUGCSession) Close() { f.closed = true }

// useFakeSteamUGCSession 替换会话工厂并返回恢复函数。
func useFakeSteamUGCSession(t *testing.T, session *fakeSteamUGCSession, recordPath *string) {
	t.Helper()
	previous := openSteamUGCSession
	openSteamUGCSession = func(dllPath string) (steamUGCSession, error) {
		if recordPath != nil {
			*recordPath = dllPath
		}
		return session, nil
	}
	t.Cleanup(func() { openSteamUGCSession = previous })
}

// useFakeDetector 替换自动检测器并返回恢复函数。
func useFakeDetector(t *testing.T, candidates []SteamAPIDllCandidate) {
	t.Helper()
	previous := steamUGCDetector
	steamUGCDetector = func() []SteamAPIDllCandidate { return candidates }
	t.Cleanup(func() { steamUGCDetector = previous })
}

// newSteamUGCTestApp 构造一个带临时配置目录的 App（零值 App 的其余字段对本组方法够用）。
func newSteamUGCTestApp(t *testing.T) *App {
	t.Helper()
	dir := t.TempDir()
	return &App{
		configDir:  dir,
		configPath: filepath.Join(dir, "config.json"),
	}
}

// writeTempDLL 造一个可用的假 DLL 文件（只校验存在性与扩展名，不会真的加载）。
func writeTempDLL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "steam_api64.dll")
	if err := os.WriteFile(path, []byte("fake-dll"), 0o644); err != nil {
		t.Fatalf("写入假 DLL 失败: %v", err)
	}
	return path
}

// ---------------------------------------------------------------- ID 规范化

func TestNormalizeSteamUGCIDList(t *testing.T) {
	targets, err := normalizeSteamUGCIDList([]string{"3734012655", " 42 ", "3734012655", ""})
	if err != nil {
		t.Fatalf("合法输入不应报错: %v", err)
	}
	if len(targets) != 2 || targets[0] != 3734012655 || targets[1] != 42 {
		t.Fatalf("去重/去空白后应保持顺序: %v", targets)
	}

	for name, ids := range map[string][]string{
		"空列表":  {},
		"全空白":  {"", "  "},
		"无效字符": {"abc"},
		"零值":   {"0"},
		"负数":   {"-5"},
		"混合无效": {"1", "2x"},
	} {
		if _, err := normalizeSteamUGCIDList(ids); err == nil {
			t.Errorf("%s 应报错", name)
		}
	}
}

// ---------------------------------------------------------------- 自动检测

func TestDetectSteamAPIDllCandidates(t *testing.T) {
	root := t.TempDir()

	// 主库：GameDirect 直接带 DLL；GameNested 在子目录里带；GameMissing 没有。
	common := filepath.Join(root, "steamapps", "common")
	writeFileT(t, filepath.Join(common, "GameDirect", steamAPIDllFileName), "dll")
	writeFileT(t, filepath.Join(common, "GameNested", "bin", steamAPIDllFileName), "dll")
	if err := os.MkdirAll(filepath.Join(common, "GameMissing", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	// 第二库（libraryfolders.vdf 指向）里再放一个。
	otherLibrary := t.TempDir()
	otherCommon := filepath.Join(otherLibrary, "steamapps", "common")
	writeFileT(t, filepath.Join(otherCommon, "AnotherGame", steamAPIDllFileName), "dll")

	libraryFolders := `"libraryfolders"
{
	"0" { "path" "` + strings.ReplaceAll(root, `\`, `\\`) + `" }
	"1" { "path" "` + strings.ReplaceAll(otherLibrary, `\`, `\\`) + `" }
}`

	readFile := func(path string) ([]byte, error) {
		if strings.HasSuffix(strings.ToLower(path), "libraryfolders.vdf") {
			return []byte(libraryFolders), nil
		}
		return nil, os.ErrNotExist
	}

	candidates := detectSteamAPIDllCandidates(root, readFile, os.ReadDir, fileExists)
	if len(candidates) != 3 {
		t.Fatalf("应找到 3 份候选，实际 %d：%+v", len(candidates), candidates)
	}
	// 顺序：安装目录（GameDirect → GameNested，按目录名排序）→ 第二库（AnotherGame）。
	if candidates[0].Game != "GameDirect" || !strings.HasSuffix(candidates[0].Path, filepath.Join("GameDirect", steamAPIDllFileName)) {
		t.Fatalf("首个候选应为 GameDirect 的浅层 DLL：%+v", candidates[0])
	}
	if candidates[1].Game != "GameNested" || !strings.HasSuffix(candidates[1].Path, filepath.Join("bin", steamAPIDllFileName)) {
		t.Fatalf("第二个候选应为 GameNested 的嵌套 DLL：%+v", candidates[1])
	}
	if candidates[2].Game != "AnotherGame" {
		t.Fatalf("第三个候选应来自第二库：%+v", candidates[2])
	}

	if got := detectSteamAPIDllCandidates("", readFile, os.ReadDir, fileExists); got != nil {
		t.Fatalf("空安装目录应返回 nil，实际 %+v", got)
	}
}

func TestFindSteamAPIDllInDirDepthAndOrder(t *testing.T) {
	// 深度超过上限时不命中；浅层与深层同时存在时取浅层。
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c", "d")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(deep, steamAPIDllFileName), "dll")
	if got := findSteamAPIDllInDir(root, os.ReadDir, fileExists); got != "" {
		t.Fatalf("深度超过 %d 层不应命中：%s", steamAPIDllMaxDepth, got)
	}

	writeFileT(t, filepath.Join(root, "a", "b", steamAPIDllFileName), "dll")
	got := findSteamAPIDllInDir(root, os.ReadDir, fileExists)
	if !strings.HasSuffix(got, filepath.Join("a", "b", steamAPIDllFileName)) {
		t.Fatalf("应命中深度 2 的副本：%s", got)
	}
}

// ---------------------------------------------------------------- 状态与设置

func TestSteamUGCStatusAutoDetectionAndCache(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)

	// 自动检测现在会对候选做一次只读可用性探测：注入假会话让探测通过。
	recorded := ""
	useFakeSteamUGCSession(t, newFakeSteamUGCSession(), &recorded)

	callCount := 0
	previous := steamUGCDetector
	steamUGCDetector = func() []SteamAPIDllCandidate {
		callCount++
		return []SteamAPIDllCandidate{{Path: dll, Game: "TestGame"}}
	}
	t.Cleanup(func() { steamUGCDetector = previous })

	status := a.GetSteamUGCStatus()
	if !status.Available || status.Source != "auto" || status.Path != dll {
		t.Fatalf("自动检测结果不对：%+v", status)
	}
	if recorded != dll {
		t.Fatalf("自动检测应通过可用性探测打开一次会话，实际 %q", recorded)
	}
	// 第二次调用应命中缓存（检测器只跑一次）。
	a.GetSteamUGCStatus()
	if callCount != 1 {
		t.Fatalf("检测结果应被缓存，检测器调用次数=%d", callCount)
	}
}

func TestSteamUGCStatusNoDll(t *testing.T) {
	a := newSteamUGCTestApp(t)
	useFakeDetector(t, nil)

	status := a.GetSteamUGCStatus()
	if status.Available || status.Source != "none" || status.Hint != "no-dll" {
		t.Fatalf("没找到 DLL 的状态不对：%+v", status)
	}
}

func TestSetSteamAPIDllPathValidatesAndPersists(t *testing.T) {
	a := newSteamUGCTestApp(t)

	if _, err := a.SetSteamAPIDllPath(filepath.Join(t.TempDir(), "missing.dll")); err == nil {
		t.Fatal("不存在的文件应报错")
	}
	notDll := filepath.Join(t.TempDir(), "steam_api64.txt")
	writeFileT(t, notDll, "x")
	if _, err := a.SetSteamAPIDllPath(notDll); err == nil {
		t.Fatal("非 .dll 文件应报错")
	}

	dll := writeTempDLL(t)
	status, err := a.SetSteamAPIDllPath(dll)
	if err != nil {
		t.Fatalf("设置有效 DLL 失败: %v", err)
	}
	if !status.Available || status.Source != "user" || status.Path != dll {
		t.Fatalf("设置后的状态不对：%+v", status)
	}

	// 写回配置：config.json 中应带上该字段。
	data, err := os.ReadFile(a.configPath)
	if err != nil {
		t.Fatalf("配置文件未写出: %v", err)
	}
	var config ConfigFile
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("配置文件解析失败: %v", err)
	}
	if config.SteamAPIDllPath != dll {
		t.Fatalf("配置里应保存路径，实际 %q", config.SteamAPIDllPath)
	}

	// 清除后回到"未设置"，并重新触发自动检测。
	useFakeDetector(t, nil)
	status, err = a.SetSteamAPIDllPath("")
	if err != nil {
		t.Fatalf("清除路径失败: %v", err)
	}
	if status.Source != "none" || status.Available {
		t.Fatalf("清除后应为未设置态：%+v", status)
	}
}

// ---------------------------------------------------------------- 批量执行

func TestUnsubscribeBatchFlow(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}

	session := newFakeSteamUGCSession(111, 222)
	useFakeSteamUGCSession(t, session, nil)

	result, err := a.UnsubscribeSteamWorkshopItems([]string{"111", "222", "333"})
	if err != nil {
		t.Fatalf("批量退订失败: %v", err)
	}
	if result.Action != steamUGCBatchActionUnsubscribe || result.Total != 3 {
		t.Fatalf("汇总不对：%+v", result)
	}
	if result.Succeeded != 2 || result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("计数不对：%+v", result)
	}
	if len(session.unsubCalls) != 2 || session.unsubCalls[0] != 111 || session.unsubCalls[1] != 222 {
		t.Fatalf("退订调用不对：%v", session.unsubCalls)
	}
	if !session.closed {
		t.Fatal("会话应被关闭")
	}
	// 333 不在订阅列表 → 跳过且说明原因；结果顺序与原顺序一致。
	last := result.Results[2]
	if !last.Skipped || !last.OK || last.PublishedFileID != "333" || last.Detail == "" {
		t.Fatalf("跳过的条目结果不对：%+v", last)
	}

	// 回滚清单：写在配置目录、含全部所选 ID。
	if result.BackupPath == "" {
		t.Fatal("退订应导出回滚清单")
	}
	if filepath.Dir(result.BackupPath) != a.configDir {
		t.Fatalf("回滚清单应落在配置目录：%s", result.BackupPath)
	}
	backupData, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatalf("回滚清单不可读: %v", err)
	}
	var backup unsubscribeBackupPayload
	if err := json.Unmarshal(backupData, &backup); err != nil {
		t.Fatalf("回滚清单解析失败: %v", err)
	}
	if backup.Kind != "lytvpk-workshop-unsubscribe-backup" || backup.Count != 3 || len(backup.Items) != 3 {
		t.Fatalf("回滚清单内容不对：%+v", backup)
	}
	if backup.Items[0].PublishedFileID != "111" {
		t.Fatalf("回滚清单顺序不对：%+v", backup.Items)
	}
}

func TestUnsubscribeBatchAbortsWhenBackupFails(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}

	// 把配置目录指向一个"文件下面"，写清单必然失败。
	blocker := filepath.Join(t.TempDir(), "blocker")
	writeFileT(t, blocker, "x")
	a.configDir = filepath.Join(blocker, "sub")

	session := newFakeSteamUGCSession(111)
	opened := false
	previous := openSteamUGCSession
	openSteamUGCSession = func(string) (steamUGCSession, error) {
		opened = true
		return session, nil
	}
	t.Cleanup(func() { openSteamUGCSession = previous })

	if _, err := a.UnsubscribeSteamWorkshopItems([]string{"111"}); err == nil {
		t.Fatal("回滚清单写不下去时应报错中止")
	}
	if opened {
		t.Fatal("回滚清单失败时不应打开 Steam 会话")
	}
	if len(session.unsubCalls) != 0 {
		t.Fatal("回滚清单失败时不应执行任何退订")
	}
}

func TestUnsubscribeBatchRecordsPerItemFailure(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}

	session := newFakeSteamUGCSession(111, 222)
	session.failIDs[222] = errors.New("等待 Steam 确认超时")
	useFakeSteamUGCSession(t, session, nil)

	result, err := a.UnsubscribeSteamWorkshopItems([]string{"111", "222"})
	if err != nil {
		t.Fatalf("单项失败不应整体报错: %v", err)
	}
	if result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("计数不对：%+v", result)
	}
	failed := result.Results[1]
	if failed.OK || failed.Detail == "" {
		t.Fatalf("失败条目应带原因：%+v", failed)
	}
	// 失败后该 ID 仍视为"订阅中"（集合里还在），语义上与真机一致。
	if !session.subscribed[222] {
		t.Fatal("失败条目不应从订阅集合移除")
	}
}

func TestSubscribeBatchSkipsAlreadySubscribed(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}

	session := newFakeSteamUGCSession(111)
	useFakeSteamUGCSession(t, session, nil)

	result, err := a.SubscribeSteamWorkshopItems([]string{"111", "222"})
	if err != nil {
		t.Fatalf("批量重订失败: %v", err)
	}
	if result.Succeeded != 1 || result.Skipped != 1 {
		t.Fatalf("计数不对：%+v", result)
	}
	if result.Results[0].Detail != "已在订阅列表中" {
		t.Fatalf("已订阅条目应跳过：%+v", result.Results[0])
	}
	if len(session.subCalls) != 1 || session.subCalls[0] != 222 {
		t.Fatalf("订阅调用不对：%v", session.subCalls)
	}
	if result.BackupPath != "" {
		t.Fatal("重订方向不应导出回滚清单")
	}
}

func TestBatchRequiresAvailableDll(t *testing.T) {
	a := newSteamUGCTestApp(t)
	useFakeDetector(t, nil)

	if _, err := a.UnsubscribeSteamWorkshopItems([]string{"111"}); err == nil {
		t.Fatal("没有 DLL 时应报错")
	}

	// 自定义路径失效（文件被删）也应拒绝执行。
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dll); err != nil {
		t.Fatal(err)
	}
	_, err := a.UnsubscribeSteamWorkshopItems([]string{"111"})
	if err == nil || !strings.Contains(err.Error(), "失效") {
		t.Fatalf("失效路径应给出明确错误，实际: %v", err)
	}
}

func TestBatchBusyGuard(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}
	a.steamUGCBusy = true
	defer func() { a.steamUGCBusy = false }()

	if _, err := a.UnsubscribeSteamWorkshopItems([]string{"111"}); err == nil {
		t.Fatal("已有批量执行时应拒绝并发")
	}
}

func TestBatchPassesResolvedDllPathToSession(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dll := writeTempDLL(t)
	if _, err := a.SetSteamAPIDllPath(dll); err != nil {
		t.Fatal(err)
	}

	session := newFakeSteamUGCSession(111)
	recordedPath := ""
	useFakeSteamUGCSession(t, session, &recordedPath)

	if _, err := a.UnsubscribeSteamWorkshopItems([]string{"111"}); err != nil {
		t.Fatal(err)
	}
	if recordedPath != dll {
		t.Fatalf("会话应以解析出的路径打开，实际 %q", recordedPath)
	}
}

// 真机复验场景：候选里前几份"存在但不可用"（模拟器改版 / 损坏 / 版本过旧），
// 自动检测必须继续逐份探测并选中第一份真正可用的；全部不可用时如实显示未就绪。
func TestSteamUGCAutoDetectSkipsUnusableCandidates(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dud1 := writeTempDLL(t)
	dud2 := writeTempDLL(t)
	good := writeTempDLL(t)
	useFakeDetector(t, []SteamAPIDllCandidate{
		{Path: dud1, Game: "Dud1"},
		{Path: dud2, Game: "Dud2"},
		{Path: good, Game: "Good"},
	})

	previous := openSteamUGCSession
	var tried []string
	openSteamUGCSession = func(dllPath string) (steamUGCSession, error) {
		tried = append(tried, dllPath)
		if dllPath == good {
			return newFakeSteamUGCSession(), nil
		}
		return nil, errors.New("DLL 里没有可用的 ISteamUGC 接口（文件损坏或版本过旧？）")
	}
	t.Cleanup(func() { openSteamUGCSession = previous })

	status := a.GetSteamUGCStatus()
	if !status.Available || status.Source != "auto" || status.Path != good {
		t.Fatalf("应跳过不可用副本选中可用的一份：%+v（探测顺序 %v）", status, tried)
	}
	if len(tried) != 3 || tried[0] != dud1 || tried[1] != dud2 || tried[2] != good {
		t.Fatalf("应逐份探测直到命中：%v", tried)
	}
	// 缓存：第二次查询不再探测。
	tried = nil
	a.GetSteamUGCStatus()
	if len(tried) != 0 {
		t.Fatalf("二次查询不应重复探测：%v", tried)
	}
}

func TestSteamUGCAutoDetectAllCandidatesUnusable(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dud := writeTempDLL(t)
	useFakeDetector(t, []SteamAPIDllCandidate{{Path: dud, Game: "Dud"}})

	previous := openSteamUGCSession
	openSteamUGCSession = func(string) (steamUGCSession, error) {
		return nil, errors.New("可以打开但不是 Steam DLL")
	}
	t.Cleanup(func() { openSteamUGCSession = previous })

	status := a.GetSteamUGCStatus()
	if status.Available || status.Source != "none" || status.Hint != "no-dll" {
		t.Fatalf("全部不可用时应显示未就绪：%+v", status)
	}
}

// 「检测本机 DLL」方法：可用副本被提到第一位；全部不可用时明确报错。
func TestDetectSteamAPIDllCandidatesPicksWorking(t *testing.T) {
	a := newSteamUGCTestApp(t)
	dud := writeTempDLL(t)
	good := writeTempDLL(t)
	useFakeDetector(t, []SteamAPIDllCandidate{
		{Path: dud, Game: "Dud"},
		{Path: good, Game: "Good"},
	})

	previous := openSteamUGCSession
	openSteamUGCSession = func(dllPath string) (steamUGCSession, error) {
		if dllPath == good {
			return newFakeSteamUGCSession(), nil
		}
		return nil, errors.New("不可用")
	}
	t.Cleanup(func() { openSteamUGCSession = previous })

	list, err := a.DetectSteamAPIDllCandidates()
	if err != nil {
		t.Fatalf("应返回可用候选列表: %v", err)
	}
	if len(list) != 2 || list[0].Path != good || list[1].Path != dud {
		t.Fatalf("可用副本应被提到第一位：%+v", list)
	}

	// 全部不可用：明确报错（新的 App 避免命中缓存）。
	a2 := newSteamUGCTestApp(t)
	useFakeDetector(t, []SteamAPIDllCandidate{{Path: dud, Game: "Dud"}})
	if _, err := a2.DetectSteamAPIDllCandidates(); err == nil {
		t.Fatal("全部不可用时应报错")
	}
}

// writeFileT 是测试里的小工具：写文件（自动建目录）。
func writeFileT(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
