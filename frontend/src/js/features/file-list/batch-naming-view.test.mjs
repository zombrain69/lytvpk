// 批量规范化命名（路线图第 20 项 P2）纯逻辑测试：
//   - batch-naming-view.mjs：计划分组 / 默认勾选 / 执行请求原样回传 / 剪贴板载荷；
//   - naming-display.mjs：依据与跳过原因的本地化映射，以及 slug → catalog 键的存在性。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import {
  buildApplyRequests,
  defaultCheckedPaths,
  selectablePaths,
  serializePlanPayload,
  serializeUndoPayload,
  splitPlanItems,
} from "./batch-naming-view.mjs";
import {
  DOMAIN_SLUGS,
  KIND_SLUGS,
  STAGE_SLUGS,
  domainLabelKey,
  kindLabelKey,
  localizeReason,
  stageLabelKey,
} from "./naming-display.mjs";

const zhCatalog = JSON.parse(readFileSync(new URL("../../../messages/zh-CN.json", import.meta.url), "utf8"));
const enCatalog = JSON.parse(readFileSync(new URL("../../../messages/en.json", import.meta.url), "utf8"));

function catalogValue(catalog, key) {
  return key.split(".").reduce((node, part) => (node ? node[part] : undefined), catalog);
}

test("splitPlanItems 按 可改名 / 待复核 / 跳过 分组", () => {
  const plan = {
    items: [
      { path: "a", actionable: true, pendingReview: false },
      { path: "b", actionable: true, pendingReview: true },
      { path: "c", actionable: false, skipReason: "workshop" },
      { path: "d", actionable: true, pendingReview: false },
    ],
  };
  const groups = splitPlanItems(plan);
  assert.deepEqual(groups.actionable.map((item) => item.path), ["a", "d"]);
  assert.deepEqual(groups.pending.map((item) => item.path), ["b"]);
  assert.deepEqual(groups.skipped.map((item) => item.path), ["c"]);
  assert.deepEqual(splitPlanItems(null), { actionable: [], pending: [], skipped: [] });
});

test("默认勾选与可选集合：低置信默认不勾", () => {
  const plan = {
    items: [
      { path: "a", actionable: true, pendingReview: false },
      { path: "b", actionable: true, pendingReview: true },
      { path: "c", actionable: false },
    ],
  };
  assert.deepEqual(defaultCheckedPaths(plan), ["a"]);
  assert.deepEqual(selectablePaths(plan), ["a", "b"]);
});

test("buildApplyRequests 原样回传勾选项的 path/oldName/newName", () => {
  const plan = {
    items: [
      { path: "C:\\addons\\A.vpk", oldName: "A.vpk", newName: "生还者-Coach-A.vpk", actionable: true },
      { path: "C:\\addons\\B.vpk", oldName: "B.vpk", newName: "地图-B.vpk", actionable: true },
      { path: "C:\\addons\\C.vpk", oldName: "C.vpk", newName: "地图-C.vpk", actionable: false },
      { path: "C:\\addons\\D.vpk", oldName: "D.vpk", newName: "", actionable: true },
    ],
  };
  const requests = buildApplyRequests(plan, ["C:\\addons\\A.vpk", "C:\\addons\\C.vpk", "C:\\addons\\D.vpk"]);
  assert.deepEqual(requests, [
    { path: "C:\\addons\\A.vpk", oldName: "A.vpk", newName: "生还者-Coach-A.vpk" },
  ]);
});

test("serializePlanPayload 只含可执行项，含新旧名对照", () => {
  const plan = {
    items: [
      { path: "a", oldName: "A.vpk", newName: "地图-A.vpk", stage: "S1", pendingReview: false, actionable: true },
      { path: "b", oldName: "B.vpk", newName: "", actionable: false, skipReason: "workshop" },
    ],
  };
  const text = serializePlanPayload(plan, "2026-10-13T00:00:00Z");
  const payload = JSON.parse(text);
  assert.equal(payload.kind, "lytvpk-naming-normalization-plan");
  assert.equal(payload.createdAt, "2026-10-13T00:00:00Z");
  assert.equal(payload.items.length, 1);
  assert.deepEqual(payload.items[0], {
    path: "a",
    oldName: "A.vpk",
    newName: "地图-A.vpk",
    stage: "S1",
    pendingReview: false,
  });
});

test("serializeUndoPayload 只含成功项：新路径 → 原路径", () => {
  const result = {
    backupPath: "C:\\config\\naming_rename_backup_1.json",
    items: [
      { path: "C:\\addons\\A.vpk", newPath: "C:\\addons\\生还者-Coach-A.vpk", ok: true },
      { path: "C:\\addons\\B.vpk", ok: false, error: "目标文件已存在" },
    ],
  };
  const payload = JSON.parse(serializeUndoPayload(result, "2026-10-13T00:00:00Z"));
  assert.equal(payload.kind, "lytvpk-naming-rename-undo");
  assert.equal(payload.backupPath, "C:\\config\\naming_rename_backup_1.json");
  assert.deepEqual(payload.items, [
    { oldPath: "C:\\addons\\A.vpk", newPath: "C:\\addons\\生还者-Coach-A.vpk" },
  ]);
});

