// 「为什么现在不能做」与「刚刚为什么失败」的统一解释层。
//
// 对齐 FireAxe.GUI 的两个设计：
//   1. `ObjectExplanationManager.Get`（`ObjectExplanationManager.cs:27-49`）：
//      按类型链逐级回退，**最后一定给出一句话** —— 界面上不会出现空白提示；
//   2. `ExceptionExplanations`（`ExceptionExplanations.cs:12-44`）的**场景**区分：
//      同一个错误出现在"输入框里"和"一次操作里"，解释不该一样。
//
// 本项目落到两件事：
//   - `explainActionAvailability`：按钮为什么是灰的 / 点了会怎样；
//   - `explainOperationError`：后端错误 → 一句人话 + 下一步怎么做。
// 规则按顺序匹配、第一条命中即返回；都不命中时也有通用兜底（保证 summary 非空）。

export const EXPLANATION_SCENES = { operation: "operation", input: "input" };

const trim = (value) => String(value ?? "").trim();

/**
 * sceneFromErrorType 按后端给的错误类型推断场景（对齐上游 `ExceptionExplanationScene`）。
 * 输入类错误（重命名 / 校验 / 设置项）用"输入不合法"的说法，
 * 其余按一次"操作失败"来解释。
 */
export function sceneFromErrorType(errorType) {
  const type = trim(errorType);
  if (!type) return EXPLANATION_SCENES.operation;
  return /输入|校验|重命名|格式|解析/.test(type) ? EXPLANATION_SCENES.input : EXPLANATION_SCENES.operation;
}

/**
 * extractSubject 从后端消息里取出"这条错误说的是哪个文件"。
 * 解释层会换掉原始描述，但**不能把文件名一起丢掉** ——
 * 批量操作时"哪个文件失败了"比"为什么失败"同样重要。
 */
function extractSubject(raw) {
  const match = trim(raw).match(/^(?:移动|复制|删除|重命名|打包|解包|修复|跳过)\s+([^:：]+?)\s*(?:失败|[:：]|$)/);
  return match ? trim(match[1]) : "";
}

/**
 * 错误规则表。`scene` 为空表示两种场景都用这条；
 * 命中后返回 { summary, hint }，summary 是"发生了什么"，hint 是"接下来怎么做"。
 */
const ERROR_RULES = [
  {
    // 路径守卫（internal/app/path_guard.go）
    match: (text) => text.includes("不在当前受管的"),
    summary: "这个文件不在当前管理的 addons / workshop / disabled 里",
    hint: "刷新一次 Mod 列表；如果刚换过游戏目录，重新选一次目录再操作。",
  },
  {
    // 文件操作闸门（internal/app/file_op_gate.go）
    match: (text) => text.includes("另一个文件操作正在进行"),
    summary: "上一个文件操作还没结束",
    hint: "等状态栏的提示消失后再试，避免两批操作互相插队。",
  },
  {
    match: (text) => text.includes("需要先转移") || (text.includes("创意工坊") && text.includes("转移")),
    summary: "创意工坊 Mod 不能直接开关",
    hint: "先点那一行的「复制到 addons」，把它转移到插件目录再操作。",
  },
  {
    match: (text) => text.includes("未选择L4D2目录") || text.includes("还没有选择 addons 目录") || text.includes("未配置配置目录"),
    summary: "还没有选择游戏目录",
    hint: "点左上角「选择 addons 目录」选一次，之后会被记住。",
  },
  {
    match: (text) => text.includes("正则表达式无效"),
    scene: EXPLANATION_SCENES.input,
    summary: "搜索里的正则写错了",
    hint: "检查 `re:` 后面的写法，或去掉 `re:` 改回普通词搜索。",
  },
  {
    match: (text) => text.includes("文件名不合法") || text.includes("文件名不能包含") || text.includes("文件名不能以") || text.includes("是 Windows 保留设备名"),
    scene: EXPLANATION_SCENES.input,
    summary: "这个名字不能用作文件名",
    hint: "避开 < > : \" / \\ | ? * 与控制字符，不要以空格或点结尾。",
  },
  {
    match: (text) => text.includes("输入不合法") || text.includes("无效的") || text.includes("缺少"),
    scene: EXPLANATION_SCENES.input,
    summary: "输入的内容不合法",
    hint: "按提示改一下输入；不确定就恢复默认值再试。",
  },
  {
    match: (text) => text.includes("找不到对应文件") || text.includes("文件不存在") || text.includes("does not exist") || text.includes("cannot find the file"),
    summary: "要操作的文件已经不在了",
    hint: "刷新列表后重试；它可能已被移动或删除。",
  },
  {
    // 注意别和上一条混：`cannot find the path` 说的是**目标路径**不可用
    // （常见于目标目录被删、或目标位置被同名文件占住），不是源文件消失了。
    match: (text) => text.includes("cannot find the path") || text.includes("系统找不到指定的路径"),
    summary: "目标路径当前不可用",
    hint: "确认目标目录还在、且没有被同名文件占住，然后重试。",
  },
  {
    match: (text) => text.includes("已存在") || text.includes("already exists"),
    summary: "目标位置已经有同名文件",
    hint: "选择「替换」或「跳过」，或先给其中一个改个名字。",
  },
  {
    match: (text) => text.includes("拒绝访问") || text.toLowerCase().includes("access is denied") || text.toLowerCase().includes("permission"),
    summary: "系统拒绝了这次读写",
    hint: "关掉正在占用该文件的程序（游戏本体、杀软、解压工具）后重试。",
  },
  {
    match: (text) => text.includes("空间不足") || text.toLowerCase().includes("not enough space") || text.toLowerCase().includes("disk full"),
    summary: "磁盘空间不够",
    hint: "清理目标盘空间，或换一个盘再操作。",
  },
  {
    match: (text) => text.includes("网络") || text.toLowerCase().includes("timeout") || text.toLowerCase().includes("connection") || text.toLowerCase().includes("econn") || text.toLowerCase().includes("dial tcp"),
    summary: "网络没连上",
    hint: "确认能访问 Steam；必要时开代理 / 加速后重试。",
  },
];

