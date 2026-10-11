import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { showConfirmModal } from "../modals/confirm.js";
import { appState } from "../state.js";
import { refreshFilesKeepFilter } from "../file-list/filters.js";
import { renameFile } from "../file-list/operations.js";
import { openSetTagsModal } from "../file-list/tags.js";
import {
  GetDownloadTasks,
  CancelDownloadTask,
  PauseDownloadTask,
  ResumeDownloadTask,
  RetryDownloadTask,
  SetDownloadTaskAutoRedownload,
  ClearCompletedTasks,
} from "../../../../wailsjs/go/app/App";

let taskListRequestId = 0;

export async function refreshTaskList() {
  const requestId = ++taskListRequestId;
  const listContainer = document.getElementById("download-tasks-list");
  if (!listContainer) return;

  try {
    const tasks = await GetDownloadTasks();
    if (requestId !== taskListRequestId || !listContainer.isConnected) return;

    if (!tasks || tasks.length === 0) {
      listContainer.innerHTML = `<div class="empty-tasks" style="text-align: center; color: #888; padding: 20px;">${t("downloads.task.empty")}</div>`;
      return;
    }

    tasks.sort((a, b) => {
      const statusOrder = {
        selecting_ip: 0,
        downloading: 1,
        pending: 2,
        paused: 3,
        interrupted: 3,
        failed: 3,
        completed: 4,
      };
      if (statusOrder[a.status] !== statusOrder[b.status]) {
        return (statusOrder[a.status] || 99) - (statusOrder[b.status] || 99);
      }
      return b.id.localeCompare(a.id);
    });

    listContainer.innerHTML = "";
    tasks.forEach((task) => {
      const item = createTaskElement(task);
      listContainer.appendChild(item);
    });
  } catch (err) {
    if (requestId !== taskListRequestId || !listContainer.isConnected) return;
    console.error("Failed to refresh tasks:", err);
    const errorState = document.createElement("div");
    errorState.className = "empty-tasks";
    errorState.style.cssText = "text-align: center; color: #b42318; padding: 20px;";
    errorState.textContent = t("downloads.task.loadFailed");
    const retryButton = document.createElement("button");
    retryButton.type = "button";
    retryButton.className = "btn btn-secondary btn-small";
    retryButton.textContent = t("downloads.task.retry");
    retryButton.addEventListener("click", refreshTaskList);
    errorState.append(" ", retryButton);
    listContainer.replaceChildren(errorState);
  }
}

