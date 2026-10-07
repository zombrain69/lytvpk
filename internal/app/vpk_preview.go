package app

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"vpk-manager/internal/parser"

	"l4d2-manager-next/pkg/valve/vpk"
	"l4d2-manager-next/pkg/valve/vtf"
)

// VPK 内容预览（对齐 VPKEdit 的"不解包直接看"思路，先做只读的一部分）。
//
// 支持：
//   - 文本 / KeyValues（.txt .cfg .vmt .vdf .nut .lua .res .json …）——按游戏文件编码解码；
//   - 图片（.jpg .png .gif .bmp）；
//   - VTF 贴图（DXT1/3/5 与常见未压缩格式）——转成 PNG 返回；
//   - 其它类型只给元信息（大小 + 说明），不做二进制预览。
//
// 边界：单条文本最多 256KB、图片/VTF 最多 8MB，超出只报"太大"而不是硬塞进界面。

const (
	vpkPreviewTextLimit  = 256 * 1024
	vpkPreviewImageLimit = 8 * 1024 * 1024
	vpkPreviewMaxEntries = 20000
)

type VPKEntryInfo struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type VPKEntryList struct {
	FilePath   string         `json:"filePath"`
	Entries    []VPKEntryInfo `json:"entries"`
	TotalCount int            `json:"totalCount"`
	Truncated  bool           `json:"truncated"`
}

type VPKPreviewResult struct {
	Path      string `json:"path"`
	Kind      string `json:"kind"` // text | image | binary
	Text      string `json:"text,omitempty"`
	DataURL   string `json:"dataUrl,omitempty"`
	Size      int64  `json:"size"`
	Truncated bool   `json:"truncated,omitempty"`
	Note      string `json:"note,omitempty"`
}

// ListVPKEntries 列出 VPK 内的文件条目（按路径排序，超过上限时截断并标记）。
func (a *App) ListVPKEntries(filePath string) (VPKEntryList, error) {
	result := VPKEntryList{FilePath: filePath}
	opener := vpk.Single(filePath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return result, fmt.Errorf("读取 VPK 失败: %w", err)
	}
	entries := make([]VPKEntryInfo, 0, len(archive.Files))
	for i := range archive.Files {
		file := &archive.Files[i]
		name := filepath.ToSlash(file.Name())
		if decoded, decodeErr := parser.DecodeVPKEntryName(name); decodeErr == nil {
			name = decoded
		}
		entries = append(entries, VPKEntryInfo{Path: name, Size: int64(file.Size())})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	result.TotalCount = len(entries)
	if len(entries) > vpkPreviewMaxEntries {
		entries = entries[:vpkPreviewMaxEntries]
		result.Truncated = true
	}
	result.Entries = entries
	return result, nil
}

// PreviewVPKEntry 读取单个条目并给出可显示的预览。
func (a *App) PreviewVPKEntry(filePath string, entryPath string) (VPKPreviewResult, error) {
	result := VPKPreviewResult{Path: entryPath}
	opener := vpk.Single(filePath)
	defer opener.Close()
	archive, err := opener.ReadArchive()
	if err != nil {
		return result, fmt.Errorf("读取 VPK 失败: %w", err)
	}
	normalized := filepath.ToSlash(strings.TrimSpace(entryPath))
	var target *vpk.File
	for i := range archive.Files {
		name := filepath.ToSlash(archive.Files[i].Name())
		if strings.EqualFold(name, normalized) {
			target = &archive.Files[i]
			break
		}
	}
	if target == nil {
		return result, fmt.Errorf("这个 VPK 里没有 %s", entryPath)
	}
	result.Size = int64(target.Size())

	ext := strings.ToLower(filepath.Ext(normalized))
	switch {
	case isVPKPreviewTextExt(ext):
		if target.Size() > vpkPreviewTextLimit {
			result.Kind = "binary"
			result.Note = fmt.Sprintf("文本文件超过 %dKB，已跳过预览", vpkPreviewTextLimit/1024)
			return result, nil
		}
		raw, readErr := readVPKEntryBytes(opener, target)
		if readErr != nil {
			return result, fmt.Errorf("读取条目失败: %w", readErr)
		}
		text, _ := parser.DecodeVPKText(raw)
		result.Kind = "text"
		result.Text = text
		return result, nil
	case isVPKPreviewImageExt(ext):
		if target.Size() > vpkPreviewImageLimit {
			result.Kind = "binary"
			result.Note = fmt.Sprintf("图片超过 %dMB，已跳过预览", vpkPreviewImageLimit/1024/1024)
			return result, nil
		}
		raw, readErr := readVPKEntryBytes(opener, target)
		if readErr != nil {
			return result, fmt.Errorf("读取条目失败: %w", readErr)
		}
		result.Kind = "image"
		result.DataURL = dataURLForImageBytes(ext, raw)
		return result, nil
	case ext == ".vtf":
		if target.Size() > vpkPreviewImageLimit {
			result.Kind = "binary"
			result.Note = fmt.Sprintf("贴图超过 %dMB，已跳过预览", vpkPreviewImageLimit/1024/1024)
			return result, nil
		}
		raw, readErr := readVPKEntryBytes(opener, target)
		if readErr != nil {
			return result, fmt.Errorf("读取条目失败: %w", readErr)
		}
		dataURL, note, convErr := previewVTFFromBytes(raw)
		if convErr != nil {
			result.Kind = "binary"
			result.Note = "贴图预览失败：" + convErr.Error()
			return result, nil
		}
		result.Kind = "image"
		result.DataURL = dataURL
		result.Note = note
		return result, nil
	default:
		result.Kind = "binary"
		result.Note = "该类型暂不支持预览（可先用「VPK 解包」取出后再看）"
		return result, nil
	}
}

func readVPKEntryBytes(opener *vpk.Opener, entry *vpk.File) ([]byte, error) {
	reader, err := entry.Open(opener)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	buffer := bytes.NewBuffer(make([]byte, 0, entry.Size()))
	if _, err := buffer.ReadFrom(reader); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func isVPKPreviewTextExt(ext string) bool {
	switch ext {
	case ".txt", ".cfg", ".vmt", ".vdf", ".nut", ".lua", ".res", ".json", ".xml", ".md",
		".radial", ".inc", ".vscript", ".inf", ".rc", ".scr", ".csv", ".ini":
		return true
	default:
		return false
	}
}

func isVPKPreviewImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp":
		return true
	default:
		return false
	}
}

