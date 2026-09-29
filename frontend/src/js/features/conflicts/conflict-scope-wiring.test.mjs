// 「对比范围」弹窗的接线契约测试。
//
// 真实回归（用户反馈）：只勾选 1 个条件时，「满足全部 / 满足任一」怎么点都
// 看不到变化 —— 预览行只在打开弹窗时刷新一次，组合按钮也没有禁用提示。
// 这里锁住三件事：控件存在、绑定点唯一、预览刷新真的被调用。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const conflictsSource = readFileSync(new URL("./conflicts.js", import.meta.url), "utf8");
const appRuntimeSource = readFileSync(
  new URL("../app-runtime.js", import.meta.url),
  "utf8",
);
const indexHtml = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");

test("弹窗提供可实时刷新的预览行与组合方式提示", () => {
  assert.ok(
    indexHtml.includes('id="conflict-scope-preview"'),
    "缺少 #conflict-scope-preview：预览行是“点了没反应”的直接反馈位",
  );
  assert.ok(
    indexHtml.includes('id="conflict-scope-mode-hint"'),
    "缺少 #conflict-scope-mode-hint：单条件时要在这里解释组合方式为何不生效",
  );
  assert.ok(
    indexHtml.includes('data-conflict-match-mode="and"') &&
      indexHtml.includes('data-conflict-match-mode="or"'),
    "缺少组合方式按钮",
  );
});

test("conflicts.js 绑定弹窗内的输入并刷新预览", () => {
  assert.ok(
    conflictsSource.includes("function bindConflictScopeDialogEvents"),
    "缺少 bindConflictScopeDialogEvents",
  );
  const body = conflictsSource.slice(
    conflictsSource.indexOf("function bindConflictScopeDialogEvents"),
  );
  assert.ok(body.includes("[data-conflict-match-mode]"), "组合方式按钮必须在 conflicts.js 里绑定");
  assert.ok(body.includes("[data-conflict-scope-rule]"), "条件勾选必须触发预览刷新");
  assert.ok(body.includes("refreshConflictScopeDialogPreview"), "绑定里必须调用预览刷新");
  assert.ok(
    conflictsSource.includes("serializeConflictScopeOptions"),
    "要用序列化比较判断“改了但还没应用”，预览才会显示“将按：…”",
  );
  assert.ok(
    conflictsSource.includes('button.disabled = disabled'),
    "只有 1 个条件时要禁用组合方式按钮",
  );
  assert.ok(
    /DOMContentLoaded,\s*bindConflictScopeDialogEvents/.test(conflictsSource) ||
      /bindConflictScopeDialogEvents\(\)/.test(conflictsSource),
    "绑定必须在模块加载（或 DOMContentLoaded）时执行一次",
  );
});

test("app-runtime.js 不再重复绑定组合方式（避免监听顺序读到旧状态）", () => {
  assert.ok(
    !appRuntimeSource.includes('querySelectorAll("[data-conflict-match-mode]")'),
    "组合方式按钮的绑定已收口到 conflicts.js，别处不应再绑",
  );
  assert.ok(
    !appRuntimeSource.includes('data-conflict-scope-rule="tag"'),
    "标签勾选同样由 conflicts.js 处理（它要在同一次事件里刷新预览）",
  );
});
