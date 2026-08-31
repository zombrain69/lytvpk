package parser

import (
	"fmt"
	"io"
	"strings"

	"l4d2-manager-next/pkg/valve/vdf"
	"l4d2-manager-next/pkg/valve/vpk"
)

// ValidateVPKAddonInfo verifies the root addoninfo.txt syntax that the game
// uses when it rebuilds and saves the Add-ons list. A malformed file can still
// mount resources, so this check is intentionally performed before every
// root-directory enable operation.
func ValidateVPKAddonInfo(filePath string) error {
	opener := vpk.Single(filePath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return fmt.Errorf("无法读取 VPK: %w", err)
	}

	var addonInfoFile *vpk.File
	for index := range archive.Files {
		entry := &archive.Files[index]
		name, decodeErr := DecodeVPKEntryName(entry.Name())
		if decodeErr != nil {
			name = entry.Name()
		}
		name = strings.ReplaceAll(name, "\\", "/")
		if strings.EqualFold(name, "addoninfo.txt") {
			addonInfoFile = entry
			break
		}
	}
	if addonInfoFile == nil {
		return fmt.Errorf("缺少根目录 addoninfo.txt")
	}

	reader, err := addonInfoFile.Open(opener)
	if err != nil {
		return fmt.Errorf("无法读取 addoninfo.txt: %w", err)
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("无法读取 addoninfo.txt: %w", err)
	}
	content, err := DecodeVPKText(data)
	if err != nil {
		return fmt.Errorf("无法解码 addoninfo.txt: %w", err)
	}
	if err := validateAddonInfoContent(content); err != nil {
		return fmt.Errorf("addoninfo.txt 格式无效: %w", err)
	}
	return nil
}

func validateAddonInfoContent(content string) error {
	var root vdf.KeyValues
	if _, err := root.ReadFrom(strings.NewReader(content)); err != nil {
		return fmt.Errorf("Valve KeyValues 解析失败: %w", err)
	}
	if !strings.EqualFold(root.Key, "AddonInfo") {
		return fmt.Errorf("根节点必须是 AddonInfo，实际为 %q", root.Key)
	}
	if root.HasValue || root.FirstSubKey() == nil {
		return fmt.Errorf("AddonInfo 必须包含键值块")
	}
	return nil
}
