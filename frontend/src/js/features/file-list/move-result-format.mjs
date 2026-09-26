// 批量文件操作结果的文案层（移动 / 删除 / 归档移动共用）。
//
// 对齐 FireAxe 的 `FailedImportResultItem`：每个失败条目都带自己的原因，
// 界面不能只显示 errors[0]（那会让人以为只有第一个文件失败）。
// 原因本身再过一遍 core/action-explanation.mjs：能认出类型时补上"接下来怎么做"，
// 认不出就保留原文（对齐 FireAxe ObjectExplanationManager 的"按类型回退 + 永远有话说"）。

import { explainOperationError, formatExplanation } from "../../core/action-explanation.mjs";

const DEFAULT_FAILURE_LIMIT = 3;

/**
 * formatMoveFailures 把 MoveResult 转成"失败 N 个 + 前几条原因"的可读文案。
 * 返回空串表示没有失败。
 */
export function formatMoveFailures(result, limit = DEFAULT_FAILURE_LIMIT) {
  const failCount = Number(result?.failCount) || 0;
  const errors = (Array.isArray(result?.errors) ? result.errors : [])
    .map((message) => String(message || "").trim())
    .filter(Boolean);
  if (failCount <= 0 && errors.length === 0) return "";

  const total = Math.max(failCount, errors.length);
  const shown = errors.slice(0, Math.max(1, Number(limit) || DEFAULT_FAILURE_LIMIT)).map((message) => {
    const explained = explainOperationError(message);
    if (!explained.matched) return message;
    const line = formatExplanation(explained);
    // 保留"哪个文件"：解释层替换的是原因描述，不该把对象名一起吞掉。
    return explained.subject && !line.startsWith(explained.subject) ? `${explained.subject}：${line}` : line;
  });
  const head = `${total} 个文件操作失败`;
  if (shown.length === 0) return head;

  const rest = total - shown.length;
  const tail = rest > 0 ? `；另有 ${rest} 条，详见控制台日志` : "";
  return `${head}：${shown.join("；")}${tail}`;
}

/** formatMoveSummary 生成成功 / 跳过 / 取消的一行汇总（失败部分由上面那个函数负责）。 */
export function formatMoveSummary(result) {
  const parts = [];
  const success = Number(result?.successCount) || 0;
  const skipped = Number(result?.skippedCount) || 0;
  if (success > 0) parts.push(`成功 ${success} 个`);
  if (skipped > 0) parts.push(`跳过 ${skipped} 个冲突文件`);
  if (result?.cancelled) parts.push("后续已取消");
  return parts.join(" · ");
}
