import assert from "node:assert/strict";
import { test } from "node:test";

import { formatProfileApplySummary } from "./profile-format.mjs";

test("formatProfileApplySummary reports switch counts", () => {
  assert.equal(
    formatProfileApplySummary({
      profileName: "写实包",
      appliedCount: 2,
      addedCount: 1,
      keptCount: 1,
    }),
    "已应用方案“写实包”：2 项已存在，1 项补回，1 项保留",
  );
});

test("formatProfileApplySummary appends automation counts when restored", () => {
  assert.equal(
    formatProfileApplySummary({
      profileName: "整套方案",
      appliedCount: 0,
      addedCount: 0,
      keptCount: 0,
      restoredGroups: 2,
      restoredDependencies: 1,
    }),
    "已应用方案“整套方案”：0 项已存在，0 项补回，0 项保留，恢复 2 个策略组，恢复 1 条依赖",
  );
});

test("formatProfileApplySummary tolerates missing fields", () => {
  assert.equal(
    formatProfileApplySummary(null),
    "已应用方案：0 项已存在，0 项补回，0 项保留",
  );
});
