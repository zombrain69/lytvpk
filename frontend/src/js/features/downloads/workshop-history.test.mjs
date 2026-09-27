import assert from "node:assert/strict";
import test from "node:test";

import {
  buildHistoryEntries,
  formatHistoryLabel,
  formatHistoryTime,
  normalizeHistoryItems,
} from "./workshop-history.mjs";

test("normalizeHistoryItems 过滤脏数据并收敛字段类型", () => {
  const items = normalizeHistoryItems([
    { rootId: " 111 ", title: "物品一", fileType: "2", parsedAt: "1700000000000", group: { root_id: "111" } },
    { rootId: "   ", title: "空 ID 应被丢弃" },
    { rootId: 222, title: "非字符串 ID 应被丢弃" },
    null,
  ]);

  assert.equal(items.length, 1);
  assert.equal(items[0].rootId, "111");
  assert.equal(items[0].fileType, 2);
  assert.equal(items[0].parsedAt, 1700000000000);
  assert.deepEqual(items[0].group, { root_id: "111" });

  assert.deepEqual(normalizeHistoryItems(null), []);
  assert.deepEqual(normalizeHistoryItems("nope"), []);
});

test("buildHistoryEntries 从解析结果生成条目（带子项的按合集记录）", () => {
  const entries = buildHistoryEntries([
    { root_id: "100", main: { publishedfileid: "100", title: "合集" }, items: [{}, {}] },
    { root_id: "200", main: { publishedfileid: "200", title: "单件" }, items: [] },
    { root_id: "  ", main: { title: "没有 ID" }, items: [] },
    { main: { publishedfileid: "300", title: "用 main 的 ID 兜底" }, items: [] },
  ]);

  assert.deepEqual(
    entries.map((entry) => [entry.rootId, entry.fileType, entry.title]),
    [
      ["100", 2, "合集"],
      ["200", 0, "单件"],
      ["300", 0, "用 main 的 ID 兜底"],
    ],
  );
  assert.deepEqual(buildHistoryEntries(undefined), []);
});

test("formatHistoryLabel 优先标题、退化到工坊 ID", () => {
  assert.equal(formatHistoryLabel({ rootId: "1", title: "我的 Mod" }), "我的 Mod");
  assert.equal(formatHistoryLabel({ rootId: "1", title: "   " }), "工坊 #1");
  assert.equal(formatHistoryLabel({ rootId: " 42 " }), "工坊 #42");
  assert.equal(formatHistoryLabel(null), "工坊 #");
});

test("formatHistoryTime 只在时间戳合理时显示", () => {
  const now = Date.UTC(2026, 8, 27, 12, 0, 0);
  assert.match(formatHistoryTime(now - 3600_000, now), /^\d{2}-\d{2} \d{2}:\d{2}$/);
  assert.equal(formatHistoryTime(0, now), "");
  assert.equal(formatHistoryTime(now + 10 * 60_000, now), "", "未来时间不显示");
  assert.equal(formatHistoryTime(Number.NaN, now), "");
});
