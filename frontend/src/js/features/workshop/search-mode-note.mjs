// 工坊浏览器搜索框的"模式说明"。
//
// 为什么单独一份：这个框和本地列表的搜索框长得一样，但它是**服务端搜索** ——
// 关键词发给 Steam，由工坊自己匹配标题/作者等，分页结果也来自服务端。
// 用户很容易照着 Mod 列表的习惯输入 `tag:步枪`、`re:^ak`，然后以为"搜索坏了"。
// 这里把"这个框不收本地语法、本地语法该用在哪"写成一句话，界面与测试共用。

export const WORKSHOP_SEARCH_PLACEHOLDER = "搜索工坊物品标题（由 Steam 搜索，不支持 tag: / re: 等本地语法）";

export const WORKSHOP_SEARCH_NOTE =
  "工坊搜索由 Steam 服务端执行：只按关键词匹配（标题、作者等），会跟着分页一起返回；" +
  "不支持 tag: / re: / -排除 这类本地语法。想用本地语法请到：Mod 列表、压缩包管理、模型统计、" +
  "策略组管理、冲突检测、体检结果。";

/** describeWorkshopSearchMode 返回悬停提示（一行版，供 title 用）。 */
export function describeWorkshopSearchMode() {
  return WORKSHOP_SEARCH_NOTE;
}
