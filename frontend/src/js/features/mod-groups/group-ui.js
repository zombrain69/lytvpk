// 策略组在 Mod 管理页的交互层：分组筛选、组徽标整组开关、整组优先级移动、分组推导建议。
//
// 三条安全约定（与后端一致）：
//   1. 推导只读扫描，不写任何文件；
//   2. 整组开关只改组成员在 addonlist.txt 里的 0/1，不重排、不动其它 Mod；
//   3. 整组优先级移动只写 priority.json（分层），要真正重排仍需在“编辑加载顺序”里点“按分层应用”。

import { appState } from "../state.js";
import { escapeHtml } from "../../core/utils.js";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import {
  ApplyTagToModKeys,
  AddModStrategyGroupMembers,
  CreateModStrategyGroupFromKeys,
  ClearExternalGroupSuggestions,
  DeleteModStrategyGroup,
  ExportGroupingCatalogDialog,
  GetGroupSuggestionAgentPrompt,
  GetGroupSuggestionInboxPath,
  ImportGroupSuggestionsOpenDialog,
  ListModStrategyGroups,
  PrepareGroupingWorkspace,
  RemoveModStrategyGroupMembers,
  RenameModStrategyGroup,
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
  formatGroupFilterLabel,
  formatGroupOptionIndent,
  formatGroupOptionLabel,
  formatGroupStrategy,
  formatSuggestionConfidence,
  formatSuggestionCreateSummary,
  formatSuggestionSignals,
  formatSuggestionSummary,
  groupEnabledVote,
  suggestionMemberRows,
  normalizeGroupKey,
} from "./group-view.mjs";
import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";
import { showConfirmModal } from "../modals/confirm.js";
import { showPromptModal } from "../modals/prompt.js";
import { buildModMemberRow } from "./member-row.js";
import { openStrategyGroupManager } from "./strategy-group-manager.js";
import { formatPriorityLabel } from "../file-list/priority-label.mjs";
import {
  DEFAULT_SUGGESTION_FILTER,
  SUGGESTION_PAGE_SIZE,
  applySuggestionView,
  countTagCoveredSuggestions,
  formatSuggestionViewSummary,
  isSuggestionTagCovered,
  paginateSuggestions,
  parseStoredSuggestionModalSize,
  suggestionPreviewMembers,
} from "./suggestion-filter.mjs";
import { openGroupTagDialog } from "./group-tag.js";
import { buildTagApplyPlan } from "./group-tag-view.mjs";
import { openAgentPromptEditor } from "./agent-prompt.js";

let controlsBound = false;
const suggestionSelections = new Map();
// 建议弹窗的"阅览层"状态：筛选条件、已加载条数、哪些卡片被展开。
// 默认折叠 + 分页渲染，234 条建议时也不会一次性塞进上千个 DOM 节点。
let suggestionFilter = { ...DEFAULT_SUGGESTION_FILTER };
let suggestionVisibleCount = SUGGESTION_PAGE_SIZE;
const suggestionExpanded = new Set();
const SUGGESTION_MODAL_SIZE_KEY = "lytvpk.modGroupSuggestModalSize";

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
    empty.textContent =
      t("strategyGroups.filterMenu.empty");
    menu.appendChild(empty);
    return;
  }

  const header = document.createElement("div");
  header.className = "mod-group-filter-header";
  const headerText = document.createElement("span");
  headerText.textContent = t("strategyGroups.filterMenu.header");
  const clearButton = document.createElement("button");
  clearButton.type = "button";
  clearButton.className = "btn-link";
  clearButton.textContent = t("strategyGroups.filterMenu.clear");
  clearButton.addEventListener("click", async (event) => {
    event.stopPropagation();
    appState.activeGroupFilter = new Set();
    renderGroupFilterMenu();
    await refreshFilesKeepFilter();
  });
  header.append(headerText, clearButton);
  menu.appendChild(header);
  // 顺序说明：这一条回答"外面这个按分组筛选是什么顺序"，也提示怎么把常用组排上去。
  const orderHint = document.createElement("div");
  orderHint.className = "mod-group-filter-order-hint";
  orderHint.textContent = t("strategyGroups.filterMenu.orderHint");
  orderHint.title = t("strategyGroups.filterMenu.orderHintTitle");
  menu.appendChild(orderHint);

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
    label.textContent = formatGroupOptionIndent(option) + formatGroupOptionLabel(option);
    label.title =
      t("strategyGroups.filterMenu.optionTitle", {
        strategy: formatGroupStrategy(option.strategy),
        count: option.memberCount,
      }) +
      (Number.isFinite(option.tier ?? NaN)
        ? t("strategyGroups.filterMenu.tierTag", { tier: option.tier })
        : t("strategyGroups.filterMenu.tierUnset")) +
      (option.missingCount > 0
        ? t("strategyGroups.filterMenu.missingTag", { count: option.missingCount })
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
    moveUp.textContent = t("strategyGroups.filterMenu.moveUp");
    moveUp.title = t("strategyGroups.filterMenu.moveUpTitle");
    moveUp.addEventListener("click", (event) => {
      event.stopPropagation();
      void shiftGroupPriority(option.id, option.name, 5);
    });
    const moveDown = document.createElement("button");
    moveDown.type = "button";
    moveDown.className = "btn-link";
    moveDown.textContent = t("strategyGroups.filterMenu.moveDown");
    moveDown.title = t("strategyGroups.filterMenu.moveDownTitle");
    moveDown.addEventListener("click", (event) => {
      event.stopPropagation();
      void shiftGroupPriority(option.id, option.name, -5);
    });
    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "btn-link";
    toggle.textContent = t("strategyGroups.common.groupToggle");
    toggle.title = t("strategyGroups.filterMenu.toggleTitle");
    toggle.addEventListener("click", (event) => {
      event.stopPropagation();
      void toggleGroupEnabled(option.id, option.name);
    });

    // 成员编辑：把列表里勾选的 Mod 批量加入 / 移出这个组（只写 groups.json）。
    const selectedKeys = selectedGroupActionKeys();
    const addSelected = document.createElement("button");
    addSelected.type = "button";
    addSelected.className = "btn-link";
    addSelected.textContent = t("strategyGroups.filterMenu.addSelected", {
      count: selectedKeys.length,
    });
    addSelected.disabled = selectedKeys.length === 0;
    addSelected.title =
      selectedKeys.length === 0
        ? t("strategyGroups.filterMenu.addSelectedEmpty")
        : t("strategyGroups.filterMenu.addSelectedTitle", {
            count: selectedKeys.length,
            name: option.name,
          });
    addSelected.addEventListener("click", (event) => {
      event.stopPropagation();
      void addSelectedModsToGroup(option.id, option.name);
    });
    const removeSelected = document.createElement("button");
    removeSelected.type = "button";
    removeSelected.className = "btn-link";
    removeSelected.textContent = t("strategyGroups.filterMenu.removeSelected", {
      count: selectedKeys.length,
    });
    removeSelected.disabled = selectedKeys.length === 0;
    removeSelected.title =
      selectedKeys.length === 0
        ? t("strategyGroups.filterMenu.removeSelectedEmpty")
        : t("strategyGroups.filterMenu.removeSelectedTitle", { name: option.name });
    removeSelected.addEventListener("click", (event) => {
      event.stopPropagation();
      void removeSelectedModsFromGroup(option.id, option.name);
    });

    // 重命名 / 删除：删除前用应用内确认弹窗说明影响范围。
    const rename = document.createElement("button");
    rename.type = "button";
    rename.className = "btn-link";
    rename.textContent = t("strategyGroups.common.rename");
    rename.title = t("strategyGroups.filterMenu.renameTitle");
    rename.addEventListener("click", (event) => {
      event.stopPropagation();
      void renameStrategyGroup(option.id, option.name);
    });
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "btn-link is-danger";
    remove.textContent = t("strategyGroups.common.delete");
    remove.title = t("strategyGroups.filterMenu.deleteTitle");
    remove.addEventListener("click", (event) => {
      event.stopPropagation();
      void confirmDeleteStrategyGroup(option);
    });

    // 组 → 标签：把这一组沉淀成一个统一标签，之后就能用标签筛选代替建组。
    const tagButton = document.createElement("button");
    tagButton.type = "button";
    tagButton.className = "btn-link";
    tagButton.textContent = t("strategyGroups.filterMenu.tag");
    tagButton.title = t("strategyGroups.filterMenu.tagTitle");
    tagButton.addEventListener("click", (event) => {
      event.stopPropagation();
      void openGroupTagDialog({
        groupId: option.id,
        groupName: option.name,
        keys: [...(option.keys || [])],
        names: (appState.modGroupMemberships || [])
          .filter((membership) => membership.groupId === option.id)
          .map((membership) => membership.name || membership.key),
        onDone: () => {
          renderFileList();
          void refreshFilesKeepFilter();
        },
      });
    });

    actions.append(moveUp, moveDown, toggle, addSelected, removeSelected, rename, tagButton, remove);

    row.append(checkbox, label, actions);
    menu.appendChild(row);
  });
}

