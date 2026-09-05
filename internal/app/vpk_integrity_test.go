package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"vpk-manager/internal/parser"
)

func TestInspectVPKIntegrityDetectsMalformedAddonInfo(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "broken.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt":      []byte("\"AddonInfo\"\n{\n\taddonSteamAppID \"550\"\n\taddonDescription \"\nunclosed\n}\n"),
		"materials/test.vmt": []byte("shader"),
	})

	report, err := (&App{}).InspectVPKIntegrity(vpkPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Valid || !report.Repairable || report.VerifiedFiles != report.TotalFiles {
		t.Fatalf("unexpected integrity report: %+v", report)
	}
	if !hasVPKIntegrityIssue(report, "addoninfo-syntax") {
		t.Fatalf("missing addoninfo syntax issue: %+v", report.Issues)
	}
}

func TestRepairVPKIntegrityCreatesVerifiedCopyAndPreservesSource(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "broken.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"addoninfo.txt":      []byte("\"AddonInfo\"\n{\n\taddontitle \"保留标题\"\n\taddonDescription \"\nunclosed\n}\n"),
		"materials/test.vmt": []byte("shader"),
	})
	original, err := os.ReadFile(vpkPath)
	if err != nil {
		t.Fatal(err)
	}

	result, err := (&App{}).RepairVPKIntegrity(vpkPath)
	if err != nil {
		t.Fatalf("repair: %v (result=%+v)", err, result)
	}
	if result.OutputPath == "" || result.OutputPath == vpkPath || !result.OriginalPreserved {
		t.Fatalf("unexpected repair result: %+v", result)
	}
	if _, err := os.Stat(result.OutputPath); err != nil {
		t.Fatalf("repaired output missing: %v", err)
	}
	defer os.Remove(result.OutputPath)
	if repaired, readErr := os.ReadFile(vpkPath); readErr != nil || string(repaired) != string(original) {
		t.Fatalf("source archive changed: readErr=%v", readErr)
	}
	verified, err := (&App{}).InspectVPKIntegrity(result.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Valid {
		t.Fatalf("repaired archive is not valid: %+v", verified)
	}
	if !verified.AddonInfoFound || !verified.AddonInfoValid {
		t.Fatalf("repaired addoninfo is not valid: %+v", verified)
	}
	if len(result.AddonInfoRepair.PreservedFields) == 0 || !result.AddonInfoRepair.RecoveredTruncatedText {
		t.Fatalf("repair metadata summary missing recovery details: %+v", result.AddonInfoRepair)
	}
}

