// i18n 文案层（与 DOM / Wails 无关，node --test 覆盖）。
//
// 约定：
//   - 文案 key 用 "点分层级"：`toolbox.title`、`toolbox.card.problemScan.title`；
//   - 缺失 key 时返回 key 本身而不是抛错：迁移期允许"先接线、后补文案"，
//     这样任何一个页面漏翻都不会让界面变成空白；
//   - 参数用 `{name}` 占位，缺失参数保持原样，便于定位。

export const DEFAULT_LOCALE = "zh-CN";

function lookup(messages, key) {
  if (!messages || typeof key !== "string" || !key) return undefined;
  const segments = key.split(".");
  let cursor = messages;
  for (const segment of segments) {
    if (!cursor || typeof cursor !== "object") return undefined;
    cursor = cursor[segment];
  }
  return typeof cursor === "string" ? cursor : undefined;
}

export function interpolate(template, params) {
  if (typeof template !== "string") return "";
  if (!params || typeof params !== "object") return template;
  return template.replace(/\{(\w+)\}/g, (match, name) => {
    if (!(name in params)) return match;
    const value = params[name];
    return value === null || value === undefined ? "" : String(value);
  });
}

export function createI18n(options = {}) {
  const catalogs = options.catalogs && typeof options.catalogs === "object" ? { ...options.catalogs } : {};
  let locale = String(options.locale || DEFAULT_LOCALE);

  const fallbackCatalog = catalogs[DEFAULT_LOCALE] || {};

  function t(key, params) {
    const catalog = catalogs[locale] || {};
    const template = lookup(catalog, key) ?? lookup(fallbackCatalog, key);
    if (template === undefined) return String(key ?? "");
    return interpolate(template, params);
  }

  return {
    t,
    getLocale: () => locale,
    setLocale(next) {
      const value = String(next || "").trim();
      if (!value) return locale;
      locale = value;
      return locale;
    },
    hasLocale: (value) => Object.prototype.hasOwnProperty.call(catalogs, String(value || "")),
    availableLocales: () => Object.keys(catalogs).sort(),
    hasKey: (key) => lookup(catalogs[locale] || {}, key) !== undefined || lookup(fallbackCatalog, key) !== undefined,
  };
}