// selectedGroupActionKeys 取当前勾选的 Mod，按 addonlist 键换算（保持列表顺序、去重）。
function selectedGroupActionKeys() {
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  const byPath = new Map(files.map((file) => [file.path, file]));
  const keys = [];
  const seen = new Set();
  for (const path of appState.selectedFiles || []) {
    const file = byPath.get(path);
    if (!file) continue;
    const fileKeys = filePriorityKeys(file, appState.currentDirectory);
    const key = fileKeys[0];
    if (!key || seen.has(key)) continue;
    seen.add(key);
    keys.push(key);
  }
  return keys;
}

async function addSelectedModsToGroup(groupId, groupName) {
  const keys = selectedGroupActionKeys();
  if (keys.length === 0) {
    showError(t("strategyGroups.groupActions.addNoSelection"));
    return;
  }
  try {
    const before = groupMemberCount(groupId);
    const group = await AddModStrategyGroupMembers(groupId, keys);
    const added = Math.max(0, (group.members || []).length - before);
    showNotification(
      added > 0
        ? t("strategyGroups.groupActions.added", {
            added,
            name: groupName,
            total: (group.members || []).length,
          })
        : t("strategyGroups.picker.allPresent", { name: groupName }),
      added > 0 ? "success" : "info",
    );
    await refreshModGroupMembership();
    renderFileList();
    renderGroupFilterMenu();
  } catch (error) {
    showError(
      t("strategyGroups.groupActions.addFailed", { error: String(error?.message || error) }),
    );
  }
}

async function removeSelectedModsFromGroup(groupId, groupName) {
  const keys = selectedGroupActionKeys();
  if (keys.length === 0) {
    showError(t("strategyGroups.groupActions.removeNoSelection"));
    return;
  }
  try {
    const group = await RemoveModStrategyGroupMembers(groupId, keys);
    showNotification(
      t("strategyGroups.groupActions.removed", {
        count: keys.length,
        name: groupName,
        total: (group.members || []).length,
      }),
      "success",
    );
    await refreshModGroupMembership();
    renderFileList();
    renderGroupFilterMenu();
  } catch (error) {
    showError(
      t("strategyGroups.groupActions.removeFailed", { error: String(error?.message || error) }),
    );
  }
}

function groupMemberCount(groupId) {
  for (const membership of appState.modGroupMemberships || []) {
    if (membership.groupId === groupId) return Number(membership.memberCount || 0);
  }
  return 0;
}

async function renameStrategyGroup(groupId, currentName) {
  let description = "";
  try {
    const groups = (await ListModStrategyGroups()) || [];
    const found = groups.find((item) => item.id === groupId);
    description = found?.description || "";
  } catch (error) {
    console.warn("读取策略组描述失败（重命名时按空描述处理）:", error);
  }
  showPromptModal(
    t("strategyGroups.common.renameTitle"),
    t("strategyGroups.groupActions.renameMessage", { name: currentName }),
    {
      defaultValue: currentName,
      confirmText: t("strategyGroups.groupActions.renameConfirm"),
      placeholder: t("strategyGroups.common.nameLabel"),
    onConfirm: async (value) => {
      try {
        const group = await RenameModStrategyGroup(groupId, value, description);
        showNotification(t("strategyGroups.common.renamed", { name: group.name }), "success");
        await refreshModGroupMembership();
        renderGroupFilterMenu();
      } catch (error) {
        showError(
          t("strategyGroups.groupActions.renameFailed", { error: String(error?.message || error) }),
        );
      }
    },
  });
}

