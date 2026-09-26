import assert from "node:assert/strict";
import test from "node:test";

import {
  STRATEGY_GROUP_MAX_DEPTH,
  applyStrategyGroupDropOrder,
  buildParentOptions,
  flattenStrategyGroupTree,
  formatStrategyGroupDropMessage,
  resolveStrategyGroupDrop,
} from "./strategy-group-tree.mjs";

// 拖放排序的测试夹具：a 是顶层组，b 是 a 的子组，c 是 b 的子组，d 是另一个顶层组。
const dragRows = [
  { group: { id: "a", name: "甲", parentId: "" }, depth: 1 },
  { group: { id: "b", name: "乙", parentId: "a" }, depth: 2 },
  { group: { id: "c", name: "丙", parentId: "b" }, depth: 3 },
  { group: { id: "d", name: "丁", parentId: "" }, depth: 1 },
];

test("resolveStrategyGroupDrop 允许放进目标组并给出新的上级", () => {
  const plan = resolveStrategyGroupDrop(dragRows, "d", "a", "inside");
  assert.equal(plan.ok, true);
  assert.equal(plan.parentId, "a");
  assert.equal(plan.newDepth, 2);
  assert.equal(plan.noop, false);
});

test("resolveStrategyGroupDrop 拒绝拖到自己与自己的子组", () => {
  assert.match(resolveStrategyGroupDrop(dragRows, "a", "a", "inside").reason, /自己/);
  const intoChild = resolveStrategyGroupDrop(dragRows, "a", "c", "inside");
  assert.equal(intoChild.ok, false);
  assert.match(intoChild.reason, /子组/);
});

test("resolveStrategyGroupDrop 拒绝超过最大层级", () => {
  const deepRows = [
    { group: { id: "a", name: "甲", parentId: "" }, depth: 1 },
    { group: { id: "b", name: "乙", parentId: "a" }, depth: 2 },
    { group: { id: "c", name: "丙", parentId: "b" }, depth: 3 },
    { group: { id: "d", name: "丁", parentId: "c" }, depth: 4 },
    { group: { id: "x", name: "戊", parentId: "" }, depth: 1 },
    { group: { id: "y", name: "己", parentId: "x" }, depth: 2 },
  ];
  const plan = resolveStrategyGroupDrop(deepRows, "x", "c", "inside");
  assert.equal(plan.ok, false);
  assert.match(plan.reason, new RegExp(`最多支持 ${STRATEGY_GROUP_MAX_DEPTH} 层`));
  // 同一棵子树拖到浅处仍然合法。
  assert.equal(resolveStrategyGroupDrop(deepRows, "d", "x", "inside").ok, true);
});

test("resolveStrategyGroupDrop 顶层与同级排序的 noop 判定", () => {
  const top = resolveStrategyGroupDrop(dragRows, "d", "", "root");
  assert.equal(top.ok, true);
  assert.equal(top.parentId, "");
  assert.equal(top.noop, true, "d 已经在顶层末尾，不应重写");

  assert.equal(resolveStrategyGroupDrop(dragRows, "d", "a", "after").noop, true);
  // 子组拖到父组的前面 = 变成父组的同级，是一次真实移动。
  const childToTop = resolveStrategyGroupDrop(dragRows, "b", "a", "before");
  assert.equal(childToTop.ok, true);
  assert.equal(childToTop.parentId, "");
  assert.equal(childToTop.noop, false);
  // 同级排序落在自己的下级上同样要按"会成环"拒绝。
  assert.equal(resolveStrategyGroupDrop(dragRows, "b", "c", "before").ok, false);
  assert.equal(resolveStrategyGroupDrop(dragRows, "b", "d", "before").noop, false);
});

test("applyStrategyGroupDropOrder 整棵子树一起移动", () => {
  // 把甲（含乙、丙）拖到丁后面：甲子树整体后移。
  assert.deepEqual(applyStrategyGroupDropOrder(dragRows, "a", "d", "after"), [
    "d",
    "a",
    "b",
    "c",
  ]);
  // 把丁放进甲里面：丁排到甲子树的末尾。
  assert.deepEqual(applyStrategyGroupDropOrder(dragRows, "d", "a", "inside"), [
    "a",
    "b",
    "c",
    "d",
  ]);
  // 把丁排到乙前面：同级重排（丁成为甲的第二个子组），只动自己。
  assert.deepEqual(applyStrategyGroupDropOrder(dragRows, "d", "b", "before"), [
    "a",
    "d",
    "b",
    "c",
  ]);
  // 拖到顶层末尾。
  assert.deepEqual(applyStrategyGroupDropOrder(dragRows, "a", "", "root"), [
    "d",
    "a",
    "b",
    "c",
  ]);
});

test("formatStrategyGroupDropMessage 给出可读提示", () => {
  assert.equal(formatStrategyGroupDropMessage(dragRows, "d", "a", "inside"), "已把「丁」移动到「甲」下面");
  assert.equal(formatStrategyGroupDropMessage(dragRows, "b", "d", "before"), "已把「乙」排到「丁」前面");
  assert.equal(formatStrategyGroupDropMessage(dragRows, "b", "", "root"), "已把「乙」移动到顶层");
});

test("flattenStrategyGroupTree keeps flat default when no parents exist", () => {
  const flat = [
    { id: "a", name: "A" },
    { id: "b", name: "B" },
  ];
  const rows = flattenStrategyGroupTree(null, flat);
  assert.deepEqual(
    rows.map((row) => [row.group.id, row.depth]),
    [["a", 1], ["b", 1]],
  );
});

test("flattenStrategyGroupTree walks nested nodes", () => {
  const tree = [
    {
      group: { id: "parent", name: "父" },
      depth: 1,
      children: [
        { group: { id: "child", name: "子" }, depth: 2, children: [] },
      ],
    },
    { group: { id: "other", name: "其它" }, depth: 1, children: [] },
  ];
  assert.deepEqual(
    flattenStrategyGroupTree(tree, []).map((row) => [row.group.id, row.depth]),
    [["parent", 1], ["child", 2], ["other", 1]],
  );
});

test("buildParentOptions excludes self and descendants", () => {
  const rows = [
    { group: { id: "a", name: "A", parentId: "" }, depth: 1 },
    { group: { id: "b", name: "B", parentId: "a" }, depth: 2 },
    { group: { id: "c", name: "C", parentId: "b" }, depth: 3 },
    { group: { id: "d", name: "D", parentId: "" }, depth: 1 },
  ];
  const options = buildParentOptions(rows, "a");
  assert.deepEqual(options.map((option) => option.id), ["d"]);
  assert.equal(options[0].label, "D");
});

test("buildParentOptions keeps indentation labels for nested candidates", () => {
  const rows = [
    { group: { id: "a", name: "A", parentId: "" }, depth: 1 },
    { group: { id: "b", name: "B", parentId: "a" }, depth: 2 },
  ];
  const options = buildParentOptions(rows, "c");
  assert.deepEqual(options.map((option) => option.label), ["A", "— B"]);
});
