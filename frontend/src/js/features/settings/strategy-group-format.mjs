// 策略组在「策略组管理」窗口（features/mod-groups/strategy-group-manager.js）里的展示文案。

const STRATEGY_LABELS = {
  single: "互斥单选",
  single_random: "随机单选",
  all: "全部开启",
  off: "全部关闭",
};

export function formatStrategyGroupStrategy(value) {
  return STRATEGY_LABELS[String(value || "").trim()] || "未设置";
}

export function formatStrategyGroupApplySummary(result) {
  const name = String(result?.groupName || "").trim();
  const strategy = formatStrategyGroupStrategy(result?.strategy);
  const enabled = Array.isArray(result?.enabled) ? result.enabled.length : 0;
  const disabled = Array.isArray(result?.disabled) ? result.disabled.length : 0;
  const picked = String(result?.pickedName || "").trim();

  const head = name ? `已应用策略组“${name}”` : "已应用策略组";
  const pickedText = picked ? `，保留 ${picked}` : "";
  return `${head}（${strategy}）：开启 ${enabled} 项，关闭 ${disabled} 项${pickedText}`;
}

// formatStrategyGroupEnforcementNotice 描述一次手动开关触发的自动联动。
// 返回空字符串表示这次操作没有联动其它成员。
export function formatStrategyGroupEnforcementNotice(changedCount) {
  const count = Number(changedCount) || 0;
  if (count <= 0) {
    return "";
  }
  return `已按策略组自动联动另外 ${count} 个 Mod`;
}

// ---- 策略组批量管理（多选组 → 批量删除 / 批量开关联动 / 批量权重）----

/**
 * summarizeStrategyGroupSelection 汇总当前选中的组：
 * 组数、成员总数、以及其中"文件已不在列表里"的成员数。
 * @param {string[]} ids 选中的组 ID
 * @param {Array<{id:string, members?:Array<unknown>}>} groups 全部策略组
 * @param {Map<string, number>} [missingByGroupId] 每个组的缺失成员数
 */
export function summarizeStrategyGroupSelection(ids, groups, missingByGroupId) {
  const selected = new Set((Array.isArray(ids) ? ids : []).map((id) => String(id)));
  const list = (Array.isArray(groups) ? groups : []).filter((group) => selected.has(String(group?.id)));
  const memberCount = list.reduce((total, group) => total + (group?.members || []).length, 0);
  const missingCount = list.reduce(
    (total, group) => total + Number(missingByGroupId?.get?.(String(group.id)) || 0),
    0,
  );
  return {
    count: list.length,
    memberCount,
    missingCount,
    names: list.map((group) => String(group?.name || group?.id || "")).filter(Boolean),
  };
}

/** formatStrategyGroupBatchConfirm 生成批量操作的确认文案。 */
export function formatStrategyGroupBatchConfirm(action, summary) {
  const count = Number(summary?.count || 0);
  const members = Number(summary?.memberCount || 0);
  const names = (summary?.names || []).slice(0, 5).join("、");
  const more = (summary?.names || []).length > 5 ? ` 等 ${summary.names.length} 个` : "";
  const list = names ? `\n\n${names}${more}` : "";
  switch (String(action || "")) {
    case "delete":
      return (
        `删除选中的 ${count} 个策略组（共 ${members} 个成员）：` +
        "只删除 groups.json 里的记录，不改动 addonlist.txt、也不删除任何 Mod 文件；" +
        "组成员已写入的分层（priority.json）会保留；被删组的下级分组会回到顶层。" +
        list
      );
    case "enforce_on":
      return `给选中的 ${count} 个策略组开启自动联动：之后手动开关组内成员会按策略联动同组其它成员。${list}`;
    case "enforce_off":
      return `关闭选中 ${count} 个策略组的自动联动：策略组仍可手动「按策略应用」，只是不再自动联动。${list}`;
    case "set_tier":
      return `给选中的 ${count} 个策略组设置同一个组权重（共 ${members} 个成员受影响）。${list}`;
    case "clear_tier":
      return `清除选中 ${count} 个策略组的组权重（组本身与成员都保留）。${list}`;
    default:
      return `对选中的 ${count} 个策略组执行批量操作。${list}`;
  }
}

/** formatStrategyGroupBatchResult 生成批量操作后的提示文案。 */
export function formatStrategyGroupBatchResult(action, result) {
  const updated = (result?.updated || []).length;
  const deleted = (result?.deleted || []).length;
  const skipped = (result?.skipped || []).length;
  const detached = (result?.detachedChildren || []).length;
  const remaining = Number(result?.remaining || 0);
  const suffix =
    (skipped > 0 ? `；${skipped} 个已经不存在，已跳过` : "") +
    (detached > 0 ? `；${detached} 个下级分组已回到顶层` : "");
  switch (String(action || "")) {
    case "delete":
      return `已删除 ${deleted} 个策略组${suffix}（现在还剩 ${remaining} 个组）`;
    case "enforce_on":
      return `已给 ${updated} 个策略组开启自动联动${suffix}`;
    case "enforce_off":
      return `已关闭 ${updated} 个策略组的自动联动${suffix}`;
    case "set_tier":
      return `已给 ${updated} 个策略组设置权重 ${result?.tier ?? ""}${suffix}（需「按分层应用」才重排）`;
    case "clear_tier":
      return `已清除 ${updated} 个策略组的权重${suffix}`;
    default:
      return `已更新 ${updated} 个策略组${suffix}`;
  }
}

/** formatStrategyGroupSelectionLabel 生成批量工具条上的"已选 N 个组（M 个成员）"。 */
export function formatStrategyGroupSelectionLabel(summary) {
  const count = Number(summary?.count || 0);
  if (count <= 0) return "未选择组";
  const members = Number(summary?.memberCount || 0);
  const missing = Number(summary?.missingCount || 0);
  return `已选 ${count} 个组（${members} 个成员${missing > 0 ? `，含 ${missing} 个缺失` : ""}）`;
}

/**
 * formatStrategyGroupFilterState 生成管理窗口里的"当前筛选"文案：
 *   - 没有筛选 → 当前筛选：全部分组
 *   - 有筛选   → 当前筛选：A、B 等 3 个组
 * 只用于显示，不参与筛选计算。
 */
export function formatStrategyGroupFilterState(ids, groups, limit = 3) {
  const list = Array.isArray(ids) ? ids.map((id) => String(id)) : [];
  if (list.length === 0) return "当前筛选：全部分组";
  const nameById = new Map(
    (Array.isArray(groups) ? groups : []).map((group) => [String(group?.id), String(group?.name || "")]),
  );
  const names = list.map((id) => nameById.get(id) || id);
  const shown = names.slice(0, Math.max(1, limit)).join("、");
  const more = names.length > limit ? ` 等 ${names.length} 个组` : "";
  return `当前筛选：${shown}${more}`;
}