async function confirmDeleteStrategyGroup(option) {
  // 影响范围直接问后端要（前端的筛选菜单只带成员数，没有层级信息）。
  let memberCount = Number(option.memberCount || 0);
  let childCount = 0;
  try {
    const groups = (await ListModStrategyGroups()) || [];
    const found = groups.find((item) => item.id === option.id);
    if (found) memberCount = (found.members || []).length;
    childCount = groups.filter((item) => String(item.parentId || "") === option.id).length;
  } catch (error) {
    console.warn("读取策略组影响范围失败（按已知信息提示）:", error);
  }
  showConfirmModal(
    t("strategyGroups.common.groupDeleteTitle"),
    t("strategyGroups.groupActions.deleteMessage", {
      name: option.name,
      count: memberCount,
      childLine:
        childCount > 0
          ? t("strategyGroups.common.deleteChildLine", { count: childCount })
          : "",
    }),
    async () => {
      try {
        await DeleteModStrategyGroup(option.id);
        const next = new Set(appState.activeGroupFilter);
        next.delete(option.id);
        appState.activeGroupFilter = next;
        showNotification(t("strategyGroups.groupActions.deleted", { name: option.name }), "success");
        await refreshModGroupMembership();
        renderFileList();
        renderGroupFilterMenu();
        await refreshFilesKeepFilter();
      } catch (error) {
        showError(
          t("strategyGroups.common.deleteFailed", { error: String(error?.message || error) }),
        );
      }
    },
  );
}

async function toggleGroupEnabled(groupId, groupName) {
  const members = (appState.modGroupMemberships || []).filter((item) => item.groupId === groupId);
  const vote = currentGroupEnabledVote(groupId);
  const nextEnabled = !vote.mostlyEnabled;
  // 使用应用内确认弹窗：WebView2 的原生 confirm 在锁屏/自动化环境下会阻塞页面。
  showConfirmModal(
    t("strategyGroups.common.groupToggle"),
    t("strategyGroups.common.toggleIntro", {
      name: groupName,
      count: members.length,
      action: t(
        nextEnabled
          ? "strategyGroups.common.groupToggleOn"
          : "strategyGroups.common.groupToggleOff",
      ),
    }) +
      t("strategyGroups.common.toggleVote", { enabled: vote.enabled, disabled: vote.disabled }) +
      t("strategyGroups.common.toggleMessage"),
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
      t("strategyGroups.common.toggleResult", {
        name: groupName,
        action: t(
          nextEnabled
            ? "strategyGroups.common.groupToggleOn"
            : "strategyGroups.common.groupToggleOff",
        ),
        count: (result.enabled?.length || 0) + (result.disabled?.length || 0),
      }) +
        (skipped.length > 0
          ? t("strategyGroups.common.skippedNote", { count: skipped.length })
          : ""),
      skipped.length > 0 ? "info" : "success",
    );
    await refreshModGroupMembership();
    renderFileList();
    await refreshFilesKeepFilter();
  } catch (error) {
    showError(t("strategyGroups.common.toggleFailed", { error: String(error?.message || error) }));
  }
}

// groupEnabledVote（group-view.mjs 里的纯函数）统计组内成员在 addonlist 里的开关情况，
// 用于决定"整组开关"的方向；「策略组管理」窗口的聚焦条复用同一份判定。
function currentGroupEnabledVote(groupId) {
  const files = appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [];
  return groupEnabledVote(
    appState.modGroupMemberships || [],
    files,
    groupId,
    appState.currentDirectory,
  );
}

async function shiftGroupPriority(groupId, groupName, delta) {
  try {
    const result = await ShiftModStrategyGroupPriorities(groupId, delta);
    const moved = result?.moved?.length || 0;
    const skipped = result?.skipped?.length || 0;
    showNotification(
      t("strategyGroups.groupActions.shiftResult", {
        name: groupName,
        action: t(
          delta > 0
            ? "strategyGroups.groupActions.shiftForward"
            : "strategyGroups.groupActions.shiftBackward",
        ),
        count: Math.abs(delta),
        moved,
      }) +
        (skipped > 0 ? t("strategyGroups.groupActions.shiftSkipped", { count: skipped }) : "") +
        t("strategyGroups.groupActions.shiftNotice"),
      skipped > 0 ? "info" : "success",
    );
    await refreshModGroupMembership();
    renderFileList();
  } catch (error) {
    showError(
      t("strategyGroups.groupActions.shiftFailed", { error: String(error?.message || error) }),
    );
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
  // 后端会先重新扫描一遍目录（增量缓存），所以这一步可能比纯导出稍慢：
  // 用按钮文案给一个"正在扫描"的反馈。
  const prepareButton = element("mod-group-suggest-prepare-btn");
  const originalLabel = prepareButton?.textContent || "";
  if (prepareButton) {
    prepareButton.disabled = true;
    prepareButton.textContent = t("strategyGroups.suggest.prepareBusy");
  }
  try {
    const workspace = await PrepareGroupingWorkspace();
    if (!workspace?.catalogPath) {
      showError(t("strategyGroups.suggest.prepareNoPath"));
      return;
    }
    const copied = await copyTextToClipboard(workspace.prompt || "");
    showNotification(
      t("strategyGroups.suggest.prepareExported", { count: workspace.modCount }) +
        (copied
          ? t("strategyGroups.suggest.prepareCopyOk")
          : t("strategyGroups.suggest.prepareCopyTodo")),
      copied ? "success" : "info",
    );
    showConfirmModal(
      t("strategyGroups.suggest.prepareReady"),
      `<div style="line-height:1.6">` +
        `<p style="margin:0 0 8px">${t("strategyGroups.suggest.prepareFreshLead")}<b>${t("strategyGroups.suggest.prepareFreshStrong")}</b>${t("strategyGroups.suggest.prepareFreshTail", { count: workspace.modCount })}</p>` +
        (workspace.notice
          ? `<p style="margin:0 0 8px;color:var(--warning,#f5a524)">${escapeHtml(workspace.notice)}</p>`
          : "") +
        `${t("strategyGroups.suggest.prepareCatalogLabel")}<code style="word-break:break-all">${escapeHtml(workspace.catalogPath)}</code><br>` +
        t("strategyGroups.suggest.prepareInboxLabel") +
        `<code style="word-break:break-all">${escapeHtml(workspace.inboxPath)}</code>` +
        `<p style="margin:10px 0 0">` +
        (copied
          ? t("strategyGroups.suggest.prepareCopied")
          : t("strategyGroups.suggest.prepareNotCopied")) +
        `</p><p style="margin:6px 0 0">${t("strategyGroups.suggest.prepareFootnote")}</p>` +
        `</div>`,
      () => {},
      true,
    );
  } catch (error) {
    showError(t("strategyGroups.suggest.prepareFailed", { error: String(error?.message || error) }));
  } finally {
    if (prepareButton) {
      prepareButton.disabled = false;
      prepareButton.textContent = originalLabel;
    }
  }
}

// copyAgentPrompt 只复制提示词（不导出清单）。
async function copyAgentPrompt() {
  try {
    const prompt = await GetGroupSuggestionAgentPrompt();
    if (!prompt) {
      showError(t("strategyGroups.suggest.promptEmpty"));
      return;
    }
    const copied = await copyTextToClipboard(prompt);
    if (copied) {
      showNotification(t("strategyGroups.suggest.promptCopied"), "success");
      return;
    }
    const path = await SaveGroupSuggestionAgentPromptDialog();
    if (path) {
      showNotification(t("strategyGroups.suggest.promptSavedFallback", { path }), "info");
    }
  } catch (error) {
    showError(t("strategyGroups.suggest.promptCopyFailed", { error: String(error?.message || error) }));
  }
}

// saveAgentPrompt 把提示词另存为 Markdown 文件。
async function saveAgentPrompt() {
  try {
    const path = await SaveGroupSuggestionAgentPromptDialog();
    if (!path) {
      return;
    }
    showNotification(t("strategyGroups.suggest.promptSaved", { path }), "success");
  } catch (error) {
    showError(t("strategyGroups.suggest.promptSaveFailed", { error: String(error?.message || error) }));
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
      t("strategyGroups.suggest.imported", {
        imported: result.imported,
        members: result.memberCount,
      }) +
        (result.skipped > 0
          ? t("strategyGroups.suggest.importSkipped", { count: result.skipped })
          : ""),
      result.skipped > 0 || warnings.length > 0 ? "info" : "success",
    );
    await refreshModGroupSuggestions();
    if (warnings.length > 0) {
      showConfirmModal(t("strategyGroups.suggest.importResultTitle"), warnings.slice(0, 8).join("\n"), () => {});
    }
  } catch (error) {
    showError(t("strategyGroups.suggest.importFailed", { error: String(error?.message || error) }));
  }
}

