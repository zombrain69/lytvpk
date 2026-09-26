import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// 阅读舒适度：设置页要有入口、要真的写配置、CSS 要有对应的颜色档位，
// 启动时还要恢复（否则"设置完重启就丢"）。

const settingsSource = readFileSync(new URL("./settings-page.js", import.meta.url), "utf8");
const runtimeSource = readFileSync(new URL("../app-runtime.js", import.meta.url), "utf8");
const cssSource = readFileSync(new URL("../../../css/app/reading-comfort.css", import.meta.url), "utf8");
const mainCssSource = readFileSync(new URL("../../../css/main.css", import.meta.url), "utf8");

test("设置页有文字大小与阅读舒适度两组单选", () => {
  assert.match(settingsSource, /id="settings-text-size"/, "缺少文字大小分组");
  assert.match(settingsSource, /id="settings-reading-comfort"/, "缺少阅读舒适度分组");
  assert.match(settingsSource, /TEXT_SIZE_OPTIONS\.map/, "文字大小档位应由选项表生成");
  assert.match(settingsSource, /READING_COMFORT_OPTIONS\.map/, "舒适度档位应由选项表生成");
  // 说明文案要讲清"只改文字、不动布局"和"行距/对比度"。
  assert.match(settingsSource, /只改文字大小、不动布局/);
  assert.match(settingsSource, /调整行距与文字对比度/);
});

test("改档位会写回配置并立刻生效", () => {
  assert.match(settingsSource, /applyReadingComfort\(\{/, "要调用 applyReadingComfort");
  assert.match(settingsSource, /config\.textSize = applied\.textSize/, "文字大小要写回配置");
  assert.match(settingsSource, /config\.readingComfort = applied\.comfort/, "舒适度要写回配置");
  assert.match(settingsSource, /await deps\.saveConfig\(config\)/, "要落盘保存");
  assert.match(settingsSource, /settings-text-size input\[name='settings-text-size'\]/, "要绑定单选事件");
  assert.match(settingsSource, /settings-reading-comfort input\[name='settings-reading-comfort'\]/);
});

test("启动时恢复阅读设置，且与界面缩放相乘", () => {
  assert.match(runtimeSource, /applyReadingComfort\(\{/, "启动要恢复阅读设置");
  assert.match(runtimeSource, /textSize: getConfig\(\)\.textSize/, "要读配置里的文字大小");
  assert.match(runtimeSource, /comfort: getConfig\(\)\.readingComfort/, "要读配置里的舒适度");
  // 顺序：先设阅读档位（写 --reading-font-scale），再 applyUIScale 合成根字号。
  const comfortIndex = runtimeSource.indexOf("applyReadingComfort({");
  const scaleIndex = runtimeSource.indexOf("applyUIScale(getConfig().uiScale)");
  assert.ok(comfortIndex > 0 && scaleIndex > comfortIndex, "先应用阅读档位再应用整体缩放");
});

test("CSS 提供行距与两套主题下的对比度档位", () => {
  assert.match(mainCssSource, /reading-comfort\.css/, "main.css 要引入阅读舒适度样式");
  assert.match(cssSource, /--reading-line-height/, "要支持行距变量");
  assert.match(cssSource, /html\[data-reading-comfort="soft"\]/, "明亮主题的柔和档");
  assert.match(cssSource, /html\[data-reading-comfort="contrast"\]/, "明亮主题的高对比档");
  assert.match(cssSource, /html\.dark-mode\[data-reading-comfort="soft"\]/, "暗色主题的柔和档");
  assert.match(cssSource, /html\.dark-mode\[data-reading-comfort="contrast"\]/, "暗色主题的高对比档");
});
