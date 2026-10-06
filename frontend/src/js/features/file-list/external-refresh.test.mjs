import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import path from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { createExternalRefreshScheduler } from "./external-refresh.mjs";

const here = path.dirname(fileURLToPath(import.meta.url));

function createHarness() {
  const state = { refreshes: 0, errors: [], resolve: null };
  const scheduler = createExternalRefreshScheduler({
    runRefresh: () => {
      state.refreshes += 1;
      return new Promise((resolve) => {
        state.resolve = resolve;
      });
    },
    onError: (error) => state.errors.push(error),
  });
  const settle = async () => {
    const resolve = state.resolve;
    state.resolve = null;
    if (resolve) resolve();
  };
  return { state, scheduler, settle };
}

test("窗口可见：一次外部改动触发一次静默刷新", async () => {
  const { state, scheduler, settle } = createHarness();
  const result = scheduler.notify();
  assert.equal(state.refreshes, 1);
  await settle();
  assert.equal(await result, "refreshed");
  assert.equal(scheduler.pending, false);
  assert.equal(scheduler.running, false);
});

test("刷新进行中又来变化：合并成一轮补刷，不并发", async () => {
  const { state, scheduler, settle } = createHarness();
  const first = scheduler.notify();
  assert.equal(state.refreshes, 1);

  // 第一轮还没结束时连来三次变化
  scheduler.notify();
  scheduler.notify();
  scheduler.notify();
  assert.equal(state.refreshes, 1, "运行中不该并发再起一轮");

  await settle(); // 第 1 轮结束 → 补第 2 轮
  await new Promise((resolve) => setImmediate(resolve));
  assert.equal(state.refreshes, 2, "挤掉的变化应该在下一轮补上");
  await settle(); // 第 2 轮结束
  assert.equal(await first, "refreshed");
  assert.equal(scheduler.pending, false);
  assert.equal(scheduler.running, false);
});

test("刷新失败只上报，不抛给调用点", async () => {
  const errors = [];
  const scheduler = createExternalRefreshScheduler({
    runRefresh: () => Promise.reject(new Error("扫描失败")),
    onError: (error) => errors.push(error),
  });
  await scheduler.notify();
  assert.equal(errors.length, 1);
  assert.match(String(errors[0].message), /扫描失败/);
  assert.equal(scheduler.running, false, "失败后仍要解锁，否则再也不刷了");
});

test("filters.js 的静默刷新契约：silent 不弹错误、也不显示加载遮罩", () => {
  const source = readFileSync(path.resolve(here, "filters.js"), "utf8");
  assert.match(source, /export async function refreshFilesKeepFilter\(options = \{\}\)/);
  assert.match(source, /if \(!silent\) showNotification\("请先选择目录", "info"\)/);
  assert.match(source, /if \(!silent\) showRefreshLoadingOnce\(\)/);
  assert.match(source, /if \(refreshFilesLoadingVisible\) showError\("刷新失败: " \+ error\)/);
});