// exportGroupingCatalog 导出分组用 Mod 清单（键、标题、作者、标签、主体、语音角色）。
async function exportGroupingCatalog() {
  // 后端在导出前会重新扫描一遍目录，这里给一个可感知的进行中状态。
  const exportButton = element("mod-group-suggest-export-btn");
  const originalLabel = exportButton?.textContent || "";
  if (exportButton) {
    exportButton.disabled = true;
    exportButton.textContent = t("strategyGroups.suggest.prepareBusy");
  }
  try {
    const path = await ExportGroupingCatalogDialog();
    if (!path) {
      return; // 用户取消
    }
    const inbox = await GetGroupSuggestionInboxPath().catch(() => "");
    showNotification(t("strategyGroups.suggest.exported", { path }), "success");
    if (inbox) {
      showConfirmModal(
        t("strategyGroups.suggest.exportDialogTitle"),
        t("strategyGroups.suggest.exportLine", { path }) +
          t("strategyGroups.suggest.exportHint") +
          t("strategyGroups.suggest.exportInbox", { path: inbox }),
        () => {},
      );
    }
  } catch (error) {
    showError(t("strategyGroups.suggest.exportFailed", { error: String(error?.message || error) }));
  } finally {
    if (exportButton) {
      exportButton.disabled = false;
      exportButton.textContent = originalLabel;
    }
  }
}

async function clearExternalGroupSuggestions() {
  try {
    await ClearExternalGroupSuggestions();
    showNotification(t("strategyGroups.suggest.cleared"), "success");
    await refreshModGroupSuggestions();
  } catch (error) {
    showError(t("strategyGroups.suggest.clearFailed", { error: String(error?.message || error) }));
  }
}

/** openModGroupSuggestionModal 打开"分组建议"弹窗并重新推导。 */
export async function openModGroupSuggestionModal() {
  const modal = element("mod-group-suggest-modal");
  if (!modal) return;
  modal.classList.remove("hidden");
  restoreSuggestionModalSize();
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
  if (summary) summary.textContent = t("strategyGroups.suggest.loading");
  try {
    const suggestions = (await SuggestModGroups()) || [];
    appState.groupSuggestions = suggestions;
    suggestionSelections.clear();
    suggestions.forEach((suggestion) => {
      suggestionSelections.set(suggestion.id, defaultSuggestionSelection(suggestion));
    });
    // 重新推导后回到第一页，避免"筛选后仍停在第 5 页"看到空白。
    suggestionVisibleCount = SUGGESTION_PAGE_SIZE;
    renderSuggestionList(suggestions);
  } catch (error) {
    if (summary)
      summary.textContent = t("strategyGroups.suggest.deriveFailed", {
        error: String(error?.message || error),
      });
  }
}

// restoreSuggestionModalSize / persistSuggestionModalSize：把用户拖出来的弹窗尺寸记在
// localStorage 里，下次打开还是这个大小（窗口变小或换了分辨率时按视口收敛）。
function restoreSuggestionModalSize() {
  const content = document.querySelector("#mod-group-suggest-modal .mod-group-suggest-content");
  if (!content) return;
  let stored = null;
  try {
    stored = parseStoredSuggestionModalSize(window.localStorage?.getItem(SUGGESTION_MODAL_SIZE_KEY));
  } catch (error) {
    console.warn("读取分组建议弹窗尺寸失败:", error);
  }
  if (!stored) return;
  const width = Math.min(stored.width, Math.round(window.innerWidth * 0.98));
  const height = Math.min(stored.height, Math.round(window.innerHeight * 0.95));
  content.style.width = `${Math.max(width, 320)}px`;
  content.style.height = `${Math.max(height, 320)}px`;
  content.style.maxWidth = "98vw";
  content.style.maxHeight = "95vh";
}

function persistSuggestionModalSize(content) {
  if (!content) return;
  try {
    const rect = content.getBoundingClientRect();
    if (!rect.width || !rect.height) return;
    window.localStorage?.setItem(
      SUGGESTION_MODAL_SIZE_KEY,
      JSON.stringify({ width: Math.round(rect.width), height: Math.round(rect.height) }),
    );
  } catch (error) {
    console.warn("保存分组建议弹窗尺寸失败:", error);
  }
}

