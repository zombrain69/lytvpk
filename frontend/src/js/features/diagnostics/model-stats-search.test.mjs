import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  describeModelStatsSearch,
  modelStatsHighlightSpec,
  modelStatsStateTags,
  searchModelStatsItems,
} from "./model-stats-search.mjs";

const items = [
  {
    title: "AK47 替换",
    name: "ak47.vpk",
    path: "D:\\addons\\ak47.vpk",
    modelCount: 1,
    models: [{ path: "models/weapons/ak47.mdl", vtxPath: "models/weapons/ak47.vtx", triangles: 1200 }],
  },
  {
    title: "武士刀",
    name: "katana.vpk",
    path: "D:\\addons\\katana.vpk",
    modelCount: 1,
    models: [{ path: "models/weapons/katana.mdl", triangles: 900, triangleStripEstimated: true }],
  },
  { title: "只有材质", name: "materials-only.vpk", path: "D:\\addons\\materials-only.vpk", message: "没检测到模型文件", models: [] },
];

const names = (result) => result.items.map((item) => item.name);

test("普通词跨字段匹配（标题 / 文件名 / 模型路径），多词是「且」", () => {
  assert.deepEqual(names(searchModelStatsItems(items, "ak47")), ["ak47.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "models weapons")), ["ak47.vpk", "katana.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "没检测到模型")), ["materials-only.vpk"]);
  assert.equal(searchModelStatsItems(items, "ak47 katana").items.length, 0, "两个词必须同时命中同一条");
});

test("-排除 与 re: 正则", () => {
  assert.deepEqual(names(searchModelStatsItems(items, "vpk -katana")), ["ak47.vpk", "materials-only.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "re:\\.mdl$")), ["ak47.vpk", "katana.vpk"]);
  const invalid = searchModelStatsItems(items, "re:[");
  assert.equal(invalid.items.length, 0);
  assert.match(invalid.regexInvalid, /regular expression/i);
  assert.match(describeModelStatsSearch({ ...invalid, query: "re:[" }), /^正则表达式无效：/);
});

test("tag: 按扫描状态筛（有模型 / 无模型 / 估算）", () => {
  assert.deepEqual(modelStatsStateTags(items[0]), ["有模型"]);
  assert.deepEqual(modelStatsStateTags(items[1]), ["有模型", "估算"]);
  assert.deepEqual(modelStatsStateTags(items[2]), ["无模型"]);

  assert.deepEqual(names(searchModelStatsItems(items, "tag:无模型")), ["materials-only.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "tag:估算")), ["katana.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "tag:有模型 -tag:估算")), ["ak47.vpk"]);
  assert.deepEqual(names(searchModelStatsItems(items, "tag:有模型|无模型")).length, 3);
});

test("计数文案与高亮参数", () => {
  assert.equal(describeModelStatsSearch({}), "");
  assert.equal(describeModelStatsSearch({ total: 3, matched: 2, query: "vpk" }), "匹配 2 / 3 个 Mod");
  assert.equal(describeModelStatsSearch({ total: 3, matched: 0, query: "zzz" }), "没有匹配「zzz」的 Mod（共 3 个）");

  assert.equal(modelStatsHighlightSpec(searchModelStatsItems(items, "")), null);
  assert.equal(modelStatsHighlightSpec(searchModelStatsItems(items, "re:[")), null);
  const spec = modelStatsHighlightSpec(searchModelStatsItems(items, "ak47 -katana"));
  assert.deepEqual(spec.terms, ["ak47"]);
});

test("模型统计面板真的接上了统一检索", () => {
  const source = readFileSync(new URL("./model-stats-scan.js", import.meta.url), "utf8");
  assert.match(source, /searchModelStatsItems\(/, "列表要用统一检索");
  assert.match(source, /describeModelStatsSearch\(/, "要有命中计数");
  assert.match(source, /modelStatsHighlightSpec\(/, "命中项要高亮");
  assert.match(source, /MODEL_STATS_SEARCH_HELP_VARIANT|buildSearchHelpHtml/, "要有语法提示（与 Mod 列表同源）");
  assert.match(source, /正则表达式无效：\$\{invalidRegex\}/, "正则写错时要说清原因，而不是「没有匹配」");
  // 键盘光标（与 Mod 列表 / 归档面板同一套）
  assert.match(source, /nextResultPath\(keys, currentCursorKey/, "缺少上下键移动光标");
  assert.match(source, /describeResultCursor\(keys, currentCursorKey, "Enter 展开 \/ 收起"\)/, "计数要显示光标位置与 Enter 的作用");
  assert.match(source, /row\.dataset\.cursorKey = `model-stats-row-/, "行上要有光标键");
  assert.match(source, /syncCursorHighlight\(list, "\[data-cursor-key\]", currentCursorKey\)/, "重画后要恢复光标");
});
