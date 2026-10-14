import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const moduleDir = dirname(fileURLToPath(import.meta.url));

test("同类互斥弹窗：逐条勾选 + 定级 + 默认只勾选严重档（源码级守卫）", () => {
	const source = readFileSync(join(moduleDir, "singleton-conflict.js"), "utf8");
	assert.match(source, /data-singleton-path/, "候选条目应带勾选框");
	assert.match(source, /severity\s*===\s*"severe"/, "严重档判定应在前端可见");
	assert.match(source, /replaceSelected/, "按钮文案应随所选数量变化");
	assert.match(source, /resolve\(\{ choice:.*paths \}\)/, "replace 应把勾选路径返回给调用方");
});

test("启用流程：关闭动作按勾选路径执行；自动处理只针对严重档", () => {
	const ops = readFileSync(join(moduleDir, "..", "file-list", "operations.js"), "utf8");
	assert.match(ops, /disableSingletonConflictPaths\(result\.paths\)/, "手动流程应按勾选执行");
	assert.match(ops, /severeSingletonPaths\(conflict\)/, "自动流程应只处理严重档");
	assert.doesNotMatch(ops, /disableSingletonConflicts\(/, "旧的全量关闭入口应已移除");
});
