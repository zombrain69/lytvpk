import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// IP 直连（对齐上游 1d581aa）的静态接线守卫：
// 入口按钮 / 弹窗 / 输入框 / 确认按钮 / 模块接线缺一不可，否则点了没反应。

const here = path.dirname(fileURLToPath(import.meta.url));
const frontendRoot = path.resolve(here, "../../../..");
const indexHtml = readFileSync(path.join(frontendRoot, "index.html"), "utf8");
const serversJs = readFileSync(path.join(here, "servers.js"), "utf8");

test("IP 直连的 DOM 入口与弹窗都存在", () => {
  const requiredIds = [
    "open-direct-connect-modal-btn",
    "direct-connect-modal",
    "direct-connect-address",
    "close-direct-connect-modal-btn",
    "cancel-direct-connect-btn",
    "confirm-direct-connect-btn",
  ];
  for (const id of requiredIds) {
    assert.ok(indexHtml.includes(`id="${id}"`), `index.html 缺少 #${id}`);
  }
});

test("servers.js 接线了 direct-connect 模块", () => {
  assert.match(serversJs, /from "\.\/direct-connect\.js"/u);
  assert.match(serversJs, /configureDirectConnect\(\{\s*showError,\s*connectServer\s*\}\)/u);
  assert.match(serversJs, /setupDirectConnectListeners\(\)/u);
});
