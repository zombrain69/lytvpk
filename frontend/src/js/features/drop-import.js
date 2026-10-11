import { t } from "../core/i18n-runtime.js";
import { showError, showNotification } from "../core/toast.js";
import { openMDMPReportFromPath } from "./diagnostics/mdmp-report.js";

let HandleFileDrop;
let EventsOn;
let progressUnsubscribe = null;
let importRunning = false;

export function configureDropImport(deps = {}) {
  HandleFileDrop = deps.HandleFileDrop;
  EventsOn = deps.EventsOn;
  if (!progressUnsubscribe && typeof EventsOn === "function") {
    progressUnsubscribe = EventsOn("drop_import_progress", renderDropImportProgress);
  }
}

export async function handleDropImportPaths(paths = [], { source = "drop" } = {}) {
  const normalizedPaths = normalizePaths(paths);
  if (normalizedPaths.length === 0) return null;

  const dumpPaths = normalizedPaths.filter(isDumpPath);
  const importPaths = normalizedPaths.filter((path) => !isDumpPath(path));

  if (dumpPaths.length > 0) {
    openFirstDump(dumpPaths);
  }

  if (importPaths.length === 0) {
    return {
      total: dumpPaths.length,
      succeeded: dumpPaths.length > 0 ? 1 : 0,
      failed: Math.max(0, dumpPaths.length - 1),
      items: [],
      hasInstallChanges: false,
    };
  }

  if (importRunning) {
    showNotification(t("mods.dropImport.busy"), "info");
    return null;
  }

  if (typeof HandleFileDrop !== "function") {
    showError(t("mods.dropImport.unsupported"));
    return null;
  }

  importRunning = true;
  showDropImportPanel(source === "select" ? t("mods.dropImport.titleSelect") : t("mods.dropImport.titleDrop"));
  renderDropImportProgress({
    current: 0,
    total: importPaths.length,
    percent: 0,
    phase: "preparing",
    name: "",
    message: t("mods.dropImport.preparing"),
  });

  try {
    const result = await HandleFileDrop(importPaths);
    renderDropImportResult(result || {});
    showImportSummary(result || {});
    return result;
  } catch (error) {
    renderDropImportError(t("mods.dropImport.processFailed", { error: formatError(error) }));
    showError(t("mods.dropImport.processFailed", { error: formatError(error) }));
    return null;
  } finally {
    importRunning = false;
  }
}

function openFirstDump(dumpPaths) {
  openMDMPReportFromPath(dumpPaths[0]).catch((error) => {
    showError(t("mods.dropImport.dumpOpenFailed", { error: formatError(error) }));
  });
  if (dumpPaths.length > 1) {
    showNotification(t("mods.dropImport.dumpOpenedExtra", { count: dumpPaths.length - 1 }), "info");
  }
}

function showDropImportPanel(title) {
  const panel = ensureDropImportPanel();
  panel.querySelector(".drop-import-title").textContent = title;
  panel.querySelector(".drop-import-phase").textContent = t("mods.dropImport.phasePreparing");
  panel.querySelector(".drop-import-summary").textContent = t("mods.dropImport.progressZero");
  panel.querySelector(".drop-import-file").textContent = t("mods.dropImport.waitingFile");
  setDropImportCurrent(panel, t("mods.dropImport.preparing"));
  panel.querySelector(".drop-import-active-list").replaceChildren();
  panel.querySelector(".drop-import-progress-fill").style.width = "0%";
  panel.querySelector(".drop-import-progress-meta").textContent = "0%";
  panel.querySelector(".drop-import-results").replaceChildren();
  panel.querySelector(".drop-import-action").classList.add("hidden");
  panel.classList.remove("hidden", "is-error", "is-complete");
  panel.classList.add("is-running");
}

function renderDropImportProgress(progress = {}) {
  const panel = ensureDropImportPanel();
  panel.classList.remove("hidden", "is-error", "is-complete");
  panel.classList.add("is-running");

  const itemPercent = clampPercent(Number(progress.percent || 0));
  const current = Number(progress.current || 0);
  const total = Number(progress.total || 0);
  const percent = getOverallPercent(itemPercent, current, total);
  const name = progress.name || basename(progress.path || "");
  const message = progress.message || t("mods.dropImport.processing");
  const activeNames = Array.isArray(progress.activeNames) ? progress.activeNames : [];
  const fileLabel = formatImportFileLabel(name, current, total);

  panel.querySelector(".drop-import-title").textContent = t("mods.dropImport.titleImporting");
  panel.querySelector(".drop-import-phase").textContent = formatPhase(progress.phase);
  panel.querySelector(".drop-import-summary").textContent = t("mods.dropImport.progress", { percent });
  panel.querySelector(".drop-import-file").textContent = fileLabel;
  panel.querySelector(".drop-import-file").title = progress.path || name || "";
  if (activeNames.length > 0) {
    setDropImportCurrent(panel, t("mods.dropImport.extractingParallel"));
  } else {
    setDropImportCurrent(panel, message);
  }
  renderActiveNames(panel.querySelector(".drop-import-active-list"), activeNames);
  panel.querySelector(".drop-import-progress-fill").style.width = `${percent}%`;
  panel.querySelector(".drop-import-progress-meta").textContent = `${percent}%`;
}