func TestRepairVPKIntegrityRestoresWorkshopMetaIntoRepairedCopy(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "3756348495.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"materials/test.vmt": []byte("shader"),
	})
	meta := WorkshopMeta{
		WorkshopID:   "3756348495",
		Title:        "Music For Marine: Flying Color",
		Author:       "Workshop Author",
		Description:  "来自创意工坊的描述",
		PreviewURL:   "https://example.test/preview.jpg",
		FileURL:      "https://example.test/file.vpk",
		DownloadedAt: "2026-09-05T00:00:00Z",
		TimeUpdated:  "2026-09-04T00:00:00Z",
		Tags:         []string{"其他", "音乐"},
	}
	metaData, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	var rawMeta map[string]any
	if err := json.Unmarshal(metaData, &rawMeta); err != nil {
		t.Fatal(err)
	}
	rawMeta["future_field"] = "preserve-me"
	metaData, err = json.Marshal(rawMeta)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(GetMetaFilePath(vpkPath), metaData, 0644); err != nil {
		t.Fatal(err)
	}
	originalMeta, err := os.ReadFile(GetMetaFilePath(vpkPath))
	if err != nil {
		t.Fatal(err)
	}
	previewPath := filepath.Join(tempDir, "3756348495.jpg")
	originalPreview := []byte("preview")
	if err := os.WriteFile(previewPath, originalPreview, 0644); err != nil {
		t.Fatal(err)
	}

	result, err := (&App{}).RepairVPKIntegrity(vpkPath)
	if err != nil {
		t.Fatalf("repair: %v (result=%+v)", err, result)
	}
	if filepath.Base(result.OutputPath) != "3756348495.repaired.vpk" {
		t.Fatalf("unexpected repaired output name: %s", result.OutputPath)
	}
	defer os.Remove(result.OutputPath)
	defer os.Remove(GetMetaFilePath(result.OutputPath))
	defer os.Remove(strings.TrimSuffix(result.OutputPath, filepath.Ext(result.OutputPath)) + ".jpg")

	outputMeta, err := LoadWorkshopMeta(result.OutputPath)
	if err != nil {
		t.Fatalf("load repaired meta: %v", err)
	}
	if outputMeta == nil || outputMeta.WorkshopID != meta.WorkshopID || outputMeta.Title != meta.Title || outputMeta.Author != meta.Author || outputMeta.Description != meta.Description || outputMeta.PreviewURL != meta.PreviewURL || outputMeta.FileURL != meta.FileURL || outputMeta.DownloadedAt != meta.DownloadedAt || outputMeta.TimeUpdated != meta.TimeUpdated || len(outputMeta.Tags) != len(meta.Tags) {
		t.Fatalf("repaired meta = %+v, want preserved workshop metadata", outputMeta)
	}
	if currentMeta, readErr := os.ReadFile(GetMetaFilePath(vpkPath)); readErr != nil || string(currentMeta) != string(originalMeta) {
		t.Fatalf("source meta changed: readErr=%v", readErr)
	}
	outputMetaBytes, err := os.ReadFile(GetMetaFilePath(result.OutputPath))
	if err != nil || !strings.Contains(string(outputMetaBytes), `"future_field":"preserve-me"`) {
		t.Fatalf("repaired meta did not preserve unknown fields: readErr=%v content=%s", err, outputMetaBytes)
	}
	outputPreview := strings.TrimSuffix(result.OutputPath, filepath.Ext(result.OutputPath)) + ".jpg"
	if _, err := os.Stat(outputPreview); err != nil {
		t.Fatalf("repaired preview sidecar missing: %v", err)
	}
	if currentPreview, readErr := os.ReadFile(previewPath); readErr != nil || string(currentPreview) != string(originalPreview) {
		t.Fatalf("source preview changed: readErr=%v", readErr)
	}

	inspectRoot := t.TempDir()
	unpacked, err := (&App{}).UnpackVPKFile(result.OutputPath, inspectRoot)
	if err != nil {
		t.Fatalf("unpack repaired VPK: %v", err)
	}
	addonInfo, err := os.ReadFile(filepath.Join(unpacked.OutputDir, "addoninfo.txt"))
	if err != nil {
		t.Fatalf("read repaired addoninfo: %v", err)
	}
	for _, expected := range []string{
		"\"addontitle\"\t\t\"Music For Marine: Flying Color\"",
		"\"addonauthor\"\t\t\"Workshop Author\"",
		"\"addonURL0\"\t\t\"https://steamcommunity.com/sharedfiles/filedetails/?id=3756348495\"",
		"\"addonDescription\"\t\t\"来自创意工坊的描述\"",
	} {
		if !strings.Contains(string(addonInfo), expected) {
			t.Fatalf("repaired addoninfo missing %q:\n%s", expected, addonInfo)
		}
	}

	scanner := &App{rootDir: tempDir, workshopMetaEnabled: true}
	scanner.processVPKFileWithCache(result.OutputPath)
	cached, ok := scanner.vpkCache.Load(result.OutputPath)
	if !ok {
		t.Fatal("repaired VPK was not added to the scan cache")
	}
	scanned := cached.(*VPKFileCache).File
	if scanned.WorkshopID != meta.WorkshopID || scanned.Title != meta.Title || scanned.Author != meta.Author || scanned.Desc != meta.Description {
		t.Fatalf("scanned repaired VPK = %+v, want Workshop metadata", scanned)
	}
}

