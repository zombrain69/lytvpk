// i18n 运行时：把 messages/*.json 挂到 createI18n 上，供各页面直接 import。
//
// 迁移约定（逐页进行，未迁移的页面保持原有中文字面量即可）：
//   1. 在 `messages/zh-CN.json` 里按 `<页面>.<区域>.<名称>` 增加 key；
//   2. 页面里用 `t("...")` 替换中文字面量；
//   3. 需要动态切换语言的纯文本节点可以直接加 `data-i18n="key"`，
//      由 `applyTranslations()` 统一回填。

import zhCN from "../../messages/zh-CN.json";
import { createI18n, DEFAULT_LOCALE } from "./i18n.mjs";

const catalogs = {
  "zh-CN": zhCN,
};

const i18n = createI18n({ catalogs, locale: DEFAULT_LOCALE });

export const t = i18n.t;
export const getLocale = i18n.getLocale;
export const availableLocales = i18n.availableLocales;

export function setLocale(locale) {
  const next = i18n.setLocale(locale);
  if (typeof document !== "undefined") {
    document.documentElement?.setAttribute("lang", next);
  }
  return next;
}

/** applyTranslations 回填带 data-i18n / data-i18n-title / data-i18n-placeholder 的节点。 */
export function applyTranslations(root = document) {
  if (!root?.querySelectorAll) return 0;
  let applied = 0;
  root.querySelectorAll("[data-i18n]").forEach((element) => {
    const key = element.getAttribute("data-i18n");
    if (!key) return;
    element.textContent = t(key);
    applied++;
  });
  root.querySelectorAll("[data-i18n-title]").forEach((element) => {
    const key = element.getAttribute("data-i18n-title");
    if (!key) return;
    element.setAttribute("title", t(key));
    applied++;
  });
  root.querySelectorAll("[data-i18n-placeholder]").forEach((element) => {
    const key = element.getAttribute("data-i18n-placeholder");
    if (!key) return;
    element.setAttribute("placeholder", t(key));
    applied++;
  });
  return applied;
}

export { i18n };
