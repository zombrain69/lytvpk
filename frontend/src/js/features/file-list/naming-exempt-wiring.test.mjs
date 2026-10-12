// 命名例外（路线图第 20 项 P3）的接线测试（Mod 列表侧）：
//   - 右键批量菜单「加入 / 移出命名例外」入口与 naming-exempt.js 的调用接线；
//   - render.js 的 lock / unlock 图标；
//   - 批量弹窗摘要透传豁免计数、跳过原因文案存在；
//   - wailsjs 绑定三件套包含命名规则方法与模型（防重生成漂移）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const read = (rel) => readFileSync(new URL(rel, import.meta.url), "utf8");
const readJson = (rel) => JSON.parse(read(rel));

test("右键批量菜单：加入 / 移出命名例外入口", () => {
  const menu = read("./context-menu.js");
  assert.match(
    menu,
    /import \{[\s\S]*?addNamingExemptionsForPaths[\s\S]*?\} from "\.\/naming-exempt\.js"/,
  );
  assert.match(menu, /t\("mods\.contextMenu\.namingExemptAdd"\), iconSvg\("lock"\)/);
  assert.match(menu, /t\("mods\.contextMenu\.namingExemptRemove"\), iconSvg\("unlock"\)/);
  assert.match(menu, /addNamingExemptionsForPaths\(selectedPaths\)/);
  assert.match(menu, /removeNamingExemptionsForPaths\(selectedPaths\)/);
});

test("naming-exempt.js：调后端绑定 + 成功 / 跳过 / 失败三类反馈", () => {
  const source = read("./naming-exempt.js");
  assert.match(source, /import \{ AddNamingExemptions, RemoveNamingExemptions \} from "\.\.\/\.\.\/\.\.\/\.\.\/wailsjs\/go\/app\/App"/);
  assert.match(source, /await action\(paths\)/);
  assert.match(source, /namingExemptAdded/);
  assert.match(source, /namingExemptRemoved/);
  assert.match(source, /namingExemptSkipped/);
  assert.match(source, /namingExemptFailed/);
});

test("render.js：lock / unlock 图标已定义", () => {
  const source = read("./render.js");
  assert.match(source, /lock: `<svg/);
  assert.match(source, /unlock: `<svg/);
});

test("批量弹窗：摘要透传豁免计数", () => {
  const wiring = read("./batch-naming.js");
  assert.match(wiring, /exempted: currentPlan\.exempted \|\| 0/);
});

test("i18n 目录：例外菜单与跳过原因文案 zh 逐字 + en 对齐", () => {
  const zh = readJson("../../../messages/zh-CN.json");
  const en = readJson("../../../messages/en.json");
  assert.equal(zh.mods.contextMenu.namingExemptAdd, "加入命名例外");
  assert.equal(zh.mods.contextMenu.namingExemptRemove, "移出命名例外");
  assert.equal(zh.modals.batchNaming.skip.exempt, "在命名例外表里（不参与批量规范化）");
  assert.match(zh.modals.batchNaming.summary, /\{exempted\}/);
  assert.match(en.modals.batchNaming.summary, /\{exempted\}/);
  for (const key of ["namingExemptAdd", "namingExemptRemove", "namingExemptAdded", "namingExemptRemoved", "namingExemptSkipped", "namingExemptFailed"]) {
    assert.equal(typeof en.mods.contextMenu[key], "string", `en 缺 contextMenu.${key}`);
  }
  assert.equal(typeof en.modals.batchNaming.skip.exempt, "string");
});

test("wailsjs 绑定三件套包含命名规则方法与模型（防重生成漂移）", () => {
  const appBindings = read("../../../../wailsjs/go/app/App.js");
  const appTypes = read("../../../../wailsjs/go/app/App.d.ts");
  const modelsTs = read("../../../../wailsjs/go/models.ts");
  assert.match(appBindings, /export function GetNamingRules\(\) \{\s*return window\['go'\]\['app'\]\['App'\]\['GetNamingRules'\]\(\);\s*\}/);
  assert.match(appBindings, /export function AddNamingRulesAlias\(arg1, arg2\)/);
  assert.match(appBindings, /export function RemoveNamingRulesAlias\(arg1\)/);
  assert.match(appBindings, /export function AddNamingExemptions\(arg1\)/);
  assert.match(appBindings, /export function RemoveNamingExemptions\(arg1\)/);
  assert.match(appTypes, /GetNamingRules\(\):Promise<app\.NamingRulesView>/);
  assert.match(appTypes, /AddNamingRulesAlias\(arg1:string,arg2:string\):Promise<app\.NamingRulesMutationResult>/);
  assert.match(appTypes, /AddNamingExemptions\(arg1:Array<string>\):Promise<app\.NamingRulesMutationResult>/);
  assert.match(modelsTs, /export class NamingRulesView \{/);
  assert.match(modelsTs, /export class NamingExemptionView \{/);
  assert.match(modelsTs, /export class NamingSubjectAlias \{/);
  assert.match(modelsTs, /export class NamingRulesMutationResult \{/);
});
