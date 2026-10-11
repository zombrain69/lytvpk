package app

import (
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/vpk"
	"vpk-manager/internal/parser"
)

// 测试基建：全部在 t.TempDir 里造 VPK（走真实打包代码），绝不触碰真实 Mod 库。

func writeXDRMoveSourceFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", full, err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func packXDRMoveTestMod(t *testing.T, a *App, srcDir string, outDir string, baseName string) string {
	t.Helper()
	result, err := a.packVPKDirectoryWithOptions(srcDir, outDir, false, baseName, nil)
	if err != nil {
		t.Fatalf("打包 %s 失败: %v", baseName, err)
	}
	return result.OutputPath
}

func seedXDRMoveCache(t *testing.T, a *App, path string, title string, slots ...parser.XDRSlotInfo) {
	t.Helper()
	size := int64(0)
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	a.vpkCache.Store(path, &VPKFileCache{File: VPKFile{
		Name:           filepath.Base(path),
		Path:           path,
		Size:           size,
		Title:          title,
		XDRSlots:       slots,
		GameEnabled:    true,
		GameStateKnown: true,
	}})
}

// readVPKEntryContents 读回整包条目（每条读完后 Close 会校验 CRC，任何不符直接 t.Fatal）。
func readVPKEntryContents(t *testing.T, path string) map[string]string {
	t.Helper()
	opener := vpk.Single(path)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		t.Fatalf("读取 %s: %v", path, err)
	}
	contents := make(map[string]string, len(archive.Files))
	for i := range archive.Files {
		entry := &archive.Files[i]
		reader, err := entry.Open(opener)
		if err != nil {
			t.Fatalf("打开条目 %s: %v", entry.Name(), err)
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("读取条目 %s: %v", entry.Name(), err)
		}
		if err := reader.Close(); err != nil {
			t.Fatalf("校验条目 %s: %v", entry.Name(), err)
		}
		contents[entry.Name()] = string(data)
	}
	return contents
}

func billAndZoeyModFiles() map[string]string {
	return map[string]string{
		"models/xdreanims/bill_slot_002.mdl":  "MDL-BILL-SLOT2",
		"models/xdreanims/bill_slot_002.ani":  "ANI-BILL-SLOT2",
		"models/xdreanims/zoey_slot_002.mdl":  "MDL-ZOEY-SLOT2",
		"models/xdreanims/decompiled/bill.qc": "// bill qc",
		"addoninfo.txt":                       "addontitle 测试动作\n",
	}
}

func TestPreviewXDRSlotMoveListsAvailableSlots(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)

	modA := packXDRMoveTestMod(t, a, writeXDRMoveTestSourceDir(t), root, "mod_a")
	modB := packXDRMoveTestMod(t, a, writeXDRMoveTestSourceDir(t), root, "mod_b")
	modC := packXDRMoveTestMod(t, a, writeXDRMoveTestSourceDir(t), root, "mod_c")
	seedXDRMoveCache(t, a, modA, "动作 A", xdrTestSlot("Bill", 2), xdrTestSlot("Zoey", 2))
	seedXDRMoveCache(t, a, modB, "动作 B", xdrTestSlot("Bill", 2))
	seedXDRMoveCache(t, a, modC, "动作 C", xdrTestSlot("Bill", 5))

	preview, err := a.PreviewXDRSlotMove(modA, "Bill", 2, 0)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if preview.DefaultSlot != 1 {
		t.Fatalf("默认槽位应为最低空槽 1，得到 %d", preview.DefaultSlot)
	}
	if len(preview.AvailableSlots) != 46 {
		t.Fatalf("可选槽位应为 46 个（排除 2 与 5），得到 %d", len(preview.AvailableSlots))
	}
	for _, option := range preview.AvailableSlots {
		if option.Slot == 2 || option.Slot == 5 {
			t.Fatalf("被占用/原槽位不应出现在可选列表：%d", option.Slot)
		}
	}
	if len(preview.Rivals) != 1 || preview.Rivals[0] != "动作 B" {
		t.Fatalf("同槽对手应列出 动作 B：%#v", preview.Rivals)
	}
	if len(preview.Files) != 2 {
		t.Fatalf("默认槽位下应有 2 个文件改名：%#v", preview.Files)
	}
	wantPairs := map[string]string{
		"models/xdreanims/bill_slot_002.mdl": "models/xdreanims/bill_slot_001.mdl",
		"models/xdreanims/bill_slot_002.ani": "models/xdreanims/bill_slot_001.ani",
	}
	for _, file := range preview.Files {
		if wantPairs[file.From] != file.To {
			t.Fatalf("改名对不符：%s → %s", file.From, file.To)
		}
	}

	preview10, err := a.PreviewXDRSlotMove(modA, "Bill", 2, 10)
	if err != nil {
		t.Fatalf("指定槽位预览失败: %v", err)
	}
	if preview10.ToSlot != 10 || preview10.ToSlotLabel != "010" {
		t.Fatalf("指定槽位应回显 010：%#v", preview10)
	}
	for _, file := range preview10.Files {
		if !strings.Contains(file.To, "bill_slot_010.") {
			t.Fatalf("指定槽位下改名结果应为 010：%s → %s", file.From, file.To)
		}
	}

	if _, err := a.PreviewXDRSlotMove(modA, "Bill", 2, 5); err == nil {
		t.Fatalf("目标槽位被其它 Mod 占用时应报错")
	}
	if _, err := a.PreviewXDRSlotMove(modA, "未指定角色/模型", 2, 10); err == nil {
		t.Fatalf("泛化角色不应允许换槽")
	}
}

