import assert from "node:assert/strict";
import test from "node:test";

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
