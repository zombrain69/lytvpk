// IP 直连：允许直接粘贴 IP/域名（含 connect 命令 / steam://connect 链接）连服务器。
// 与上游 1d581aa 对齐；只做输入解析与连接调用，不写入收藏列表。
import { parseServerInput } from "./address.js";
import { ClipboardGetText } from "../../../../wailsjs/runtime/runtime";

let showError;
let connectServer;
// 每次打开弹窗递增：剪贴板是异步读取的，过期结果不能覆盖用户已经输入的地址。
let openToken = 0;

export function configureDirectConnect(deps) {
  ({ showError, connectServer } = deps);
}

export function openDirectConnectModal() {
  const modal = document.getElementById("direct-connect-modal");
  const addressInput = document.getElementById("direct-connect-address");
  if (!modal || !addressInput) return;

  addressInput.value = "";
  modal.classList.remove("hidden");
  document.getElementById("global-dropdown")?.classList.add("hidden");
  addressInput.focus();
  void prefillFromClipboard(++openToken);
}

// 剪贴板里如果有可用的服务器地址就自动填入（对齐上游 a17e39a）。
// 解析不了（普通文本/工坊链接等）就保持为空，绝不报错打扰用户。
async function prefillFromClipboard(token) {
  const modal = document.getElementById("direct-connect-modal");
  const addressInput = document.getElementById("direct-connect-address");
  if (!modal || !addressInput || typeof ClipboardGetText !== "function") return;
  let text = "";
  try {
    text = (await ClipboardGetText()) || "";
  } catch (error) {
    return;
  }
  if (!text || token !== openToken || modal.classList.contains("hidden")) return;
  if (addressInput.value) return;
  let address = "";
  try {
    address = parseServerInput(text);
  } catch (error) {
    return;
  }
  if (!address || token !== openToken || modal.classList.contains("hidden") || addressInput.value) return;
  addressInput.value = address;
  addressInput.select();
}

export function closeDirectConnectModal() {
  document.getElementById("direct-connect-modal")?.classList.add("hidden");
}

function submitDirectConnect() {
  const input = document.getElementById("direct-connect-address")?.value;

  let address;
  try {
    address = parseServerInput(input);
  } catch (error) {
    showError?.(error.message);
    return;
  }

  connectServer?.(address, { notify: true });
  closeDirectConnectModal();
}

export function setupDirectConnectListeners() {
  const modal = document.getElementById("direct-connect-modal");
  if (!modal) return;

  document
    .getElementById("open-direct-connect-modal-btn")
    ?.addEventListener("click", openDirectConnectModal);
  document
    .getElementById("close-direct-connect-modal-btn")
    ?.addEventListener("click", closeDirectConnectModal);
  document
    .getElementById("cancel-direct-connect-btn")
    ?.addEventListener("click", closeDirectConnectModal);
  document
    .getElementById("confirm-direct-connect-btn")
    ?.addEventListener("click", submitDirectConnect);

  document
    .getElementById("direct-connect-address")
    ?.addEventListener("keydown", (event) => {
      if (event.key === "Enter") {
        event.preventDefault();
        submitDirectConnect();
      }
    });

  modal.addEventListener("click", (event) => {
    if (event.target === modal) {
      closeDirectConnectModal();
    }
  });
}
