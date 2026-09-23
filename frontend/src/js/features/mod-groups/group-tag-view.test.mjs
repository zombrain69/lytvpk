import assert from "node:assert/strict";
import test from "node:test";

import {
  buildTagApplyPlan,
  formatTagApplySummary,
  formatTagOrigin,
  formatTagSuggestionLine,
} from "./group-tag-view.mjs";

test("formatTagApplySummary 汇总打标签结果", () => {
  assert.equal(
    formatTagApplySummary({ tag: "M16 武器", applied: ["a.vpk", "b.vpk"], skipped: [], missing: [], failed: [] }),
    "已给 2 个 Mod 打上标签「M16 武器」",
  );
  assert.equal(
    formatTagApplySummary({
      tag: "M16 武器",
      applied: ["a.vpk"],
      skipped: ["b.vpk"],
      missing: ["ghost.vpk"],
      failed: ["c.vpk"],
    }),
    "已给 1 个 Mod 打上标签「M16 武器」；1 个已经有这个标签；1 个文件已不在列表里；1 个写入失败",
  );
  assert.equal(
    formatTagApplySummary({ tag: "M16 武器" }),
    "没有需要打标签的 Mod（标签「M16 武器」）",
  );
});

test("formatTagOrigin 说明标签来源", () => {
  assert.match(formatTagOrigin("common-tag"), /共同标签/);
  assert.match(formatTagOrigin("subject"), /主体识别/);
  assert.match(formatTagOrigin("label"), /组名/);
  assert.equal(formatTagOrigin(""), "");
});

test("formatTagSuggestionLine 生成对话框说明", () => {
  assert.equal(
    formatTagSuggestionLine("M16 武器", "label"),
    "建议标签：M16 武器（来自组名）；可以直接改成别的名字。",
  );
  assert.equal(
    formatTagSuggestionLine("", ""),
    "没有自动推导出标签（组名太长或成员没有共同主体），可以自己填一个。",
  );
});

test("buildTagApplyPlan 合并整组标签与逐成员标签", () => {
  // 只给整组标签：整组成员都套用同一个标签。
  assert.deepEqual(
    buildTagApplyPlan({ tag: "sg552", memberKeys: ["a.vpk", "b.vpk"], memberNames: ["A", "B"] }),
    [{ tag: "sg552", keys: ["a.vpk", "b.vpk"], names: ["A", "B"] }],
  );
  // 逐成员标签按"标签"归并，成员顺序保持（先整组、再逐成员），键去重。
  const plan = buildTagApplyPlan({
    tag: "武士刀",
    memberKeys: ["a.vpk", "b.vpk", "c.vpk"],
    memberTags: {
      "b.vpk": ["武士刀", "皮肤"],
      "c.vpk": ["皮肤"],
    },
  });
  assert.deepEqual(plan, [
    // 整组标签覆盖全部成员；逐成员标签再补上更细的分类。
    { tag: "武士刀", keys: ["a.vpk", "b.vpk", "c.vpk"], names: ["a.vpk", "b.vpk", "c.vpk"] },
    { tag: "皮肤", keys: ["b.vpk", "c.vpk"], names: ["b.vpk", "c.vpk"] },
  ]);
  // 大小写不同视为同一个标签，避免重复打两次。
  assert.deepEqual(
    buildTagApplyPlan({ tag: "SG552", memberKeys: ["a.vpk"], memberTags: { "a.vpk": ["sg552"] } }),
    [{ tag: "SG552", keys: ["a.vpk"], names: ["a.vpk"] }],
  );
});

test("buildTagApplyPlan 在没有标签时返回空计划", () => {
  assert.deepEqual(buildTagApplyPlan({}), []);
  assert.deepEqual(buildTagApplyPlan(null), []);
  assert.deepEqual(buildTagApplyPlan({ memberTools: ["x"] }), []);
});
