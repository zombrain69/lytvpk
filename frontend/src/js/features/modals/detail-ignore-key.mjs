// 详情页忽略清单用的 addonlist 键推导（与 DOM / Wails 无关，便于 node --test 覆盖）。
//
// 必须与 Go 侧 addonListKeyForManagedVPKPathFromRoot 保持一致：
// root 用文件名，workshop 用 workshop\<name>，disabled 去掉 disabled\ 前缀，
// 统一小写并把 / 归一化成 \。

export function addonListKeyForDetail(file) {
  const name = String(file?.name || "")
    .trim()
    .replaceAll("/", "\\")
    .replace(/^\.\\/, "")
    .toLowerCase();
  if (!name) return "";
  const location = String(file?.location || "").trim().toLowerCase();
  if (location === "workshop") return `workshop\\${name}`;
  if (location === "disabled") return name.replace(/^disabled\\/, "");
  return name;
}
