import { defineConfig } from "vitepress";

// 文档部署在 GitHub Pages 的**项目站点**：https://<owner>.github.io/<repo>/，
// 所以必须给 VitePress 一个 base（否则 /assets、/logo.png 这些绝对路径会 404）。
// 默认 `"/lytvpk/"`；工作流会用 DOCS_BASE 传仓库名，这样 Fork 到别的仓库也不用改这份配置。
const base = process.env.DOCS_BASE || "/lytvpk/";

// sitemap 的 hostname 是"站点根"，而 VitePress 生成的条目是 `/guide/xxx` 这种绝对路径，
// 直接交给 sitemap 库会按"绝对路径替换"解析，把 base 段吃掉（变成 .../guide/xxx）。
// 所以这里把 base 明确补到每个条目上。
const normalizedBase = base.endsWith("/") ? base.slice(0, -1) : base;

export default defineConfig({
  base,
  lang: "zh-CN",
  title: "LytVPK",
  description: "Left 4 Dead 2 VPK Mod 管理器使用文档",
  cleanUrls: true,
  sitemap: {
    hostname: "https://zombrain69.github.io",
    transformItems: (items) =>
      items.map((item) => ({
        ...item,
        url: `${normalizedBase}/${String(item.url || "").replace(/^\//, "")}`,
      })),
  },
  appearance: "force-dark",
  lastUpdated: true,
  head: [
    ["meta", { name: "theme-color", content: "#07142f" }],
    ["link", { rel: "icon", type: "image/png", href: `${base}logo.png` }],
    ["link", { rel: "apple-touch-icon", href: `${base}logo.png` }],
  ],
  themeConfig: {
    logo: "/logo.png",
    nav: [
      { text: "快速开始", link: "/guide/quick-start" },
      { text: "功能说明", link: "/features/mod-management" },
      {
        text: "立即下载",
        link: "https://github.com/zombrain69/lytvpk/releases",
      },
    ],
    sidebar: [
      {
        text: "开始使用",
        items: [
          { text: "项目介绍", link: "/" },
          { text: "快速开始", link: "/guide/quick-start" },
          { text: "全功能地图（能做什么 · 在哪儿点）", link: "/guide/feature-tour" },
          { text: "本 Fork 新增功能（入口 + 用法）", link: "/guide/whats-new" },
          { text: "为什么用这个 Fork（对比上游与 FireAxe）", link: "/guide/why-this-fork" },
          { text: "上游同步状态（v2.5.15 → v2.7.1）", link: "/guide/upstream-sync" },
          { text: "安装与运行", link: "/guide/install" },
        ],
      },
      {
        text: "功能说明",
        items: [
          { text: "MOD 管理", link: "/features/mod-management" },
          { text: "分组建议与外部导入", link: "/features/group-suggestion-import" },
          { text: "导入与拖拽", link: "/features/import-drag-drop" },
          { text: "创意工坊浏览", link: "/features/workshop" },
          { text: "下载与解析", link: "/features/downloads" },
          { text: "收藏服务器", link: "/features/servers" },
          {
            text: "工具箱",
            link: "/toolbox/",
            collapsed: false,
            items: [
              { text: "问题 Mod 查找", link: "/toolbox/problem-mod-scan" },
              { text: "Mod 冲突检测", link: "/toolbox/conflict-check" },
              { text: "Mod 体检", link: "/toolbox/mod-health-check" },
              { text: "模型面数检测", link: "/toolbox/model-stats" },
              { text: "VPK 解包", link: "/toolbox/vpk-unpack" },
              { text: "VPK 打包", link: "/toolbox/vpk-pack" },
              { text: "VPK 完整性检测", link: "/toolbox/vpk-integrity" },
              { text: "VPK 内容预览", link: "/toolbox/vpk-preview" },
              { text: "合并为整合包", link: "/toolbox/vpk-merge" },
              { text: "Mod 快照", link: "/toolbox/mod-snapshot" },
              { text: "压缩包管理", link: "/toolbox/archive-manager" },
              { text: "autoexec.cfg 编辑器", link: "/toolbox/autoexec" },
              { text: "崩溃转储查看器", link: "/toolbox/mdmp-report" },
              { text: "喷漆制作", link: "/toolbox/spray-tool" },
            ],
          },
          { text: "设置", link: "/features/settings" },
          { text: "关于与更新", link: "/features/about-update" },
        ],
      },
    ],
    outline: {
      label: "本页目录",
      level: [2, 3],
    },
    docFooter: {
      prev: "上一页",
      next: "下一页",
    },
    lastUpdated: {
      text: "最后更新",
      formatOptions: {
        dateStyle: "medium",
        timeStyle: "short",
      },
    },
    search: {
      provider: "local",
      options: {
        translations: {
          button: {
            buttonText: "搜索文档",
            buttonAriaLabel: "搜索文档",
          },
          modal: {
            noResultsText: "没有找到结果",
            resetButtonTitle: "清空搜索",
            footer: {
              selectText: "选择",
              navigateText: "切换",
              closeText: "关闭",
            },
          },
        },
      },
    },
    socialLinks: [
      { icon: "github", link: "https://github.com/zombrain69/lytvpk" },
    ],
  },
});
