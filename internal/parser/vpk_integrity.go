package parser

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"l4d2-manager-next/pkg/valve/vdf"
	"l4d2-manager-next/pkg/valve/vpk"
)

type VPKIntegrityIssue struct {
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	Path       string `json:"path"`
	Message    string `json:"message"`
	Repairable bool   `json:"repairable"`
}

type VPKIntegrityReport struct {
	Path       string `json:"path"`
	Name       string `json:"name"`
	Valid      bool   `json:"valid"`
	TotalFiles int    `json:"totalFiles"`
	// ScanMode 说明这次是怎么校验的：
	//   VPKIntegrityScanIndex（默认）：只读目录表 + 每个条目末尾一个字节 + addoninfo.txt。
	//     能发现缺 addoninfo、语法错、重复路径、路径不安全、数据越界/被截断、分卷丢失。
	//   VPKIntegrityScanFull：逐条目把数据整包读一遍并核对 CRC —— 慢，耗时与包体成正比
	//     （真机 1.42GB 的包要 4 分钟），只在显式深度校验时使用。
	ScanMode string `json:"scanMode"`
	// VerifiedFiles 是通过校验的条目数：索引级 = 数据范围确实可读的条目数；
	// 整包级 = 连数据一起读完并核对过 CRC 的条目数。
	VerifiedFiles  int                 `json:"verifiedFiles"`
	AddonInfoFound bool                `json:"addonInfoFound"`
	AddonInfoValid bool                `json:"addonInfoValid"`
	Repairable     bool                `json:"repairable"`
	Issues         []VPKIntegrityIssue `json:"issues"`
}

const (
	// VPKIntegrityScanIndex 是默认的索引级校验（快，不读包体）。
	VPKIntegrityScanIndex = "index"
	// VPKIntegrityScanFull 是整包校验（逐条目读完 + CRC，慢）。
	VPKIntegrityScanFull = "full"
)

// vpkSelfArchive 是 VPK 目录表里"数据就在本文件（_dir.vpk / 单文件 VPK）"的标记。
// 与 vpk 库的 selfArchive 常量一致（库没有导出它）。
const vpkSelfArchive = 0x7fff

// VPKAddonInfoRepairSummary explains which metadata survived a repair and
// which fields were derived because the source archive did not provide them.
type VPKAddonInfoRepairSummary struct {
	PreservedFields        []string `json:"preservedFields"`
	DerivedFields          []string `json:"derivedFields"`
	RecoveredTruncatedText bool     `json:"recoveredTruncatedText"`
}

// InspectVPKIntegrity checks the archive tree, every entry checksum and the
// root addoninfo.txt syntax that the game uses to accept an addon.
// InspectVPKIntegrity 做一次**索引级**校验：读 VPK 目录表（索引本身、不是数据），
// 逐条目核对路径与数据范围，并把 addoninfo.txt 完整读出来验证语法。
//
// 为什么不是整包读：这条路径挂在"启用游戏内 Mod"之前的风险提示上
// （frontend vpk-risk-warning.js）。老实现对每个条目 io.Copy(io.Discard, reader)，
// 等于把整个 VPK 读一遍再核对 CRC：真机 1.42GB 的包实测 4 分 03 秒、
// 16.8MB 的包 1.3 秒，而"关闭游戏内开关"不做这一步 —— 于是启用比关闭慢几个数量级。
//
// 索引级校验改成"每个条目只读最后一个字节"：数据被截断 / 分卷丢失都会在这里报错，
// 成本从 O(包体) 降到 O(条目数)+每卷一次 Stat。需要逐字节 CRC 的场景用 InspectVPKIntegrityDeep。
func InspectVPKIntegrity(filePath string) (VPKIntegrityReport, error) {
	return inspectVPKIntegrity(filePath, false)
}

// inspectVPKIntegrityDeep 是整包校验：逐条目把数据读完并核对 CRC。
// 耗时与包体成正比（真机 1.42GB 冷盘 4 分钟），所以**不挂在任何界面路径上**：
// 目前只用于测试对照（证明索引级校验没有把该报的问题放过去），
// 将来要做显式的「深度校验」入口时再把它导出。
func inspectVPKIntegrityDeep(filePath string) (VPKIntegrityReport, error) {
	return inspectVPKIntegrity(filePath, true)
}

