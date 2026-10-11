// VPK 内容预览的音频分支：目录文案 + 源码接线（kind=audio → 浏览器原生播放器）+ 样式。
import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import { t } from "../../core/i18n-runtime.js";

installTestCatalog();

const previewSource = readFileSync(new URL("./vpk-preview.js", import.meta.url), "utf8");
const cssSource = readFileSync(new URL("../../../css/app/diagnostics.css", import.meta.url), "utf8");

test("音频兜底文案与卡片描述（真实目录）", () => {
  assert.match(t("toolbox.vpkPreview.audioFallback"), /浏览器不支持播放/);
  assert.match(t("toolbox.card.vpkPreview.description"), /音频/);
});

test("kind=audio 时用浏览器原生播放器（不写盘、不解包）", () => {
  assert.match(previewSource, /result\.kind === "audio" && result\.dataUrl/);
  assert.match(previewSource, /document\.createElement\("audio"\)/);
  assert.match(previewSource, /audio\.controls = true/);
  assert.match(previewSource, /audio\.src = result\.dataUrl/);
  assert.match(previewSource, /audio\.className = "vpk-preview-audio"/);
  assert.match(previewSource, /t\("toolbox\.vpkPreview\.audioFallback"\)/);
});

test("音频样式已接线", () => {
  assert.match(cssSource, /\.vpk-preview-audio \{/);
});
