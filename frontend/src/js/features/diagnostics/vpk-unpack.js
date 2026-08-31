import { showError, showNotification } from "../../core/toast.js";
import { beginMessageModalSession } from "../../core/message-modal.js";

let unpackRunning = false;

export async function openVPKUnpackTool() {
  if (unpackRunning) {
    showNotification("已有 VPK 正在解包", "info");
    return;
  }

  unpackRunning = true;
  try {
    const vpkPath = await callApp("SelectVPKFile");
    if (!vpkPath) return;
    await unpackSelectedVPK(vpkPath);
  } catch (error) {
    showError("选择 VPK 失败: " + formatError(error));
  } finally {
    unpackRunning = false;
  }
}

export async function unpackVPKFromPath(vpkPath) {
  if (unpackRunning) {
    showNotification("已有 VPK 正在解包", "info");
    return;
  }

  if (!vpkPath) {
    showError("VPK 文件路径不能为空");
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
    showError("选择解包位置失败: " + formatError(error));
    return;
  }
  if (!targetRoot) return;

  showNotification("正在解包 VPK...", "info");
  try {
    const result = await callApp("UnpackVPKFile", vpkPath, targetRoot);
    showVPKUnpackResult(result);
    showNotification("VPK 解包完成", "success");
  } catch (error) {
    showError("解包失败: " + formatError(error));
  }
}

function showVPKUnpackResult(result = {}) {
  const session = beginMessageModalSession();
  if (!session) return;

  const cancelBtn = document.createElement("button");
  cancelBtn.type = "button";
  cancelBtn.className = "btn btn-secondary";
  cancelBtn.textContent = "关闭";

  session.titleEl.textContent = "解包完成";
  session.contentEl.replaceChildren(createResultContent(result));
  session.confirmBtn.textContent = "打开目标位置";
  session.addActionButton(cancelBtn);

  cancelBtn.onclick = () => session.close("close");
  session.closeBtn.onclick = () => session.close("close");
  session.confirmBtn.onclick = async () => {
    session.close("confirm");
    if (!result.outputDir) return;
    try {
      await callApp("OpenFileLocation", result.outputDir);
    } catch (error) {
      showError("打开目标位置失败: " + formatError(error));
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
  summary.textContent = `已解包 ${extracted} / ${total} 个文件。`;

  const pathBlock = document.createElement("div");
  pathBlock.className = "vpk-unpack-result-path";
  const label = document.createElement("span");
  label.textContent = "输出目录";
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
    return Promise.reject(new Error(`当前后端不支持 ${methodName}`));
  }
  return method(...args);
}

function formatError(error) {
  if (error?.message) return error.message;
  return String(error || "未知错误");
}
