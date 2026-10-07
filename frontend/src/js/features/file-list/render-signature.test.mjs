// 卡片渲染签名的静态契约测试。
//
// 背景（真实回归）：卡片会被复用，只有签名变化时才重绘。签名注释写着
// "contains every value rendered inside a card"，但后加的
// 「优先级分层角标」与「变更驱动复检角标」没有进签名，
// 于是保存/清除分层后卡片角标停留在旧值，直到重新扫描才更新。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./render.js", import.meta.url), "utf8");

function functionBody(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `应存在函数 ${name}`);
  const braceStart = source.indexOf("{", start);
  let depth = 0;
  let index = braceStart;
  for (; index < source.length; index += 1) {
    const char = source[index];
    if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  return source.slice(braceStart, index + 1);
}

test("卡片签名覆盖分层角标的值", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(
    body.includes("getFilePriorityEntry"),
    "签名必须包含分层信息，否则设置/清除分层后角标不会重绘",
  );
  assert.ok(body.includes("priority:"), "签名应显式记录 priority 字段");
});

test("卡片签名覆盖变更驱动复检角标的值", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(
    body.includes("findConflictRecheckBadge"),
    "签名必须包含复检角标，否则自动复检后角标不会重绘",
  );
  assert.ok(body.includes("conflictRecheck:"), "签名应显式记录 conflictRecheck 字段");
});

// 真实回归：对比范围分析结果里新增了“覆盖关系”（override_groups）之后，
// 卡片签名只记录冲突组数，于是「0 冲突 + 9 处覆盖」与「什么都没有」签名相同，
// 复用的卡片会一直停在“无冲突”。
test("卡片签名覆盖对比范围角标的值", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(
    body.includes("getScopedConflictBadgeModel"),
    "签名必须包含对比范围角标，否则冲突/覆盖数量变化后角标不会重绘",
  );
  assert.ok(body.includes("conflict:"), "签名应显式记录 conflict 字段");
});

test("对比范围角标的文案同时反映冲突与覆盖", () => {
  const body = functionBody("getConflictSummaryBadge");
  assert.ok(
    body.includes("getScopedConflictBadgeModel"),
    "角标必须走统一的模型函数（它同时统计冲突组与覆盖组）",
  );
  const model = functionBody("getScopedConflictBadgeModel");
  assert.ok(
    model.includes("formatScopedConflictLabel"),
    "角标文案必须来自 scoped-conflict-summary，避免再把已判定的覆盖写成“无冲突”",
  );
  assert.ok(
    model.includes("matchesConflictBaseline"),
    "需要区分“没有重叠”和“不满足对比范围、根本没参与分析”",
  );
});

// XDR 动作生效角标（官方规则：同角色同槽只会随机生效一个）同样是"渲染出来的值"，
// 漏进签名会让卡片复用后角标停在旧状态 —— 切开关、装/卸动作 Mod 后看不出来。
test("卡片签名覆盖 XDR 动作生效角标", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(body.includes("xdrPriority"), "签名必须包含 XDR 动作生效结论");
  assert.ok(body.includes("randomSlots"), "签名要记录同槽（随机生效）槽位数量");

  const badge = functionBody("buildXDRPriorityBadge");
  assert.ok(badge.includes("xdr-priority-badge"), "角标要有独立 class，方便样式与检索");
  for (const state of ["is-active", "is-random", "is-partial"]) {
    assert.ok(badge.includes(state), `角标必须覆盖状态 ${state}`);
  }
  assert.ok(
    badge.includes("同角色同槽只会随机生效一个"),
    "随机生效的说明必须写清官方规则，并点明与加载顺序无关",
  );

  const card = functionBody("createFileCard");
  assert.ok(card.includes("buildXDRPriorityBadge(file)"), "卡片要渲染 XDR 动作生效角标");
});
