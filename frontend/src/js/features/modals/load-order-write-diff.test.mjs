import assert from "node:assert/strict";
import test from "node:test";

import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import {
  buildRestoreConfirmHtml,
  buildWriteDiffHtml,
  hasWriteDiffChanges,
  summarizeWriteDiff,
} from "./load-order-write-diff.mjs";

// 文案已 i18n 化（第 13 项第十九批·modals 家族）：安装真实目录后继续断言中文。
installTestCatalog();

const fullDiff = {
  countBefore: 4,
  countAfter: 3,
  removed: ["ghost.vpk"],
  added: ["new.vpk"],
  movedTotal: 2,
  moved: [
    { key: "b.vpk", oldOrder: 3, newOrder: 1 },
    { key: "workshop\\123.vpk", oldOrder: 2, newOrder: 3 },
  ],
  unchanged: 1,
  duplicatesFolded: 1,
};

test("hasWriteDiffChanges：没有变化时为 false，任何一类变化都为 true", () => {
  assert.equal(hasWriteDiffChanges(null), false);
  assert.equal(hasWriteDiffChanges(undefined), false);
  assert.equal(hasWriteDiffChanges({}), false);
  assert.equal(hasWriteDiffChanges({ movedTotal: 0, removed: [], added: [], duplicatesFolded: 0 }), false);
  assert.equal(hasWriteDiffChanges({ removed: ["a.vpk"] }), true);
  assert.equal(hasWriteDiffChanges({ added: ["a.vpk"] }), true);
  assert.equal(hasWriteDiffChanges({ movedTotal: 2 }), true);
  assert.equal(hasWriteDiffChanges({ duplicatesFolded: 1 }), true);
});

test("summarizeWriteDiff：摘要覆盖条目数与各维度，空差异有明确说明", () => {
  const lines = summarizeWriteDiff(fullDiff);
  assert.equal(lines[0], "条目数：4 → 3");
  assert.ok(lines.some((line) => line.includes("重排 2 条")));
  assert.ok(lines.some((line) => line === "移除 1 条"));
  assert.ok(lines.some((line) => line === "新增 1 条"));
  assert.ok(lines.some((line) => line.includes("合并重复条目 1 条")));

  assert.deepEqual(summarizeWriteDiff(null), []);
  assert.deepEqual(summarizeWriteDiff({ countBefore: 3, countAfter: 3 }), ["条目数：3 → 3", "没有需要写入的变化"]);
});

test("buildWriteDiffHtml：明细、位置与转义", () => {
  const html = buildWriteDiffHtml(fullDiff);
  assert.ok(html.includes("ghost.vpk"), "移除明细要列出条目");
  assert.ok(html.includes("b.vpk"), "重排明细要列出条目");
  assert.ok(html.includes("第 3 位 → 第 1 位"), "重排位置格式");
  assert.ok(html.includes("write-diff-note"), "要带提醒文案");

  // 键名里的尖括号必须被转义（列表与弹窗都渲染这些字符串）。
  const evil = buildWriteDiffHtml({ countBefore: 1, countAfter: 1, removed: ["<img src=x onerror=alert(1)>.vpk"] });
  assert.ok(!evil.includes("<img"), "必须转义");
  assert.ok(evil.includes("&lt;img"), "应保留转义后的可见文本");
});

test("buildWriteDiffHtml：超过 20 条截断并给出剩余数量", () => {
  const removed = Array.from({ length: 25 }, (_, index) => `mod-${index}.vpk`);
  const html = buildWriteDiffHtml({ countBefore: 25, countAfter: 0, removed });
  assert.ok(html.includes("mod-19.vpk"), "前 20 条要显示");
  assert.ok(!html.includes("mod-20.vpk"), "第 21 条起不显示");
  assert.ok(html.includes("还有 5 条"), "要给出剩余数量");
});

test("buildRestoreConfirmHtml：区分首次编辑备份与受管备份", () => {
  const firstEdit = buildRestoreConfirmHtml({
    found: true,
    source: "first-edit",
    kind: "first-edit",
    createdAt: "2026-10-01T10:00:00+08:00",
    diff: fullDiff,
  });
  assert.ok(firstEdit.includes("首次编辑备份"), "要说明来源");
  assert.ok(firstEdit.includes("before-restore"), "要说明当前内容会留档");

  const managed = buildRestoreConfirmHtml({
    found: true,
    source: "managed",
    name: "manual-20261011T120000.000000000+08.txt",
    kind: "manual",
    createdAt: "2026-10-11T12:00:00+08:00",
    diff: { countBefore: 1, countAfter: 1 },
  });
  assert.ok(managed.includes("manual-20261011T120000.000000000+08.txt"), "要带上备份名");
  assert.ok(managed.includes("类型：manual"), "要带上类型");

  assert.equal(buildRestoreConfirmHtml(null), "还没有可恢复的备份。");
});
