// 工坊订阅清单（退订引导 + 一键退订）：打开模态框展示订阅、分类筛选、批量打开工坊页、
// 直达 Steam 官方退订页；配置好自备的 steam_api64.dll 后，可用 Steam 官方本机接口
// （ISteamUGC）直接「退订所选」/「重新订阅所选」。
//
// 纪律：
//  - 一键退订只在用户当次点击确认后执行（授权不跨会话）；后端在真正写之前会先导出回滚清单；
//  - 确认弹窗展示 ID + 标题 + 总数 + 不可逆说明 + 回退方式（结果面板「重新订阅本次退订」）；
//  - 未配置 DLL 时退订按钮不出现，只保留官方页面引导（零风险路径）；
//  - 批量执行期间界面只读进度事件，第二次执行会被后端互斥闸门拒绝。
// 数据来自后端 GetWorkshopSubscriptions / GetSteamUGCStatus（只读解析 550_subscriptions.vdf + 本地文件对照）。
// 安全：列表用 innerHTML/insertAdjacentHTML 渲染，所有动态字段都经 escapeHtmlText 转义
// 或来自本地 i18n 目录（t()，编译期固定文案）；不渲染任何未转义的远端数据（标题字段已转义）。
import { showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { confirmInApp, confirmHtmlInApp } from "../modals/confirm.js";
import {
	DetectSteamAPIDllCandidates,
	GetSteamUGCStatus,
	GetWorkshopSubscriptions,
	OpenURLInSteam,
	PickSteamAPIDllFile,
	SetSteamAPIDllPath,
	SubscribeSteamWorkshopItems,
	UnsubscribeSteamWorkshopItems,
} from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL, EventsOn } from "../../../../wailsjs/runtime/runtime";
import {
	SUBSCRIPTION_BADGE_KEYS,
	SUBSCRIPTION_FILTERS,
	UNSUBSCRIBE_GUIDE_URL,
	buildResubscribeConfirmHtml,
	buildUnsubscribeConfirmHtml,
	countSubscriptions,
	filterSubscriptions,
	formatSubscribedDate,
	subscriptionBadges,
	summarizeBatchResult,
	summarizeSubscriptionCounts,
	workshopItemURL,
} from "./subscription-audit-view.mjs";

const RENDER_CHUNK = 200;
// 批量结果面板最多逐条列出的条数（超出折叠为一行说明）。
const BATCH_RESULT_LIMIT = 60;

let modalSessionId = 0;
let summary = null;
let activeFilter = "all";
let selectedIDs = new Set();
let renderedCount = 0;
let bound = false;
// 一键退订状态：DLL 可用性 / 执行中 / 最近一次执行结果（跨弹窗开关保留，便于回看）。
let ugcStatus = null;
let ugcBusy = false;
let batchProgress = null; // { action, done, total }
let lastBatch = null; // 后端 SteamUGCBatchResult
let lastBatchTitles = null; // Map<id, title>：退订后订阅快照里标题会消失，结果区仍要显示
let progressBound = false;

function escapeHtmlText(value) {
	return String(value ?? "")
		.replaceAll("&", "&amp;")
		.replaceAll("<", "&lt;")
		.replaceAll(">", "&gt;")
		.replaceAll('"', "&quot;")
		.replaceAll("'", "&#39;");
}

function modalEl() {
	return document.getElementById("subscription-audit-modal");
}

function bodyEl() {
	return document.getElementById("subscription-audit-body");
}

function footerEl() {
	return document.getElementById("subscription-audit-footer");
}

