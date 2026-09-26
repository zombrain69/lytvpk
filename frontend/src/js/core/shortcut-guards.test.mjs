// 快捷键"该不该抢键"的判定测试。
// 真机踩过的坑：列表行的复选框是 <input type="checkbox">，如果一律当成"输入框"跳过，
// 勾选之后按 Ctrl+X / Ctrl+V 就完全没反应（Windows 上 SendKeys 复现）。

import assert from "node:assert/strict";
import test from "node:test";

import { isTextEntryElement } from "./shortcuts.mjs";

test("isTextEntryElement 只把真正的文本输入当成输入框", () => {
  assert.equal(isTextEntryElement({ tagName: "INPUT", type: "text" }), true);
  assert.equal(isTextEntryElement({ tagName: "INPUT", type: undefined }), true, "缺省 type 就是 text");
  assert.equal(isTextEntryElement({ tagName: "INPUT", type: "search" }), true);
  assert.equal(isTextEntryElement({ tagName: "TEXTAREA" }), true);
  assert.equal(isTextEntryElement({ tagName: "DIV", isContentEditable: true }), true);

  assert.equal(isTextEntryElement({ tagName: "INPUT", type: "checkbox" }), false, "复选框不该吞掉 Ctrl+X");
  assert.equal(isTextEntryElement({ tagName: "INPUT", type: "radio" }), false);
  assert.equal(isTextEntryElement({ tagName: "INPUT", type: "button" }), false);
  assert.equal(isTextEntryElement({ tagName: "BUTTON" }), false);
  assert.equal(isTextEntryElement({ tagName: "DIV" }), false);
  assert.equal(isTextEntryElement(null), false);
});
