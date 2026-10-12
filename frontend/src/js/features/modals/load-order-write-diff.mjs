// 加载顺序写入前的「条目级差异」展示层（纯函数，node --test 覆盖）。
//
// 与后端 AddonListWriteDiff 一一对应；只负责把数字与明细变成可读文案 / 安全 HTML。
// 差异的权威计算在后端（预览与写入共用同一份计划），这里不做任何过滤或推断。

import { t } from "../../core/i18n-runtime.js";
import { listSeparator } from "../../core/format-list.mjs";

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

const WRITE_DIFF_LIST_LIMIT = 20;

/** hasWriteDiffChanges 判断后端返回的差异里是否真的有事要发生。 */
export function hasWriteDiffChanges(diff) {
  if (!diff) return false;
  const added = Array.isArray(diff.added) ? diff.added.length : 0;
  const removed = Array.isArray(diff.removed) ? diff.removed.length : 0;
  const moved = Number(diff.movedTotal) || 0;
  const folded = Number(diff.duplicatesFolded) || 0;
  return added + removed > 0 || moved > 0 || folded > 0;
}

/** summarizeWriteDiff 生成摘要文案（条目数 + 各维度变化）。 */
export function summarizeWriteDiff(diff) {
  const lines = [];
  if (!diff) return lines;
  const before = Number(diff.countBefore) || 0;
  const after = Number(diff.countAfter) || 0;
  lines.push(t("loadOrder.writeDiff.countLine", { before, after }));
  const moved = Number(diff.movedTotal) || 0;
  if (moved > 0) lines.push(t("loadOrder.writeDiff.moved", { count: moved }));
  const removed = Array.isArray(diff.removed) ? diff.removed : [];
  if (removed.length > 0) lines.push(t("loadOrder.writeDiff.removed", { count: removed.length }));
  const added = Array.isArray(diff.added) ? diff.added : [];
  if (added.length > 0) lines.push(t("loadOrder.writeDiff.added", { count: added.length }));
  const folded = Number(diff.duplicatesFolded) || 0;
  if (folded > 0) lines.push(t("loadOrder.writeDiff.folded", { count: folded }));
  if (lines.length === 1) lines.push(t("loadOrder.writeDiff.noChanges"));
  return lines;
}

function renderKeyList(label, keys) {
  if (!keys.length) return "";
  const shown = keys
    .slice(0, WRITE_DIFF_LIST_LIMIT)
    .map((key) => `<code>${escapeHtmlText(key)}</code>`)
    .join(listSeparator());
  const rest = keys.length > WRITE_DIFF_LIST_LIMIT ? t("loadOrder.writeDiff.moreCount", { count: keys.length - WRITE_DIFF_LIST_LIMIT }) : "";
  return `<div class="write-diff-row"><span class="write-diff-label">${escapeHtmlText(label)}</span><span class="write-diff-values">${shown}${rest}</span></div>`;
}

/** buildWriteDiffHtml 生成确认弹窗的正文（全部转义）。 */
export function buildWriteDiffHtml(diff, { note = t("loadOrder.writeDiff.defaultNote") } = {}) {
  const summary = summarizeWriteDiff(diff);
  const movedEntries = Array.isArray(diff?.moved) ? diff.moved : [];
  const movedShown = movedEntries.slice(0, WRITE_DIFF_LIST_LIMIT);
  const movedLines = movedShown
    .map(
      (entry) =>
        `<li><code>${escapeHtmlText(entry?.key)}</code>${t("loadOrder.writeDiff.movedPosition", { old: Number(entry?.oldOrder) || 0, new: Number(entry?.newOrder) || 0 })}</li>`,
    )
    .join("");
  const movedRest = (Number(diff?.movedTotal) || 0) - movedShown.length;

  const sections = [];
  sections.push(`<div class="write-diff-summary">${summary.map((line) => `<p>${escapeHtmlText(line)}</p>`).join("")}</div>`);
  sections.push(renderKeyList(t("loadOrder.writeDiff.removedLabel"), Array.isArray(diff?.removed) ? diff.removed : []));
  sections.push(renderKeyList(t("loadOrder.writeDiff.addedLabel"), Array.isArray(diff?.added) ? diff.added : []));
  if (movedLines) {
    sections.push(
      `<div class="write-diff-row write-diff-moved"><span class="write-diff-label">${t("loadOrder.writeDiff.movedListLabel", { count: movedShown.length })}</span><ul>${movedLines}${
        movedRest > 0 ? `<li>${t("loadOrder.writeDiff.movedListRest", { count: movedRest })}</li>` : ""
      }</ul></div>`,
    );
  }
  sections.push(`<p class="write-diff-note">${escapeHtmlText(note)}</p>`);
  return sections.filter(Boolean).join("");
}

/** buildRestoreConfirmHtml 生成「恢复上一份备份」确认正文（调用方保证 candidate.found 为真）。 */
export function buildRestoreConfirmHtml(candidate) {
  if (!candidate?.found) return t("loadOrder.writeDiff.restoreNone");
  const sourceLabel =
    candidate.source === "first-edit"
      ? t("loadOrder.writeDiff.backupFirstEdit")
      : t("loadOrder.writeDiff.backupManaged", { name: candidate.name || "", kind: candidate.kind || t("loadOrder.writeDiff.unknown") });
  const lines = [t("loadOrder.writeDiff.restoreSource", { source: sourceLabel }), t("loadOrder.writeDiff.restoreTime", { time: candidate.createdAt || t("loadOrder.writeDiff.unknown") })];
  const diffHtml = buildWriteDiffHtml(candidate.diff, {
    note: t("loadOrder.writeDiff.restoreNote"),
  });
  return `<div class="write-diff-summary">${lines.map((line) => `<p>${escapeHtmlText(line)}</p>`).join("")}</div>${diffHtml}`;
}
