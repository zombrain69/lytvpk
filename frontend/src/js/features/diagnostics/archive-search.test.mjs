import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { test } from "node:test";

import {
  archivePackageStateTags,
  archiveHighlightSpec,
  describeArchiveSearchResult,
  searchArchivePackages,
} from "./archive-search.mjs";
import { ARCHIVE_SEARCH_HELP_VARIANT, buildSearchHelpTitle } from "../file-list/search-help.mjs";

const packages = [
  {
    name: "mods-a.zip",
    path: "D:\\backup\\mods-a.zip",
    size: 100,
    vpks: [{ name: "ak47.vpk", entryPath: "weapons/ak47.vpk", matchState: "new" }],
  },
  {
    name: "武器包.zip",
    path: "D:\\backup\\武器包.zip",
    size: 200,
    requiresPassword: true,
    vpks: [],
  },
  { name: "broken.zip", path: "D:\\backup\\broken.zip", size: 300, error: "损坏", vpks: [] },
  {
    name: "old.zip",
    path: "D:\\backup\\old.zip",
    size: 400,
    vpks: [{ name: "ak47.vpk", entryPath: "weapons/ak47.vpk", matchState: "existing" }],
  },
];

const names = (result) => result.items.map((item) => item.name);

test("普通词跨字段做「且」匹配，并在多词时同时要求命中", () => {
  assert.deepEqual(names(searchArchivePackages(packages, "ak47")), ["mods-a.zip", "old.zip"]);
  // 压缩包名 + 里面的 VPK 名各命中一个词 → 两个词不同来源也要能同时命中
  assert.deepEqual(names(searchArchivePackages(packages, "mods ak47")), ["mods-a.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "backup ak47")), ["mods-a.zip", "old.zip"]);
  assert.equal(searchArchivePackages(packages, "ak47 不存在").items.length, 0);
});

test("引号短语与 -排除（排除词必须让命中消失）", () => {
  assert.deepEqual(names(searchArchivePackages(packages, '"mods-a"')), ["mods-a.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "backup -ak47")), ["武器包.zip", "broken.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "-zip")), []);
});

test("re: 正则生效；写错正则要明确报错而不是静默空列表", () => {
  assert.deepEqual(names(searchArchivePackages(packages, "re:^mods-")), ["mods-a.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "re:ak\\d\\d\\.vpk")), ["mods-a.zip", "old.zip"]);

  const invalid = searchArchivePackages(packages, "re:[");
  assert.equal(invalid.items.length, 0);
  assert.match(invalid.regexInvalid, /regular expression/i);
  // 界面文案负责把它说成人话（与 Mod 列表同风格）
  assert.match(describeArchiveSearchResult({ ...invalid, query: "re:[" }), /^正则表达式无效：/);
  // 合法正则不该报错
  assert.equal(searchArchivePackages(packages, "re:[a-z]+").regexInvalid, "");
});

test("tag: 在归档列表里筛包状态，且支持 | 的「或」写法", () => {
  assert.deepEqual(names(searchArchivePackages(packages, "tag:密码")), ["武器包.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "tag:错误")), ["broken.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "tag:待导入")), ["mods-a.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "tag:已有")), ["old.zip"]);
  assert.deepEqual(names(searchArchivePackages(packages, "tag:密码|错误")), ["武器包.zip", "broken.zip"]);
  // 不认识的 tag 不该把整张表清空（和 Mod 列表一样：无命中就是不命中，但要能解释）
  assert.equal(searchArchivePackages(packages, "tag:不存在的状态").items.length, 0);
});

test("空查询返回全部，且统计与命中数一致", () => {
  const all = searchArchivePackages(packages, "   ");
  assert.equal(all.items.length, packages.length);
  assert.equal(all.matched, packages.length);
  assert.equal(all.total, packages.length);
  assert.equal(all.regexInvalid, "");

  const some = searchArchivePackages(packages, "ak47");
  assert.equal(some.matched, 2);
  assert.equal(some.total, packages.length);
});

test("archivePackageStateTags 覆盖四种包状态", () => {
  assert.deepEqual(archivePackageStateTags(packages[0]), ["待导入"]);
  assert.deepEqual(archivePackageStateTags(packages[1]), ["密码"]);
  assert.deepEqual(archivePackageStateTags(packages[2]), ["错误"]);
  assert.deepEqual(archivePackageStateTags(packages[3]), ["已有"]);
  // 语法提示与语法表同源，不在两个文件里各写一份
  const title = buildSearchHelpTitle(ARCHIVE_SEARCH_HELP_VARIANT);
  assert.match(title, /tag:密码/);
  assert.match(title, /re:/);
  assert.match(title, /匹配：压缩包名/);
});

test("计数文案与 Mod 列表同风格，只是名词不同", () => {
  assert.equal(describeArchiveSearchResult({}), "");
  assert.equal(
    describeArchiveSearchResult({ total: 4, matched: 2, query: "ak47" }),
    "匹配 2 / 4 个压缩包",
  );
  assert.equal(
    describeArchiveSearchResult({ total: 4, matched: 0, query: "zzz" }),
    "没有匹配「zzz」的压缩包（共 4 个）",
  );
});

test("高亮参数只跟正向普通词与合法正则走", () => {
  assert.equal(archiveHighlightSpec(searchArchivePackages(packages, "")), null);
  assert.equal(archiveHighlightSpec(searchArchivePackages(packages, "re:[")), null);

  const spec = archiveHighlightSpec(searchArchivePackages(packages, "ak47 -old re:vpk"));
  assert.deepEqual(spec.terms, ["ak47"]);
  assert.ok(spec.regex instanceof RegExp);
  assert.equal(spec.regex.test("ak47.vpk"), true);
});

// 纯函数绿了不代表界面接上了：这里静态检查归档管理器真的用上这套检索。
test("归档管理器接上了统一检索（语法提示 / 计数 / 高亮 / 状态 chip）", () => {
  const source = readFileSync(new URL("./archive-manager.js", import.meta.url), "utf8");
  assert.match(source, /searchArchivePackages\(archiveManagerPackages, archiveManagerQuery\)/, "文本检索要走统一语法");
  assert.match(source, /describeArchiveSearchResult\(/, "要有命中计数文案");
  assert.match(source, /highlightMatches\(titleText, highlightSpec\)/, "命中的压缩包名要高亮");
  assert.match(source, /search\.title = buildSearchHelpTitle\(ARCHIVE_SEARCH_HELP_VARIANT\)/, "搜索框提示要与说明书同源");
  assert.match(source, /helpPopover\.innerHTML = buildSearchHelpHtml\(ARCHIVE_SEARCH_HELP_VARIANT\)/, "要挂上同一份说明书浮层");
  assert.match(source, /tag:状态/, "占位文字要能看出支持 tag: 语法");
  assert.match(source, /archivePackageStateTags\(item\)/, "tag: 命中要说清是哪个状态");
  // 键盘光标（与 Mod 列表同一套：↑↓ 移动、Enter 展开、Esc 清空）
  assert.match(source, /nextResultPath\(keys, archiveCursorPath/, "缺少上下键移动光标");
  assert.match(source, /describeResultCursor\(keys, archiveCursorPath, "Enter 展开 \/ 收起"\)/, "计数要显示光标位置与 Enter 的作用");
  assert.match(source, /section\.dataset\.cursorKey = String\(item\.path/, "行上要有光标键");
  assert.match(source, /syncCursorHighlight\(list, "\.archive-manager-package\[data-cursor-key\]", archiveCursorPath\)/, "重画后要恢复光标");
});
