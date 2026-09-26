import assert from "node:assert/strict";
import test from "node:test";

import {
  buildGroupFilterOptions,
  buildGroupIndex,
  buildSuggestionFileIndex,
  defaultSuggestionSelection,
  describeSuggestionMember,
  fileMatchesGroupFilter,
  formatGroupChip,
  formatGroupChipTitle,
  formatGroupFilterLabel,
  formatGroupMissingNotice,
  formatGroupSubtreeMissingNotice,
  formatGroupOptionIndent,
  formatGroupOptionLabel,
  formatGroupStrategy,
  formatFileGroupImpact,
  formatSuggestionCreateSummary,
  formatSuggestionSummary,
  groupsForFile,
  normalizeGroupKey,
  sortGroupFilterOptions,
  suggestionMemberRows,
} from "./group-view.mjs";

const memberships = [
  { key: "a.vpk", groupId: "g1", groupName: "打包组", strategy: "single", memberCount: 3, tier: -2, enforce: true },
  { key: "b.vpk", groupId: "g1", groupName: "打包组", strategy: "single", memberCount: 3, tier: -2, enforce: true },
  { key: "workshop\\123.vpk", groupId: "g1", groupName: "打包组", strategy: "single", memberCount: 3, tier: -2, enforce: true },
  { key: "a.vpk", groupId: "g2", groupName: "皮肤组", strategy: "all", memberCount: 2 },
];

test("buildSuggestionFileIndex 按 addonlist 键索引文件", () => {
  const files = [
    { name: "a.vpk", path: "D:/addons/a.vpk", location: "root" },
    { name: "123.vpk", path: "D:/addons/workshop/123.vpk", location: "workshop" },
  ];
  const index = buildSuggestionFileIndex(files, "D:/addons");
  assert.equal(index.get("a.vpk")?.name, "a.vpk");
  assert.equal(index.get("workshop\\123.vpk")?.name, "123.vpk");
  // filePriorityKeys 同时给出"带目录的键"和"裸文件名"，两种键都能命中同一个文件。
  assert.equal(index.get("123.vpk")?.name, "123.vpk");
  assert.equal(index.get("nope.vpk"), undefined);
});

test("describeSuggestionMember 按位置给出可用的开关操作", () => {
  const root = describeSuggestionMember(
    { location: "root", gameStateKnown: true, gameEnabled: true },
    { priorityLabel: "优先级 #12（分层 3）" },
  );
  assert.equal(root.location, "根目录");
  assert.equal(root.gameState, "游戏开关：开");
  assert.equal(root.fileAction, "禁用");
  assert.equal(root.fileActionKind, "disable");
  assert.equal(root.canToggleFile, true);
  assert.equal(root.canEditGameState, true);
  assert.equal(root.priority, "优先级 #12（分层 3）");

  const workshop = describeSuggestionMember({ location: "workshop", gameStateKnown: false });
  assert.equal(workshop.location, "创意工坊");
  assert.equal(workshop.gameState, "游戏开关：未记录");
  assert.equal(workshop.fileAction, "复制到 addons");
  assert.equal(workshop.fileActionKind, "transfer");
  assert.equal(workshop.canToggleFile, false);
  assert.equal(workshop.canEditGameState, true);

  const disabled = describeSuggestionMember({ location: "disabled", gameStateKnown: true, gameEnabled: false });
  assert.equal(disabled.location, "已禁用");
  assert.equal(disabled.fileAction, "启用");
  assert.equal(disabled.canToggleFile, true);
  assert.equal(disabled.canEditGameState, false);
  assert.equal(disabled.priority, "未写入 addonlist");

  const missing = describeSuggestionMember(null);
  assert.equal(missing.found, false);
  assert.equal(missing.location, "未知位置");
});
test("缺失成员：菜单标签与提示文案", () => {
  const options = buildGroupFilterOptions([
    { key: "a.vpk", groupId: "g1", groupName: "打包组", strategy: "all", memberCount: 3 },
    { key: "b.vpk", groupId: "g1", groupName: "打包组", strategy: "all", memberCount: 3, missing: true },
    { key: "c.vpk", groupId: "g1", groupName: "打包组", strategy: "all", memberCount: 3, missing: true },
  ]);
  assert.equal(options.length, 1);
  assert.equal(options[0].missingCount, 2);
  assert.equal(formatGroupOptionLabel(options[0]), "打包组（3） · 含 2 个缺失");
  assert.equal(formatGroupOptionLabel({ name: "完整组", memberCount: 2 }), "完整组（2）");

  const notice = formatGroupMissingNotice(options[0].missingNames);
  assert.ok(notice.includes("文件缺失 2 个"));
  assert.ok(notice.includes("放回同名文件"));
  assert.equal(formatGroupMissingNotice([]), "");
});