let suggestionSizeObserver = null;
let suggestionSizeTimer = null;

// observeSuggestionModalSize：监听用户拖动右下角缩放产生的尺寸变化（带防抖）。
function observeSuggestionModalSize() {
  const content = document.querySelector("#mod-group-suggest-modal .mod-group-suggest-content");
  if (!content || suggestionSizeObserver) return;
  if (typeof ResizeObserver !== "function") return;
  suggestionSizeObserver = new ResizeObserver(() => {
    if (suggestionSizeTimer) clearTimeout(suggestionSizeTimer);
    suggestionSizeTimer = setTimeout(() => {
      suggestionSizeTimer = null;
      persistSuggestionModalSize(content);
    }, 400);
  });
  suggestionSizeObserver.observe(content);
}

// syncSuggestionFilterControls：让筛选控件与内存状态一致（打开弹窗、重置时调用）。
function syncSuggestionFilterControls() {
  const search = element("mod-group-suggest-search");
  const confidence = element("mod-group-suggest-confidence");
  const source = element("mod-group-suggest-source");
  const sort = element("mod-group-suggest-sort");
  const tagCovered = element("mod-group-suggest-tag-covered");
  if (search) search.value = suggestionFilter.query;
  if (confidence) confidence.value = suggestionFilter.confidence;
  if (source) source.value = suggestionFilter.source;
  if (sort) sort.value = suggestionFilter.sort;
  if (tagCovered) tagCovered.value = suggestionFilter.tagCovered;
}

function readSuggestionFilterFromControls() {
  suggestionFilter = {
    query: String(element("mod-group-suggest-search")?.value || ""),
    confidence: String(element("mod-group-suggest-confidence")?.value || ""),
    source: String(element("mod-group-suggest-source")?.value || ""),
    tagCovered: String(element("mod-group-suggest-tag-covered")?.value || "hide") || "hide",
    sort: String(element("mod-group-suggest-sort")?.value || "recommended") || "recommended",
  };
}

// rerenderSuggestionView：筛选条件或"显示更多"变化时重画列表（保留展开状态与勾选）。
function rerenderSuggestionView({ resetPage = true } = {}) {
  if (resetPage) suggestionVisibleCount = SUGGESTION_PAGE_SIZE;
  renderSuggestionList(appState.groupSuggestions || []);
}