// writeXDRMoveTestSourceDir 造一份含 bill/zoey 槽位文件的最小源目录。
func writeXDRMoveTestSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeXDRMoveSourceFiles(t, dir, billAndZoeyModFiles())
	return dir
}

func TestPreviewXDRSlotMoveSkipsSlotsThatWouldCollide(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	dir := t.TempDir()
	writeXDRMoveSourceFiles(t, dir, map[string]string{
		"models/xdreanims/bill_slot_002.mdl":    "MDL",
		"models/xdreanims/bill_slot_002.config": "CFG-A",
		"models/xdreanims/bill_slot_044.config": "CFG-B",
	})
	mod := packXDRMoveTestMod(t, a, dir, root, "mod_collide")
	seedXDRMoveCache(t, a, mod, "冲突测试", xdrTestSlot("Bill", 2))

	preview, err := a.PreviewXDRSlotMove(mod, "Bill", 2, 0)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	for _, option := range preview.AvailableSlots {
		if option.Slot == 44 {
			t.Fatalf("改名会与包内 bill_slot_044.config 重名，44 不应可选")
		}
	}
}

func TestMoveXDRModToSlotRebuildsArchiveAndKeepsBackup(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	dir := t.TempDir()
	writeXDRMoveSourceFiles(t, dir, billAndZoeyModFiles())
	mod := packXDRMoveTestMod(t, a, dir, root, "mod_move")
	seedXDRMoveCache(t, a, mod, "重建测试", xdrTestSlot("Bill", 2), xdrTestSlot("Zoey", 2))

	original, err := os.ReadFile(mod)
	if err != nil {
		t.Fatalf("读取原件: %v", err)
	}

	result, err := a.MoveXDRModToSlot(mod, "Bill", 2, 44)
	if err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if result.RenamedFiles != 2 {
		t.Fatalf("应改名 2 个条目，得到 %d（%#v）", result.RenamedFiles, result.ProcessedFiles)
	}
	if len(result.ProcessedFiles) != 1 || result.ProcessedFiles[0].Error != "" || result.ProcessedFiles[0].Skipped {
		t.Fatalf("单文件结果不符：%#v", result.ProcessedFiles)
	}

	contents := readVPKEntryContents(t, mod)
	if _, ok := contents["models/xdreanims/bill_slot_044.mdl"]; !ok {
		t.Fatalf("改名后的 .mdl 不在包里：%#v", keysOf(contents))
	}
	if contents["models/xdreanims/bill_slot_044.mdl"] != "MDL-BILL-SLOT2" {
		t.Fatalf(".mdl 内容应原样保留")
	}
	if contents["models/xdreanims/bill_slot_044.ani"] != "ANI-BILL-SLOT2" {
		t.Fatalf(".ani 内容应原样保留")
	}
	if _, ok := contents["models/xdreanims/bill_slot_002.mdl"]; ok {
		t.Fatalf("旧槽位文件不应保留")
	}
	if _, ok := contents["models/xdreanims/bill_slot_002.ani"]; ok {
		t.Fatalf("旧槽位文件不应保留")
	}
	if contents["models/xdreanims/zoey_slot_002.mdl"] != "MDL-ZOEY-SLOT2" {
		t.Fatalf("其它角色文件应原样保留")
	}
	if contents["addoninfo.txt"] != "addontitle 测试动作\n" {
		t.Fatalf("addoninfo.txt 应原样保留")
	}
	if len(contents) != len(billAndZoeyModFiles()) {
		t.Fatalf("条目数应不变：%d != %d", len(contents), len(billAndZoeyModFiles()))
	}

	backup, err := os.ReadFile(mod + ".lytvpk.bak")
	if err != nil {
		t.Fatalf("应创建首次编辑备份: %v", err)
	}
	if !bytes.Equal(backup, original) {
		t.Fatalf("备份应与原件逐字节一致")
	}

	// 重新解析（走应用自己的解析器）：槽位证据应变成 Bill 44，Zoey 2 不变。
	parsed, err := parser.ParseVPKFile(mod)
	if err != nil {
		t.Fatalf("重解析失败: %v", err)
	}
	hasBill44, hasZoey2 := false, false
	for _, slot := range parsed.XDRSlots {
		if slot.Character == "Bill" && slot.Slot == 44 {
			hasBill44 = true
		}
		if slot.Character == "Zoey" && slot.Slot == 2 {
			hasZoey2 = true
		}
	}
	if !hasBill44 || !hasZoey2 {
		t.Fatalf("重解析证据不符（Bill44=%v Zoey2=%v）：%#v", hasBill44, hasZoey2, parsed.XDRSlots)
	}
}

