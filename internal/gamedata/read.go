package gamedata

import (
	"path/filepath"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"

	"vpk-manager/internal/parser"
)

// ReadGameFiles 按引擎挂载顺序（低优先级 → 高优先级）读取匹配 match 的本体文件内容。
//
// 用途：实体表生成器要读 `scripts/weapon_*.txt` / `scripts/melee/*.txt` 的**内容**
// （不只是路径）；W4 读 `.mdl` 材质表也会用到。高优先级挂载点覆盖低优先级，
// 与引擎实际加载顺序一致。
//
// 只读：不写任何游戏文件。单个挂载点损坏时跳过，不影响其余挂载点。
func ReadGameFiles(gameRoot string, match func(path string) bool) (map[string][]byte, error) {
	root, err := NormalizeGameRoot(gameRoot)
	if err != nil {
		return nil, err
	}

	out := make(map[string][]byte)
	for _, mount := range MountNames {
		pakPath := filepath.Join(root, mount, "pak01_dir.vpk")
		if !fileExists(pakPath) {
			continue
		}
		// 脚本内容在分卷（pak01_000.vpk …）里，必须用 Dir 打开才能读到数据；
		// Single 只能读目录树，取内容会报 "cannot request archive volume"。
		opener := vpk.Dir(strings.TrimSuffix(pakPath, "_dir.vpk"))
		archive, readErr := opener.ReadArchive()
		if readErr != nil {
			opener.Close()
			continue
		}
		for i := range archive.Files {
			name := archive.Files[i].Name()
			if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
				name = decoded
			}
			normalized := NormalizePath(name)
			if normalized == "" || !match(normalized) {
				continue
			}
			data, err := archive.Files[i].Bytes(opener)
			if err != nil {
				continue
			}
			out[normalized] = data // 后写的挂载点优先级更高
		}
		opener.Close()
	}
	return out, nil
}
