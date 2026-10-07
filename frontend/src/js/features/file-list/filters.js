import { appState, updateStatusBar, showFileListLoading, hideFileListLoading } from "../state.js";
import { showError, showNotification } from "../../core/toast.js";
import { renderFileList } from "./render.js";
import { getLocationDisplayName, escapeHtml } from "../../core/utils.js";
import {
  applySort,
  ensureVisibleModelMetrics,
  saveSortPreference,
  updateSortButtonUI,
  refreshLoadOrderMap,
} from "./sorting.js";
import { resetBoxSelection } from "./box-selection.js";
import { scheduleScopedConflictAnalysis } from "../conflicts/conflicts.js";
import { fileMatchesGroupFilter } from "../mod-groups/group-view.mjs";
import { refreshModGroupMembershipState } from "../mod-groups/group-state.mjs";
import { refreshCategorySidebar, syncCategorySidebarSelection } from "./category-tree.js";
import { PRESET_GROUPS as SECONDARY_TAG_PRESET_GROUPS } from "./category-tree.mjs";
import { GetPrimaryTags, GetSecondaryTags, GetSecondaryTagCounts, SearchVPKFiles, ScanVPKFiles, GetVPKFiles } from "../../../../wailsjs/go/app/App";
import { compareSecondaryTagsByUsage } from "./secondary-tag-usage.mjs";

const LOCATION_FILTERS = ["root", "workshop", "disabled"];
const GAME_STATE_FILTERS = ["enabled", "disabled", "unknown"];
const SEARCH_INPUT_DEBOUNCE_MS = 160;
const FILE_LIST_IDLE_POLL_MS = 32;

let searchInputTimer = null;
// Refreshes are requested by file moves, game-state changes, VPK tools and
// addonlist operations. They must not be lost just because a scan or filter
// reset is still in progress. Keep one caller-visible Promise and only add a
// follow-up scan when a request arrives after the current scan has begun.
let refreshFilesPromise = null;
let refreshFilesPhase = "idle";
let refreshFilesNeedsFollowUp = false;
// 加载遮罩当前是否由"刷新"显示（静默轮次不显示；用户显式刷新会把它升格成显示）。
let refreshFilesLoadingVisible = false;
let searchRequestId = 0;
let tagFilterRenderId = 0;
let secondaryTagRenderId = 0;


function getGameStateDisplayName(state) {
  switch (state) {
    case "enabled":
      return "游戏内开启";
    case "disabled":
      return "游戏内关闭";
    default:
      return "未记录";
  }
}

function getGameState(file) {
  // disabled 目录中的 VPK 不会被游戏实际加载，不能被残留的 addonlist=1
  // 误判为“游戏内开启”；与后端冲突分析的 enabled 规则保持一致。
  if (file.enabled === false) return "disabled";
  if (!file.gameStateKnown) return "unknown";
  return file.gameEnabled ? "enabled" : "disabled";
}

function snapshotFilterState() {
  return {
    searchQuery: String(appState.searchQuery || ""),
    primaryTag: String(appState.selectedPrimaryTag || ""),
    secondaryTags: [...(appState.selectedSecondaryTags || [])],
    secondaryMatchMode: appState.secondaryMatchMode === "all" ? "all" : "any",
    locations: [...(appState.selectedLocations || [])],
    gameStates: [...(appState.selectedGameStates || [])],
    groups: [...(appState.activeGroupFilter || [])],
    showHidden: Boolean(appState.showHidden),
  };
}

function filterStateStillMatches(snapshot) {
  const current = snapshotFilterState();
  return (
    current.searchQuery === snapshot.searchQuery &&
    current.primaryTag === snapshot.primaryTag &&
    current.secondaryMatchMode === snapshot.secondaryMatchMode &&
    current.showHidden === snapshot.showHidden &&
    current.secondaryTags.join("\u0000") === snapshot.secondaryTags.join("\u0000") &&
    current.locations.join("\u0000") === snapshot.locations.join("\u0000") &&
    current.gameStates.join("\u0000") === snapshot.gameStates.join("\u0000") &&
    current.groups.join("\u0000") === snapshot.groups.join("\u0000")
  );
}

function isCurrentSecondaryTagRender(secondaryGroup, renderId) {
  return (
    renderId === secondaryTagRenderId &&
    document.getElementById("secondary-tag-group") === secondaryGroup
  );
}

function getSecondaryMatchModeLabel() {
  return appState.secondaryMatchMode === "all" ? "全部匹配" : "任一匹配";
}

function getSelectedSecondaryLabel() {
  const selected = appState.selectedSecondaryTags || [];
  if (selected.length === 0) return "全部";
  if (selected.length <= 2) return selected.join("、");
  return `${selected.slice(0, 2).join("、")} 等 ${selected.length} 个`;
}

function uniqueSecondaryTags(tags) {
  return [...new Set((tags || []).map((tag) => String(tag || "").trim()).filter(Boolean))];
}

function setSelectedSecondaryTags(tags) {
  appState.selectedSecondaryTags = uniqueSecondaryTags(tags);
}

function getPresetAggregateTags(group) {
  return [group.allTag, ...(group.groups || []).map((item) => item.allTag)].filter(Boolean);
}

function getPresetGroupTags(group) {
  return uniqueSecondaryTags([
    ...getPresetAggregateTags(group),
    ...(group.groups || []).flatMap((item) => item.tags || []),
  ]);
}

function getPresetGroupForTag(tag) {
  return SECONDARY_TAG_PRESET_GROUPS.find((group) => getPresetGroupTags(group).includes(tag));
}

function syncSecondaryTagFilterUI() {
  const selected = new Set(appState.selectedSecondaryTags || []);

  document.querySelectorAll("[data-secondary-tag], [data-preset-tag]").forEach((control) => {
    const tag = control.dataset.secondaryTag || control.dataset.presetTag;
    const isSelected = selected.has(tag);
    if (control instanceof HTMLInputElement) {
      control.checked = isSelected;
    } else {
      control.classList.toggle("active", isSelected);
    }
  });

  document.querySelectorAll(".secondary-filter-dropdown .multi-select-trigger").forEach((trigger) => {
    trigger.textContent = getSelectedSecondaryLabel();
    trigger.classList.toggle("has-selection", selected.size > 0);
    trigger.setAttribute("aria-label", selected.size > 0 ? `子标签筛选，已选 ${selected.size} 项` : "子标签筛选，未选择");
  });
  document.querySelectorAll(".secondary-match-mode-btn").forEach((button) => {
    button.textContent = `匹配方式：${getSecondaryMatchModeLabel()}`;
  });
  renderActiveFilterSummary();
}

function setSecondaryTagChecked(tag, checked) {
  const selected = new Set(appState.selectedSecondaryTags || []);
  if (checked) {
    // 从“查看全部”细化到具体项目时，去掉同一集合的聚合标签，避免任一匹配下
    // 聚合标签把单件选择完全覆盖掉。
    const presetGroup = getPresetGroupForTag(tag);
    if (presetGroup) {
      if (tag === presetGroup.allTag) {
        getPresetGroupTags(presetGroup).forEach((groupTag) => selected.delete(groupTag));
      } else {
        getPresetAggregateTags(presetGroup).forEach((aggregateTag) => {
          if (aggregateTag !== tag) selected.delete(aggregateTag);
        });
      }
    }
    selected.add(tag);
  } else {
    selected.delete(tag);
  }
  setSelectedSecondaryTags([...selected]);
  syncSecondaryTagFilterUI();
  performSearch();
}

function selectPresetAggregate(tag) {
  if (!tag) return;
  // “查看全部”是独立预设：替换旧的二级标签，保证一键查看的结果可预期。
  setSelectedSecondaryTags([tag]);
  syncSecondaryTagFilterUI();
  performSearch();
}

function clearPresetGroup(group) {
  const groupTags = new Set(getPresetGroupTags(group));
  setSelectedSecondaryTags((appState.selectedSecondaryTags || []).filter((tag) => !groupTags.has(tag)));
  syncSecondaryTagFilterUI();
  performSearch();
}

