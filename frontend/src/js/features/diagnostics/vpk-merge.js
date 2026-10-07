// 把选中的多个 Mod 合并成一个 VPK（导出整合包）。
//
// 用途：只想装一个文件 / 对抗模式场景。合并是"复制"，不会替代 addonlist：
// 原 Mod 的启停、优先级、冲突分析都不受影响；后一个包的同名条目覆盖前一个。

import { showError, showNotification } from "../../core/toast.js";
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
    showNotification("请先在 Mod 列表里勾选至少两个 VPK，再点「合并为整合包」", "info");
    return;
  }
  try {
    const outputPath = await deps.SelectVPKMergeOutputFile("merged_addon.vpk");
    if (!outputPath) return;
    if (!(await confirmInApp(
      `把选中的 ${selected.length} 个 VPK 合并成一个整合包？\n\n` +
        "· 后一个包的同名文件会覆盖前一个（顺序 = 你看到的列表顺序）；\n" +
        "· 合并只是导出，**不会**改动原 Mod、addonlist.txt 或优先级；\n" +
        "· 想用整合包时请自己在游戏里启用它。",
      { title: "合并为整合包" },
    ))) {
      return;
    }
    const result = await deps.MergeVPKFiles(selected, outputPath);
    showNotification(
      `已导出整合包：${result.totalEntries} 个条目（覆盖 ${result.overwritten}）→ ${result.outputPath}`,
      "success",
    );
  } catch (error) {
    showError("合并 VPK 失败: " + error);
  }
}
