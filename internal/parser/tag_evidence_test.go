package parser

import "testing"

// TestTagEvidenceRecordsLevelsAndSources 覆盖 W6：证据要能说清"哪条规则、哪条路径、什么强度"。
func TestTagEvidenceRecordsLevelsAndSources(t *testing.T) {
	archive := testArchiveFiles(
		vpkFile("models/w_models/weapons", "w_rifle_ak47", "mdl"),          // 本体精确锚点 → exact
		vpkFile("sound/player/hunter/voice/warn", "hunter_warn_01", "wav"), // 角色目录 → pattern
		vpkFile("particles", "weapon_fx", "pcf"),                           // 文件类别 → pattern
	)
	index := buildArchivePathIndex(archive)
	items := index.evidence.list()
	if len(items) == 0 {
		t.Fatalf("没有记录任何证据")
	}

	byTag := make(map[string]TagEvidence, len(items))
	for _, item := range items {
		byTag[item.Tag] = item
	}

	ak := byTag["AK47"]
	if ak.Level != EvidenceLevelExact {
		t.Fatalf("AK47 证据强度应为 exact，实际 %+v", ak)
	}
	if ak.Rule != "entity:weapon_rifle_ak47" {
		t.Fatalf("AK47 规则应为 entity:weapon_rifle_ak47，实际 %+v", ak)
	}
	if len(ak.Source) == 0 || ak.Source[0] != "models/w_models/weapons/w_rifle_ak47.mdl" {
		t.Fatalf("AK47 应带真实来源路径，实际 %+v", ak)
	}

	if byTag["粒子特效"].Level != EvidenceLevelPattern {
		t.Fatalf("粒子特效应为 pattern，实际 %+v", byTag["粒子特效"])
	}
	if byTag["声音"].Level != EvidenceLevelPattern {
		t.Fatalf("声音应为 pattern，实际 %+v", byTag["声音"])
	}
	if _, ok := byTag["hunter"]; ok {
		// hunter 由 collectCharacterTags 记录（pattern）；这里只要求存在且强度合理
		if byTag["hunter"].Level == EvidenceLevelInferred {
			t.Fatalf("hunter 不应是 inferred，实际 %+v", byTag["hunter"])
		}
	}
}

// TestEvidenceIsCappedAndDeduped 锁定体量上限：清单要被大模型读进上下文。
func TestEvidenceIsCappedAndDeduped(t *testing.T) {
	recorder := newTagEvidenceRecorder()
	for i := 0; i < 10; i++ {
		recorder.record("铁喷", "entity:weapon_shotgun_chrome", EvidenceLevelExact, "models/w_pumpshotgun_a.mdl")
	}
	recorder.record("铁喷", "entity:weapon_shotgun_chrome", EvidenceLevelExact, "models/v_shotgun_chrome.mdl")
	items := recorder.list()
	if len(items) != 1 {
		t.Fatalf("同名标签应合并，实际 %d 条", len(items))
	}
	if len(items[0].Source) != 2 {
		t.Fatalf("来源应去重且 ≤3，实际 %v", items[0].Source)
	}

	for i := 0; i < 100; i++ {
		recorder.record("标签"+string(rune('A'+i%26))+string(rune('a'+i%26)), "content:x", EvidenceLevelInferred, "")
	}
	if got := len(recorder.list()); got > tagEvidenceLimit {
		t.Fatalf("证据条数应受上限约束，实际 %d", got)
	}
}

// TestEvidenceUpgradeReplacesSource 锁定"强度升级要连来源一起换"：
// 否则会出现 rule=本体锚点、source=关键词路径 的自相矛盾证据。
func TestEvidenceUpgradeReplacesSource(t *testing.T) {
	recorder := newTagEvidenceRecorder()
	// 先被关键词命中（弱证据）
	recorder.record("消音", "weapon:消音", EvidenceLevelInferred, "sound/weapons/smg_silenced/gunother/x.wav")
	// 再被本体锚点命中（强证据）
	recorder.record("消音", "entity:weapon_smg_silenced", EvidenceLevelExact, "models/w_models/weapons/w_smg_a.mdl")

	items := recorder.list()
	if len(items) != 1 {
		t.Fatalf("应合并为一条，实际 %d", len(items))
	}
	item := items[0]
	if item.Level != EvidenceLevelExact || item.Rule != "entity:weapon_smg_silenced" {
		t.Fatalf("规则未升级到最强证据：%+v", item)
	}
	if len(item.Source) != 1 || item.Source[0] != "models/w_models/weapons/w_smg_a.mdl" {
		t.Fatalf("升级后来源必须换成锚点路径，实际 %v", item.Source)
	}
}
