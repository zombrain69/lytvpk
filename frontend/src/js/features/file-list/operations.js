import {
  appState,
  showLoadingScreen,
  showMainScreen,
  updateLoadingMessage,
  updateStatusBar,
} from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { unpackVPKFromPath } from "../diagnostics/vpk-unpack.js";
import { showConfirmModal } from "../modals/confirm.js";
import { beginMessageModalSession } from "../../core/message-modal.js";
import { performSearch, refreshFilesKeepFilter } from "./filters.js";
import { getFileLoadOrderIndex, refreshLoadOrderMap } from "./sorting.js";
import { updateSingleFileDisplay } from "./render.js";
import { getUnrecordedGameStateOptions } from "./unrecorded-game-state.mjs";
import { buildRenamePreview, computeRenameEditName, sanitizeRenameTitle } from "./rename-edit.mjs";
import { explainActionAvailability, formatExplanation } from "../../core/action-explanation.mjs";
import { formatStrategyGroupEnforcementNotice } from "../settings/strategy-group-format.mjs";
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
import { filePriorityKeys } from "../conflicts/conflict-badge.mjs";
import { formatFileGroupImpact, normalizeGroupKey } from "../mod-groups/group-view.mjs";
import { promptSingletonConflict } from "../conflicts/singleton-conflict.js";

// checkSingletonConflict 调后端判定"同类已启用"，失败时按"没有冲突"处理：
// 这只是体验增强，不能因为它把启用流程整个挡住。
async function checkSingletonConflict(filePath) {
  try {
    const conflict = await getBackendMethod("CheckModEnableConflict")(filePath);
    return conflict && Array.isArray(conflict.conflicts) && conflict.conflicts.length > 0 ? conflict : null;
  } catch (error) {
    console.warn("同类互斥检查失败（已跳过提示）:", error);
    return null;
  }
}

// disableSingletonConflicts 把用户选择"关闭旧的"落实到游戏内开关上。
async function disableSingletonConflicts(conflict) {
  const setGameEnabled = getBackendMethod("SetVPKGameEnabled");
  for (const item of conflict.conflicts || []) {
    try {
      await setGameEnabled(item.path, false);
      [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
        files.forEach((file) => {
          if (file.path === item.path) {
            file.gameStateKnown = true;
            file.gameEnabled = false;
          }
        });
      });
    } catch (error) {
      showError(t("mods.ops.gameState.singletonDisableFailed", { name: item.name || item.path, error }));
    }
  }
  await refreshLoadOrderMap({ silent: true });
}

function getBackendMethod(name) {
  const method = window?.go?.app?.App?.[name];
  if (typeof method !== "function") {
    throw new Error(t("mods.ops.common.backendMissingMethod", { name }));
  }
  return method;
}

export async function toggleGameEnabled(filePath) {
  const file =
    appState.allVpkFiles.find((item) => item.path === filePath) ||
    appState.vpkFiles.find((item) => item.path === filePath);
  if (!file) {
    showError(t("mods.ops.gameState.toggleMissing"));
    return;
  }
  // 前置条件与提示文案共用同一层判断（core/action-explanation.mjs），
  // 避免"这里拦住了，但那边的按钮还亮着、文案也对不上"。
  const gameStateAvailability = explainActionAvailability({ action: "game-state", file });
  if (!gameStateAvailability.available) {
    showError(formatExplanation(gameStateAvailability));
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
    showError(t("mods.ops.gameState.setMissing"));
    return;
  }
  const gameStateAvailability = explainActionAvailability({ action: "game-state", file });
  if (!gameStateAvailability.available) {
    showError(formatExplanation(gameStateAvailability));
    return;
  }

  if (state === "disabled") {
    const disableAvailability = explainActionAvailability({ action: "disable-file", file });
    if (!disableAvailability.available) {
      showError(formatExplanation(disableAvailability));
      return;
    }
    await disableUnrecordedFile(filePath);
    return;
  }

  if (state === "game-enabled" || state === "game-disabled") {
    await setGameEnabled(filePath, state === "game-enabled", !file.gameStateKnown);
    return;
  }

  showError(t("mods.ops.gameState.unknownAction"));
}

