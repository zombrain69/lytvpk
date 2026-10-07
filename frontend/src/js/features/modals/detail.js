import { appState } from "../state.js";
import { escapeHtml, formatFileSize, getLocationDisplayName } from "../../core/utils.js";
import { showError } from "../../core/toast.js";
import {
  GetModEvidence,
  GetWorkshopBrowserTarget,
  ParseWorkshopID,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import { handleProtocolWorkshop } from "../workshop/workshop-browser.js";
import {
  getCachedVPKPreview,
  loadVPKPreviewWithOptions,
} from "../shared/vpk-preview-cache.js";
import { initDetailIgnoreControls, syncDetailIgnoreEditor } from "./detail-ignore.js";
import { buildTagEvidenceRows } from "./tag-evidence-view.mjs";

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
  // 幂等绑定：放在打开详情时执行，避免依赖脚本与 DOM 的加载顺序。
  initDetailIgnoreControls();
  void syncDetailIgnoreEditor(file);

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
    ? `<span class="tag primary-tag">${escapeHtml(file.primaryTag)}</span>`
    : "";
  const subjectSummary = String(file.subjectSummary || "").trim();
  tagsContainer.innerHTML = primaryTagHtml;
  const subjectElement = document.getElementById("detail-subject");
  if (subjectElement) {
    const xdrSummary = String(file.xdrSummary || "").trim();
    const xdrSlots = Array.isArray(file.xdrSlots) ? file.xdrSlots : [];
    const xdrPriority = file.xdrPriority || null;
    // 每个槽位的"会不会播"结论（后端按同槽占用算好）：随机=有别的 Mod 抢同一槽。
    const slotStatusByKey = new Map(
      (xdrPriority?.slots || []).map((entry) => [`${entry.character || ""}|${entry.slot ?? 0}`, entry]),
    );
    const xdrPriorityHtml = (() => {
      if (!xdrPriority?.state) return "";
      const randomCount = Number(xdrPriority.randomSlots || 0);
      const activeCount = Number(xdrPriority.activeSlots || 0);
      if (xdrPriority.state === "active") {
        return `<div class="detail-xdr-priority is-active">▶ 动作生效：${activeCount} 个槽位都没有同槽竞争者，游戏里会按预期播放</div>`;
      }
      if (xdrPriority.state === "random") {
        return `<div class="detail-xdr-priority is-random">⚠ 动作随机生效：${randomCount} 个槽位被其它 Mod 同槽占用 —— 官方规则是同角色同槽只会随机生效一个（与加载顺序无关），要确定播哪个就把其中一个改到空槽</div>`;
      }
      return `<div class="detail-xdr-priority is-partial">◐ 动作部分生效：${activeCount} 个槽位会播放，${randomCount} 个槽位与其它 Mod 同槽、游戏随机生效</div>`;
    })();
    const xdrRows = xdrSlots
      .map((slot) => {
        const actions = Array.isArray(slot.actions) && slot.actions.length > 0
          ? ` · 动作：${slot.actions.join("、")}`
          : "";
        // 槽位用途只是官方建议（作者可以自选别的槽），所以写成"官方建议"而不是断言。
        const designation = slot.slotName
          ? ` · 官方建议：${escapeHtml(slot.slotName)}${slot.slotGroup ? `（${escapeHtml(slot.slotGroup)}）` : ""}`
          : "";
        const status = slotStatusByKey.get(`${slot.character || ""}|${slot.slot ?? 0}`);
        let stateHtml = "";
        if (status?.state === "active") {
          stateHtml = `<span class="detail-xdr-state is-active">▶ 会生效</span>`;
        } else if (status?.state === "random") {
          const rivals = (status.rivals || [])
            .map((rival) => String(rival.title || rival.name || "").trim())
            .filter(Boolean)
            .slice(0, 3)
            .join("、");
          stateHtml = `<span class="detail-xdr-state is-random" title="同槽只会随机生效一个">⚠ 随机${rivals ? `（同槽：${escapeHtml(rivals)}）` : ""}</span>`;
        }
        return `<div class="detail-xdr-slot"><strong>${escapeHtml(slot.character || "未指定角色/模型")}</strong> · ${escapeHtml(slot.model || "未知模型")} · slot ${escapeHtml(slot.slotLabel || String(slot.slot ?? "?"))}${designation}${escapeHtml(actions)}${stateHtml}</div>`;
      })
      .join("");
    subjectElement.innerHTML = `${xdrSummary ? `<div class="detail-xdr-summary">${escapeHtml(xdrSummary)}</div>` : ""}${xdrPriorityHtml}${xdrRows}${subjectSummary ? `<div class="detail-subject-text">${escapeHtml(subjectSummary)}</div>` : ""}` || "主体：未识别";
    subjectElement.dataset.confidence = String(file.subjectConfidence || "低");
  }

  const detailTagsContainer = document.getElementById("detail-detail-tags");
  const voiceCharacters = [...new Set((file.voiceCharacters || []).map((tag) => String(tag || "").trim()).filter(Boolean))];
  const voiceTagsHtml = voiceCharacters.length > 0
    ? `<span class="tag voice-replacement-tag" title="按 sound/player 标准语音目录识别">语音替换：${voiceCharacters.map(escapeHtml).join("、")}</span>`
    : "";
  const secondaryTagsHtml =
    file.secondaryTags && file.secondaryTags.length > 0
      ? file.secondaryTags
          .map((tag) => `<span class="tag secondary-tag">${escapeHtml(tag)}</span>`)
          .join("")
      : "";
  detailTagsContainer.innerHTML = voiceTagsHtml + secondaryTagsHtml;
  renderTagEvidence(file, detailTagsContainer);
  void loadTagEvidenceIfMissing(file, detailTagsContainer);

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

// renderTagEvidence 展示"每个标签是怎么来的"（W6）。
// 只用创建元素 + textContent，避免 innerHTML 注入；证据缺失时整块不显示。
function renderTagEvidence(file, anchor) {
  const existing = document.getElementById("detail-tag-evidence");
  if (existing) existing.remove();
  if (!anchor) return;

  const { rows, hiddenCount } = buildTagEvidenceRows(file.tagEvidence);
  if (rows.length === 0) return;

  const block = document.createElement("div");
  block.id = "detail-tag-evidence";
  block.className = "detail-tag-evidence";

  const title = document.createElement("div");
  title.className = "detail-tag-evidence-title";
  title.textContent = "标签依据";
  block.appendChild(title);

  rows.forEach((item) => {
    const row = document.createElement("div");
    row.className = "detail-tag-evidence-row";

    const tag = document.createElement("span");
    tag.className = "detail-tag-evidence-tag";
    tag.textContent = item.tag;
    row.appendChild(tag);

    const level = document.createElement("span");
    level.className = "detail-tag-evidence-level";
    level.dataset.level = item.level;
    level.textContent = item.levelLabel;
    row.appendChild(level);

    const detail = document.createElement("span");
    detail.className = "detail-tag-evidence-detail";
    detail.textContent = item.detail;
    detail.title = detail.textContent;
    row.appendChild(detail);

    block.appendChild(row);
  });

  if (hiddenCount > 0) {
    const more = document.createElement("div");
    more.className = "detail-tag-evidence-more";
    more.textContent = `…另有 ${hiddenCount} 条`;
    block.appendChild(more);
  }

  anchor.insertAdjacentElement("afterend", block);
}

/**
 * loadTagEvidenceIfMissing 按需取「标签依据」。
 *
 * 列表 IPC（GetVPKFiles / SearchVPKFiles）刻意不再带 tagEvidence —— 真机上这份依据
 * 占列表 payload 的 35%（2904 个 Mod 合计 2.15MB，整包 6.11MB / 一次 IPC 340ms），
 * 而它只在详情弹窗里显示。所以打开详情时才对这一个 Mod 取一次。
 * 结果回来时如果用户已经换了文件或关了弹窗，就直接丢弃。
 */
async function loadTagEvidenceIfMissing(file, anchor) {
  if (!file?.path || !anchor) return;
  if (Array.isArray(file.tagEvidence) && file.tagEvidence.length > 0) return;
  const path = String(file.path);
  try {
    const evidence = await GetModEvidence(path);
    if (!evidence) return;
    if (String(currentDetailFile?.path || "") !== path) return;
    const modal = document.getElementById("file-detail-modal");
    if (!modal || modal.classList.contains("hidden")) return;
    if (!anchor.isConnected) return;
    renderTagEvidence({ ...file, tagEvidence: evidence.tagEvidence }, anchor);
  } catch (error) {
    // 依据取不到不影响详情的其它内容：静默降级，不弹错误。
    console.warn("读取标签依据失败:", error);
  }
}

export function closeModal() {
  const modal = document.getElementById("file-detail-modal");
  modal?.classList.add("hidden");
  restoreDetailModalState(modal);
  currentDetailFile = null;
}
