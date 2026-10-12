// 命名例外（路线图第 20 项 P3）——右键批量菜单的快捷入口。
//
// 把所选文件加入 / 移出「命名规范化例外表」（配置目录 naming_rules.json）。
// 例外表只影响批量规范化的建议（计划阶段直接跳过），不动任何 Mod 文件本身；
// 设置页「命名规范化」卡片里可以查看与移除条目。
import { showNotification, showError } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { AddNamingExemptions, RemoveNamingExemptions } from "../../../../wailsjs/go/app/App";

async function runNamingExemption(paths, action) {
  if (!Array.isArray(paths) || paths.length === 0) return;
  try {
    const result = await action(paths);
    const added = Number(result?.added) || 0;
    const removed = Number(result?.removed) || 0;
    const skipped = Array.isArray(result?.skipped) ? result.skipped.length : 0;
    if (added > 0) {
      showNotification(t("mods.contextMenu.namingExemptAdded", { count: added }), "success");
    }
    if (removed > 0) {
      showNotification(t("mods.contextMenu.namingExemptRemoved", { count: removed }), "success");
    }
    if (skipped > 0) {
      showNotification(t("mods.contextMenu.namingExemptSkipped", { count: skipped }), "info");
    }
  } catch (error) {
    showError(
      t("mods.contextMenu.namingExemptFailed", { error: String(error?.message || error) }),
    );
  }
}

// addNamingExemptionsForPaths 把所选文件加入命名例外表。
export function addNamingExemptionsForPaths(paths) {
  return runNamingExemption(paths, AddNamingExemptions);
}

// removeNamingExemptionsForPaths 把所选文件移出命名例外表。
export function removeNamingExemptionsForPaths(paths) {
  return runNamingExemption(paths, RemoveNamingExemptions);
}
