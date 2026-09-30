// 「策略组管理」窗口的成员批量操作：纯函数层（与 DOM 无关，node --test 覆盖）。
//
// 一组成员可能分散在三个位置、addonlist 记录也可能缺失，所以批量按钮不能
// 把同一套动作硬套到所有人身上：
//   - 「启用」只对 disabled 目录里的成员有意义（要把文件搬回 addons）；
//   - 「禁用」只对 addons 根目录里的成员有意义（要搬进 disabled）；
//   - 工坊成员对应的是「复制到 addons」（原件保留、只关工坊那份）；
//   - 「游戏开关」只改 addonlist.txt 的 0/1，disabled 里的会被跳过。
// 这里把"哪些按钮可用、为什么不可用"算清楚，界面照结果渲染。

import { normalizeGroupKey } from "./group-view.mjs";

/**
 * collectMemberBatchTargets 把勾选的成员键按可执行的动作分流。
 *
 * @param {Iterable<string>|Array<string>} memberKeys 勾选的 addonlist 键
 * @param {Map<string, object>} fileIndex buildSuggestionFileIndex 建出来的"键 -> VPKFile"
 */
export function collectMemberBatchTargets(memberKeys, fileIndex) {
  const targets = {
    total: 0,
    found: [],
    missing: [],
    enable: [],
    disable: [],
    transfer: [],
    game: [],
    gameSkipped: [],
    gameUnrecorded: [],
  };
  const seen = new Set();
  for (const rawKey of Array.isArray(memberKeys) ? memberKeys : Array.from(memberKeys || [])) {
    const key = String(rawKey ?? "").trim();
    if (!key) continue;
    const dedupe = normalizeGroupKey(key);
    if (!dedupe || seen.has(dedupe)) continue;
    seen.add(dedupe);
    targets.total += 1;

    const file = fileIndex?.get?.(dedupe) || null;
    if (!file?.path) {
      // 组成员不会因为文件被删就消失；找不到文件时只提示，不猜路径。
      targets.missing.push(key);
      continue;
    }
    targets.found.push(file.path);

    const location = String(file.location || "root");
    if (location === "disabled") {
      targets.enable.push(file.path);
      targets.gameSkipped.push(file.path);
      continue;
    }
    if (location === "workshop") {
      targets.transfer.push(file.path);
    } else if (file.enabled !== false) {
      targets.disable.push(file.path);
    }
    targets.game.push(file.path);
    if (!file.gameStateKnown) targets.gameUnrecorded.push(file.path);
  }
  return targets;
}

/**
 * memberKeysOfGroup 取一个组的成员键（去空、去重）。
 *
 * 键一律按字符串处理：groups.json 里成员既可能是字符串，也可能是 { key, name }。
 */
export function memberKeysOfGroup(group) {
  const keys = (Array.isArray(group?.members) ? group.members : [])
    .map((member) => String(typeof member === "string" ? member : member?.key || "").trim())
    .filter(Boolean);
  return [...new Set(keys)];
}

/** collectAllMemberKeys 所有策略组的成员键（同一个 Mod 属于多个组时只算一次）。 */
export function collectAllMemberKeys(groups = []) {
  const all = new Set();
  (Array.isArray(groups) ? groups : []).forEach((group) => {
    memberKeysOfGroup(group).forEach((key) => all.add(key));
  });
  return [...all];
}

/**
 * countMemberKeysOutsideScope 数出"勾选了、但不在给定范围里"的成员个数。
 *
 * 用途：成员勾选是跨组共用一个集合的（可以一次处理多个组的成员），
 * 但折叠起来的组在界面上看不见 —— 不把这个数报出来，就变成了"看不见的勾选"，
 * 批量按钮会动到用户以为没选的东西。
 */
export function countMemberKeysOutsideScope(selection, scopeKeys) {
  const scope = new Set(scopeKeys || []);
  return [...(selection || [])].filter((key) => !scope.has(key)).length;
}

/**
 * applyMemberScopeToggle 按范围整体勾选 / 取消，返回新的选择集合。
 *
 * 只动 scopeKeys 里的键，范围外的勾选**原样保留**（跨组批量是刻意保留的能力）；
 * 怕它变成"看不见的勾选"，靠 formatMemberBatchSelectionLabel 的第二个参数把数量说出来。
 */
export function applyMemberScopeToggle(selection, scopeKeys, checked) {
  const next = new Set(selection || []);
  (scopeKeys || []).forEach((key) => {
    if (checked) next.add(key);
    else next.delete(key);
  });
  return next;
}

/**
 * formatMemberBatchSelectionLabel 批量工具条上的"已选 N 个成员"。
 *
 * hiddenCount：其中有多少个在未展开的组里（看不见但生效）。有的话必须说出来，
 * 否则用户点完「全选成员」会看到计数比眼前的行数多，却不知道多在哪。
 */
export function formatMemberBatchSelectionLabel(count, hiddenCount = 0) {
  const total = Number(count) || 0;
  if (total <= 0) return "先在成员行左侧勾选 Mod";
  const hidden = Number(hiddenCount) || 0;
  return hidden > 0
    ? `已选 ${total} 个成员（其中 ${hidden} 个在未展开的组里）`
    : `已选 ${total} 个成员`;
}

// 成员批量条上的按钮顺序（与 index.html 一致）。
const MEMBER_BATCH_ACTION_IDS = ["game-on", "game-off", "enable", "disable", "transfer", "remove"];

