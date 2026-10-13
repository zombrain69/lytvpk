import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { SINGLETON_CONFLICT_MODES, normalizeSingletonConflictMode } from "./singleton-conflict-mode.mjs";

test("normalizeSingletonConflictMode：白名单归一化，未知/空回退 ask（保持旧行为）", () => {
	assert.deepEqual(SINGLETON_CONFLICT_MODES, ["ask", "replace", "coexist"]);
	assert.equal(normalizeSingletonConflictMode("replace"), "replace");
	assert.equal(normalizeSingletonConflictMode(" Coexist "), "coexist");
	assert.equal(normalizeSingletonConflictMode(""), "ask");
	assert.equal(normalizeSingletonConflictMode(undefined), "ask");
	assert.equal(normalizeSingletonConflictMode(null), "ask");
	assert.equal(normalizeSingletonConflictMode("auto"), "ask");
});

test("启用流程按设置分支：coexist 跳过检查；replace 在弹窗之前短路", () => {
	const moduleDir = dirname(fileURLToPath(import.meta.url));
	const source = readFileSync(join(moduleDir, "..", "file-list", "operations.js"), "utf8");
	assert.match(
		source,
		/normalizeSingletonConflictMode\(getConfig\(\)\.singletonConflictMode\)/,
		"启用流程必须读取设置",
	);
	assert.match(source, /conflictMode !== "coexist"/, "coexist 模式跳过检查与弹窗");
	const replaceIndex = source.indexOf('if (conflictMode === "replace")');
	const promptIndex = source.indexOf("await promptSingletonConflict(conflict)");
	assert.ok(replaceIndex > -1 && promptIndex > replaceIndex, "replace 模式应在弹窗之前短路");
});
