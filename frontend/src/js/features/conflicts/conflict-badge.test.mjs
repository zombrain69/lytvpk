import assert from "node:assert/strict";
import test from "node:test";

import {
  buildConflictBadgeMap,
  conflictBadgeLevel,
  filePriorityKeys,
  formatConflictBadgeLabel,
  shouldShowConflictBadge,
} from "./conflict-badge.mjs";

test("formatConflictBadgeLabel reports conflicts and overrides separately", () => {
  assert.equal(formatConflictBadgeLabel({ conflictFiles: 2, overrideFiles: 0 }), "2 处冲突");
  assert.equal(formatConflictBadgeLabel({ conflictFiles: 0, overrideFiles: 3 }), "3 处覆盖");
  assert.equal(
    formatConflictBadgeLabel({ conflictFiles: 1, overrideFiles: 4 }),
    "1 处冲突 · 4 处覆盖",
  );
  assert.equal(formatConflictBadgeLabel(null), "");
  assert.equal(formatConflictBadgeLabel({}), "");
});

test("conflictBadgeLevel prefers conflicts and falls back to override", () => {
  assert.equal(conflictBadgeLevel({ conflictFiles: 1, severity: "critical" }), "critical");
  assert.equal(conflictBadgeLevel({ conflictFiles: 1, severity: "warning" }), "warning");
  assert.equal(conflictBadgeLevel({ conflictFiles: 1, severity: "info" }), "info");
  assert.equal(conflictBadgeLevel({ conflictFiles: 0, overrideFiles: 2, severity: "critical" }), "override");
  assert.equal(conflictBadgeLevel({ conflictFiles: 0, overrideFiles: 0 }), "none");
  assert.equal(shouldShowConflictBadge({ overrideFiles: 1 }), true);
  assert.equal(shouldShowConflictBadge({}), false);
});

test("buildConflictBadgeMap indexes by path and normalized key", () => {
  const { byPath, byKey } = buildConflictBadgeMap([
    { path: "C:\\addons\\a.vpk", key: "A.VPK", conflictFiles: 1, severity: "info" },
    { path: "C:\\addons\\workshop\\123.vpk", key: "workshop/123.vpk", conflictFiles: 0, overrideFiles: 2 },
  ]);
  assert.equal(byPath.get("C:\\addons\\a.vpk").conflictFiles, 1);
  assert.equal(byKey.get("a.vpk").conflictFiles, 1);
  assert.equal(byKey.get("workshop\\123.vpk").overrideFiles, 2);
  assert.equal(buildConflictBadgeMap(null).byPath.size, 0);
});

test("filePriorityKeys mirrors the Go-side addonlist key derivation", () => {
  assert.deepEqual(
    filePriorityKeys({ name: "a.vpk", path: "C:\\addons\\a.vpk", location: "root" }, "C:\\addons"),
    ["a.vpk"],
  );
  assert.deepEqual(
    filePriorityKeys(
      { name: "123.vpk", path: "C:\\addons\\workshop\\123.vpk", location: "workshop" },
      "C:\\addons",
    ),
    ["workshop\\123.vpk", "123.vpk"],
  );
  assert.deepEqual(
    filePriorityKeys({ name: "old.vpk", path: "C:\\addons\\disabled\\old.vpk", location: "disabled" }, "C:\\addons"),
    ["disabled\\old.vpk", "old.vpk"],
  );
});
