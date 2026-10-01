// 界面性能探针：在真实库上量"点按钮 / 开窗口 / 搜索 / 大结果集重画"的耗时与长任务。
//
// 用法（先按 docs/development/manual-verification.md §0 起沙箱，端口与 -Port 一致）：
//
//   pwsh -File scripts/devtools/build-cua.ps1 -OutName LytVPK-Community-Fork-cua.exe
//   pwsh -File scripts/devtools/launch-cua-sandbox.ps1 -Port 38999
//   python scripts/devtools/cua-eval.py --base http://127.0.0.1:38999 --file scripts/devtools/measure-ui-perf.js
//
// 只读：只派发真实事件、读 DOM 与 PerformanceObserver，不改任何文件。
// 唯一会写盘的用例是"点游戏内开关"（只改沙箱里的 addonlist.txt），
// 所以请在只读沙箱里跑 —— 不要对着真实库点。
//
// 判据看两个数：**长任务（longtask）**与"首屏可见时间"。长任务 > 100ms 就是用户
// 能明显感到的卡顿；本项目的历史基线是"清空搜索一次 519ms、开分组窗口 4.3s"。
(async () => {
  const out = {};
  const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
  const cards = () => document.querySelectorAll("#file-list .file-card").length;
  const hit = () => (document.getElementById("search-hit-count")?.textContent || "").trim();

  const wait = async (check, timeoutMs) => {
    const deadline = Date.now() + timeoutMs;
    while (Date.now() < deadline) {
      if (check()) return true;
      await sleep(20);
    }
    return false;
  };

  const measure = async (action, done, { expectLongTasks = true } = {}) => {
    const tasks = [];
    const observer = new PerformanceObserver((list) => {
      list.getEntries().forEach((entry) => tasks.push(Math.round(entry.duration)));
    });
    observer.observe({ entryTypes: ["longtask"] });
    const t0 = performance.now();
    await action();
    const ok = await done();
    const ms = Math.round(performance.now() - t0);
    observer.disconnect();
    return { ok, ms, longTasks: tasks, note: expectLongTasks ? undefined : "等待条件未满足" };
  };

  // ① 输入延迟：每个字符到下一帧（>100ms 就是打字卡）。
  //    先等列表补齐完，量的是"空闲打字"；正在补齐时的打字延迟见 ② 的 framesOver100ms。
  const input = document.getElementById("search-input");
  if (input) {
    let settleLast = -1;
    let settleRounds = 0;
    while (settleRounds < 4) {
      const now = cards();
      settleRounds = now === settleLast ? settleRounds + 1 : 0;
      settleLast = now;
      await sleep(200);
    }
    const typing = [];
    let typed = "";
    for (const ch of "材质包") {
      typed += ch;
      const t0 = performance.now();
      input.value = typed;
      input.dispatchEvent(new Event("input", { bubbles: true }));
      await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)));
      typing.push(Math.round(performance.now() - t0));
    }
    out.typingMsPerChar = typing;
  }

  // ② 大结果集重画：先搜到小结果集，再清空（2904 张卡）—— 最卡的路径。
  if (input) {
    const hitBefore = hit();
    input.value = "材质";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await wait(() => hit() !== hitBefore, 20000);
    // 等小结果集**渲染完**再取基准：否则会把上一轮的补齐算进这一轮。
    let lastCount = -1;
    let stable = 0;
    while (stable < 4) {
      const now = cards();
      stable = now === lastCount ? stable + 1 : 0;
      lastCount = now;
      await sleep(120);
    }
    const from = cards();
    const frames = [];
    let running = true;
    const frameLoop = () => {
      if (!running) return;
      const started = performance.now();
      requestAnimationFrame(() => {
        frames.push(Math.round(performance.now() - started));
        frameLoop();
      });
    };
    frameLoop();
    const clearBefore = hit();
    const t0 = performance.now();
    input.value = "";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await wait(() => hit() === hitBefore, 20000);
    const firstContentMs = Math.round(performance.now() - t0);
    // 等卡片数稳定（分帧补齐完成）——不能只看"大于某个数"。
    // 注意：窗口最小化时 rAF 会被暂停，探针要按"稳定 6 次"来判完成，不要写死时间。
    lastCount = -1;
    stable = 0;
    while (stable < 6) {
      const now = cards();
      stable = now === lastCount ? stable + 1 : 0;
      lastCount = now;
      await sleep(200);
      // 注意：t0 来自 performance.now()，这里不能用 Date.now() 比较（量纲不同，
      // 第一次就会 break —— 本探针第一版就这么错过"补齐到底用了多久"）。
      if (performance.now() - t0 > 30000) break;
    }
    running = false;
    out.clearSearch = {
      from,
      to: cards(),
      firstContentMs,
      fullMs: Math.round(performance.now() - t0),
      maxFrameGapMs: frames.length ? Math.max(...frames) : null,
      framesOver100ms: frames.filter((ms) => ms > 100).length,
    };
  }

  // ③ 开「策略组管理」窗口（真机历史 4265ms）。
  //    先把上一次留下的窗口内容清空并关掉窗口，否则"行还在 DOM 里"会让等待条件
  //    立刻成立，量到的是 0ms（这是本探针第一版踩过的坑）。
  document.getElementById("strategy-group-close-btn")?.click();
  document.getElementById("strategy-group-list")?.replaceChildren();
  await sleep(200);
  out.openGroupManager = await measure(
    () => document.getElementById("mod-group-manager-btn")?.click(),
    () => wait(() => document.querySelectorAll("#strategy-group-list [data-group-row]").length > 0, 25000),
  );
  out.groupRows = document.querySelectorAll("#strategy-group-list [data-group-row]").length;
  out.parentSelectOptions = {
    // 惰性填充：开窗口只画"当前值"，点开某一行的下拉才补候选。
    beforeOpen: document.querySelectorAll("#strategy-group-list option").length,
  };
  const parentSelect = document.querySelector("#strategy-group-list .settings-strategy-parent-select");
  parentSelect?.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
  await sleep(150);
  out.parentSelectOptions.afterOpen = document.querySelectorAll("#strategy-group-list option").length;
  document.getElementById("strategy-group-close-btn")?.click();
  await sleep(300);

  // ④ 其它窗口 / 菜单（都是"点开窗口"类交互）。
  out.surfaces = {};
  const surface = async (label, open, done, close) => {
    out.surfaces[label] = await measure(open, done);
    if (close) {
      close();
      await sleep(250);
    }
  };
  const visible = (id) => {
    const el = document.getElementById(id);
    return Boolean(el) && !el.classList.contains("hidden");
  };
  // 卡片模式下「详情」在按需生成的「⋮」菜单里（见 render.js 的 ensureMoreActionsMenu）：
  // 探针要走真实路径——先点开菜单，再点菜单里的「详情」。
  const openCardDetail = () => {
    const card = document.querySelector("#file-list .file-card");
    card?.querySelector(".more-btn")?.click();
    const menu = [...document.querySelectorAll(".dropdown-content")].find(
      (el) => !el.classList.contains("hidden"),
    );
    (menu || card)?.querySelector(".detail-btn")?.click();
  };
  await surface(
    "cardDetail",
    openCardDetail,
    () => wait(() => visible("file-detail-modal") || visible("detail-modal"), 15000),
    () => document.getElementById("close-detail-modal-btn")?.click(),
  );
  await surface(
    "loadOrderModal",
    () => document.getElementById("load-order-toolbar-btn")?.click(),
    () => wait(() => visible("load-order-modal"), 15000),
    () => document.getElementById("load-order-close-btn")?.click(),
  );
  await surface(
    "settingsPage",
    () => document.getElementById("global-settings-btn")?.click(),
    () => wait(() => document.querySelector("#settings-page, .settings-page"), 15000),
  );
  await surface(
    "backToMods",
    () => document.querySelector('[data-page="mods"], [data-nav="mods"]')?.click(),
    () => wait(() => cards() > 0, 15000),
  );

  // ⑤ 单卡按钮：游戏内开关（真机历史 289ms / 长任务 206+111ms）。
  //    等分帧补齐结束再点，避免把"列表补齐"的帧算到按钮头上
  //    （固定 sleep 在 2904 张卡时要 2.6s，等不稳定就会把补齐的帧记成按钮的锅）。
  let buttonSettleLast = -1;
  let buttonSettleRounds = 0;
  while (buttonSettleRounds < 6) {
    const now = cards();
    buttonSettleRounds = now === buttonSettleLast ? buttonSettleRounds + 1 : 0;
    buttonSettleLast = now;
    await sleep(200);
  }
  const toggle = document.querySelector('#file-list .file-card .game-toggle-btn[data-action="toggle-game"]');
  if (toggle) {
    const path = toggle.getAttribute("data-file-path");
    const before = (toggle.textContent || "").trim();
    out.toggleGameState = await measure(
      () => toggle.click(),
      () =>
        wait(() => {
          const now = document.querySelector(
            `.file-card[data-path="${CSS.escape(path)}"] .game-toggle-btn[data-action="toggle-game"]`,
          );
          return now && (now.textContent || "").trim() !== before;
        }, 20000),
    );
    // 切回去，保持沙箱数据不变。
    await sleep(300);
    document
      .querySelector(`.file-card[data-path="${CSS.escape(path)}"] .game-toggle-btn[data-action="toggle-game"]`)
      ?.click();
    await sleep(500);
  }

  out.dom = {
    cards: cards(),
    nodes: document.getElementsByTagName("*").length,
    htmlBytes: (document.getElementById("file-list")?.innerHTML || "").length,
  };
  return JSON.stringify(out, null, 1);
})()
