import assert from "node:assert/strict";
import test from "node:test";

import { addonListKeyForDetail } from "./detail-ignore-key.mjs";

test("addonListKeyForDetail maps root, workshop and disabled locations", () => {
  assert.equal(addonListKeyForDetail({ name: "MyMod.vpk", location: "root" }), "mymod.vpk");
  assert.equal(
    addonListKeyForDetail({ name: "123456.vpk", location: "workshop" }),
    "workshop\\123456.vpk",
  );
  assert.equal(addonListKeyForDetail({ name: "Old.vpk", location: "disabled" }), "old.vpk");
});

test("addonListKeyForDetail normalizes separators and leading dot segments", () => {
  assert.equal(
    addonListKeyForDetail({ name: "nested\\Sub/File.VPK", location: "root" }),
    "nested\\sub\\file.vpk",
  );
  assert.equal(addonListKeyForDetail({ name: ".\\Legacy.vpk", location: "root" }), "legacy.vpk");
  assert.equal(addonListKeyForDetail({ name: "", location: "root" }), "");
  assert.equal(addonListKeyForDetail(null), "");
});

test("addonListKeyForDetail strips a duplicated disabled prefix", () => {
  assert.equal(addonListKeyForDetail({ name: "disabled\\x.vpk", location: "disabled" }), "x.vpk");
});
