import logoUrl from "../../../assets/images/logo.png";
import { t } from "../../core/i18n-runtime.js";

const ABOUT_LINKS = {
  repo: "https://github.com/zombrain69/lytvpk",
  license: "https://github.com/zombrain69/lytvpk/blob/master/LICENSE",
  issue: "https://github.com/zombrain69/lytvpk/issues/new",
  upstream: "https://github.com/LaoYutang/lytvpk",
};

export async function renderAboutPage({
  BrowserOpenURL,
  GetAppVersion,
  CheckUpdate,
  showUpdateModal,
  GetCrashReportDirectory,
  ListCrashReports,
  OpenFileLocation,
} = {}) {
  const container = document.getElementById("about-page-content");
  if (!container) return;

  container.innerHTML = `
    <div class="about-page-shell">
      <section class="about-hero">
        <div class="about-logo-mark">
          <img src="${logoUrl}" alt="" />
        </div>
        <div class="about-hero-copy">
          <div class="about-kicker">${t("about.hero.kicker")}</div>
          <h2>${t("about.hero.title")}</h2>
          <p>${t("about.hero.description")}</p>
        </div>
      </section>

      <section class="about-grid" aria-label="${t("about.info.ariaLabel")}">
        <div class="about-info-panel">
          <div class="about-panel-title">${t("about.info.title")}</div>
          <dl class="about-meta-list">
            <div>
              <dt>${t("about.info.item.upstreamAuthor")}</dt>
              <dd>LaoYutang</dd>
            </div>
            <div>
              <dt>${t("about.info.item.forkMaintainer")}</dt>
              <dd>zombrain69</dd>
            </div>
            <div>
              <dt>${t("about.info.item.license")}</dt>
              <dd>GPL-3.0-only</dd>
            </div>
            <div>
              <dt>${t("about.info.item.repo")}</dt>
              <dd>zombrain69/lytvpk</dd>
            </div>
            <div>
              <dt>${t("about.info.item.issues")}</dt>
              <dd>GitHub Issues</dd>
            </div>
            <div>
              <dt>${t("about.info.item.version")}</dt>
              <dd id="about-current-version">${t("about.version.detecting")}</dd>
            </div>
          </dl>
        </div>

        <div class="about-info-panel about-action-panel">
          <div class="about-panel-title">${t("about.action.title")}</div>
          <div class="about-actions">
            <button class="about-action-btn primary" type="button" data-about-url="${ABOUT_LINKS.repo}">
              <span>${t("about.action.openRepo")}</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.upstream}">
              <span>${t("about.action.openUpstream")}</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.license}">
              <span>${t("about.action.openLicense")}</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.issue}">
              <span>${t("about.action.feedback")}</span>
            </button>
            <button id="about-check-update-btn" class="about-action-btn" type="button">
              <span>${t("about.action.checkUpdate")}</span>
            </button>
          </div>
          <div id="about-update-status" class="about-update-status" aria-live="polite"></div>
        </div>

        <div class="about-info-panel about-action-panel">
          <div class="about-panel-title">${t("about.crash.title")}</div>
          <p class="about-panel-desc">${t("about.crash.description")}</p>
          <div class="about-actions">
            <button id="about-open-crash-dir-btn" class="about-action-btn" type="button">
              <span>${t("about.crash.openDirectory")}</span>
            </button>
          </div>
          <div id="about-crash-status" class="about-update-status" aria-live="polite"></div>
        </div>
      </section>
    </div>
  `;

  bindAboutActions({
    BrowserOpenURL,
    CheckUpdate,
    showUpdateModal,
    GetCrashReportDirectory,
    ListCrashReports,
    OpenFileLocation,
  });

  await hydrateVersion(GetAppVersion);
}

