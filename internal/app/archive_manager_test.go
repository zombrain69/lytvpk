package app

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bodgit/sevenzip"
)

func TestArchiveScanCacheReusesMatchingSignatureAndPassword(t *testing.T) {
	cache := newArchiveScanCache()
	signature := archiveFileSignature{Size: 42, ModTime: time.Unix(100, 0)}
	want := ArchivePackageInfo{Path: `C:\archives\locked.7z`, Name: "locked.7z", Format: "7z"}
	cache.put(want.Path, signature, "secret", want)

	got, ok := cache.get(want.Path, signature, "secret")
	if !ok || got.Path != want.Path {
		t.Fatalf("matching cache lookup = (%+v, %v), want hit for %q", got, ok, want.Path)
	}
	if _, ok := cache.get(want.Path, signature, "wrong"); ok {
		t.Fatal("cache lookup with a different password must miss")
	}
	if _, ok := cache.get(want.Path, archiveFileSignature{Size: 43, ModTime: signature.ModTime}, "secret"); ok {
		t.Fatal("cache lookup with a changed file signature must miss")
	}
}

func TestArchiveExistingIndexSignatureChangesWhenAddonListChanges(t *testing.T) {
	root := t.TempDir()
	addonList := filepath.Join(root, "addonlist.txt")
	if err := os.WriteFile(addonList, []byte("one"), 0644); err != nil {
		t.Fatal(err)
	}
	first := archiveExistingIndexSignatureForRoot(root)
	if first.AddonList.Size == 0 {
		t.Fatal("expected addonlist signature to include file size")
	}
	if err := os.WriteFile(addonList, []byte("two-two"), 0644); err != nil {
		t.Fatal(err)
	}
	second := archiveExistingIndexSignatureForRoot(root)
	if first == second {
		t.Fatalf("index signature did not change after addonlist update: before=%+v after=%+v", first, second)
	}
}