function renderSuggestionList(suggestions) {
  const list = element("mod-group-suggest-list");
  const summary = element("mod-group-suggest-summary");
  const filterSummary = element("mod-group-suggest-filter-summary");
  const moreButton = element("mod-group-suggest-more-btn");
  if (!list) return;
  const all = Array.isArray(suggestions) ? suggestions : [];
  list.replaceChildren();
  if (summary) {
    const externalCount = all.filter((item) => item?.source === "external").length;
    const tagCoveredCount = countTagCoveredSuggestions(all);
    summary.textContent =
      all.length === 0
        ? t("strategyGroups.suggest.summaryNone")
        : t("strategyGroups.suggest.summaryTotal", { count: all.length }) +
          (externalCount > 0
            ? t("strategyGroups.suggest.summaryExternal", { count: externalCount })
            : "") +
          (tagCoveredCount > 0
            ? t("strategyGroups.suggest.summaryTagCovered", { count: tagCoveredCount })
            : "") +
          t("strategyGroups.suggest.summaryHint");
  }

  const view = applySuggestionView(all, suggestionFilter);
  const page = paginateSuggestions(view, suggestionVisibleCount);
  if (filterSummary) {
    filterSummary.textContent = formatSuggestionViewSummary({
      total: view.length,
      shown: page.items.length,
      hidden: page.hiddenCount,
    });
  }
  if (moreButton) {
    moreButton.classList.toggle("hidden", !page.hasMore);
    moreButton.textContent = page.hasMore
      ? t("strategyGroups.suggest.more", { count: page.hiddenCount })
      : t("strategyGroups.suggest.moreIdle");
  }

  if (all.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-suggest-empty";
    empty.textContent = t("strategyGroups.suggest.emptyHint");
    list.appendChild(empty);
    return;
  }
  if (view.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-suggest-empty";
    empty.textContent = t("strategyGroups.suggest.filterEmpty");
    const reset = document.createElement("button");
    reset.type = "button";
    reset.className = "btn btn-small btn-outline";
    reset.textContent = t("strategyGroups.window.batchFilterClear");
    reset.addEventListener("click", () => {
      suggestionFilter = { ...DEFAULT_SUGGESTION_FILTER };
      syncSuggestionFilterControls();
      rerenderSuggestionView();
    });
    empty.appendChild(reset);
    list.appendChild(empty);
    return;
  }

  page.items.forEach((suggestion) => {
    const expanded = suggestionExpanded.has(suggestion.id);
    const card = document.createElement("section");
    card.className =
      `mod-group-suggest-card confidence-${suggestion.confidence || "low"}` +
      (expanded ? "" : " is-collapsed");
    card.dataset.suggestionId = suggestion.id || "";

    const head = document.createElement("div");
    head.className = "mod-group-suggest-card-head";
    let titleInput = null;
    if (expanded) {
      titleInput = document.createElement("input");
      titleInput.type = "text";
      titleInput.className = "mod-group-suggest-name";
      titleInput.value = suggestion.label || t("strategyGroups.suggest.defaultLabel");
      titleInput.maxLength = 60;
      titleInput.setAttribute("aria-label", t("strategyGroups.common.nameLabel"));
      head.append(titleInput);
    } else {
      const titleText = document.createElement("button");
      titleText.type = "button";
      titleText.className = "mod-group-suggest-title";
      titleText.textContent = suggestion.label || t("strategyGroups.suggest.defaultLabel");
      titleText.title = t("strategyGroups.suggest.expandTitle");
      titleText.addEventListener("click", () => {
        suggestionExpanded.add(suggestion.id);
        renderSuggestionList(appState.groupSuggestions || []);
      });
      head.append(titleText);
    }
    const confidence = document.createElement("span");
    confidence.className = `mod-group-suggest-confidence ${suggestion.confidence || "low"}`;
    confidence.textContent = formatSuggestionConfidence(suggestion.confidence);
    if (suggestion.existingGroupId) {
      confidence.textContent += t("strategyGroups.suggest.existingGroupTag");
    }
    head.append(confidence);
    if (suggestion.source === "external") {
      const sourceBadge = document.createElement("span");
      sourceBadge.className = "mod-group-suggest-source";
      sourceBadge.textContent = t("strategyGroups.suggest.badgeImported");
      sourceBadge.title = t("strategyGroups.suggest.badgeImportedTitle");
      head.append(sourceBadge);
    }
    const memberCountBadge = document.createElement("span");
    memberCountBadge.className = "mod-group-suggest-count";
    memberCountBadge.textContent = t("strategyGroups.view.modCount", {
      count: (suggestion.memberKeys || []).length,
    });
    head.append(memberCountBadge);
    if (isSuggestionTagCovered(suggestion)) {
      // 标签恰好只能筛出这一批 Mod：建组的增量价值低，给一个直接跳到标签筛选的入口。
      const tagBadge = document.createElement("span");
      tagBadge.className = "mod-group-suggest-tag-badge";
      tagBadge.textContent = t("strategyGroups.suggest.tagCoveredBadge", { tag: suggestion.tagKey });
      tagBadge.title = t("strategyGroups.suggest.tagCoveredBadgeTitle");
      head.append(tagBadge);
      const filterByTag = document.createElement("button");
      filterByTag.type = "button";
      filterByTag.className = "btn btn-small btn-outline";
      filterByTag.textContent = t("strategyGroups.suggest.useTagFilter");
      filterByTag.title = t("strategyGroups.suggest.useTagFilterTitle", {
        tag: suggestion.tagKey,
      });
      filterByTag.addEventListener("click", () => {
        void applyTagFilterFromSuggestion(suggestion);
      });
      head.append(filterByTag);
    }
    const importedTagPlan = buildTagApplyPlan(suggestion);
    if (importedTagPlan.length > 0) {
      // 外部建议文件里带的标签（智能体给的最准）：一键给这批 Mod 打上。
      const importedBadge = document.createElement("span");
      importedBadge.className = "mod-group-suggest-import-tag";
      importedBadge.textContent =
        importedTagPlan.length === 1
          ? t("strategyGroups.suggest.importTagBadge", { tag: importedTagPlan[0].tag })
          : t("strategyGroups.suggest.importTagBadgeMore", {
              tag: importedTagPlan[0].tag,
              count: importedTagPlan.length,
            });
      importedBadge.title = [suggestion.tagReason, t("strategyGroups.suggest.importTagOrigin")]
        .filter(Boolean)
        .join(" · ");
      head.append(importedBadge);
      const applyTagButton = document.createElement("button");
      applyTagButton.type = "button";
      applyTagButton.className = "btn btn-small btn-outline";
      applyTagButton.textContent = t("strategyGroups.suggest.applyTags");
      applyTagButton.title = t("strategyGroups.suggest.applyTagsTitle", {
        summary: importedTagPlan
          .map((entry) =>
            t("strategyGroups.suggest.applyTagsPlanEntry", {
              tag: entry.tag,
              count: entry.keys.length,
            }),
          )
          .join("、"),
      });
      applyTagButton.addEventListener("click", () => {
        void applyImportedTags(suggestion, importedTagPlan);
      });
      head.append(applyTagButton);
    }

    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "btn btn-small btn-outline mod-group-suggest-toggle";
    toggle.textContent = expanded
      ? t("strategyGroups.suggest.collapse")
      : t("strategyGroups.suggest.expand");
    toggle.setAttribute("aria-expanded", expanded ? "true" : "false");
    toggle.addEventListener("click", () => {
      if (expanded) suggestionExpanded.delete(suggestion.id);
      else suggestionExpanded.add(suggestion.id);
      renderSuggestionList(appState.groupSuggestions || []);
    });
    head.append(toggle);

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

    // 折叠态：给出成员预览，帮助用户判断"这条建议是不是我要的"。
    const selection = suggestionSelections.get(suggestion.id) || defaultSuggestionSelection(suggestion);
    const preview = document.createElement("div");
    preview.className = "mod-group-suggest-preview";
    const previewInfo = suggestionPreviewMembers(suggestion);
    previewInfo.names.forEach((name) => {
      const chip = document.createElement("span");
      chip.className = "mod-group-suggest-preview-item";
      chip.textContent = name;
      chip.title = name;
      preview.appendChild(chip);
    });
    if (previewInfo.more > 0) {
      const more = document.createElement("span");
      more.className = "mod-group-suggest-preview-more";
      more.textContent = t("strategyGroups.tag.preview.moreNames", { count: previewInfo.more });
      preview.appendChild(more);
    }

    const actions = document.createElement("div");
    actions.className = "mod-group-suggest-actions";
    const footer = document.createElement("span");
    footer.className = "mod-group-suggest-create-summary";
    footer.textContent = formatSuggestionCreateSummary(suggestion, selection);
    const memberList = expanded
      ? buildSuggestionMemberList(suggestion, selection, () => {
          footer.textContent = formatSuggestionCreateSummary(
            suggestion,
            suggestionSelections.get(suggestion.id) || new Set(),
          );
        })
      : null;
    // 大组（几十个成员）逐个勾选太累，给一个整卡全选 / 全不选。
    const selectAllButton = document.createElement("button");
    selectAllButton.type = "button";
    selectAllButton.className = "btn btn-small btn-outline";
    selectAllButton.textContent = t("strategyGroups.suggest.selectAll");
    selectAllButton.title = t("strategyGroups.suggest.selectAllTitle");
    selectAllButton.addEventListener("click", () => {
      const all = defaultSuggestionSelection(suggestion);
      suggestionSelections.set(suggestion.id, all);
      applySelectionToCard(card, suggestion);
    });
    const clearAllButton = document.createElement("button");
    clearAllButton.type = "button";
    clearAllButton.className = "btn btn-small btn-outline";
    clearAllButton.textContent = t("strategyGroups.suggest.clearAll");
    clearAllButton.title = t("strategyGroups.suggest.clearAllTitle");
    clearAllButton.addEventListener("click", () => {
      suggestionSelections.set(suggestion.id, new Set());
      applySelectionToCard(card, suggestion);
    });
    const createButton = document.createElement("button");
    createButton.type = "button";
    createButton.className = "btn btn-small btn-primary";
    createButton.textContent = expanded
      ? t("strategyGroups.suggest.create")
      : t("strategyGroups.suggest.createWithCount", { count: selection.size });
    if (suggestion.existingGroupId) {
      createButton.disabled = true;
      createButton.title = t("strategyGroups.suggest.createDisabled");
    }
    createButton.addEventListener("click", () => {
      const name = titleInput?.value || suggestion.label || t("strategyGroups.suggest.defaultLabel");
      void createGroupFromSuggestion(suggestion, name, suggestionSelections.get(suggestion.id));
    });
    const selectActions = document.createElement("div");
    selectActions.className = "mod-group-suggest-select-actions";
    selectActions.append(selectAllButton, clearAllButton);
    actions.append(footer, selectActions, createButton);

    card.append(head, meta, signals, preview);
    if (memberList) card.append(memberList);
    card.append(actions);
    list.appendChild(card);
  });
}

