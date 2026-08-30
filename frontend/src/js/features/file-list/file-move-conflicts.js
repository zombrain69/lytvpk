import {
  CheckFileMoveConflicts,
  GetRootDirectory,
  MoveVpkFilesWithConflictAction,
  MoveWorkshopToAddonsWithConflictAction,
} from "../../../../wailsjs/go/app/App";

function formatBytes(value) {
  const bytes = Number(value);
  if (!Number.isFinite(bytes) || bytes < 0) return "未知";
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KB", "MB", "GB", "TB"];
  let size = bytes;
  let unit = -1;
  do {
    size /= 1024;
    unit += 1;
  } while (size >= 1024 && unit < units.length - 1);
  return `${size.toFixed(size >= 10 ? 1 : 2)} ${units[unit]}`;
}

function formatTime(value) {
  if (!value) return "未知";
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? String(value) : date.toLocaleString();
}

function createConflictEntry(conflict) {
  const entry = document.createElement("div");
  entry.className = "file-conflict-entry";

  const title = document.createElement("div");
  title.className = "file-conflict-entry-title";
  title.textContent = conflict.targetPath || "目标文件";
  const kind = document.createElement("span");
  kind.className = "file-conflict-entry-kind";
  kind.textContent = conflict.fileType || "文件";
  title.appendChild(kind);
  entry.appendChild(title);

  const details = document.createElement("div");
  details.className = "file-conflict-entry-details";
  const source = document.createElement("div");
  const sourceLabel = document.createElement("strong");
  sourceLabel.textContent = "源文件";
  source.appendChild(sourceLabel);
  const sourcePath = document.createElement("div");
  sourcePath.className = "file-conflict-entry-path";
  sourcePath.textContent = conflict.sourcePath || "未知";
  source.appendChild(sourcePath);
  const sourceInfo = document.createElement("div");
  sourceInfo.textContent = `大小：${formatBytes(conflict.sourceSize)} · 修改时间：${formatTime(conflict.sourceModTime)}`;
  source.appendChild(sourceInfo);

  const target = document.createElement("div");
  const targetLabel = document.createElement("strong");
  targetLabel.textContent = "目标文件";
  target.appendChild(targetLabel);
  const targetPath = document.createElement("div");
  targetPath.className = "file-conflict-entry-path";
  targetPath.textContent = conflict.targetPath || "未知";
  target.appendChild(targetPath);
  const targetInfo = document.createElement("div");
  targetInfo.textContent = `大小：${formatBytes(conflict.targetSize)} · 修改时间：${formatTime(conflict.targetModTime)}`;
  target.appendChild(targetInfo);

  details.append(source, target);
  entry.appendChild(details);
  return entry;
}

export function showFileConflictDialog(conflicts) {
  const modal = document.getElementById("file-conflict-modal");
  const summary = document.getElementById("file-conflict-summary");
  const list = document.getElementById("file-conflict-list");
  const compareBtn = document.getElementById("file-conflict-compare-btn");
  const applyAll = document.getElementById("file-conflict-apply-all");
  const cancelBtn = document.getElementById("file-conflict-cancel-btn");
  const skipBtn = document.getElementById("file-conflict-skip-btn");
  const replaceBtn = document.getElementById("file-conflict-replace-btn");
  const closeBtn = document.getElementById("close-file-conflict-modal-btn");

  if (!modal || !summary || !list || !compareBtn || !applyAll || !cancelBtn || !skipBtn || !replaceBtn || !closeBtn) {
    return Promise.resolve({ action: "cancel", applyToAll: false });
  }

  const entries = Array.isArray(conflicts) ? conflicts : [];
  list.replaceChildren(...entries.map(createConflictEntry));
  list.classList.remove("is-compared");
  compareBtn.textContent = "比较文件信息";
  applyAll.checked = false;
  summary.textContent = `发现 ${entries.length} 个目标文件已存在。请选择如何处理；默认只处理当前冲突。`;
  modal.classList.remove("hidden");

  return new Promise((resolve) => {
    let settled = false;
    const cleanup = (action) => {
      if (settled) return;
      settled = true;
      modal.classList.add("hidden");
      compareBtn.onclick = null;
      cancelBtn.onclick = null;
      skipBtn.onclick = null;
      replaceBtn.onclick = null;
      closeBtn.onclick = null;
      resolve({ action, applyToAll: Boolean(applyAll.checked) });
    };

    compareBtn.onclick = () => {
      const compared = list.classList.toggle("is-compared");
      compareBtn.textContent = compared ? "收起文件信息" : "比较文件信息";
    };
    cancelBtn.onclick = () => cleanup("cancel");
    closeBtn.onclick = () => cleanup("cancel");
    skipBtn.onclick = () => cleanup("skip");
    replaceBtn.onclick = () => cleanup("replace");
  });
}

function mergeMoveResult(total, result) {
  if (!result) return;
  total.successCount += Number(result.successCount) || 0;
  total.failCount += Number(result.failCount) || 0;
  total.skippedCount += Number(result.skippedCount) || 0;
  total.cancelled = total.cancelled || Boolean(result.cancelled);
  if (Array.isArray(result.errors)) total.errors.push(...result.errors);
}

async function moveWithConflictResolution(filePaths, destDir, executor) {
  const total = { successCount: 0, failCount: 0, skippedCount: 0, cancelled: false, errors: [], successPaths: [] };
  let rememberedAction = "";

  for (const filePath of filePaths) {
    let action = rememberedAction;
    let conflicts = [];
    try {
      conflicts = await CheckFileMoveConflicts([filePath], destDir);
    } catch (error) {
      total.failCount += 1;
      total.errors.push(`检查 ${filePath} 冲突失败: ${String(error)}`);
      continue;
    }

    if (conflicts.length > 0 && !action) {
      const decision = await showFileConflictDialog(conflicts);
      action = decision.action;
      if (decision.applyToAll && action !== "cancel") rememberedAction = action;
    }

    if (action === "cancel") {
      total.cancelled = true;
      break;
    }

    try {
      const result = await executor(filePath, action);
      mergeMoveResult(total, result);
      if (Number(result?.successCount) > 0) total.successPaths.push(filePath);
      if (total.cancelled) break;
    } catch (error) {
      total.failCount += 1;
      total.errors.push(`移动 ${filePath} 失败: ${String(error)}`);
    }
  }
  return total;
}

export async function moveVpkFilesWithConflictResolution(filePaths, destDir) {
  return moveWithConflictResolution(filePaths, destDir, (filePath, action) =>
    MoveVpkFilesWithConflictAction([filePath], destDir, action),
  );
}

export async function moveWorkshopFileWithConflictResolution(filePath) {
  const destDir = await GetRootDirectory();
  if (!destDir) throw new Error("未选择 L4D2 addons 目录");
  return moveWithConflictResolution([filePath], destDir, (path, action) =>
    MoveWorkshopToAddonsWithConflictAction(path, action),
  );
}