export function createTaskElement(task) {
  const div = document.createElement("div");
  div.className = "task-item";
  div.id = `task-${task.id}`;
  div.style.cssText = "padding: 10px; border-bottom: 1px solid #eee; display: flex; gap: 10px; align-items: center;";

  const statusColors = {
    pending: "#ff9800",
    selecting_ip: "#9c27b0",
    downloading: "#2196f3",
    completed: "#4caf50",
    failed: "#f44336",
    interrupted: "#ff9800",
    paused: "#2196f3",
  };

  const statusText = {
    pending: t("downloads.task.status.pending"),
    selecting_ip: t("downloads.task.status.selectingIp"),
    downloading: t("downloads.task.status.downloading"),
    completed: t("downloads.task.status.completed"),
    failed: t("downloads.task.status.failed"),
    cancelled: t("downloads.task.status.cancelled"),
    interrupted: t("downloads.task.status.interrupted"),
    paused: t("downloads.task.status.paused"),
  };

  const taskId = escapeHtml(task.id);
  const taskTitle = escapeHtml(task.title || "");
  const taskFilename = escapeHtml(task.filename || "");
  const taskError = escapeHtml(task.error || "");
  const taskStatus = escapeHtml(statusText[task.status] || task.status || "");
  const taskFileURL = escapeHtml(task.file_url || "");
  const progress = clampProgress(task.progress);
  const copyBtn = `
    <button class="task-action-btn copy-link-btn" data-url="${taskFileURL}" title="${t("downloads.task.action.copyLink")}">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect>
        <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path>
      </svg>
    </button>`;

  let actionButtons = "";
  if (task.status === "downloading" || task.status === "pending" || task.status === "selecting_ip") {
    actionButtons = `
      ${copyBtn}
      <button class="task-action-btn pause-task-btn" data-id="${taskId}" title="${t("downloads.task.action.pause")}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="9" y1="5" x2="9" y2="19"></line>
          <line x1="15" y1="5" x2="15" y2="19"></line>
        </svg>
      </button>
      <button class="task-action-btn cancel-btn cancel-task-btn" data-id="${taskId}" title="${t("downloads.task.action.cancel")}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>`;
  } else if (task.status === "paused") {
    // 暂停是"保留断点"的状态：继续 = 断点续传，取消 = 丢弃这次下载。
    actionButtons = `
      ${copyBtn}
      <button class="task-action-btn resume-task-btn" data-id="${taskId}" title="${t("downloads.task.action.resume")}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polygon points="6 4 20 12 6 20 6 4"></polygon>
        </svg>
      </button>
      <button class="task-action-btn cancel-btn cancel-task-btn" data-id="${taskId}" title="${t("downloads.task.action.cancelDiscard")}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <line x1="18" y1="6" x2="6" y2="18"></line>
          <line x1="6" y1="6" x2="18" y2="18"></line>
        </svg>
      </button>`;
  } else if (task.status === "failed" || task.status === "cancelled" || task.status === "interrupted") {
    actionButtons = `
      ${copyBtn}
      <button class="task-action-btn retry-btn retry-task-btn" data-id="${taskId}" title="${t("downloads.task.action.retry")}">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M23 4v6h-6"></path>
          <path d="M1 20v-6h6"></path>
          <path d="M3.51 9a9 9 0 0 1 14.85-3.36L23 10M1 14l4.64 4.36A9 9 0 0 0 20.49 15"></path>
        </svg>
      </button>`;
  } else if (task.status === "completed") {
    actionButtons = copyBtn;
  }

  let previewHtml = "";
  const previewURL = safeImageURL(task.preview_url);
  if (previewURL) {
    previewHtml = `<img src="${escapeHtml(previewURL)}" loading="lazy" decoding="async" style="width: 50px; height: 50px; object-fit: cover; border-radius: 4px;">`;
  } else {
    previewHtml = `
      <div style="width: 50px; height: 50px; background-color: #f0f0f0; border-radius: 4px; display: flex; align-items: center; justify-content: center; color: #888;">
        <svg viewBox="0 0 24 24" width="24" height="24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
          <polyline points="7 10 12 15 17 10"></polyline>
          <line x1="12" y1="15" x2="12" y2="3"></line>
        </svg>
      </div>`;
  }

  div.innerHTML = `
    ${previewHtml}
    <div class="task-content" style="flex: 1; min-width: 0;">
      <div style="display: flex; justify-content: space-between; margin-bottom: 5px;">
        <span class="task-title" style="font-weight: bold; font-size: 0.875rem; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; max-width: 250px;">${taskTitle}</span>
        <div style="display: flex; align-items: center; gap: 5px;">
          <span class="task-status" style="font-size: 0.75rem; color: ${statusColors[task.status] || "var(--text-secondary)"};">${taskStatus}</span>
          ${actionButtons}
        </div>
      </div>
      <div style="font-size: 0.75rem; color: var(--text-secondary); margin-bottom: 5px;">${taskFilename}</div>
      <div class="progress-bar" style="width: 100%; height: 6px; background-color: #eee; border-radius: 3px; overflow: hidden;">
        <div class="progress-fill" style="width: ${progress}%; height: 100%; background-color: ${statusColors[task.status] || "#ccc"}; transition: width 0.3s;"></div>
      </div>
      <div style="display: flex; justify-content: space-between; font-size: 0.6875rem; color: var(--text-tertiary); margin-top: 2px;">
        <span class="task-size">${formatBytes(task.downloaded_size)} / ${formatBytes(task.total_size)} ${task.speed ? `(${task.speed})` : ""}</span>
        <span class="task-percent">${progress}%</span>
      </div>
      ${formatTaskAutoRedownloadToggle(task, taskId)}
      ${taskError ? `<div style="color: var(--danger, #f44336); font-size: 0.6875rem; margin-top: 2px;">${taskError}</div>` : ""}
    </div>
  `;

  const completedActions = createCompletedTaskActions(task);
  if (completedActions) {
    div.querySelector(".task-content")?.appendChild(completedActions);
  }

  const copyLinkBtn = div.querySelector(".copy-link-btn");
  if (copyLinkBtn) {
    copyLinkBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      const url = copyLinkBtn.dataset.url;
      if (url) {
        navigator.clipboard.writeText(url)
          .then(() => showNotification(t("downloads.task.notification.linkCopied"), "success"))
          .catch((err) => {
            console.error("复制失败:", err);
            const el = document.createElement("textarea");
            el.value = url;
            document.body.appendChild(el);
            el.select();
            document.execCommand("copy");
            document.body.removeChild(el);
            showNotification(t("downloads.task.notification.linkCopied"), "success");
          });
      } else {
        showError(t("downloads.common.invalidLink"));
      }
    });
  }

  const cancelBtn = div.querySelector(".cancel-task-btn");
  if (cancelBtn) {
    cancelBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      showConfirmModal(t("downloads.task.cancelTitle"), t("downloads.task.cancelConfirm"), async () => {
        try {
          await CancelDownloadTask(task.id);
          showNotification(t("downloads.task.notification.cancelled"), "info");
        } catch (err) {
          console.error("取消任务失败:", err);
          showError(t("downloads.task.error.cancelFailed", { error: err }));
        }
      });
    });
  }

  const retryBtn = div.querySelector(".retry-task-btn");
  if (retryBtn) {
    retryBtn.addEventListener("click", async (e) => {
      e.stopPropagation();
      try {
        await RetryDownloadTask(task.id);
        showNotification(t("downloads.task.notification.retried"), "success");
      } catch (err) {
        console.error("重试任务失败:", err);
        showError(t("downloads.task.error.retryFailed", { error: err }));
      }
    });
  }

  const pauseBtn = div.querySelector(".pause-task-btn");
  if (pauseBtn) {
    pauseBtn.addEventListener("click", async (e) => {
      e.stopPropagation();
      try {
        await PauseDownloadTask(task.id);
        showNotification(t("downloads.task.notification.paused"), "info");
      } catch (err) {
        console.error("暂停任务失败:", err);
        showError(t("downloads.task.error.pauseFailed", { error: err }));
      }
    });
  }

  const resumeBtn = div.querySelector(".resume-task-btn");
  if (resumeBtn) {
    resumeBtn.addEventListener("click", async (e) => {
      e.stopPropagation();
      try {
        await ResumeDownloadTask(task.id);
        showNotification(t("downloads.task.notification.resumed"), "success");
      } catch (err) {
        console.error("继续任务失败:", err);
        showError(t("downloads.task.error.resumeFailed", { error: err }));
      }
    });
  }

  // 任务级「失败后自动重下」：后端已有 SetDownloadTaskAutoRedownload，
  // 这里补上界面入口，让"在下载队列里单独开关某个任务"真的可用。
  const autoRedownloadInput = div.querySelector(".task-auto-redownload-input");
  if (autoRedownloadInput) {
    autoRedownloadInput.addEventListener("change", async (e) => {
      e.stopPropagation();
      const enabled = autoRedownloadInput.checked;
      autoRedownloadInput.disabled = true;
      try {
        await SetDownloadTaskAutoRedownload(task.id, enabled);
        showNotification(enabled ? t("downloads.task.notification.autoRedownloadOn") : t("downloads.task.notification.autoRedownloadOff"), "info");
      } catch (err) {
        // 写回失败时把勾选状态还原，避免界面显示与后端不一致。
        autoRedownloadInput.checked = !enabled;
        console.error("切换任务自动重下失败:", err);
        showError(t("downloads.task.error.autoRedownloadFailed", { error: err }));
      } finally {
        autoRedownloadInput.disabled = false;
      }
    });
  }

  return div;
}

