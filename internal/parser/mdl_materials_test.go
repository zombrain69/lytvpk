package parser

import (
	"encoding/binary"
	"testing"
)

// buildTestMDL 造一个最小可解析的 .mdl 头部：1 个 cdtexture + 2 个材质名。
func buildTestMDL(cdTextures []string, textureNames []string) []byte {
	data := make([]byte, 2048)
	binary.LittleEndian.PutUint32(data[0:4], 0x54534449) // 'IDST'
	binary.LittleEndian.PutUint32(data[4:8], 49)         // version

	// 字符串区放在头部之后。
	// 字符串区放在记录区之后，避免互相覆盖。
	cursor := 1024
	writeString := func(value string) int {
		offset := cursor
		copy(data[cursor:], value)
		cursor += len(value) + 1
		return offset
	}

	// cdtextures 数组
	cdIndex := 240
	for i, value := range cdTextures {
		offset := writeString(value)
		binary.LittleEndian.PutUint32(data[cdIndex+i*4:cdIndex+i*4+4], uint32(int32(offset-cdIndex)))
	}

	// texture 记录区
	textureIndex := 240 + len(cdTextures)*4
	for i, name := range textureNames {
		record := textureIndex + i*mdlTextureRecordSize
		offset := writeString(name)
		binary.LittleEndian.PutUint32(data[record:record+4], uint32(int32(offset-record)))
	}

	binary.LittleEndian.PutUint32(data[204:208], uint32(len(textureNames)))
	binary.LittleEndian.PutUint32(data[208:212], uint32(textureIndex))
	binary.LittleEndian.PutUint32(data[212:216], uint32(len(cdTextures)))
	binary.LittleEndian.PutUint32(data[216:220], uint32(cdIndex))
	return data
}

func TestModelMaterialTokensReadsHeader(t *testing.T) {
	data := buildTestMDL(
		[]string{"models/weapons/v_models/v_rifle_ak47"},
		[]string{"v_rifle_ak47", "v_rifle_ak47_slide"},
	)
	tokens := ModelMaterialTokens(data)
	want := map[string]bool{
		"models/weapons/v_models/v_rifle_ak47": true,
		"v_rifle_ak47":                         true,
		"v_rifle_ak47_slide":                   true,
	}
	if len(tokens) != len(want) {
		t.Fatalf("token 数量不符：%v", tokens)
	}
	for _, token := range tokens {
		if !want[token] {
			t.Fatalf("出现意外 token：%q（全部 %v）", token, tokens)
		}
	}
}

func TestModelMaterialTokensRejectsBrokenInput(t *testing.T) {
	if got := ModelMaterialTokens(nil); got != nil {
		t.Fatalf("空输入应返回 nil，实际 %v", got)
	}
	if got := ModelMaterialTokens([]byte("this is not a mdl file at all, but it is long enough to pass the size check ................")); got != nil {
		t.Fatalf("魔数不符应返回 nil，实际 %v", got)
	}
	broken := buildTestMDL([]string{"models/x"}, []string{"y"})
	binary.LittleEndian.PutUint32(broken[204:208], 0xFFFFFF) // 超量材质数
	if got := ModelMaterialTokens(broken); len(got) != 1 {
		t.Fatalf("异常材质数应只保留可解析的 cdtexture，实际 %v", got)
	}
}
