// Mod 体检结果的展示文案。

const HEALTH_ISSUE_KIND_LABELS = {
  missing_addonlist: "缺少 addonlist.txt",
  missing_file: "条目缺少文件",
  disabled_only: "只剩 disabled 副本",
  unrecorded: "未写入开关记录",
  duplicate_entry: "重复条目",
  invalid_vpk: "VPK 无法解析",
  orphan_meta: "孤立的工坊信息",
  missing_meta: "缺少工坊信息",
  dependency_disabled: "依赖未开启",
  dependency_missing: "依赖缺失",
  dependency_cycle: "依赖成环",
  file_type_mismatch: "同名路径是文件夹",
  meta_id_mismatch: "工坊信息对不上作品",
  outside_root: "条目指向受管目录之外",
  duplicate_vpk_copy: "工坊作品有多份副本",
};

export function formatHealthIssueKind(kind) {
  const key = String(kind || "").trim();
  if (!key) {
    return "未知问题";
  }
  return HEALTH_ISSUE_KIND_LABELS[key] || key;
}

const HEALTH_SEVERITY_LABELS = {
  critical: "严重",
  warning: "警告",
  info: "提示",
};

export function formatHealthSeverity(severity) {
  return HEALTH_SEVERITY_LABELS[String(severity || "").trim()] || "提示";
}

/**
 * healthIssueAutoFixAction 返回这一条体检问题能不能"一键修复"，以及走哪个动作。
 *
 * 对齐 FireAxe 的 `Problem.CanAutomaticallyFix` / `TryAutomaticallyFix` 分级：
 * 只有**语义明确、改动可逆、且已有专用接口**的问题才进白名单 ——
 *   1. 重复条目：删掉 addonlist.txt 里重复的键（后端 RemoveDuplicateAddonListEntries）；
 *   2. 条目缺少文件：清掉指向已不存在文件的条目（后端 RemoveMissingFileAddonListEntries）；
 *   3. 依赖未开启：给这条问题的主 Mod 启用依赖（后端 EnableModDependencies）——
 *      这一条正是上游唯一声明可自动修复的问题（`AddonDependencyProblem.cs:12-26`）。
 * 其余问题（VPK 损坏、依赖缺失 / 成环、工坊信息问题…）需要人工判断，不自动改，只给建议。
 */
const HEALTH_AUTO_FIX_ACTIONS = [
  {
    id: "remove-duplicates",
    kinds: ["duplicate_entry"],
    label: "一键修复：删除重复条目",
    title: "删除 addonlist.txt 里重复的同一 Mod 条目（会先备份、保留第一条）",
    needsTarget: false,
  },
  {
    id: "remove-missing",
    kinds: ["missing_file"],
    label: "一键修复：清理缺失条目",
    title: "把指向已不存在文件的条目从 addonlist.txt 里清掉（会先备份）",
    needsTarget: false,
  },
  {
    id: "enable-dependencies",
    kinds: ["dependency_disabled"],
    label: "一键修复：启用依赖",
    title: "给这个主 Mod 打开它声明的全部依赖（缺文件的依赖会跳过并报告）",
    needsTarget: true,
  },
];

export function healthIssueAutoFixAction(kind) {
  const key = String(kind || "").trim();
  if (!key) {
    return null;
  }
  const action = HEALTH_AUTO_FIX_ACTIONS.find((item) => item.kinds.includes(key));
  return action ? { ...action } : null;
}

/** summarizeHealthAutoFix 统计体检结果里可自动修复的问题数量（按动作分组）。 */
export function summarizeHealthAutoFix(report) {
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  const counts = new Map();
  issues.forEach((issue) => {
    const action = healthIssueAutoFixAction(issue?.kind);
    if (!action) return;
    counts.set(action.id, (counts.get(action.id) || 0) + 1);
  });
  // 按白名单声明顺序输出，界面上的统计条不会因为问题顺序变化而抖动。
  const actions = HEALTH_AUTO_FIX_ACTIONS.filter((item) => counts.has(item.id)).map((item) => ({
    id: item.id,
    count: counts.get(item.id),
    label: item.label,
    title: item.title,
  }));
  return {
    total: actions.reduce((sum, item) => sum + item.count, 0),
    actions,
  };
}

export function formatHealthReportSummary(report) {
  if (!report) {
    return "还没有体检结果";
  }
  const total = Number(report.totalIssues) || 0;
  if (total === 0) {
    return "未发现问题";
  }

  const counts = { critical: 0, warning: 0, info: 0 };
  for (const issue of report.issues || []) {
    const severity = String(issue?.severity || "").trim();
    if (severity in counts) {
      counts[severity] += 1;
    }
  }

  const parts = [];
  if (counts.critical > 0) parts.push(`严重 ${counts.critical}`);
  if (counts.warning > 0) parts.push(`警告 ${counts.warning}`);
  if (counts.info > 0) parts.push(`提示 ${counts.info}`);
  return parts.length > 0 ? `发现 ${total} 个问题：${parts.join("、")}` : `发现 ${total} 个问题`;
}

// buildHealthIssueRows 把体检结果整理成可直接渲染的行，并给出被截断的条数。
export function buildHealthIssueRows(report, limit = 100) {
  const issues = Array.isArray(report?.issues) ? report.issues : [];
  const max = Number.isFinite(limit) && limit > 0 ? Math.floor(limit) : issues.length;
  const rows = issues.slice(0, Math.max(max, 0)).map((issue) => {
    const severity = String(issue?.severity || "info").trim() || "info";
    return {
      severity,
      severityLabel: formatHealthSeverity(severity),
      title: `${formatHealthIssueKind(issue?.kind)} · ${issue?.name || ""}`,
      message: issue?.message || "",
      kind: String(issue?.kind || ""),
      autoFix: healthIssueAutoFixAction(issue?.kind),
      // 白名单动作要操作谁：后端给的 target（addonlist 键 / 路径）优先，退回问题名。
      fixTarget: String(issue?.target || issue?.name || ""),
    };
  });
  return { rows, hiddenCount: Math.max(issues.length - rows.length, 0) };
}
