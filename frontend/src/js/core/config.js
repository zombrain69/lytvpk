import {
  GetAppConfig,
  GetConfigMigrationVersion,
  MigrateLocalStorageConfig,
  SaveAppConfig,
} from "../../../wailsjs/go/app/App";
import { DEFAULT_UI_SCALE, normalizeUIScale } from "./ui-scale.js";

const LEGACY_STORAGE_KEYS = {
  config: "vpk-manager-config",
  theme: "theme",
  lastUpdateCheckTime: "lastUpdateCheckTime",
  servers: "vpk-manager-servers",
  recentServers: "vpk-manager-recent-servers",
  watchLaterItems: "workshop-watch-later-items",
};

const DEFAULT_CONFIG = {
  modRotationConfig: {
    enableCharacters: false,
    enableWeapons: false,
  },
  workshopPreferredIP: true,
  workshopFixedIP: "",
  workshopMetaEnabled: true,
  workshopUpdateCheckEnabled: false,
  workshopBrowserTarget: "mirror",
  workshopTranslateProvider: "microsoft",
  defaultDirectory: "",
  savedDirectories: [],
  lastActiveDirectory: "",
  displayMode: "list",
  filterLayoutMode: "compact",
  boxSelectionEnabled: true,
  ctrlClickSelectionEnabled: true,
  uiScale: DEFAULT_UI_SCALE,
  // addonlist.txt 的运行时监控属于显式选择，默认绝不启动。
  addonListGuardEnabled: false,
  theme: "",
  ignoredVersion: "",
  lastUpdateCheckTime: "",
  migrationVersion: 0,
};

// 最大保存目录数量
export const MAX_DIRECTORIES = 10;

let configCache = cloneConfig(DEFAULT_CONFIG);
// Wails 方法调用是异步的。连续快速修改多个设置时，如果直接并发调用
// SaveAppConfig，后发请求可能先落盘，导致旧快照覆盖新设置。用一个可继续的
// Promise 队列保持提交顺序；即使某次保存失败，后续保存也不会被拒绝链阻塞。
let configSaveQueue = Promise.resolve();

export async function migrateLegacyLocalStorageIfNeeded() {
  const migrationVersion = await GetConfigMigrationVersion();
  if (Number(migrationVersion) >= 2) {
    return false;
  }

  const payload = {
    config: localStorage.getItem(LEGACY_STORAGE_KEYS.config) || "",
    theme: localStorage.getItem(LEGACY_STORAGE_KEYS.theme) || "",
    lastUpdateCheckTime:
      localStorage.getItem(LEGACY_STORAGE_KEYS.lastUpdateCheckTime) || "",
    servers: localStorage.getItem(LEGACY_STORAGE_KEYS.servers) || "",
    recentServers:
      localStorage.getItem(LEGACY_STORAGE_KEYS.recentServers) || "",
    watchLaterItems:
      localStorage.getItem(LEGACY_STORAGE_KEYS.watchLaterItems) || "",
  };

  await MigrateLocalStorageConfig(payload);

  Object.values(LEGACY_STORAGE_KEYS).forEach((key) => {
    localStorage.removeItem(key);
  });

  return true;
}

export async function initConfig() {
  const config = await GetAppConfig();
  configCache = normalizeConfig(config);
  return getConfig();
}

// 获取配置。运行时读取内存缓存，启动时由 initConfig 从后端配置文件填充。
export function getConfig() {
  return cloneConfig(configCache);
}

// 保存配置到后端 config.json，同时立即更新本地缓存。
export function saveConfig(config) {
  configCache = normalizeConfig({
    ...configCache,
    ...config,
  });
  // 每次保存都冻结独立快照，避免调用方随后修改对象影响排队中的请求。
  const snapshot = cloneConfig(configCache);
  const savePromise = configSaveQueue
    .catch(() => undefined)
    .then(() => SaveAppConfig(snapshot));
  configSaveQueue = savePromise;
  savePromise.catch((error) => {
    console.error("保存配置失败:", error);
  });
  return savePromise;
}

export function updateConfigCache(patch) {
  configCache = normalizeConfig({
    ...configCache,
    ...patch,
  });
  return getConfig();
}

function normalizeConfig(config = {}) {
  const next = {
    ...DEFAULT_CONFIG,
    ...config,
  };

  next.modRotationConfig = {
    ...DEFAULT_CONFIG.modRotationConfig,
    ...(config.modRotationConfig || {}),
  };
  next.savedDirectories = Array.isArray(config.savedDirectories)
    ? config.savedDirectories
    : [];
  next.displayMode = next.displayMode || DEFAULT_CONFIG.displayMode;
  next.filterLayoutMode =
    next.filterLayoutMode || DEFAULT_CONFIG.filterLayoutMode;
  next.workshopBrowserTarget =
    next.workshopBrowserTarget || DEFAULT_CONFIG.workshopBrowserTarget;
  next.workshopTranslateProvider =
    next.workshopTranslateProvider === "yandex" || next.workshopTranslateProvider === "custom"
      ? next.workshopTranslateProvider
      : DEFAULT_CONFIG.workshopTranslateProvider;
  next.uiScale = normalizeUIScale(next.uiScale);
  next.migrationVersion = Number(next.migrationVersion) || 0;

  return cloneConfig(next);
}

function cloneConfig(config) {
  return JSON.parse(JSON.stringify(config));
}
