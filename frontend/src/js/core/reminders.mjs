// 「提醒窗口」统一开关体系（glue 层；键与 internal/app/reminders.go 同一套）。
//
//   - 设置默认：config.json 的 remindersHidden[key] === true = 每次启动应用默认不显示该提醒；
//   - 会话级：每个提醒在"本次运行第一次弹出"时给一次「本次运行期间不再显示」勾选，
//     选择只保留到应用关闭（Go 进程内存态；重开应用即重置，回落到设置默认）；
//   - Choice：会话级选择可带一个动作（同类冲突的 replace / coexist），
//     静音后按当时选择自动处理。
//
// 用法（弹窗侧）：
//   弹出前：先 ensureReminderSessionLoaded；再用 isReminderActive 判断是否放行/渲染；
//   渲染时：把 reminderToggleHtml 的输出填进容器；
//   关闭时：调用 recordReminderSessionChoice 记录本次选择。

import { GetReminderSessionStates, SetReminderSessionState } from "../../../wailsjs/go/app/App.js";

export const REMINDER_KEYS = {
  singletonConflict: "singleton-conflict",
  vpkRisk: "vpk-risk",
};

let sessionStates = {};
let sessionLoaded = false;
let loadingPromise = null;

/** ensureReminderSessionLoaded 懒加载会话状态（每次运行只拉取一次；失败按全空处理）。 */
export function ensureReminderSessionLoaded() {
  if (sessionLoaded) return Promise.resolve();
  if (loadingPromise) return loadingPromise;
  loadingPromise = (async () => {
    try {
      const states = await GetReminderSessionStates();
      sessionStates = states && typeof states === "object" ? states : {};
    } catch (error) {
      console.warn("读取提醒会话状态失败（按全空处理）:", error);
      sessionStates = {};
    }
    sessionLoaded = true;
  })();
  return loadingPromise;
}

function stateOf(key) {
  const entry = sessionStates[key];
  return entry && typeof entry === "object"
    ? { asked: entry.asked === true, suppressed: entry.suppressed === true, choice: String(entry.choice || "") }
    : { asked: false, suppressed: false, choice: "" };
}

/** reminderHiddenBySetting 设置里的默认开关（true = 默认不显示）。 */
export function reminderHiddenBySetting(config, key) {
  return config?.remindersHidden?.[key] === true;
}

export function reminderAskedThisSession(key) {
  return stateOf(key).asked;
}

export function reminderSuppressedThisSession(key) {
  return stateOf(key).suppressed;
}

export function reminderChoiceThisSession(key) {
  return stateOf(key).choice;
}

/** isReminderActive：当前是否应该弹出该提醒（设置未关 && 本次运行未静音）。 */
export function isReminderActive(key, config) {
  return !reminderHiddenBySetting(config, key) && !reminderSuppressedThisSession(key);
}

function escapeHtmlText(value) {
  return String(value ?? "")
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&#39;");
}

/** reminderToggleHtml 生成「本次运行期间不再显示」勾选框；已问过则返回空串（只问第一次）。
 *  动态文本全部转义（label 通常来自本地文案目录；转义后即便传入外部文本也安全）。 */
export function reminderToggleHtml(key, label) {
  if (reminderAskedThisSession(key)) return "";
  const safeLabel = escapeHtmlText(label);
  return (
    `<label class="reminder-session-toggle" title="${safeLabel}">` +
    `<input type="checkbox" id="reminder-session-toggle-${escapeHtmlText(key)}">` +
    `<span>${safeLabel}</span></label>`
  );
}

/** recordReminderSessionChoice 提醒弹窗关闭时调用；勾选框不存在（非首次弹窗）则不改动任何状态。 */
export function recordReminderSessionChoice(key, choice = "") {
  const box = document.getElementById(`reminder-session-toggle-${key}`);
  if (!box) return;
  void persistReminderSession(key, box.checked, choice);
}

/** persistReminderSession 落会话状态（内存缓存立即生效；Go 写入失败不阻塞界面）。 */
export async function persistReminderSession(key, suppressed, choice = "") {
  const entry = { asked: true, suppressed: Boolean(suppressed), choice: String(choice || "") };
  sessionStates[key] = entry;
  try {
    await SetReminderSessionState(key, entry.suppressed, entry.choice);
  } catch (error) {
    console.warn("保存提醒会话状态失败（本次运行仍按缓存生效）:", error);
  }
}

/** resetReminderSessionCacheForTest 仅测试用：清空本地缓存。 */
export function resetReminderSessionCacheForTest() {
  sessionStates = {};
  sessionLoaded = false;
  loadingPromise = null;
}
