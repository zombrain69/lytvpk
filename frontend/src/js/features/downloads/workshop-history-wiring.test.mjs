// 「工坊解析历史」+「工坊 ID 直达」的接线契约（对齐上游 b635ea3 / ce2b268）。
// 纯逻辑在 workshop-history.mjs 里另有单测，这里只保证"界面元素、绑定、后端调用"三者对齐。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const html = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
const runtime = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const modal = readFileSync(new URL("./workshop-modal.js", import.meta.url), "utf8");
const history = readFileSync(new URL("./workshop-history.js", import.meta.url), "utf8");
const idJump = readFileSync(new URL("../workshop/id-jump.js", import.meta.url), "utf8");

test("解析历史：下拉元素齐全，并在启动时接线", () => {
  for (const id of [
    "workshop-history-btn",
    "workshop-history-menu",
    "workshop-history-list",
    "workshop-history-empty",
    "workshop-history-clear",
  ]) {
    assert.match(html, new RegExp(`id="${id}"`), `缺少 #${id}`);
  }

  assert.match(history, /GetWorkshopHistory/, "读取历史要调用后端");
  assert.match(history, /AddWorkshopHistoryEntries/, "解析成功要写入历史");
  assert.match(history, /ClearWorkshopHistory/, "清空要调用后端");
  assert.match(runtime, /setupWorkshopHistory\(/, "启动时要接线历史下拉");
  assert.match(runtime, /onSelect:[\s\S]{0,120}applyWorkshopGroups\(\[item\.group\]\)/, "点历史要用快照重画，不再请求接口");
});

test("解析历史：解析成功写入，切换页面时收起下拉", () => {
  assert.match(modal, /export function applyWorkshopGroups/, "渲染逻辑要抽成可复用函数");
  assert.match(modal, /applyWorkshopGroups\(groupedResult\?\.groups/, "解析成功后走同一条渲染路径");
  assert.match(modal, /recordWorkshopHistory\(groupedResult\?\.groups\)/, "解析成功后写入历史");
  assert.match(modal, /closeWorkshopHistory\(\)/, "重置/切页时要收起下拉");
});

test("ID 直达：按钮、弹窗与解析链路都在", () => {
  for (const id of [
    "browser-id-jump-btn",
    "workshop-id-jump-modal",
    "workshop-id-jump-input",
    "workshop-id-jump-confirm-btn",
    "workshop-id-jump-cancel-btn",
    "close-workshop-id-jump-btn",
  ]) {
    assert.match(html, new RegExp(`id="${id}"`), `缺少 #${id}`);
  }

  assert.match(idJump, /ParseWorkshopID/, "ID 解析要复用后端接口");
  assert.match(idJump, /openWorkshopDetail\(\{ publishedfileid: id \}\)/, "识别出 ID 后直接打开详情");
  assert.match(idJump, /event\.key === "Enter"/, "输入框里回车应直接前往");
  assert.match(runtime, /setupWorkshopIdJump\(\)/, "启动时要接线 ID 直达");
});
