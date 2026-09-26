import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 下载暂停 / 继续（对齐 FireAxe DownloadService 的 Pause / Resume）：
// 后端已有 PauseDownloadTask / ResumeDownloadTask，这里锁住界面接线，
// 免得出现"后端支持了但界面没有入口"的半成品。

const source = readFileSync(new URL("./task-list.js", import.meta.url), "utf8");

test("任务列表接入暂停与继续两个后端方法", () => {
  assert.match(source, /PauseDownloadTask,/, "缺少 PauseDownloadTask 导入");
  assert.match(source, /ResumeDownloadTask,/, "缺少 ResumeDownloadTask 导入");
  assert.match(source, /await PauseDownloadTask\(task\.id\)/, "暂停按钮要真的调用后端");
  assert.match(source, /await ResumeDownloadTask\(task\.id\)/, "继续按钮要真的调用后端");
});

test("三种状态各有正确的按钮组合", () => {
  assert.match(source, /pause-task-btn/, "进行中的任务要有暂停按钮");
  assert.match(source, /resume-task-btn/, "暂停中的任务要有继续按钮");
  // 暂停中的任务同时要能"取消并丢弃断点"。
  assert.match(source, /task\.status === "paused"[\s\S]{0,700}cancel-task-btn/, "暂停中也要能取消");
  // 文案要说清"继续是断点续传"而不是重下。
  assert.match(source, /只补没下完的区块/, "继续按钮要说明是断点续传");
  assert.match(source, /已完成的区块会保留/, "暂停按钮要说明会保留进度");
});

test("暂停状态有独立的显示名与颜色，并排在失败之前", () => {
  assert.match(source, /paused: "已暂停"/, "缺少「已暂停」状态文案");
  assert.match(source, /paused: "#2196f3"/, "缺少暂停状态颜色");
  assert.match(source, /paused: 3,\s*interrupted: 3,\s*failed: 3,/, "暂停应与中断/失败同档排序");
});

test("任务列表的字号用 rem/变量，跟随「文字大小」档位", () => {
  // 硬编码 px 字号不会跟着「文字大小」设置变 —— 任务列表以前就是这样（11/12/14px）。
  const hardcoded = [...source.matchAll(/font-size:\s*[0-9]+px/g)].map((match) => match[0]);
  assert.deepEqual(hardcoded, [], `不应再用硬编码 px 字号：${hardcoded.join(", ")}`);
  assert.match(source, /font-size: 0\.875rem/, "标题字号应改为 rem");
  assert.match(source, /font-size: 0\.6875rem/, "次要文字应改为 rem");
  // 颜色也要走主题变量，才能配合"阅读舒适度"的高对比/柔和档。
  assert.match(source, /color: var\(--text-secondary\)/);
  assert.match(source, /color: var\(--text-tertiary\)/);
});
