// 「按分类建议填充」（路线图第 20 项 P1）的接线测试：
// 纯逻辑（S1–S5 推演、幂等、63B）由 Go 侧 naming_rules_test.go 覆盖；
// 这里锁前端接线 —— index.html 按钮与备注元素、operations.js 的调用与回填、
// i18n 目录条目（zh 逐字 + en 存在）、以及 wailsjs 绑定三件套（防重生成漂移）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const operationsSource = readFileSync(new URL("./operations.js", import.meta.url), "utf8");
const indexHtml = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
const zhCatalog = JSON.parse(readFileSync(new URL("../../../messages/zh-CN.json", import.meta.url), "utf8"));
const enCatalog = JSON.parse(readFileSync(new URL("../../../messages/en.json", import.meta.url), "utf8"));
const appBindings = readFileSync(new URL("../../../../wailsjs/go/app/App.js", import.meta.url), "utf8");
const appTypes = readFileSync(new URL("../../../../wailsjs/go/app/App.d.ts", import.meta.url), "utf8");
const modelsTs = readFileSync(new URL("../../../../wailsjs/go/models.ts", import.meta.url), "utf8");

test("重命名弹窗有「按分类建议填充」按钮与建议备注元素", () => {
  assert.match(indexHtml, /id="fill-rename-from-category-btn"/);
  assert.match(indexHtml, /data-i18n="modals\.rename\.useCategorySuggestion"/);
  assert.match(indexHtml, /data-i18n-title="modals\.rename\.useCategoryTooltip"/);
  assert.match(indexHtml, /id="rename-suggestion-note"/);
});

test("operations.js 接线：导入绑定、点击调用、回填输入框与备注", () => {
  assert.match(operationsSource, /SuggestNormalizedName,/, "要从 wailsjs 导入绑定");
  assert.match(operationsSource, /getElementById\("fill-rename-from-category-btn"\)/);
  assert.match(operationsSource, /getElementById\("rename-suggestion-note"\)/);
  assert.match(operationsSource, /await SuggestNormalizedName\(filePath\)/, "点击要调用后端建议接口");
  assert.match(operationsSource, /input\.value = suggestion\.stem;/, "建议要回填到输入框");
  assert.match(operationsSource, /updateRenameLengthHint\(\);/, "回填后要刷新长度提示");
  // 备注要带依据与三个旗标（待复核 / 截断 / 放不下）。
  assert.match(operationsSource, /suggestNote", \{ reason: suggestion\.reason \}/);
  assert.match(operationsSource, /suggestion\.pendingReview/);
  assert.match(operationsSource, /suggestion\.truncated/);
  assert.match(operationsSource, /suggestion\.fits/);
  // 失败路径要还原按钮状态并提示。
  assert.match(operationsSource, /modals\.rename\.suggestFailed/);
  assert.match(operationsSource, /fillFromCategoryBtn\.disabled = false;[\s\S]*fillFromCategoryBtn\.disabled = false;/, "结束时要还原按钮");
  // 关闭弹窗时清掉 handler（与既有 fillFromTitle 的清理口径一致）。
  assert.match(operationsSource, /fillFromCategoryBtn\.onclick = null;/);
});

test("i18n 目录：8 个新键 zh 逐字 + en 对齐", () => {
  const zh = zhCatalog.modals.rename;
  assert.equal(zh.useCategorySuggestion, "按分类建议填充");
  assert.equal(zh.suggestNote, "建议依据：{reason}");
  assert.equal(zh.suggestFailed, "生成命名建议失败: {error}");
  assert.equal(zh.suggestPending, "（低置信，请复核）");
  assert.equal(zh.suggestTruncated, "（标题已按 63 字节上限截断）");
  assert.equal(zh.suggestUnfittable, "（分类前缀与替换对象已超出 63 字节，无法压缩）");
  assert.equal(zh.suggestEmpty, "没有生成可用的命名建议");
  assert.ok(zh.useCategoryTooltip && zh.useCategoryTooltip.length > 10);

  const en = enCatalog.modals.rename;
  for (const key of [
    "useCategorySuggestion",
    "useCategoryTooltip",
    "suggestFailed",
    "suggestEmpty",
    "suggestNote",
    "suggestPending",
    "suggestTruncated",
    "suggestUnfittable",
  ]) {
    assert.ok(typeof en[key] === "string" && en[key].trim(), `en 缺键 ${key}`);
  }
});

test("wailsjs 绑定三件套包含 SuggestNormalizedName（防重生成漂移）", () => {
  assert.match(appBindings, /export function SuggestNormalizedName\(arg1\) \{\s*return window\['go'\]\['app'\]\['App'\]\['SuggestNormalizedName'\]\(arg1\);\s*\}/);
  assert.match(appTypes, /export function SuggestNormalizedName\(arg1:string\):Promise<parser\.NamingSuggestion>;/);
  assert.match(modelsTs, /export class NamingSuggestion \{/);
  assert.match(modelsTs, /this\.pendingReview = source\["pendingReview"\];/);
});