function createActiveFilterChip(label, onClear) {
  const chip = document.createElement("button");
  chip.type = "button";
  chip.className = "active-filter-chip";
  chip.title = `取消筛选：${label}`;

  const text = document.createElement("span");
  text.className = "active-filter-chip-text";
  text.textContent = label;

  const close = document.createElement("span");
  close.className = "active-filter-chip-close";
  close.setAttribute("aria-hidden", "true");
  close.textContent = "×";

  chip.append(text, close);
  chip.addEventListener("click", async (event) => {
    event.stopPropagation();
    await onClear();
  });
  return chip;
}

function syncPrimaryTagButtons() {
  document.querySelectorAll(".primary-tag-btn").forEach((button) => {
    button.classList.toggle("active", button.dataset.value === appState.selectedPrimaryTag);
  });
}

async function clearAllActiveFilters() {
  const searchInput = document.getElementById("search-input");
  if (searchInput) searchInput.value = "";

  appState.searchQuery = "";
  appState.selectedPrimaryTag = "";
  appState.selectedSecondaryTags = [];
  appState.secondaryMatchMode = "any";
  appState.selectedLocations = [];
  appState.selectedGameStates = [];

  updatePrimaryTagDropdownUI();
  updateLocationFilterDropdownUI();
  updateGameStateFilterDropdownUI();
  await renderSecondaryTags("");
  syncSecondaryTagFilterUI();
  await performSearch();
}

function renderActiveFilterSummary() {
  const container = document.getElementById("active-filter-summary");
  if (!container) return;

  const filterChips = [];
  const searchText = String(appState.searchQuery || "").trim();
  if (searchText) {
    const displayText = searchText.length > 36 ? `${searchText.slice(0, 36)}…` : searchText;
    filterChips.push(
      createActiveFilterChip(`搜索：${displayText}`, async () => {
        const searchInput = document.getElementById("search-input");
        if (searchInput) searchInput.value = "";
        appState.searchQuery = "";
        await performSearch();
      }),
    );
  }

  if (appState.selectedPrimaryTag) {
    filterChips.push(
      createActiveFilterChip(`标签：${appState.selectedPrimaryTag}`, async () => {
        appState.selectedPrimaryTag = "";
        updatePrimaryTagDropdownUI();
        await renderSecondaryTags("");
        await performSearch();
      }),
    );
  }

  (appState.selectedSecondaryTags || []).forEach((tag) => {
    const modePrefix = appState.secondaryMatchMode === "all" ? "子标签（全部）" : "子标签";
    filterChips.push(
      createActiveFilterChip(`${modePrefix}：${tag}`, async () => {
        setSelectedSecondaryTags(appState.selectedSecondaryTags.filter((item) => item !== tag));
        syncSecondaryTagFilterUI();
        await performSearch();
      }),
    );
  });

  (appState.selectedLocations || []).forEach((location) => {
    filterChips.push(
      createActiveFilterChip(`位置：${getLocationDisplayName(location)}`, async () => {
        appState.selectedLocations = appState.selectedLocations.filter((item) => item !== location);
        updateLocationFilterDropdownUI();
        await performSearch();
      }),
    );
  });

  (appState.selectedGameStates || []).forEach((state) => {
    filterChips.push(
      createActiveFilterChip(`游戏内：${getGameStateDisplayName(state)}`, async () => {
        appState.selectedGameStates = appState.selectedGameStates.filter((item) => item !== state);
        updateGameStateFilterDropdownUI();
        await performSearch();
      }),
    );
  });

  container.replaceChildren();
  container.classList.toggle("hidden", filterChips.length === 0);
  if (filterChips.length === 0) return;

  const title = document.createElement("span");
  title.className = "active-filter-summary-title";
  title.textContent = `已筛选 ${filterChips.length} 项`;

  const chipList = document.createElement("div");
  chipList.className = "active-filter-chip-list";
  filterChips.forEach((chip) => chipList.appendChild(chip));

  const clearAllButton = document.createElement("button");
  clearAllButton.type = "button";
  clearAllButton.className = "active-filter-clear-all";
  clearAllButton.textContent = "清空筛选";
  clearAllButton.addEventListener("click", async () => {
    await clearAllActiveFilters();
  });

  container.append(title, chipList, clearAllButton);
}

function matchesSecondaryTags(file, selected = appState.selectedSecondaryTags || []) {
  if (selected.length === 0) return true;
  const available = new Set((file.secondaryTags || []).map((tag) => String(tag)));
  if (appState.secondaryMatchMode === "all") {
    return selected.every((tag) => available.has(String(tag)));
  }
  return selected.some((tag) => available.has(String(tag)));
}

function addSecondaryMatchModeControl(container) {
  if (!container) return;
  container.querySelectorAll(".secondary-match-mode-btn").forEach((button) => button.remove());

  const button = document.createElement("button");
  button.type = "button";
  button.className = "secondary-match-mode-btn";
  button.textContent = `匹配方式：${getSecondaryMatchModeLabel()}`;
  button.title = "切换二级标签筛选：任一匹配 / 全部匹配";
  button.addEventListener("click", (event) => {
    event.stopPropagation();
    appState.secondaryMatchMode = appState.secondaryMatchMode === "all" ? "any" : "all";
    addSecondaryMatchModeControl(container);
    performSearch();
  });
  container.appendChild(button);
}

function createPresetTagCheckbox(tag, displayLabel = tag) {
  const option = document.createElement("label");
  option.className = "preset-tag-option";

  const input = document.createElement("input");
  input.type = "checkbox";
  input.dataset.presetTag = tag;
  input.checked = appState.selectedSecondaryTags.includes(tag);
  input.addEventListener("change", (event) => {
    event.stopPropagation();
    setSecondaryTagChecked(tag, event.target.checked);
  });

  const text = document.createElement("span");
  text.textContent = displayLabel;
  if (displayLabel !== tag) text.title = tag;
  option.append(input, text);
  return option;
}

function createPresetSubgroup(subgroup) {
  const section = document.createElement("section");
  section.className = "preset-subgroup";

  const header = document.createElement("div");
  header.className = "preset-subgroup-header";

  const expandButton = document.createElement("button");
  expandButton.type = "button";
  expandButton.className = "preset-subgroup-expand";
  expandButton.textContent = subgroup.label;
  expandButton.title = `展开 ${subgroup.label} 的具体项目`;
  expandButton.setAttribute("aria-expanded", "false");

  const items = document.createElement("div");
  items.className = "preset-tag-options";
  items.hidden = true;
  (subgroup.tags || []).forEach((tag) => {
    items.appendChild(createPresetTagCheckbox(tag, subgroup.tagLabels?.[tag] || tag));
  });

  expandButton.addEventListener("click", (event) => {
    event.stopPropagation();
    const willExpand = items.hidden;
    items.hidden = !willExpand;
    expandButton.classList.toggle("is-expanded", willExpand);
    expandButton.setAttribute("aria-expanded", String(willExpand));
  });
  header.appendChild(expandButton);

  if (subgroup.allTag) {
    const allButton = document.createElement("button");
    allButton.type = "button";
    allButton.className = "preset-subgroup-all";
    allButton.dataset.presetTag = subgroup.allTag;
    allButton.textContent = "查看全部";
    allButton.title = `只查看${subgroup.label}`;
    allButton.addEventListener("click", (event) => {
      event.stopPropagation();
      selectPresetAggregate(subgroup.allTag);
    });
    header.appendChild(allButton);
  }

  section.append(header, items);
  return section;
}

