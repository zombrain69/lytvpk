package parser

import (
	"os"
	"testing"
)

// 临时探针：把真实 VPK 喂给解析器，打印最终标签（只在设置 LYTVPK_PROBE_VPK 时运行）。
func TestProbeRealMod(t *testing.T) {
	path := os.Getenv("LYTVPK_PROBE_VPK")
	if path == "" {
		t.Skip("LYTVPK_PROBE_VPK 未设置")
	}
	file, err := ParseVPKFileMetadata(path)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	t.Logf("name=%s primary=%s title=%q", file.Name, file.PrimaryTag, file.Title)
	t.Logf("secondary=%v", file.SecondaryTags)
	t.Logf("subjects=%v confidence=%s", file.ContentSubjects, file.SubjectConfidence)
	t.Logf("voiceCharacters=%v", file.VoiceCharacters)
}