/** openSubscriptionAuditModal 打开清单（每次打开都重新拉取最新数据）。 */
export async function openSubscriptionAuditModal() {
	const modal = modalEl();
	if (!modal) return;
	bindOnce();
	modal.classList.remove("hidden");
	const session = ++modalSessionId;
	activeFilter = "all";
	selectedIDs = new Set();
	renderedCount = 0;
	summary = null;
	ugcStatus = null;
	renderLoading();

	// 订阅清单与退订状态并行拉取；退订状态拿不到时界面退化为「无一键退订」（引导路径仍可用）。
	const [subscriptionsResult, statusResult] = await Promise.allSettled([
		GetWorkshopSubscriptions(),
		GetSteamUGCStatus(),
	]);
	if (session !== modalSessionId || modal.classList.contains("hidden")) return;

	if (subscriptionsResult.status !== "fulfilled") {
		renderError(String(subscriptionsResult.reason));
		return;
	}
	summary = subscriptionsResult.value || null;
	ugcStatus = statusResult.status === "fulfilled" ? statusResult.value || null : null;
	renderAll();
}

export function closeSubscriptionAuditModal() {
	modalSessionId += 1;
	modalEl()?.classList.add("hidden");
}

function renderLoading() {
	bodyEl().innerHTML = `<div class="subscription-audit-loading">${escapeHtmlText(t("toolbox.subscriptionAudit.loading"))}</div>`;
	footerEl().innerHTML = "";
}

function renderError(message) {
	bodyEl().innerHTML = `<div class="subscription-audit-empty">${escapeHtmlText(
		t("toolbox.subscriptionAudit.loadFailed", { error: message }),
	)}</div>`;
	footerEl().innerHTML = `
		<button type="button" class="btn btn-secondary" id="subscription-audit-close-btn">${escapeHtmlText(t("toolbox.subscriptionAudit.footer.close"))}</button>
		<button type="button" class="btn btn-primary" id="subscription-audit-retry-btn">${escapeHtmlText(t("toolbox.subscriptionAudit.retry"))}</button>`;
	footerEl()
		.querySelector("#subscription-audit-close-btn")
		?.addEventListener("click", closeSubscriptionAuditModal);
	footerEl()
		.querySelector("#subscription-audit-retry-btn")
		?.addEventListener("click", () => void openSubscriptionAuditModal());
}

function renderAll() {
	if (!summary?.found) {
		const reasonKey =
			summary?.reason === "no-steam"
				? "toolbox.subscriptionAudit.reason.noSteam"
				: "toolbox.subscriptionAudit.reason.noVdf";
		bodyEl().innerHTML = `<div class="subscription-audit-empty">${escapeHtmlText(t(reasonKey))}</div>`;
		footerEl().innerHTML = `<button type="button" class="btn btn-secondary" id="subscription-audit-close-btn">${escapeHtmlText(
			t("toolbox.subscriptionAudit.footer.close"),
		)}</button>`;
		footerEl()
			.querySelector("#subscription-audit-close-btn")
			?.addEventListener("click", closeSubscriptionAuditModal);
		return;
	}

	const counts = summarizeSubscriptionCounts(summary.entries);
	const accountLine = summary.accountId
		? `<div class="subscription-audit-account">${escapeHtmlText(
				t("toolbox.subscriptionAudit.accountLine", {
					accountId: summary.accountId,
					updated: formatSubscribedDate(summary.timeLastUpdated) || "—",
				}),
			)}</div>`
		: "";

	bodyEl().innerHTML = `
		<div class="subscription-audit-toolbar">
			<div class="subscription-audit-summary">${escapeHtmlText(
				t("toolbox.subscriptionAudit.summaryLine", {
					total: counts.total,
					downloaded: counts.downloaded,
					rootCopies: counts.rootCopies,
					missing: counts.missing,
					disabled: counts.disabled,
				}),
			)}</div>
			${accountLine}
		</div>
		${ugcPanelHtml()}
		<div class="subscription-audit-guide">
			<div class="subscription-audit-guide-title">${escapeHtmlText(t("toolbox.subscriptionAudit.guide.title"))}</div>
			<ul>
				<li>${escapeHtmlText(t("toolbox.subscriptionAudit.guide.step1"))}</li>
				<li>${escapeHtmlText(t("toolbox.subscriptionAudit.guide.step2"))}</li>
				<li>${escapeHtmlText(t("toolbox.subscriptionAudit.guide.step3"))}</li>
			</ul>
		</div>
		<div id="subscription-audit-filters" class="subscription-audit-filters"></div>
		<div class="subscription-audit-batch">
			<label class="subscription-audit-select-all">
				<input type="checkbox" id="subscription-audit-select-all">
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.selectAll"))}
			</label>
			<span id="subscription-audit-selected-count" class="subscription-audit-selected">${escapeHtmlText(
				t("toolbox.subscriptionAudit.batch.selected", { count: 0 }),
			)}</span>
			<button type="button" class="btn btn-small btn-primary" id="subscription-audit-open-selected" disabled>
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.openSelected"))}
			</button>
			${ugcActionButtonsHtml()}
			<button type="button" class="btn btn-small btn-outline" id="subscription-audit-clear-selected" disabled>
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.clear"))}
			</button>
		</div>
		<div id="subscription-audit-batch-result" class="subscription-batch-result"></div>
		<div id="subscription-audit-rows" class="subscription-audit-rows"></div>
		<div id="subscription-audit-more" class="subscription-audit-more"></div>
	`;

	renderFilters();
	renderRows(true);
	renderFooter();
	renderBatchResult();
	updateBatchBar();
}

