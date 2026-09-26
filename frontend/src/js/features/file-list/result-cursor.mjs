// 搜索结果的键盘光标（↑ / ↓ 移动、Enter 打开详情）。
//
// 为什么需要：搜索框本身很好用（Ctrl+F 聚焦、Esc 清空），但选结果还得把手从键盘挪到鼠标。
// 这里只做"光标落在哪一行"的纯计算，DOM 操作留给调用方，方便 node --test 覆盖。

/**
 * nextResultPath 在结果路径列表里移动光标。
 *
 * 约定：
 *   - 列表为空 ⇒ 返回空串；
 *   - 当前光标不在列表里（例如换了搜索词）⇒ 从列表头开始（向下）或从列表尾开始（向上）；
 *   - 越界环绕（↓ 到最后一行再 ↓ 回到第一行）。
 */
export function nextResultPath(paths, currentPath, delta) {
  const list = Array.isArray(paths) ? paths.filter((path) => String(path || "") !== "") : [];
  if (list.length === 0) return "";
  // 非数字 / NaN 一律按"向下"处理：方向参数写错时至少行为可预期，而不是随机反向。
  const rawDelta = Number(delta);
  const step = Number.isFinite(rawDelta) && rawDelta < 0 ? -1 : 1;
  const current = String(currentPath || "");
  const index = list.indexOf(current);
  if (index < 0) {
    return step > 0 ? list[0] : list[list.length - 1];
  }
  const next = (index + step + list.length) % list.length;
  return list[next];
}

/**
 * describeResultCursor 生成"第 3 / 12 个结果"这样的位置说明（给提示条与无障碍用）。
 * hint 由调用方给（不同面板 Enter 做的事不同：打开详情 / 展开压缩包 / 展开模型）。
 */
export function describeResultCursor(paths, currentPath, hint = "Enter 打开详情") {
  const list = Array.isArray(paths) ? paths : [];
  const index = list.indexOf(String(currentPath || ""));
  if (index < 0 || list.length === 0) return "";
  return `第 ${index + 1} / ${list.length} 个结果（${hint}）`;
}

/** collectResultPaths 从渲染出的行里收集路径（按显示顺序）。 */
export function collectResultPaths(container) {
  if (!container?.querySelectorAll) return [];
  return [...container.querySelectorAll(".file-item[data-path]")]
    .map((row) => String(row.dataset.path || ""))
    .filter(Boolean);
}

/**
 * collectCursorKeys 从任意容器里按 selector 收集"光标键"（data-cursor-key，按显示顺序）。
 * 归档管理器、模型统计等面板用它复用同一套 ↑↓ / Enter 导航，不必各写一遍。
 */
export function collectCursorKeys(container, selector) {
  if (!container?.querySelectorAll || !selector) return [];
  return [...container.querySelectorAll(selector)]
    .map((row) => String(row.dataset.cursorKey || ""))
    .filter(Boolean);
}

/**
 * syncCursorHighlight 把光标类同步到渲染出的行上（其余行清掉），并把当前行滚进可视区。
 * 返回真正生效的光标键（列表为空 / 键不在列表里时为空串）。
 */
export function syncCursorHighlight(container, selector, currentKey, activeClass = "is-cursor") {
  const rows = container?.querySelectorAll ? [...container.querySelectorAll(selector)] : [];
  let active = "";
  rows.forEach((row) => {
    const key = String(row.dataset.cursorKey || "");
    const isActive = key !== "" && key === String(currentKey || "");
    row.classList.toggle(activeClass, isActive);
    if (!isActive) return;
    active = key;
    row.scrollIntoView?.({ block: "nearest" });
  });
  return active;
}
