// 卡片渲染签名的静态契约测试。
//
// 背景（真实回归）：卡片会被复用，只有签名变化时才重绘。签名注释写着
// "contains every value rendered inside a card"，但后加的
// 「优先级分层角标」与「变更驱动复检角标」没有进签名，
// 于是保存/清除分层后卡片角标停留在旧值，直到重新扫描才更新。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const source = readFileSync(new URL("./render.js", import.meta.url), "utf8");

function functionBody(name) {
  const start = source.indexOf(`function ${name}(`);
  assert.ok(start >= 0, `应存在函数 ${name}`);
  const braceStart = source.indexOf("{", start);
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

test("卡片签名覆盖分层角标的值", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(
    body.includes("getFilePriorityEntry"),
    "签名必须包含分层信息，否则设置/清除分层后角标不会重绘",
  );
  assert.ok(body.includes("priority:"), "签名应显式记录 priority 字段");
});

test("卡片签名覆盖变更驱动复检角标的值", () => {
  const body = functionBody("getFileCardRenderSignature");
  assert.ok(
    body.includes("findConflictRecheckBadge"),
    "签名必须包含复检角标，否则自动复检后角标不会重绘",
  );
  assert.ok(body.includes("conflictRecheck:"), "签名应显式记录 conflictRecheck 字段");
});