test("删除前的策略组影响提示", () => {
  const message = formatFileGroupImpact(["打包组", "语音组"]);
  assert.ok(message.includes("2 个策略组"));
  assert.ok(message.includes("打包组、语音组"));
  assert.ok(message.includes("缺失成员"));
  assert.equal(formatFileGroupImpact([]), "");
});
test("buildGroupIndex 按归一化键建索引，支持一个 Mod 属于多个组", () => {
  const index = buildGroupIndex(memberships);
  assert.equal(index.size, 3);
  assert.equal(index.get("a.vpk").length, 2);
  assert.equal(index.get("workshop\\123.vpk").length, 1);
  // 索引的键是归一化后的；调用方（如 groupsForFile）会先归一化再查。
  assert.equal(index.get(normalizeGroupKey("WORKSHOP/123.VPK")).length, 1);
});

test("groupsForFile 用文件路径推导 addonlist 键并返回所属组", () => {
  const index = buildGroupIndex(memberships);
  const rootFile = { name: "a.vpk", path: "C:\\addons\\a.vpk", location: "root" };
  const groups = groupsForFile(rootFile, index, "C:\\addons");
  assert.deepEqual(groups.map((item) => item.groupId), ["g1", "g2"]);

  const workshopFile = { name: "123.vpk", path: "C:\\addons\\workshop\\123.vpk", location: "workshop" };
  assert.deepEqual(
    groupsForFile(workshopFile, index, "C:\\addons").map((item) => item.groupId),
    ["g1"],
  );
  assert.deepEqual(groupsForFile({ name: "zzz.vpk", location: "root" }, index, "C:\\addons"), []);
});

test("组徽标与悬浮说明包含关键信息", () => {
  assert.equal(formatGroupChip({ groupName: "打包组" }), "组：打包组");
  const title = formatGroupChipTitle(memberships[0]);
  assert.ok(title.includes("打包组") && title.includes("3 个成员"));
  assert.ok(title.includes("互斥单选") && title.includes("组权重 -2") && title.includes("自动联动"));
  assert.ok(title.includes("单击整组开关"), "徽标提示要说明单击行为");
  assert.ok(title.includes("分组"), "徽标提示要指向整组优先级移动所在位置");
  assert.equal(formatGroupStrategy("single_random"), "随机单选");
  assert.equal(formatGroupStrategy("unknown"), "unknown");
});

test("buildGroupFilterOptions 去重并按组聚合成员键", () => {
  const options = buildGroupFilterOptions(memberships);
  assert.deepEqual(options.map((item) => item.id), ["g1", "g2"]);
  assert.equal(options[0].keys.size, 3);
  assert.equal(options[0].memberCount, 3);
});

// —— 「按分组筛选」的顺序：由组权重（组内那格"权重"）决定，未设置权重的排在最后 ——

const hierarchyMemberships = [
  { key: "a.vpk", groupId: "root-b", groupName: "乙组", strategy: "all", memberCount: 1 },
  { key: "b.vpk", groupId: "child-b1", groupName: "乙-子组", strategy: "all", memberCount: 1, parentId: "root-b", tier: 5 },
  { key: "c.vpk", groupId: "root-a", groupName: "甲组", strategy: "all", memberCount: 1, tier: 9 },
  { key: "d.vpk", groupId: "child-a1", groupName: "甲-子组", strategy: "all", memberCount: 1, parentId: "root-a" },
  { key: "e.vpk", groupId: "root-c", groupName: "丙组", strategy: "all", memberCount: 1, tier: 1 },
];