func keysOf(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

// TestXDRSlotMoveRebuildsRealModCopy 是对真实库 VPK 的只读抽样验证：
// 默认跳过；设 LYTVPK_XDR_MOVE_SAMPLE=<某个真实 .vpk 路径> 时，把它复制到临时目录再执行换槽，
// 验证"真实 Mod 的整包重建"可读、可解析、可回滚 —— 全程不碰真实库里的原件。
func TestXDRSlotMoveRebuildsRealModCopy(t *testing.T) {
	sample := strings.TrimSpace(os.Getenv("LYTVPK_XDR_MOVE_SAMPLE"))
	if sample == "" {
		t.Skip("未提供 LYTVPK_XDR_MOVE_SAMPLE 样例路径，跳过真实库抽样")
	}
	data, err := os.ReadFile(sample)
	if err != nil {
		t.Fatalf("读取样例: %v", err)
	}
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	mod := filepath.Join(root, filepath.Base(sample))
	if err := os.WriteFile(mod, data, 0o644); err != nil {
		t.Fatalf("复制样例: %v", err)
	}

	parsed, err := parser.ParseVPKFile(mod)
	if err != nil {
		t.Fatalf("样例解析失败: %v", err)
	}
	if len(parsed.XDRSlots) == 0 {
		t.Fatalf("样例没有 XDR 槽位证据，无法作为抽样：%s", sample)
	}
	target := parsed.XDRSlots[0]
	t.Logf("样例槽位：%s slot %03d（共 %d 组）", target.Character, target.Slot, len(parsed.XDRSlots))

	seedXDRMoveCache(t, a, mod, "真实样例", parsed.XDRSlots...)
	otherSlot := target.Slot + 7
	if otherSlot > maxXDRSlotNumber {
		otherSlot = target.Slot - 7
	}
	before := readVPKEntryContents(t, mod)
	result, err := a.MoveXDRModToSlot(mod, target.Character, target.Slot, otherSlot)
	if err != nil {
		t.Fatalf("真实样例换槽失败: %v", err)
	}
	if result.RenamedFiles == 0 {
		t.Fatalf("真实样例应至少有 1 个条目改名：%#v", result)
	}
	after := readVPKEntryContents(t, mod)
	if len(before) != len(after) {
		t.Fatalf("条目数应不变：%d → %d", len(before), len(after))
	}
	// 每个旧名要么原样保留，要么改成新名字；不允许丢内容。
	for name, content := range before {
		if current, ok := after[name]; ok {
			if current != content {
				t.Fatalf("未改名条目内容被改动：%s", name)
			}
			continue
		}
		if !strings.Contains(name, fmt.Sprintf("slot_%03d", target.Slot)) {
			t.Fatalf("条目 %s 既没有保留也没有改名", name)
		}
	}
	// 重新解析：槽位证据应指向新槽。
	reparsed, err := parser.ParseVPKFile(mod)
	if err != nil {
		t.Fatalf("重建后解析失败: %v", err)
	}
	found := false
	for _, slot := range reparsed.XDRSlots {
		if slot.Character == target.Character && slot.Slot == otherSlot {
			found = true
		}
	}
	if !found {
		t.Fatalf("重建后应出现 %s slot %03d：%#v", target.Character, otherSlot, reparsed.XDRSlots)
	}
	if _, err := os.Stat(mod + ".lytvpk.bak"); err != nil {
		t.Fatalf("应有首次编辑备份: %v", err)
	}
}

func TestMoveXDRModToSlotProcessesAllCopies(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	dir := t.TempDir()
	writeXDRMoveSourceFiles(t, dir, billAndZoeyModFiles())
	mod := packXDRMoveTestMod(t, a, dir, root, "mod_copy")
	original, err := os.ReadFile(mod)
	if err != nil {
		t.Fatalf("读取原件: %v", err)
	}
	workshopDir := filepath.Join(root, "workshop")
	if err := os.MkdirAll(workshopDir, 0o755); err != nil {
		t.Fatalf("mkdir workshop: %v", err)
	}
	copyPath := filepath.Join(workshopDir, filepath.Base(mod))
	if err := os.WriteFile(copyPath, original, 0o644); err != nil {
		t.Fatalf("复制副本: %v", err)
	}
	seedXDRMoveCache(t, a, mod, "副本测试", xdrTestSlot("Bill", 2))
	seedXDRMoveCache(t, a, copyPath, "副本测试", xdrTestSlot("Bill", 2))

	preview, err := a.PreviewXDRSlotMove(mod, "Bill", 2, 0)
	if err != nil {
		t.Fatalf("预览失败: %v", err)
	}
	if len(preview.SiblingPaths) != 1 || !strings.EqualFold(preview.SiblingPaths[0], copyPath) {
		t.Fatalf("应识别工坊副本：%#v", preview.SiblingPaths)
	}

	result, err := a.MoveXDRModToSlot(mod, "Bill", 2, 44)
	if err != nil {
		t.Fatalf("执行失败: %v（%#v）", err, result.ProcessedFiles)
	}
	if result.RenamedFiles != 4 {
		t.Fatalf("两份副本共应改名 4 个条目，得到 %d（%#v）", result.RenamedFiles, result.ProcessedFiles)
	}
	for _, path := range []string{mod, copyPath} {
		contents := readVPKEntryContents(t, path)
		if _, ok := contents["models/xdreanims/bill_slot_044.mdl"]; !ok {
			t.Fatalf("%s 应改名到 044：%#v", path, keysOf(contents))
		}
		if _, err := os.Stat(path + ".lytvpk.bak"); err != nil {
			t.Fatalf("%s 应有备份: %v", path, err)
		}
	}
}

func TestMoveXDRModToSlotRejectsOccupiedTarget(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	dirA := t.TempDir()
	writeXDRMoveSourceFiles(t, dirA, billAndZoeyModFiles())
	modA := packXDRMoveTestMod(t, a, dirA, root, "mod_target_a")
	seedXDRMoveCache(t, a, modA, "被占目标 A", xdrTestSlot("Bill", 2))

	dirB := t.TempDir()
	writeXDRMoveSourceFiles(t, dirB, map[string]string{
		"models/xdreanims/bill_slot_044.mdl": "MDL-B",
	})
	modB := packXDRMoveTestMod(t, a, dirB, root, "mod_target_b")
	seedXDRMoveCache(t, a, modB, "占位者 B", xdrTestSlot("Bill", 44))

	before := readVPKEntryContents(t, modA)
	if _, err := a.MoveXDRModToSlot(modA, "Bill", 2, 44); err == nil || !strings.Contains(err.Error(), "占用") {
		t.Fatalf("目标被其它 Mod 占用时应报错：%v", err)
	}
	after := readVPKEntryContents(t, modA)
	if len(before) != len(after) {
		t.Fatalf("校验失败时不应改文件")
	}
	if _, err := os.Stat(modA + ".lytvpk.bak"); !os.IsNotExist(err) {
		t.Fatalf("校验失败时不应创建备份")
	}
}

func TestMoveXDRModToSlotReportsMissingFileAsError(t *testing.T) {
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	ghost := filepath.Join(root, "ghost.vpk")
	seedXDRMoveCache(t, a, ghost, "幽灵", xdrTestSlot("Bill", 2))

	result, err := a.MoveXDRModToSlot(ghost, "Bill", 2, 44)
	if err == nil {
		t.Fatalf("文件缺失且无任何成功文件时应返回错误：%#v", result)
	}
	if len(result.ProcessedFiles) != 1 || result.ProcessedFiles[0].Error == "" {
		t.Fatalf("应记录逐文件错误：%#v", result.ProcessedFiles)
	}
}

func TestMoveXDRModToSlotPreservesPreloadMetadata(t *testing.T) {
	// 带 preload（目录内嵌小文件）的包也要原样重建：
	// 打包工具不会产出 preload，这里直接用 vpk 包手工构造一份。
	root := t.TempDir()
	a := newProfileTestApp(t, root)
	mod := filepath.Join(root, "mod_preload.vpk")
	metadata := "META-PART"
	data := "DATA-PART"
	hasher := crc32.NewIEEE()
	hasher.Write([]byte(metadata))
	hasher.Write([]byte(data))
	archive := &vpk.Archive{
		Header: vpk.Header{Magic: vpk.Magic, Version: 1},
		Files: []vpk.File{
			{
				Dir: "models/xdreanims", Base: "bill_slot_002", Ext: "mdl",
				DirEntry: vpk.DirEntry{
					CRC:           hasher.Sum32(),
					MetadataBytes: uint16(len(metadata)),
					DataLocation:  []vpk.DataChunk{{ArchiveIndex: 0x7fff, EntryOffset: 0, EntryLength: uint32(len(data))}},
				},
				Metadata: []byte(metadata),
			},
		},
	}
	file, err := os.Create(mod)
	if err != nil {
		t.Fatalf("创建构造包: %v", err)
	}
	if err := vpk.WriteDirectory(file, archive); err != nil {
		t.Fatalf("写目录: %v", err)
	}
	if _, err := file.Write([]byte(data)); err != nil {
		t.Fatalf("写数据: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("关闭: %v", err)
	}
	seedXDRMoveCache(t, a, mod, "preload 测试", xdrTestSlot("Bill", 2))

	if _, err := a.MoveXDRModToSlot(mod, "Bill", 2, 30); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	contents := readVPKEntryContents(t, mod)
	if contents["models/xdreanims/bill_slot_030.mdl"] != metadata+data {
		t.Fatalf("preload 包改名后内容应保留：%#v", contents)
	}
	// 直接核对重建后的条目仍带 preload 元数据（数据段只放 DATA-PART）。
	opener := vpk.Single(mod)
	defer opener.Close()
	rebuilt, err := opener.ReadArchive()
	if err != nil {
		t.Fatalf("重读: %v", err)
	}
	if len(rebuilt.Files) != 1 || string(rebuilt.Files[0].Metadata) != metadata {
		t.Fatalf("preload 元数据应保留：%#v", rebuilt.Files)
	}
	if rebuilt.Files[0].DataLocation[0].EntryLength != uint32(len(data)) {
		t.Fatalf("数据段长度应只含 DATA-PART：%#v", rebuilt.Files[0].DataLocation)
	}
}
