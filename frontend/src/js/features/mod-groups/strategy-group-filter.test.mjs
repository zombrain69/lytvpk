import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  describeStrategyGroupSearch,
  filterStrategyGroupRows,
  formatBatchHint,
  groupTags,
} from "./strategy-group-filter.mjs";

const rows = [
  { group: { id: "box", name: "!!医疗箱 系列", members: [{ name: "bbb武器.vpk" }] }, depth: 1 },
  { group: { id: "fire", name: "!花火零食 系列", parentId: "box", members: [{ name: "aaa武器.vpk" }] }, depth: 2 },
  { group: { id: "bin", name: "【BA垃姬桶】垃圾桶系列", parentId: "box", members: [{ name: "ccc角色.vpk" }] }, depth: 2 },
  { group: { id: "chiffon", name: "Chiffon 下午茶", members: [{ name: "ddd角色.vpk" }] }, depth: 1 },
];

// 带策略 / 层级的夹具：验证 tag: 与 -排除 / re: 这些统一语法的写法。
const taggedRows = [
  { group: { id: "single1", name: "AK47 互斥", strategy: "single", members: [{ name: "ak47-a.vpk" }, { name: "ak47-b.vpk" }] }, depth: 1 },
  { group: { id: "all1", name: "医疗箱套装", strategy: "all", enforce: true, members: [{ name: "medkit-model.vpk" }, { name: "medkit-mat.vpk" }] }, depth: 1 },
  { group: { id: "child1", name: "AK47 互斥 · 备用", parentId: "single1", strategy: "single", members: [{ name: "ak47-c.vpk" }] }, depth: 2 },
  { group: { id: "off1", name: "旧套装", strategy: "off", members: [] }, depth: 1 },
];

test("空关键字返回全部（保持原顺序）", () => {
  assert.deepEqual(
    filterStrategyGroupRows(rows, "").map((row) => row.group.id),
    ["box", "fire", "bin", "chiffon"],
  );
  assert.deepEqual(
    filterStrategyGroupRows(rows, "   ").map((row) => row.group.id),
    ["box", "fire", "bin", "chiffon"],
  );
});

test("按组名搜索（忽略大小写）", () => {
  assert.deepEqual(filterStrategyGroupRows(rows, "医疗箱").map((row) => row.group.id), ["box", "fire", "bin"]);
  assert.deepEqual(filterStrategyGroupRows(rows, "chiffon").map((row) => row.group.id), ["chiffon"]);
});

test("按成员名搜索：命中子组时带上它的上级，缩进才不会指向看不见的父组", () => {
  assert.deepEqual(filterStrategyGroupRows(rows, "ccc角色").map((row) => row.group.id), ["box", "bin"]);
  assert.deepEqual(filterStrategyGroupRows(rows, "ddd角色").map((row) => row.group.id), ["chiffon"]);
});

test("父组命中时保留它的下级（整棵树一起显示）", () => {
  const kept = filterStrategyGroupRows(rows, "医疗箱").map((row) => row.group.id);
  assert.deepEqual(kept, ["box", "fire", "bin"]);
});

test("没有匹配时返回空数组", () => {
  assert.deepEqual(filterStrategyGroupRows(rows, "不存在的组"), []);
});

test("describeStrategyGroupSearch 说明显示数量", () => {
  assert.equal(describeStrategyGroupSearch({ total: 19, shown: 19, query: "" }), "共 19 个组");
  assert.equal(describeStrategyGroupSearch({ total: 19, shown: 3, query: "医疗箱" }), "显示 3 / 19 个组");
  assert.equal(describeStrategyGroupSearch({ total: 19, shown: 0, query: "x" }), "没有匹配的组（共 19 个）");
});

test("formatBatchHint 在没勾选时说明怎么勾、勾了之后给数量", () => {
  const empty = formatBatchHint(0);
  assert.match(empty, /先勾选组/);
  assert.match(empty, /批量删除/);
  assert.equal(formatBatchHint(3), "已勾选 3 个组，下面的批量按钮已可用");
});

test("统一语法：多个普通词要同时命中，引号短语当作一个词", () => {
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "AK47 互斥").map((row) => row.group.id),
    ["single1", "child1"],
  );
  // 两个词可以分别命中组名与成员名
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "医疗箱 medkit-mat").map((row) => row.group.id),
    ["all1"],
  );
  assert.deepEqual(filterStrategyGroupRows(taggedRows, '"AK47 互斥"').map((row) => row.group.id), ["single1", "child1"]);
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "医疗箱 AK47").map((row) => row.group.id), []);
});

test("统一语法：-排除 与 re: 正则", () => {
  // 排除掉子组后只剩顶层那个
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "AK47 -备用").map((row) => row.group.id),
    ["single1"],
  );
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "re:^ak47-").map((row) => row.group.id),
    ["single1", "child1"],
  );
  // 正则写错 → 不返回任何组（调用方会把原因写在计数文案里）
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "re:["), []);
  assert.match(describeStrategyGroupSearch({ total: 4, shown: 0, query: "re:[" }), /^正则表达式无效：/);
});

test("统一语法：tag: 按策略 / 层级 / 自动联动筛组", () => {
  assert.deepEqual(groupTags(taggedRows[1].group), ["全开", "顶层", "自动联动"]);
  assert.deepEqual(groupTags(taggedRows[2].group), ["单选", "子组"]);

  assert.deepEqual(filterStrategyGroupRows(taggedRows, "tag:单选").map((row) => row.group.id), ["single1", "child1"]);
  // 注意：`tag:顶层` 命中父组后，子组会按"整棵树一起显示"的既有约定一起留下
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "tag:顶层").map((row) => row.group.id),
    ["single1", "all1", "child1", "off1"],
  );
  // 想只要顶层、不要子组：用 -tag:子组（排除行不会被树形补全加回来）
  assert.deepEqual(
    filterStrategyGroupRows(taggedRows, "tag:顶层 -tag:子组").map((row) => row.group.id),
    ["single1", "all1", "off1"],
  );
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "tag:自动联动").map((row) => row.group.id), ["all1"]);
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "tag:全开|全关").map((row) => row.group.id), ["all1", "off1"]);
  // tag 与普通词可以组合："单选的组里名字带 AK47 的"
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "tag:单选 AK47").map((row) => row.group.id), ["single1", "child1"]);
  assert.deepEqual(filterStrategyGroupRows(taggedRows, "tag:单选 -tag:子组").map((row) => row.group.id), ["single1"]);
});

test("窗口真的接上了统一语法与说明书（接线断言）", () => {
  const manager = readFileSync(new URL("./strategy-group-manager.js", import.meta.url), "utf8");
  const html = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
  const help = readFileSync(new URL("../file-list/search-help.mjs", import.meta.url), "utf8");

  assert.match(manager, /buildSearchHelpTitle\(GROUP_MANAGER_SEARCH_HELP_VARIANT\)/, "搜索框提示要与说明书同源");
  assert.match(manager, /buildSearchHelpHtml\(GROUP_MANAGER_SEARCH_HELP_VARIANT\)/, "要挂上同一份说明书浮层");
  assert.match(manager, /parseSearchSyntax\(managerQuery\)\.regexInvalid/, "正则写错时要直说原因");
  assert.match(html, /id="strategy-group-search-help-btn"/, "缺少 `?` 按钮");
  assert.match(html, /id="strategy-group-search-help-popover"/, "缺少浮层容器");
  assert.match(html, /tag:状态/, "占位文字要能看出支持 tag: 语法");
  assert.match(help, /GROUP_MANAGER_SEARCH_HELP_VARIANT/, "说明书要有一份组的变体");
});
