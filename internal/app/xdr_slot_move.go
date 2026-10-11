package app

import (
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"l4d2-manager-next/pkg/valve/vpk"
	"vpk-manager/internal/parser"
)

// XDR 动作槽位调整（"移到空槽"）。
//
// 背景：官方对同角色同槽冲突的处置建议是 "unpack and move the anim file to another
// slot"。本项目扫描已经给出同槽冲突（xdr_slot_collision / 详情面板的"随机"角标），
// 这里补上"一键处理"：把某个 Mod 在某个角色上的槽位文件改名到另一个空槽。
//
// 实现纪律（对齐本项目既有写盘约定）：
//   - 与移动 / 删除共用文件操作闸门（beginFileOperation，含沙箱只读拒绝）；
//   - 重写走"流式重建"：不解包到磁盘，直接按新目录树重写数据段，
//     拷贝时逐条目校验 CRC，任何不符立即中止、不动原件；
//   - 替换前先给原件留"首次编辑备份"（<文件>.vpk.lytvpk.bak，只在不存在时创建，
//     保留的是 LytVPK 第一次改动前的内容，永不自动删除）；
//   - 写入临时文件 + 读回校验（条目数 / 改名结果齐全）后才 os.Rename 原子替换；
//   - 同一份 Mod 的根目录 / 工坊副本（文件名+大小一致）视为一件事，一起处理。
const (
	// maxXDRSlotNumber 是框架每个角色的槽位上限（1–48）。
	maxXDRSlotNumber = 48
	// xdrSelfArchiveIndex 是 VPK 目录里"数据在本文件内"的存档索引（格式常量 0x7fff）。
	xdrSelfArchiveIndex = 0x7fff
	// xdrSlotMoveBackupSuffix 是首次编辑备份的后缀（与 addonlist.txt 的 .lytvpk.bak 同口径）。
	xdrSlotMoveBackupSuffix = ".lytvpk.bak"
)

// XDRSlotMovePreview 是"移到空槽"弹窗需要的全部信息。
type XDRSlotMovePreview struct {
	SourcePath     string              `json:"sourcePath"`
	Character      string              `json:"character"`
	FromSlot       int                 `json:"fromSlot"`
	FromSlotLabel  string              `json:"fromSlotLabel"`
	ToSlot         int                 `json:"toSlot"`
	ToSlotLabel    string              `json:"toSlotLabel"`
	DefaultSlot    int                 `json:"defaultSlot"`
	AvailableSlots []XDRSlotMoveOption `json:"availableSlots"`
	Files          []XDRSlotMoveFile   `json:"files"`
	SiblingPaths   []string            `json:"siblingPaths"`
	Rivals         []string            `json:"rivals"`
}

// XDRSlotMoveOption 是一个可选的目标空槽（带官方建议用途）。
type XDRSlotMoveOption struct {
	Slot      int    `json:"slot"`
	SlotLabel string `json:"slotLabel"`
	SlotName  string `json:"slotName"`
	SlotGroup string `json:"slotGroup"`
}

// XDRSlotMoveFile 是一条将改名的 VPK 内条目。
type XDRSlotMoveFile struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// XDRSlotMoveResult 是执行结果（逐文件，允许部分成功）。
type XDRSlotMoveResult struct {
	SourcePath     string                  `json:"sourcePath"`
	Character      string                  `json:"character"`
	FromSlot       int                     `json:"fromSlot"`
	ToSlot         int                     `json:"toSlot"`
	RenamedFiles   int                     `json:"renamedFiles"`
	ProcessedFiles []XDRSlotMoveFileResult `json:"processedFiles"`
}

// XDRSlotMoveFileResult 是单个 VPK 文件的处理结果。
type XDRSlotMoveFileResult struct {
	Path         string `json:"path"`
	RenamedCount int    `json:"renamedCount"`
	Skipped      bool   `json:"skipped,omitempty"`
	Error        string `json:"error,omitempty"`
}

