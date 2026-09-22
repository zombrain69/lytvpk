// 设置页依赖注入契约测试。
//
// 背景：设置页把后端绑定从 deps 里**解构**成裸变量使用。只要漏掉一个名字，
// 运行时会抛 `XXX is not defined`，整页变成"设置页面加载失败"——
// 这类问题 `node --test` / `npm run build` 都发现不了（Vite 把未知标识符当全局变量）。
// 这个测试直接静态检查源码，把两类问题挡在提交前：
//   1. 页面里 `deps.X(...)` 调用的 X，必须真的由 app-runtime 注入；
//   2. 页面里裸用（未解构、未 import、未声明）的后端绑定名，必须报错。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const SETTINGS_PAGE_URL = new URL("./settings-page.js", import.meta.url);
const APP_RUNTIME_URL = new URL("../app-runtime.js", import.meta.url);
const SETTINGS_MODULE_URL = new URL("./settings.js", import.meta.url);

const BINDING_PREFIXES = [
  "Get", "Set", "List", "Capture", "Apply", "Delete", "Import", "Export", "Preview",
  "Check", "Refresh", "Download", "Generate", "Reload", "Parse", "Move", "Run", "Enable",
  "Remove", "Save", "Select", "Analyze", "Open", "Create", "Restore", "Write", "Has", "Is",
  "Show", "Toggle", "Run", "Cancel", "Retry", "Clear",
];

// 语言内置的全局对象，恰好也以这些前缀开头（例如 Set / Map / Date / Error）。
const JS_GLOBALS = new Set([
  "Set", "Map", "Date", "Error", "Promise", "RegExp", "Number", "String", "Boolean",
  "Object", "Array", "JSON", "Math", "Intl", "URL", "Blob", "File", "FileReader", "Image",
  "Node", "Element", "CSS", "Set", "WeakSet", "WeakMap", "Symbol", "BigInt", "Proxy",
]);

function readSource(url) {
  return readFileSync(url, "utf8");
}

/** 提取一个 `名称({...})` 调用里的顶层 key。 */
function readObjectCallKeys(source, callHead, description) {
  const start = source.indexOf(callHead);
  assert.ok(start >= 0, `应存在 ${description} 调用`);
  const braceStart = source.indexOf("{", start);
  assert.ok(braceStart > start, `${description} 调用应包含对象字面量`);
  // 用括号配平找到对象字面量的结尾：不同调用点的收尾可能是 `});` 也可能是 `}),`。
  let depth = 0;
  let index = braceStart;
  for (; index < source.length; index += 1) {
    const char = source[index];
    if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  assert.ok(index > braceStart && depth === 0, `${description} 调用应正常结束`);
  const body = source.slice(braceStart, index + 1);
  const keys = new Set();
  // 简写形式 `Name,` 与显式形式 `Name: ...` 都算注入。
  for (const match of body.matchAll(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*,/gm)) {
    keys.add(match[1]);
  }
  for (const match of body.matchAll(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:/gm)) {
    keys.add(match[1]);
  }
  return keys;
}

/**
 * 设置页最终拿到的 deps = app-runtime 的 configureSettings 对象
 * + settings.js 里 buildSettingsDeps 的覆盖项。两者都要算作"已注入"。
 */
function readConfiguredDepsKeys() {
  const keys = readObjectCallKeys(
    readSource(APP_RUNTIME_URL),
    "configureSettings({",
    "app-runtime.js 的 configureSettings({...})",
  );
  const overrides = readObjectCallKeys(
    readSource(SETTINGS_MODULE_URL),
    "buildSettingsDeps(settingsDeps, {",
    "settings.js 的 buildSettingsDeps(settingsDeps, {...})",
  );
  for (const key of overrides) keys.add(key);
  return keys;
}

function readModuleDeclarations(pageSource) {
  const imported = new Set();
  for (const match of pageSource.matchAll(/import\s*\{([^}]*)\}\s*from/g)) {
    for (const part of match[1].split(",")) {
      const name = part.trim().split(/\s+as\s+/).pop()?.trim();
      if (name) imported.add(name);
    }
  }
  const declared = new Set();
  for (const match of pageSource.matchAll(/^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)/gm)) {
    declared.add(match[1]);
  }
  for (const match of pageSource.matchAll(/^\s*(?:const|let|var)\s+([A-Za-z_][A-Za-z0-9_]*)/gm)) {
    declared.add(match[1]);
  }
  return { imported, declared };
}

