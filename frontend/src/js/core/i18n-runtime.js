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

/**
 * installCatalogs 把**全部随包目录**一次性装到运行时（应用启动时调用；测试也用它）。
 * 传 `{ "zh-CN": zhMessages, en: enMessages }`；缺翻的 key 由运行时回退链兜底
 * （当前语言 → zh-CN → key 本身），所以第二语言的目录可以渐进补齐。
 */
export function installCatalogs(catalogs, locale = DEFAULT_LOCALE) {
  const normalized = {};
  if (catalogs && typeof catalogs === "object") {
    for (const [key, value] of Object.entries(catalogs)) {
      if (value && typeof value === "object") normalized[String(key)] = value;
    }
  }
  i18n = createI18n({ catalogs: normalized, locale: String(locale) });
  if (typeof document !== "undefined") {
    document.documentElement?.setAttribute("lang", String(locale));
  }
  return true;
}

/** installCatalog 单目录入口（兼容旧调用；等价于 installCatalogs 只装一份）。 */
export function installCatalog(messages, locale = DEFAULT_LOCALE) {
  if (!messages || typeof messages !== "object") return false;
  return installCatalogs({ [String(locale)]: messages }, String(locale));
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

export function setLocale(locale, options = {}) {
  const previous = i18n.getLocale();
  const next = i18n.setLocale(locale);
  if (typeof document !== "undefined") {
    document.documentElement?.setAttribute("lang", next);
    // 语言切换后立即把全部静态节点按新语言回填 —— 与启动时回填走同一条
    // `applyTranslations()` 路径，页面/弹窗都不需要各自处理。
    applyTranslations();
    // 动态界面重绘（路线图第 19 项 P1）：语言真正变化时广播 `app:locale-change`，
    // 各页面模块按自己的入口重画（mods 列表/标签条/目录下拉、工坊缓存卡片、设置页整页）。
    // 启动时应用持久化语言要传 `{ notify: false }`：此刻页面还没渲染，各页面随后
    // 按当前语言首次渲染，不需要广播。
    if (options.notify !== false && next !== previous) {
      document.dispatchEvent(
        new CustomEvent("app:locale-change", {
          detail: { locale: next, previous },
        })
      );
    }
  }
  return next;
}

/** hasLocale 判断某个语言的目录是否已装载（设置页据此决定下拉里有哪些选项可切）。 */
export function hasLocale(locale) {
  return i18n.hasLocale(locale);
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
