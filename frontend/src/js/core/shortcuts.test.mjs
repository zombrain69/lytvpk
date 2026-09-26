import assert from "node:assert/strict";
import test from "node:test";

import {
  SHORTCUT_GROUPS,
  allShortcutRows,
  buildShortcutsHtml,
  buildShortcutsTitle,
  shortcutRowsByIds,
} from "./shortcuts.mjs";

test("快捷键表分组覆盖应用里真实绑定的键位", () => {
  assert.ok(SHORTCUT_GROUPS.length >= 3, "至少要有全局 / 搜索 / 窗口三组");
  const keys = allShortcutRows().map((row) => row.keys);
  // 这些快捷键在代码里真的有绑定（app-runtime.js / ui-scale.js / floating-modal.js）
  ["Ctrl + K", "Ctrl + F", "Ctrl + =", "Ctrl + -", "Ctrl + 0", "?", "↑ / ↓", "Enter", "Esc"].forEach((key) => {
    assert.ok(keys.includes(key), `快捷键表应包含 ${key}`);
  });

  const ids = allShortcutRows().map((row) => row.id);
  assert.equal(new Set(ids).size, ids.length, "id 不能重复");
  assert.ok(
    allShortcutRows().every((row) => row.description.trim().length > 0),
    "每行都要有说明",
  );
});

test("shortcutRowsByIds 按给定顺序取行，忽略不存在的 id", () => {
  assert.deepEqual(
    shortcutRowsByIds(["focus-search", "search-open"]).map((row) => row.id),
    ["focus-search", "search-open"],
  );
  assert.deepEqual(shortcutRowsByIds(["不存在", "search-clear"]).map((row) => row.id), ["search-clear"]);
  assert.deepEqual(shortcutRowsByIds(null), []);
});

test("浮层 HTML 分组渲染且全部转义", () => {
  const html = buildShortcutsHtml();
  SHORTCUT_GROUPS.forEach((group) => assert.ok(html.includes(group.title), `缺少分组标题 ${group.title}`));
  assert.ok(html.includes("Ctrl + K"), "要列出命令面板");
  assert.ok(html.includes("双击标题栏"), "要列出窗口复位");
  assert.match(html, /class="search-help-row"/, "复用搜索说明书的行样式");

  // 特殊字符必须转义（用假数据验证渲染层本身）
  const escaped = buildShortcutsHtml([{ id: "x", title: "<脚本>", rows: [{ id: "y", keys: "<b>", description: "&\"" }] }]);
  assert.ok(!escaped.includes("<脚本>"), "标题要转义");
  assert.ok(!escaped.includes("<b>"), "键位要转义");
  assert.ok(escaped.includes("&amp;"), "描述要转义");
});

test("悬停提示是一行版，且与浮层同源", () => {
  const title = buildShortcutsTitle();
  assert.match(title, /快捷键总览：/);
  assert.match(title, /Ctrl \+ K/);
  assert.match(title, /Ctrl \+ F/);
  assert.match(title, /\?/);
  assert.ok(title.split("\n").length === 1, "悬停提示应当只有一行");
});
