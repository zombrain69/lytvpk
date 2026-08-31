import { showError, showNotification } from "../../core/toast.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let packRunning = false;

export async function openVPKPackTool({ refreshFilesKeepFilter } = {}) {
  if (packRunning) {
    showNotification("已有 VPK 正在打包", "info");
    return;
  }

  packRunning = true;
  try {
    const sourceDir = await callApp("SelectVPKPackSourceDirectory");
    if (!sourceDir) return;

    const choice = await choosePackOutput(sourceDir);
    if (!choice.outputDir) return;

    showNotification("正在打包 VPK...", "info");
    const result = await callApp(
      "PackVPKDirectory",
      sourceDir,
      choice.outputDir,
      !!choice.isAddons,
    );
    showVPKPackResult(result);
    showNotification("VPK 打包完成", "success");
    if (result.outputIsAddons && typeof refreshFilesKeepFilter === "function") {
      await refreshFilesKeepFilter();
    }
  } catch (error) {
    showError("打包流程失败: " + formatError(error));
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
    otherBtn.textContent = "选择其他位置";

    session.titleEl.textContent = "选择打包输出位置";
    session.contentEl.replaceChildren(createOutputChoiceContent(sourceDir));
    session.confirmBtn.textContent = "放入当前 addons";
    session.addActionButton(otherBtn);

    session.closeBtn.onclick = () => session.close("close");

    session.confirmBtn.onclick = async () => {
      session.setPending(true);
      try {
        const addons = await callApp("GetRootDirectory");
        if (!addons) {
          showError("未设置 addons 目录，请先在主界面选择 L4D2 addons 目录");
          return;
        }
        done({ outputDir: addons, isAddons: true });
      } catch (error) {
        showError("获取 addons 目录失败: " + formatError(error));
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
        showError("选择目录失败: " + formatError(error));
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
  note.textContent = "请选择打包后的 VPK 输出位置。";

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = "打包目录";
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
  cancelBtn.textContent = "关闭";

  session.titleEl.textContent = "打包完成";
  session.contentEl.replaceChildren(createPackResultContent(result));
  session.confirmBtn.textContent = "打开目标位置";
  session.addActionButton(cancelBtn);

  cancelBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  session.confirmBtn.onclick = async () => {
    session.close("confirm");
    if (!result.outputPath) return;
    try {
      await callApp("OpenFileLocation", result.outputPath);
    } catch (error) {
      showError("打开目标位置失败: " + formatError(error));
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
  summary.textContent = `已打包 ${packed} / ${total} 个文件。`;

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = "输出文件";
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
    return Promise.reject(new Error(`当前后端不支持 ${methodName}`));
  }
  return method(...args);
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || "未知错误");
}