async function setGameEnabled(filePath, nextEnabled, wasUnrecorded) {
  if (nextEnabled && !(await confirmVPKOperationWarning(filePath, t("mods.ops.label.enableGame")))) return;
  // 同类互斥：同一角色/同一把武器的替换只会生效一个。启用前先问一次，
  // 避免"启用了却没生效"的困惑（借鉴其它管理器的 SingletonResource 思路）。
  if (nextEnabled) {
    const conflict = await checkSingletonConflict(filePath);
    if (conflict) {
      const choice = await promptSingletonConflict(conflict);
      if (choice === "cancel") return;
      if (choice === "replace") {
        await disableSingletonConflicts(conflict);
      }
    }
  }
  try {
    // 返回值是策略组自动联动改动的其它成员数量（0 表示没有联动）。
    const enforcedCount = await getBackendMethod("SetVPKGameEnabled")(filePath, nextEnabled);

    [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
      files.forEach((item) => {
        if (item.path === filePath) {
          item.gameStateKnown = true;
          item.gameEnabled = nextEnabled;
        }
      });
    });

    // 什么时候需要重建加载顺序映射（顺序真的会变）：
    //   ① 这个 Mod 原本没记录，开启后会在 addonlist.txt 里新增一行；
    //   ② 开启时策略组自动联动也可能替"原本没记录"的组员新增行；
    //      关闭方向的联动只改已有行，顺序不变。
    // 其余情况（最常见：已经在 addonlist 里，翻一下 0/1）顺序没变 ——
    // 省掉 2 次 IPC + 重建，这正是"点一下游戏开关"里剩下的那 80ms 长任务。
    const loadOrderMayChange = wasUnrecorded || (nextEnabled && enforcedCount > 0);
    if (loadOrderMayChange) {
      await refreshLoadOrderMap({ silent: true });
    }
    const file =
      appState.allVpkFiles.find((item) => item.path === filePath) ||
      appState.vpkFiles.find((item) => item.path === filePath);
    // 最常见的路径（改动的是已经在 addonlist 里的记录、没有组联动、也没按
    // 游戏内状态筛选）只影响这一张卡：局部重画它就行。
    // 真机上整表 performSearch() 会重画 2904 张卡，用户点一下就看到 200ms 卡顿。
    const gameStateFilterActive = (appState.selectedGameStates?.length || 0) > 0;
    const canUpdateInPlace =
      !wasUnrecorded && enforcedCount === 0 && !gameStateFilterActive && Boolean(file);
    if (canUpdateInPlace) {
      updateSingleFileDisplay(file);
      updateStatusBar();
    } else {
      await performSearch();
    }
    const orderIndex = getFileLoadOrderIndex(file);
    const priorityHint = wasUnrecorded && nextEnabled && Number.isInteger(orderIndex)
      ? t("mods.ops.gameState.priorityHint", { order: orderIndex + 1 })
      : "";
    showNotification(nextEnabled ? t("mods.ops.gameState.on", { priority: priorityHint }) : t("mods.ops.gameState.off"), "success");

    const enforcementNotice = formatStrategyGroupEnforcementNotice(enforcedCount);
    if (enforcementNotice) {
      showNotification(enforcementNotice, "success");
      // 组内其它成员的开关状态已经变化，重新扫描一次让列表同步。
      await refreshFilesKeepFilter();
    }

    // XDR 动作的"会不会播"取决于同槽的其它 Mod 是否也会被游戏挂载：
    // 有动作槽位的 Mod 一开关，它和同槽对手的角标都该跟着变。
    // 只对这类 Mod 付一次后台静默刷新，普通 Mod 开关的零刷新路径不受影响。
    if ((file?.xdrSlots?.length || 0) > 0 && !enforcementNotice) {
      void refreshFilesKeepFilter({ silent: true });
    }
  } catch (error) {
    console.error("切换游戏内开关失败:", error);
    showError(t("mods.ops.gameState.writeFailed", { error: String(error) }));
  }
}

export const confirmVPKOperationWarning = confirmVPKIntegrityWarning;

