import assert from "node:assert/strict";
import test from "node:test";

import {
  applyMemberScopeToggle,
  collectAllMemberKeys,
  collectMemberBatchTargets,
  countMemberKeysOutsideScope,
  describeMemberBatchActions,
  formatMemberBatchResult,
  formatMemberBatchSelectionLabel,
  memberKeysOfGroup,
  planMemberRemoval,
} from "./member-batch.mjs";

// 成员批量操作的判据：同一组里可能同时有根目录 / 工坊 / disabled 三种成员，
// 每个批量按钮只对其中一类有意义，剩下的要"说清为什么不能点"而不是静默无事发生。

function fileIndex(entries) {
  return new Map(
    entries.map((entry) => [
      String(entry.key).toLowerCase(),
      { path: entry.path, location: entry.location, enabled: true, gameEnabled: true, gameStateKnown: true, ...entry },
    ]),
  );
}

const index = fileIndex([
  { key: "root_a.vpk", path: "E:/addons/root_a.vpk", location: "root" },
  { key: "workshop\\111.vpk", path: "E:/addons/workshop/111.vpk", location: "workshop" },
  { key: "off.vpk", path: "E:/addons/disabled/off.vpk", location: "disabled", enabled: false, gameStateKnown: false },
]);

test("按位置分流：启用只针对 disabled，禁用只针对根目录，复制只针对工坊", () => {
  const targets = collectMemberBatchTargets(
    ["root_a.vpk", "workshop\\111.vpk", "off.vpk"],
    index,
  );
  assert.equal(targets.total, 3);
  assert.deepEqual(targets.enable, ["E:/addons/disabled/off.vpk"]);
  assert.deepEqual(targets.disable, ["E:/addons/root_a.vpk"]);
  assert.deepEqual(targets.transfer, ["E:/addons/workshop/111.vpk"]);
  assert.deepEqual(targets.game, ["E:/addons/root_a.vpk", "E:/addons/workshop/111.vpk"]);
  assert.deepEqual(targets.gameSkipped, ["E:/addons/disabled/off.vpk"]);
  assert.deepEqual(targets.gameUnrecorded, []);
  assert.deepEqual(targets.missing, []);
});

test("找不到文件的成员只记进 missing，不猜路径", () => {
  const targets = collectMemberBatchTargets(["ghost.vpk", "root_a.vpk"], index);
  assert.deepEqual(targets.missing, ["ghost.vpk"]);
  assert.deepEqual(targets.found, ["E:/addons/root_a.vpk"]);
});

test("键的写法不敏感：斜杠 / 大小写不同也认得出同一个成员", () => {
  const targets = collectMemberBatchTargets(["Workshop/111.VPK", "workshop\\111.vpk"], index);
  assert.equal(targets.total, 1, "同一个成员不应被算两次");
  assert.deepEqual(targets.transfer, ["E:/addons/workshop/111.vpk"]);
});

test("未记录在 addonlist 的成员单独计数，供确认弹窗提示", () => {
  const onlyRoot = fileIndex([
    { key: "new.vpk", path: "E:/addons/new.vpk", location: "root", gameStateKnown: false },
  ]);
  const targets = collectMemberBatchTargets(["new.vpk"], onlyRoot);
  assert.deepEqual(targets.gameUnrecorded, ["E:/addons/new.vpk"]);
});

test("一个成员都没勾选时，按钮理由说的是「还没选」", () => {
  const none = describeMemberBatchActions(collectMemberBatchTargets([], index));
  // 真机联调：没勾选时原来会说"没有可禁用的成员：只有 addons 根目录里的 Mod 能搬进 disabled"，
  // 答非所问。六个按钮的理由应与组级批量条同形（先勾选）。
  assert.deepEqual(
    none.map((item) => item.id),
    ["game-on", "game-off", "enable", "disable", "transfer", "remove"],
    "返回顺序要与界面上的按钮顺序一致",
  );
  for (const item of none) {
    assert.equal(item.disabled, true, `${item.id} 没勾选时应不可点`);
    assert.match(item.title, /先勾选/, `${item.id} 的理由应说明"还没勾选"`);
  }
});

test("勾了成员但某个动作不适用时，说明具体原因", () => {
  const onlyRoot = describeMemberBatchActions(collectMemberBatchTargets(["root_a.vpk"], index));
  const rootById = Object.fromEntries(onlyRoot.map((item) => [item.id, item]));
  assert.equal(rootById.disable.disabled, false, "根目录成员可以禁用");
  assert.equal(rootById.enable.disabled, true);
  assert.match(rootById.enable.title, /disabled 目录/);
  assert.match(rootById.transfer.title, /创意工坊/);

  const onlyDisabled = describeMemberBatchActions(
    collectMemberBatchTargets(["off.vpk"], index),
  );
  const offById = Object.fromEntries(onlyDisabled.map((item) => [item.id, item]));
  assert.equal(offById.enable.disabled, false);
  assert.equal(offById["game-on"].disabled, true);
  assert.match(offById["game-on"].title, /disabled 目录/);
});