// ugcPanelHtml 渲染「一键退订」设置区（DLL 状态 + 检测/指定/恢复按钮 + 许可说明）。
function ugcPanelHtml() {
	if (!ugcStatus) return "";
	const ready = Boolean(ugcStatus.available);
	const statusLine = ready
		? t(
				ugcStatus.source === "user"
					? "toolbox.subscriptionAudit.ugc.statusReadyUser"
					: "toolbox.subscriptionAudit.ugc.statusReadyAuto",
				{ path: ugcStatus.path || "" },
			)
		: ugcStatus.source === "user" && ugcStatus.hint === "path-missing"
			? t("toolbox.subscriptionAudit.ugc.statusPathMissing", { path: ugcStatus.path || "" })
			: t("toolbox.subscriptionAudit.ugc.statusMissing");
	const buttons = [
		`<button type="button" class="btn btn-small btn-outline" id="subscription-ugc-detect">${escapeHtmlText(
			t("toolbox.subscriptionAudit.ugc.btnDetect"),
		)}</button>`,
		`<button type="button" class="btn btn-small btn-outline" id="subscription-ugc-pick">${escapeHtmlText(
			t("toolbox.subscriptionAudit.ugc.btnPick"),
		)}</button>`,
	];
	if (ugcStatus.source === "user") {
		buttons.push(
			`<button type="button" class="btn btn-small btn-outline" id="subscription-ugc-reset">${escapeHtmlText(
				t("toolbox.subscriptionAudit.ugc.btnReset"),
			)}</button>`,
		);
	}
	return `
		<div class="subscription-ugc-panel${ready ? " is-ready" : ""}">
			<div class="subscription-ugc-head">
				<span class="subscription-ugc-title">${escapeHtmlText(t("toolbox.subscriptionAudit.ugc.title"))}</span>
				<span class="subscription-ugc-status${ready ? " is-ready" : ""}" title="${escapeHtmlText(
					ugcStatus.path || "",
				)}">${escapeHtmlText(statusLine)}</span>
			</div>
			<div class="subscription-ugc-actions">${buttons.join("")}</div>
			<div class="subscription-ugc-note">${escapeHtmlText(t("toolbox.subscriptionAudit.ugc.licenseNote"))}</div>
		</div>`;
}

