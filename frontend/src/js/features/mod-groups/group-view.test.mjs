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
  formatGroupOptionLabel,
  formatGroupStrategy,
  formatFileGroupImpact,
  formatSuggestionCreateSummary,
  formatSuggestionSummary,
  groupsForFile,
  normalizeGroupKey,
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
