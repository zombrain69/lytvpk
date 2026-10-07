import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  applySortOrder,
  compareByPriority,
  nextSortState,
  sortByPriority,
} from "./priority-sort.mjs";

test("按有效分层升序，越小越先加载", () => {
  const list = [
    { name: "c", layer: 30, order: 0 },
    { name: "a", layer: 5, order: 7 },
    { name: "b", layer: 12, order: 1 },
  ];
  assert.deepEqual(
    sortByPriority(list).map((item) => item.name),
    ["a", "b", "c"],
  );
});

test("同层内按 addonlist 顺序号保持真实相对顺序", () => {
  const list = [
    { name: "second", layer: 3, order: 9 },
    { name: "first", layer: 3, order: 2 },
  ];
  assert.deepEqual(
    sortByPriority(list).map((item) => item.name),
    ["first", "second"],
  );
});

test("未记录进 addonlist 的 Mod 排在末尾，并按名称排序", () => {
  const list = [
    { name: "zzz", layer: undefined, order: undefined },
    { name: "aaa", layer: undefined, order: undefined },
    { name: "listed", layer: 9, order: 4 },
  ];
  assert.deepEqual(
    sortByPriority(list).map((item) => item.name),
    ["listed", "aaa", "zzz"],
  );
});

test("没有分层信息时退化为顺序号排序", () => {
  const list = [
    { name: "b", layer: 1, order: 1 },
    { name: "a", layer: 0, order: 0 },
  ];
  assert.deepEqual(
    sortByPriority(list).map((item) => item.name),
    ["a", "b"],
  );
});

test("layer 为 0 也算已设置（不能被当成未设置）", () => {
  assert.ok(compareByPriority({ name: "z", layer: 0 }, { name: "a", layer: null }) < 0);
});

test("入参不会被修改", () => {
  const list = [{ name: "b", layer: 2 }, { name: "a", layer: 1 }];
  const sorted = sortByPriority(list);
  assert.notEqual(sorted, list);
  assert.deepEqual(list.map((item) => item.name), ["b", "a"]);
});

// 用户明确要求：默认排序就是优先级排序。这里用源码级断言钉住默认值，
// 并确认排序实现确实使用了有效分层（而不是只按文件名）。
test("默认排序是优先级排序，且排序键包含有效分层", () => {
  const stateSource = readFileSync(new URL("../state.js", import.meta.url), "utf8");
  // 排序设置现在会被持久化（对齐上游 7b0818c）：默认值来自 config，
  // 但"没设置过"时的兜底仍然必须是 loadOrder。
  assert.match(
    stateSource,
    /sortType:\s*getConfig\(\)\.sortType\s*\|\|\s*"loadOrder"/,
    "appState.sortType 必须从配置读取，缺省回落到 loadOrder（优先级排序）",
  );
  const configSource = readFileSync(new URL("../../core/config.js", import.meta.url), "utf8");
  assert.match(configSource, /sortType:\s*"loadOrder"/, "配置默认排序必须是 loadOrder");

  const sortingSource = readFileSync(new URL("./sorting.js", import.meta.url), "utf8");
  // 只看 applySort 内部，避免命中「更新排序按钮文案」那处同名判断。
  const applySortStart = sortingSource.indexOf("export function applySort(");
  assert.ok(applySortStart >= 0, "sorting.js 应导出 applySort");
  const branchStart = sortingSource.indexOf('appState.sortType === "loadOrder"', applySortStart);
  assert.ok(branchStart >= 0, "sorting.js 应处理 loadOrder 排序");
  const branch = sortingSource.slice(branchStart, branchStart + 700);
  assert.ok(
    branch.includes("compareByPriority") || branch.includes("getFileEffectiveLayer"),
    "优先级排序必须使用有效分层作为排序键",
  );
});

