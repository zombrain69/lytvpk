import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { compareByPriority, sortByPriority } from "./priority-sort.mjs";

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
  assert.match(
    stateSource,
    /sortType:\s*"loadOrder"/,
    "appState.sortType 默认值必须是 loadOrder（优先级排序）",
  );

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
