package parser

import (
	"path/filepath"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"
)

// archivePathEntry keeps the normalized directory-index path together with its
// source entry.  Building this index never opens or extracts a file from the
// VPK; it only examines the directory entries already read by the VPK parser.
type archivePathEntry struct {
	name string
}

// archivePathIndex is the per-VPK evidence collected in one pass over
// archive.Files.  The individual parsers consume only their candidate entries,
// rather than repeatedly walking the whole archive.
type archivePathIndex struct {
	hasMap bool

	// 结构摘要：给"分组推导 / 外部智能体"用的 VPK 内部结构信息。
	// 这些数据在同一遍遍历里顺带统计，不需要额外读取 VPK。
	topDirs     map[string]int
	fileCount   int
	totalSize   int64
	samplePaths []string
	// 更紧凑的"替换目标"摘要（如 props_interiors/medicalcabinet02）：
	// 给智能体判断"覆盖了哪些资源"，同时避免清单体积失控。
	resourceTargets []string
	targetSeen      map[string]struct{}
	// 作者/套件命名空间（models/<作者>/<套件>）的出现次数：同一个套件的多个 VPK 会共享它。
	sourceRoots map[string]int

	characterFiles  []archivePathEntry
	weaponFiles     []archivePathEntry
	missionFiles    []*vpk.File
	contentTags     map[string]bool
	voiceCharacters map[string]bool
	subjectEvidence map[string]subjectEvidence
	xdrSlots        map[string]xdrSlotEvidence
	hasXDRMarker    bool

	addonImageFile *vpk.File
	addonInfoFile  *vpk.File
	previewFile    *vpk.File
}

func buildArchivePathIndex(archive *vpk.Archive) archivePathIndex {
	index := archivePathIndex{
		topDirs:         make(map[string]int),
		targetSeen:      make(map[string]struct{}, 16),
		sourceRoots:     make(map[string]int),
		characterFiles:  make([]archivePathEntry, 0),
		weaponFiles:     make([]archivePathEntry, 0),
		missionFiles:    make([]*vpk.File, 0),
		contentTags:     make(map[string]bool),
		voiceCharacters: make(map[string]bool),
		subjectEvidence: make(map[string]subjectEvidence),
		xdrSlots:        make(map[string]xdrSlotEvidence),
	}

	for i := range archive.Files {
		file := &archive.Files[i]
		name := normalizeArchivePath(file.Name())
		entry := archivePathEntry{name: name}

		index.fileCount++
		index.totalSize += int64(file.Size())
		index.collectStructure(name)

		if strings.HasSuffix(name, ".bsp") {
			index.hasMap = true
		}
		if isMissionPath(name) {
			index.missionFiles = append(index.missionFiles, file)
		}
		isItem := collectContentTags(name, index.contentTags)
		collectSubjectEvidence(name, isItem, &index)
		collectXDRSlotEvidence(name, &index)
		if character := detectVoiceCharacter(name); character != "" {
			index.voiceCharacters[character] = true
		}
		if isCharacterAssetPath(name) {
			index.characterFiles = append(index.characterFiles, entry)
		}
		if isWeaponAssetPath(name, isItem) {
			index.weaponFiles = append(index.weaponFiles, entry)
		}

		if index.addonImageFile == nil && name == "addonimage.jpg" {
			index.addonImageFile = file
		}
		if index.addonInfoFile == nil && name == "addoninfo.txt" {
			index.addonInfoFile = file
		}
		if index.previewFile == nil && isPreviewImagePath(name) {
			index.previewFile = file
		}
	}

	return index
}

// 结构摘要的规模上限：清单要被大模型读进上下文，路径条数必须克制。
const (
	structureSamplePathLimit     = 5
	structureResourceTargetLimit = 6
	structureSamplePathMaxRunes  = 100
	// 一个 VPK 最多记录 6 个套件命名空间（清单要被读进上下文，必须克制）。
	structureResourceRootLimit = 6
)

// collectStructure 统计顶层目录、条目数、体积，并挑选有代表性的资源路径。
// 只在同一遍遍历里做，避免为了导出清单再读一次 VPK。
func (index *archivePathIndex) collectStructure(name string) {
	if name == "" {
		return
	}
	if slash := strings.Index(name, "/"); slash > 0 {
		index.topDirs[name[:slash]]++
	} else {
		index.topDirs["（根）"]++
	}
	if isStructureNoisePath(name) {
		return
	}
	if len(index.samplePaths) < structureSamplePathLimit {
		index.samplePaths = append(index.samplePaths, truncateRunes(name, structureSamplePathMaxRunes))
	}
	if target := structureResourceTarget(name); target != "" && len(index.resourceTargets) < structureResourceTargetLimit {
		if _, seen := index.targetSeen[target]; !seen {
			index.targetSeen[target] = struct{}{}
			index.resourceTargets = append(index.resourceTargets, target)
		}
	}
	if root := structureSuiteNamespace(name); root != "" {
		index.sourceRoots[root]++
	}
}

