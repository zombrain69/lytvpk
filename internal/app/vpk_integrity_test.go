package app

import (
	"os"
	"path/filepath"
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