test("buildGroupFilterOptions 透出层级与组权重（筛选菜单排序要用）", () => {
  const options = buildGroupFilterOptions(hierarchyMemberships);
  const byId = new Map(options.map((item) => [item.id, item]));
  assert.equal(byId.get("child-b1").parentId, "root-b");
  assert.equal(byId.get("child-b1").parentName, "乙组");
  assert.equal(byId.get("child-b1").tier, 5);
  assert.equal(byId.get("child-b1").depth, 2);
  assert.equal(byId.get("root-b").tier, null, "未设置权重时给 null，便于排序时排在最后");
  assert.equal(byId.get("root-a").depth, 1);
});

test("sortGroupFilterOptions 按组权重升序 + 子树整体移动（子组不跟父组脱开）", () => {
  const options = buildGroupFilterOptions(hierarchyMemberships);
  const sorted = sortGroupFilterOptions(options).map((item) => item.id);
  // 子组 5 的权重把「乙组」整棵子树抬起来（整棵树的最小权重=5），排在「甲组(9)」前面；
  // 未设置权重的「甲-子组」跟着自己的父组「甲组」走。
  assert.deepEqual(sorted, ["root-c", "root-b", "child-b1", "root-a", "child-a1"]);
  assert.equal(
    sorted.indexOf("root-b") + 1,
    sorted.indexOf("child-b1"),
    "子组紧跟父组（父组在前，子组紧随其后），不能各排各的",
  );
  // 权重更小的组（-1）排最前，权重优先于名称。
  assert.equal(sortGroupFilterOptions(buildGroupFilterOptions([
    { key: "x.vpk", groupId: "g", groupName: "G", strategy: "all", memberCount: 1, tier: -1 },
    ...hierarchyMemberships,
  ]))[0].id, "g");
});

test("子树整体移动：名字前缀不同的子组不会再被甩到列表另一头", () => {
  // 真实数据形状：父组 `!!医疗箱`，子组里既有 `!花火` 也有 `【BA桶】`；
  // 按纯名称排序时 `【` 会沉到最后，子组就和父组脱开了。
  const memberships = [
    { key: "a.vpk", groupId: "box", groupName: "!!医疗箱", strategy: "all", memberCount: 6 },
    { key: "b.vpk", groupId: "fire", groupName: "!花火零食", strategy: "all", memberCount: 9, parentId: "box" },
    { key: "c.vpk", groupId: "bin", groupName: "【BA垃圾桶】", strategy: "all", memberCount: 5, parentId: "box" },
    { key: "d.vpk", groupId: "other", groupName: "Chiffon 下午茶", strategy: "all", memberCount: 13 },
  ];
  const sorted = sortGroupFilterOptions(buildGroupFilterOptions(memberships)).map((item) => item.id);
  assert.equal(sorted[0], "box", "父组在最前");
  assert.deepEqual(
    [...sorted.slice(1, 3)].sort(),
    ["bin", "fire"],
    "两个子组都要紧跟父组（名字前缀不同也不再被拆开）",
  );
  assert.equal(sorted[3], "other", "其它顶层组排在整棵子树之后");
  const rows = buildGroupFilterOptions(memberships);
  const byId = new Map(rows.map((item) => [item.id, item]));
  assert.equal(byId.get("bin").depth, 2, "子组仍要缩进显示");
});

test("sortGroupFilterOptions 对成环的层级不递归（按顶层降级处理）", () => {
  const options = [
    { id: "x", name: "X", parentId: "y", depth: 1, tier: null },
    { id: "y", name: "Y", parentId: "x", depth: 1, tier: null },
  ];
  const sorted = sortGroupFilterOptions(options).map((item) => item.id);
  assert.equal(sorted.length, 2, "成环时不能丢组，也不能无限递归");
  assert.deepEqual([...sorted].sort(), ["x", "y"]);
});

