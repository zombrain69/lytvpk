import assert from "node:assert/strict";
import test from "node:test";

import {
  SCOPED_CONFLICT_LOCK_MAX_RETRIES,
  isConflictCheckBusyError,
  scopedConflictRetryDelay,
  shouldRetryScopedConflict,
} from "./scoped-conflict-retry.mjs";

test("只有后端互斥锁拒绝才算“忙”，其它错误照旧走失败分支", () => {
  assert.equal(isConflictCheckBusyError("冲突检测正在进行中，请稍候"), true);
  assert.equal(isConflictCheckBusyError(new Error("冲突检测正在进行中，请稍候")), true);
  assert.equal(isConflictCheckBusyError("未选择L4D2目录"), false);
  assert.equal(isConflictCheckBusyError(""), false);
  assert.equal(isConflictCheckBusyError(null), false);
});

test("重试延迟指数退避且有上限", () => {
  assert.equal(scopedConflictRetryDelay(1), 300);
  assert.equal(scopedConflictRetryDelay(2), 600);
  assert.equal(scopedConflictRetryDelay(3), 1200);
  assert.equal(scopedConflictRetryDelay(4), 2000);
  assert.equal(scopedConflictRetryDelay(99), 2000);
  assert.equal(scopedConflictRetryDelay(0), 300);
});

test("重试次数到上限后不再重试", () => {
  assert.equal(shouldRetryScopedConflict(1), true);
  assert.equal(shouldRetryScopedConflict(SCOPED_CONFLICT_LOCK_MAX_RETRIES), true);
  assert.equal(shouldRetryScopedConflict(SCOPED_CONFLICT_LOCK_MAX_RETRIES + 1), false);
  assert.equal(shouldRetryScopedConflict(0), true);
});