test("localizeReason：domain / kind 走目录词，alreadyNormalized 追加后缀，缺 key 回退中文", () => {
  const calls = [];
  const translate = (key, params) => {
    calls.push({ key, params });
    return key === "modals.naming.domain.survivor" ? "Survivor" : key;
  };

  const text = localizeReason(
    {
      reasonKey: "modals.naming.reason.domain",
      reasonArgs: { domain: "生还者", percent: "32" },
      reason: "S2 物件域=生还者（占比 32%）",
    },
    translate,
  );
  assert.equal(text, "modals.naming.reason.domain");
  assert.deepEqual(calls[0], { key: "modals.naming.domain.survivor", params: undefined });
  assert.deepEqual(calls[1].params, { domain: "Survivor", percent: "32" });

  calls.length = 0;
  const kindText = localizeReason(
    { reasonKey: "modals.naming.reason.resource", reasonArgs: { kind: "音效" } },
    translate,
  );
  assert.equal(kindText, "modals.naming.reason.resource");
  assert.equal(calls[0].key, "modals.naming.kind.sound");
  assert.equal(calls[1].params.kind, "modals.naming.kind.sound");

  calls.length = 0;
  const normalized = localizeReason(
    { reasonKey: "modals.naming.reason.map", alreadyNormalized: true },
    translate,
  );
  assert.equal(normalized, "modals.naming.reason.mapmodals.naming.reason.alreadySuffix");
  assert.equal(calls[1].key, "modals.naming.reason.alreadySuffix");

  assert.equal(localizeReason({ reason: "中文兜底" }, translate), "中文兜底");
  assert.equal(localizeReason({}, translate), "");
});

test("未知 domain / kind 词保持原文，不丢信息", () => {
  const captured = [];
  const translate = (key, params) => {
    captured.push(params);
    return "X";
  };
  localizeReason({ reasonKey: "modals.naming.reason.domain", reasonArgs: { domain: "未知域" } }, translate);
  assert.equal(captured[0].domain, "未知域");
});

test("slug 映射与 catalog 键：每个词条 zh / en 都有对应文案", () => {
  for (const slug of Object.values(STAGE_SLUGS)) {
    const key = `modals.naming.stage.${slug}`;
    assert.equal(typeof catalogValue(zhCatalog, key), "string", `zh 缺 ${key}`);
    assert.equal(typeof catalogValue(enCatalog, key), "string", `en 缺 ${key}`);
  }
  for (const slug of Object.values(DOMAIN_SLUGS)) {
    const key = `modals.naming.domain.${slug}`;
    assert.equal(typeof catalogValue(zhCatalog, key), "string", `zh 缺 ${key}`);
    assert.equal(typeof catalogValue(enCatalog, key), "string", `en 缺 ${key}`);
  }
  for (const slug of Object.values(KIND_SLUGS)) {
    const key = `modals.naming.kind.${slug}`;
    assert.equal(typeof catalogValue(zhCatalog, key), "string", `zh 缺 ${key}`);
    assert.equal(typeof catalogValue(enCatalog, key), "string", `en 缺 ${key}`);
  }
  assert.equal(stageLabelKey("S2"), "modals.naming.stage.s2");
  assert.equal(stageLabelKey("专项"), "modals.naming.stage.specialist");
  assert.equal(stageLabelKey("unknown"), "");
  assert.equal(domainLabelKey("生还者"), "modals.naming.domain.survivor");
  assert.equal(domainLabelKey("unknown"), "");
  assert.equal(kindLabelKey("音效"), "modals.naming.kind.sound");
  assert.equal(kindLabelKey("unknown"), "");
});

test("ReasonKey 分支 —— 前端覆盖后端全部 reason key（防单边新增漂移）", () => {
  const reasonKeys = Object.keys(zhCatalog.modals.naming.reason);
  for (const key of reasonKeys) {
    const full = `modals.naming.reason.${key}`;
    assert.equal(typeof catalogValue(enCatalog, full), "string", `en 缺 ${full}`);
  }
  // 后端 Go 侧常量齐平（源码级检查，防只加了目录没加分支）。
  const goSource = readFileSync(new URL("../../../../../internal/parser/naming_rules.go", import.meta.url), "utf8");
  for (const key of reasonKeys) {
    if (key === "alreadySuffix") continue;
    assert.match(goSource, new RegExp(`"modals\\.naming\\.reason\\.${key}"`), `Go 常量缺 ${key}`);
  }
});
