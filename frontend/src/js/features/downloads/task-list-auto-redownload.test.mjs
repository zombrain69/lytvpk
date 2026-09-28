import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 下载队列的任务级「失败后自动重下」开关：
// 后端早就有 SetDownloadTaskAutoRedownload，文档也写着"可以在下载队列里单独开关某个任务"，
// 但界面一直没有入口（只有全局默认开关）。这里锁住接线，避免再出现
// "后端支持了但界面没有入口"的半成品。

const source = readFileSync(new URL("./task-list.js", import.meta.url), "utf8");

test("任务列表接入 SetDownloadTaskAutoRedownload", () => {
  assert.match(source, /SetDownloadTaskAutoRedownload,/, "缺少 SetDownloadTaskAutoRedownload 导入");
  assert.match(
    source,
    /await SetDownloadTaskAutoRedownload\(task\.id, enabled\)/,
    "任务级开关要真的调用后端",
  );
});

test("渲染任务级开关并回显后端状态", () => {
  assert.match(source, /class="task-auto-redownload-input"/, "缺少任务级开关元素");
  assert.match(source, /失败后自动重下/, "开关要有可读文案");
  assert.match(
    source,
    /task\.auto_redownload === true \? " checked" : ""/,
    "开关要回显后端的 auto_redownload",
  );
});

test("失败与已中断的任务也能单独开关", () => {
  // 文档承诺"中断任务同样可以单独开关"；已完成的任务才不需要这个策略。
  assert.match(
    source,
    /if \(!task \|\| task\.status === "completed"\) return "";/,
    "只应排除已完成任务",
  );
});

test("写入失败要回滚勾选状态并提示", () => {
  assert.match(source, /autoRedownloadInput\.checked = !enabled;/, "失败时要回滚勾选");
  assert.match(source, /切换自动重下失败/, "失败时要提示用户");
});

test("开关文案说明自动重下只做一次且不会覆盖主动取消", () => {
  assert.match(source, /自动重试一次/, "要说明只自动重试一次");
  assert.match(source, /主动取消的任务不会重试/, "要说明主动取消不重试");
});
