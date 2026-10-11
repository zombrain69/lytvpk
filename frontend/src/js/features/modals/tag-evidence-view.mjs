// 「标签依据」的纯逻辑（W6）：把 Go 侧 tagEvidence 转成可渲染的行。
// 单独成模块是为了能在 node:test 里直接测，不需要 DOM。

import { t } from "../../core/i18n-runtime.js";

export const EVIDENCE_LEVEL_LABEL_KEYS = {
  exact: "modals.tagEvidence.levelExact",
  pattern: "modals.tagEvidence.levelPattern",
  inferred: "modals.tagEvidence.levelInferred",
};

// 模块求值期不取文案：按 key 延迟解析（语言可在运行时切换）。
const evidenceLevelLabel = (level) => (EVIDENCE_LEVEL_LABEL_KEYS[level] ? t(EVIDENCE_LEVEL_LABEL_KEYS[level]) : level);

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
      levelLabel: evidenceLevelLabel(level),
      detail: sources.length > 0 ? `${rule} · ${sources.join(" , ")}` : rule,
    };
  }).filter((row) => row.tag !== "" || row.detail !== "");

  return {
    rows,
    hiddenCount: Math.max(0, items.length - limit),
  };
}
