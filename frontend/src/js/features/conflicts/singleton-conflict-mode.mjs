// 「启用 Mod 时的同类冲突提醒」处理方式（设置 → addonlist → 「Mod 冲突分析」）。
//
//   ask     —— 每次询问（默认）：启用时弹三选一（关闭旧的并启用 / 两者共存 / 取消）；
//   replace —— 自动「关闭旧的并启用」：不再弹窗，直接关闭同类旧 Mod 再启用；
//   coexist —— 不再提醒：直接启用，不处理旧 Mod（游戏按加载顺序/随机决定谁生效）。
//
// 存于 config.json 的 singletonConflictMode 字段；未知/空值一律回退 ask（保持旧行为）。

export const SINGLETON_CONFLICT_MODES = ["ask", "replace", "coexist"];

/** normalizeSingletonConflictMode 归一化设置值；未知/空 → "ask"。 */
export function normalizeSingletonConflictMode(raw) {
	const value = String(raw ?? "").trim().toLowerCase();
	return SINGLETON_CONFLICT_MODES.includes(value) ? value : "ask";
}
