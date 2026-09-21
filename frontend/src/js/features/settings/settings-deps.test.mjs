import assert from "node:assert/strict";
import test from "node:test";

import { buildSettingsDeps } from "./settings-deps.mjs";

test("buildSettingsDeps keeps injected bindings that are not overridden", () => {
  const injected = {
    appState: {},
    SetModStrategyGroupTier: () => "tier",
    ListModEnableProfiles: () => [],
  };
  const deps = buildSettingsDeps(injected, { appState: { a: 1 }, showNotification: () => {} });

  assert.equal(typeof deps.SetModStrategyGroupTier, "function");
  assert.equal(deps.SetModStrategyGroupTier(), "tier");
  assert.equal(typeof deps.ListModEnableProfiles, "function");
  assert.deepEqual(deps.appState, { a: 1 });
});

test("buildSettingsDeps does not let undefined overrides mask injected bindings", () => {
  const injected = { SetModStrategyGroupTier: () => "tier" };
  const deps = buildSettingsDeps(injected, { SetModStrategyGroupTier: undefined });

  assert.equal(typeof deps.SetModStrategyGroupTier, "function");
});

test("buildSettingsDeps tolerates missing or invalid input", () => {
  assert.deepEqual(buildSettingsDeps(null, null), {});
  assert.deepEqual(buildSettingsDeps(undefined, { a: 1 }), { a: 1 });
});
