// 策略组在 Mod 管理页的交互层：分组筛选、组徽标整组开关、整组优先级移动、分组推导建议。
//
// 三条安全约定（与后端一致）：
//   1. 推导只读扫描，不写任何文件；
//   2. 整组开关只改组成员在 addonlist.txt 里的 0/1，不重排、不动其它 Mod；
//   3. 整组优先级移动只写 priority.json（分层），要真正重排仍需在“编辑加载顺序”里点“按分层应用”。

import { appState } from "../state.js";
import { escapeHtml } from "../../core/utils.js";
import { showError, showNotification } from "../../core/toast.js";
import {
  CreateModStrategyGroupFromKeys,
  ClearExternalGroupSuggestions,
  ExportGroupingCatalogDialog,
  GetGroupSuggestionAgentPrompt,
  GetGroupSuggestionInboxPath,
  ImportGroupSuggestionsOpenDialog,
  PrepareGroupingWorkspace,
  SaveGroupSuggestionAgentPromptDialog,
  SetModStrategyGroupEnabled,
  ShiftModStrategyGroupPriorities,
  SuggestModGroups,
} from "../../../../wailsjs/go/app/App";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import { renderFileList } from "../file-list/render.js";
import { onModGroupMembershipChanged, refreshModGroupMembershipState } from "./group-state.mjs";
import {
  buildSuggestionFileIndex,
  defaultSuggestionSelection,
  describeSuggestionMember,
  formatGroupFilterLabel,
  formatGroupOptionLabel,
  formatGroupStrategy,
  formatSuggestionConfidence,
  formatSuggestionCreateSummary,
  formatSuggestionSignals,
  formatSuggestionSummary,
  suggestionMemberRows,
  normalizeGroupKey,
} from "./group-view.mjs";
import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";
import { showConfirmModal } from "../modals/confirm.js";
import { showPromptModal } from "../modals/prompt.js";
import { showFileDetail } from "../modals/detail.js";
import { moveFileToAddons, toggleFile, toggleGameEnabled } from "../file-list/operations.js";
import { formatPriorityLabel } from "../file-list/priority-label.mjs";

let controlsBound = false;
const suggestionSelections = new Map();

function element(id) {
  return document.getElementById(id);
}

/** refreshModGroupMembership 读取组归属并刷新筛选菜单（best-effort，不阻断页面）。 */
export async function refreshModGroupMembership({ silent = true } = {}) {
  return refreshModGroupMembershipState({ silent });
}

