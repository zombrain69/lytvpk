package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// 扫描时"同名封面图 / .meta"改成从目录项里取（不再每个 VPK stat 5 次）之后，
// 语义必须一字不变：新增 / 更换 / 删除封面图都要让对应 Mod 的缓存失效。
//
// 真机背景：2904 个 Mod × 5 次 stat ≈ 14,500 次系统调用，实测占整轮扫描的 60%
// （908ms → 362ms）。省掉它们的前提是"目录项里已经有的信息等价于 stat 的结果"。
func TestScanPicksUpSidecarImageChanges(t *testing.T) {
	a, addonsDir := newPriorityTestApp(t)
	vpkPath := filepath.Join(addonsDir, "a.vpk")

	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("首次扫描失败: %v", err)
	}
	withoutImage := a.mustCachedVPKFile(t, vpkPath)
	if withoutImage.PreviewRevision == "" {
		t.Fatalf("扫描后应写入 PreviewRevision")
	}

	// ① 加一张同名封面图：缓存必须失效并带上新的预览版本。
	imagePath := filepath.Join(addonsDir, "a.jpg")
	if err := os.WriteFile(imagePath, []byte("fake-image"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 文件系统的 mtime 精度有限：显式往前推一点，避免"同一秒"导致判定不出变化。
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(imagePath, future, future); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("加封面图后扫描失败: %v", err)
	}
	withImage := a.mustCachedVPKFile(t, vpkPath)
	if withImage.PreviewRevision == withoutImage.PreviewRevision {
		t.Fatalf(
			"新增同名封面图后必须更新缓存（旧=%s 新=%s）",
			withoutImage.PreviewRevision,
			withImage.PreviewRevision,
		)
	}

	// ② 换一张（mtime 变化）：还必须再次失效。
	later := future.Add(5 * time.Second)
	if err := os.WriteFile(imagePath, []byte("another-image"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(imagePath, later, later); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("换封面图后扫描失败: %v", err)
	}
	replaced := a.mustCachedVPKFile(t, vpkPath)
	if replaced.PreviewRevision == withImage.PreviewRevision {
		t.Fatalf("更换封面图后必须更新缓存（旧=%s）", withImage.PreviewRevision)
	}

	// ③ 删掉封面图：回到"没有封面"的状态，同样要失效。
	if err := os.Remove(imagePath); err != nil {
		t.Fatal(err)
	}
	if err := a.ScanVPKFiles(); err != nil {
		t.Fatalf("删除封面图后扫描失败: %v", err)
	}
	removed := a.mustCachedVPKFile(t, vpkPath)
	if removed.PreviewRevision != withoutImage.PreviewRevision {
		t.Fatalf(
			"删掉封面图后应回到最初的状态（最初=%s 现在=%s）",
			withoutImage.PreviewRevision,
			removed.PreviewRevision,
		)
	}
}

// 索引路径（扫描）与逐键 stat 路径（单文件刷新传 nil）必须算出同一个结果。
func TestSidecarIndexMatchesStatFallback(t *testing.T) {
	_, addonsDir := newPriorityTestApp(t)
	imagePath := filepath.Join(addonsDir, "a.jpg")
	if err := os.WriteFile(imagePath, []byte("fake-image"), 0o644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(imagePath, future, future); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(addonsDir, "a.vpk")

	indexApp := &App{rootDir: addonsDir}
	sidecars := make(map[string]sidecarInfo)
	paths := make([]string, 0)
	if err := indexApp.scanRootDirectory(addonsDir, &paths, sidecars); err != nil {
		t.Fatal(err)
	}
	if len(sidecars) == 0 {
		t.Fatalf("侧车索引应包含 a.jpg")
	}
	indexApp.processVPKFileWithCache(vpkPath, sidecars)
	fromIndex := indexApp.mustCachedVPKFile(t, vpkPath)

	statApp := &App{rootDir: addonsDir}
	statApp.processVPKFileWithCache(vpkPath, nil)
	fromStat := statApp.mustCachedVPKFile(t, vpkPath)

	if fromIndex.PreviewRevision != fromStat.PreviewRevision {
		t.Fatalf(
			"索引路径与 stat 兜底路径的预览版本必须一致（索引=%s stat=%s）",
			fromIndex.PreviewRevision,
			fromStat.PreviewRevision,
		)
	}
}
