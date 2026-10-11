// VPK 内容预览（只读）：不解包直接看容器里的文本 / KeyValues / 图片 / VTF 贴图。
//
// 为什么做：社区里最常见的困惑就是"这个 VPK 里到底装了什么"。以前只能先解包再翻目录；
// 这里给出条目列表 + 就地预览，路径事实与标签识别用的是同一套 VPK 读取代码。

import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { appState } from "../state.js";

let deps = {};
let currentFile = "";

export function configureVPKPreview(nextDeps = {}) {
  deps = nextDeps || {};
}

function modalElement() {
  return document.getElementById("vpk-preview-modal");
}

export function closeVPKPreview() {
  modalElement()?.classList.add("hidden");
}

export async function openVPKPreviewTool(explicitPath) {
  const modal = modalElement();
  if (!modal) return;
  let target = explicitPath;
  if (!target) {
    const selected = [...(appState.selectedFiles || [])]
      .filter((path) => typeof path === "string" && path.toLowerCase().endsWith(".vpk"));
    if (selected.length === 1) {
      target = selected[0];
    } else if (selected.length > 1) {
      showNotification(t("toolbox.vpkPreview.singleOnly"), "info");
      return;
    }
  }
  if (!target) {
    try {
      target = await deps.SelectVPKFile();
    } catch (error) {
      showError(t("toolbox.common.selectVpkFailed", { error }));
      return;
    }
  }
  if (!target) return;

  currentFile = target;
  modal.classList.remove("hidden");
  const title = document.getElementById("vpk-preview-title");
  if (title) title.textContent = t("toolbox.vpkPreview.title", { name: target.split(/[\\/]/).pop() });
  const preview = document.getElementById("vpk-preview-body");
  if (preview) preview.innerHTML = `<div class="setting-row-desc">${t("toolbox.vpkPreview.loadingEntries")}</div>`;
  const list = document.getElementById("vpk-preview-entries");
  if (list) list.innerHTML = "";

  let payload = null;
  try {
    payload = await deps.ListVPKEntries(target);
  } catch (error) {
    showError(t("toolbox.vpkPreview.readFailed", { error }));
    return;
  }
  const summary = document.getElementById("vpk-preview-summary");
  if (summary) {
    const truncatedPart = payload.truncated
      ? t("toolbox.vpkPreview.summaryTruncated", { count: payload.entries.length })
      : "";
    summary.textContent = t("toolbox.vpkPreview.summary", { count: payload.totalCount, truncatedPart });
  }
  if (!list) return;
  payload.entries.forEach((entry) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "vpk-preview-entry";
    button.dataset.path = entry.path;
    const name = document.createElement("span");
    name.className = "vpk-preview-entry-name";
    name.textContent = entry.path;
    const size = document.createElement("span");
    size.className = "vpk-preview-entry-size";
    size.textContent = entry.size >= 1024 ? `${Math.round(entry.size / 1024)} KB` : `${entry.size} B`;
    button.append(name, size);
    button.addEventListener("click", () => {
      list.querySelectorAll(".vpk-preview-entry").forEach((item) => item.classList.remove("active"));
      button.classList.add("active");
      void loadEntry(entry.path);
    });
    list.appendChild(button);
  });
  if (payload.entries.length === 0) {
    if (preview) preview.innerHTML = `<div class="setting-row-desc">${t("toolbox.vpkPreview.empty")}</div>`;
  }
}

async function loadEntry(entryPath) {
  const body = document.getElementById("vpk-preview-body");
  if (!body) return;
  body.innerHTML = `<div class="setting-row-desc">${t("toolbox.vpkPreview.loadingEntry", { path: entryPath })}</div>`;
  let result = null;
  try {
    result = await deps.PreviewVPKEntry(currentFile, entryPath);
  } catch (error) {
    body.innerHTML = "";
    const message = document.createElement("div");
    message.className = "addonlist-status-error";
    message.textContent = t("toolbox.vpkPreview.previewFailed", { error });
    body.appendChild(message);
    return;
  }
  body.innerHTML = "";
  const meta = document.createElement("div");
  meta.className = "setting-row-desc";
  const sizeText = result.size >= 1024 ? `${Math.round(result.size / 1024)} KB` : `${result.size} B`;
  meta.textContent = `${entryPath} · ${sizeText}${result.note ? ` · ${result.note}` : ""}`;
  body.appendChild(meta);

  // 文本 / 模型信息卡 / 地图信息卡都用等宽文本渲染（后两者是后端格式化好的报告）。
  if (result.kind === "text" || result.kind === "model" || result.kind === "map") {
    const pre = document.createElement("pre");
    pre.className = "vpk-preview-text";
    pre.textContent = result.text || "";
    body.appendChild(pre);
    return;
  }
  if (result.kind === "image" && result.dataUrl) {
    const img = document.createElement("img");
    img.className = "vpk-preview-image";
    img.src = result.dataUrl;
    img.alt = entryPath;
    body.appendChild(img);
    return;
  }
  // 音频（.wav / .mp3）：data URL 直接喂给浏览器原生播放器；元素内的文本是
  // 不支持的浏览器看到的兜底说明。
  if (result.kind === "audio" && result.dataUrl) {
    const audio = document.createElement("audio");
    audio.className = "vpk-preview-audio";
    audio.controls = true;
    audio.preload = "metadata";
    audio.src = result.dataUrl;
    audio.textContent = t("toolbox.vpkPreview.audioFallback");
    body.appendChild(audio);
    return;
  }
  const note = document.createElement("div");
  note.className = "setting-row-desc";
  note.textContent = result.note || t("toolbox.vpkPreview.unsupported");
  body.appendChild(note);
}
