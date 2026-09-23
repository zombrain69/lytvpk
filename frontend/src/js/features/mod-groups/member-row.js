// 单个 Mod 的成员行：详情 / 游戏开关 / 启用·禁用 / 复制到 addons（+ 可选的自定义动作）。
//
// 「分组建议」卡片与「策略组管理」窗口共用这一份实现 —— 两处各写一遍必然走样
// （按钮语义、禁用条件、位置/游戏开关徽标都要跟冲突检测界面保持一致）。
//
// 元素上同时保留 `mod-group-suggest-member*` 这套历史类名：现有 CSS 与
// "折叠时隐藏成员明细与操作区"的规则都按它匹配，重命名只会带来无谓的样式回归。

import { showError } from "../../core/toast.js";
import { showFileDetail } from "../modals/detail.js";
import { moveFileToAddons, toggleFile, toggleGameEnabled } from "../file-list/operations.js";
import { describeSuggestionMember } from "./group-view.mjs";

/**
 * buildModMemberRow 生成一行成员。
 *
 * @param {object} options
 * @param {{key: string, name: string, selected?: boolean}} options.row 成员（addonlist 键 + 显示名）
 * @param {object|null} options.file 当前文件列表里对应的文件（找不到时传 null）
 * @param {string} [options.priorityLabel] 已格式化的优先级/分层文案
 * @param {{checked: boolean, title?: string, onChange?: (checked: boolean) => void}} [options.pick]
 *        是否显示勾选框（分组建议用；管理窗口不传）
 * @param {Array<{text: string, className?: string, title?: string, disabled?: boolean, onClick: () => any}>} [options.extraActions]
 *        追加的动作按钮（例如管理窗口的「移出本组」）
 * @param {() => any} [options.onActionDone] 动作成功后的刷新回调
 * @param {(message: string) => void} [options.onError] 失败提示（默认 showError）
 * @param {string} [options.extraClass] 追加到行上的类名
 */
export function buildModMemberRow({
  row,
  file = null,
  priorityLabel = "",
  pick = null,
  extraActions = [],
  onActionDone,
  onError,
  extraClass = "",
} = {}) {
  const described = describeSuggestionMember(file, { priorityLabel });
  const reportError = typeof onError === "function" ? onError : (message) => showError(message);

  const item = document.createElement("div");
  item.className = ["mod-member-row", "mod-group-suggest-member", extraClass]
    .filter(Boolean)
    .join(" ");
  item.dataset.memberKey = String(row?.key || "");
  if (!described.found) item.classList.add("is-missing");

  if (pick) {
    const label = document.createElement("label");
    label.className = "mod-group-suggest-member-pick mod-member-pick";
    const checkbox = document.createElement("input");
    checkbox.type = "checkbox";
    checkbox.checked = Boolean(pick.checked);
    checkbox.dataset.memberKey = String(row?.key || "");
    if (pick.title) checkbox.title = pick.title;
    checkbox.addEventListener("change", () => pick.onChange?.(checkbox.checked));
    label.appendChild(checkbox);
    item.appendChild(label);
  }

  const info = document.createElement("div");
  info.className = "conflict-vpk-info mod-group-suggest-member-info mod-member-info";

  const text = document.createElement("span");
  text.className = "conflict-vpk-title mod-group-suggest-member-name mod-member-name";
  text.textContent = row?.name || row?.key || "";
  text.title = String(row?.key || "");
  info.appendChild(text);

  // 标题相同的 Mod（同一工坊条目在根目录与 workshop 各一份）要靠文件名键区分。
  const keyText = document.createElement("span");
  keyText.className = "conflict-vpk-filename mod-group-suggest-member-key mod-member-key";
  keyText.textContent = String(row?.key || "");
  info.appendChild(keyText);

  const locationBadge = document.createElement("span");
  locationBadge.className = "mod-group-suggest-member-badge mod-member-badge";
  locationBadge.textContent = described.location;
  info.appendChild(locationBadge);

  const gameStateBadge = document.createElement("span");
  gameStateBadge.className = "mod-group-suggest-member-badge game-state mod-member-badge";
  gameStateBadge.textContent = described.gameState;
  info.appendChild(gameStateBadge);

  if (described.found) {
    const priorityBadge = document.createElement("span");
    priorityBadge.className = `conflict-vpk-priority mod-member-priority ${priorityLabel ? "known" : "unknown"}`;
    priorityBadge.textContent = described.priority;
    priorityBadge.title = "编号来自 addonlist.txt 顺序；数字越大越靠后加载";
    info.appendChild(priorityBadge);
  }
  item.appendChild(info);

  const actions = document.createElement("div");
  actions.className = "conflict-vpk-actions mod-group-suggest-member-actions mod-member-actions";

  const runAction = async (action) => {
    try {
      await action();
      if (typeof onActionDone === "function") await onActionDone();
    } catch (error) {
      reportError("操作失败: " + String(error?.message || error));
    }
  };

  const detailButton = document.createElement("button");
  detailButton.type = "button";
  detailButton.className = "btn btn-small btn-conflict-action mod-member-action";
  detailButton.textContent = "详情";
  detailButton.title = described.found ? "查看这个 Mod 的详情" : "该 Mod 不在当前列表中，无法查看详情";
  detailButton.disabled = !described.found;
  detailButton.addEventListener("click", () => {
    if (file?.path) showFileDetail(file.path);
  });
  actions.appendChild(detailButton);

  if (described.found) {
    const gameButton = document.createElement("button");
    gameButton.type = "button";
    gameButton.className = "btn btn-small btn-conflict-action mod-member-action";
    gameButton.textContent = described.gameState.replace("游戏开关：", "游戏开关 ");
    gameButton.disabled = !described.canEditGameState;
    gameButton.title = described.canEditGameState
      ? "编辑这个 Mod 在 addonlist.txt 里的游戏开关"
      : "该 Mod 位于 disabled 目录，无法直接编辑游戏开关";
    gameButton.addEventListener("click", () => void runAction(() => toggleGameEnabled(file.path)));
    actions.appendChild(gameButton);
  }

  if (described.fileAction) {
    const fileButton = document.createElement("button");
    fileButton.type = "button";
    // 复用冲突检测界面的语义类名，保证配色与按钮宽度一致。
    const actionClass =
      described.fileActionKind === "transfer"
        ? "btn-transfer"
        : described.fileActionKind === "enable"
          ? "btn-enable"
          : "btn-disable";
    fileButton.className = `btn btn-small btn-conflict-action ${actionClass} mod-group-suggest-member-file-action mod-member-action`;
    fileButton.textContent = described.fileAction;
    fileButton.title =
      described.fileActionKind === "transfer"
        ? "复制到 addons 目录并关闭工坊原件"
        : described.fileActionKind === "enable"
          ? "启用这个 Mod"
          : "禁用这个 Mod";
    fileButton.addEventListener("click", () => {
      const action =
        described.fileActionKind === "transfer"
          ? () => moveFileToAddons(file.path)
          : () => toggleFile(file.path);
      void runAction(action);
    });
    actions.appendChild(fileButton);
  }

  (Array.isArray(extraActions) ? extraActions : []).forEach((action) => {
    if (!action?.text) return;
    const button = document.createElement("button");
    button.type = "button";
    button.className =
      "btn btn-small btn-conflict-action mod-member-action " + String(action.className || "");
    button.textContent = action.text;
    if (action.title) button.title = action.title;
    button.disabled = Boolean(action.disabled);
    button.addEventListener("click", () => void runAction(action.onClick));
    actions.appendChild(button);
  });

  item.appendChild(actions);
  return item;
}
