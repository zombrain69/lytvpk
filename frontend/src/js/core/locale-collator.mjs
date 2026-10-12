// 本地化排序（路线图第 19 项 P3）：排序比较器跟随当前界面语言。
//
// 背景：此前各模块把 zh-CN 硬编码进 localeCompare / Intl.Collator（≥10 处），
// 界面语言切到 en 后排序仍按中文 collation 走。现在统一经本模块创建比较器：
//   - 活动 locale 取自 i18n 运行时的当前界面语言，经 core/locale.mjs 白名单归一化
//     （未知值回落 zh-CN，与 config.json / localStorage 的取值纪律一致）；
//   - 底层 Intl.Collator 按 locale 缓存复用——只有语言真正变化时才重建，
//     不违反 priority-sort 里「复用 collator、避免每次比较新建」的性能约定
//     （真机 2298 条排序 ≈ 25k 次比较）。
import { getLocale } from "./i18n-runtime.js";
import { normalizeLocale } from "./locale.mjs";

/** activeSortLocale 返回排序应使用的 locale（界面语言归一化后的 id）。 */
export function activeSortLocale() {
  return normalizeLocale(getLocale());
}

/**
 * createLocaleCollator 创建「跟随界面语言」的比较器（用法同 Intl.Collator：
 * `collator.compare(a, b)`）。compare 每次调用只做一次 locale 检查（字符串比较），
 * 语言变化时自动按新 locale 重建底层 collator。
 */
export function createLocaleCollator(options = {}) {
  let currentLocale = "";
  let collator = null;

  function ensure() {
    const locale = activeSortLocale();
    if (!collator || locale !== currentLocale) {
      currentLocale = locale;
      collator = new Intl.Collator(locale, options);
    }
    return collator;
  }

  return {
    compare(left, right) {
      return ensure().compare(left, right);
    },
    /** resolvedLocale 返回底层 collator 实际解析到的 locale（测试 / 调试用）。 */
    resolvedLocale() {
      return ensure().resolvedOptions().locale;
    },
  };
}