// PreviewXDRSlotMove 预览「把某 Mod 的某个角色槽位移到空槽」：
// 返回可选目标槽（已排除被其它已启用 Mod 占用的、本 Mod 自己占用的、以及改名会重名的）、
// 默认推荐槽（最低可用槽）、将改名的文件清单与同内容副本路径。
//
// toSlot 传 0 表示按默认推荐槽生成文件清单；传具体槽号则按该槽生成。
func (a *App) PreviewXDRSlotMove(sourcePath string, character string, fromSlot int, toSlot int) (XDRSlotMovePreview, error) {
	result := XDRSlotMovePreview{}
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	character = strings.TrimSpace(character)
	result.SourcePath = sourcePath
	result.Character = character
	result.FromSlot = fromSlot
	if sourcePath == "" {
		return result, fmt.Errorf("VPK 路径不能为空")
	}
	if fromSlot < 1 || fromSlot > maxXDRSlotNumber {
		return result, fmt.Errorf("槽位号必须在 1–%d 之间", maxXDRSlotNumber)
	}
	if !isConcreteXDRCharacter(character) {
		return result, fmt.Errorf("这个槽位没有识别出具体角色，无法自动换槽")
	}
	if toSlot != 0 && (toSlot < 1 || toSlot > maxXDRSlotNumber || toSlot == fromSlot) {
		return result, fmt.Errorf("目标槽位不合法")
	}
	result.FromSlotLabel = fmt.Sprintf("%03d", fromSlot)

	files := a.allVPKFilesSnapshot()
	source, found := findVPKFileByPath(files, sourcePath)
	if !found {
		return result, fmt.Errorf("未找到该 Mod 的解析结果，请刷新列表后重试")
	}
	if !xdrFileHasSlot(source, character, fromSlot) {
		return result, fmt.Errorf("解析结果里没有 %s 的 slot %03d 证据，无法换槽", character, fromSlot)
	}
	copies := xdrModCopies(files, source)
	selfKeys := xdrModKeySet(copies)
	occupancy := a.xdrCharacterSlotOccupancy(files, character, selfKeys)
	result.Rivals = occupancy[fromSlot]
	ownSlots := xdrOwnedSlotsForCharacter(copies, character)

	opener := vpk.Single(sourcePath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return result, fmt.Errorf("无法读取 VPK：%v", err)
	}
	entries := xdrArchiveEntryNames(archive)
	probeSlot := 1
	if fromSlot == 1 {
		probeSlot = 2
	}
	if len(parser.PlanXDRSlotRenames(entries, character, fromSlot, probeSlot)) == 0 {
		return result, fmt.Errorf("这个 VPK 里没有匹配 %s slot %03d 的文件", character, fromSlot)
	}

	available := make([]XDRSlotMoveOption, 0, maxXDRSlotNumber)
	for slot := 1; slot <= maxXDRSlotNumber; slot++ {
		if slot == fromSlot || len(occupancy[slot]) > 0 || ownSlots[slot] {
			continue
		}
		candidatePlan := parser.PlanXDRSlotRenames(entries, character, fromSlot, slot)
		if len(candidatePlan) == 0 || !xdrRenamesKeepNamesUnique(entries, candidatePlan) {
			continue
		}
		designation := parser.XDRSlotDesignationFor(slot)
		available = append(available, XDRSlotMoveOption{
			Slot:      slot,
			SlotLabel: fmt.Sprintf("%03d", slot),
			SlotName:  designation.Name,
			SlotGroup: designation.Group,
		})
	}
	result.AvailableSlots = available
	if len(available) > 0 {
		result.DefaultSlot = available[0].Slot
	}

	target := toSlot
	if target == 0 {
		target = result.DefaultSlot
	}
	if target != 0 {
		okTarget := false
		for _, option := range available {
			if option.Slot == target {
				okTarget = true
				break
			}
		}
		if !okTarget {
			return result, fmt.Errorf("目标槽位 %03d 不可用（可能已被其它 Mod 占用，或改名后会重名）", target)
		}
		result.ToSlot = target
		result.ToSlotLabel = fmt.Sprintf("%03d", target)
		result.Files = xdrSlotMoveFiles(parser.PlanXDRSlotRenames(entries, character, fromSlot, target))
	}

	siblingPaths := make([]string, 0, len(copies))
	for _, copy := range copies {
		if !strings.EqualFold(filepath.Clean(copy.Path), sourcePath) {
			siblingPaths = append(siblingPaths, copy.Path)
		}
	}
	sort.Strings(siblingPaths)
	result.SiblingPaths = siblingPaths
	return result, nil
}

