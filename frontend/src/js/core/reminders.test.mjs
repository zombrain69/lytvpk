import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import {
	REMINDER_KEYS,
	isReminderActive,
	persistReminderSession,
	reminderAskedThisSession,
	reminderChoiceThisSession,
	reminderSuppressedThisSession,
	reminderToggleHtml,
	resetReminderSessionCacheForTest,
} from "./reminders.mjs";

const moduleDir = dirname(fileURLToPath(import.meta.url));

test("键集合与来源守卫（singleton-conflict / vpk-risk）", () => {
	assert.deepEqual(Object.values(REMINDER_KEYS).sort(), ["singleton-conflict", "vpk-risk"]);
	// Go 侧与前端同一套键（源码级保鲜）。
	const goSource = readFileSync(join(moduleDir, "../../../../internal/app/reminders.go"), "utf8");
	assert.match(goSource, /ReminderSessionState/);
});

test("isReminderActive：设置关闭或本次运行静音都要隐藏", () => {
	resetReminderSessionCacheForTest();
	assert.equal(isReminderActive(REMINDER_KEYS.vpkRisk, {}), true);
	assert.equal(isReminderActive(REMINDER_KEYS.vpkRisk, { remindersHidden: { "vpk-risk": true } }), false);
});

test("会话级：首次给勾选框；记录后静音 + choice 记忆；勾选框不再出现", async () => {
	resetReminderSessionCacheForTest();
	assert.equal(reminderAskedThisSession(REMINDER_KEYS.singletonConflict), false);
	assert.match(reminderToggleHtml(REMINDER_KEYS.singletonConflict, "不再显示"), /checkbox/);

	await persistReminderSession(REMINDER_KEYS.singletonConflict, true, "replace");
	assert.equal(reminderAskedThisSession(REMINDER_KEYS.singletonConflict), true);
	assert.equal(reminderSuppressedThisSession(REMINDER_KEYS.singletonConflict), true);
	assert.equal(reminderChoiceThisSession(REMINDER_KEYS.singletonConflict), "replace");
	assert.equal(reminderToggleHtml(REMINDER_KEYS.singletonConflict, "不再显示"), "");
	assert.equal(isReminderActive(REMINDER_KEYS.singletonConflict, {}), false);
});

test("接线守卫：三处弹窗按体系接好", () => {
	const read = (relative) => readFileSync(join(moduleDir, relative), "utf8");
	const ops = read("../features/file-list/operations.js");
	assert.match(ops, /reminderSuppressedThisSession\(REMINDER_KEYS\.singletonConflict\)/);
	const singleton = read("../features/conflicts/singleton-conflict.js");
	assert.match(singleton, /recordReminderSessionChoice/);
	assert.match(singleton, /singleton-conflict-session-row/);
	const risk = read("../features/file-list/vpk-risk-warning.js");
	assert.match(risk, /isReminderActive\(REMINDER_KEYS\.vpkRisk/);
	assert.match(risk, /recordReminderSessionChoice\(REMINDER_KEYS\.vpkRisk/);
});
