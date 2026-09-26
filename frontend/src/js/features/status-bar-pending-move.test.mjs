// 真机回归：状态栏「待移动」角标必须真的能刷新。
// 之前的实现引用了未声明的变量，`updateStatusBar()` 每次都会抛 ReferenceError，
// 表现是"Ctrl+X 标记成功但界面毫无反应、连提示都没有"——单测没覆盖到，
// 所以这里直接 import 真实的 state.js，用最小 DOM 桩跑一遍。

import assert from "node:assert/strict";
import { register } from "node:module";
import test from "node:test";

// state.js 会（间接）import Wails 生成的绑定；这些模块用 Vite 风格的省略扩展名导入，
// Node 的 ESM 解析器默认拒绝。注册一个"补试扩展名"的解析 hook 后再动态 import。
register("../test-utils/extension-resolve-hook.mjs", import.meta.url);

function installDocumentStub() {
  const elements = new Map();
  const makeElement = (id) => ({
    id,
    textContent: "",
    hidden: true,
    checked: false,
    disabled: false,
    classList: { add() {}, remove() {}, toggle() {} },
    querySelector: () => null,
    addEventListener() {},
  });
  globalThis.document = {
    getElementById(id) {
      if (!elements.has(id)) elements.set(id, makeElement(id));
      return elements.get(id);
    },
    querySelector: () => null,
    addEventListener() {},
  };
  return elements;
}

test("updateStatusBar 刷新「待移动」角标且不再抛错", async () => {
  installDocumentStub();
  const { appState, updateStatusBar } = await import("./state.js");

  appState.moveClipboard = new Set();
  assert.doesNotThrow(() => updateStatusBar(), "空标记时也要能安全刷新");
  const pending = document.getElementById("pending-move-files");
  assert.equal(pending.hidden, true, "没有待移动项时角标应隐藏");

  appState.moveClipboard = new Set(["a.vpk", "b.vpk"]);
  assert.doesNotThrow(() => updateStatusBar(), "有标记时不应抛错（真机就是在这里静默失败）");
  assert.equal(pending.textContent, "待移动: 2");
  assert.equal(pending.hidden, false, "有标记时角标要显示出来");

  appState.moveClipboard = new Set();
  updateStatusBar();
  assert.equal(pending.hidden, true, "清空后要重新隐藏");
});
