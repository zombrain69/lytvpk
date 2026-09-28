import assert from "node:assert/strict";
import test from "node:test";

import {
  buildScopedConflictSummary,
  buildScopedConflictTitle,
  formatScopedConflictLabel,
  matchesConflictBaseline,
} from "./scoped-conflict-summary.mjs";

function group(files, severity, paths) {
  return {
    file_count: files,
    severity,
    vpk_files: paths.map((path) => ({ path, name: path })),
  };
}

test("buildScopedConflictSummary 同时统计冲突组与覆盖组", () => {
  const summary = buildScopedConflictSummary({
    conflict_groups: [group(9, "warning", ["A", "B"])],
    override_groups: [
      { ...group(5, "info", ["A", "C"]), winner: { path: "C", name: "C" } },
      { ...group(4, "critical", ["A", "D"]), winner: { path: "D", name: "D" } },
    ],
  });

  assert.deepEqual(summary.get("A"), {
    severity: "critical",
    groups: 1,
    files: 9,
    overrides: 2,
    overrideFiles: 9,
    winners: ["C", "D"],
  });
  assert.deepEqual(summary.get("B"), {
    severity: "warning",
    groups: 1,
    files: 9,
    overrides: 0,
    overrideFiles: 0,
    winners: [],
  });
  assert.equal(summary.get("C").overrides, 1);
});

test("buildScopedConflictSummary 只统计覆盖组时也不丢结果", () => {
  const summary = buildScopedConflictSummary({
    conflict_groups: [],
    override_groups: [{ ...group(9, "warning", ["A", "B"]), winner: { path: "B", name: "B" } }],
  });
  assert.deepEqual(summary.get("A"), {
    severity: "warning",
    groups: 0,
    files: 0,
    overrides: 1,
    overrideFiles: 9,
    winners: ["B"],
  });
});

test("formatScopedConflictLabel 不会把已判定的覆盖说成什么都没有", () => {
  assert.equal(formatScopedConflictLabel(null), "无冲突");
  assert.equal(
    formatScopedConflictLabel(null, { matchedBaseline: false }),
    "未参与对比",
  );
  assert.equal(
    formatScopedConflictLabel({ groups: 2, files: 14, severity: "warning", overrides: 0, overrideFiles: 0 }),
    "冲突 2 组 · 14 文件 · 警告",
  );
  assert.equal(
    formatScopedConflictLabel({ groups: 0, files: 0, severity: "warning", overrides: 1, overrideFiles: 9 }),
    "无冲突 · 覆盖 9 处",
  );
  assert.equal(
    formatScopedConflictLabel({ groups: 2, files: 14, severity: "critical", overrides: 3, overrideFiles: 9 }),
    "冲突 2 组 · 覆盖 9 处 · 严重",
  );
});

test("buildScopedConflictTitle 说明冲突、覆盖与胜者", () => {
  const title = buildScopedConflictTitle(
    {
      groups: 1,
      files: 9,
      severity: "warning",
      overrides: 1,
      overrideFiles: 5,
      winners: ["tf 小红帽m1887 (1).vpk"],
    },
    { scopeLabel: "游戏内开启" },
  );
  assert.match(title, /冲突 1 组/);
  assert.match(title, /覆盖 1 组/);
  assert.match(title, /tf 小红帽m1887 \(1\)\.vpk/);
  assert.match(title, /游戏内开启/);
  assert.match(
    buildScopedConflictTitle(null, { scopeLabel: "游戏内开启" }),
    /未发现重叠文件/,
  );
  assert.match(
    buildScopedConflictTitle(null, { matchedBaseline: false, scopeLabel: "游戏内开启" }),
    /不满足对比范围/,
  );
});

test("matchesConflictBaseline 复刻后端基线规则", () => {
  const enabled = { location: "workshop", gameStateKnown: true, gameEnabled: true };
  assert.equal(matchesConflictBaseline(enabled, [{ type: "enabled" }], "or"), true);
  assert.equal(
    matchesConflictBaseline({ ...enabled, gameEnabled: false }, [{ type: "enabled" }], "or"),
    false,
  );
  assert.equal(
    matchesConflictBaseline({ ...enabled, location: "disabled" }, [{ type: "enabled" }], "or"),
    false,
  );
  assert.equal(
    matchesConflictBaseline({ location: "disabled", gameStateKnown: false }, [{ type: "not_disabled" }], "or"),
    false,
  );
  assert.equal(matchesConflictBaseline({ location: "root" }, [{ type: "root" }], "or"), true);
  assert.equal(matchesConflictBaseline({ location: "root" }, [{ type: "workshop" }], "or"), false);
  assert.equal(
    matchesConflictBaseline({ location: "disabled" }, [{ type: "root" }, { type: "workshop" }], "or"),
    false,
  );
  assert.equal(
    matchesConflictBaseline(everything(), [{ type: "enabled" }, { type: "workshop" }], "and"),
    true,
  );
  assert.equal(
    matchesConflictBaseline({ ...enabled, gameEnabled: false }, [{ type: "enabled" }, { type: "workshop" }], "and"),
    false,
  );
  assert.equal(
    matchesConflictBaseline({ ...enabled, primaryTag: "武器" }, [{ type: "tag", value: "武器" }], "or"),
    true,
  );
  assert.equal(
    matchesConflictBaseline({ ...enabled, secondaryTags: ["声音"] }, [{ type: "tag", value: "声音" }], "or"),
    true,
  );
  assert.equal(matchesConflictBaseline(enabled, [{ type: "tag", value: "" }], "or"), false);
  // 空规则 = 默认的游戏内开启基线。
  assert.equal(matchesConflictBaseline(enabled, [], "or"), true);
  assert.equal(matchesConflictBaseline({ location: "disabled" }, [], "or"), false);
});

function everything() {
  return {
    location: "workshop",
    gameStateKnown: true,
    gameEnabled: true,
    primaryTag: "武器",
    secondaryTags: ["声音"],
  };
}
