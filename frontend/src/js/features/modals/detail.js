import { appState } from "../state.js";
import { formatFileSize, getLocationDisplayName } from "../../core/utils.js";
import { showError } from "../../core/toast.js";
import {
  GetWorkshopBrowserTarget,
  ParseWorkshopID,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import { handleProtocolWorkshop } from "../workshop/workshop-browser.js";
import {
  getCachedVPKPreview,
  loadVPKPreviewWithOptions,
} from "../shared/vpk-preview-cache.js";

let currentDetailFile = null;
let detailPreviousFocus = null;
let detailUnderlayStates = [];

document.addEventListener("keydown", (event) => {
  const modal = document.getElementById("file-detail-modal");
  if (!modal || modal.classList.contains("hidden")) return;
  if (event.key === "Escape") {
    event.preventDefault();
    closeModal();
  }
});

// 文件详情可能从冲突、加载顺序等其他模态窗口打开。将它提升到 body
// 直属层，避免受来源窗口的 stacking context / overflow 影响。
function promoteDetailModal(modal) {
  if (!modal) return;

  if (modal.classList.contains("hidden")) {
    const active = document.activeElement;
    detailPreviousFocus = active instanceof HTMLElement ? active : null;
  }

  if (modal.parentElement !== document.body) {
    document.body.appendChild(modal);
  }

  detailUnderlayStates.forEach(({ element, ariaHidden, inert }) => {
    if (!element?.isConnected) return;
    if (ariaHidden === null) element.removeAttribute("aria-hidden");
    else element.setAttribute("aria-hidden", ariaHidden);
    if (!inert) element.removeAttribute("inert");
    element.removeAttribute("data-detail-underlay");
  });
  detailUnderlayStates = [];

  document.querySelectorAll(".modal:not(#file-detail-modal):not(.hidden)").forEach((element) => {
    detailUnderlayStates.push({
      element,
      ariaHidden: element.getAttribute("aria-hidden"),
      inert: element.hasAttribute("inert"),
    });
    element.setAttribute("aria-hidden", "true");
    element.setAttribute("inert", "");
    element.setAttribute("data-detail-underlay", "true");
  });

  modal.classList.add("modal-overlay-top");
  modal.setAttribute("role", "dialog");
  modal.setAttribute("aria-modal", "true");
  modal.setAttribute("aria-hidden", "false");
  modal.setAttribute("tabindex", "-1");
  modal.style.setProperty("z-index", "30000", "important");
}

function restoreDetailModalState(modal) {
  if (!modal) return;
  modal.classList.remove("modal-overlay-top");
  modal.removeAttribute("aria-modal");
  modal.setAttribute("aria-hidden", "true");
  modal.removeAttribute("tabindex");
  modal.style.removeProperty("z-index");

  detailUnderlayStates.forEach(({ element, ariaHidden, inert }) => {
    if (!element?.isConnected) return;
    if (ariaHidden === null) element.removeAttribute("aria-hidden");
    else element.setAttribute("aria-hidden", ariaHidden);
    if (!inert) element.removeAttribute("inert");
    element.removeAttribute("data-detail-underlay");
  });
  detailUnderlayStates = [];

  const focusTarget = detailPreviousFocus;
  detailPreviousFocus = null;
  if (focusTarget?.isConnected && typeof focusTarget.focus === "function") {
    focusTarget.focus();
  }
}

async function resolveWorkshopID(file) {
  const workshopFileNameID =
    file.location === "workshop"
      ? String(file.name || "").replace(/\.vpk$/i, "")
      : "";
  for (const candidate of [file.workshopId, workshopFileNameID, file.addonURL0]) {
    if (!candidate) continue;
    try {
      return await ParseWorkshopID(candidate);
    } catch {
      // 没有可验证的工坊 ID 时，不显示工坊操作。
    }
  }
  return "";
}

function buildWorkshopBrowserURL(workshopID, target) {
  const id = encodeURIComponent(workshopID);
  return target === "mirror"
    ? `https://l4d2ws.com?workshop-id=${id}`
    : `https://steamcommunity.com/sharedfiles/filedetails/?id=${id}`;
}

export function showFileDetail(filePath) {
  // 冲突分析的基线可能来自当前筛选之外，详情按钮仍应能打开它。
  const file =
    appState.vpkFiles.find((f) => f.path === filePath) ||
    appState.allVpkFiles.find((f) => f.path === filePath);
  if (!file) {
    console.error("未找到文件:", filePath);
    return;
  }

  currentDetailFile = file;

  const modal = document.getElementById("file-detail-modal");
  if (!modal) {
    console.error("模态框元素不存在!");
    return;
  }

  document.getElementById("detail-file-name").textContent = file.name;
  document.getElementById("detail-name").textContent = file.name;
  document.getElementById("detail-size").textContent = formatFileSize(file.size);
  document.getElementById("detail-location").textContent = getLocationDisplayName(file.location);
  document.getElementById("detail-status").textContent = file.enabled ? "启用" : "禁用";
  document.getElementById("detail-modified").textContent = new Date(file.lastModified).toLocaleString();

  const previewSection = document.getElementById("preview-section");
  const previewImage = document.getElementById("detail-preview-image");
  const previewLoading = document.getElementById("detail-preview-loading");

  previewSection.classList.remove("hidden");
  previewImage.style.display = "none";
  previewImage.removeAttribute("src");
  previewLoading?.classList.remove("hidden");

  const cachedPreview = getCachedVPKPreview(file);
  if (cachedPreview) {
    previewImage.src = cachedPreview;
    previewImage.style.display = "block";
    previewLoading?.classList.add("hidden");
  } else if (cachedPreview === "") {
    previewSection.classList.add("hidden");
    previewLoading?.classList.add("hidden");
  } else {
    loadVPKPreviewWithOptions(file, { priority: true })
      .then((imgData) => {
        if (currentDetailFile?.path !== file.path) return;
        if (imgData) {
          previewImage.src = imgData;
          previewImage.style.display = "block";
          previewLoading?.classList.add("hidden");
        } else {
          previewSection.classList.add("hidden");
          previewLoading?.classList.add("hidden");
        }
      })
      .catch((err) => {
        console.error("加载预览图失败:", err);
        previewSection.classList.add("hidden");
        previewLoading?.classList.add("hidden");
      });
  }

  const tagsContainer = document.getElementById("detail-tags");
  const primaryTagHtml = file.primaryTag
    ? `<span class="tag primary-tag">${file.primaryTag}</span>`
    : "";
  tagsContainer.innerHTML = primaryTagHtml;

  const detailTagsContainer = document.getElementById("detail-detail-tags");
  const secondaryTagsHtml =
    file.secondaryTags && file.secondaryTags.length > 0
      ? file.secondaryTags
          .map((tag) => `<span class="tag secondary-tag">${tag}</span>`)
          .join("")
      : "";
  detailTagsContainer.innerHTML = secondaryTagsHtml;

  const vpkInfoSection = document.getElementById("vpk-info-section");
  document.getElementById("detail-vpk-title").textContent = file.title || "无标题";

  const authorItem = document.getElementById("detail-vpk-author-item");
  if (file.author && file.author !== "") {
    authorItem.style.display = "grid";
    document.getElementById("detail-vpk-author").textContent = file.author;
  } else {
    authorItem.style.display = "none";
  }

  const versionItem = document.getElementById("detail-vpk-version-item");
  if (file.version && file.version !== "") {
    versionItem.style.display = "grid";
    document.getElementById("detail-vpk-version").textContent = file.version;
  } else {
    versionItem.style.display = "none";
  }

  const descItem = document.getElementById("detail-vpk-desc-item");
  if (file.desc && file.desc !== "") {
    descItem.style.display = "grid";
    document.getElementById("detail-vpk-desc").textContent = file.desc;
  } else {
    descItem.style.display = "none";
  }

  const urlItem = document.getElementById("detail-vpk-url-item");
  const urlLink = document.getElementById("detail-vpk-url");
  const openBrowserButton = document.getElementById(
    "detail-vpk-open-browser-btn",
  );
  urlItem.style.display = "none";
  urlLink.textContent = "";
  urlLink.onclick = null;
  openBrowserButton.classList.add("hidden");
  openBrowserButton.onclick = null;

  (async () => {
    const workshopId = await resolveWorkshopID(file);

    if (currentDetailFile?.path !== file.path || !workshopId) return;

    urlItem.style.display = "grid";
    urlLink.textContent = `工坊 #${workshopId}`;
    urlLink.href = "javascript:void(0)";
    urlLink.removeAttribute("target");
    urlLink.onclick = (e) => {
      e.preventDefault();
      handleProtocolWorkshop(workshopId);
    };
    openBrowserButton.classList.remove("hidden");
    openBrowserButton.onclick = async () => {
      const target = await GetWorkshopBrowserTarget();
      BrowserOpenURL(buildWorkshopBrowserURL(workshopId, target));
    };
  })();

  const mapInfoSection = document.getElementById("map-info-section");
  if (file.primaryTag === "地图") {
    mapInfoSection.classList.remove("hidden");

    const campaignElement = document.getElementById("detail-campaign");
    campaignElement.textContent = file.campaign || "未知战役";

    const chaptersListElement = document.getElementById("detail-chapters-list");
    if (file.chapters && Object.keys(file.chapters).length > 0) {
      let chaptersHtml = "";
      Object.entries(file.chapters).forEach(([chapterCode, chapterInfo]) => {
        const chapterName = chapterInfo.title || chapterCode;
        const modes = chapterInfo.modes || [];
        chaptersHtml += `
          <div class="chapter-item">
            <div class="chapter-header">
              <div class="chapter-name">${chapterName}</div>
              <div class="chapter-code">${chapterCode}</div>
            </div>
            <div class="chapter-modes">${
              modes.length > 0 ? modes.join(" | ") : "未知模式"
            }</div>
          </div>
        `;
      });
      chaptersListElement.innerHTML = chaptersHtml;
    } else {
      chaptersListElement.innerHTML = '<div class="no-chapters">无章节信息</div>';
    }
  } else {
    mapInfoSection.classList.add("hidden");
  }

  promoteDetailModal(modal);
  modal.classList.remove("hidden");

  setTimeout(() => {
    const modalContent = modal.querySelector(".modal-content");
    const modalBody = modal.querySelector(".modal-body");
    if (modalContent) modalContent.scrollTop = 0;
    if (modalBody) modalBody.scrollTop = 0;
    const closeButton = modal.querySelector("#close-modal-header-btn");
    (closeButton || modal).focus?.();
  }, 0);

}

export function closeModal() {
  const modal = document.getElementById("file-detail-modal");
  modal?.classList.add("hidden");
  restoreDetailModalState(modal);
  currentDetailFile = null;
}
