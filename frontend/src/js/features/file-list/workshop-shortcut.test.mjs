import assert from "node:assert/strict";
import test from "node:test";

import { hasWorkshopShortcutSource } from "./workshop-shortcut.mjs";

// 候选来源预判与后端解析链同序：.meta（workshopId）→ workshop 纯数字文件名 → addonURL0。

test("hasWorkshopShortcutSource：有 .meta 的 workshopId 即候选", () => {
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "123456789", location: "root", name: "自定名.vpk" }),
    true,
  );
});

test("hasWorkshopShortcutSource：workshop 目录下的纯数字文件名是候选", () => {
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "", location: "workshop", name: "3778253592.vpk" }),
    true,
  );
  // 隐藏（_ 前缀）的文件名里 ID 仍然有效
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "", location: "workshop", name: "_3778253592.vpk" }),
    true,
  );
});

test("hasWorkshopShortcutSource：非 workshop 位置的数字文件名不算候选", () => {
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "", location: "root", name: "3778253592.vpk" }),
    false,
  );
});

test("hasWorkshopShortcutSource：重命名后没有其它线索不算候选", () => {
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "", location: "workshop", name: "999000111_备用.vpk" }),
    false,
  );
});

test("hasWorkshopShortcutSource：addonURL0 是候选（第三层）", () => {
  assert.equal(
    hasWorkshopShortcutSource({
      workshopId: "",
      location: "root",
      name: "别的来源.vpk",
      addonURL0: "https://steamcommunity.com/sharedfiles/filedetails/?id=123456789",
    }),
    true,
  );
});

test("hasWorkshopShortcutSource：位数不足的数字名不做候选（留给后端复核）", () => {
  // 5 位数字不是有效工坊 ID（后端要求 ≥ 100000 的 6 位数）
  assert.equal(
    hasWorkshopShortcutSource({ workshopId: "", location: "workshop", name: "12345.vpk" }),
    false,
  );
});

test("hasWorkshopShortcutSource：空对象/缺字段安全返回 false", () => {
  assert.equal(hasWorkshopShortcutSource(null), false);
  assert.equal(hasWorkshopShortcutSource(undefined), false);
  assert.equal(hasWorkshopShortcutSource({}), false);
  assert.equal(hasWorkshopShortcutSource({ workshopId: "   ", location: "workshop", name: "" }), false);
});