// structureSuiteNamespace 识别"作者/套件命名空间"：
//
//	materials/models/<作者>/<套件>/…  → <作者>/<套件>
//	models/<作者>/<套件>/…            → <作者>/<套件>
//
// 官方根（weapons/survivors/w_models/v_models/infected/props… 等）不算 —— 它们下面共享的是
// "同一个游戏对象的多个替换"，属于互斥候选，而不是同一套件的配套模块。
// 套件段明显是文件名残留（例如 `rescue_pilot_01.vvd`）时也丢弃。
func structureSuiteNamespace(name string) string {
	parts := strings.Split(name, "/")
	modelsIndex := -1
	for i, part := range parts {
		if part == "models" {
			modelsIndex = i
			break
		}
	}
	if modelsIndex >= 0 && len(parts) >= modelsIndex+3 {
		root := parts[modelsIndex+1]
		suite := parts[modelsIndex+2]
		if root != "" && suite != "" &&
			!isOfficialResourceRoot(root) && !isGenericSuiteSegment(suite) && !looksLikeAssetFileName(suite) {
			return root + "/" + suite
		}
		return ""
	}
	// 没有 models 段时（真实例子：死库水套件在 materials 下且层级不统一）：
	//   materials/sikushui/mo/white_2.vtf   → sikushui
	//   materials/qkl/mo/sikushui/waitao.vtf → sikushui
	// 取 materials 之后**第一个长度 ≥4、非通用的目录段**作为套件候选片段，
	// 这样不同层级的同名套件目录也能收敛到同一个值。
	if len(parts) < 3 || parts[0] != "materials" {
		return ""
	}
	for _, segment := range parts[1 : len(parts)-1] {
		if segment == "" || looksLikeAssetFileName(segment) {
			continue
		}
		// 一旦走进官方/通用资源树（props / vgui / models / particle…），后面的段都是
		// 游戏对象名（crates、hud、crosshair…），不能再当套件命名空间 —— 直接放弃。
		if isOfficialResourceRoot(segment) || isGenericSuiteSegment(segment) {
			return ""
		}
		if len([]rune(segment)) >= 4 {
			return segment
		}
	}
	return ""
}

// officialResourceRoots 是游戏自带资源树的根：这些目录下的多个替换是"互斥候选"，
// 不是"同一套件的配套模块"。
var officialResourceRoots = map[string]struct{}{
	"weapons": {}, "survivors": {}, "w_models": {}, "v_models": {},
	"infected": {}, "zombie": {}, "zombie_classic": {}, "humans": {}, "player": {},
	"props": {}, "props_unique": {}, "props_junk": {}, "props_vehicles": {},
	"props_equipment": {}, "props_interiors": {}, "props_urban": {}, "props_office": {},
	"props_doors": {}, "props_placeable": {}, "props_debris": {}, "props_downtown": {},
	"props_foliage": {}, "props_street": {}, "props_misc": {}, "props_industrial": {},
	"props_energy": {}, "props_crates": {}, "deadbodies": {}, "error": {},
	"lights": {}, "detail": {}, "overlay": {}, "shared": {}, "hybridphysx": {},
	"xdreanims": {},
	// 地图/临时容器：里面的目录是地图道具，不是套件（真实误报：
	// `static/nmrih_officeboxes1` 把两个地图 Mod 凑成了"套装"）。
	"static": {}, "tmp_mod": {}, "graffiti": {}, "brick": {},
}

// genericSuiteSegments 是出现频率极高、没有区分度的目录名（materials 分支用）。
var genericSuiteSegments = map[string]struct{}{
	"models": {}, "materials": {}, "weapons": {}, "w_models": {}, "v_models": {},
	"props": {}, "props_unique": {}, "survivors": {}, "infected": {}, "zombie": {},
	"vgui": {}, "ui": {}, "gui": {}, "hud": {}, "particle": {}, "particles": {},
	"effects": {}, "decals": {}, "sprites": {}, "skybox": {}, "console": {},
	"detail": {}, "overlay": {}, "lights": {}, "tools": {}, "toolstextures": {},
	"scripts": {}, "sound": {}, "sounds": {}, "shared": {}, "common": {}, "default": {},
	"base": {}, "body": {}, "face": {}, "hair": {}, "cloth": {}, "clothes": {},
	"texture": {}, "textures": {}, "icon": {}, "icons": {}, "temp": {}, "tmp": {},
	// 地图道具容器（真实误报：`ill_hanger/props`）。
	"static": {}, "tmp_mod": {},
}

