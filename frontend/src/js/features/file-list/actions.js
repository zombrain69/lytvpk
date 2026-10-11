import {
  appState,
  toggleFileSelection,
  updateStatusBar,
  showLoadingScreen,
  showMainScreen,
  updateLoadingMessage,
} from "../state.js";
import { showError, showNotification, showSuccess } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { showConfirmModal } from "../modals/confirm.js";
import { renderFileList, updateSingleFileDisplay } from "./render.js";
import { performSearch, refreshFilesKeepFilter } from "./filters.js";
import {
  SetVPKGameEnabledBatch,
  ToggleVPKFile,
  ExportVPKFilesToZip,
  DeleteVPKFiles,
  SelectDirectory,
  GetVPKFiles,
  ToggleVPKVisibility,
  GenerateWorkshopShortcuts,
} from "../../../../wailsjs/go/app/App";
import { EventsOn } from "../../../../wailsjs/runtime/runtime";
import { moveVpkFilesWithConflictResolution } from "./file-move-conflicts.js";
import { formatMoveFailures } from "./move-result-format.mjs";
import { confirmVPKOperationWarning, moveWorkshopFilesToAddons } from "./operations.js";
import {
  formatBatchGameStateConfirm,
  formatBatchGameStateSummary,
  canApplyBatchGameStateLocally,
  batchGameStateActionLabel,
} from "./batch-game-state-format.mjs";

/**
 * setSelectedGameEnabled 批量设置"游戏内开关"（addonlist.txt 的 0/1）。
 *
 * 与 enableSelected / disableSelected 的区别：那两个在 addons 与 disabled 目录之间搬文件，
 * 这个只改游戏读取 addonlist.txt 时用的开关，不动任何文件。
 */
export async function setSelectedGameEnabled(enabled) {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }
  const files = Array.from(appState.selectedFiles)
    .map(
      (filePath) =>
        appState.vpkFiles.find((file) => file.path === filePath) ||
        appState.allVpkFiles?.find((file) => file.path === filePath) ||
        null,
    )
    .filter(Boolean);
  const targets = files.filter((file) => file.location !== "disabled");
  const skipped = files.length - targets.length;
  if (targets.length === 0) {
    showNotification(t("mods.ops.gameState.noTargets"), "info");
    return;
  }
  const unrecorded = targets.filter((file) => !file.gameStateKnown).length;
  showConfirmModal(
    t("mods.batch.gameStateTitle", { action: batchGameStateActionLabel(enabled) }),
    formatBatchGameStateConfirm({ total: targets.length, skipped, unrecorded }, enabled),
    async () => {
      try {
        const result = await SetVPKGameEnabledBatch(
          targets.map((file) => file.path),
          enabled,
        );
        const updatedPaths = Array.isArray(result?.updated) ? result.updated : [];
        // 与单个开关同一套判据：能本地应用就不要全量重扫（真机 165–175ms 长帧）。
        const canApplyLocally = canApplyBatchGameStateLocally({
          targets,
          enforced: result?.enforced,
        });
        if (canApplyLocally) {
          const changed = new Set(updatedPaths);
          [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
            (files || []).forEach((file) => {
              if (!changed.has(file.path)) return;
              file.gameStateKnown = true;
              file.gameEnabled = enabled;
            });
          });
          // 当前按游戏内状态筛选时列表成员会变：本地重筛一次（performSearch 在
          // "只有筛选条件"这条路径上是纯本地的，不会碰后端扫描）。
          if ((appState.selectedGameStates?.length || 0) > 0) {
            await performSearch();
          } else {
            updatedPaths.forEach((path) => {
              const file =
                appState.allVpkFiles?.find((item) => item.path === path) ||
                appState.vpkFiles?.find((item) => item.path === path);
              if (file) updateSingleFileDisplay(file);
            });
            updateStatusBar();
          }
        } else {
          // 策略组联动 / 未记录条目进了 addonlist：顺序号与成员都可能有变，必须整表重扫。
          await refreshFilesKeepFilter();
        }
        showNotification(
          formatBatchGameStateSummary(result, enabled),
          updatedPaths.length > 0 ? "success" : "info",
        );
      } catch (error) {
        showError(t("mods.ops.gameState.batchFailed", { error: String(error?.message || error) }));
      }
    },
  );
}