function bindAboutActions({
  BrowserOpenURL,
  CheckUpdate,
  showUpdateModal,
  GetCrashReportDirectory,
  ListCrashReports,
  OpenFileLocation,
} = {}) {
  document.querySelectorAll("#about-page-content [data-about-url]").forEach((button) => {
    button.addEventListener("click", () => {
      const url = button.dataset.aboutUrl;
      if (!url) return;
      if (typeof BrowserOpenURL === "function") {
        BrowserOpenURL(url);
      } else {
        window.open(url, "_blank", "noopener,noreferrer");
      }
    });
  });

  document.getElementById("about-check-update-btn")?.addEventListener("click", async () => {
    await checkAboutUpdate({ CheckUpdate, showUpdateModal });
  });

  // 崩溃报告：只在本机，用户可自行打开目录查看/删除。
  const crashStatus = document.getElementById("about-crash-status");
  void (async () => {
    if (!crashStatus) return;
    // 静默 return 会让"绑定没注入"看起来像"功能正常但没数据"，
    // 这里必须把缺失本身显示出来，方便一眼定位。
    if (typeof ListCrashReports !== "function") {
      crashStatus.textContent = t("about.crash.bindingsMissing");
      console.warn("ListCrashReports 未注入，跳过崩溃报告统计");
      return;
    }
    try {
      const reports = (await ListCrashReports()) || [];
      crashStatus.textContent =
        reports.length === 0
          ? t("about.crash.empty")
          : t("about.crash.count", { count: reports.length, latest: reports[0]?.createdAt || t("about.crash.unknownTime") });
    } catch (error) {
      crashStatus.textContent = t("about.crash.readFailed", { error: String(error?.message || error) });
    }
  })();
  document.getElementById("about-open-crash-dir-btn")?.addEventListener("click", async () => {
    if (typeof GetCrashReportDirectory !== "function" || typeof OpenFileLocation !== "function") {
      if (crashStatus) crashStatus.textContent = t("about.crash.bindingsMissing");
      console.warn("崩溃报告目录接口未注入", {
        GetCrashReportDirectory: typeof GetCrashReportDirectory,
        OpenFileLocation: typeof OpenFileLocation,
      });
      return;
    }
    try {
      const dir = await GetCrashReportDirectory();
      if (!dir) {
        if (crashStatus) crashStatus.textContent = t("about.crash.noConfigDir");
        return;
      }
      await OpenFileLocation(dir);
    } catch (error) {
      if (crashStatus) crashStatus.textContent = t("about.crash.openFailed", { error: String(error?.message || error) });
    }
  });
}

async function hydrateVersion(GetAppVersion) {
  const versionEl = document.getElementById("about-current-version");
  if (!versionEl || typeof GetAppVersion !== "function") return;

  try {
    const version = await GetAppVersion();
    versionEl.textContent = version ? `v${version}` : t("about.version.unknown");
  } catch (error) {
    versionEl.textContent = t("about.version.fetchFailed");
  }
}

async function checkAboutUpdate({ CheckUpdate, showUpdateModal } = {}) {
  const button = document.getElementById("about-check-update-btn");
  const status = document.getElementById("about-update-status");
  const versionEl = document.getElementById("about-current-version");
  if (!button || !status || typeof CheckUpdate !== "function") return;

  button.disabled = true;
  button.querySelector("span").textContent = t("about.action.checking");
  status.className = "about-update-status";
  status.textContent = "";

  try {
    const info = await CheckUpdate();
    if (versionEl && info.current_ver) {
      versionEl.textContent = `v${info.current_ver}`;
    }

    if (info.error) {
      status.textContent = t("about.update.failed", { error: info.error });
      status.classList.add("error");
    } else if (info.has_update) {
      status.textContent = t("about.update.discovered", { version: info.latest_ver });
      status.classList.add("success");
      if (typeof showUpdateModal === "function") {
        showUpdateModal(info);
      }
    } else {
      status.textContent = t("about.update.upToDate", { version: info.latest_ver || info.current_ver });
      status.classList.add("success");
      if (info.release_note && typeof showUpdateModal === "function") {
        showUpdateModal(info);
      }
    }
  } catch (error) {
    status.textContent = t("about.update.error", { error });
    status.classList.add("error");
  } finally {
    button.disabled = false;
    button.querySelector("span").textContent = t("about.action.checkUpdate");
  }
}
