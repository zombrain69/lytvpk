// 「当前筛选目标 × 对比范围」分析结果的纯汇总层（node --test 覆盖）。
//
// 后端一次分析会同时返回两类重叠：
//   - conflict_groups：无法判定胜负的重叠（真冲突）
//   - override_groups：能按 addonlist.txt 判定胜负的重叠（覆盖关系）
//
// 这里必须两类都统计：只统计冲突时，打开“按加载顺序判定胜负”的用户会看到
// 卡片上同时挂着“14 处覆盖”和“无冲突”两个自相矛盾的角标
// （复现见 scoped-conflict-summary 的测试与真机快照）。

export function conflictSeverityRank(severity) {
  if (severity === "critical") return 3;
  if (severity === "warning") return 2;
  return 1;
}

function severityText(severity) {
  if (severity === "critical") return "严重";
  if (severity === "warning") return "警告";
  return "普通";
}

function ensureEntry(byPath, vpk) {
  const path = String(vpk?.path || "");
  if (!path) return null;
  let entry = byPath.get(path);
  if (!entry) {
    entry = {
      severity: "info",
      groups: 0,
      files: 0,
      overrides: 0,
      overrideFiles: 0,
      winners: [],
    };
    byPath.set(path, entry);
  }
  return entry;
}

/**
 * buildScopedConflictSummary 把一次 ScopedConflictResult 压成「每个 Mod 一行」：
 * 冲突组数与文件数、覆盖组数与文件数、最高严重度、覆盖胜者名单。
 */
export function buildScopedConflictSummary(result) {
  const byPath = new Map();
  for (const group of result?.conflict_groups || []) {
    const severity = group?.severity || "info";
    const fileCount = Number(group?.file_count || 0);
    for (const vpk of group?.vpk_files || []) {
      const entry = ensureEntry(byPath, vpk);
      if (!entry) continue;
      entry.groups += 1;
      entry.files += fileCount;
      if (conflictSeverityRank(severity) > conflictSeverityRank(entry.severity)) {
        entry.severity = severity;
      }
    }
  }
  for (const group of result?.override_groups || []) {
    const severity = group?.severity || "info";
    const fileCount = Number(group?.file_count || 0);
    const winnerName = String(group?.winner?.name || group?.winner?.path || "").trim();
    for (const vpk of group?.vpk_files || []) {
      const entry = ensureEntry(byPath, vpk);
      if (!entry) continue;
      entry.overrides += 1;
      entry.overrideFiles += fileCount;
      if (winnerName && !entry.winners.includes(winnerName)) {
        entry.winners.push(winnerName);
      }
      if (conflictSeverityRank(severity) > conflictSeverityRank(entry.severity)) {
        entry.severity = severity;
      }
    }
  }
  return byPath;
}

/**
 * formatScopedConflictLabel 生成卡片角标文案。
 *
 * 约定：只要该 Mod 参与任何已判定覆盖，角标就必须把覆盖数量说出来，
 * 不能只剩一句“无冲突”。
 */
export function formatScopedConflictLabel(summary, options = {}) {
  const matchedBaseline = options.matchedBaseline !== false;
  if (!summary) return matchedBaseline ? "无冲突" : "未参与对比";

  const groups = Number(summary.groups || 0);
  const files = Number(summary.files || 0);
  const overrides = Number(summary.overrides || 0);
  const overrideFiles = Number(summary.overrideFiles || 0);

  if (groups === 0 && overrideFiles === 0) {
    return matchedBaseline ? "无冲突" : "未参与对比";
  }
  if (groups === 0) {
    return `无冲突 · 覆盖 ${overrideFiles} 处`;
  }
  const severity = severityText(summary.severity);
  if (overrideFiles > 0) {
    return `冲突 ${groups} 组 · 覆盖 ${overrideFiles} 处 · ${severity}`;
  }
  return `冲突 ${groups} 组 · ${files} 文件 · ${severity}`;
}

/** buildScopedConflictTitle 生成角标悬停说明（区分“没重叠”与“不参与对比”）。 */
export function buildScopedConflictTitle(summary, options = {}) {
  const matchedBaseline = options.matchedBaseline !== false;
  const scopeLabel = String(options.scopeLabel || "游戏内开启");
  if (!summary) {
    return matchedBaseline
      ? `已与“${scopeLabel}”对比：未发现重叠文件`
      : `此 Mod 不满足对比范围“${scopeLabel}”，本次分析未把它计入对比`;
  }
  const groups = Number(summary.groups || 0);
  const files = Number(summary.files || 0);
  const overrides = Number(summary.overrides || 0);
  const overrideFiles = Number(summary.overrideFiles || 0);
  const parts = [];
  if (groups > 0) {
    parts.push(`冲突 ${groups} 组（${files} 个文件，最高严重度：${severityText(summary.severity)}）`);
  }
  if (overrides > 0) {
    const winners = Array.isArray(summary.winners) ? summary.winners.filter(Boolean) : [];
    const winnerText = winners.length ? `，胜者：${winners.join("、")}` : "";
    parts.push(`覆盖 ${overrides} 组（${overrideFiles} 个文件，胜负已按 addonlist.txt 判定${winnerText}）`);
  }
  if (parts.length === 0) {
    return `已与“${scopeLabel}”对比：未发现重叠文件`;
  }
  return `与“${scopeLabel}”对比：${parts.join("；")}。点击查看详情`;
}

function conflictFileHasTag(file, wanted) {
  const target = String(wanted || "").trim().toLowerCase();
  if (!target) return false;
  if (String(file?.primaryTag || "").trim().toLowerCase() === target) return true;
  return (file?.secondaryTags || []).some(
    (tag) => String(tag || "").trim().toLowerCase() === target,
  );
}

function conflictRuleMatches(file, rule) {
  const type = String(rule?.type || "").trim().toLowerCase();
  const location = String(file?.location || "").trim().toLowerCase();
  switch (type) {
    case "enabled":
      return Boolean(file?.gameStateKnown) && Boolean(file?.gameEnabled) && location !== "disabled";
    case "not_disabled":
      return location !== "disabled";
    case "root":
      return location === "root";
    case "workshop":
      return location === "workshop";
    case "tag":
      return conflictFileHasTag(file, rule?.value);
    default:
      return false;
  }
}

/**
 * matchesConflictBaseline 复刻后端 conflictBaselineMatches：
 * 判断该 Mod 是否属于当前“对比范围”，用于区分“无冲突”和“未参与对比”。
 */
export function matchesConflictBaseline(file, rules, matchMode) {
  const list = Array.isArray(rules) && rules.length ? rules : [{ type: "enabled" }];
  const matchAll = String(matchMode || "").toLowerCase() === "and";
  for (const rule of list) {
    const matched = conflictRuleMatches(file, rule);
    if (matchAll && !matched) return false;
    if (!matchAll && matched) return true;
  }
  return matchAll;
}
