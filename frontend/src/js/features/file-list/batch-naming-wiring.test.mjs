// 批量规范化命名（路线图第 20 项 P2）的接线测试：
// 纯逻辑由 batch-naming-view.test.mjs 覆盖；这里锁 —— index.html 弹窗与工具栏入口、
// batch-naming.js / context-menu.js / app-runtime.js 的调用接线（含「确认先于执行」）、
// i18n 目录（zh 逐字 + en 对齐）、以及 wailsjs 绑定三件套（防重生成漂移）。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const wiringSource = readFileSync(new URL("./batch-naming.js", import.meta.url), "utf8");
const menuSource = readFileSync(new URL("./context-menu.js", import.meta.url), "utf8");
const runtimeSource = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const escapeSource = readFileSync(new URL("../../core/modal-escape.mjs", import.meta.url), "utf8");
const indexHtml = readFileSync(new URL("../../../../index.html", import.meta.url), "utf8");
const zhCatalog = JSON.parse(readFileSync(new URL("../../../messages/zh-CN.json", import.meta.url), "utf8"));
const enCatalog = JSON.parse(readFileSync(new URL("../../../messages/en.json", import.meta.url), "utf8"));
const appBindings = readFileSync(new URL("../../../../wailsjs/go/app/App.js", import.meta.url), "utf8");
const appTypes = readFileSync(new URL("../../../../wailsjs/go/app/App.d.ts", import.meta.url), "utf8");
const modelsTs = readFileSync(new URL("../../../../wailsjs/go/models.ts", import.meta.url), "utf8");

test("index.html：弹窗节点、工具栏与右键菜单入口的静态挂点齐全", () => {
  assert.match(indexHtml, /id="batch-naming-modal"/);
  assert.match(indexHtml, /id="close-batch-naming-modal-btn"/);
  assert.match(indexHtml, /id="cancel-batch-naming-btn"/);
  assert.match(indexHtml, /id="apply-batch-naming-btn"/);
  assert.match(indexHtml, /id="copy-batch-naming-plan-btn"/);
  assert.match(indexHtml, /id="copy-batch-naming-undo-btn"/);
  assert.match(indexHtml, /id="batch-naming-list"/);
  assert.match(indexHtml, /id="batch-naming-summary"/);
  assert.match(indexHtml, /id="batch-naming-select-all"/);
  assert.match(indexHtml, /id="batch-normalize-names-btn"/);
  assert.match(indexHtml, /data-i18n="mods\.batch\.normalizeNames"/);
  assert.match(indexHtml, /data-i18n="modals\.batchNaming\.title"/);
  assert.match(indexHtml, /data-i18n="modals\.batchNaming\.copyPlan"/);
  assert.match(indexHtml, /data-i18n="modals\.batchNaming\.copyUndo"/);
  assert.match(indexHtml, /data-i18n="modals\.batchNaming\.apply"/);
});

