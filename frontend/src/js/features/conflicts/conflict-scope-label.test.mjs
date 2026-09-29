import assert from "node:assert/strict";
import test from "node:test";

import {
  conflictScopeRuleLabel,
  conflictScopeSummaryLabel,
  describeConflictScope,
  formatConflictScopeText,
  normalizeConflictScopeRules,
  serializeConflictScopeOptions,
} from "./conflict-scope-label.mjs";

test("单条件时不再写“满足任一 / 同时满足”前缀", () => {
  const described = describeConflictScope({
    baselineRules: [{ type: "enabled" }],
    matchMode: "and",
  });
  assert.equal(described.label, "游戏内开启");
  assert.equal(described.usesCombination, false);
});

test("多条件时按组合方式给出全文案", () => {
  const rules = [{ type: "enabled" }, { type: "workshop" }];
  assert.deepEqual(describeConflictScope({ baselineRules: rules, matchMode: "and" }), {
    label: "同时满足：游戏内开启 + 创意工坊",
    usesCombination: true,
    ruleLabels: ["游戏内开启", "创意工坊"],
    matchMode: "and",
  });
  assert.equal(
    describeConflictScope({ baselineRules: rules, matchMode: "or" }).label,
    "满足任一：游戏内开启 / 创意工坊",
  );
  // 非法/缺省 matchMode 一律按“满足任一”处理（与后端 normalize 一致）。
  assert.equal(
    describeConflictScope({ baselineRules: rules, matchMode: "xor" }).label,
    "满足任一：游戏内开启 / 创意工坊",
  );
});

test("空规则回落到默认的“游戏内开启”", () => {
  assert.deepEqual(normalizeConflictScopeRules([]), [{ type: "enabled", value: "" }]);
  assert.deepEqual(normalizeConflictScopeRules(null), [{ type: "enabled", value: "" }]);
  assert.equal(describeConflictScope({}).label, "游戏内开启");
  assert.equal(describeConflictScope({ baselineRules: [{ type: "  " }] }).label, "游戏内开启");
});

test("标签条件要带出所选标签值", () => {
  assert.equal(conflictScopeRuleLabel({ type: "tag", value: "武器" }), "标签：武器");
  assert.equal(conflictScopeRuleLabel({ type: "tag", value: "" }), "标签：未指定");
  assert.equal(
    describeConflictScope({ baselineRules: [{ type: "root" }, { type: "tag", value: "武器" }], matchMode: "or" })
      .label,
    "满足任一：根目录 / 标签：武器",
  );
});

test("formatConflictScopeText 支持“当前 / 将按”前缀与覆盖判定后缀", () => {
  const options = {
    baselineRules: [{ type: "enabled" }, { type: "workshop" }],
    matchMode: "and",
    priorityAware: true,
  };
  assert.equal(
    conflictScopeSummaryLabel(options),
    "同时满足：游戏内开启 + 创意工坊 · 按加载顺序判定覆盖",
  );
  assert.equal(
    formatConflictScopeText(options, "当前"),
    "当前：同时满足：游戏内开启 + 创意工坊 · 按加载顺序判定覆盖",
  );
  assert.equal(
    formatConflictScopeText(options, "将按"),
    "将按：同时满足：游戏内开启 + 创意工坊 · 按加载顺序判定覆盖",
  );
  assert.equal(
    formatConflictScopeText({ baselineRules: [{ type: "enabled" }], matchMode: "or" }, "将按"),
    "将按：游戏内开启",
  );
});

test("serializeConflictScopeOptions 能识别“改了但没应用”", () => {
  const saved = {
    baselineRules: [{ type: "enabled" }],
    matchMode: "or",
    priorityAware: false,
  };
  const same = {
    baselineRules: [{ type: "enabled", value: "" }],
    matchMode: "or",
    priorityAware: false,
  };
  const otherRule = {
    baselineRules: [{ type: "enabled" }, { type: "workshop" }],
    matchMode: "or",
    priorityAware: false,
  };
  const otherMode = { ...saved, matchMode: "and" };
  const otherPriority = { ...saved, priorityAware: true };
  assert.equal(serializeConflictScopeOptions(saved), serializeConflictScopeOptions(same));
  assert.notEqual(serializeConflictScopeOptions(saved), serializeConflictScopeOptions(otherRule));
  assert.notEqual(serializeConflictScopeOptions(saved), serializeConflictScopeOptions(otherMode));
  assert.notEqual(serializeConflictScopeOptions(saved), serializeConflictScopeOptions(otherPriority));
  // 规则顺序不影响结果（后端同样不区分顺序）。
  assert.equal(
    serializeConflictScopeOptions({ baselineRules: [{ type: "root" }, { type: "workshop" }], matchMode: "or" }),
    serializeConflictScopeOptions({ baselineRules: [{ type: "workshop" }, { type: "root" }], matchMode: "or" }),
  );
});
