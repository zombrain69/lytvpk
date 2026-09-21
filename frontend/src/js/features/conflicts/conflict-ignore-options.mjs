// 冲突忽略清单的文本解析。
//
// 设置页用多行文本维护清单，后端按 VPK 归档内路径匹配。这里的归一化规则必须
// 与 Go 侧 normalizeConflictIgnoreFileList 保持一致：去掉空行与注释行、统一使用
// "/" 分隔符、转为小写并按出现顺序去重。

export const CONFLICT_IGNORE_LIST_DESCRIPTION =
  "每行一个 VPK 内部路径，例如 materials/shared.vtf；以 / 结尾表示整个目录，例如 scripts/vscripts/。以 # 开头的行视为注释。";

export function parseConflictIgnoreList(text) {
  const entries = [];
  const seen = new Set();

  for (const rawLine of String(text ?? "").split(/\r?\n/)) {
    const trimmed = rawLine.trim();
    if (!trimmed || trimmed.startsWith("#") || trimmed.startsWith("//")) {
      continue;
    }
    const normalized = trimmed.replace(/\\/g, "/").toLowerCase();
    if (seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    entries.push(normalized);
  }

  return entries;
}

export function formatConflictIgnoreList(entries) {
  if (!Array.isArray(entries) || entries.length === 0) {
    return "";
  }
  return parseConflictIgnoreList(entries.join("\n")).join("\n");
}