// MoveXDRModToSlot 执行「移到空槽」：把源文件（及同内容副本）里该角色的槽位文件改名到目标槽。
//
// 校验失败 / 闸门占用返回 error；单个文件处理失败收集在结果的 ProcessedFiles 里（允许部分成功）。
func (a *App) MoveXDRModToSlot(sourcePath string, character string, fromSlot int, toSlot int) (XDRSlotMoveResult, error) {
	result := XDRSlotMoveResult{}
	sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
	character = strings.TrimSpace(character)
	result.SourcePath = sourcePath
	result.Character = character
	result.FromSlot = fromSlot
	result.ToSlot = toSlot
	if sourcePath == "" {
		return result, fmt.Errorf("VPK 路径不能为空")
	}
	if fromSlot < 1 || fromSlot > maxXDRSlotNumber {
		return result, fmt.Errorf("槽位号必须在 1–%d 之间", maxXDRSlotNumber)
	}
	if toSlot < 1 || toSlot > maxXDRSlotNumber || toSlot == fromSlot {
		return result, fmt.Errorf("目标槽位不合法")
	}
	if !isConcreteXDRCharacter(character) {
		return result, fmt.Errorf("这个槽位没有识别出具体角色，无法自动换槽")
	}

	if err := a.beginFileOperation(); err != nil {
		return result, err
	}
	defer a.endFileOperation()

	files := a.allVPKFilesSnapshot()
	source, found := findVPKFileByPath(files, sourcePath)
	if !found {
		return result, fmt.Errorf("未找到该 Mod 的解析结果，请刷新列表后重试")
	}
	if !xdrFileHasSlot(source, character, fromSlot) {
		return result, fmt.Errorf("解析结果里没有 %s 的 slot %03d 证据，无法换槽", character, fromSlot)
	}

	// 目标槽位复检：预览之后列表可能已经变化（别的 Mod 换过去了、开关变了）。
	copies := xdrModCopies(files, source)
	selfKeys := xdrModKeySet(copies)
	occupancy := a.xdrCharacterSlotOccupancy(files, character, selfKeys)
	if len(occupancy[toSlot]) > 0 {
		return result, fmt.Errorf("目标槽位 %03d 已被其它 Mod 占用（%s），请刷新后重新选择", toSlot, strings.Join(occupancy[toSlot], "、"))
	}
	if xdrOwnedSlotsForCharacter(copies, character)[toSlot] {
		return result, fmt.Errorf("本 Mod 自己已占用槽位 %03d，请换一个", toSlot)
	}

	targets := make([]string, 0, len(copies))
	for _, copy := range copies {
		targets = append(targets, copy.Path)
	}
	// 源文件排在第一个：它的失败原因最重要。
	sort.SliceStable(targets, func(i, j int) bool {
		iSelf := strings.EqualFold(filepath.Clean(targets[i]), sourcePath)
		jSelf := strings.EqualFold(filepath.Clean(targets[j]), sourcePath)
		if iSelf != jSelf {
			return iSelf
		}
		return targets[i] < targets[j]
	})

	for _, targetPath := range targets {
		item := XDRSlotMoveFileResult{Path: targetPath}
		renamedCount, err := a.moveXDRFileSlot(targetPath, character, fromSlot, toSlot)
		switch {
		case err != nil:
			item.Error = err.Error()
		case renamedCount == 0:
			item.Skipped = true
		default:
			item.RenamedCount = renamedCount
			result.RenamedFiles += renamedCount
		}
		result.ProcessedFiles = append(result.ProcessedFiles, item)
	}

	// 一个文件都没成功、且有失败原因：直接作为错误返回（界面保留弹窗、允许换槽重试）；
	// 部分成功时错误收集在 ProcessedFiles 里，让界面按"部分完成"提示。
	if result.RenamedFiles == 0 {
		for _, item := range result.ProcessedFiles {
			if item.Error != "" {
				return result, fmt.Errorf("%s", item.Error)
			}
		}
	}

	a.InvalidateConflictRecheck("XDR 动作槽位已调整")
	return result, nil
}