function renderGroupFilterMenu() {
  const menu = element("mod-group-filter-list");
  const text = element("mod-group-filter-text");
  if (text) {
    text.textContent = formatGroupFilterLabel(appState.activeGroupFilter, appState.groupFilterOptions);
  }
  if (!menu) return;
  menu.replaceChildren();

  const options = appState.groupFilterOptions || [];
  if (options.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-filter-empty";
    empty.textContent = "还没有策略组。可以先点「分组建议」自动推导，或在设置页用选中的 Mod 建组。";
    menu.appendChild(empty);
    return;
  }

  const header = document.createElement("div");
  header.className = "mod-group-filter-header";
  const headerText = document.createElement("span");
  headerText.textContent = "按分组筛选";
  const clearButton = document.createElement("button");
  clearButton.type = "button";
  clearButton.className = "btn-link";
  clearButton.textContent = "清空";
  clearButton.addEventListener("click", async (event) => {
    event.stopPropagation();
    appState.activeGroupFilter = new Set();
    renderGroupFilterMenu();
    await refreshFilesKeepFilter();
  });
  header.append(headerText, clearButton);
  menu.appendChild(header);

  options.forEach((option) => {
    const row = document.createElement("div");
    row.className = "mod-group-filter-row";

    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = appState.activeGroupFilter.has(option.id);
    checkbox.addEventListener("change", async () => {
      const next = new Set(appState.activeGroupFilter);
      if (checkbox.checked) next.add(option.id);
      else next.delete(option.id);
      appState.activeGroupFilter = next;
      renderGroupFilterMenu();
      await refreshFilesKeepFilter();
    });

    const label = document.createElement("button");
    label.type = "button";
    label.className = "mod-group-filter-name";
    label.textContent = formatGroupOptionLabel(option);
    label.title =
      `${formatGroupStrategy(option.strategy)} · 共 ${option.memberCount} 个成员` +
      (option.missingCount > 0
        ? ` · ${option.missingCount} 个成员的文件已不在列表里（放回同名文件会自动回到组里）`
        : "");
    label.addEventListener("click", async () => {
      checkbox.checked = !checkbox.checked;
      checkbox.dispatchEvent(new Event("change"));
    });

    const actions = document.createElement("span");
    actions.className = "mod-group-filter-actions";
    const moveUp = document.createElement("button");
    moveUp.type = "button";
    moveUp.className = "btn-link";
    moveUp.textContent = "↑ 前移";
    moveUp.title = "整组优先级前移 5 层（只写 priority.json，不会立刻重排）";
    moveUp.addEventListener("click", (event) => {
      event.stopPropagation();
      void shiftGroupPriority(option.id, option.name, 5);
    });
    const moveDown = document.createElement("button");
    moveDown.type = "button";
    moveDown.className = "btn-link";
    moveDown.textContent = "↓ 后移";
    moveDown.title = "整组优先级后移 5 层（只写 priority.json，不会立刻重排）";
    moveDown.addEventListener("click", (event) => {
      event.stopPropagation();
      void shiftGroupPriority(option.id, option.name, -5);
    });
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "btn-link";
    toggle.textContent = "整组开关";
    toggle.title = "把该组所有成员一起启用或关闭（只改 addonlist.txt 的 0/1）";
    toggle.addEventListener("click", (event) => {
      event.stopPropagation();
      void toggleGroupEnabled(option.id, option.name);
    });
    actions.append(moveUp, moveDown, toggle);

    row.append(checkbox, label, actions);
    menu.appendChild(row);
  });
}

async function toggleGroupEnabled(groupId, groupName) {
  const members = (appState.modGroupMemberships || []).filter((item) => item.groupId === groupId);
  const vote = groupEnabledVote(groupId);
  const nextEnabled = !vote.mostlyEnabled;
  // 使用应用内确认弹窗：WebView2 的原生 confirm 在锁屏/自动化环境下会阻塞页面。
  showConfirmModal(
    "整组开关",
    `把「${groupName}」的 ${members.length} 个成员一起${nextEnabled ? "启用" : "关闭"}。\n` +
      `（当前统计：启用 ${vote.enabled} 个、关闭 ${vote.disabled} 个）\n` +
      "只会修改 addonlist.txt 里的 0/1，不会重排顺序，也不会改动 Mod 文件。是否继续？",
    async () => {
      await applyGroupEnabledToggle(groupId, groupName, nextEnabled);
    },
  );
}

async function applyGroupEnabledToggle(groupId, groupName, nextEnabled) {
  try {
    const result = await SetModStrategyGroupEnabled(groupId, nextEnabled);
    const skipped = Array.isArray(result.skipped) ? result.skipped : [];
    showNotification(
      `「${groupName}」已整组${nextEnabled ? "启用" : "关闭"} ${(result.enabled?.length || 0) + (result.disabled?.length || 0)} 个成员` +
        (skipped.length > 0 ? `，${skipped.length} 个成员的文件已不在列表里，已跳过` : ""),
      skipped.length > 0 ? "info" : "success",
    );
    await refreshModGroupMembership();
    renderFileList();
    await refreshFilesKeepFilter();
  } catch (error) {
    showError("整组开关失败: " + String(error?.message || error));
  }
}

