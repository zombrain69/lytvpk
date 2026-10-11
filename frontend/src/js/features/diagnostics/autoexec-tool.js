import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { attachLineNumberGutter } from "../../core/line-number-editor.js";
import { makeModalResizable } from "../../core/modal-resizer.js";

const MODAL_ID = "autoexec-tool-modal";
let openRequestId = 0;

export async function openAutoexecTool() {
  const requestId = ++openRequestId;
  const modal = ensureModal();
  modal.classList.remove("hidden");
  const body = modal.querySelector("[data-autoexec-tool-body]");
  if (!body) return;
  body.innerHTML = `<div class="autoexec-tool-loading">${t("toolbox.autoexec.loading")}</div>`;

  try {
    const [config, help] = await Promise.all([
      callApp("GetAutoexecConfig"),
      callApp("GetAutoexecCommandHelp", ""),
    ]);
    if (!isCurrentOpenSession(modal, requestId)) return;
    renderEditor(modal, config || {}, Array.isArray(help) ? help : [], requestId);
  } catch (error) {
    if (!isCurrentOpenSession(modal, requestId)) return;
    body.innerHTML = `<div class="autoexec-tool-error">${escapeHtml(formatError(error))}</div>`;
    showError(t("toolbox.autoexec.readFailed", { error: formatError(error) }));
  }
}

function ensureModal() {
  const existing = document.getElementById(MODAL_ID);
  if (existing) return existing;

  const modal = document.createElement("div");
  modal.id = MODAL_ID;
  modal.className = "modal hidden";
  modal.style.zIndex = "20002";
  modal.innerHTML = `
    <div class="modal-content autoexec-tool-modal-content">
      <div class="modal-header">
        <h3>${t("toolbox.card.autoexec.title")}</h3>
        <button type="button" class="close-btn" data-autoexec-tool-close aria-label="${t("toolbox.common.close")}">&times;</button>
      </div>
      <div class="modal-body autoexec-tool-body" data-autoexec-tool-body></div>
      <div class="modal-footer">
        <span class="autoexec-resize-hint" aria-hidden="true">${t("toolbox.autoexec.resizeHint")}</span>
        <button type="button" class="btn btn-secondary" data-autoexec-tool-close>${t("toolbox.common.close")}</button>
        <button type="button" class="btn btn-primary" data-autoexec-tool-save disabled>${t("toolbox.autoexec.save")}</button>
      </div>
    </div>
  `;
  document.body.appendChild(modal);
  makeModalResizable(modal.querySelector(".modal-content"));
  modal.querySelectorAll("[data-autoexec-tool-close]").forEach((button) => {
    button.addEventListener("click", () => closeModal(modal));
  });
  modal.addEventListener("click", (event) => {
    if (event.target === modal) closeModal(modal);
  });
  return modal;
}