function createPresetGroup(group) {
  const section = document.createElement("section");
  section.className = "preset-filter-group";
  section.dataset.presetGroup = group.id;

  const header = document.createElement("div");
  header.className = "preset-filter-group-header";

  const expandButton = document.createElement("button");
  expandButton.type = "button";
  expandButton.className = "preset-filter-group-expand";
  expandButton.textContent = group.label;
  expandButton.title = group.description
    ? `展开 ${group.label} 的分类与具体项目：${group.description}`
    : `展开 ${group.label} 的分类与具体项目`;
  expandButton.setAttribute("aria-expanded", "false");

  const selectedCount = document.createElement("span");
  selectedCount.className = "preset-group-selected-count";
  selectedCount.textContent = "未选择";
  expandButton.appendChild(selectedCount);

  const content = document.createElement("div");
  content.className = "preset-filter-group-content";
  content.hidden = true;
  (group.groups || []).forEach((subgroup) => content.appendChild(createPresetSubgroup(subgroup)));

  expandButton.addEventListener("click", (event) => {
    event.stopPropagation();
    const willExpand = content.hidden;
    content.hidden = !willExpand;
    expandButton.classList.toggle("is-expanded", willExpand);
    expandButton.setAttribute("aria-expanded", String(willExpand));
  });
  header.appendChild(expandButton);

  if (group.allTag) {
    const allButton = document.createElement("button");
    allButton.type = "button";
    allButton.className = "preset-group-all";
    allButton.dataset.presetTag = group.allTag;
    allButton.textContent = "查看全部";
    allButton.title = `一键只查看${group.label}`;
    allButton.addEventListener("click", (event) => {
      event.stopPropagation();
      selectPresetAggregate(group.allTag);
    });
    header.appendChild(allButton);
  }

  const clearButton = document.createElement("button");
  clearButton.type = "button";
  clearButton.className = "preset-group-clear";
  clearButton.textContent = "清空本组";
  clearButton.title = `清除已选择的${group.label}标签`;
  clearButton.addEventListener("click", (event) => {
    event.stopPropagation();
    clearPresetGroup(group);
  });
  header.appendChild(clearButton);

  section.append(header, content);
  return section;
}

function createFilterFlyoutHeader(title, description) {
  const header = document.createElement("div");
  header.className = "filter-flyout-header";

  const copy = document.createElement("div");
  copy.className = "filter-flyout-header-copy";
  const heading = document.createElement("strong");
  heading.textContent = title;
  const hint = document.createElement("span");
  hint.textContent = description;
  copy.append(heading, hint);
  header.appendChild(copy);
  return header;
}

document.addEventListener("app:page-change", (event) => {
  if (event.detail?.page === "mods") {
    requestAnimationFrame(updateClassicSecondaryTagsCollapse);
  }
});

window.addEventListener("resize", () => {
  requestAnimationFrame(updateClassicSecondaryTagsCollapse);
  requestAnimationFrame(repositionOpenFilterFlyoutMenus);
});

export async function renderTagFilters() {
  const tagContainer = document.getElementById("tag-filters");
  const locationContainer = document.getElementById("location-filter-section");
  const filterRow = tagContainer?.closest(".filter-row-filters");
  const renderId = ++tagFilterRenderId;

  if (!tagContainer || !locationContainer) return;
  tagContainer.innerHTML = "";
  locationContainer.innerHTML = "";
  const classicLayout = appState.filterLayoutMode === "classic";
  filterRow?.classList.toggle("filter-layout-classic", classicLayout);
  filterRow?.classList.toggle("filter-layout-compact", !classicLayout);
  filterRow?.setAttribute("data-filter-layout", classicLayout ? "classic" : "compact");
  tagContainer.classList.toggle("classic-tag-filters", classicLayout);
  locationContainer.classList.toggle("classic-location-placeholder", classicLayout);

  try {
    const primaryTags = await GetPrimaryTags();
    // Settings can be switched while the backend is returning tags. Do not let
    // a superseded renderer repopulate the newly selected layout.
    if (renderId !== tagFilterRenderId || !document.body.contains(tagContainer)) return;
    if (classicLayout) {
      renderClassicFilters(tagContainer, locationContainer, primaryTags);
    } else {
      renderSelectBasedFilters(tagContainer, locationContainer, primaryTags);
    }
    await renderSecondaryTags(appState.selectedPrimaryTag);
    renderActiveFilterSummary();
  } catch (error) {
    console.error("渲染标签筛选器失败:", error);
  }
}

function renderSelectBasedFilters(tagContainer, locationContainer, primaryTags) {
  const primaryGroup = document.createElement("div");
  primaryGroup.className = "filter-select-group primary-tag-group";
  primaryGroup.innerHTML = '<span class="filter-label">标签</span>';

  const dropdown = document.createElement("div");
  dropdown.className = "single-select-dropdown primary-filter-dropdown";
  dropdown.innerHTML = `
    <button type="button" id="primary-tag-filter-trigger" class="select-trigger"></button>
    <div id="primary-tag-filter-menu" class="select-menu hidden"></div>
  `;

  const trigger = dropdown.querySelector("#primary-tag-filter-trigger");
  const menu = dropdown.querySelector("#primary-tag-filter-menu");
  const options = [{ value: "", text: "全部" }, ...primaryTags.map((tag) => ({ value: tag, text: tag }))];

  options.forEach((option) => {
    const button = document.createElement("button");
    button.type = "button";
    button.className = "select-option";
    button.dataset.value = option.value;
    button.textContent = option.text;
    button.addEventListener("click", async () => {
      appState.selectedPrimaryTag = option.value;
      appState.selectedSecondaryTags = [];
      updatePrimaryTagDropdownUI();
      menu.classList.add("hidden");
      await renderSecondaryTags(appState.selectedPrimaryTag);
      performSearch();
    });
    menu.appendChild(button);
  });

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleFilterMenu(trigger, menu);
  });

  primaryGroup.appendChild(dropdown);

  // 标签筛选控件重建 = 一次扫描/刷新完成：分类树这时重算数量（分组并集，不重复计数）。
  refreshCategorySidebar();
  tagContainer.appendChild(primaryGroup);
  updatePrimaryTagDropdownUI();

  const secondaryGroup = document.createElement("div");
  secondaryGroup.className = "filter-select-group secondary-tag-group";
  secondaryGroup.id = "secondary-tag-group";
  secondaryGroup.innerHTML = '<span class="filter-label">子标签</span>';
  tagContainer.appendChild(secondaryGroup);

  renderLocationFilterDropdown(locationContainer);
  renderGameStateFilterDropdown(locationContainer);
}

function renderClassicFilters(tagContainer, locationContainer, primaryTags) {
  const primaryLine = document.createElement("div");
  primaryLine.className = "classic-filter-line classic-primary-line";

  const locationGroup = document.createElement("div");
  locationGroup.className = "classic-filter-group classic-location-group";
  locationGroup.innerHTML = '<span class="filter-label">位置</span>';
  const locationList = document.createElement("div");
  locationList.className = "classic-filter-chip-list";
  LOCATION_FILTERS.forEach((location) => {
    locationList.appendChild(createLocationTagButton(location));
  });
  locationGroup.appendChild(locationList);

  const gameStateGroup = document.createElement("div");
  gameStateGroup.className = "classic-filter-group classic-game-state-group";
  gameStateGroup.innerHTML = '<span class="filter-label">游戏内</span>';
  const gameStateList = document.createElement("div");
  gameStateList.className = "classic-filter-chip-list";
  GAME_STATE_FILTERS.forEach((state) => {
    gameStateList.appendChild(createGameStateTagButton(state));
  });
  gameStateGroup.appendChild(gameStateList);

  const primaryGroup = document.createElement("div");
  primaryGroup.className = "classic-filter-group classic-primary-group";
  primaryGroup.innerHTML = '<span class="filter-label">标签</span>';
  const primaryList = document.createElement("div");
  primaryList.className = "classic-filter-chip-list";
  [{ value: "", text: "全部" }, ...primaryTags.map((tag) => ({ value: tag, text: tag }))].forEach((option) => {
    primaryList.appendChild(createPrimaryTagButton(option.value, option.text));
  });
  primaryGroup.appendChild(primaryList);

  const secondarySearchGroup = document.createElement("div");
  secondarySearchGroup.id = "classic-secondary-search-group";
  secondarySearchGroup.className = "classic-secondary-search-group";
  secondarySearchGroup.innerHTML = `
    <input
      id="classic-secondary-filter-input"
      class="classic-secondary-filter-input"
      type="text"
      placeholder="筛选子标签..."
      aria-label="筛选子标签"
      autocomplete="off"
    >
  `;
  secondarySearchGroup.querySelector("input").addEventListener("input", (event) => {
    filterClassicSecondaryTagButtons(event.target.value);
  });
  primaryGroup.appendChild(secondarySearchGroup);

  const secondaryGroup = document.createElement("div");
  secondaryGroup.id = "secondary-tag-group";
  secondaryGroup.className = "classic-filter-line classic-secondary-row";
  secondaryGroup.innerHTML = `
    <span class="filter-label">子标签</span>
    <div class="classic-secondary-tags-slot"></div>
    <div class="classic-secondary-action-slot"></div>
  `;

  primaryLine.appendChild(locationGroup);
  primaryLine.appendChild(gameStateGroup);
  primaryLine.appendChild(primaryGroup);
  tagContainer.appendChild(primaryLine);
  tagContainer.appendChild(secondaryGroup);
}