func TestArchiveFormatForPathRecognizesCommonFormats(t *testing.T) {
	tests := map[string]string{
		"mods.zip":    "zip",
		"mods.rar":    "rar",
		"mods.7z":     "7z",
		"mods.tar":    "tar",
		"mods.tar.gz": "tar.gz",
		"mods.tgz":    "tar.gz",
		"mods.txt":    "",
	}
	for path, want := range tests {
		if got := archiveFormatForPath(path); got != want {
			t.Errorf("archiveFormatForPath(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestArchiveVPKMatchStateUsesCaseInsensitiveBaseName(t *testing.T) {
	existing := map[string]struct{}{"hero.vpk": {}}
	if got := archiveVPKMatchState("nested/HERO.VPK", existing); got != "existing" {
		t.Fatalf("match state = %q, want existing", got)
	}
	if got := archiveVPKMatchState("nested/new.vpk", existing); got != "new" {
		t.Fatalf("match state = %q, want new", got)
	}
	if got := archiveVPKMatchState("nested/readme.txt", existing); got != "" {
		t.Fatalf("non-VPK match state = %q, want empty", got)
	}
}

func TestScanArchiveDirectoryListsVPKAndMatchesAddon(t *testing.T) {
	tempDir := t.TempDir()
	addonsDir := filepath.Join(tempDir, "addons")
	archiveDir := filepath.Join(tempDir, "archives", "nested")
	if err := os.MkdirAll(addonsDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archiveDir, 0755); err != nil {
		t.Fatal(err)
	}
	existingPath := filepath.Join(addonsDir, "Existing.vpk")
	writeTestVPK(t, existingPath, map[string][]byte{"scripts/addoninfo.txt": []byte("\"AddonInfo\"\n")})

	newVPK := filepath.Join(t.TempDir(), "New.vpk")
	writeTestVPK(t, newVPK, map[string][]byte{"materials/test/a.txt": []byte("hello")})
	archivePath := filepath.Join(archiveDir, "mods.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	for _, item := range []struct {
		name string
		path string
	}{
		{name: "pack/EXISTING.VPK", path: existingPath},
		{name: "pack/new.vpk", path: newVPK},
	} {
		entry, err := zipWriter.Create(item.name)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	app := &App{rootDir: addonsDir}
	packages, err := app.ScanArchiveDirectory(filepath.Join(tempDir, "archives"))
	if err != nil {
		t.Fatalf("scan archives: %v", err)
	}
	if len(packages) != 1 || len(packages[0].VPKs) != 2 {
		t.Fatalf("unexpected scan result: %+v", packages)
	}
	states := map[string]string{}
	for _, vpkInfo := range packages[0].VPKs {
		states[vpkInfo.Name] = vpkInfo.MatchState
		if !vpkInfo.Valid || len(vpkInfo.InternalFiles) == 0 {
			t.Fatalf("nested vpk was not inspected: %+v", vpkInfo)
		}
	}
	if states["EXISTING.VPK"] != "existing" || states["new.vpk"] != "new" {
		t.Fatalf("unexpected match states: %#v", states)
	}
}

func TestMoveArchiveFilesSupportsConflictActions(t *testing.T) {
	tempDir := t.TempDir()
	sourceDir := filepath.Join(tempDir, "source")
	destDir := filepath.Join(tempDir, "dest")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(destDir, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(sourceDir, "mods.zip")
	target := filepath.Join(destDir, "mods.zip")
	if err := os.WriteFile(source, []byte("source"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target"), 0644); err != nil {
		t.Fatal(err)
	}

	app := &App{}
	result, err := app.MoveArchiveFiles([]string{source}, destDir, "skip")
	if err != nil || result.SkippedCount != 1 {
		t.Fatalf("skip result: %+v, err=%v", result, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "target" {
		t.Fatalf("skip unexpectedly changed target: %q", got)
	}
	result, err = app.MoveArchiveFiles([]string{source}, destDir, "replace")
	if err != nil || result.SuccessCount != 1 {
		t.Fatalf("replace result: %+v, err=%v", result, err)
	}
	if got, _ := os.ReadFile(target); string(got) != "source" {
		t.Fatalf("replace did not update target: %q", got)
	}
}

func TestScanArchiveDirectorySupportsTarGzip(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "map.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"maps/test.bsp": []byte("bsp")})
	archivePath := filepath.Join(tempDir, "mods.tgz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gz)
	data, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.WriteHeader(&tar.Header{Name: "nested/map.vpk", Mode: 0644, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	packages, err := (&App{}).ScanArchiveDirectory(tempDir)
	if err != nil {
		t.Fatalf("scan tgz: %v", err)
	}
	if len(packages) != 1 || packages[0].Format != "tar.gz" || len(packages[0].VPKs) != 1 || !packages[0].VPKs[0].Valid {
		t.Fatalf("unexpected tgz scan result: %+v", packages)
	}
}

func TestScanArchiveDirectoryInspectsLargeVPKByDirectoryOnly(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "large.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo")})

	file, err := os.OpenFile(vpkPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	// This is just over the obsolete whole-file threshold. The VPK directory
	// remains tiny, so a correct scanner must inspect it without loading the
	// 32 MiB payload into memory.
	padding := (32 << 20) + 1
	if _, err := file.Seek(int64(padding)-1, io.SeekCurrent); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	archivePath := filepath.Join(tempDir, "large-vpk.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	entry, err := zipWriter.Create("nested/large.vpk")
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.Open(vpkPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(entry, source); err != nil {
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	packages, err := (&App{}).ScanArchiveDirectory(tempDir)
	if err != nil {
		t.Fatalf("scan archive: %v", err)
	}
	if len(packages) != 1 || len(packages[0].VPKs) != 1 {
		t.Fatalf("unexpected scan result: %+v", packages)
	}
	vpkInfo := packages[0].VPKs[0]
	if !vpkInfo.Valid || vpkInfo.InspectionStatus != archiveVPKInspectionValid || vpkInfo.FileCount != 1 {
		t.Fatalf("large VPK should be inspected from its directory, got %+v", vpkInfo)
	}
}

func TestClassifySevenZipErrorMarksEncryptedHeadersAsPasswordRequired(t *testing.T) {
	state := classifySevenZipError(&sevenzip.ReadError{
		Encrypted: true,
		Err:       errors.New("decoder rejected encrypted header"),
	})
	if !state.RequiresPassword || state.Kind != archiveErrorKindPasswordRequired {
		t.Fatalf("encrypted 7Z state = %+v, want password-required", state)
	}
	if !strings.Contains(state.Message, "密码") {
		t.Fatalf("encrypted 7Z message should explain the password requirement, got %q", state.Message)
	}
}

func TestOpenArchivePackageRejectsUnsupportedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "not-an-archive.txt")
	if err := os.WriteFile(path, []byte("not an archive"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := (&App{}).OpenArchivePackage(path); err == nil || !strings.Contains(err.Error(), "不支持的压缩格式") {
		t.Fatalf("unsupported file error = %v, want unsupported archive format", err)
	}
}

func TestExistingVPKIndexTracksRootDisabledAndWorkshopLocations(t *testing.T) {
	tempDir := t.TempDir()
	addonsDir := filepath.Join(tempDir, "addons")
	for _, dir := range []string{addonsDir, filepath.Join(addonsDir, "disabled"), filepath.Join(addonsDir, "workshop")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{
		filepath.Join(addonsDir, "root.vpk"),
		filepath.Join(addonsDir, "disabled", "disabled.vpk"),
		filepath.Join(addonsDir, "workshop", "123.vpk"),
	} {
		writeTestVPK(t, path, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo")})
	}
	addonListPath := filepath.Join(addonsDir, "addonlist.txt")
	if err := os.WriteFile(addonListPath, []byte("\"AddonList\"\n{\n\t\"root.vpk\" \"1\"\n\t\"disabled.vpk\" \"0\"\n\t\"workshop\\123.vpk\" \"0\"\n}\n"), 0644); err != nil {
		t.Fatal(err)
	}

	index := (&App{rootDir: addonsDir}).existingVPKIndex()
	if got := archiveVPKMatchStateFromIndex("root.vpk", index); got != "existing" {
		t.Fatalf("root state = %q, want existing", got)
	}
	if locations, state := archiveVPKExistingDetails("disabled.vpk", index); state != "disabled" || len(locations) != 1 || locations[0] != "disabled" {
		t.Fatalf("disabled details = locations:%v state:%q, want disabled/disabled", locations, state)
	}
	if locations, state := archiveVPKExistingDetails("123.vpk", index); state != "disabled" || len(locations) != 1 || locations[0] != "workshop" {
		t.Fatalf("workshop details = locations:%v state:%q, want workshop/disabled", locations, state)
	}
}

func TestScanArchivePackageWithPasswordRefreshesOnlyOneArchive(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "single.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{"scripts/addoninfo.txt": []byte("AddonInfo")})
	archivePath := filepath.Join(tempDir, "single.zip")
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	zipWriter := zip.NewWriter(archiveFile)
	entry, err := zipWriter.Create("single.vpk")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := entry.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := archiveFile.Close(); err != nil {
		t.Fatal(err)
	}

	packageInfo, err := (&App{}).ScanArchivePackageWithPassword(archivePath, "")
	if err != nil || len(packageInfo.VPKs) != 1 || !packageInfo.VPKs[0].Valid {
		t.Fatalf("single archive refresh = %+v, err=%v", packageInfo, err)
	}
}
