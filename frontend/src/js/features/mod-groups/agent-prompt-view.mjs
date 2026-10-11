// 「编辑智能体提示词」的纯文案/编辑逻辑（与 DOM 无关，node --test 覆盖）。

import { t } from "../../core/i18n-runtime.js";

/** formatPromptStatus 生成编辑器顶部的状态行。 */
export function formatPromptStatus(state, currentText) {
  const savedAt = state?.savedAt ? t("strategyGroups.agentPrompt.status.savedAt", { time: state.savedAt }) : "";
  const base = state?.isCustom
    ? t("strategyGroups.agentPrompt.status.custom") + savedAt
    : t("strategyGroups.agentPrompt.status.default");
  // state.text 是"上一次加载/保存"的内容：和它不一致就说明编辑器里有未保存的修改。
  const dirty = typeof currentText === "string" && currentText !== state?.text;
  return dirty ? t("strategyGroups.agentPrompt.status.dirty", { base }) : base;
}

/** insertPromptPlaceholder 在光标处插入占位符，返回新文本与新光标位置。 */
export function insertPromptPlaceholder(text, placeholder, selectionStart, selectionEnd) {
  const source = String(text ?? "");
  const token = String(placeholder ?? "");
  const length = source.length;
  let start = Number.isFinite(selectionStart) ? Math.trunc(selectionStart) : length;
  let end = Number.isFinite(selectionEnd) ? Math.trunc(selectionEnd) : start;
  start = Math.min(Math.max(start, 0), length);
  end = Math.min(Math.max(end, start), length);
  const nextText = source.slice(0, start) + token + source.slice(end);
  return { text: nextText, caret: start + token.length };
}

/** summarizePromptChange 保存/恢复默认后的提示语。 */
export function summarizePromptChange(action, state) {
  if (action === "reset") {
    return state?.isCustom
      ? t("strategyGroups.agentPrompt.change.resetRetry")
      : t("strategyGroups.agentPrompt.change.resetDone");
  }
  return t("strategyGroups.agentPrompt.change.saved");
}
