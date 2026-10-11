// 「批量游戏内启用 / 关闭」的结果文案（与 DOM 无关，node --test 覆盖）。
//
// 语义提醒：这一组操作只写 addonlist.txt 的 0/1；
// 工具栏上的「批量启用 / 批量禁用」才是在 addons 与 disabled 目录之间搬文件。
// 文案走目录（mods.batch.gameState*；动作名复用 mods.batch.gameEnable|gameDisable）。

import { t } from "../../core/i18n-runtime.js";

export function batchGameStateActionLabel(enabled) {
  return enabled ? t("mods.batch.gameEnable") : t("mods.batch.gameDisable");
}

/**
 * canApplyBatchGameStateLocally 判断批量改完游戏内开关后能不能"本地更新状态"。
 *
 * 与单个开关（operations.js setGameEnabled）同一套判据 —— 两种情况必须整表重扫：
 *   ① 策略组自动联动改了别的成员（接口只给数量不给名单，本地拿不到）；
 *   ② 选中的里面有"未记录"条目：会被写进 addonlist.txt，顺序号/优先级跟着变。
 * 其余情况（最常见：已经在 addonlist 里，只翻 0/1）本地改 appState 即可 ——
 * 真机上一次全量 refreshFilesKeepFilter 是 165–175ms 长帧。
 *
 * 注意：返回值只说明"状态来源可信"；当前若按游戏内状态筛选，调用方仍要重筛一次
 * （列表成员会变），那是 filters.js 里本地重筛函数的活。
 */
export function canApplyBatchGameStateLocally({ targets, enforced = 0 } = {}) {
  if (Number(enforced) > 0) return false;
  const list = Array.isArray(targets) ? targets : [];
  return list.every((file) => Boolean(file?.gameStateKnown));
}

/** formatBatchGameStateConfirm 生成应用内确认弹窗的正文。 */
export function formatBatchGameStateConfirm(result, enabled) {
  const total = Number(result?.total || 0);
  const skipped = Number(result?.skipped || 0);
  const unrecorded = Number(result?.unrecorded || 0);
  const action = batchGameStateActionLabel(enabled);
  const lines = [
    t("mods.batch.gameStateConfirm", { total, action }),
    t("mods.batch.gameStateConfirmNote"),
  ];
  if (unrecorded > 0) {
    lines.push(t("mods.batch.gameStateConfirmUnrecorded", { count: unrecorded }));
  }
  if (skipped > 0) {
    lines.push(t("mods.batch.gameStateConfirmSkipped", { count: skipped }));
  }
  lines.push(t("mods.batch.gameStateConfirmProceed"));
  return lines.join("\n");
}

/** formatBatchGameStateSummary 生成操作后的提示文案。 */
export function formatBatchGameStateSummary(result, enabled) {
  const updated = (result?.updated || []).length;
  const unchanged = (result?.unchanged || []).length;
  const skipped = (result?.skipped || []).length;
  const enforced = Number(result?.enforced || 0);
  if (updated === 0 && unchanged === 0 && skipped === 0) {
    return t("mods.batch.gameStateEmpty");
  }
  const parts = [t("mods.batch.gameStateSummary", { count: updated, action: batchGameStateActionLabel(enabled) })];
  if (unchanged > 0) parts.push(t("mods.batch.gameStateSummaryUnchanged", { count: unchanged }));
  if (skipped > 0) parts.push(t("mods.batch.gameStateSummarySkipped", { count: skipped }));
  if (enforced > 0) parts.push(t("mods.batch.gameStateSummaryEnforced", { count: enforced }));
  return parts.join("；");
}
