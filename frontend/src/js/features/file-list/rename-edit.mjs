// 重命名弹窗的纯逻辑：输入框预填、addoninfo 标题清洗、最终文件名预览。
// 标签以「[主标签+副标签]名字.vpk」形式落盘（见 internal/app/vpk_actions.go 的 SetVPKTags），
// 编辑框只编辑名字本体；标签前缀的保留口径与后端 RenameVPKFile 一致。
// 从 operations.js 抽出以便 node --test 直测（重命名预填曾发生标签重复回归）。

export function sanitizeRenameTitle(title) {
  const cleaned = String(title || "")
    .replace(/[\u0000-\u001f<>:"/\\|?*]+/g, " ")
    .replace(/\.vpk$/i, "")
    .replace(/\s+/g, " ")
    .trim()
    .replace(/[. ]+$/g, "");

  if (/^(con|prn|aux|nul|com[1-9]|lpt[1-9])$/i.test(cleaned)) {
    return `_${cleaned}`;
  }

  return cleaned;
}

export function parseFilenameTags(filename) {
  const match = String(filename || "").match(/^(_?)\[(.*?)\](.*)$/);
  if (!match) {
    return { hasTags: false, tags: [] };
  }

  const tags = match[2]
    .split(/[,+]/)
    .map((tag) => tag.trim())
    .filter(Boolean);

  return { hasTags: true, tags };
}

// computeRenameEditName 计算重命名弹窗输入框的预填值：[标签] 前缀在编辑时隐藏，
// 保存时由 buildRenamePreview 与后端 RenameVPKFile 同口径自动还原；隐藏（_）状态同理不进输入框。
// 回归修复：此处曾误把标签文字拼进预填值（[Airi]A.vpk 编辑保存后变成 [Airi]AiriA.vpk），
// 且 _[标签] 组合会丢掉标签文字的首字符。
export function computeRenameEditName(fileName) {
  const name = String(fileName || "");
  const isHidden = name.startsWith("_");

  let editName = name;
  const tagMatch = name.match(/^_?\[(.*?)\](.*)$/);
  if (tagMatch) {
    editName = tagMatch[2];
  } else if (isHidden) {
    editName = editName.substring(1);
  }
  if (editName.toLowerCase().endsWith(".vpk")) {
    editName = editName.substring(0, editName.length - 4);
  }
  return editName;
}

function getDirectoryPath(filePath) {
  const path = String(filePath || "");
  const separatorIndex = Math.max(path.lastIndexOf("\\"), path.lastIndexOf("/"));
  return separatorIndex >= 0 ? path.slice(0, separatorIndex) : "";
}

function joinPathForPreview(dir, filename) {
  if (!dir) return filename;
  if (dir.endsWith("\\") || dir.endsWith("/")) {
    return dir + filename;
  }
  return `${dir}\\${filename}`;
}

export function buildRenamePreview(filePath, oldFileName, inputName, isHidden) {
  let finalName = inputName.trim();
  if (!finalName.toLowerCase().endsWith(".vpk")) {
    finalName += ".vpk";
  }
  if (isHidden) {
    finalName = "_" + finalName;
  }

  const oldTags = parseFilenameTags(oldFileName);
  const newTags = parseFilenameTags(finalName);
  if (oldTags.hasTags && !newTags.hasTags && oldTags.tags.length > 0) {
    let prefix = "";
    let body = finalName;
    if (finalName.startsWith("_")) {
      prefix = "_";
      body = finalName.substring(1);
    }

    finalName = `${prefix}[${oldTags.tags.join("+")}]${body}`;
  }

  return {
    filename: finalName,
    path: joinPathForPreview(getDirectoryPath(filePath), finalName),
  };
}
