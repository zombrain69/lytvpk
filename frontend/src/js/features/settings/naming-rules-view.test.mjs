// 命名规范化规则（路线图第 20 项 P3）纯逻辑测试：词表替换输入校验（与后端同口径）、
// 结果计数容错、视图归一（排序 / 缺字段容错）。
import assert from "node:assert/strict";
import test from "node:test";
import {
  ALIAS_ERROR_KEYS,
  aliasEntries,
  exemptEntries,
  mutationCounts,
  validateAliasInput,
} from "./naming-rules-view.mjs";

test("validateAliasInput：通过 / 空值 / 相同 / 非法字符", () => {
  assert.equal(validateAliasInput("木喷", "泵动霰弹枪"), "");
  assert.equal(validateAliasInput(" 木喷 ", " 泵动霰弹枪 "), "");
  assert.equal(validateAliasInput("", "X"), "empty");
  assert.equal(validateAliasInput("X", "   "), "empty");
  assert.equal(validateAliasInput("木喷", "木喷"), "same");
  assert.equal(validateAliasInput("木喷", "泵动/霰弹枪"), "invalidChars");
  assert.equal(validateAliasInput("木喷", "泵动:霰弹枪"), "invalidChars");
  assert.equal(validateAliasInput("木\n喷", "泵动霰弹枪"), "invalidChars");
  assert.equal(validateAliasInput("木喷", "泵动\n霰弹枪"), "invalidChars");
  // 破折号 / 空格 / 全角标点等合法（和后端允许的范围一致）。
  assert.equal(validateAliasInput("AK47", "AK-47 突击步枪"), "");
});

test("ALIAS_ERROR_KEYS：覆盖全部错误码且指向命名卡片文案", () => {
  assert.deepEqual(Object.keys(ALIAS_ERROR_KEYS).sort(), ["empty", "invalidChars", "same"]);
  for (const key of Object.values(ALIAS_ERROR_KEYS)) {
    assert.match(key, /^settings\.addonlist\.maintenance\.naming\./);
  }
});

test("mutationCounts：缺字段 / null 容错", () => {
  assert.deepEqual(mutationCounts({ added: 2, skipped: ["a"] }), {
    added: 2,
    removed: 0,
    skipped: 1,
  });
  assert.deepEqual(mutationCounts(null), { added: 0, removed: 0, skipped: 0 });
  assert.deepEqual(mutationCounts({}), { added: 0, removed: 0, skipped: 0 });
});

test("aliasEntries：归一 + 过滤空条目", () => {
  const rows = aliasEntries({
    subjectAliases: [{ from: "木喷", to: "泵动霰弹枪" }, { from: "", to: "X" }, null],
  });
  assert.deepEqual(rows, [{ from: "木喷", to: "泵动霰弹枪" }]);
  assert.deepEqual(aliasEntries(null), []);
});

test("exemptEntries：排序稳定 + exists 归一 + 名字回退", () => {
  const rows = exemptEntries({
    exemptions: [
      { path: "b/Nested.vpk", name: "", exists: 0 },
      { path: "A.vpk", name: "A.vpk", exists: true },
      { path: "", name: "skip" },
    ],
  });
  assert.deepEqual(rows, [
    { path: "A.vpk", name: "A.vpk", exists: true },
    { path: "b/Nested.vpk", name: "b/Nested.vpk", exists: false },
  ]);
  assert.deepEqual(exemptEntries(undefined), []);
});