// groupEnabledVote 统计组内成员在 addonlist 里的开关情况，用于决定"整组开关"的方向。
function groupEnabledVote(groupId) {
  const keys = new Set(
    (appState.modGroupMemberships || [])
      .filter((item) => item.groupId === groupId)
      .map((item) => normalizeGroupKey(item.key)),
  );
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  let enabled = 0;
  let disabled = 0;
  for (const file of files) {
    const fileKeys = filePriorityKeys(file, appState.currentDirectory);
    if (!fileKeys.some((key) => keys.has(key))) continue;
    if (file.gameEnabled) enabled += 1;
    else if (file.gameStateKnown) disabled += 1;
  }
  return { enabled, disabled, mostlyEnabled: enabled >= disabled };
}

async function shiftGroupPriority(groupId, groupName, delta) {
  try {
    const result = await ShiftModStrategyGroupPriorities(groupId, delta);
    const moved = result?.moved?.length || 0;
    const skipped = result?.skipped?.length || 0;
    showNotification(
      `「${groupName}」已整组${delta > 0 ? "前移" : "后移"} ${Math.abs(delta)} 层（${moved} 个成员）` +
        (skipped > 0 ? `，${skipped} 个成员不在 addonlist 中已跳过` : "") +
        "；到“编辑加载顺序”点“按分层应用”后生效",
      skipped > 0 ? "info" : "success",
    );
    await refreshModGroupMembership();
    renderFileList();
  } catch (error) {
    showError("整组优先级移动失败: " + String(error?.message || error));
  }
}

// copyTextToClipboard 把文本放进剪贴板，优先用浏览器 API，失败再退回 Wails 绑定。
async function copyTextToClipboard(text) {
  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return true;
    }
  } catch (error) {
    console.warn("剪贴板 API 不可用:", error);
  }
  try {
    const binding = window.go?.app?.App?.ClipboardSetText;
    if (typeof binding === "function") {
      await binding(text);
      return true;
    }
  } catch (error) {
    console.warn("Wails 剪贴板绑定不可用:", error);
  }
  return false;
}

// prepareAgentWorkspace 一步到位：导出 Mod 清单 + 复制提示词（提示词里已填好路径）。
async function prepareAgentWorkspace() {
  try {
    const workspace = await PrepareGroupingWorkspace();
    if (!workspace?.catalogPath) {
      showError("准备材料失败：清单路径为空");
      return;
    }
    const copied = await copyTextToClipboard(workspace.prompt || "");
    showNotification(
      `已导出 Mod 清单（${workspace.modCount} 个）` + (copied ? "，提示词已复制到剪贴板" : "；提示词请点「复制智能体提示词」"),
      copied ? "success" : "info",
    );
    showConfirmModal(
      "交给智能体的材料已就绪",
      `<div style="line-height:1.6">` +
        `Mod 清单：<code style="word-break:break-all">${escapeHtml(workspace.catalogPath)}</code><br>` +
        `建议文件（智能体写完后放这里，然后点「重新推导」）：` +
        `<code style="word-break:break-all">${escapeHtml(workspace.inboxPath)}</code>` +
        `<p style="margin:10px 0 0">` +
        (copied
          ? "提示词已复制到剪贴板，直接粘贴给智能体（Codex / Claude 等）即可。"
          : "请点「复制智能体提示词」把提示词交给智能体。") +
        `</p><p style="margin:6px 0 0">提示词里已经写清了清单字段、标准格式、可选策略和硬性要求。</p>` +
        `</div>`,
      () => {},
      true,
    );
  } catch (error) {
    showError("准备材料失败: " + String(error?.message || error));
  }
}

// copyAgentPrompt 只复制提示词（不导出清单）。
async function copyAgentPrompt() {
  try {
    const prompt = await GetGroupSuggestionAgentPrompt();
    if (!prompt) {
      showError("提示词为空");
      return;
    }
    const copied = await copyTextToClipboard(prompt);
    if (copied) {
      showNotification("智能体提示词已复制到剪贴板", "success");
      return;
    }
    const path = await SaveGroupSuggestionAgentPromptDialog();
    if (path) {
      showNotification(`剪贴板不可用，已另存提示词：${path}`, "info");
    }
  } catch (error) {
    showError("复制提示词失败: " + String(error?.message || error));
  }
}