func dataURLForImageBytes(ext string, raw []byte) string {
	mime := "image/png"
	switch ext {
	case ".jpg", ".jpeg":
		mime = "image/jpeg"
	case ".gif":
		mime = "image/gif"
	case ".bmp":
		mime = "image/bmp"
	}
	return "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// previewVTFFromBytes 把 VTF 贴图转成 PNG data URL（只取第一帧、第一层 mipmap）。
func previewVTFFromBytes(raw []byte) (string, string, error) {
	texture, err := vtf.Read(bytes.NewReader(raw))
	if err != nil {
		return "", "", fmt.Errorf("解析 VTF 失败：%w", err)
	}
	if len(texture.Image) == 0 || len(texture.Image[0]) == 0 {
		return "", "", fmt.Errorf("VTF 里没有可显示的图像")
	}
	bitmap := texture.Image[0][0]
	img, err := vtfBitmapToImage(bitmap)
	if err != nil {
		return "", "", err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return "", "", fmt.Errorf("编码 PNG 失败：%w", err)
	}
	note := ""
	if len(texture.Image[0]) > 1 {
		note = fmt.Sprintf("显示最高 mipmap（%d×%d，共 %d 层）", bitmap.Width, bitmap.Height, len(texture.Image[0]))
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(out.Bytes()), note, nil
}

// vtfBitmapToImage 把 VTF 位图转成 image.Image（支持常见的未压缩格式与 DXT1/3/5）。
func vtfBitmapToImage(bitmap *vtf.Bitmap) (image.Image, error) {
	if bitmap == nil {
		return nil, fmt.Errorf("位图为空")
	}
	width, height := bitmap.Width, bitmap.Height
	if width <= 0 || height <= 0 {
		return nil, fmt.Errorf("位图尺寸非法（%d×%d）", width, height)
	}
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	switch bitmap.Format {
	case vtf.ImageFormatRGBA8888, vtf.ImageFormatLinearRGBA8888:
		copyRGBA(img, bitmap, "rgba")
	case vtf.ImageFormatABGR8888:
		copyRGBA(img, bitmap, "abgr")
	case vtf.ImageFormatARGB8888:
		copyRGBA(img, bitmap, "argb")
	case vtf.ImageFormatBGRA8888, vtf.ImageFormatLinearBGRX8888:
		copyRGBA(img, bitmap, "bgra")
	case vtf.ImageFormatBGRX8888, vtf.ImageFormatRGBX8888:
		copyRGBA(img, bitmap, "bgrx")
	case vtf.ImageFormatRGB888:
		copyRGBA(img, bitmap, "rgb")
	case vtf.ImageFormatBGR888:
		copyRGBA(img, bitmap, "bgr")
	case vtf.ImageFormatA8:
		copyRGBA(img, bitmap, "a8")
	case vtf.ImageFormatI8:
		copyRGBA(img, bitmap, "i8")
	case vtf.ImageFormatRGB565:
		copyRGBA(img, bitmap, "rgb565")
	case vtf.ImageFormatDXT1, vtf.ImageFormatDXT1OneBitAlpha, vtf.ImageFormatDXT3, vtf.ImageFormatDXT5:
		decodeDXT(img, bitmap)
	default:
		return nil, fmt.Errorf("暂不支持该贴图格式（%s）", bitmap.Format.String())
	}
	return img, nil
}

func copyRGBA(img *image.RGBA, bitmap *vtf.Bitmap, layout string) {
	width, height := bitmap.Width, bitmap.Height
	pix := bitmap.Pix
	for y := 0; y < height; y++ {
		rowStart := y * bitmap.Stride
		for x := 0; x < width; x++ {
			var r, g, b, a uint8
			switch layout {
			case "rgba":
				offset := rowStart + x*4
				if offset+3 >= len(pix) {
					continue
				}
				r, g, b, a = pix[offset], pix[offset+1], pix[offset+2], pix[offset+3]
			case "abgr":
				offset := rowStart + x*4
				if offset+3 >= len(pix) {
					continue
				}
				a, b, g, r = pix[offset], pix[offset+1], pix[offset+2], pix[offset+3]
			case "argb":
				offset := rowStart + x*4
				if offset+3 >= len(pix) {
					continue
				}
				a, r, g, b = pix[offset], pix[offset+1], pix[offset+2], pix[offset+3]
			case "bgra":
				offset := rowStart + x*4
				if offset+3 >= len(pix) {
					continue
				}
				b, g, r, a = pix[offset], pix[offset+1], pix[offset+2], pix[offset+3]
			case "bgrx":
				offset := rowStart + x*4
				if offset+3 >= len(pix) {
					continue
				}
				b, g, r, a = pix[offset], pix[offset+1], pix[offset+2], 255
			case "rgb":
				offset := rowStart + x*3
				if offset+2 >= len(pix) {
					continue
				}
				r, g, b, a = pix[offset], pix[offset+1], pix[offset+2], 255
			case "bgr":
				offset := rowStart + x*3
				if offset+2 >= len(pix) {
					continue
				}
				b, g, r, a = pix[offset], pix[offset+1], pix[offset+2], 255
			case "a8":
				offset := rowStart + x
				if offset >= len(pix) {
					continue
				}
				r, g, b, a = 255, 255, 255, pix[offset]
			case "i8":
				offset := rowStart + x
				if offset >= len(pix) {
					continue
				}
				r, g, b, a = pix[offset], pix[offset], pix[offset], 255
			case "rgb565":
				offset := rowStart + x*2
				if offset+1 >= len(pix) {
					continue
				}
				value := uint16(pix[offset]) | uint16(pix[offset+1])<<8
				r = uint8((value >> 11) & 0x1f * 255 / 31)
				g = uint8((value >> 5) & 0x3f * 255 / 63)
				b = uint8(value & 0x1f * 255 / 31)
				a = 255
			}
			img.SetRGBA(x, y, color.RGBA{R: r, G: g, B: b, A: a})
		}
	}
}

// decodeDXT 解 DXT1/3/5 到 RGBA（4×4 块，标准的 BC1/2/3 算法）。
func decodeDXT(img *image.RGBA, bitmap *vtf.Bitmap) {
	width, height := bitmap.Width, bitmap.Height
	pix := bitmap.Pix
	blockSize := 8
	if bitmap.Format == vtf.ImageFormatDXT3 || bitmap.Format == vtf.ImageFormatDXT5 {
		blockSize = 16
	}
	blocksPerRow := (width + 3) / 4
	for by := 0; by < (height+3)/4; by++ {
		for bx := 0; bx < blocksPerRow; bx++ {
			offset := (by*blocksPerRow + bx) * blockSize
			if offset+blockSize > len(pix) {
				return
			}
			block := pix[offset : offset+blockSize]
			var colors [4]color.RGBA
			var indices uint32
			alphaBytes := block[:8]
			colorBytes := block
			switch bitmap.Format {
			case vtf.ImageFormatDXT3:
				colorBytes = block[8:16]
			case vtf.ImageFormatDXT5:
				colorBytes = block[8:16]
				alphaBytes = block[:8]
			}
			color0 := uint16(colorBytes[0]) | uint16(colorBytes[1])<<8
			color1 := uint16(colorBytes[2]) | uint16(colorBytes[3])<<8
			colors[0] = rgb565ToRGBA(color0, 255)
			colors[1] = rgb565ToRGBA(color1, 255)
			punchThrough := bitmap.Format == vtf.ImageFormatDXT1 || bitmap.Format == vtf.ImageFormatDXT1OneBitAlpha
			if !punchThrough || color0 > color1 {
				colors[2] = mixRGBA(colors[0], colors[1], 2, 1, 3)
				colors[3] = mixRGBA(colors[0], colors[1], 1, 2, 3)
			} else {
				colors[2] = mixRGBA(colors[0], colors[1], 1, 1, 2)
				colors[3] = color.RGBA{A: 0}
			}
			indices = uint32(colorBytes[4]) | uint32(colorBytes[5])<<8 | uint32(colorBytes[6])<<16 | uint32(colorBytes[7])<<24
			for py := 0; py < 4; py++ {
				for px := 0; px < 4; px++ {
					x := bx*4 + px
					y := by*4 + py
					if x >= width || y >= height {
						continue
					}
					colorValue := colors[(indices>>uint(2*(4*py+px)))&0x3]
					switch bitmap.Format {
					case vtf.ImageFormatDXT3:
						nibble := alphaBytes[(4*py+px)/2]
						if (4*py+px)%2 == 0 {
							nibble >>= 4
						}
						colorValue.A = uint8((nibble & 0x0f) * 17)
					case vtf.ImageFormatDXT5:
						colorValue.A = dxt5Alpha(alphaBytes, 4*py+px)
					}
					img.SetRGBA(x, y, colorValue)
				}
			}
		}
	}
}

func rgb565ToRGBA(value uint16, alpha uint8) color.RGBA {
	r := uint8((value >> 11) & 0x1f)
	g := uint8((value >> 5) & 0x3f)
	b := uint8(value & 0x1f)
	return color.RGBA{R: r<<3 | r>>2, G: g<<2 | g>>4, B: b<<3 | b>>2, A: alpha}
}

func mixRGBA(a, b color.RGBA, weightA, weightB, divisor uint16) color.RGBA {
	mix := func(x, y uint8) uint8 {
		return uint8((uint16(x)*weightA + uint16(y)*weightB) / divisor)
	}
	return color.RGBA{R: mix(a.R, b.R), G: mix(a.G, b.G), B: mix(a.B, b.B), A: mix(a.A, b.A)}
}

func dxt5Alpha(block []byte, index int) uint8 {
	a0 := block[0]
	a1 := block[1]
	var table [8]uint8
	table[0], table[1] = a0, a1
	if a0 > a1 {
		for i := uint16(1); i <= 6; i++ {
			table[i+1] = uint8(((7-i)*uint16(a0) + i*uint16(a1)) / 7)
		}
	} else {
		for i := uint16(1); i <= 4; i++ {
			table[i+1] = uint8(((5-i)*uint16(a0) + i*uint16(a1)) / 5)
		}
		table[6], table[7] = 0, 255
	}
	bits := uint64(block[2]) | uint64(block[3])<<8 | uint64(block[4])<<16 |
		uint64(block[5])<<24 | uint64(block[6])<<32 | uint64(block[7])<<40
	return table[(bits>>uint(3*index))&0x7]
}

// OpenVPKPreviewInExplorer 占位：保持与其它工具一致的"打开所在位置"能力。
func (a *App) OpenVPKPreviewInExplorer(filePath string) error {
	if _, err := os.Stat(filePath); err != nil {
		return err
	}
	return a.OpenFileLocation(filePath)
}