export function updatePrimaryTagDropdownUI() {
  const trigger = document.getElementById("primary-tag-filter-trigger");
  const menu = document.getElementById("primary-tag-filter-menu");
  if (trigger && menu) {
    trigger.textContent = appState.selectedPrimaryTag || "全部";
    menu.querySelectorAll(".select-option").forEach((option) => {
      option.classList.toggle("active", option.dataset.value === appState.selectedPrimaryTag);
    });
  }
  syncPrimaryTagButtons();
  renderActiveFilterSummary();
}

export function createPrimaryTagButton(value, text) {
  const button = document.createElement("button");
  button.className = "primary-tag-btn";
  button.textContent = text;
  button.dataset.value = value;

  if (appState.selectedPrimaryTag === value) {
    button.classList.add("active");
  }

  button.addEventListener("click", async function () {
    document.querySelectorAll(".primary-tag-btn").forEach((btn) => {
      btn.classList.remove("active");
    });
    button.classList.add("active");
    appState.selectedPrimaryTag = value;
    appState.selectedSecondaryTags = [];
    const secondaryFilterInput = document.getElementById("classic-secondary-filter-input");
    if (secondaryFilterInput) secondaryFilterInput.value = "";
    await renderSecondaryTags(appState.selectedPrimaryTag);
    performSearch();
  });

  return button;
}

export async function renderSecondaryTags(primaryTag) {
  const secondaryGroup = document.getElementById("secondary-tag-group");
  const renderId = ++secondaryTagRenderId;
  if (!secondaryGroup) return;

  if (secondaryGroup?.classList.contains("filter-select-group")) {
    await renderSecondaryTagDropdown(secondaryGroup, primaryTag, renderId);
    return;
  }

  await renderSecondaryTagButtons(secondaryGroup, primaryTag, renderId);
}

async function renderSecondaryTagButtons(secondaryGroup, primaryTag, renderId) {
  const tagsSlot = secondaryGroup.querySelector(".classic-secondary-tags-slot") || secondaryGroup;
  const actionSlot = secondaryGroup.querySelector(".classic-secondary-action-slot") || secondaryGroup;
  const existingContainer = secondaryGroup.querySelector(".secondary-tags-container");
  if (existingContainer) existingContainer.remove();

  const existingExpandBtn = secondaryGroup.querySelector(".expand-tags-btn");
  if (existingExpandBtn) existingExpandBtn.remove();

  const existingEmptyHint = secondaryGroup.querySelector(".classic-secondary-empty-hint");
  if (existingEmptyHint) existingEmptyHint.remove();
  secondaryGroup.querySelectorAll(".secondary-match-mode-btn").forEach((button) => button.remove());

  try {
    // 子标签默认只显示一行：按"命中 Mod 数"排序，收起时看到的就是最常用的那批
    // （过去按字典序，显示的是偶然项，用户反馈"只显示一些些子标签没什么用"）。
    const [secondaryTags, tagCounts] = await Promise.all([
      GetSecondaryTags(primaryTag || ""),
      loadSecondaryTagCounts(primaryTag),
    ]);
    if (!isCurrentSecondaryTagRender(secondaryGroup, renderId)) return;

    if (secondaryTags.length > 0) {
      secondaryTags.sort((a, b) => compareSecondaryTagsByUsage(a, b, tagCounts));
      secondaryGroup.style.display = "flex";
      setClassicSecondarySearchVisible(true);

      const container = document.createElement("div");
      container.className = "secondary-tags-container";

      secondaryTags.forEach((tag) => {
        const tagBtn = createSecondaryTagButton(tag, tagCounts[tag.toLowerCase()] || 0);
        container.appendChild(tagBtn);
      });

      tagsSlot.appendChild(container);

      const emptyHint = document.createElement("span");
      emptyHint.className = "classic-secondary-empty-hint hidden";
      emptyHint.textContent = "没有匹配的子标签";
      tagsSlot.appendChild(emptyHint);

      addSecondaryMatchModeControl(actionSlot);

      filterClassicSecondaryTagButtons();
      scheduleSecondaryTagsCollapse(container, actionSlot);
    } else {
      // 固定预设仍应可用：空目录或暂未识别到普通二级标签时，也能直接选择
      // 游戏内的标准物品集合，而不是把整条筛选入口隐藏掉。
      secondaryGroup.style.display = "flex";
      setClassicSecondarySearchVisible(false);
      const emptyHint = document.createElement("span");
      emptyHint.className = "classic-secondary-empty-hint";
      emptyHint.textContent = "当前目录没有已识别的子标签，可使用右侧预设筛选。";
      tagsSlot.appendChild(emptyHint);
      addSecondaryMatchModeControl(actionSlot);
      syncSecondaryTagFilterUI();
    }
  } catch (error) {
    if (!isCurrentSecondaryTagRender(secondaryGroup, renderId)) return;
    console.error("获取二级标签失败:", error);
    secondaryGroup.style.display = "none";
    setClassicSecondarySearchVisible(false);
  }
}

function setClassicSecondarySearchVisible(isVisible) {
  const searchGroup = document.getElementById("classic-secondary-search-group");
  if (searchGroup) {
    searchGroup.classList.toggle("is-empty", !isVisible);
  }
}

function filterClassicSecondaryTagButtons(filterText) {
  const secondaryGroup = document.getElementById("secondary-tag-group");
  if (!secondaryGroup || secondaryGroup.classList.contains("filter-select-group")) return;

  const container = secondaryGroup.querySelector(".secondary-tags-container");
  if (!container) return;

  const searchInput = document.getElementById("classic-secondary-filter-input");
  const normalizedFilter = (filterText ?? searchInput?.value ?? "").trim().toLowerCase();
  let visibleCount = 0;

  container.querySelectorAll(".secondary-tag-btn").forEach((button) => {
    const tag = button.dataset.tag || button.textContent || "";
    const isVisible = !normalizedFilter || tag.toLowerCase().includes(normalizedFilter);
    button.hidden = !isVisible;
    if (isVisible) visibleCount += 1;
  });

  secondaryGroup
    .querySelector(".classic-secondary-empty-hint")
    ?.classList.toggle("hidden", visibleCount > 0);

  updateClassicSecondaryTagsCollapse();
}

// 子标签默认只占一行，右侧给「展开全部 N 项」；展开状态在同一次运行里保持。
let secondaryTagsExpanded = false;
let secondaryTagsResizeObserver = null;

function createSecondaryTagsExpandButton(container, actionSlot) {
  const button = document.createElement("button");
  button.type = "button";
  button.className = "expand-tags-btn";
  button.addEventListener("click", () => {
    secondaryTagsExpanded = !secondaryTagsExpanded;
    syncSecondaryTagsCollapse(container, actionSlot);
    // 展开/收起后行高变化大，下一帧再量一次，避免按钮状态与真实行数不一致。
    requestAnimationFrame(() => syncSecondaryTagsCollapse(container, actionSlot));
  });
  actionSlot.appendChild(button);
  return button;
}