// saveAgentPrompt 把提示词另存为 Markdown 文件。
async function saveAgentPrompt() {
  try {
    const path = await SaveGroupSuggestionAgentPromptDialog();
    if (!path) {
      return;
    }
    showNotification(`提示词已保存：${path}`, "success");
  } catch (error) {
    showError("保存提示词失败: " + String(error?.message || error));
  }
}

// importExternalGroupSuggestions 导入标准格式的组建议文件并刷新弹窗。
async function importExternalGroupSuggestions() {
  try {
    const result = await ImportGroupSuggestionsOpenDialog();
    if (!result || !result.file) {
      return; // 用户取消
    }
    const warnings = Array.isArray(result.warnings) ? result.warnings : [];
    showNotification(
      `已导入 ${result.imported} 条建议（${result.memberCount} 个成员）` +
        (result.skipped > 0 ? `，跳过 ${result.skipped} 条` : ""),
      result.skipped > 0 || warnings.length > 0 ? "info" : "success",
    );
    await refreshModGroupSuggestions();
    if (warnings.length > 0) {
      showConfirmModal("导入结果", warnings.slice(0, 8).join("\n"), () => {});
    }
  } catch (error) {
    showError("导入建议失败: " + String(error?.message || error));
  }
}

// exportGroupingCatalog 导出分组用 Mod 清单（键、标题、作者、标签、主体、语音角色）。
async function exportGroupingCatalog() {
  try {
    const path = await ExportGroupingCatalogDialog();
    if (!path) {
      return; // 用户取消
    }
    const inbox = await GetGroupSuggestionInboxPath().catch(() => "");
    showNotification(`已导出 Mod 清单：${path}`, "success");
    if (inbox) {
      showConfirmModal(
        "交给大模型分析",
        `Mod 清单已导出：${path}\n\n` +
          "可以把这份清单交给大模型（或人工）分析，让它按标准格式写出组建议文件，然后回到这里点「导入建议文件…」。\n\n" +
          `建议文件的标准位置是：${inbox}（放进去后点「重新推导」也会自动读取）。`,
        () => {},
      );
    }
  } catch (error) {
    showError("导出 Mod 清单失败: " + String(error?.message || error));
  }
}

async function clearExternalGroupSuggestions() {
  try {
    await ClearExternalGroupSuggestions();
    showNotification("已清除导入的组建议", "success");
    await refreshModGroupSuggestions();
  } catch (error) {
    showError("清除导入建议失败: " + String(error?.message || error));
  }
}

/** openModGroupSuggestionModal 打开"分组建议"弹窗并重新推导。 */
export async function openModGroupSuggestionModal() {
  const modal = element("mod-group-suggest-modal");
  if (!modal) return;
  modal.classList.remove("hidden");
  await refreshModGroupSuggestions();
}

function closeModGroupSuggestionModal() {
  element("mod-group-suggest-modal")?.classList.add("hidden");
}

async function refreshModGroupSuggestions() {
  const list = element("mod-group-suggest-list");
  const summary = element("mod-group-suggest-summary");
  if (!list) return;
  list.replaceChildren();
  if (summary) summary.textContent = "正在推导…";
  try {
    const suggestions = (await SuggestModGroups()) || [];
    appState.groupSuggestions = suggestions;
    suggestionSelections.clear();
    suggestions.forEach((suggestion) => {
      suggestionSelections.set(suggestion.id, defaultSuggestionSelection(suggestion));
    });
    renderSuggestionList(suggestions);
  } catch (error) {
    if (summary) summary.textContent = "推导失败: " + String(error?.message || error);
  }
}

