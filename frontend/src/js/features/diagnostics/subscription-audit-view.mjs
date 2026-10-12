// 工坊订阅清单的纯逻辑层（Node 单测直接跑，不碰 DOM）：
// 筛选分类 / 徽标 / 日期格式化 / 计数汇总 / 链接拼装 / 一键退订的确认单内容。
//
// 与后端 WorkshopSubscriptionsSummary 的字段一一对应：
//   publishedFileId / timeSubscribed / disabledLocally / hasWorkshopFile / hasRootCopy / hasDisabledCopy / title
// 与后端 SteamUGCBatchResult 的字段一一对应：
//   action / total / succeeded / skipped / failed / backupPath / results[]

import { t } from "../../core/i18n-runtime.js";

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

/** 确认弹窗里最多列出的条目数（超出折叠为"另有 N 项"）。 */
export const CONFIRM_LIST_LIMIT = 50;

function escapeHtmlText(value) {
	return String(value ?? "")
		.replaceAll("&", "&amp;")
		.replaceAll("<", "&lt;")
		.replaceAll(">", "&gt;")
		.replaceAll('"', "&quot;")
		.replaceAll("'", "&#39;");
}

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

/** summarizeBatchResult 汇总一次批量退订/重订的结果计数。 */
export function summarizeBatchResult(result) {
	return {
		total: Number(result?.total) || 0,
		succeeded: Number(result?.succeeded) || 0,
		skipped: Number(result?.skipped) || 0,
		failed: Number(result?.failed) || 0,
	};
}

/** buildConfirmListHtml 把所选项列成确认弹窗里的清单（全部转义，超限折叠）。 */
function buildConfirmListHtml(entries) {
	const list = Array.isArray(entries) ? entries : [];
	const shown = list.slice(0, CONFIRM_LIST_LIMIT);
	const rows = shown
		.map((entry) => {
			const id = String(entry?.publishedFileId ?? "");
			const title = String(entry?.title ?? "").trim();
			return `<div class="subscription-confirm-row"><code>${escapeHtmlText(id)}</code><span>${escapeHtmlText(
				title || t("toolbox.subscriptionAudit.untitled"),
			)}</span></div>`;
		})
		.join("");
	const rest = list.length - shown.length;
	const more =
		rest > 0
			? `<div class="subscription-confirm-more">${escapeHtmlText(
					t("toolbox.subscriptionAudit.ugc.confirmListMore", { count: rest }),
				)}</div>`
			: "";
	return `<div class="subscription-confirm-list">${rows}${more}</div>`;
}

/** buildUnsubscribeConfirmHtml 生成退订确认弹窗正文（调用方负责标题与弹窗骨架）。 */
export function buildUnsubscribeConfirmHtml(entries) {
	const list = Array.isArray(entries) ? entries : [];
	const lines = [
		t("toolbox.subscriptionAudit.ugc.unsubSummary", { count: list.length }),
		t("toolbox.subscriptionAudit.ugc.unsubNote"),
	];
	return `<div class="write-diff-summary">${lines
		.map((line) => `<p>${escapeHtmlText(line)}</p>`)
		.join("")}</div>${buildConfirmListHtml(list)}`;
}

/** buildResubscribeConfirmHtml 生成「重新订阅所选」确认弹窗正文。 */
export function buildResubscribeConfirmHtml(entries) {
	const list = Array.isArray(entries) ? entries : [];
	const lines = [
		t("toolbox.subscriptionAudit.ugc.resubSummary", { count: list.length }),
		t("toolbox.subscriptionAudit.ugc.resubNote"),
	];
	return `<div class="write-diff-summary">${lines
		.map((line) => `<p>${escapeHtmlText(line)}</p>`)
		.join("")}</div>${buildConfirmListHtml(list)}`;
}
