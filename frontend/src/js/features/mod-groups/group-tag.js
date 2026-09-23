// 「给这组打标签」对话框：把策略组 / 导入建议沉淀成一个统一标签。
//
// 方向与「分组建议」相反：分组建议是"标签能不能代替组"，这里是"用组来补标签"。
// 外部智能体导入的高精度建议正是最好的标签来源 —— 它已经帮你判断了这批 Mod 属于同一类。
//
// 只调用 ApplyTagToModKeys（后端复用 SetVPKTags）；文件名变化后的键改绑由后端完成。

import { showError, showNotification } from "../../core/toast.js";
import {
  ApplyTagToModKeys,
  GetGroupTagSuggestions,
} from "../../../../wailsjs/go/app/App";
import { refreshModGroupMembershipState } from "./group-state.mjs";
import { normalizeGroupKey } from "./group-view.mjs";
import { formatTagApplySummary, formatTagSuggestionLine } from "./group-tag-view.mjs";

let tagDialogState = null;

function element(id) {
  return document.getElementById(id);
}

function renderPreview() {
  const preview = element("group-tag-preview");
  if (!preview || !tagDialogState) return;
  preview.replaceChildren();
  const { names, alreadyTagged, keyCount } = tagDialogState;
  const line = document.createElement("div");
  line.className = "group-tag-preview-line";
  line.textContent =
    `这一组共 ${keyCount} 个成员` +
    (alreadyTagged > 0 ? `，其中 ${alreadyTagged} 个已经带这个标签（会跳过）` : "");
  preview.appendChild(line);
  if (names.length > 0) {
    const list = document.createElement("div");
    list.className = "group-tag-preview-names";
    names.slice(0, 12).forEach((name) => {
      const chip = document.createElement("span");
      chip.className = "group-tag-preview-name";
      chip.textContent = name;
      chip.title = name;
      list.appendChild(chip);
    });
    if (names.length > 12) {
      const more = document.createElement("span");
      more.className = "group-tag-preview-name is-more";
      more.textContent = `…还有 ${names.length - 12} 个`;
      list.appendChild(more);
    }
    preview.appendChild(list);
  }
}

/**
 * openGroupTagDialog 打开"给这组打标签"对话框。
 * @param {object} options
 * @param {string} options.groupId 策略组 ID
 * @param {string} options.groupName 组名（用于默认标签）
 * @param {string[]} options.keys 成员的 addonlist 键
 * @param {string[]} [options.names] 成员显示名
 * @param {() => void} [options.onDone] 完成后的刷新回调
 */
export async function openGroupTagDialog({ groupId, groupName, keys, names = [], onDone } = {}) {
  const modal = element("group-tag-modal");
  if (!modal) return;
  const targetKeys = [...new Set((keys || []).map((key) => normalizeGroupKey(key)).filter(Boolean))];
  if (targetKeys.length === 0) {
    showError("这个组还没有可用的成员键");
    return;
  }

  // 后端已经算好"这组该补什么标签"：优先用它（含已有共同标签、组名、主体识别三种来源）。
  let suggested = "";
  let origin = "";
  let alreadyTagged = 0;
  try {
    const suggestions = (await GetGroupTagSuggestions()) || [];
    const match = suggestions.find((item) => item.groupId === groupId);
    if (match) {
      suggested = match.tag || "";
      origin = match.tagOrigin || "";
      alreadyTagged = Number(match.alreadyTagged || 0);
    }
  } catch (error) {
    console.warn("读取标签建议失败:", error);
  }

  tagDialogState = {
    groupId,
    groupName: String(groupName || ""),
    keys: targetKeys,
    names: (names || []).length > 0 ? names : targetKeys,
    keyCount: targetKeys.length,
    alreadyTagged,
    onDone,
  };

  const title = element("group-tag-title");
  if (title) title.textContent = `给「${tagDialogState.groupName || groupId}」打标签`;
  const summary = element("group-tag-summary");
  if (summary) {
    summary.textContent = formatTagSuggestionLine(suggested, origin);
  }
  const input = element("group-tag-input");
  if (input) input.value = suggested;

  renderPreview();
  modal.classList.remove("hidden");
  input?.focus();
}

export function closeGroupTagDialog() {
  element("group-tag-modal")?.classList.add("hidden");
  tagDialogState = null;
}

async function applyGroupTag() {
  if (!tagDialogState) return;
  const input = element("group-tag-input");
  const tag = String(input?.value || "").trim();
  if (!tag) {
    showError("请先填写标签名");
    input?.focus();
    return;
  }
  const button = element("group-tag-apply-btn");
  if (button) button.disabled = true;
  try {
    const result = await ApplyTagToModKeys(tag, tagDialogState.keys);
    const failed = (result?.failed || []).length;
    showNotification(
      formatTagApplySummary(result),
      failed > 0 ? "warning" : "success",
    );
    if (failed > 0 && Array.isArray(result?.reasons)) {
      console.warn("打标签失败详情:", result.reasons);
    }
    await refreshModGroupMembershipState();
    if (typeof tagDialogState.onDone === "function") tagDialogState.onDone();
    closeGroupTagDialog();
  } catch (error) {
    showError("打标签失败: " + String(error?.message || error));
    if (button) button.disabled = false;
  }
}

let tagDialogBound = false;

/** initGroupTagDialog 绑定对话框按钮（幂等）。 */
export function initGroupTagDialog() {
  if (tagDialogBound) return;
  tagDialogBound = true;
  element("group-tag-apply-btn")?.addEventListener("click", () => void applyGroupTag());
  element("group-tag-cancel-btn")?.addEventListener("click", closeGroupTagDialog);
  element("group-tag-close-btn")?.addEventListener("click", closeGroupTagDialog);
}