// ugcActionButtonsHtml 渲染批量条里的「退订所选 / 重新订阅所选」（仅 DLL 可用时出现）。
function ugcActionButtonsHtml() {
	if (!ugcStatus?.available) return "";
	return `
			<button type="button" class="btn btn-small btn-danger" id="subscription-ugc-unsub" disabled>
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.unsubscribe"))}
			</button>
			<button type="button" class="btn btn-small btn-outline" id="subscription-ugc-resub" disabled>
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.resubscribe"))}
			</button>`;
}

function renderFilters() {
	const wrap = document.getElementById("subscription-audit-filters");
	if (!wrap) return;
	wrap.innerHTML = SUBSCRIPTION_FILTERS.map((filter) => {
		const count = countSubscriptions(summary.entries, filter.id);
		return `<button type="button" class="subscription-filter-chip${
			activeFilter === filter.id ? " is-active" : ""
		}" data-filter="${filter.id}">${escapeHtmlText(t(filter.labelKey))}<span class="subscription-filter-count">${count}</span></button>`;
	}).join("");
}

function filteredRows() {
	return filterSubscriptions(summary?.entries || [], activeFilter);
}

function rowHtml(entry) {
	const id = String(entry.publishedFileId || "");
	const title = String(entry.title || "").trim();
	const badges = subscriptionBadges(entry)
		.map(
			(key) =>
				`<span class="subscription-badge is-${key}">${escapeHtmlText(t(SUBSCRIPTION_BADGE_KEYS[key]))}</span>`,
		)
		.join("");
	const date = formatSubscribedDate(entry.timeSubscribed);
	const metaParts = [`ID ${escapeHtmlText(id)}`];
	if (date) metaParts.push(escapeHtmlText(t("toolbox.subscriptionAudit.subscribedAt", { date })));
	return `<div class="subscription-row" data-row-id="${escapeHtmlText(id)}">
		<label class="subscription-check">
			<input type="checkbox" data-sub-id="${escapeHtmlText(id)}"${selectedIDs.has(id) ? " checked" : ""}>
		</label>
		<div class="subscription-row-main">
			<div class="subscription-row-title">
				<span class="subscription-row-name">${escapeHtmlText(
					title || t("toolbox.subscriptionAudit.untitled"),
				)}</span>
				${badges}
			</div>
			<div class="subscription-row-meta">${metaParts.join(" · ")}</div>
		</div>
		<button type="button" class="btn btn-small btn-outline subscription-open-btn" data-open-id="${escapeHtmlText(id)}">
			${escapeHtmlText(t("toolbox.subscriptionAudit.row.open"))}
		</button>
	</div>`;
}

function renderRows(reset = false) {
	const rowsEl = document.getElementById("subscription-audit-rows");
	const moreEl = document.getElementById("subscription-audit-more");
	if (!rowsEl || !moreEl) return;
	const rows = filteredRows();
	if (reset) {
		renderedCount = 0;
		rowsEl.innerHTML = "";
	}
	const slice = rows.slice(renderedCount, renderedCount + RENDER_CHUNK);
	rowsEl.insertAdjacentHTML("beforeend", slice.map(rowHtml).join(""));
	renderedCount += slice.length;

	const rest = rows.length - renderedCount;
	if (rows.length === 0) {
		moreEl.innerHTML = `<div class="subscription-audit-empty-inline">${escapeHtmlText(
			t("toolbox.subscriptionAudit.emptyFiltered"),
		)}</div>`;
		return;
	}
	moreEl.innerHTML =
		rest > 0
			? `<button type="button" class="btn btn-outline" id="subscription-audit-more-btn">${escapeHtmlText(
					t("toolbox.subscriptionAudit.more", { count: rest }),
				)}</button>`
			: "";
}

