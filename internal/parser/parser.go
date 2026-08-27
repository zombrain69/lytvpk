package parser

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"
)

// ParseVPKFile 解析VPK文件的主入口函数
// 输入文件路径,返回解析后的VPKFile结构
func ParseVPKFile(filePath string) (*VPKFile, error) {
	return parseVPKFile(filePath, true)
}

// ParseVPKFileMetadata parses all list and tagging metadata but defers image
// decoding. Directory scans use it so large addon libraries do not retain every
// preview as a Base64 string; callers can fetch a preview later on demand.
func ParseVPKFileMetadata(filePath string) (*VPKFile, error) {
	return parseVPKFile(filePath, false)
}

func parseVPKFile(filePath string, includePreview bool) (*VPKFile, error) {
	// 打开VPK文件
	opener := vpk.Single(filePath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return nil, err
	}

	// 创建基础文件信息
	vpkFile := &VPKFile{
		Name:          filepath.Base(filePath),
		Path:          filePath,
		PrimaryTag:    "其他", // 默认为"其他"
		SecondaryTags: make([]string, 0),
		Chapters:      make(map[string]ChapterInfo),
	}

	// 第一步: 一次性建立目录索引；它只读取 VPK 的条目名称，不解压资源。
	index := buildArchivePathIndex(archive)
	vpkType := determineVPKType(index)

	// addoninfo is part of normal metadata. Preview image decoding is optional:
	// it can allocate several MiB per VPK and is only needed when the UI displays
	// a particular card or detail dialog.
	if includePreview {
		ExtractVPKResources(opener, index, vpkFile, filePath)
	} else {
		ExtractVPKMetadata(opener, index, vpkFile)
	}

	secondaryTags := make(map[string]bool)
	chapters := make(map[string]ChapterInfo)

	// 第二步：根据类型进行专门的检测
	switch vpkType {
	case "地图":
		ProcessMapVPK(opener, index, vpkFile, secondaryTags, chapters)
	case "人物":
		ProcessCharacterVPK(index, vpkFile, secondaryTags)
	case "武器":
		ProcessWeaponVPK(index, vpkFile, secondaryTags)
	default:
		// 其他类型
		vpkFile.PrimaryTag = "其他"
		vpkFile.SecondaryTags = []string{}
		vpkFile.Chapters = make(map[string]ChapterInfo)
		// 注意：不在这里 return，让它继续执行提取预览图的逻辑
	}

	// 主分类保持地图 > 人物 > 武器的既有优先级；但混合包仍可带有另一类
	// 已确认资源。把这些作为附加二级标签后，前端便能筛选“人物 + AK47”、
	// “地图 + HUD”等组合，而不会把 Mod 的主分类改错。
	collectSupplementaryTypeTags(index, vpkType, secondaryTags)

	// 一级类型表示 Mod 的主内容；额外的 UI、道具、环境等资源仍保留为
	// 中文二级标签。纯“其他”内容无需调整前端即可按这些标签筛选；混合包
	// 也会保留其附加内容证据。
	mergeTagSet(secondaryTags, index.contentTags)
	delete(secondaryTags, vpkFile.PrimaryTag)

	// 设置最终的标签
	vpkFile.SecondaryTags = sortedTagSet(secondaryTags)

	vpkFile.Chapters = chapters

	// 检查自定义标签并覆盖
	if pTag, sTags, _, ok := ParseFilenameTags(vpkFile.Name); ok {
		// 只有当有明确的自定义标签结构时才覆盖
		// 允许 PrimaryTag 为空字符串（如果用户删除了）?
		// 但通常 [Primary,Secondary] 格式意味着至少有一个为空?
		// 如果 [] 空的，len(tagParts)==1 ("") -> primaryTag=""
		vpkFile.PrimaryTag = strings.TrimSpace(pTag)
		vpkFile.SecondaryTags = UniqueTagsExcluding(sTags, vpkFile.PrimaryTag)
	}

	return vpkFile, nil
}