func TestRepairVPKIntegrityDegradesSafelyWhenWorkshopMetaIsInvalid(t *testing.T) {
	tempDir := t.TempDir()
	vpkPath := filepath.Join(tempDir, "broken.vpk")
	writeTestVPK(t, vpkPath, map[string][]byte{
		"materials/test.vmt": []byte("shader"),
	})
	if err := os.WriteFile(GetMetaFilePath(vpkPath), []byte("{invalid"), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := (&App{}).RepairVPKIntegrity(vpkPath)
	if err != nil {
		t.Fatalf("repair with invalid meta should degrade safely: %v", err)
	}
	defer os.Remove(result.OutputPath)
	if _, err := os.Stat(GetMetaFilePath(result.OutputPath)); !os.IsNotExist(err) {
		t.Fatalf("invalid source meta should not be propagated, stat err=%v", err)
	}
	inspectRoot := t.TempDir()
	unpacked, err := (&App{}).UnpackVPKFile(result.OutputPath, inspectRoot)
	if err != nil {
		t.Fatalf("unpack repaired VPK: %v", err)
	}
	addonInfo, err := os.ReadFile(filepath.Join(unpacked.OutputDir, "addoninfo.txt"))
	if err != nil {
		t.Fatalf("read repaired addoninfo: %v", err)
	}
	if !strings.Contains(string(addonInfo), `"addontitle"`) || !strings.Contains(string(addonInfo), `"broken"`) {
		t.Fatalf("filename fallback missing after invalid meta:\n%s", addonInfo)
	}
}

func TestInspectVPKIntegrityBatchKeepsItemLevelResults(t *testing.T) {
	tempDir := t.TempDir()
	validPath := filepath.Join(tempDir, "valid.vpk")
	brokenPath := filepath.Join(tempDir, "broken.vpk")
	writeTestVPK(t, validPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddontitle \"Valid\"\n}\n"),
	})
	writeTestVPK(t, brokenPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonDescription \"unfinished\n"),
	})

	results := (&App{}).InspectVPKIntegrityBatch([]string{validPath, brokenPath, validPath})
	if len(results) != 2 {
		t.Fatalf("got %d batch results, want 2 after de-duplication", len(results))
	}
	if !results[0].Report.Valid || results[0].Error != "" {
		t.Fatalf("valid result = %+v", results[0])
	}
	if results[1].Report.Valid || !results[1].Report.Repairable || results[1].Error != "" {
		t.Fatalf("broken result = %+v", results[1])
	}
}

func TestRepairVPKIntegrityBatchContinuesAfterNonRepairableItem(t *testing.T) {
	tempDir := t.TempDir()
	brokenPath := filepath.Join(tempDir, "broken.vpk")
	validPath := filepath.Join(tempDir, "valid.vpk")
	writeTestVPK(t, brokenPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddonDescription \"unfinished\n"),
	})
	writeTestVPK(t, validPath, map[string][]byte{
		"addoninfo.txt": []byte("\"AddonInfo\"\n{\n\taddontitle \"Valid\"\n}\n"),
	})

	results := (&App{}).RepairVPKIntegrityBatch([]string{validPath, brokenPath})
	if len(results) != 2 {
		t.Fatalf("got %d batch results, want 2", len(results))
	}
	if results[0].Error == "" || results[0].OutputPath != "" {
		t.Fatalf("valid archive should remain untouched: %+v", results[0])
	}
	if results[1].Error != "" || results[1].OutputPath == "" {
		t.Fatalf("repairable archive should produce a copy: %+v", results[1])
	}
	if _, err := os.Stat(results[1].OutputPath); err != nil {
		t.Fatalf("batch repair output missing: %v", err)
	}
	defer os.Remove(results[1].OutputPath)
}

func hasVPKIntegrityIssue(report parser.VPKIntegrityReport, code string) bool {
	for _, issue := range report.Issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
