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
      "还没有策略组。可以先点「分组建议」自动推导，或点「策略组管理…」用选中的 Mod 建组。";
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
  // 顺序说明：这一条回答"外面这个按分组筛选是什么顺序"，也提示怎么把常用组排上去。
  const orderHint = document.createElement("div");
  orderHint.className = "mod-group-filter-order-hint";
  orderHint.textContent = "顺序＝组权重（未设置排最后）· 子组紧跟上级分组";
  orderHint.title =
    "组权重在「分组 → 策略组管理…」窗口里设置；给常用组填一个更小的权重，它会排到这份列表最上面。\n" +
    "子组永远紧跟自己的上级分组：给子组设权重，它所在的整棵分组树会一起上浮（子组仍缩进显示）。\n" +
    "「上级分组」只决定层级归属与缩进，不影响优先级。";
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
      `${formatGroupStrategy(option.strategy)} · 共 ${option.memberCount} 个成员` +
      (Number.isFinite(option.tier ?? NaN) ? ` · 组权重 ${option.tier}` : " · 未设置组权重") +
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

    // 成员编辑：把列表里勾选的 Mod 批量加入 / 移出这个组（只写 groups.json）。
    const selectedKeys = selectedGroupActionKeys();
    const addSelected = document.createElement("button");
    addSelected.type = "button";
    addSelected.className = "btn-link";
    addSelected.textContent = `＋ 选中的 ${selectedKeys.length} 个`;
    addSelected.disabled = selectedKeys.length === 0;
    addSelected.title =
      selectedKeys.length === 0
        ? "先在列表里勾选要加入这个组的 Mod"
        : `把当前勾选的 ${selectedKeys.length} 个 Mod 加入「${option.name}」（只写 groups.json）`;
    addSelected.addEventListener("click", (event) => {
      event.stopPropagation();
      void addSelectedModsToGroup(option.id, option.name);
    });
    const removeSelected = document.createElement("button");
    removeSelected.type = "button";
    removeSelected.className = "btn-link";
    removeSelected.textContent = `－ 选中的 ${selectedKeys.length} 个`;
    removeSelected.disabled = selectedKeys.length === 0;
    removeSelected.title =
      selectedKeys.length === 0
        ? "先在列表里勾选要移出这个组的 Mod"
        : `把当前勾选的 Mod 移出「${option.name}」（只写 groups.json，不动 Mod 文件）`;
    removeSelected.addEventListener("click", (event) => {
      event.stopPropagation();
      void removeSelectedModsFromGroup(option.id, option.name);
    });

    // 重命名 / 删除：删除前用应用内确认弹窗说明影响范围。
    const rename = document.createElement("button");
    rename.type = "button";
    rename.className = "btn-link";
    rename.textContent = "重命名";
    rename.title = "修改这个策略组的名称与描述（成员与优先级都不受影响）";
    rename.addEventListener("click", (event) => {
      event.stopPropagation();
      void renameStrategyGroup(option.id, option.name);
    });
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "btn-link is-danger";
    remove.textContent = "删除";
    remove.title = "删除这个策略组（只删 groups.json 里的记录，不改动 addonlist.txt 与 Mod 文件）";
    remove.addEventListener("click", (event) => {
      event.stopPropagation();
      void confirmDeleteStrategyGroup(option);
    });

    // 组 → 标签：把这一组沉淀成一个统一标签，之后就能用标签筛选代替建组。
    const tagButton = document.createElement("button");
    tagButton.type = "button";
    tagButton.className = "btn-link";
    tagButton.textContent = "打标签…";
    tagButton.title = "给这一组的全部成员补一个统一标签（标签写在文件名里，会自动改绑本地记录的键）";
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
    showError("请先在列表里勾选要加入这个组的 Mod");
    return;
  }
  try {
    const before = groupMemberCount(groupId);
    const group = await AddModStrategyGroupMembers(groupId, keys);
    const added = Math.max(0, (group.members || []).length - before);
    showNotification(
      added > 0
        ? `已把 ${added} 个 Mod 加入「${groupName}」（现在共 ${(group.members || []).length} 个成员）`
        : `选中的 Mod 已经都在「${groupName}」里了`,
      added > 0 ? "success" : "info",
    );
    await refreshModGroupMembership();
    renderFileList();
    renderGroupFilterMenu();
  } catch (error) {
    showError("加入策略组失败: " + String(error?.message || error));
  }
}

