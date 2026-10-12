// 批量规范化命名（路线图第 20 项 P2）的计划器 / 执行器测试：
// 计划生成（跳过明细与冲突）、撤销清单落盘、执行前二次校验、只读闸门、忙互斥、幂等往返。
// 全部在 t.TempDir 下构造，不触碰真实 Mod 库。
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newNamingBatchTestApp(t *testing.T) (*App, string) {
	t.Helper()
	gameDir := t.TempDir()
	addonsDir := filepath.Join(gameDir, "left4dead2", "addons")
	if err := os.MkdirAll(filepath.Join(addonsDir, "workshop"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(addonsDir, "disabled"), 0o755); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(gameDir, "config")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &App{rootDir: addonsDir, configDir: configDir}, addonsDir
}

// writeCoachPackVPK 写一个会被识别为「生还者-Coach」的包（与 parser e2e 夹具同形）。
func writeCoachPackVPK(t *testing.T, path string) {
	t.Helper()
	writeTestVPK(t, path, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\t\"addontitle\" \"Airi\"\n}\n"),
		"models/survivors/survivor_coach/coach.mdl":          []byte("mdl"),
		"sound/player/survivor/voice/coach/line01.wav":       []byte("wav"),
		"materials/models/survivors/survivor_coach/skin.vtf": []byte("vtf"),
	})
}

func writeMapPackVPK(t *testing.T, path string) {
	t.Helper()
	writeTestVPK(t, path, map[string][]byte{
		"maps/zm_deadcity.bsp": []byte("bsp"),
	})
}

func namingItemByPath(t *testing.T, plan NamingBatchPlan, path string) NamingBatchItem {
	t.Helper()
	for _, item := range plan.Items {
		if strings.EqualFold(item.Path, path) {
			return item
		}
	}
	t.Fatalf("计划里找不到 %s（%+v）", path, plan.Items)
	return NamingBatchItem{}
}

func TestPlanNamingNormalizationClassifiesAndSkips(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)
	normalized := filepath.Join(addonsDir, "[人物]生还者-Coach-Airi.vpk")
	writeCoachPackVPK(t, normalized)
	mapPack := filepath.Join(addonsDir, "地图包.vpk")
	writeMapPackVPK(t, mapPack)
	workshopFile := filepath.Join(addonsDir, "workshop", "123.vpk")
	writeCoachPackVPK(t, workshopFile)
	notVPK := filepath.Join(addonsDir, "readme.txt")
	if err := os.WriteFile(notVPK, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(addonsDir, "missing.vpk")

	plan, err := app.PlanNamingNormalization([]string{coach, normalized, mapPack, workshopFile, notVPK, missing, coach})
	if err != nil {
		t.Fatalf("PlanNamingNormalization: %v", err)
	}
	if plan.Total != 6 {
		t.Fatalf("重复路径应去重（total=%d）: %+v", plan.Total, plan.Items)
	}
	if plan.Renamable != 2 || plan.Pending != 0 || plan.Skipped != 4 {
		t.Fatalf("计数不对: renamable=%d pending=%d skipped=%d", plan.Renamable, plan.Pending, plan.Skipped)
	}

	coachItem := namingItemByPath(t, plan, coach)
	if !coachItem.Actionable || coachItem.NewName != "生还者-Coach-Airi.vpk" ||
		coachItem.Prefix != "生还者" || coachItem.Subject != "Coach" || coachItem.Stage != "S2" {
		t.Fatalf("生还者包计划项不对: %+v", coachItem)
	}
	if coachItem.ReasonKey != "modals.naming.reason.domain" {
		t.Fatalf("计划项应带依据 key: %+v", coachItem)
	}

	mapItem := namingItemByPath(t, plan, mapPack)
	if !mapItem.Actionable || mapItem.NewName != "地图-地图包.vpk" || mapItem.Stage != "S1" {
		t.Fatalf("地图包计划项不对: %+v", mapItem)
	}

	normalizedItem := namingItemByPath(t, plan, normalized)
	if normalizedItem.Actionable || normalizedItem.SkipReason != "unchanged" || !normalizedItem.AlreadyNormalized {
		t.Fatalf("已规范名应跳过 unchanged: %+v", normalizedItem)
	}

	workshopItem := namingItemByPath(t, plan, workshopFile)
	if workshopItem.Actionable || workshopItem.SkipReason != "workshop" || workshopItem.Location != "workshop" {
		t.Fatalf("工坊文件应跳过 workshop: %+v", workshopItem)
	}

	if item := namingItemByPath(t, plan, notVPK); item.SkipReason != "invalid" {
		t.Fatalf("非 .vpk 应跳过 invalid: %+v", item)
	}
	if item := namingItemByPath(t, plan, missing); item.SkipReason != "invalid" {
		t.Fatalf("缺失文件应跳过 invalid: %+v", item)
	}
}