// syncSecondaryTagsCollapse 是折叠/展开的唯一同步点：
// 1) 无论布局有没有就绪，先把容器落到用户要的状态（默认收起 = 一行）；
// 2) 只有"收起后确实会被截断"才显示「展开全部 N 项」按钮；
// 3) 量不到尺寸时返回 false，让调用方稍后重试（旧实现只重试一次，布局慢时就会整片铺开）。
function syncSecondaryTagsCollapse(container, actionSlot) {
  if (!document.body.contains(container)) return true;

  const visibleButtons = Array.from(container.children).filter((button) => !button.hidden);
  let expandBtn = actionSlot.querySelector(".expand-tags-btn");
  if (visibleButtons.length === 0) {
    container.classList.remove("collapsed");
    container.dataset.expanded = "";
    expandBtn?.remove();
    return true;
  }
  if (container.getBoundingClientRect().width <= 0) return false;

  expandBtn = expandBtn || createSecondaryTagsExpandButton(container, actionSlot);

  // 展开态没有 max-height，量不出"收起会不会截断"，所以先强制收起量一次。
  const wantCollapsed = !secondaryTagsExpanded;
  container.classList.add("collapsed");
  const overflows = container.scrollHeight > container.clientHeight + 1;
  container.classList.toggle("collapsed", wantCollapsed);
  container.dataset.expanded = secondaryTagsExpanded ? "true" : "false";

  expandBtn.hidden = !overflows;
  syncExpandButton(expandBtn, secondaryTagsExpanded, visibleButtons.length);
  return true;
}

function updateClassicSecondaryTagsCollapse() {
  const container = document.querySelector(".filter-row-filters.filter-layout-classic .secondary-tags-container");
  const actionSlot = document.querySelector(".filter-row-filters.filter-layout-classic .classic-secondary-action-slot");
  if (container && actionSlot) {
    syncSecondaryTagsCollapse(container, actionSlot);
  }
}

function attachSecondaryTagsResizeObserver(container, actionSlot) {
  if (typeof ResizeObserver !== "function") return;
  secondaryTagsResizeObserver?.disconnect();
  secondaryTagsResizeObserver = new ResizeObserver(() => {
    if (!document.body.contains(container)) {
      secondaryTagsResizeObserver?.disconnect();
      return;
    }
    syncSecondaryTagsCollapse(container, actionSlot);
  });
  secondaryTagsResizeObserver.observe(container);
}

function scheduleSecondaryTagsCollapse(container, actionSlot) {
  let attempts = 0;
  const run = () => {
    attempts += 1;
    if (!syncSecondaryTagsCollapse(container, actionSlot)) {
      if (attempts < 12) setTimeout(run, 80);
      return;
    }
    attachSecondaryTagsResizeObserver(container, actionSlot);
  };
  requestAnimationFrame(run);
}

function syncExpandButton(button, isExpanded, count = 0) {
  button.innerHTML = isExpanded
    ? '<span class="icon">▲</span> 收起'
    : `<span class="icon">▼</span> 展开全部${count ? ` ${count} 项` : ""}`;
}

async function renderSecondaryTagDropdown(secondaryGroup, primaryTag, renderId) {
  secondaryGroup.querySelectorAll(".secondary-filter-dropdown").forEach((el) => el.remove());
  secondaryGroup.querySelectorAll(".multi-select-trigger.is-disabled").forEach((el) => el.remove());

  // 移除原有的隐藏逻辑，始终显示子标签
  // 当 primaryTag 为空时，后端会返回所有文件的二级标签去重
  secondaryGroup.classList.remove("is-empty");
  secondaryGroup.style.display = "flex";
  secondaryGroup.style.visibility = "visible";

  try {
    // 后端已支持空 primaryTag，返回所有二级标签去重
    const [secondaryTags, tagCounts] = await Promise.all([
      GetSecondaryTags(primaryTag || ""),
      loadSecondaryTagCounts(primaryTag),
    ]);
    if (!isCurrentSecondaryTagRender(secondaryGroup, renderId)) return;
    if (!secondaryTags.length) {
      const emptyTrigger = document.createElement("button");
      emptyTrigger.type = "button";
      emptyTrigger.className = "select-trigger multi-select-trigger is-disabled";
      emptyTrigger.textContent = "暂无已识别标签";
      emptyTrigger.disabled = true;
      secondaryGroup.appendChild(emptyTrigger);
      syncSecondaryTagFilterUI();
      return;
    }

    // 下拉布局同样按常用度排：搜索框在，但列表第一屏先给最常用的标签。
    secondaryTags.sort((a, b) => compareSecondaryTagsByUsage(a, b, tagCounts));

    const dropdown = document.createElement("div");
    dropdown.className = "multi-select-dropdown secondary-filter-dropdown";
    dropdown.innerHTML = `
      <button type="button" class="select-trigger multi-select-trigger">${getSelectedSecondaryLabel()}</button>
      <div class="select-menu multi-select-menu filter-flyout-menu secondary-tag-filter-menu hidden" role="dialog" aria-label="子标签筛选">
        <div class="multi-select-search-wrapper">
          <input type="text" class="multi-select-search-input" placeholder="筛选子标签...">
        </div>
        <button type="button" class="secondary-match-mode-btn" title="切换二级标签筛选：任一匹配 / 全部匹配"></button>
        <div class="multi-select-options"></div>
      </div>
    `;

    const trigger = dropdown.querySelector(".multi-select-trigger");
    const menu = dropdown.querySelector(".multi-select-menu");
    const searchInput = dropdown.querySelector(".multi-select-search-input");
    const optionsContainer = dropdown.querySelector(".multi-select-options");
    const matchModeButton = dropdown.querySelector(".secondary-match-mode-btn");
    menu.prepend(createFilterFlyoutHeader("子标签筛选", "可搜索、多选，并切换任一 / 全部匹配"));
    matchModeButton.textContent = `匹配方式：${getSecondaryMatchModeLabel()}`;
    matchModeButton.addEventListener("click", (event) => {
      event.stopPropagation();
      appState.secondaryMatchMode = appState.secondaryMatchMode === "all" ? "any" : "all";
      syncSecondaryTagFilterUI();
      performSearch();
    });

    // 渲染选项的函数
    const renderOptions = (filterText = "") => {
      optionsContainer.innerHTML = "";
      const filteredTags = filterText
        ? secondaryTags.filter((tag) => tag.toLowerCase().includes(filterText.toLowerCase()))
        : secondaryTags;

      filteredTags.forEach((tag) => {
        const optionCount = tagCounts[tag.toLowerCase()] || 0;
        const label = document.createElement("label");
        label.className = "multi-select-option";
        label.innerHTML = `
          <input type="checkbox" value="${escapeHtml(tag)}" ${appState.selectedSecondaryTags.includes(tag) ? "checked" : ""}>
          <span>${escapeHtml(tag)}</span>
          ${optionCount > 0 ? `<span class="multi-select-option-count">${optionCount}</span>` : ""}
        `;
        const tagInput = label.querySelector("input");
        tagInput.dataset.secondaryTag = tag;
        tagInput.addEventListener("change", (event) => {
          const isChecked = event.target.checked;
          setSecondaryTagChecked(tag, isChecked);

          // 选中后清除输入框并重新渲染所有选项
          searchInput.value = "";
          renderOptions();
        });
        optionsContainer.appendChild(label);
      });
    };

    // 初始渲染所有选项
    renderOptions();

    // 输入筛选事件
    searchInput.addEventListener("input", (event) => {
      renderOptions(event.target.value);
    });

    // 阻止输入框点击事件冒泡，避免关闭菜单
    searchInput.addEventListener("click", (event) => {
      event.stopPropagation();
    });

    // 阻止输入框键盘事件冒泡
    searchInput.addEventListener("keydown", (event) => {
      event.stopPropagation();
    });

    trigger.addEventListener("click", (event) => {
      event.stopPropagation();
      toggleFilterMenu(trigger, menu);
    });

    secondaryGroup.appendChild(dropdown);
    syncSecondaryTagFilterUI();
  } catch (error) {
    if (!isCurrentSecondaryTagRender(secondaryGroup, renderId)) return;
    console.error("获取二级标签失败:", error);
    secondaryGroup.classList.add("is-empty");
    secondaryGroup.style.display = "flex";
    secondaryGroup.style.visibility = "hidden";
  }
}

