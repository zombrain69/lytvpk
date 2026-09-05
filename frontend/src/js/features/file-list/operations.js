import {
  appState,
  showLoadingScreen,
  showMainScreen,
  updateLoadingMessage,
} from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { unpackVPKFromPath } from "../diagnostics/vpk-unpack.js";
import { showConfirmModal } from "../modals/confirm.js";
import { beginMessageModalSession } from "../../core/message-modal.js";
import { performSearch, refreshFilesKeepFilter } from "./filters.js";
import { getFileLoadOrderIndex, refreshLoadOrderMap } from "./sorting.js";
import { getUnrecordedGameStateOptions } from "./unrecorded-game-state.mjs";
import {
  ToggleVPKFile,
  DeleteVPKFile,
  OpenFileLocation,
  RenameVPKFile,
  ToggleVPKVisibility,
  MoveWorkshopFilesToAddons,
} from "../../../../wailsjs/go/app/App";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import { moveWorkshopFileWithConflictResolution } from "./file-move-conflicts.js";
import { confirmVPKIntegrityWarning } from "./vpk-risk-warning.js";

function getBackendMethod(name) {
  const method = window?.go?.app?.App?.[name];
  if (typeof method !== "function") {
    throw new Error(`当前应用未提供 ${name}，请重新构建并启动 LytVPK`);
  }
  return method;
}

export async function toggleGameEnabled(filePath) {
  const file =
    appState.allVpkFiles.find((item) => item.path === filePath) ||
    appState.vpkFiles.find((item) => item.path === filePath);
  if (!file) {
    showError("未找到要切换游戏内开关的 Mod，请刷新后重试");
    return;
  }
  if (file.location === "disabled") {
    showError("该 Mod 位于 disabled 目录，请先恢复文件后再编辑游戏内开关");
    return;
  }

  const wasUnrecorded = !file.gameStateKnown;
  if (wasUnrecorded) {
    const action = await chooseUnrecordedGameStateAction(file);
    if (!action) return;
    if (action === "disabled") {
      await disableUnrecordedFile(filePath);
      return;
    }
    await setGameEnabled(filePath, action === "game-enabled", true);
    return;
  }

  await setGameEnabled(filePath, !file.gameEnabled, false);
}

export async function setGameState(filePath, state) {
  const file =
    appState.allVpkFiles.find((item) => item.path === filePath) ||
    appState.vpkFiles.find((item) => item.path === filePath);
  if (!file) {
    showError("未找到要设置游戏内状态的 Mod，请刷新后重试");
    return;
  }
  if (file.location === "disabled") {
    showError("该 Mod 位于 disabled 目录，请先恢复文件后再编辑游戏内开关");
    return;
  }

  if (state === "disabled") {
    if (file.location === "workshop") {
      showError("创意工坊 Mod 不能直接禁用，请先复制到 addons");
      return;
    }
    await disableUnrecordedFile(filePath);
    return;
  }

  if (state === "game-enabled" || state === "game-disabled") {
    await setGameEnabled(filePath, state === "game-enabled", !file.gameStateKnown);
    return;
  }

  showError("未知的游戏内状态操作，请刷新后重试");
}

async function setGameEnabled(filePath, nextEnabled, wasUnrecorded) {
  if (nextEnabled && !(await confirmVPKOperationWarning(filePath, "启用游戏内 Mod"))) return;
  try {
    await getBackendMethod("SetVPKGameEnabled")(filePath, nextEnabled);

    [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
      files.forEach((item) => {
        if (item.path === filePath) {
          item.gameStateKnown = true;
          item.gameEnabled = nextEnabled;
        }
      });
    });

    // SetVPKGameEnabled 可能首次把根目录 Mod 写入 addonlist.txt；同步重建
    // 加载顺序映射，避免新条目直到下一次完整刷新才出现优先级编号。
    await refreshLoadOrderMap({ silent: true });
    await performSearch();
    const file =
      appState.allVpkFiles.find((item) => item.path === filePath) ||
      appState.vpkFiles.find((item) => item.path === filePath);
    const orderIndex = getFileLoadOrderIndex(file);
    const priorityHint = wasUnrecorded && nextEnabled && Number.isInteger(orderIndex)
      ? `（新增为优先级 #${orderIndex + 1}）`
      : "";
    showNotification(nextEnabled ? `已在 addonlist.txt 中开启 Mod${priorityHint}` : "已在 addonlist.txt 中关闭 Mod", "success");
  } catch (error) {
    console.error("切换游戏内开关失败:", error);
    const message = String(error || "");
    if (message.includes("游戏不会接受此 Mod") || message.includes("addoninfo.txt 格式无效")) {
      showError(message);
    } else {
      showError("写入 addonlist.txt 失败: " + error);
    }
  }
}

