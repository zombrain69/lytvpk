package parser

import (
	"bytes"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

var (
	utf8BOM    = []byte{0xEF, 0xBB, 0xBF}
	utf16LEBOM = []byte{0xFF, 0xFE}
	utf16BEBOM = []byte{0xFE, 0xFF}
)

// DecodeVPKText converts textual VPK payloads to UTF-8 for parsers and the UI.
// Source's VPK format stores bytes rather than a charset marker. The game's
// normal files are UTF-8 or GBK/ANSI on this installation, with a small number
// of Workshop payloads using Western Windows-1252; UTF-16 is accepted only
// when it declares a byte-order mark. Opaque resources must not use this
// helper and are copied as bytes.
func DecodeVPKText(data []byte) (string, error) {
	if bytes.HasPrefix(data, utf8BOM) {
		data = data[len(utf8BOM):]
		if !utf8.Valid(data) {
			return "", fmt.Errorf("UTF-8 BOM 后的文本不是有效 UTF-8")
		}
		return string(data), nil
	}
	if bytes.HasPrefix(data, utf16LEBOM) || bytes.HasPrefix(data, utf16BEBOM) {
		endian := unicode.LittleEndian
		if bytes.HasPrefix(data, utf16BEBOM) {
			endian = unicode.BigEndian
		}
		decoded, _, err := transform.Bytes(unicode.UTF16(endian, unicode.ExpectBOM).NewDecoder(), data)
		if err != nil {
			return "", fmt.Errorf("无法解码 UTF-16 文本: %w", err)
		}
		return string(decoded), nil
	}
	if utf8.Valid(data) {
		return string(data), nil
	}

	decoded, _, gbkErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), data)
	if gbkErr == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
		return string(decoded), nil
	}

	// Some Workshop authors save mission/addoninfo files with the Western
	// Windows ANSI code page (Windows-1252), which is not valid GBK. Try it
	// only after strict GBK decoding so Chinese GBK payloads keep their native
	// characters while legacy Western payloads remain readable.
	decoded, _, ansiErr := transform.Bytes(charmap.Windows1252.NewDecoder(), data)
	if ansiErr != nil {
		return "", fmt.Errorf("无法按 GBK/ANSI 解码文本（GBK: %v；Windows-1252: %w）", gbkErr, ansiErr)
	}
	if strings.ContainsRune(string(decoded), '\uFFFD') {
		return "", fmt.Errorf("无法按 GBK/ANSI 解码文本：Windows-1252 结果包含替换字符（GBK: %v）", gbkErr)
	}
	return string(decoded), nil
}

// DecodeVPKEntryName converts the raw VPK directory bytes into a Windows
// filename. Existing UTF-8 archives remain untouched; legacy GBK directory
// entries are decoded before any Unicode case conversion or filesystem write.
func DecodeVPKEntryName(name string) (string, error) {
	if utf8.ValidString(name) {
		return name, nil
	}

	decoded, _, gbkErr := transform.Bytes(simplifiedchinese.GBK.NewDecoder(), []byte(name))
	if gbkErr == nil && !strings.ContainsRune(string(decoded), '\uFFFD') {
		return string(decoded), nil
	}
	decoded, _, ansiErr := transform.Bytes(charmap.Windows1252.NewDecoder(), []byte(name))
	if ansiErr != nil {
		return "", fmt.Errorf("无法按 GBK/ANSI 解码 VPK 文件名（GBK: %v；Windows-1252: %w）", gbkErr, ansiErr)
	}
	if strings.ContainsRune(string(decoded), '\uFFFD') {
		return "", fmt.Errorf("无法按 GBK/ANSI 解码 VPK 文件名：Windows-1252 结果包含替换字符（GBK: %v）", gbkErr)
	}
	return string(decoded), nil
}

// EncodeVPKEntryName creates Source-compatible VPK directory bytes from a
// Windows filename. GBK is preferred because it is the legacy encoding found
// in the local game's archives. Characters that GBK cannot represent retain
// their UTF-8 bytes so packing never silently changes a filename.
func EncodeVPKEntryName(name string) string {
	if !utf8.ValidString(name) {
		return name
	}

	encoded, _, err := transform.Bytes(simplifiedchinese.GBK.NewEncoder(), []byte(name))
	if err != nil {
		return name
	}
	return string(encoded)
}