function renderDropImportResult(result = {}) {
  const panel = ensureDropImportPanel();
  const failed = Number(result.failed || 0);
  const succeeded = Number(result.succeeded || 0);
  const total = Number(result.total || 0);

  panel.classList.toggle("is-error", failed > 0 && succeeded === 0);
  panel.classList.remove("is-running");
  panel.classList.add("is-complete");
  panel.querySelector(".drop-import-title").textContent = failed > 0 ? t("mods.dropImport.titleDoneFailed") : t("mods.dropImport.titleDone");
  panel.querySelector(".drop-import-phase").textContent = failed > 0 ? t("mods.dropImport.phaseComplete") : t("mods.dropImport.phaseComplete");
  panel.querySelector(".drop-import-summary").textContent = t("mods.dropImport.summaryLine", { succeeded, total, failed });
  panel.querySelector(".drop-import-file").textContent = failed > 0 ? t("mods.dropImport.viewResults") : t("mods.dropImport.allDone");
  setDropImportCurrent(panel, failed > 0 ? t("mods.dropImport.partialFail") : t("mods.dropImport.allDone"));
  panel.querySelector(".drop-import-active-list").replaceChildren();
  panel.querySelector(".drop-import-progress-fill").style.width = "100%";
  panel.querySelector(".drop-import-progress-meta").textContent = "100%";
  panel.querySelector(".drop-import-action").classList.remove("hidden");

  const results = panel.querySelector(".drop-import-results");
  results.replaceChildren();
  (result.items || []).forEach((item) => results.appendChild(createResultRow(item)));
}

function renderDropImportError(message) {
  const panel = ensureDropImportPanel();
  panel.classList.remove("hidden");
  panel.classList.remove("is-running");
  panel.classList.add("is-error", "is-complete");
  panel.querySelector(".drop-import-title").textContent = t("mods.dropImport.titleFailed");
  panel.querySelector(".drop-import-phase").textContent = t("mods.dropImport.failed");
  panel.querySelector(".drop-import-summary").textContent = message || t("mods.dropImport.processFilesFailed");
  panel.querySelector(".drop-import-file").textContent = t("mods.dropImport.processFailedShort");
  setDropImportCurrent(panel, t("mods.dropImport.errorRetry"));
  panel.querySelector(".drop-import-active-list").replaceChildren();
  panel.querySelector(".drop-import-progress-fill").style.width = "100%";
  panel.querySelector(".drop-import-progress-meta").textContent = t("mods.dropImport.failed");
  panel.querySelector(".drop-import-results").replaceChildren();
  panel.querySelector(".drop-import-action").classList.remove("hidden");
}

function createResultRow(item = {}) {
  const row = document.createElement("div");
  row.className = `drop-import-result ${item.success ? "is-success" : "is-failed"}`;

  const body = document.createElement("div");
  body.className = "drop-import-result-body";

  const name = document.createElement("strong");
  name.textContent = item.name || basename(item.path || "") || "-";
  name.title = item.path || "";

  const message = document.createElement("span");
  message.textContent = item.message || "";

  body.append(name, message);
  row.appendChild(body);
  if (!item.success) {
    const status = document.createElement("span");
    status.className = "drop-import-result-status";
    status.textContent = t("mods.dropImport.failed");
    row.appendChild(status);
  }
  return row;
}

function setDropImportCurrent(panel, text, hidden = false) {
  const current = panel.querySelector(".drop-import-current");
  if (!current) return;
  current.textContent = text || "";
  current.classList.toggle("hidden", hidden);
}

function showImportSummary(result = {}) {
  const failed = Number(result.failed || 0);
  const succeeded = Number(result.succeeded || 0);
  if (failed > 0 && succeeded === 0) {
    showError(t("mods.dropImport.summaryFailAll", { failed }));
  } else if (failed > 0) {
    showNotification(t("mods.dropImport.summaryPartial", { succeeded, failed }), "info");
  } else {
    showNotification(t("mods.dropImport.summaryOk", { succeeded }), "success");
  }
}

