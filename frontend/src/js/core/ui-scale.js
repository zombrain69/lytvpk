export const DEFAULT_UI_SCALE = 1;
export const MIN_UI_SCALE = 0.8;
export const MAX_UI_SCALE = 1.4;
export const UI_SCALE_STEP = 0.05;

export function normalizeUIScale(value) {
  const numericValue = Number(value);
  if (!Number.isFinite(numericValue) || numericValue < MIN_UI_SCALE || numericValue > MAX_UI_SCALE) {
    return DEFAULT_UI_SCALE;
  }
  return Math.round(numericValue * 100) / 100;
}

export function applyUIScale(value) {
  const scale = normalizeUIScale(value);
  if (typeof document === "undefined") return scale;

  const percent = String(Math.round(scale * 100));
  document.documentElement.style.fontSize = `${percent}%`;
  document.querySelectorAll("[data-ui-scale-input]").forEach((input) => {
    input.value = percent;
  });
  document.querySelectorAll("[data-ui-scale-value]").forEach((output) => {
    output.textContent = `${percent}%`;
  });
  return scale;
}

export function setupUIScaleShortcuts({ getConfig, saveConfig }) {
  if (typeof document === "undefined" || document.documentElement.dataset.uiScaleShortcutsBound === "true") return;
  document.documentElement.dataset.uiScaleShortcutsBound = "true";

  document.addEventListener("keydown", (event) => {
    if ((!event.ctrlKey && !event.metaKey) || event.altKey) return;

    const key = event.key;
    let direction = 0;
    if (key === "+" || key === "=") direction = 1;
    if (key === "-") direction = -1;
    const shouldReset = key === "0";
    if (direction === 0 && !shouldReset) return;

    event.preventDefault();
    const config = getConfig();
    const current = normalizeUIScale(config.uiScale);
    const next = shouldReset
      ? DEFAULT_UI_SCALE
      : normalizeUIScale(Math.min(MAX_UI_SCALE, Math.max(MIN_UI_SCALE, current + direction * UI_SCALE_STEP)));
    if (next === current) return;

    config.uiScale = applyUIScale(next);
    saveConfig(config);
  });
}