// 回归（2026-10-07 用户反馈）：优先级排序的"倒序"曾经完全不可达 ——
// ① handleLoadOrderSort 写死 asc（上游与早期实现也如此）；② 装饰-排序重构时
//    loadOrder 分支漏了按 sortOrder 取反。这两条都必须钉住。
test("优先级排序支持顺序/倒序：点第二次切换方向，且比较结果按方向取反", () => {
  const source = readFileSync(new URL("./sorting.js", import.meta.url), "utf8");
  const applySortStart = source.indexOf("export function applySort(");
  assert.ok(applySortStart >= 0, "sorting.js 应导出 applySort");
  const branchStart = source.indexOf('appState.sortType === "loadOrder"', applySortStart);
  assert.ok(branchStart >= 0, "sorting.js 应处理 loadOrder 排序");
  const branch = source.slice(branchStart, branchStart + 900);
  assert.match(
    branch,
    /applySortOrder\(/,
    "loadOrder 分支必须按 appState.sortOrder 取反，否则「优先级排序（倒序）」点了不反转",
  );
  assert.match(
    source,
    /handleLoadOrderSort[\s\S]{0,400}nextSortState\(/,
    "点第二次「优先级排序」要切换 顺序/倒序",
  );
  assert.match(source, /sort-direction-chip/, "排序菜单要有方向 chip，把两个方向显式摆出来");
});

test("nextSortState：同项切换方向，换项用该项默认方向", () => {
  assert.deepEqual(nextSortState("loadOrder", "asc", "loadOrder"), { type: "loadOrder", order: "desc" });
  assert.deepEqual(nextSortState("loadOrder", "desc", "loadOrder"), { type: "loadOrder", order: "asc" });
  assert.deepEqual(nextSortState("name", "asc", "loadOrder"), { type: "loadOrder", order: "asc" });
  assert.deepEqual(nextSortState("loadOrder", "asc", "date"), { type: "date", order: "desc" });
  assert.deepEqual(nextSortState("date", "desc", "size"), { type: "size", order: "desc" });
  assert.deepEqual(nextSortState("date", "desc", "modelComplexity"), {
    type: "modelComplexity",
    order: "desc",
  });
  assert.deepEqual(nextSortState("date", "desc", "name"), { type: "name", order: "asc" });
});

test("applySortOrder：降序取反、平局保持 0、升序原样", () => {
  assert.equal(applySortOrder(3, "asc"), 3);
  assert.equal(applySortOrder(3, "desc"), -3);
  assert.equal(applySortOrder(-2, "desc"), 2);
  assert.equal(Object.is(applySortOrder(0, "desc"), 0), true, "平局必须保持 0（稳定排序依赖它）");
});

// 排序设置持久化（对齐上游 7b0818c）：改排序 → 写 config.json → 下次启动恢复。
// 上游踩过的坑就是"排序只在本次会话生效"，重启又回到默认排序。
test("排序设置会持久化并在启动时恢复", () => {
  const here = new URL("./", import.meta.url);
  const sorting = readFileSync(new URL("./sorting.js", here), "utf8");
  const state = readFileSync(new URL("../state.js", here), "utf8");
  const config = readFileSync(new URL("../../core/config.js", here), "utf8");

  assert.match(sorting, /export function saveSortPreference\(\)/, "排序变化要有统一的持久化入口");
  assert.match(sorting, /saveConfig\(\{[\s\S]{0,120}sortType/, "持久化要真的写 sortType / sortOrder");
  const calls = sorting.match(/saveSortPreference\(\);/g) || [];
  assert.ok(calls.length >= 3, `名称/日期/大小、优先级、模型复杂度三类排序入口都要持久化，实际 ${calls.length} 处`);
  assert.match(state, /sortType: getConfig\(\)\.sortType/, "appState 启动时要读配置里的排序");
  assert.match(state, /appState\.sortType = config\.sortType/, "applyConfigToAppState 要同步排序设置");
  assert.match(config, /sortType: "loadOrder"/, "默认排序保持「优先级」");
});

test("优先级降序：compareByPriority + applySortOrder 组合真的反转", () => {
  const entries = [
    { name: "a", layer: 1, order: 0 },
    { name: "b", layer: 2, order: 1 },
    { name: "c", layer: 3, order: 2 },
  ];
  const asc = [...entries]
    .sort((left, right) => applySortOrder(compareByPriority(left, right), "asc"))
    .map((item) => item.name);
  const desc = [...entries]
    .sort((left, right) => applySortOrder(compareByPriority(left, right), "desc"))
    .map((item) => item.name);
  assert.deepEqual(asc, ["a", "b", "c"]);
  assert.deepEqual(desc, ["c", "b", "a"]);
});
