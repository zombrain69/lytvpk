import { getConfig, saveConfig } from "./config.js";
import { t } from "./i18n-runtime.js";

export function initTheme() {
  const savedTheme = getConfig().theme;
  const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches;

  if (savedTheme === "dark" || (!savedTheme && prefersDark)) {
    document.documentElement.classList.add("dark-mode");
  } else {
    document.documentElement.classList.remove("dark-mode");
  }
}

export function updateThemeIcon() {
  const isDark = document.documentElement.classList.contains("dark-mode");
  const sunIcon = document.querySelector("#theme-toggle-btn .sun-icon");
  const moonIcon = document.querySelector("#theme-toggle-btn .moon-icon");

  if (sunIcon && moonIcon) {
    if (isDark) {
      sunIcon.style.display = "block";
      moonIcon.style.display = "none";
    } else {
      sunIcon.style.display = "none";
      moonIcon.style.display = "block";
    }
  }

  const themeLabel = document.querySelector("#theme-toggle-btn .quick-label");
  if (themeLabel) {
    // 深色时提示「开灯模式」，浅色时提示「关灯模式」；data-i18n 跟着当前状态走，
    // 语言切换的全量重放才能拿到正确的 key。
    if (isDark) {
      themeLabel.setAttribute("data-i18n", "app.shell.theme.toLight");
      themeLabel.textContent = t("app.shell.theme.toLight");
    } else {
      themeLabel.setAttribute("data-i18n", "app.shell.theme.toDark");
      themeLabel.textContent = t("app.shell.theme.toDark");
    }
  }
}

export function setupThemeToggle() {
  const themeBtn = document.getElementById("theme-toggle-btn");
  if (!themeBtn) return;

  themeBtn.addEventListener("click", () => {
    const isDark = document.documentElement.classList.toggle("dark-mode");
    const config = getConfig();
    config.theme = isDark ? "dark" : "light";
    saveConfig(config);
    updateThemeIcon();
  });

  updateThemeIcon();
}
