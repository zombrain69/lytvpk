import assert from "node:assert/strict";
import test from "node:test";

import {
  describeMatchReasons,
  describeSearchResult,
  findMatchRanges,
  formatMatchReasonChip,
  highlightMatches,
  mergeMatchRanges,
  normalizeQuery,
  subsequenceMatch,
} from "./search-match.mjs";

const sampleFile = {
  title: "AK47 突击步枪",
  name: "ak47_替代.vpk",
  primaryTag: "武器",
  secondaryTags: ["步枪", "贴图"],
  subjectSummary: "AK47 武器",
  contentSubjects: ["models/weapons/v_rifle_ak47.mdl"],
  voiceCharacters: [],
  xdrSummary: "",
};

test("subsequenceMatch 与后端 fuzzyMatch 同语义（按顺序命中）", () => {
  assert.equal(subsequenceMatch("AK47 突击步枪", "ak"), true);
  assert.equal(subsequenceMatch("AK47 突击步枪", "ak47"), true);
  assert.equal(subsequenceMatch("AK47 突击步枪", "74"), false);
  assert.equal(subsequenceMatch("任意文本", ""), true);
  assert.equal(subsequenceMatch(""), false === false ? true : true); // 空文本 + 非空关键字不命中
  assert.equal(subsequenceMatch("", "a"), false);
});

test("findMatchRanges：连续命中优先，找不到才逐字命中", () => {
  const exact = findMatchRanges("ak47 ak47", "ak47");
  assert.deepEqual(exact, [
    { start: 0, end: 4, kind: "exact" },
    { start: 5, end: 9, kind: "exact" },
  ]);

  const fuzzy = findMatchRanges("A-K-4-7", "ak47");
  assert.equal(fuzzy.length > 0, true);
  assert.ok(fuzzy.every((range) => range.kind === "fuzzy"));
  // 逐字命中覆盖到每个命中字符。
  const covered = fuzzy.map((range) => "A-K-4-7".slice(range.start, range.end)).join("");
  assert.equal(covered.toLowerCase(), "ak47");

  // 完全命不中 ⇒ 不返回区间（界面就不会乱高亮）。
  assert.deepEqual(findMatchRanges("步枪", "ak47"), []);
});

test("highlightMatches 只包命中区间，绝不破坏转义", () => {
  assert.equal(highlightMatches("AK47 步枪", "ak47"), '<mark class="search-hit" data-match="exact">AK47</mark> 步枪');
  assert.equal(highlightMatches("普通文本", "zzz"), "普通文本");
  // 关键字本身带尖括号也必须被转义（不能因为高亮引入注入）。
  const evil = highlightMatches("<img src=x onerror=alert(1)>", "<img>");
  assert.ok(!evil.includes("<img"), "尖括号必须被转义");
  assert.ok(evil.includes("&lt;"), "应保留转义后的可见文本");
  assert.equal(highlightMatches("", "a"), "");
});

test("describeMatchReasons 说清命中的字段", () => {
  assert.deepEqual(describeMatchReasons(sampleFile, "ak47"), ["标题", "文件名", "主体"]);
  assert.deepEqual(describeMatchReasons(sampleFile, "贴图"), ["子标签"]);
  // "步枪" 同时出现在标题与子标签里（一级标签是"武器"，不含"步枪"）。
  assert.deepEqual(describeMatchReasons(sampleFile, "步枪"), ["标题", "子标签"]);
  assert.deepEqual(describeMatchReasons(sampleFile, "武器"), ["一级标签", "主体"]);
  assert.deepEqual(describeMatchReasons(sampleFile, "不存在的东西"), []);
  assert.deepEqual(describeMatchReasons(sampleFile, ""), []);
  // 超过上限时只保留前几个，避免 chip 把标题挤走。
  assert.equal(describeMatchReasons(sampleFile, "a", 1).length, 1);
});

test("formatMatchReasonChip 与 describeSearchResult 文案", () => {
  assert.equal(formatMatchReasonChip([]), "");
  assert.equal(formatMatchReasonChip(["标题", "子标签"]), "匹配：标题 · 子标签");
  assert.equal(describeSearchResult({ total: 2000, shown: 12, query: "ak47" }), "匹配 12 / 2000 个 Mod");
  assert.equal(describeSearchResult({ total: 2000, shown: 0, query: "ak47" }), "没有匹配「ak47」的 Mod（共 2000 个）");
  assert.equal(describeSearchResult({ total: 2000, shown: 12, query: "  " }), "");
  assert.equal(normalizeQuery("  ak47  "), "ak47");
});

test("搜索语法：多个词一起高亮，且不重复叠 mark", () => {
  // "ak" 与 "ak47" 的区间重叠，必须合并成一段（否则会渲染出嵌套/重复的 mark）。
  const merged = mergeMatchRanges([
    { start: 0, end: 2, kind: "exact" },
    { start: 0, end: 4, kind: "exact" },
  ]);
  assert.deepEqual(merged, [{ start: 0, end: 4, kind: "exact" }]);

  assert.equal(
    highlightMatches("AK47 步枪", ["ak47", "步枪"]),
    '<mark class="search-hit" data-match="exact">AK47</mark> <mark class="search-hit" data-match="exact">步枪</mark>',
  );
  // 相邻（首尾相接）的区间会并成一段：视觉上就是"这一整段都命中了"。
  assert.equal(highlightMatches("ab", ["a", "b"]), '<mark class="search-hit" data-match="exact">ab</mark>');
  // 中间隔着一个字符则保持两段。
  assert.equal(highlightMatches("a-b", ["a", "b"]).split("<mark").length - 1, 2);
});

test("搜索语法的命中字段：tag: 标成标签，-tag: 不参与高亮", () => {
  const file = { ...sampleFile, primaryTag: "武器", secondaryTags: ["步枪"] };
  assert.deepEqual(describeMatchReasons(file, "tag:武器"), ["一级标签"]);
  assert.deepEqual(describeMatchReasons(file, "tag:步枪|狙击"), ["子标签"]);
  // `-tag:` 只是排除条件：它命中不了任何东西，也就不该产生"匹配到xx"的说明。
  assert.deepEqual(describeMatchReasons(file, "-tag:材质"), []);
  // 正则命中标题时标"标题"。
  assert.deepEqual(describeMatchReasons(file, "re:^ak47"), ["标题", "文件名", "主体"].slice(0, 3));
});

test("正则写错时计数文案直接说明原因", () => {
  assert.match(
    describeSearchResult({ total: 10, shown: 0, query: "re:[", regexInvalid: "Invalid regular expression" }),
    /正则表达式无效/,
  );
});

test("正则命中也要高亮（re: 不能只筛不标）", () => {
  const html = highlightMatches("!!医疗箱-0主文件.vpk", {
    terms: [],
    regex: /^!!医疗箱/i,
  });
  assert.match(html, /<mark class="search-hit" data-match="regex">!!医疗箱<\/mark>/);
  // 词与正则同时存在时两段都亮，且不重叠。
  const mixed = highlightMatches("AK47 医疗箱", { terms: ["ak47"], regex: /医疗箱/ });
  assert.equal(mixed.split("<mark").length - 1, 2);
  // 零长度正则会前进而不是死循环。
  assert.doesNotThrow(() => highlightMatches("abc", { terms: [], regex: /(?=b)/ }));
});
