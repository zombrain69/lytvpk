// 「给这组打标签」的纯文案层（与 DOM 无关，node --test 覆盖）。

import { t } from "../../core/i18n-runtime.js";
import { clauseSeparator } from "../../core/format-list.mjs";

/** formatTagApplySummary 汇总一次打标签的结果。 */
export function formatTagApplySummary(result) {
  const applied = (result?.applied || []).length;
  const skipped = (result?.skipped || []).length;
  const missing = (result?.missing || []).length;
  const failed = (result?.failed || []).length;
  const tag = String(result?.tag || "");
  if (applied === 0 && skipped === 0 && missing === 0 && failed === 0) {
    return tag ? t("strategyGroups.tag.summary.noneWithTag", { tag }) : t("strategyGroups.tag.summary.noneEmpty");
  }
  const parts = [t("strategyGroups.tag.summary.applied", { count: applied, tag })];
  if (skipped > 0) parts.push(t("strategyGroups.tag.summary.skipped", { count: skipped }));
  if (missing > 0) parts.push(t("strategyGroups.tag.summary.missing", { count: missing }));
  if (failed > 0) parts.push(t("strategyGroups.tag.summary.failed", { count: failed }));
  return parts.join(clauseSeparator());
}

/** formatTagOrigin 说明标签是怎么推导出来的。 */
export function formatTagOrigin(origin) {
  switch (String(origin || "")) {
    case "common-tag":
      return t("strategyGroups.tag.origin.commonTag");
    case "subject":
      return t("strategyGroups.tag.origin.subject");
    case "label":
      return t("strategyGroups.tag.origin.label");
    default:
      return "";
  }
}

/** formatTagSuggestionLine 生成对话框顶部的说明。 */
export function formatTagSuggestionLine(suggested, origin) {
  const tag = String(suggested || "").trim();
  if (!tag) {
    return t("strategyGroups.tag.suggest.none");
  }
  const originText = formatTagOrigin(origin);
  return originText
    ? t("strategyGroups.tag.suggest.lineWithOrigin", { tag, origin: originText })
    : t("strategyGroups.tag.suggest.line", { tag });
}

/**
 * buildTagApplyPlan 把一条建议（或一个组）的标签提案整理成"标签 → 成员键"的应用计划：
 *
 *   - `tag`（整组统一标签）套用到全部成员；
 *   - `memberTags`（逐成员标签，来自外部建议文件）按标签归并；
 *   - 标签名大小写不同视为同一个；同一个键不会重复出现。
 *
 * 返回顺序：先整组标签，再逐成员标签（按首次出现顺序）。
 */
export function buildTagApplyPlan(suggestion) {
  if (!suggestion) return [];
  const memberKeys = Array.isArray(suggestion.memberKeys) ? suggestion.memberKeys : [];
  const memberNames = Array.isArray(suggestion.memberNames) ? suggestion.memberNames : [];
  const nameByKey = new Map();
  memberKeys.forEach((key, index) => {
    if (key) nameByKey.set(String(key), String(memberNames[index] || key));
  });

  const plan = [];
  const byFoldedTag = new Map();
  const addTag = (rawTag, key) => {
    const tag = String(rawTag || "").trim();
    if (!tag) return;
    const folded = tag.toLowerCase();
    if (!byFoldedTag.has(folded)) {
      const entry = { tag, keys: [], names: [] };
      byFoldedTag.set(folded, entry);
      plan.push(entry);
    }
    const entry = byFoldedTag.get(folded);
    if (!key) return;
    const normalizedKey = String(key);
    if (entry.keys.includes(normalizedKey)) return;
    entry.keys.push(normalizedKey);
    if (nameByKey.has(normalizedKey)) entry.names.push(nameByKey.get(normalizedKey));
  };

  // 整组标签优先：它代表"这一组的统一分类"。
  if (String(suggestion.tag || "").trim()) {
    memberKeys.forEach((key) => addTag(suggestion.tag, key));
  }
  const memberTags = suggestion.memberTags || {};
  Object.keys(memberTags).forEach((key) => {
    const tags = Array.isArray(memberTags[key]) ? memberTags[key] : [];
    tags.forEach((tag) => addTag(tag, key));
  });
  return plan;
}
