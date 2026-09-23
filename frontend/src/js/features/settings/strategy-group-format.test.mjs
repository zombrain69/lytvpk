import assert from "node:assert/strict";
import { test } from "node:test";

import {
  formatStrategyGroupApplySummary,
  formatStrategyGroupEnforcementNotice,
  formatStrategyGroupBatchConfirm,
  formatStrategyGroupBatchResult,
  formatStrategyGroupFilterState,
  formatStrategyGroupSelectionLabel,
  formatStrategyGroupStrategy,
  summarizeStrategyGroupSelection,
} from "./strategy-group-format.mjs";

test("formatStrategyGroupEnforcementNotice only fires when members changed", () => {
  assert.equal(formatStrategyGroupEnforcementNotice(0), "");
  assert.equal(formatStrategyGroupEnforcementNotice(undefined), "");
  assert.equal(
    formatStrategyGroupEnforcementNotice(2),
    "已按策略组自动联动另外 2 个 Mod",
  );
});

test("formatStrategyGroupStrategy maps known strategies and falls back", () => {
  assert.equal(formatStrategyGroupStrategy("single"), "互斥单选");
  assert.equal(formatStrategyGroupStrategy("single_random"), "随机单选");
  assert.equal(formatStrategyGroupStrategy("all"), "全部开启");
  assert.equal(formatStrategyGroupStrategy("off"), "全部关闭");
  assert.equal(formatStrategyGroupStrategy(""), "未设置");
  assert.equal(formatStrategyGroupStrategy(undefined), "未设置");
});

test("formatStrategyGroupApplySummary reports counts and the kept member", () => {
  assert.equal(
    formatStrategyGroupApplySummary({
      groupName: "角色包",
      strategy: "single",
      enabled: ["b.vpk"],
      disabled: ["a.vpk", "c.vpk"],
      pickedName: "b.vpk",
    }),
    "已应用策略组“角色包”（互斥单选）：开启 1 项，关闭 2 项，保留 b.vpk",
  );
});

test("formatStrategyGroupApplySummary tolerates missing fields", () => {
  assert.equal(
    formatStrategyGroupApplySummary(null),
    "已应用策略组（未设置）：开启 0 项，关闭 0 项",
  );
  assert.equal(
    formatStrategyGroupApplySummary({ groupName: "全部包", strategy: "all", enabled: [{}, {}] }),
    "已应用策略组“全部包”（全部开启）：开启 2 项，关闭 0 项",
  );
});

const batchGroups = [
  { id: "g1", name: "死库水校服 全套", members: [{ key: "a" }, { key: "b" }, { key: "c" }, { key: "d" }] },
  { id: "g2", name: "「激光剑」系列", members: [{ key: "e" }, { key: "f" }] },
  { id: "g3", name: "未被选中的组", members: [{ key: "g" }] },
];

test("summarizeStrategyGroupSelection 汇总组数/成员数/缺失数", () => {
  const missing = new Map([["g2", 1]]);
  assert.deepEqual(summarizeStrategyGroupSelection(["g1", "g2"], batchGroups, missing), {
    count: 2,
    memberCount: 6,
    missingCount: 1,
    names: ["死库水校服 全套", "「激光剑」系列"],
  });
  // 不存在的 ID 不计入，未知分组列表也不炸
  assert.deepEqual(summarizeStrategyGroupSelection(["nope"], batchGroups), {
    count: 0,
    memberCount: 0,
    missingCount: 0,
    names: [],
  });
  assert.equal(summarizeStrategyGroupSelection(null, null).count, 0);
});

test("formatStrategyGroupSelectionLabel 说明已选组与成员数", () => {
  assert.equal(formatStrategyGroupSelectionLabel({ count: 0 }), "未选择组");
  assert.equal(
    formatStrategyGroupSelectionLabel({ count: 2, memberCount: 6, missingCount: 1 }),
    "已选 2 个组（6 个成员，含 1 个缺失）",
  );
  assert.equal(
    formatStrategyGroupSelectionLabel({ count: 1, memberCount: 2, missingCount: 0 }),
    "已选 1 个组（2 个成员）",
  );
});

test("formatStrategyGroupBatchConfirm 覆盖删除与其它批量动作", () => {
  const summary = summarizeStrategyGroupSelection(["g1", "g2"], batchGroups);
  const del = formatStrategyGroupBatchConfirm("delete", summary);
  assert.match(del, /删除选中的 2 个策略组（共 6 个成员）/);
  assert.match(del, /不改动 addonlist\.txt/);
  assert.match(del, /下级分组会回到顶层/);
  assert.match(del, /死库水校服 全套、「激光剑」系列/);
  assert.match(formatStrategyGroupBatchConfirm("enforce_on", summary), /开启自动联动/);
  assert.match(formatStrategyGroupBatchConfirm("set_tier", summary), /同一个组权重/);
});

test("formatStrategyGroupBatchResult 报告删除/权重/跳过与剩余", () => {
  assert.equal(
    formatStrategyGroupBatchResult("delete", { deleted: ["a", "b"], remaining: 7 }),
    "已删除 2 个策略组（现在还剩 7 个组）",
  );
  assert.equal(
    formatStrategyGroupBatchResult("set_tier", { updated: ["a"], tier: -5, remaining: 9 }),
    "已给 1 个策略组设置权重 -5（需「按分层应用」才重排）",
  );
  assert.equal(
    formatStrategyGroupBatchResult("delete", {
      deleted: ["a"],
      skipped: ["x"],
      detachedChildren: ["y"],
      remaining: 3,
    }),
    "已删除 1 个策略组；1 个已经不存在，已跳过；1 个下级分组已回到顶层（现在还剩 3 个组）",
  );
  assert.equal(formatStrategyGroupBatchResult("clear_tier", { updated: [], remaining: 0 }), "已清除 0 个策略组的权重");
});

test("formatStrategyGroupFilterState 说明管理窗口的当前筛选", () => {
  const groups = [
    { id: "g1", name: "医疗箱 系列" },
    { id: "g2", name: "烟火零食 系列" },
    { id: "g3", name: "街机 系列" },
    { id: "g4", name: "Imikumachine" },
  ];
  assert.equal(formatStrategyGroupFilterState([], groups), "当前筛选：全部分组");
  assert.equal(formatStrategyGroupFilterState(["g1"], groups), "当前筛选：医疗箱 系列");
  assert.equal(
    formatStrategyGroupFilterState(["g1", "g2", "g3"], groups),
    "当前筛选：医疗箱 系列、烟火零食 系列、街机 系列",
  );
  assert.equal(
    formatStrategyGroupFilterState(["g1", "g2", "g3", "g4"], groups),
    "当前筛选：医疗箱 系列、烟火零食 系列、街机 系列 等 4 个组",
  );
  // 组已经被删掉时退回显示 id，不能显示空白。
  assert.equal(formatStrategyGroupFilterState(["gone"], groups), "当前筛选：gone");
});
