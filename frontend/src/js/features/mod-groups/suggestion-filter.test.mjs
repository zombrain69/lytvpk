import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_SUGGESTION_FILTER,
  SUGGESTION_PAGE_SIZE,
  applySuggestionView,
  countTagCoveredSuggestions,
  filterSuggestions,
  formatSuggestionViewSummary,
  normalizeSuggestionQuery,
  paginateSuggestions,
  parseStoredSuggestionModalSize,
  suggestionMatchesTagCoverage,
  sortSuggestions,
  suggestionMatchesQuery,
  suggestionPreviewMembers,
} from "./suggestion-filter.mjs";

const suggestions = [
  {
    id: "s1",
    label: "Chiffon 下午茶-甘回 全副模块",
    confidence: "high",
    score: 120,
    source: "external",
    reason: "同一主题的必备材质与道具模型，整套启用",
    signals: ["外部建议", "同一 mod 的配套模块"],
    memberKeys: ["a.vpk", "b.vpk", "c.vpk", "d.vpk"],
    memberNames: ["甘回-必备材质.vpk", "甘回-型号-低模.vpk", "甘回-型号-加特林.vpk", "甘回-型号-可乐.vpk"],
  },
  {
    id: "s2",
    label: "M16 武器",
    confidence: "medium",
    score: 70,
    source: "",
    reason: "主体识别：M16",
    signals: ["主体识别"],
    memberKeys: ["m16a.vpk", "m16b.vpk"],
    memberNames: ["M16A.vpk", "M16B.vpk"],
  },
  {
    id: "s3",
    label: "武士刀集合",
    confidence: "low",
    score: 45,
    source: "",
    reason: "文件名前缀：武士刀",
    signals: ["文件名前缀"],
    memberKeys: ["katana1.vpk", "katana2.vpk", "katana3.vpk"],
    memberNames: ["武士刀1.vpk", "武士刀2.vpk", "武士刀3.vpk"],
  },
];

test("normalizeSuggestionQuery 去掉首尾空白并转小写", () => {
  assert.equal(normalizeSuggestionQuery("  Chiffon  "), "chiffon");
  assert.equal(normalizeSuggestionQuery(null), "");
});

test("suggestionMatchesQuery 命中标题、成员名、成员键与理由", () => {
  assert.ok(suggestionMatchesQuery(suggestions[0], "chiffon"));
  assert.ok(suggestionMatchesQuery(suggestions[0], "加特林"));
  assert.ok(suggestionMatchesQuery(suggestions[0], "c.vpk"));
  assert.ok(suggestionMatchesQuery(suggestions[1], "主体识别"));
  assert.ok(!suggestionMatchesQuery(suggestions[1], "武士刀"));
  // 空查询不过滤
  assert.ok(suggestionMatchesQuery(suggestions[1], ""));
});

test("filterSuggestions 支持置信度与来源过滤", () => {
  assert.deepEqual(
    filterSuggestions(suggestions, { confidence: "high" }).map((item) => item.id),
    ["s1"],
  );
  assert.deepEqual(
    filterSuggestions(suggestions, { source: "external" }).map((item) => item.id),
    ["s1"],
  );
  assert.deepEqual(
    filterSuggestions(suggestions, { source: "builtin" }).map((item) => item.id),
    ["s2", "s3"],
  );
  assert.deepEqual(
    filterSuggestions(suggestions, { confidence: "medium", query: "m16" }).map((item) => item.id),
    ["s2"],
  );
  assert.equal(filterSuggestions(suggestions, {}).length, 3);
});

test("sortSuggestions 的置信度优先模式把高置信度排在前面", () => {
  const shuffled = [suggestions[2], suggestions[1], suggestions[0]];
  assert.deepEqual(
    sortSuggestions(shuffled, "confidence").map((item) => item.id),
    ["s1", "s2", "s3"],
  );
  // 同置信度时按分数降序
  const sameLevel = [
    { id: "low-score", confidence: "high", score: 60, memberKeys: ["a.vpk"] },
    { id: "high-score", confidence: "high", score: 120, memberKeys: ["a.vpk"] },
  ];
  assert.deepEqual(
    sortSuggestions(sameLevel, "confidence").map((item) => item.id),
    ["high-score", "low-score"],
  );
  // 推荐顺序（recommended）保持引擎给的顺序
  assert.deepEqual(
    sortSuggestions(shuffled, "recommended").map((item) => item.id),
    ["s3", "s2", "s1"],
  );
});

test("sortSuggestions 支持按成员数量排序", () => {
  assert.deepEqual(
    sortSuggestions(suggestions, "members").map((item) => item.id),
    ["s1", "s3", "s2"],
  );
});

