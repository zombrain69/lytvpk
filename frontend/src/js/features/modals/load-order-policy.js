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
let currentMode = "global";
let selectedSourceKeys = new Set();
let selectedPaths = [];
let selectedPathNames = [];
let entryFileByKey = new Map();
let entrySearchTextByKey = new Map();

const LOAD_ORDER_PRESETS = {
  recommended: {
    label: "推荐：目录规整",
    policy: { rootFirst: true, groupWorkshop: true, stateOrder: "keep" },
  },
  "root-first": {
    label: "根目录优先",
    policy: { rootFirst: true, groupWorkshop: false, stateOrder: "keep" },
  },
  "group-workshop": {
    label: "工坊聚集",
    policy: { rootFirst: false, groupWorkshop: true, stateOrder: "keep" },
  },
  "enabled-first": {
    label: "开启项优先",
    policy: { rootFirst: false, groupWorkshop: false, stateOrder: "enabled-first" },
  },
  "disabled-first": {
    label: "关闭项优先",
    policy: { rootFirst: false, groupWorkshop: false, stateOrder: "disabled-first" },
  },
};

function createEmptyPolicy() {
  return { rootFirst: false, groupWorkshop: false, stateOrder: "keep", constraints: [] };
}

export async function openEnhancedLoadOrderModal(context = {}) {
  const normalizedContext = normalizeLoadOrderContext(context);
  const file = normalizedContext.filePath
    ? appState.vpkFiles.find((item) => item.path === normalizedContext.filePath)
    : null;
  if (normalizedContext.mode === "single" && !file) return;

  currentMode = normalizedContext.mode;
  currentLoadOrderFile = file?.path || null;
  currentOrder = -1;
  currentEntries = [];
  currentPolicy = createEmptyPolicy();
  selectedSourceKeys = new Set();
  selectedPaths = normalizedContext.selectedPaths;
  selectedPathNames = selectedPaths
    .map((path) => appState.vpkFiles.find((item) => item.path === path)?.name)
    .filter(Boolean);
  setupLoadOrderControls();

  renderModalMode(file);
  syncPolicyControls();
  document.getElementById("load-order-current").textContent = "正在获取...";
  document.getElementById("load-order-input").value = "";
  document.getElementById("load-order-rule-sources-search").value = "";
  document.getElementById("load-order-rule-target-search").value = "";
  document.getElementById("load-order-modal").classList.remove("hidden");

  try {
    await refreshLoadOrderModal();
    if (currentMode === "single") {
      document.getElementById("load-order-input").focus();
    } else {
      document.getElementById("load-order-rule-sources-search").focus();
    }
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
  selectedSourceKeys = new Set();
  selectedPaths = [];
  selectedPathNames = [];
}

export async function saveEnhancedLoadOrder() {
  if (currentMode !== "single" || !currentLoadOrderFile) return;
  if (currentOrder <= 0) {
    showError("该 Mod 尚未写入 addonlist.txt；加载顺序只会重排已有条目，不会改变游戏内开关状态");
    return;
  }
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
  document.getElementById("load-order-presets")?.addEventListener("click", (event) => {
    const presetButton = event.target.closest("[data-load-order-preset]");
    if (!presetButton) return;
    applyLoadOrderPreset(presetButton.dataset.loadOrderPreset);
  });
  document.getElementById("preview-load-order-policy-btn")?.addEventListener("click", previewLoadOrderPolicy);
  document.getElementById("apply-load-order-policy-btn")?.addEventListener("click", applyLoadOrderPolicy);
  document.getElementById("load-order-root-first")?.addEventListener("change", (event) => {
    currentPolicy.rootFirst = event.target.checked;
    renderActivePolicy();
  });
  document.getElementById("load-order-group-workshop")?.addEventListener("change", (event) => {
    currentPolicy.groupWorkshop = event.target.checked;
    renderActivePolicy();
  });
  document.getElementById("load-order-state-order")?.addEventListener("change", (event) => {
    currentPolicy.stateOrder = event.target.value;
    renderActivePolicy();
  });
  document
    .getElementById("load-order-rule-sources-search")
    ?.addEventListener("input", () => renderRuleSelectors());
  document
    .getElementById("load-order-rule-target-search")
    ?.addEventListener("input", () => renderRuleSelectors());
  document.getElementById("load-order-rule-sources")?.addEventListener("change", (event) => {
    selectedSourceKeys = new Set(Array.from(event.target.selectedOptions).map((option) => option.value));
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
  const entries = await GetAddonListLoadOrderEntries();
  currentOrder = currentLoadOrderFile ? await GetVPKLoadOrder(currentLoadOrderFile) : -1;
  currentEntries = entries || [];
  prepareEntryMetadata();
  preselectContextEntries();
  renderModalMode(currentLoadOrderFile ? appState.vpkFiles.find((item) => item.path === currentLoadOrderFile) : null);
  renderCurrentOrder();
  renderRuleSelectors();
  renderRules();
  renderActivePolicy();
  renderPreview(currentEntries, "当前 addonlist.txt 顺序");
}

function renderCurrentOrder() {
  const singleSection = document.getElementById("load-order-single-section");
  if (singleSection?.classList.contains("hidden")) return;
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
  if (!sources || !target) return;
  const selectedSources = new Set(selectedSourceKeys);
  const selectedTarget = target.value;
  const sourceQuery = document.getElementById("load-order-rule-sources-search").value;
  const targetQuery = document.getElementById("load-order-rule-target-search").value;
  sources.replaceChildren();
  target.replaceChildren();
  currentEntries.filter((entry) => entryMatchesSearch(entry, sourceQuery)).forEach((entry) => {
    const label = entryLabel(entry);
    sources.add(new Option(label, entry.key, false, selectedSources.has(entry.key)));
  });
  currentEntries.filter((entry) => entryMatchesSearch(entry, targetQuery)).forEach((entry) => {
    const label = entryLabel(entry);
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
    empty.textContent = "尚未添加本次排序的前后关系。";
    container.appendChild(empty);
    renderActivePolicy();
    return;
  }
  currentPolicy.constraints.forEach((constraint, index) => {
    const row = document.createElement("div");
    row.className = "load-order-rule";
    const text = document.createElement("span");
    text.className = "load-order-rule-text";
    text.textContent = `${entryDisplayName(constraint.before)} 排在 ${entryDisplayName(constraint.after)} 前面`;
    const remove = document.createElement("button");
    remove.type = "button";
    remove.className = "load-order-rule-delete";
    remove.dataset.loadOrderRuleIndex = String(index);
    remove.textContent = "移除";
    row.append(text, remove);
    container.appendChild(row);
  });
  renderActivePolicy();
}

async function previewLoadOrderPolicy() {
  try {
    const preview = await PreviewAddonListLoadOrderPolicy(currentPolicy);
    renderPreview(preview.entries || [], "预览：尚未写入");
    showNotification("已生成加载顺序预览", "info");
  } catch (err) {
    console.error("预览加载顺序失败:", err);
    showError("无法应用该排序策略: " + err);
  }
}

async function applyLoadOrderPolicy(options = {}) {
  try {
    const result = await ApplyAddonListLoadOrderPolicy(currentPolicy);
    currentEntries = result.entries || [];
    prepareEntryMetadata();
    renderRuleSelectors();
    renderPreview(currentEntries, "已写入 addonlist.txt，并同步保护快照");
    currentOrder = currentLoadOrderFile ? await GetVPKLoadOrder(currentLoadOrderFile) : -1;
    renderCurrentOrder();
    renderActivePolicy();
    showNotification(
      options.successMessage || "加载顺序已写入 addonlist.txt，所有 Mod 开关状态保持不变",
      "success"
    );
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

  const sourceKeys = new Set(currentPolicy.constraints.map((constraint) => constraint.before));
  const targetKeys = new Set(currentPolicy.constraints.map((constraint) => constraint.after));
  const fragment = document.createDocumentFragment();
  entries.forEach((entry) => {
    const row = document.createElement("div");
    row.className = "load-order-preview-item";
    if (sourceKeys.has(entry.key)) row.classList.add("is-rule-source");
    if (targetKeys.has(entry.key)) row.classList.add("is-rule-target");
    const order = document.createElement("span");
    order.className = "load-order-preview-number";
    order.textContent = String(entry.order);
    const type = document.createElement("span");
    const location = entry.isWorkshop ? "workshop" : entry.isRoot ? "root" : "other";
    type.className = `load-order-preview-type ${location}`;
    type.textContent = entry.isWorkshop ? "工坊" : entry.isRoot ? "根目录" : "子目录";
    const key = document.createElement("span");
    key.className = "load-order-preview-key";
    key.title = entryLabel(entry);
    key.textContent = entryDisplayName(entry.key);
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
  return `${entry.order}. ${entryDisplayName(entry.key)}${entry.value === "1" ? "（开启）" : "（关闭）"}`;
}

function normalizeLoadOrderContext(context) {
  if (typeof context === "string") {
    return { mode: "single", filePath: context, selectedPaths: [] };
  }
  const selectedPaths = Array.from(new Set((context?.selectedPaths || []).filter(Boolean)));
  if (context?.mode === "single" && context.filePath) {
    return { mode: "single", filePath: context.filePath, selectedPaths: [] };
  }
  if (context?.mode === "selection" || selectedPaths.length > 0) {
    return { mode: "selection", filePath: null, selectedPaths };
  }
  return { mode: "global", filePath: null, selectedPaths: [] };
}

function renderModalMode(file) {
  const isSingle = currentMode === "single";
  const isSelection = currentMode === "selection";
  document.getElementById("load-order-title").textContent = isSingle ? "调整单个 Mod 加载顺序" : isSelection ? "批量调整 Mod 加载顺序" : "加载顺序优化";
  const filename = document.getElementById("load-order-filename");
  const contextNote = document.getElementById("load-order-context-note");
  const singleSection = document.getElementById("load-order-single-section");
  const confirmButton = document.getElementById("confirm-load-order-btn");
  singleSection?.classList.toggle("hidden", !isSingle);
  confirmButton?.classList.toggle("hidden", !isSingle);

  if (isSingle) {
    filename.textContent = file?.name || "当前 Mod";
    contextNote.textContent = "单项模式：可上移、下移或指定现有条目的新序号。";
    return;
  }
  if (isSelection) {
    filename.textContent = `已选择 ${selectedPathNames.length} 个 Mod`;
    contextNote.textContent = "批量模式：已选 Mod 会预填到“要移动的 Mod”；再搜索并选择一个锚点即可调整到其前面或后面。";
    return;
  }
  filename.textContent = "全部 addonlist.txt 条目";
  contextNote.textContent = "全局模式：可一键按规则重排全部条目；不会新增条目，也不会修改任何 Mod 的游戏内开关。";
}

function preselectContextEntries() {
  if (currentMode !== "selection" || selectedSourceKeys.size > 0) return;
  const entryKeys = new Set(currentEntries.map((entry) => entry.key));
  const unmatchedNames = [];
  selectedPaths
    .map((path) => appState.vpkFiles.find((file) => file.path === path))
    .filter(Boolean)
    .forEach((file) => {
      const key = addonListKeyForFile(file);
      if (key && entryKeys.has(key)) {
        selectedSourceKeys.add(key);
      } else {
        unmatchedNames.push(file.name);
      }
    });
  if (unmatchedNames.length > 0) {
    const contextNote = document.getElementById("load-order-context-note");
    contextNote.textContent += ` ${unmatchedNames.join("、")} 尚未写入 addonlist.txt，不能仅通过重排改变其状态。`;
  }
}

function applyLoadOrderPreset(presetName) {
  const preset = LOAD_ORDER_PRESETS[presetName];
  if (!preset) return;
  currentPolicy = { ...createEmptyPolicy(), ...preset.policy };
  syncPolicyControls();
  renderRules();
  applyLoadOrderPolicy({ successMessage: `已按“${preset.label}”重排 addonlist.txt，所有开关状态保持不变` });
}

function syncPolicyControls() {
  const rootFirst = document.getElementById("load-order-root-first");
  const groupWorkshop = document.getElementById("load-order-group-workshop");
  const stateOrder = document.getElementById("load-order-state-order");
  if (rootFirst) rootFirst.checked = currentPolicy.rootFirst;
  if (groupWorkshop) groupWorkshop.checked = currentPolicy.groupWorkshop;
  if (stateOrder) stateOrder.value = currentPolicy.stateOrder || "keep";
  renderActivePolicy();
}

function renderActivePolicy() {
  const summary = document.getElementById("load-order-active-policy");
  if (!summary) return;
  const labels = [];
  if (currentPolicy.rootFirst) labels.push("根目录优先");
  if (currentPolicy.groupWorkshop) labels.push("工坊聚集");
  if (currentPolicy.stateOrder === "enabled-first") labels.push("开启项优先");
  if (currentPolicy.stateOrder === "disabled-first") labels.push("关闭项优先");
  if (currentPolicy.constraints.length > 0) labels.push(`${currentPolicy.constraints.length} 条锚点规则`);
  summary.textContent = labels.length > 0 ? `当前策略：${labels.join(" · ")}` : "当前策略：保持原有顺序";
}

function addonListKeyForFile(file) {
  if (!file?.name) return "";
  const name = String(file.name).replaceAll("/", "\\").toLowerCase();
  return file.location === "workshop" ? `workshop\\${name}` : name;
}

function entryMatchesSearch(entry, query) {
  const needle = String(query || "").trim().toLocaleLowerCase();
  if (!needle) return true;
  return entrySearchText(entry).includes(needle);
}

function entrySearchText(entry) {
  const key = String(entry.key || "").toLocaleLowerCase();
  return entrySearchTextByKey.get(key) || key;
}

function entryDisplayName(key) {
  const normalizedKey = String(key || "").toLocaleLowerCase();
  const file = entryFileByKey.get(normalizedKey);
  if (!file) return key;
  const title = String(file.title || "").trim();
  return title && title !== file.name ? `${title} · ${file.name}` : file.name;
}

function prepareEntryMetadata() {
  entryFileByKey = new Map();
  entrySearchTextByKey = new Map();
  appState.vpkFiles.forEach((file) => {
    const key = addonListKeyForFile(file);
    if (key && !entryFileByKey.has(key)) {
      entryFileByKey.set(key, file);
    }
  });
  currentEntries.forEach((entry) => {
    const key = String(entry.key || "").toLocaleLowerCase();
    const file = entryFileByKey.get(key);
    entrySearchTextByKey.set(
      key,
      `${key} ${file?.name || ""} ${file?.title || ""}`.toLocaleLowerCase()
    );
  });
}
