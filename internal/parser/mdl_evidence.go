package parser

import (
	"io"

	"l4d2-manager-next/pkg/valve/vpk"
)

// 通道 3：.mdl 材质表证据。
//
// 为什么需要：作者把模型重命名成自己的名字后，路径再也看不出替换目标；但模型头部仍记着
// 自己引用的材质名与搜索目录，这些名字通常保留着本体特征（`v_rifle_ak47`、`w_eq_medkit`…）。
//
// 成本控制（列表页要快）：
//   - 只在"还没有任何 exact 级证据"时触发（已经有本体精确路径的包不需要再读模型）；
//   - 每个包最多读 mdlEvidenceFileLimit 个模型、单个模型最多读 mdlEvidenceMaxBytes；
//   - 任何读取/解析失败都安静跳过，绝不影响既有标签。
const (
	mdlEvidenceFileLimit = 4
	mdlEvidenceMaxBytes  = 8 << 20
)

// applyModelMaterialEvidence 读取模型材质表，把材质名 token 喂给通道 2 的索引。
func applyModelMaterialEvidence(opener *vpk.Opener, index archivePathIndex, tags map[string]bool, evidence *tagEvidenceRecorder) {
	if opener == nil || len(index.modelFiles) == 0 || len(stockTokenIndex) == 0 {
		return
	}
	if hasExactEvidence(evidence) {
		return
	}
	for _, file := range index.modelFiles {
		if file == nil || file.Size() > mdlEvidenceMaxBytes {
			continue
		}
		reader, err := file.Open(opener)
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(reader, mdlEvidenceMaxBytes))
		_ = reader.Close()
		if err != nil {
			continue
		}
		tokens := ModelMaterialTokens(data)
		if len(tokens) == 0 {
			continue
		}
		applyStockTokenLookup(tokens, tags, evidence, "mdl:", file.Name())
	}
}

// hasExactEvidence 判断当前是否已经拿到"本体精确路径"级证据。
func hasExactEvidence(evidence *tagEvidenceRecorder) bool {
	if evidence == nil {
		return false
	}
	for _, item := range evidence.items {
		if item.Level == EvidenceLevelExact {
			return true
		}
	}
	return false
}
