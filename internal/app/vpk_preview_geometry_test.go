package app

import (
	"encoding/binary"
	"math"
	"strings"
	"testing"

	"l4d2-manager-next/pkg/valve/bsp"
)

// 造一个最小可解析的 studiohdr_t：只填我们会读的字段，验证解析与信息卡。
func buildStudioHeaderFixture() []byte {
	data := make([]byte, 240)
	binary.LittleEndian.PutUint32(data[0:4], 0x54534449) // IDST
	binary.LittleEndian.PutUint32(data[4:8], uint32(48)) // version
	binary.LittleEndian.PutUint32(data[8:12], uint32(1234))
	copy(data[12:76], []byte("w_rifle_ak47.mdl"))
	binary.LittleEndian.PutUint32(data[76:80], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[104:108], math.Float32bits(-10))
	binary.LittleEndian.PutUint32(data[116:120], math.Float32bits(10))
	binary.LittleEndian.PutUint32(data[156:160], uint32(37))  // numbones
	binary.LittleEndian.PutUint32(data[188:192], uint32(12))  // numlocalseq
	binary.LittleEndian.PutUint32(data[204:208], uint32(3))   // numtextures
	binary.LittleEndian.PutUint32(data[232:236], uint32(2))   // numbodyparts
	// numlocalseq 之后是字符表等，不属于解析范围
	return data
}

func TestParseStudioHeaderAndModelPreview(t *testing.T) {
	header, ok := parseStudioHeader(buildStudioHeaderFixture())
	if !ok {
		t.Fatal("合法 IDST 头应能解析")
	}
	if header.Name != "w_rifle_ak47.mdl" || header.NumBones != 37 || header.NumBodyParts != 2 || header.NumLocalSeq != 12 {
		t.Fatalf("解析结果不对: %#v", header)
	}
	if got := math.Abs(float64(header.HullMax[0] - header.HullMin[0])); got != 20 {
		t.Fatalf("hull 尺寸 = %v，期望 20", got)
	}

	report := buildModelPreview(buildStudioHeaderFixture(), "models/w_rifle_ak47.mdl")
	for _, want := range []string{"w_rifle_ak47.mdl", "骨骼：37", "体块(bodyparts)：2", "序列：12"} {
		if !strings.Contains(report, want) {
			t.Fatalf("信息卡缺少 %q：\n%s", want, report)
		}
	}

	// 非模型数据：给出可读说明而不是报错。
	broken := buildModelPreview([]byte("not a model"), "x.mdl")
	if !strings.Contains(broken, "无法解析模型头部") {
		t.Fatalf("损坏模型应给说明：%s", broken)
	}
}

// 造一个最小 VBSP 头（只有标识与版本）验证"信息卡不为空、且坏数据不 panic"。
func TestBuildMapPreviewHandlesTruncatedBSP(t *testing.T) {
	data := make([]byte, 8)
	copy(data[0:4], []byte("VBSP"))
	binary.LittleEndian.PutUint32(data[4:8], uint32(20))
	report := buildMapPreview(data, "maps/c1m1_hotel.bsp")
	if !strings.Contains(report, "BSP 版本：20") {
		t.Fatalf("应显示版本：%s", report)
	}
	if !strings.Contains(report, "解析未完成") && !strings.Contains(report, "实体总数") {
		t.Fatalf("截断的地图应给出说明或部分信息：%s", report)
	}
	if strings.Contains(report, "panic") {
		t.Fatalf("坏数据不应把 panic 泄露到输出：%s", report)
	}
	if out := buildMapPreview([]byte("nope"), "x.bsp"); !strings.Contains(out, "不是有效的 VBSP") {
		t.Fatalf("非 BSP 数据应给说明：%s", out)
	}
}

