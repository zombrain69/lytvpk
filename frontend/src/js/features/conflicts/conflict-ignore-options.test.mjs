import assert from "node:assert/strict";
import { test } from "node:test";

import { formatConflictIgnoreList, parseConflictIgnoreList } from "./conflict-ignore-options.mjs";

test("parseConflictIgnoreList normalizes paths, skips comments and de-duplicates", () => {
  const parsed = parseConflictIgnoreList(
    [
      " Materials/Shared.VTF ",
      "",
      "# 说明行",
      "// 另一种注释",
      "scripts\\vscripts\\",
      "scripts/vscripts/",
      "Materials/Shared.VTF",
    ].join("\r\n"),
  );

  assert.deepEqual(parsed, ["materials/shared.vtf", "scripts/vscripts/"]);
});

test("parseConflictIgnoreList tolerates empty and non-string input", () => {
  assert.deepEqual(parseConflictIgnoreList(""), []);
  assert.deepEqual(parseConflictIgnoreList(undefined), []);
  assert.deepEqual(parseConflictIgnoreList(null), []);
});

test("formatConflictIgnoreList renders stored entries one per line", () => {
  assert.equal(formatConflictIgnoreList([]), "");
  assert.equal(formatConflictIgnoreList(undefined), "");
  assert.equal(
    formatConflictIgnoreList(["materials/a.vtf", "MATERIALS/A.VTF", "scripts/"]),
    "materials/a.vtf\nscripts/",
  );
});
