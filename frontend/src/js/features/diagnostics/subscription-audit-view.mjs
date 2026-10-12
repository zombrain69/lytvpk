// 工坊订阅清单的纯逻辑层（Node 单测直接跑，不碰 DOM）：
// 筛选分类 / 徽标 / 日期格式化 / 计数汇总 / 链接拼装。
//
// 与后端 WorkshopSubscriptionsSummary 的字段一一对应：
//   publishedFileId / timeSubscribed / disabledLocally / hasWorkshopFile / hasRootCopy / hasDisabledCopy / title

export const SUBSCRIPTION_FILTERS = [
	{ id: "all", labelKey: "toolbox.subscriptionAudit.filter.all" },
	{ id: "rootCopy", labelKey: "toolbox.subscriptionAudit.filter.rootCopy" },
	{ id: "missing", labelKey: "toolbox.subscriptionAudit.filter.missing" },
	{ id: "downloaded", labelKey: "toolbox.subscriptionAudit.filter.downloaded" },
	{ id: "disabled", labelKey: "toolbox.subscriptionAudit.filter.disabled" },
];

export const SUBSCRIPTION_BADGE_KEYS = {
	downloaded: "toolbox.subscriptionAudit.badge.downloaded",
	rootCopy: "toolbox.subscriptionAudit.badge.rootCopy",
	missing: "toolbox.subscriptionAudit.badge.missing",
	disabled: "toolbox.subscriptionAudit.badge.disabled",
};

/** Steam 官方「我的工坊订阅」页（页面上有单项取消订阅与「取消全部订阅」）。 */
export const UNSUBSCRIBE_GUIDE_URL =
	"https://steamcommunity.com/my/myworkshopfiles/?appid=550&browsefilter=mysubscriptions";

/** workshopItemURL 拼工坊作品页链接。 */
export function workshopItemURL(publishedFileId) {
	return `https://steamcommunity.com/sharedfiles/filedetails/?id=${encodeURIComponent(String(publishedFileId ?? ""))}`;
}

/** isMissingLocally 三个本地位置（workshop / 根目录 / disabled）都没有文件。 */
export function isMissingLocally(entry) {
	return !entry?.hasWorkshopFile && !entry?.hasRootCopy && !entry?.hasDisabledCopy;
}

/** matchesSubscriptionFilter 判断条目是否属于某个筛选分类。 */
export function matchesSubscriptionFilter(entry, filterId) {
	switch (filterId) {
		case "rootCopy":
			return Boolean(entry?.hasRootCopy);
		case "missing":
			return isMissingLocally(entry);
		case "downloaded":
			return Boolean(entry?.hasWorkshopFile);
		case "disabled":
			return Boolean(entry?.disabledLocally);
		default:
			return true;
	}
}

export function filterSubscriptions(entries, filterId) {
	return (entries || []).filter((entry) => matchesSubscriptionFilter(entry, filterId));
}

export function countSubscriptions(entries, filterId) {
	return filterSubscriptions(entries, filterId).length;
}

/** subscriptionBadges 返回该条目要显示的徽标 key 列表（可同时有多个）。 */
export function subscriptionBadges(entry) {
	const badges = [];
	if (entry?.hasWorkshopFile) badges.push("downloaded");
	if (entry?.hasRootCopy) badges.push("rootCopy");
	if (isMissingLocally(entry)) badges.push("missing");
	if (entry?.disabledLocally) badges.push("disabled");
	return badges;
}

/** formatSubscribedDate Unix 秒 → 本地日期 yyyy-MM-dd；非法值返回空串。 */
export function formatSubscribedDate(unixSeconds) {
	const value = Number(unixSeconds);
	if (!Number.isFinite(value) || value <= 0) return "";
	const date = new Date(value * 1000);
	const pad = (n) => String(n).padStart(2, "0");
	return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
}

/** summarizeSubscriptionCounts 汇总行用的计数（与后端口径一致，前端重算以便筛选联动）。 */
export function summarizeSubscriptionCounts(entries) {
	const list = entries || [];
	return {
		total: list.length,
		downloaded: list.filter((entry) => entry?.hasWorkshopFile).length,
		rootCopies: list.filter((entry) => entry?.hasRootCopy).length,
		missing: list.filter(isMissingLocally).length,
		disabled: list.filter((entry) => entry?.disabledLocally).length,
	};
}
