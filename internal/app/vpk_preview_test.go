package app

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScanVPKFilesDefersPreviewImageUntilRequested(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	sourceDir := filepath.Join(tempDir, "preview_fixture")
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	var imageData bytes.Buffer
	// Ensure the fixture has a visible non-transparent pixel while keeping it tiny.
	fixtureImage := image.NewRGBA(image.Rect(0, 0, 1, 1))
	fixtureImage.Set(0, 0, color.RGBA{R: 0x4c, G: 0xb8, B: 0x6a, A: 0xff})
	if err := png.Encode(&imageData, fixtureImage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "addonimage.png"), imageData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := &App{}
	if _, err := packer.PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("pack preview VPK: %v", err)
	}

	vpkPath := filepath.Join(rootDir, "preview_fixture.vpk")
	app := &App{rootDir: rootDir} // nil pool exercises the synchronous safe fallback.
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("scan VPK files: %v", err)
	}
	cached, ok := app.vpkCache.Load(vpkPath)
	if !ok {
		t.Fatalf("scan did not cache %s", vpkPath)
	}
	if preview := cached.(*VPKFileCache).File.PreviewImage; preview != "" {
		t.Fatalf("normal scan retained preview image (%d bytes)", len(preview))
	}

	preview := app.GetVPKPreviewImage(vpkPath)
	if preview == "" {
		t.Fatal("on-demand preview image was empty")
	}
	if cached.(*VPKFileCache).File.PreviewImage != "" {
		t.Fatal("on-demand preview must not make the full scan cache retain Base64 data")
	}
}

func TestGetVPKCardPreviewImageBoundsAndNoPreview(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	sourceDir := filepath.Join(tempDir, "card_preview_fixture")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	var imageData bytes.Buffer
	fixtureImage := image.NewRGBA(image.Rect(0, 0, 1280, 720))
	fixtureImage.Set(0, 0, color.RGBA{R: 0x4c, G: 0xb8, B: 0x6a, A: 0xff})
	if err := png.Encode(&imageData, fixtureImage); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "addonimage.png"), imageData.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := &App{}
	if _, err := packer.PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("pack card preview VPK: %v", err)
	}
	vpkPath := filepath.Join(rootDir, "card_preview_fixture.vpk")
	app := &App{rootDir: rootDir}
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("scan VPK files: %v", err)
	}

	dataURL := app.GetVPKCardPreviewImage(vpkPath)
	if dataURL == "" {
		t.Fatal("card preview image was empty")
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(dataURL, prefix) {
		t.Fatalf("card preview format = %q, want PNG data URL", dataURL[:min(len(dataURL), 32)])
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, prefix))
	if err != nil {
		t.Fatalf("decode card preview: %v", err)
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(decoded))
	if err != nil {
		t.Fatalf("decode card preview config: %v", err)
	}
	if format != "png" || config.Width > 640 || config.Height > 360 {
		t.Fatalf("card preview dimensions = %dx%d %s, want <= 640x360 PNG", config.Width, config.Height, format)
	}

	noPreviewDir := filepath.Join(tempDir, "no_preview_fixture")
	if err := os.MkdirAll(filepath.Join(noPreviewDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(noPreviewDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := packer.PackVPKDirectory(noPreviewDir, rootDir, false); err != nil {
		t.Fatalf("pack no-preview VPK: %v", err)
	}
	noPreviewPath := filepath.Join(rootDir, "no_preview_fixture.vpk")
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("rescan VPK files: %v", err)
	}
	if got := app.GetVPKCardPreviewImage(noPreviewPath); got != "" {
		t.Fatalf("no-preview card result = %q, want empty", got)
	}
}

func TestGetVPKPreviewImageCachesAndInvalidatesExternalImage(t *testing.T) {
	tempDir := t.TempDir()
	rootDir := filepath.Join(tempDir, "addons")
	sourceDir := filepath.Join(tempDir, "preview_cache_fixture")
	if err := os.MkdirAll(filepath.Join(sourceDir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "scripts", "addon.txt"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	var internalImage bytes.Buffer
	imageA := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imageA.Set(0, 0, color.RGBA{R: 0xff, A: 0xff})
	if err := png.Encode(&internalImage, imageA); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "addonimage.png"), internalImage.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	packer := &App{}
	if _, err := packer.PackVPKDirectory(sourceDir, rootDir, false); err != nil {
		t.Fatalf("pack preview VPK: %v", err)
	}
	vpkPath := filepath.Join(rootDir, "preview_cache_fixture.vpk")
	app := &App{rootDir: rootDir}
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("scan VPK files: %v", err)
	}
	initialCache, ok := app.vpkCache.Load(vpkPath)
	if !ok || initialCache.(*VPKFileCache).File.PreviewRevision == "" {
		t.Fatal("scan did not expose a stable preview source revision")
	}
	initialRevision := initialCache.(*VPKFileCache).File.PreviewRevision

	first := app.GetVPKPreviewImage(vpkPath)
	if first == "" {
		t.Fatal("initial preview was empty")
	}
	if cached, ok := app.previewCache.Load(vpkPath); !ok || cached.(*VPKPreviewCache).Data != first {
		t.Fatal("initial preview was not retained in the bounded on-demand cache")
	}
	if second := app.GetVPKPreviewImage(vpkPath); second != first {
		t.Fatal("unchanged preview should be served from the cache")
	}

	var externalImage bytes.Buffer
	imageB := image.NewRGBA(image.Rect(0, 0, 2, 1))
	imageB.Set(0, 0, color.RGBA{B: 0xff, A: 0xff})
	imageB.Set(1, 0, color.RGBA{G: 0xff, A: 0xff})
	if err := png.Encode(&externalImage, imageB); err != nil {
		t.Fatal(err)
	}
	sidecarPath := filepath.Join(rootDir, "preview_cache_fixture.png")
	if err := os.WriteFile(sidecarPath, externalImage.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	forcedImageTime := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(sidecarPath, forcedImageTime, forcedImageTime); err != nil {
		t.Fatal(err)
	}

	updated := app.GetVPKPreviewImage(vpkPath)
	if updated == "" || updated == first {
		t.Fatal("adding an external sidecar image must invalidate the previous preview")
	}
	if err := app.ScanVPKFiles(); err != nil {
		t.Fatalf("rescan after external preview change: %v", err)
	}
	refreshedCache, ok := app.vpkCache.Load(vpkPath)
	if !ok || refreshedCache.(*VPKFileCache).File.PreviewRevision == initialRevision {
		t.Fatal("external preview change must produce a new preview source revision")
	}
}

func TestDeleteVPKPreviewCachesClearsFullAndCardEntries(t *testing.T) {
	app := &App{}
	filePath := filepath.Join(t.TempDir(), "cache.vpk")
	stamp := time.Now()
	app.previewCache.Store(filePath, &VPKPreviewCache{Data: "full", ModTime: stamp})
	app.cardPreviewCache.Store(filePath, &VPKPreviewCache{Data: "card", ModTime: stamp})

	app.deleteVPKPreviewCaches(filePath)

	if _, ok := app.previewCache.Load(filePath); ok {
		t.Fatal("full preview cache entry was not cleared")
	}
	if _, ok := app.cardPreviewCache.Load(filePath); ok {
		t.Fatal("card preview cache entry was not cleared")
	}
}
