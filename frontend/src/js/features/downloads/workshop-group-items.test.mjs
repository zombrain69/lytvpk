import assert from "node:assert/strict";
import test from "node:test";

import { isCollectionGroup, shouldRenderWorkshopGroupItem } from "./workshop-group-items.mjs";

const collection = {
  main: { publishedfileid: "100", file_type: 2, title: "合集" },
};

test("合集本体不再渲染成卡片，成员照常渲染", () => {
  assert.equal(isCollectionGroup(collection), true);
  assert.equal(shouldRenderWorkshopGroupItem(collection, collection.main, 0), false);
  assert.equal(shouldRenderWorkshopGroupItem(collection, { publishedfileid: "101" }, 1), true);
  assert.equal(shouldRenderWorkshopGroupItem(collection, { publishedfileid: "102" }, 2), true);
});

test("普通物品组保持原样（主物品仍显示）", () => {
  const single = { main: { publishedfileid: "200", file_type: 0 } };
  assert.equal(isCollectionGroup(single), false);
  assert.equal(shouldRenderWorkshopGroupItem(single, single.main, 0), true);
});

test("主物品 ID 缺失时宁可不跳过，也不要把第一个成员误删", () => {
  const weird = { main: { file_type: 2 } };
  assert.equal(shouldRenderWorkshopGroupItem(weird, { publishedfileid: "301" }, 0), true);
});