function renderSuggestionList(suggestions) {
  const list = element("mod-group-suggest-list");
  const summary = element("mod-group-suggest-summary");
  if (!list) return;
  list.replaceChildren();
  if (summary) {
    const externalCount = suggestions.filter((item) => item?.source === "external").length;
    summary.textContent =
      suggestions.length === 0
        ? "没有发现明显同组的 Mod（可能已经都分好组了）"
        : `共 ${suggestions.length} 条建议` +
          (externalCount > 0 ? `（其中 ${externalCount} 条来自导入的建议文件）` : "") +
          " · 客观证据（合集/文件夹结构/前缀/标签/作者）与小规模组优先";
  }
  if (suggestions.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-suggest-empty";
    empty.textContent = "可以先用标签或工坊合集整理出更明确的特征，再回来推导。";
    list.appendChild(empty);
    return;
  }

  suggestions.forEach((suggestion) => {
    const card = document.createElement("section");
    card.className = `mod-group-suggest-card confidence-${suggestion.confidence || "low"}`;

    const head = document.createElement("div");
    head.className = "mod-group-suggest-card-head";
    const title = document.createElement("input");
    title.type = "text";
    title.className = "mod-group-suggest-name";
    title.value = suggestion.label || "新策略组";
    title.maxLength = 60;
    title.setAttribute("aria-label", "策略组名称");
    const confidence = document.createElement("span");
    confidence.className = `mod-group-suggest-confidence ${suggestion.confidence || "low"}`;
    confidence.textContent = formatSuggestionConfidence(suggestion.confidence);
    if (suggestion.existingGroupId) {
      confidence.textContent += " · 已建组";
    }
    head.append(title, confidence);
    if (suggestion.source === "external") {
      const sourceBadge = document.createElement("span");
      sourceBadge.className = "mod-group-suggest-source";
      sourceBadge.textContent = "导入建议";
      sourceBadge.title = "来自你导入的建议文件（大模型 / 人工整理），优先展示";
      head.append(sourceBadge);
    }

    const meta = document.createElement("div");
    meta.className = "mod-group-suggest-meta";
    meta.textContent = formatSuggestionSummary(suggestion);

    const signals = document.createElement("div");
    signals.className = "mod-group-suggest-signals";
    formatSuggestionSignals(suggestion).forEach((signal) => {
      const chip = document.createElement("span");
      chip.className = "mod-group-suggest-signal";
      chip.textContent = signal;
      signals.appendChild(chip);
    });

    const memberList = document.createElement("div");
    memberList.className = "mod-group-suggest-members";
    const selection = suggestionSelections.get(suggestion.id) || defaultSuggestionSelection(suggestion);
    const fileIndex = buildSuggestionFileIndex(
      appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [],
      appState.currentDirectory,
    );
    suggestionMemberRows(suggestion, selection).forEach((row) => {
      // 行结构对齐"冲突检测"列表：标题 + 文件名 + 优先级 + 位置/游戏开关 + 操作按钮。
      const file = fileIndex.get(normalizeGroupKey(row.key)) || null;
      const planEntry = appState.priorityPlanMap?.get(normalizeGroupKey(row.key)) || null;
      const described = describeSuggestionMember(file, {
        priorityLabel: planEntry ? formatPriorityLabel(planEntry) : "",
      });

      const item = document.createElement("div");
      item.className = "mod-group-suggest-member";

      const pick = document.createElement("label");
      pick.className = "mod-group-suggest-member-pick";
      const checkbox = document.createElement("input");
      checkbox.type = "checkbox";
      checkbox.checked = row.selected;
      checkbox.title = row.selected ? "取消勾选这个 Mod" : "把这个 Mod 加入该组";
      checkbox.addEventListener("change", () => {
        const next = new Set(suggestionSelections.get(suggestion.id) || []);
        if (checkbox.checked) next.add(row.key);
        else next.delete(row.key);
        suggestionSelections.set(suggestion.id, next);
        footer.textContent = formatSuggestionCreateSummary(suggestion, next);
      });
      pick.appendChild(checkbox);

      const info = document.createElement("div");
      info.className = "conflict-vpk-info mod-group-suggest-member-info";

      const text = document.createElement("span");
      text.className = "conflict-vpk-title mod-group-suggest-member-name";
      text.textContent = row.name;
      text.title = row.key;
      // 标题相同的 Mod（例如同一工坊条目在根目录与 workshop 各有一份）需要靠
      // 文件名键区分，否则用户无法判断自己在勾选哪一个。
      const keyText = document.createElement("span");
      keyText.className = "conflict-vpk-filename mod-group-suggest-member-key";
      keyText.textContent = row.key;
      const locationBadge = document.createElement("span");
      locationBadge.className = "mod-group-suggest-member-badge";
      locationBadge.textContent = described.location;
      const gameStateBadge = document.createElement("span");
      gameStateBadge.className = "mod-group-suggest-member-badge game-state";
      gameStateBadge.textContent = described.gameState;
      info.append(text, keyText, locationBadge, gameStateBadge);
      if (described.found) {
        const priorityBadge = document.createElement("span");
        priorityBadge.className = `conflict-vpk-priority ${planEntry ? "known" : "unknown"}`;
        priorityBadge.textContent = described.priority;
        priorityBadge.title = "编号来自 addonlist.txt 顺序；数字越大越靠后加载";
        info.appendChild(priorityBadge);
      }

      const memberActions = document.createElement("div");
      memberActions.className = "conflict-vpk-actions mod-group-suggest-member-actions";
      const detailButton = document.createElement("button");
      detailButton.type = "button";
      detailButton.className = "btn btn-small btn-conflict-action";
      detailButton.textContent = "详情";
      detailButton.title = described.found ? "查看这个 Mod 的详情" : "该 Mod 不在当前列表中，无法查看详情";
      detailButton.disabled = !described.found;
      detailButton.addEventListener("click", () => {
        if (file?.path) showFileDetail(file.path);
      });
      memberActions.appendChild(detailButton);

      if (described.found) {
        const gameButton = document.createElement("button");
        gameButton.type = "button";
        gameButton.className = "btn btn-small btn-conflict-action";
        gameButton.textContent = described.gameState.replace("游戏开关：", "游戏开关 ");
        gameButton.disabled = !described.canEditGameState;
        gameButton.title = described.canEditGameState
          ? "编辑这个 Mod 在 addonlist.txt 里的游戏开关"
          : "该 Mod 位于 disabled 目录，无法直接编辑游戏开关";
        gameButton.addEventListener("click", () => {
          void runSuggestionMemberAction(suggestion, () => toggleGameEnabled(file.path));
        });
        memberActions.appendChild(gameButton);
      }

      if (described.fileAction) {
        const fileButton = document.createElement("button");
        fileButton.type = "button";
        // 复用冲突检测界面的语义类名，保证配色与按钮宽度一致。
        const actionClass =
          described.fileActionKind === "transfer"
            ? "btn-transfer"
            : described.fileActionKind === "enable"
              ? "btn-enable"
              : "btn-disable";
        fileButton.className = `btn btn-small btn-conflict-action ${actionClass} mod-group-suggest-member-file-action`;
        fileButton.textContent = described.fileAction;
        fileButton.title =
          described.fileActionKind === "transfer"
            ? "复制到 addons 目录并关闭工坊原件"
            : described.fileActionKind === "enable"
              ? "启用这个 Mod"
              : "禁用这个 Mod";
        fileButton.addEventListener("click", () => {
          const action =
            described.fileActionKind === "transfer"
              ? () => moveFileToAddons(file.path)
              : () => toggleFile(file.path);
          void runSuggestionMemberAction(suggestion, action);
        });
        memberActions.appendChild(fileButton);
      }

      item.append(pick, info, memberActions);
      memberList.appendChild(item);
    });

    const actions = document.createElement("div");
    actions.className = "mod-group-suggest-actions";
    const footer = document.createElement("span");
    footer.className = "mod-group-suggest-create-summary";
    footer.textContent = formatSuggestionCreateSummary(suggestion, selection);
    // 大组（几十个成员）逐个勾选太累，给一个整卡全选 / 全不选。
    const selectAllButton = document.createElement("button");
    selectAllButton.type = "button";
    selectAllButton.className = "btn btn-small btn-outline";
    selectAllButton.textContent = "全选";
    selectAllButton.title = "勾选这一组的全部 Mod";
    selectAllButton.addEventListener("click", () => {
      const all = defaultSuggestionSelection(suggestion);
      suggestionSelections.set(suggestion.id, all);
      renderSuggestionList(appState.groupSuggestions || []);
    });
    const clearAllButton = document.createElement("button");
    clearAllButton.type = "button";
    clearAllButton.className = "btn btn-small btn-outline";
    clearAllButton.textContent = "全不选";
    clearAllButton.title = "取消勾选这一组的全部 Mod";
    clearAllButton.addEventListener("click", () => {
      suggestionSelections.set(suggestion.id, new Set());
      renderSuggestionList(appState.groupSuggestions || []);
    });
    const createButton = document.createElement("button");
    createButton.type = "button";
    createButton.className = "btn btn-small btn-primary";
    createButton.textContent = "创建为组";
    createButton.addEventListener("click", () => {
      void createGroupFromSuggestion(suggestion, title.value, suggestionSelections.get(suggestion.id));
    });
    const selectActions = document.createElement("div");
    selectActions.className = "mod-group-suggest-select-actions";
    selectActions.append(selectAllButton, clearAllButton);
    actions.append(footer, selectActions, createButton);

    card.append(head, meta, signals, memberList, actions);
    list.appendChild(card);
  });
}