export const confirmVPKOperationWarning = confirmVPKIntegrityWarning;

async function disableUnrecordedFile(filePath) {
  try {
    if (!(await confirmVPKOperationWarning(filePath, "禁用 Mod"))) return;
    await ToggleVPKFile(filePath);
    await refreshFilesKeepFilter();
    showNotification("已禁用 Mod，并移入 disabled 目录", "success");
  } catch (error) {
    console.error("禁用未记录 Mod 失败:", error);
    showError("禁用失败: " + error);
  }
}

function chooseUnrecordedGameStateAction(file) {
  return new Promise((resolve) => {
    let settled = false;
    const session = beginMessageModalSession({
      onClose: () => {
        if (!settled) {
          settled = true;
          resolve(null);
        }
      },
    });
    if (!session) {
      resolve(null);
      return;
    }

    session.addModalClass("unrecorded-game-state-modal-content");
    session.titleEl.textContent = "未记录 Mod：选择状态";
    const content = document.createElement("div");
    content.className = "unrecorded-game-state-content";

    const intro = document.createElement("p");
    intro.className = "unrecorded-game-state-intro";
    intro.textContent = `“${file.title || file.name || "此 Mod"}” 尚未记录在 addonlist.txt，请选择要切换到的状态：`;
    content.appendChild(intro);

    const list = document.createElement("div");
    list.className = "unrecorded-game-state-options";
    getUnrecordedGameStateOptions(file).forEach((option) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "btn unrecorded-game-state-option";
      button.disabled = Boolean(option.disabled);
      button.title = option.disabled ? option.disabledReason : option.description;

      const label = document.createElement("strong");
      label.textContent = option.label;
      button.appendChild(label);
      const description = document.createElement("span");
      description.textContent = option.disabled ? option.disabledReason : option.description;
      button.appendChild(description);

      button.onclick = () => {
        if (button.disabled || !session.isCurrent()) return;
        settled = true;
        session.close("choice");
        resolve(option.id);
      };
      list.appendChild(button);
    });
    content.appendChild(list);
    session.contentEl.replaceChildren(content);

    session.confirmBtn.textContent = "取消";
    session.confirmBtn.onclick = () => {
      if (!session.isCurrent()) return;
      settled = true;
      session.close("cancel");
      resolve(null);
    };
    session.closeBtn.onclick = () => {
      if (!session.isCurrent()) return;
      settled = true;
      session.close("close");
      resolve(null);
    };
    session.show();
  });
}

export async function toggleFile(filePath) {
  const file =
    appState.allVpkFiles.find((item) => item.path === filePath) ||
    appState.vpkFiles.find((item) => item.path === filePath);
  if (!(await confirmVPKOperationWarning(
    filePath,
    file?.location === "disabled" ? "恢复并启用 Mod" : "禁用 Mod",
  ))) {
    return;
  }
  try {
    console.log("切换文件状态:", filePath);
    await ToggleVPKFile(filePath);
    await refreshFilesKeepFilter();
    showNotification("文件状态已更新", "success");
  } catch (error) {
    console.error("切换文件状态失败:", error);
    showError("操作失败: " + error);
  }
}

