// 主窗口几何记忆（对齐 FireAxe v0.7.3："the width, height and maximized state of the
// MainWindow will be saved into the AppSettings"）。
//
// 纯逻辑放这里（node --test 覆盖），DOM / Wails 运行时只做接线：
//   - sanitizeMainWindowGeometry：把存档里的尺寸钳制到"当前屏幕能放下"的范围，
//     避免换了小屏之后窗口比屏幕还大、按钮够不着；
//   - shouldApplyMainWindowGeometry：没记录过（null）时不设置，保持 Wails 默认；
//   - 尺寸非法（0 / 负数 / 非数字）一律当"没记录过"。

/** 主窗口允许的最小尺寸：比这个还小就会把工具栏挤掉。 */
export const MAIN_WINDOW_MIN_WIDTH = 900;
export const MAIN_WINDOW_MIN_HEIGHT = 600;

/** 主窗口允许的最大尺寸上限（防御手改配置写出离谱的值）。 */
export const MAIN_WINDOW_MAX_WIDTH = 10000;
export const MAIN_WINDOW_MAX_HEIGHT = 10000;

const isPositiveNumber = (value) => Number.isFinite(value) && value > 0;

/**
 * sanitizeMainWindowGeometry 校验并钳制一组几何值。
 * 返回 { width, height, maximised } 或 null（表示"没有可用记录"）。
 * screen 可选：{ width, height } —— 传入时会把尺寸压到屏幕尺寸以内。
 */
export function sanitizeMainWindowGeometry(geometry, screen = null) {
  if (!geometry || typeof geometry !== "object") return null;
  const width = Number(geometry.width);
  const height = Number(geometry.height);
  if (!isPositiveNumber(width) || !isPositiveNumber(height)) return null;

  let maxWidth = MAIN_WINDOW_MAX_WIDTH;
  let maxHeight = MAIN_WINDOW_MAX_HEIGHT;
  if (screen && isPositiveNumber(Number(screen.width)) && isPositiveNumber(Number(screen.height))) {
    maxWidth = Math.min(maxWidth, Math.floor(Number(screen.width)));
    maxHeight = Math.min(maxHeight, Math.floor(Number(screen.height)));
  }

  // 屏幕比最小尺寸还小（例如很窄的分屏）时，以屏幕为准，不要反而撑出屏幕。
  const minWidth = Math.min(MAIN_WINDOW_MIN_WIDTH, maxWidth);
  const minHeight = Math.min(MAIN_WINDOW_MIN_HEIGHT, maxHeight);
  return {
    width: Math.min(maxWidth, Math.max(minWidth, Math.round(width))),
    height: Math.min(maxHeight, Math.max(minHeight, Math.round(height))),
    maximised: geometry.maximised === true,
  };
}

/** shouldApplyMainWindowGeometry 判断这份记录是否值得应用（null / 非法 → 不应用）。 */
export function shouldApplyMainWindowGeometry(geometry) {
  return sanitizeMainWindowGeometry(geometry) !== null;
}

/**
 * describeMainWindowGeometry 生成一行说明（设置页/日志用，便于确认记忆生效）。
 * maximised 为真时只写"最大化"。
 */
export function describeMainWindowGeometry(geometry) {
  const clean = sanitizeMainWindowGeometry(geometry);
  if (!clean) return "还没记录过主窗口大小（使用默认尺寸）";
  if (clean.maximised) return `主窗口记忆：最大化（还原尺寸 ${clean.width}×${clean.height}）`;
  return `主窗口记忆：${clean.width}×${clean.height}`;
}

/**
 * pickPrimaryScreen 从 Wails `ScreenGetAll()` 的结果里挑出主窗口所在/主屏幕。
 * 找不到时返回 null（调用方就不做屏幕钳制）。
 */
export function pickPrimaryScreen(screens) {
  if (!Array.isArray(screens) || screens.length === 0) return null;
  const primary = screens.find((screen) => screen?.isPrimary) || screens[0];
  if (!primary) return null;
  const width = Number(primary.width);
  const height = Number(primary.height);
  if (!isPositiveNumber(width) || !isPositiveNumber(height)) return null;
  return { width, height };
}
