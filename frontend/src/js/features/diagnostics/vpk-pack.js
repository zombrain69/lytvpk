import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let packRunning = false;

export async function openVPKPackTool({ refreshFilesKeepFilter } = {}) {
  if (packRunning) {
    showNotification(t("toolbox.vpkPack.busy"), "info");
    return;
  }

  packRunning = true;
  try {
    const sourceDir = await callApp("SelectVPKPackSourceDirectory");
    if (!sourceDir) return;

    const choice = await choosePackOutput(sourceDir);
    if (!choice.outputDir) return;

    showNotification(t("toolbox.vpkPack.packing"), "info");
    const result = await callApp(
      "PackVPKDirectory",
      sourceDir,
      choice.outputDir,
      !!choice.isAddons,
    );
    showVPKPackResult(result);
    showNotification(t("toolbox.vpkPack.done"), "success");
    if (result.outputIsAddons && typeof refreshFilesKeepFilter === "function") {
      await refreshFilesKeepFilter();
    }
  } catch (error) {
    showError(t("toolbox.vpkPack.failed", { error: formatError(error) }));
  } finally {
    packRunning = false;
  }
}

// choosePackOutput shows a two-option modal: pack into current addons, or pick another location.
// Resolves with { outputDir, isAddons } or { outputDir: "" } when cancelled.
function choosePackOutput(sourceDir) {
  return new Promise((resolve) => {
    let settled = false;
    let session = null;
    const done = (value, reason = "choice") => {
      if (settled) return;
      settled = true;
      session?.close(reason);
      resolve(value);
    };
    session = beginMessageModalSession({
      onClose: () => done({ outputDir: "" }, "cancel"),
    });
    if (!session) return done({ outputDir: "" }, "unavailable");

    const otherBtn = document.createElement("button");
    otherBtn.type = "button";
    otherBtn.className = "btn btn-secondary";
    otherBtn.textContent = t("toolbox.vpkPack.chooseOther");

    session.titleEl.textContent = t("toolbox.vpkPack.chooseTitle");
    session.contentEl.replaceChildren(createOutputChoiceContent(sourceDir));
    session.confirmBtn.textContent = t("toolbox.vpkPack.useAddons");
    session.addActionButton(otherBtn);

    session.closeBtn.onclick = () => session.close("close");

    session.confirmBtn.onclick = async () => {
      session.setPending(true);
      try {
        const addons = await callApp("GetRootDirectory");
        if (!addons) {
          showError(t("toolbox.vpkPack.addonsUnset"));
          return;
        }
        done({ outputDir: addons, isAddons: true });
      } catch (error) {
        showError(t("toolbox.vpkPack.addonsFailed", { error: formatError(error) }));
      } finally {
        session.setPending(false);
      }
    };

    otherBtn.onclick = async () => {
      session.setPending(true);
      try {
        const dir = await callApp("SelectDirectory");
        if (!dir) return;
        done({ outputDir: dir, isAddons: false });
      } catch (error) {
        showError(t("toolbox.vpkPack.selectDirFailed", { error: formatError(error) }));
      } finally {
        session.setPending(false);
      }
    };

    session.show();
  });
}

function createOutputChoiceContent(sourceDir) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-unpack-result";

  const note = document.createElement("p");
  note.textContent = t("toolbox.vpkPack.chooseNote");

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = t("toolbox.vpkPack.labelSource");
  const value = document.createElement("strong");
  value.textContent = sourceDir || "";
  value.title = sourceDir || "";
  pathBlock.append(label, value);

  wrapper.append(note, pathBlock);
  return wrapper;
}

function showVPKPackResult(result = {}) {
  const session = beginMessageModalSession();
  if (!session) return;

  const cancelBtn = document.createElement("button");
  cancelBtn.type = "button";
  cancelBtn.className = "btn btn-secondary";
  cancelBtn.textContent = t("toolbox.common.close");

  session.titleEl.textContent = t("toolbox.vpkPack.resultTitle");
  session.contentEl.replaceChildren(createPackResultContent(result));
  session.confirmBtn.textContent = t("toolbox.common.openLocation");
  session.addActionButton(cancelBtn);

  cancelBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  session.confirmBtn.onclick = async () => {
    session.close("confirm");
    if (!result.outputPath) return;
    try {
      await callApp("OpenFileLocation", result.outputPath);
    } catch (error) {
      showError(t("toolbox.common.openLocationFailed", { error: formatError(error) }));
    }
  };

  session.show();
}

function createPackResultContent(result = {}) {
  const wrapper = document.createElement("div");
  wrapper.className = "vpk-unpack-result";

  const summary = document.createElement("p");
  const total = Number(result.totalFiles || 0);
  const packed = Number(result.packedFiles || 0);
  summary.textContent = t("toolbox.vpkPack.resultSummary", { packed, total });

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = t("toolbox.vpkPack.labelOutput");
  const value = document.createElement("strong");
  value.textContent = result.outputPath || "";
  value.title = result.outputPath || "";
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