export function selectAll() {
  // 只作用于当前列表里的文件；不要用 DOM 序号去索引文件数组 ——
  // 页面上还有别的 .file-checkbox（例如导出弹窗的两个选项），序号一对不上就会
  // 勾错文件、还把弹窗的选项一起改掉。按路径选择，再让 DOM 跟着状态走。
  appState.vpkFiles.forEach((file) => {
    if (file?.path) toggleFileSelection(file.path, true);
  });
  syncSelectedFiles();
  const lastVisibleFile = appState.vpkFiles[appState.vpkFiles.length - 1];
  appState.selectionAnchorPath = lastVisibleFile?.path || "";
}

export function deselectAll() {
  appState.selectedFiles.clear();
  appState.selectionAnchorPath = "";
  document.querySelectorAll("#file-list .file-checkbox").forEach((checkbox) => {
    checkbox.checked = false;
  });
  updateStatusBar();
}

export async function enableSelected() {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }

  const filesToToggle = Array.from(appState.selectedFiles).filter((filePath) => {
    // 只按当前筛选结果查，会在"选了 Mod 之后再改筛选"时把看不见的已选项静默丢掉：
    // 状态栏还写着 已选择: N，实际操作却只处理了筛选后仍然可见的那几个。
    // 选择集本身是跨筛选保留的，这里必须回退到完整列表。
    const file =
      appState.vpkFiles.find((f) => f.path === filePath) ||
      appState.allVpkFiles?.find((f) => f.path === filePath);
    return file && !file.enabled && file.location === "disabled";
  });

  if (filesToToggle.length === 0) {
    showNotification(t("mods.ops.enable.noTargets"), "info");
    return;
  }

  if (!(await confirmVPKOperationWarning(filesToToggle, t("mods.ops.label.batchEnable")))) return;

  try {
    // 失败的第一个原因要跟着计数一起说出来：只说"N 个失败"用户没法排查
    // （是不是被游戏占用、没权限、目标同名……）。
    const failures = [];
    const promises = filesToToggle.map(async (filePath) => {
      try {
        await ToggleVPKFile(filePath);
        return filePath;
      } catch (error) {
        failures.push(String(error?.message || error));
        console.error("启用文件失败:", filePath, error);
        return null;
      }
    });

    const results = await Promise.all(promises);
    const successFiles = results.filter((path) => path !== null);

    await batchUpdateFileStatus(successFiles);
    await refreshFilesKeepFilter();

    showNotification(t("mods.ops.enable.success", { count: successFiles.length }), "success");

    if (successFiles.length < filesToToggle.length) {
      const failedCount = filesToToggle.length - successFiles.length;
      const reason = failures[0] ? `：${failures[0]}` : "";
      showNotification(t("mods.ops.enable.failed", { count: failedCount, reason }), "error");
    }
  } catch (error) {
    console.error("批量启用失败:", error);
    showError(t("mods.ops.enable.batchFailed", { error: String(error) }));
  }
}