// loadSecondaryTagCounts 取"每个子标签命中了多少个 Mod"。绑定缺失或后端报错时退回空表：
// 排序与角标是增强，不该让整条筛选条渲染失败（失败时仍然按字典序展示全部标签）。
async function loadSecondaryTagCounts(primaryTag) {
  if (typeof GetSecondaryTagCounts !== "function") return {};
  try {
    const counts = await GetSecondaryTagCounts(primaryTag || "");
    return counts && typeof counts === "object" ? counts : {};
  } catch (error) {
    console.warn("获取子标签计数失败，退回字典序:", error);
    return {};
  }
}

function createSecondaryTagButton(tag, count = 0) {
  const button = document.createElement("button");
  button.className = "secondary-tag-btn";
  button.dataset.tag = tag;
  button.dataset.secondaryTag = tag;

  const label = document.createElement("span");
  label.className = "secondary-tag-label";
  label.textContent = tag;
  button.appendChild(label);
  if (count > 0) {
    const badge = document.createElement("span");
    badge.className = "secondary-tag-count";
    badge.textContent = String(count);
    button.appendChild(badge);
    button.title = `${tag}：${count} 个 Mod`;
  } else {
    button.title = tag;
  }

  if (appState.selectedSecondaryTags.includes(tag)) {
    button.classList.add("active");
  }

  button.addEventListener("click", function () {
    toggleSecondaryTag(tag);
  });

  return button;
}

function createLocationTagButton(location) {
  const button = document.createElement("button");
  button.className = `location-tag-btn filter-location-${location}`;
  button.textContent = getLocationDisplayName(location);
  button.dataset.location = location;

  if (appState.selectedLocations.includes(location)) {
    button.classList.add("active");
  }

  button.addEventListener("click", function () {
    toggleLocationFilter(location, button);
  });

  return button;
}

function toggleSecondaryTag(tag) {
  setSecondaryTagChecked(tag, !appState.selectedSecondaryTags.includes(tag));
}

export function renderLocationFilterDropdown(locationContainer) {
  const group = document.createElement("div");
  group.className = "filter-select-group location-filter-group";
  group.innerHTML = '<span class="filter-label">位置</span>';

  const dropdown = document.createElement("div");
  dropdown.className = "multi-select-dropdown location-filter-dropdown";
  dropdown.innerHTML = `
    <button type="button" id="location-filter-trigger" class="select-trigger multi-select-trigger"></button>
    <div id="location-filter-menu" class="select-menu multi-select-menu hidden"></div>
  `;

  const trigger = dropdown.querySelector("#location-filter-trigger");
  const menu = dropdown.querySelector("#location-filter-menu");

  LOCATION_FILTERS.forEach((tag) => {
    const label = document.createElement("label");
    label.className = "multi-select-option";
    label.innerHTML = `
      <input type="checkbox" value="${tag}" ${appState.selectedLocations.includes(tag) ? "checked" : ""}>
      <span>${getLocationDisplayName(tag)}</span>
    `;
    label.querySelector("input").addEventListener("change", (event) => {
      if (event.target.checked) {
        if (!appState.selectedLocations.includes(tag)) {
          appState.selectedLocations.push(tag);
        }
      } else {
        appState.selectedLocations = appState.selectedLocations.filter((item) => item !== tag);
      }
      updateLocationFilterDropdownUI();
      performSearch();
    });
    menu.appendChild(label);
  });

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleFilterMenu(trigger, menu);
  });

  group.appendChild(dropdown);
  locationContainer.appendChild(group);
  updateLocationFilterDropdownUI();
}

export function updateLocationFilterDropdownUI() {
  const trigger = document.getElementById("location-filter-trigger");
  const menu = document.getElementById("location-filter-menu");
  const selectedNames = appState.selectedLocations
    .map((location) => getLocationDisplayName(location))
    .filter(Boolean);
  if (trigger && menu) {
    if (selectedNames.length === 0) {
      trigger.textContent = "全部";
    } else if (selectedNames.length <= 2) {
      // 显示具体位置，避免“已选 1 个”掩盖了另一个同名 VPK 被筛掉的原因。
      trigger.textContent = selectedNames.join("、");
    } else {
      trigger.textContent = `${selectedNames.slice(0, 2).join("、")} 等 ${selectedNames.length} 个`;
    }

    menu.querySelectorAll("input[type='checkbox']").forEach((checkbox) => {
      checkbox.checked = appState.selectedLocations.includes(checkbox.value);
    });
  }
  document.querySelectorAll(".location-tag-btn").forEach((button) => {
    button.classList.toggle("active", appState.selectedLocations.includes(button.dataset.location));
  });
  renderActiveFilterSummary();
}

function renderGameStateFilterDropdown(locationContainer) {
  const group = document.createElement("div");
  group.className = "filter-select-group game-state-filter-group";
  group.innerHTML = '<span class="filter-label">游戏内</span>';

  const dropdown = document.createElement("div");
  dropdown.className = "multi-select-dropdown game-state-filter-dropdown";
  dropdown.innerHTML = `
    <button type="button" id="game-state-filter-trigger" class="select-trigger multi-select-trigger"></button>
    <div id="game-state-filter-menu" class="select-menu multi-select-menu hidden"></div>
  `;

  const trigger = dropdown.querySelector("#game-state-filter-trigger");
  const menu = dropdown.querySelector("#game-state-filter-menu");
  GAME_STATE_FILTERS.forEach((state) => {
    const label = document.createElement("label");
    label.className = "multi-select-option";
    label.innerHTML = `
      <input type="checkbox" value="${state}" ${appState.selectedGameStates.includes(state) ? "checked" : ""}>
      <span>${getGameStateDisplayName(state)}</span>
    `;
    label.querySelector("input").addEventListener("change", (event) => {
      if (event.target.checked) {
        if (!appState.selectedGameStates.includes(state)) {
          appState.selectedGameStates.push(state);
        }
      } else {
        appState.selectedGameStates = appState.selectedGameStates.filter((item) => item !== state);
      }
      updateGameStateFilterDropdownUI();
      performSearch();
    });
    menu.appendChild(label);
  });

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleFilterMenu(trigger, menu);
  });

  group.appendChild(dropdown);
  locationContainer.appendChild(group);
  updateGameStateFilterDropdownUI();
}

function updateGameStateFilterDropdownUI() {
  const trigger = document.getElementById("game-state-filter-trigger");
  const menu = document.getElementById("game-state-filter-menu");
  const selectedNames = appState.selectedGameStates.map(getGameStateDisplayName);
  if (trigger && menu) {
    trigger.textContent = selectedNames.length
      ? selectedNames.length <= 2
        ? selectedNames.join("、")
        : `${selectedNames.slice(0, 2).join("、")} 等 ${selectedNames.length} 个`
      : "全部";
    menu.querySelectorAll("input[type='checkbox']").forEach((checkbox) => {
      checkbox.checked = appState.selectedGameStates.includes(checkbox.value);
    });
  }
  document.querySelectorAll(".game-state-tag-btn").forEach((button) => {
    button.classList.toggle("active", appState.selectedGameStates.includes(button.dataset.gameState));
  });
  renderActiveFilterSummary();
}

export function toggleLocationFilter(location, button) {
  const index = appState.selectedLocations.indexOf(location);
  if (index > -1) {
    appState.selectedLocations.splice(index, 1);
    button.classList.remove("active");
  } else {
    appState.selectedLocations.push(location);
    button.classList.add("active");
  }
  performSearch();
}

