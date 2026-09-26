// 窗口几何的纯函数层（node --test 覆盖）。
//
// 为什么单独一个 .mjs：floating-modal.js 是 DOM 模块，直接 import 会在 node 里
// 触发 ESM 解析告警；把这些"只算数"的逻辑拆出来，既能测又不吵。

/**
 * clampWindowSize 把窗口尺寸钳制在可视区内。
 *
 * 为什么需要：窗口尺寸是按窗口记住的（localStorage）。用户在大屏上把管理窗口拉得很大，
 * 之后换到更小的屏幕或缩小主窗口，恢复出来的尺寸就会超出可视区 —— 标题栏被推到屏幕外、
 * 按钮点不到，只能去清 localStorage 才能救回来。
 */
export function clampWindowSize(width, height, viewportWidth, viewportHeight) {
  const safeWidth = Number.isFinite(width) && width > 0 ? width : 0;
  const safeHeight = Number.isFinite(height) && height > 0 ? height : 0;
  const maxWidth = Math.max(240, Math.round(Number(viewportWidth) * 0.96));
  const maxHeight = Math.max(200, Math.round(Number(viewportHeight) * 0.94));
  return {
    width: Math.round(Math.min(safeWidth || maxWidth, maxWidth)),
    height: Math.round(Math.min(safeHeight || maxHeight, maxHeight)),
  };
}
