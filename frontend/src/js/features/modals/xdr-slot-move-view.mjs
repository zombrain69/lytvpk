// XDR「移到空槽」弹窗的纯展示逻辑（不碰 DOM，Node 单测直接跑）。
//
// 复用约定：槽位用途标签复用详情面板的 `modals.detail.slotDesignationGroup`；
// 本模块只做「数据 → 文案」的组装，不做任何 IO。

/** buildSlotOptionLabel 组装目标槽位下拉里的选项文案（带官方建议用途）。 */
export function buildSlotOptionLabel(option, t) {
  const label = String(option?.slotLabel || "").trim() ||
    String(option?.slot ?? "").padStart(3, "0");
  const name = String(option?.slotName || "").trim();
  if (!name) {
    return t("modals.xdrMove.optionLabelPlain", { label });
  }
  const group = String(option?.slotGroup || "").trim();
  return t("modals.xdrMove.optionLabel", {
    label,
    name,
    group: group ? t("modals.detail.slotDesignationGroup", { group }) : "",
  });
}

/** summarizeMoveResult 把后端结果折成展示需要的计数与失败清单。 */
export function summarizeMoveResult(result) {
  const processedFiles = Array.isArray(result?.processedFiles) ? result.processedFiles : [];
  const failures = processedFiles
    .filter((item) => item && item.error)
    .map((item) => ({ path: String(item.path || ""), error: String(item.error || "") }));
  const skipped = processedFiles.filter((item) => item && item.skipped && !item.error).length;
  return {
    renamedFiles: Number(result?.renamedFiles || 0),
    processedCount: processedFiles.length,
    skipped,
    failures,
  };
}

/** formatMoveNotice 生成执行后的提示文案（{level, message}）。 */
export function formatMoveNotice(result, t) {
  const info = summarizeMoveResult(result);
  const from = String(result?.fromSlot ?? "").padStart(3, "0");
  const to = String(result?.toSlot ?? "").padStart(3, "0");
  if (info.renamedFiles > 0 && info.failures.length === 0) {
    return {
      level: "success",
      message: info.skipped > 0
        ? t("modals.xdrMove.doneWithSkipped", {
          count: info.renamedFiles,
          from,
          to,
          skipped: info.skipped,
        })
        : t("modals.xdrMove.done", { count: info.renamedFiles, from, to }),
    };
  }
  if (info.renamedFiles > 0) {
    return {
      level: "warning",
      message: t("modals.xdrMove.partial", {
        count: info.renamedFiles,
        failed: info.failures.length,
        reason: info.failures[0].error,
      }),
    };
  }
  if (info.failures.length > 0) {
    return { level: "error", message: info.failures[0].error };
  }
  return { level: "info", message: t("modals.xdrMove.noChange") };
}

/** splitMoveFiles 把文件清单拆成「展示 + 折叠计数」，避免一屏刷上百行。 */
export function splitMoveFiles(files, limit = 24) {
  const list = Array.isArray(files) ? files : [];
  const max = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : 24;
  return {
    visible: list.slice(0, max),
    hiddenCount: Math.max(0, list.length - max),
  };
}