func TestPlanNamingNormalizationSkipsCollision(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)
	// 目标名已被占用 → 计划阶段就应跳过（不让执行阶段才失败）。
	occupied := filepath.Join(addonsDir, "生还者-Coach-Airi.vpk")
	writeCoachPackVPK(t, occupied)

	plan, err := app.PlanNamingNormalization([]string{coach})
	if err != nil {
		t.Fatalf("PlanNamingNormalization: %v", err)
	}
	item := namingItemByPath(t, plan, coach)
	if item.Actionable || item.SkipReason != "collision" {
		t.Fatalf("目标已存在应跳过 collision: %+v", item)
	}
	if plan.Renamable != 0 || plan.Skipped != 1 {
		t.Fatalf("计数不对: %+v", plan)
	}
}

func TestApplyNamingNormalizationRoundTripAndBackup(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)
	mapPack := filepath.Join(addonsDir, "地图包.vpk")
	writeMapPackVPK(t, mapPack)

	plan, err := app.PlanNamingNormalization([]string{coach, mapPack})
	if err != nil {
		t.Fatalf("PlanNamingNormalization: %v", err)
	}
	requests := make([]NamingBatchRenameRequest, 0, 2)
	for _, item := range plan.Items {
		if !item.Actionable {
			t.Fatalf("应全部可执行: %+v", item)
		}
		requests = append(requests, NamingBatchRenameRequest{Path: item.Path, OldName: item.OldName, NewName: item.NewName})
	}

	result, err := app.ApplyNamingNormalization(requests)
	if err != nil {
		t.Fatalf("ApplyNamingNormalization: %v", err)
	}
	if result.Success != 2 || result.Failed != 0 {
		t.Fatalf("执行结果不对: %+v", result)
	}

	// 撤销清单必须先于写入落盘，且内容可用。
	if result.BackupPath == "" {
		t.Fatalf("缺少撤销清单路径: %+v", result)
	}
	data, err := os.ReadFile(result.BackupPath)
	if err != nil {
		t.Fatalf("读取撤销清单: %v", err)
	}
	var payload struct {
		Version int    `json:"version"`
		Kind    string `json:"kind"`
		Count   int    `json:"count"`
		Items   []struct {
			Path    string `json:"path"`
			OldName string `json:"oldName"`
			NewName string `json:"newName"`
		} `json:"items"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("解析撤销清单: %v", err)
	}
	if payload.Kind != "lytvpk-naming-rename-backup" || payload.Count != 2 || len(payload.Items) != 2 {
		t.Fatalf("撤销清单内容不对: %+v", payload)
	}
	if !strings.HasSuffix(payload.Items[0].NewName, ".vpk") || payload.Items[0].OldName == "" {
		t.Fatalf("撤销清单条目不对: %+v", payload.Items[0])
	}

	// 磁盘上文件已改名，旧名不存在。
	renamedCoach := filepath.Join(addonsDir, "生还者-Coach-Airi.vpk")
	if _, err := os.Stat(renamedCoach); err != nil {
		t.Fatalf("新名文件应存在: %v", err)
	}
	if _, err := os.Stat(coach); !os.IsNotExist(err) {
		t.Fatalf("旧名文件应已不存在: %v", err)
	}

	// 幂等：对改名后的文件重跑计划 → 全部落在「未变化」。
	replan, err := app.PlanNamingNormalization([]string{renamedCoach, filepath.Join(addonsDir, "地图-地图包.vpk")})
	if err != nil {
		t.Fatalf("re-plan: %v", err)
	}
	if replan.Renamable != 0 || replan.Pending != 0 || replan.Skipped != 2 {
		t.Fatalf("规范库重跑应 0 改动: %+v", replan)
	}
	for _, item := range replan.Items {
		if item.SkipReason != "unchanged" {
			t.Fatalf("重跑应全部 unchanged: %+v", item)
		}
	}
}

func TestApplyNamingNormalizationRejectsStaleAndCollidingRequests(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)
	occupied := filepath.Join(addonsDir, "占位.vpk")
	writeMapPackVPK(t, occupied)

	// ① 当前名与请求里的 oldName 不一致（计划过期）。
	result, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Other.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Failed != 1 || result.Success != 0 || !strings.Contains(result.Items[0].Error, "文件已变化") {
		t.Fatalf("过期计划应被拒绝: %+v", result)
	}
	if result.BackupPath != "" {
		t.Fatalf("全量校验失败时不应写撤销清单: %+v", result)
	}
	if _, err := os.Stat(coach); err != nil {
		t.Fatalf("文件不应被改动: %v", err)
	}

	// ② 目标已存在。
	result2, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "占位.vpk"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result2.Failed != 1 || !strings.Contains(result2.Items[0].Error, "目标文件已存在") {
		t.Fatalf("目标冲突应被拒绝: %+v", result2)
	}

	// ③ 重复路径。
	result3, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result3.Failed != 2 {
		t.Fatalf("重复项目应整组拒绝: %+v", result3)
	}
	for _, item := range result3.Items {
		if !strings.Contains(item.Error, "重复") {
			t.Fatalf("错误说明不对: %+v", item)
		}
	}
}

func TestApplyNamingNormalizationReadonlyGate(t *testing.T) {
	t.Setenv(readonlyLibraryEnv, "1")
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)

	_, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	if err == nil || !strings.Contains(err.Error(), "沙箱只读模式") {
		t.Fatalf("只读闸门应拒绝批量命名: %v", err)
	}
	if _, statErr := os.Stat(coach); statErr != nil {
		t.Fatalf("只读模式下文件不应被改动: %v", statErr)
	}
}

func TestApplyNamingNormalizationBackupFailureAborts(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)
	// 配置目录不可用 → 撤销清单写不出去 → 整体中止、不碰文件。
	app.configDir = ""
	app.configPath = ""

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)

	_, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	if err == nil || !strings.Contains(err.Error(), "写入撤销清单失败") {
		t.Fatalf("撤销清单失败应中止: %v", err)
	}
	if _, statErr := os.Stat(coach); statErr != nil {
		t.Fatalf("中止后文件不应被改动: %v", statErr)
	}
}

func TestApplyNamingNormalizationRenamesSidecars(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)

	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)
	if err := os.WriteFile(filepath.Join(addonsDir, "Airi.url"),
		[]byte("[InternetShortcut]\nURL=steam://url/CommunityFilePage/123\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(addonsDir, "Airi.png"), []byte("png"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.Success != 1 || result.Failed != 0 {
		t.Fatalf("执行失败: %+v", result)
	}

	// 侧车联动：工坊快捷方式（.url）与同图名（.png）要随主文件一起改名。
	for _, ext := range []string{".vpk", ".url", ".png"} {
		if _, err := os.Stat(filepath.Join(addonsDir, "生还者-Coach-Airi"+ext)); err != nil {
			t.Fatalf("侧车 %s 应随主文件改名: %v", ext, err)
		}
		if _, err := os.Stat(filepath.Join(addonsDir, "Airi"+ext)); !os.IsNotExist(err) {
			t.Fatalf("旧侧车 %s 应已不存在", ext)
		}
	}
}

func TestApplyNamingNormalizationBusyMutex(t *testing.T) {
	app, addonsDir := newNamingBatchTestApp(t)
	coach := filepath.Join(addonsDir, "Airi.vpk")
	writeCoachPackVPK(t, coach)

	app.namingBatchMu.Lock()
	_, err := app.ApplyNamingNormalization([]NamingBatchRenameRequest{
		{Path: coach, OldName: "Airi.vpk", NewName: "生还者-Coach-Airi.vpk"},
	})
	app.namingBatchMu.Unlock()
	if err == nil || !strings.Contains(err.Error(), "已有批量命名任务在执行") {
		t.Fatalf("忙互斥应生效: %v", err)
	}
}

func TestPlanNamingNormalizationRequiresPaths(t *testing.T) {
	app, _ := newNamingBatchTestApp(t)
	if _, err := app.PlanNamingNormalization(nil); err == nil {
		t.Fatalf("空路径应报错")
	}
	if _, err := app.PlanNamingNormalization([]string{"  ", ""}); err == nil {
		t.Fatalf("空白路径应报错")
	}
}
