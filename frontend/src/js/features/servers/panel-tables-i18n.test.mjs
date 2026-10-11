// 服务器面板数据表 i18n（第 13 项第二十批）：
// 模式代码 / 官方战役 / 章节显示名进目录（`servers.panel.mode|campaign|chapter.*`）、
// 难度表值显分离（值=后端指令参数留在源码，显示名走 `servers.panel.difficulty.*`）。
// 断言口径：源码接线 + 目录内容双查。

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import { t } from "../../core/i18n-runtime.js";

const servers = readFileSync(new URL("./servers.js", import.meta.url), "utf8");
const panelModal = readFileSync(new URL("./panel-modal.js", import.meta.url), "utf8");
const catalog = JSON.parse(
  readFileSync(new URL("../../../messages/zh-CN.json", import.meta.url), "utf8"),
);
const catalogGet = (key) =>
  key.split(".").reduce((node, part) => (node == null ? undefined : node[part]), catalog);

installTestCatalog();

test("PANEL_MODE_LABEL_KEYS：7 个模式代码齐备，文案还原原表", () => {
  const expected = {
    coop: "合作",
    realism: "写实",
    versus: "对抗",
    survival: "生还",
    scavenge: "清道夫",
    mutation: "突变",
    mutations: "突变",
  };
  const pairs = [...servers.matchAll(/([a-z]+): "servers\.panel\.mode\.([a-z]+)"/g)];
  assert.equal(pairs.length, 7, "模式 key 表应有 7 项");
  for (const [, code, suffix] of pairs) {
    assert.equal(suffix, code, `key 后缀要与模式代码一致：${code}`);
    const key = `servers.panel.mode.${code}`;
    assert.equal(catalogGet(key), expected[code], `${key} 的文案要对齐目录`);
    assert.equal(t(key), expected[code], `${key} 要能通过运行时解析`);
  }
  assert.ok(!servers.includes("PANEL_MODE_LABELS"), "旧符号 PANEL_MODE_LABELS 应已改名");
  assert.match(panelModal, /PANEL_MODE_LABEL_KEYS\[normalized\]/, "getPanelModeLabel 要消费 key 表");
});

test("OFFICIAL_CAMPAIGNS：14 战役 / 57 章节 titleKey 可解析，code 保持原值", () => {
  const campaignTitles = [
    "死亡中心", "黑色狂欢节", "沼泽激战", "暴风骤雨", "教区", "短暂时刻", "牺牲",
    "毫不留情", "坠机险途", "死亡丧钟", "死亡机场", "血腥收获", "刺骨寒溪", "临死一搏",
  ];
  const campaignKeys = [
    ...servers.matchAll(/titleKey: "servers\.panel\.campaign\.c(\d+)"/g),
  ].map((match) => `servers.panel.campaign.c${match[1]}`);
  assert.equal(campaignKeys.length, 14, "战役数量应为 14");
  campaignKeys.forEach((key, index) => {
    assert.equal(key, `servers.panel.campaign.c${index + 1}`, "战役 key 顺序要连续");
    assert.equal(catalogGet(key), campaignTitles[index], `第 ${index + 1} 个战役文案要对齐`);
    assert.equal(t(key), campaignTitles[index], `${key} 要能通过运行时解析`);
  });

  const chapterPairs = [
    ...servers.matchAll(
      /\{ code: "([a-z0-9_]+)", titleKey: "servers\.panel\.chapter\.([A-Za-z0-9]+)" \}/g,
    ),
  ].map((match) => [match[1], match[2]]);
  assert.equal(chapterPairs.length, 57, "章节数量应为 57");
  const camel = (code) => code.replace(/_([a-z])/g, (_, ch) => ch.toUpperCase());
  const seenKeys = new Set();
  for (const [code, suffix] of chapterPairs) {
    assert.equal(suffix, camel(code), `章节 key 应由 code 机械派生：${code}`);
    const key = `servers.panel.chapter.${suffix}`;
    assert.ok(!seenKeys.has(key), `章节 key 不能重复：${key}`);
    seenKeys.add(key);
    assert.equal(typeof catalogGet(key), "string", `${key} 要存在于目录`);
    assert.notEqual(t(key), key, `${key} 要能通过运行时解析`);
  }
  // 显示名抽查（含跨战役重名：c6m3_port / c7m3_port 均为「港口」）
  assert.equal(catalogGet("servers.panel.chapter.c1m1Hotel"), "旅馆");
  assert.equal(catalogGet("servers.panel.chapter.c4m5MilltownEscape"), "逃离小镇");
  assert.equal(catalogGet("servers.panel.chapter.c6m3Port"), "港口");
  assert.equal(catalogGet("servers.panel.chapter.c7m3Port"), "港口");
  assert.equal(catalogGet("servers.panel.chapter.c13m3Memorialbridge"), "纪念桥");
  assert.equal(catalogGet("servers.panel.chapter.c14m2Lighthouse"), "灯塔");
  // code 是地图切换指令参数：抽查保持原值
  const codes = chapterPairs.map(([code]) => code);
  for (const code of ["c1m1_hotel", "c4m5_milltown_escape", "c13m2_southpinestream"]) {
    assert.ok(codes.includes(code), `${code} 要保持原值（后端指令参数）`);
  }
  // 消费方接线：官方表取文案、三方地图维持字面标题
  assert.match(panelModal, /t\(campaign\.titleKey\)/, "战役标题要走目录取值");
  assert.match(panelModal, /t\(chapter\.titleKey\)/, "章节标题要走目录取值");
});

test("难度表值显分离：值仍是后端指令参数，显示名进目录", () => {
  const expected = [
    ["简单", "servers.panel.difficulty.easy"],
    ["普通", "servers.panel.difficulty.normal"],
    ["高级", "servers.panel.difficulty.hard"],
    ["专家", "servers.panel.difficulty.impossible"],
  ];
  for (const [value, key] of expected) {
    assert.ok(
      panelModal.includes(`value: "${value}"`),
      `难度值 ${value} 要保留在源码（server_panel.go 校验的后端指令参数）`,
    );
    assert.equal(catalogGet(key), value, `${key} 的显示名要还原为 ${value}`);
    assert.equal(t(key), value, `${key} 要能通过运行时解析`);
  }
  assert.match(panelModal, /label\.textContent = t\(difficulty\.labelKey\)/, "按钮显示名要走目录");
  assert.match(
    panelModal,
    /item\.aliases\.includes\(normalized\)/,
    "难度别名表要由值表派生（不再重复维护中文别名）",
  );
  assert.match(
    panelModal,
    /difficulty: getPanelDifficultyLabel\(difficulty\)/,
    "切换通知的 {difficulty} 要走显示名",
  );
});
