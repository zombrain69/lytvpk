// 本地化排序（路线图第 19 项 P3）：collator 跟随界面语言 + 前端不再硬编码中文 collation。
import assert from "node:assert/strict";
import { readdirSync, readFileSync } from "node:fs";
import test from "node:test";
import { activeSortLocale, createLocaleCollator } from "./locale-collator.mjs";
import { setLocale } from "./i18n-runtime.js";

test("默认界面语言（zh-CN）：按拼音排序", () => {
  const collator = createLocaleCollator();
  assert.equal(activeSortLocale(), "zh-CN");
  // 爱(ài) < 波(bō) < 张(zhāng)：拼音序与汉字码点序相反，用来区分 collation。
  assert.ok(collator.compare("爱丽丝", "张飞") < 0);
  const order = ["张飞", "爱丽丝", "波本"].sort((a, b) => collator.compare(a, b));
  assert.deepEqual(order, ["爱丽丝", "波本", "张飞"]);
});

test("切到 en：同一比较器自动重建为英文 collation（汉字按码点序）", () => {
  const collator = createLocaleCollator();
  assert.ok(collator.compare("爱丽丝", "张飞") < 0, "zh 下按拼音：爱 < 张");
  setLocale("en");
  try {
    assert.equal(activeSortLocale(), "en");
    // en 下汉字按 Unicode 码点：张(U+5F20) < 爱(U+7231)，与拼音序相反。
    assert.ok(collator.compare("爱丽丝", "张飞") > 0);
    assert.ok(collator.compare("alice", "Charles") < 0, "字母序保持大小写不敏感语义由调用方处理");
  } finally {
    setLocale("zh-CN");
  }
  assert.ok(collator.compare("爱丽丝", "张飞") < 0, "切回后恢复拼音序");
});

test("numeric 选项生效，且语言切换不影响选项", () => {
  const collator = createLocaleCollator({ numeric: true });
  const sorted = () => ["mod10", "mod2", "mod1"].sort((a, b) => collator.compare(a, b));
  assert.deepEqual(sorted(), ["mod1", "mod2", "mod10"]);
  setLocale("en");
  try {
    assert.deepEqual(sorted(), ["mod1", "mod2", "mod10"]);
  } finally {
    setLocale("zh-CN");
  }
});

test("未知 locale 归一化回 zh-CN（与白名单纪律一致）", () => {
  setLocale("xx-YY");
  try {
    assert.equal(activeSortLocale(), "zh-CN");
  } finally {
    setLocale("zh-CN");
  }
});

test("扫描：前端非测试代码不再硬编码中文 collation（第 19 项 P3）", () => {
  const root = new URL("../..", import.meta.url);
  const files = [];
  const walk = (dir) => {
    for (const entry of readdirSync(dir, { withFileTypes: true })) {
      if (entry.name === "node_modules") continue;
      const target = new URL(entry.name + (entry.isDirectory() ? "/" : ""), dir);
      if (entry.isDirectory()) walk(target);
      else if (/\.(js|mjs)$/u.test(entry.name) && !/\.test\.mjs$/u.test(entry.name)) files.push(target);
    }
  };
  walk(root);
  assert.ok(files.length > 100, `扫描范围异常，仅 ${files.length} 个文件`);
  const offenders = [];
  for (const file of files) {
    const text = readFileSync(file, "utf8");
    if (/localeCompare\([^)]*"zh/u.test(text)) offenders.push(`localeCompare: ${file.pathname}`);
    if (/new Intl\.Collator\("zh/u.test(text)) offenders.push(`Intl.Collator: ${file.pathname}`);
  }
  assert.deepEqual(offenders, [], "排序 collation 必须走 core/locale-collator.mjs");
});
