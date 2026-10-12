// 策略组在「策略组管理」窗口（features/mod-groups/strategy-group-manager.js）里的展示文案。

import { t } from "../../core/i18n-runtime.js";
import { listSeparator } from "../../core/format-list.mjs";

const STRATEGY_KEYS = {
  single: "strategyGroups.strategy.single",
  single_random: "strategyGroups.strategy.singleRandom",
  all: "strategyGroups.strategy.all",
  off: "strategyGroups.strategy.off",
};

export function formatStrategyGroupStrategy(value) {
  return t(STRATEGY_KEYS[String(value || "").trim()] || "strategyGroups.strategy.unset");
}

export function formatStrategyGroupApplySummary(result) {
  const name = String(result?.groupName || "").trim();
  const strategy = formatStrategyGroupStrategy(result?.strategy);
  const enabled = Array.isArray(result?.enabled) ? result.enabled.length : 0;
  const disabled = Array.isArray(result?.disabled) ? result.disabled.length : 0;
  const picked = String(result?.pickedName || "").trim();

  const head = name ? t("strategyGroups.apply.titled", { name }) : t("strategyGroups.apply.untitled");
  const pickedText = picked ? t("strategyGroups.apply.kept", { name: picked }) : "";
  return t("strategyGroups.apply.summary", { head, strategy, enabled, disabled, kept: pickedText });
}

// formatStrategyGroupEnforcementNotice 描述一次手动开关触发的自动联动。
// 返回空字符串表示这次操作没有联动其它成员。
export function formatStrategyGroupEnforcementNotice(changedCount) {
  const count = Number(changedCount) || 0;
  if (count <= 0) {
    return "";
  }
  return t("strategyGroups.enforcement.notice", { count });
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
  const names = (summary?.names || []).slice(0, 5).join(listSeparator());
  const more = (summary?.names || []).length > 5 ? t("strategyGroups.batch.confirm.more", { count: summary.names.length }) : "";
  const list = names ? `\n\n${names}${more}` : "";
  switch (String(action || "")) {
    case "delete":
      return t("strategyGroups.batch.confirm.delete", { count, members }) + list;
    case "enforce_on":
      return t("strategyGroups.batch.confirm.enforceOn", { count, list });
    case "enforce_off":
      return t("strategyGroups.batch.confirm.enforceOff", { count, list });
    case "set_tier":
      return t("strategyGroups.batch.confirm.setTier", { count, members, list });
    case "clear_tier":
      return t("strategyGroups.batch.confirm.clearTier", { count, list });
    default:
      return t("strategyGroups.batch.confirm.default", { count, list });
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
    (skipped > 0 ? t("strategyGroups.batch.result.skipped", { count: skipped }) : "") +
    (detached > 0 ? t("strategyGroups.batch.result.detached", { count: detached }) : "");
  switch (String(action || "")) {
    case "delete":
      return t("strategyGroups.batch.result.delete", { deleted, suffix, remaining });
    case "enforce_on":
      return t("strategyGroups.batch.result.enforceOn", { updated, suffix });
    case "enforce_off":
      return t("strategyGroups.batch.result.enforceOff", { updated, suffix });
    case "set_tier":
      return t("strategyGroups.batch.result.setTier", { updated, tier: result?.tier ?? "", suffix });
    case "clear_tier":
      return t("strategyGroups.batch.result.clearTier", { updated, suffix });
    default:
      return t("strategyGroups.batch.result.default", { updated, suffix });
  }
}

/**
 * formatStrategyGroupSelectionLabel 生成批量工具条上的"已选 N 个组（M 个成员）"。
 *
 * hiddenCount：其中有多少个组被当前搜索筛掉了（列表里看不见但仍是批量目标）。
 * 组勾选和成员勾选是同一类问题：看不见的勾选必须报出来，否则"全选"看起来只选了眼前的。
 */
export function formatStrategyGroupSelectionLabel(summary, hiddenCount = 0) {
  const count = Number(summary?.count || 0);
  if (count <= 0) return t("strategyGroups.selection.none");
  const members = Number(summary?.memberCount || 0);
  const missing = Number(summary?.missingCount || 0);
  const hidden = Number(hiddenCount) || 0;
  const missingPart = missing > 0 ? t("strategyGroups.selection.missingPart", { count: missing }) : "";
  const base = t("strategyGroups.selection.base", { count, members, missing: missingPart });
  return hidden > 0 ? t("strategyGroups.selection.hidden", { base, count: hidden }) : base;
}

/**
 * formatStrategyGroupFilterState 生成管理窗口里的"当前筛选"文案：
 *   - 没有筛选 → 当前筛选：全部分组
 *   - 有筛选   → 当前筛选：A、B 等 3 个组
 * 只用于显示，不参与筛选计算。
 */
export function formatStrategyGroupFilterState(ids, groups, limit = 3) {
  const list = Array.isArray(ids) ? ids.map((id) => String(id)) : [];
  if (list.length === 0) return t("strategyGroups.filter.all");
  const nameById = new Map(
    (Array.isArray(groups) ? groups : []).map((group) => [String(group?.id), String(group?.name || "")]),
  );
  const names = list.map((id) => nameById.get(id) || id);
  const shown = names.slice(0, Math.max(1, limit)).join(listSeparator());
  const more = names.length > limit ? t("strategyGroups.filter.more", { count: names.length }) : "";
  return t("strategyGroups.filter.state", { shown, more });
}
