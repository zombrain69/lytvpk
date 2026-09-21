/**
 * buildSettingsDeps 合并"注入的依赖对象"与"模块级覆盖项"。
 *
 * 设置页需要的绑定（策略组、启用方案、依赖、体检、工具箱…）数量很多，历史上只用手写
 * 字面量转发，导致 configureSettings 收到但没转发的绑定在运行时是 undefined——
 * 例如点击"自动联动"或保存组权重时会直接报错。这里改为：只要注入过就整体透传，
 * 显式覆盖项（已经在 configureSettings 里解构出来的模块变量）优先。
 *
 * overrides 中值为 undefined 时视为"没有覆盖"，避免把注入的绑定清空。
 */
export function buildSettingsDeps(injected, overrides = {}) {
  const base = injected && typeof injected === "object" ? injected : {};
  const result = { ...base };
  const extra = overrides && typeof overrides === "object" ? overrides : {};
  Object.keys(extra).forEach((key) => {
    const value = extra[key];
    if (value !== undefined) {
      result[key] = value;
    }
  });
  return result;
}
