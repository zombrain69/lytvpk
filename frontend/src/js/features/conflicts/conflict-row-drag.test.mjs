import test from "node:test";
import assert from "node:assert/strict";

import {
  computeInsertionIndex,
  describeRowDrop,
  planRowDrop,
} from "./conflict-row-drag.mjs";

const rows = (...tops) => tops.map((top) => ({ top, height: 40 }));

test("computeInsertionIndex：按中线判断落点", () => {
  const rects = rows(0, 40, 80);
  assert.equal(computeInsertionIndex(10, rects), 0); // 第一行上半 → 放最前
  assert.equal(computeInsertionIndex(25, rects), 1); // 越过第一行中线 → 落在一、二之间
  assert.equal(computeInsertionIndex(75, rects), 2);
  assert.equal(computeInsertionIndex(200, rects), 3); // 全在下方 → 放最后
});

test("computeInsertionIndex：空列表 / 缺矩形也不炸", () => {
  assert.equal(computeInsertionIndex(50, []), 0);
  assert.equal(computeInsertionIndex(50, [null, { top: 100, height: 40 }]), 0);
});

test("planRowDrop：最前 → before 第一行；中间与末尾 → after 上一行", () => {
  const others = ["a.vpk", "b.vpk", "c.vpk"];
  assert.deepEqual(planRowDrop(others, 0), { targetPath: "a.vpk", direction: "before" });
  assert.deepEqual(planRowDrop(others, 1), { targetPath: "a.vpk", direction: "after" });
  assert.deepEqual(planRowDrop(others, 2), { targetPath: "b.vpk", direction: "after" });
  assert.deepEqual(planRowDrop(others, 3), { targetPath: "c.vpk", direction: "after" });
  assert.equal(planRowDrop([], 0), null);
});

test("describeRowDrop：位置没变不算移动，位置变了要能识别", () => {
  const original = ["d.vpk", "a.vpk", "b.vpk", "c.vpk"];
  const others = ["a.vpk", "b.vpk", "c.vpk"];
  // 拖回原位（插回下标 0）→ 顺序不变
  assert.deepEqual(describeRowDrop(original, others, "d.vpk", 0), {
    moved: false,
    dropped: ["d.vpk", "a.vpk", "b.vpk", "c.vpk"],
  });
  // 拖到 a 之后 → 顺序变化
  assert.deepEqual(describeRowDrop(original, others, "d.vpk", 1), {
    moved: true,
    dropped: ["a.vpk", "d.vpk", "b.vpk", "c.vpk"],
  });
  // 拖到最后
  assert.deepEqual(describeRowDrop(original, others, "d.vpk", 3), {
    moved: true,
    dropped: ["a.vpk", "b.vpk", "c.vpk", "d.vpk"],
  });
});
