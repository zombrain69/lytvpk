// 命名规范化的展示层映射（路线图第 20 项 P2）：把后端结构化字段转成当前语言文案。
//
// 纯数据 + 纯函数，不 import i18n 运行时（catalog 注入由测试完成），可直接 node --test。
// 约定：命名建议的「分类 / 替换对象」词本身就是名字的一部分（数据层，按用户决策不做映射），
// 这里只本地化**说明文字**：判定阶段、物件域 / 资源类型词、依据模板、跳过原因。

// 判定阶段 → i18n slug（catalog: modals.naming.stage.*）。
export const STAGE_SLUGS = {
  S1: "s1",
  S2: "s2",
  S3: "s3",
  S4: "s4",
  S5: "s5",
  "专项": "specialist",
};

// 物件域词 → i18n slug（catalog: modals.naming.domain.*）。
export const DOMAIN_SLUGS = {
  "生还者": "survivor",
  "特感": "specialInfected",
  "普感": "commonInfected",
  "主武器": "primaryWeapon",
  "副武器": "secondaryWeapon",
  "近战": "melee",
  "投掷物": "throwable",
  "消耗品": "consumable",
  "场景道具": "prop",
  "场景": "scene",
};

// 资源类型词 → i18n slug（catalog: modals.naming.kind.*）。
export const KIND_SLUGS = {
  "界面": "ui",
  "音效": "sound",
  "材质": "texture",
  "脚本": "script",
  "特效": "effect",
  "模型": "model",
};

export function stageLabelKey(stage) {
  const slug = STAGE_SLUGS[stage];
  return slug ? `modals.naming.stage.${slug}` : "";
}

export function domainLabelKey(value) {
  const slug = DOMAIN_SLUGS[value];
  return slug ? `modals.naming.domain.${slug}` : "";
}

export function kindLabelKey(value) {
  const slug = KIND_SLUGS[value];
  return slug ? `modals.naming.kind.${slug}` : "";
}

/**
 * localizeReason 把建议 / 计划项的 ReasonKey + ReasonArgs 转成当前语言文案：
 *   - domain / kind 参数先本地化（缺失映射时保持原文，宁可展示原文也不丢信息）；
 *   - AlreadyNormalized 时追加「已符合规范」后缀；
 *   - 没有 ReasonKey（旧数据）时回退后端中文 reason。
 * translate 即 i18n 运行时的实现（测试里可注入收集器）。
 */
export function localizeReason(item, translate) {
  const reasonKey = String((item && item.reasonKey) || "");
  if (!reasonKey) {
    return String((item && item.reason) || "");
  }
  const params = { ...((item && item.reasonArgs) || {}) };
  if (params.domain) {
    const slug = DOMAIN_SLUGS[params.domain];
    if (slug) params.domain = translate(`modals.naming.domain.${slug}`);
  }
  if (params.kind) {
    const slug = KIND_SLUGS[params.kind];
    if (slug) params.kind = translate(`modals.naming.kind.${slug}`);
  }
  let text = translate(reasonKey, params);
  if (item && item.alreadyNormalized) {
    text += translate("modals.naming.reason.alreadySuffix");
  }
  return text;
}
