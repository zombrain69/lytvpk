// 「编辑智能体提示词」对话框：默认用 LytVPK 内置的一套，用户可以保存自己的版本或恢复默认。
//
// 编辑器里展示的是**模板**（保留 {{...}} 占位符）：真正复制/导出给智能体时，
// 后端会把占位符替换成本机真实路径（见 GetGroupSuggestionAgentPrompt）。

import { showConfirmModal } from "../modals/confirm.js";
import { showError, showNotification } from "../../core/toast.js";
import {
  GetGroupSuggestionAgentPromptState,
  ResetGroupSuggestionAgentPrompt,
  SaveGroupSuggestionAgentPrompt,
} from "../../../../wailsjs/go/app/App";
import {
  formatPromptStatus,
  insertPromptPlaceholder,
  summarizePromptChange,
} from "./agent-prompt-view.mjs";

let promptState = null;

function element(id) {
  return document.getElementById(id);
}

function refreshStatus() {
  const status = element("agent-prompt-status");
  const textarea = element("agent-prompt-text");
  if (!status) return;
  status.textContent = formatPromptStatus(promptState, textarea?.value ?? "");
}

function renderPlaceholders() {
  const list = element("agent-prompt-placeholder-list");
  if (!list) return;
  list.replaceChildren();
  (promptState?.placeholders || []).forEach((placeholder) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "agent-prompt-placeholder";
    button.textContent = placeholder;
    button.title = "在光标处插入这个占位符";
    button.addEventListener("click", () => {
      const textarea = element("agent-prompt-text");
      if (!textarea) return;
      const applied = insertPromptPlaceholder(
        textarea.value,
        placeholder,
        textarea.selectionStart,
        textarea.selectionEnd,
      );
      textarea.value = applied.text;
      textarea.focus();
      textarea.setSelectionRange(applied.caret, applied.caret);
      refreshStatus();
    });
    list.appendChild(button);
  });
}

/** openAgentPromptEditor 打开编辑器并载入当前状态。 */
export async function openAgentPromptEditor() {
  const modal = element("agent-prompt-modal");
  if (!modal) return;
  try {
    promptState = await GetGroupSuggestionAgentPromptState();
  } catch (error) {
    showError("读取提示词失败: " + String(error?.message || error));
    return;
  }
  const textarea = element("agent-prompt-text");
  if (textarea) textarea.value = promptState?.text || "";
  renderPlaceholders();
  refreshStatus();
  modal.classList.remove("hidden");
  textarea?.focus();
}

export function closeAgentPromptEditor() {
  element("agent-prompt-modal")?.classList.add("hidden");
}

async function saveAgentPrompt() {
  const textarea = element("agent-prompt-text");
  const text = String(textarea?.value || "");
  if (!text.trim()) {
    showError("提示词不能为空；如需回到内置版本请点「恢复默认」");
    return;
  }
  const button = element("agent-prompt-save-btn");
  if (button) button.disabled = true;
  try {
    await SaveGroupSuggestionAgentPrompt(text);
    promptState = await GetGroupSuggestionAgentPromptState();
    if (textarea) textarea.value = promptState?.text || text;
    refreshStatus();
    showNotification(summarizePromptChange("save", promptState), "success");
    closeAgentPromptEditor();
  } catch (error) {
    showError("保存提示词失败: " + String(error?.message || error));
  } finally {
    if (button) button.disabled = false;
  }
}

function resetAgentPrompt() {
  showConfirmModal(
    "恢复默认提示词",
    "会把自定义提示词删掉，回到 LytVPK 内置的那一套（含处理范围、建议文件格式与自检要求）。\n" +
      "已经复制/导出给智能体的文件不受影响。是否继续？",
    async () => {
      try {
        await ResetGroupSuggestionAgentPrompt();
        promptState = await GetGroupSuggestionAgentPromptState();
        const textarea = element("agent-prompt-text");
        if (textarea) textarea.value = promptState?.text || "";
        refreshStatus();
        showNotification(summarizePromptChange("reset", promptState), "success");
      } catch (error) {
        showError("恢复默认失败: " + String(error?.message || error));
      }
    },
  );
}

let editorBound = false;

/** initAgentPromptEditor 绑定按钮（幂等）。 */
export function initAgentPromptEditor() {
  if (editorBound) return;
  editorBound = true;
  element("agent-prompt-save-btn")?.addEventListener("click", () => void saveAgentPrompt());
  element("agent-prompt-reset-btn")?.addEventListener("click", resetAgentPrompt);
  element("agent-prompt-cancel-btn")?.addEventListener("click", closeAgentPromptEditor);
  element("agent-prompt-close-btn")?.addEventListener("click", closeAgentPromptEditor);
  element("agent-prompt-text")?.addEventListener("input", refreshStatus);
}