function renderFooter() {
	footerEl().innerHTML = `
		<button type="button" class="btn btn-outline" id="subscription-audit-open-browser">${escapeHtmlText(
			t("toolbox.subscriptionAudit.footer.openBrowser"),
		)}</button>
		<button type="button" class="btn btn-primary" id="subscription-audit-open-steam">${escapeHtmlText(
			t("toolbox.subscriptionAudit.footer.openSteam"),
		)}</button>`;
	footerEl()
		.querySelector("#subscription-audit-open-browser")
		?.addEventListener("click", () => BrowserOpenURL(UNSUBSCRIBE_GUIDE_URL));
	footerEl()
		.querySelector("#subscription-audit-open-steam")
		?.addEventListener("click", async () => {
			try {
				await OpenURLInSteam(UNSUBSCRIBE_GUIDE_URL);
			} catch (error) {
				// Steam 协议调用失败时退回系统浏览器，不让用户卡住。
				BrowserOpenURL(UNSUBSCRIBE_GUIDE_URL);
				showNotification(String(error), "warning");
			}
		});
}

function updateBatchBar() {
	const countEl = document.getElementById("subscription-audit-selected-count");
	const openBtn = document.getElementById("subscription-audit-open-selected");
	const clearBtn = document.getElementById("subscription-audit-clear-selected");
	const selectAll = document.getElementById("subscription-audit-select-all");
	const unsubBtn = document.getElementById("subscription-ugc-unsub");
	const resubBtn = document.getElementById("subscription-ugc-resub");
	const size = selectedIDs.size;
	if (countEl) {
		countEl.textContent = batchProgress
			? t("toolbox.subscriptionAudit.batch.executing", { done: batchProgress.done, total: batchProgress.total })
			: t("toolbox.subscriptionAudit.batch.selected", { count: size });
	}
	if (openBtn) openBtn.disabled = size === 0 || ugcBusy;
	if (clearBtn) clearBtn.disabled = size === 0 || ugcBusy;
	if (unsubBtn) unsubBtn.disabled = size === 0 || ugcBusy;
	if (resubBtn) resubBtn.disabled = size === 0 || ugcBusy;
	if (selectAll) {
		selectAll.disabled = ugcBusy;
		const rows = filteredRows();
		const selectedInFilter = rows.filter((entry) => selectedIDs.has(String(entry.publishedFileId || ""))).length;
		selectAll.checked = rows.length > 0 && selectedInFilter === rows.length;
		selectAll.indeterminate = selectedInFilter > 0 && selectedInFilter < rows.length;
	}
}

async function openSelectedItems() {
	const ids = [...selectedIDs];
	if (ids.length === 0) return;
	if (ids.length > 10) {
		const confirmed = await confirmInApp(
			t("toolbox.subscriptionAudit.batch.confirmOpen", { count: ids.length }),
		);
		if (!confirmed) return;
	}
	for (const id of ids) {
		BrowserOpenURL(workshopItemURL(id));
	}
	showNotification(t("toolbox.subscriptionAudit.batch.opened", { count: ids.length }), "success");
}

// selectedEntries 按订阅清单顺序返回当前所选的条目（选择可跨筛选）。
function selectedEntries() {
	const map = new Map((summary?.entries || []).map((entry) => [String(entry.publishedFileId || ""), entry]));
	const entries = [];
	for (const id of selectedIDs) {
		const entry = map.get(id);
		if (entry) entries.push(entry);
	}
	return entries;
}

// unsubscribeSelected 一键退订入口：先弹清单确认（ID + 标题 + 总数 + 不可逆说明），
// 用户当次确认后才执行；授权不跨会话（每次都要重新确认）。
async function unsubscribeSelected() {
	const entries = selectedEntries();
	if (entries.length === 0) return;
	const confirmed = await confirmHtmlInApp(
		t("toolbox.subscriptionAudit.ugc.unsubConfirmTitle", { count: entries.length }),
		buildUnsubscribeConfirmHtml(entries),
		"subscription-unsub-confirm",
	);
	if (!confirmed) return;
	await runBatch("unsubscribe", entries);
}

