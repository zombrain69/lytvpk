// 加载顺序弹窗里的"优先级分层"面板。
//
// 关键约束（与后端一致）：
// - 保存/清除分层只写 priority.json，绝不改动 addonlist.txt；
// - 只有显式点击"按分层应用"才会按有效分层重排 addonlist.txt；
// - 未设置分层时有效分层等于顺序号，界面不显示额外噪音文案。

import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import {
  ApplyModPriorityLayers,
  ClearModPriority,
  GetModPriorityPlan,
  SetModPriority,
} from "../../../../wailsjs/go/app/App";
import { refreshLoadOrderMap } from "../file-list/sorting.js";
import { renderFileList } from "../file-list/render.js";
import {
  buildPriorityPlanMap,
  formatEffectiveLayer,
  normalizePriorityTier,
} from "../file-list/priority-label.mjs";

let controlsBound = false;
let pendingPriorityPlan = null;
let onAppliedCallback = null;

function addonListKeyForFile(file) {
  if (!file?.name) return "";
  const name = String(file.name).replaceAll("/", "\\").toLowerCase();
  return file.location === "workshop" ? `workshop\\${name}` : name;
}

function currentFileFromState() {
  const filePath = appState.loadOrderPriorityFilePath || null;
  if (!filePath) return null;
  return (appState.vpkFiles || []).find((item) => item.path === filePath) || null;
}

function element(id) {
  return document.getElementById(id);
}

function renderPriorityPanel(entry) {
  const input = element("load-order-tier-input");
  const effective = element("load-order-priority-effective");
  const panel = element("load-order-priority-panel");
  if (!entry) {
    if (input) input.value = "";
    if (effective) effective.textContent = "";
    panel?.classList.add("hidden");
    return;
  }
  panel?.classList.remove("hidden");
  if (input) {
    input.value = entry.tier === null || entry.tier === undefined ? "" : String(entry.tier);
  }
  if (effective) {
    const order = Number.isInteger(entry.order) ? entry.order : 0;
    const parts = [`顺序号 #${order > 0 ? order : "未记录"}`];
    if (entry.groupTier !== null && entry.groupTier !== undefined) {
      parts.push(`策略组权重 ${entry.groupTier}`);
    }
    const explanation = formatEffectiveLayer(entry);
    if (explanation) parts.push(explanation);
    else parts.push(`有效分层 ${entry.effective}`);
    effective.textContent = parts.join(" · ");
  }
}

function readTierInput() {
  const input = element("load-order-tier-input");
  if (!input) return null;
  return normalizePriorityTier(input.value);
}

// 保存/清除分层都会改变"有效分层"，列表角标必须跟着更新，
// 否则用户看到的是过期标签（例如已设置分层 42 却仍显示「优先级 #998」）。
// 宿主弹窗提供了统一的刷新回调（同时刷新列表与弹窗），没有回调时退化为只刷列表。
async function refreshAfterTierChange() {
  if (typeof onAppliedCallback === "function") {
    await onAppliedCallback();
    return;
  }
  try {
    await refreshLoadOrderMap({ silent: true });
    renderFileList();
  } catch (error) {
    console.warn("保存分层后刷新列表失败:", error);
  }
  await refreshPriorityPanel();
}

async function saveCurrentTier() {
  const file = currentFileFromState();
  const key = addonListKeyForFile(file);
  if (!key) {
    showError("当前 Mod 没有可用的 addonlist 键，无法设置分层");
    return;
  }
  const tier = readTierInput();
  if (tier === null) {
    showError("请输入整数分层（可为负数；数值越小越先加载）");
    return;
  }
  try {
    await SetModPriority(key, file?.name || key, tier);
    await refreshAfterTierChange();
    showNotification("已保存分层；点击“按分层应用”后才会重排 addonlist.txt", "success");
  } catch (err) {
    console.error("保存优先级分层失败:", err);
    showError("保存分层失败: " + err);
  }
}

async function clearCurrentTier() {
  const file = currentFileFromState();
  const key = addonListKeyForFile(file);
  if (!key) {
    showError("当前 Mod 没有可用的 addonlist 键，无法清除分层");
    return;
  }
  try {
    await ClearModPriority(key);
    await refreshAfterTierChange();
    showNotification("已清除分层，该 Mod 回到按顺序号判定", "success");
  } catch (err) {
    console.error("清除优先级分层失败:", err);
    showError("清除分层失败: " + err);
  }
}

function stepTier(delta) {
  const input = element("load-order-tier-input");
  if (!input) return;
  const current = normalizePriorityTier(input.value);
  const base = current === null ? 0 : current;
  input.value = String(base + delta);
}

async function applyPriorityLayers() {
  try {
    const preview = await ApplyModPriorityLayers();
    const changed = Array.isArray(preview?.entries) ? preview.entries.length : 0;
    if (typeof onAppliedCallback === "function") {
      await onAppliedCallback();
    } else {
      await refreshPriorityPanel();
    }
    showNotification(`已按分层重排 ${changed} 个条目`, "success");
  } catch (err) {
    console.error("按分层应用失败:", err);
    showError("按分层应用失败: " + err);
  }
}

async function refreshPriorityPanel() {
  const file = currentFileFromState();
  if (!file) {
    renderPriorityPanel(null);
    return;
  }
  try {
    const plan = await GetModPriorityPlan();
    pendingPriorityPlan = buildPriorityPlanMap(plan);
  } catch (err) {
    console.error("读取优先级分层失败:", err);
    pendingPriorityPlan = new Map();
  }
  renderPriorityPanel(pendingPriorityPlan.get(addonListKeyForFile(file)) || null);
}

/**
 * initLoadOrderPriorityControls 只绑定一次事件；onApplied 用于让宿主弹窗刷新自己的预览。
 */
export function initLoadOrderPriorityControls(onApplied) {
  if (typeof onApplied === "function") onAppliedCallback = onApplied;
  if (controlsBound) return;
  controlsBound = true;

  element("load-order-tier-minus-btn")?.addEventListener("click", () => stepTier(-1));
  element("load-order-tier-plus-btn")?.addEventListener("click", () => stepTier(1));
  element("load-order-tier-apply-btn")?.addEventListener("click", saveCurrentTier);
  element("load-order-tier-clear-btn")?.addEventListener("click", clearCurrentTier);
  element("apply-priority-layers-btn")?.addEventListener("click", applyPriorityLayers);
}

/** syncLoadOrderPriorityPanel 在单 Mod 模式下刷新分层面板。 */
export async function syncLoadOrderPriorityPanel(filePath) {
  appState.loadOrderPriorityFilePath = filePath || null;
  await refreshPriorityPanel();
}
