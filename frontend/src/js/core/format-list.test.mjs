// 列表 / 句读的 locale 感知分隔符（路线图第 21 项）：
//   - 行为：zh-CN 与替换前逐字节一致（、；，：。（x））；en 用半角 + 空格、句末句点；
//   - 回归守卫：非测试源码里不得再出现 join("、") / join("；") / join("，") / `+ "。"` 硬编码。
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";
import { setLocale } from "./i18n-runtime.js";
import {
  clauseSeparator,
  commaSeparator,
  formatParenthetical,
  labelSeparator,
  listSeparator,
  sentenceEnd,
} from "./format-list.mjs";

const coreDir = dirname(fileURLToPath(import.meta.url)); // src/js/core
const srcRoot = dirname(coreDir); // src/js

test("zh-CN：分隔符保持全角（与替换前逐字节一致）", () => {
  setLocale("zh-CN", { notify: false });
  assert.equal(listSeparator(), "、");
  assert.equal(clauseSeparator(), "；");
  assert.equal(commaSeparator(), "，");
  assert.equal(labelSeparator(), "：");
  assert.equal(sentenceEnd(), "。");
  assert.equal(formatParenthetical("a、b"), "（a、b）");
});

test("en：分隔符切半角 + 空格，句末为句点", () => {
  setLocale("en", { notify: false });
  assert.equal(listSeparator(), ", ");
  assert.equal(clauseSeparator(), "; ");
  assert.equal(commaSeparator(), ", ");
  assert.equal(labelSeparator(), ": ");
  assert.equal(sentenceEnd(), ".");
  assert.equal(formatParenthetical("a, b"), "(a, b)");
  // 复位，避免同进程后续用例（本文件内）受语言状态影响。
  setLocale("zh-CN", { notify: false });
});

test("取值随当前语言实时变化（切换后无需重载模块）", () => {
  setLocale("en", { notify: false });
  assert.equal(listSeparator(), ", ");
  setLocale("zh-CN", { notify: false });
  assert.equal(listSeparator(), "、");
});

// 回归守卫（路线图第 21 项「源码扫描测试防回归」）：
// 这些硬编码在 en 界面会渲染成全角标点混排；format-list.mjs 自身的注释里就带反例，跳过。
const SCAN_EXCLUDED = new Set(["format-list.mjs"]);
const BANNED_PATTERNS = [
  [/\.join\("、"\)/, '.join("、")'],
  [/\.join\("；"\)/, '.join("；")'],
  [/\.join\("，"\)/, '.join("，")'],
  [/\+ "。"/, '+ "。"'],
];

function listSourceFiles(dir) {
  const files = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      files.push(...listSourceFiles(path));
      continue;
    }
    if (!entry.name.endsWith(".js") && !entry.name.endsWith(".mjs")) continue;
    if (entry.name.endsWith(".test.mjs")) continue;
    if (SCAN_EXCLUDED.has(entry.name)) continue;
    files.push(path);
  }
  return files;
}

test("回归守卫：非测试源码不得硬编码全角列表分隔符 / 句末句点", () => {
  const offenders = [];
  for (const file of listSourceFiles(srcRoot)) {
    const source = readFileSync(file, "utf8");
    for (const [pattern, label] of BANNED_PATTERNS) {
      if (pattern.test(source)) {
        offenders.push(`${relative(srcRoot, file)}: ${label}`);
      }
    }
  }
  assert.deepEqual(
    offenders,
    [],
    `以下文件仍硬编码了全角分隔符（应改用 core/format-list.mjs）：\n${offenders.join("\n")}`,
  );
});
