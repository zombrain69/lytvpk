import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import test from "node:test";
import { fileURLToPath } from "node:url";

import { installTestCatalog } from "../../core/i18n-test-catalog.mjs";
import {
	CONFIRM_LIST_LIMIT,
	SUBSCRIPTION_FILTERS,
	UNSUBSCRIBE_GUIDE_URL,
	buildResubscribeConfirmHtml,
	buildUnsubscribeConfirmHtml,
	countSubscriptions,
	filterSubscriptions,
	formatSubscribedDate,
	isMissingLocally,
	matchesSubscriptionFilter,
	subscriptionBadges,
	summarizeBatchResult,
	summarizeSubscriptionCounts,
	workshopItemURL,
} from "./subscription-audit-view.mjs";

// 纯模块单测要吃真实中文目录（确认单文案断言），与其它 i18n 批次的测试同款。
installTestCatalog();

const moduleDir = dirname(fileURLToPath(import.meta.url));

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

test("summarizeBatchResult：缺字段兜底为 0", () => {
	assert.deepEqual(
		summarizeBatchResult({ total: 5, succeeded: 3, skipped: 1, failed: 1 }),
		{ total: 5, succeeded: 3, skipped: 1, failed: 1 },
	);
	assert.deepEqual(summarizeBatchResult(null), { total: 0, succeeded: 0, skipped: 0, failed: 0 });
});

test("buildUnsubscribeConfirmHtml：中文文案 + 转义 + 超长折叠", () => {
	const entries = [
		{ publishedFileId: "111", title: "示例 <Mod> & \"引号\"" },
		{ publishedFileId: "222", title: "" },
	];
	const html = buildUnsubscribeConfirmHtml(entries);
	assert.match(html, /退订 2 项/);
	assert.match(html, /无法撤销/);
	assert.match(html, /回滚清单/);
	// 转义：尖括号/引号不能原样出现。
	assert.ok(!html.includes("<Mod>"));
	assert.match(html, /&lt;Mod&gt;/);
	assert.match(html, /&quot;引号&quot;/);
	// 空标题回退 + ID 均转义进 <code>。
	assert.match(html, /（标题未解析）/);
	assert.match(html, /<code>111<\/code>/);
	// 不超限时不出现折叠行。
	assert.ok(!html.includes("未展开"));

	const many = Array.from({ length: CONFIRM_LIST_LIMIT + 3 }, (_, index) => ({
		publishedFileId: String(index + 1),
		title: `T${index + 1}`,
	}));
	const folded = buildUnsubscribeConfirmHtml(many);
	assert.match(folded, /另有 3 项未展开/);
	assert.ok(!folded.includes(`<code>${CONFIRM_LIST_LIMIT + 1}</code>`));
});

test("buildResubscribeConfirmHtml：重订口径与跳过说明", () => {
	const html = buildResubscribeConfirmHtml([{ publishedFileId: "333", title: "T" }]);
	assert.match(html, /重新订阅 1 项/);
	assert.match(html, /自动跳过/);
	assert.match(html, /<code>333<\/code>/);
});

test("源码接线：一键退订用到的绑定 / 事件 / 帮助函数都已接上", () => {
	const source = readFileSync(join(moduleDir, "subscription-audit.js"), "utf8");
	for (const binding of [
		"GetSteamUGCStatus",
		"SetSteamAPIDllPath",
		"DetectSteamAPIDllCandidates",
		"PickSteamAPIDllFile",
		"UnsubscribeSteamWorkshopItems",
		"SubscribeSteamWorkshopItems",
	]) {
		assert.match(source, new RegExp(`${binding},`), `应导入 ${binding}`);
	}
	assert.match(source, /EventsOn\("steam_ugc_progress"/, "应订阅批量进度事件");
	assert.match(source, /buildUnsubscribeConfirmHtml/);
	assert.match(source, /buildResubscribeConfirmHtml/);
	assert.match(source, /subscription-batch-resub-all/, "结果面板应有「重新订阅本次退订」入口");
	assert.match(source, /resubscribeLastBatch/, "回滚入口应接在结果面板上");
	assert.match(source, /confirmHtmlInApp\(/, "退订前必须走清单确认弹窗");
	// 账户红线的 UI 侧执行点：只在确认之后调用批量退订。
	const confirmIndex = source.indexOf("await confirmHtmlInApp(");
	const runIndex = source.indexOf('await runBatch("unsubscribe", entries)');
	assert.ok(confirmIndex > -1 && runIndex > confirmIndex, "退订执行必须在确认弹窗之后");
});
