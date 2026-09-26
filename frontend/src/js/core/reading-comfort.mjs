// 阅读舒适度：文字大小档位 + 对比度/行距档位。
//
// 为什么和「界面缩放」分开：
//   - 界面缩放（uiScale）是整体缩放，间距、图标、控件一起变大，用来适配屏幕尺寸；
//   - 文字大小（textSize）只改文字，不改布局，用来解决"字太小看不清"；
//   - 阅读舒适度（readingComfort）改的是行距与文字对比度，用来解决"看久了累"。
// 三者相乘作用于根字号（uiScale × textSize），所以互不干扰、可以任意组合。

export const TEXT_SIZE_DATASET_KEY = "textSize";
export const READING_FONT_SCALE_DATASET_KEY = "readingFontScale";

export const TEXT_SIZE_OPTIONS = [
  { key: "compact", label: "紧凑", scale: 0.94, description: "同屏放更多内容" },
  { key: "standard", label: "标准", scale: 1, description: "默认大小" },
  { key: "large", label: "大", scale: 1.12, description: "文字更清楚" },
  { key: "xlarge", label: "特大", scale: 1.25, description: "最舒服但同屏更少" },
];

export const READING_COMFORT_OPTIONS = [
  {
    key: "soft",
    label: "柔和",
    lineHeight: 1.8,
    description: "行距更松、对比更柔，长时间阅读更省眼",
  },
  {
    key: "standard",
    label: "标准",
    lineHeight: 1.65,
    description: "默认的对比度与行距",
  },
  {
    key: "contrast",
    label: "高对比",
    lineHeight: 1.55,
    description: "文字更黑、行距更紧，弱视或强光下更清楚",
  },
];

export function normalizeTextSize(value) {
	if (typeof value === "string") {
		const key = value.trim().toLowerCase();
		if (TEXT_SIZE_OPTIONS.some((option) => option.key === key)) return key;
	}
	return "standard";
}

export function normalizeReadingComfort(value) {
	if (typeof value === "string") {
		const key = value.trim().toLowerCase();
		if (READING_COMFORT_OPTIONS.some((option) => option.key === key)) return key;
	}
	return "standard";
}

/** textScaleFor 返回某个字号档位的倍率。 */
export function textScaleFor(textSize) {
	const key = normalizeTextSize(textSize);
	const option = TEXT_SIZE_OPTIONS.find((item) => item.key === key);
	return option ? option.scale : 1;
}

/** lineHeightFor 返回某个舒适度档位的行高。 */
export function lineHeightFor(comfort) {
	const key = normalizeReadingComfort(comfort);
	const option = READING_COMFORT_OPTIONS.find((item) => item.key === key);
	return option ? option.lineHeight : 1.65;
}

/** textSizeLabel / readingComfortLabel 供设置页与提示文案使用。 */
export function textSizeLabel(textSize) {
	const key = normalizeTextSize(textSize);
	return TEXT_SIZE_OPTIONS.find((item) => item.key === key)?.label || "标准";
}

export function readingComfortLabel(comfort) {
	const key = normalizeReadingComfort(comfort);
	return READING_COMFORT_OPTIONS.find((item) => item.key === key)?.label || "标准";
}

/**
 * fontPercentFor 计算根字号百分比：界面缩放 × 文字大小档位。
 * uiScale 已经由调用方归一化（见 core/ui-scale.js）。
 */
export function fontPercentFor(uiScale, textSize) {
	const scale = Number(uiScale);
	const safeScale = Number.isFinite(scale) && scale > 0 ? scale : 1;
	return Math.round(safeScale * textScaleFor(textSize) * 100);
}

/** readingFontScaleFromRoot 读出当前挂在根节点上的文字倍率（ui-scale 合成时要用）。 */
export function readingFontScaleFromRoot(root) {
	const dataset = root?.dataset || {};
	const raw = Number(dataset[READING_FONT_SCALE_DATASET_KEY]);
	return Number.isFinite(raw) && raw > 0 ? raw : 1;
}

/**
 * applyReadingComfort 把两个档位写到根节点上：
 *   - data-text-size / data-reading-comfort：给 CSS 选颜色用（[data-reading-comfort="soft"] …）；
 *   - --reading-line-height：正文行高；
 *   - style.fontSize：界面缩放 × 文字倍率（所以两个功能不会互相覆盖）。
 * 返回归一化后的档位，便于调用方写回配置。
 */
export function applyReadingComfort({ textSize, comfort, uiScale } = {}, root) {
	const target = root || (typeof document !== "undefined" ? document.documentElement : null);
	const normalizedTextSize = normalizeTextSize(textSize);
	const normalizedComfort = normalizeReadingComfort(comfort);
	if (!target) {
		return { textSize: normalizedTextSize, comfort: normalizedComfort };
	}

	const scale = textScaleFor(normalizedTextSize);
	target.dataset[TEXT_SIZE_DATASET_KEY] = normalizedTextSize;
	target.dataset[READING_FONT_SCALE_DATASET_KEY] = String(scale);
	target.dataset.readingComfort = normalizedComfort;
	target.style.setProperty("--reading-line-height", String(lineHeightFor(normalizedComfort)));
	if (target.style) {
		target.style.fontSize = `${fontPercentFor(uiScale, normalizedTextSize)}%`;
	}
	return { textSize: normalizedTextSize, comfort: normalizedComfort };
}
