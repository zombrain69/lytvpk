// 文档站内部链接自检。
//
// 为什么需要它：VitePress 只会校验 **markdown 语法**写出来的链接；首页按钮、工具箱卡片这类
// **手写 HTML** 的 `href` 不会被检查 —— 曾经就因为少了 base 前缀（`/guide/xxx` 而不是
// `/lytvpk/guide/xxx`）在线上整片点不动。这个脚本直接扫构建产物：
//   1. 解析每个 .html 里的 href/src；
//   2. 相对链接按浏览器规则基于当前页面 URL 解析（`./x`、`x`、`../x`）；
//   3. 站点根绝对链接必须带 base 前缀；
//   4. 目标必须在 dist 里真实存在（支持 cleanUrls 的 `x.html` 与目录 `x/index.html`）。

import { readdirSync, readFileSync } from "node:fs";
import path from "node:path";
import process from "node:process";
import { fileURLToPath } from "node:url";

// 默认取脚本旁边的 ../.vitepress/dist：这样无论从 docs/ 还是仓库根目录调用都找得到。
const defaultDist = fileURLToPath(new URL("../.vitepress/dist", import.meta.url));
const distDir = path.resolve(process.argv[2] || defaultDist);
const base = (process.env.DOCS_BASE || "/lytvpk/").replace(/\/$/, "");

function walk(dir) {
  const out = [];
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) out.push(...walk(full));
    else out.push(full);
  }
  return out;
}

const files = walk(distDir);
const existing = new Set(
  files.map((file) => file.replace(/\\/g, "/").slice(distDir.replace(/\\/g, "/").length)),
);
const htmlFiles = files.filter((file) => file.endsWith(".html"));

const broken = [];
let checked = 0;

for (const file of htmlFiles) {
  const html = readFileSync(file, "utf8");
  const pagePath = file.replace(/\\/g, "/").slice(distDir.replace(/\\/g, "/").length);
  const pageUrl = pagePath.replace(/index\.html$/, "");

  for (const match of html.matchAll(/(?:href|src)="([^"]*)"/g)) {
    const raw = match[1].trim();
    if (!raw || raw === "#" || raw.startsWith("javascript:") || raw.startsWith("mailto:")) continue;
    if (/^(https?:)?\/\//.test(raw)) continue;

    const withoutHash = raw.split("#")[0].split("?")[0];
    if (!withoutHash) continue;

    let resolved;
    if (withoutHash.startsWith(`${base}/`)) {
      resolved = withoutHash.slice(base.length);
    } else if (withoutHash.startsWith("/")) {
      broken.push(`${pagePath} :: 站点根链接缺少 base 前缀（${base}/）-> ${raw}`);
      continue;
    } else {
      resolved = path.posix.normalize(path.posix.join(pageUrl, withoutHash));
    }

    checked += 1;
    const candidates = [resolved, `${resolved}.html`, resolved.replace(/\/$/, "/index.html")];
    if (!candidates.some((candidate) => existing.has(candidate))) {
      broken.push(`${pagePath} :: 目标不存在 -> ${raw}（解析为 ${resolved}）`);
    }
  }
}

if (broken.length > 0) {
  console.error(`文档站内部链接自检失败：${broken.length} 处`);
  for (const item of broken) console.error(`  - ${item}`);
  process.exit(1);
}

console.log(`文档站内部链接自检通过：${htmlFiles.length} 个页面 / ${checked} 条内部链接，0 处失效`);
