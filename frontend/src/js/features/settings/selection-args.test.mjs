import assert from "node:assert/strict";
import { test } from "node:test";

import {
  buildDependencyArgs,
  buildGroupMembersFromSelection,
} from "./selection-args.mjs";

test("buildGroupMembersFromSelection normalizes the selected paths", () => {
  assert.deepEqual(buildGroupMembersFromSelection(["b.vpk", " a.vpk ", "", "b.vpk"]), [
    "b.vpk",
    "a.vpk",
  ]);
  assert.deepEqual(buildGroupMembersFromSelection(undefined), []);
  assert.deepEqual(buildGroupMembersFromSelection([null, 42]), ["42"]);
});

test("buildDependencyArgs removes the chosen target from the dependencies", () => {
  assert.deepEqual(buildDependencyArgs(["a.vpk", "b.vpk", "c.vpk"], "a.vpk"), {
    target: "a.vpk",
    dependencies: ["b.vpk", "c.vpk"],
  });
  assert.deepEqual(buildDependencyArgs(["a.vpk", "b.vpk"], ""), { target: "", dependencies: [] });
  // 目标不在选择里时，所有选中项都作为依赖。
  assert.deepEqual(buildDependencyArgs(["a.vpk"], "z.vpk"), {
    target: "z.vpk",
    dependencies: ["a.vpk"],
  });
});
