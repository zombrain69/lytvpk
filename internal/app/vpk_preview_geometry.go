package app

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	"vpk-manager/internal/parser"

	"l4d2-manager-next/pkg/valve/bsp"
)

// 模型（.mdl）与地图（.bsp）的"信息卡"预览。
//
// 为什么不做 3D 渲染：Source 1 的模型要 VVD/VTX + D3D/OpenGL 才能画出网格，
// 地图还要 VIS/光照与材质管线——那是另一个量级的工程（VPKEdit 也依赖 sourcepp 的完整解析栈）。
// 这里先把"看一眼就知道这是什么"的信息给全：模型头部的骨骼/材质/序列，地图的实体/几何/贴图/静态道具。
// 真正的 3D 预览留作后续（见 docs/toolbox/vpk-preview.md 的说明）。

// studioHeaderInfo 是 studiohdr_t 里我们会用到的字段（PC 版偏移）。
type studioHeaderInfo struct {
	ID           string
	Version      int32
	Checksum     int32
	Name         string
	Length       int32
	Flags        int32
	NumBones     int32
	NumBodyParts int32
	NumTextures  int32
	NumCDTexture int32
	NumLocalAnim int32
	NumLocalSeq  int32
	NumHitboxSet int32
	HullMin      [3]float32
	HullMax      [3]float32
}

// parseStudioHeader 解析模型头部（纯函数，便于单测；失败返回 ok=false）。
func parseStudioHeader(data []byte) (studioHeaderInfo, bool) {
	var info studioHeaderInfo
	if len(data) < 240 {
		return info, false
	}
	if binary.LittleEndian.Uint32(data[0:4]) != 0x54534449 { // 'IDST'
		return info, false
	}
	readInt32 := func(offset int) int32 { return int32(binary.LittleEndian.Uint32(data[offset : offset+4])) }
	readFloat32 := func(offset int) float32 {
		return math.Float32frombits(binary.LittleEndian.Uint32(data[offset : offset+4]))
	}

	info.ID = "IDST"
	info.Version = readInt32(4)
	info.Checksum = readInt32(8)
	info.Name = readCString(data[12:76])
	info.Length = readInt32(76)
	info.HullMin = [3]float32{readFloat32(104), readFloat32(108), readFloat32(112)}
	info.HullMax = [3]float32{readFloat32(116), readFloat32(120), readFloat32(124)}
	info.Flags = readInt32(152)
	info.NumBones = readInt32(156)
	info.NumHitboxSet = readInt32(172)
	info.NumLocalAnim = readInt32(180)
	info.NumLocalSeq = readInt32(188)
	info.NumTextures = readInt32(204)
	info.NumCDTexture = readInt32(212)
	info.NumBodyParts = readInt32(232)
	return info, true
}

func readCString(raw []byte) string {
	if index := bytes.IndexByte(raw, 0); index >= 0 {
		raw = raw[:index]
	}
	return strings.TrimSpace(string(raw))
}