// UniqueTagsExcluding trims and de-duplicates tags while preserving their first
// appearance.  Comparison is case-insensitive so manually entered variants do
// not create duplicate filter options; excluded is normally the primary tag.
func UniqueTagsExcluding(tags []string, excluded ...string) []string {
	excludedSet := make(map[string]struct{}, len(excluded))
	for _, tag := range excluded {
		if key := strings.ToLower(CanonicalTag(tag)); key != "" {
			excludedSet[key] = struct{}{}
		}
	}
	seen := make(map[string]struct{}, len(tags))
	result := make([]string, 0, len(tags))
	for _, tag := range tags {
		tag = CanonicalTag(tag)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, skip := excludedSet[key]; skip {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

// CanonicalTag maps historical aliases to one display/filter label.
// "榴弹" was used by older versions while the category is "榴弹发射器";
// keeping only the latter prevents semantically duplicate tags.
func CanonicalTag(tag string) string {
	tag = strings.TrimSpace(tag)
	switch strings.ToLower(tag) {
	case "榴弹":
		return "榴弹发射器"
	default:
		return tag
	}
}

func collectSupplementaryTypeTags(index archivePathIndex, primaryTag string, secondaryTags map[string]bool) {
	if primaryTag != "人物" {
		collectCharacterTags(index, secondaryTags)
	}
	if primaryTag != "武器" {
		collectWeaponPathTags(index, secondaryTags)
	}
}

var tagRegex = regexp.MustCompile(`^(_)?\[(.*?)\](.*)$`)

// tagSanitizeRegex 匹配会导致文件名非法或破坏标签解析的字符：
// Windows 文件名禁止字符 (< > : " / \ | ? *) 以及标签解析相关字符 ([ ] , +)
var tagSanitizeRegex = regexp.MustCompile(`[<>:"/\\|?*\[\],+]`)

// SanitizeTag 清理标签中的特殊字符，使其能安全地写入文件名
// 将禁止字符替换为下划线并修剪两端空白
func SanitizeTag(tag string) string {
	cleaned := tagSanitizeRegex.ReplaceAllString(tag, "_")
	return strings.TrimSpace(cleaned)
}

// ParseFilenameTags 解析文件名中的标签
// 返回: primaryTag, secondaryTags, realNameWithoutTags, hasTags
func ParseFilenameTags(filename string) (string, []string, string, bool) {
	matches := tagRegex.FindStringSubmatch(filename)
	if matches == nil {
		return "", nil, filename, false
	}

	// matches[1] 是前缀 "_"
	// matches[2] 是标签内容
	// matches[3] 是剩余文件名

	// hiddenPrefix := matches[1]
	tagsContent := matches[2]
	// realName := hiddenPrefix + matches[3]

	tagParts := strings.FieldsFunc(tagsContent, func(r rune) bool {
		return r == ',' || r == '+'
	})
	var primaryTag string
	var secondaryTags []string

	if len(tagParts) > 0 {
		primaryTag = strings.TrimSpace(tagParts[0])
		for _, t := range tagParts[1:] {
			t = strings.TrimSpace(t)
			if t != "" {
				secondaryTags = append(secondaryTags, t)
			}
		}
	}

	return primaryTag, secondaryTags, matches[1] + matches[3], true
}

// GetPrimaryTags 获取所有主要标签
func GetPrimaryTags() []string {
	return []string{"地图", "人物", "武器", "其他"}
}

// GetSecondaryTags 获取指定主标签下的所有二级标签
// 从给定的VPK文件列表中提取二级标签
func GetSecondaryTags(vpkFiles []VPKFile, primaryTag string) []string {
	tagSet := make(map[string]bool)
	for _, vpkFile := range vpkFiles {
		if primaryTag == "" || vpkFile.PrimaryTag == primaryTag {
			for _, tag := range vpkFile.SecondaryTags {
				if canonical := CanonicalTag(tag); canonical != "" && canonical != primaryTag {
					tagSet[canonical] = true
				}
			}
		}
	}

	return sortedTagSet(tagSet)
}

func sortedTagSet(tagSet map[string]bool) []string {
	values := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		if normalized := CanonicalTag(tag); normalized != "" {
			values = append(values, normalized)
		}
	}
	sort.Strings(values)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, tag := range values {
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func mergeTagSet(destination map[string]bool, source map[string]bool) {
	for tag := range source {
		if canonical := CanonicalTag(tag); canonical != "" {
			destination[canonical] = true
		}
	}
}

// ExtractPreviewImage 从VPK中提取预览图并转换为Base64
// 采用三级查找策略:
// 1. 优先查找 addonimage.jpg (Steam 创意工坊标准)
// 2. 查找内部其他预览图
// 3. 查找外部同名 .jpg 文件
func ExtractPreviewImage(opener *vpk.Opener, archive *vpk.Archive, vpkFilePath string) string {
	// ========== 优先级 1: 查找 addonimage.jpg ==========
	// Steam 创意工坊的标准缩略图文件名
	addonImageFile := findFileInArchive(archive, "addonimage.jpg")
	if addonImageFile != nil {
		base64Data := readAndEncodeImage(opener, addonImageFile)
		if base64Data != "" {
			return base64Data
		}
	}

	// ========== 优先级 2: 查找其他预览图 (原有逻辑) ==========
	// 常见的预览图路径模式
	previewPatterns := []string{
		".jpg",
		".jpeg",
		".png",
		"materials/vgui/maps/menu/",
		"materials/vgui/loadingscreen",
		"resource/overviews/",
	}

	var previewFile *vpk.File

	// 遍历所有文件，查找预览图
	for i := range archive.Files {
		file := &archive.Files[i]
		filename := file.Name()
		if decoded, err := DecodeVPKEntryName(filename); err == nil {
			filename = decoded
		}
		filename = strings.ToLower(filename)

		// 检查是否匹配预览图模式
		for _, pattern := range previewPatterns {
			if strings.Contains(filename, pattern) {
				// 确保是图片文件
				if strings.HasSuffix(filename, ".png") ||
					strings.HasSuffix(filename, ".jpg") ||
					strings.HasSuffix(filename, ".jpeg") ||
					strings.HasSuffix(filename, ".gif") {
					previewFile = file
					break
				}
			}
		}

		if previewFile != nil {
			break
		}
	}

	if previewFile != nil {
		if base64Data := readAndEncodeImage(opener, previewFile); base64Data != "" {
			return base64Data
		}
	}

	// ========== 优先级 3: 查找外部同名图片文件 (.jpg, .png, .jpeg) ==========
	// 例如: xxx.vpk -> xxx.jpg
	basePath := strings.TrimSuffix(vpkFilePath, filepath.Ext(vpkFilePath))
	exts := []string{".jpg", ".png", ".jpeg", ".gif"}

	for _, ext := range exts {
		externalPath := basePath + ext
		if fileExists(externalPath) {
			if base64Data := readExternalImageFile(externalPath); base64Data != "" {
				return base64Data
			}
		}
	}

	return ""
}

// findFileInArchive 在 VPK 中查找指定文件名（不区分大小写）
func findFileInArchive(archive *vpk.Archive, targetName string) *vpk.File {
	targetLower := strings.ToLower(targetName)
	for i := range archive.Files {
		file := &archive.Files[i]
		name := file.Name()
		if decoded, err := DecodeVPKEntryName(name); err == nil {
			name = decoded
		}
		if strings.ToLower(name) == targetLower {
			return file
		}
	}
	return nil
}

// readAndEncodeImage 读取 VPK 内部文件并编码为 Base64
func readAndEncodeImage(opener *vpk.Opener, file *vpk.File) string {
	reader, err := file.Open(opener)
	if err != nil {
		return ""
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return ""
	}

	return encodeImageToBase64(data)
}

// readExternalImageFile 读取外部图片文件并编码为 Base64
func readExternalImageFile(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return ""
	}

	return encodeImageToBase64(data)
}

// encodeImageToBase64 将图片数据编码为 Base64 Data URL
func encodeImageToBase64(data []byte) string {
	// 只读取图片头验证格式和尺寸，不完整解码像素。详情预览可能是高分辨率
	// 图片，image.Decode 会额外分配整张像素图，导致打开详情时出现明显卡顿。
	_, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return ""
	}

	// 如果是VTF格式（Valve纹理格式），我们暂时跳过
	// 因为需要特殊的VTF解码器
	if format != "png" && format != "jpeg" && format != "gif" {
		return ""
	}

	// 将图片数据转换为Base64
	base64Str := base64.StdEncoding.EncodeToString(data)

	// 根据格式添加Data URL前缀
	var dataURL string
	switch format {
	case "png":
		dataURL = "data:image/png;base64," + base64Str
	case "jpeg":
		dataURL = "data:image/jpeg;base64," + base64Str
	case "gif":
		dataURL = "data:image/gif;base64," + base64Str
	default:
		return ""
	}

	return dataURL
}

// fileExists 检查文件是否存在
func fileExists(filePath string) bool {
	_, err := os.Stat(filePath)
	return err == nil
}

// ExtractVPKResources 一次性提取VPK中的预览图和addoninfo信息
// 优化性能：只遍历一次archive，同时查找预览图和addoninfo.txt
func ExtractVPKResources(opener *vpk.Opener, index archivePathIndex, vpkFile *VPKFile, vpkFilePath string) {
	// 索引在 ParseVPKFile 的唯一一次 archive.Files 遍历中已收集这些条目。
	vpkFile.PreviewImage = extractPreviewImageFromFiles(opener, index.addonImageFile, index.previewFile, vpkFilePath)

	ExtractVPKMetadata(opener, index, vpkFile)
}

// ExtractVPKMetadata reads the lightweight addoninfo metadata already located
// by the archive index. It deliberately does not open image payloads.
func ExtractVPKMetadata(opener *vpk.Opener, index archivePathIndex, vpkFile *VPKFile) {
	parseAddonInfoFromFile(opener, index.addonInfoFile, vpkFile)
}

// ExtractVPKPreviewImage reopens a single archive to read only the preview
// requested by the UI. It keeps normal scans metadata-only while retaining the
// existing three-tier preview selection behavior.
func ExtractVPKPreviewImage(filePath string) (string, error) {
	opener := vpk.Single(filePath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return "", err
	}
	index := buildArchivePathIndex(archive)
	return extractPreviewImageFromFiles(opener, index.addonImageFile, index.previewFile, filePath), nil
}

// extractPreviewImageFromFiles 从找到的文件中提取预览图
func extractPreviewImageFromFiles(opener *vpk.Opener, addonImageFile, previewFile *vpk.File, vpkFilePath string) string {
	// 优先级1: 外部同名图片文件
	basePath := strings.TrimSuffix(vpkFilePath, filepath.Ext(vpkFilePath))
	exts := []string{".jpg", ".png", ".jpeg", ".gif"}
	for _, ext := range exts {
		externalPath := basePath + ext
		if fileExists(externalPath) {
			if base64Data := readExternalImageFile(externalPath); base64Data != "" {
				return base64Data
			}
		}
	}

	// 优先级2: addonimage.jpg
	if addonImageFile != nil {
		if base64Data := readAndEncodeImage(opener, addonImageFile); base64Data != "" {
			return base64Data
		}
	}

	// 优先级3: 其他预览图
	if previewFile != nil {
		if base64Data := readAndEncodeImage(opener, previewFile); base64Data != "" {
			return base64Data
		}
	}

	return ""
}

// parseAddonInfoFromFile 从addoninfo.txt文件解析信息
func parseAddonInfoFromFile(opener *vpk.Opener, addonInfoFile *vpk.File, vpkFile *VPKFile) {
	// 初始化默认值
	vpkFile.Title = ""
	vpkFile.Author = ""
	vpkFile.Version = ""
	vpkFile.Desc = ""

	if addonInfoFile == nil {
		return
	}

	// 读取文件内容
	reader, err := addonInfoFile.Open(opener)
	if err != nil {
		return
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return
	}

	// VPK 文本没有独立的 charset 标记；按 BOM、UTF-8、GBK/ANSI 顺序解码。
	content, decodeErr := DecodeVPKText(data)
	if decodeErr != nil {
		return
	}
	lines := strings.Split(content, "\n")

	// 解析每一行
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 跳过空行、注释
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		var key, value string

		// 检查键是否被引用 "key" "value"
		if strings.HasPrefix(line, "\"") {
			// 找到键的结束位置 (从第1个字符后开始查找第一个引号)
			keyEnd := strings.Index(line[1:], "\"")
			if keyEnd == -1 {
				continue
			}
			keyEnd++ // 调整索引，因为我们切片了

			key = line[1:keyEnd]

			// 找到值的开始位置 (必须在键之后)
			if keyEnd+1 >= len(line) {
				continue
			}
			remainder := line[keyEnd+1:]

			valStart := strings.Index(remainder, "\"")
			if valStart == -1 {
				continue
			}

			// 找到值的结束位置
			valEnd := strings.LastIndex(remainder, "\"")
			// 确保 valEnd 严格大于 valStart
			if valEnd <= valStart {
				continue
			}

			value = remainder[valStart+1 : valEnd]

		} else {
			// key "value" 模式 (遗留/宽松)
			// 确保不以 { 或 } 开头，这些是结构标记
			if strings.HasPrefix(line, "{") || strings.HasPrefix(line, "}") {
				continue
			}

			// 找到值的开始位置
			valStart := strings.Index(line, "\"")
			if valStart == -1 {
				continue
			}

			key = strings.TrimSpace(line[:valStart])

			valEnd := strings.LastIndex(line, "\"")
			if valEnd <= valStart {
				continue
			}

			value = line[valStart+1 : valEnd]
		}

		// 根据键设置对应的值
		switch strings.ToLower(key) {
		case "addontitle":
			vpkFile.Title = value
		case "addonauthor":
			vpkFile.Author = value
		case "addonversion":
			vpkFile.Version = value
		case "addondescription":
			vpkFile.Desc = value
		case "addonurl0":
			vpkFile.AddonURL0 = value
		}
	}
}
