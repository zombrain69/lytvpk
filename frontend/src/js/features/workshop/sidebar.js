import { loadWorkshopList } from "./list.js";
import { browserState, resetWorkshopPaging } from "./state.js";
import { t } from "../../core/i18n-runtime.js";

const SORT_ICONS = {
  trend: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><path d="M12 20V4"/><path d="M17 9l-5-5-5 5"/></svg>`,
  recent: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><polyline points="12 6 12 12 16 14"/></svg>`,
  top: `<svg class="icon-svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>`,
};

const WORKSHOP_CATEGORIES = [
  {
    nameKey: "workshop.filter.survivors",
    children: [
      { name: "Bill", tag: "Bill" },
      { name: "Francis", tag: "Francis" },
      { name: "Louis", tag: "Louis" },
      { name: "Zoey", tag: "Zoey" },
      { name: "Coach", tag: "Coach" },
      { name: "Ellis", tag: "Ellis" },
      { name: "Nick", tag: "Nick" },
      { name: "Rochelle", tag: "Rochelle" },
    ],
  },
  {
    nameKey: "workshop.filter.infected",
    children: [
      { nameKey: "workshop.filter.specialInfected", tag: "Special Infected" },
      { name: "Tank", tag: "Tank" },
      { name: "Witch", tag: "Witch" },
      { name: "Hunter", tag: "Hunter" },
      { name: "Smoker", tag: "Smoker" },
      { name: "Boomer", tag: "Boomer" },
      { name: "Charger", tag: "Charger" },
      { name: "Jockey", tag: "Jockey" },
      { name: "Spitter", tag: "Spitter" },
      { nameKey: "workshop.filter.commonInfected", tag: "Common Infected" },
    ],
  },
  {
    nameKey: "workshop.filter.modes",
    children: [
      { nameKey: "workshop.filter.campaigns", tag: "Campaigns" },
      { nameKey: "workshop.filter.coop", tag: "Co-op" },
      { nameKey: "workshop.filter.survival", tag: "Survival" },
      { nameKey: "workshop.filter.versus", tag: "Versus" },
      { nameKey: "workshop.filter.scavenge", tag: "Scavenge" },
      { nameKey: "workshop.filter.realism", tag: "Realism" },
      { nameKey: "workshop.filter.realismVersus", tag: "Realism Versus" },
      { nameKey: "workshop.filter.mutations", tag: "Mutations" },
      { nameKey: "workshop.filter.singlePlayer", tag: "Single Player" },
    ],
  },
  {
    nameKey: "workshop.filter.weapons",
    children: [
      { nameKey: "workshop.filter.rifle", tag: "Rifle" },
      { nameKey: "workshop.filter.smg", tag: "SMG" },
      { nameKey: "workshop.filter.shotgun", tag: "Shotgun" },
      { nameKey: "workshop.filter.sniper", tag: "Sniper" },
      { nameKey: "workshop.filter.pistol", tag: "Pistol" },
      { nameKey: "workshop.filter.melee", tag: "Melee" },
      { nameKey: "workshop.filter.grenadeLauncher", tag: "Grenade Launcher" },
      { name: "M60", tag: "M60" },
      { nameKey: "workshop.filter.throwable", tag: "Throwable" },
    ],
  },
  {
    nameKey: "workshop.filter.items",
    children: [
      { nameKey: "workshop.filter.medkit", tag: "Medkit" },
      { nameKey: "workshop.filter.defibrillator", tag: "Defibrillator" },
      { nameKey: "workshop.filter.adrenaline", tag: "Adrenaline" },
      { nameKey: "workshop.filter.pills", tag: "Pills" },
    ],
  },
  {
    nameKey: "workshop.filter.moreResources",
    children: [
      { name: "UI", tag: "UI" },
      { nameKey: "workshop.filter.sounds", tag: "Sounds" },
      { nameKey: "workshop.filter.scripts", tag: "Scripts" },
      { nameKey: "workshop.filter.models", tag: "Models" },
      { nameKey: "workshop.filter.textures", tag: "Textures" },
      { nameKey: "workshop.filter.miscellaneous", tag: "Miscellaneous" },
      { nameKey: "workshop.filter.other", tag: "Other" },
    ],
  },
];

export function renderWorkshopSidebar() {
  const container = document.getElementById("browser-sidebar-content");
  if (!container) return;

  container.innerHTML = "";

  WORKSHOP_CATEGORIES.forEach((cat) => {
    const group = document.createElement("div");
    group.className = "filter-group filter-group-category";

    if (cat.name !== "全部") {
      const title = document.createElement("h4");
      title.textContent = displayFilterName(cat);
      group.appendChild(title);
    }

    const list = document.createElement("ul");
    list.className = "filter-list";

    if (cat.tag !== undefined) {
      renderFilterItem(list, displayFilterName(cat), cat.tag, cat.searchText, true);
    }

    if (cat.children) {
      cat.children.forEach((child) => {
        renderFilterItem(list, displayFilterName(child), child.tag, child.searchText);
      });
    }

    group.appendChild(list);
    container.appendChild(group);
  });

  addFilterIcons();
  initBrowserIndicators();
}

