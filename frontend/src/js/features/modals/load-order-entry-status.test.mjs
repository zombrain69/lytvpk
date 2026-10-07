import assert from "node:assert/strict";
import test from "node:test";

import {
  LOAD_ORDER_ENTRY_STATUS,
  classifyLoadOrderEntries,
  normalizeLoadOrderKey,
} from "./load-order-entry-status.mjs";

const rootFile = (name) => ({ name, location: "root" });
const workshopFile = (name) => ({ name, location: "workshop" });
const disabledFile = (name) => ({ name, location: "disabled" });
const entry = (key) => ({ key });

test("normalizeLoadOrderKey 统一斜杠与大小写", () => {
  assert.equal(normalizeLoadOrderKey("Workshop\\Zoey.VPK"), "workshop\\zoey.vpk");
  assert.equal(normalizeLoadOrderKey("materials/models/a.VMT"), "materials\\models\\a.vmt");
});

test("classifyLoadOrderEntries 区分存在 / 已禁用 / 文件不存在", () => {
  const entries = [
    entry("zztest_rifle_a.vpk"),
    entry("workshop\\999000111.vpk"),
    entry("zztest_off.vpk"),
    entry("zztest_ghost.vpk"),
  ];
  const files = [
    rootFile("zztest_rifle_a.vpk"),
    workshopFile("999000111.vpk"),
    disabledFile("zztest_off.vpk"),
  ];

  const { statuses, invalidCount, unrecorded } = classifyLoadOrderEntries(entries, files);
  assert.equal(statuses.get("zztest_rifle_a.vpk"), LOAD_ORDER_ENTRY_STATUS.existing);
  assert.equal(statuses.get("workshop\\999000111.vpk"), LOAD_ORDER_ENTRY_STATUS.existing);
  assert.equal(statuses.get("zztest_off.vpk"), LOAD_ORDER_ENTRY_STATUS.disabled);
  assert.equal(statuses.get("zztest_ghost.vpk"), LOAD_ORDER_ENTRY_STATUS.missing);
  assert.equal(invalidCount, 2);
  assert.equal(unrecorded, 0);
});

test("classifyLoadOrderEntries 统计未记录的新 Mod", () => {
  const entries = [entry("a.vpk")];
  const files = [rootFile("a.vpk"), rootFile("b.vpk"), workshopFile("c.vpk")];

  const { statuses, unrecorded, invalidCount, unrecordedEntries } = classifyLoadOrderEntries(entries, files);
  assert.equal(statuses.get("a.vpk"), LOAD_ORDER_ENTRY_STATUS.existing);
  assert.equal(unrecorded, 2);
  assert.equal(invalidCount, 0);
  // 新增条目的明细（key/name/location）要能拿到，加载顺序预览靠它渲染「新增」角标。
  assert.deepEqual(
    unrecordedEntries.map((item) => item.key).sort(),
    ["b.vpk", "workshop\\c.vpk"],
  );
  const workshopItem = unrecordedEntries.find((item) => item.key === "workshop\\c.vpk");
  assert.equal(workshopItem.location, "workshop");
  assert.equal(workshopItem.name, "c.vpk");
  // disabled 里的文件不算「新增」：它已经有一条（或应该有一条）addonlist 记录，只是被禁用。
  const { unrecordedEntries: withDisabled } = classifyLoadOrderEntries([], [disabledFile("d.vpk")]);
  assert.deepEqual(withDisabled, []);
});

test("classifyLoadOrderEntries 容忍空条目 / 重复键 / 缺文件名", () => {
  const entries = [entry(""), entry("dup.vpk"), entry("DUP.vpk"), entry(null)];
  const files = [rootFile("dup.vpk"), { location: "root" }, null];

  const { statuses, invalidCount, unrecorded } = classifyLoadOrderEntries(entries, files);
  assert.equal(statuses.size, 1);
  assert.equal(statuses.get("dup.vpk"), LOAD_ORDER_ENTRY_STATUS.existing);
  assert.equal(invalidCount, 0);
  assert.equal(unrecorded, 0);
});
