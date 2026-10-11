// i18n 运行时：面向页面 / DOM 的入口（t / setLocale / applyTranslations）。
//
// 目录装配与运行时分离（`installCatalog`）：
//   - `messages/*.json` 的导入放在 `core/i18n-setup.js`（应用启动最先接线）；
//   - 本文件自己不 import JSON，因此可以被 Node 直接导入（`node --test` 里的纯模块
//     单测需要它）。JSON 模块的 import 属性语法（`with { type: "json" }`）Node 认、
//     Rollup 2（Vite 3）不认，两边不可兼得，所以拆成两个文件。
//   - 测试里要用真实中文目录时走 `core/i18n-test-catalog.mjs`。
//
// 迁移约定（逐页进行，未迁移的页面保持原有中文字面量即可）：
//   1. 在 `messages/zh-CN.json` 里按 `<页面>.<区域>.<名称>` 增加 key；
//   2. 页面里用 `t("...")` 替换中文字面量；
//   3. 需要动态切换语言的纯文本节点可以直接加 `data-i18n="key"`，
//      由 `applyTranslations()` 统一回填。

import { createI18n, DEFAULT_LOCALE } from "./i18n.mjs";

let i18n = createI18n({ catalogs: {}, locale: DEFAULT_LOCALE });

/** installCatalog 把一份文案目录装到运行时（应用启动时调用；测试也用它）。 */
export function installCatalog(messages, locale = DEFAULT_LOCALE) {
  if (!messages || typeof messages !== "object") return false;
  i18n = createI18n({ catalogs: { [String(locale)]: messages }, locale: String(locale) });
  if (typeof document !== "undefined") {
    document.documentElement?.setAttribute("lang", String(locale));
  }
  return true;
}

export function t(key, params) {
  return i18n.t(key, params);
}

export function getLocale() {
  return i18n.getLocale();
}

export function availableLocales() {
  return i18n.availableLocales();
}

export function setLocale(locale) {
  const next = i18n.setLocale(locale);
  if (typeof document !== "undefined") {
    document.documentElement?.setAttribute("lang", next);
    // 语言切换后立即把全部静态节点按新语言回填 —— 与启动时回填走同一条
    // `applyTranslations()` 路径，页面/弹窗都不需要各自处理。
    applyTranslations();
  }
  return next;
}

// 属性回填绑定：data-i18n-* 属性名 → 要写入的目标属性。
// 纯文本用 `data-i18n`（写入 textContent），其余按属性对应表处理。
const ATTRIBUTE_BINDINGS = [
  ["data-i18n-title", "title"],
  ["data-i18n-placeholder", "placeholder"],
  ["data-i18n-aria-label", "aria-label"],
  ["data-i18n-alt", "alt"],
];

/** applyTranslations 回填带 data-i18n / data-i18n-title / data-i18n-placeholder /
 *  data-i18n-aria-label / data-i18n-alt 的节点（全量、幂等；启动与语言切换共用）。 */
export function applyTranslations(root = typeof document !== "undefined" ? document : null) {
  if (!root?.querySelectorAll) return 0;
  let applied = 0;
  root.querySelectorAll("[data-i18n]").forEach((element) => {
    const key = element.getAttribute("data-i18n");
    if (!key) return;
    element.textContent = t(key);
    applied++;
  });
  for (const [sourceAttr, targetAttr] of ATTRIBUTE_BINDINGS) {
    root.querySelectorAll(`[${sourceAttr}]`).forEach((element) => {
      const key = element.getAttribute(sourceAttr);
      if (!key) return;
      element.setAttribute(targetAttr, t(key));
      applied++;
    });
  }
  return applied;
}

export { i18n };