function renderEditor(modal, config, help, sessionId) {
  const body = modal.querySelector("[data-autoexec-tool-body]");
  const saveButton = modal.querySelector("[data-autoexec-tool-save]");
  if (!body || !saveButton) return;

  const content = String(config.content || "");
  body.innerHTML = `
    <div class="autoexec-tool-intro">${t("toolbox.autoexec.intro")}</div>
    <div class="autoexec-meta autoexec-tool-meta">
      <span class="autoexec-path">${escapeHtml(config.path || "")}</span>
      <span>${config.exists ? t("toolbox.autoexec.fileExists") : t("toolbox.autoexec.fileMissing")}</span>
      <span>${escapeHtml(config.encoding || "UTF-8")} · ${escapeHtml(config.lineEnding || "CRLF")}</span>
      <span>${formatBytes(config.size)}</span>
    </div>
    <div class="autoexec-tool-layout">
      <div class="autoexec-editor-column">
        <div class="autoexec-editor-shell">
          <pre class="autoexec-line-numbers" data-autoexec-tool-line-numbers aria-hidden="true"></pre>
          <textarea class="autoexec-editor" data-autoexec-tool-editor spellcheck="false" wrap="off" aria-label="${t("toolbox.autoexec.editorAria")}">${escapeHtml(content)}</textarea>
        </div>
        <div class="autoexec-analysis" data-autoexec-tool-analysis>${t("toolbox.autoexec.analyzing")}</div>
        <div class="autoexec-tool-matches" data-autoexec-tool-matches></div>
        <div class="autoexec-action-row">
          <button type="button" class="trigger-check-btn addonlist-action-btn" data-autoexec-tool-reload>${t("toolbox.autoexec.reload")}</button>
          <button type="button" class="trigger-check-btn addonlist-action-btn" data-autoexec-tool-open ${config.exists ? "" : "disabled"}>${t("toolbox.autoexec.openLocation")}</button>
        </div>
      </div>
      <aside class="autoexec-help-column">
        <div class="autoexec-help-title">${t("toolbox.autoexec.helpTitle")}</div>
        <input type="search" class="form-input" data-autoexec-tool-search placeholder="${t("toolbox.autoexec.searchPlaceholder")}">
        <div class="autoexec-help-list" data-autoexec-tool-help>${renderHelpItems(help)}</div>
      </aside>
    </div>
  `;

  const editor = body.querySelector("[data-autoexec-tool-editor]");
  const lineNumbers = body.querySelector("[data-autoexec-tool-line-numbers]");
  const analysis = body.querySelector("[data-autoexec-tool-analysis]");
  const matches = body.querySelector("[data-autoexec-tool-matches]");
  const lineNumberEditor = attachLineNumberGutter(editor, lineNumbers);
  let analysisRequest = 0;
  let helpSearchRequest = 0;
  let helpSearchTimer = null;
  const analyze = async () => {
    if (!editor || !isCurrentOpenSession(modal, sessionId) || typeof window?.go?.app?.App?.AnalyzeAutoexecCommands !== "function") return;
    const requestId = ++analysisRequest;
    try {
      const result = await callApp("AnalyzeAutoexecCommands", editor.value);
      if (
        requestId !== analysisRequest ||
        !isCurrentOpenSession(modal, sessionId) ||
        !editor.isConnected
      ) return;
      const items = Array.isArray(result) ? result : [];
      const known = items.filter((item) => item.known).length;
      const unknown = items.length - known;
      const highRisk = items.filter((item) => item.known && item.help?.risk?.startsWith("高")).length;
      const mediumRisk = items.filter((item) => item.known && item.help?.risk?.startsWith("中")).length;
      if (analysis) {
        analysis.textContent = t("toolbox.autoexec.analysisSummary", {
          known,
          unknown,
          high: highRisk ? t("toolbox.autoexec.analysisHighRisk", { count: highRisk }) : "",
          medium: mediumRisk ? t("toolbox.autoexec.analysisMediumRisk", { count: mediumRisk }) : "",
        });
        analysis.classList.toggle("is-warning", unknown > 0 || highRisk > 0);
      }
      renderMatches(matches, items);
    } catch (error) {
      if (
        requestId !== analysisRequest ||
        !isCurrentOpenSession(modal, sessionId) ||
        !editor.isConnected
      ) return;
      if (analysis) analysis.textContent = t("toolbox.autoexec.analysisFailed", { error: formatError(error) });
    }
  };
  editor?.addEventListener("input", analyze);
  analyze();

  body.querySelector("[data-autoexec-tool-search]")?.addEventListener("input", (event) => {
    if (helpSearchTimer) window.clearTimeout(helpSearchTimer);
    const query = event.target.value || "";
    helpSearchTimer = window.setTimeout(async () => {
      helpSearchTimer = null;
      if (!isCurrentOpenSession(modal, sessionId)) return;
      const requestId = ++helpSearchRequest;
      try {
        const filtered = await callApp("GetAutoexecCommandHelp", query);
        if (
          requestId !== helpSearchRequest ||
          !isCurrentOpenSession(modal, sessionId) ||
          !body.isConnected
        ) return;
        const list = body.querySelector("[data-autoexec-tool-help]");
        if (list) list.innerHTML = renderHelpItems(filtered);
      } catch (error) {
        if (
          requestId !== helpSearchRequest ||
          !isCurrentOpenSession(modal, sessionId) ||
          !body.isConnected
        ) return;
        showNotification(t("toolbox.autoexec.searchHelpFailed", { error: formatError(error) }), "error");
      }
    }, 160);
  });
  body.querySelector("[data-autoexec-tool-help]")?.addEventListener("click", (event) => {
    const button = event.target.closest("[data-autoexec-command]");
    if (!button || !editor) return;
    const command = button.dataset.autoexecCommand;
    const start = editor.selectionStart ?? editor.value.length;
    const end = editor.selectionEnd ?? start;
    editor.setRangeText(`${command} `, start, end, "end");
    editor.focus();
    lineNumberEditor.refresh();
    analyze();
  });
  body.querySelector("[data-autoexec-tool-reload]")?.addEventListener("click", () => {
    closeModal(modal);
    openAutoexecTool();
  });
  body.querySelector("[data-autoexec-tool-open]")?.addEventListener("click", async () => {
    if (!config.path) return;
    try {
      await callApp("OpenFileLocation", config.path);
    } catch (error) {
      showError(t("toolbox.autoexec.openFailed", { error: formatError(error) }));
    }
  });
  saveButton.disabled = !config.path;
  saveButton.onclick = async () => {
    if (!editor || !isCurrentOpenSession(modal, sessionId)) return;
    saveButton.disabled = true;
    try {
      await callApp("SaveAutoexecConfig", editor.value);
      if (!isCurrentOpenSession(modal, sessionId)) return;
      showNotification(t("toolbox.autoexec.saved"), "success");
      closeModal(modal);
    } catch (error) {
      if (!isCurrentOpenSession(modal, sessionId)) return;
      showError(t("toolbox.autoexec.saveFailed", { error: formatError(error) }));
      saveButton.disabled = false;
    }
  };
}

