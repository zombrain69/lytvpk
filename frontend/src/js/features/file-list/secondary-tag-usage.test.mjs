import assert from "node:assert/strict";
import test from "node:test";

import { compareSecondaryTagsByUsage, secondaryTagCount } from "./secondary-tag-usage.mjs";

test("常用子标签按命中 Mod 数降序，同分按字典序", () => {
  const counts = { ak47: 12, 步枪: 12, 贴图: 40, hud: 1 };
  const tags = ["AK47", "步枪", "贴图", "HUD"].sort((a, b) => compareSecondaryTagsByUsage(a, b, counts));
  assert.equal(tags[0], "贴图", "命中最多的排最前");
  assert.equal(tags[3], "HUD", "命中最少的排最后");
  assert.deepEqual(new Set(tags.slice(1, 3)), new Set(["AK47", "步枪"]), "同分的两个标签挨在一起");
});

test("缺计数/缺数据时退回字典序，不抛错", () => {
  const tags = ["Zoey", "Bill", "Coach"].sort((a, b) => compareSecondaryTagsByUsage(a, b, undefined));
  assert.deepEqual(tags, ["Bill", "Coach", "Zoey"]);
  assert.equal(secondaryTagCount(null, "贴图"), 0);
  assert.equal(secondaryTagCount({ 贴图: "40" }, "贴图"), 0, "非数字计数按 0 处理");
  assert.equal(secondaryTagCount({ ak47: 3 }, "AK47"), 3, "大小写不敏感");
});
