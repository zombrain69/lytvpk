// 后端错误展示桥（路线图第 21 项阶段 B ④）。
//
// 背景：Go 后端的错误串是中文，经 Wails 边界以字符串原文到达前端；批量 / 单文件
// 操作的失败明细会直接插进通知的 {reason} / {error} —— en 界面会显示中文。
// 本模块是**展示层桥**：
//   - zh（默认）：原样返回（与替换前逐字节一致；后端原文永远是最准确的文案）；
//   - en：把已知的后端文件操作错误模板翻成英文；认不出的一律原样返回（不吞原因）。
//
// 覆盖边界（词条与 Go 侧产出模板对齐；backend-error.test.mjs 会扫描 Go 源做保鲜，
// 缺条目即失败）：
//   1. internal/app/file_operation_errors.go 的「{动作} {文件名} 失败：{原因}」句子族
//      （动作词表 + describeFileMoveFailure 的六条规范原因）；
//   2. 文件操作链路上直接可达的高频整句（文件不存在 / 未选择目录 / 工坊文件需先转移 /
//      目标已存在 / addonlist 同步失败 / 文件操作闸门 / 路径守卫等）。
// 新增后端错误文案时：若它会出现在文件操作失败明细里，在此补一条（测试兜底）。
//
// 注意：不经过本桥的路径（如 toast 解释层 explainOperationError 的未识别兜底）仍是
// 原文，属既定设计；边界与升级路径见 docs/development/i18n.md「后端错误展示桥」。
import { getLocale } from "./i18n-runtime.js";

// 动作词 → 英文动词（formatFileMoveError 的全部调用点由测试扫描 Go 源保鲜）。
const ACTION_EN = new Map([
  ["读取", "read"],
  ["写入", "write"],
  ["移动", "move"],
  ["禁用", "disable"],
  ["启用", "enable"],
  ["重命名", "rename"],
  ["改名", "rename"],
  ["设置标签", "update the tags of"],
  ["保存下载文件", "save"],
  ["调整槽位", "adjust the slot of"],
]);

// describeFileMoveFailure 的六条规范原因（中文原文 → 英文）。
const MOVE_REASON_EN = new Map([
  [
    "文件正被其它程序占用（游戏、杀毒软件、资源管理器预览都可能占用它），关闭占用的程序后重试",
    "The file is in use by another program (the game, antivirus, or Explorer preview may be holding it); close it and retry",
  ],
  [
    "没有权限修改这个文件（可能被设为只读，或被安全软件拦截）",
    "No permission to modify this file (it may be read-only, or blocked by security software)",
  ],
  [
    "目标目录里已经有同名文件，请先改名或删除它",
    "A file with the same name already exists in the target folder; rename or delete it first",
  ],
  [
    "路径超过 Windows 的 260 字符上限，请缩短名称或把 Mod 放到更浅的目录",
    "The path exceeds the Windows 260-character limit; shorten the name or move the Mod to a shallower folder",
  ],
  ["磁盘空间不足", "Not enough disk space"],
  [
    "找不到这个文件，可能已被移动或删除，请刷新列表后重试",
    "The file cannot be found; it may have been moved or deleted — refresh the list and retry",
  ],
]);