export async function moveFileToAddons(filePath) {
  if (!(await confirmVPKOperationWarning(filePath, "复制 Workshop Mod"))) return;
  try {
    console.log("复制文件到插件目录:", filePath);
    const result = await moveWorkshopFileWithConflictResolution(filePath);
    await refreshFilesKeepFilter();
    if (result.successCount > 0) {
      showNotification("文件已复制到 addons，workshop 原件已保留并关闭", "success");
    }
    if (result.skippedCount > 0) {
      showNotification("目标文件已存在，已跳过复制", "info");
    }
    if (result.cancelled) {
      showNotification("已取消复制", "info");
    }
    if (result.failCount > 0) {
      showError(`复制失败: ${result.errors[0] || "未知错误"}`);
    }
  } catch (error) {
    console.error("复制文件失败:", error);
    showError("复制失败: " + error);
  }
}

export async function moveWorkshopFilesToAddons(filePaths) {
  const paths = (Array.isArray(filePaths) ? filePaths : [])
    .filter(Boolean)
    .filter((filePath) => {
      const file =
        appState.allVpkFiles.find((item) => item.path === filePath) ||
        appState.vpkFiles.find((item) => item.path === filePath);
      return !file || file.location === "workshop";
    });
  if (paths.length === 0) {
    showNotification("没有可转移的创意工坊文件", "info");
    return null;
  }

  if (!(await confirmVPKOperationWarning(paths, "复制 Workshop Mod"))) return null;

  const cleanupProgress = EventsOn("workshop_transfer_progress", (progress) => {
    const current = Number(progress?.current || 0);
    const total = Number(progress?.total || 0);
    const name = String(progress?.name || "").trim();
    const message = String(progress?.message || "正在转移...");
    const suffix = total > 0 ? ` (${current}/${total})` : "";
    updateLoadingMessage(`${message}${name ? `：${name}` : ""}${suffix}`);
  });

  showLoadingScreen();
  updateLoadingMessage("正在准备转移创意工坊文件...");
  try {
    const result = await MoveWorkshopFilesToAddons(paths);
    await refreshFilesKeepFilter();

    const successCount = Number(result?.successCount || 0);
    const failCount = Number(result?.failCount || 0);
    const skippedCount = Number(result?.skippedCount || 0);
    const items = Array.isArray(result?.items) ? result.items : [];
    const firstError = items.find((item) => item?.error)?.error || "";
    const warning = "当前 Fork 会保留 workshop 原件，并同步 addonlist.txt 状态";

    if (failCount > 0) {
      showNotification(
        `转移完成：成功 ${successCount} 个，失败 ${failCount} 个，跳过 ${skippedCount} 个${firstError ? `。${firstError}` : ""}`,
        "warning",
      );
    } else if (successCount > 0) {
      showNotification(`成功转移 ${successCount} 个文件，跳过 ${skippedCount} 个。${warning}`, "success");
    } else {
      showNotification(`没有可转移的创意工坊文件，已跳过 ${skippedCount} 个`, "info");
    }
    return result;
  } catch (error) {
    console.error("批量转移创意工坊文件失败:", error);
    showError("批量转移失败: " + error);
    return null;
  } finally {
    if (typeof cleanupProgress === "function") cleanupProgress();
    showMainScreen();
  }
}

export function deleteFile(filePath) {
  showConfirmModal("确认删除", "确定要将此文件移至回收站吗？", async () => {
    try {
      console.log("删除文件:", filePath);
      if (!(await confirmVPKOperationWarning(filePath, "删除 Mod"))) return;
      await DeleteVPKFile(filePath);
      await refreshFilesKeepFilter();
      showNotification("文件已移至回收站", "success");
    } catch (error) {
      console.error("删除文件失败:", error);
      showError("删除失败: " + error);
    }
  });
}

export async function openFileLocation(filePath) {
  try {
    console.log("打开文件所在位置:", filePath);
    await OpenFileLocation(filePath);
    showNotification("已打开文件所在位置", "success");
  } catch (error) {
    console.error("打开文件位置失败:", error);
    showError("打开位置失败: " + error);
  }
}

