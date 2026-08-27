package parser

import (
	"l4d2-manager-next/pkg/valve/vpk"
)

// GetVPKFileList 获取VPK文件中的所有文件路径列表
func GetVPKFileList(filePath string) ([]string, error) {
	opener := vpk.Single(filePath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return nil, err
	}

	var files []string
	for _, file := range archive.Files {
		name, err := DecodeVPKEntryName(file.Name())
		if err != nil {
			name = file.Name()
		}
		files = append(files, name)
	}

	return files, nil
}