function createGameStateTagButton(state) {
  const button = document.createElement("button");
  button.className = `game-state-tag-btn filter-game-${state}`;
  button.textContent = getGameStateDisplayName(state);
  button.dataset.gameState = state;
  button.classList.toggle("active", appState.selectedGameStates.includes(state));
  button.addEventListener("click", () => {
    const index = appState.selectedGameStates.indexOf(state);
    if (index > -1) {
      appState.selectedGameStates.splice(index, 1);
      button.classList.remove("active");
    } else {
      appState.selectedGameStates.push(state);
      button.classList.add("active");
    }
    performSearch();
  });
  return button;
}

export async function resetFilters() {
  if (appState.isLoading) {
    console.log("正在加载中，请稍候...");
    return;
  }

  appState.isLoading = true;
  showFileListLoading("正在重置筛选...");

  try {
    document.getElementById("search-input").value = "";
    appState.searchQuery = "";

    document.querySelectorAll(".primary-tag-btn").forEach((btn) => {
      btn.classList.remove("active");
      if (btn.dataset.value === "") {
        btn.classList.add("active");
      }
    });
    appState.selectedPrimaryTag = "";
    updatePrimaryTagDropdownUI();
    const secondaryFilterInput = document.getElementById("classic-secondary-filter-input");
    if (secondaryFilterInput) secondaryFilterInput.value = "";

    appState.selectedSecondaryTags = [];
    appState.secondaryMatchMode = "any";
    appState.selectedLocations = [];
    appState.selectedGameStates = [];
    document.querySelectorAll(".location-tag-btn").forEach((btn) => {
      btn.classList.remove("active");
    });
    document.querySelectorAll(".game-state-tag-btn").forEach((btn) => {
      btn.classList.remove("active");
    });
    updateLocationFilterDropdownUI();
    updateGameStateFilterDropdownUI();

    await renderSecondaryTags("");
    syncSecondaryTagFilterUI();

    appState.sortType = "name";
    appState.sortOrder = "asc";
    saveSortPreference();
    updateSortButtonUI();

    await performSearch();
  } finally {
    appState.isLoading = false;
    hideFileListLoading();
  }
}

export function handleSearch(event) {
  appState.searchQuery = event.target.value;
  if (searchInputTimer) clearTimeout(searchInputTimer);
  searchInputTimer = setTimeout(() => {
    searchInputTimer = null;
    void performSearch();
  }, SEARCH_INPUT_DEBOUNCE_MS);
}

export async function performSearch() {
  // Checkbox/chip actions should be immediate. A pending keystroke search is
  // redundant once a more explicit filter action starts.
  if (searchInputTimer) {
    clearTimeout(searchInputTimer);
    searchInputTimer = null;
  }

  const requestId = ++searchRequestId;
  const filters = snapshotFilterState();

  try {
    console.log(
      "执行搜索，查询词:", filters.searchQuery,
      "一级标签:", filters.primaryTag,
      "二级标签:", filters.secondaryTags,
      "位置:", filters.locations,
      "游戏内:", filters.gameStates,
      "二级匹配:", filters.secondaryMatchMode
    );

    let files;
    if (
      !filters.searchQuery &&
      !filters.primaryTag &&
      filters.secondaryTags.length === 0
    ) {
      files = [...appState.allVpkFiles];
    } else {
      files = await SearchVPKFiles(
        filters.searchQuery,
        filters.primaryTag,
        filters.secondaryTags,
      );
    }

    // Wails calls cannot be cancelled after dispatch. Instead, make their
    // completion harmless: only the latest still-matching filter state may
    // update the list. This prevents rapid clicks from showing stale results.
    if (requestId !== searchRequestId || !filterStateStillMatches(filters)) return;

    if (filters.locations.length > 0) {
      files = files.filter((file) =>
        filters.locations.includes(file.location),
      );
    }

    if (filters.gameStates.length > 0) {
      files = files.filter((file) =>
        filters.gameStates.includes(getGameState(file)),
      );
    }

    // 分组筛选：勾选策略组后只显示属于这些组的 Mod（组归属按 addonlist 键匹配）。
    if ((appState.activeGroupFilter?.size || 0) > 0) {
      files = files.filter((file) =>
        fileMatchesGroupFilter(
          file,
          appState.groupFilterOptions || [],
          appState.activeGroupFilter,
          appState.currentDirectory,
        ),
      );
    }

    if (filters.secondaryTags.length > 0 && filters.secondaryMatchMode === "all") {
      files = files.filter((file) => matchesSecondaryTags(file, filters.secondaryTags));
    }

    if (!filters.showHidden) {
      files = files.filter(
        (file) => !file.name.startsWith("_")
      );
    }

    if (requestId !== searchRequestId || !filterStateStillMatches(filters)) return;

    applySort(files);
    appState.vpkFiles = files;
    renderFileList();
    // 模型复杂度排序处于激活状态时，筛选可能带出"还没算过模型指标"的新文件：
    // 这里按需补齐（命中缓存时是纯内存操作），补完再重排一次。
    void ensureVisibleModelMetrics();
    updateStatusBar();
    renderActiveFilterSummary();
    scheduleScopedConflictAnalysis();
    // 组归属是异步附加信息：刷新完成后由 group-state 通知重绘徽标，不阻塞列表渲染。
    void refreshModGroupMembershipState({ silent: true });

    // 分类树的高亮跟着实际筛选状态走（工具栏标签菜单、筛选芯片等改了筛选时也要同步）。
    syncCategorySidebarSelection();
    console.log(`搜索完成，显示 ${appState.vpkFiles.length} 个文件`);
  } catch (error) {
    if (requestId !== searchRequestId || !filterStateStillMatches(filters)) return;
    console.error("搜索失败:", error);
    showError("搜索失败: " + error);
  }
}

// toggleCategoryFilterSelection 给分类侧边栏用：与「内容预设」同一语义 ——
// 点一下「加上这一类」，再点一下取消，可以同时选多类；不动其它维度的既有筛选。
// 树只描述"选什么"，真正的筛选状态与联动（芯片、摘要、下拉框同步）仍然由这里统一处理。
export async function toggleCategoryFilterSelection(selection = {}) {
  const tags = Array.isArray(selection?.tags) ? selection.tags : [];
  const locations = Array.isArray(selection?.locations) ? selection.locations : [];
  const gameStates = Array.isArray(selection?.gameStates) ? selection.gameStates : [];

  if (tags.length > 0) {
    setSelectedSecondaryTags(toggleFilterValues(appState.selectedSecondaryTags, tags));
    syncSecondaryTagFilterUI();
  }
  if (locations.length > 0) {
    appState.selectedLocations = toggleFilterValues(appState.selectedLocations, locations);
    updateLocationFilterDropdownUI();
  }
  if (gameStates.length > 0) {
    appState.selectedGameStates = toggleFilterValues(appState.selectedGameStates, gameStates);
    updateGameStateFilterDropdownUI();
  }
  await performSearch();
}

// clearCategoryFilterSelection 只清「分类」这一层：标签 + 位置 + 游戏内状态。
export async function clearCategoryFilterSelection() {
  setSelectedSecondaryTags([]);
  syncSecondaryTagFilterUI();
  appState.selectedLocations = [];
  updateLocationFilterDropdownUI();
  appState.selectedGameStates = [];
  updateGameStateFilterDropdownUI();
  await performSearch();
}

// 分类树上的「查看全部」：选中该组/子组的聚合标签（与旧内容预设菜单同一个函数）。
export async function applyPresetAggregateTag(tag) {
  if (!tag) return;
  selectPresetAggregate(tag);
}

// 分类树上的「清空本组」：只取消这一组挂着的标签，其它选择保持不动。
export async function clearPresetGroupTags(tags) {
  const targets = Array.isArray(tags) ? tags : [];
  if (targets.length === 0) return;
  setSelectedSecondaryTags((appState.selectedSecondaryTags || []).filter((tag) => !targets.includes(tag)));
  syncSecondaryTagFilterUI();
  await performSearch();
}