function renderHelpItems(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return `<div class="autoexec-help-empty">${t("toolbox.autoexec.helpEmpty")}</div>`;
  }
  return items.map((item) => `
    <button type="button" class="autoexec-help-item" data-autoexec-command="${escapeAttr(item.command)}" data-autoexec-risk="${escapeAttr(item.risk || "")}">
      <span class="autoexec-help-command">${escapeHtml(item.command)}</span>
      <span class="autoexec-help-summary">${escapeHtml(item.summary)}</span>
      <span class="autoexec-help-meta">${escapeHtml([item.scope, item.risk, item.source].filter(Boolean).join(" · "))}</span>
    </button>
  `).join("");
}

function renderMatches(container, items) {
  if (!container) return;
  container.replaceChildren();
  if (!Array.isArray(items) || items.length === 0) return;
  items.forEach((item) => {
    const row = document.createElement("div");
    row.className = `autoexec-match ${item.known ? "is-known" : "is-unknown"}`;
    const title = document.createElement("div");
    title.className = "autoexec-match-title";
    title.textContent = t("toolbox.autoexec.matchLine", { line: item.line, command: item.command, unknown: item.known ? "" : t("toolbox.autoexec.matchUnknownTag") });
    row.appendChild(title);
    if (item.known && item.help) {
      const detail = document.createElement("div");
      detail.className = "autoexec-match-detail";
      detail.textContent = `${item.help.summary} · ${item.help.scope} · ${item.help.risk} · ${item.help.source}`;
      row.appendChild(detail);
    } else {
      const detail = document.createElement("div");
      detail.className = "autoexec-match-detail";
      detail.textContent = t("toolbox.autoexec.matchHint");
      row.appendChild(detail);
    }
    container.appendChild(row);
  });
}

function closeModal(modal) {
  // 关闭后使当前编辑器及其搜索/保存请求全部失效；否则快速重开时旧回调
  // 可能更新新会话的帮助列表，甚至把新窗口一起关闭。
  openRequestId += 1;
  modal.classList.add("hidden");
}

function isCurrentOpenSession(modal, sessionId) {
  return (
    sessionId === openRequestId &&
    modal?.isConnected &&
    !modal.classList.contains("hidden")
  );
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") return Promise.reject(new Error(t("toolbox.common.backendUnsupported", { method: methodName })));
  return method(...args);
}

function formatBytes(size) {
  const bytes = Number(size || 0);
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(2)} MB`;
}

function formatError(error) {
  return error?.message || String(error || t("toolbox.common.unknownError"));
}

function escapeAttr(value) {
  return String(value || "").replace(/&/g, "&amp;").replace(/"/g, "&quot;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}

function escapeHtml(value) {
  return String(value || "")
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}
