import { switchAppPage } from "../../core/ui-shell.js";
import { renderAboutPage } from "../about/about.js";
import { showUpdateModal } from "../update/updates.js";
import { BrowserOpenURL } from "../../../../wailsjs/runtime/runtime";
import {
  GetAppVersion,
  CheckUpdate,
  GetCrashReportDirectory,
  ListCrashReports,
  OpenFileLocation,
} from "../../../../wailsjs/go/app/App";

export function showInfoModal() {
  switchAppPage("about", { silent: true });
  // 这里的参数表必须与 about.js 的 renderAboutPage 签名保持一致：
  // 少传一项，对应面板就会在运行时静默失效（历史上崩溃报告面板就是这样丢的）。
  renderAboutPage({
    BrowserOpenURL,
    GetAppVersion,
    CheckUpdate,
    showUpdateModal,
    GetCrashReportDirectory,
    ListCrashReports,
    OpenFileLocation,
  });
}

export function closeInfoModal() {
  document.getElementById("info-modal").classList.add("hidden");
}
