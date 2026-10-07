// 应用内弹窗的 Esc 关闭（纯逻辑层，DOM 操作留给调用方）。
//
// 为什么需要集中一份：真机扫过 9 个常用窗口后发现 7 个按 Esc 完全没反应
// （加载顺序 / 问题查找 / 冲突检测 / 模型统计 / 分组建议 / 添加服务器 / 确认弹窗），
// 而 Mod 详情、策略组管理、命令面板、输入弹窗、分组选择器各自实现了一遍 —— 风格不一。
// 这里统一成"Esc = 点最上层窗口自己的关闭/取消按钮"，行为与用户点按钮完全一致，
// 各个窗口原有的收尾逻辑（取消回调、恢复状态、锁定态）都不会被绕过。

/**
 * 按"最上层优先"列出可用 Esc 关闭的窗口（id 与 index.html 里的 modal 一致）。
 *
 * 顺序 = 层叠优先级：确认/输入/详情这类会叠在别的窗口之上，排前面；
 * 其余的通常同一时刻只会开一个。
 */
export const ESC_CLOSABLE_MODAL_IDS = [
  "command-palette-modal",
  "confirm-modal",
  "prompt-modal",
  "message-modal",
  "file-detail-modal",
  "problem-scan-modal",
  "model-stats-modal",
  "conflict-modal",
  "load-order-modal",
  "mod-group-suggest-modal",
  "group-picker-modal",
  "set-tags-modal",
  "batch-set-tags-modal",
  "rename-modal",
  "group-tag-modal",
  "server-form-modal",
  "server-details-modal",
  "direct-connect-modal",
  "conflict-scope-modal",
  "workshop-id-jump-modal",
  "agent-prompt-modal",
  "strategy-group-modal",
  "update-modal",
  "file-conflict-modal",
  "exit-confirm-modal",
  "info-modal",
  "mod-snapshot-modal",
  "vpk-preview-modal",
  "singleton-conflict-modal",
];

/**
 * 关闭按钮的查找顺序：关闭（×）→ 显式标记 → 页脚里的次要按钮。
 * **故意不含 `.btn-primary` / `.btn-danger`**：Esc 是"取消"，不能替用户点确认或删除。
 */
export const ESC_CLOSE_SELECTORS = [
  ".close-btn",
  "[data-modal-close]",
  ".modal-footer .btn-secondary",
  ".modal-footer .btn-outline",
];

/**
 * pickEscClosableModalId 从"当前可见的窗口 id"里挑出该由 Esc 关掉的那个。
 * 不在注册表里的 id 一律忽略（例如加载遮罩、图片预览这类不该被 Esc 关的）。
 */
export function pickEscClosableModalId(visibleIds) {
  const visible = new Set(
    (Array.isArray(visibleIds) ? visibleIds : []).map((id) => String(id || "")),
  );
  if (visible.size === 0) return "";
  return ESC_CLOSABLE_MODAL_IDS.find((id) => visible.has(id)) || "";
}