func inspectVPKIntegrity(filePath string, deep bool) (VPKIntegrityReport, error) {
	filePath = filepath.Clean(strings.TrimSpace(filePath))
	report := VPKIntegrityReport{
		Path:     filePath,
		Name:     filepath.Base(filePath),
		ScanMode: VPKIntegrityScanIndex,
		Issues:   make([]VPKIntegrityIssue, 0),
	}
	if deep {
		report.ScanMode = VPKIntegrityScanFull
	}
	if filePath == "" || filePath == "." {
		return report, fmt.Errorf("VPK 文件路径不能为空")
	}
	if !strings.EqualFold(filepath.Ext(filePath), ".vpk") {
		return report, fmt.Errorf("请选择 .vpk 文件")
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return report, fmt.Errorf("无法访问 VPK 文件: %w", err)
	}
	if info.IsDir() {
		return report, fmt.Errorf("请选择 VPK 文件，而不是文件夹")
	}

	opener := vpk.Single(filePath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "archive-read", Severity: "error", Message: fmt.Sprintf("VPK 目录读取失败: %v", err)})
		return report, nil
	}
	report.TotalFiles = len(archive.Files)
	if len(archive.Files) == 0 {
		report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "empty-archive", Severity: "error", Message: "VPK 内没有可用文件"})
	}

	seen := make(map[string]struct{}, len(archive.Files))
	var addonInfoFile *vpk.File
	// 数据范围核对：先纯算术统计每个数据卷的"末端点"，循环结束后按卷核对一次。
	// 逐条目做随机读在机械盘上要几百次寻道（真机 305 条 ≈ 3 秒），按卷核对只要每卷一次。
	volumes := make(map[int]*vpkVolumeRange)
	entryVolumes := make([][]int, len(archive.Files))
	for index := range archive.Files {
		entry := &archive.Files[index]
		entryName, decodeErr := DecodeVPKEntryName(entry.Name())
		if decodeErr != nil {
			report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "entry-name-encoding", Severity: "warning", Path: entry.Name(), Message: fmt.Sprintf("文件名无法按 GBK/ANSI 或 UTF-8 解码: %v", decodeErr)})
			entryName = entry.Name()
		}
		entryName = strings.ReplaceAll(entryName, "\\", "/")
		key := strings.ToLower(path.Clean(entryName))
		if _, exists := seen[key]; exists {
			report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "duplicate-entry", Severity: "error", Path: entryName, Message: "VPK 内存在重复文件路径，游戏可能随机使用其中一个"})
		}
		seen[key] = struct{}{}
		if !isSafeVPKIntegrityPath(entryName) {
			report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "unsafe-entry-path", Severity: "error", Path: entryName, Message: "文件路径包含绝对路径或 ..，不能安全解包修复"})
		}

		reader, openErr := entry.Open(opener)
		if openErr != nil {
			report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "entry-open", Severity: "error", Path: entryName, Message: fmt.Sprintf("文件数据无法读取: %v", openErr)})
			continue
		}
		if deep {
			_, copyErr := io.Copy(io.Discard, reader)
			closeErr := reader.Close()
			if copyErr != nil || closeErr != nil {
				checkErr := copyErr
				if checkErr == nil {
					checkErr = closeErr
				}
				report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "checksum", Severity: "error", Path: entryName, Message: fmt.Sprintf("文件校验失败: %v", checkErr)})
				continue
			}
		}
		report.VerifiedFiles++
		for _, chunk := range entry.DataLocation {
			archiveIndex := int(chunk.ArchiveIndex)
			probe := volumes[archiveIndex]
			if probe == nil {
				probe = &vpkVolumeRange{}
				volumes[archiveIndex] = probe
			}
			if end := uint64(chunk.EntryOffset) + uint64(chunk.EntryLength); end > probe.maxEnd {
				probe.maxEnd = end
				probe.lastEntry = entryName
				probe.lastFile = entry
			}
			entryVolumes[index] = append(entryVolumes[index], archiveIndex)
		}
		if strings.EqualFold(path.Clean(entryName), "addoninfo.txt") {
			addonInfoFile = entry
			report.AddonInfoFound = true
		}
	}

	// 按数据卷核对数据范围（索引级才做；整包校验已经逐字节读过，不需要）：
	//   - 分卷 VPK：偏移是卷内绝对偏移，直接和卷文件大小比，纯算术；
	//   - 单文件 VPK（L4D2 的 Mod 都是这种）：偏移相对数据区起点，拿不到库内部的
	//     fileOffset，所以只在"末端点最靠后的那一条"上读一个字节 —— 截断必然先打到它。
	volumeEntries := make(map[int]int, len(volumes))
	for _, refs := range entryVolumes {
		for _, ref := range refs {
			volumeEntries[ref]++
		}
	}
	if !deep {
		for archiveIndex, probe := range volumes {
			if probe.maxEnd == 0 {
				continue
			}
			truncated, detail := verifyVPKVolumeRange(opener, archiveIndex, probe)
			if !truncated {
				continue
			}
			report.Issues = append(report.Issues, VPKIntegrityIssue{
				Code:     "entry-data-range",
				Severity: "error",
				Path:     probe.lastEntry,
				Message:  fmt.Sprintf("文件数据不完整（越界或已被截断）: %s", detail),
			})
			// 这个卷上的条目全部不算"已校验"。
			report.VerifiedFiles -= volumeEntries[archiveIndex]
		}
	}

	if addonInfoFile == nil {
		report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "addoninfo-missing", Severity: "error", Path: "addoninfo.txt", Message: "缺少根目录 addoninfo.txt；游戏可能不会记录此 Mod", Repairable: true})
	} else {
		reader, openErr := addonInfoFile.Open(opener)
		if openErr != nil {
			report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "addoninfo-open", Severity: "error", Path: "addoninfo.txt", Message: fmt.Sprintf("无法读取 addoninfo.txt: %v", openErr)})
		} else {
			data, readErr := io.ReadAll(reader)
			closeErr := reader.Close()
			if readErr != nil || closeErr != nil {
				checkErr := readErr
				if checkErr == nil {
					checkErr = closeErr
				}
				report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "addoninfo-read", Severity: "error", Path: "addoninfo.txt", Message: fmt.Sprintf("无法完整读取 addoninfo.txt: %v", checkErr)})
			} else if content, decodeErr := DecodeVPKText(data); decodeErr != nil {
				report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "addoninfo-encoding", Severity: "error", Path: "addoninfo.txt", Message: fmt.Sprintf("addoninfo.txt 编码无法识别: %v", decodeErr)})
			} else if parseErr := validateAddonInfoContent(content); parseErr != nil {
				report.Issues = append(report.Issues, VPKIntegrityIssue{Code: "addoninfo-syntax", Severity: "error", Path: "addoninfo.txt", Message: fmt.Sprintf("addoninfo.txt 的 Valve KeyValues 语法无效: %v", parseErr), Repairable: true})
			} else {
				report.AddonInfoValid = true
			}
		}
	}

	report.Valid = len(report.Issues) == 0
	report.Repairable = false
	for _, issue := range report.Issues {
		if issue.Repairable {
			report.Repairable = true
			break
		}
	}
	if report.VerifiedFiles != report.TotalFiles {
		report.Repairable = false
	}
	for _, issue := range report.Issues {
		if issue.Severity == "error" && !issue.Repairable {
			report.Repairable = false
		}
	}
	return report, nil
}