function ensureDropImportPanel() {
  let panel = document.getElementById("drop-import-panel");
  if (panel) return panel;

  panel = document.createElement("section");
  panel.id = "drop-import-panel";
  panel.className = "drop-import-panel hidden";
  panel.setAttribute("aria-live", "polite");

  const header = document.createElement("div");
  header.className = "drop-import-header";
  const stateIcon = document.createElement("div");
  stateIcon.className = "drop-import-state-icon";
  stateIcon.setAttribute("aria-hidden", "true");
  const titleWrap = document.createElement("div");
  titleWrap.className = "drop-import-title-wrap";
  const title = document.createElement("h3");
  title.className = "drop-import-title";
  title.textContent = t("mods.dropImport.titleImporting");
  const summary = document.createElement("p");
  summary.className = "drop-import-summary";
  summary.textContent = t("mods.dropImport.preparing");
  const phase = document.createElement("span");
  phase.className = "drop-import-phase";
  phase.textContent = t("mods.dropImport.phasePreparing");
  titleWrap.append(title, summary);
  header.append(stateIcon, titleWrap, phase);

  const progressSection = document.createElement("div");
  progressSection.className = "drop-import-progress-section";
  const file = document.createElement("strong");
  file.className = "drop-import-file";
  file.textContent = t("mods.dropImport.waitingFile");
  const current = document.createElement("p");
  current.className = "drop-import-current";
  const activeList = document.createElement("div");
  activeList.className = "drop-import-active-list";

  const progress = document.createElement("div");
  progress.className = "drop-import-progress";
  const bar = document.createElement("div");
  bar.className = "drop-import-progress-bar";
  const fill = document.createElement("div");
  fill.className = "drop-import-progress-fill";
  bar.appendChild(fill);
  const meta = document.createElement("span");
  meta.className = "drop-import-progress-meta";
  meta.textContent = "0%";
  progress.append(bar, meta);

  const results = document.createElement("div");
  results.className = "drop-import-results";

  const footer = document.createElement("div");
  footer.className = "drop-import-footer";
  const actionBtn = document.createElement("button");
  actionBtn.type = "button";
  actionBtn.className = "btn btn-primary drop-import-action hidden";
  actionBtn.textContent = t("mods.dropImport.close");
  actionBtn.addEventListener("click", () => panel.classList.add("hidden"));
  footer.appendChild(actionBtn);

  progressSection.append(file, current, activeList, progress);
  panel.append(header, progressSection, results, footer);
  document.body.appendChild(panel);
  return panel;
}

function normalizePaths(paths) {
  const seen = new Set();
  const normalized = [];
  paths.forEach((path) => {
    const value = String(path || "").trim();
    if (!value || seen.has(value)) return;
    seen.add(value);
    normalized.push(value);
  });
  return normalized;
}

function renderActiveNames(container, names = []) {
  if (!container) return;
  container.replaceChildren();
  names.forEach((name) => {
    const item = document.createElement("span");
    item.className = "drop-import-active-item";
    item.textContent = name;
    item.title = name;
    container.appendChild(item);
  });
}

function isDumpPath(path) {
  return /\.(mdmp|dmp)$/i.test(String(path || ""));
}

function clampPercent(value) {
  if (!Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, Math.round(value)));
}

function getOverallPercent(itemPercent, current, total) {
  if (!total || total <= 1) return itemPercent;
  const safeCurrent = Math.max(1, Math.min(current || 1, total));
  return clampPercent(((safeCurrent - 1) * 100 + itemPercent) / total);
}

function formatImportFileLabel(name, current, total) {
  if (total > 1) {
    if (current >= 1) {
      const safeCurrent = Math.min(current, total);
      return t("mods.dropImport.fileLabelIndexed", { current: safeCurrent, total, name: name || t("mods.dropImport.currentFile") });
    }
    return t("mods.dropImport.fileLabelWaiting", { total });
  }
  return name || t("mods.dropImport.currentFile");
}

function basename(path = "") {
  return String(path).split(/[\\/]/).pop() || path;
}

function formatPhase(phase = "") {
  const labels = {
    preparing: t("mods.dropImport.phasePreparing"),
    copying: t("mods.dropImport.phaseCopying"),
    extracting: t("mods.dropImport.phaseExtracting"),
    packing: t("mods.dropImport.phasePacking"),
    complete: t("mods.dropImport.phaseComplete"),
    failed: t("mods.dropImport.failed"),
  };
  return labels[phase] || t("mods.dropImport.phaseFallback");
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || t("mods.dropImport.unknownError"));
}
