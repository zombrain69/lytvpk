// 跨模块调用检查：调用了本项目其它模块导出的名字，但本文件既没导入也没定义。
//
// 背景（真实回归）：`load-order-policy.js` 调用了 `renderFileList(...)` 却没有 import，
// 运行时抛 ReferenceError 被 try/catch 吞掉，导致"保存分层后列表角标不刷新"这种
// 表面看像渲染问题、实际是漏 import 的缺陷。同类的还有 `filters.js` 的 `showNotification`。
// `node --test` / `npm run build` 都不会报这种问题（未声明标识符被当作全局变量），
// 所以这里用源码级检查兜住。

import assert from "node:assert/strict";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { join, relative } from "node:path";
import test from "node:test";

const SRC_ROOT = fileURLToPath(new URL("../", import.meta.url));

const EXPORT_RE =
  /export\s+(?:async\s+)?(?:function|class|const|let|var)\s+([\w$]+)|export\s*\{([^}]*)\}/g;
const IMPORT_RE = /import\s+(?:([\w$]+)\s*,?\s*)?(?:\{([^}]*)\})?\s*from/g;
const IMPORT_NS_RE = /import\s+\*\s+as\s+([\w$]+)\s+from/g;
const LOCAL_DECL_RE =
  /(?:^|[;{}\n])\s*(?:export\s+)?(?:async\s+)?(?:function|class)\s+([\w$]+)|(?:^|[;{}\n])\s*(?:export\s+)?(?:const|let|var)\s+([\w$]+)/g;
const DESTRUCT_RE = /(?:const|let|var)\s*\{([^}]*)\}\s*=/g;
const ARROW_PARAMS_RE = /\(([^()]*)\)\s*=>/g;
const FUNCTION_PARAMS_RE = /function\s*[\w$]*\s*\(([^()]*)\)/g;
const CALL_RE = /(?<![.\w$])([A-Za-z_$][\w$]*)\s*\(/g;
const KEYWORDS = new Set(
  "if for while switch catch function return typeof new await async do else try throw delete in of case default void yield super class extends import export const let var this instanceof".split(
    " ",
  ),
);

function listSourceFiles(dir) {
  const result = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) {
      result.push(...listSourceFiles(path));
      continue;
    }
    if (!entry.name.endsWith(".js") && !entry.name.endsWith(".mjs")) continue;
    if (entry.name.endsWith(".test.mjs")) continue;
    result.push(path);
  }
  return result;
}

function exportedNames(files) {
  const names = new Set();
  for (const file of files) {
    const text = readFileSync(file, "utf8");
    for (const match of text.matchAll(EXPORT_RE)) {
      if (match[1]) names.add(match[1]);
      if (match[2]) {
        for (const part of match[2].split(",")) {
          const alias = part.split(" as ").pop()?.trim();
          if (/^[\w$]+$/.test(alias || "")) names.add(alias);
        }
      }
    }
  }
  return names;
}

function localNames(text) {
  const names = new Set();
  for (const match of text.matchAll(IMPORT_RE)) {
    if (match[1]) names.add(match[1]);
    if (match[2]) {
      for (const part of match[2].split(",")) {
        const alias = part.split(" as ").pop()?.trim();
        if (alias) names.add(alias);
      }
    }
  }
  for (const match of text.matchAll(IMPORT_NS_RE)) names.add(match[1]);
  for (const match of text.matchAll(LOCAL_DECL_RE)) {
    for (const group of match.slice(1)) {
      if (group) names.add(group);
    }
  }
  for (const match of text.matchAll(DESTRUCT_RE)) {
    for (const part of match[1].split(",")) {
      const alias = part.split(":")[0].split("=")[0].trim();
      if (/^[\w$]+$/.test(alias || "")) names.add(alias);
    }
  }
  // 参数（含解构参数）：把参数表里的所有标识符都算作本地绑定，
  // 避免把 `({ refreshFilesKeepFilter } = {})` 这类参数误报成漏 import。
  for (const regex of [ARROW_PARAMS_RE, FUNCTION_PARAMS_RE]) {
    for (const match of text.matchAll(regex)) {
      for (const token of match[1].matchAll(/[\w$]+/g)) {
        if (!KEYWORDS.has(token[0])) names.add(token[0]);
      }
    }
  }
  return names;
}

/** 找到与给定左括号配对的右括号下标。 */
function matchingParen(text, openIndex) {
  let depth = 0;
  for (let index = openIndex; index < text.length; index += 1) {
    const char = text[index];
    if (char === "(") depth += 1;
    else if (char === ")") {
      depth -= 1;
      if (depth === 0) return index;
    }
  }
  return -1;
}

test("没有跨模块调用漏 import 的函数", () => {
  const files = listSourceFiles(SRC_ROOT);
  assert.ok(files.length > 50, `应扫描到大量源文件，实际 ${files.length}`);
  const exported = exportedNames(files);
  assert.ok(exported.size > 50, `应收集到大量导出名，实际 ${exported.size}`);

  const findings = [];
  for (const file of files) {
    const text = readFileSync(file, "utf8");
    const locals = localNames(text);
    for (const match of text.matchAll(CALL_RE)) {
      const name = match[1];
      if (KEYWORDS.has(name) || locals.has(name) || !exported.has(name)) continue;
      const openIndex = text.indexOf("(", match.index);
      const closeIndex = matchingParen(text, openIndex);
      const after = closeIndex >= 0 ? text.slice(closeIndex + 1).match(/^\s*([^\s])/) : null;
      if (after && after[1] === "{") continue; // 对象字面量里的方法定义：name(args) { ... }
      const before = text.slice(Math.max(0, match.index - 10), match.index);
      if (/(?:^|\s)(?:new|function)\s*$/.test(before)) continue;
      const line = text.slice(0, match.index).split("\n").length;
      findings.push(`${relative(SRC_ROOT, file)}:${line} ${name}`);
    }
  }

  assert.deepEqual(
    findings,
    [],
    `以下位置调用了项目内导出的函数但没有 import/定义：\n${findings.join("\n")}`,
  );
});