// vpkVolumeRange 是一个数据卷上"数据末端点"的记录：
// maxEnd 是卷内最大末端偏移，lastEntry 是达到这个偏移的那一条（出问题时用它报位置），
// lastFile 则用于只读一次的末端探测。
type vpkVolumeRange struct {
	maxEnd    uint64
	lastEntry string
	lastFile  *vpk.File
}

// verifyVPKVolumeRange 核对该数据卷里的数据是否真的存在，返回 (是否被截断, 说明)。
//
// 索引级校验替代"整包读"的关键就在这：数据被截断 / 分卷丢失时，
// 位于最末端的那一条必然读不出来，而它前面的条目仍然可读。
// 所以只读**一个**字节（最末端条目的最后一个字节），而不是把整包读一遍
// —— 逐条目读在机械盘上要几百次寻道（真机 305 条约 3 秒），这里只要 1 次。
func verifyVPKVolumeRange(opener *vpk.Opener, archiveIndex int, probe *vpkVolumeRange) (bool, string) {
	volume, err := opener.Archive(archiveIndex)
	if err != nil {
		return true, fmt.Sprintf("数据分卷缺失: %v", err)
	}
	info, err := volume.Stat()
	if err != nil {
		return true, fmt.Sprintf("无法读取数据分卷信息: %v", err)
	}
	size := uint64(info.Size())
	if archiveIndex != vpkSelfArchive {
		// 分卷 VPK：偏移是卷内绝对偏移，纯算术即可判定，不用读。
		if probe.maxEnd > size {
			return true, fmt.Sprintf("数据超出文件 %d 字节", probe.maxEnd-size)
		}
		return false, ""
	}
	// 单文件 VPK：偏移相对"数据区起点"（库内部的 fileOffset），拿不到它，
	// 所以先做一次必要条件判断（数据区必然从文件里某个位置开始），
	// 真正的判定靠下面读最末端条目的最后一个字节——被截断时它必然先读不到。
	if probe.maxEnd > size {
		return true, fmt.Sprintf("数据超出文件 %d 字节", probe.maxEnd-size)
	}
	if probe.lastFile == nil || probe.lastFile.Size() <= 0 {
		return false, ""
	}
	// 注意**不调用 reader.Close()** —— vpk 库的 Close() 会把剩余数据读完并核对 CRC，
	// 那正是要避免的整包读。reader 不持有文件句柄（句柄归 opener），不 Close 不会泄漏。
	reader, openErr := probe.lastFile.Open(opener)
	if openErr != nil {
		return true, fmt.Sprintf("数据分卷不可读: %v", openErr)
	}
	readerAt, ok := reader.(io.ReaderAt)
	if !ok {
		return false, ""
	}
	var tail [1]byte
	if _, readErr := readerAt.ReadAt(tail[:], int64(probe.lastFile.Size()-1)); readErr != nil {
		return true, readErr.Error()
	}
	return false, ""
}

