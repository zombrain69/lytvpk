import { appState, applyFileSelectionGesture } from "../state.js";
import {
  formatFileSize,
  getLocationDisplayName,
  getActionButton,
  formatTags,
  getUniqueDisplayTags,
  escapeHtml,
} from "../../core/utils.js";
import { showFileDetail } from "../modals/detail.js";
import { getServers } from "../servers/servers.js";
import {
  getCachedVPKPreview,
  loadVPKPreview,
} from "../shared/vpk-preview-cache.js";

let cardPreviewObserver = null;
const pendingCardPreviews = new Map();
const checkboxByPath = new Map();

function hasPanelServers() {
  return getServers().some((s) => s.panelUrl && s.panelPasswordSet);
}

function getGameStateInfo(file) {
  if (!file.gameStateKnown) {
    return { className: "game-state-unknown", label: "未记录", title: "addonlist.txt 中未记录此 Mod；点击可写入并开启" };
  }
  if (file.gameEnabled) {
    return { className: "game-state-enabled", label: "游戏内开启", title: "addonlist.txt：1；点击关闭游戏内 Mod" };
  }
  return { className: "game-state-disabled", label: "游戏内关闭", title: "addonlist.txt：0；点击开启游戏内 Mod" };
}

function applySelectionGesture(filePath, event, selected) {
  const ctrlEnabled = Boolean(appState.ctrlClickSelectionEnabled);
  const changedPaths = applyFileSelectionGesture(filePath, {
    shiftKey: Boolean(event.shiftKey),
    ctrlKey: ctrlEnabled && Boolean(event.ctrlKey),
    metaKey: ctrlEnabled && Boolean(event.metaKey),
    selected,
  });
  const pathsToSync = changedPaths instanceof Set ? changedPaths : new Set([filePath]);
  pathsToSync.forEach((path) => {
    const checkbox = checkboxByPath.get(path);
    if (checkbox) checkbox.checked = appState.selectedFiles.has(path);
  });
  // click 事件在浏览器完成 checkbox 预切换后触发；即使缓存映射暂时为空，
  // 也必须立即同步当前控件，避免用户看到“点击无反应”。
  const currentCheckbox = event.currentTarget?.classList?.contains("file-checkbox")
    ? event.currentTarget
    : event.target?.closest?.(".file-checkbox");
  if (currentCheckbox) currentCheckbox.checked = appState.selectedFiles.has(filePath);
}

function applyModStateClasses(element, file, prefix) {
  const state = getGameStateInfo(file).className.replace("game-state-", "");
  element.classList.add(`${prefix}-state-${state}`);
  if (file.location === "disabled") {
    element.classList.add(`${prefix}-location-disabled`);
  }
}

function getGameStateBadge(file, className = "game-state-badge") {
  const state = getGameStateInfo(file);
  return `<span class="${className} ${state.className}" title="${state.title}">${state.label}</span>`;
}

function getLoadOrderBadge(file, className = "load-order-badge") {
  const order = getLoadOrderValue(file);
  return Number.isInteger(order)
    ? `<span class="${className}" title="编号来自 addonlist.txt 加载顺序；数字越大表示越靠后加载。实际覆盖结果还取决于游戏资源与 Mod 规则">优先级 #${order + 1}</span>`
    : "";
}

