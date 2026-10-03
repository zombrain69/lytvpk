// 加载顺序预览的"窗口"换算（纯函数，方便单测）。
//
// 背景（真机 2607 行、2560×1440）：整表渲染 + content-visibility 时滚动
// p99 60ms、23 帧 >50ms、12 个 50–70ms 长任务；去掉 content-visibility 滚动变顺
// （203 FPS / 0 长任务）但开窗/填充涨到 25 个长任务（50–249ms）。
// 结论：只保留"视口 ± overscan"的行、滚出即回收，两条路都顺。

export const LOAD_ORDER_PREVIEW_OVERSCAN_ROWS = 6;
export const LOAD_ORDER_PREVIEW_FALLBACK_ROW_PITCH = 40;

function toPositiveInt(value, fallback) {
  const number = Number(value);
  if (!Number.isFinite(number) || number <= 0) return fallback;
  return Math.floor(number);
}

/**
 * computePreviewWindow 计算当前要物化的行区间 [start, end)。
 * 视口高度量不到时用 240px 兜底，避免只物化一行。
 */
export function computePreviewWindow({
  total,
  rowPitch,
  scrollTop,
  clientHeight,
  overscanRows = LOAD_ORDER_PREVIEW_OVERSCAN_ROWS,
} = {}) {
  const count = Math.max(0, Math.floor(Number(total) || 0));
  if (count === 0) return { start: 0, end: 0 };
  const pitch = toPositiveInt(rowPitch, LOAD_ORDER_PREVIEW_FALLBACK_ROW_PITCH);
  const overscan = Math.max(0, Math.floor(Number(overscanRows) || 0));
  const viewport = Math.max(Math.floor(Number(clientHeight) || 0), 240);
  const firstVisible = Math.max(0, Math.floor((Number(scrollTop) || 0) / pitch));
  const visibleRows = Math.max(1, Math.ceil(viewport / pitch) + 1);
  // 滚过头（比如列表变短 / scrollTop 还没被浏览器夹回）时也要落在最后一行上，
  // 否则 start 会大于 total，窗口里一行都建不出来。
  const start = Math.min(count - 1, Math.max(0, firstVisible - overscan));
  const end = Math.min(count, firstVisible + visibleRows + overscan);
  return { start, end: Math.max(start + 1, end) };
}

/** previewSpacerHeights 上下占位块高度：两者之和 + 窗口行数*行高 == 真实总高度。 */
export function previewSpacerHeights({ total, start, end, rowPitch } = {}) {
  const count = Math.max(0, Math.floor(Number(total) || 0));
  const pitch = toPositiveInt(rowPitch, LOAD_ORDER_PREVIEW_FALLBACK_ROW_PITCH);
  const safeStart = Math.max(0, Math.min(Math.floor(Number(start) || 0), count));
  const safeEnd = Math.max(safeStart, Math.min(Math.floor(Number(end) || 0), count));
  return {
    top: safeStart * pitch,
    bottom: Math.max(0, count - safeEnd) * pitch,
  };
}
