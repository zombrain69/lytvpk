// 静态护栏：重构删掉常量/函数后，同文件里的旧引用必须跟着改掉。
//
// 真机踩过：把"分批填充"换成"窗口化渲染"时删掉了 LOAD_ORDER_FIRST_CHUNK，
// 但下拉填充那条路径还在引用它 —— Vite 构建只给 warning，测试也不 import 这个
// DOM 重的模块，结果运行到"打开加载顺序"才抛 ReferenceError（界面表现为点了没反应）。

import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

const here = path.dirname(fileURLToPath(import.meta.url));
const policyPath = path.resolve(here, "load-order-policy.js");

test("load-order-policy.js 里引用的 LOAD_ORDER_* 常量都必须已定义或已 import", () => {
  const source = readFileSync(policyPath, "utf8");
  const used = new Set([...source.matchAll(/\bLOAD_ORDER_[A-Z0-9_]+\b/g)].map((m) => m[0]));
  const defined = new Set(
    [...source.matchAll(/(?:const|let|var)\s+(LOAD_ORDER_[A-Z0-9_]+)\s*=/g)].map((m) => m[1]),
  );
  // import 块里出现的名字也算已声明（跨文件常量）。
  const imported = new Set(
    [...source.matchAll(/import\s*\{([^}]*)\}\s*from/g)]
      .flatMap((m) => m[1].split(","))
      .map((name) => name.trim().split(/\s+as\s+/)[0].trim())
      .filter(Boolean),
  );
  const missing = [...used].filter((name) => !defined.has(name) && !imported.has(name));
  assert.deepEqual(missing, [], `这些常量没有定义/导入，运行时会 ReferenceError：${missing.join(", ")}`);
});

test("预览行走窗口化渲染，关闭弹窗要断开监听", () => {
  const source = readFileSync(policyPath, "utf8");
  assert.match(source, /computePreviewWindow\(/, "预览行要用窗口换算");
  assert.match(source, /previewSpacerHeights\(/, "预览行要有上下占位块");
  assert.match(
    source,
    /function closeEnhancedLoadOrderModal\(\)[\s\S]{0,200}detachPreviewWindow\(\)/,
    "关闭弹窗要拆掉窗口（否则 scroll 监听泄漏）",
  );
});