// formatTaskAutoRedownloadToggle 渲染任务级的「失败后自动重下」开关。
// 已完成的任务不再需要这个策略，直接不渲染；其余状态（含失败 / 已中断）
// 都允许单独开关，与文档里"中断任务同样可以单独开关"一致。
export function formatTaskAutoRedownloadToggle(task, escapedTaskId = "") {
  if (!task || task.status === "completed") return "";
  const id = escapedTaskId || escapeHtml(task.id);
  const checked = task.auto_redownload === true ? " checked" : "";
  return `
      <label class="task-auto-redownload" style="display: inline-flex; align-items: center; gap: 4px; font-size: 0.6875rem; color: var(--text-tertiary); margin-top: 4px; cursor: pointer;" title="${t("downloads.task.autoRedownloadHint")}">
        <input type="checkbox" class="task-auto-redownload-input" data-id="${id}"${checked}>
        <span>${t("downloads.task.autoRedownloadLabel")}</span>
      </label>`;
}

function createCompletedTaskActions(task) {
  if (task.status !== "completed" || !getCompletedTaskFilePathCandidate(task)) {
    return null;
  }

  const actions = document.createElement("div");
  actions.className = "task-completed-actions";

  const renameBtn = createCompletedTaskButton(t("downloads.task.action.rename"), "task-rename-shortcut-btn");
  renameBtn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    await openCompletedTaskFile(task, renameFile);
  });

  const tagsBtn = createCompletedTaskButton(t("downloads.task.action.setTags"), "task-tags-shortcut-btn");
  tagsBtn.addEventListener("click", async (e) => {
    e.preventDefault();
    e.stopPropagation();
    await openCompletedTaskFile(task, openSetTagsModal);
  });

  actions.appendChild(renameBtn);
  actions.appendChild(tagsBtn);
  return actions;
}

