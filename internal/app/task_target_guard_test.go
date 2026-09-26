package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 对齐 FireAxe 的 ValidRef / ValidTaskCreator
// （ValidRef.cs:14-38、ValidTaskCreator.cs:22-36、IValidityExtensions.cs:8-42）：
// 异步任务开始时捕获目标，写回结果之前复查目标是否还有效；
// 目标已经不在原处或已被换掉时，结果必须被丢弃，而不是落到旧路径上。
func TestModTaskTargetGuardDetectsInvalidTargets(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "a.vpk")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}

	target, ok := captureModTaskTarget(path)
	if !ok {
		t.Fatal("刚写入的文件应该可以捕获")
	}
	if !target.stillValid() {
		t.Fatal("没有改动时目标应当仍然有效")
	}

	// 内容被换掉（例如重新下载）后，基于旧文件算出的结果不再适用。
	if err := os.WriteFile(path, []byte("second-and-much-longer"), 0o644); err != nil {
		t.Fatal(err)
	}
	if target.stillValid() {
		t.Fatal("文件被替换后旧目标应当失效")
	}

	// 目录不是受管的 Mod 文件。
	if err := os.MkdirAll(filepath.Join(root, "dir.vpk"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, ok := captureModTaskTarget(filepath.Join(root, "dir.vpk")); ok {
		t.Fatal("目录不该被当成有效目标")
	}

	// 文件被移走后失效。
	target, ok = captureModTaskTarget(path)
	if !ok {
		t.Fatal("重新捕获应当成功")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if target.stillValid() {
		t.Fatal("文件被删除后旧目标应当失效")
	}
}

// 更新检测的每个异步结果都要复查目标：真机上"检测还在路上，用户把 Mod 挪走"
// 会让旧实现把时间戳写进已经不存在的路径（留下孤立 .meta）。
func newUpdateCheckFixture(t *testing.T, workshopRel string) (*App, string, string) {
	t.Helper()
	root := t.TempDir()
	vpkPath := filepath.Join(root, filepath.FromSlash(workshopRel))
	if err := os.MkdirAll(filepath.Dir(vpkPath), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestVPK(t, vpkPath, map[string][]byte{"materials/fixture.vtf": {1}})
	id := strings.TrimSuffix(filepath.Base(vpkPath), filepath.Ext(vpkPath))
	if err := saveWorkshopMeta(vpkPath, &WorkshopMeta{
		WorkshopID:   id,
		Title:        "夹具作品",
		DownloadedAt: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	a := newProfileTestApp(t, root)
	a.workshopUpdateCheckEnabled = true
	a.workshopMetaEnabled = true
	resetModUpdateCheckState()
	t.Cleanup(resetModUpdateCheckState)
	a.vpkCache.Store(vpkPath, &VPKFileCache{File: VPKFile{
		Path:       vpkPath,
		Name:       filepath.Base(vpkPath),
		Location:   "workshop",
		WorkshopID: id,
	}})
	return a, vpkPath, id
}

func TestUpdateCheckSkipsMetaWriteWhenTargetMovedMidFlight(t *testing.T) {
	a, vpkPath, id := newUpdateCheckFixture(t, "workshop/123.vpk")
	disabledPath := filepath.Join(filepath.Dir(filepath.Dir(vpkPath)), "disabled", filepath.Base(vpkPath))
	if err := os.MkdirAll(filepath.Dir(disabledPath), 0o755); err != nil {
		t.Fatal(err)
	}

	// 检测请求还在飞行中，用户把 Mod 挪进了 disabled。
	a.workshopItemDetailFetcher = func(requestedID string) (WorkshopItemDetail, error) {
		if requestedID != id {
			t.Errorf("unexpected workshop id %q", requestedID)
		}
		if err := os.Rename(vpkPath, disabledPath); err != nil {
			return WorkshopItemDetail{}, err
		}
		return WorkshopItemDetail{PublishedFileId: id, TimeUpdated: float64(time.Now().Unix())}, nil
	}

	result := a.CheckModUpdates()
	if result.NewDetected != 0 {
		t.Fatalf("目标已经不在原处，不该记成检测到更新：%#v", result)
	}
	if result.SkippedStale != 1 {
		t.Fatalf("expected exactly one skipped stale target, got %#v", result)
	}

	// 不能在新位置凭空生成 .meta，也不能把时间戳写回旧位置。
	disabledMetaPath := filepath.Join(filepath.Dir(disabledPath), strings.TrimSuffix(filepath.Base(vpkPath), filepath.Ext(vpkPath))+".meta")
	if _, err := os.Stat(disabledMetaPath); !os.IsNotExist(err) {
		t.Fatalf("disabled 下不该出现新的 .meta：err=%v", err)
	}
	meta, err := LoadWorkshopMeta(vpkPath)
	if err != nil {
		t.Fatalf("load meta: %v", err)
	}
	if meta == nil {
		t.Fatal("夹具的 .meta 不该被删除")
	}
	if meta.TimeUpdated != "" {
		t.Fatalf("目标已经失效，不该再写入更新时间：%q", meta.TimeUpdated)
	}
}

func TestUpdateCheckWritesMetaWhenTargetUnchanged(t *testing.T) {
	a, vpkPath, id := newUpdateCheckFixture(t, "workshop/123.vpk")
	a.workshopItemDetailFetcher = func(requestedID string) (WorkshopItemDetail, error) {
		if requestedID != id {
			t.Errorf("unexpected workshop id %q", requestedID)
		}
		return WorkshopItemDetail{PublishedFileId: id, TimeUpdated: float64(time.Now().Unix())}, nil
	}

	result := a.CheckModUpdates()
	if result.NewDetected != 1 || result.SkippedStale != 0 {
		t.Fatalf("目标没动时应记成检测到 1 个更新：%#v", result)
	}
	meta, err := LoadWorkshopMeta(vpkPath)
	if err != nil || meta == nil {
		t.Fatalf("load meta: meta=%#v err=%v", meta, err)
	}
	if meta.TimeUpdated == "" {
		t.Fatal("目标有效时应把更新时间写回本地记录")
	}
}
