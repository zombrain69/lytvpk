import { appState, updateStatusBar, showFileListLoading, hideFileListLoading } from "../state.js";
import { showError } from "../../core/toast.js";
import { renderFileList } from "./render.js";
import { getLocationDisplayName, escapeHtml } from "../../core/utils.js";
import { applySort, updateSortButtonUI } from "./sorting.js";
import { resetBoxSelection } from "./box-selection.js";
import { GetPrimaryTags, GetSecondaryTags, SearchVPKFiles, ScanVPKFiles, GetVPKFiles } from "../../../../wailsjs/go/app/App";

const LOCATION_FILTERS = ["root", "workshop", "disabled"];
const GAME_STATE_FILTERS = ["enabled", "disabled", "unknown"];

// 这些预设不依赖当前目录恰好扫描到哪些二级标签。这样即使某个目录暂时没有
// 例如 MP5 或草叉，用户也能看到完整的游戏内物品分类，并可直接切换筛选。
// 根级“查看全部”使用后端已经写入 VPK 的聚合标签；展开后则可以精确勾选到单件。
const SECONDARY_TAG_PRESET_GROUPS = [
  {
    id: "firearms",
    label: "枪械",
    allTag: "所有枪械",
    description: "19 把便携枪械 + 固定机关枪",
    groups: [
      { label: "手枪", allTag: "手枪", tags: ["小手枪", "马格南"] },
      { label: "步枪", allTag: "步枪", tags: ["AK47", "M16", "三连发", "sg552"] },
      { label: "冲锋枪", allTag: "冲锋枪", tags: ["乌兹", "消音", "MP5"] },
      { label: "狙击枪", allTag: "狙击枪", tags: ["大狙", "猎枪", "军狙", "鸟狙"] },
      { label: "霰弹枪", allTag: "霰弹枪", tags: ["木喷", "一代连喷", "铁喷", "二代连喷"] },
      { label: "特殊 / 重型", tags: ["M60", "榴弹发射器", "固定机关枪"] },
    ],
  },
  {
    id: "official-melee",
    label: "官方近战",
    allTag: "所有官方近战",
    description: "13 把官方近战；隐藏近战单独保留",
    groups: [
      { label: "钝器", tags: ["棒球棍", "板球拍", "吉他", "平底锅", "高尔夫球杆", "警棍"] },
      { label: "锐器", tags: ["消防斧", "砍刀", "武士刀", "撬棍"] },
      { label: "工具", tags: ["电锯", "草叉", "铁铲"] },
      { label: "隐藏近战", tags: ["匕首", "防爆盾"] },
    ],
  },
  {
    id: "throwables",
    label: "投掷物品",
    allTag: "所有投掷物品",
    groups: [
      { label: "全部投掷物", allTag: "投掷物", tags: ["土制炸弹", "燃烧瓶", "胆汁"] },
    ],
  },
  {
    id: "medical-items",
    label: "医疗物品",
    allTag: "所有医疗物品",
    groups: [
      { label: "全部医疗物品", allTag: "医疗物品", tags: ["医疗包", "电击器", "止痛药", "肾上腺"] },
    ],
  },
  {
    id: "supply-boxes",
    label: "补给盒",
    allTag: "盒子",
    description: "弹药堆、升级弹药与激光瞄准盒",
    groups: [
      { label: "弹药堆", allTag: "弹药堆", tags: ["一代子弹堆", "二代子弹堆"] },
      { label: "燃烧弹", allTag: "燃烧弹", tags: ["燃烧弹盒"] },
      { label: "高爆弹", allTag: "高爆弹", tags: ["高爆弹盒"] },
      { label: "镭射", allTag: "镭射", tags: ["激光瞄准盒"] },
    ],
  },
];

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
  if (!file.gameStateKnown) return "unknown";
  return file.gameEnabled ? "enabled" : "disabled";
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
  document.querySelectorAll(".secondary-preset-dropdown .preset-filter-trigger").forEach((trigger) => {
    trigger.textContent = selected.size > 0 ? `预设 · 已选 ${selected.size}` : "预设";
    trigger.classList.toggle("has-selection", selected.size > 0);
    trigger.setAttribute("aria-label", selected.size > 0 ? `内容预设，已选 ${selected.size} 项` : "内容预设，未选择");
  });
  document.querySelectorAll(".secondary-match-mode-btn").forEach((button) => {
    button.textContent = `匹配方式：${getSecondaryMatchModeLabel()}`;
  });

  SECONDARY_TAG_PRESET_GROUPS.forEach((group) => {
    const groupTags = getPresetGroupTags(group);
    const selectedCount = groupTags.filter((tag) => selected.has(tag)).length;
    document.querySelectorAll(`[data-preset-group="${group.id}"]`).forEach((element) => {
      element.classList.toggle("has-selection", selectedCount > 0);
      element.querySelectorAll(".preset-group-selected-count").forEach((count) => {
        count.textContent = selectedCount ? `已选 ${selectedCount}` : "未选择";
      });
    });
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

function matchesSecondaryTags(file) {
  const selected = appState.selectedSecondaryTags || [];
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

function createPresetTagCheckbox(tag) {
  const label = document.createElement("label");
  label.className = "preset-tag-option";

  const input = document.createElement("input");
  input.type = "checkbox";
  input.dataset.presetTag = tag;
  input.checked = appState.selectedSecondaryTags.includes(tag);
  input.addEventListener("change", (event) => {
    event.stopPropagation();
    setSecondaryTagChecked(tag, event.target.checked);
  });

  const text = document.createElement("span");
  text.textContent = tag;
  label.append(input, text);
  return label;
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
  (subgroup.tags || []).forEach((tag) => items.appendChild(createPresetTagCheckbox(tag)));

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
  expandButton.title = `展开 ${group.label} 的分类与具体项目`;
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

function renderSecondaryTagPresets(container) {
  if (!container) return;
  container.querySelectorAll(".secondary-preset-dropdown").forEach((dropdown) => dropdown.remove());

  const dropdown = document.createElement("div");
  dropdown.className = "multi-select-dropdown secondary-preset-dropdown";
  dropdown.innerHTML = `
    <button type="button" class="preset-filter-trigger" title="打开枪械、近战、投掷物、医疗物品和补给盒的快捷预设">预设</button>
    <div class="select-menu multi-select-menu filter-flyout-menu preset-filter-menu hidden" role="dialog" aria-label="内容预设筛选"></div>
  `;

  const trigger = dropdown.querySelector(".preset-filter-trigger");
  const menu = dropdown.querySelector(".preset-filter-menu");
  menu.appendChild(createFilterFlyoutHeader("内容预设", "展开分类后可查看全部或勾选具体项目"));
  const help = document.createElement("p");
  help.className = "preset-filter-help";
  help.textContent = "点“查看全部”一键筛选；展开后可勾选具体项目，并与任一 / 全部匹配联动。";
  menu.appendChild(help);
  SECONDARY_TAG_PRESET_GROUPS.forEach((group) => menu.appendChild(createPresetGroup(group)));

  trigger.addEventListener("click", (event) => {
    event.stopPropagation();
    toggleFilterMenu(trigger, menu);
  });
  menu.addEventListener("click", (event) => event.stopPropagation());

  container.appendChild(dropdown);
  syncSecondaryTagFilterUI();
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

  if (!tagContainer || !locationContainer) return;
  tagContainer.innerHTML = "";
  locationContainer.innerHTML = "";
  filterRow?.classList.toggle("filter-layout-classic", appState.filterLayoutMode === "classic");
  tagContainer.classList.toggle("classic-tag-filters", appState.filterLayoutMode === "classic");
  locationContainer.classList.toggle("classic-location-placeholder", appState.filterLayoutMode === "classic");

  try {
    const primaryTags = await GetPrimaryTags();
    if (appState.filterLayoutMode === "classic") {
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
  if (!secondaryGroup) return;

  if (secondaryGroup?.classList.contains("filter-select-group")) {
    await renderSecondaryTagDropdown(secondaryGroup, primaryTag);
    return;
  }

  await renderSecondaryTagButtons(secondaryGroup, primaryTag);
}

async function renderSecondaryTagButtons(secondaryGroup, primaryTag) {
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
    const secondaryTags = await GetSecondaryTags(primaryTag || "");

    if (secondaryTags.length > 0) {
      secondaryTags.sort((a, b) => a.localeCompare(b, "zh-CN"));
      secondaryGroup.style.display = "flex";
      setClassicSecondarySearchVisible(true);

      const container = document.createElement("div");
      container.className = "secondary-tags-container";

      secondaryTags.forEach((tag) => {
        const tagBtn = createSecondaryTagButton(tag);
        container.appendChild(tagBtn);
      });

      tagsSlot.appendChild(container);

      const emptyHint = document.createElement("span");
      emptyHint.className = "classic-secondary-empty-hint hidden";
      emptyHint.textContent = "没有匹配的子标签";
      tagsSlot.appendChild(emptyHint);

      renderSecondaryTagPresets(actionSlot);
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
      renderSecondaryTagPresets(actionSlot);
      addSecondaryMatchModeControl(actionSlot);
      syncSecondaryTagFilterUI();
    }
  } catch (error) {
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

function scheduleSecondaryTagsCollapse(container, actionSlot) {
  const run = () => updateSecondaryTagsCollapse(container, actionSlot);
  requestAnimationFrame(() => {
    if (!run()) {
      setTimeout(run, 80);
    }
  });
}

function updateClassicSecondaryTagsCollapse() {
  const container = document.querySelector(".filter-row-filters.filter-layout-classic .secondary-tags-container");
  const actionSlot = document.querySelector(".filter-row-filters.filter-layout-classic .classic-secondary-action-slot");
  if (container && actionSlot) {
    updateSecondaryTagsCollapse(container, actionSlot);
  }
}

function updateSecondaryTagsCollapse(container, actionSlot) {
  if (!document.body.contains(container)) return true;

  const rect = container.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return false;

  let expandBtn = actionSlot.querySelector(".expand-tags-btn");
  const tagButtons = Array.from(container.children).filter((button) => !button.hidden);
  if (tagButtons.length === 0) {
    container.classList.remove("collapsed");
    container.dataset.expanded = "";
    expandBtn?.remove();
    return true;
  }

  const firstRowTop = tagButtons[0]?.offsetTop ?? 0;
  const hasMultipleRows = tagButtons.some((button) => button.offsetTop > firstRowTop);

  if (!hasMultipleRows) {
    container.classList.remove("collapsed");
    container.dataset.expanded = "";
    expandBtn?.remove();
    return true;
  }

  if (!expandBtn) {
    container.classList.add("collapsed");
    container.dataset.expanded = "false";

    expandBtn = document.createElement("button");
    expandBtn.className = "expand-tags-btn";
    expandBtn.onclick = () => {
      const willExpand = container.classList.contains("collapsed");
      container.classList.toggle("collapsed", !willExpand);
      container.dataset.expanded = willExpand ? "true" : "false";
      syncExpandButton(expandBtn, willExpand);
    };
    actionSlot.appendChild(expandBtn);
  } else if (container.dataset.expanded !== "true") {
    container.classList.add("collapsed");
  }

  syncExpandButton(expandBtn, container.dataset.expanded === "true");
  return true;
}

function syncExpandButton(button, isExpanded) {
  button.innerHTML = isExpanded
    ? '<span class="icon">▲</span> 收起'
    : '<span class="icon">▼</span> 展开';
}

async function renderSecondaryTagDropdown(secondaryGroup, primaryTag) {
  secondaryGroup.querySelectorAll(".secondary-filter-dropdown").forEach((el) => el.remove());
  secondaryGroup.querySelectorAll(".multi-select-trigger.is-disabled").forEach((el) => el.remove());

  // 移除原有的隐藏逻辑，始终显示子标签
  // 当 primaryTag 为空时，后端会返回所有文件的二级标签去重
  secondaryGroup.classList.remove("is-empty");
  secondaryGroup.style.display = "flex";
  secondaryGroup.style.visibility = "visible";

  try {
    // 后端已支持空 primaryTag，返回所有二级标签去重
    const secondaryTags = await GetSecondaryTags(primaryTag || "");
    if (!secondaryTags.length) {
      const emptyTrigger = document.createElement("button");
      emptyTrigger.type = "button";
      emptyTrigger.className = "select-trigger multi-select-trigger is-disabled";
      emptyTrigger.textContent = "暂无已识别标签";
      emptyTrigger.disabled = true;
      secondaryGroup.appendChild(emptyTrigger);
      renderSecondaryTagPresets(secondaryGroup);
      syncSecondaryTagFilterUI();
      return;
    }

    secondaryTags.sort((a, b) => a.localeCompare(b, "zh-CN"));

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
        const label = document.createElement("label");
        label.className = "multi-select-option";
        label.innerHTML = `
          <input type="checkbox" value="${escapeHtml(tag)}" ${appState.selectedSecondaryTags.includes(tag) ? "checked" : ""}>
          <span>${escapeHtml(tag)}</span>
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
    renderSecondaryTagPresets(secondaryGroup);
    syncSecondaryTagFilterUI();
  } catch (error) {
    console.error("获取二级标签失败:", error);
    secondaryGroup.classList.add("is-empty");
    secondaryGroup.style.display = "flex";
    secondaryGroup.style.visibility = "hidden";
  }
}

function createSecondaryTagButton(tag) {
  const button = document.createElement("button");
  button.className = "secondary-tag-btn";
  button.textContent = tag;
  button.dataset.tag = tag;

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
  button.className = "location-tag-btn";
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
  button.className = "game-state-tag-btn";
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
    updateSortButtonUI();

    await performSearch();
  } finally {
    appState.isLoading = false;
    hideFileListLoading();
  }
}

export function handleSearch(event) {
  appState.searchQuery = event.target.value;
  performSearch();
}

export async function performSearch() {
  try {
    console.log(
      "执行搜索，查询词:", appState.searchQuery,
      "一级标签:", appState.selectedPrimaryTag,
      "二级标签:", appState.selectedSecondaryTags,
      "位置:", appState.selectedLocations,
      "游戏内:", appState.selectedGameStates,
      "二级匹配:", appState.secondaryMatchMode
    );

    if (
      !appState.searchQuery &&
      !appState.selectedPrimaryTag &&
      appState.selectedSecondaryTags.length === 0
    ) {
      appState.vpkFiles = [...appState.allVpkFiles];
    } else {
      const results = await SearchVPKFiles(
        appState.searchQuery,
        appState.selectedPrimaryTag,
        appState.selectedSecondaryTags
      );
      appState.vpkFiles = results;
    }

    if (appState.selectedLocations.length > 0) {
      appState.vpkFiles = appState.vpkFiles.filter((file) =>
        appState.selectedLocations.includes(file.location)
      );
    }

    if (appState.selectedGameStates.length > 0) {
      appState.vpkFiles = appState.vpkFiles.filter((file) =>
        appState.selectedGameStates.includes(getGameState(file))
      );
    }

    if (appState.selectedSecondaryTags.length > 0 && appState.secondaryMatchMode === "all") {
      appState.vpkFiles = appState.vpkFiles.filter(matchesSecondaryTags);
    }

    if (!appState.showHidden) {
      appState.vpkFiles = appState.vpkFiles.filter(
        (file) => !file.name.startsWith("_")
      );
    }

    applySort(appState.vpkFiles);
    renderFileList();
    updateStatusBar();
    renderActiveFilterSummary();

    console.log(`搜索完成，显示 ${appState.vpkFiles.length} 个文件`);
  } catch (error) {
    console.error("搜索失败:", error);
    showError("搜索失败: " + error);
  }
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

export async function refreshFilesKeepFilter() {
  resetBoxSelection();

  if (!appState.currentDirectory) {
    showNotification("请先选择目录", "info");
    return;
  }

  if (appState.isLoading) {
    console.log("正在加载中，请稍候...");
    return;
  }

  const currentFilters = {
    searchText: document.getElementById("search-input")?.value || "",
    primaryTag: appState.selectedPrimaryTag || "",
    secondaryTags: [...appState.selectedSecondaryTags],
    secondaryMatchMode: appState.secondaryMatchMode,
    locationTags: [...appState.selectedLocations],
    gameStates: [...appState.selectedGameStates],
  };

  appState.isLoading = true;
  showFileListLoading("正在刷新文件列表...");

  try {
    await ScanVPKFiles();

    const [files, primaryTags] = await Promise.all([
      GetVPKFiles(),
      GetPrimaryTags(),
    ]);

    applySort(files);

    appState.allVpkFiles = files;
    appState.primaryTags = primaryTags;

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

    updateStatusBar();

    console.log("文件列表已刷新，筛选状态已恢复");
  } catch (error) {
    console.error("刷新文件列表失败:", error);
    showError("刷新失败: " + error);
  } finally {
    appState.isLoading = false;
    hideFileListLoading();
  }
}
