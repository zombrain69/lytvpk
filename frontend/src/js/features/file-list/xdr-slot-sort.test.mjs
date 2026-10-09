import assert from "node:assert/strict";
import test from "node:test";

import { applySortOrder } from "./priority-sort.mjs";
import { compareByXDRSlot, minXDRSlot } from "./xdr-slot-sort.mjs";

test("minXDRSlot 取最小槽位号，无槽位/异常值返回 null", () => {
  assert.equal(minXDRSlot({ xdrSlots: [{ slot: 32 }, { slot: 7 }] }), 7);
  assert.equal(minXDRSlot({ xdrSlots: [{ slot: 48 }] }), 48);
  assert.equal(minXDRSlot({ xdrSlots: [] }), null);
  assert.equal(minXDRSlot({}), null);
  assert.equal(minXDRSlot({ xdrSlots: [{ slot: 0 }, { slot: "x" }, {}] }), null);
});

test("compareByXDRSlot：槽位升序、同槽按名称、无槽位排末尾", () => {
  const decorate = (name, slot) => ({ name, slot });
  const list = [
    decorate("zzz.vpk", null),
    decorate("c.vpk", 48),
    decorate("b.vpk", 7),
    decorate("a.vpk", 7),
  ];

  const asc = [...list]
    .sort((left, right) => applySortOrder(compareByXDRSlot(left, right), "asc"))
    .map((item) => item.name);
  assert.deepEqual(asc, ["a.vpk", "b.vpk", "c.vpk", "zzz.vpk"]);

  // 倒序与其它排序一致：整体取反（无槽位的反而排最前，与优先级排序倒序的语义相同）。
  const desc = [...list]
    .sort((left, right) => applySortOrder(compareByXDRSlot(left, right), "desc"))
    .map((item) => item.name);
  assert.deepEqual(desc, ["zzz.vpk", "c.vpk", "b.vpk", "a.vpk"]);
});

test("compareByXDRSlot：两边都没有槽位时按名称兜底", () => {
  const list = [
    { name: "b.vpk", slot: null },
    { name: "a.vpk", slot: null },
  ];
  const sorted = [...list]
    .sort((left, right) => applySortOrder(compareByXDRSlot(left, right), "asc"))
    .map((item) => item.name);
  assert.deepEqual(sorted, ["a.vpk", "b.vpk"]);
});
