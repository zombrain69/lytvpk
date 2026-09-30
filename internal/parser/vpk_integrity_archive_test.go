package parser

import (
	"bytes"
	"hash/crc32"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
)

// VPK 完整性校验的归档级测试。
//
// 背景（真机实测）：老实现逐条目 io.Copy 读完整包再核对 CRC ——
// 1.42GB 的包要 4 分 03 秒、16.8MB 的包 1.3 秒，而这条路径挂在
// 「启用游戏内 Mod」之前，于是"启用"比"关闭"慢几个数量级。
// 现在默认是索引级校验，这里锁住它的两个方向：
//   1. 该抓的还是要抓到（缺 addoninfo、数据被截断/越界）；
//   2. 明确记录下来哪些东西索引级**不**查（包内部的字节损坏 → 只有整包校验才发现）。

type testVPKEntry struct {
	name    string
	content []byte
}

// splitVPKEntryName 把 "materials/models/x.vmt" 拆成 VPK 目录表的 Dir/Base/Ext。
func splitVPKEntryName(name string) (dir, base, ext string) {
	normalized := strings.ReplaceAll(name, "\\", "/")
	dir = " "
	base = normalized
	if index := strings.LastIndex(normalized, "/"); index >= 0 {
		dir = normalized[:index]
		base = normalized[index+1:]
	}
	ext = " "
	if index := strings.LastIndex(base, "."); index > 0 {
		ext = base[index+1:]
		base = base[:index]
	}
	return dir, base, ext
}

// buildSingleFileVPK 造一个单文件 VPK：目录表 + 紧跟其后的数据区。
func buildSingleFileVPK(t *testing.T, filePath string, entries []testVPKEntry) {
	t.Helper()
	archive := &vpk.Archive{Header: vpk.Header{Magic: vpk.Magic, Version: 1}}
	var data bytes.Buffer
	for _, entry := range entries {
		offset := uint32(data.Len())
		data.Write(entry.content)
		dir, base, ext := splitVPKEntryName(entry.name)
		archive.Files = append(archive.Files, vpk.File{
			Dir: dir, Base: base, Ext: ext,
			DirEntry: vpk.DirEntry{
				CRC: crc32.ChecksumIEEE(entry.content),
				DataLocation: []vpk.DataChunk{{
					ArchiveIndex: 0x7fff,
					EntryOffset:  offset,
					EntryLength:  uint32(len(entry.content)),
				}},
			},
		})
	}
	var buffer bytes.Buffer
	if err := vpk.WriteDirectory(&buffer, archive); err != nil {
		t.Fatalf("写入 VPK 目录表失败: %v", err)
	}
	buffer.Write(data.Bytes())
	if err := os.WriteFile(filePath, buffer.Bytes(), 0o644); err != nil {
		t.Fatalf("写入测试 VPK 失败: %v", err)
	}
}

func healthyVPKEntries() []testVPKEntry {
	return []testVPKEntry{
		{name: "addoninfo.txt", content: []byte("\"AddonInfo\"\n{\n\t\"addontitle\" \"测试包\"\n}\n")},
		{name: "materials/models/test.vmt", content: bytes.Repeat([]byte("abcd"), 64)},
	}
}

func issueCodes(report VPKIntegrityReport) []string {
	codes := make([]string, 0, len(report.Issues))
	for _, issue := range report.Issues {
		codes = append(codes, issue.Code)
	}
	return codes
}

