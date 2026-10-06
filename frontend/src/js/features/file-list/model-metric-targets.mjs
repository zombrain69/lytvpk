// 「模型复杂度排序」要分析哪些文件（纯函数，node --test 覆盖）。
//
// 背景（真机 2912 个 Mod）：这个排序原来永远把**全库**路径发给后端做模型扫描，
// 第一次实测 43s；而排序本身只作用于"当前可见结果"。
// 规则：
//   · 有筛选（可见数 < 全量）→ 只分析当前可见结果；其余文件等被筛出来时再按需分析；
//   · 没有筛选 → 分析全部；
//   · 可见结果为空 → 不分析（此时也没有东西可排序）。
export function modelMetricScanTargets(allFiles, visibleFiles) {
  const all = Array.isArray(allFiles) ? allFiles : [];
  const visible = Array.isArray(visibleFiles) ? visibleFiles : [];
  const list = visible.length < all.length ? visible : all;
  return list.map((file) => String(file?.path || "")).filter(Boolean);
}
