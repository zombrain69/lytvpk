import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  conflictGroupTags,
  conflictHighlightSpec,
  describeConflictSearch,
  filterConflictGroupsByQuery,
  searchConflictGroups,
} from "./conflict-search.mjs";

const groups = [
  {
    severity: "critical",
    layer: 3,
    files: ["materials/models/ak47/ak47.vtf", "materials/models/ak47/ak47.vmt"],
    file_count: 2,
    vpk_files: [
      { name: "ak47-hd.vpk", title: "AK47 高清", path: "D:\\addons\\ak47-hd.vpk", order: 3 },
      { name: "ak47-cn.vpk", title: "AK47 国风", path: "D:\\addons\\ak47-cn.vpk", order: 3 },
    ],
  },
  {
    severity: "warning",
    layer: null,
    files: ["sound/weapons/katana.wav"],
    file_count: 1,
    vpk_files: [
      { name: "katana-a.vpk", title: "武士刀 A", path: "D:\\addons\\katana-a.vpk", order: 8 },
      { name: "katana-b.vpk", title: "武士刀 B", path: "D:\\addons\\katana-b.vpk", order: -1 },
    ],
  },
  {
    severity: "info",
    layer: 9,
    files: ["scripts/weapons/medkit.nut"],
    file_count: 1,
    vpk_files: [{ name: "medkit.vpk", title: "医疗箱", path: "D:\\addons\\medkit.vpk", order: 9 }],
  },
];

const names = (result) => result.map((group) => group.vpk_files[0].name);

test("普通词跨字段匹配（资源路径 / Mod 名 / 标题），多词是「且」", () => {
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "ak47")), ["ak47-hd.vpk"]);
  // 一个词命中资源路径、另一个词命中 Mod 名，也要能同时成立
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "materials ak47-cn")), ["ak47-hd.vpk"]);
  // "weapons" 只在武士刀的 wav 路径与医疗箱的 nut 路径里（AK47 那组是 materials 路径）
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "weapons")), ["katana-a.vpk", "medkit.vpk"]);
  assert.equal(filterConflictGroupsByQuery(groups, "ak47 katana").length, 0);
});

test("-排除 与 re: 正则；正则写错时给原因而不是空列表", () => {
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "vpk -katana")), ["ak47-hd.vpk", "medkit.vpk"]);
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "re:\\.nut$")), ["medkit.vpk"]);
  const invalid = searchConflictGroups(groups, "re:[");
  assert.equal(invalid.items.length, 0);
  assert.match(invalid.regexInvalid, /regular expression/i);
  assert.match(describeConflictSearch({ ...invalid, query: "re:[" }), /^正则表达式无效：/);
});

test("tag: 按严重度 / 同层 / 未记录筛冲突", () => {
  assert.deepEqual(conflictGroupTags(groups[0]), ["严重", "同层"]);
  assert.deepEqual(conflictGroupTags(groups[1]), ["警告", "未记录"]);
  assert.deepEqual(conflictGroupTags(groups[2]), ["提示", "同层"]);

  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "tag:严重")), ["ak47-hd.vpk"]);
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "tag:未记录")), ["katana-a.vpk"]);
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "tag:严重|警告")), ["ak47-hd.vpk", "katana-a.vpk"]);
  // 与资源路径组合："同层冲突里涉及 materials 的"
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "tag:同层 materials")), ["ak47-hd.vpk"]);
  assert.deepEqual(names(filterConflictGroupsByQuery(groups, "tag:同层 -tag:严重")), ["medkit.vpk"]);
});

test("计数文案与高亮参数", () => {
  assert.equal(describeConflictSearch({}), "");
  assert.equal(describeConflictSearch({ total: 3, matched: 2, query: "ak47" }), "匹配 2 / 3 个组冲突");
  assert.equal(describeConflictSearch({ total: 3, matched: 0, query: "zzz" }), "没有匹配「zzz」的组冲突（共 3 个）");
  assert.equal(conflictHighlightSpec(searchConflictGroups(groups, "")), null);
  assert.equal(conflictHighlightSpec(searchConflictGroups(groups, "re:[")), null);
  assert.deepEqual(conflictHighlightSpec(searchConflictGroups(groups, "ak47 -katana")).terms, ["ak47"]);
});

test("冲突弹窗真的接上了统一检索", () => {
  const source = readFileSync(new URL("./conflicts.js", import.meta.url), "utf8");
  assert.match(source, /searchConflictGroups\(/, "列表要用统一检索");
  assert.match(source, /describeConflictSearch\(/, "要有命中计数");
  assert.match(source, /conflictHighlightSpec\(/, "命中项要高亮");
  assert.match(source, /CONFLICT_SEARCH_HELP_VARIANT|buildSearchHelpHtml/, "要有语法提示（与其它面板同源）");
  assert.match(source, /已过滤/, "覆盖关系区被过滤时要写清「显示 / 总数」");
});

// 真机踩出来的坑：字段里有完整路径，排除词如果也用模糊匹配，
// `-katana` 会在 `…\lytvpk\…\addons\ak47-hd.vpk` 里"凑字母"命中，把不相干的组排掉。
test("排除词用子串，不会在长路径里凑字母误伤", () => {
  const longPathGroup = [
    {
      severity: "info",
      layer: null,
      files: ["materials/models/ak47/ak47.vtf"],
      file_count: 1,
      vpk_files: [
        {
          name: "ak47-hd.vpk",
          title: "ak47-hd.vpk",
          path: "E:\\SteamLibrary\\steamapps\\common\\Left 4 Dead 2\\program\\lytvpk\\internal\\.tmp-cua\\fixture8\\left4dead2\\addons\\ak47-hd.vpk",
          order: -1,
        },
      ],
    },
  ];
  assert.deepEqual(names(filterConflictGroupsByQuery(longPathGroup, "vpk -katana")), ["ak47-hd.vpk"]);
  assert.deepEqual(names(filterConflictGroupsByQuery(longPathGroup, "vpk -old")), ["ak47-hd.vpk"]);
  // 真正包含时照旧排除
  assert.equal(filterConflictGroupsByQuery(longPathGroup, "vpk -ak47").length, 0);
  assert.equal(filterConflictGroupsByQuery(longPathGroup, "vpk -hd.vpk").length, 0);
  // 正向词仍然是模糊的（与 Mod 列表一致）
  assert.deepEqual(names(filterConflictGroupsByQuery(longPathGroup, "a4")), ["ak47-hd.vpk"]);
});
