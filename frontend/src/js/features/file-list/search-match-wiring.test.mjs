import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 「明确的匹配」交互：命中高亮 + 命中字段 chip + 命中计数 + 快捷键。
// 纯函数语义由 search-match.test.mjs 覆盖，这里只锁"接线是否真的接上了"。

const renderSource = readFileSync(new URL("./render.js", import.meta.url), "utf8");
const runtimeSource = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const indexHtml = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");

test("列表与卡片都把标题/文件名做了命中高亮", () => {
  assert.match(renderSource, /highlightMatches\(displayTitle, searchHighlight\)/, "列表标题要高亮");
  assert.match(renderSource, /highlightMatches\(file\.name, searchHighlight\)/, "文件名要高亮");
  assert.match(renderSource, /terms: positiveTerms\(cardSearchSyntax\)/, "卡片标题要高亮");
  // re: 写法也要高亮（只有词高亮会让人以为正则没生效）。
  assert.match(renderSource, /regex: compiledRegex\(searchSyntax\)/, "正则命中要参与高亮");
  assert.match(renderSource, /regex: compiledRegex\(cardSearchSyntax\)/, "卡片同样处理正则");
  // 高亮的词来自搜索语法解析（tag: / re: 之类的条件不该被当成要高亮的文字）。
  assert.match(renderSource, /parseSearchSyntax\(appState\.searchQuery\)/, "要用语法解析搜索框内容");
  assert.match(renderSource, /positiveTerms\(searchSyntax\)/, "只高亮正向普通词");
  // 高亮结果直接进 innerHTML，所以属性位置必须单独转义，避免标题里的引号破坏结构。
  assert.match(renderSource, /title="\$\{escapeHtml\(displayTitle\)\}"/, "卡片 title 属性要转义");
  assert.match(renderSource, /alt="\$\{escapeHtml\(displayTitle\)\}"/, "卡片 alt 属性要转义");
});

test("命中字段以 chip 形式贴在标题旁边", () => {
  assert.match(renderSource, /describeMatchReasons\(file, searchSyntax\.raw\)/, "要计算命中字段");
  assert.match(renderSource, /describeMatchReasons\(file, cardSearchSyntax\.raw\)/, "卡片同样要算");
  assert.match(renderSource, /formatMatchReasonChip\(/, "要格式化成文案");
  assert.match(renderSource, /class="search-reason-chip"/, "要有 chip 元素");
  assert.match(renderSource, /这行是被这些字段匹配到的/, "chip 要有 tooltip 说明");
  // chip 不能塞进 title 这种带省略号的元素里，否则会被裁掉（真机截图时发现）。
  assert.ok(
    !/class="file-title">[^<]*\$\{matchReasonChip\}/.test(renderSource),
    "file-title 会省略号截断，chip 不能放进去",
  );
  assert.match(renderSource, /<div class="file-tags">\s*\n\s*\$\{matchReasonChip\}/, "chip 要放进可换行的标签区");
});

test("搜索框旁边显示命中数量", () => {
  assert.match(indexHtml, /id="search-hit-count"/, "index.html 要有命中计数元素");
  assert.match(renderSource, /getElementById\("search-hit-count"\)/, "渲染时要更新计数");
  assert.match(renderSource, /describeSearchResult\(\{/, "计数文案走纯函数");
  // 正则写错时要把原因显示出来，而不是只给一个空列表。
  assert.match(renderSource, /regexInvalid: parseSearchSyntax\(appState\.searchQuery\)\.regexInvalid/, "要显示正则错误");
  // 搜索框自身要提示支持哪些字段与快捷键。
  assert.match(indexHtml, /placeholder="[^"]*Ctrl\+F 聚焦[^"]*"/, "placeholder 要提示 Ctrl+F 并正确闭合");
  assert.match(indexHtml, /placeholder="[^"]*Esc 清空[^"]*"/, "placeholder 要提示 Esc");
  assert.match(indexHtml, /placeholder="[^"]*↑↓ 选结果，Enter 打开详情[^"]*"/, "placeholder 要提示键盘导航");
  assert.match(indexHtml, /placeholder="[^"]*tag:武器[^"]*"/, "placeholder 要提示搜索语法");
  // 悬停提示与 `?` 浮层现在都由 search-help.mjs 生成（单一事实来源），
  // index.html 只保留按钮与浮层容器；内容断言在 search-help.test.mjs 里。
  assert.match(indexHtml, /id="search-help-btn"/, "搜索框旁要有 ? 帮助按钮");
  assert.match(indexHtml, /id="search-help-popover"/, "要有帮助浮层容器");
});

test("Ctrl+F 聚焦搜索、Esc 清空", () => {
  assert.match(runtimeSource, /event\.key\.toLowerCase\(\) === "f"/, "缺少 Ctrl+F 处理");
  assert.match(runtimeSource, /searchInput\.focus\(\)/, "Ctrl+F 要聚焦搜索框");
  assert.match(runtimeSource, /event\.key === "Escape" && document\.activeElement === searchInput/, "Esc 只在搜索框聚焦且非空时清空");
});