async function disableUnrecordedFile(filePath) {
  try {
    if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.disable")))) return;
    await ToggleVPKFile(filePath);
    await refreshFilesKeepFilter();
    showNotification(t("mods.ops.disable.disabledMoved"), "success");
  } catch (error) {
    console.error("禁用未记录 Mod 失败:", error);
    showError(t("mods.ops.disable.singleFailed", { error: String(error) }));
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
    session.titleEl.textContent = t("mods.ops.gameState.unrecordedTitle");
    const content = document.createElement("div");
    content.className = "unrecorded-game-state-content";

    const intro = document.createElement("p");
    intro.className = "unrecorded-game-state-intro";
    intro.textContent = t("mods.ops.gameState.unrecordedIntro", {
      name: file.title || file.name || t("mods.ops.common.thisMod"),
    });
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

    session.confirmBtn.textContent = t("modals.message.cancel");
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
    file?.location === "disabled" ? t("mods.ops.label.restoreEnable") : t("mods.ops.label.disable"),
  ))) {
    return;
  }
  try {
    console.log("切换文件状态:", filePath);
    await ToggleVPKFile(filePath);
    await refreshFilesKeepFilter();
    showNotification(t("mods.ops.toggle.statusUpdated"), "success");
  } catch (error) {
    console.error("切换文件状态失败:", error);
    showError(t("mods.ops.toggle.operationFailed", { error: String(error) }));
  }
}

export async function moveFileToAddons(filePath) {
  if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.copyWorkshop")))) return;
  try {
    console.log("复制文件到插件目录:", filePath);
    const result = await moveWorkshopFileWithConflictResolution(filePath);
    await refreshFilesKeepFilter();
    if (result.successCount > 0) {
      showNotification(t("mods.ops.copy.done"), "success");
    }
    if (result.skippedCount > 0) {
      showNotification(t("mods.ops.copy.skipped"), "info");
    }
    if (result.cancelled) {
      showNotification(t("mods.ops.copy.cancelled"), "info");
    }
    if (result.failCount > 0) {
      showError(t("mods.ops.copy.failed", { error: result.errors[0] || t("mods.ops.common.unknownError") }));
    }
  } catch (error) {
    console.error("复制文件失败:", error);
    showError(t("mods.ops.copy.failed", { error: String(error) }));
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
    showNotification(t("mods.ops.common.noWorkshopFiles"), "info");
    return null;
  }

  if (!(await confirmVPKOperationWarning(paths, t("mods.ops.label.copyWorkshop")))) return null;

  const cleanupProgress = EventsOn("workshop_transfer_progress", (progress) => {
    const current = Number(progress?.current || 0);
    const total = Number(progress?.total || 0);
    const name = String(progress?.name || "").trim();
    const message = String(progress?.message || t("mods.ops.transfer.progressFallback"));
    const suffix = total > 0 ? ` (${current}/${total})` : "";
    updateLoadingMessage(`${message}${name ? `：${name}` : ""}${suffix}`);
  });

  showLoadingScreen();
  updateLoadingMessage(t("mods.ops.transfer.preparing"));
  try {
    const result = await MoveWorkshopFilesToAddons(paths);
    await refreshFilesKeepFilter();

    const successCount = Number(result?.successCount || 0);
    const failCount = Number(result?.failCount || 0);
    const skippedCount = Number(result?.skippedCount || 0);
    const items = Array.isArray(result?.items) ? result.items : [];
    const firstError = items.find((item) => item?.error)?.error || "";
    const warning = t("mods.ops.transfer.warning");

    if (failCount > 0) {
      showNotification(
        `${t("mods.ops.transfer.done", { success: successCount, fail: failCount, skipped: skippedCount })}${firstError ? `。${firstError}` : ""}`,
        "warning",
      );
    } else if (successCount > 0) {
      showNotification(t("mods.ops.transfer.success", { count: successCount, skipped: skippedCount, warning }), "success");
    } else {
      showNotification(t("mods.ops.transfer.none", { count: skippedCount }), "info");
    }
    return result;
  } catch (error) {
    console.error("批量转移创意工坊文件失败:", error);
    showError(t("mods.ops.transfer.failed", { error: String(error) }));
    return null;
  } finally {
    if (typeof cleanupProgress === "function") cleanupProgress();
    showMainScreen();
  }
}

// groupNamesForFilePath 返回该文件所属的策略组名（按 addonlist 键匹配）。
function groupNamesForFilePath(filePath) {
  const file =
    (appState.allVpkFiles || []).find((item) => item.path === filePath) ||
    (appState.vpkFiles || []).find((item) => item.path === filePath);
  if (!file) return [];
  const memberships = appState.modGroupMemberships || [];
  if (memberships.length === 0) return [];
  const keys = new Set(filePriorityKeys(file, appState.currentDirectory).map((key) => normalizeGroupKey(key)));
  const names = [];
  memberships.forEach((membership) => {
    if (!keys.has(normalizeGroupKey(membership?.key))) return;
    const name = String(membership?.groupName || membership?.groupId || "").trim();
    if (name && !names.includes(name)) names.push(name);
  });
  return names;
}

