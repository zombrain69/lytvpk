// 「禁用所有 X」范围词的 locale 感知（路线图第 21 项阶段 B ③）。
//
// 背景：`disableAllMods(primaryTag)` 曾用 `${primaryTag}mod` 直接拼范围词 ——
// en 界面会渲染成「正文里插着中文的 character mods」：「人物mod」。
// 现在范围词走目录（mods.ops.disableAll.scope.*）：zh 保留「人物mod」既有写法，
// en 用独立文案。断言分三层：源码接线（不再手拼）、目录内容（zh 逐字 / en 无中文）、
// 插值实测（两种语言各渲染一遍）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { installCatalogs, setLocale, t } from "../../core/i18n-runtime.js";

const read = (rel) => readFileSync(new URL(rel, import.meta.url), "utf8");
const readJson = (rel) => JSON.parse(read(rel));

const actions = read("./actions.js");
const zh = readJson("../../../messages/zh-CN.json");
const en = readJson("../../../messages/en.json");

installCatalogs({ "zh-CN": zh, en }, "zh-CN");

const CJK = /[\u4e00-\u9fff]/;

test("源码接线：范围词走目录 key，不再手拼 `${tag}mod`", () => {
  assert.match(actions, /const DISABLE_SCOPE_LABEL_KEYS = \{/);
  assert.match(actions, /"人物": "mods\.ops\.disableAll\.scope\.characters"/);
  assert.match(actions, /"武器": "mods\.ops\.disableAll\.scope\.weapons"/);
  assert.match(
    actions,
    /t\(DISABLE_SCOPE_LABEL_KEYS\[primaryTag\] \|\| "mods\.ops\.disableAll\.scope\.tag", \{ tag: primaryTag \}\)/,
    "已知标签查表、未知标签走 scope.tag 模板",
  );
  assert.match(actions, /t\("mods\.ops\.disableAll\.scope\.all"\)/);
  assert.ok(
    !actions.includes("${primaryTag}mod"),
    "不得再把中文标签直接拼进范围词（en 界面会混排）",
  );
});

test("目录内容：zh 逐字保留既有写法，en 无中文残渣", () => {
  const scope = (catalog) => catalog.mods.ops.disableAll.scope;
  assert.equal(scope(zh).all, "mod");
  assert.equal(scope(zh).characters, "人物mod");
  assert.equal(scope(zh).weapons, "武器mod");
  assert.equal(scope(zh).tag, "{tag}mod");

  assert.equal(scope(en).all, "all Mods");
  assert.equal(scope(en).characters, "character Mods");
  assert.equal(scope(en).weapons, "weapon Mods");
  assert.equal(scope(en).tag, "{tag} Mods");
  for (const value of Object.values(scope(en))) {
    assert.ok(!CJK.test(value), `en 范围词仍含中文：${value}`);
  }
});

test("插值实测：zh 渲染与替换前逐字一致，en 不再混排中文", () => {
  setLocale("zh-CN");
  assert.equal(
    t("mods.ops.disableAll.confirmTitle", { scope: t("mods.ops.disableAll.scope.characters") }),
    "确认禁用人物mod",
  );
  assert.equal(
    t("mods.ops.disableAll.progress", { scope: t("mods.ops.disableAll.scope.all") }),
    "正在禁用mod...",
  );
  assert.equal(
    t("mods.ops.disableAll.scope.tag", { tag: "动画版" }),
    "动画版mod",
  );

  setLocale("en");
  assert.equal(
    t("mods.ops.disableAll.confirmTitle", { scope: t("mods.ops.disableAll.scope.characters") }),
    "Confirm disabling character Mods",
  );
  assert.equal(
    t("mods.ops.disableAll.progress", { scope: t("mods.ops.disableAll.scope.weapons") }),
    "Disabling weapon Mods...",
  );
  assert.equal(t("mods.ops.disableAll.scope.tag", { tag: "Milfy" }), "Milfy Mods");
});
