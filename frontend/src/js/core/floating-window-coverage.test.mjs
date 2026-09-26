import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 「复杂管理窗口都要能浮动、能拉伸、背景不虚化」是用户的明确要求。
// 这条测试锁定"哪些窗口必须注册浮动能力"，以及"哪些小对话框刻意不注册"，
// 免得以后新增窗口时漏掉，或者把确认框也变成浮动窗口。

const runtimeSource = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");

// 必须有浮动能力的管理窗口。
const REQUIRED_FLOATING = [
  "mod-group-suggest-modal",
  "load-order-modal",
  "conflict-modal",
  "file-conflict-modal",
  "model-stats-modal",
  "file-detail-modal",
  "agent-prompt-modal",
  "set-tags-modal",
  "batch-set-tags-modal",
  "group-picker-modal",
  "group-tag-modal",
  "server-details-modal",
  "panel-map-modal",
  "panel-upload-modal",
  "panel-rcon-modal",
  "panel-difficulty-modal",
  "panel-server-details-modal",
  "problem-scan-modal",
];

// 这三个"曾经是弹窗"，现在被 ui-shell.js 搬成了独立页面：
// 它们的 .modal-content 会被移进页面容器，原 modal 只剩隐藏外壳，
// 所以不该注册浮动（也没有 .modal-content 可浮动）。
const EMBEDDED_PAGES = ["browser-modal", "workshop-modal", "server-modal"];

// 小而临时的对话框：刻意保持模态遮罩，不做浮动（浮动它们只会增加误触）。
const TRANSIENT_DIALOGS = [
  "confirm-modal",
  "prompt-modal",
  "message-modal",
  "rename-modal",
  "exit-confirm-modal",
  "image-preview-modal",
  "update-modal",
  "info-modal",
  "conflict-scope-modal",
  "command-palette-modal",
];

test("复杂管理窗口都注册了浮动能力", () => {
  for (const id of REQUIRED_FLOATING) {
    // 两种写法都接受：批量列表里的名字，或单独的 setupFloatingModal("<id>"。
    const inList = new RegExp(`"${id}",`).test(runtimeSource);
    const single = new RegExp(`setupFloatingModal\\("${id}"`).test(runtimeSource);
    assert.ok(inList || single, `${id} 没有注册浮动能力`);
  }
});

test("已改成独立页面的窗口不注册浮动（否则浮动按钮永远不出现）", () => {
  const shellSource = readFileSync(new URL("./ui-shell.js", import.meta.url), "utf8");
  for (const id of EMBEDDED_PAGES) {
    assert.match(shellSource, new RegExp(`embedModalAsPage\\("${id}"`), `${id} 应该由 ui-shell 搬成页面`);
    assert.ok(
      !new RegExp(`setupFloatingModal\\("${id}"`).test(runtimeSource),
      `${id} 已经是页面，不该注册浮动`,
    );
  }
});

test("临时对话框刻意保持模态，不被注册成浮动窗口", () => {
  for (const id of TRANSIENT_DIALOGS) {
    const inList = new RegExp(`"${id}",`).test(runtimeSource);
    const single = new RegExp(`setupFloatingModal\\("${id}"`).test(runtimeSource);
    assert.ok(!inList && !single, `${id} 是临时对话框，不应注册浮动`);
  }
});
