import assert from "node:assert/strict";
import test from "node:test";

import {
  COMMANDS,
  describeCommandPaletteState,
  nextCommandId,
  orderCommandsByRecents,
  pushRecentCommand,
  searchCommands,
  subsequenceMatch,
} from "./command-palette.mjs";

test("命令清单覆盖各页面与常用动作，且 id/title 不重复", () => {
  const ids = COMMANDS.map((command) => command.id);
  assert.equal(new Set(ids).size, ids.length, "命令 id 不能重复");
  const titles = COMMANDS.map((command) => command.title);
  assert.equal(new Set(titles).size, titles.length, "命令标题不能重复");

  ["page-mods", "page-workshop", "page-downloads", "page-servers", "page-diagnostics", "page-settings", "page-about"].forEach(
    (id) => assert.ok(ids.includes(id), `缺少页面命令 ${id}`),
  );
  ["focus-mod-search", "strategy-group-manager", "group-suggest", "load-order", "health-check", "toggle-theme"].forEach(
    (id) => assert.ok(ids.includes(id), `缺少动作命令 ${id}`),
  );
  assert.ok(COMMANDS.every((command) => command.hint), "每条命令都要有去处提示");
});

test("空查询保持原顺序并截断", () => {
  const all = searchCommands(COMMANDS, "");
  assert.deepEqual(
    all.map((command) => command.id),
    COMMANDS.slice(0, all.length).map((command) => command.id),
  );
  assert.equal(searchCommands(COMMANDS, "", 3).length, 3);
});

test("检索：标题优先、关键字兜底、无匹配返回空", () => {
  assert.equal(searchCommands(COMMANDS, "冲突")[0].id, "conflict-analysis");
  assert.equal(searchCommands(COMMANDS, "体检")[0].id, "health-check");
  assert.equal(searchCommands(COMMANDS, "主题")[0].id, "toggle-theme");
  // 关键字命中（标题里没有"字号"这两个字，但关键字里有）。
  assert.equal(searchCommands(COMMANDS, "字号")[0].id, "interface-settings");
  // 英文关键字也能命中。
  assert.equal(searchCommands(COMMANDS, "mdmp")[0].id, "page-diagnostics");
  assert.deepEqual(searchCommands(COMMANDS, "不存在的命令zzz"), []);
});

test("subsequenceMatch 与 Mod 搜索同语义", () => {
  assert.equal(subsequenceMatch("策略组管理", "策管"), true);
  assert.equal(subsequenceMatch("策略组管理", "管策"), false);
  assert.equal(subsequenceMatch("", "a"), false);
});

test("状态文案与光标移动", () => {
  assert.match(describeCommandPaletteState({ total: 16, query: "" }), /共 16 条命令/);
  // 空查询时顺带给出全局快捷键，省得用户到处找。
  assert.match(describeCommandPaletteState({ total: 16, query: "" }), /Ctrl\+K 命令面板/);
  assert.match(describeCommandPaletteState({ total: 16, shown: 3, query: "组" }), /匹配 3 \/ 16 条命令/);
  assert.match(describeCommandPaletteState({ total: 16, shown: 0, query: "zzz" }), /没有匹配「zzz」的命令/);

  const ids = ["a", "b", "c"];
  assert.equal(nextCommandId(ids, "a", 1), "b");
  assert.equal(nextCommandId(ids, "c", 1), "a");
  assert.equal(nextCommandId(ids, "a", -1), "c");
  assert.equal(nextCommandId(ids, "", 1), "a");
  assert.equal(nextCommandId(ids, "", -1), "c");
  assert.equal(nextCommandId([], "a", 1), "");
});

test("最近使用：去重、最近在前、超上限截断", () => {
  assert.deepEqual(pushRecentCommand([], "page-mods"), ["page-mods"]);
  assert.deepEqual(pushRecentCommand(["a", "b"], "b"), ["b", "a"]);
  assert.deepEqual(pushRecentCommand(["a", "b"], "c"), ["c", "a", "b"]);
  assert.deepEqual(pushRecentCommand(["a", "b", "c", "d", "e"], "f"), ["f", "a", "b", "c", "d"]);
  // 空 id / 坏数据不该破坏记录
  assert.deepEqual(pushRecentCommand(["a"], ""), ["a"]);
  assert.deepEqual(pushRecentCommand(null, "a"), ["a"]);
  assert.deepEqual(pushRecentCommand(["a", "a", "", "b"], "a"), ["a", "b"]);
});

test("最近使用：空查询时排最上面，其余保持命令表原顺序", () => {
  const commands = [{ id: "a" }, { id: "b" }, { id: "c" }];
  assert.deepEqual(orderCommandsByRecents(commands, []).map((item) => item.id), ["a", "b", "c"]);
  assert.deepEqual(orderCommandsByRecents(commands, ["c"]).map((item) => item.id), ["c", "a", "b"]);
  assert.deepEqual(orderCommandsByRecents(commands, ["c", "a"]).map((item) => item.id), ["c", "a", "b"]);
  // 记忆里已经不存在的 id 直接忽略
  assert.deepEqual(orderCommandsByRecents(commands, ["zzz"]).map((item) => item.id), ["a", "b", "c"]);
  assert.deepEqual(orderCommandsByRecents(null, ["a"]), []);
  // 不能打乱原数组
  assert.deepEqual(commands.map((item) => item.id), ["a", "b", "c"]);
});

test("searchCommands 带最近使用时，只影响空查询的顺序", () => {
  const recent = ["toggle-theme"];
  const empty = searchCommands(COMMANDS, "", 10, recent);
  assert.equal(empty[0].id, "toggle-theme", "最近用过的应排最前");
  assert.equal(empty.length, 10);

  const typed = searchCommands(COMMANDS, "体检", 10, recent);
  assert.equal(typed[0].id, "health-check", "有查询时仍按相关度排序");
});

test("状态文案会把「最近用过」说清楚，但没有记录时不提", () => {
  assert.match(describeCommandPaletteState({ total: 17, query: "", recentCount: 3 }), /最近用过 3 条/);
  assert.ok(!/最近用过/.test(describeCommandPaletteState({ total: 17, query: "" })), "没有最近记录时不该出现这句");
  assert.match(describeCommandPaletteState({ total: 17, shown: 2, query: "组" }), /匹配 2 \/ 17 条命令/);
});
