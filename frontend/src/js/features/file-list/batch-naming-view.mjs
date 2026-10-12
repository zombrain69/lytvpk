// 批量规范化命名的纯逻辑（路线图第 20 项 P2）：计划分组、默认勾选、执行请求、
// 剪贴板载荷序列化。UI 接线在 batch-naming.js，展示映射在 naming-display.mjs。
//
// 关键纪律（与后端 ApplyNamingNormalization 同口径）：
//   - 执行请求把预览里逐字展示的 path / oldName / newName 原样回传，执行阶段不再推演；
//   - 低置信（pendingReview）默认不勾选；无法改名的项（actionable=false）不参与执行。

// splitPlanItems 把计划拆成三组：可改名（高置信）、待复核（低置信）、跳过（含原因）。
export function splitPlanItems(plan) {
  const items = Array.isArray(plan && plan.items) ? plan.items : [];
  return {
    actionable: items.filter((item) => item.actionable && !item.pendingReview),
    pending: items.filter((item) => item.actionable && item.pendingReview),
    skipped: items.filter((item) => !item.actionable),
  };
}

// selectablePaths 全部可勾选路径（高置信 + 待复核）。
export function selectablePaths(plan) {
  const groups = splitPlanItems(plan);
  return [...groups.actionable, ...groups.pending].map((item) => item.path);
}

// defaultCheckedPaths 默认勾选「可改名且非低置信」的项（低置信项默认不勾）。
export function defaultCheckedPaths(plan) {
  return splitPlanItems(plan).actionable.map((item) => item.path);
}

// buildApplyRequests 把勾选项拼成执行请求（path / oldName / newName 原样回传）。
// checkedPaths 允许是数组或 Set（UI 状态用 Set；测试与序列化路径常用数组）——
// 只用 Array.isArray 判别会把 Set 当成空集（真机回归：按钮计数恒 0、执行静默不触发）。
export function buildApplyRequests(plan, checkedPaths) {
  const source =
    checkedPaths instanceof Set ? checkedPaths : Array.isArray(checkedPaths) ? checkedPaths : [];
  const checked = new Set(source);
  const items = Array.isArray(plan && plan.items) ? plan.items : [];
  return items
    .filter((item) => item.actionable && item.newName && checked.has(item.path))
    .map((item) => ({ path: item.path, oldName: item.oldName, newName: item.newName }));
}

// serializePlanPayload 生成「复制计划」的 JSON 文本（含旧名 ↔ 新名对照与判定依据）。
export function serializePlanPayload(plan, createdAt) {
  const items = Array.isArray(plan && plan.items) ? plan.items : [];
  const payload = {
    version: 1,
    kind: "lytvpk-naming-normalization-plan",
    createdAt: String(createdAt || ""),
    items: items
      .filter((item) => item.actionable && item.newName)
      .map((item) => ({
        path: item.path,
        oldName: item.oldName,
        newName: item.newName,
        stage: item.stage,
        pendingReview: Boolean(item.pendingReview),
      })),
  };
  return JSON.stringify(payload, null, 2);
}

// serializeUndoPayload 生成「复制撤销清单」的 JSON 文本：
// 对每个成功项记录 新路径 → 原路径，撤销时把 newPath 改回 oldPath。
export function serializeUndoPayload(result, createdAt) {
  const items = Array.isArray(result && result.items) ? result.items : [];
  const payload = {
    version: 1,
    kind: "lytvpk-naming-rename-undo",
    createdAt: String(createdAt || ""),
    backupPath: String((result && result.backupPath) || ""),
    items: items
      .filter((item) => item.ok && item.newPath)
      .map((item) => ({
        oldPath: item.path,
        newPath: item.newPath,
      })),
  };
  return JSON.stringify(payload, null, 2);
}
