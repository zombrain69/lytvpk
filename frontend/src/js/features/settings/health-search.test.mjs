import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  describeHealthSearch,
  healthHighlightSpec,
  healthIssueTags,
  searchHealthIssues,
} from "./health-search.mjs";

const issues = [
  {
    kind: "missing_file",
    severity: "critical",
    name: "ak47.vpk",
    path: "D:\\addons\\ak47.vpk",
    message: "addonlist.txt 记录了 ak47.vpk，但磁盘上找不到对应文件",
  },
  {
    kind: "file_type_mismatch",
    severity: "warning",
    name: "ghost.vpk",
    path: "D:\\addons\\ghost.vpk",
    message: "同名路径是一个文件夹：游戏只加载 *.vpk 文件",
  },
  {
    kind: "dependency_disabled",
    severity: "warning",
    name: "master.vpk",
    target: "master.vpk",
    message: "master.vpk 已开启，但依赖 dep-off.vpk 处于关闭状态",
  },
  {
    kind: "unrecorded",
    severity: "info",
    name: "new.vpk",
    path: "D:\\addons\\new.vpk",
    location: "root",
    message: "没有写入 addonlist.txt：游戏内开关状态未记录",
  },
];

const names = (result) => result.map((issue) => issue.name);

test("普通词跨字段匹配（对象名 / 路径 / 文案），多词是「且」", () => {
  assert.deepEqual(names(searchHealthIssues(issues, "").items), ["ak47.vpk", "ghost.vpk", "master.vpk", "new.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "ghost").items), ["ghost.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "文件夹").items), ["ghost.vpk"]);
  // 两个词必须落在同一条上：master.vpk 的文案里有 master 与 dep-off
  assert.deepEqual(names(searchHealthIssues(issues, "master dep-off").items), ["master.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "addonlist dep-off").items), [], "分属两条时不该命中");
  assert.equal(searchHealthIssues(issues, "ghost ak47").items.length, 0);
});

test("-排除 用子串（不误伤长路径）与 re: 正则", () => {
  assert.deepEqual(names(searchHealthIssues(issues, "vpk -ghost").items), ["ak47.vpk", "master.vpk", "new.vpk"]);
  // 长路径里的凑字母不该误伤（这里 path 是完整路径）
  assert.deepEqual(names(searchHealthIssues(issues, "vpk -old").items).length, 4);
  // 四条的名字都以 .vpk 结尾（master.vpk 也算）
  assert.deepEqual(names(searchHealthIssues(issues, "re:\\.vpk$").items), ["ak47.vpk", "ghost.vpk", "master.vpk", "new.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "re:^ghost").items), ["ghost.vpk"]);
  const invalid = searchHealthIssues(issues, "re:[");
  assert.equal(invalid.items.length, 0);
  assert.match(describeHealthSearch({ ...invalid, query: "re:[" }), /^正则表达式无效：/);
});

test("tag: 按严重度与问题类型筛", () => {
  assert.deepEqual(healthIssueTags(issues[0]), ["严重", "条目缺少文件"]);
  assert.deepEqual(healthIssueTags(issues[1]), ["警告", "同名路径是文件夹"]);

  assert.deepEqual(names(searchHealthIssues(issues, "tag:严重").items), ["ak47.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "tag:警告").items), ["ghost.vpk", "master.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "tag:条目缺少文件").items), ["ak47.vpk"]);
  assert.deepEqual(names(searchHealthIssues(issues, "tag:严重|提示").items), ["ak47.vpk", "new.vpk"]);
  // 与文本组合："警告里名字含 master 的"
  assert.deepEqual(names(searchHealthIssues(issues, "tag:警告 master").items), ["master.vpk"]);
});

test("计数文案与高亮参数", () => {
  assert.equal(describeHealthSearch({}), "");
  assert.equal(describeHealthSearch({ total: 4, matched: 2, query: "vpk" }), "匹配 2 / 4 个问题");
  assert.equal(describeHealthSearch({ total: 4, matched: 0, query: "zzz" }), "没有匹配「zzz」的问题（共 4 个）");
  assert.equal(healthHighlightSpec(searchHealthIssues(issues, "")), null);
  assert.equal(healthHighlightSpec(searchHealthIssues(issues, "re:[")), null);
  assert.deepEqual(healthHighlightSpec(searchHealthIssues(issues, "ghost -ak47")).terms, ["ghost"]);
});

test("设置页真的接上了检索（接线断言）", () => {
  const page = readFileSync(new URL("./settings-page.js", import.meta.url), "utf8");
  assert.match(page, /searchHealthIssues\(/, "体检列表要用统一检索");
  assert.match(page, /describeHealthSearch\(/, "要有命中计数");
  assert.match(page, /healthHighlightSpec\(/, "命中项要高亮");
  assert.match(page, /HEALTH_SEARCH_HELP_VARIANT|buildSearchHelpHtml/, "要有语法提示（与其它面板同源）");
});