export async function disableSelected() {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }

  const filesToToggle = Array.from(appState.selectedFiles).filter((filePath) => {
    // 同上：批量操作的作用域是"整个选择"，不是"当前筛选可见的那几个"。
    const file =
      appState.vpkFiles.find((f) => f.path === filePath) ||
      appState.allVpkFiles?.find((f) => f.path === filePath);
    return file && file.enabled && file.location === "root";
  });

  if (filesToToggle.length === 0) {
    showNotification(t("mods.ops.disable.noTargets"), "info");
    return;
  }

  if (!(await confirmVPKOperationWarning(filesToToggle, t("mods.ops.label.batchDisable")))) return;

  try {
    const failures = [];
    const promises = filesToToggle.map(async (filePath) => {
      try {
        await ToggleVPKFile(filePath);
        return filePath;
      } catch (error) {
        failures.push(String(error?.message || error));
        console.error("禁用文件失败:", filePath, error);
        return null;
      }
    });

    const results = await Promise.all(promises);
    const successFiles = results.filter((path) => path !== null);

    await batchUpdateFileStatus(successFiles);
    await refreshFilesKeepFilter();

    showNotification(t("mods.ops.disable.success", { count: successFiles.length }), "success");

    if (successFiles.length < filesToToggle.length) {
      const failedCount = filesToToggle.length - successFiles.length;
      const reason = failures[0] ? `：${failures[0]}` : "";
      showNotification(t("mods.ops.disable.failed", { count: failedCount, reason }), "error");
    }
  } catch (error) {
    console.error("批量禁用失败:", error);
    showError(t("mods.ops.disable.batchFailed", { error: String(error) }));
  }
}

export async function disableAllMods(primaryTag = "") {
  const scopeLabel = primaryTag ? `${primaryTag}mod` : "mod";
  const filesToToggle = appState.allVpkFiles
    .filter((file) => {
      if (!file || !file.enabled || file.location !== "root") return false;
      return primaryTag ? file.primaryTag === primaryTag : true;
    })
    .map((file) => file.path);

  if (filesToToggle.length === 0) {
    showNotification(t("mods.ops.disableAll.noTargets", { scope: scopeLabel }), "info");
    return;
  }

  showConfirmModal(
    t("mods.ops.disableAll.confirmTitle", { scope: scopeLabel }),
    t("mods.ops.disableAll.confirmBody", { count: filesToToggle.length, scope: scopeLabel }),
    async () => {
      if (!(await confirmVPKOperationWarning(filesToToggle, t("mods.ops.label.disableScope", { scope: scopeLabel })))) return;
      updateLoadingMessage(t("mods.ops.disableAll.progress", { scope: scopeLabel }));
      showLoadingScreen();

      const successFiles = [];
      let failedCount = 0;
      const failures = [];

      try {
        for (const filePath of filesToToggle) {
          try {
            await ToggleVPKFile(filePath);
            successFiles.push(filePath);
          } catch (error) {
            failedCount++;
            failures.push(String(error?.message || error));
            console.error("禁用文件失败:", filePath, error);
          }
        }

        await batchUpdateFileStatus(successFiles);
        await refreshFilesKeepFilter();

        if (successFiles.length > 0) {
          showNotification(t("mods.ops.disableAll.success", { count: successFiles.length, scope: scopeLabel }), "success");
        }

        if (failedCount > 0) {
          const reason = failures[0] ? `：${failures[0]}` : "";
          showNotification(t("mods.ops.disableAll.failed", { count: failedCount, scope: scopeLabel, reason }), "error");
        }
      } catch (error) {
        console.error(`批量禁用${scopeLabel}失败:`, error);
        showError(t("mods.ops.disableAll.batchFailed", { scope: scopeLabel, error: String(error) }));
      } finally {
        showMainScreen();
      }
    }
  );
}