// runSuggestionMemberAction 在建议弹窗里执行单个 Mod 的开关 / 详情操作：
// 操作完重新推导一次，保证成员列表（位置、游戏开关、所属键）立即反映真实状态。
async function runSuggestionMemberAction(suggestion, action) {
  try {
    await action();
    renderFileList();
    await refreshModGroupMembership();
    await refreshModGroupSuggestions();
  } catch (error) {
    showError("操作失败: " + String(error?.message || error));
    try {
      await refreshModGroupSuggestions();
    } catch (refreshError) {
      console.warn("刷新分组建议失败:", refreshError);
    }
  }
}

async function createGroupFromSuggestion(suggestion, name, selection) {
  const keys = [...(selection || defaultSuggestionSelection(suggestion))];
  if (keys.length === 0) {
    showError("请至少勾选一个 Mod");
    return;
  }
  try {
    const group = await CreateModStrategyGroupFromKeys(
      String(name || "").trim() || suggestion.label || "新策略组",
      suggestion.reason || "来自分组建议",
      suggestion.strategy || "single",
      keys,
    );
    showNotification(`已创建策略组「${group.name}」（${keys.length} 个成员）`, "success");
    await refreshModGroupMembership();
    await refreshModGroupSuggestions();
    renderFileList();
  } catch (error) {
    showError("创建策略组失败: " + String(error?.message || error));
  }
}

