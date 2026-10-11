import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  buildRenamePreview,
  computeRenameEditName,
  parseFilenameTags,
  sanitizeRenameTitle,
} from "./rename-edit.mjs";

// —— computeRenameEditName：输入框预填只显示「名字」本体 ——

test("computeRenameEditName：[标签] 前缀在预填中隐藏（回归：曾把标签文字拼进预填值）", () => {
  assert.equal(computeRenameEditName("[Airi]WoolyWinter_Coach.vpk"), "WoolyWinter_Coach");
});

test("computeRenameEditName：_[标签] 组合不丢首字符（回归：曾丢标签文字首字母）", () => {
  assert.equal(computeRenameEditName("_[Boomer]Snorlax (Boomer).vpk"), "Snorlax (Boomer)");
});

test("computeRenameEditName：多段标签（+ 分隔）整体隐藏", () => {
  assert.equal(computeRenameEditName("[人物+特感]Nick.vpk"), "Nick");
});

test("computeRenameEditName：普通隐藏文件去掉 _ 前缀（保存时还原）", () => {
  assert.equal(computeRenameEditName("_foo.vpk"), "foo");
});

test("computeRenameEditName：普通文件只剩 .vpk 后缀差异", () => {
  assert.equal(computeRenameEditName("foo.vpk"), "foo");
  assert.equal(computeRenameEditName("foo"), "foo");
});

test("computeRenameEditName：方括号不在行首时不算标签（不误剥）", () => {
  assert.equal(computeRenameEditName("bar[v2].vpk"), "bar[v2]");
  assert.equal(computeRenameEditName("地图[最终版].vpk"), "地图[最终版]");
});

test("computeRenameEditName：无闭合方括号时原样保留", () => {
  assert.equal(computeRenameEditName("[未闭合.vpk"), "[未闭合");
  assert.equal(computeRenameEditName("_[未闭合.vpk"), "[未闭合");
});

// —— 往返：预填值直接进入预览，必须与原文件名完全一致（不重复、不丢字） ——

test("往返幂等：[标签] 文件预填→预览 = 原名", () => {
  const name = "[Airi]WoolyWinter_Coach.vpk";
  const prefill = computeRenameEditName(name);
  assert.equal(buildRenamePreview("C:/x/" + name, name, prefill, false).filename, name);
});

test("往返幂等：_[标签] 文件预填→预览 = 原名", () => {
  const name = "_[Boomer]Snorlax (Boomer).vpk";
  const prefill = computeRenameEditName(name);
  assert.equal(buildRenamePreview("C:/x/" + name, name, prefill, true).filename, name);
});

test("往返幂等：隐藏 / 普通文件预填→预览 = 原名", () => {
  for (const [name, hidden] of [
    ["_foo.vpk", true],
    ["foo.vpk", false],
    ["_[人物+Hunter]X.vpk", true],
  ]) {
    const prefill = computeRenameEditName(name);
    assert.equal(buildRenamePreview("C:/x/" + name, name, prefill, hidden).filename, name);
  }
});

// —— 编辑场景：改名字本体，标签前缀自动保留 ——

test("编辑本体：[标签] 前缀保留且不重复", () => {
  assert.equal(
    buildRenamePreview("C:/x/[Airi]A.vpk", "[Airi]A.vpk", "A v2", false).filename,
    "[Airi]A v2.vpk",
  );
});

test("编辑本体：_[标签] 前缀保留（含隐藏状态）", () => {
  assert.equal(
    buildRenamePreview("C:/x/_[Airi]A.vpk", "_[Airi]A.vpk", "A v2", true).filename,
    "_[Airi]A v2.vpk",
  );
});

test("显式改标签：新名自带 [标签] 时以用户输入为准", () => {
  assert.equal(
    buildRenamePreview("C:/x/[Airi]A.vpk", "[Airi]A.vpk", "[Iris]A", false).filename,
    "[Iris]A.vpk",
  );
});

test("原名无标签：新名带 [标签] 原样保留", () => {
  assert.equal(buildRenamePreview("C:/x/A.vpk", "A.vpk", "[X]B", false).filename, "[X]B.vpk");
});

test("预览路径：拼回原目录（反斜杠目录 + 自动 .vpk 后缀）", () => {
  const preview = buildRenamePreview("C:\\mods\\A.vpk", "A.vpk", "B", false);
  assert.equal(preview.filename, "B.vpk");
  assert.equal(preview.path, "C:\\mods\\B.vpk");
});

// —— parseFilenameTags（与后端 parser.ParseFilenameTags 同形） ——

test("parseFilenameTags：+ 与 , 都算分隔符", () => {
  assert.deepEqual(parseFilenameTags("[A+B]x.vpk"), { hasTags: true, tags: ["A", "B"] });
  assert.deepEqual(parseFilenameTags("_[A,B]x.vpk"), { hasTags: true, tags: ["A", "B"] });
});

test("parseFilenameTags：无标签返回 hasTags=false", () => {
  assert.deepEqual(parseFilenameTags("x.vpk"), { hasTags: false, tags: [] });
});

// —— sanitizeRenameTitle（addoninfo 标题 → 文件名） ——

test("sanitizeRenameTitle：Windows 非法字符换成空格并压缩", () => {
  assert.equal(sanitizeRenameTitle('a<b>c:"d/e\\f|g?h*i'), "a b c d e f g h i");
});

test("sanitizeRenameTitle：去尾部句点 / 空格与 .vpk 后缀", () => {
  assert.equal(sanitizeRenameTitle("  x  "), "x");
  assert.equal(sanitizeRenameTitle("x. "), "x");
  assert.equal(sanitizeRenameTitle("x.vpk"), "x");
});

test("sanitizeRenameTitle：Windows 保留名加 _ 前缀", () => {
  assert.equal(sanitizeRenameTitle("CON"), "_CON");
  assert.equal(sanitizeRenameTitle("com1"), "_com1");
});

test("sanitizeRenameTitle：中文与常规字符保留", () => {
  assert.equal(sanitizeRenameTitle("冷锋 Cold Front（第一幕）"), "冷锋 Cold Front（第一幕）");
});

// —— 接线：operations.js 必须消费抽出后的模块 ——

test("接线：operations.js 从 rename-edit.mjs 导入并调用 computeRenameEditName", () => {
  const source = readFileSync(new URL("./operations.js", import.meta.url), "utf8");
  assert.ok(source.includes('from "./rename-edit.mjs"'));
  assert.ok(source.includes("computeRenameEditName(fileName)"));
  // 旧的坏写法（把标签文字拼进预填）不得回归
  assert.ok(!source.includes("tagMatch"));
});