function createCompletedTaskButton(label, extraClass) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = `task-completed-action-btn ${extraClass}`;
  button.textContent = label;
  return button;
}

async function openCompletedTaskFile(task, action) {
  const filePath = await resolveCompletedTaskFilePath(task);
  if (!filePath) {
    showError(t("downloads.task.error.fileNotFound"));
    return;
  }

  action(filePath);
}

async function resolveCompletedTaskFilePath(task) {
  let file = findDownloadedTaskFile(task);
  if (file?.path) {
    return file.path;
  }

  if (!getTaskVPKPath(task)) {
    return "";
  }

  await refreshFilesKeepFilter();
  file = findDownloadedTaskFile(task);
  return file?.path || "";
}

function getCompletedTaskFilePathCandidate(task) {
  const file = findDownloadedTaskFile(task);
  if (file?.path) {
    return file.path;
  }

  return getTaskVPKPath(task);
}

function getTaskVPKPath(task) {
  const filePath = String(task.file_path || "").trim();
  if (!filePath || !filePath.toLowerCase().endsWith(".vpk")) {
    return "";
  }
  return filePath;
}

function findDownloadedTaskFile(task) {
  const files = getKnownVPKFiles();
  const taskPath = normalizePathForCompare(getTaskVPKPath(task));
  if (taskPath) {
    const file = files.find((item) => normalizePathForCompare(item.path) === taskPath);
    if (file) return file;
  }

  const workshopId = String(task.workshop_id || "").trim();
  if (workshopId && !workshopId.startsWith("direct-")) {
    const matches = files.filter((item) => String(item.workshopId || "") === workshopId);
    if (matches.length === 1) {
      return matches[0];
    }
  }

  const filename = String(task.filename || "").trim().toLowerCase();
  if (filename.endsWith(".vpk")) {
    const matches = files.filter((item) => String(item.name || "").toLowerCase() === filename);
    if (matches.length === 1) {
      return matches[0];
    }
  }

  return null;
}

function getKnownVPKFiles() {
  const filesByPath = new Map();
  [...(appState.allVpkFiles || []), ...(appState.vpkFiles || [])].forEach((file) => {
    if (file?.path && !filesByPath.has(file.path)) {
      filesByPath.set(file.path, file);
    }
  });
  return Array.from(filesByPath.values());
}

function normalizePathForCompare(filePath) {
  return String(filePath || "").replace(/\//g, "\\").toLowerCase();
}

export function updateTaskInList(task) {
  taskListRequestId += 1;
  const existing = document.getElementById(`task-${task.id}`);
  if (existing) {
    const newItem = createTaskElement(task);
    existing.replaceWith(newItem);
  } else {
    refreshTaskList();
  }

  if (task.status === "completed") {
    refreshFilesKeepFilter();
  }
}

export function updateTaskProgress(task) {
  const el = document.getElementById(`task-${task.id}`);
  if (!el) {
    refreshTaskList();
    return;
  }

  taskListRequestId += 1;
  const progress = clampProgress(task.progress);
  const fill = el.querySelector(".progress-fill");
  const percentText = el.querySelector(".task-percent");
  const sizeText = el.querySelector(".task-size");

  if (fill) fill.style.width = `${progress}%`;
  if (percentText) percentText.textContent = `${progress}%`;
  if (sizeText) {
    sizeText.textContent = `${formatBytes(task.downloaded_size)} / ${formatBytes(task.total_size)} ${task.speed ? `(${task.speed})` : ""}`;
  }
}

export function setupClearCompletedTasks() {
  const btn = document.getElementById("clear-completed-tasks-btn");
  if (btn) {
    btn.addEventListener("click", async () => {
      await ClearCompletedTasks();
    });
  }
}

function formatBytes(bytes, decimals = 2) {
  if (!Number.isFinite(Number(bytes)) || Number(bytes) <= 0) return "0 Bytes";
  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ["Bytes", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + " " + sizes[i];
}

function clampProgress(value) {
  const progress = Number(value);
  if (!Number.isFinite(progress)) return 0;
  return Math.min(100, Math.max(0, Math.round(progress)));
}

function escapeHtml(value) {
  return String(value ?? "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function safeImageURL(value) {
  const raw = String(value || "").trim();
  if (!raw) return "";
  try {
    const parsed = new URL(raw, window.location.href);
    if (parsed.protocol === "http:" || parsed.protocol === "https:") return parsed.href;
    if (parsed.protocol === "data:" && parsed.pathname.startsWith("image/")) return raw;
  } catch {
    return "";
  }
  return "";
}
