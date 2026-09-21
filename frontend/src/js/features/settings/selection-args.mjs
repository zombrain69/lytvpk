// 把“当前选中的 Mod”整理成建组 / 建依赖需要的参数。

function normalizeSelectedPaths(paths) {
  const result = [];
  const seen = new Set();
  for (const raw of Array.isArray(paths) ? paths : []) {
    const path = String(raw ?? "").trim();
    if (!path || seen.has(path)) {
      continue;
    }
    seen.add(path);
    result.push(path);
  }
  return result;
}

export function buildGroupMembersFromSelection(selectedPaths) {
  return normalizeSelectedPaths(selectedPaths);
}

// buildDependencyArgs 返回主 Mod 与其依赖：依赖是选中项里除主 Mod 之外的部分。
export function buildDependencyArgs(selectedPaths, targetPath) {
  const target = String(targetPath ?? "").trim();
  if (!target) {
    return { target: "", dependencies: [] };
  }
  return {
    target,
    dependencies: normalizeSelectedPaths(selectedPaths).filter((path) => path !== target),
  };
}