func TestInspectVPKIntegrityIndexScanAcceptsHealthyArchive(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "healthy.vpk")
	buildSingleFileVPK(t, filePath, healthyVPKEntries())

	report, err := InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatalf("索引级校验报错: %v", err)
	}
	if !report.Valid {
		t.Fatalf("健康包被判成有问题: %v", issueCodes(report))
	}
	if report.ScanMode != VPKIntegrityScanIndex {
		t.Fatalf("默认校验模式应为索引级，实际 %q", report.ScanMode)
	}
	if report.TotalFiles != 2 || report.VerifiedFiles != 2 {
		t.Fatalf("条目数不对: total=%d verified=%d", report.TotalFiles, report.VerifiedFiles)
	}
	if !report.AddonInfoFound || !report.AddonInfoValid {
		t.Fatalf("addoninfo.txt 没被识别: found=%v valid=%v", report.AddonInfoFound, report.AddonInfoValid)
	}

	deepReport, err := inspectVPKIntegrityDeep(filePath)
	if err != nil {
		t.Fatalf("整包校验报错: %v", err)
	}
	if !deepReport.Valid || deepReport.ScanMode != VPKIntegrityScanFull {
		t.Fatalf("整包校验结论与索引级不一致: valid=%v mode=%q", deepReport.Valid, deepReport.ScanMode)
	}
}

func TestInspectVPKIntegrityDetectsTruncatedEntryData(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "truncated.vpk")
	buildSingleFileVPK(t, filePath, healthyVPKEntries())

	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	// 砍掉数据区尾部：最后一个条目的数据范围就落到文件之外了（下载不完整的典型形态）。
	if err := os.Truncate(filePath, info.Size()-32); err != nil {
		t.Fatal(err)
	}

	report, err := InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatalf("索引级校验报错: %v", err)
	}
	if report.Valid {
		t.Fatal("被截断的包却判成健康")
	}
	if !strings.Contains(strings.Join(issueCodes(report), ","), "entry-data-range") {
		t.Fatalf("没报出数据越界/截断: %v", issueCodes(report))
	}
	if report.VerifiedFiles >= report.TotalFiles {
		t.Fatalf("越界条目不该计入已校验: verified=%d total=%d", report.VerifiedFiles, report.TotalFiles)
	}

	deepReport, err := inspectVPKIntegrityDeep(filePath)
	if err != nil {
		t.Fatalf("整包校验报错: %v", err)
	}
	if deepReport.Valid {
		t.Fatal("整包校验没发现截断")
	}
}

func TestInspectVPKIntegrityDetectsMissingAddonInfo(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "no-addoninfo.vpk")
	buildSingleFileVPK(t, filePath, []testVPKEntry{
		{name: "materials/models/test.vmt", content: []byte("x")},
	})

	report, err := InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatalf("索引级校验报错: %v", err)
	}
	if report.AddonInfoFound {
		t.Fatal("不该报告找到了 addoninfo.txt")
	}
	if !strings.Contains(strings.Join(issueCodes(report), ","), "addoninfo-missing") {
		t.Fatalf("没报出缺 addoninfo.txt: %v", issueCodes(report))
	}
	if !report.Repairable {
		t.Fatal("缺 addoninfo.txt 属于可修复问题")
	}
}

// 这条测试记录的是**明确的取舍**，不是缺陷：
// 索引级校验不读包体，所以"数据大小没变、内容被改坏"这类字节级损坏它看不出来
// （要看出来就必须把整包读一遍 —— 那正是 1.42GB 要 4 分钟的原因）。
// 需要逐字节核对时用 inspectVPKIntegrityDeep。
func TestInspectVPKIntegrityIndexScanDoesNotReadEntryData(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "corrupted.vpk")
	entries := healthyVPKEntries()
	buildSingleFileVPK(t, filePath, entries)

	// 在数据区里翻一个字节，但保持文件大小与修改时间不变。
	raw, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := append([]byte(nil), raw...)
	corrupted[len(corrupted)-4] ^= 0xff
	if err := os.WriteFile(filePath, corrupted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filePath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	indexReport, err := InspectVPKIntegrity(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !indexReport.Valid {
		t.Fatalf("索引级校验按设计不读包体，不该报出字节级损坏: %v", issueCodes(indexReport))
	}

	deepReport, err := inspectVPKIntegrityDeep(filePath)
	if err != nil {
		t.Fatal(err)
	}
	if deepReport.Valid {
		t.Fatal("整包校验应该能发现被改坏的数据（CRC 不符）")
	}
}
