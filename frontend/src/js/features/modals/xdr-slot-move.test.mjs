// XDR「移到空槽」前端测试：纯展示逻辑（真实目录文案）+ 源码接线双校验。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import { t } from "../../core/i18n-runtime.js";
import {
  buildSlotOptionLabel,
  formatMoveNotice,
  splitMoveFiles,
  summarizeMoveResult,
} from "./xdr-slot-move-view.mjs";

installTestCatalog();

const detailSource = readFileSync(new URL("./detail.js", import.meta.url), "utf8");
const dialogSource = readFileSync(new URL("./xdr-slot-move.js", import.meta.url), "utf8");
const viewSource = readFileSync(new URL("./xdr-slot-move-view.mjs", import.meta.url), "utf8");
const appJs = readFileSync(new URL("../../../../wailsjs/go/app/App.js", import.meta.url), "utf8");
const appDts = readFileSync(new URL("../../../../wailsjs/go/app/App.d.ts", import.meta.url), "utf8");
const modelsTs = readFileSync(new URL("../../../../wailsjs/go/models.ts", import.meta.url), "utf8");

test("槽位下拉文案带官方建议用途（复用详情面板的组标签）", () => {
  const label = buildSlotOptionLabel(
    { slot: 44, slotLabel: "044", slotName: "其它（44–47）", slotGroup: "其它" },
    t,
  );
  assert.match(label, /^044 · 官方建议：其它（44–47）/);

  const heal = buildSlotOptionLabel(
    { slot: 32, slotLabel: "032", slotName: "Heal_self", slotGroup: "救援/治疗" },
    t,
  );
  assert.equal(heal, "032 · 官方建议：Heal_self（救援/治疗）");

  // 没有建议用途的槽位（防御分支）只显示编号。
  const plain = buildSlotOptionLabel({ slot: 7, slotLabel: "007" }, t);
  assert.equal(plain, "007");
});

test("summarizeMoveResult 折叠出改名数 / 失败 / 跳过", () => {
  const info = summarizeMoveResult({
    renamedFiles: 3,
    processedFiles: [
      { path: "a.vpk", renamedCount: 2 },
      { path: "b.vpk", renamedCount: 1 },
      { path: "c.vpk", skipped: true },
      { path: "d.vpk", error: "文件被占用" },
    ],
  });
  assert.equal(info.renamedFiles, 3);
  assert.equal(info.processedCount, 4);
  assert.equal(info.skipped, 1);
  assert.deepEqual(info.failures, [{ path: "d.vpk", error: "文件被占用" }]);

  const empty = summarizeMoveResult(null);
  assert.deepEqual(empty, { renamedFiles: 0, processedCount: 0, skipped: 0, failures: [] });
});

test("formatMoveNotice 生成成功 / 部分完成 / 全失败提示", () => {
  const ok = formatMoveNotice({ renamedFiles: 4, fromSlot: 2, toSlot: 44, processedFiles: [] }, t);
  assert.equal(ok.level, "success");
  assert.match(ok.message, /已移到空槽：4 个文件/);
  assert.match(ok.message, /slot 002 → 044/);

  const withSkip = formatMoveNotice(
    { renamedFiles: 2, fromSlot: 2, toSlot: 1, processedFiles: [{ path: "a.vpk", skipped: true }] },
    t,
  );
  assert.equal(withSkip.level, "success");
  assert.match(withSkip.message, /1 个副本不含该槽位/);

  const partial = formatMoveNotice(
    {
      renamedFiles: 2,
      fromSlot: 2,
      toSlot: 1,
      processedFiles: [{ path: "b.vpk", error: "文件被占用" }],
    },
    t,
  );
  assert.equal(partial.level, "warning");
  assert.match(partial.message, /部分完成：2 个文件已改名、1 个失败/);
  assert.match(partial.message, /文件被占用/);

  const failed = formatMoveNotice(
    { renamedFiles: 0, fromSlot: 2, toSlot: 1, processedFiles: [{ path: "b.vpk", error: "写不进去" }] },
    t,
  );
  assert.equal(failed.level, "error");
  assert.equal(failed.message, "写不进去");

  const noop = formatMoveNotice({ renamedFiles: 0, fromSlot: 2, toSlot: 1, processedFiles: [] }, t);
  assert.equal(noop.level, "info");
  assert.equal(noop.message, "没有文件被改动");
});

test("splitMoveFiles 折叠长清单", () => {
  const files = Array.from({ length: 30 }, (_, i) => ({ from: `f${i}`, to: `t${i}` }));
  const { visible, hiddenCount } = splitMoveFiles(files, 24);
  assert.equal(visible.length, 24);
  assert.equal(hiddenCount, 6);
  const single = splitMoveFiles(files, 0);
  assert.equal(single.visible.length, 24, "非法 limit 退回默认上限");
});

test("detail.js 在随机行上接线「移到空槽」按钮", () => {
  assert.match(detailSource, /import \{ openXDRSlotMoveDialog \} from "\.\/xdr-slot-move\.js"/);
  assert.match(detailSource, /data-xdr-move="1"/);
  assert.match(detailSource, /status\?\.state === "random"/, "只有同槽冲突行给入口");
  assert.match(detailSource, /wireXDRSlotMoveButtons\(subjectElement, file\)/);
  assert.match(detailSource, /openXDRSlotMoveDialog\(\{/);
  assert.match(detailSource, /fromSlot: slot/);
});

test("xdr-slot-move.js 走绑定 + 消息弹窗 + 列表刷新（并遵守无 innerHTML / 无原生弹窗约定）", () => {
  assert.match(dialogSource, /PreviewXDRSlotMove,/);
  assert.match(dialogSource, /MoveXDRModToSlot,/);
  assert.match(dialogSource, /from "\.\.\/\.\.\/\.\.\/\.\.\/wailsjs\/go\/app\/App"/);
  assert.match(dialogSource, /beginMessageModalSession/);
  assert.match(dialogSource, /refreshFilesKeepFilter\(\{ silent: true \}\)/);
  assert.equal(/innerHTML/.test(dialogSource), false, "新模块不允许 innerHTML");
  assert.equal(/window\.confirm|window\.prompt/.test(dialogSource), false, "不允许原生弹窗");
  assert.match(viewSource, /modals\.xdrMove\.optionLabel/);
});

test("wailsjs 绑定已同步 PreviewXDRSlotMove / MoveXDRModToSlot", () => {
  assert.match(appJs, /export function PreviewXDRSlotMove\(arg1, arg2, arg3, arg4\)/);
  assert.match(appJs, /export function MoveXDRModToSlot\(arg1, arg2, arg3, arg4\)/);
  assert.match(appJs, /window\['go'\]\['app'\]\['App'\]\['PreviewXDRSlotMove'\]/);
  assert.match(appDts, /PreviewXDRSlotMove\(arg1:string,arg2:string,arg3:number,arg4:number\):Promise<app\.XDRSlotMovePreview>/);
  assert.match(appDts, /MoveXDRModToSlot\(arg1:string,arg2:string,arg3:number,arg4:number\):Promise<app\.XDRSlotMoveResult>/);
  for (const name of ["XDRSlotMoveFile", "XDRSlotMoveFileResult", "XDRSlotMoveOption", "XDRSlotMovePreview", "XDRSlotMoveResult"]) {
    assert.match(modelsTs, new RegExp(`export class ${name} \\{`));
  }
});
