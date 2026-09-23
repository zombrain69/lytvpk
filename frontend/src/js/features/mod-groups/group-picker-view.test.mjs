import assert from "node:assert/strict";
import test from "node:test";

import {
  GROUP_PICKER_MODES,
  buildGroupPickerRows,
  filterGroupPickerRows,
  formatGroupPickerVisibleSummary,
  formatGroupPickerResult,
  formatGroupPickerSummary,
  groupPickerConfirmLabel,
  groupPickerRowDetail,
  groupPickerRowSearchText,
  groupPickerRowTags,
  nextSelectableRowId,
  pickerRowDisabledReason,
  sortGroupPickerRows,
} from "./group-picker-view.mjs";

const groups = [
  {
    id: "g1",
    name: "猎枪 武器",
    strategy: "single",
    members: [{ key: "a.vpk" }, { key: "b.vpk" }],
  },
  {
    id: "g2",
    name: "Chiffon 下午茶",
    strategy: "all",
    members: [{ key: "a.vpk" }, { key: "c.vpk" }, { key: "d.vpk" }],
  },
  {
    id: "g3",
    name: "空壳组",
    strategy: "off",
    members: [{ key: "z.vpk" }],
  },
];

test("add 模式：已包含全部选中项的分组被禁用", () => {
  const rows = buildGroupPickerRows({
    groups,
    keys: ["a.vpk", "b.vpk"],
    mode: GROUP_PICKER_MODES.add,
  });
  assert.equal(rows.length, 3);
  assert.equal(rows[0].selectedInGroup, 2);
  assert.equal(rows[0].disabled, true);
  assert.match(rows[0].disabledReason, /都已经在这个组里/);
  assert.equal(rows[1].selectedInGroup, 1);
  assert.equal(rows[1].disabled, false);
  assert.equal(rows[2].selectedInGroup, 0);
  assert.equal(rows[2].disabled, false);
});

test("remove 模式：不包含任何选中项的分组被禁用，移空整组也会被拦下", () => {
  const noneSelected = buildGroupPickerRows({
    groups,
    keys: ["z.vpk"],
    mode: GROUP_PICKER_MODES.remove,
  });
  assert.equal(noneSelected[0].disabled, true);
  assert.match(noneSelected[0].disabledReason, /都不在这个组里/);

  // g3 只有一个成员，选中它就是"把整组移空"。
  assert.equal(noneSelected[2].selectedInGroup, 1);
  assert.equal(noneSelected[2].wouldEmptyGroup, true);
  assert.equal(noneSelected[2].disabled, true);
  assert.match(noneSelected[2].disabledReason, /全部移出/);

  // 组里还有别的成员时，移出选中项是允许的。
  const partial = buildGroupPickerRows({
    groups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.remove,
  });
  assert.equal(partial[0].selectedInGroup, 1);
  assert.equal(partial[0].wouldEmptyGroup, false);
  assert.equal(partial[0].disabled, false);
});

test("move 模式：源组自己不能当目标组", () => {
  const rows = buildGroupPickerRows({
    groups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.move,
    sourceGroupId: "g2",
  });
  const sourceRow = rows.find((row) => row.id === "g2");
  assert.equal(sourceRow.disabled, true);
  assert.match(sourceRow.disabledReason, /就是当前所在的组/);
  const otherRow = rows.find((row) => row.id === "g1");
  assert.equal(otherRow.disabled, false);
  // 移动的"移出 + 加入"两个动作都报告出来，方便界面提示。
  assert.equal(otherRow.selectedInGroup, 1);
  assert.equal(otherRow.missingSelected, 0);
});

test("pickerRowDisabledReason 在可点时返回空字符串", () => {
  const rows = buildGroupPickerRows({
    groups,
    // d.vpk 只在 g2 里：对 g1 来说是一次真正的"加入"。
    keys: ["d.vpk"],
    mode: GROUP_PICKER_MODES.add,
  });
  assert.equal(rows[0].disabled, false);
  assert.equal(pickerRowDisabledReason(rows[0], GROUP_PICKER_MODES.add), "");
  // 选中项已经在 g2 里 → 重复加入，应该被禁用。
  assert.equal(rows[1].disabled, true);
});