// 一个成员都没勾选时，六个按钮的理由都是"还没选"。
// 组级批量条（strategy-group-manager.js）也是这套说法，两处保持一致。
const MEMBER_BATCH_EMPTY_TITLE = "先勾选要批量操作的成员（每行最左边的方框，或点「全选成员」）";

/**
 * describeMemberBatchActions 算出每个批量按钮是否可点、不可点时为什么。
 * 返回顺序与界面上的按钮顺序一致。
 */
export function describeMemberBatchActions(targets) {
  const t = targets || {};
  if ((t.total || 0) === 0) {
    // 原来这里回的是"没有可禁用的成员：只有 addons 根目录里的 Mod 能搬进 disabled" ——
    // 用户还没勾任何东西，这句话答非所问（真机联调时在策略组管理窗口复现）。
    return MEMBER_BATCH_ACTION_IDS.map((id) => ({
      id,
      disabled: true,
      title: MEMBER_BATCH_EMPTY_TITLE,
    }));
  }
  const game = (t.game || []).length;
  const enable = (t.enable || []).length;
  const disable = (t.disable || []).length;
  const transfer = (t.transfer || []).length;
  const missing = (t.missing || []).length;
  const skipped = (t.gameSkipped || []).length;

  const missingNote = missing > 0 ? `；${missing} 个成员的文件当前不在列表里，会被忽略` : "";
  return [
    {
      id: "game-on",
      disabled: game === 0,
      title:
        game === 0
          ? `选中的成员都不能改游戏开关（都在 disabled 目录）${missingNote}`
          : `把这些成员在 addonlist.txt 里设为开启${skipped > 0 ? `；${skipped} 个在 disabled 目录会被跳过` : ""}${missingNote}`,
    },
    {
      id: "game-off",
      disabled: game === 0,
      title:
        game === 0
          ? `选中的成员都不能改游戏开关（都在 disabled 目录）${missingNote}`
          : `把这些成员在 addonlist.txt 里设为关闭${skipped > 0 ? `；${skipped} 个在 disabled 目录会被跳过` : ""}${missingNote}`,
    },
    {
      id: "enable",
      disabled: enable === 0,
      title:
        enable === 0
          ? `没有可启用的成员：只有 disabled 目录里的 Mod 需要搬回 addons${missingNote}`
          : `把 ${enable} 个 disabled 里的成员搬回 addons${missingNote}`,
    },
    {
      id: "disable",
      disabled: disable === 0,
      title:
        disable === 0
          ? `没有可禁用的成员：只有 addons 根目录里的 Mod 能搬进 disabled${missingNote}`
          : `把 ${disable} 个根目录成员搬进 disabled${missingNote}`,
    },
    {
      id: "transfer",
      disabled: transfer === 0,
      title:
        transfer === 0
          ? `没有可复制的成员：只有创意工坊里的 Mod 需要复制到 addons${missingNote}`
          : `把 ${transfer} 个工坊成员复制到 addons（原件保留并关闭）${missingNote}`,
    },
    {
      id: "remove",
      disabled: (t.total || 0) === 0,
      title:
        (t.total || 0) === 0
          ? "先勾选要移出策略组的成员"
          : "把勾选的成员从它们所属的策略组里移出（每个组至少要保留 1 个成员）",
    },
  ];
}

/**
 * planMemberRemoval 把"要移出的成员"按所属策略组分桶。
 *
 * 一个成员只属于当前窗口里的某一个组；`wouldEmpty` 表示这一移出会把整组清空 ——
 * 后端会拒绝（组至少要保留 1 个成员），所以界面要提前把这条说清楚。
 */
export function planMemberRemoval(memberKeys, groups) {
  const wanted = new Set(
    (Array.isArray(memberKeys) ? memberKeys : Array.from(memberKeys || []))
      .map((key) => normalizeGroupKey(key))
      .filter(Boolean),
  );
  const plan = [];
  if (wanted.size === 0) return plan;

  (Array.isArray(groups) ? groups : []).forEach((group) => {
    const memberKeysOfGroup = (Array.isArray(group?.members) ? group.members : [])
      .map((member) => (typeof member === "string" ? member : member?.key))
      .filter(Boolean);
    const keys = memberKeysOfGroup.filter((key) => wanted.has(normalizeGroupKey(key)));
    if (keys.length === 0) return;
    plan.push({
      groupId: String(group.id ?? ""),
      groupName: String(group.name ?? ""),
      keys,
      memberCount: memberKeysOfGroup.length,
      wouldEmpty: keys.length >= memberKeysOfGroup.length,
    });
  });
  return plan;
}

/**
 * formatMemberBatchResult 生成批量动作完成后的一句话总结。
 *
 * firstError 是第一个失败原因：只报"1 个失败"用户没法排查
 * （文件被游戏占用 / 没权限 / 目标同名……），所以要跟着一起说。
 */
export function formatMemberBatchResult({
  label,
  succeeded,
  failed = 0,
  skipped = 0,
  firstError = "",
}) {
  const parts = [`已${label} ${succeeded} 个成员`];
  if (skipped > 0) parts.push(`${skipped} 个不适用于这个动作，已跳过`);
  if (failed > 0) parts.push(`${failed} 个失败`);
  const summary = parts.join("；");
  const reason = String(firstError || "").trim();
  return reason ? `${summary}（${reason}）` : summary;
}
