import { showError, showNotification } from "../../core/toast.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let integrityRunning = false;

export async function openVPKIntegrityTool() {
  if (integrityRunning) {
    showNotification("已有 VPK 正在检测", "info");
    return;
  }

  integrityRunning = true;
  try {
    const vpkPath = await callApp("SelectVPKFile");
    if (!vpkPath) return;
    await inspectVPKIntegrityPaths([vpkPath]);
  } catch (error) {
    showError("选择 VPK 失败: " + formatError(error));
  } finally {
    integrityRunning = false;
  }
}

// Opens the same integrity workflow for one or multiple selected VPK files.
// A single file keeps the detailed report; multiple files get a compact
// per-file summary and one confirmation before all repairable files are copied.
export async function openVPKIntegrityForPaths(filePaths = []) {
  if (integrityRunning) {
    showNotification("已有 VPK 正在检测或修复", "info");
    return;
  }

  const paths = normalizePaths(filePaths);
  if (paths.length === 0) {
    showNotification("请先选择 VPK 文件", "info");
    return;
  }

  integrityRunning = true;
  try {
    await inspectVPKIntegrityPaths(paths);
  } finally {
    integrityRunning = false;
  }
}

async function inspectVPKIntegrityPaths(paths) {
  showNotification(
    paths.length === 1
      ? "正在检查 VPK 文件和 addoninfo.txt..."
      : `正在检查 ${paths.length} 个 VPK 文件...`,
    "info",
  );
  try {
    if (paths.length === 1) {
      const report = await callApp("InspectVPKIntegrity", paths[0]);
      showIntegrityReport(report);
      showNotification(
        report?.valid ? "VPK 检测通过" : "VPK 检测发现问题",
        report?.valid ? "success" : "info",
      );
    } else {
      const results = await callApp("InspectVPKIntegrityBatch", paths);
      showIntegrityBatchReport(results);
      const validCount = results.filter((item) => item?.report?.valid).length;
      showNotification(`批量检测完成：${validCount} / ${results.length} 个通过`, "info");
    }
  } catch (error) {
    showError("VPK 检测失败: " + formatError(error));
  }
}

function showIntegrityReport(report = {}) {
  const session = beginMessageModalSession();
  if (!session) return;

  const repairBtn = document.createElement("button");
  repairBtn.type = "button";
  repairBtn.className = "btn btn-primary";
  repairBtn.textContent = "修复并另存为";
  repairBtn.hidden = !report.repairable;
  session.addClass(
    session.modal.querySelector(".modal-content"),
    "vpk-integrity-modal-content",
  );

  session.titleEl.textContent = report.valid ? "VPK 检测通过" : "VPK 检测结果";
  session.contentEl.replaceChildren(createIntegrityContent(report));
  session.confirmBtn.textContent = "关闭";
  session.confirmBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  if (report.repairable) {
    session.addActionButton(repairBtn);
    repairBtn.onclick = async () => {
      integrityRunning = true;
      session.setPending(true);
      repairBtn.textContent = "修复中...";
      try {
        const result = await callApp("RepairVPKIntegrity", report.path);
        if (session.isCurrent()) {
          session.close("repair");
          showRepairResult(result);
        }
        showNotification("VPK 已修复并生成新文件，原文件未改动", "success");
      } catch (error) {
        if (session.isCurrent()) {
          session.setPending(false);
          repairBtn.textContent = "修复并另存为";
        }
        showError("VPK 修复失败: " + formatError(error));
      } finally {
        integrityRunning = false;
      }
    };
  }
  session.show();
}