// buildModelPreview 生成模型信息卡。
func buildModelPreview(data []byte, entryPath string) string {
	var builder strings.Builder
	builder.WriteString("模型信息（.mdl 头部）\n")
	builder.WriteString(strings.Repeat("─", 32) + "\n")
	builder.WriteString(fmt.Sprintf("条目：%s\n", entryPath))
	builder.WriteString(fmt.Sprintf("文件大小：%s\n", humanBytes(int64(len(data)))))

	info, ok := parseStudioHeader(data)
	if !ok {
		builder.WriteString("\n无法解析模型头部（不是 IDST / 文件被截断）。\n")
		builder.WriteString("提示：模型的实际网格在配套的 .vvd / .vtx 里，本条目只是头部与材质引用。\n")
		return builder.String()
	}
	builder.WriteString(fmt.Sprintf("模型名：%s\n", fallbackText(info.Name, "（未命名）")))
	builder.WriteString(fmt.Sprintf("头部版本：%d　校验和：%d　声明长度：%s\n",
		info.Version, info.Checksum, humanBytes(int64(info.Length))))
	builder.WriteString(fmt.Sprintf("骨骼：%d　体块(bodyparts)：%d　命中盒集合：%d\n",
		info.NumBones, info.NumBodyParts, info.NumHitboxSet))
	builder.WriteString(fmt.Sprintf("序列：%d　动画：%d\n", info.NumLocalSeq, info.NumLocalAnim))
	builder.WriteString(fmt.Sprintf("Hull 尺寸：%.1f×%.1f×%.1f\n",
		math.Abs(float64(info.HullMax[0]-info.HullMin[0])),
		math.Abs(float64(info.HullMax[1]-info.HullMin[1])),
		math.Abs(float64(info.HullMax[2]-info.HullMin[2]))))

	materials := parser.ModelMaterialTokens(data)
	builder.WriteString(fmt.Sprintf("\n材质引用：%d 个（头部声明 %d）\n", len(materials), info.NumTextures))
	for index, material := range materials {
		if index >= 10 {
			builder.WriteString(fmt.Sprintf("… 其余 %d 个略\n", len(materials)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("  · %s\n", material))
	}
	builder.WriteString("\n说明：顶点/三角形统计请用 Mod 列表的「模型复杂度排序」（LOD0 总顶点），\n")
	builder.WriteString("3D 网格预览尚未支持（需要 VVD/VTX 与渲染管线）。\n")
	return builder.String()
}

// buildMapPreview 生成地图信息卡。
func buildMapPreview(data []byte, entryPath string) string {
	var builder strings.Builder
	builder.WriteString("地图信息（.bsp）\n")
	builder.WriteString(strings.Repeat("─", 32) + "\n")
	builder.WriteString(fmt.Sprintf("条目：%s\n", entryPath))
	builder.WriteString(fmt.Sprintf("文件大小：%s\n", humanBytes(int64(len(data)))))
	if len(data) < 8 || string(data[0:4]) != "VBSP" {
		builder.WriteString("\n不是有效的 VBSP 地图文件（缺少 VBSP 标识）。\n")
		return builder.String()
	}
	version := int32(binary.LittleEndian.Uint32(data[4:8]))
	builder.WriteString(fmt.Sprintf("BSP 版本：%d\n", version))

	// ① 自研的 lump 目录 + 实体统计：与 BSP 版本无关，L4D2 的 v21 也能读到实打实的数据。
	lumps := readBSPLumps(data)
	if len(lumps) > 0 {
		nonEmpty := 0
		for _, lump := range lumps {
			if lump.Length > 0 {
				nonEmpty++
			}
		}
		builder.WriteString(fmt.Sprintf("Lump：%d 个（非空 %d）\n", len(lumps), nonEmpty))
		largest := append([]bspLumpInfo(nil), lumps...)
		sort.Slice(largest, func(i, j int) bool { return largest[i].Length > largest[j].Length })
		for index, lump := range largest {
			if index >= 5 || lump.Length == 0 {
				break
			}
			builder.WriteString(fmt.Sprintf("  · %-22s %s\n", lump.DisplayName(), humanBytes(int64(lump.Length))))
		}
	}
	if entityLump := bspLumpByIndex(lumps, 0); entityLump != nil && entityLump.Length > 0 {
		entities, classes := parseBSPEntities(data, *entityLump)
		builder.WriteString("\n【实体】\n")
		builder.WriteString(fmt.Sprintf("实体总数：%d\n", entities))
		for _, item := range classes {
			builder.WriteString(fmt.Sprintf("  · %-28s %d\n", item.class, item.count))
		}
	}

	// ② 完整解析（几何/贴图/道具）：第三方解析器对版本有要求，失败就只保留上面的信息。
	file, err := readBSPFileSafe(data)
	if err != nil {
		builder.WriteString(fmt.Sprintf("\n【几何】\n解析未完成：%v\n", err))
		builder.WriteString("说明：这是 L4D2 常见的 BSP v21 布局，第三方解析器只覆盖到旧版本；\n")
		builder.WriteString("实体与 lump 目录仍然有效。要完整检查地图请在游戏里进图。\n")
		return builder.String()
	}

	builder.WriteString("\n【世界几何】\n")
	builder.WriteString(fmt.Sprintf("平面 %d　顶点 %d　面 %d　画刷 %d　画刷面 %d\n",
		len(file.Planes), len(file.Vertexes), len(file.FaceIDs), len(file.Brushes), len(file.BrushSides)))
	builder.WriteString(fmt.Sprintf("位移面 %d　贴花(overlay) %d　水面 %d\n",
		len(file.DispInfo), len(file.Overlays), len(file.WaterOverlays)))
	builder.WriteString(fmt.Sprintf("模型(models lump) %d 个\n", len(file.BrushModels)))

	builder.WriteString("\n【资源引用】\n")
	builder.WriteString(fmt.Sprintf("贴图：%d 张\n", len(file.TexNames)))
	builder.WriteString(fmt.Sprintf("静态道具：%d 个（模型 %d 种）\n", len(file.StaticProps.Props), len(file.StaticProps.Names)))
	builder.WriteString(fmt.Sprintf("细节道具：%d 个\n", len(file.DetailProps.Props)))
	if lights := len(file.LDR.WorldLights) + len(file.HDR.WorldLights); lights > 0 {
		builder.WriteString(fmt.Sprintf("灯光：LDR %d + HDR %d\n", len(file.LDR.WorldLights), len(file.HDR.WorldLights)))
	}
	if file.PakFile != nil {
		builder.WriteString(fmt.Sprintf("内嵌 pakfile：%d 个文件\n", len(file.PakFile.File)))
	}
	for index, name := range file.TexNames {
		if index >= 8 {
			builder.WriteString(fmt.Sprintf("  … 其余贴图 %d 张略\n", len(file.TexNames)-index))
			break
		}
		builder.WriteString(fmt.Sprintf("  · %s\n", name))
	}
	builder.WriteString("\n说明：这是「信息卡」，不是 3D 渲染（渲染需要完整材质/光照管线）。\n")
	return builder.String()
}

// bspLumpInfo 是 BSP 头部的 lump 目录项（64 项，每项 16 字节）。
type bspLumpInfo struct {
	Index   int
	Version uint32
	Offset  uint32
	Length  uint32
	FourCC  [4]byte
}

func (l bspLumpInfo) DisplayName() string {
	if name, ok := bspLumpNames[l.Index]; ok {
		return name
	}
	return fmt.Sprintf("Lump #%d", l.Index)
}

// bspLumpNames：Source 1 的 lump 名称表。
// 0–40 在 v20/v21 上完全一致（真机用 L4D2 地图核对过：0 实体、2 贴图数据、7 面、40 pakfile 都能对上）；
// 41–53 沿用 Source 2013 的顺序；53 在 L4D2（v21）是 HDR 光照（实测它与 8=LIGHTING 体积相当）。
var bspLumpNames = map[int]string{
	0: "ENTITIES", 1: "PLANES", 2: "TEXDATA", 3: "VERTEXES", 4: "VISIBILITY",
	5: "NODES", 6: "TEXINFO", 7: "FACES", 8: "LIGHTING", 9: "OCCLUSION",
	10: "LEAFS", 11: "FACEIDS", 12: "EDGES", 13: "SURFEDGES", 14: "MODELS",
	15: "WORLDLIGHTS", 16: "LEAFFACES", 17: "LEAFBRUSHES", 18: "BRUSHES",
	19: "BRUSHSIDES", 20: "AREAS", 21: "AREAPORTALS",
	26: "DISPINFO", 27: "ORIGINALFACES", 28: "PHYSDISP", 29: "PHYSCOLLIDE",
	30: "VERTNORMALS", 31: "VERTNORMALINDICES", 33: "DISP_VERTS",
	35: "GAME_LUMP", 36: "LEAFWATERDATA", 37: "PRIMITIVES", 38: "PRIMVERTS",
	39: "PRIMINDICES", 40: "PAKFILE", 41: "CLIPPORTALVERTS", 42: "CUBEMAPS",
	43: "TEXDATA_STRING_DATA", 44: "TEXDATA_STRING_TABLE", 45: "OVERLAYS",
	46: "LEAFMINDISTTOWATER", 47: "FACE_MACRO_TEXTURE_INFO", 48: "DISP_TRIS",
	49: "PROPCOLLISION", 50: "PROPHULLS", 51: "PROPHULLVERTS", 52: "PROPTRIS",
	53: "LIGHTING_HDR",
}

func readBSPLumps(data []byte) []bspLumpInfo {
	const lumpSize = 16
	if len(data) < 12+64*lumpSize {
		return nil
	}
	version := int32(binary.LittleEndian.Uint32(data[4:8]))
	// 布局差异（真机实测）：
	//   - Source 1 v20 及更早：ident(4) + version(4) + lump[64]
	//   - **L4D2 / Portal 2 的 v21：ident(4) + version(4) + 额外 4 字节(mapRevision) + lump[64]**
	// 先用版本给出候选顺序，再用"第 0 个 lump（ENTITIES）看起来是否合理"兜底，
	// 这样以后遇到别的布局也不会整体读错（读错的后果是实体/几何全空，而不是崩溃）。
	primary, secondary := 8, 12
	if version >= 21 {
		primary, secondary = 12, 8
	}
	if lumps, ok := parseBSPLumpsAt(data, primary, lumpSize); ok {
		return lumps
	}
	if lumps, ok := parseBSPLumpsAt(data, secondary, lumpSize); ok {
		return lumps
	}
	return nil
}

// parseBSPLumpsAt 按给定起点解析 lump 目录；ok=false 表示这个起点明显不对。
func parseBSPLumpsAt(data []byte, headerSize, lumpSize int) ([]bspLumpInfo, bool) {
	if len(data) < headerSize+64*lumpSize {
		return nil, false
	}
	lumps := make([]bspLumpInfo, 0, 64)
	for index := 0; index < 64; index++ {
		base := headerSize + index*lumpSize
		lump := bspLumpInfo{
			Index:   index,
			Offset:  binary.LittleEndian.Uint32(data[base : base+4]),
			Length:  binary.LittleEndian.Uint32(data[base+4 : base+8]),
			Version: binary.LittleEndian.Uint32(data[base+8 : base+12]),
		}
		copy(lump.FourCC[:], data[base+12:base+16])
		// 越界的 lump 直接按空处理：手工打包坏的地图不该让预览报错。
		if int(lump.Offset)+int(lump.Length) > len(data) {
			lump.Offset, lump.Length = 0, 0
		}
		lumps = append(lumps, lump)
	}
	// 合理性判据：ENTITIES 必须落在文件内且非空——读错布局时这一项通常是 offset=0。
	entities := lumps[0]
	if entities.Length == 0 || entities.Offset == 0 || int(entities.Offset)+int(entities.Length) > len(data) {
		return nil, false
	}
	return lumps, true
}

func bspLumpByIndex(lumps []bspLumpInfo, index int) *bspLumpInfo {
	for i := range lumps {
		if lumps[i].Index == index {
			return &lumps[i]
		}
	}
	return nil
}

// parseBSPEntities 从实体 lump 里数出实体与 classname 直方图。
// 实体块就是 KeyValues 文本，直接按 `"classname" "值"` 统计，与 BSP 版本无关。
func parseBSPEntities(data []byte, lump bspLumpInfo) (int, []entityClassCount) {
	payload := data[lump.Offset : lump.Offset+lump.Length]
	text := string(payload)
	counts := make(map[string]int, 32)
	entities := strings.Count(text, "\"classname\"")
	if entities == 0 {
		entities = strings.Count(text, "{")
	}
	for _, match := range entityClassPattern.FindAllStringSubmatch(text, -1) {
		class := strings.TrimSpace(match[1])
		if class == "" {
			continue
		}
		counts[class]++
	}
	list := make([]entityClassCount, 0, len(counts))
	for class, count := range counts {
		list = append(list, entityClassCount{class: class, count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].class < list[j].class
	})
	if len(list) > 8 {
		list = list[:8]
	}
	return entities, list
}

// readBSPFileSafe 调用第三方 BSP 解析器，并把 panic 转成错误：
// 作者手工打包坏的地图不应该把预览（甚至整个应用）带崩。
func readBSPFileSafe(data []byte) (file *bsp.File, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			file = nil
			err = fmt.Errorf("地图数据异常：%v", recovered)
		}
	}()
	parsed := &bsp.File{}
	if err := parsed.ReadFrom(bytes.NewReader(data)); err != nil {
		return nil, err
	}
	return parsed, nil
}