test("batch-naming.js 接线：计划 → 勾选 → 确认 → 执行 → 结果，且订阅进度事件", () => {
  assert.match(wiringSource, /import \{ ApplyNamingNormalization, PlanNamingNormalization \} from "\.\.\/\.\.\/\.\.\/\.\.\/wailsjs\/go\/app\/App";/);
  assert.match(wiringSource, /await PlanNamingNormalization\(paths\)/, "打开弹窗要调用只读计划接口");
  assert.match(wiringSource, /await ApplyNamingNormalization\(requests\)/, "执行要走批量接口");
  assert.match(wiringSource, /EventsOn\("naming_batch_progress"/, "要订阅后端进度事件");
  assert.match(wiringSource, /buildApplyRequests\(currentPlan, checkedPaths\)/, "执行请求要由勾选拼装");
  // 低置信项被勾选时必须先确认（确认先于执行）。
  const confirmIndex = wiringSource.indexOf("await confirmHtmlInApp(");
  const applyIndex = wiringSource.indexOf("await ApplyNamingNormalization(");
  assert.ok(confirmIndex >= 0, "缺少低置信确认弹窗");
  assert.ok(applyIndex > confirmIndex, "确认必须先于执行");
  assert.match(wiringSource, /modals\.batchNaming\.pendingConfirmTitle/);
  assert.match(wiringSource, /modals\.batchNaming\.pendingConfirmBody/);
  // 结果：逐项状态 + 汇总 + 撤销清单路径；剪贴板带 Wails 回退。
  assert.match(wiringSource, /modals\.batchNaming\.rowOk/);
  assert.match(wiringSource, /modals\.batchNaming\.rowFailed/);
  assert.match(wiringSource, /modals\.batchNaming\.backupNote/);
  assert.match(wiringSource, /window\.go\?\.app\?\.App\?\.ClipboardSetText/);
  assert.match(wiringSource, /serializePlanPayload\(/);
  assert.match(wiringSource, /serializeUndoPayload\(/);
  // 执行后刷新列表并清空选择，避免旧路径选择态残留。
  assert.match(wiringSource, /await refreshFilesKeepFilter\(\)/);
  assert.match(wiringSource, /deselectAll\(\)/);
});

test("入口接线：右键批量菜单 + 工具栏下拉 + 运行时初始化 + Esc 注册表", () => {
  assert.match(menuSource, /import \{ openBatchNamingModal \} from "\.\/batch-naming\.js"/);
  assert.match(menuSource, /t\("mods\.batch\.normalizeNames"\), iconSvg\("edit"\), \(\) => openBatchNamingModal\(\)/);
  assert.match(runtimeSource, /setupBatchNamingListeners\(\);/);
  assert.match(runtimeSource, /openBatchNamingModal/);
  assert.match(runtimeSource, /"batch-naming-modal",/, "要注册浮动能力");
  assert.match(escapeSource, /"batch-naming-modal",/, "要进 Esc 关闭注册表");
});

test("i18n 目录：界面文案 zh 逐字 + en 对齐 + 动态 key 全存在", () => {
  assert.equal(zhCatalog.mods.batch.normalizeNames, "批量规范化命名");
  assert.equal(zhCatalog.modals.batchNaming.title, "批量规范化命名");
  assert.equal(zhCatalog.modals.batchNaming.apply, "执行改名");
  assert.equal(zhCatalog.modals.batchNaming.selectAll, "全选");
  assert.equal(
    zhCatalog.modals.batchNaming.skip.workshop,
    "工坊文件不改名（Steam 需要原始文件名）",
  );
  assert.equal(zhCatalog.modals.batchNaming.skip.collision, "目标名已存在或与其它项目标重名");
  assert.equal(zhCatalog.modals.naming.reason.alreadySuffix, "；已符合规范");

  // 弹窗全部键在 en 侧齐平（含动态拼装的 skip.* —— 它们只能靠这里兜住）。
  const skipCodes = ["workshop", "unchanged", "unfittable", "noStem", "collision", "tooLong", "invalid", "readError"];
  for (const code of skipCodes) {
    assert.equal(typeof enCatalog.modals.batchNaming.skip[code], "string", `en 缺 skip.${code}`);
    assert.equal(typeof enCatalog.modals.batchNaming.skip[code], "string");
  }
  for (const key of Object.keys(zhCatalog.modals.batchNaming)) {
    if (key === "skip") continue;
    assert.equal(typeof enCatalog.modals.batchNaming[key], "string", `en 缺 batchNaming.${key}`);
  }
  for (const group of ["reason", "stage", "domain", "kind"]) {
    for (const key of Object.keys(zhCatalog.modals.naming[group])) {
      assert.equal(typeof enCatalog.modals.naming[group][key], "string", `en 缺 naming.${group}.${key}`);
    }
  }
});

test("wailsjs 绑定三件套包含批量方法与模型（防重生成漂移）", () => {
  assert.match(appBindings, /export function PlanNamingNormalization\(arg1\) \{\s*return window\['go'\]\['app'\]\['App'\]\['PlanNamingNormalization'\]\(arg1\);\s*\}/);
  assert.match(appBindings, /export function ApplyNamingNormalization\(arg1\) \{\s*return window\['go'\]\['app'\]\['App'\]\['ApplyNamingNormalization'\]\(arg1\);\s*\}/);
  assert.match(appTypes, /export function PlanNamingNormalization\(arg1:Array<string>\):Promise<app\.NamingBatchPlan>;/);
  assert.match(
    appTypes,
    /export function ApplyNamingNormalization\(arg1:Array<app\.NamingBatchRenameRequest>\):Promise<app\.NamingBatchApplyResult>;/,
  );
  assert.match(modelsTs, /export class NamingBatchPlan \{/);
  assert.match(modelsTs, /export class NamingBatchItem \{/);
  assert.match(modelsTs, /export class NamingBatchRenameRequest \{/);
  assert.match(modelsTs, /export class NamingBatchApplyItem \{/);
  assert.match(modelsTs, /export class NamingBatchApplyResult \{/);
  // NamingSuggestion 新增的 i18n 依据字段（重命名弹窗备注也依赖它）。
  assert.match(modelsTs, /reasonKey: string;/);
  assert.match(modelsTs, /reasonArgs\?: Record<string, string>;/);
});
