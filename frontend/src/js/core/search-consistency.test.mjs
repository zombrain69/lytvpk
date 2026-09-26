// 全局检索一致性审计。
//
// 目标：搜索体验不能"每个面板一套"。这份测试把现状锁成三类，并要求界面与实现一致：
//   1. 统一语法面板（6 个）：都要有 `?` 说明书 + 悬停提示，且内容来自 search-help.mjs 的同一个变体；
//   2. 服务端搜索（工坊浏览器）：必须明说"由 Steam 搜索、不支持本地语法"，并指出本地语法用在哪；
//   3. 局部小过滤器（加载顺序选择 / 组选择器 / 建议窗口 / autoexec / mdmp / 服务器地图等）：
//      保持简单过滤，**占位文案不得宣称支持本地语法**（免得用户照着写 tag: / re: 却发现没反应）。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const root = new URL("../../../", import.meta.url); // → frontend/
const read = (relative) => readFileSync(new URL(relative, root), "utf8");

const indexHtml = read("index.html");
const searchHelp = read("src/js/features/file-list/search-help.mjs");
const appRuntime = read("src/js/features/app-runtime.js");
const workshopNote = read("src/js/features/workshop/search-mode-note.mjs");
const workshopList = read("src/js/features/workshop/list.js");

/** 走统一语法的面板：id → 需要的 `?` 按钮 / 浮层 / 变体 / 实现文件。 */
const UNIFIED_PANELS = [
  {
    name: "Mod 列表",
    buttonId: "search-help-btn",
    popoverId: "search-help-popover",
    variant: "", // 默认变体：app-runtime 调 buildSearchHelpHtml() 不带参数
    source: appRuntime,
    usage: /buildSearchHelpHtml\(\)/,
  },
  {
    name: "压缩包管理",
    buttonId: "archive-search-help-btn",
    popoverId: "archive-search-help-popover",
    variant: "ARCHIVE_SEARCH_HELP_VARIANT",
    source: read("src/js/features/diagnostics/archive-manager.js"),
    usage: /buildSearchHelpHtml\(ARCHIVE_SEARCH_HELP_VARIANT\)/,
  },
  {
    name: "模型统计",
    buttonId: "model-stats-search-help-btn",
    popoverId: "model-stats-search-help-popover",
    variant: "MODEL_STATS_SEARCH_HELP_VARIANT",
    source: read("src/js/features/diagnostics/model-stats-scan.js"),
    usage: /buildSearchHelpHtml\(MODEL_STATS_SEARCH_HELP_VARIANT\)/,
  },
  {
    name: "策略组管理",
    buttonId: "strategy-group-search-help-btn",
    popoverId: "strategy-group-search-help-popover",
    variant: "GROUP_MANAGER_SEARCH_HELP_VARIANT",
    source: read("src/js/features/mod-groups/strategy-group-manager.js"),
    usage: /buildSearchHelpHtml\(GROUP_MANAGER_SEARCH_HELP_VARIANT\)/,
  },
  {
    name: "冲突检测",
    buttonId: "conflict-search-help-btn",
    popoverId: "conflict-search-help-popover",
    variant: "CONFLICT_SEARCH_HELP_VARIANT",
    source: read("src/js/features/conflicts/conflicts.js"),
    usage: /buildSearchHelpHtml\(CONFLICT_SEARCH_HELP_VARIANT\)/,
  },
  {
    name: "体检结果",
    buttonId: "settings-health-search-help-btn",
    popoverId: "settings-health-search-help-popover",
    variant: "HEALTH_SEARCH_HELP_VARIANT",
    source: read("src/js/features/settings/settings-page.js"),
    usage: /buildSearchHelpHtml\(HEALTH_SEARCH_HELP_VARIANT\)/,
  },
];

