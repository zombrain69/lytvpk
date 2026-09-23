import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// 静态守卫：命名导入必须真的存在于目标模块。
//
// 真实缺陷：context-menu.js 曾经从 group-picker.js 里 import GROUP_PICKER_MODES，
// 而该常量定义在 group-picker-view.mjs。node --test 不会加载这两个 DOM 模块，
// 所以测试全绿，直到 npm run build（rollup）才报
// "'GROUP_PICKER_MODES' is not exported by ..."。这里把这类错误提前到测试阶段。
//
// 只检查项目内的相对导入，并跳过 .ts（wails 生成的绑定）。

const here = path.dirname(fileURLToPath(import.meta.url));
const jsRoot = path.resolve(here, "..");

function listJsFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      result.push(...listJsFiles(full));
      continue;
    }
    if (!entry.name.endsWith(".js") && !entry.name.endsWith(".mjs")) continue;
    if (entry.name.endsWith(".test.mjs")) continue;
    result.push(full);
  }
  return result;
}

function collectExports(source) {
  const names = new Set();
  const declaration = /export\s+(?:async\s+)?(?:function|const|let|var|class)\s+([A-Za-z0-9_$]+)/g;
  let match;
  while ((match = declaration.exec(source)) !== null) {
    names.add(match[1]);
  }
  const list = /export\s*\{([^}]*)\}/g;
  while ((match = list.exec(source)) !== null) {
    match[1]
      .split(",")
      .map((item) => item.trim())
      .filter(Boolean)
      .forEach((item) => {
        const [original, alias] = item.split(/\s+as\s+/).map((part) => part.trim());
        names.add(alias || original);
      });
  }
  if (/export\s+default\b/.test(source)) names.add("default");
  return names;
}

function collectImports(source) {
  const results = [];
  const pattern =
    /import\s+(?:([A-Za-z0-9_$]+)\s*,\s*)?(?:\{([^}]*)\}|\*\s+as\s+([A-Za-z0-9_$]+)|([A-Za-z0-9_$]+))?\s*from\s*["']([^"']+)["']/g;
  let match;
  while ((match = pattern.exec(source)) !== null) {
    const [, , namedList, , defaultImport, specifier] = match;
    const names = [];
    if (namedList) {
      namedList
        .split(",")
        .map((item) => item.trim())
        .filter(Boolean)
        .forEach((item) => {
          const [original, alias] = item.split(/\s+as\s+/).map((part) => part.trim());
          if (original) names.push({ imported: original, local: alias || original });
        });
    }
    if (defaultImport) names.push({ imported: "default", local: defaultImport });
    results.push({ specifier, names });
  }
  return results;
}

const projectFiles = listJsFiles(jsRoot);

// wailsjs 绑定与静态资源都省略/没有 .js 后缀，这里按候选后缀解析。
function resolveTarget(fromFile, specifier) {
  const base = path.resolve(path.dirname(fromFile), specifier);
  for (const candidate of [base, `${base}.js`, `${base}.mjs`, `${base}.ts`, `${base}.json`]) {
    try {
      if (statSync(candidate).isFile()) return candidate;
    } catch {
      // 继续尝试下一个候选后缀
    }
  }
  return null;
}

test("项目内的每个命名导入都能在目标模块里找到对应导出", () => {
  const problems = [];
  for (const file of projectFiles) {
    const source = readFileSync(file, "utf8");
    for (const { specifier, names } of collectImports(source)) {
      if (!specifier.startsWith(".")) continue;
      if (names.length === 0) continue;
      const target = resolveTarget(file, specifier);
      if (!target) {
        problems.push(`${path.relative(jsRoot, file)}: 找不到模块 ${specifier}`);
        continue;
      }
      // 只校验项目内的 JS 模块：wails 生成的 .ts 绑定与 .json / 图片资源不在范围内。
      if (!target.endsWith(".js") && !target.endsWith(".mjs")) continue;
      const targetSource = readFileSync(target, "utf8");
      const exports = collectExports(targetSource);
      names.forEach(({ imported, local }) => {
        if (!exports.has(imported)) {
          problems.push(
            `${path.relative(jsRoot, file)}: "${local}" 来自 ${specifier}，但该模块没有导出 ${imported}`,
          );
        }
      });
    }
  }
  assert.deepEqual(problems, [], `静态导入契约被破坏：\n${problems.join("\n")}`);
});
