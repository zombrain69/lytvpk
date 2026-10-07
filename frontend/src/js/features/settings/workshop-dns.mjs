// 工坊 DNS 的纯逻辑（对齐后端 internal/network/workshop_dns.go 的校验口径）。
//
// 为什么要两端都校验：后端是唯一事实源（写盘/生效都在那边），但等一次 IPC 往返才报错
// 体验很差；前端先挡一次，错误立刻出现在输入框旁边。

/** 只接受单个 IPv4 / IPv6 地址：不带端口、不带域名、不是通配或组播地址。 */
export function isValidDNSAddress(value) {
  const address = String(value ?? "").trim();
  if (!address) return false;

  // IPv4：四段 0-255，去掉前导零（01 这种写法在很多解析器里含义不同，宁可不收）。
  if (/^\d{1,3}(?:\.\d{1,3}){3}$/.test(address)) {
    const parts = address.split(".");
    if (parts.some((part) => String(Number(part)) !== part || Number(part) > 255)) return false;
    const first = Number(parts[0]);
    if (first === 0 || (first >= 224 && first <= 239)) return false; // 未指定 / 组播
    return true;
  }

  // IPv6：用 URL 解析做一次规范化校验（同时挡掉 [::]、ff00::/8）。
  if (!address.includes(":") || !/^[0-9a-fA-F:.]+$/.test(address)) return false;
  try {
    const host = new URL(`http://[${address}]/`).hostname.toLowerCase();
    if (host === "[::]" || host.startsWith("[ff")) return false;
    return host.length > 2;
  } catch {
    return false;
  }
}

/** 归一化配置：非法地址一律退回"系统 DNS"，与后端行为一致（宁可退化，也不要坏地址）。 */
export function normalizeWorkshopDNSConfig(config) {
  const mode = config && typeof config === "object" && config.mode === "custom" ? "custom" : "system";
  const customAddress = mode === "custom" ? String(config.customAddress ?? "").trim() : "";
  if (mode === "custom" && !isValidDNSAddress(customAddress)) {
    return { mode: "system", customAddress: "" };
  }
  return { mode, customAddress };
}