function readDestructuredDeps(pageSource) {
  const names = new Set();
  const start = pageSource.indexOf("export async function renderSettingsPage(deps)");
  assert.ok(start >= 0, "settings-page.js 应导出 renderSettingsPage(deps)");
  const open = pageSource.indexOf("const {", start);
  const close = pageSource.indexOf("} = deps;", open);
  assert.ok(open > start && close > open, "renderSettingsPage 应从 deps 解构绑定");
  for (const match of pageSource.slice(open, close).matchAll(/([A-Za-z_][A-Za-z0-9_]*)\s*,/g)) {
    names.add(match[1]);
  }
  return names;
}

test("设置页调用的每个 deps.X(...) 都由 app-runtime 注入", () => {
  const page = readSource(SETTINGS_PAGE_URL);
  const provided = readConfiguredDepsKeys();
  assert.ok(provided.size > 50, `configureSettings 应提供大量绑定，实际 ${provided.size} 个`);

  const missing = new Set();
  for (const match of page.matchAll(/\bdeps\.([A-Za-z_][A-Za-z0-9_]*)\s*\(/g)) {
    const name = match[1];
    // 该调用点自己写成可选调用 `deps.X?.(...)` 时属于显式防御，允许缺失。
    const callSite = page.slice(match.index, match.index + match[0].length);
    if (callSite.includes("?.")) continue;
    if (!provided.has(name)) missing.add(name);
  }

  assert.deepEqual(
    [...missing].sort(),
    [],
    `设置页调用了 app-runtime 未注入的绑定：${[...missing].sort().join("、")}；` +
      "请在 app-runtime.js 的 import 与 configureSettings({...}) 中同时补上。",
  );
});

test("设置页没有裸用未声明的后端绑定", () => {
  const page = readSource(SETTINGS_PAGE_URL);
  const provided = readConfiguredDepsKeys();
  const { imported, declared } = readModuleDeclarations(page);
  const destructured = readDestructuredDeps(page);

  const available = new Set([...imported, ...declared, ...destructured, ...JS_GLOBALS]);
  const undeclared = new Set();
  for (const match of page.matchAll(/(?<![.\w$])([A-Za-z_][A-Za-z0-9_]*)\s*(?:\(|,|\)|;|\.|\?|$)/gm)) {
    const name = match[1];
    if (!BINDING_PREFIXES.some((prefix) => name.startsWith(prefix))) continue;
    if (available.has(name)) continue;
    if (!provided.has(name)) continue; // 只关心后端绑定名，忽略普通业务变量
    const before = page.slice(Math.max(0, match.index - 6), match.index);
    if (/\bnew\s+$/.test(before)) continue;
    const after = page.slice(match.index + name.length, match.index + name.length + 1);
    if (after === ":") continue; // 对象字面量的 key
    undeclared.add(name);
  }

  assert.deepEqual(
    [...undeclared].sort(),
    [],
    `设置页裸用了未声明的后端绑定：${[...undeclared].sort().join("、")}；` +
      "要么加入 renderSettingsPage 的解构列表，要么改写成 deps.X。",
  );
});

test("设置页解构的每个依赖都由 app-runtime 注入", () => {
  const destructured = readDestructuredDeps(readSource(SETTINGS_PAGE_URL));
  const provided = readConfiguredDepsKeys();
  const missing = [...destructured].filter((name) => !provided.has(name));

  assert.deepEqual(
    missing.sort(),
    [],
    `renderSettingsPage 解构了 app-runtime 未注入的名字：${missing.sort().join("、")}；` +
      "解构出来会是 undefined，控件静默失效。",
  );
});

// “关于”页用另一种注入风格（函数签名解构 + 调用点字面量），同样会因为漏传而静默失效。
// 关键点：renderAboutPage 有**多个**调用点（关于页 + 关于弹窗），必须全部检查。
test("关于页需要的绑定都由每个调用点传入", () => {
  const aboutUrl = new URL("../about/about.js", import.meta.url);
  const about = readSource(aboutUrl);
  const declaredParams = new Set();
  const signatureStart = about.indexOf("export async function renderAboutPage({");
  assert.ok(signatureStart >= 0, "about.js 应导出 renderAboutPage({...})");
  const signatureEnd = about.indexOf("} = {})", signatureStart);
  assert.ok(signatureEnd > signatureStart, "renderAboutPage 签名应正常结束");
  for (const match of about.slice(signatureStart, signatureEnd).matchAll(/([A-Za-z_][A-Za-z0-9_]*)\s*,/g)) {
    declaredParams.add(match[1]);
  }

  const callSites = [
    { label: "app-runtime.js", source: readSource(APP_RUNTIME_URL) },
    { label: "modals/info.js", source: readSource(new URL("../modals/info.js", import.meta.url)) },
  ];

  const problems = [];
  let callCount = 0;
  for (const { label, source } of callSites) {
    let searchFrom = 0;
    while (true) {
      const callStart = source.indexOf("renderAboutPage({", searchFrom);
      if (callStart < 0) break;
      callCount += 1;
      searchFrom = callStart + 1;
      const braceStart = source.indexOf("{", callStart);
      let depth = 0;
      let index = braceStart;
      for (; index < source.length; index += 1) {
        const char = source[index];
        if (char === "{") depth += 1;
        else if (char === "}") {
          depth -= 1;
          if (depth === 0) break;
        }
      }
      const body = source.slice(braceStart, index + 1);
      const passed = new Set();
      for (const match of body.matchAll(/([A-Za-z_][A-Za-z0-9_]*)\s*,/g)) passed.add(match[1]);
      for (const match of body.matchAll(/^\s*([A-Za-z_][A-Za-z0-9_]*)\s*:/gm)) passed.add(match[1]);
      const missing = [...declaredParams].filter((name) => !passed.has(name));
      if (missing.length > 0) {
        problems.push(`${label}: ${missing.sort().join("、")}`);
      }
    }
  }

  assert.ok(callCount >= 2, `应该至少有两个 renderAboutPage 调用点，实际 ${callCount} 个`);
  assert.deepEqual(problems, [], `renderAboutPage 调用点漏传参数：${problems.join("；")}`);
});

// bindSettingsPage 只在这一个调用点拿到绑定对象。历史上它是手写子集，
// 于是"解构列表"和"子集字面量"成了两个必须同步的清单，漏一处就静默失效。
test("bindSettingsPage 整体透传 deps（不再维护第二份绑定清单）", () => {
  const page = readSource(SETTINGS_PAGE_URL);
  const callStart = page.indexOf("bindSettingsPage({");
  assert.ok(callStart >= 0, "settings-page.js 应调用 bindSettingsPage({...})");
  const braceStart = page.indexOf("{", callStart);
  let depth = 0;
  let index = braceStart;
  for (; index < page.length; index += 1) {
    const char = page[index];
    if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  const body = page.slice(braceStart, index + 1);

  assert.ok(
    /^\s*\.\.\.deps,\s*$/m.test(body),
    "bindSettingsPage({...}) 必须以 `...deps` 开头整体透传；" +
      "手写绑定子集会导致新增绑定在运行时变成 `deps.X is not a function`。",
  );
});
