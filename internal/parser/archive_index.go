package parser

import (
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

	characterFiles []archivePathEntry
	weaponFiles    []archivePathEntry
	missionFiles   []*vpk.File
	contentTags    map[string]bool

	addonImageFile *vpk.File
	addonInfoFile  *vpk.File
	previewFile    *vpk.File
}

func buildArchivePathIndex(archive *vpk.Archive) archivePathIndex {
	index := archivePathIndex{
		characterFiles: make([]archivePathEntry, 0),
		weaponFiles:    make([]archivePathEntry, 0),
		missionFiles:   make([]*vpk.File, 0),
		contentTags:    make(map[string]bool),
	}

	for i := range archive.Files {
		file := &archive.Files[i]
		name := normalizeArchivePath(file.Name())
		entry := archivePathEntry{name: name}

		if strings.HasSuffix(name, ".bsp") {
			index.hasMap = true
		}
		if isMissionPath(name) {
			index.missionFiles = append(index.missionFiles, file)
		}
		isItem := collectContentTags(name, index.contentTags)
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

func normalizeArchivePath(name string) string {
	name = strings.TrimSpace(name)
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
		strings.HasSuffix(name, ".jpeg")
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
