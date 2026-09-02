import assert from "node:assert/strict";
import test from "node:test";

import { getUnrecordedGameStateOptions } from "./unrecorded-game-state.mjs";

test("未记录的根目录 Mod 提供游戏内关闭、游戏内启用和禁用三种选项", () => {
  const options = getUnrecordedGameStateOptions({ location: "root" });

  assert.deepEqual(
    options.map((option) => option.id),
    ["game-disabled", "game-enabled", "disabled"],
  );
  assert.deepEqual(
    options.map((option) => option.label),
    ["游戏内关闭", "游戏内启用", "禁用"],
  );
  assert.ok(options.every((option) => option.disabled === false));
});

test("未记录的 workshop Mod 保留游戏开关选项，但禁用动作明确不可用", () => {
  const options = getUnrecordedGameStateOptions({ location: "workshop" });
  const disableOption = options.find((option) => option.id === "disabled");

  assert.ok(disableOption);
  assert.equal(disableOption.disabled, true);
  assert.match(disableOption.disabledReason, /先复制到 addons/);
});
