package parser

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"

	"l4d2-manager-next/pkg/valve/vpk"
)

const (
	cardPreviewMaxWidth  = 512
	cardPreviewMaxHeight = 288
	// Card previews are deliberately bounded independently of the full-detail
	// preview path. A malformed or unusually large image must not allocate an
	// unbounded byte slice during a grid scan.
	cardPreviewMaxSourceBytes = 32 * 1024 * 1024
	cardPreviewMaxPixels      = 16 * 1024 * 1024
)

// ExtractVPKCardPreviewImage extracts only the selected preview payload and
// returns a static PNG thumbnail. GIF input is decoded as its first frame, so
// the card grid never creates one animation loop per visible Mod.
func ExtractVPKCardPreviewImage(filePath string) (string, error) {
	opener := vpk.Single(filePath)
	defer opener.Close()

	archive, err := opener.ReadArchive()
	if err != nil {
		return "", err
	}
	index := buildArchivePathIndex(archive)
	data, err := extractPreviewImageBytesFromFiles(opener, index.addonImageFile, index.previewFile, filePath)
	if err != nil || len(data) == 0 {
		return "", err
	}
	return encodeCardPreviewReader(bytes.NewReader(data))
}

func extractPreviewImageBytesFromFiles(opener *vpk.Opener, addonImageFile, previewFile *vpk.File, vpkFilePath string) ([]byte, error) {
	basePath := strings.TrimSuffix(vpkFilePath, filepath.Ext(vpkFilePath))
	for _, ext := range []string{".jpg", ".png", ".jpeg", ".gif"} {
		candidate := basePath + ext
		if info, statErr := os.Stat(candidate); statErr == nil && info.Size() > cardPreviewMaxSourceBytes {
			continue
		}
		if data, err := os.ReadFile(candidate); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	for _, file := range []*vpk.File{addonImageFile, previewFile} {
		if file == nil {
			continue
		}
		reader, err := file.Open(opener)
		if err != nil {
			continue
		}
		data, readErr := readBoundedPreview(reader)
		reader.Close()
		if readErr == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, nil
}

func readBoundedPreview(reader io.Reader) ([]byte, error) {
	limited := io.LimitReader(reader, cardPreviewMaxSourceBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(data) > cardPreviewMaxSourceBytes {
		return nil, fmt.Errorf("预览图超过 %d MiB 限制", cardPreviewMaxSourceBytes/(1024*1024))
	}
	return data, nil
}

// encodeCardPreviewReader decodes one image frame and emits a bounded static
// PNG. GIFs intentionally use image.Decode's first frame, preventing one
// animation decoder per visible card while preserving the original animated
// image for the detail dialog.
func encodeCardPreviewReader(reader io.Reader) (string, error) {
	data, err := readBoundedPreview(reader)
	if err != nil {
		return "", err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > cardPreviewMaxPixels {
		return "", fmt.Errorf("预览图像素数超过 %d MP 限制", cardPreviewMaxPixels/(1024*1024))
	}
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	bounds := decoded.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width <= 0 || height <= 0 {
		return "", fmt.Errorf("预览图尺寸无效")
	}
	scale := 1.0
	if width > cardPreviewMaxWidth || height > cardPreviewMaxHeight {
		scale = minFloat64(float64(cardPreviewMaxWidth)/float64(width), float64(cardPreviewMaxHeight)/float64(height))
	}
	targetWidth := maxInt(1, int(float64(width)*scale+0.5))
	targetHeight := maxInt(1, int(float64(height)*scale+0.5))
	thumbnail := image.NewRGBA(image.Rect(0, 0, targetWidth, targetHeight))
	resizeNearest(thumbnail, decoded)
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, thumbnail); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}

func resizeNearest(dst *image.RGBA, src image.Image) {
	dstBounds := dst.Bounds()
	srcBounds := src.Bounds()
	width, height := srcBounds.Dx(), srcBounds.Dy()
	targetWidth, targetHeight := dstBounds.Dx(), dstBounds.Dy()
	xMap := make([]int, targetWidth)
	yMap := make([]int, targetHeight)
	for x := range xMap {
		xMap[x] = srcBounds.Min.X + x*width/targetWidth
	}
	for y := range yMap {
		yMap[y] = srcBounds.Min.Y + y*height/targetHeight
	}
	for y, sourceY := range yMap {
		for x, sourceX := range xMap {
			dst.SetRGBA(x, y, colorModelRGBA(src.At(sourceX, sourceY)))
		}
	}
}

func colorModelRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA()
	return color.RGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func minFloat64(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
