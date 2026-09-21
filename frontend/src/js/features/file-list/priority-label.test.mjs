import assert from "node:assert/strict";
import test from "node:test";

import {
  buildPriorityPlanMap,
  describePrioritySource,
  formatEffectiveLayer,
  formatPriorityLabel,
  normalizePriorityTier,
} from "./priority-label.mjs";

test("formatPriorityLabel renders order with an explicit layer", () => {
  assert.equal(
    formatPriorityLabel({ order: 3, tier: 2, effective: 2, source: "tier" }),
    "优先级 #3（分层 2）",
  );
  assert.equal(
    formatPriorityLabel({ order: 1, tier: null, effective: 1, source: "order" }),
    "优先级 #1",
  );
  assert.equal(formatPriorityLabel({ order: 0, known: false, tier: -5, effective: -5 }), "未写入 addonlist（分层 -5）");
  assert.equal(formatPriorityLabel(null), "");
});

test("formatPriorityLabel falls back to the load order when no layer is set", () => {
  assert.equal(formatPriorityLabel({ order: 7 }), "优先级 #7");
  assert.equal(formatPriorityLabel({ order: 7, layer: 7 }), "优先级 #7");
});

test("formatEffectiveLayer explains group derived layers", () => {
  assert.equal(
    formatEffectiveLayer({ order: 4, tier: null, groupTier: 1, effective: 1, source: "group" }),
    "有效分层 1（来自策略组权重，顺序号 #4）",
  );
  assert.equal(
    formatEffectiveLayer({ order: 4, tier: 9, groupTier: null, effective: 9, source: "tier" }),
    "有效分层 9（来自显式分层）",
  );
  assert.equal(formatEffectiveLayer({ order: 4, tier: null, groupTier: null, effective: 4, source: "order" }), "");
});

test("describePrioritySource maps known sources and falls back", () => {
  assert.equal(describePrioritySource("tier"), "显式分层");
  assert.equal(describePrioritySource("group"), "策略组权重");
  assert.equal(describePrioritySource("order"), "addonlist 顺序号");
  assert.equal(describePrioritySource("unknown"), "未设置分层");
  assert.equal(describePrioritySource(undefined), "未设置分层");
});

test("normalizePriorityTier accepts integers only", () => {
  assert.equal(normalizePriorityTier(" 12 "), 12);
  assert.equal(normalizePriorityTier("-3"), -3);
  assert.equal(normalizePriorityTier(""), null);
  assert.equal(normalizePriorityTier("2.5"), null);
  assert.equal(normalizePriorityTier("abc"), null);
  assert.equal(normalizePriorityTier(0), 0);
});

test("buildPriorityPlanMap keys entries by normalized key", () => {
  const map = buildPriorityPlanMap([
    { key: "A.VPK", name: "a.vpk", order: 1, effective: 1 },
    { key: "workshop|123.vpk", order: 2, effective: 2 },
  ]);
  assert.equal(map.size, 2);
  assert.equal(map.get("a.vpk").order, 1);
  assert.equal(map.get("workshop|123.vpk").effective, 2);
  assert.equal(buildPriorityPlanMap(null).size, 0);
});
