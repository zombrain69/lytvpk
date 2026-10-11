import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let integrityRunning = false;

export async function openVPKIntegrityTool() {
  if (integrityRunning) {
    showNotification(t("toolbox.vpkIntegrity.busy"), "info");
    return;
  }

  integrityRunning = true;
  try {
    const vpkPath = await callApp("SelectVPKFile");
    if (!vpkPath) return;
    await inspectVPKIntegrityPaths([vpkPath]);
  } catch (error) {
    showError(t("toolbox.common.selectVpkFailed", { error: formatError(error) }));
  } finally {
    integrityRunning = false;
  }
}

// Opens the same integrity workflow for one or multiple selected VPK files.
// A single file keeps the detailed report; multiple files get a compact
// per-file summary and one confirmation before all repairable files are copied.
export async function openVPKIntegrityForPaths(filePaths = []) {
  if (integrityRunning) {
    showNotification(t("toolbox.vpkIntegrity.busyRepair"), "info");
    return;
  }

  const paths = normalizePaths(filePaths);
  if (paths.length === 0) {
    showNotification(t("toolbox.vpkIntegrity.selectFirst"), "info");
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
      ? t("toolbox.vpkIntegrity.checkingSingle")
      : t("toolbox.vpkIntegrity.checkingCount", { count: paths.length }),
    "info",
  );
  try {
    if (paths.length === 1) {
      const report = await callApp("InspectVPKIntegrity", paths[0]);
      showIntegrityReport(report);
      showNotification(
        report?.valid ? t("toolbox.vpkIntegrity.pass") : t("toolbox.vpkIntegrity.issuesFound"),
        report?.valid ? "success" : "info",
      );
    } else {
      const results = await callApp("InspectVPKIntegrityBatch", paths);
      showIntegrityBatchReport(results);
      const validCount = results.filter((item) => item?.report?.valid).length;
      showNotification(t("toolbox.vpkIntegrity.batchDone", { valid: validCount, total: results.length }), "info");
    }
  } catch (error) {
    showError(t("toolbox.vpkIntegrity.checkFailed", { error: formatError(error) }));
  }
}

