import logoUrl from "../../../assets/images/logo.png";

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
          <div class="about-kicker">Left 4 Dead 2 MOD Manager</div>
          <h2>LytVPK Community Fork</h2>
          <p>基于 LaoYutang/lytvpk 的非官方修改版。更新、源码和反馈均由本 Fork 维护；上游链接只用于出处与致谢。</p>
        </div>
      </section>

      <section class="about-grid" aria-label="应用信息">
        <div class="about-info-panel">
          <div class="about-panel-title">项目信息</div>
          <dl class="about-meta-list">
            <div>
              <dt>上游作者</dt>
              <dd>LaoYutang</dd>
            </div>
            <div>
              <dt>Fork 维护</dt>
              <dd>zombrain69</dd>
            </div>
            <div>
              <dt>开源协议</dt>
              <dd>GPL-3.0-only</dd>
            </div>
            <div>
              <dt>更新与源码</dt>
              <dd>zombrain69/lytvpk</dd>
            </div>
            <div>
              <dt>问题反馈</dt>
              <dd>GitHub Issues</dd>
            </div>
            <div>
              <dt>当前版本</dt>
              <dd id="about-current-version">检测中...</dd>
            </div>
          </dl>
        </div>

        <div class="about-info-panel about-action-panel">
          <div class="about-panel-title">操作</div>
          <div class="about-actions">
            <button class="about-action-btn primary" type="button" data-about-url="${ABOUT_LINKS.repo}">
              <span>打开 GitHub 仓库</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.upstream}">
              <span>查看上游项目</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.license}">
              <span>查看开源协议</span>
            </button>
            <button class="about-action-btn" type="button" data-about-url="${ABOUT_LINKS.issue}">
              <span>问题反馈</span>
            </button>
            <button id="about-check-update-btn" class="about-action-btn" type="button">
              <span>检查更新</span>
            </button>
          </div>
          <div id="about-update-status" class="about-update-status" aria-live="polite"></div>
        </div>

        <div class="about-info-panel about-action-panel">
          <div class="about-panel-title">崩溃报告</div>
          <p class="about-panel-desc">
            本应用捕获自己的 panic 与前端未处理异常，写成**只保存在本机**的报告
            （版本、堆栈、日志尾部），不会联网上传。游戏/系统的原生崩溃转储仍用
            “工具箱 → 崩溃转储查看器”打开。
          </p>
          <div class="about-actions">
            <button id="about-open-crash-dir-btn" class="about-action-btn" type="button">
              <span>打开崩溃报告目录</span>
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
    if (typeof ListCrashReports !== "function" || !crashStatus) return;
    try {
      const reports = (await ListCrashReports()) || [];
      crashStatus.textContent =
        reports.length === 0
          ? "本机还没有崩溃报告"
          : `本机有 ${reports.length} 份崩溃报告（最新：${reports[0]?.createdAt || "未知时间"}）`;
    } catch (error) {
      crashStatus.textContent = "读取崩溃报告失败: " + String(error?.message || error);
    }
  })();
  document.getElementById("about-open-crash-dir-btn")?.addEventListener("click", async () => {
    if (typeof GetCrashReportDirectory !== "function" || typeof OpenFileLocation !== "function") return;
    try {
      const dir = await GetCrashReportDirectory();
      if (!dir) {
        if (crashStatus) crashStatus.textContent = "未配置配置目录，暂无可打开的崩溃报告目录";
        return;
      }
      await OpenFileLocation(dir);
    } catch (error) {
      if (crashStatus) crashStatus.textContent = "打开崩溃报告目录失败: " + String(error?.message || error);
    }
  });
}

async function hydrateVersion(GetAppVersion) {
  const versionEl = document.getElementById("about-current-version");
  if (!versionEl || typeof GetAppVersion !== "function") return;

  try {
    const version = await GetAppVersion();
    versionEl.textContent = version ? `v${version}` : "未知";
  } catch (error) {
    versionEl.textContent = "获取失败";
  }
}

async function checkAboutUpdate({ CheckUpdate, showUpdateModal } = {}) {
  const button = document.getElementById("about-check-update-btn");
  const status = document.getElementById("about-update-status");
  const versionEl = document.getElementById("about-current-version");
  if (!button || !status || typeof CheckUpdate !== "function") return;

  button.disabled = true;
  button.querySelector("span").textContent = "检查中...";
  status.className = "about-update-status";
  status.textContent = "";

  try {
    const info = await CheckUpdate();
    if (versionEl && info.current_ver) {
      versionEl.textContent = `v${info.current_ver}`;
    }

    if (info.error) {
      status.textContent = `检查失败: ${info.error}`;
      status.classList.add("error");
    } else if (info.has_update) {
      status.textContent = `发现新版本 v${info.latest_ver}`;
      status.classList.add("success");
      if (typeof showUpdateModal === "function") {
        showUpdateModal(info);
      }
    } else {
      status.textContent = `当前已是最新版本 v${info.latest_ver || info.current_ver}`;
      status.classList.add("success");
      if (info.release_note && typeof showUpdateModal === "function") {
        showUpdateModal(info);
      }
    }
  } catch (error) {
    status.textContent = `发生错误: ${error}`;
    status.classList.add("error");
  } finally {
    button.disabled = false;
    button.querySelector("span").textContent = "检查更新";
  }
}
