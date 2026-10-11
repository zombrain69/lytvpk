// 工坊快捷方式（.url）的候选来源预判 —— 仅用于右键菜单的入口可见性。
//
// 真正的 ID 解析与校验在后端 `GenerateWorkshopShortcuts`（internal/app/workshop_shortcut.go）
// 里做，解析链与前端 modals/detail.js 的 resolveWorkshopID 同序：
//   1. .meta 里的 workshop_id（扫描结果落在 file.workshopId）；
//   2. workshop\ 目录下的纯数字文件名（Steam 官方投递的原样命名，
//      隐藏的 _ 前缀先剥掉再判断）；
//   3. addoninfo.txt 的 addonURL0。
// 这里只做同步粗筛（合法 ID 的严格校验 —— 纯数字且 ≥ 100000 —— 留给后端），
// 避免给"根本无从解析"的文件显示入口；后端仍旧对每个文件独立复核。

// 有效工坊 ID 至少 6 位（protocol.IsValidWorkshopID 要求 ≥ 100000），
// 这里放宽到 6 位起做粗筛，真实 ID 通常是 9–10 位。
const WORKSHOP_NUMERIC_STEM = /^\d{6,}$/;

/** hasWorkshopShortcutSource 判断该 Mod 是否可能解析出工坊 ID。 */
export function hasWorkshopShortcutSource(file) {
  if (!file) return false;
  if (String(file.workshopId ?? "").trim() !== "") return true;
  if (file.location === "workshop") {
    const stem = String(file.name ?? "")
      .replace(/\.vpk$/i, "")
      .replace(/^_+/, "");
    if (WORKSHOP_NUMERIC_STEM.test(stem)) return true;
  }
  if (String(file.addonURL0 ?? "").trim() !== "") return true;
  return false;
}