// 整句错误（原文全文匹配 → 英文）。模式里的捕获组原样拼回（多为文件名 / 路径 / 内层原因）。
const WHOLE_MESSAGE_RULES = [
  [/^文件未找到: (.+)$/, (m) => `File not found: ${m[1]}`],
  [/^文件不存在: (.+)$/, (m) => `File does not exist: ${m[1]}`],
  [/^未选择L4D2目录$/, () => "No L4D2 folder selected"],
  [
    /^还没有选择 addons 目录，无法确认这个文件是否受管$/,
    () => "No addons folder selected, so it cannot be verified that this file is managed",
  ],
  [
    /^workshop文件需要先转移到插件目录才能启用\/禁用$/,
    () => "Workshop files must be moved to the addons folder before they can be enabled or disabled",
  ],
  [/^无效的文件状态转换$/, () => "Invalid file state transition"],
  [/^目标文件已存在: (.+)$/, (m) => `A file with the same name already exists: ${m[1]}`],
  [
    /^文件已移动，但 addonlist\.txt 同步失败: (.+)$/,
    (m) => `The file was moved, but syncing addonlist.txt failed: ${m[1]}`,
  ],
  [
    /^文件已移入回收站，但 addonlist\.txt 同步失败: (.+)$/,
    (m) => `The file was moved to the Recycle Bin, but syncing addonlist.txt failed: ${m[1]}`,
  ],
  [
    /^已复制到 addons，但 addonlist\.txt 同步失败: (.+)$/,
    (m) => `Copied to addons, but syncing addonlist.txt failed: ${m[1]}`,
  ],
  [
    /^另一个文件操作正在进行（移动 \/ 删除 \/ 打包），请等它完成后再试$/,
    () => "Another file operation is in progress (move / delete / pack); wait for it to finish and try again",
  ],
  [
    /^这个文件不在当前受管的 addons \/ workshop \/ disabled 目录里：(.+)$/,
    (m) => `This file is not inside the managed addons / workshop / disabled folders: ${m[1]}`,
  ],
  [
    /^只接受完整路径，收到的是相对路径: (.+)$/,
    (m) => `Only absolute paths are accepted; received a relative path: ${m[1]}`,
  ],
  [/^文件路径为空$/, () => "File path is empty"],
  [/^目标目录不能为空$/, () => "Target folder cannot be empty"],
  [/^目标路径不是目录: (.+)$/, (m) => `Target path is not a folder: ${m[1]}`],
  [/^删除文件失败: (.+)$/, (m) => `Failed to delete the file: ${m[1]}`],
  [/^源文件不存在或无法访问: (.+)$/, (m) => `Source file does not exist or cannot be accessed: ${m[1]}`],
];

// 「{动作} {文件名} 失败：{原因}」句子族（formatFileMoveError 的两个分支都用全角冒号，
// 这里同时容忍半角，保持与真实产出宽松对齐）。
const FILE_OP_SENTENCE = /^(\S+) (.+?) 失败[：:](.+)$/;

function translateFileOperationSentence(text) {
  const match = text.match(FILE_OP_SENTENCE);
  if (!match) return "";
  const actionEn = ACTION_EN.get(match[1]);
  if (!actionEn) return "";
  // 原因命中规范词表就翻英文；命中不了（如未识别的系统错误原文）保持原样 ——
  // 至少要指出「哪个动作、哪个文件」出了问题。
  const reasonEn = MOVE_REASON_EN.get(match[3]) || match[3];
  return `Failed to ${actionEn} ${match[2]}: ${reasonEn}`;
}

function toText(raw) {
  if (raw == null) return "";
  if (typeof raw === "string") return raw;
  if (raw instanceof Error) return String(raw.message || "");
  if (typeof raw === "object" && typeof raw.message === "string") return raw.message;
  return String(raw);
}

/**
 * formatBackendErrorDetail 把后端错误原文转成当前语言下的展示明细。
 * 用于通知模板里的 {reason} / {error} 插值位：
 *   - zh（默认）：原样返回；en：已知模板翻英文，认不出原样返回；
 *   - 接受字符串或 Error（取 message），空值安全（返回空串）。
 */
export function formatBackendErrorDetail(raw) {
  const text = toText(raw);
  if (getLocale() !== "en") return text;
  const trimmed = text.trim();
  if (!trimmed) return text;
  const sentence = translateFileOperationSentence(trimmed);
  if (sentence) return sentence;
  for (const [pattern, build] of WHOLE_MESSAGE_RULES) {
    const match = trimmed.match(pattern);
    if (match) return build(match);
  }
  return text;
}
