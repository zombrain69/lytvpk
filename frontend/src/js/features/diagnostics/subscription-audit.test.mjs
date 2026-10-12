import assert from "node:assert/strict";
import test from "node:test";

import {
	SUBSCRIPTION_FILTERS,
	UNSUBSCRIBE_GUIDE_URL,
	countSubscriptions,
	filterSubscriptions,
	formatSubscribedDate,
	isMissingLocally,
	matchesSubscriptionFilter,
	subscriptionBadges,
	summarizeSubscriptionCounts,
	workshopItemURL,
} from "./subscription-audit-view.mjs";

const entryDownloaded = { publishedFileId: "1", hasWorkshopFile: true, timeSubscribed: 1791369027 };
const entryRootCopy = { publishedFileId: "2", hasWorkshopFile: true, hasRootCopy: true };
const entryMissing = { publishedFileId: "3" };
const entryDisabledOnly = { publishedFileId: "4", hasDisabledCopy: true, disabledLocally: true };

test("筛选分类：全部 / 已转存 / 无本地文件 / 已下载 / 本地停用", () => {
	const entries = [entryDownloaded, entryRootCopy, entryMissing, entryDisabledOnly];

	assert.equal(filterSubscriptions(entries, "all").length, 4);
	assert.deepEqual(
		filterSubscriptions(entries, "rootCopy").map((e) => e.publishedFileId),
		["2"],
	);
	assert.deepEqual(
		filterSubscriptions(entries, "missing").map((e) => e.publishedFileId),
		["3"],
	);
	assert.deepEqual(
		filterSubscriptions(entries, "downloaded").map((e) => e.publishedFileId),
		["1", "2"],
	);
	assert.deepEqual(
		filterSubscriptions(entries, "disabled").map((e) => e.publishedFileId),
		["4"],
	);
	assert.equal(countSubscriptions(entries, "missing"), 1);
	// 未知筛选 id：宽松地当作"全部"。
	assert.equal(countSubscriptions(entries, "unknown"), 4);
});

test("isMissingLocally：只有三个位置都没有文件才算无本地文件", () => {
	assert.equal(isMissingLocally(entryMissing), true);
	assert.equal(isMissingLocally(entryDownloaded), false);
	assert.equal(isMissingLocally(entryRootCopy), false);
	assert.equal(isMissingLocally(entryDisabledOnly), false);
	assert.equal(isMissingLocally(null), true);
});

test("matchesSubscriptionFilter：disabled 用 Steam 本地停用标记", () => {
	assert.equal(matchesSubscriptionFilter(entryDisabledOnly, "disabled"), true);
	assert.equal(matchesSubscriptionFilter(entryMissing, "disabled"), false);
});

test("subscriptionBadges：一个人可以同时挂多个徽标", () => {
	assert.deepEqual(subscriptionBadges(entryDownloaded), ["downloaded"]);
	assert.deepEqual(subscriptionBadges(entryRootCopy), ["downloaded", "rootCopy"]);
	assert.deepEqual(subscriptionBadges(entryMissing), ["missing"]);
	assert.deepEqual(subscriptionBadges(entryDisabledOnly), ["disabled"]);
	assert.deepEqual(subscriptionBadges({ hasDisabledCopy: true }), []);
});

test("formatSubscribedDate：Unix 秒 → 本地 yyyy-MM-dd，非法值给空串", () => {
	const formatted = formatSubscribedDate(1791369027);
	assert.match(formatted, /^\d{4}-\d{2}-\d{2}$/);
	assert.equal(formatSubscribedDate(0), "");
	assert.equal(formatSubscribedDate(-5), "");
	assert.equal(formatSubscribedDate("bad"), "");
	assert.equal(formatSubscribedDate(null), "");
});

test("summarizeSubscriptionCounts：与后端口径一致", () => {
	const counts = summarizeSubscriptionCounts([entryDownloaded, entryRootCopy, entryMissing, entryDisabledOnly]);
	assert.deepEqual(counts, { total: 4, downloaded: 2, rootCopies: 1, missing: 1, disabled: 1 });
	assert.deepEqual(summarizeSubscriptionCounts(null), {
		total: 0,
		downloaded: 0,
		rootCopies: 0,
		missing: 0,
		disabled: 0,
	});
});

test("链接：工坊页与官方退订页", () => {
	assert.equal(workshopItemURL("3815004017"), "https://steamcommunity.com/sharedfiles/filedetails/?id=3815004017");
	assert.ok(UNSUBSCRIBE_GUIDE_URL.startsWith("https://steamcommunity.com/my/myworkshopfiles/"));
	assert.ok(UNSUBSCRIBE_GUIDE_URL.includes("appid=550"));
	assert.ok(UNSUBSCRIBE_GUIDE_URL.includes("browsefilter=mysubscriptions"));
});

test("筛选定义：首项为「全部」，且每个筛选都有 labelKey", () => {
	assert.equal(SUBSCRIPTION_FILTERS[0].id, "all");
	for (const filter of SUBSCRIPTION_FILTERS) {
		assert.match(filter.labelKey, /^toolbox\.subscriptionAudit\.filter\./);
	}
});
