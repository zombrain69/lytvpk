// spray 家族 worker 错误桥：vtf-core.js 被 spray-worker（工作线程）与主线程共用，
// worker 里没有语言目录、不能直接取 t() 文案 —— 抛错时统一携带 `code`（= i18n
// 目录 key），由主线程展示层（spray-tool.js 的 formatError）按前缀翻译。
// 这里锁三件事：
//   1) vtf-core 的三条帧数据校验真的抛「带 code」的错误（行为口径）；
//   2) 这些 code 在中文目录里能解析出真实文案（不是原样 key）；
//   3) worker 把 code 一并回传、展示层按前缀翻译（源码接线）。
import assert from "node:assert/strict";
import test from "node:test";
import { readFileSync } from "node:fs";

import { t } from "../../core/i18n-runtime.js";
import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import { buildVTFBytes } from "./vtf-core.js";

installTestCatalog();

const PREFIX = "toolbox.spray.error.";

test("vtf-core 帧数据校验抛带 i18n code 的错误", () => {
  assert.throws(
    () => buildVTFBytes({}),
    (error) => error?.code === `${PREFIX}noWriteFrames`,
  );
  assert.throws(
    () => buildVTFBytes({ mipmaps: [[]] }),
    (error) => error?.code === `${PREFIX}mipFramesMissing`,
  );
  assert.throws(
    () => buildVTFBytes({ mipmaps: [[{ width: 4, height: 4 }]] }),
    (error) => error?.code === `${PREFIX}framesIncomplete`,
  );
});

test("worker 错误 code 在中文目录里可解析", () => {
  for (const name of ["noWriteFrames", "mipFramesMissing", "framesIncomplete"]) {
    const key = `${PREFIX}${name}`;
    assert.notEqual(t(key), key, `目录缺少 ${key}`);
  }
});

test("worker 回传 code；展示层按前缀翻译", () => {
  const worker = readFileSync(new URL("./spray-worker.js", import.meta.url), "utf8");
  assert.match(worker, /code:\s*typeof error\?\.code/);
  const tool = readFileSync(new URL("./spray-tool.js", import.meta.url), "utf8");
  assert.match(tool, /code\.startsWith\("toolbox\.spray\."\)/);
});
