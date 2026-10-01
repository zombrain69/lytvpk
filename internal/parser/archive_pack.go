package parser

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"strings"
)

// 工坊里有一类条目：扩展名是 .vpk，内容其实是压缩包。
//
// 真机样本（addons\workshop\，4269 个 .vpk 里 2 个）：
//   - 3558049615.vpk（22.1MB，91 条）—— Left4Neko 工具包：bin/left4neko.dll、
//     l4n_magic_converter.exe、*.bat、plugins/*.h
//   - 3787239937.vpk（6.5MB，76 条）—— DIY 教程/素材包：DIY教程/…/SKILL.md、
//     references/*.md、scripts/*.py、*.txt
//
// 这类条目是作者**特意**打成压缩包的插件/工具/教程，游戏既不会加载它，它也不该进
// addonlist.txt；玩家的用法是"关掉它、自己去文件夹解压"。所以它们不是"异常 Mod"，
// 而是一类需要单独报出来的特殊包 —— 之前统一按"VPK 解析失败"处理，会在界面上弹红色
// 「解析错误」，属于误报。

// ArchivePackKind 是这类条目的类别（按压缩包内的文件树估计）。
type ArchivePackKind string

const (
	// ArchivePackToolkit 工具/插件包：bin/ 目录、*.dll / *.exe / *.bat。
	ArchivePackToolkit ArchivePackKind = "toolkit"
	// ArchivePackModBundle Mod 压缩包：里面装着 .vpk（解压后放回 addons）。
	ArchivePackModBundle ArchivePackKind = "mod-bundle"
	// ArchivePackDocs 教程/素材包：*.md / *.txt / 教程路径 / 图片素材。
	ArchivePackDocs ArchivePackKind = "docs"
	// ArchivePackOther 其它压缩包（或没能列出条目）。
	ArchivePackOther ArchivePackKind = "archive"
)

// ArchivePackInfo 描述一个"其实是压缩包"的 .vpk。
type ArchivePackInfo struct {
	Path   string          `json:"path"`
	Format string          `json:"format"`
	Kind   ArchivePackKind `json:"kind"`
	Label  string          `json:"label"`
	// Note 是一句给用户/智能体看的"这东西怎么用"：这类包一律不参与 addonlist。
	Note string `json:"note"`
	// EntryCount 仅在能列出条目时给出（当前只有 zip）。
	EntryCount int `json:"entryCount"`
	// Evidence 是最多 3 条判定依据（关键条目路径）。
	Evidence []string `json:"evidence"`
}

// 容器格式常量：只读文件头就能判断的那几种。
// 这是**全仓唯一**的文件头表 —— app 侧的拖入/解包（sniffDropImportContainer）与
// 这里的扫描分类都走 SniffContainer，避免两份魔术数字表各自漂移。
const (
	ContainerUnknown = "unknown"
	ContainerVPK     = "vpk"
	ContainerZIP     = "zip"
	ContainerRAR     = "rar"
	Container7z      = "7z"
	ContainerGzip    = "gzip"
	ContainerBzip2   = "bzip2"
	ContainerXz      = "xz"
	ContainerCab     = "cab"
)

// archiveMagicFormats 按文件头判断容器格式。顺序有意义：先长后短。
var archiveMagicFormats = []struct {
	magic  []byte
	format string
}{
	{[]byte{0x34, 0x12, 0xaa, 0x55}, ContainerVPK},
	{[]byte("Rar!\x1a\x07\x01\x00"), "rar"},
	{[]byte("Rar!\x1a\x07\x00"), "rar"},
	{[]byte("7z\xbc\xaf\x27\x1c"), "7z"},
	{[]byte{'P', 'K', 0x03, 0x04}, "zip"},
	{[]byte{'P', 'K', 0x05, 0x06}, "zip"},
	{[]byte{'P', 'K', 0x07, 0x08}, "zip"},
	{[]byte{0x1f, 0x8b}, "gzip"},
	{[]byte("BZh"), "bzip2"},
	{[]byte{0xfd, '7', 'z', 'X', 'Z', 0x00}, "xz"},
	{[]byte("MSCF"), "cab"},
}

// SniffContainer 只读前 8 字节判断容器格式。
// readable=false 表示文件头读不到（被独占占用、权限不足等）——调用方不要据此拒绝文件。
func SniffContainer(filePath string) (container string, readable bool) {
	handle, err := os.Open(filePath)
	if err != nil {
		return ContainerUnknown, false
	}
	defer handle.Close()

	var head [8]byte
	count, _ := io.ReadFull(handle, head[:])
	if count <= 0 {
		return ContainerUnknown, true
	}
	if format := archiveFormatFromHeader(head[:count]); format != "" {
		return format, true
	}
	return ContainerUnknown, true
}

// ArchiveFormatForFile 只读文件头判断它是不是压缩包；不是则返回空字符串。
func ArchiveFormatForFile(filePath string) (string, error) {
	container, readable := SniffContainer(filePath)
	if !readable {
		return "", fmt.Errorf("无法读取文件头: %s", filePath)
	}
	if container == ContainerVPK || container == ContainerUnknown {
		return "", nil
	}
	return container, nil
}

func archiveFormatFromHeader(head []byte) string {
	for _, candidate := range archiveMagicFormats {
		if bytes.HasPrefix(head, candidate.magic) {
			return candidate.format
		}
	}
	return ""
}

