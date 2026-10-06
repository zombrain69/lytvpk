// 「模型复杂度排序」只分析当前可见结果：真机 2912 个 Mod 时原来永远全库扫（首次 43s），
// 而排序本身只作用于当前结果。这里钉住纯函数与接线。

import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

import { modelMetricScanTargets } from "./model-metric-targets.mjs";

test("有筛选（可见 < 全量）：只分析当前可见结果", () => {
  const all = [{ path: "a" }, { path: "b" }, { path: "c" }];
  assert.deepEqual(modelMetricScanTargets(all, [all[1]]), ["b"]);
});

test("没有筛选（可见 == 全量）：分析全部", () => {
  const all = [{ path: "a" }, { path: "b" }];
  assert.deepEqual(modelMetricScanTargets(all, all), ["a", "b"]);
});

test("可见为空：不分析（此时也没有东西可排序）", () => {
  assert.deepEqual(modelMetricScanTargets([{ path: "a" }], []), []);
});

test("脏数据不炸：缺 path 的条目会被过滤", () => {
  const all = [{ path: "a" }, {}];
  assert.deepEqual(modelMetricScanTargets(all, [{}, { path: "a" }]), ["a"]);
});

test("接线：排序只分析可见结果，筛选变化后按需补齐指标", () => {
  const sortingSource = readFileSync(new URL("./sorting.js", import.meta.url), "utf8");
  assert.match(
    sortingSource,
    /modelMetricScanTargets\(appState\.allVpkFiles, appState\.vpkFiles\)/,
    "模型复杂度排序要按可见结果下发分析目标",
  );
  assert.match(
    sortingSource,
    /export async function ensureVisibleModelMetrics\(/,
    "要提供筛选变化后补齐指标的入口",
  );
  assert.match(sortingSource, /命中缓存/, "结果提示要说明缓存命中情况");

  const filtersSource = readFileSync(new URL("./filters.js", import.meta.url), "utf8");
  assert.match(filtersSource, /ensureVisibleModelMetrics\(\)/, "筛选/搜索改变可见集合后要调用补齐");
});
