// 「工坊解析历史」的下拉菜单接线（对齐上游 b635ea3）。
// 纯逻辑在 workshop-history.mjs（有单测），这里只负责 DOM 与后端调用。

import { showError, showNotification } from "../../core/toast.js";
import { escapeHtml } from "../../core/utils.js";
import { showConfirmModal } from "../modals/confirm.js";
import {
  AddWorkshopHistoryEntries,
  ClearWorkshopHistory,
  GetWorkshopHistory,
} from "../../../../wailsjs/go/app/App";
import {
  buildHistoryEntries,
  formatHistoryLabel,
  formatHistoryTime,
  normalizeHistoryItems,
} from "./workshop-history.mjs";

// 解析历史最多 10 条，由后端截断；前端只做展示与选择。
let historyItems = [];
let selectHandler = null;
let selectionBound = false;

const getTrigger = () => document.getElementById("workshop-history-btn");
const getMenu = () => document.getElementById("workshop-history-menu");
const isMenuOpen = () => !getMenu()?.classList.contains("hidden");

function setMenuOpen(open) {
  const trigger = getTrigger();
  const menu = getMenu();
  if (!trigger || !menu) return;
  menu.classList.toggle("hidden", !open);
  trigger.classList.toggle("active", open);
  trigger.setAttribute("aria-expanded", String(open));
}

/** closeWorkshopHistory 关闭历史下拉（切页 / 重新解析时调用）。 */
export function closeWorkshopHistory() {
  setMenuOpen(false);
}

function renderHistoryList() {
  const listEl = document.getElementById("workshop-history-list");
  const emptyEl = document.getElementById("workshop-history-empty");
  if (!listEl) return;

  listEl.replaceChildren();
  if (historyItems.length === 0) {
    emptyEl?.classList.remove("hidden");
    return;
  }
  emptyEl?.classList.add("hidden");

  historyItems.forEach((item) => {
    const label = formatHistoryLabel(item);
    const time = formatHistoryTime(item.parsedAt);
    const row = document.createElement("button");
    row.type = "button";
    row.className = "workshop-history-item";
    row.title = `${item.rootId} · ${label}${time ? ` · ${time}` : ""}`;
    row.innerHTML = `
      <span class="workshop-history-title">${escapeHtml(label)}</span>
      <span class="workshop-history-meta">#${escapeHtml(item.rootId)}${time ? ` · ${escapeHtml(time)}` : ""}</span>
    `;
    row.addEventListener("click", () => selectHistoryItem(item));
    listEl.appendChild(row);
  });
}

function selectHistoryItem(item) {
  closeWorkshopHistory();

  const input = document.getElementById("workshop-url");
  if (input) input.value = item.rootId;

  if (item.group && typeof selectHandler === "function") {
    selectHandler(item);
    return;
  }

  // 快照缺失（老数据）时退化成真实解析，复用「解析」按钮的既有绑定
  document.getElementById("check-workshop-btn")?.click();
}

async function loadHistory() {
  try {
    const storage = await GetWorkshopHistory();
    historyItems = normalizeHistoryItems(storage?.items);
    renderHistoryList();
  } catch (error) {
    console.error("读取解析历史失败:", error);
  }
}

/**
 * recordWorkshopHistory 解析成功后把结果快照写进历史。
 * 写历史失败不影响解析流程（只打日志）。
 */
export function recordWorkshopHistory(groups) {
  const entries = buildHistoryEntries(groups);
  if (entries.length === 0) return;

  AddWorkshopHistoryEntries(entries)
    .then((storage) => {
      historyItems = normalizeHistoryItems(storage?.items);
      renderHistoryList();
    })
    .catch((error) => {
      console.warn("写入解析历史失败:", error);
    });
}

function clearHistory() {
  showConfirmModal("清空解析历史", "确定要清空所有解析历史记录吗？", async () => {
    try {
      await ClearWorkshopHistory();
      historyItems = [];
      renderHistoryList();
      showNotification("解析历史已清空", "success");
    } catch (error) {
      console.error("清空解析历史失败:", error);
      showError("清空解析历史失败: " + error);
    }
  });
}

/**
 * setupWorkshopHistory 绑定历史下拉。
 * onSelect(item) 由下载页提供（用快照直接重画结果，不再请求接口）。
 */
export function setupWorkshopHistory({ onSelect } = {}) {
  selectHandler = typeof onSelect === "function" ? onSelect : null;

  const trigger = getTrigger();
  if (!trigger) return;

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    const willOpen = !isMenuOpen();
    setMenuOpen(willOpen);
    if (willOpen) void loadHistory();
  });

  if (!selectionBound) {
    selectionBound = true;
    document.addEventListener("click", (event) => {
      if (
        !event.target.closest?.("#workshop-history-menu") &&
        !event.target.closest?.("#workshop-history-btn")
      ) {
        closeWorkshopHistory();
      }
    });
    document.addEventListener("keydown", (event) => {
      if (event.key === "Escape") closeWorkshopHistory();
    });
    document
      .getElementById("workshop-history-clear")
      ?.addEventListener("click", () => {
        closeWorkshopHistory();
        clearHistory();
      });
  }

  renderHistoryList();
}
