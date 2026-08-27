import { appState } from "../state.js";
import { showNotification, showError } from "../../core/toast.js";
import { renderFileList } from "./render.js";
import { GetAddonListOrder } from "../../../../wailsjs/go/app/App";

export function setupSortEvents() {
  const sortBtn = document.getElementById("sort-btn");
  const dropdown = document.getElementById("sort-dropdown-content");

  if (sortBtn && dropdown) {
    sortBtn.addEventListener("click", (e) => {
      e.stopPropagation();
      dropdown.classList.toggle("hidden");
    });

    document.addEventListener("click", (e) => {
      if (!sortBtn.contains(e.target) && !dropdown.contains(e.target)) {
        dropdown.classList.add("hidden");
      }
    });
  }

  document
    .getElementById("sort-name-btn")
    ?.addEventListener("click", () => handleSortChange("name"));
  document
    .getElementById("sort-date-btn")
    ?.addEventListener("click", () => handleSortChange("date"));
  document
    .getElementById("sort-size-btn")
    ?.addEventListener("click", () => handleSortChange("size"));
  document
    .getElementById("sort-model-complexity-btn")
    ?.addEventListener("click", () => handleModelComplexitySort());
  document
    .getElementById("sort-load-order-btn")
    ?.addEventListener("click", () => handleLoadOrderSort());

  updateSortButtonUI();
}

export async function handleLoadOrderSort() {
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  try {
    const orderList = await refreshLoadOrderMap();
    console.log("获取到加载顺序:", orderList.length, "个条目");

    appState.sortType = "loadOrder";
    appState.sortOrder = "asc";

    updateSortButtonUI();
    applySort(appState.vpkFiles);
    renderFileList();

    showNotification("已按加载顺序排序", "success");
  } catch (err) {
    console.error("获取加载顺序失败:", err);
    showError("addonlist.txt 错误: " + err);
  }
}

export function handleSortChange(type) {
  if (appState.sortType === type) {
    appState.sortOrder = appState.sortOrder === "asc" ? "desc" : "asc";
  } else {
    appState.sortType = type;
    appState.sortOrder = type === "date" || type === "size" ? "desc" : "asc";
  }

  updateSortButtonUI();
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  applySort(appState.vpkFiles);
  renderFileList();
}

export async function handleModelComplexitySort() {
  document.getElementById("sort-dropdown-content")?.classList.add("hidden");

  try {
    const method = window?.go?.app?.App?.GetVPKModelMetrics;
    if (typeof method !== "function") {
      throw new Error("当前应用未提供模型复杂度统计，请重新构建并启动 LytVPK");
    }

    const paths = appState.allVpkFiles.map((file) => file.path);
    showNotification("正在分析 VPK 模型复杂度...", "info");
    const metrics = await method(paths);
    const metricMap = new Map(metrics.map((metric) => [metric.path, metric]));

    [appState.allVpkFiles, appState.vpkFiles].forEach((files) => {
      files.forEach((file) => {
        const metric = metricMap.get(file.path);
        if (!metric || metric.error) return;
        file.modelStatsKnown = true;
        file.modelCount = metric.modelCount || 0;
        file.modelVertices = metric.totalVertices || 0;
        file.modelTriangles = metric.totalTriangles || 0;
      });
    });

    if (appState.sortType === "modelComplexity") {
      appState.sortOrder = appState.sortOrder === "asc" ? "desc" : "asc";
    } else {
      appState.sortType = "modelComplexity";
      appState.sortOrder = "desc";
    }

    updateSortButtonUI();
    applySort(appState.vpkFiles);
    renderFileList();
    showNotification("已按模型复杂度排序（LOD0 总顶点）", "success");
  } catch (error) {
    console.error("模型复杂度排序失败:", error);
    showError("模型复杂度排序失败: " + error);
  }
}

/**
 * 重新读取 addonlist.txt 的顺序映射。
 *
 * 列表排序使用的是 addonlist.txt 的相对键（例如
 * workshop\\123456.vpk），不能只按文件名建立映射，否则根目录与工坊中
 * 同名 VPK 会互相覆盖。该函数单独导出，供加载顺序写入后先同步映射，
 * 再触发完整文件列表刷新。
 */
export async function refreshLoadOrderMap() {
  const orderList = await GetAddonListOrder();
  appState.loadOrderMap.clear();
  (orderList || []).forEach((name, index) => {
    const key = normalizeLoadOrderKey(name);
    if (key) {
      appState.loadOrderMap.set(key, index);
    }
  });
  return orderList || [];
}

function normalizeLoadOrderKey(value) {
  return String(value || "")
    .trim()
    .replaceAll("/", "\\")
    .replace(/^\.\\/, "")
    .toLowerCase();
}