function toggleFilterValues(current, values) {
  const list = Array.isArray(current) ? current : [];
  const targets = Array.isArray(values) ? values : [];
  if (targets.length === 0) return [...list];
  const allSelected = targets.every((value) => list.includes(value));
  return allSelected
    ? list.filter((value) => !targets.includes(value))
    : [...new Set([...list, ...targets])];
}

export function toggleFilterMenu(trigger, menu) {
  const willOpen = menu.classList.contains("hidden");
  closeFilterMenus(willOpen ? menu : null);

  if (!willOpen) {
    menu.classList.add("hidden");
    return;
  }

  menu.classList.remove("hidden");
  positionFilterFlyoutMenu(trigger, menu);
}

function positionFilterFlyoutMenu(trigger, menu) {
  if (!menu.classList.contains("filter-flyout-menu")) return;

  const rect = trigger.getBoundingClientRect();
  const viewportPadding = 12;
  const availableWidth = Math.max(0, window.innerWidth - viewportPadding * 2);
  const isPreset = menu.classList.contains("preset-filter-menu");
  const preferredWidth = isPreset ? 720 : 520;
  const width = Math.min(preferredWidth, availableWidth);
  const desiredLeft = isPreset ? rect.right - width : rect.left;
  const left = Math.max(viewportPadding, Math.min(desiredLeft, window.innerWidth - viewportPadding - width));

  const roomBelow = window.innerHeight - rect.bottom - viewportPadding;
  const roomAbove = rect.top - viewportPadding;
  const openUpward = roomBelow < 260 && roomAbove > roomBelow;
  const maxHeight = Math.max(160, Math.min(isPreset ? 640 : 480, (openUpward ? roomAbove : roomBelow) - 8));

  menu.style.setProperty("--filter-flyout-width", `${width}px`);
  menu.style.setProperty("--filter-flyout-max-height", `${maxHeight}px`);
  menu.style.left = `${left}px`;
  menu.style.right = "auto";
  menu.style.top = openUpward ? "auto" : `${Math.min(window.innerHeight - viewportPadding, rect.bottom + 8)}px`;
  menu.style.bottom = openUpward ? `${Math.max(viewportPadding, window.innerHeight - rect.top + 8)}px` : "auto";
  menu.classList.toggle("opens-upward", openUpward);
  menu._filterFlyoutTrigger = trigger;
}

function repositionOpenFilterFlyoutMenus() {
  document.querySelectorAll(".filter-flyout-menu:not(.hidden)").forEach((menu) => {
    if (menu._filterFlyoutTrigger instanceof HTMLElement) {
      positionFilterFlyoutMenu(menu._filterFlyoutTrigger, menu);
    }
  });
}

export function closeFilterMenus(exceptMenu = null) {
  document.querySelectorAll(".select-menu, .multi-select-menu").forEach((menu) => {
    if (menu !== exceptMenu) {
      menu.classList.add("hidden");
    }
  });
}

export async function refreshFilesKeepFilter(options = {}) {
  // silent：后台自动刷新（外部改动、定时更新检测）。不显示加载遮罩、不禁用工具栏，
  // 失败也不弹错误——这类刷新不该打断用户正在做的事。
  const silent = options?.silent === true;
  resetBoxSelection();

  if (!appState.currentDirectory) {
    // 没选目录时的提示只给用户手动刷新用；后台自动刷新弹这个只会莫名其妙。
    if (!silent) showNotification("请先选择目录", "info");
    return;
  }

  if (refreshFilesPromise) {
    // If scanning has already started, an operation that just completed may
    // have missed the current scan's filesystem snapshot. Run one coalesced
    // follow-up scan so every awaiter observes the final state.
    if (refreshFilesPhase === "scanning") {
      refreshFilesNeedsFollowUp = true;
    }
    // 用户点了刷新却撞上后台静默轮次：把加载提示补上，别让人觉得没反应。
    if (!silent) showRefreshLoadingOnce();
    return refreshFilesPromise;
  }

  refreshFilesPromise = runRefreshFilesKeepFilter({ silent }).finally(() => {
    refreshFilesPromise = null;
    refreshFilesPhase = "idle";
    refreshFilesNeedsFollowUp = false;
    refreshFilesLoadingVisible = false;
  });
  return refreshFilesPromise;
}

// 加载提示的生命周期挂在"整轮刷新"上（含合并出来的 follow-up 扫描），
// 而不是单次扫描：合并刷新不会再出现遮罩闪两下。
function showRefreshLoadingOnce() {
  if (refreshFilesLoadingVisible) return;
  refreshFilesLoadingVisible = true;
  showFileListLoading("正在刷新文件列表...");
}

async function runRefreshFilesKeepFilter({ silent = false } = {}) {
  try {
    do {
      refreshFilesNeedsFollowUp = false;
      refreshFilesPhase = "waiting";
      await waitForFileListIdle();
      if (!silent) showRefreshLoadingOnce();
      refreshFilesPhase = "scanning";
      await refreshFilesKeepFilterOnce();
    } while (refreshFilesNeedsFollowUp);
  } finally {
    // 只有真的显示过提示（可见轮次，或被用户的显式刷新升格过）才收起来，
    // 避免静默轮次"顺手"解禁别人禁用的按钮 / 关掉别人的加载提示。
    if (refreshFilesLoadingVisible) {
      refreshFilesLoadingVisible = false;
      hideFileListLoading();
    }
  }
}

// Directory switching changes the backend's process-wide root directory, so
// it must wait for any active list scan before changing that shared state.
export function waitForFileListIdle() {
  if (!appState.isLoading) return Promise.resolve();

  return new Promise((resolve) => {
    const wait = () => {
      if (!appState.isLoading) {
        resolve();
        return;
      }
      window.setTimeout(wait, FILE_LIST_IDLE_POLL_MS);
    };
    wait();
  });
}

async function refreshFilesKeepFilterOnce() {
  // Capture immediately before rebuilding the filter controls rather than at
  // request time. This preserves filter changes a user makes while the
  // filesystem scan is still running instead of restoring a stale snapshot.
  const getCurrentFilters = () => ({
    searchText: document.getElementById("search-input")?.value || "",
    primaryTag: appState.selectedPrimaryTag || "",
    secondaryTags: [...appState.selectedSecondaryTags],
    secondaryMatchMode: appState.secondaryMatchMode,
    locationTags: [...appState.selectedLocations],
    gameStates: [...appState.selectedGameStates],
  });

  appState.isLoading = true;

  try {
    await ScanVPKFiles();
    await refreshLoadOrderMap({ silent: true });

    const [files, primaryTags] = await Promise.all([
      GetVPKFiles(),
      GetPrimaryTags(),
    ]);

    applySort(files);

    appState.allVpkFiles = files;
    appState.primaryTags = primaryTags;

    const currentFilters = getCurrentFilters();
    appState.searchQuery = currentFilters.searchText || "";
    appState.selectedPrimaryTag = currentFilters.primaryTag || "";
    appState.selectedSecondaryTags = currentFilters.secondaryTags || [];
    appState.secondaryMatchMode = currentFilters.secondaryMatchMode || "any";
    appState.selectedLocations = currentFilters.locationTags || [];
    appState.selectedGameStates = currentFilters.gameStates || [];

    await renderTagFilters();

    const searchInput = document.getElementById("search-input");
    if (searchInput) {
      searchInput.value = currentFilters.searchText || "";
    }

    await performSearch();

    const currentFilePaths = new Set(appState.allVpkFiles.map((f) => f.path));
    for (const path of appState.selectedFiles) {
      if (!currentFilePaths.has(path)) {
        appState.selectedFiles.delete(path);
      }
    }
    if (!currentFilePaths.has(appState.selectionAnchorPath)) {
      appState.selectionAnchorPath = "";
    }

    updateStatusBar();

    console.log("文件列表已刷新，筛选状态已恢复");
  } catch (error) {
    console.error("刷新文件列表失败:", error);
    // 静默刷新失败只进控制台：后台自动刷新弹错误框属于打扰（用户手动刷新时才提示）。
    if (refreshFilesLoadingVisible) showError("刷新失败: " + error);
  } finally {
    appState.isLoading = false;
  }
}
