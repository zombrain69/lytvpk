// Mod 详情里的"该 Mod 的冲突忽略文件"编辑器。
//
// 语义与后端一致（对齐 FireAxe 的 ConflictIgnoringFiles）：
// 命中的路径只让**这个 Mod** 退出对应资源的冲突判定，
// 内置规则与设置页的全局忽略清单始终生效，且这里的清单只写 ignore.json。

import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import {
  DeleteModIgnoreFiles,
  GetModIgnoreFiles,
  SetModIgnoreFiles,
} from "../../../../wailsjs/go/app/App";
import {
  formatConflictIgnoreList,
  parseConflictIgnoreList,
} from "../conflicts/conflict-ignore-options.mjs";
import { addonListKeyForDetail } from "./detail-ignore-key.mjs";

let controlsBound = false;
let currentKey = "";
let currentName = "";

function element(id) {
  return document.getElementById(id);
}

function setStatus(message, isError = false) {
  const status = element("detail-ignore-status");
  if (!status) return;
  status.textContent = message;
  status.classList.toggle("error", Boolean(isError));
}

export async function syncDetailIgnoreEditor(file) {
  const section = element("detail-ignore-section");
  const input = element("detail-ignore-input");
  currentKey = addonListKeyForDetail(file);
  currentName = String(file?.name || "");
  if (!section || !input) return;

  if (!currentKey) {
    section.classList.add("hidden");
    return;
  }
  section.classList.remove("hidden");
  input.value = "";
  setStatus(t("modals.detailIgnore.loading"));
  try {
    const entries = await GetModIgnoreFiles(currentKey);
    input.value = formatConflictIgnoreList(entries);
    setStatus(
      Array.isArray(entries) && entries.length > 0
        ? t("modals.detailIgnore.savedCount", { count: entries.length })
        : t("modals.detailIgnore.notSet"),
    );
  } catch (error) {
    setStatus(t("modals.detailIgnore.readFailed", { error: String(error?.message || error) }), true);
  }
}

async function saveCurrentIgnoreList() {
  if (!currentKey) {
    showError(t("modals.detailIgnore.noKeySave"));
    return;
  }
  const input = element("detail-ignore-input");
  if (!input) return;
  const files = parseConflictIgnoreList(input.value);
  try {
    const record = await SetModIgnoreFiles(currentKey, currentName, files);
    input.value = formatConflictIgnoreList(record?.files || files);
    setStatus(files.length > 0 ? t("modals.detailIgnore.savedCount", { count: files.length }) : t("modals.detailIgnore.cleared"));
    showNotification(
      files.length > 0
        ? t("modals.detailIgnore.savedFile")
        : t("modals.detailIgnore.clearedFile"),
      "success",
    );
  } catch (error) {
    setStatus(t("modals.detailIgnore.saveFailed", { error: String(error?.message || error) }), true);
  }
}

async function clearCurrentIgnoreList() {
  if (!currentKey) return;
  const input = element("detail-ignore-input");
  if (input) input.value = "";
  try {
    await DeleteModIgnoreFiles(currentKey);
    setStatus(t("modals.detailIgnore.cleared"));
    showNotification(t("modals.detailIgnore.clearedFile"), "success");
  } catch (error) {
    setStatus(t("modals.detailIgnore.clearFailed", { error: String(error?.message || error) }), true);
  }
}

export function initDetailIgnoreControls() {
  if (controlsBound) return;
  controlsBound = true;
  element("detail-ignore-save-btn")?.addEventListener("click", saveCurrentIgnoreList);
  element("detail-ignore-clear-btn")?.addEventListener("click", clearCurrentIgnoreList);
}

// 供测试与其它模块复用：当前编辑的 addonlist 键。
export function currentDetailIgnoreKey() {
  return appState?.fileDetailIgnoreKey || currentKey;
}
