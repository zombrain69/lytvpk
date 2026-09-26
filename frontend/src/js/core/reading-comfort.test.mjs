import assert from "node:assert/strict";
import test from "node:test";

import {
  READING_COMFORT_OPTIONS,
  TEXT_SIZE_OPTIONS,
  applyReadingComfort,
  fontPercentFor,
  lineHeightFor,
  normalizeReadingComfort,
  normalizeTextSize,
  readingComfortLabel,
  readingFontScaleFromRoot,
  textScaleFor,
  textSizeLabel,
} from "./reading-comfort.mjs";

// 假根节点：只要 dataset / style 两个属性，够验证"写到哪儿、写成什么"。
function fakeRoot() {
  return {
    dataset: {},
    style: { fontSize: "", values: {}, setProperty(name, value) { this.values[name] = value; } },
  };
}

test("档位取值会被归一化，未知值回落到标准", () => {
  assert.equal(normalizeTextSize("LARGE"), "large");
  assert.equal(normalizeTextSize("特大"), "standard");
  assert.equal(normalizeTextSize(undefined), "standard");
  assert.equal(normalizeReadingComfort(" Contrast "), "contrast");
  assert.equal(normalizeReadingComfort("不存在"), "standard");
});

test("倍率与行高按档位变化，且单调", () => {
  const scales = TEXT_SIZE_OPTIONS.map((option) => option.scale);
  for (let index = 1; index < scales.length; index += 1) {
    assert.ok(scales[index] > scales[index - 1], "字号档位应递增");
  }
  const lines = READING_COMFORT_OPTIONS.map((option) => option.lineHeight);
  for (let index = 1; index < lines.length; index += 1) {
    assert.ok(lines[index] < lines[index - 1], "越靠后行距越紧（柔和 → 标准 → 高对比）");
  }
  assert.equal(textScaleFor("xlarge"), 1.25);
  assert.equal(lineHeightFor("soft"), 1.8);
  assert.equal(textSizeLabel("compact"), "紧凑");
  assert.equal(readingComfortLabel("contrast"), "高对比");
});

test("根字号 = 界面缩放 × 文字档位（两者互不覆盖）", () => {
  assert.equal(fontPercentFor(1, "standard"), 100);
  assert.equal(fontPercentFor(1.2, "large"), 134);
  assert.equal(fontPercentFor(0.8, "xlarge"), 100);
  // 非法缩放不产生 NaN。
  assert.equal(fontPercentFor(Number.NaN, "standard"), 100);
});

test("applyReadingComfort 写档位、行高与合成后的根字号", () => {
  const root = fakeRoot();
  const applied = applyReadingComfort({ textSize: "large", comfort: "soft", uiScale: 1.1 }, root);

  assert.deepEqual(applied, { textSize: "large", comfort: "soft" });
  assert.equal(root.dataset.textSize, "large");
  assert.equal(root.dataset.readingComfort, "soft");
  assert.equal(root.dataset.readingFontScale, "1.12");
  assert.equal(root.style.values["--reading-line-height"], "1.8");
  assert.equal(root.style.fontSize, "123%");
  // 供 ui-scale 合成时读取。
  assert.equal(readingFontScaleFromRoot(root), 1.12);
});

test("没有 document 时也不会抛错（纯函数可测）", () => {
  const applied = applyReadingComfort({ textSize: "huge", comfort: "weird" }, null);
  assert.deepEqual(applied, { textSize: "standard", comfort: "standard" });
});
