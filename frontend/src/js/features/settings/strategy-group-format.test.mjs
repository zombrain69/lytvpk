import assert from "node:assert/strict";
import { test } from "node:test";

import {
  formatStrategyGroupApplySummary,
  formatStrategyGroupEnforcementNotice,
  formatStrategyGroupStrategy,
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
