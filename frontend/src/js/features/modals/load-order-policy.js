import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import {
  ApplyAddonListLoadOrderPolicy,
  GetAddonListLoadOrderEntries,
  GetVPKLoadOrder,
  PreviewAddonListLoadOrderPolicy,
  SetVPKLoadOrder,
} from "../../../../wailsjs/go/app/App";

let currentLoadOrderFile = null;
let currentOrder = -1;
let currentEntries = [];
let currentPolicy = createEmptyPolicy();
let controlsBound = false;

function createEmptyPolicy() {
  return { rootFirst: false, groupWorkshop: false, constraints: [] };
}

export async function openEnhancedLoadOrderModal(filePath) {
  const file = appState.vpkFiles.find((item) => item.path === filePath);
  if (!file) return;

  currentLoadOrderFile = filePath;
  currentOrder = -1;
  currentEntries = [];
  currentPolicy = createEmptyPolicy();
  setupLoadOrderControls();

  document.getElementById("load-order-filename").textContent = file.name;
  document.getElementById("load-order-root-first").checked = false;
  document.getElementById("load-order-group-workshop").checked = false;
  document.getElementById("load-order-current").textContent = "正在获取...";
  document.getElementById("load-order-input").value = "";
  document.getElementById("load-order-modal").classList.remove("hidden");

  try {
    await refreshLoadOrderModal();
    document.getElementById("load-order-input").focus();
  } catch (err) {
    console.error("获取加载顺序失败:", err);
    closeEnhancedLoadOrderModal();
    if (String(err).includes("addonlist.txt 不存在")) {
      showError("未找到 addonlist.txt 文件，无法设置加载顺序");
      return;
    }
    showError("获取加载顺序失败: " + err);
  }
}

export function closeEnhancedLoadOrderModal() {
  document.getElementById("load-order-modal").classList.add("hidden");
  currentLoadOrderFile = null;
}

export async function saveEnhancedLoadOrder() {
  if (!currentLoadOrderFile) return;
  const order = Number.parseInt(document.getElementById("load-order-input").value.trim(), 10);
  if (!Number.isInteger(order)) {
    showError("请输入有效的序号");
    return;
  }
  try {
    await SetVPKLoadOrder(currentLoadOrderFile, order);
    await refreshLoadOrderModal();
    showNotification("单项加载顺序已保存", "success");
  } catch (err) {
    console.error("保存加载顺序失败:", err);
    showError("保存失败: " + err);
  }
}

function setupLoadOrderControls() {
  if (controlsBound) return;
  controlsBound = true;

  document.getElementById("move-load-order-up-btn")?.addEventListener("click", () => moveCurrentFileBy(-1));
  document.getElementById("move-load-order-down-btn")?.addEventListener("click", () => moveCurrentFileBy(1));
  document.getElementById("add-load-order-before-rule-btn")?.addEventListener("click", () => addConstraints("before"));
  document.getElementById("add-load-order-after-rule-btn")?.addEventListener("click", () => addConstraints("after"));
  document.getElementById("preview-load-order-policy-btn")?.addEventListener("click", previewLoadOrderPolicy);
  document.getElementById("apply-load-order-policy-btn")?.addEventListener("click", applyLoadOrderPolicy);
  document.getElementById("load-order-root-first")?.addEventListener("change", (event) => {
    currentPolicy.rootFirst = event.target.checked;
  });
  document.getElementById("load-order-group-workshop")?.addEventListener("change", (event) => {
    currentPolicy.groupWorkshop = event.target.checked;
  });
  document.getElementById("load-order-rules")?.addEventListener("click", (event) => {
    const button = event.target.closest("[data-load-order-rule-index]");
    if (!button) return;
    const index = Number.parseInt(button.dataset.loadOrderRuleIndex, 10);
    if (!Number.isInteger(index)) return;
    currentPolicy.constraints.splice(index, 1);
    renderRules();
  });
}

async function refreshLoadOrderModal() {
  if (!currentLoadOrderFile) return;
  const [order, entries] = await Promise.all([
    GetVPKLoadOrder(currentLoadOrderFile),
    GetAddonListLoadOrderEntries(),
  ]);
  currentOrder = order;
  currentEntries = entries || [];
  renderCurrentOrder();
  renderRuleSelectors();
  renderRules();
  renderPreview(currentEntries, "当前 addonlist.txt 顺序");
}

function renderCurrentOrder() {
  const currentOrderEl = document.getElementById("load-order-current");
  const input = document.getElementById("load-order-input");
  const upButton = document.getElementById("move-load-order-up-btn");
  const downButton = document.getElementById("move-load-order-down-btn");
  if (currentOrder > 0) {
    currentOrderEl.textContent = currentOrder;
    input.placeholder = String(currentOrder);
  } else {
    currentOrderEl.textContent = "未生成";
    input.placeholder = "输入新的序号";
  }
  upButton.disabled = currentOrder <= 1;
  downButton.disabled = currentOrder <= 0 || currentOrder >= currentEntries.length;
}

