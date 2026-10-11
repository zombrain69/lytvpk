import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let unpackRunning = false;

export async function openVPKUnpackTool() {
  if (unpackRunning) {
    showNotification(t("toolbox.vpkUnpack.busy"), "info");
    return;
  }

  unpackRunning = true;
  try {
    const vpkPath = await callApp("SelectVPKFile");
    if (!vpkPath) return;
    await unpackSelectedVPK(vpkPath);
  } catch (error) {
    showError(t("toolbox.common.selectVpkFailed", { error: formatError(error) }));
  } finally {
    unpackRunning = false;
  }
}

export async function unpackVPKFromPath(vpkPath) {
  if (unpackRunning) {
    showNotification(t("toolbox.vpkUnpack.busy"), "info");
    return;
  }

  if (!vpkPath) {
    showError(t("toolbox.vpkUnpack.pathRequired"));
    return;
  }

  unpackRunning = true;
  try {
    await unpackSelectedVPK(vpkPath);
  } finally {
    unpackRunning = false;
  }
}

async function unpackSelectedVPK(vpkPath) {
  let targetRoot = "";
  try {
    targetRoot = await callApp("SelectVPKUnpackOutputDirectory");
  } catch (error) {
    showError(t("toolbox.vpkUnpack.selectDirFailed", { error: formatError(error) }));
    return;
  }
  if (!targetRoot) return;

  showNotification(t("toolbox.vpkUnpack.unpacking"), "info");
  try {
    const result = await callApp("UnpackVPKFile", vpkPath, targetRoot);
    showVPKUnpackResult(result);
    showNotification(t("toolbox.vpkUnpack.done"), "success");
  } catch (error) {
    showError(t("toolbox.vpkUnpack.failed", { error: formatError(error) }));
  }
}

function showVPKUnpackResult(result = {}) {
  const session = beginMessageModalSession();
  if (!session) return;

  const cancelBtn = document.createElement("button");
  cancelBtn.type = "button";
  cancelBtn.className = "btn btn-secondary";
  cancelBtn.textContent = t("toolbox.common.close");

  session.titleEl.textContent = t("toolbox.vpkUnpack.resultTitle");
  session.contentEl.replaceChildren(createResultContent(result));
  session.confirmBtn.textContent = t("toolbox.common.openLocation");
  session.addActionButton(cancelBtn);

  cancelBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  session.confirmBtn.onclick = async () => {
    session.close("confirm");
    if (!result.outputDir) return;
    try {
      await callApp("OpenFileLocation", result.outputDir);
    } catch (error) {
      showError(t("toolbox.common.openLocationFailed", { error: formatError(error) }));
    }
  };

  session.show();
}

function createResultContent(result = {}) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-unpack-result";

  const summary = document.createElement("p");
  const total = Number(result.totalFiles || 0);
  const extracted = Number(result.extractedFiles || 0);
  summary.textContent = t("toolbox.vpkUnpack.resultSummary", { extracted, total });

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = t("toolbox.vpkUnpack.labelOutput");
  const value = document.createElement("strong");
  value.textContent = result.outputDir || "";
  value.title = result.outputDir || "";
  pathBlock.append(label, value);

  wrapper.append(summary, pathBlock);
  return wrapper;
}

function callApp(methodName, ...args) {
  const method = window?.go?.app?.App?.[methodName];
  if (typeof method !== "function") {
    return Promise.reject(new Error(t("toolbox.common.backendUnsupported", { method: methodName })));
  }
  return method(...args);
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || t("toolbox.common.unknownError"));
}
