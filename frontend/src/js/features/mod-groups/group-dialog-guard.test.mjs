import { test } from "node:test";
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

// WebView2 对 window.prompt 没有默认实现，window.confirm 又是阻塞式原生对话框，
// 在自动化与锁屏环境下都会让流程卡住或直接返回 null。
// 策略组交互必须使用应用内弹窗（features/modals/confirm.js 与 prompt.js）。

const here = path.dirname(fileURLToPath(import.meta.url));
const groupUiPath = path.join(here, "group-ui.js");
const source = readFileSync(groupUiPath, "utf8");

test("策略组交互不使用 window.confirm", () => {
  assert.equal(
    /window\.confirm\s*\(/.test(source),
    false,
    "group-ui.js 不允许调用 window.confirm，请改用 showConfirmModal",
  );
});

test("策略组交互不使用 window.prompt", () => {
  assert.equal(
    /window\.prompt\s*\(/.test(source),
    false,
    "group-ui.js 不允许调用 window.prompt，请改用 showPromptModal",
  );
});

test("策略组交互导入了应用内确认与输入弹窗", () => {
  assert.match(source, /from\s+"\.\.\/modals\/confirm\.js"/, "缺少 showConfirmModal 导入");
  assert.match(source, /from\s+"\.\.\/modals\/prompt\.js"/, "缺少 showPromptModal 导入");
});
