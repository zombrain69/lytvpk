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
    // 两种导入写法都要认：features 下是 "../core/i18n-runtime.js"，core 内部是 "./i18n-runtime.js"。
    if (!source.includes("i18n-runtime.js")) continue;
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

// 反向检查：index.html 静态节点上 data-i18n* 引用的 key 也必须在目录里。
// 静态节点走 `applyTranslations()` 回填，写错 key 同样会静默失效（渲染为 key 本身），
// 与页面代码里的 t() 调用是同一类问题，这里一起兜住。
test("index.html 静态节点 data-i18n* 引用的 key 都在目录里", () => {
  const htmlPath = join(srcRoot, "..", "..", "index.html");
  const html = readFileSync(htmlPath, "utf8");
  const catalogKeys = loadCatalogKeys();
  const referenced = [];
  for (const match of html.matchAll(/data-i18n(?:-title|-placeholder|-aria-label|-alt)?="([\w.]+)"/g)) {
    referenced.push(match[1]);
  }
  assert.ok(referenced.length > 0, "没有扫描到任何 data-i18n 属性，检查正则或文件位置");
  const missing = referenced.filter((key) => !catalogKeys.has(key));
  assert.deepEqual(missing, [], `index.html 里以下 data-i18n key 没有对应的中文文案：\n${missing.join("\n")}`);
});

// en 目录一致性（第 19 项 P0/P2：P2 已全量机翻产出）：
//   - 每个 en key 必须在 zh-CN 里存在（防拼写漂移 / 陈旧 key 一路留在目录里）；
//   - 每个 en 值的 {placeholder} 集合必须与该 key 的 zh 值一致（漏翻占位符 = 运行时少显示数据）；
//   - en 值不允许中文残渣（CJK）、不允许空值；**例外**：en-verbatim-allowlist.json 里列出的
//     key 是「有意保持原文」的数据层取值（如解析器生成的标签名，按既定决策不做映射层），
//     这类 key 的 en 值必须与 zh 完全一致（白名单自身也做保鲜检查）；
//   - 规模下限兜底：P2 全量机翻后基线 3750；将来 zh 新增文案短期缺翻属预期，但不应大面积丢失。
test("messages/en.json 与 zh-CN 对齐：key 在 zh 内、占位符一致、无中文残渣", () => {
  const zh = JSON.parse(readFileSync(catalogPath, "utf8"));
  const enPath = join(srcRoot, "..", "messages", "en.json");
  const en = JSON.parse(readFileSync(enPath, "utf8"));
  const allowPath = join(srcRoot, "..", "messages", "en-verbatim-allowlist.json");
  const allowList = JSON.parse(readFileSync(allowPath, "utf8"));
  const verbatimKeys = new Set(allowList.keys || []);

  const flatten = (node, prefix = "", out = new Map()) => {
    for (const [name, value] of Object.entries(node)) {
      const key = prefix ? `${prefix}.${name}` : name;
      if (typeof value === "string") out.set(key, value);
      else if (value && typeof value === "object") flatten(value, key, out);
    }
    return out;
  };
  const zhMap = flatten(zh);
  const enMap = flatten(en);

  assert.ok(enMap.size >= 3000, `en 目录规模异常（${enMap.size} keys）——检查是否被误删`);

  const placeholders = (value) =>
    [...String(value).matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();
  const issues = [];
  for (const [key, value] of enMap) {
    if (!zhMap.has(key)) {
      issues.push(`${key}: zh-CN 里不存在该 key`);
      continue;
    }
    if (!value.trim()) issues.push(`${key}: 空值`);
    if (/[\u4e00-\u9fff]/.test(value) && !(verbatimKeys.has(key) && value === zhMap.get(key))) {
      issues.push(`${key}: 含中文残渣`);
    }
    const zhPlaceholders = placeholders(zhMap.get(key));
    const enPlaceholders = placeholders(value);
    if (zhPlaceholders.join(",") !== enPlaceholders.join(",")) {
      issues.push(`${key}: 占位符不一致 zh=[${zhPlaceholders}] en=[${enPlaceholders}]`);
    }
  }
  // 白名单保鲜：列出的 key 必须真实存在，且值与 zh 完全一致（否则白名单已陈旧）。
  for (const key of verbatimKeys) {
    if (!enMap.has(key)) issues.push(`白名单 ${key}: en 目录里不存在`);
    else if (enMap.get(key) !== zhMap.get(key)) issues.push(`白名单 ${key}: 必须与 zh 原文完全一致`);
  }
  assert.deepEqual(issues, [], `en.json 存在问题：\n${issues.join("\n")}`);
});
