// 加载顺序写入前的「条目级差异」展示层（纯函数，node --test 覆盖）。
//
// 与后端 AddonListWriteDiff 一一对应；只负责把数字与明细变成可读文案 / 安全 HTML。
// 差异的权威计算在后端（预览与写入共用同一份计划），这里不做任何过滤或推断。

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
  lines.push(`条目数：${before} → ${after}`);
  const moved = Number(diff.movedTotal) || 0;
  if (moved > 0) lines.push(`重排 ${moved} 条（相对顺序发生变化）`);
  const removed = Array.isArray(diff.removed) ? diff.removed : [];
  if (removed.length > 0) lines.push(`移除 ${removed.length} 条`);
  const added = Array.isArray(diff.added) ? diff.added : [];
  if (added.length > 0) lines.push(`新增 ${added.length} 条`);
  const folded = Number(diff.duplicatesFolded) || 0;
  if (folded > 0) lines.push(`合并重复条目 ${folded} 条`);
  if (lines.length === 1) lines.push("没有需要写入的变化");
  return lines;
}

function renderKeyList(label, keys) {
  if (!keys.length) return "";
  const shown = keys
    .slice(0, WRITE_DIFF_LIST_LIMIT)
    .map((key) => `<code>${escapeHtmlText(key)}</code>`)
    .join("、");
  const rest = keys.length > WRITE_DIFF_LIST_LIMIT ? `（还有 ${keys.length - WRITE_DIFF_LIST_LIMIT} 条）` : "";
  return `<div class="write-diff-row"><span class="write-diff-label">${escapeHtmlText(label)}</span><span class="write-diff-values">${shown}${rest}</span></div>`;
}

/** buildWriteDiffHtml 生成确认弹窗的正文（全部转义）。 */
export function buildWriteDiffHtml(diff, { note = "写入前会自动保留备份；所有 Mod 的游戏内开关状态保持不变。" } = {}) {
  const summary = summarizeWriteDiff(diff);
  const movedEntries = Array.isArray(diff?.moved) ? diff.moved : [];
  const movedShown = movedEntries.slice(0, WRITE_DIFF_LIST_LIMIT);
  const movedLines = movedShown
    .map(
      (entry) =>
        `<li><code>${escapeHtmlText(entry?.key)}</code>：第 ${Number(entry?.oldOrder) || 0} 位 → 第 ${Number(entry?.newOrder) || 0} 位</li>`,
    )
    .join("");
  const movedRest = (Number(diff?.movedTotal) || 0) - movedShown.length;

  const sections = [];
  sections.push(`<div class="write-diff-summary">${summary.map((line) => `<p>${escapeHtmlText(line)}</p>`).join("")}</div>`);
  sections.push(renderKeyList("移除：", Array.isArray(diff?.removed) ? diff.removed : []));
  sections.push(renderKeyList("新增：", Array.isArray(diff?.added) ? diff.added : []));
  if (movedLines) {
    sections.push(
      `<div class="write-diff-row write-diff-moved"><span class="write-diff-label">重排明细（前 ${movedShown.length} 条）：</span><ul>${movedLines}${
        movedRest > 0 ? `<li>…另有 ${movedRest} 条重排未逐条列出</li>` : ""
      }</ul></div>`,
    );
  }
  sections.push(`<p class="write-diff-note">${escapeHtmlText(note)}</p>`);
  return sections.filter(Boolean).join("");
}

/** buildRestoreConfirmHtml 生成「恢复上一份备份」确认正文（调用方保证 candidate.found 为真）。 */
export function buildRestoreConfirmHtml(candidate) {
  if (!candidate?.found) return "还没有可恢复的备份。";
  const sourceLabel =
    candidate.source === "first-edit"
      ? "首次编辑备份（LytVPK 第一次改写前保存的游戏原始内容）"
      : `历史备份 ${candidate.name || ""}（类型：${candidate.kind || "未知"}）`;
  const lines = [`将用：${sourceLabel}`, `备份时间：${candidate.createdAt || "未知"}`];
  const diffHtml = buildWriteDiffHtml(candidate.diff, {
    note: "恢复会覆盖当前 addonlist.txt；当前内容会先自动另存为 before-restore 备份。",
  });
  return `<div class="write-diff-summary">${lines.map((line) => `<p>${escapeHtmlText(line)}</p>`).join("")}</div>${diffHtml}`;
}
