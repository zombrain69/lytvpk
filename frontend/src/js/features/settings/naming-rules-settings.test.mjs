// 命名规范化规则配置（路线图第 20 项 P3）的接线测试（设置页侧）：
//   - 设置页「命名规范化」卡片：渲染数据来自 GetNamingRules、增删走对应绑定、
//     校验走 naming-rules-view.mjs、重画后恢复分区；
//   - app-runtime 依赖注入齐平（漏一处会静默失效）；
//   - i18n 目录：zh 逐字 + en 对齐（占位符全量对齐由 i18n-catalog.test.mjs 兜底）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (rel) => readFileSync(new URL(rel, import.meta.url), "utf8");
const readJson = (rel) => JSON.parse(read(rel));

test("设置页：命名规范化卡片（输入行 / 列表容器 / 增删接线 / 分区恢复）", () => {
  const source = read("./settings-page.js");
  assert.match(source, /settings\.addonlist\.maintenance\.naming\.cardTitle/);
  assert.match(source, /id="settings-naming-alias-from"/);
  assert.match(source, /id="settings-naming-alias-to"/);
  assert.match(source, /id="settings-naming-alias-add"/);
  assert.match(source, /id="settings-naming-alias-list"/);
  assert.match(source, /id="settings-naming-exempt-list"/);
  assert.match(source, /deps\.GetNamingRules\?\.\(\)/, "渲染时读取规则视图");
  assert.match(source, /deps\.AddNamingRulesAlias\(/, "添加词表替换走绑定");
  assert.match(source, /deps\.RemoveNamingRulesAlias\(/, "移除词表替换走绑定");
  assert.match(source, /deps\.RemoveNamingExemptions\(\[relPath\]\)/, "移出例外走绑定");
  assert.match(source, /validateAliasInput\(/, "输入校验走纯逻辑模块");
  assert.match(source, /data-naming-alias-from/, "列表移除按钮带数据属性");
  assert.match(source, /data-naming-exempt-path/);
  // 增删后整页重画并把分区恢复到 addonlist（否则跳回第一个分区）。
  assert.match(source, /refreshNamingRulesPanel/);
  assert.match(source, /data-panel="addonlist"/);
});

test("app-runtime：新绑定已导入并注入设置页 deps", () => {
  const runtime = read("../app-runtime.js");
  for (const name of [
    "GetNamingRules",
    "AddNamingRulesAlias",
    "RemoveNamingRulesAlias",
    "AddNamingExemptions",
    "RemoveNamingExemptions",
  ]) {
    assert.ok(
      (runtime.match(new RegExp(`\\b${name},`, "g")) || []).length >= 2,
      `${name} 应同时出现在 wailsjs 导入与 configureSettings 注入里`,
    );
  }
});

test("i18n 目录：命名卡片文案 zh 逐字 + en 对齐", () => {
  const zh = readJson("../../../messages/zh-CN.json").settings.addonlist.maintenance.naming;
  const en = readJson("../../../messages/en.json").settings.addonlist.maintenance.naming;
  assert.equal(zh.cardTitle, "命名规范化");
  assert.equal(en.cardTitle, "Name normalization");
  for (const key of Object.keys(zh)) {
    assert.equal(typeof en[key], "string", `en 缺 naming.${key}`);
  }
  // 动态拼装用到的占位符（aliasAdded 的 {from}/{to}，exemptRemoved 的 {count}）。
  assert.match(zh.aliasAdded, /\{from\}/);
  assert.match(zh.aliasAdded, /\{to\}/);
  assert.match(zh.exemptRemoved, /\{count\}/);
  assert.equal(zh.aliasRemove, "移除");
  assert.equal(zh.exemptRemove, "移出");
});
