import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

import {
  compiledRegex,
  negatedTerms,
  parseSearchSyntax,
  positiveTerms,
  splitTagAlternatives,
  tokenizeSearchQuery,
} from "./search-syntax.mjs";

// 与 Go 侧共用同一份用例：前端镜像解析器的输出必须和 Go 一样，
// 否则会出现"后端筛出来了、前端却没高亮/没说明"的错位。
const here = path.dirname(fileURLToPath(import.meta.url));
const caseFile = path.resolve(here, "../../../../../internal/app/testdata/search_query_cases.json");
const { cases } = JSON.parse(readFileSync(caseFile, "utf8"));

test("共享用例：前端解析结果与 Go 一致", () => {
  assert.ok(cases.length > 0, "共享用例为空");
  for (const item of cases) {
    const spec = parseSearchSyntax(item.query);
    assert.deepEqual(spec.terms, item.terms, `terms 不一致：${item.query}`);
    assert.deepEqual(spec.includeTags, item.includeTags, `includeTags 不一致：${item.query}`);
    assert.deepEqual(spec.excludeTags, item.excludeTags, `excludeTags 不一致：${item.query}`);
    assert.equal(spec.regex, item.regex, `regex 不一致：${item.query}`);
    if (item.regexError) {
      assert.notEqual(spec.regexInvalid, "", `应当报告正则错误：${item.query}`);
    } else {
      assert.equal(spec.regexInvalid, "", `不应有正则错误：${item.query}`);
    }
  }
});

test("引号与空白切词", () => {
  assert.deepEqual(tokenizeSearchQuery('  "ak 47"   武器  '), ["ak 47", "武器"]);
  assert.deepEqual(tokenizeSearchQuery(""), []);
  assert.deepEqual(splitTagAlternatives("步枪| 狙击 |步枪|"), ["步枪", "狙击"]);
});

test("正反向词与正则的取用", () => {
  const spec = parseSearchSyntax("ak47 -材质 re:^ak 武器");
  assert.deepEqual(positiveTerms(spec), ["ak47", "武器"]);
  assert.deepEqual(negatedTerms(spec), ["材质"]);
  assert.equal(compiledRegex(spec).test("AK47_替换"), true);
  // 正则写错时不返回正则对象（界面据此显示"正则无效"并且不高亮任何东西）。
  assert.equal(compiledRegex(parseSearchSyntax("re:[")), null);
  assert.equal(compiledRegex(parseSearchSyntax("ak47")), null);
});
