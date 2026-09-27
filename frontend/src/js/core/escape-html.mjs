// DOM 无关的 HTML 转义：纯函数模块，可以在 node --test 里直接跑。
// core/utils.js 的 escapeHtml 也委托到这里，保证浏览器与测试用的是同一套规则。

const ESCAPE_MAP = {
  "&": "&amp;",
  "<": "&lt;",
  ">": "&gt;",
  '"': "&quot;",
  "'": "&#39;",
};

/** escapeHtmlText 把文本里的 HTML 元字符转成实体。 */
export function escapeHtmlText(text) {
  return String(text ?? "").replace(/[&<>"']/g, (char) => ESCAPE_MAP[char]);
}
