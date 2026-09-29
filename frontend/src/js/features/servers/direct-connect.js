// IP 直连：允许直接粘贴 IP/域名（含 connect 命令 / steam://connect 链接）连服务器。
// 与上游 1d581aa 对齐；只做输入解析与连接调用，不写入收藏列表。
import { parseServerInput } from "./address.js";

let showError;
let connectServer;

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
