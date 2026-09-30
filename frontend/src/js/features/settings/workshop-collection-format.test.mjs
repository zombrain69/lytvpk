import assert from "node:assert/strict";
import test from "node:test";

import {
  countMissingCollectionMembers,
  formatCollectionCheckAllSummary,
  formatCollectionQueueSummary,
  formatCollectionRefreshSummary,
  formatCollectionSummary,
} from "./workshop-collection-format.mjs";

test("formatCollectionSummary reports downloads and truncation", () => {
  assert.equal(
    formatCollectionSummary({ members: [{ present: true }, { present: false }] }),
    "2 个成员，其中 1 个未下载",
  );
  assert.equal(
    formatCollectionSummary({ members: [{ present: true }], childCollectionsTruncated: true }),
    "1 个成员，已全部下载（子合集过多，已截断）",
  );
  assert.equal(formatCollectionSummary({ members: [] }), "没有可下载成员");
  assert.equal(formatCollectionSummary(null), "");
  assert.equal(countMissingCollectionMembers({ members: [{ present: false }, {}] }), 2);
});

test("formatCollectionRefreshSummary describes node changes", () => {
  assert.equal(
    formatCollectionRefreshSummary({
      addedCount: 1,
      removedCount: 1,
      missingCount: 2,
      addedTitles: ["新物品"],
      removedTitles: ["旧物品"],
    }),
    "新增 1 个（新物品…）；下架 1 个（旧物品…）；本地缺少 2 个",
  );
  assert.equal(formatCollectionRefreshSummary({ addedCount: 0, removedCount: 0, missingCount: 0 }), "合集内容没有变化");
  assert.equal(formatCollectionRefreshSummary(null), "");
});

test("formatCollectionQueueSummary reports queued members", () => {
  assert.equal(formatCollectionQueueSummary(["1", "2"]), "已加入下载队列 2 个成员");
  assert.equal(formatCollectionQueueSummary([]), "没有需要下载的成员");
  assert.equal(formatCollectionQueueSummary(null), "没有需要下载的成员");
});

test("formatCollectionCheckAllSummary 汇总变化与失败原因", () => {
  assert.equal(formatCollectionCheckAllSummary([]), "还没有保存过工坊合集");
  assert.equal(
    formatCollectionCheckAllSummary([
      { collectionId: "1", addedCount: 0, removedCount: 0, missingCount: 0 },
      { collectionId: "2", addedCount: 0, removedCount: 0, missingCount: 3 },
    ]),
    "所有合集都没有成员变化",
  );
  assert.equal(
    formatCollectionCheckAllSummary([
      { collectionId: "1", title: "好合集", addedCount: 0, removedCount: 2, removedTitles: ["旧物"] },
      { collectionId: "9", title: "坏合集", error: "这个合集在工坊里已经不存在了（工坊返回 result=9）" },
    ]),
    "好合集：下架 2 个（旧物…）；1 个检查失败：坏合集（这个合集在工坊里已经不存在了（工坊返回 result=9））",
  );
  assert.equal(
    formatCollectionCheckAllSummary([
      { collectionId: "9", title: "坏合集", error: "工坊接口返回 HTTP 500" },
      { collectionId: "8", title: "也坏", error: "网络不可用" },
    ]),
    "全部 2 个合集都检查失败；2 个检查失败：坏合集（工坊接口返回 HTTP 500）",
  );
});