async function moveCurrentFileBy(delta) {
  if (!currentLoadOrderFile || currentOrder <= 0) {
    showError("该 Mod 尚未写入 addonlist.txt，请先输入序号保存");
    return;
  }
  const destination = Math.min(Math.max(currentOrder + delta, 1), currentEntries.length);
  if (destination === currentOrder) return;
  try {
    await SetVPKLoadOrder(currentLoadOrderFile, destination);
    await refreshLoadOrderModal();
    showNotification(delta < 0 ? "Mod 已上移一位" : "Mod 已下移一位", "success");
  } catch (err) {
    showError("调整优先级失败: " + err);
  }
}

function renderRuleSelectors() {
  const sources = document.getElementById("load-order-rule-sources");
  const target = document.getElementById("load-order-rule-target");
  const selectedSources = new Set(Array.from(sources.selectedOptions).map((option) => option.value));
  const selectedTarget = target.value;
  sources.replaceChildren();
  target.replaceChildren();
  currentEntries.forEach((entry) => {
    const label = entryLabel(entry);
    sources.add(new Option(label, entry.key, false, selectedSources.has(entry.key)));
    target.add(new Option(label, entry.key, false, selectedTarget === entry.key));
  });
}

function addConstraints(direction) {
  const sources = Array.from(document.getElementById("load-order-rule-sources").selectedOptions).map((option) => option.value);
  const target = document.getElementById("load-order-rule-target").value;
  if (!target || sources.length === 0) {
    showError("请至少选择一个 Mod，并选择相对目标");
    return;
  }

  let added = 0;
  sources.forEach((source) => {
    if (source === target) return;
    const constraint = direction === "before" ? { before: source, after: target } : { before: target, after: source };
    const exists = currentPolicy.constraints.some((item) => item.before === constraint.before && item.after === constraint.after);
    if (!exists) {
      currentPolicy.constraints.push(constraint);
      added++;
    }
  });
  if (added === 0) {
    showError("不能把同一个 Mod 设为自身的前后关系，或该规则已存在");
    return;
  }
  renderRules();
}

function renderRules() {
  const container = document.getElementById("load-order-rules");
  container.replaceChildren();
  if (currentPolicy.constraints.length === 0) {
    const empty = document.createElement("div");
    empty.className = "load-order-rule-empty";
    empty.textContent = "尚未添加强制前后关系。";
    container.appendChild(empty);
    return;
  }
  currentPolicy.constraints.forEach((constraint, index) => {
    const row = document.createElement("div");
    row.className = "load-order-rule";
    const text = document.createElement("span");
    text.className = "load-order-rule-text";
    text.textContent = `${constraint.before} 始终在 ${constraint.after} 前面`;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "load-order-rule-delete";
    remove.dataset.loadOrderRuleIndex = String(index);
    remove.textContent = "移除";
    row.append(text, remove);
    container.appendChild(row);
  });
}

async function previewLoadOrderPolicy() {
  if (!currentLoadOrderFile) return;
  try {
    const preview = await PreviewAddonListLoadOrderPolicy(currentPolicy);
    renderPreview(preview.entries || [], "预览：尚未写入");
    showNotification("已生成加载顺序预览", "info");
  } catch (err) {
    console.error("预览加载顺序失败:", err);
    showError("无法应用该排序策略: " + err);
  }
}

async function applyLoadOrderPolicy() {
  if (!currentLoadOrderFile) return;
  try {
    const result = await ApplyAddonListLoadOrderPolicy(currentPolicy);
    currentEntries = result.entries || [];
    renderRuleSelectors();
    renderPreview(currentEntries, "已写入 addonlist.txt，并同步保护快照");
    currentOrder = await GetVPKLoadOrder(currentLoadOrderFile);
    renderCurrentOrder();
    showNotification("加载顺序优化已写入 addonlist.txt", "success");
  } catch (err) {
    console.error("应用加载顺序失败:", err);
    showError("写入失败: " + err);
  }
}

function renderPreview(entries, summary) {
  const container = document.getElementById("load-order-preview");
  document.getElementById("load-order-preview-summary").textContent = `${summary} · ${entries.length} 个条目`;
  container.replaceChildren();
  if (entries.length === 0) {
    const empty = document.createElement("div");
    empty.className = "load-order-preview-empty";
    empty.textContent = "addonlist.txt 中尚无 Mod 条目。";
    container.appendChild(empty);
    return;
  }

  const fragment = document.createDocumentFragment();
  entries.forEach((entry) => {
    const row = document.createElement("div");
    row.className = "load-order-preview-item";
    const order = document.createElement("span");
    order.className = "load-order-preview-number";
    order.textContent = String(entry.order);
    const type = document.createElement("span");
    const location = entry.isWorkshop ? "workshop" : entry.isRoot ? "root" : "other";
    type.className = `load-order-preview-type ${location}`;
    type.textContent = entry.isWorkshop ? "工坊" : entry.isRoot ? "根目录" : "子目录";
    const key = document.createElement("span");
    key.className = "load-order-preview-key";
    key.title = entry.key;
    key.textContent = entry.key;
    const state = document.createElement("span");
    const enabled = entry.value === "1";
    state.className = `load-order-preview-state ${enabled ? "enabled" : "disabled"}`;
    state.textContent = enabled ? "游戏内开启" : "游戏内关闭";
    row.append(order, type, key, state);
    fragment.appendChild(row);
  });
  container.appendChild(fragment);
}

function entryLabel(entry) {
  return `${entry.order}. ${entry.key}${entry.value === "1" ? "（开启）" : "（关闭）"}`;
}
