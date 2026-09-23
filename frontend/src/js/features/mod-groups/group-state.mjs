// 策略组归属状态的加载与变更通知。
//
// 单独成模块是为了避免循环依赖：filters.js 需要在刷新列表后顺带刷新组归属，
// 而 group-ui.js 又需要调用 filters.js 的刷新函数。

import { appState } from "../state.js";
import { GetModGroupMembership } from "../../../../wailsjs/go/app/App";
import { buildGroupFilterOptions, buildGroupIndex, sortGroupFilterOptions } from "./group-view.mjs";

const listeners = new Set();

/** onModGroupMembershipChanged 注册"组归属变化"回调，返回取消函数。 */
export function onModGroupMembershipChanged(listener) {
  if (typeof listener !== "function") return () => {};
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function notifyMembershipChanged() {
  listeners.forEach((listener) => {
    try {
      listener();
    } catch (error) {
      console.warn("组归属变更回调失败:", error);
    }
  });
}

/**
 * refreshModGroupMembershipState 拉取组归属并写入 appState。
 * 失败时清空而不是沿用旧值：宁可暂时不显示组徽标，也不要展示过期的分组。
 */
export async function refreshModGroupMembershipState({ silent = true } = {}) {
  let memberships = [];
  try {
    memberships = (await GetModGroupMembership()) || [];
  } catch (error) {
    if (!silent) console.warn("读取策略组归属失败:", error);
    memberships = [];
  }
  appState.modGroupMemberships = memberships;
  appState.modGroupIndex = buildGroupIndex(memberships);
  // 顺序＝组权重升序（未设置权重的组排最后）+ 子组紧跟父组：
  // 常用组在「策略组管理」窗口里给个权重，就能排到「按分组筛选」最上面。
  appState.groupFilterOptions = sortGroupFilterOptions(buildGroupFilterOptions(memberships));
  const valid = new Set(appState.groupFilterOptions.map((option) => option.id));
  appState.activeGroupFilter = new Set(
    [...(appState.activeGroupFilter || [])].filter((id) => valid.has(id)),
  );
  notifyMembershipChanged();
  return memberships;
}
