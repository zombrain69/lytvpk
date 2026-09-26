import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 命令面板的接线检查：浮层结构、快捷键、动作注册、样式。
// 检索与排序逻辑在 command-palette.test.mjs 里覆盖。

const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
const runtime = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");
const ui = readFileSync(new URL("./command-palette-ui.js", import.meta.url), "utf8");
const css = readFileSync(new URL("../../css/app/reading-comfort.css", import.meta.url), "utf8");
const help = readFileSync(new URL("../features/file-list/search-help.mjs", import.meta.url), "utf8");

test("命令面板结构在 index.html 里", () => {
  ["command-palette-modal", "command-palette-input", "command-palette-list", "command-palette-hint"].forEach((id) => {
    assert.match(html, new RegExp(`id="${id}"`), `缺少 #${id}`);
  });
  // 除了 Ctrl+K，标题栏还要有一个看得见的入口（否则功能是"藏起来的"）。
  assert.match(html, /id="command-palette-btn"/, "标题栏缺少命令面板按钮");
  assert.match(runtime, /getElementById\("command-palette-btn"\)[\s\S]{0,80}openCommandPalette\(\)/, "按钮要打开面板");
});

test("Ctrl+K 打开面板，键盘交互齐全", () => {
  assert.match(runtime, /event\.key\.toLowerCase\(\) === "k"/, "缺少 Ctrl+K");
  assert.match(runtime, /openCommandPalette\(\)/, "Ctrl+K 要真的打开面板");
  assert.match(ui, /ArrowDown/, "缺少 ↑↓ 选择");
  assert.match(ui, /event\.key === "Enter"/, "缺少 Enter 执行");
  assert.match(ui, /event\.key === "Escape"/, "缺少 Esc 关闭");
  assert.match(ui, /event\.target === modal/, "点面板外应关闭");
});

test("每条命令都注册了动作，且动作复用已有入口", () => {
  // 命令 id 必须与 command-palette.mjs 的表一致（这里抽 id 做交叉断言）。
  // 动作注册表在 app-runtime.js 的 setupCommandPaletteWithDeps 里（缩进 4 空格的 "id": ...）。
  const declared = [...runtime.matchAll(/^\s{4}"([a-z0-9-]+)":/gm)].map((match) => match[1]);
  const fromModule = [...readFileSync(new URL("./command-palette.mjs", import.meta.url), "utf8").matchAll(/id: "([a-z0-9-]+)"/g)].map(
    (match) => match[1],
  );
  assert.deepEqual(declared.sort(), fromModule.sort(), "命令表与动作注册必须一一对应");
  // 关键动作走的是既有按钮 / 页面，而不是另写一套。
  assert.match(ui + runtime, /settings-health-run/, "体检命令要用已有按钮");
  assert.match(runtime, /switchAppPage\("settings"\)/, "设置类命令要切到设置页");
  assert.match(runtime, /mod-group-manager-btn/, "策略组命令要用已有入口");
});

test("命令面板记住最近用过的命令（可注入存储、命名空间独立）", () => {
  assert.match(ui, /searchCommands\(COMMANDS, query, 10, recentCommandIds\)/, "空查询要按最近使用排序");
  assert.match(ui, /recentCommandIds = pushRecentCommand\(recentCommandIds, id\)/, "执行后要写回最近列表");
  assert.match(ui, /"lytvpk\.commandPalette\.recent"/, "存储键要有独立命名空间");
  assert.match(ui, /storage = null/, "存储要可注入（测试不碰真实 localStorage）");
  assert.match(ui, /return window\.localStorage;/, "要取真实 localStorage 作为默认实现");
  assert.match(ui, /最近 · /, "最近用过的条目要加标记");
  assert.match(ui, /recentCount: recentCommandIds\.length/, "状态文案要说明最近用过几条");
  // 记录发生在"执行"这条路径上：定义 1 处 + Enter 与点击各 1 处。
  const recordCalls = [...ui.matchAll(/recordCommandUse\(/g)].length;
  assert.ok(recordCalls >= 3, `执行路径上应记录最近使用，实际 ${recordCalls} 处`);
});

test("样式：面板居中靠上、有选中态，命中高亮复用 Mod 搜索那套", () => {
  assert.match(css, /\.command-palette-content/, "缺少面板容器样式");
  assert.match(css, /\.command-palette-item\.is-active/, "缺少选中态样式");
  assert.match(css, /\.command-palette-hint/, "缺少状态文案样式");
  // 命中高亮复用 mark.search-hit（同一套视觉），不要在命令面板里再定义一份。
  const matchSource = readFileSync(new URL("../features/file-list/search-match.mjs", import.meta.url), "utf8");
  assert.match(matchSource, /mark class="search-hit"/, "高亮应由 search-match 生成");
  assert.match(css, /mark\.search-hit/, "命中高亮样式应保持一份");
  assert.ok(!/mark\.command-hit/.test(css), "不要再定义命令面板专属高亮类");
});

test("搜索语法帮助里的快捷键来自共享快捷键表（含 Ctrl+K）", async () => {
  // 断言"渲染出来的内容"，而不是源码里恰好出现过某个字符串：
  // 快捷键现在由 core/shortcuts.mjs 单点维护，说明书只是它的一份投影。
  const { buildSearchHelpHtml } = await import("../features/file-list/search-help.mjs");
  const shortcuts = await import("./shortcuts.mjs");
  const html = buildSearchHelpHtml();
  assert.match(html, /Ctrl \+ K/, "快捷键清单要提到命令面板");
  assert.match(html, /Ctrl \+ F/, "也要提到聚焦搜索框");
  const ids = shortcuts.shortcutRowsByIds(["focus-search", "command-palette", "search-cursor", "search-open", "search-clear"]).map((row) => row.id);
  assert.deepEqual(ids, ["focus-search", "command-palette", "search-cursor", "search-open", "search-clear"]);
  assert.ok(!/SEARCH_SHORTCUT_ROWS = \[\s*\{/.test(help), "说明书里不该再手写一份快捷键数组");
});
