import assert from "node:assert/strict";
import test from "node:test";

import { buildParentOptions, flattenStrategyGroupTree } from "./strategy-group-tree.mjs";

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
