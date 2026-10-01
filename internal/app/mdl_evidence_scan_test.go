package app

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildMDLFixture 造一个最小 .mdl 头部：cdtexture + 材质名（与 parser 的单测同构）。
func buildMDLFixture(cdTextures []string, textureNames []string) []byte {
	data := make([]byte, 2048)
	binary.LittleEndian.PutUint32(data[0:4], 0x54534449) // 'IDST'
	binary.LittleEndian.PutUint32(data[4:8], 49)

	cursor := 1024
	writeString := func(value string) int {
		offset := cursor
		copy(data[cursor:], value)
		cursor += len(value) + 1
		return offset
	}
	cdIndex := 240
	for i, value := range cdTextures {
		offset := writeString(value)
		binary.LittleEndian.PutUint32(data[cdIndex+i*4:cdIndex+i*4+4], uint32(int32(offset-cdIndex)))
	}
	textureIndex := 240 + len(cdTextures)*4
	for i, name := range textureNames {
		record := textureIndex + i*64
		offset := writeString(name)
		binary.LittleEndian.PutUint32(data[record:record+4], uint32(int32(offset-record)))
	}
	binary.LittleEndian.PutUint32(data[204:208], uint32(len(textureNames)))
	binary.LittleEndian.PutUint32(data[208:212], uint32(textureIndex))
	binary.LittleEndian.PutUint32(data[212:216], uint32(len(cdTextures)))
	binary.LittleEndian.PutUint32(data[216:220], uint32(cdIndex))
	return data
}

// TestScanUsesModelMaterialTableForRenamedModels 覆盖 W4 通道 3：
// 模型被改名成作者自己的名字（路径看不出替换目标）时，靠模型头部的材质名把实体找回来。
func TestScanUsesModelMaterialTableForRenamedModels(t *testing.T) {
	addonsDir := filepath.Join(t.TempDir(), "left4dead2", "addons")
	if err := os.MkdirAll(addonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	vpkPath := filepath.Join(addonsDir, "renamed_rifle.vpk")
	mdl := buildMDLFixture(
		[]string{"models/weapons/v_models/w_rifle_ak47"},
		[]string{"w_rifle_ak47", "custom_slide"},
	)
	writeTestVPK(t, vpkPath, map[string][]byte{
		"models/mysuite/my_cool_rifle.mdl": mdl,
	})

	app := &App{rootDir: addonsDir}
	app.processVPKFileWithCache(vpkPath, nil)
	cached := app.mustCachedVPKFile(t, vpkPath)

	if !containsInheritedTag(cached.SecondaryTags, "AK47") {
		t.Fatalf("应通过模型材质表识别出 AK47，实际 %v", cached.SecondaryTags)
	}
	found := false
	for _, item := range cached.TagEvidence {
		// 命中的 token 可能是整段（`w_rifle_ak47`）或拆分后的子片段（`ak47`）。
		if item.Tag == "AK47" && strings.HasPrefix(item.Rule, "mdl:") {
			found = true
		}
	}
	if !found {
		t.Fatalf("应记录 mdl 通道证据：%+v", cached.TagEvidence)
	}
}
