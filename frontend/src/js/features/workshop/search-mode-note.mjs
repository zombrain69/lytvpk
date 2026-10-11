// 工坊浏览器搜索框的"模式说明"。
//
// 为什么单独一份：这个框和本地列表的搜索框长得一样，但它是**服务端搜索** ——
// 关键词发给 Steam，由工坊自己匹配标题/作者等，分页结果也来自服务端。
// 用户很容易照着 Mod 列表的习惯输入 `tag:步枪`、`re:^ak`，然后以为"搜索坏了"。
// 这里把"这个框不收本地语法、本地语法该用在哪"写成一句话，界面与测试共用。
// 文案本体在 messages/zh-CN.json 的 `workshop.search.*`；内容约束由
// core/search-consistency.test.mjs 继续核对（含"六个面板"清单）。

import { t } from "../../core/i18n-runtime.js";

/** getWorkshopSearchPlaceholder 返回搜索框占位文案（来自文案目录）。 */
export function getWorkshopSearchPlaceholder() {
  return t("workshop.search.placeholder");
}

/** getWorkshopSearchNote 返回完整模式说明（来自文案目录，供悬停提示用）。 */
export function getWorkshopSearchNote() {
  return t("workshop.search.note");
}

/** describeWorkshopSearchMode 返回悬停提示（一行版，供 title 用）。 */
export function describeWorkshopSearchMode() {
  return getWorkshopSearchNote();
}