export async function exportZipSelected() {
  const selectedFiles = Array.from(appState.selectedFiles);
  if (selectedFiles.length === 0) {
    showError(t("mods.ops.export.noSelection"));
    return;
  }

  const confirmMessage = `
    <div style="display: flex; flex-direction: column; gap: 12px;">
      <p>${t("mods.ops.export.selectedCount", { count: selectedFiles.length })}</p>
      <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
        <input type="checkbox" id="export-include-extra" class="file-checkbox" checked>
        <span>${t("mods.ops.export.includeExtra")}</span>
      </label>
      <label style="display: flex; align-items: center; gap: 8px; cursor: pointer;">
        <input type="checkbox" id="export-rename-to-title" class="file-checkbox">
        <span>${t("mods.ops.export.renameToTitle")}</span>
      </label>
    </div>
  `;

  showConfirmModal(
    t("mods.batch.exportZip"),
    confirmMessage,
    async () => {
      document.getElementById("confirm-modal").classList.add("hidden");

      const includeExtra = document.getElementById("export-include-extra").checked;
      const renameToTitle = document.getElementById("export-rename-to-title").checked;

      const cleanup = EventsOn("export-progress", (progress) => {
        updateLoadingMessage(`${progress.message} (${progress.current}/${progress.total})`);
      });

      showLoadingScreen();
      updateLoadingMessage(t("mods.ops.export.preparing"));

      try {
        const result = await ExportVPKFilesToZip(
          selectedFiles,
          includeExtra,
          renameToTitle
        );
        if (result === "cancelled") return;
        showSuccess(result);
      } catch (error) {
        console.error("导出ZIP失败:", error);
        showError(t("mods.ops.export.failed", { error: String(error) }));
      } finally {
        showMainScreen();
        if (typeof cleanup === "function") cleanup();
      }
    },
    true
  );
}

export async function deleteSelected() {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }

  showConfirmModal(
    t("mods.ops.delete.confirmBatchTitle"),
    t("mods.ops.delete.confirmBatchBody", { count: appState.selectedFiles.size }),
    async () => {
      const filesToDelete = Array.from(appState.selectedFiles);

      if (!(await confirmVPKOperationWarning(filesToDelete, t("mods.ops.label.batchDelete")))) return;

      try {
        console.log(`批量删除 ${filesToDelete.length} 个文件...`);
        await DeleteVPKFiles(filesToDelete);
        filesToDelete.forEach((filePath) => appState.selectedFiles.delete(filePath));
        await refreshFilesKeepFilter();
        showNotification(t("mods.ops.delete.success", { count: filesToDelete.length }), "success");
      } catch (error) {
        console.error("批量删除失败:", error);
        showError(t("mods.ops.delete.batchFailed", { error: String(error) }));
      }
    }
  );
}

export async function moveSelected() {
  if (appState.selectedFiles.size === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }
  await movePathsToDestination(Array.from(appState.selectedFiles));
}

/**
 * movePathsToDestination 把给定的一批 Mod 移到用户选择的目录（"移动到…"的公共实现）。
 * 现有的"批量 → 移动到…"按钮与 Ctrl+X/Ctrl+V 都走这里，保证两条入口行为一致。
 */
export async function movePathsToDestination(filesToMove) {
  if (!Array.isArray(filesToMove) || filesToMove.length === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }
  try {
    const destDir = await SelectDirectory();
    if (!destDir) return;

    if (!(await confirmVPKOperationWarning(filesToMove, t("mods.ops.label.move")))) return;

    showNotification(t("mods.ops.move.progress"), "info");

    const result = await moveVpkFilesWithConflictResolution(filesToMove, destDir);

    if (result.successCount > 0) {
      showSuccess(t("mods.ops.move.success", { count: result.successCount }));
      result.successPaths?.forEach((filePath) => appState.selectedFiles.delete(filePath));
    }

    if (result.failCount > 0) {
      // 逐项失败原因都带上（对齐 FireAxe 的 FailedImportResultItem）：只说第一条会让人以为只有它失败。
      showError(formatMoveFailures(result));
      console.error("移动失败详情:", result.errors);
    }

    if (result.skippedCount > 0) {
      showNotification(t("mods.ops.move.skipped", { count: result.skippedCount }), "info");
    }

    if (result.cancelled) {
      showNotification(t("mods.ops.move.cancelled"), "info");
    }

    await refreshFilesKeepFilter();
    updateStatusBar();
  } catch (error) {
    console.error("移动文件出错:", error);
    showError(t("mods.ops.move.error", { error: String(error) }));
  }
}

/**
 * cutSelected 标记选中的 Mod 为"待移动"（对齐 FireAxe v0.5.1 的 Ctrl+X）。
 * 与文件管理器一样：标记会跨选择变化保留，Ctrl+V 时才真正选目标目录并移动。
 */
