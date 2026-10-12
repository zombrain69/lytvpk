// 界面语言（locale）的纯逻辑层（node --test 覆盖，不碰 DOM）。
//
// 背景与拆包见 `docs/development/i18n-language-switch-eval.md`：
//   - P0（本批）：多目录装载 + `locale` 持久化 + 启动时应用；切换入口在全部就绪前保持隐藏
//     （`LANGUAGE_SWITCH_ENABLED = false`）——P1（动态界面重绘）/ P2（en 全量产出）/ P3 与
//     en 审校迭代（2026-10-13）完成后已翻 true，用户可见设置行。
//   - locale id 与 `frontend/src/messages/*.json` 文件名一致；语言名称用各自语言书写
//     （endonym，如「简体中文 / English」）——这是界面语言选择的通行做法，不进翻译目录。
//   - `normalizeLocale` 与后端 `normalizeLocaleKey`（internal/app/config.go）保持同一套白名单：
//     未知值一律回落 zh-CN，别让手改配置把界面变成未知状态。

export const DEFAULT_LOCALE_ID = "zh-CN";

/** 随包语言目录（id 与 messages/*.json 对应）。 */
export const SUPPORTED_LOCALES = [
  { id: "zh-CN", label: "简体中文" },
  { id: "en", label: "English" },
];

/** LANGUAGE_SWITCH_ENABLED 控制设置页「界面语言」入口的可见性。
 *  P0–P3 与 en 审校迭代（2026-10-13）完成后第 19 项收口：已翻 true（入口开放；回退即改回 false）。 */
export const LANGUAGE_SWITCH_ENABLED = true;

/** normalizeLocale 把任意输入归一化成受支持的 locale id（未知值回落 zh-CN）。 */
export function normalizeLocale(value) {
  const key = String(value ?? "").trim();
  if (!key) return DEFAULT_LOCALE_ID;
  const lower = key.toLowerCase().replace(/_/g, "-");
  for (const locale of SUPPORTED_LOCALES) {
    if (locale.id.toLowerCase() === lower) return locale.id;
  }
  // 宽松别名：zh / zh-cn / zh-tw → 简体中文（当前只随包 zh-CN 一份中文目录）；
  // en / en-us → English（将来按区域细分再加 en-US 目录）。
  if (lower === "zh" || lower.startsWith("zh-")) return "zh-CN";
  if (lower === "en" || lower.startsWith("en-")) return "en";
  return DEFAULT_LOCALE_ID;
}

/** localeLabel 返回语言的自称（设置页展示用；未知 id 归一化后兜底）。 */
export function localeLabel(id) {
  const key = normalizeLocale(id);
  return SUPPORTED_LOCALES.find((locale) => locale.id === key)?.label || key;
}
