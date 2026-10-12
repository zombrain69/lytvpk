// 工坊订阅清单（退订引导 · 只读）：打开模态框展示订阅、分类筛选、批量打开工坊页、
// 直达 Steam 官方退订页。
//
// 纪律：本模块**不执行任何退订或写入**。所有退订动作都在 Steam 官方页面上由用户完成。
// 数据来自后端 GetWorkshopSubscriptions（只读解析 550_subscriptions.vdf + 本地文件对照）。
// 安全：列表用 innerHTML/insertAdjacentHTML 渲染，所有动态字段都经 escapeHtmlText 转义
// 或来自本地 i18n 目录（t()，编译期固定文案）；不渲染任何未转义的远端数据（标题字段已转义）。
import { showNotification } from "../../core/toast.js";
import { t } from "../../core/i18n-runtime.js";
import { confirmInApp } from "../modals/confirm.js";
import { GetWorkshopSubscriptions, OpenURLInSteam } from "../../../../wailsjs/go/app/App";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import {
	SUBSCRIPTION_BADGE_KEYS,
	SUBSCRIPTION_FILTERS,
	UNSUBSCRIBE_GUIDE_URL,
	countSubscriptions,
	filterSubscriptions,
	formatSubscribedDate,
	subscriptionBadges,
	summarizeSubscriptionCounts,
	workshopItemURL,
} from "./subscription-audit-view.mjs";

const RENDER_CHUNK = 200;

let modalSessionId = 0;
let summary = null;
let activeFilter = "all";
let selectedIDs = new Set();
let renderedCount = 0;
let bound = false;

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
	renderLoading();

	let result = null;
	try {
		result = await GetWorkshopSubscriptions();
	} catch (error) {
		if (session !== modalSessionId) return;
		renderError(String(error));
		return;
	}
	if (session !== modalSessionId || modal.classList.contains("hidden")) return;
	summary = result || null;
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
			<button type="button" class="btn btn-small btn-outline" id="subscription-audit-clear-selected" disabled>
				${escapeHtmlText(t("toolbox.subscriptionAudit.batch.clear"))}
			</button>
		</div>
		<div id="subscription-audit-rows" class="subscription-audit-rows"></div>
		<div id="subscription-audit-more" class="subscription-audit-more"></div>
	`;

	renderFilters();
	renderRows(true);
	renderFooter();
	updateBatchBar();
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
	const size = selectedIDs.size;
	if (countEl) {
		countEl.textContent = t("toolbox.subscriptionAudit.batch.selected", { count: size });
	}
	if (openBtn) openBtn.disabled = size === 0;
	if (clearBtn) clearBtn.disabled = size === 0;
	if (selectAll) {
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
}