export function cutSelected() {
  const paths = Array.from(appState.selectedFiles || []);
  if (paths.length === 0) {
    showNotification(t("mods.ops.move.noSelection"), "info");
    return;
  }
  appState.moveClipboard = new Set(paths);
  updateStatusBar();
  showNotification(
    t("mods.ops.move.cutMarked", { count: paths.length }),
    "success",
  );
}

/** pasteMoveClipboard 执行"粘贴"：把待移动的 Mod 移到用户选择的目录（对齐 Ctrl+V）。 */
export async function pasteMoveClipboard() {
  const pending = Array.from(appState.moveClipboard || []);
  if (pending.length === 0) {
    showNotification(t("mods.ops.move.noPending"), "info");
    return;
  }
  await movePathsToDestination(pending);
  // 移动流程结束（无论成没成）都清掉标记：失败项会由逐项错误提示说明。
  clearMoveClipboard();
}

/** clearMoveClipboard 取消"待移动"标记（Esc 或粘贴完成后调用）。 */
export function clearMoveClipboard() {
  if (!appState.moveClipboard || appState.moveClipboard.size === 0) return;
  appState.moveClipboard = new Set();
  updateStatusBar();
}

export function transferSelectedWorkshopFiles() {
  const selectedPaths = Array.from(appState.selectedFiles);
  if (selectedPaths.length === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }

  const selectedFiles = selectedPaths
    .map(
      (filePath) =>
        appState.vpkFiles.find((file) => file.path === filePath) ||
        appState.allVpkFiles.find((file) => file.path === filePath),
    )
    .filter(Boolean);
  const eligibleCount = selectedFiles.filter(
    (file) => file.location === "workshop",
  ).length;
  const skippedCount = selectedPaths.length - eligibleCount;

  if (eligibleCount === 0) {
    showNotification(t("mods.ops.common.noWorkshopFiles"), "info");
    return;
  }

  const skippedText = skippedCount > 0 ? t("mods.ops.transfer.skippedSuffix", { count: skippedCount }) : "";
  showConfirmModal(
    t("mods.ops.transfer.confirmTitle"),
    t("mods.ops.transfer.confirmBody", { count: eligibleCount, skipped: skippedText }),
    async () => {
      document.getElementById("confirm-modal")?.classList.add("hidden");
      return moveWorkshopFilesToAddons(
        selectedFiles.filter((file) => file.location === "workshop").map((file) => file.path),
      );
    },
  );
}

/**
 * generateWorkshopShortcutsForPaths 为一批 Mod 生成工坊快捷方式（.url）。
 *
 * ID 解析（.meta → workshop 纯数字文件名 → addonURL0）与校验都在后端
 * GenerateWorkshopShortcuts 里做；这里只负责发起调用与结果提示。
 * 无法确定 ID 的文件由后端逐项跳过，不猜测、不写文件；写入失败逐条计数。
 */
export async function generateWorkshopShortcutsForPaths(filePaths) {
  const paths = Array.from(new Set((filePaths || []).filter(Boolean)));
  if (paths.length === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }
  try {
    const result = await GenerateWorkshopShortcuts(paths);
    const created = Number(result?.created || 0);
    const updated = Number(result?.updated || 0);
    const unchanged = Number(result?.unchanged || 0);
    const skipped = Array.isArray(result?.skipped) ? result.skipped.length : 0;
    const writeErrors = Array.isArray(result?.errors) ? result.errors.length : 0;
    if (created + updated + unchanged === 0) {
      showNotification(t("mods.ops.shortcut.allSkipped", { count: skipped }), "info");
    } else {
      const skippedText = skipped > 0 ? t("mods.ops.shortcut.skippedSuffix", { count: skipped }) : "";
      showNotification(
        t("mods.ops.shortcut.done", { created, updated, unchanged, skipped: skippedText }),
        "success",
      );
    }
    if (writeErrors > 0) {
      showError(t("mods.ops.shortcut.writeErrors", { count: writeErrors }));
    }
  } catch (err) {
    console.error("生成工坊快捷方式失败:", err);
    showError(t("mods.ops.shortcut.failed", { error: String(err) }));
  }
}