test("formatGroupPickerSummary 说明选中数量与可用分组数", () => {
  const rows = buildGroupPickerRows({
    groups,
    keys: ["a.vpk", "b.vpk"],
    mode: GROUP_PICKER_MODES.add,
  });
  assert.equal(
    formatGroupPickerSummary({ rows, keyCount: 2, mode: GROUP_PICKER_MODES.add }),
    "已选中 2 个 Mod；下面 3 个策略组里 2 个可以加入",
  );
  assert.equal(
    formatGroupPickerSummary({ rows, keyCount: 1, mode: GROUP_PICKER_MODES.remove }),
    "已选中 1 个 Mod；下面 3 个策略组里 2 个可以移出",
  );
  assert.equal(
    formatGroupPickerSummary({ rows, keyCount: 1, mode: GROUP_PICKER_MODES.move, sourceName: "猎枪 武器" }),
    "已选中 1 个 Mod；从「猎枪 武器」移动到下面 2 个策略组之一",
  );
});

test("formatGroupPickerResult 按模式生成提示文案", () => {
  assert.equal(
    formatGroupPickerResult(
      { name: "猎枪 武器", members: [{ key: "a.vpk" }, { key: "b.vpk" }] },
      { mode: GROUP_PICKER_MODES.add, keyCount: 2 },
    ),
    "已把 2 个 Mod 加入「猎枪 武器」（现在共 2 个成员）",
  );
  assert.equal(
    formatGroupPickerResult(
      { name: "猎枪 武器", members: [{ key: "a.vpk" }] },
      { mode: GROUP_PICKER_MODES.remove, keyCount: 1 },
    ),
    "已把 1 个 Mod 移出「猎枪 武器」（现在共 1 个成员）",
  );
  assert.equal(
    formatGroupPickerResult(
      {
        sourceName: "猎枪 武器",
        targetName: "Chiffon 下午茶",
        moved: ["a.vpk", "b.vpk"],
        alreadyInTarget: ["c.vpk"],
        targetTotal: 5,
      },
      { mode: GROUP_PICKER_MODES.move },
    ),
    "已把 2 个 Mod 从「猎枪 武器」移到「Chiffon 下午茶」（目标组现在共 5 个成员，另有 1 个本来就在目标组里）",
  );
  assert.equal(
    formatGroupPickerResult(
      { sourceName: "A", targetName: "B", moved: [], alreadyInTarget: ["a.vpk"], targetTotal: 3 },
      { mode: GROUP_PICKER_MODES.move },
    ),
    "这些 Mod 本来就在「B」里，没有需要移动的成员",
  );
});

test("groupPickerConfirmLabel 给按钮文案", () => {
  assert.equal(groupPickerConfirmLabel(GROUP_PICKER_MODES.add), "加入这个组");
  assert.equal(groupPickerConfirmLabel(GROUP_PICKER_MODES.remove), "移出这个组");
  assert.equal(groupPickerConfirmLabel(GROUP_PICKER_MODES.move), "移动到这个组");
});

// —— 便捷性：行详情 / 搜索 / 排序 / 键盘导航 ——

const detailGroups = [
  {
    id: "g1",
    name: "猎枪 武器",
    strategy: "single",
    tier: 3,
    enforce: true,
    parentId: "p1",
    members: [{ key: "a.vpk" }],
  },
  { id: "p1", name: "武器套装", strategy: "all", members: [{ key: "x.vpk" }] },
  { id: "g3", name: "空的组", strategy: "off", members: [] },
];