function showIntegrityBatchReport(results = []) {
  const session = beginMessageModalSession();
  if (!session) return;

  const safeResults = Array.isArray(results) ? results : [];
  const repairPaths = safeResults
    .filter((item) => !item?.error && item?.report?.repairable && item?.path)
    .map((item) => item.path);
  const repairBtn = document.createElement("button");
  repairBtn.type = "button";
  repairBtn.className = "btn btn-primary";
  repairBtn.textContent = `修复可修复项 (${repairPaths.length})`;
  repairBtn.hidden = repairPaths.length === 0;
  session.addClass(
    session.modal.querySelector(".modal-content"),
    "vpk-integrity-modal-content",
  );

  session.titleEl.textContent = `VPK 批量检测结果 (${safeResults.length})`;
  session.contentEl.replaceChildren(createBatchIntegrityContent(safeResults));
  session.confirmBtn.textContent = "关闭";
  session.confirmBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  if (repairPaths.length > 0) {
    session.addActionButton(repairBtn);
    repairBtn.onclick = async () => {
      integrityRunning = true;
      session.setPending(true);
      repairBtn.textContent = "批量修复中...";
      try {
        const repaired = await callApp("RepairVPKIntegrityBatch", repairPaths);
        if (session.isCurrent()) {
          session.close("repair");
          showRepairBatchResult(repaired);
        }
        const successCount = repaired.filter((item) => item?.outputPath && !item?.error).length;
        showNotification(`批量修复完成：成功生成 ${successCount} 个 VPK`, "success");
      } catch (error) {
        if (session.isCurrent()) {
          session.setPending(false);
          repairBtn.textContent = `修复可修复项 (${repairPaths.length})`;
        }
        showError("批量 VPK 修复失败: " + formatError(error));
      } finally {
        integrityRunning = false;
      }
    };
  }
  session.show();
}

function showRepairResult(result = {}) {
  showRepairBatchResult([result], true);
}

function showRepairBatchResult(results = [], single = false) {
  const session = beginMessageModalSession();
  if (!session) return;

  const safeResults = Array.isArray(results) ? results : [];
  const outputPaths = safeResults.filter((item) => item?.outputPath).map((item) => item.outputPath);
  session.titleEl.textContent = single ? "VPK 修复完成" : "VPK 批量修复结果";
  session.contentEl.replaceChildren(createRepairBatchContent(safeResults));
  session.confirmBtn.textContent = outputPaths.length > 0 ? "打开输出位置" : "关闭";
  session.addClass(
    session.modal.querySelector(".modal-content"),
    "vpk-integrity-modal-content",
  );
  session.closeBtn.onclick = () => session.close("close");
  session.confirmBtn.onclick = async () => {
    if (outputPaths.length === 0) {
      session.close("close");
      return;
    }
    session.close("confirm");
    try {
      await callApp("OpenFileLocation", outputPaths[0]);
    } catch (error) {
      showError("打开修复文件位置失败: " + formatError(error));
    }
  };
  session.show();
}

function createIntegrityContent(report = {}) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-integrity-report";

  const status = document.createElement("div");
  status.className = report.valid ? "vpk-integrity-status is-valid" : "vpk-integrity-status is-invalid";
  status.textContent = report.valid
    ? "未发现问题"
    : report.repairable
      ? "发现可自动修复的问题"
      : "发现需要重新下载或手动处理的问题";

  wrapper.append(
    status,
    createPathBlock("文件", report.path || ""),
    createIntegrityStats(report),
  );
  appendIssueList(wrapper, report.issues);
  return wrapper;
}

function createBatchIntegrityContent(results = []) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-integrity-report vpk-integrity-batch-report";
  const validCount = results.filter((item) => item?.report?.valid).length;
  const repairableCount = results.filter((item) => item?.report?.repairable && !item?.error).length;
  const summary = document.createElement("div");
  summary.className = "vpk-integrity-batch-summary";
  summary.textContent = `共检测 ${results.length} 个 VPK：${validCount} 个通过，${repairableCount} 个可修复。`;
  wrapper.appendChild(summary);

  const list = document.createElement("div");
  list.className = "vpk-integrity-batch-list";
  results.forEach((item) => {
    const report = item?.report || {};
    const card = document.createElement("section");
    card.className = "vpk-integrity-batch-item";
    const header = document.createElement("div");
    header.className = "vpk-integrity-batch-item-header";
    const name = document.createElement("strong");
    name.textContent = basename(item?.path || report.path || "未知 VPK");
    name.title = item?.path || report.path || "";
    const badge = document.createElement("span");
    badge.className = report.valid ? "is-valid" : report.repairable ? "is-repairable" : "is-invalid";
    badge.textContent = item?.error
      ? "检测失败"
      : report.valid
        ? "通过"
        : report.repairable
          ? "可修复"
          : "需手动处理";
    header.append(name, badge);
    card.append(header, createPathBlock("路径", item?.path || report.path || ""));
    if (item?.error) {
      const error = document.createElement("p");
      error.className = "vpk-integrity-batch-error";
      error.textContent = item.error;
      card.appendChild(error);
    } else {
      card.appendChild(createIntegrityStats(report));
      appendIssueList(card, report.issues, true);
    }
    list.appendChild(card);
  });
  wrapper.appendChild(list);
  return wrapper;
}