function getLoadOrderValue(file) {
  if (!appState.loadOrderMap?.size) return undefined;
  const name = String(file?.name || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const path = String(file?.path || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const root = String(appState.currentDirectory || "").trim().replaceAll("/", "\\").replace(/^\.\\/, "").toLowerCase();
  const keys = [];
  if (root && path.startsWith(`${root}\\`)) keys.push(path.slice(root.length + 1));
  if (file?.location === "workshop" && name) keys.push(`workshop\\${name}`);
  if (file?.location === "disabled" && name) keys.push(`disabled\\${name}`);
  if (name) keys.push(name);
  return [...new Set(keys)]
    .map((key) => appState.loadOrderMap.get(key))
    .find((value) => Number.isInteger(value));
}

function getCardPreviewRevision(file) {
	const revision = String(file?.previewRevision || "").trim();
	if (revision) return revision;
	return `${String(file?.name || "")}\u0000${String(file?.size || "")}\u0000${String(file?.lastModified || "")}`;
}

// The signature contains every value rendered inside a card. Keeping this
// separate from the file object lets a scan return fresh objects without
// forcing Chromium to rebuild hundreds of unchanged image elements.
function getFileCardRenderSignature(file, panelServersAvailable) {
  const conflict = appState.conflictByPath?.get(file.path);
  return JSON.stringify({
    name: file.name,
    title: file.title,
    location: file.location,
    enabled: Boolean(file.enabled),
    gameStateKnown: Boolean(file.gameStateKnown),
    gameEnabled: Boolean(file.gameEnabled),
    hasUpdate: Boolean(file.hasUpdate),
    workshopId: file.workshopId || "",
    primaryTag: file.primaryTag || "",
    secondaryTags: file.secondaryTags || [],
    subjectSummary: file.subjectSummary || "",
    xdrSummary: file.xdrSummary || "",
    previewRevision: getCardPreviewRevision(file),
    loadOrder: getLoadOrderValue(file),
    conflictEnabled: Boolean(appState.conflictAnalysisEnabled),
    conflictLoading: Boolean(appState.conflictAnalysisLoading),
    conflict: conflict
      ? [conflict.groups, conflict.files, conflict.severity]
      : null,
    panelServersAvailable,
  });
}

function syncReusedCardSelection(card, file) {
  const checkbox = card.querySelector(".file-checkbox.card-checkbox");
  if (!checkbox) return;
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkboxByPath.set(file.path, checkbox);
}

function takeExistingCard(file, existingCards, cardsByIdentity) {
  const exact = existingCards.get(file.path);
  if (exact) {
    existingCards.delete(file.path);
    removeCardFromIdentityIndex(exact, cardsByIdentity);
    return exact;
  }

  const identity = getCardPreviewRevision(file);
  const candidates = cardsByIdentity.get(identity);
  while (candidates?.length) {
    const candidate = candidates.shift();
    if (!candidate) continue;
    const oldPath = candidate.dataset.path;
    if (existingCards.get(oldPath) !== candidate) continue;
    existingCards.delete(oldPath);
    return candidate;
  }
  return null;
}

function removeCardFromIdentityIndex(card, cardsByIdentity) {
  if (!card) return;
  const identity = card.dataset.cardIdentity || card.dataset.previewRevision;
  const candidates = cardsByIdentity.get(identity);
  if (!candidates) return;
  const index = candidates.indexOf(card);
  if (index >= 0) candidates.splice(index, 1);
  if (candidates.length === 0) cardsByIdentity.delete(identity);
}

function ensureCardPreviewObservation(card, file) {
  const image = card.querySelector(".card-preview-img");
  const placeholder = card.querySelector(".card-preview-placeholder");
  if (!image || !placeholder || image.getAttribute("src")) return;
  if (getCachedVPKPreview(file) === undefined) {
    observeCardPreview(card, file, image, placeholder);
  }
}

function getGameToggleButton(file) {
  const state = getGameStateInfo(file);
  const isFileDisabled = file.location === "disabled";
  const label = isFileDisabled ? "游戏开关不可用" : state.label;
  const title = isFileDisabled
    ? "文件位于 disabled 目录，请先恢复文件后再编辑 addonlist.txt"
    : state.title;
  return `
    <button class="btn-small action-btn game-toggle-btn ${state.className}"
            data-file-path="${file.path}" data-action="toggle-game"
            title="${title}" ${isFileDisabled ? "disabled" : ""}>
      <span class="btn-icon">${iconSvg("power")}</span>
      <span class="btn-text">${label}</span>
    </button>
  `;
}

function getConflictSummaryBadge(file, className = "mod-conflict-badge") {
  if (!appState.conflictAnalysisEnabled) return "";
  if (appState.conflictAnalysisLoading) {
    return `<span class="${className} pending" title="正在将当前筛选目标与全部游戏内开启 Mod 对比">冲突分析中…</span>`;
  }

  const summary = appState.conflictByPath?.get(file.path);
  if (!summary) {
    return `<span class="${className} none" title="未发现此 Mod 与任何游戏内开启 Mod 重叠的文件">无冲突</span>`;
  }

  const severityText =
    summary.severity === "critical"
      ? "严重"
      : summary.severity === "warning"
        ? "警告"
        : "普通";
  return `
    <button class="${className} has-conflict ${summary.severity}"
            data-action="view-conflicts"
            data-file-path="${escapeHtml(file.path)}"
            title="点击查看此 Mod 的冲突文件与风险分级">
      <span class="mod-conflict-dot" aria-hidden="true"></span>
      冲突 ${summary.groups} 组 · ${summary.files} 文件 · ${severityText}
    </button>
  `;
}

export function renderFileList() {
  const container = document.getElementById("file-list");
  const listHeader = document.querySelector(".file-list-header");
  const statusBar = document.querySelector(".status-bar");

  if (!container) return;
  checkboxByPath.clear();

  if (appState.displayMode === "card") {
    container.classList.add("file-list-grid");
    container.classList.remove("file-list");
    if (listHeader) listHeader.style.display = "none";
    if (statusBar) statusBar.style.display = "flex";

    const panelServersAvailable = hasPanelServers();
    const existingCards = new Map();
    const cardsByIdentity = new Map();
    const currentChildren = Array.from(container.children);
    currentChildren.forEach((child) => {
      if (child.classList?.contains("file-card") && child.dataset.path) {
        existingCards.set(child.dataset.path, child);
        const identity = child.dataset.cardIdentity || child.dataset.previewRevision;
        if (identity) {
          const candidates = cardsByIdentity.get(identity) || [];
          candidates.push(child);
          cardsByIdentity.set(identity, candidates);
        }
      }
    });

    const nextCards = appState.vpkFiles.map((file) => {
      const existingCard = takeExistingCard(file, existingCards, cardsByIdentity);
      const signature = getFileCardRenderSignature(file, panelServersAvailable);

      if (existingCard?.dataset.renderSignature === signature) {
        syncReusedCardSelection(existingCard, file);
        ensureCardPreviewObservation(existingCard, file);
        return existingCard;
      }

      // A changed card may still have an in-flight intersection-observer task.
      // Remove only this card's task; unchanged cards keep their observation
      // and do not churn the observer on every refresh/filter action.
      unobserveCardPreview(existingCard);
      const card = createFileCard(file, existingCard, panelServersAvailable);
      card.dataset.renderSignature = signature;
      return card;
    });

    // Any old cards not present in the next result must no longer retain an
    // observer entry. Their detached image nodes are otherwise kept alive by
    // pendingCardPreviews until the next full render.
    existingCards.forEach((card) => unobserveCardPreview(card));

    const orderUnchanged =
      currentChildren.length === nextCards.length &&
      currentChildren.every((card, index) => card === nextCards[index]);
    if (!orderUnchanged) {
      const fragment = document.createDocumentFragment();
      nextCards.forEach((card) => fragment.appendChild(card));
      container.replaceChildren(fragment);
    }
  } else {
    // Switching away from card mode detaches every observed card. Release
    // those observer entries immediately so the old card graph is collectible.
    clearPendingCardPreviews();
    container.classList.add("file-list");
    container.classList.remove("file-list-grid");
    if (listHeader) listHeader.style.display = "grid";
    if (statusBar) statusBar.style.display = "flex";

    const fragment = document.createDocumentFragment();
    appState.vpkFiles.forEach((file) => {
      fragment.appendChild(createFileItem(file));
    });
    container.replaceChildren(fragment);
  }
}

export function createFileItem(file) {
  const item = document.createElement("div");
  item.className = "file-item";
  item.dataset.path = file.path;
  applyModStateClasses(item, file, "mod-row");

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.className = "file-checkbox";
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkbox.addEventListener("click", function (event) {
    event.stopPropagation();
    applySelectionGesture(file.path, event, event.currentTarget.checked);
  });

  const displayTitle = file.title || file.name;
  const isHidden = file.name.startsWith("_");
  const hideBtnText = isHidden ? "取消隐藏" : "隐藏";
  const hideBtnIcon = isHidden ? iconSvg("eye") : iconSvg("eyeOff");
  const locationBadgeClass = `location-${file.location || "unknown"}`;
  const hasUpdate = file.hasUpdate;

  const updateTagHtml = hasUpdate
    ? `<span class="update-available-tag" data-workshop-id="${file.workshopId}" title="点击更新此Mod">待更新</span>`
    : "";

  // 列表模式：更新标签放在文件名后面

  const moreActionsHtml = `
    <div class="more-actions-dropdown">
      <button class="btn-small action-btn more-btn" title="更多操作">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
          <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z"/>
        </svg>
      </button>
      <div class="dropdown-content hidden">
        <button class="dropdown-item detail-btn" data-file-path="${file.path}">
          <span class="btn-icon">${iconSvg("info")}</span> 详情
        </button>
        ${file.workshopId ? `
        <button class="dropdown-item workshop-btn" data-file-path="${file.path}" data-workshop-id="${file.workshopId}">
          <span class="btn-icon">${iconSvg("external")}</span> 跳转工坊
        </button>
        <button class="dropdown-item share-workshop-btn" data-file-path="${file.path}" data-action="share-workshop">
          <span class="btn-icon">${iconSvg("share")}</span> 分享物品
        </button>
        ` : ""}
        <button class="dropdown-item set-tags-btn" data-file-path="${file.path}" data-action="set-tags">
          <span class="btn-icon">${iconSvg("tag")}</span> 设置标签
        </button>
        ${hasPanelServers() ? `
        <button class="dropdown-item upload-server-btn" data-file-path="${file.path}" data-action="upload-server">
          <span class="btn-icon">${iconSvg("upload")}</span>
          <span class="menu-item-text">上传服务器</span>
          <span class="menu-item-arrow">${iconSvg("chevronRight")}</span>
        </button>
        ` : ""}
        <button class="dropdown-item rename-btn" data-file-path="${file.path}" data-action="rename">
          <span class="btn-icon">${iconSvg("edit")}</span> 重命名
        </button>
        <button class="dropdown-item load-order-btn" data-file-path="${file.path}" data-action="load-order">
          <span class="btn-icon">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="10" y1="6" x2="21" y2="6"></line>
              <line x1="10" y1="12" x2="21" y2="12"></line>
              <line x1="10" y1="18" x2="21" y2="18"></line>
              <path d="M4 6h1v4"></path>
              <path d="M4 10h2"></path>
              <path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"></path>
            </svg>
          </span> 加载顺序
        </button>
        <button class="dropdown-item unpack-btn" data-file-path="${file.path}" data-action="unpack">
          <span class="btn-icon">${iconSvg("package")}</span> 解包
        </button>
        <button class="dropdown-item open-location-btn" data-file-path="${file.path}" data-action="open-location">
          <span class="btn-icon">${iconSvg("folderOpen")}</span> 打开位置
        </button>
        <button class="dropdown-item hide-btn" data-file-path="${file.path}" data-action="hide">
          <span class="btn-icon">${hideBtnIcon}</span> ${hideBtnText}
        </button>
        <div class="dropdown-divider"></div>
        <button class="dropdown-item delete-btn" data-file-path="${file.path}" data-action="delete">
          <span class="btn-icon">${iconSvg("trash")}</span> 删除
        </button>
      </div>
    </div>
  `;

  item.innerHTML = `
    <div class="file-checkbox-container"></div>
    <div class="file-name" title="${file.path}">
      <div class="file-title">${displayTitle}</div>
      <div class="file-filename">${file.name}${updateTagHtml}</div>
    </div>
    <div class="file-size">${formatFileSize(file.size)}</div>
    <div class="file-location">
      <span class="location-state-tag ${locationBadgeClass}">
        ${getLocationSvg(file.location)}
        <span>${getLocationDisplayName(file.location)}</span>
      </span>
    </div>
      <div class="file-game-state">${getGameStateBadge(file)}${getLoadOrderBadge(file)}</div>
    <div class="file-tags">
      ${formatTags(file.primaryTag, file.secondaryTags, file.voiceCharacters, file.subjectSummary, file.xdrSummary)}
      ${getConflictSummaryBadge(file)}
    </div>
    <div class="file-actions">
      <button class="btn-small action-btn detail-btn" data-file-path="${file.path}">
        <span class="btn-icon">${iconSvg("info")}</span>
        <span class="btn-text">详情</span>
      </button>
      ${getGameToggleButton(file)}
      ${getActionButton(file)}
      ${moreActionsHtml}
    </div>
  `;

  const checkboxContainer = item.querySelector(".file-checkbox-container");
  checkboxContainer.appendChild(checkbox);
  checkboxByPath.set(file.path, checkbox);
  // 只有真正点击 checkbox 才改变选择；点击容器空白不能穿透到行处理器。
  checkboxContainer.addEventListener("click", function (event) {
    event.stopPropagation();
  });

  item.addEventListener("click", function (e) {
    if (
      e.target.closest(".file-checkbox-container") ||
      e.target.closest(".file-actions") ||
      e.target.type === "checkbox" ||
      e.target.closest("button")
    ) {
      return;
    }

    if (e.shiftKey || (appState.ctrlClickSelectionEnabled && (e.ctrlKey || e.metaKey))) {
      e.preventDefault();
      e.stopPropagation();
      // Shift/Ctrl 点击行主体恢复桌面文件管理器的整行选择语义；
      // 普通点击仍不会因为点到文字或空白而误选。
      applySelectionGesture(file.path, e, !appState.selectedFiles.has(file.path));
    }
  });

  item.addEventListener("dblclick", function (e) {
    if (
      e.target.closest(".file-checkbox-container") ||
      e.target.closest(".file-actions") ||
      e.target.type === "checkbox" ||
      e.target.closest("button")
    ) {
      return;
    }
    e.preventDefault();
    e.stopPropagation();
    showFileDetail(file.path);
  });

  return item;
}

export function createFileCard(file, existingCard = null, panelServersAvailable = hasPanelServers()) {
  const card = existingCard || document.createElement("div");
  const previousPreview = existingCard?.querySelector(".card-preview-img") || null;
  const previewRevision = getCardPreviewRevision(file);
  const canPreservePreview = Boolean(
    previousPreview &&
      existingCard.dataset.previewRevision === previewRevision,
  );

  card.className = "file-card";
  card.dataset.path = file.path;
  card.dataset.previewRevision = previewRevision;
  card.dataset.cardIdentity = previewRevision;
  applyModStateClasses(card, file, "mod-card");

  if (!file.enabled) {
    card.classList.add("disabled");
  }

  const displayTitle = file.title || file.name;
  const isHidden = file.name.startsWith("_");
  const hideBtnText = isHidden ? "取消隐藏" : "隐藏";
  const hideBtnIcon = isHidden ? iconSvg("eye") : iconSvg("eyeOff");
  const hasUpdate = file.hasUpdate;

  const updateBtnHtml = hasUpdate
    ? `<button class="btn-small action-btn update-btn" data-workshop-id="${file.workshopId}" title="点击更新此Mod">
        <span class="btn-icon">
          <svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
            <polyline points="7 10 12 15 17 10"></polyline>
            <line x1="12" y1="15" x2="12" y2="3"></line>
          </svg>
        </span>
        <span class="btn-text">待更新</span>
      </button>`
    : "";

  const cachedPreview = getCachedVPKPreview(file);
  const previewSrc = canPreservePreview
    ? previousPreview.getAttribute("src")
    : cachedPreview || "";
  // An empty src points an <img> at the current document in some browsers.
  // Omit the attribute until the queued preview request actually returns.
  const previewSrcAttribute = previewSrc ? ` src="${previewSrc}"` : "";
  const showPlaceholder = !previewSrc;

  let secondaryTagsHtml = "";
  const subjectSummary = String(file.subjectSummary || "").trim();
  const xdrSummary = String(file.xdrSummary || "").trim();
  const subjectBadgeHtml = subjectSummary
    ? `<span class="card-badge subject-badge" title="${escapeHtml(subjectSummary)}">${escapeHtml(subjectSummary)}</span>`
    : "";
  const xdrBadgeHtml = xdrSummary
    ? `<span class="card-badge xdr-badge" title="${escapeHtml(xdrSummary)}">${escapeHtml(xdrSummary)}</span>`
    : "";
  const uniqueDisplayTags = getUniqueDisplayTags(file.primaryTag, file.secondaryTags);
  if (uniqueDisplayTags.secondary.length > 0) {
    const displayTags = uniqueDisplayTags.secondary.slice(0, 2);
    const hasMore = uniqueDisplayTags.secondary.length > 2;

    secondaryTagsHtml = displayTags
      .map((tag) => {
        const longTagClass = tag.length > 16 ? " is-long" : "";
        return `<span class="card-badge secondary-tag-badge${longTagClass}" title="${escapeHtml(tag)}">${escapeHtml(tag)}</span>`;
      })
      .join("");

    if (hasMore) {
      secondaryTagsHtml += `<span class="card-badge more-tag-badge" title="${uniqueDisplayTags.secondary
        .slice(2)
        .map(escapeHtml)
        .join(", ")}">+${uniqueDisplayTags.secondary.length - 2}</span>`;
    }
  }

  let actionBtn = "";
  if (file.location === "workshop") {
    actionBtn = `
      <button class="btn-small action-btn move-btn" data-file-path="${file.path}" data-action="move" title="复制到 addons">
        <span class="btn-icon">${iconSvg("package")}</span>
        <span class="btn-text">复制到 addons</span>
      </button>
    `;
  } else {
    actionBtn = `
      <button class="btn-small action-btn toggle-btn ${file.enabled ? "toggle-disable" : "toggle-enable"}"
              data-file-path="${file.path}" data-action="toggle"
              title="${file.enabled ? "点击禁用" : "点击启用"}">
        <span class="btn-icon">${iconSvg("power")}</span>
        <span class="btn-text">${file.enabled ? "禁用" : "启用"}</span>
      </button>
    `;
  }

  const moreActionsHtml = `
    <div class="more-actions-dropdown">
      <button class="btn-small action-btn more-btn" title="更多操作">
        <svg viewBox="0 0 24 24" width="16" height="16" fill="currentColor">
          <path d="M12 8c1.1 0 2-.9 2-2s-.9-2-2-2-2 .9-2 2 .9 2 2 2zm0 2c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2zm0 6c-1.1 0-2 .9-2 2s.9 2 2 2 2-.9 2-2-.9-2-2-2z"/>
        </svg>
      </button>
      <div class="dropdown-content hidden">
        <button class="dropdown-item detail-btn" data-file-path="${file.path}">
          <span class="btn-icon">${iconSvg("info")}</span> 详情
        </button>
        ${file.workshopId ? `
        <button class="dropdown-item workshop-btn" data-file-path="${file.path}" data-workshop-id="${file.workshopId}">
          <span class="btn-icon">${iconSvg("external")}</span> 跳转工坊
        </button>
        <button class="dropdown-item share-workshop-btn" data-file-path="${file.path}" data-action="share-workshop">
          <span class="btn-icon">${iconSvg("share")}</span> 分享物品
        </button>
        ` : ""}
        <button class="dropdown-item set-tags-btn" data-file-path="${file.path}" data-action="set-tags">
          <span class="btn-icon">${iconSvg("tag")}</span> 设置标签
        </button>
        ${panelServersAvailable ? `
        <button class="dropdown-item upload-server-btn" data-file-path="${file.path}" data-action="upload-server">
          <span class="btn-icon">${iconSvg("upload")}</span>
          <span class="menu-item-text">上传服务器</span>
          <span class="menu-item-arrow">${iconSvg("chevronRight")}</span>
        </button>
        ` : ""}
        <button class="dropdown-item rename-btn" data-file-path="${file.path}" data-action="rename">
          <span class="btn-icon">${iconSvg("edit")}</span> 重命名
        </button>
        <button class="dropdown-item load-order-btn" data-file-path="${file.path}" data-action="load-order">
          <span class="btn-icon">
            <svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <line x1="10" y1="6" x2="21" y2="6"></line>
              <line x1="10" y1="12" x2="21" y2="12"></line>
              <line x1="10" y1="18" x2="21" y2="18"></line>
              <path d="M4 6h1v4"></path>
              <path d="M4 10h2"></path>
              <path d="M6 18H4c0-1 2-2 2-3s-1-1.5-2-1"></path>
            </svg>
          </span> 加载顺序
        </button>
        <button class="dropdown-item unpack-btn" data-file-path="${file.path}" data-action="unpack">
          <span class="btn-icon">${iconSvg("package")}</span> 解包
        </button>
        <button class="dropdown-item open-location-btn" data-file-path="${file.path}" data-action="open-location">
          <span class="btn-icon">${iconSvg("folderOpen")}</span> 打开位置
        </button>
        <button class="dropdown-item hide-btn" data-file-path="${file.path}" data-action="hide">
          <span class="btn-icon">${hideBtnIcon}</span> ${hideBtnText}
        </button>
        <div class="dropdown-divider"></div>
        <button class="dropdown-item delete-btn" data-file-path="${file.path}" data-action="delete">
          <span class="btn-icon">${iconSvg("trash")}</span> 删除
        </button>
      </div>
    </div>
  `;

  card.innerHTML = `
    <div class="card-preview-container">
      <div class="card-preview-placeholder ${showPlaceholder ? "" : "hidden"}">
        <svg class="icon-svg placeholder-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
          <circle cx="8.5" cy="8.5" r="1.5"></circle>
          <polyline points="21 15 16 10 5 21"></polyline>
        </svg>
      </div>
      <img class="card-preview-img ${showPlaceholder ? "hidden" : ""}"${previewSrcAttribute} alt="${displayTitle}" loading="lazy" decoding="async" fetchpriority="low" />
      <div class="card-checkbox-container"></div>
      <div class="card-badges">
        <span class="card-badge location-badge">${getLocationDisplayName(file.location)}</span>
        ${getGameStateBadge(file, "card-badge game-state-badge")}
        ${getLoadOrderBadge(file, "card-badge load-order-badge")}
        ${
          file.primaryTag
            ? `<span class="card-badge tag-badge" title="${escapeHtml(file.primaryTag)}">${escapeHtml(file.primaryTag)}</span>`
            : ""
        }
        ${xdrBadgeHtml}${subjectBadgeHtml}
        ${secondaryTagsHtml}
        ${getConflictSummaryBadge(file, "card-badge mod-conflict-badge")}
      </div>
    </div>
    <div class="card-content">
      <div class="card-title" title="${displayTitle}">${displayTitle}</div>
      <div class="card-filename" title="${file.name}">${file.name}</div>
      <div class="card-actions">
        <div class="card-actions-left">
          ${getGameToggleButton(file)}
          ${actionBtn}
          ${updateBtnHtml}
        </div>
        ${moreActionsHtml}
      </div>
    </div>
  `;

  const checkbox = document.createElement("input");
  checkbox.type = "checkbox";
  checkbox.className = "file-checkbox card-checkbox";
  checkbox.checked = appState.selectedFiles.has(file.path);
  checkbox.addEventListener("click", function (event) {
    event.stopPropagation();
    applySelectionGesture(file.path, event, event.currentTarget.checked);
  });
  const checkboxContainer = card.querySelector(".card-checkbox-container");
  checkboxContainer.appendChild(checkbox);
  checkboxByPath.set(file.path, checkbox);
  checkboxContainer.addEventListener("click", function (e) {
    e.stopPropagation();
  });

  let img = card.querySelector(".card-preview-img");
  const placeholder = card.querySelector(".card-preview-placeholder");

  if (canPreservePreview) {
    const renderedPreview = img;
    previousPreview.className = renderedPreview.className;
    previousPreview.alt = displayTitle;
    previousPreview.loading = "lazy";
    previousPreview.decoding = "async";
    renderedPreview.replaceWith(previousPreview);
    img = previousPreview;
  }

  // A preserved image can still be waiting for its first load (for example
  // when a Mod moved between addons and disabled). Re-observe it when there is
  // no cached result and no source yet; otherwise the move would preserve a
  // blank placeholder without ever starting the request again.
  if (cachedPreview === undefined && !img.getAttribute("src")) {
    observeCardPreview(card, file, img, placeholder);
  }

  if (!existingCard) {
    card.addEventListener("click", function (e) {
      if (
        e.target.closest("button") ||
        e.target.closest(".more-actions-dropdown") ||
        e.target.closest(".card-checkbox-container")
      ) {
        return;
      }

      if (e.shiftKey || (appState.ctrlClickSelectionEnabled && (e.ctrlKey || e.metaKey))) {
        e.preventDefault();
        e.stopPropagation();
        // Shift/Ctrl 点击卡片主体也可以选择；普通点击仍打开详情。
        applySelectionGesture(file.path, e, !appState.selectedFiles.has(file.path));
        return;
      }

      showFileDetail(file.path);
    });
  }

  return card;
}

export function iconSvg(name) {
  const icons = {
    search: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="11" cy="11" r="7"></circle><path d="m20 20-3.5-3.5"></path></svg>`,
    info: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="M12 11v5"></path><path d="M12 8h.01"></path></svg>`,
    external: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 3h6v6"></path><path d="M10 14 21 3"></path><path d="M21 14v5a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5"></path></svg>`,
    eye: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7-10-7-10-7Z"></path><circle cx="12" cy="12" r="3"></circle></svg>`,
    eyeOff: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m3 3 18 18"></path><path d="M10.6 10.6A2 2 0 0 0 13.4 13.4"></path><path d="M9.9 4.2A10.4 10.4 0 0 1 12 4c6.5 0 10 8 10 8a18 18 0 0 1-2.2 3.2"></path><path d="M6.6 6.6C3.6 8.6 2 12 2 12s3.5 8 10 8a10.6 10.6 0 0 0 4.1-.8"></path></svg>`,
    tag: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0L3 13V3h10l7.6 7.6a2 2 0 0 1 0 2.8Z"></path><circle cx="7.5" cy="7.5" r=".8"></circle></svg>`,
    edit: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 20h9"></path><path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z"></path></svg>`,
    folderOpen: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M6 14 8 8h13l-2 8a2 2 0 0 1-2 1.5H5a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h5l2 2h4"></path></svg>`,
    trash: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18"></path><path d="M8 6V4h8v2"></path><path d="m19 6-1 14H6L5 6"></path><path d="M10 11v5"></path><path d="M14 11v5"></path></svg>`,
    package: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16Z"></path><path d="m3.3 7 8.7 5 8.7-5"></path><path d="M12 22V12"></path></svg>`,
    power: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 2v10"></path><path d="M18.4 6.6a9 9 0 1 1-12.8 0"></path></svg>`,
    check: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 6 9 17l-5-5"></path></svg>`,
    x: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M18 6 6 18"></path><path d="m6 6 12 12"></path></svg>`,
    chevronRight: `<svg class="icon-svg submenu-arrow" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m9 18 6-6-6-6"></path></svg>`,
    upload: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path><polyline points="17 8 12 3 7 8"></polyline><line x1="12" y1="3" x2="12" y2="15"></line></svg>`,
    share: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="18" cy="5" r="3"></circle><circle cx="6" cy="12" r="3"></circle><circle cx="18" cy="19" r="3"></circle><path d="m8.6 10.5 6.8-4"></path><path d="m8.6 13.5 6.8 4"></path></svg>`,
  };
  return icons[name] || "";
}

export function getLocationSvg(location) {
  if (location === "workshop") {
    return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M4 7h16"></path><path d="M5 7l1-3h12l1 3"></path><path d="M6 7v12h12V7"></path><path d="M9 11h6"></path></svg>`;
  }
  if (location === "disabled") {
    return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"></circle><path d="m5.7 5.7 12.6 12.6"></path></svg>`;
  }
  return `<svg class="location-tag-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.1" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 7h7l2 2h9v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z"></path><path d="M3 7V5a2 2 0 0 1 2-2h4l2 2h4"></path></svg>`;
}

export async function loadCardPreview(file, imgElement) {
  try {
    const imgData = await loadVPKPreview(file);
    // A card can refresh while an archive read is running. The image element is
    // deliberately preserved across that refresh; resolve the current
    // placeholder at completion instead of holding a stale DOM reference.
    if (imgData && imgElement.isConnected) {
      imgElement.src = imgData;
      imgElement.classList.remove("hidden");
      imgElement
        .closest(".card-preview-container")
        ?.querySelector(".card-preview-placeholder")
        ?.classList.add("hidden");
    }
  } catch (err) {
    console.warn("加载预览图失败:", file.name);
  }
}

function getCardPreviewObserver() {
  if (cardPreviewObserver) return cardPreviewObserver;
  cardPreviewObserver = new IntersectionObserver((entries) => {
    entries.forEach((entry) => {
      if (!entry.isIntersecting) return;
      const target = pendingCardPreviews.get(entry.target);
      cardPreviewObserver.unobserve(entry.target);
      pendingCardPreviews.delete(entry.target);
      if (target) {
        void loadCardPreview(target.file, target.img);
      }
    });
  }, {
    root: document.getElementById("file-list") || null,
    // Start a little before the card reaches the viewport so rapid scrolling
    // does not expose placeholders, while the bounded queue keeps I/O calm.
    rootMargin: "320px 0px",
    threshold: 0.01,
  });
  return cardPreviewObserver;
}

function observeCardPreview(card, file, img, placeholder) {
  pendingCardPreviews.set(card, { file, img, placeholder });
  getCardPreviewObserver().observe(card);
}

function unobserveCardPreview(card) {
  if (!card) return;
  if (cardPreviewObserver) {
    cardPreviewObserver.unobserve(card);
  }
  pendingCardPreviews.delete(card);
}

function clearPendingCardPreviews() {
  if (!cardPreviewObserver) return;
  pendingCardPreviews.forEach((_, card) => {
    cardPreviewObserver.unobserve(card);
  });
  pendingCardPreviews.clear();
}