// moveXDRFileSlot 处理单个 VPK：读目录 → 规划改名 → 重建到临时文件 → 首次编辑备份 → 原子替换。
// 返回改名条目数（0 = 该文件没有这个槽位，跳过）。
func (a *App) moveXDRFileSlot(sourcePath string, character string, fromSlot int, toSlot int) (int, error) {
	info, err := os.Stat(sourcePath)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, fmt.Errorf("找不到 %s，可能已被移动或删除", filepath.Base(sourcePath))
		}
		return 0, fmt.Errorf("无法访问 %s：%v", filepath.Base(sourcePath), err)
	}
	if info.IsDir() {
		return 0, fmt.Errorf("%s 是文件夹，不是 VPK 文件", filepath.Base(sourcePath))
	}

	opener := vpk.Single(sourcePath)
	openerClosed := false
	closeOpener := func() {
		if !openerClosed {
			openerClosed = true
			_ = opener.Close()
		}
	}
	defer closeOpener()
	archive, err := opener.ReadArchive()
	if err != nil {
		return 0, fmt.Errorf("无法读取 %s：%v", filepath.Base(sourcePath), err)
	}

	entries := xdrArchiveEntryNames(archive)
	renames := parser.PlanXDRSlotRenames(entries, character, fromSlot, toSlot)
	if len(renames) == 0 {
		return 0, nil
	}
	if !xdrRenamesKeepNamesUnique(entries, renames) {
		return 0, fmt.Errorf("改名后会与压缩包内现有文件重名，未处理")
	}

	tempPath := fmt.Sprintf("%s.lytvpk-tmp-%d", sourcePath, time.Now().UnixNano())
	defer os.Remove(tempPath)
	if err := xdrWriteRebuiltVPK(tempPath, opener, archive, renames); err != nil {
		return 0, err
	}

	// 先释放源文件句柄：Windows 上覆盖"自己还打开着"的文件会 Access Denied。
	closeOpener()
	if err := xdrEnsureFirstEditBackup(sourcePath); err != nil {
		return 0, err
	}
	if err := os.Rename(tempPath, sourcePath); err != nil {
		return 0, formatFileMoveError("调整槽位", sourcePath, err)
	}

	// 内容变了但路径没变：清预览缓存，再走单文件重解析 —— 走缓存路径而不是先删条目，
	// 这样游戏内开关（addonlist 状态）会按 processVPKFileWithCache 的设计保留下来。
	a.deleteVPKPreviewCaches(sourcePath)
	a.processVPKFileWithCache(sourcePath, nil)
	return len(renames), nil
}