// applySelectionToCard 只更新这张卡片的勾选与"将创建 N 个成员"文案，
// 不重画整个列表（234 条建议时全量重画会明显卡顿）。
function applySelectionToCard(card, suggestion) {
  const selection = suggestionSelections.get(suggestion.id) || new Set();
  card.querySelectorAll("input[data-member-key]").forEach((checkbox) => {
    checkbox.checked = selection.has(checkbox.dataset.memberKey);
  });
  const footer = card.querySelector(".mod-group-suggest-create-summary");
  if (footer) footer.textContent = formatSuggestionCreateSummary(suggestion, selection);
  const createButton = card.querySelector(".mod-group-suggest-actions .btn-primary");
  if (createButton && !suggestion.existingGroupId) {
    createButton.textContent =
      selection.size === 0
        ? t("strategyGroups.suggest.create")
        : t("strategyGroups.suggest.createWithCount", { count: selection.size });
  }
}

// applyTagFilterFromSuggestion 关闭建议弹窗，改用标签筛选（组建议 → 标签 的出口）。
async function applyTagFilterFromSuggestion(suggestion) {
  const tag = String(suggestion?.tagKey || "").trim();
  if (!tag) return;
  closeModGroupSuggestionModal();
  appState.selectedSecondaryTags = [tag];
  showNotification(t("strategyGroups.suggest.tagFilterApplied", { tag }), "success");
  await refreshFilesKeepFilter();
  renderFileList();
}

// applyImportedTags 应用建议文件里带的标签（智能体给的 tag / memberTags）。
// 会改文件名（＝改 addonlist 键），所以先用应用内确认框说明影响，后端负责把本地记录改绑。
async function applyImportedTags(suggestion, plan) {
  const summary = plan
    .map((entry) =>
      t("strategyGroups.suggest.tagPlanEntry", { tag: entry.tag, count: entry.keys.length }),
    )
    .join("、");
  showConfirmModal(
    t("strategyGroups.suggest.applyImportedTitle"),
    t("strategyGroups.suggest.applyImportedMessage", {
      name: suggestion.label || t("strategyGroups.suggest.thisSuggestion"),
      summary,
    }),
    async () => {
      try {
        let applied = 0;
        let skipped = 0;
        let missing = 0;
        let failed = 0;
        const notes = [];
        for (const entry of plan) {
          const result = await ApplyTagToModKeys(entry.tag, entry.keys);
          applied += (result?.applied || []).length;
          skipped += (result?.skipped || []).length;
          missing += (result?.missing || []).length;
          failed += (result?.failed || []).length;
          if (Array.isArray(result?.reasons)) notes.push(...result.reasons);
        }
        showNotification(
          t("strategyGroups.suggest.applyTagsDone", {
            count: plan.length,
            applied,
          }) +
            (skipped > 0 ? t("strategyGroups.suggest.applyTagsExisting", { count: skipped }) : "") +
            (missing > 0 ? t("strategyGroups.suggest.applyTagsMissing", { count: missing }) : "") +
            (failed > 0 ? t("strategyGroups.suggest.applyTagsFailed", { count: failed }) : ""),
          failed > 0 ? "warning" : "success",
        );
        if (notes.length > 0) console.warn("应用导入标签失败详情:", notes);
        await refreshModGroupMembership();
        renderFileList();
        await refreshModGroupSuggestions();
      } catch (error) {
        showError(
          t("strategyGroups.suggest.applyImportedFailed", {
            error: String(error?.message || error),
          }),
        );
      }
    },
  );
}

// buildSuggestionMemberList 渲染"展开后"的成员明细（与冲突检测界面同构）。
function buildSuggestionMemberList(suggestion, selection, onSelectionChange) {
  const memberList = document.createElement("div");
  memberList.className = "mod-group-suggest-members";
  const fileIndex = buildSuggestionFileIndex(
    appState.allVpkFiles?.length ? appState.allVpkFiles : appState.vpkFiles || [],
    appState.currentDirectory,
  );
  suggestionMemberRows(suggestion, selection).forEach((row) => {
    // 行结构对齐"冲突检测"列表：标题 + 文件名 + 优先级 + 位置/游戏开关 + 操作按钮。
    // 具体 DOM 由 mod-groups/member-row.js 生成，与「策略组管理」窗口共用同一份实现。
    const file = fileIndex.get(normalizeGroupKey(row.key)) || null;
    const planEntry = appState.priorityPlanMap?.get(normalizeGroupKey(row.key)) || null;
    memberList.appendChild(
      buildModMemberRow({
        row,
        file,
        priorityLabel: planEntry ? formatPriorityLabel(planEntry) : "",
        pick: {
          checked: row.selected,
          title: row.selected
            ? t("strategyGroups.suggest.unpickTitle")
            : t("strategyGroups.suggest.pickTitle"),
          onChange: (checked) => {
            const next = new Set(suggestionSelections.get(suggestion.id) || []);
            if (checked) next.add(row.key);
            else next.delete(row.key);
            suggestionSelections.set(suggestion.id, next);
            onSelectionChange?.();
          },
        },
        onActionDone: refreshAfterSuggestionMemberAction,
      }),
    );
  });
  return memberList;
}

// refreshAfterSuggestionMemberAction 是成员行动作（游戏开关 / 启用禁用 / 复制到 addons）
// 成功后的刷新：列表 → 组归属 → 建议（建议里的"已建组 / 缺失"状态会跟着变）。
// 动作本身由 member-row.js 调用 file-list/operations 执行，这里不再重复执行。
async function refreshAfterSuggestionMemberAction() {
  renderFileList();
  await refreshModGroupMembership();
  await refreshModGroupSuggestions();
}

