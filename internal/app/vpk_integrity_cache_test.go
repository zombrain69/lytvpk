package app

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
	"time"

	"l4d2-manager-next/pkg/valve/vpk"
	"vpk-manager/internal/parser"
)

// 完整性校验的记忆化：同一个包被反复启用/禁用时不能反复读盘。
// 键是 (路径, 大小, 修改时间)，任何一项变了都必须重新校验。

type fakeFileInfo struct {
	size    int64
	modTime time.Time
}

func (f fakeFileInfo) Name() string       { return "x.vpk" }
func (f fakeFileInfo) Size() int64        { return f.size }
func (f fakeFileInfo) Mode() os.FileMode  { return 0o644 }
func (f fakeFileInfo) ModTime() time.Time { return f.modTime }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }

func TestVPKIntegrityCacheReusesResultUntilFileChanges(t *testing.T) {
	var cache vpkIntegrityCache
	base := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	info := fakeFileInfo{size: 128, modTime: base}
	report := parser.VPKIntegrityReport{
		Path:          `C:\addons\a.vpk`,
		Valid:         true,
		ScanMode:      parser.VPKIntegrityScanIndex,
		TotalFiles:    2,
		VerifiedFiles: 2,
		Issues:        []parser.VPKIntegrityIssue{{Code: "sample", Message: "留个切片验证深拷贝"}},
	}
	cache.store(`C:\addons\a.vpk`, info, report)

	hit, ok := cache.lookup(`C:\addons\a.vpk`, info)
	if !ok {
		t.Fatal("同一个文件应该命中缓存")
	}
	if hit.TotalFiles != report.TotalFiles || len(hit.Issues) != 1 {
		t.Fatalf("缓存内容不完整: %+v", hit)
	}

	// 返回的是副本：调用方改动不能污染缓存。
	hit.Issues[0].Message = "被改过"
	hit.Issues = append(hit.Issues, parser.VPKIntegrityIssue{Code: "extra"})
	again, ok := cache.lookup(`C:\addons\a.vpk`, info)
	if !ok || len(again.Issues) != 1 || again.Issues[0].Message != "留个切片验证深拷贝" {
		t.Fatalf("缓存被调用方改到了: %+v", again)
	}

	// 大小变了 → 失效
	if _, ok := cache.lookup(`C:\addons\a.vpk`, fakeFileInfo{size: 129, modTime: base}); ok {
		t.Fatal("大小变化后不该命中缓存")
	}
	// 修改时间变了 → 失效
	if _, ok := cache.lookup(`C:\addons\a.vpk`, fakeFileInfo{size: 128, modTime: base.Add(time.Second)}); ok {
		t.Fatal("修改时间变化后不该命中缓存")
	}
	// 另一个路径 → 不该命中
	if _, ok := cache.lookup(`C:\addons\b.vpk`, info); ok {
		t.Fatal("不同路径不该命中缓存")
	}
}

func TestVPKIntegrityCacheNormalizesWindowsPathCase(t *testing.T) {
	var cache vpkIntegrityCache
	info := fakeFileInfo{size: 1, modTime: time.Now()}
	cache.store(`C:\Addons\A.VPK`, info, parser.VPKIntegrityReport{TotalFiles: 1})
	if _, ok := cache.lookup(`c:\addons\a.vpk`, info); !ok {
		t.Fatal("Windows 路径大小写不同也应命中同一条缓存")
	}
}

// buildTestVPK 造一个能被 App 校验的最小 VPK。
func buildTestVPK(t *testing.T, filePath string) {
	t.Helper()
	addonInfo := []byte("\"AddonInfo\"\n{\n\t\"addontitle\" \"cached\"\n}\n")
	archive := &vpk.Archive{
		Header: vpk.Header{Magic: vpk.Magic, Version: 1},
		Files: []vpk.File{{
			Dir: " ", Base: "addoninfo", Ext: "txt",
			DirEntry: vpk.DirEntry{
				CRC: crc32.ChecksumIEEE(addonInfo),
				DataLocation: []vpk.DataChunk{{
					ArchiveIndex: 0x7fff,
					EntryOffset:  0,
					EntryLength:  uint32(len(addonInfo)),
				}},
			},
		}},
	}
	var buffer bytes.Buffer
	if err := vpk.WriteDirectory(&buffer, archive); err != nil {
		t.Fatalf("写入 VPK 目录表失败: %v", err)
	}
	buffer.Write(addonInfo)
	if err := os.WriteFile(filePath, buffer.Bytes(), 0o644); err != nil {
		t.Fatalf("写入测试 VPK 失败: %v", err)
	}
}

func TestInspectVPKIntegrityUsesCacheOnSecondCall(t *testing.T) {
	app := &App{}
	filePath := filepath.Join(t.TempDir(), "cached.vpk")
	buildTestVPK(t, filePath)

	first, err := app.InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatalf("第一次校验失败: %v", err)
	}
	if !first.Valid || first.ScanMode != parser.VPKIntegrityScanIndex {
		t.Fatalf("校验结果不对: valid=%v mode=%q", first.Valid, first.ScanMode)
	}

	second, err := app.InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatalf("第二次校验失败: %v", err)
	}
	if second.TotalFiles != first.TotalFiles || second.Valid != first.Valid {
		t.Fatalf("两次结论不一致: %+v vs %+v", first, second)
	}
	if _, ok := app.integrityCache.lookup(filePath, mustStat(t, filePath)); !ok {
		t.Fatal("校验完成后应写入缓存")
	}
}

func TestInspectVPKIntegrityBatchKeepsRequestOrderAndDedupes(t *testing.T) {
	app := &App{}
	dir := t.TempDir()
	firstPath := filepath.Join(dir, "first.vpk")
	secondPath := filepath.Join(dir, "second.vpk")
	buildTestVPK(t, firstPath)
	buildTestVPK(t, secondPath)

	results := app.InspectVPKIntegrityBatch([]string{secondPath, firstPath, secondPath})
	if len(results) != 2 {
		t.Fatalf("重复路径应去重，实际 %d 条", len(results))
	}
	if results[0].Path != secondPath || results[1].Path != firstPath {
		t.Fatalf("结果顺序应与请求一致: %q / %q", results[0].Path, results[1].Path)
	}
	for _, item := range results {
		if item.Error != "" || !item.Report.Valid {
			t.Fatalf("批量校验出现问题: %+v", item)
		}
	}
}

func TestInspectVPKIntegrityBatchReportsPerItemError(t *testing.T) {
	app := &App{}
	dir := t.TempDir()
	goodPath := filepath.Join(dir, "good.vpk")
	buildTestVPK(t, goodPath)
	missingPath := filepath.Join(dir, "missing.vpk")

	results := app.InspectVPKIntegrityBatch([]string{missingPath, goodPath})
	if len(results) != 2 {
		t.Fatalf("应保留两条结果，实际 %d", len(results))
	}
	if results[0].Error == "" {
		t.Fatal("不存在的文件要给出条目级错误，而不是整体失败")
	}
	if results[1].Error != "" || !results[1].Report.Valid {
		t.Fatalf("单个文件失败不该影响其它文件: %+v", results[1])
	}
}

func mustStat(t *testing.T, path string) os.FileInfo {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info
}
