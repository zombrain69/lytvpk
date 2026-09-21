// Mod 详情里的"该 Mod 的冲突忽略文件"编辑器。
//
// 语义与后端一致（对齐 FireAxe 的 ConflictIgnoringFiles）：
// 命中的路径只让**这个 Mod** 退出对应资源的冲突判定，
// 内置规则与设置页的全局忽略清单始终生效，且这里的清单只写 ignore.json。

import { appState } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
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
  setStatus("正在读取…");
  try {
    const entries = await GetModIgnoreFiles(currentKey);
    input.value = formatConflictIgnoreList(entries);
    setStatus(
      Array.isArray(entries) && entries.length > 0
        ? `已保存 ${entries.length} 条忽略规则`
        : "尚未设置：该 Mod 参与全部资源重叠判定",
    );
  } catch (error) {
    setStatus("读取忽略清单失败: " + String(error?.message || error), true);
  }
}

async function saveCurrentIgnoreList() {
  if (!currentKey) {
    showError("当前 Mod 没有可用的 addonlist 键，无法保存忽略清单");
    return;
  }
  const input = element("detail-ignore-input");
  if (!input) return;
  const files = parseConflictIgnoreList(input.value);
  try {
    const record = await SetModIgnoreFiles(currentKey, currentName, files);
    input.value = formatConflictIgnoreList(record?.files || files);
    setStatus(files.length > 0 ? `已保存 ${files.length} 条忽略规则` : "已清空该 Mod 的忽略规则");
    showNotification(
      files.length > 0
        ? "已保存该 Mod 的冲突忽略文件（下次分析生效）"
        : "已清空该 Mod 的冲突忽略文件",
      "success",
    );
  } catch (error) {
    setStatus("保存忽略清单失败: " + String(error?.message || error), true);
  }
}

async function clearCurrentIgnoreList() {
  if (!currentKey) return;
  const input = element("detail-ignore-input");
  if (input) input.value = "";
  try {
    await DeleteModIgnoreFiles(currentKey);
    setStatus("已清空该 Mod 的忽略规则");
    showNotification("已清空该 Mod 的冲突忽略文件", "success");
  } catch (error) {
    setStatus("清空忽略清单失败: " + String(error?.message || error), true);
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
