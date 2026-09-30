// 「标签依据」的纯逻辑（W6）：把 Go 侧 tagEvidence 转成可渲染的行。
// 单独成模块是为了能在 node:test 里直接测，不需要 DOM。

export const EVIDENCE_LEVEL_LABELS = {
  exact: "本体精确",
  pattern: "本体特征",
  inferred: "名称/关键词",
};

export const TAG_EVIDENCE_DISPLAY_LIMIT = 12;

/**
 * buildTagEvidenceRows 把 tagEvidence 数组转成渲染行。
 * 返回 { rows, hiddenCount }；rows 每项含 tag/level/levelLabel/detail。
 */
export function buildTagEvidenceRows(evidence, limit = TAG_EVIDENCE_DISPLAY_LIMIT) {
  const items = Array.isArray(evidence) ? evidence.filter(Boolean) : [];
  if (items.length === 0) {
    return { rows: [], hiddenCount: 0 };
  }

  const rows = items.slice(0, limit).map((item) => {
    const tag = String(item.tag || "").trim();
    const level = String(item.level || "").trim();
    const rule = String(item.rule || "").trim();
    const sources = Array.isArray(item.source)
      ? item.source.map((value) => String(value || "").trim()).filter(Boolean)
      : [];
    return {
      tag,
      level,
      levelLabel: EVIDENCE_LEVEL_LABELS[level] || level,
      detail: sources.length > 0 ? `${rule} · ${sources.join(" , ")}` : rule,
    };
  }).filter((row) => row.tag !== "" || row.detail !== "");

  return {
    rows,
    hiddenCount: Math.max(0, items.length - limit),
  };
}
