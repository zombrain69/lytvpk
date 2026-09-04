import assert from "node:assert/strict";
import test from "node:test";

import { createAutoexecSettingsController } from "./autoexec-settings-controller.mjs";

test("reload replaces the controller metadata with the latest autoexec config", async () => {
  const initialInfo = { path: "C:/cfg/autoexec.cfg", content: "fps_max 60", exists: true };
  const latestInfo = { path: initialInfo.path, content: "fps_max 144", exists: true };
  const controller = createAutoexecSettingsController(initialInfo, {
    getConfig: async () => latestInfo,
  });

  const result = await controller.reload();

  assert.deepEqual(result.info, latestInfo);
  assert.deepEqual(controller.getInfo(), latestInfo);
});

test("save reports a post-save reload error separately from a successful write", async () => {
  const calls = [];
  const controller = createAutoexecSettingsController({ path: "C:/cfg/autoexec.cfg" }, {
    saveConfig: async (content) => {
      calls.push(content);
    },
    getConfig: async () => {
      throw new Error("读取状态失败");
    },
  });

  const result = await controller.save("fps_max 240\n");

  assert.deepEqual(calls, ["fps_max 240\n"]);
  assert.equal(result.saved, true);
  assert.equal(result.reloadError?.message, "读取状态失败");
  assert.equal(controller.getInfo().path, "C:/cfg/autoexec.cfg");
});