test("统一语法的 6 个面板都有 `?` 说明书，且内容来自同一份表", () => {
  assert.equal(UNIFIED_PANELS.length, 6, "面板清单要与实现同步");
  for (const panel of UNIFIED_PANELS) {
    // DOM 入口：按钮 + 浮层（浮层由 4 个静态面板写在 index.html，压缩包/模型/体检是动态创建）
    const hasStaticButton = indexHtml.includes(`id="${panel.buttonId}"`);
    const hasDynamicButton = panel.source.includes(`"${panel.buttonId}"`);
    assert.ok(hasStaticButton || hasDynamicButton, `${panel.name} 缺少 ? 按钮`);

    const hasStaticPopover = indexHtml.includes(`id="${panel.popoverId}"`);
    const hasDynamicPopover = panel.source.includes(`"${panel.popoverId}"`);
    assert.ok(hasStaticPopover || hasDynamicPopover, `${panel.name} 缺少说明书浮层`);

    // 内容同源：都调 buildSearchHelpHtml(对应变体)
    assert.match(panel.source, panel.usage, `${panel.name} 的浮层内容应来自 search-help.mjs`);
    assert.match(
      panel.source,
      /buildSearchHelpTitle\((?:[A-Z_]+)?\)/,
      `${panel.name} 的悬停提示也应同源`,
    );
    if (panel.variant) {
      assert.ok(searchHelp.includes(`export const ${panel.variant}`), `search-help.mjs 缺少变体 ${panel.variant}`);
    }
  }
});

test("工坊浏览器是服务端搜索：明说不支持本地语法，并指出去哪用", () => {
  assert.match(workshopNote, /Steam 服务端执行/, "要说明这是服务端搜索");
  assert.match(workshopNote, /不支持 tag: \/ re:/, "要明说不支持本地语法");
  for (const panel of ["Mod 列表", "压缩包管理", "模型统计", "策略组管理", "冲突检测", "体检结果"]) {
    assert.ok(workshopNote.includes(panel), `工坊提示里应指出本地语法可用于「${panel}」`);
  }
  // 界面真的用上了这份文案（占位 + 悬停提示）
  assert.match(workshopList, /WORKSHOP_SEARCH_PLACEHOLDER/, "占位文案要用同一份说明");
  assert.match(workshopList, /describeWorkshopSearchMode\(\)/, "悬停提示要用同一份说明");
  assert.match(indexHtml, /id="browser-search-input"[\s\S]{0,240}不支持 tag:/, "静态占位也要写清楚");
  // 服务端搜索的占位不能再写成本地语法那一套
  assert.ok(!/id="browser-search-input"[^>]*tag:/.test(indexHtml.replace("不支持 tag:", "")), "不要把 tag: 宣传成工坊搜索的写法");
});

test("局部小过滤器不冒充本地语法（占位文案里不出现 tag: / re: / -排除）", () => {
  const LOCAL_FILTERS = [
    { name: "加载顺序：来源列表", snippet: 'id="load-order-rule-sources-search"' },
    { name: "加载顺序：参照列表", snippet: 'id="load-order-rule-target-search"' },
    { name: "组选择器", snippet: 'class="group-picker-search"' },
    { name: "分组建议窗口", snippet: 'class="mod-group-suggest-search"' },
    { name: "工坊浏览器", snippet: 'id="browser-search-input"' },
  ];
  for (const filter of LOCAL_FILTERS) {
    const start = indexHtml.indexOf(filter.snippet);
    assert.ok(start >= 0, `找不到 ${filter.name} 的输入框`);
    const snippet = indexHtml.slice(start, start + 320);
    const placeholder = snippet.match(/placeholder="([^"]*)"/)?.[1] || "";
    assert.ok(placeholder, `${filter.name} 应有占位文案`);
    // 允许"不支持 tag:"这种否定说法，但不能再把它宣传成可用写法
    const advertised = placeholder.replace(/不支持[^）)]*/g, "");
    assert.ok(!/tag:|re:|排除/.test(advertised), `${filter.name} 的占位文案不该宣称支持本地语法：${placeholder}`);
  }

  // 动态生成的局部过滤器同样检查
  const autoexec = read("src/js/features/diagnostics/autoexec-tool.js");
  const mdmp = read("src/js/features/diagnostics/mdmp-report.js");
  const panelModal = read("src/js/features/servers/panel-modal.js");
  for (const [name, source] of [["autoexec 命令列表", autoexec], ["mdmp 表格", mdmp], ["服务器地图", panelModal]]) {
    const placeholders = [...source.matchAll(/placeholder\s*[:=]\s*"([^"]*)"/g)].map((match) => match[1]);
    for (const value of placeholders) {
      assert.ok(!/tag:|re: |排除/.test(value), `${name} 的占位文案不该宣称支持本地语法：${value}`);
    }
  }
});
