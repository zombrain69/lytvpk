import { showError, showNotification } from "../../core/toast.js";
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
  body.innerHTML = `<div class="autoexec-tool-loading">正在读取 autoexec.cfg...</div>`;

  try {
    const [config, help] = await Promise.all([
      callApp("GetAutoexecConfig"),
      callApp("GetAutoexecCommandHelp", ""),
    ]);
    if (requestId !== openRequestId || modal.classList.contains("hidden")) return;
    renderEditor(modal, config || {}, Array.isArray(help) ? help : []);
  } catch (error) {
    if (requestId !== openRequestId || modal.classList.contains("hidden")) return;
    body.innerHTML = `<div class="autoexec-tool-error">${escapeHtml(formatError(error))}</div>`;
    showError("读取 autoexec.cfg 失败: " + formatError(error));
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
        <h3>autoexec.cfg 编辑器</h3>
        <button type="button" class="close-btn" data-autoexec-tool-close aria-label="关闭">&times;</button>
      </div>
      <div class="modal-body autoexec-tool-body" data-autoexec-tool-body></div>
      <div class="modal-footer">
        <span class="autoexec-resize-hint" aria-hidden="true">可拖动右下角调整窗口大小</span>
        <button type="button" class="btn btn-secondary" data-autoexec-tool-close>关闭</button>
        <button type="button" class="btn btn-primary" data-autoexec-tool-save disabled>保存 autoexec.cfg</button>
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

function renderEditor(modal, config, help) {
  const body = modal.querySelector("[data-autoexec-tool-body]");
  const saveButton = modal.querySelector("[data-autoexec-tool-save]");
  if (!body || !saveButton) return;

  const content = String(config.content || "");
  body.innerHTML = `
    <div class="autoexec-tool-intro">编辑只写回文件，不会在本程序中执行命令。保存时保留原文件的编码、BOM 和换行格式。</div>
    <div class="autoexec-meta autoexec-tool-meta">
      <span class="autoexec-path">${escapeHtml(config.path || "")}</span>
      <span>${config.exists ? "文件存在" : "文件不存在，将在保存时创建"}</span>
      <span>${escapeHtml(config.encoding || "UTF-8")} · ${escapeHtml(config.lineEnding || "CRLF")}</span>
      <span>${formatBytes(config.size)}</span>
    </div>
    <div class="autoexec-tool-layout">
      <div class="autoexec-editor-column">
        <div class="autoexec-editor-shell">
          <pre class="autoexec-line-numbers" data-autoexec-tool-line-numbers aria-hidden="true"></pre>
          <textarea class="autoexec-editor" data-autoexec-tool-editor spellcheck="false" wrap="off" aria-label="autoexec.cfg 内容">${escapeHtml(content)}</textarea>
        </div>
        <div class="autoexec-analysis" data-autoexec-tool-analysis>正在分析命令...</div>
        <div class="autoexec-tool-matches" data-autoexec-tool-matches></div>
        <div class="autoexec-action-row">
          <button type="button" class="trigger-check-btn addonlist-action-btn" data-autoexec-tool-reload>重新读取</button>
          <button type="button" class="trigger-check-btn addonlist-action-btn" data-autoexec-tool-open ${config.exists ? "" : "disabled"}>打开文件位置</button>
        </div>
      </div>
      <aside class="autoexec-help-column">
        <div class="autoexec-help-title">命令说明（点击插入）</div>
        <input type="search" class="form-input" data-autoexec-tool-search placeholder="搜索命令或含义">
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
    if (!editor || typeof window?.go?.app?.App?.AnalyzeAutoexecCommands !== "function") return;
    const requestId = ++analysisRequest;
    try {
      const result = await callApp("AnalyzeAutoexecCommands", editor.value);
      if (requestId !== analysisRequest) return;
      const items = Array.isArray(result) ? result : [];
      const known = items.filter((item) => item.known).length;
      const unknown = items.length - known;
      const highRisk = items.filter((item) => item.known && item.help?.risk?.startsWith("高")).length;
      const mediumRisk = items.filter((item) => item.known && item.help?.risk?.startsWith("中")).length;
      if (analysis) {
        analysis.textContent = `已识别 ${known} 个已收录指令，${unknown} 个未知指令${highRisk ? `；高风险 ${highRisk}` : ""}${mediumRisk ? `；中风险 ${mediumRisk}` : ""}`;
        analysis.classList.toggle("is-warning", unknown > 0 || highRisk > 0);
      }
      renderMatches(matches, items);
    } catch (error) {
      if (requestId !== analysisRequest) return;
      if (analysis) analysis.textContent = `指令识别失败：${formatError(error)}`;
    }
  };
  editor?.addEventListener("input", analyze);
  analyze();

  body.querySelector("[data-autoexec-tool-search]")?.addEventListener("input", (event) => {
    if (helpSearchTimer) window.clearTimeout(helpSearchTimer);
    const query = event.target.value || "";
    helpSearchTimer = window.setTimeout(async () => {
      helpSearchTimer = null;
      const requestId = ++helpSearchRequest;
      try {
        const filtered = await callApp("GetAutoexecCommandHelp", query);
        if (requestId !== helpSearchRequest || modal.classList.contains("hidden") || !body.isConnected) return;
        const list = body.querySelector("[data-autoexec-tool-help]");
        if (list) list.innerHTML = renderHelpItems(filtered);
      } catch (error) {
        if (requestId !== helpSearchRequest || modal.classList.contains("hidden") || !body.isConnected) return;
        showNotification("搜索指令说明失败: " + formatError(error), "error");
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
      showError("打开 autoexec.cfg 位置失败: " + formatError(error));
    }
  });
  saveButton.disabled = !config.path;
  saveButton.onclick = async () => {
    if (!editor) return;
    saveButton.disabled = true;
    try {
      await callApp("SaveAutoexecConfig", editor.value);
      showNotification("已保存 autoexec.cfg（编码与换行格式已保留）", "success");
      closeModal(modal);
    } catch (error) {
      showError("保存 autoexec.cfg 失败: " + formatError(error));
      saveButton.disabled = false;
    }
  };
}

function renderHelpItems(items) {
  if (!Array.isArray(items) || items.length === 0) {
    return `<div class="autoexec-help-empty">未找到匹配指令</div>`;
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
    title.textContent = `第 ${item.line} 行 · ${item.command}${item.known ? "" : "（未知指令）"}`;
    row.appendChild(title);
    if (item.known && item.help) {
      const detail = document.createElement("div");
      detail.className = "autoexec-match-detail";
      detail.textContent = `${item.help.summary} · ${item.help.scope} · ${item.help.risk} · ${item.help.source}`;
      row.appendChild(detail);
    } else {
      const detail = document.createElement("div");
      detail.className = "autoexec-match-detail";
      detail.textContent = "可能来自插件或 Mod，建议确认来源后再保存。";
      row.appendChild(detail);
    }
    container.appendChild(row);
  });
}

function closeModal(modal) {
  modal.classList.add("hidden");
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") return Promise.reject(new Error(`当前后端不支持 ${methodName}`));
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
  return error?.message || String(error || "未知错误");
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
