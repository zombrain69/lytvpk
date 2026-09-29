// “对比范围分析偶尔显示不对”的接线契约。
//
// 真实回归（用户反馈）：筛选/开启分析后，有时整屏变成“无冲突”。
// 根因是后端 TryLock 被自动复检占用时，前端直接放弃并清空结果 ——
// 于是 enabled=true + result=null，所有卡片都渲染成“无冲突”。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const conflicts = readFileSync(new URL("./conflicts.js", import.meta.url), "utf8");
const recheck = readFileSync(new URL("./conflict-recheck.js", import.meta.url), "utf8");
const render = readFileSync(
  new URL("../file-list/render.js", import.meta.url),
  "utf8",
);

test("撞上后端互斥锁时保持“分析中”并自动重试", () => {
  assert.ok(
    conflicts.includes("isConflictCheckBusyError"),
    "必须区分“正在检测中”和其它失败",
  );
  assert.ok(
    conflicts.includes("shouldRetryScopedConflict"),
    "互斥锁冲突要走重试策略，而不是直接放弃",
  );
  const busyBranch = conflicts.slice(
    conflicts.indexOf("if (isConflictCheckBusyError(errorMessage))"),
  );
  assert.ok(
    busyBranch.includes("appState.conflictAnalysisLoading = true"),
    "重试期间要保持 loading，否则卡片会退化成“无冲突”",
  );
  assert.ok(
    /scopedConflictTimer = setTimeout\(/.test(busyBranch),
    "要安排下一次重试",
  );
  assert.ok(
    busyBranch.includes("retry: true"),
    "重试要带 retry 标记，避免把重试计数重置掉",
  );
});

test("没有结果时角标显示“待分析”，不再谎报“无冲突”", () => {
  assert.ok(
    render.includes("const analyzed = Boolean(appState.conflictAnalysisResult)"),
    "角标模型要知道“本轮分析是否有结果”",
  );
  assert.ok(
    render.includes('const stateClass = model.analyzed ? "none" : "pending"'),
    "没有结果时用 pending 样式，和“确实没有重叠”区分开",
  );
});

test("自动复检完成后会重算对比范围结果", () => {
  assert.ok(
    recheck.includes('"conflict-badges-refreshed"'),
    "复检完成要广播事件（后端锁此时已释放）",
  );
  assert.ok(
    conflicts.includes('window.addEventListener("conflict-badges-refreshed"'),
    "对比范围分析要监听复检完成事件",
  );
  assert.ok(
    conflicts.includes("scheduleScopedConflictAnalysis();"),
    "监听里要触发重新分析（自带防抖与开关判断）",
  );
});
