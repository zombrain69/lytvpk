// 文案目录一致性：页面代码里 t("...") 引用的每个 key 都必须真实存在于
// messages/zh-CN.json —— 迁移页面时最常见的错误是"接好线了但忘了加文案"，
// 那会静默渲染成 key 本身（i18n 运行时的设计如此），只有靠这个测试兜住。
import assert from "node:assert/strict";
import { readFileSync, readdirSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

const coreDir = dirname(fileURLToPath(import.meta.url)); // src/js/core
const srcRoot = dirname(coreDir); // src/js
const catalogPath = join(srcRoot, "..", "messages", "zh-CN.json");

// i18n 基础设施自身不参与检查（注释里就带有 t("...") 示例）。
const EXCLUDED_BASENAMES = new Set(["i18n.mjs", "i18n-runtime.js"]);

function loadCatalogKeys() {
  const catalog = JSON.parse(readFileSync(catalogPath, "utf8"));
  const keys = new Set();
  const walk = (node, prefix) => {
    for (const [name, value] of Object.entries(node)) {
      const key = prefix ? `${prefix}.${name}` : name;
      if (typeof value === "string") {
        keys.add(key);
      } else if (value && typeof value === "object") {
        walk(value, key);
      }
    }
  };
  walk(catalog, "");
  return keys;
}

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
    if (EXCLUDED_BASENAMES.has(entry.name)) continue;
    files.push(path);
  }
  return files;
}

// 匹配 t("some.key")（含带参数调用的 t("some.key", {...})）。
// `t` 前面不允许是标识符或属性访问的一部分，避免把 element(...) / setRangeText(...)
// 这类正常调用误当成翻译调用。
const TRANSLATION_CALL = /(?:^|[^\w$.])t\("([\w.]+)"/g;

test("messages/zh-CN.json 覆盖所有页面 t() 引用的 key", () => {
  const catalogKeys = loadCatalogKeys();
  const missing = [];
  let referenced = 0;
  for (const file of listSourceFiles(srcRoot)) {
    const source = readFileSync(file, "utf8");
    // 只有真正引入过运行时的文件才可能用 t()，其余文件（含同名小助手函数）跳过。
    if (!source.includes("core/i18n-runtime.js")) continue;
    for (const match of source.matchAll(TRANSLATION_CALL)) {
      referenced += 1;
      const key = match[1];
      if (!catalogKeys.has(key)) {
        missing.push(`${relative(srcRoot, file)}: ${key}`);
      }
    }
  }
  // 反向保险：如果正则因重构失效（一条都扫不到），这个测试也会直接失败。
  assert.ok(referenced > 0, "没有扫描到任何 t() 调用，检查正则是否失效");
  assert.deepEqual(missing, [], `以下 t() key 没有对应的中文文案：\n${missing.join("\n")}`);
});