// xdrWriteRebuiltVPK 把 archive 按 rename 规划重写到 tempPath：
// 目录树用新名字，数据段按 (Ext, Dir, Base) 排序顺序重排；逐条目流式拷贝并校验 CRC。
// 任何一步失败都只删临时文件、不动原件。
func xdrWriteRebuiltVPK(tempPath string, opener *vpk.Opener, archive *vpk.Archive, renames []parser.XDRSlotRename) error {
	// 只重写 v1 目录树（WriteDirectory 的能力边界）；v2 包直接拒绝，避免丢失嵌入数据。
	if archive.Version != 1 {
		return fmt.Errorf("这个 VPK 是格式版本 %d，暂不支持自动换槽", archive.Version)
	}
	// 预检：数据块必须都在本文件内（单文件 VPK）；引用外部分卷的包不支持自动换槽。
	for i := range archive.Files {
		entry := &archive.Files[i]
		for _, chunk := range entry.DataLocation {
			if chunk.ArchiveIndex != xdrSelfArchiveIndex {
				return fmt.Errorf("这个 VPK 引用了外部分卷，暂不支持自动换槽")
			}
		}
	}

	renameDict := make(map[string]string, len(renames))
	for _, rename := range renames {
		renameDict[rename.From] = rename.To
	}

	rebuilt := make([]vpk.File, 0, len(archive.Files))
	srcIndexes := make([]int, 0, len(archive.Files))
	var running uint64
	for i := range archive.Files {
		src := archive.Files[i]
		name := src.Name()
		entry := vpk.File{
			Dir:      src.Dir,
			Base:     src.Base,
			Ext:      src.Ext,
			DirEntry: vpk.DirEntry{CRC: src.CRC, MetadataBytes: src.MetadataBytes},
			Metadata: src.Metadata,
		}
		if newName, ok := renameDict[name]; ok {
			dir, base, ext := splitVPKPackPath(newName)
			if vpkEntryName(dir, base, ext) != newName {
				return fmt.Errorf("内部错误：改名结果无法还原（%s → %s）", name, newName)
			}
			entry.Dir, entry.Base, entry.Ext = dir, base, ext
		}
		var total uint32
		for _, chunk := range src.DataLocation {
			total += chunk.EntryLength
		}
		running += uint64(total)
		if running > math.MaxUint32 {
			return fmt.Errorf("Mod 体积超出 VPK 格式上限，无法重写")
		}
		entry.DataLocation = []vpk.DataChunk{{ArchiveIndex: xdrSelfArchiveIndex, EntryOffset: 0, EntryLength: total}}
		rebuilt = append(rebuilt, entry)
		srcIndexes = append(srcIndexes, i)
	}

	// WriteDirectory 要求按 (Ext, Dir, Base) 分组写入；数据段按同一顺序排列。
	order := make([]int, len(rebuilt))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(x, y int) bool {
		left := rebuilt[order[x]]
		right := rebuilt[order[y]]
		if left.Ext != right.Ext {
			return left.Ext < right.Ext
		}
		if left.Dir != right.Dir {
			return left.Dir < right.Dir
		}
		return left.Base < right.Base
	})

	var offset uint32
	sorted := make([]vpk.File, 0, len(rebuilt))
	for _, idx := range order {
		entry := rebuilt[idx]
		entry.DataLocation[0].EntryOffset = offset
		offset += entry.DataLocation[0].EntryLength
		sorted = append(sorted, entry)
	}

	out, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("无法创建临时文件：%v", err)
	}
	abort := func(failErr error) error {
		_ = out.Close()
		_ = os.Remove(tempPath)
		return failErr
	}

	outArchive := &vpk.Archive{
		Header: vpk.Header{Magic: vpk.Magic, Version: 1},
		Files:  sorted,
	}
	if err := vpk.WriteDirectory(out, outArchive); err != nil {
		return abort(fmt.Errorf("写入 VPK 目录失败：%v", err))
	}

	buffer := make([]byte, 1024*1024)
	for _, idx := range order {
		srcEntry := &archive.Files[srcIndexes[idx]]
		if err := xdrCopyEntryData(out, opener, srcEntry, buffer); err != nil {
			return abort(err)
		}
	}

	if err := out.Close(); err != nil {
		_ = os.Remove(tempPath)
		return fmt.Errorf("写入临时文件失败：%v", err)
	}

	// 读回校验：条目数量与名字集合必须与预期完全一致（防止写出结构损坏的包）。
	if err := xdrVerifyRebuiltVPK(tempPath, sorted); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

// xdrCopyEntryData 把条目数据（目录内嵌的 preload 数据 + 数据块）搬进新包的数据段，
// metadata 只进 CRC 校验、不进数据段（它在新目录树里原样保留）。
func xdrCopyEntryData(out io.Writer, opener *vpk.Opener, entry *vpk.File, buffer []byte) error {
	reader, err := entry.Open(opener)
	if err != nil {
		return fmt.Errorf("无法读取 %s 数据：%v", entry.Name(), err)
	}
	defer reader.Close()

	hasher := crc32.NewIEEE()
	if n := len(entry.Metadata); n > 0 {
		if _, err := io.CopyBuffer(hasher, io.LimitReader(reader, int64(n)), buffer); err != nil {
			return fmt.Errorf("读取 %s 失败：%v", entry.Name(), err)
		}
	}
	if _, err := io.CopyBuffer(io.MultiWriter(out, hasher), reader, buffer); err != nil {
		return fmt.Errorf("写入 %s 数据失败：%v", entry.Name(), err)
	}
	if sum := hasher.Sum32(); sum != entry.CRC {
		return fmt.Errorf("源 VPK 校验失败：%s 的内容与目录校验值不符（文件可能已损坏），已中止", entry.Name())
	}
	return nil
}