test("formatGroupOptionIndent / formatGroupOptionLabel 体现层级与权重", () => {
  assert.equal(formatGroupOptionIndent({ depth: 1 }), "");
  assert.equal(formatGroupOptionIndent({ depth: 2 }), "└ ");
  assert.equal(formatGroupOptionIndent({ depth: 3 }), "　└ ");
  assert.equal(formatGroupOptionLabel({ name: "甲组", memberCount: 3, tier: 9 }), "甲组（3） · 权重 9");
  assert.equal(formatGroupOptionLabel({ name: "甲组", memberCount: 3, tier: null }), "甲组（3）");
});

test("fileMatchesGroupFilter 按勾选的分组过滤", () => {
  const options = buildGroupFilterOptions(memberships);
  const fileA = { name: "a.vpk", path: "C:\\addons\\a.vpk", location: "root" };
  const fileC = { name: "c.vpk", path: "C:\\addons\\c.vpk", location: "root" };

  assert.equal(fileMatchesGroupFilter(fileC, options, new Set(), "C:\\addons"), true);
  assert.equal(fileMatchesGroupFilter(fileA, options, new Set(["g1"]), "C:\\addons"), true);
  assert.equal(fileMatchesGroupFilter(fileC, options, new Set(["g1"]), "C:\\addons"), false);
  assert.equal(fileMatchesGroupFilter(fileC, options, new Set(["g1", "g2"]), "C:\\addons"), false);
  assert.equal(formatGroupFilterLabel(new Set(), options), "全部分组");
  assert.equal(formatGroupFilterLabel(new Set(["g1"]), options), "打包组");
  assert.equal(formatGroupFilterLabel(new Set(["g1", "g2"]), options), "2 个分组");
});

test("建议文案：摘要、置信度、成员勾选与创建提示", () => {
  const suggestion = {
    id: "s1",
    label: "hit marker",
    reason: "主体相同：HUD",
    confidence: "medium",
    memberKeys: ["a.vpk", "b.vpk"],
    memberNames: ["Hit Marker Overhaul", "Hit Marker Overhaul Image Support"],
    signals: ["主体识别", "文件名前缀"],
  };
  const summary = formatSuggestionSummary(suggestion);
  assert.ok(summary.includes("2 个 Mod") && summary.includes("中置信度") && summary.includes("主体相同"));

  const selection = defaultSuggestionSelection(suggestion);
  assert.equal(selection.size, 2);
  const rows = suggestionMemberRows(suggestion, selection);
  assert.deepEqual(rows.map((row) => row.name), ["Hit Marker Overhaul", "Hit Marker Overhaul Image Support"]);
  assert.ok(rows.every((row) => row.selected));

  assert.equal(formatSuggestionCreateSummary(suggestion, new Set()), "请至少勾选一个 Mod");
  assert.equal(formatSuggestionCreateSummary(suggestion, selection), "将创建包含全部 2 个 Mod 的策略组");
  assert.equal(
    formatSuggestionCreateSummary(suggestion, new Set(["a.vpk"])),
    "将创建包含 1 / 2 个 Mod 的策略组",
  );
});

// 对齐 FireAxe 的 AddonChildrenProblem：父组自己的成员不缺，但子组有缺失时，
// 父组行也要给出汇总提示，否则用户会以为整棵树都健康。
test("formatGroupSubtreeMissingNotice 汇总子组缺失", () => {
  assert.equal(formatGroupSubtreeMissingNotice(0, 0), "");
  assert.equal(formatGroupSubtreeMissingNotice(undefined, undefined), "");
  assert.equal(
    formatGroupSubtreeMissingNotice(3, 2),
    "子组里有 3 个缺失文件（涉及 2 个子组）：展开子组即可看到",
  );
  // 只有数量、没有子组数时也能给出可读文案。
  assert.equal(formatGroupSubtreeMissingNotice(1, 0), "子组里有 1 个缺失文件：展开子组即可看到");
});
