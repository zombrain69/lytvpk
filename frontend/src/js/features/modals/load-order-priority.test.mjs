// 优先级分层面板的静态契约测试。
//
// 背景（真实回归）：保存/清除分层曾经只刷新弹窗里的"有效分层"文字，没有刷新 Mod 列表，
// 于是卡片角标一直显示旧值（设置了分层 42 却仍是「优先级 #998」），
// 直到用户手动刷新或重新扫描才更新。这里用源码级断言把该行为钉住。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const SOURCE_URL = new URL("./load-order-priority.js", import.meta.url);
const source = readFileSync(SOURCE_URL, "utf8");

function functionBody(name) {
  const start = source.indexOf(`async function ${name}(`);
  const syncStart = start >= 0 ? start : source.indexOf(`function ${name}(`);
  assert.ok(syncStart >= 0, `应存在函数 ${name}`);
  const braceStart = source.indexOf("{", syncStart);
  let depth = 0;
  let index = braceStart;
  for (; index < source.length; index += 1) {
    const char = source[index];
    if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  return source.slice(braceStart, index + 1);
}

test("保存分层后必须刷新列表角标", () => {
  const body = functionBody("saveCurrentTier");
  assert.ok(
    body.includes("refreshAfterTierChange("),
    "saveCurrentTier 必须调用 refreshAfterTierChange()，否则列表角标会停留在旧分层",
  );
});

test("清除分层后必须刷新列表角标", () => {
  const body = functionBody("clearCurrentTier");
  assert.ok(
    body.includes("refreshAfterTierChange("),
    "clearCurrentTier 必须调用 refreshAfterTierChange()，否则列表角标会停留在已清除的分层",
  );
});

test("refreshAfterTierChange 覆盖宿主回调与兜底刷新两条路径", () => {
  const body = functionBody("refreshAfterTierChange");
  assert.ok(body.includes("onAppliedCallback"), "应优先使用宿主弹窗提供的刷新回调");
  assert.ok(body.includes("refreshLoadOrderMap"), "没有回调时应至少刷新加载顺序映射");
  assert.ok(body.includes("renderFileList"), "没有回调时应至少重绘列表");
});

// 真实回归：宿主刷新函数曾经在「不是按加载顺序排序」时直接 return，
// 导致保存/清除分层后列表完全没重绘，卡片角标停留在旧值。
test("刷新列表的函数在任何排序方式下都会重绘", () => {
  const policySource = readFileSync(new URL("./load-order-policy.js", import.meta.url), "utf8");
  const start = policySource.indexOf("async function refreshModListAfterLoadOrderChange()");
  assert.ok(start >= 0, "load-order-policy.js 应包含 refreshModListAfterLoadOrderChange");
  const braceStart = policySource.indexOf("{", start);
  let depth = 0;
  let index = braceStart;
  for (; index < policySource.length; index += 1) {
    const char = policySource[index];
    if (char === "{") depth += 1;
    else if (char === "}") {
      depth -= 1;
      if (depth === 0) break;
    }
  }
  const body = policySource.slice(braceStart, index + 1);

  assert.ok(
    body.includes("refreshLoadOrderMap"),
    "无论排序方式如何，都必须刷新加载顺序/分层映射，否则角标是旧值",
  );
  assert.ok(
    body.includes("renderFileList"),
    "非按加载顺序排序时也必须重绘列表，否则卡片角标不会更新",
  );
  assert.ok(
    !/if\s*\(\s*appState\.sortType\s*!==\s*"loadOrder"\s*\)\s*return/.test(body),
    "不得在非加载顺序排序时直接 return",
  );
});