// xdrVerifyRebuiltVPK 读回临时文件，核对条目数量与名字集合。
func xdrVerifyRebuiltVPK(tempPath string, expected []vpk.File) error {
	opener := vpk.Single(tempPath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return fmt.Errorf("重写结果无法读取，已中止：%v", err)
	}
	if len(archive.Files) != len(expected) {
		return fmt.Errorf("重写结果条目数不符（%d != %d），已中止", len(archive.Files), len(expected))
	}
	have := make(map[string]bool, len(archive.Files))
	for i := range archive.Files {
		have[strings.ToLower(archive.Files[i].Name())] = true
	}
	for i := range expected {
		if !have[strings.ToLower(expected[i].Name())] {
			return fmt.Errorf("重写结果缺少条目 %s，已中止", expected[i].Name())
		}
	}
	return nil
}

// xdrEnsureFirstEditBackup 在第一次改动前给原件留一份备份（<文件>.vpk.lytvpk.bak）。
// 已存在时保留旧备份不动 —— 它保存的是 LytVPK 第一次改动前的内容，是最后的还原点。
func xdrEnsureFirstEditBackup(sourcePath string) error {
	backupPath := sourcePath + xdrSlotMoveBackupSuffix
	if _, err := os.Stat(backupPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("无法检查备份文件：%v", err)
	}
	in, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("无法读取原件做备份：%v", err)
	}
	defer in.Close()
	out, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return fmt.Errorf("无法创建备份文件：%v", err)
	}
	buffer := make([]byte, 1024*1024)
	if _, err := io.CopyBuffer(out, in, buffer); err != nil {
		_ = out.Close()
		_ = os.Remove(backupPath)
		return fmt.Errorf("写入备份失败：%v", err)
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(backupPath)
		return fmt.Errorf("写入备份失败：%v", err)
	}
	return nil
}

// xdrArchiveEntryNames 返回归档内全部条目名（正斜杠分隔）。
func xdrArchiveEntryNames(archive *vpk.Archive) []string {
	names := make([]string, 0, len(archive.Files))
	for i := range archive.Files {
		names = append(names, archive.Files[i].Name())
	}
	return names
}

