import { showWarning } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { beginMessageModalSession } from "../../core/message-modal.js";
import { openVPKIntegrityForPaths } from "../diagnostics/vpk-integrity.js";

/**
 * Normalize paths while preserving their first-seen order.
 * @param {string|string[]} filePaths
 * @returns {string[]}
 */
export function normalizeVPKPaths(filePaths) {
  const values = Array.isArray(filePaths) ? filePaths : [filePaths];
  const seen = new Set();
  return values
    .map((value) => String(value || "").trim())
    .filter((value) => {
      const key = value.toLowerCase();
      if (!value || seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

/**
 * Convert InspectVPKIntegrity/InspectVPKIntegrityBatch output into a compact
 * representation suitable for a confirmation warning and for unit tests.
 */
export function summarizeVPKIntegrityResults(results = []) {
  const entries = Array.isArray(results) ? results : [results];
  const items = entries.map((entry) => {
    const report = entry?.report || entry || {};
    const path = String(entry?.path || report.path || "");
    const issues = Array.isArray(report.issues)
      ? report.issues
          .map((issue) => {
            const message = String(issue?.message || "").trim();
            if (!message) return "";
            const issuePath = String(issue?.path || "").trim();
            return issuePath ? `${issuePath}: ${message}` : message;
          })
          .filter(Boolean)
      : [];
    if (entry?.error) issues.push(String(entry.error));
    return {
      path,
      name: basename(path || t("modals.vpkRisk.unknownVpk")),
      valid: Boolean(report.valid) && !entry?.error,
      repairable: Boolean(report.repairable),
      issues,
      error: entry?.error ? String(entry.error) : "",
    };
  });

  const problemItems = items.filter((item) => !item.valid || item.issues.length > 0);
  return {
    items: problemItems,
    problemCount: problemItems.length,
    repairableCount: problemItems.filter((item) => item.repairable).length,
  };
}

/** Inspection failures are warnings, not a new hard block. */
export function shouldAllowAfterInspectionFailure() {
  return true;
}

/**
 * Build the plain-text warning used by the risk modal. Kept pure so wording
 * can be validated without a browser or Wails runtime.
 */
export function formatVPKRiskWarning(operationLabel = null, items = []) {
  const label = operationLabel || t("mods.ops.label.continue");
  const safeItems = Array.isArray(items) ? items : [];
  const details = safeItems
    .map((item) => {
      const issueText = Array.isArray(item?.issues) ? item.issues.filter(Boolean).join("；") : "";
      const detail = issueText || item?.detail || item?.error || t("modals.vpkRisk.issueFallback");
      return t("modals.vpkRisk.bullet", {
        name: item?.name || basename(item?.path || t("modals.vpkRisk.unknownVpk")),
        detail,
      });
    })
    .join("\n");
  const repairHint = safeItems.some((item) => item?.repairable)
    ? t("modals.vpkRisk.repairableHint")
    : t("modals.vpkRisk.detailsHint");
  return t("modals.vpkRisk.plainText", { label, details, hint: repairHint });
}

/**
 * Inspect one or more VPKs before a state-changing operation. Valid files
 * continue immediately; problematic files get a conspicuous modal with
 * Continue, Cancel, and Open integrity tool actions. Inspection failures are
 * surfaced as a warning toast and deliberately allow the operation.
 */
export async function confirmVPKIntegrityWarning(filePaths, operationLabel = null) {
  const label = operationLabel || t("mods.ops.label.continue");
  const paths = normalizeVPKPaths(filePaths);
  if (paths.length === 0) return true;

  let results;
  try {
    if (paths.length === 1) {
      const report = await callApp("InspectVPKIntegrity", paths[0]);
      results = [{ path: paths[0], report }];
    } else {
      results = await callApp("InspectVPKIntegrityBatch", paths);
    }
  } catch (error) {
    showWarning(t("modals.vpkRisk.inspectFailed", { label, error: formatError(error) }));
    return shouldAllowAfterInspectionFailure(error);
  }

  const summary = summarizeVPKIntegrityResults(results);
  if (summary.problemCount === 0) return true;
  return showRiskModal(paths, label, summary.items);
}

function showRiskModal(paths, operationLabel, items) {
  return new Promise((resolve) => {
    let settled = false;
    const settle = (value, reason) => {
      if (settled) return;
      settled = true;
      session?.close(reason);
      resolve(value);
    };

    const session = beginMessageModalSession({
      onClose: () => {
        if (!settled) {
          settled = true;
          resolve(false);
        }
      },
    });
    if (!session) {
      resolve(true);
      return;
    }

    session.addClass(session.modal.querySelector(".modal-content"), "vpk-risk-warning-modal");
    session.titleEl.textContent = t("modals.vpkRisk.title", { label: operationLabel });
    session.contentEl.replaceChildren(createRiskContent(items, operationLabel));

    const inspectButton = document.createElement("button");
    inspectButton.type = "button";
    inspectButton.className = "btn btn-secondary";
    inspectButton.textContent = t("modals.vpkRisk.inspect");
    inspectButton.onclick = async () => {
      if (!session.isCurrent()) return;
      session.close("inspect");
      if (!settled) {
        settled = true;
        resolve(false);
      }
      await openVPKIntegrityForPaths(paths);
    };
    const cancelButton = document.createElement("button");
    cancelButton.type = "button";
    cancelButton.className = "btn btn-secondary";
    cancelButton.textContent = t("modals.vpkRisk.cancel");
    cancelButton.onclick = () => settle(false, "cancel");
    session.addActionButton(cancelButton, session.confirmBtn);
    session.addActionButton(inspectButton, session.confirmBtn);

    session.confirmBtn.textContent = t("mods.ops.label.continue");
    session.confirmBtn.onclick = () => settle(true, "continue");
    session.closeBtn.onclick = () => settle(false, "cancel");
    session.show();
  });
}

function createRiskContent(items, operationLabel) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-risk-warning-content";

  const intro = document.createElement("p");
  intro.textContent = t("modals.vpkRisk.intro", { label: operationLabel });
  wrapper.appendChild(intro);

  const list = document.createElement("ul");
  list.className = "vpk-risk-warning-list";
  items.forEach((item) => {
    const listItem = document.createElement("li");
    const name = document.createElement("strong");
    name.textContent = item.name;
    listItem.appendChild(name);
    const details = document.createElement("span");
    details.textContent = `：${item.issues.join("；") || t("modals.vpkRisk.issueFallback")}`;
    listItem.appendChild(details);
    if (item.repairable) {
      const badge = document.createElement("small");
      badge.textContent = t("modals.vpkRisk.repairable");
      listItem.appendChild(badge);
    }
    list.appendChild(listItem);
  });
  wrapper.appendChild(list);

  const note = document.createElement("p");
  note.className = "vpk-risk-warning-note";
  note.textContent = t("modals.vpkRisk.note");
  wrapper.appendChild(note);
  return wrapper;
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") {
    return Promise.reject(new Error(t("modals.vpkRisk.unsupportedMethod", { method: methodName })));
  }
  return method(...args);
}

function basename(filePath) {
  return String(filePath || "").split(/[\\/]/).pop() || t("modals.vpkRisk.unknownVpk");
}

function formatError(error) {
  return error?.message ? error.message : String(error || t("mods.ops.common.unknownError"));
}
