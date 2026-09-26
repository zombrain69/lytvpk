import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  FILE_OPERATION_BUSY_CLASS,
  FILE_OPERATION_POLL_INTERVAL_MS,
  describeFileOperationBusy,
  formatFileOperationBlockedMessage,
  shouldKeepBusyStateAfterProbeError,
} from "./file-operation-busy.mjs";

test("忙碌文案与后端闸门提示一致", () => {
  assert.equal(describeFileOperationBusy(false), "");
  const busy = describeFileOperationBusy(true);
  assert.match(busy, /正在处理文件/);
  assert.match(busy, /移动 \/ 删除 \/ 打包/);
  // 与 Go 侧 errFileOperationBusy 的措辞保持同一句，用户看到的两处提示不会互相矛盾。
  assert.equal(
    formatFileOperationBlockedMessage(),
    "另一个文件操作正在进行（移动 / 删除 / 打包），请等它完成后再试",
  );
  assert.equal(FILE_OPERATION_BUSY_CLASS, "is-file-operation-busy");
  assert.ok(FILE_OPERATION_POLL_INTERVAL_MS >= 500, "轮询间隔不能太密");
  assert.equal(shouldKeepBusyStateAfterProbeError(), true, "轮询失败时保留忙碌状态");
});

test("状态栏徽标、轮询器与 CSS 都接上了", () => {
  const html = readFileSync(new URL("../../../index.html", import.meta.url), "utf8");
  const runtime = readFileSync(new URL("../features/app-runtime.js", import.meta.url), "utf8");
  const watcher = readFileSync(new URL("./file-operation-watch.js", import.meta.url), "utf8");
  const css = readFileSync(new URL("../../css/app/reading-comfort.css", import.meta.url), "utf8");

  assert.match(html, /id="file-operation-busy"/, "状态栏要有忙碌提示位");
  assert.match(runtime, /startFileOperationWatcher\(\{/, "启动时要开监视器");
  assert.match(runtime, /subscribe: \(handler\) => EventsOn\("file_operation_state"/, "主通道是后端事件");
  assert.match(watcher, /FILE_OPERATION_BUSY_CLASS/, "轮询器要切换 body class");
  assert.match(watcher, /deps\.subscribe/, "要支持后端事件订阅");
  // 刻意不判断 document.hidden：窗口被遮挡时 WebView2 也报 hidden，会整段错过状态。
  // （注释里会提到这个词，所以只查"真的拿它做判断"的写法。）
  assert.ok(!/if \(document\.hidden\)/.test(watcher), "不要用 document.hidden 跳过状态更新");
  assert.match(css, /body\.is-file-operation-busy #move-selected-btn/, "忙碌时要灰掉移动按钮");
  assert.match(css, /body\.is-file-operation-busy #delete-selected-btn/, "忙碌时要灰掉删除按钮");
  assert.match(css, /\.file-operation-busy\[hidden\]/, "徽标隐藏样式要写全");
});