export async function unpackFile(filePath) {
  if (!(await confirmVPKOperationWarning(filePath, "解包 Mod"))) return;
  await unpackVPKFromPath(filePath);
}
export async function toggleFileVisibility(filePath) {
  try {
    if (!(await confirmVPKOperationWarning(filePath, "切换 Mod 隐藏状态"))) return;
    console.log("切换文件隐藏状态:", filePath);
    await ToggleVPKVisibility(filePath);
    await refreshFilesKeepFilter();
    showNotification("文件隐藏状态已更新", "success");
  } catch (error) {
    console.error("切换隐藏状态失败:", error);
    showError("操作失败: " + error);
  }
}

function sanitizeRenameTitle(title) {
  const cleaned = String(title || "")
    .replace(/[\u0000-\u001f<>:"/\\|?*]+/g, " ")
    .replace(/\.vpk$/i, "")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/[. ]+$/g, "");

  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i.test(cleaned)) {
    return `_${cleaned}`;
  }

  return cleaned;
}

const WINDOWS_PATH_WARNING_LENGTH = 240;
const WINDOWS_MAX_PATH_LENGTH = 260;
const WINDOWS_MAX_FILENAME_LENGTH = 255;

function getDirectoryPath(filePath) {
  const path = String(filePath || "");
  const separatorIndex = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
  return separatorIndex >= 0 ? path.slice(0, separatorIndex) : "";
}

function joinPathForPreview(dir, filename) {
  if (!dir) return filename;
  if (dir.endsWith("\\") || dir.endsWith("/")) {
    return dir + filename;
  }
  return `${dir}\\${filename}`;
}

function parseFilenameTags(filename) {
  const match = String(filename || "").match(/^(_?)\[(.*?)\](.*)$/);
  if (!match) {
    return { hasTags: false, tags: [] };
  }

  const tags = match[2]
    .split(/[,+]/)
    .map((tag) => tag.trim())
    .filter(Boolean);

  return { hasTags: true, tags };
}

function buildRenamePreview(filePath, oldFileName, inputName, isHidden) {
  let finalName = inputName.trim();
  if (!finalName.toLowerCase().endsWith(".vpk")) {
    finalName += ".vpk";
  }
  if (isHidden) {
    finalName = "_" + finalName;
  }

  const oldTags = parseFilenameTags(oldFileName);
  const newTags = parseFilenameTags(finalName);
  if (oldTags.hasTags && !newTags.hasTags && oldTags.tags.length > 0) {
    let prefix = "";
    let body = finalName;
    if (finalName.startsWith("_")) {
      prefix = "_";
      body = finalName.substring(1);
    }

    finalName = `${prefix}[${oldTags.tags.join("+")}]${body}`;
  }

  return {
    filename: finalName,
    path: joinPathForPreview(getDirectoryPath(filePath), finalName),
  };
}

