// 「对比范围」分析撞上后端互斥锁时的重试策略（node --test 覆盖）。
//
// 背景（真实反馈）：后端 checkConflicts 用 TryLock，同一时间只允许一轮冲突扫描；
// 自动复检（GetConflictBadges → RecheckConflictsNow）在每次游戏侧状态变化后都会跑一次全量扫描。
// 两者撞上时前端原先直接放弃、并把结果清空 —— 于是整屏 Mod 都显示成“无冲突”，
// 用户看到的就是“分析开着，但结果不对”。

export const SCOPED_CONFLICT_LOCK_MAX_RETRIES = 8;

/** isConflictCheckBusyError 识别后端“正在检测中”的互斥锁拒绝。 */
export function isConflictCheckBusyError(message) {
  return String(message || "").includes("冲突检测正在进行中");
}

/** scopedConflictRetryDelay 指数退避（300ms 起，单次上限 2s）。 */
export function scopedConflictRetryDelay(attempt) {
  const safeAttempt = Math.max(1, Math.trunc(Number(attempt) || 1));
  return Math.min(2000, 300 * 2 ** (safeAttempt - 1));
}

/** shouldRetryScopedConflict 第 attempt 次失败后是否还值得再试。 */
export function shouldRetryScopedConflict(attempt) {
  const safeAttempt = Math.max(0, Math.trunc(Number(attempt) || 0));
  return safeAttempt <= SCOPED_CONFLICT_LOCK_MAX_RETRIES;
}
