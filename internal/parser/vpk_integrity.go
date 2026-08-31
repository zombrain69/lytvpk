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
	Path           string              `json:"path"`
	Name           string              `json:"name"`
	Valid          bool                `json:"valid"`
	TotalFiles     int                 `json:"totalFiles"`
	VerifiedFiles  int                 `json:"verifiedFiles"`
	AddonInfoFound bool                `json:"addonInfoFound"`
	AddonInfoValid bool                `json:"addonInfoValid"`
	Repairable     bool                `json:"repairable"`
	Issues         []VPKIntegrityIssue `json:"issues"`
}

// VPKAddonInfoRepairSummary explains which metadata survived a repair and
// which fields were derived because the source archive did not provide them.
type VPKAddonInfoRepairSummary struct {
	PreservedFields        []string `json:"preservedFields"`
	DerivedFields          []string `json:"derivedFields"`
	RecoveredTruncatedText bool     `json:"recoveredTruncatedText"`
}

// InspectVPKIntegrity checks the archive tree, every entry checksum and the
// root addoninfo.txt syntax that the game uses to accept an addon.
func InspectVPKIntegrity(filePath string) (VPKIntegrityReport, error) {
	filePath = filepath.Clean(strings.TrimSpace(filePath))
	report := VPKIntegrityReport{Path: filePath, Name: filepath.Base(filePath), Issues: make([]VPKIntegrityIssue, 0)}
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
		report.VerifiedFiles++
		if strings.EqualFold(path.Clean(entryName), "addoninfo.txt") {
			addonInfoFile = entry
			report.AddonInfoFound = true
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
	if field := findRepairField(root, "addonDescription"); field == nil || strings.TrimSpace(field.Value) == "" {
		if ensureRepairField(root, "addonDescription", "文件名："+fallbackTitle) {
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
