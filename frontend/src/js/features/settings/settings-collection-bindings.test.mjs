import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";
import test from "node:test";

// 真机回归（2026-09-30）：
// 「设置 → 工坊设置 → 工坊合集」的保存 / 检查全部 / 检查更新 / 删除四个入口，
// 都在调用一个**不存在**的 refresh()（那是相邻两个绑定函数各自作用域里的局部函数）。
// 真机表现为：合集明明保存成功，状态栏却写「保存合集失败: refresh is not defined」，
// 记录列表也不刷新，于是「检查更新 / 下载缺失成员 / 删除记录」按钮一直不出现。
//
// 这里把两个约束钉住：
//   1) 这块代码里不许再出现裸 refresh()；
//   2) 刷新必须走注入进来的 deps.refreshAddonListPanel（设置页整页重渲染）。

const here = path.dirname(fileURLToPath(import.meta.url));
const settingsPagePath = path.resolve(here, "settings-page.js");

function workshopCollectionBlock(source) {
  // 工作区是 CRLF，先归一化再按行边界截取，避免 \n}\n 找不到结尾。
  const normalized = source.replace(/\r\n/g, "\n");
  const start = normalized.indexOf("// 工坊合集实体化");
  assert.ok(start >= 0, "settings-page.js 里找不到「工坊合集实体化」绑定代码");
  const end = normalized.indexOf("\n}\n", start);
  assert.ok(end > start, "找不到工坊合集绑定代码的结尾（bind*Settings 函数结束）");
  return normalized.slice(start, end);
}

test("工坊合集绑定不再调用未定义的 refresh()", () => {
  const source = readFileSync(settingsPagePath, "utf8");
  const block = workshopCollectionBlock(source);

  assert.doesNotMatch(
    block,
    /(^|[^.\w])refresh\s*\(\s*\)/m,
    "块里又出现了裸 refresh() 调用：它不在这个函数作用域里，真机会抛 refresh is not defined",
  );
  assert.match(
    block,
    /const\s+refreshCollections\s*=/,
    "刷新要先在本块定义 refreshCollections（转发 deps.refreshAddonListPanel）",
  );
  assert.match(
    block,
    /deps\.refreshAddonListPanel/,
    "刷新必须调用注入的 deps.refreshAddonListPanel，不能自己找别的函数",
  );
});

// 同一轮真机还发现：保存 / 检查更新 / 删除成功后都会立刻整页重渲染，
// 状态文字只写在 DOM 里的话会被新的模板冲掉（用户看不到"已保存…"、
// "新增 X 个 / 本地缺少 Y 个"）。所以状态要有一份模块级副本，并参与渲染。
test("工坊合集的状态文字能跨整页重渲染保留", () => {
  const source = readFileSync(settingsPagePath, "utf8");
  assert.match(
    source,
    /let\s+workshopCollectionStatusText\s*=\s*""/,
    "缺少模块级 workshopCollectionStatusText：状态会被重渲染清空",
  );
  assert.match(
    source,
    /settings-collection-status[\s\S]{0,120}escapeHtml\(workshopCollectionStatusText \|\| workshopCollectionsError\)/,
    "渲染时要把保存过的状态文字带上（状态优先，其次才是读取错误）",
  );
  assert.match(
    source,
    /workshopCollectionStatusText = message \|\| ""/,
    "setCollectionStatus 要同步写模块级副本",
  );
});