func isSafeVPKIntegrityPath(entryName string) bool {
	entryName = strings.ReplaceAll(entryName, "\\", "/")
	if entryName == "" || strings.HasPrefix(entryName, "/") || filepath.VolumeName(entryName) != "" {
		return false
	}
	clean := path.Clean(entryName)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}

// BuildRepairedAddonInfo returns a valid AddonInfo block while retaining all
// recoverable metadata from the original text. Well-formed Valve KeyValues are
// cloned as-is (including unknown and nested fields); malformed files are read
// with a tolerant lexer so a truncated description does not discard the other
// metadata. Missing title/description values are derived from fallbackTitle.
func BuildRepairedAddonInfo(content, fallbackTitle string) string {
	repaired, _ := BuildRepairedAddonInfoWithSummary(content, fallbackTitle)
	return repaired
}

// BuildRepairedAddonInfoWithSummary is the metadata-preserving repair entry
// point used by the app layer. The plain BuildRepairedAddonInfo wrapper keeps
// the parser API convenient for callers that only need the generated text.
func BuildRepairedAddonInfoWithSummary(content, fallbackTitle string) (string, VPKAddonInfoRepairSummary) {
	return BuildRepairedAddonInfoWithMetadata(content, fallbackTitle, nil)
}

// BuildRepairedAddonInfoWithMetadata repairs AddonInfo while using trusted
// external metadata only as fallbacks. Existing non-empty fields always win;
// callers can therefore merge Workshop metadata without overwriting author
// supplied values already present in the archive.
func BuildRepairedAddonInfoWithMetadata(content, fallbackTitle string, metadata map[string]string) (string, VPKAddonInfoRepairSummary) {
	root := parseRepairAddonInfo(content)
	if root == nil || !strings.EqualFold(root.Key, "AddonInfo") {
		root = &repairKVNode{Key: "AddonInfo"}
	}
	summary := VPKAddonInfoRepairSummary{
		PreservedFields: collectRepairFields(root, ""),
		DerivedFields:   make([]string, 0),
	}

	fallbackTitle = strings.TrimSpace(fallbackTitle)
	if fallbackTitle == "" {
		fallbackTitle = "未命名 VPK Mod"
	}
	if metadata != nil {
		if metadataTitle := strings.TrimSpace(metadata["addontitle"]); metadataTitle != "" {
			fallbackTitle = metadataTitle
		}
	}
	for _, field := range []struct {
		key   string
		value string
	}{
		{key: "addonSteamAppID", value: "550"},
		{key: "addontitle", value: fallbackTitle},
		{key: "addonversion", value: "1.0"},
	} {
		if ensureRepairField(root, field.key, field.value) {
			summary.DerivedFields = append(summary.DerivedFields, field.key)
		}
	}
	for _, field := range []struct {
		key   string
		value string
	}{
		{key: "addonauthor", value: strings.TrimSpace(metadata["addonauthor"])},
		{key: "addonURL0", value: strings.TrimSpace(metadata["addonURL0"])},
	} {
		if field.value != "" && ensureRepairField(root, field.key, field.value) {
			summary.DerivedFields = append(summary.DerivedFields, field.key)
		}
	}
	if field := findRepairField(root, "addonDescription"); field == nil || strings.TrimSpace(field.Value) == "" {
		description := strings.TrimSpace(metadata["addonDescription"])
		if description == "" {
			description = "文件名：" + fallbackTitle
		}
		if ensureRepairField(root, "addonDescription", description) {
			summary.DerivedFields = append(summary.DerivedFields, "addonDescription")
		}
	}
	summary.RecoveredTruncatedText = repairTreeRecovered(root)
	return serializeRepairAddonInfo(root), summary
}

