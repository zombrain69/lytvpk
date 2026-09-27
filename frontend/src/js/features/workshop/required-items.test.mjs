import assert from "node:assert/strict";
import test from "node:test";

import { buildRequiredItemsHtml, normalizeRequiredItems } from "./required-items.mjs";

test("normalizeRequiredItems 过滤无 ID 的脏数据", () => {
  const items = normalizeRequiredItems({
    required_items: [
      { publishedfileid: "111", title: "前置库", preview_url: "https://x/1.jpg", subscriptions: "5" },
      { publishedfileid: "   ", title: "没有 ID" },
      { title: "缺字段" },
      null,
    ],
  });

  assert.equal(items.length, 1);
  assert.deepEqual(items[0], {
    id: "111",
    title: "前置库",
    previewUrl: "https://x/1.jpg",
    views: 0,
    subscriptions: 5,
  });

  assert.deepEqual(normalizeRequiredItems(null), []);
  assert.deepEqual(normalizeRequiredItems({ required_items: "nope" }), []);
});

test("buildRequiredItemsHtml：没有依赖时不占位", () => {
  assert.equal(buildRequiredItemsHtml({}), "");
  assert.equal(buildRequiredItemsHtml({ required_items: [] }), "");
  assert.equal(buildRequiredItemsHtml({ required_items: [{ title: "没有 ID" }] }), "");
});

test("buildRequiredItemsHtml：渲染标题、ID、点击目标与转义", () => {
  const html = buildRequiredItemsHtml({
    required_items: [
      { publishedfileid: "111", title: "前置<库>", preview_url: "https://x/1.jpg", subscriptions: 5 },
      { publishedfileid: "222", title: "", preview_url: "" },
    ],
  });

  assert.match(html, /依赖物品/, "要有区块标题");
  assert.match(html, /2 个前置/, "要显示数量");
  assert.match(html, /data-workshop-id="111"/, "卡片要带跳转 ID");
  assert.match(html, /前置&lt;库&gt;/, "标题要转义");
  assert.match(html, /工坊 #222/, "无标题时退化成工坊 #ID");
  assert.match(html, /无预览图/, "无预览图时给占位");
  assert.doesNotMatch(html, /<库>/, "不能输出未转义标签");
});