function getFileLoadOrderKeys(file) {
  const keys = [];
  const name = normalizeLoadOrderKey(file?.name);
  const path = normalizeLoadOrderKey(file?.path);
  const root = normalizeLoadOrderKey(appState.currentDirectory);

  if (root && path.startsWith(`${root}\\`)) {
    keys.push(path.slice(root.length + 1));
  }
  if (file?.location === "disabled" && name) {
    // disabled 目录中的文件在受管理的 addonlist 中仍对应其启用前的键。
    keys.push(`disabled\\${name}`);
    keys.push(name);
  } else if (file?.location === "workshop" && name) {
    keys.push(`workshop\\${name}`);
    keys.push(name);
  } else if (name) {
    keys.push(name);
  }

  return [...new Set(keys.filter(Boolean))];
}

function getFileLoadOrderIndex(file) {
  for (const key of getFileLoadOrderKeys(file)) {
    const index = appState.loadOrderMap.get(key);
    if (index !== undefined) return index;
  }
  return undefined;
}

export function updateSortButtonUI() {
  const btnText = document.getElementById("sort-btn-text");
  const nameBtn = document.getElementById("sort-name-btn");
  const dateBtn = document.getElementById("sort-date-btn");
  const sizeBtn = document.getElementById("sort-size-btn");
  const modelComplexityBtn = document.getElementById("sort-model-complexity-btn");
  const loadOrderBtn = document.getElementById("sort-load-order-btn");

  let text = "文件名排序";
  let arrow = "";

  if (appState.sortType === "name") {
    text = "文件名排序";
    arrow = appState.sortOrder === "asc" ? "(A-Z)" : "(Z-A)";
  } else if (appState.sortType === "date") {
    text = "更新时间排序";
    arrow = appState.sortOrder === "desc" ? "(最新)" : "(最旧)";
  } else if (appState.sortType === "loadOrder") {
    text = "加载顺序排序";
    arrow = appState.sortOrder === "asc" ? "(顺序)" : "(倒序)";
  } else if (appState.sortType === "size") {
    text = "VPK 大小排序";
    arrow = appState.sortOrder === "desc" ? "(由大到小)" : "(由小到大)";
  } else if (appState.sortType === "modelComplexity") {
    text = "模型复杂度排序";
    arrow = appState.sortOrder === "desc" ? "(高到低)" : "(低到高)";
  }

  if (btnText) btnText.textContent = `${text} ${arrow}`;

  if (nameBtn) {
    nameBtn.classList.toggle("active", appState.sortType === "name");
  }
  if (dateBtn) {
    dateBtn.classList.toggle("active", appState.sortType === "date");
  }
  if (sizeBtn) {
    sizeBtn.classList.toggle("active", appState.sortType === "size");
  }
  if (modelComplexityBtn) {
    modelComplexityBtn.classList.toggle("active", appState.sortType === "modelComplexity");
  }
  if (loadOrderBtn) {
    loadOrderBtn.classList.toggle("active", appState.sortType === "loadOrder");
  }
}

export function applySort(files) {
  return files.sort((a, b) => {
    let result = 0;

    if (appState.sortType === "date") {
      const dateA = a.lastModified ? new Date(a.lastModified).getTime() : 0;
      const dateB = b.lastModified ? new Date(b.lastModified).getTime() : 0;
      result = dateA - dateB;
    } else if (appState.sortType === "size") {
      result = Number(a.size || 0) - Number(b.size || 0);
    } else if (appState.sortType === "modelComplexity") {
      result = Number(a.modelVertices || 0) - Number(b.modelVertices || 0);
    } else if (appState.sortType === "loadOrder") {
      const orderA = getFileLoadOrderIndex(a);
      const orderB = getFileLoadOrderIndex(b);
      const inListA = orderA !== undefined;
      const inListB = orderB !== undefined;

      if (inListA && inListB) {
        result = orderA - orderB;
      } else if (!inListA && !inListB) {
        const nameA = a.name.toLowerCase();
        const nameB = b.name.toLowerCase();
        result = nameA.localeCompare(nameB, "zh-CN", {
          numeric: true,
          sensitivity: "accent",
        });
      } else {
        if (inListA) {
          result = -1;
        } else {
          result = 1;
        }
      }
      return result;
    } else {
      const nameA = a.name.toLowerCase();
      const nameB = b.name.toLowerCase();

      result = nameA.localeCompare(nameB, "zh-CN", {
        numeric: true,
        sensitivity: "accent",
      });
    }

    if (appState.sortOrder === "desc") {
      result = -result;
    }

    if (result === 0) {
      if (appState.sortType === "date") {
        return a.name.localeCompare(b.name, "zh-CN", { numeric: true });
      }
      return a.path.localeCompare(b.path);
    }

    return result;
  });
}