// DescribeArchivePack 判断这个文件是不是"扩展名是 .vpk、实际是压缩包"，并按包内文件树
// 估计它属于哪一类特殊包。第二个返回值是 false 表示它不是压缩包（调用方应走原来的错误路径）。
//
// 只有 zip 会做文件树分类（stdlib 能直接读中央目录，成本可忽略）；rar/7z 等只报格式 ——
// 本仓的 rar/7z 条目列表在 app 包里，parser 不能反向依赖它。
func DescribeArchivePack(filePath string) (ArchivePackInfo, bool) {
	info := ArchivePackInfo{Path: filePath, Evidence: []string{}}
	format, err := ArchiveFormatForFile(filePath)
	if err != nil || format == "" {
		return info, false
	}
	info.Format = format
	info.Kind = ArchivePackOther

	if format != "zip" {
		info.Label, info.Note = archivePackLabel(ArchivePackOther)
		return info, true
	}
	names, listErr := zipEntryNames(filePath)
	if listErr != nil {
		// 打不开的 zip 仍按"压缩包"报，但不猜类别。
		info.Label, info.Note = archivePackLabel(ArchivePackOther)
		info.Note = fmt.Sprintf("%s（压缩包内容无法列出: %v）", info.Note, listErr)
		return info, true
	}
	info.EntryCount = len(names)
	info.Kind, info.Evidence = classifyArchiveTree(names)
	info.Label, info.Note = archivePackLabel(info.Kind)
	return info, true
}

func zipEntryNames(filePath string) ([]string, error) {
	reader, err := zip.OpenReader(filePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	names := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		name := strings.TrimSpace(strings.ReplaceAll(entry.Name, "\\", "/"))
		if name == "" || strings.HasSuffix(name, "/") {
			continue // 目录条目不参与分类
		}
		names = append(names, name)
	}
	return names, nil
}

// classifyArchiveTree 按文件树判断类别，返回类别与最多 3 条依据。
//
// 规则保持"可预测"优先，只按存在性判断，不搞加权打分（打分出问题时没人能一眼看出为什么）：
//  1. 有 .vpk → Mod 压缩包：这包的主要内容就是 Mod（解压后放回 addons）
//  2. 有可执行物（bin/ 顶层、*.dll / *.exe / *.bat / *.cmd / *.ps1）→ 工具/插件包：
//     这类包的用法是"按说明解压安装"，文档是它的附件
//  3. 有文档/素材（*.md / *.txt / *.pdf / 教程·tutorial·docs 路径 / 图片与设计文件）→ 教程/素材包
//  4. 其余 → 压缩包
func classifyArchiveTree(names []string) (ArchivePackKind, []string) {
	var vpkEvidence, toolkitEvidence, docsEvidence []string

	for _, name := range names {
		lower := strings.ToLower(name)
		ext := strings.ToLower(path.Ext(lower))
		base := path.Base(lower)

		switch {
		case ext == ".vpk":
			vpkEvidence = appendEvidence(vpkEvidence, name)
		case archivePackToolkitExtension(ext) || strings.HasPrefix(lower, "bin/"):
			toolkitEvidence = appendEvidence(toolkitEvidence, name)
		case archivePackDocsName(lower, ext, base):
			docsEvidence = appendEvidence(docsEvidence, name)
		}
	}

	switch {
	case len(vpkEvidence) > 0:
		return ArchivePackModBundle, vpkEvidence
	case len(toolkitEvidence) > 0:
		return ArchivePackToolkit, toolkitEvidence
	case len(docsEvidence) > 0:
		return ArchivePackDocs, docsEvidence
	default:
		return ArchivePackOther, []string{}
	}
}

func archivePackToolkitExtension(ext string) bool {
	switch ext {
	case ".dll", ".exe", ".bat", ".cmd", ".ps1", ".jar":
		return true
	default:
		return false
	}
}

func archivePackDocsName(lower, ext, base string) bool {
	if base == "skill.md" || base == "readme.md" || base == "readme.txt" {
		return true
	}
	if strings.Contains(lower, "教程") || strings.Contains(lower, "tutorial") ||
		strings.Contains(lower, "/docs/") || strings.Contains(lower, "说明") {
		return true
	}
	switch ext {
	case ".md", ".txt", ".pdf", ".rtf", ".png", ".jpg", ".jpeg", ".webp",
		".psd", ".aseprite", ".blend", ".sai", ".clip":
		return true
	default:
		return false
	}
}

// appendEvidence 收集判定依据，最多留 3 条。
func appendEvidence(existing []string, name string) []string {
	if len(existing) >= 3 {
		return existing
	}
	return append(existing, name)
}

func archivePackLabel(kind ArchivePackKind) (string, string) {
	const noAddonList = "游戏不会加载它，也不参与 addonlist.txt"
	switch kind {
	case ArchivePackToolkit:
		return "工具/插件包", "工坊作者特意打成压缩包的插件/工具（首次安装或自动更新用）。" +
			noAddonList + "；按包内说明解压到对应目录后使用，不需要放进 addons 让游戏加载。"
	case ArchivePackModBundle:
		return "Mod 压缩包", "压缩包里装着 .vpk。" + noAddonList +
			"；解压后把里面的 .vpk 放进 addons（工坊来源放 addons\\workshop）再刷新列表。"
	case ArchivePackDocs:
		return "教程/素材包", "教程、素材或说明文档。" + noAddonList + "；用解压工具打开查看即可。"
	default:
		return "压缩包", "扩展名是 .vpk，实际是压缩包。" + noAddonList + "；需要时用解压工具打开。"
	}
}