test("行详情带上策略 / 权重 / 自动联动 / 上级分组，便于一眼分辨", () => {
  const rows = buildGroupPickerRows({
    groups: detailGroups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.add,
  });
  const row = rows.find((item) => item.id === "g1");
  assert.equal(row.parentName, "武器套装");
  assert.equal(row.tier, 3);
  assert.equal(row.enforce, true);
  assert.deepEqual(groupPickerRowTags(row), ["互斥单选", "权重 3", "自动联动", "上级：武器套装"]);
  assert.match(groupPickerRowDetail(row), /1 个成员/);
  // 全覆盖的情况只由 disabledReason 说明，不要在详情里重复一遍。
  assert.equal(/选中项已全部在组内/.test(groupPickerRowDetail(row)), false);
  assert.match(groupPickerRowDetail(row), /都已经在这个组里/);
  const removeRow = buildGroupPickerRows({
    groups: detailGroups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.remove,
  }).find((item) => item.id === "g1");
  assert.match(groupPickerRowDetail(removeRow), /会把该组成员全部移出/);
  // 部分命中时给出"选中里已有 N 个"，这是 disabledReason 覆盖不到的信息。
  const partialRow = buildGroupPickerRows({
    groups: detailGroups,
    keys: ["a.vpk", "zzz.vpk"],
    mode: GROUP_PICKER_MODES.add,
  }).find((item) => item.id === "g1");
  assert.match(groupPickerRowDetail(partialRow), /选中里已有 1 个/);
  assert.match(groupPickerRowSearchText(row), /猎枪 武器/);
  assert.match(groupPickerRowSearchText(row), /武器套装/, "搜索要能按上级分组名命中");
  assert.match(groupPickerRowSearchText(row), /互斥单选/, "搜索要能按策略名命中");
});

test("filterGroupPickerRows 支持搜索与「只看相关」", () => {
  const rows = buildGroupPickerRows({
    groups: detailGroups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.remove,
  });
  assert.equal(filterGroupPickerRows(rows, { query: "猎枪" }).length, 1);
  assert.equal(filterGroupPickerRows(rows, { query: "不存在的组" }).length, 0);
  // 只看相关：只有 g1 含选中项；g1 会被移空所以不可点，但仍应显示（并说明原因）。
  const relevant = filterGroupPickerRows(rows, { relevantOnly: true });
  assert.deepEqual(relevant.map((row) => row.id), ["g1"]);
  assert.equal(relevant[0].disabled, true);
  assert.match(relevant[0].disabledReason, /全部移出/);
});

test("sortGroupPickerRows 让可点、含选中项的组排在前面", () => {
  const rows = buildGroupPickerRows({
    groups: detailGroups,
    keys: ["a.vpk"],
    mode: GROUP_PICKER_MODES.add,
  });
  const sorted = sortGroupPickerRows(rows);
  // g1 里的选中项已经全在组里（不可点）→ 应该沉到"可点组"之后。
  assert.equal(sorted[sorted.length - 1].id, "g1");
  assert.ok(sorted.slice(0, 2).every((row) => !row.disabled));
});

test("formatGroupPickerVisibleSummary 说明显示数量与被隐藏数量", () => {
  assert.equal(
    formatGroupPickerVisibleSummary({ total: 12, shown: 3, usable: 2, hiddenByFilter: 9 }),
    "显示 3 / 12 个组 · 2 个可操作 · 已按筛选隐藏 9 个",
  );
  assert.equal(
    formatGroupPickerVisibleSummary({ total: 3, shown: 3, usable: 1 }),
    "显示 3 / 3 个组 · 1 个可操作",
  );
});

test("nextSelectableRowId 只在可点行之间循环，空高亮时 ↓ 到首个、↑ 到末尾", () => {
  const rows = [
    { id: "a", disabled: false },
    { id: "b", disabled: true },
    { id: "c", disabled: false },
  ];
  assert.equal(nextSelectableRowId(rows, "", 1), "a");
  assert.equal(nextSelectableRowId(rows, "", -1), "c");
  assert.equal(nextSelectableRowId(rows, "a", 1), "c", "跳过不可点的 b");
  assert.equal(nextSelectableRowId(rows, "a", -1), "c", "循环回最后一个可点行");
  assert.equal(nextSelectableRowId([{ id: "x", disabled: true }], "", 1), "", "没有可点行时返回空");
});