type entityClassCount struct {
	class string
	count int
}

// entityClassPattern 匹配实体块里的 `"classname" "xxx"`（允许任意空白分隔）。
var entityClassPattern = regexp.MustCompile(`"classname"\s+"([^"]*)"`)

// entityClassHistogram 统计实体 classname 出现次数，按次数降序（并列按名字）。
func entityClassHistogram(entities []bsp.Entity, limit int) []entityClassCount {
	counts := make(map[string]int, 32)
	for _, entity := range entities {
		class := ""
		for _, pair := range entity.Pairs {
			if strings.EqualFold(pair.Key, "classname") {
				class = strings.TrimSpace(pair.Value)
				break
			}
		}
		if class == "" {
			class = "（无 classname）"
		}
		counts[class]++
	}
	list := make([]entityClassCount, 0, len(counts))
	for class, count := range counts {
		list = append(list, entityClassCount{class: class, count: count})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].count != list[j].count {
			return list[i].count > list[j].count
		}
		return list[i].class < list[j].class
	})
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	return list
}

func humanBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KB", "MB", "GB"}
	value := float64(size)
	for _, unit := range units {
		value /= 1024
		if value < 1024 {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
	}
	return fmt.Sprintf("%.1f TB", value/1024)
}

func fallbackText(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
