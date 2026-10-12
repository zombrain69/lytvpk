// 列表 / 句读的 locale 感知分隔符（路线图第 21 项）。
//
// 背景：第 19 项审校全量复核发现，多处界面文本在代码层硬编码了中文标点（`join("、")`、
// `` `：${detail}` ``、`+ "。"`）——en 界面会渲染成「A、B」「失败：原因」这类混排。
// 这里收敛为一组随 getLocale() 实时取值的分隔符函数：
//
//   listSeparator()     zh「、」   en「,  」
//   clauseSeparator()   zh「；」   en「;  」
//   commaSeparator()    zh「，」   en「,  」
//   labelSeparator()    zh「：」   en「:  」
//   sentenceEnd()       zh「。」   en「.」
//   formatParenthetical zh「（x）」 en「(x)」
//
// 约定：
//   - 调用点用 `items.join(listSeparator())` / `` `${label}${labelSeparator()}${detail}` `` 组装；
//     zh 行为与替换前逐字节一致，en 走半角 + 空格；
//   - 取值发生在**渲染时**：语言切换后凡是走重新渲染路径的文本都会用新语言；
//     长期驻留的节点沿用第 19 项既有的重绘机制（app:locale-change 等）；
//   - 本模块无 DOM 依赖（i18n-runtime 的 getLocale 在 Node 单测里同样可用），
//     纯 .mjs 模块（tests 里默认 locale = zh-CN，既有中文断言不受影响）。
import { getLocale } from "./i18n-runtime.js";

function isEnglish() {
  return getLocale() === "en";
}

export function listSeparator() {
  return isEnglish() ? ", " : "、";
}

export function clauseSeparator() {
  return isEnglish() ? "; " : "；";
}

export function commaSeparator() {
  return isEnglish() ? ", " : "，";
}

export function labelSeparator() {
  return isEnglish() ? ": " : "：";
}

export function sentenceEnd() {
  return isEnglish() ? "." : "。";
}

export function formatParenthetical(text) {
  return isEnglish() ? `(${text})` : `（${text}）`;
}