// resubscribeSelected 回滚入口：重新订阅所选（对已订阅的条目后端会自动跳过）。
async function resubscribeSelected() {
	const entries = selectedEntries();
	if (entries.length === 0) return;
	const confirmed = await confirmHtmlInApp(
		t("toolbox.subscriptionAudit.ugc.resubConfirmTitle", { count: entries.length }),
		buildResubscribeConfirmHtml(entries),
		"subscription-unsub-confirm",
	);
	if (!confirmed) return;
	await runBatch("subscribe", entries);
}

// resubscribeLastBatch 回滚入口（结果面板）：退订成功后条目会随刷新离开订阅清单，
// 选择集里再也选不到它们——所以「把本次退订的条目订阅回来」必须以结果面板为入口
// （标题来自 lastBatchTitles；退订后订阅快照里已经拿不到）。
async function resubscribeLastBatch() {
	if (!lastBatch || lastBatch.action !== "unsubscribe") return;
	const entries = (Array.isArray(lastBatch.results) ? lastBatch.results : [])
		.filter((item) => item?.ok && !item?.skipped)
		.map((item) => {
			const id = String(item?.publishedFileId || "");
			return {
				publishedFileId: id,
				title: String(item?.title || "").trim() || lastBatchTitles?.get(id) || "",
			};
		})
		.filter((entry) => entry.publishedFileId !== "");
	if (entries.length === 0) return;
	const confirmed = await confirmHtmlInApp(
		t("toolbox.subscriptionAudit.ugc.resubConfirmTitle", { count: entries.length }),
		buildResubscribeConfirmHtml(entries),
		"subscription-unsub-confirm",
	);
	if (!confirmed) return;
	await runBatch("subscribe", entries);
}

// runBatch 执行一轮批量操作：置忙（禁用按钮）→ 调后端 → 刷新清单 → 渲染逐条结果。
// 进度来自后端事件 steam_ugc_progress（done/total）。
async function runBatch(action, entries) {
	if (ugcBusy) return;
	ugcBusy = true;
	batchProgress = { action, done: 0, total: entries.length };
	lastBatch = null;
	lastBatchTitles = new Map(entries.map((entry) => [String(entry.publishedFileId || ""), String(entry.title || "")]));
	updateBatchBar();
	try {
		const ids = entries.map((entry) => String(entry.publishedFileId || ""));
		const result =
			action === "unsubscribe"
				? await UnsubscribeSteamWorkshopItems(ids)
				: await SubscribeSteamWorkshopItems(ids);
		lastBatch = result || null;
		const counts = summarizeBatchResult(result);
		showNotification(
			t("toolbox.subscriptionAudit.ugc.batchDone", {
				ok: counts.succeeded,
				skipped: counts.skipped,
				failed: counts.failed,
			}),
			counts.failed > 0 ? "warning" : "success",
		);
		selectedIDs = new Set();
		await refreshSummary();
	} catch (error) {
		showNotification(String(error), "error");
	} finally {
		ugcBusy = false;
		batchProgress = null;
		renderAll();
	}
}

// refreshSummary 批量执行后重新拉取订阅清单（VDF 快照会很快同步；失败则保留旧数据）。
async function refreshSummary() {
	const session = modalSessionId;
	try {
		const result = await GetWorkshopSubscriptions();
		if (session !== modalSessionId) return;
		summary = result || summary;
	} catch (error) {
		// 刷新失败不阻塞结果展示：旧清单 + 结果区已经能说明发生了什么。
	}
}