export function addFilterIcons() {
  document.querySelectorAll("#browser-sort-list .filter-item").forEach((item) => {
    const sort = item.dataset.sort;
    if (!sort || item.querySelector(".filter-item-icon")) return;
    item.insertAdjacentHTML(
      "afterbegin",
      `<span class="filter-item-icon">${SORT_ICONS[sort] || SORT_ICONS.trend}</span>`
    );
  });
}

export function initBrowserIndicators() {
  const sortList = document.querySelector("#browser-sort-list");
  if (sortList && !sortList.querySelector(".browser-sort-indicator")) {
    sortList.insertAdjacentHTML("afterbegin", `<div class="browser-sort-indicator" aria-hidden="true"></div>`);
  }

  const sidebarContent = document.getElementById("browser-sidebar-content");
  if (sidebarContent && !sidebarContent.querySelector(".browser-category-indicator")) {
    sidebarContent.insertAdjacentHTML("afterbegin", `<div class="browser-category-indicator" aria-hidden="true"></div>`);
  }

  updateBrowserSortIndicator(true);
  updateBrowserCategoryIndicator(true);
}

export function updateBrowserSortIndicator(skipTransition = false) {
  const sortList = document.querySelector("#browser-sort-list");
  const indicator = sortList?.querySelector(".browser-sort-indicator");
  const activeItem = sortList?.querySelector(".filter-item.active");

  if (!sortList || !indicator || !activeItem) return;

  if (skipTransition) {
    indicator.style.transition = "none";
  }

  const listRect = sortList.getBoundingClientRect();
  const itemRect = activeItem.getBoundingClientRect();
  const offset = itemRect.top - listRect.top;

  indicator.style.height = `${itemRect.height}px`;
  indicator.style.transform = `translateY(${offset}px)`;

  if (skipTransition) {
    indicator.offsetHeight;
    indicator.style.transition = "";
  }
}

export function updateBrowserCategoryIndicator(skipTransition = false) {
  const sidebarContent = document.getElementById("browser-sidebar-content");
  const indicator = sidebarContent?.querySelector(".browser-category-indicator");
  const activeItem = sidebarContent?.querySelector(".filter-item.active");

  if (!sidebarContent || !indicator) return;

  if (!activeItem) {
    indicator.classList.remove("visible");
    return;
  }

  const containerRect = sidebarContent.getBoundingClientRect();
  const itemRect = activeItem.getBoundingClientRect();
  const newOffset = itemRect.top - containerRect.top;
  const wasVisible = indicator.classList.contains("visible");

  if (!wasVisible || skipTransition) {
    indicator.style.transition = "none";
    indicator.style.height = `${itemRect.height}px`;
    indicator.style.transform = `translateY(${newOffset}px)`;
    indicator.offsetHeight;
    indicator.style.transition = "";
    indicator.classList.add("visible");
  } else {
    indicator.classList.add("visible");
    indicator.style.height = `${itemRect.height}px`;
    indicator.style.transform = `translateY(${newOffset}px)`;
  }
}

/** displayFilterName 分类条目存 key（nameKey）或字面名（name），显示时统一取文案。 */
function displayFilterName(entry) {
  return entry.nameKey ? t(entry.nameKey) : entry.name;
}

function renderFilterItem(parentList, name, tag, searchText) {
  const li = document.createElement("li");
  li.className = "filter-item";

  const currentTag = browserState.tags[0] || "";
  if (tag === currentTag) {
    li.classList.add("active");
  }

  li.dataset.tag = tag;
  li.textContent = name;

  li.addEventListener("click", () => {
    document.querySelectorAll("#browser-sidebar-content .filter-item").forEach((item) => item.classList.remove("active"));
    li.classList.add("active");

    updateBrowserCategoryIndicator();

    const tagsToSend = [];
    if (tag) {
      tagsToSend.push(tag);
    }
    browserState.tags = tagsToSend;

    const input = document.getElementById("browser-search-input");
    if (searchText) {
      browserState.query = searchText;
      if (input) input.value = searchText;
    } else {
      browserState.query = "";
      if (input) input.value = "";
    }

    resetWorkshopPaging();
    loadWorkshopList();
  });

  parentList.appendChild(li);
}

window.addEventListener("resize", () => {
  updateBrowserSortIndicator(true);
  updateBrowserCategoryIndicator(true);
});
