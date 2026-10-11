// Toast 通知系统
//
// 后端错误在这里统一过一次解释层（core/action-explanation.mjs，对齐 FireAxe
// ObjectExplanationManager + ExceptionExplanations）：能认出类型就给"人话 + 下一步"，
// 认不出就原样显示 —— 无论如何都有一句话，不会出现空白提示。

import { explainOperationError, sceneFromErrorType } from "./action-explanation.mjs";
import { t } from "./i18n-runtime.js";

let errorQueue = [];
let errorTimer = null;

export function handleError(errorInfo) {
  // 直接打印对象只会得到 "Object"，无法定位是哪个后端调用失败。
  // 这里优先输出 type/message/file，其次退回字符串化结果。
  let detail;
  if (typeof errorInfo === "string") {
    detail = errorInfo;
  } else if (errorInfo && typeof errorInfo === "object") {
    detail = [errorInfo.type, errorInfo.message, errorInfo.file].filter(Boolean).join(" | ");
    if (!detail) {
      try {
        detail = JSON.stringify(errorInfo);
      } catch {
        detail = String(errorInfo);
      }
    }
  } else {
    detail = String(errorInfo);
  }
  console.error("应用错误:", detail);
  errorQueue.push(errorInfo);

  if (errorTimer) {
    clearTimeout(errorTimer);
  }

  // 300ms 防抖，聚合短时间内的错误
  errorTimer = setTimeout(processErrorQueue, 300);
}

export function processErrorQueue() {
  if (errorQueue.length === 0) return;

  const errors = [...errorQueue];
  errorQueue = []; // 清空队列

  if (errors.length === 1) {
    const errorInfo = errors[0];
    let title = errorInfo.type === "VPK解析" ? t("app.toast.parseError") : errorInfo.type;
    // 场景从后端给的错误类型推断（输入类 → "输入不合法"，其余 → "操作失败"）。
    const explained = explainOperationError(errorInfo.message, { scene: sceneFromErrorType(errorInfo.type) });
    let msg = `<strong>${title}</strong><br>`;

    if (errorInfo.file) {
      const fileName = errorInfo.file.split(/[\\/]/).pop();
      msg += `${t("app.toast.fileName", { name: fileName })}<br>${t("app.toast.content", { text: explained.summary })}`;
    } else {
      msg += t("app.toast.content", { text: explained.summary });
    }
    if (explained.hint) msg += `<br>${t("app.toast.suggestion", { text: explained.hint })}`;
    showError(msg, 5000);
  } else {
    // 多个错误聚合显示
    const type = errors[0].type === "VPK解析" ? t("app.toast.parseError") : errors[0].type;
    let msg = `<strong>${t("app.toast.batchTitle", { type, count: errors.length })}</strong><br>`;

    // 显示前3个详情
    const maxShow = 3;
    for (let i = 0; i < Math.min(errors.length, maxShow); i++) {
      const err = errors[i];
      const fileName = err.file ? err.file.split(/[\\/]/).pop() : t("app.toast.unknownFile");
      msg += `<div style="margin-top:4px; border-bottom:1px solid rgba(255,255,255,0.1); padding-bottom:2px;">
            ${t("app.toast.fileName", { name: fileName })}<br>
            <span style="opacity:0.8; font-size:0.9em;">${t("app.toast.content", { text: err.message })}</span>
        </div>`;
    }

    if (errors.length > maxShow) {
      msg += `<div style="margin-top:4px; font-style:italic;">${t("app.toast.moreFiles", { count: 
        errors.length - maxShow
      })}</div>`;
    }

    showError(msg, 8000); // 多个错误显示时间长一点
  }
}

export function showError(message, duration = 3000) {
  createToast(message, "error", duration);
}

export function showNotification(message, type = "info") {
  console.log(`显示通知: ${message} (类型: ${type})`);

  switch (type) {
    case "success":
      showSuccess(message);
      break;
    case "error":
      showError(message);
      break;
    case "info":
    default:
      showInfo(message);
      break;
    case "warning":
      showWarning(message);
      break;
  }
}

export function showSuccess(message) {
  createToast(message, "success", 3000);
}

export function showInfo(message) {
  createToast(message, "info", 3000);
}

export function showWarning(message, duration = 6000) {
  createToast(message, "warning", duration);
}

function createToast(message, type = "info", duration = 3000) {
  const container = getToastContainer();
  const toast = document.createElement("div");
  const normalizedType = ["success", "error", "info", "warning"].includes(type) ? type : "info";
  const titleMap = {
    success: "Success",
    error: "Error",
    info: "Notification",
    warning: "Warning",
  };

  toast.className = `${normalizedType}-notification app-toast`;
  toast.innerHTML = `
    <div class="toast-icon" aria-hidden="true">${getToastIcon(normalizedType)}</div>
    <div class="toast-body">
      <div class="toast-title">${titleMap[normalizedType]}</div>
      <div class="toast-message">${message}</div>
    </div>
    <button class="toast-close" type="button" aria-label="Close">&times;</button>
  `;

  const removeToast = () => {
    if (!toast.parentNode) return;
    toast.classList.add("is-leaving");
    setTimeout(() => toast.remove(), 180);
  };

  toast.querySelector(".toast-close")?.addEventListener("click", removeToast);
  container.appendChild(toast);
  setTimeout(removeToast, duration);
}

function getToastContainer() {
  let container = document.getElementById("toast-container");
  if (!container) {
    container = document.createElement("div");
    container.id = "toast-container";
    container.setAttribute("aria-live", "polite");
    container.setAttribute("aria-atomic", "false");
    document.body.appendChild(container);
  }
  return container;
}

function getToastIcon(type) {
  if (type === "success") {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`;
  }
  if (type === "error") {
    return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`;
  }
  return `<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"><path d="M12 17v-6"/><path d="M12 7h.01"/></svg>`;
}