function createRepairBatchContent(results = []) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-integrity-report vpk-integrity-batch-report";
  const success = results.filter((item) => item?.outputPath && !item?.error).length;
  const failed = results.length - success;
  const summary = document.createElement("div");
  summary.className = "vpk-integrity-batch-summary";
  summary.textContent = `成功生成 ${success} 个修复文件${failed > 0 ? `，${failed} 个未能修复` : ""}。原文件均未覆盖。`;
  wrapper.appendChild(summary);

  const list = document.createElement("div");
  list.className = "vpk-integrity-batch-list";
  results.forEach((item) => {
    const card = document.createElement("section");
    card.className = "vpk-integrity-batch-item";
    const source = document.createElement("strong");
    source.textContent = basename(item?.sourcePath || "未知 VPK");
    source.title = item?.sourcePath || "";
    card.appendChild(source);
    if (item?.error) {
      const error = document.createElement("p");
      error.className = "vpk-integrity-batch-error";
      error.textContent = item.error;
      card.appendChild(error);
    } else {
      card.appendChild(createPathBlock("修复文件", item.outputPath || ""));
      const metadata = createRepairMetadataSummary(item.addonInfoRepair);
      if (metadata) card.appendChild(metadata);
    }
    list.appendChild(card);
  });
  wrapper.appendChild(list);
  return wrapper;
}

function createRepairMetadataSummary(summary = {}) {
  const preserved = Array.isArray(summary?.preservedFields) ? summary.preservedFields : [];
  const derived = Array.isArray(summary?.derivedFields) ? summary.derivedFields : [];
  const recovered = Boolean(summary?.recoveredTruncatedText);
  if (preserved.length === 0 && derived.length === 0 && !recovered) return null;

  const note = document.createElement("p");
  note.className = "vpk-integrity-repair-meta";
  const parts = [];
  if (preserved.length > 0) {
    parts.push(`保留 ${preserved.length} 项原有元信息`);
    note.title = `已保留：${preserved.join("、")}`;
  }
  if (derived.length > 0) parts.push(`补全：${derived.join("、")}`);
  if (recovered) parts.push("已恢复截断文本");
  note.textContent = parts.join("；") + "。";
  return note;
}

function createIntegrityStats(report = {}) {
  const stats = document.createElement("p");
  stats.className = "vpk-integrity-stats";
  stats.textContent = `目录文件 ${Number(report.totalFiles || 0)} 个，已校验 ${Number(report.verifiedFiles || 0)} 个；addoninfo.txt ${report.addonInfoFound ? (report.addonInfoValid ? "有效" : "无效") : "缺失"}。`;
  return stats;
}

function appendIssueList(wrapper, issues, compact = false) {
  const safeIssues = Array.isArray(issues) ? issues : [];
  if (safeIssues.length === 0) return;
  const heading = document.createElement("h4");
  heading.textContent = compact ? `问题 (${safeIssues.length})` : "检测到的问题";
  const list = document.createElement("ul");
  list.className = "vpk-integrity-issues";
  safeIssues.forEach((issue) => {
    const item = document.createElement("li");
    item.className = issue.severity === "error" ? "is-error" : "is-warning";
    const message = document.createElement("span");
    message.textContent = issue.path ? `${issue.path}: ${issue.message}` : issue.message;
    item.appendChild(message);
    if (issue.repairable) {
      const badge = document.createElement("small");
      badge.textContent = "可修复";
      item.appendChild(badge);
    }
    list.appendChild(item);
  });
  wrapper.append(heading, list);
}

function createPathBlock(label, value) {
  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-integrity-path";
  const pathLabel = document.createElement("span");
  pathLabel.textContent = label;
  const pathValue = document.createElement("code");
  pathValue.textContent = value;
  pathValue.title = value;
  pathBlock.append(pathLabel, pathValue);
  return pathBlock;
}

function normalizePaths(filePaths) {
  const seen = new Set();
  return (Array.isArray(filePaths) ? filePaths : [filePaths])
    .map((filePath) => String(filePath || "").trim())
    .filter((filePath) => {
      const key = filePath.toLowerCase();
      if (!filePath || seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function basename(filePath) {
  return String(filePath || "").split(/[\\/]/).pop() || filePath || "";
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") {
    return Promise.reject(new Error(`当前应用不支持 ${methodName}，请重新构建并启动 LytVPK`));
  }
  return method(...args);
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || "未知错误");
}
