import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";

import {
  buildTagEvidenceRows,
  EVIDENCE_LEVEL_LABELS,
  TAG_EVIDENCE_DISPLAY_LIMIT,
} from "./tag-evidence-view.mjs";

test("buildTagEvidenceRows 把证据转成可渲染行", () => {
  const { rows, hiddenCount } = buildTagEvidenceRows([
    {
      tag: "AK47",
      rule: "entity:weapon_rifle_ak47",
      level: "exact",
      source: ["models/w_models/weapons/w_rifle_ak47.mdl"],
    },
    {
      tag: "步枪",
      rule: "weapon:步枪",
      level: "inferred",
      source: [],
    },
  ]);

  assert.equal(hiddenCount, 0);
  assert.equal(rows.length, 2);
  assert.deepEqual(rows[0], {
    tag: "AK47",
    level: "exact",
    levelLabel: EVIDENCE_LEVEL_LABELS.exact,
    detail: "entity:weapon_rifle_ak47 · models/w_models/weapons/w_rifle_ak47.mdl",
  });
  // 没有来源路径时只显示规则，不留多余分隔符
  assert.equal(rows[1].detail, "weapon:步枪");
});

test("buildTagEvidenceRows 对空输入与脏数据安全", () => {
  assert.deepEqual(buildTagEvidenceRows(null), { rows: [], hiddenCount: 0 });
  assert.deepEqual(buildTagEvidenceRows(undefined), { rows: [], hiddenCount: 0 });
  assert.deepEqual(buildTagEvidenceRows([]), { rows: [], hiddenCount: 0 });

  const { rows } = buildTagEvidenceRows([null, { tag: "  ", rule: "  " }, { tag: "贴图", source: "not-an-array" }]);
  assert.equal(rows.length, 1);
  assert.equal(rows[0].tag, "贴图");
  assert.equal(rows[0].detail, "");
});

test("buildTagEvidenceRows 受显示上限约束并报告剩余条数", () => {
  const evidence = Array.from({ length: TAG_EVIDENCE_DISPLAY_LIMIT + 3 }, (_, index) => ({
    tag: `标签${index}`,
    rule: `content:标签${index}`,
    level: "pattern",
    source: ["particles/x.pcf"],
  }));
  const { rows, hiddenCount } = buildTagEvidenceRows(evidence);
  assert.equal(rows.length, TAG_EVIDENCE_DISPLAY_LIMIT);
  assert.equal(hiddenCount, 3);
});

test("未知证据强度保留原文而不是显示空白", () => {
  const { rows } = buildTagEvidenceRows([{ tag: "X", rule: "r", level: "future-level" }]);
  assert.equal(rows[0].levelLabel, "future-level");
});

// 「标签依据」按需加载的接线：列表 IPC 不再带 tagEvidence（真机占 payload 的 35%），
// 详情弹窗必须自己取一次，否则这块功能会静默消失。
test("详情弹窗在列表没带依据时会按需调 GetModEvidence", () => {
  const here = path.dirname(fileURLToPath(import.meta.url));
  const source = readFileSync(path.join(here, "detail.js"), "utf8");

  assert.match(source, /import\s*\{[^}]*GetModEvidence[^}]*\}\s*from/, "详情要 import GetModEvidence");
  assert.match(
    source,
    /void loadTagEvidenceIfMissing\(file, detailTagsContainer\)/,
    "渲染详情时要触发按需加载",
  );
  assert.match(
    source,
    /async function loadTagEvidenceIfMissing\(file, anchor\)/,
    "缺少按需加载实现",
  );
  assert.match(
    source,
    /Array\.isArray\(file\.tagEvidence\) && file\.tagEvidence\.length > 0\) return/,
    "已经带了依据（例如老路径）就不要重复请求",
  );
  assert.match(
    source,
    /currentDetailFile\?\.path \|\| ""\) !== path\) return/,
    "结果回来时用户已经换/关了详情就丢弃（避免串到别的 Mod 上）",
  );
  assert.match(source, /console\.warn\("读取标签依据失败:", error\)/, "取不到时静默降级，不弹错误");
});
