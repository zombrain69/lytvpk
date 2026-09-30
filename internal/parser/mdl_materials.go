package parser

import (
	"encoding/binary"
	"strings"
)

// Source 模型（.mdl）头部里的材质表（W4 通道 3）。
//
// 用途：作者把模型换成自己的名字时，路径上再也看不出它替换的是什么武器；
// 但模型头部仍然记着自己引用的材质名与搜索目录（cdtextures），这些名字通常
// 保留着本体特征（`v_rifle_ak47`、`w_eq_medkit`…）。把它们喂给"本体基名 token"通道，
// 就能把"只换贴图/换模型"的包绑回实体。
//
// 只读：只解析头部，不修改任何文件；解析失败一律返回 nil（降级，不影响既有标签）。

const (
	mdlHeaderMinSize = 220
	// mstudiotexture_t 在 PC 版是 64 字节：sznameindex/flags/used/unused1/material/clientmaterial + 10 个保留 int。
	mdlTextureRecordSize = 64
	mdlMaxTextures       = 64
	mdlMaxCDTextures     = 16
)

// ModelMaterialTokens 返回模型引用的材质名与搜索目录（去重、保序、限量）。
func ModelMaterialTokens(data []byte) []string {
	if len(data) < mdlHeaderMinSize {
		return nil
	}
	// 头部魔数 'IDST'（0x54534449）。
	if binary.LittleEndian.Uint32(data[0:4]) != 0x54534449 {
		return nil
	}

	numTextures := int(int32(binary.LittleEndian.Uint32(data[204:208])))
	textureIndex := int(int32(binary.LittleEndian.Uint32(data[208:212])))
	numCDTextures := int(int32(binary.LittleEndian.Uint32(data[212:216])))
	cdTextureIndex := int(int32(binary.LittleEndian.Uint32(data[216:220])))

	seen := make(map[string]struct{}, 16)
	tokens := make([]string, 0, 16)
	add := func(value string) {
		value = strings.Trim(strings.ToLower(strings.TrimSpace(value)), "/")
		value = strings.ReplaceAll(value, "\\", "/")
		if value == "" {
			return
		}
		if _, exists := seen[value]; exists {
			return
		}
		seen[value] = struct{}{}
		tokens = append(tokens, value)
	}

	// cdtextures：一组相对偏移（相对于 cdtextureindex 起点）指向的字符串。
	if numCDTextures > 0 && numCDTextures <= mdlMaxCDTextures {
		for i := 0; i < numCDTextures; i++ {
			offset := cdTextureIndex + i*4
			if offset+4 > len(data) {
				break
			}
			relative := int(int32(binary.LittleEndian.Uint32(data[offset : offset+4])))
			if value, ok := readMDLString(data, cdTextureIndex+relative); ok {
				add(value)
			}
		}
	}

	if numTextures <= 0 || numTextures > mdlMaxTextures {
		return tokens
	}
	for i := 0; i < numTextures; i++ {
		record := textureIndex + i*mdlTextureRecordSize
		if record+4 > len(data) {
			break
		}
		relative := int(int32(binary.LittleEndian.Uint32(data[record : record+4])))
		if value, ok := readMDLString(data, record+relative); ok {
			add(value)
		}
	}
	return tokens
}

// readMDLString 读取以 NUL 结尾的字符串（限制长度，避免损坏文件读出天量内容）。
func readMDLString(data []byte, offset int) (string, bool) {
	if offset <= 0 || offset >= len(data) {
		return "", false
	}
	end := offset
	for end < len(data) && data[end] != 0 && end-offset < 128 {
		end++
	}
	if end == offset {
		return "", false
	}
	return string(data[offset:end]), true
}