// xdrRenamesKeepNamesUnique 检查应用改名后包内是否出现重名（大小写不敏感，Windows 口径）。
func xdrRenamesKeepNamesUnique(entries []string, renames []parser.XDRSlotRename) bool {
	dict := make(map[string]string, len(renames))
	for _, rename := range renames {
		dict[rename.From] = rename.To
	}
	seen := make(map[string]bool, len(entries))
	for _, name := range entries {
		final := name
		if to, ok := dict[name]; ok {
			final = to
		}
		key := strings.ToLower(strings.ReplaceAll(final, "\\", "/"))
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

// xdrSlotMoveFiles 把改名规划转成前端展示用的清单。
func xdrSlotMoveFiles(renames []parser.XDRSlotRename) []XDRSlotMoveFile {
	files := make([]XDRSlotMoveFile, 0, len(renames))
	for _, rename := range renames {
		files = append(files, XDRSlotMoveFile{From: rename.From, To: rename.To})
	}
	return files
}

// vpkEntryName 复刻 vpk.File.Name() 的拼名规则，用于改名结果的还原性校验。
func vpkEntryName(dir, base, ext string) string {
	name := base
	if name == " " {
		name = ""
	}
	if ext != " " {
		name += "." + ext
	}
	if dir != " " {
		name = dir + "/" + name
	}
	return name
}

// findVPKFileByPath 在快照里按路径找文件（Windows 大小写不敏感）。
func findVPKFileByPath(files []VPKFile, path string) (VPKFile, bool) {
	for _, file := range files {
		if strings.EqualFold(filepath.Clean(file.Path), path) {
			return file, true
		}
	}
	return VPKFile{}, false
}

// xdrFileHasSlot 判断解析结果里是否有该角色该槽位的证据。
func xdrFileHasSlot(file VPKFile, character string, slot int) bool {
	for _, entry := range file.XDRSlots {
		if entry.Slot == slot && strings.EqualFold(strings.TrimSpace(entry.Character), character) {
			return true
		}
	}
	return false
}

// isConcreteXDRCharacter 排除泛化角色（未指定 / Survivor / Infected），它们不适合自动换槽。
func isConcreteXDRCharacter(character string) bool {
	switch strings.TrimSpace(character) {
	case "", "未指定角色/模型", "Survivor", "Infected":
		return false
	}
	return true
}

// xdrModFileKey 是"同一份 Mod"的判据键（文件名小写 + 大小），与 xdrPriorityIndex 同口径。
func xdrModFileKey(file VPKFile) string {
	return fmt.Sprintf("%s|%d", strings.ToLower(file.Name), file.Size)
}

// xdrModCopies 返回同一份 Mod 的全部副本（根目录 / 工坊 / disabled 各一份的情况）。
func xdrModCopies(files []VPKFile, source VPKFile) []VPKFile {
	key := xdrModFileKey(source)
	copies := make([]VPKFile, 0, 2)
	for _, file := range files {
		if xdrModFileKey(file) == key {
			copies = append(copies, file)
		}
	}
	sort.SliceStable(copies, func(i, j int) bool { return copies[i].Path < copies[j].Path })
	return copies
}

func xdrModKeySet(files []VPKFile) map[string]bool {
	keys := make(map[string]bool, len(files))
	for _, file := range files {
		keys[xdrModFileKey(file)] = true
	}
	return keys
}

// xdrCharacterSlotOccupancy 统计某角色各槽位被哪些"会被游戏挂载"的其它 Mod 占用。
// excludeKeys 里的 Mod（本次要处理的文件及其副本）不参与统计。返回 slot → 占用者显示名。
func (a *App) xdrCharacterSlotOccupancy(files []VPKFile, character string, excludeKeys map[string]bool) map[int][]string {
	occupancy := make(map[int][]string)
	seen := make(map[string]bool)
	for _, file := range files {
		if len(file.XDRSlots) == 0 || !xdrModCountsAsActive(a, file) {
			continue
		}
		fileKey := xdrModFileKey(file)
		if excludeKeys[fileKey] {
			continue
		}
		for _, entry := range file.XDRSlots {
			if entry.Slot < 1 || entry.Slot > maxXDRSlotNumber {
				continue
			}
			if !strings.EqualFold(strings.TrimSpace(entry.Character), character) {
				continue
			}
			comboKey := fmt.Sprintf("%d|%s", entry.Slot, fileKey)
			if seen[comboKey] {
				continue
			}
			seen[comboKey] = true
			label := strings.TrimSpace(file.Title)
			if label == "" {
				label = file.Name
			}
			occupancy[entry.Slot] = append(occupancy[entry.Slot], label)
		}
	}
	for slot := range occupancy {
		sort.Strings(occupancy[slot])
	}
	return occupancy
}

// xdrOwnedSlotsForCharacter 返回这些文件自己占用的该角色槽位（改名进这些槽会在包内/副本间重名）。
func xdrOwnedSlotsForCharacter(files []VPKFile, character string) map[int]bool {
	own := make(map[int]bool)
	for _, file := range files {
		for _, entry := range file.XDRSlots {
			if entry.Slot < 1 || entry.Slot > maxXDRSlotNumber {
				continue
			}
			if strings.EqualFold(strings.TrimSpace(entry.Character), character) {
				own[entry.Slot] = true
			}
		}
	}
	return own
}
