import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  ARCHIVE_SEARCH_HELP_VARIANT,
  SEARCH_FIELD_LABELS,
  SEARCH_SHORTCUT_ROWS,
  SEARCH_SYNTAX_ROWS,
  buildSearchHelpHtml,
  buildSearchHelpTitle,
} from "./search-help.mjs";

test("归档面板复用同一份说明书，但字段与 tag: 含义不同", () => {
  const html = buildSearchHelpHtml(ARCHIVE_SEARCH_HELP_VARIANT);
  assert.match(html, /压缩包名/);
  assert.match(html, /包状态/);
  assert.match(html, /tag:密码/);
  assert.match(html, /tag:密码\|错误/);
  assert.ok(!html.includes("发音角色"), "归档面板不该出现 Mod 列表的字段");
  // 语法与快捷键段落仍与 Mod 列表同源
  assert.match(html, /正则表达式无效/);
  assert.match(html, /Ctrl \+ F/);
  assert.ok(SEARCH_SHORTCUT_ROWS.every((row) => html.includes(row.syntax.replace("+", "+"))), "快捷键来自同一份数据");

  const title = buildSearchHelpTitle(ARCHIVE_SEARCH_HELP_VARIANT);
  assert.match(title, /匹配：压缩包名、路径/);
  assert.match(title, /tag:密码/);
  assert.ok(title.split("\n").length <= 4, "悬停提示不应过长");
});

test("语法表覆盖实际支持的全部写法", () => {
  const syntaxes = SEARCH_SYNTAX_ROWS.map((row) => row.syntax).join(" ");
  ["tag:", "|", "-tag:", "re:", '"'].forEach((token) => {
    assert.ok(syntaxes.includes(token), `语法表应包含 ${token} 的示例`);
  });
  // 每一行都要有说明，不能只有写法。
  assert.ok(SEARCH_SYNTAX_ROWS.every((row) => row.description.trim().length > 0), "语法行要有说明");
  assert.ok(SEARCH_SHORTCUT_ROWS.length >= 4, "快捷键至少列出 Ctrl+F / ↑↓ / Enter / Esc");
});

test("浮层 HTML 全部转义，并把范围与快捷键都写清", () => {
  const html = buildSearchHelpHtml();
  assert.match(html, /匹配范围/);
  assert.match(html, /快捷键/);
  assert.match(html, /Ctrl \+ F/);
  assert.match(html, /在结果里移动光标/);
  assert.match(html, /正则表达式无效/, "要说明写错正则会发生什么");
  // 反斜杠与引号这类字符必须被转义进 code 里，而不是当成 HTML。
  assert.match(html, /re:\^ak\\d\+\$/, "正则示例要原样显示");
  assert.ok(!html.includes('"ak 47"'), "双引号示例应被转义");
  assert.ok(SEARCH_FIELD_LABELS.every((label) => html.includes(label)), "匹配范围要逐项列出");
});

test("悬停提示是一行版，且与浮层同源", () => {
  const title = buildSearchHelpTitle();
  assert.match(title, /匹配：标题、文件名/);
  assert.match(title, /语法：/);
  assert.match(title, /快捷键：Ctrl\+F 聚焦/);
  assert.ok(title.split("\n").length <= 4, "悬停提示不应过长");
});

test("搜索框旁接了 `?` 按钮与浮层，并支持点击外部/Esc 关闭", () => {
  const html = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
  const runtime = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
  const css = readFileSync(new URL("../../../css/app/reading-comfort.css", import.meta.url), "utf8");

  assert.match(html, /id="search-help-btn"/, "搜索框旁要有 ? 按钮");
  assert.match(html, /id="search-help-popover"/, "要有浮层容器");
  assert.match(runtime, /buildSearchHelpTitle\(\)/, "悬停提示应由同源数据生成");
  assert.match(runtime, /searchHelpPopover\.innerHTML = buildSearchHelpHtml\(\)/, "浮层内容由同源数据生成");
  assert.match(runtime, /aria-expanded/, "按钮要有 aria-expanded 状态");
  assert.match(runtime, /if \(searchHelpPopover\.contains\(event\.target\)/, "点击外部要关闭浮层");
  assert.match(css, /\.search-help-popover/, "缺少浮层样式");
  assert.match(css, /\.search-help-row code/, "缺少语法行的等宽样式");
});