/** initModGroupUI 绑定筛选菜单、建议弹窗与组徽标点击（幂等）。 */
export function initModGroupUI() {
  if (controlsBound) return;
  controlsBound = true;

  // 组归属变化时刷新筛选菜单并重绘列表（徽标跟随更新）。
  onModGroupMembershipChanged(() => {
    renderGroupFilterMenu();
    try {
      renderFileList();
    } catch (error) {
      console.warn("刷新组徽标失败:", error);
    }
  });

  const filterButton = element("mod-group-filter-btn");
  const filterMenu = element("mod-group-filter-menu");
  if (filterButton && filterMenu) {
    filterButton.addEventListener("click", (event) => {
      event.stopPropagation();
      // 打开时先按最新状态重绘，避免设置页改完组后菜单还是旧的。
      renderGroupFilterMenu();
      filterMenu.classList.toggle("hidden");
    });
    document.addEventListener("click", (event) => {
      if (!filterButton.contains(event.target) && !filterMenu.contains(event.target)) {
        filterMenu.classList.add("hidden");
      }
    });
  }

  element("mod-group-suggest-btn")?.addEventListener("click", () => {
    void openModGroupSuggestionModal();
  });
  element("mod-group-capture-btn")?.addEventListener("click", () => {
    void captureGroupFromSelection();
  });
  element("mod-group-suggest-refresh-btn")?.addEventListener("click", () => {
    void refreshModGroupSuggestions();
  });
  // 「导入建议文件…」：把大模型 / 人工写好的标准格式建议导入收件箱，然后重新推导。
  element("mod-group-suggest-import-btn")?.addEventListener("click", () => {
    void importExternalGroupSuggestions();
  });
  // 「导出 Mod 清单…」：给外部分析方（大模型）看的元数据快照。
  element("mod-group-suggest-export-btn")?.addEventListener("click", () => {
    void exportGroupingCatalog();
  });
  element("mod-group-suggest-prepare-btn")?.addEventListener("click", () => {
    void prepareAgentWorkspace();
  });
  element("mod-group-suggest-clear-btn")?.addEventListener("click", () => {
    void clearExternalGroupSuggestions();
  });
  element("mod-group-suggest-close-btn")?.addEventListener("click", closeModGroupSuggestionModal);
  element("mod-group-suggest-close-footer-btn")?.addEventListener("click", closeModGroupSuggestionModal);

  // 组徽标点击 = 整组开关（真实按钮，键盘可达）。
  document.addEventListener("click", (event) => {
    const badge = event.target.closest(".mod-group-badge");
    if (!badge) return;
    event.preventDefault();
    event.stopPropagation();
    const groupId = badge.dataset.groupId;
    const name = (badge.textContent || "").replace(/^组：/, "");
    if (groupId) void toggleGroupEnabled(groupId, name);
  });

  void refreshModGroupMembership();
}