// renderBatchResult 渲染「最近一次执行结果」面板（含逐条结果与回滚清单路径）。
function renderBatchResult() {
	const panel = document.getElementById("subscription-audit-batch-result");
	if (!panel) return;
	if (!lastBatch) {
		panel.innerHTML = "";
		return;
	}
	const counts = summarizeBatchResult(lastBatch);
	const results = Array.isArray(lastBatch.results) ? lastBatch.results : [];
	const shown = results.slice(0, BATCH_RESULT_LIMIT);
	const rows = shown
		.map((item) => {
			const id = String(item?.publishedFileId || "");
			const title = String(item?.title || "").trim() || lastBatchTitles?.get(id) || "";
			const stateClass = !item?.ok ? "is-fail" : item?.skipped ? "is-skip" : "is-ok";
			const stateText = !item?.ok
				? t("toolbox.subscriptionAudit.ugc.itemFailed")
				: item?.skipped
					? t("toolbox.subscriptionAudit.ugc.itemSkipped")
					: t("toolbox.subscriptionAudit.ugc.itemOk");
			return `<div class="subscription-batch-row ${stateClass}">
				<span class="subscription-batch-state">${escapeHtmlText(stateText)}</span>
				<code>${escapeHtmlText(id)}</code>
				<span class="subscription-batch-title">${escapeHtmlText(title)}</span>
				<span class="subscription-batch-detail">${escapeHtmlText(item?.detail || "")}</span>
			</div>`;
		})
		.join("");
	const more =
		results.length > shown.length
			? `<div class="subscription-batch-more">${escapeHtmlText(
					t("toolbox.subscriptionAudit.ugc.resultMore", { count: results.length - shown.length }),
				)}</div>`
			: "";
	const backupLine = lastBatch.backupPath
		? `<div class="subscription-batch-backup">${escapeHtmlText(
				t("toolbox.subscriptionAudit.ugc.backupLine", { path: lastBatch.backupPath }),
			)}</div>`
		: "";
	const resubAllCount = lastBatch.action === "unsubscribe"
		? (Array.isArray(lastBatch.results) ? lastBatch.results : []).filter((item) => item?.ok && !item?.skipped).length
		: 0;
	const resubAllButton = resubAllCount > 0
		? `<button type="button" class="btn btn-small btn-outline" id="subscription-batch-resub-all">${escapeHtmlText(
				t("toolbox.subscriptionAudit.ugc.resubLastBatch", { count: resubAllCount }),
			)}</button>`
		: "";
	panel.innerHTML = `
		<div class="subscription-batch-head">
			<span class="subscription-batch-title-main">${escapeHtmlText(t("toolbox.subscriptionAudit.ugc.resultTitle"))}</span>
			<div class="subscription-batch-head-actions">
				${resubAllButton}
				<button type="button" class="btn btn-small btn-outline" id="subscription-batch-result-close">
					${escapeHtmlText(t("toolbox.subscriptionAudit.ugc.resultClose"))}
				</button>
			</div>
		</div>
		<div class="subscription-batch-summary">${escapeHtmlText(
			t("toolbox.subscriptionAudit.ugc.batchDone", {
				ok: counts.succeeded,
				skipped: counts.skipped,
				failed: counts.failed,
			}),
		)}</div>
		${backupLine}
		<div class="subscription-batch-rows">${rows}</div>
		${more}`;
}

// detectDll 检测本机 Steam 游戏目录里的 steam_api64.dll；找到首个后设为自定义路径。
async function detectDll() {
	try {
		const candidates = await DetectSteamAPIDllCandidates();
		const first = String(candidates?.[0]?.path || "");
		if (!first) return;
		const status = await SetSteamAPIDllPath(first);
		ugcStatus = status || ugcStatus;
		showNotification(
			t("toolbox.subscriptionAudit.ugc.detected", { count: candidates.length, path: first }),
			"success",
		);
		renderAll();
	} catch (error) {
		showNotification(String(error), "warning");
	}
}

// pickDll 手动指定一份 steam_api64.dll（系统文件选择器）。
async function pickDll() {
	try {
		const path = await PickSteamAPIDllFile();
		if (!path) return; // 用户取消
		const status = await SetSteamAPIDllPath(path);
		ugcStatus = status || ugcStatus;
		showNotification(t("toolbox.subscriptionAudit.ugc.manualSet", { path }), "success");
		renderAll();
	} catch (error) {
		showNotification(String(error), "error");
	}
}