test("移出计划按组拆分，并标出「会把整组清空」的情况", () => {
  const groups = [
    { id: "g1", name: "角色包", members: [{ key: "a.vpk" }, { key: "b.vpk" }] },
    { id: "g2", name: "武器包", members: [{ key: "c.vpk" }] },
  ];
  const plan = planMemberRemoval(["A.VPK", "c.vpk"], groups);
  assert.equal(plan.length, 2);
  assert.deepEqual(plan[0], {
    groupId: "g1",
    groupName: "角色包",
    keys: ["a.vpk"],
    memberCount: 2,
    wouldEmpty: false,
  });
  assert.deepEqual(plan[1], {
    groupId: "g2",
    groupName: "武器包",
    keys: ["c.vpk"],
    memberCount: 1,
    wouldEmpty: true,
  });
});

test("选择标签与结果文案", () => {
  assert.equal(formatMemberBatchSelectionLabel(0), "先在成员行左侧勾选 Mod");
  assert.equal(formatMemberBatchSelectionLabel(3), "已选 3 个成员");
  // 有隐藏勾选时必须说出来：否则"全选成员"之后计数比眼前的行数多，用户不知道多在哪。
  assert.equal(
    formatMemberBatchSelectionLabel(15, 6),
    "已选 15 个成员（其中 6 个在未展开的组里）",
  );
  assert.equal(formatMemberBatchSelectionLabel(15, 0), "已选 15 个成员");
  assert.equal(
    formatMemberBatchResult({ label: "禁用", succeeded: 2, failed: 1, skipped: 4 }),
    "已禁用 2 个成员；4 个不适用于这个动作，已跳过；1 个失败",
  );
  assert.equal(formatMemberBatchResult({ label: "启用", succeeded: 1 }), "已启用 1 个成员");
  // 只报"1 个失败"没法排查，第一个原因要跟着一起说。
  assert.equal(
    formatMemberBatchResult({
      label: "禁用",
      succeeded: 2,
      failed: 1,
      firstError: "文件正被其它程序占用，关闭占用的程序后重试",
    }),
    "已禁用 2 个成员；1 个失败（文件正被其它程序占用，关闭占用的程序后重试）",
  );
  assert.equal(
    formatMemberBatchResult({ label: "禁用", succeeded: 2, failed: 1, firstError: "   " }),
    "已禁用 2 个成员；1 个失败",
  );
});

// ── 「范围勾选」：成员勾选是跨组共用一个集合的，范围必须显式 ────────────────
// 真机复现过的缺陷：在 A 组选过成员后，展开 B 组点「全选成员」，
// 集合变成 A∪B；把 A 折叠起来，A 的 6 个勾选仍然生效、界面上却看不见，
// 批量按钮的 title 已经写着「把 15 个根目录成员搬进 disabled」。

const GROUPS = [
  { id: "g1", name: "A 组", members: ["a1.vpk", "a2.vpk", "a3.vpk"] },
  { id: "g2", name: "B 组", members: ["b1.vpk", { key: "b2.vpk", name: "B2" }, "shared.vpk"] },
  { id: "g3", name: "C 组", members: ["shared.vpk"] },
];

test("memberKeysOfGroup 兼容字符串与对象成员，并去空去重", () => {
  assert.deepEqual(memberKeysOfGroup(GROUPS[0]), ["a1.vpk", "a2.vpk", "a3.vpk"]);
  assert.deepEqual(memberKeysOfGroup(GROUPS[1]), ["b1.vpk", "b2.vpk", "shared.vpk"]);
  assert.deepEqual(memberKeysOfGroup({ members: ["x.vpk", "", "x.vpk", null] }), ["x.vpk"]);
  assert.deepEqual(memberKeysOfGroup(null), []);
});

test("collectAllMemberKeys 跨组去重", () => {
  const all = collectAllMemberKeys(GROUPS);
  assert.deepEqual(all, ["a1.vpk", "a2.vpk", "a3.vpk", "b1.vpk", "b2.vpk", "shared.vpk"]);
  assert.equal(all.length, 6, "shared.vpk 同时属于 B/C 两组，只能算一次");
});

test("applyMemberScopeToggle 只动范围内的键", () => {
  // 先选了 B 组全部
  let selection = new Set(["b1.vpk", "b2.vpk", "shared.vpk"]);
  // 再对 A 组「本组全选」：范围外的 B 组原样保留
  selection = applyMemberScopeToggle(selection, memberKeysOfGroup(GROUPS[0]), true);
  assert.deepEqual([...selection].sort(), ["a1.vpk", "a2.vpk", "a3.vpk", "b1.vpk", "b2.vpk", "shared.vpk"]);
  // 取消 A 组：只掉 A 组的三个
  selection = applyMemberScopeToggle(selection, memberKeysOfGroup(GROUPS[0]), false);
  assert.deepEqual([...selection].sort(), ["b1.vpk", "b2.vpk", "shared.vpk"]);
  // 传入不存在的键不会报错
  assert.deepEqual([...applyMemberScopeToggle(selection, ["nope.vpk"], true)].sort(), [
    "b1.vpk",
    "b2.vpk",
    "nope.vpk",
    "shared.vpk",
  ]);
});

test("countMemberKeysOutsideScope 数出「看不见的勾选」", () => {
  const selection = new Set(["a1.vpk", "a2.vpk", "b1.vpk"]);
  // 只展开了 B 组：A 组的两个是看不见的
  assert.equal(countMemberKeysOutsideScope(selection, ["b1.vpk", "shared.vpk"]), 2);
  // 都展开时没有隐藏项
  assert.equal(countMemberKeysOutsideScope(selection, ["a1.vpk", "a2.vpk", "b1.vpk"]), 0);
  assert.equal(countMemberKeysOutsideScope(new Set(), ["a1.vpk"]), 0);
});