// repairKVNode is intentionally separate from vdf.KeyValues because the
// recovery parser must represent an unterminated quoted token without failing.
type repairKVNode struct {
	Key       string
	Value     string
	Cond      string
	HasValue  bool
	Recovered bool
	Children  []*repairKVNode
}

type repairToken struct {
	kind         byte // s=string, q=quoted, {, }, 0=EOF
	value        string
	unterminated bool
}

type repairLexer struct{ reader *bufio.Reader }

func newRepairLexer(content string) *repairLexer {
	return &repairLexer{reader: bufio.NewReader(strings.NewReader(strings.TrimPrefix(content, "\ufeff")))}
}

// next delegates tokenization to the project's Valve KeyValues tokenizer. It
// only adds recovery for the tokenizer's one useful failure mode here:
// unterminated quoted text at EOF.
func (l *repairLexer) next() repairToken {
	for {
		raw, kind, err := vdf.ReadToken(l.reader)
		if len(raw) == 0 && err != nil {
			return repairToken{}
		}
		switch kind {
		case vdf.TokenSpace, vdf.TokenComment:
			continue
		case vdf.TokenOpenBrace:
			return repairToken{kind: '{'}
		case vdf.TokenCloseBrace:
			return repairToken{kind: '}'}
		case vdf.TokenQuoted:
			value := raw
			if strings.HasPrefix(value, "\"") {
				value = value[1:]
			}
			if strings.HasSuffix(value, "\"") {
				value = value[:len(value)-1]
				return repairToken{kind: 'q', value: decodeRepairEscapes(value)}
			}
			return repairToken{kind: 'q', value: recoverRepairQuotedValue(value), unterminated: true}
		case vdf.TokenString, vdf.TokenCond:
			return repairToken{kind: 's', value: raw}
		default:
			if err != nil {
				return repairToken{}
			}
		}
	}
}

func parseRepairAddonInfo(content string) *repairKVNode {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	var parsed vdf.KeyValues
	if _, err := parsed.ReadFrom(strings.NewReader(content)); err == nil && strings.EqualFold(parsed.Key, "AddonInfo") {
		return cloneRepairVDF(&parsed)
	}

	l := newRepairLexer(content)
	first := l.next()
	if first.kind == 0 {
		return nil
	}
	root := &repairKVNode{Key: first.value}
	if first.kind == '}' || first.kind == '{' {
		return &repairKVNode{Key: "AddonInfo"}
	}
	if !parseRepairNodeValue(l, root) {
		return root
	}
	return root
}

