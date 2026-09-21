// 变更驱动的冲突自动复检（前端侧）。
//
// 后端在 addonlist.txt 写入、Mod 文件移动/重命名/删除之后只标记"待复检"并广播
// conflict_recheck_invalidated；这里做防抖后按需拉取角标与修复建议。
// 修复建议永远是"建议"：采纳只会写入 priority.json，真正重排仍需要用户显式应用分层。

import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import {
  GetConflictBadges,
  GetConflictFixSuggestions,
  GetConflictRecheckStatus,
  SetModPriority,
} from "../../../../wailsjs/go/app/App";
import { buildConflictBadgeMap } from "./conflict-badge.mjs";
import { renderFileList } from "../file-list/render.js";

const RECHECK_DEBOUNCE_MS = 400;

let initialized = false;
let debounceTimer = null;
let pendingSuggestionApply = false;
let onSuggestionsChanged = null;

function scheduleBadgeRefresh() {
  if (debounceTimer) window.clearTimeout(debounceTimer);
  debounceTimer = window.setTimeout(() => {
    debounceTimer = null;
    void refreshConflictBadges();
  }, RECHECK_DEBOUNCE_MS);
}

/** refreshConflictBadges 拉取角标数据并刷新列表；失败时清空角标而不是展示过期数据。 */
export async function refreshConflictBadges() {
  try {
    const badges = await GetConflictBadges();
    const { byPath, byKey } = buildConflictBadgeMap(badges);
    appState.conflictBadgeByPath = byPath;
    appState.conflictBadgeByKey = byKey;
  } catch (error) {
    appState.conflictBadgeByPath = new Map();
    appState.conflictBadgeByKey = new Map();
    console.warn("刷新冲突角标失败:", error);
  }
  try {
    renderFileList();
  } catch (error) {
    console.warn("刷新冲突角标后重绘列表失败:", error);
  }
}

/** getConflictRecheckStatus 供设置页/诊断页展示缓存状态。 */
export async function getConflictRecheckStatus() {
  try {
    return await GetConflictRecheckStatus();
  } catch (error) {
    console.warn("读取自动复检状态失败:", error);
    return null;
  }
}

/** loadConflictFixSuggestions 读取修复建议；只读，不会改动任何文件。 */
export async function loadConflictFixSuggestions() {
  try {
    return (await GetConflictFixSuggestions()) || [];
  } catch (error) {
    console.warn("读取修复建议失败:", error);
    return [];
  }
}

/**
 * applyConflictFixSuggestion 采纳一条建议：只保存分层（priority.json），
 * 不重排 addonlist.txt。用户仍需显式执行“按分层应用”才会改变游戏侧顺序。
 */
export async function applyConflictFixSuggestion(suggestion) {
  if (!suggestion || suggestion.action !== "set-tier" || !suggestion.targetKey) {
    showError("这条建议需要手动处理（请先让该 Mod 写入 addonlist.txt）");
    return false;
  }
  if (pendingSuggestionApply) return false;
  pendingSuggestionApply = true;
  try {
    await SetModPriority(
      suggestion.targetKey,
      suggestion.targetName || suggestion.targetKey,
      Number(suggestion.suggestedTier),
    );
    showNotification(
      `已把“${suggestion.targetName || suggestion.targetKey}”的分层设为 ${suggestion.suggestedTier}；到“编辑加载顺序”点“按分层应用”后生效`,
      "success",
    );
    if (typeof onSuggestionsChanged === "function") {
      await onSuggestionsChanged();
    }
    return true;
  } catch (error) {
    showError("保存分层失败: " + String(error?.message || error));
    return false;
  } finally {
    pendingSuggestionApply = false;
  }
}

/** initConflictRecheck 注册事件监听并做一次初始刷新（幂等）。 */
export function initConflictRecheck(options = {}) {
  if (typeof options.onSuggestionsChanged === "function") {
    onSuggestionsChanged = options.onSuggestionsChanged;
  }
  if (initialized) return;
  initialized = true;
  EventsOn("conflict_recheck_invalidated", () => scheduleBadgeRefresh());
  void refreshConflictBadges();
}