test("applySuggestionView 先过滤再排序，且不改动入参", () => {
  const input = [suggestions[2], suggestions[0], suggestions[1]];
  const snapshot = input.map((item) => item.id);
  const view = applySuggestionView(input, { confidence: "high", sort: "members" });
  assert.deepEqual(view.map((item) => item.id), ["s1"]);
  assert.deepEqual(input.map((item) => item.id), snapshot);
  assert.deepEqual(DEFAULT_SUGGESTION_FILTER, {
    query: "",
    confidence: "",
    source: "",
    tagCovered: "hide",
    sort: "recommended",
  });
});

test("suggestionPreviewMembers 给出折叠卡片的成员预览", () => {
  const preview = suggestionPreviewMembers(suggestions[0], 3);
  assert.deepEqual(preview.names, [
    "甘回-必备材质.vpk",
    "甘回-型号-低模.vpk",
    "甘回-型号-加特林.vpk",
  ]);
  assert.equal(preview.more, 1);
  assert.equal(preview.total, 4);
  // 成员名缺失时回退到键
  const fallback = suggestionPreviewMembers({ memberKeys: ["x.vpk"], memberNames: [] }, 3);
  assert.deepEqual(fallback.names, ["x.vpk"]);
  assert.equal(fallback.more, 0);
});

test("paginateSuggestions 分页并报告剩余数量", () => {
  const many = Array.from({ length: 30 }, (_, index) => ({ id: `s${index}` }));
  const first = paginateSuggestions(many, SUGGESTION_PAGE_SIZE);
  assert.equal(first.items.length, SUGGESTION_PAGE_SIZE);
  assert.equal(first.hasMore, true);
  assert.equal(first.hiddenCount, 30 - SUGGESTION_PAGE_SIZE);
  const all = paginateSuggestions(many, 100);
  assert.equal(all.items.length, 30);
  assert.equal(all.hasMore, false);
  assert.equal(all.hiddenCount, 0);
});

test("formatSuggestionViewSummary 同时报告筛选结果与总量", () => {
  assert.equal(
    formatSuggestionViewSummary({ total: 234, shown: 24, hidden: 210 }),
    "显示 24 / 234 条建议（还有 210 条未显示）",
  );
  assert.equal(
    formatSuggestionViewSummary({ total: 3, shown: 3, hidden: 0 }),
    "显示 3 / 3 条建议",
  );
  assert.equal(
    formatSuggestionViewSummary({ total: 0, shown: 0, hidden: 0 }),
    "没有符合筛选条件的建议",
  );
});

test("parseStoredSuggestionModalSize 只接受合理尺寸", () => {
  assert.deepEqual(parseStoredSuggestionModalSize('{"width":1200,"height":800}'), {
    width: 1200,
    height: 800,
  });
  assert.equal(parseStoredSuggestionModalSize("{bad json"), null);
  assert.equal(parseStoredSuggestionModalSize('{"width":0,"height":800}'), null);
  assert.equal(parseStoredSuggestionModalSize('{"width":1200}'), null);
  assert.equal(parseStoredSuggestionModalSize(null), null);
});

const taggedSuggestions = [
  { id: "t1", label: "SG552 武器", confidence: "high", tagKey: "sg552", tagScope: "exact", memberKeys: ["a.vpk", "b.vpk"] },
  { id: "t2", label: "AK47 武器", confidence: "high", tagKey: "ak47", tagScope: "partial", memberKeys: ["c.vpk", "d.vpk"] },
  { id: "t3", label: "无标签组", confidence: "low", memberKeys: ["e.vpk", "f.vpk"] },
];

test("suggestionMatchesTagCoverage 区分 exact / partial / 无标签", () => {
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[0], "hide"), false);
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[0], "only"), true);
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[0], "show"), true);
  // 部分覆盖不算"标签已覆盖"：标签筛出来的是更大的集合。
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[1], "only"), false);
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[2], "hide"), true);
  assert.equal(suggestionMatchesTagCoverage(taggedSuggestions[2], "only"), false);
});

test("filterSuggestions 默认隐藏标签已覆盖的建议，可切换为显示或只看", () => {
  assert.deepEqual(
    filterSuggestions(taggedSuggestions, { tagCovered: "hide" }).map((item) => item.id),
    ["t2", "t3"],
  );
  assert.deepEqual(
    filterSuggestions(taggedSuggestions, { tagCovered: "show" }).map((item) => item.id),
    ["t1", "t2", "t3"],
  );
  assert.deepEqual(
    filterSuggestions(taggedSuggestions, { tagCovered: "only" }).map((item) => item.id),
    ["t1"],
  );
  // 默认值就是"隐藏"
  assert.deepEqual(
    filterSuggestions(taggedSuggestions, {}).map((item) => item.id),
    ["t2", "t3"],
  );
  assert.equal(DEFAULT_SUGGESTION_FILTER.tagCovered, "hide");
});

test("countTagCoveredSuggestions 统计标签已覆盖的条数", () => {
  assert.equal(countTagCoveredSuggestions(taggedSuggestions), 1);
  assert.equal(countTagCoveredSuggestions([]), 0);
  assert.equal(countTagCoveredSuggestions(null), 0);
});