// confirmGroupImpactForDelete 删除前提示"这个 Mod 还在哪些策略组里"：
// 删除只影响文件，组成员会保留为缺失成员，放回同名文件即自动恢复。
function confirmGroupImpactForDelete(filePath) {
  const names = groupNamesForFilePath(filePath);
  const message = formatFileGroupImpact(names);
  if (!message) return Promise.resolve(true);
  return new Promise((resolve) => {
    showConfirmModal(
      t("mods.ops.common.groupImpactTitle"),
      message,
      () => resolve(true),
      false,
      "",
      () => resolve(false),
    );
  });
}

export function deleteFile(filePath) {
  showConfirmModal(t("mods.ops.delete.confirmTitle"), t("mods.ops.delete.confirmBody"), async () => {
    try {
      console.log("删除文件:", filePath);
      if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.delete")))) return;
      if (!(await confirmGroupImpactForDelete(filePath))) return;
      await DeleteVPKFile(filePath);
      await refreshFilesKeepFilter();
      showNotification(t("mods.ops.delete.trashed"), "success");
    } catch (error) {
      console.error("删除文件失败:", error);
      showError(t("mods.ops.delete.failed", { error: String(error) }));
    }
  });
}

export async function openFileLocation(filePath) {
  try {
    console.log("打开文件所在位置:", filePath);
    await OpenFileLocation(filePath);
    showNotification(t("mods.ops.openLocation.done"), "success");
  } catch (error) {
    console.error("打开文件位置失败:", error);
    showError(t("mods.ops.openLocation.failed", { error: String(error) }));
  }
}

export async function unpackFile(filePath) {
  if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.unpack")))) return;
  await unpackVPKFromPath(filePath);
}
export async function toggleFileVisibility(filePath) {
  try {
    if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.toggleVisibility")))) return;
    console.log("切换文件隐藏状态:", filePath);
    await ToggleVPKVisibility(filePath);
    await refreshFilesKeepFilter();
    showNotification(t("mods.ops.visibility.updated"), "success");
  } catch (error) {
    console.error("切换隐藏状态失败:", error);
    showError(t("mods.ops.toggle.operationFailed", { error: String(error) }));
  }
}

const WINDOWS_PATH_WARNING_LENGTH = 240;
const WINDOWS_MAX_PATH_LENGTH = 260;
const WINDOWS_MAX_FILENAME_LENGTH = 255;

export async function renameFile(filePath) {
  const file =
    (appState.vpkFiles || []).find((f) => f.path === filePath) ||
    (appState.allVpkFiles || []).find((f) => f.path === filePath);
  if (!file) {
    showError(t("mods.ops.rename.missing"));
    return;
  }

  const fileName = file.name;
  const isHidden = fileName.startsWith("_");
  const editName = computeRenameEditName(fileName);

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
      ? t("modals.rename.useModNameTooltip")
      : t("modals.rename.noModTitle");
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
        message: t("modals.rename.lengthTooLong", { current: filenameLength, max: WINDOWS_MAX_FILENAME_LENGTH }),
      };
    } else if (pathLength > WINDOWS_MAX_PATH_LENGTH) {
      renameLengthState = {
        hasError: true,
        message: t("modals.rename.lengthPathTooLong", { current: pathLength, max: WINDOWS_MAX_PATH_LENGTH }),
      };
    } else if (pathLength >= WINDOWS_PATH_WARNING_LENGTH) {
      renameLengthState = {
        hasError: false,
        message: t("modals.rename.lengthPathWarning", { current: pathLength, max: WINDOWS_MAX_PATH_LENGTH }),
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
      showError(t("modals.rename.noModTitle"));
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
      showError(t("modals.rename.emptyName"));
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
      if (!(await confirmVPKOperationWarning(filePath, t("mods.ops.label.rename")))) return;
      await RenameVPKFile(filePath, finalName);
      showNotification(t("mods.ops.rename.done"), "success");
      cleanup();
      await refreshFilesKeepFilter();
    } catch (error) {
      console.error("重命名失败:", error);
      showError(t("mods.ops.rename.failed", { error: String(error) }));
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
