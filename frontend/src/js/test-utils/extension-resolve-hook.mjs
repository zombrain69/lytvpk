// 让 node --test 能 import 真实前端模块：Wails 生成的绑定/浏览器代码里
// 有一些"不含扩展名"的相对导入（由 Vite 处理），Node 的 ESM 解析器默认会拒绝。
// 这个 hook 只在找不到模块时补试 .js / .mjs，不改变正常解析结果。

export async function resolve(specifier, context, nextResolve) {
  try {
    return await nextResolve(specifier, context);
  } catch (error) {
    const isRelative = specifier.startsWith("./") || specifier.startsWith("../");
    if (error?.code !== "ERR_MODULE_NOT_FOUND" || !isRelative) throw error;
    for (const suffix of [".js", ".mjs"]) {
      try {
        return await nextResolve(specifier + suffix, context);
      } catch {
        // 继续尝试下一个后缀
      }
    }
    throw error;
  }
}
