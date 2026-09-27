// 「依赖物品」区块（对齐上游 361dc9a）：工坊接口返回的 required_items 是普通物品声明的
// 必需前置（合集走 child_items），这里负责把它渲染成和合集子项一致的卡片列表。
//
// 抽成纯模块是为了能单测：详情页本身是 DOM 模块，渲染规则单独验证更稳。

import { escapeHtmlText as escapeHtml } from "../../core/escape-html.mjs";

/** normalizeRequiredItems 过滤脏数据，只保留有 ID 的依赖项。 */
export function normalizeRequiredItems(detail) {
  const raw = Array.isArray(detail?.required_items) ? detail.required_items : [];
  return raw
    .map((item) => ({
      id: String(item?.publishedfileid || "").trim(),
      title: String(item?.title || "").trim(),
      previewUrl: String(item?.preview_url || "").trim(),
      views: Number(item?.views) || 0,
      subscriptions: Number(item?.subscriptions) || 0,
    }))
    .filter((item) => item.id !== "");
}

/**
 * buildRequiredItemsHtml 生成依赖物品区块的 HTML；没有依赖时返回空串（不占位）。
 * 复用合集子项的样式类，保证视觉一致。
 */
export function buildRequiredItemsHtml(detail, { formatCount = (value) => String(value ?? 0) } = {}) {
  const items = normalizeRequiredItems(detail);
  if (items.length === 0) return "";

  const cards = items
    .map((item) => {
      const title = item.title || `工坊 #${item.id}`;
      return `
        <div class="collection-child-card" role="button" tabindex="0" data-workshop-id="${escapeHtml(item.id)}">
          <div class="collection-child-thumb">
            ${
              item.previewUrl
                ? `<img src="${escapeHtml(item.previewUrl)}" alt="${escapeHtml(title)}" loading="lazy" decoding="async">`
                : `<div class="collection-child-thumb-empty" aria-hidden="true">无预览图</div>`
            }
          </div>
          <div class="collection-child-info">
            <div class="collection-child-title">${escapeHtml(title)}</div>
            <div class="collection-child-meta">
              <span>ID ${escapeHtml(item.id)}</span>
              <span>订阅 ${escapeHtml(formatCount(item.subscriptions))}</span>
            </div>
          </div>
        </div>
      `;
    })
    .join("");

  return `
    <div class="collection-items-section required-items-section">
      <div class="collection-items-header">
        <h3>依赖物品</h3>
        <span>${items.length} 个前置</span>
      </div>
      <div class="required-items-hint">该 Mod 声明了以下必需前置，缺了可能不生效或直接报错。</div>
      <div class="collection-items-list">${cards}</div>
    </div>
  `;
}