async function createGroupFromSuggestion(suggestion, name, selection) {
  const keys = [...(selection || defaultSuggestionSelection(suggestion))];
  if (keys.length === 0) {
    showError(t("strategyGroups.view.createNone"));
    return;
  }
  try {
    const group = await CreateModStrategyGroupFromKeys(
      String(name || "").trim() || suggestion.label || t("strategyGroups.suggest.defaultLabel"),
      suggestion.reason || t("strategyGroups.suggest.defaultReason"),
      suggestion.strategy || "single",
      keys,
    );
    showNotification(
      t("strategyGroups.common.groupCreated", { name: group.name, count: keys.length }),
      "success",
    );
    await refreshModGroupMembership();
    await refreshModGroupSuggestions();
    renderFileList();
  } catch (error) {
    showError(t("strategyGroups.groupActions.createFailed", { error: String(error?.message || error) }));
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
      // 打开时先按最新状态重绘，避免在策略组管理窗口改完组后菜单还是旧的。
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
  // 「策略组管理…」：策略组生命周期的独立窗口（应用策略 / 权重 / 层级 / 多选批量管理）。
  element("mod-group-manager-btn")?.addEventListener("click", () => {
    void openStrategyGroupManager();
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
  element("mod-group-suggest-prompt-btn")?.addEventListener("click", () => {
    void openAgentPromptEditor();
  });
  element("mod-group-suggest-clear-btn")?.addEventListener("click", () => {
    void clearExternalGroupSuggestions();
  });
  // 筛选栏：搜索框防抖，下拉立即生效；「全部展开 / 全部折叠」批量切换卡片形态。
  let searchTimer = null;
  element("mod-group-suggest-search")?.addEventListener("input", () => {
    if (searchTimer) clearTimeout(searchTimer);
    searchTimer = setTimeout(() => {
      searchTimer = null;
      readSuggestionFilterFromControls();
      rerenderSuggestionView();
    }, 150);
  });
  ["mod-group-suggest-confidence", "mod-group-suggest-source", "mod-group-suggest-sort"].forEach((id) => {
    element(id)?.addEventListener("change", () => {
      readSuggestionFilterFromControls();
      rerenderSuggestionView();
    });
  });
  element("mod-group-suggest-tag-covered")?.addEventListener("change", () => {
    readSuggestionFilterFromControls();
    rerenderSuggestionView();
  });
  element("mod-group-suggest-expand-all-btn")?.addEventListener("click", () => {
    const view = applySuggestionView(appState.groupSuggestions || [], suggestionFilter);
    view.forEach((suggestion) => {
      if (suggestion?.id) suggestionExpanded.add(suggestion.id);
    });
    renderSuggestionList(appState.groupSuggestions || []);
  });
  element("mod-group-suggest-collapse-all-btn")?.addEventListener("click", () => {
    suggestionExpanded.clear();
    renderSuggestionList(appState.groupSuggestions || []);
  });
  element("mod-group-suggest-more-btn")?.addEventListener("click", () => {
    suggestionVisibleCount += SUGGESTION_PAGE_SIZE;
    rerenderSuggestionView({ resetPage: false });
  });
  element("mod-group-suggest-close-btn")?.addEventListener("click", closeModGroupSuggestionModal);
  element("mod-group-suggest-close-footer-btn")?.addEventListener("click", closeModGroupSuggestionModal);

  syncSuggestionFilterControls();
  observeSuggestionModalSize();

  // 组徽标点击 = 打开「策略组管理」窗口并定位到这一组（真实按钮，键盘可达）。
  //
  // 这里曾经是"就地整组开关"，但用户要的是"从这个 Mod 一眼看到它属于哪些组，
  // 然后在这一组里管理成员"。整组开关本身没有丢：工具栏「分组」菜单里每个组
  // 都还有「整组开关」，管理窗口的聚焦条上也有一个（见 strategy-group-manager.js）。
  document.addEventListener("click", (event) => {
    const badge = event.target.closest(".mod-group-badge");
    if (!badge) return;
    event.preventDefault();
    event.stopPropagation();
    const groupId = badge.dataset.groupId;
    if (!groupId) return;
    void openStrategyGroupManager({
      focusGroupId: groupId,
      focusFilePath: badge.dataset.filePath || "",
    });
  });

  void refreshModGroupMembership();
}

// captureGroupFromSelection 把当前勾选的 Mod 直接保存成一个策略组。
// 复用 CreateModStrategyGroupFromKeys：只写 groups.json，不动任何 Mod 文件。
async function captureGroupFromSelection() {
  const selected = [...(appState.selectedFiles || [])];
  if (selected.length === 0) {
    showError(t("strategyGroups.groupActions.captureNoSelection"));
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
    showError(t("strategyGroups.groupActions.captureNoKeys"));
    return;
  }
  // 用应用内输入弹窗代替 window.prompt：WebView2 没有默认 prompt 实现。
  showPromptModal(
    t("strategyGroups.groupActions.captureTitle"),
    t("strategyGroups.groupActions.captureMessage", { count: keys.length }),
    {
      confirmText: t("strategyGroups.groupActions.captureConfirm"),
      placeholder: t("strategyGroups.groupActions.capturePlaceholder"),
      onConfirm: (name) => createGroupFromSelectionKeys(name, keys),
    },
  );
}

async function createGroupFromSelectionKeys(name, keys) {
  const trimmed = String(name || "").trim();
  if (!trimmed) {
    showError(t("strategyGroups.groupActions.nameRequired"));
    return false;
  }
  try {
    const group = await CreateModStrategyGroupFromKeys(
      trimmed,
      t("strategyGroups.groupActions.captureReason"),
      "single",
      keys,
    );
    showNotification(
      t("strategyGroups.common.groupCreated", { name: group.name, count: keys.length }),
      "success",
    );
    await refreshModGroupMembership();
    await refreshFilesKeepFilter();
    renderFileList();
  } catch (error) {
    showError(t("strategyGroups.groupActions.createFailed", { error: String(error?.message || error) }));
    return false;
  }
}

// 语言切换（第 19 项 P2 收尾）：分组筛选按钮文案与下拉内容由 JS 渲染、挂在 mods 页常驻位置，
// 不在 page-change / locale-change 的既有重绘路径上——语言切换时原地重画（纯内存态）。
document.addEventListener("app:locale-change", () => {
  renderGroupFilterMenu();
});