async function removeSelectedModsFromGroup(groupId, groupName) {
  const keys = selectedGroupActionKeys();
  if (keys.length === 0) {
    showError("请先在列表里勾选要移出这个组的 Mod");
    return;
  }
  try {
    const group = await RemoveModStrategyGroupMembers(groupId, keys);
    showNotification(
      `已把 ${keys.length} 个 Mod 移出「${groupName}」（现在共 ${(group.members || []).length} 个成员）`,
      "success",
    );
    await refreshModGroupMembership();
    renderFileList();
    renderGroupFilterMenu();
  } catch (error) {
    showError("移出策略组失败: " + String(error?.message || error));
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
  showPromptModal("重命名策略组", `修改「${currentName}」的名称；描述可以留空。只写 groups.json 里的这一条记录。`, {
    defaultValue: currentName,
    confirmText: "保存名称",
    placeholder: "策略组名称",
    onConfirm: async (value) => {
      try {
        const group = await RenameModStrategyGroup(groupId, value, description);
        showNotification(`已重命名为「${group.name}」`, "success");
        await refreshModGroupMembership();
        renderGroupFilterMenu();
      } catch (error) {
        showError("重命名失败: " + String(error?.message || error));
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
    "删除策略组",
    `删除「${option.name}」：\n` +
      `· 只删除 groups.json 里的这一条记录（${memberCount} 个成员）\n` +
      "· 不会改动 addonlist.txt，也不会删除任何 Mod 文件\n" +
      "· 组成员里已经写入的 Mod 分层（priority.json）会保留\n" +
      (childCount > 0
        ? `· 它的 ${childCount} 个下级分组会回到顶层，不会被一起删除\n`
        : "") +
      "删除后这个组就没了（可以重新用「分组建议」或勾选 Mod 建组）。是否继续？",
    async () => {
      try {
        await DeleteModStrategyGroup(option.id);
        const next = new Set(appState.activeGroupFilter);
        next.delete(option.id);
        appState.activeGroupFilter = next;
        showNotification(`已删除策略组「${option.name}」`, "success");
        await refreshModGroupMembership();
        renderFileList();
        renderGroupFilterMenu();
        await refreshFilesKeepFilter();
      } catch (error) {
        showError("删除策略组失败: " + String(error?.message || error));
      }
    },
  );
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
  // 后端会先重新扫描一遍目录（增量缓存），所以这一步可能比纯导出稍慢：
  // 用按钮文案给一个"正在扫描"的反馈。
  const prepareButton = element("mod-group-suggest-prepare-btn");
  const originalLabel = prepareButton?.textContent || "";
  if (prepareButton) {
    prepareButton.disabled = true;
    prepareButton.textContent = "正在扫描并导出…";
  }
  try {
    const workspace = await PrepareGroupingWorkspace();
    if (!workspace?.catalogPath) {
      showError("准备材料失败：清单路径为空");
      return;
    }
    const copied = await copyTextToClipboard(workspace.prompt || "");
    showNotification(
      `已按最新扫描导出 Mod 清单（${workspace.modCount} 个）` +
        (copied ? "，提示词已复制到剪贴板" : "；提示词请点「复制智能体提示词」"),
      copied ? "success" : "info",
    );
    showConfirmModal(
      "交给智能体的材料已就绪",
      `<div style="line-height:1.6">` +
        `<p style="margin:0 0 8px">这份材料是<b>刚重新扫描过</b>的最新状态（${workspace.modCount} 个 Mod）。</p>` +
        (workspace.notice
          ? `<p style="margin:0 0 8px;color:var(--warning,#f5a524)">${escapeHtml(workspace.notice)}</p>`
          : "") +
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
  // 后端在导出前会重新扫描一遍目录，这里给一个可感知的进行中状态。
  const exportButton = element("mod-group-suggest-export-btn");
  const originalLabel = exportButton?.textContent || "";
  if (exportButton) {
    exportButton.disabled = true;
    exportButton.textContent = "正在扫描并导出…";
  }
  try {
    const path = await ExportGroupingCatalogDialog();
    if (!path) {
      return; // 用户取消
    }
    const inbox = await GetGroupSuggestionInboxPath().catch(() => "");
    showNotification(`已按最新扫描导出 Mod 清单：${path}`, "success");
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
  if (summary) summary.textContent = "正在推导…";
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
    if (summary) summary.textContent = "推导失败: " + String(error?.message || error);
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
        ? "没有发现明显同组的 Mod（可能已经都分好组了）"
        : `共 ${all.length} 条建议` +
          (externalCount > 0 ? `（其中 ${externalCount} 条来自导入的建议文件）` : "") +
          (tagCoveredCount > 0 ? ` · ${tagCoveredCount} 条标签已覆盖（可直接用标签筛选）` : "") +
          " · 客观证据（合集/文件夹结构/前缀/标签/作者）与小规模组优先";
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
      ? `显示更多建议（还有 ${page.hiddenCount} 条）`
      : "显示更多建议";
  }

  if (all.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-suggest-empty";
    empty.textContent = "可以先用标签或工坊合集整理出更明确的特征，再回来推导。";
    list.appendChild(empty);
    return;
  }
  if (view.length === 0) {
    const empty = document.createElement("div");
    empty.className = "mod-group-suggest-empty";
    empty.textContent = "没有符合筛选条件的建议。可以清空搜索词，或把置信度改回“全部置信度”。";
    const reset = document.createElement("button");
    reset.type = "button";
    reset.className = "btn btn-small btn-outline";
    reset.textContent = "清除筛选";
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
      titleInput.value = suggestion.label || "新策略组";
      titleInput.maxLength = 60;
      titleInput.setAttribute("aria-label", "策略组名称");
      head.append(titleInput);
    } else {
      const titleText = document.createElement("button");
      titleText.type = "button";
      titleText.className = "mod-group-suggest-title";
      titleText.textContent = suggestion.label || "新策略组";
      titleText.title = "展开这条建议，查看成员与逐个操作";
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
      confidence.textContent += " · 已建组";
    }
    head.append(confidence);
    if (suggestion.source === "external") {
      const sourceBadge = document.createElement("span");
      sourceBadge.className = "mod-group-suggest-source";
      sourceBadge.textContent = "导入建议";
      sourceBadge.title = "来自你导入的建议文件（大模型 / 人工整理），优先展示";
      head.append(sourceBadge);
    }
    const memberCountBadge = document.createElement("span");
    memberCountBadge.className = "mod-group-suggest-count";
    memberCountBadge.textContent = `${(suggestion.memberKeys || []).length} 个 Mod`;
    head.append(memberCountBadge);
    if (isSuggestionTagCovered(suggestion)) {
      // 标签恰好只能筛出这一批 Mod：建组的增量价值低，给一个直接跳到标签筛选的入口。
      const tagBadge = document.createElement("span");
      tagBadge.className = "mod-group-suggest-tag-badge";
      tagBadge.textContent = `标签已覆盖：${suggestion.tagKey}`;
      tagBadge.title =
        "这个标签恰好只能筛出这一批 Mod（没有别的 Mod 带它）：直接用标签筛选就能达到同样效果，不必再建组";
      head.append(tagBadge);
      const filterByTag = document.createElement("button");
      filterByTag.type = "button";
      filterByTag.className = "btn btn-small btn-outline";
      filterByTag.textContent = "用标签筛选";
      filterByTag.title = `关闭弹窗，并在标签筛选里选中「${suggestion.tagKey}」`;
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
          ? `导入标签：${importedTagPlan[0].tag}`
          : `导入标签：${importedTagPlan[0].tag} 等 ${importedTagPlan.length} 个`;
      importedBadge.title = [suggestion.tagReason, "来自建议文件里的 tag / memberTags"]
        .filter(Boolean)
        .join(" · ");
      head.append(importedBadge);
      const applyTagButton = document.createElement("button");
      applyTagButton.type = "button";
      applyTagButton.className = "btn btn-small btn-outline";
      applyTagButton.textContent = "应用标签";
      applyTagButton.title = `给这条建议的成员打上标签：${importedTagPlan
        .map((entry) => `${entry.tag}（${entry.keys.length} 个）`)
        .join("、")}`;
      applyTagButton.addEventListener("click", () => {
        void applyImportedTags(suggestion, importedTagPlan);
      });
      head.append(applyTagButton);
    }

    const toggle = document.createElement("button");
    toggle.type = "button";
    toggle.className = "btn btn-small btn-outline mod-group-suggest-toggle";
    toggle.textContent = expanded ? "收起" : "展开详情";
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
      more.textContent = `…还有 ${previewInfo.more} 个`;
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
    selectAllButton.textContent = "全选";
    selectAllButton.title = "勾选这一组的全部 Mod";
    selectAllButton.addEventListener("click", () => {
      const all = defaultSuggestionSelection(suggestion);
      suggestionSelections.set(suggestion.id, all);
      applySelectionToCard(card, suggestion);
    });
    const clearAllButton = document.createElement("button");
    clearAllButton.type = "button";
    clearAllButton.className = "btn btn-small btn-outline";
    clearAllButton.textContent = "全不选";
    clearAllButton.title = "取消勾选这一组的全部 Mod";
    clearAllButton.addEventListener("click", () => {
      suggestionSelections.set(suggestion.id, new Set());
      applySelectionToCard(card, suggestion);
    });
    const createButton = document.createElement("button");
    createButton.type = "button";
    createButton.className = "btn btn-small btn-primary";
    createButton.textContent = expanded
      ? "创建为组"
      : `创建为组（${selection.size}）`;
    if (suggestion.existingGroupId) {
      createButton.disabled = true;
      createButton.title = "这批 Mod 已经属于同一个策略组，无需重复创建";
    }
    createButton.addEventListener("click", () => {
      const name = titleInput?.value || suggestion.label || "新策略组";
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
    createButton.textContent = selection.size === 0 ? "创建为组" : `创建为组（${selection.size}）`;
  }
}

// applyTagFilterFromSuggestion 关闭建议弹窗，改用标签筛选（组建议 → 标签 的出口）。
async function applyTagFilterFromSuggestion(suggestion) {
  const tag = String(suggestion?.tagKey || "").trim();
  if (!tag) return;
  closeModGroupSuggestionModal();
  appState.selectedSecondaryTags = [tag];
  showNotification(`已按标签「${tag}」筛选（标签筛选里点「全部」可取消）`, "success");
  await refreshFilesKeepFilter();
  renderFileList();
}

// applyImportedTags 应用建议文件里带的标签（智能体给的 tag / memberTags）。
// 会改文件名（＝改 addonlist 键），所以先用应用内确认框说明影响，后端负责把本地记录改绑。
async function applyImportedTags(suggestion, plan) {
  const summary = plan.map((entry) => `「${entry.tag}」× ${entry.keys.length} 个`).join("、");
  showConfirmModal(
    "应用导入标签",
    `给「${suggestion.label || "这条建议"}」的成员打标签：${summary}。\n` +
      "标签写在文件名里（工坊 Mod 写进 .meta），会改变 addonlist 键；" +
      "工具会自动把策略组 / 分层 / 依赖 / 忽略清单里的旧键改绑到新键。\n" +
      "已经带该标签的成员会被跳过。是否继续？",
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
          `已应用 ${plan.length} 个标签：新增 ${applied} 个${skipped > 0 ? `，${skipped} 个已有` : ""}` +
            `${missing > 0 ? `，${missing} 个文件缺失` : ""}${failed > 0 ? `，${failed} 个失败` : ""}`,
          failed > 0 ? "warning" : "success",
        );
        if (notes.length > 0) console.warn("应用导入标签失败详情:", notes);
        await refreshModGroupMembership();
        renderFileList();
        await refreshModGroupSuggestions();
      } catch (error) {
        showError("应用导入标签失败: " + String(error?.message || error));
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
          title: row.selected ? "取消勾选这个 Mod" : "把这个 Mod 加入该组",
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