func TestEntityClassHistogramSortsByCount(t *testing.T) {
	entities := []bsp.Entity{
		{Pairs: []bsp.EPair{{Key: "classname", Value: "prop_static"}}},
		{Pairs: []bsp.EPair{{Key: "classname", Value: "prop_static"}}},
		{Pairs: []bsp.EPair{{Key: "classname", Value: "info_player_start"}}},
		{Pairs: []bsp.EPair{{Key: "origin", Value: "0 0 0"}}},
	}
	list := entityClassHistogram(entities, 3)
	if len(list) != 3 || list[0].class != "prop_static" || list[0].count != 2 {
		t.Fatalf("直方图排序不对: %#v", list)
	}
	if list[2].class != "（无 classname）" {
		t.Fatalf("没有 classname 的实体应归到兜底分类: %#v", list)
	}
}

func TestHumanBytes(t *testing.T) {
	cases := map[int64]string{512: "512 B", 2048: "2.0 KB", 5 * 1024 * 1024: "5.0 MB"}
	for size, want := range cases {
		if got := humanBytes(size); got != want {
			t.Fatalf("humanBytes(%d) = %q，期望 %q", size, got, want)
		}
	}
}

// L4D2 的 BSP v21 第三方解析器不认识：自研的 lump 目录 + 实体统计必须仍然给出实打实的数据。
func TestBSPLumpDirectoryAndEntityParsing(t *testing.T) {
	entityText := strings.Join([]string{
		"{",
		"\"classname\" \"worldspawn\"",
		"\"skyname\" \"sky_l4d_rural02_hdr\"",
		"}",
		"{",
		"\"classname\" \"prop_static\"",
		"\"model\" \"models/props/crate.mdl\"",
		"}",
		"{",
		"\"classname\" \"prop_static\"",
		"\"model\" \"models/props/barrel.mdl\"",
		"}",
		"{",
		"\"classname\" \"info_player_start\"",
		"}",
	}, "\n")
	entityLump := []byte(entityText)
	// L4D2 的 v21：ident + version + 额外 4 字节 + lump[64]
	const headerSize = 12
	const lumpSize = 16
	const dirSize = headerSize + 64*lumpSize
	data := make([]byte, dirSize+len(entityLump))
	copy(data[0:4], []byte("VBSP"))
	binary.LittleEndian.PutUint32(data[4:8], uint32(21)) // L4D2 常见版本
	binary.LittleEndian.PutUint32(data[headerSize+0*lumpSize:headerSize+0*lumpSize+4], uint32(dirSize))
	binary.LittleEndian.PutUint32(data[headerSize+0*lumpSize+4:headerSize+0*lumpSize+8], uint32(len(entityLump)))
	copy(data[dirSize:], entityLump)

	lumps := readBSPLumps(data)
	if len(lumps) != 64 {
		t.Fatalf("应读出 64 个 lump，实际 %d", len(lumps))
	}
	entityLumpInfo := bspLumpByIndex(lumps, 0)
	if entityLumpInfo == nil || entityLumpInfo.Length != uint32(len(entityLump)) {
		t.Fatalf("实体 lump 解析不对: %#v", entityLumpInfo)
	}
	total, classes := parseBSPEntities(data, *entityLumpInfo)
	if total != 4 {
		t.Fatalf("实体总数 = %d，期望 4", total)
	}
	if len(classes) != 3 || classes[0].class != "prop_static" || classes[0].count != 2 {
		t.Fatalf("classname 直方图不对: %#v", classes)
	}
	if name := (bspLumpInfo{Index: 40}).DisplayName(); name != "PAKFILE" {
		t.Fatalf("lump 名称表不对: %q", name)
	}
	if name := (bspLumpInfo{Index: 63}).DisplayName(); name != "Lump #63" {
		t.Fatalf("未登记 lump 应回退为编号: %q", name)
	}

	// 整张信息卡：v21 也要有版本 / lump / 实体统计，即使完整解析失败。
	report := buildMapPreview(data, "maps/azcity_end.bsp")
	for _, want := range []string{"BSP 版本：21", "Lump：64 个", "实体总数：4", "prop_static"} {
		if !strings.Contains(report, want) {
			t.Fatalf("信息卡缺少 %q：\n%s", want, report)
		}
	}
}
