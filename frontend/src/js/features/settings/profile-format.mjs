// 启用方案应用结果的提示文案。

export function formatProfileApplySummary(result) {
  const name = String(result?.profileName || "").trim();
  const applied = Number(result?.appliedCount) || 0;
  const added = Number(result?.addedCount) || 0;
  const kept = Number(result?.keptCount) || 0;
  const groups = Number(result?.restoredGroups) || 0;
  const dependencies = Number(result?.restoredDependencies) || 0;

  const parts = [`${applied} 项已存在`, `${added} 项补回`, `${kept} 项保留`];
  if (groups > 0) parts.push(`恢复 ${groups} 个策略组`);
  if (dependencies > 0) parts.push(`恢复 ${dependencies} 条依赖`);

  const head = name ? `已应用方案“${name}”` : "已应用方案";
  return `${head}：${parts.join("，")}`;
}