function showIntegrityReport(report = {}) {
  const session = beginMessageModalSession();
  if (!session) return;

  const repairBtn = document.createElement("button");
  repairBtn.type = "button";
  repairBtn.className = "btn btn-primary";
  repairBtn.textContent = t("toolbox.vpkIntegrity.repairSaveAs");
  repairBtn.hidden = !report.repairable;
  session.addClass(
    session.modal.querySelector(".modal-content"),
    "vpk-integrity-modal-content",
  );

  session.titleEl.textContent = report.valid ? t("toolbox.vpkIntegrity.pass") : t("toolbox.vpkIntegrity.reportTitle");
  session.contentEl.replaceChildren(createIntegrityContent(report));
  session.confirmBtn.textContent = t("toolbox.common.close");
  session.confirmBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  if (report.repairable) {
    session.addActionButton(repairBtn);
    repairBtn.onclick = async () => {
      integrityRunning = true;
      session.setPending(true);
      repairBtn.textContent = t("toolbox.vpkIntegrity.repairing");
      try {
        const result = await callApp("RepairVPKIntegrity", report.path);
        if (session.isCurrent()) {
          session.close("repair");
          showRepairResult(result);
        }
        showNotification(t("toolbox.vpkIntegrity.repaired"), "success");
      } catch (error) {
        if (session.isCurrent()) {
          session.setPending(false);
          repairBtn.textContent = t("toolbox.vpkIntegrity.repairSaveAs");
        }
        showError(t("toolbox.vpkIntegrity.repairFailed", { error: formatError(error) }));
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
  repairBtn.textContent = t("toolbox.vpkIntegrity.repairRepairable", { count: repairPaths.length });
  repairBtn.hidden = repairPaths.length === 0;
  session.addClass(
    session.modal.querySelector(".modal-content"),
    "vpk-integrity-modal-content",
  );

  session.titleEl.textContent = t("toolbox.vpkIntegrity.batchReportTitle", { count: safeResults.length });
  session.contentEl.replaceChildren(createBatchIntegrityContent(safeResults));
  session.confirmBtn.textContent = t("toolbox.common.close");
  session.confirmBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  if (repairPaths.length > 0) {
    session.addActionButton(repairBtn);
    repairBtn.onclick = async () => {
      integrityRunning = true;
      session.setPending(true);
      repairBtn.textContent = t("toolbox.vpkIntegrity.batchRepairing");
      try {
        const repaired = await callApp("RepairVPKIntegrityBatch", repairPaths);
        if (session.isCurrent()) {
          session.close("repair");
          showRepairBatchResult(repaired);
        }
        const successCount = repaired.filter((item) => item?.outputPath && !item?.error).length;
        showNotification(t("toolbox.vpkIntegrity.batchRepaired", { count: successCount }), "success");
      } catch (error) {
        if (session.isCurrent()) {
          session.setPending(false);
          repairBtn.textContent = t("toolbox.vpkIntegrity.repairRepairable", { count: repairPaths.length });
        }
        showError(t("toolbox.vpkIntegrity.batchRepairFailed", { error: formatError(error) }));
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
  session.titleEl.textContent = single ? t("toolbox.vpkIntegrity.repairDone") : t("toolbox.vpkIntegrity.batchRepairReportTitle");
  session.contentEl.replaceChildren(createRepairBatchContent(safeResults));
  session.confirmBtn.textContent = outputPaths.length > 0 ? t("toolbox.vpkIntegrity.openOutput") : t("toolbox.common.close");
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
      showError(t("toolbox.vpkIntegrity.openRepairFailed", { error: formatError(error) }));
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
    ? t("toolbox.vpkIntegrity.statusClean")
    : report.repairable
      ? t("toolbox.vpkIntegrity.statusRepairable")
      : t("toolbox.vpkIntegrity.statusManual");

  wrapper.append(
    status,
    createPathBlock(t("toolbox.vpkIntegrity.labelFile"), report.path || ""),
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
  summary.textContent = t("toolbox.vpkIntegrity.batchSummary", {
    total: results.length,
    valid: validCount,
    repairable: repairableCount,
  });
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
    name.textContent = basename(item?.path || report.path || t("modals.vpkRisk.unknownVpk"));
    name.title = item?.path || report.path || "";
    const badge = document.createElement("span");
    badge.className = report.valid ? "is-valid" : report.repairable ? "is-repairable" : "is-invalid";
    badge.textContent = item?.error
      ? t("toolbox.vpkIntegrity.badgeFailed")
      : report.valid
        ? t("toolbox.vpkIntegrity.badgePass")
        : report.repairable
          ? t("modals.vpkRisk.repairable")
          : t("toolbox.vpkIntegrity.badgeManual");
    header.append(name, badge);
    card.append(header, createPathBlock(t("toolbox.common.labelPath"), item?.path || report.path || ""));
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
  const failedPart = failed > 0 ? t("toolbox.vpkIntegrity.repairSummaryFailed", { count: failed }) : "";
  summary.textContent = t("toolbox.vpkIntegrity.repairSummary", { success, failedPart });
  wrapper.appendChild(summary);

  const list = document.createElement("div");
  list.className = "vpk-integrity-batch-list";
  results.forEach((item) => {
    const card = document.createElement("section");
    card.className = "vpk-integrity-batch-item";
    const source = document.createElement("strong");
    source.textContent = basename(item?.sourcePath || t("modals.vpkRisk.unknownVpk"));
    source.title = item?.sourcePath || "";
    card.appendChild(source);
    if (item?.error) {
      const error = document.createElement("p");
      error.className = "vpk-integrity-batch-error";
      error.textContent = item.error;
      card.appendChild(error);
    } else {
      card.appendChild(createPathBlock(t("toolbox.vpkIntegrity.labelRepairFile"), item.outputPath || ""));
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
    parts.push(t("toolbox.vpkIntegrity.metaPreserved", { count: preserved.length }));
    note.title = t("toolbox.vpkIntegrity.metaPreservedList", { list: preserved.join("、") });
  }
  if (derived.length > 0) parts.push(t("toolbox.vpkIntegrity.metaDerivedList", { list: derived.join("、") }));
  if (recovered) parts.push(t("toolbox.vpkIntegrity.metaRecovered"));
  note.textContent = parts.join("；") + "。";
  return note;
}

function createIntegrityStats(report = {}) {
  const stats = document.createElement("p");
  stats.className = "vpk-integrity-stats";
  // 默认是索引级校验（读目录表 + 每个条目末尾一字节 + addoninfo.txt），不做整包 CRC；
  // 说清是哪种，免得用户以为"已校验 M 个"连数据都读过了。
  const scanLabel = report.scanMode === "full" ? t("toolbox.vpkIntegrity.scanLevelFull") : t("toolbox.vpkIntegrity.scanLevelIndex");
  const addonInfoLabel = report.addonInfoFound
    ? t(report.addonInfoValid ? "toolbox.vpkIntegrity.addonInfoValid" : "toolbox.vpkIntegrity.addonInfoInvalid")
    : t("toolbox.vpkIntegrity.addonInfoMissing");
  stats.textContent = t("toolbox.vpkIntegrity.statsLine", {
    total: Number(report.totalFiles || 0),
    verified: Number(report.verifiedFiles || 0),
    scanLabel,
    addonInfo: addonInfoLabel,
  });
  return stats;
}

function appendIssueList(wrapper, issues, compact = false) {
  const safeIssues = Array.isArray(issues) ? issues : [];
  if (safeIssues.length === 0) return;
  const heading = document.createElement("h4");
  heading.textContent = compact ? t("toolbox.vpkIntegrity.issuesCount", { count: safeIssues.length }) : t("toolbox.vpkIntegrity.issuesHeading");
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
      badge.textContent = t("modals.vpkRisk.repairable");
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
    return Promise.reject(new Error(t("modals.vpkRisk.unsupportedMethod", { method: methodName })));
  }
  return method(...args);
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || t("toolbox.common.unknownError"));
}
