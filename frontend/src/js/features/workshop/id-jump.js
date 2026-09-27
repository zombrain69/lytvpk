// 「工坊 ID 直达」（对齐上游 ce2b268）：输入作品 ID 或链接，直接打开详情，
// 不用先搜索再翻列表。ID 解析复用后端已有的 ParseWorkshopID。

import { showError } from "../../core/toast.js";
import { ParseWorkshopID } from "../../../../wailsjs/go/app/App";
import { openWorkshopDetail } from "./detail.js";

const MODAL_ID = "workshop-id-jump-modal";
const INPUT_ID = "workshop-id-jump-input";
let bound = false;

function openWorkshopIdJumpModal() {
  const modal = document.getElementById(MODAL_ID);
  const input = document.getElementById(INPUT_ID);
  if (!modal || !input) return;
  input.value = "";
  modal.classList.remove("hidden");
  setTimeout(() => input.focus(), 30);
}

export function closeWorkshopIdJumpModal() {
  document.getElementById(MODAL_ID)?.classList.add("hidden");
}

async function jumpToWorkshopId() {
  const raw = String(document.getElementById(INPUT_ID)?.value || "").trim();
  if (!raw) {
    showError("请输入工坊 ID 或链接");
    return;
  }

  try {
    const id = String(await ParseWorkshopID(raw));
    if (!id || id === "0") {
      showError("无法识别工坊 ID");
      return;
    }
    closeWorkshopIdJumpModal();
    await openWorkshopDetail({ publishedfileid: id });
  } catch (error) {
    showError("无法识别工坊 ID: " + error);
  }
}

/** setupWorkshopIdJump 绑定「ID 直达」按钮与弹窗（重复调用安全）。 */
export function setupWorkshopIdJump() {
  if (bound) return;
  bound = true;

  document
    .getElementById("browser-id-jump-btn")
    ?.addEventListener("click", openWorkshopIdJumpModal);
  document
    .getElementById("close-workshop-id-jump-btn")
    ?.addEventListener("click", closeWorkshopIdJumpModal);
  document
    .getElementById("workshop-id-jump-cancel-btn")
    ?.addEventListener("click", closeWorkshopIdJumpModal);
  document
    .getElementById("workshop-id-jump-confirm-btn")
    ?.addEventListener("click", jumpToWorkshopId);
  document.getElementById(INPUT_ID)?.addEventListener("keydown", (event) => {
    if (event.key === "Enter") {
      event.preventDefault();
      void jumpToWorkshopId();
    }
  });
  document.getElementById(MODAL_ID)?.addEventListener("click", (event) => {
    if (event.target.id === MODAL_ID) closeWorkshopIdJumpModal();
  });
}
