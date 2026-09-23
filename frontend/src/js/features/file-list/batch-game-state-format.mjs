// 「批量游戏内启用 / 关闭」的结果文案（与 DOM 无关，node --test 覆盖）。
//
// 语义提醒：这一组操作只写 addonlist.txt 的 0/1；
// 工具栏上的「批量启用 / 批量禁用」才是在 addons 与 disabled 目录之间搬文件。

export function batchGameStateActionLabel(enabled) {
  return enabled ? "游戏内启用" : "游戏内关闭";
}

/** formatBatchGameStateConfirm 生成应用内确认弹窗的正文。 */
export function formatBatchGameStateConfirm(result, enabled) {
  const total = Number(result?.total || 0);
  const skipped = Number(result?.skipped || 0);
  const unrecorded = Number(result?.unrecorded || 0);
  const lines = [
    `把选中的 ${total} 个 Mod 设为${batchGameStateActionLabel(enabled)}？`,
    "· 只改 addonlist.txt 的 0/1，不移动、不删除任何文件",
  ];
  if (unrecorded > 0) {
    lines.push(`· 其中 ${unrecorded} 个还没记录在 addonlist.txt，会按「未记录 Mod 插入位置」设置写入`);
  }
  if (skipped > 0) {
    lines.push(`· ${skipped} 个在 disabled 目录里，会被跳过（要改游戏开关请先用「批量启用」放回 addons）`);
  }
  lines.push("是否继续？");
  return lines.join("\n");
}

/** formatBatchGameStateSummary 生成操作后的提示文案。 */
export function formatBatchGameStateSummary(result, enabled) {
  const updated = (result?.updated || []).length;
  const unchanged = (result?.unchanged || []).length;
  const skipped = (result?.skipped || []).length;
  const enforced = Number(result?.enforced || 0);
  if (updated === 0 && unchanged === 0 && skipped === 0) {
    return "没有需要处理的 Mod";
  }
  const parts = [`已把 ${updated} 个 Mod 设为${batchGameStateActionLabel(enabled)}`];
  if (unchanged > 0) parts.push(`${unchanged} 个本来就是这个状态`);
  if (skipped > 0) parts.push(`${skipped} 个跳过（disabled 目录或不在列表里）`);
  if (enforced > 0) parts.push(`并按策略组自动联动另外 ${enforced} 个`);
  return parts.join("；");
}
