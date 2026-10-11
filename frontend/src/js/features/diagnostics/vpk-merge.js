// 把选中的多个 Mod 合并成一个 VPK（导出整合包）。
//
// 用途：只想装一个文件 / 对抗模式场景。合并是"复制"，不会替代 addonlist：
// 原 Mod 的启停、优先级、冲突分析都不受影响；后一个包的同名条目覆盖前一个。

import { showError, showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { confirmInApp } from "../modals/confirm.js";
import { appState } from "../state.js";

let deps = {};

export function configureVPKMerge(nextDeps = {}) {
  deps = nextDeps || {};
}

export async function openVPKMergeTool() {
  const selected = [...(appState.selectedFiles || [])]
    .filter((path) => typeof path === "string" && path.toLowerCase().endsWith(".vpk"));
  if (selected.length < 2) {
    showNotification(t("toolbox.vpkMerge.selectFirst"), "info");
    return;
  }
  try {
    const outputPath = await deps.SelectVPKMergeOutputFile("merged_addon.vpk");
    if (!outputPath) return;
    if (!(await confirmInApp(
      t("toolbox.vpkMerge.confirmBody", { count: selected.length }),
      { title: t("toolbox.vpkMerge.title") },
    ))) {
      return;
    }
    const result = await deps.MergeVPKFiles(selected, outputPath);
    showNotification(
      t("toolbox.vpkMerge.done", { total: result.totalEntries, overwritten: result.overwritten, path: result.outputPath }),
      "success",
    );
  } catch (error) {
    showError(t("toolbox.vpkMerge.failed", { error }));
  }
}