const DEFAULT_ERROR_HINT = "如果反复出现，把这条消息和对应的 Mod 名一起反馈。";

/**
 * explainOperationError 把后端（或系统）的原始错误转成"人话 + 下一步"。
 * 永远返回非空 summary：找不到规则时就用原始消息，再没有就用「操作失败」。
 */
export function explainOperationError(rawMessage, { scene = EXPLANATION_SCENES.operation } = {}) {
  const raw = trim(typeof rawMessage === "object" && rawMessage !== null ? rawMessage.message : rawMessage);
  const subject = extractSubject(raw);
  for (const rule of ERROR_RULES) {
    if (rule.scene && rule.scene !== scene) continue;
    if (!rule.match(raw)) continue;
    return { summary: rule.summary, hint: rule.hint, raw, subject, matched: true };
  }
  return { summary: raw || "操作失败", hint: DEFAULT_ERROR_HINT, raw, subject, matched: false };
}

/** 会写磁盘的操作：忙碌时这些操作要等（只读操作不受影响）。 */
const WRITE_ACTIONS = new Set(["move", "delete", "pack", "transfer-workshop", "toggle", "apply-priority", "apply-group"]);

/**
 * explainActionAvailability 判断一个操作现在能不能做，并给出原因与出路。
 * 返回 { available, reason, hint }：available=false 时 reason 一定非空。
 */
export function explainActionAvailability({ action = "", file = null, context = {} } = {}) {
  const { busy = false, hasRoot = true, selectionCount = 0 } = context || {};
  const name = trim(action);

  if (!hasRoot) {
    return { available: false, reason: "还没有选择游戏目录", hint: "先选一次 addons 目录，之后会被记住。" };
  }
  if (busy && WRITE_ACTIONS.has(name)) {
    return { available: false, reason: "正在处理另一个文件操作（移动 / 删除 / 打包）", hint: "等状态栏的提示消失后再试。" };
  }
  if (name === "toggle" && file?.location === "workshop") {
    return { available: false, reason: "创意工坊 Mod 不能直接启用 / 禁用", hint: "先把它「复制到 addons」再开关。" };
  }
  if (name === "game-state" && file?.location === "disabled") {
    return { available: false, reason: "这个 Mod 在 disabled 目录里，游戏不会加载它", hint: "先把它移回 addons，再改游戏内开关。" };
  }
  if (name === "disable-file") {
    // 模型统计等面板里的"禁用"按钮：只对 addons 根目录里的 Mod 直接可用。
    if (!trim(file?.path)) {
      return { available: false, reason: "这个条目没有文件路径", hint: "刷新一次列表再试。" };
    }
    if (file?.location === "disabled") {
      return { available: false, reason: "这个 Mod 已经在 disabled 目录里", hint: "需要时先把它移回 addons。" };
    }
    if (file?.location !== "root") {
      return { available: false, reason: "只有 addons 根目录里的 Mod 能直接禁用", hint: "创意工坊 Mod 先「复制到 addons」，再从这里禁用。" };
    }
  }
  if ((name === "batch" || name === "group-apply") && Number(selectionCount) <= 0) {
    return { available: false, reason: "还没有选中任何 Mod", hint: "勾选若干 Mod，或用搜索 / 筛选缩小范围后再操作。" };
  }
  return { available: true, reason: "", hint: "" };
}

/** formatExplanation 把解释拼成一行（hint 为空时不加括号）。 */
export function formatExplanation(explanation) {
  const summary = trim(explanation?.summary);
  const hint = trim(explanation?.hint);
  if (!summary) return hint;
  return hint ? `${summary}（${hint}）` : summary;
}