func parseRepairNodeValue(l *repairLexer, node *repairKVNode) bool {
	tok := l.next()
	switch tok.kind {
	case '{':
		for {
			key := l.next()
			if key.kind == 0 || key.kind == '}' {
				break
			}
			if key.kind != 's' && key.kind != 'q' {
				continue
			}
			child := &repairKVNode{Key: key.value}
			if !parseRepairNodeValue(l, child) {
				continue
			}
			node.Children = append(node.Children, child)
		}
		return true
	case 'q', 's':
		node.Value = tok.value
		node.HasValue = true
		node.Recovered = tok.unterminated
		return true
	default:
		return false
	}
}

func cloneRepairVDF(src *vdf.KeyValues) *repairKVNode {
	if src == nil {
		return nil
	}
	dst := &repairKVNode{Key: src.Key, Value: src.Value, Cond: src.Cond, HasValue: src.HasValue}
	for child := src.FirstSubKey(); child != nil; child = child.NextSubKey() {
		dst.Children = append(dst.Children, cloneRepairVDF(child))
	}
	return dst
}

func findRepairField(root *repairKVNode, key string) *repairKVNode {
	if root == nil {
		return nil
	}
	for _, child := range root.Children {
		if strings.EqualFold(child.Key, key) && child.HasValue {
			return child
		}
	}
	return nil
}

func ensureRepairField(root *repairKVNode, key, value string) bool {
	if field := findRepairField(root, key); field != nil && strings.TrimSpace(field.Value) != "" {
		return false
	}
	for _, child := range root.Children {
		if strings.EqualFold(child.Key, key) {
			child.Value, child.HasValue = value, true
			child.Children = nil
			return true
		}
	}
	root.Children = append(root.Children, &repairKVNode{Key: key, Value: value, HasValue: true})
	return true
}

func collectRepairFields(node *repairKVNode, prefix string) []string {
	if node == nil {
		return nil
	}
	fields := make([]string, 0)
	for _, child := range node.Children {
		name := child.Key
		if prefix != "" {
			name = prefix + "." + name
		}
		if child.HasValue && strings.TrimSpace(child.Value) != "" {
			fields = append(fields, name)
		}
		fields = append(fields, collectRepairFields(child, name)...)
	}
	return fields
}

func repairTreeRecovered(node *repairKVNode) bool {
	if node == nil {
		return false
	}
	if node.Recovered {
		return true
	}
	for _, child := range node.Children {
		if repairTreeRecovered(child) {
			return true
		}
	}
	return false
}

func serializeRepairAddonInfo(root *repairKVNode) string {
	var b strings.Builder
	writeRepairNode(&b, root, 0)
	return b.String()
}

func writeRepairNode(b *strings.Builder, node *repairKVNode, indent int) {
	if node == nil {
		return
	}
	b.WriteString(strings.Repeat("\t", indent))
	b.WriteByte('"')
	b.WriteString(escapeRepairValue(node.Key))
	b.WriteByte('"')
	if node.HasValue {
		b.WriteString("\t\t\"")
		b.WriteString(escapeRepairValue(node.Value))
		b.WriteString("\"\n")
		return
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("\t", indent))
	b.WriteString("{\n")
	for _, child := range node.Children {
		writeRepairNode(b, child, indent+1)
	}
	b.WriteString(strings.Repeat("\t", indent))
	b.WriteString("}\n")
}

func escapeRepairValue(value string) string {
	return strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\r", "\\r", "\n", "\\n", "\t", "\\t").Replace(value)
}

func decodeRepairEscapes(value string) string {
	return strings.NewReplacer("\\n", "\n", "\\r", "\r", "\\t", "\t", "\\\\", "\\", "\\\"", "\"").Replace(value)
}

func recoverRepairQuotedValue(value string) string {
	lines := strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n")
	kept := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "}" {
			break
		}
		kept = append(kept, line)
	}
	return strings.TrimSpace(decodeRepairEscapes(strings.Join(kept, "\n")))
}