// captureGroupFromSelection 把当前勾选的 Mod 直接保存成一个策略组。
// 复用 CreateModStrategyGroupFromKeys：只写 groups.json，不动任何 Mod 文件。
async function captureGroupFromSelection() {
  const selected = [...(appState.selectedFiles || [])];
  if (selected.length === 0) {
    showError("请先在列表里勾选要归入同一组的 Mod");
    return;
  }
  const keys = [];
  for (const path of selected) {
    const file = (appState.allVpkFiles || appState.vpkFiles || []).find((item) => item.path === path);
    if (!file) continue;
    const fileKeys = filePriorityKeys(file, appState.currentDirectory);
    if (fileKeys.length > 0) keys.push(fileKeys[0]);
  }
  if (keys.length === 0) {
    showError("选中的 Mod 没有可用的 addonlist 键，无法建组");
    return;
  }
  // 用应用内输入弹窗代替 window.prompt：WebView2 没有默认 prompt 实现。
  showPromptModal(
    "用选中的 Mod 建组",
    `把选中的 ${keys.length} 个 Mod 保存为同一个策略组，请输入组名。\n只写 groups.json，不会改动 Mod 文件，也不会改动 addonlist.txt。`,
    {
      confirmText: "创建为组",
      placeholder: "例如：命中反馈整合包",
      onConfirm: (name) => createGroupFromSelectionKeys(name, keys),
    },
  );
}

async function createGroupFromSelectionKeys(name, keys) {
  const trimmed = String(name || "").trim();
  if (!trimmed) {
    showError("组名不能为空");
    return false;
  }
  try {
    const group = await CreateModStrategyGroupFromKeys(trimmed, "来自 Mod 管理页的勾选", "single", keys);
    showNotification(`已创建策略组「${group.name}」（${keys.length} 个成员）`, "success");
    await refreshModGroupMembership();
    await refreshFilesKeepFilter();
    renderFileList();
  } catch (error) {
    showError("创建策略组失败: " + String(error?.message || error));
    return false;
  }
}