// resetDll 清除自定义路径，恢复本机自动检测。
async function resetDll() {
	try {
		const status = await SetSteamAPIDllPath("");
		ugcStatus = status || ugcStatus;
		showNotification(t("toolbox.subscriptionAudit.ugc.cleared"), "info");
		renderAll();
	} catch (error) {
		showNotification(String(error), "error");
	}
}

function onBodyClick(event) {
	const target = event.target;
	if (!(target instanceof Element)) return;

	const openBtn = target.closest("[data-open-id]");
	if (openBtn) {
		BrowserOpenURL(workshopItemURL(openBtn.getAttribute("data-open-id")));
		return;
	}
	const filterChip = target.closest("[data-filter]");
	if (filterChip) {
		activeFilter = filterChip.getAttribute("data-filter") || "all";
		renderFilters();
		renderRows(true);
		updateBatchBar();
		return;
	}
	if (target.closest("#subscription-audit-more-btn")) {
		renderRows(false);
		return;
	}
	if (target.closest("#subscription-audit-open-selected")) {
		void openSelectedItems();
		return;
	}
	if (target.closest("#subscription-ugc-detect")) {
		void detectDll();
		return;
	}
	if (target.closest("#subscription-ugc-pick")) {
		void pickDll();
		return;
	}
	if (target.closest("#subscription-ugc-reset")) {
		void resetDll();
		return;
	}
	if (target.closest("#subscription-ugc-unsub")) {
		void unsubscribeSelected();
		return;
	}
	if (target.closest("#subscription-ugc-resub")) {
		void resubscribeSelected();
		return;
	}
	if (target.closest("#subscription-batch-resub-all")) {
		void resubscribeLastBatch();
		return;
	}
	if (target.closest("#subscription-batch-result-close")) {
		lastBatch = null;
		renderBatchResult();
		return;
	}
	if (target.closest("#subscription-audit-clear-selected")) {
		selectedIDs = new Set();
		for (const box of bodyEl().querySelectorAll("input[data-sub-id]")) {
			box.checked = false;
		}
		updateBatchBar();
		return;
	}
}

function onBodyChange(event) {
	const target = event.target;
	if (!(target instanceof HTMLInputElement)) return;
	if (target.id === "subscription-audit-select-all") {
		const rows = filteredRows();
		for (const entry of rows) {
			const id = String(entry.publishedFileId || "");
			if (target.checked) selectedIDs.add(id);
			else selectedIDs.delete(id);
		}
		for (const box of bodyEl().querySelectorAll("input[data-sub-id]")) {
			box.checked = selectedIDs.has(box.getAttribute("data-sub-id") || "");
		}
		updateBatchBar();
		return;
	}
	const id = target.getAttribute("data-sub-id");
	if (id) {
		if (target.checked) selectedIDs.add(id);
		else selectedIDs.delete(id);
		updateBatchBar();
	}
}

function bindOnce() {
	if (bound) return;
	bound = true;
	document.getElementById("close-subscription-audit-btn")?.addEventListener("click", closeSubscriptionAuditModal);
	modalEl()?.addEventListener("click", (event) => {
		if (event.target === modalEl()) closeSubscriptionAuditModal();
	});
	bodyEl()?.addEventListener("click", onBodyClick);
	bodyEl()?.addEventListener("change", onBodyChange);
	bindProgressOnce();
}

// bindProgressOnce 订阅后端的批量进度事件（steam_ugc_progress：done/total/item），
// 执行中刷新计数文案；只有执行中才响应，避免事件穿透到未打开的弹窗。
function bindProgressOnce() {
	if (progressBound) return;
	progressBound = true;
	EventsOn("steam_ugc_progress", (payload) => {
		if (!ugcBusy || !payload) return;
		batchProgress = {
			action: String(payload.action || ""),
			done: Number(payload.done) || 0,
			total: Number(payload.total) || 0,
		};
		updateBatchBar();
	});
}