export async function batchToggleVisibility(hide) {
  const selectedFiles = Array.from(appState.selectedFiles);
  if (selectedFiles.length === 0) {
    showNotification(t("mods.batch.selectFilesFirst"), "info");
    return;
  }

  const actionName = hide ? t("mods.ops.common.actionUnhide") : t("mods.ops.common.actionHide");

  showConfirmModal(
    hide ? t("mods.batch.unhide") : t("mods.batch.hide"),
    t("mods.ops.visibility.confirmBody", { action: actionName, count: selectedFiles.length }),
    async () => {
      if (!(await confirmVPKOperationWarning(selectedFiles, hide ? t("mods.ops.label.batchUnhide") : t("mods.ops.label.batchHide")))) return;
      updateLoadingMessage(t("mods.ops.visibility.progress", { action: actionName }));
      showLoadingScreen();

      let successCount = 0;
      let skippedCount = 0;
      let failCount = 0;
      const failures = [];

      for (const filePath of selectedFiles) {
        try {
          const fileName = filePath.split(/[\\/]/).pop();
          const isHidden = fileName.startsWith("_");

          if ((!hide && !isHidden) || (hide && isHidden)) {
            await ToggleVPKVisibility(filePath);
            successCount++;
          } else {
            skippedCount++;
          }
        } catch (err) {
          console.error(`处理文件 ${filePath} 失败:`, err);
          failCount++;
          failures.push(String(err?.message || err));
        }
      }

      await refreshFilesKeepFilter();
      showMainScreen();

      if (failCount > 0 || skippedCount > 0) {
        const skippedText = skippedCount > 0 ? t("mods.ops.visibility.skipped", { count: skippedCount }) : "";
        // 同批量启用/禁用：失败时把第一个原因一起说出来，否则用户只知道"有失败"。
        const reason = failCount > 0 && failures[0] ? `：${failures[0]}` : "";
        showNotification(
          t("mods.ops.visibility.done", { success: successCount, fail: failCount, reason, skipped: skippedText }),
          "warning",
        );
      } else {
        showNotification(t("mods.ops.visibility.success", { action: actionName, count: successCount }), "success");
      }

      deselectAll();
    }
  );
}

export async function batchUpdateFileStatus(filePaths) {
  if (!filePaths || filePaths.length === 0) return;

  try {
    const updatedFiles = await GetVPKFiles();
    const updatedFileMap = new Map(updatedFiles.map((f) => [f.path, f]));

    filePaths.forEach((filePath) => {
      const updatedFile = updatedFileMap.get(filePath);
      if (updatedFile) {
        const allFileIndex = appState.allVpkFiles.findIndex((f) => f.path === filePath);
        if (allFileIndex >= 0) {
          appState.allVpkFiles[allFileIndex] = updatedFile;
        }

        const displayFileIndex = appState.vpkFiles.findIndex((f) => f.path === filePath);
        if (displayFileIndex >= 0) {
          appState.vpkFiles[displayFileIndex] = updatedFile;
          updateSingleFileDisplay(updatedFile);
        }
      }
    });

    updateStatusBar();
    syncSelectedFiles();
  } catch (error) {
    console.error("批量更新文件状态失败:", error);
    await refreshFilesKeepFilter();
  }
}

export function syncSelectedFiles() {
  // 与框选那份实现保持同一种做法：按行上的 data-path 判断，不用序号。
  document
    .querySelectorAll("#file-list .file-item, #file-list .file-card")
    .forEach((item) => {
      const checkbox = item.querySelector(".file-checkbox");
      if (checkbox && item.dataset.path) {
        checkbox.checked = appState.selectedFiles.has(item.dataset.path);
      }
    });
}