export async function renameFile(filePath) {
  const file =
    (appState.vpkFiles || []).find((f) => f.path === filePath) ||
    (appState.allVpkFiles || []).find((f) => f.path === filePath);
  if (!file) {
    showError("未找到要重命名的文件，请刷新列表后重试");
    return;
  }

  const fileName = file.name;
  const isHidden = fileName.startsWith("_");

  let editName = fileName;
  const tagMatch = fileName.match(/^_?\[(.*?)\](.*)$/);
  if (tagMatch) {
    editName = (tagMatch[1] || "") + tagMatch[2];
  }

  if (isHidden) {
    editName = editName.substring(1);
  }
  if (editName.toLowerCase().endsWith(".vpk")) {
    editName = editName.substring(0, editName.length - 4);
  }

  const modal = document.getElementById("rename-modal");
  const input = document.getElementById("rename-input");
  const fillFromTitleBtn = document.getElementById("fill-rename-from-title-btn");
  const lengthHint = document.getElementById("rename-length-hint");
  const confirmBtn = document.getElementById("confirm-rename-btn");
  const cancelBtn = document.getElementById("cancel-rename-btn");
  const closeBtn = document.getElementById("close-rename-modal-btn");
  const modTitle = sanitizeRenameTitle(file.title);
  let renameLengthState = { hasError: false, message: "" };

  input.value = editName;
  if (fillFromTitleBtn) {
    fillFromTitleBtn.disabled = !modTitle;
    fillFromTitleBtn.title = modTitle
      ? "使用 addoninfo 中的 Mod 名称"
      : "未解析到 Mod 名称";
  }
  modal.classList.remove("hidden");
  input.focus();
  input.select();

  const cleanup = () => {
    modal.classList.add("hidden");
    confirmBtn.onclick = null;
    cancelBtn.onclick = null;
    closeBtn.onclick = null;
    if (fillFromTitleBtn) {
      fillFromTitleBtn.onclick = null;
    }
    input.oninput = null;
    input.onkeydown = null;
  };

  const updateRenameLengthHint = () => {
    const newName = input.value.trim();
    renameLengthState = { hasError: false, message: "" };
    confirmBtn.disabled = false;

    if (!lengthHint) return renameLengthState;

    lengthHint.classList.add("hidden");
    lengthHint.classList.remove("is-warning", "is-error");
    lengthHint.textContent = "";

    if (!newName) {
      return renameLengthState;
    }

    const preview = buildRenamePreview(filePath, fileName, newName, isHidden);
    const filenameLength = preview.filename.length;
    const pathLength = preview.path.length;

    if (filenameLength > WINDOWS_MAX_FILENAME_LENGTH) {
      renameLengthState = {
        hasError: true,
        message: `文件名过长：${filenameLength}/${WINDOWS_MAX_FILENAME_LENGTH}，请缩短名称`,
      };
    } else if (pathLength > WINDOWS_MAX_PATH_LENGTH) {
      renameLengthState = {
        hasError: true,
        message: `完整路径过长：${pathLength}/${WINDOWS_MAX_PATH_LENGTH}，请缩短名称或移动 Mod 目录`,
      };
    } else if (pathLength >= WINDOWS_PATH_WARNING_LENGTH) {
      renameLengthState = {
        hasError: false,
        message: `完整路径较长：${pathLength}/${WINDOWS_MAX_PATH_LENGTH}，接近 Windows 限制`,
      };
    }

    if (renameLengthState.message) {
      lengthHint.textContent = renameLengthState.message;
      lengthHint.classList.remove("hidden");
      lengthHint.classList.add(renameLengthState.hasError ? "is-error" : "is-warning");
    }

    confirmBtn.disabled = renameLengthState.hasError;
    return renameLengthState;
  };

  const fillFromTitle = () => {
    if (!modTitle) {
      showError("未解析到 Mod 名称");
      return;
    }

    input.value = modTitle;
    updateRenameLengthHint();
    input.focus();
    input.setSelectionRange(input.value.length, input.value.length);
  };

  const doRename = async () => {
    const newName = input.value.trim();
    if (!newName) {
      showError("文件名不能为空");
      return;
    }

    if (newName === editName) {
      cleanup();
      return;
    }

    const lengthState = updateRenameLengthHint();
    if (lengthState.hasError) {
      showError(lengthState.message);
      return;
    }

    let finalName = newName;
    if (!finalName.toLowerCase().endsWith(".vpk")) {
      finalName += ".vpk";
    }
    if (isHidden) {
      finalName = "_" + finalName;
    }

    try {
      if (!(await confirmVPKOperationWarning(filePath, "重命名 Mod"))) return;
      await RenameVPKFile(filePath, finalName);
      showNotification("重命名成功", "success");
      cleanup();
      await refreshFilesKeepFilter();
    } catch (error) {
      console.error("重命名失败:", error);
      showError("重命名失败: " + error);
    }
  };

  if (fillFromTitleBtn) {
    fillFromTitleBtn.onclick = fillFromTitle;
  }
  confirmBtn.onclick = doRename;
  cancelBtn.onclick = cleanup;
  closeBtn.onclick = cleanup;
  input.oninput = updateRenameLengthHint;

  input.onkeydown = (e) => {
    if (e.key === "Enter") {
      doRename();
    } else if (e.key === "Escape") {
      cleanup();
    }
  };

  updateRenameLengthHint();
}
