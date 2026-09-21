// Mod 依赖相关操作的提示文案。

function bucketCount(value) {
  return Array.isArray(value) ? value.length : 0;
}

export function formatDependencyEnableSummary(name, result) {
  const label = String(name || "").trim();
  const parts = [];
  const enabled = bucketCount(result?.enabled);
  const alreadyEnabled = bucketCount(result?.alreadyEnabled);
  const missing = bucketCount(result?.missing);

  if (enabled > 0) parts.push(`已启用 ${enabled} 个`);
  if (alreadyEnabled > 0) parts.push(`${alreadyEnabled} 个本就开启`);
  if (missing > 0) parts.push(`${missing} 个文件缺失`);

  const body = parts.join("，") || "没有需要处理的依赖";
  return label ? `${label}：${body}` : body;
}

export function formatDependencyBatchEnableSummary(result) {
  const enabled = bucketCount(result?.enabled);
  const masters = Number(result?.masterCount) || 0;
  if (enabled === 0 || masters === 0) {
    return "没有需要启用的依赖";
  }
  return `已为 ${masters} 个 Mod 启用 ${enabled} 个依赖`;
}