func isOfficialResourceRoot(segment string) bool {
	_, official := officialResourceRoots[segment]
	return official
}

func isGenericSuiteSegment(segment string) bool {
	_, generic := genericSuiteSegments[segment]
	return generic
}

// looksLikeAssetFileName 判断这一段是不是"具体文件名"（而不是目录名）。
func looksLikeAssetFileName(segment string) bool {
	if !strings.Contains(segment, ".") {
		return false
	}
	switch filepath.Ext(segment) {
	case ".vmt", ".vtf", ".mdl", ".vvd", ".vtx", ".phy", ".wav", ".mp3", ".txt",
		".res", ".png", ".jpg", ".jpeg", ".gif", ".ani", ".bsp", ".vbsp", ".nav", ".pcf":
		return true
	}
	return false
}

func truncateRunes(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

// structureResourceTarget 把内部路径压缩成"替换目标"，例如
// models/props_interiors/medicalcabinet02.mdl → props_interiors/medicalcabinet02
// materials/honkai3/theresa/body.vmt          → honkai3/theresa
func structureResourceTarget(name string) string {
	parts := strings.Split(name, "/")
	if len(parts) < 2 {
		return ""
	}
	rest := parts[1:]
	last := strings.TrimSuffix(rest[len(rest)-1], filepath.Ext(rest[len(rest)-1]))
	segments := rest[:len(rest)-1]
	if len(segments) > 2 {
		segments = segments[len(segments)-2:]
	}
	values := append(append([]string(nil), segments...), last)
	return strings.Trim(strings.Join(values, "/"), "/")
}

// isStructureNoisePath 过滤掉对"判断替换目标"没有帮助的条目（addoninfo / 预览图）。
func isStructureNoisePath(name string) bool {
	switch {
	case name == "addoninfo.txt", name == "addonimage.jpg":
		return true
	case strings.HasPrefix(name, "addonimage"), strings.HasPrefix(name, "addonpreview"):
		return true
	}
	return false
}

func normalizeArchivePath(name string) string {
	name = strings.TrimSpace(name)
	if decoded, err := DecodeVPKEntryName(name); err == nil {
		name = decoded
	}
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "./")
	return strings.ToLower(name)
}

func isMissionPath(name string) bool {
	return strings.HasSuffix(name, ".txt") &&
		(strings.Contains(name, "missions/") || strings.Contains(name, "mission"))
}

func isPreviewImagePath(name string) bool {
	return strings.HasSuffix(name, ".png") ||
		strings.HasSuffix(name, ".jpg") ||
		strings.HasSuffix(name, ".jpeg") ||
		strings.HasSuffix(name, ".gif")
}

func isCharacterAssetPath(name string) bool {
	if strings.HasPrefix(name, "resource/ui/") || strings.HasPrefix(name, "scripts/") || strings.HasSuffix(name, ".res") {
		return false
	}

	characterRoots := []string{
		"models/survivors/",
		"models/infected/",
		"models/zombie/",
		"materials/models/survivors/",
		"materials/models/infected/",
		"materials/models/zombie/",
		"materials/survivors/",
		"materials/infected/",
		"materials/zombie/",
		"sound/player/survivor/",
		"sound/player/infected/",
	}
	for _, root := range characterRoots {
		if strings.HasPrefix(name, root) {
			return true
		}
	}

	// A few well-formed sound/model mods use a custom subdirectory.  Retain the
	// old broad compatibility rule only under content roots, so UI resource
	// files mentioning a survivor never turn the whole VPK into a character mod.
	if !(strings.HasPrefix(name, "models/") || strings.HasPrefix(name, "materials/models/") || strings.HasPrefix(name, "sound/player/")) {
		return false
	}
	return strings.Contains(name, "survivor") || strings.Contains(name, "infected") || strings.Contains(name, "zombie")
}

func isWeaponAssetPath(name string, isItem bool) bool {
	if isItem {
		return false
	}

	weaponRoots := []string{
		"models/weapons/",
		"models/v_models/weapons/",
		"models/w_models/weapons/",
		"materials/models/weapons/",
		"materials/models/v_models/weapons/",
		"materials/weapons/",
		"scripts/weapons/",
		"sound/weapons/",
	}
	for _, root := range weaponRoots {
		if strings.HasPrefix(name, root) {
			return true
		}
	}

	// Some stock models are stored in models/w_models/<weapon>/ rather than
	// models/w_models/weapons/. Do not treat the whole directory as a weapon:
	// it also contains medkits, explosives and other carryable equipment.
	if strings.HasPrefix(name, "models/w_models/") || strings.HasPrefix(name, "materials/models/w_models/") {
		return weaponPathTag(name) != ""
	}

	// Some Source weapon scripts live directly in scripts/ as weapon_*.txt.
	return strings.HasPrefix(name, "scripts/weapon_")
}
