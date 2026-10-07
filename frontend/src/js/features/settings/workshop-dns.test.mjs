import assert from "node:assert/strict";
import test from "node:test";

import { isValidDNSAddress, normalizeWorkshopDNSConfig } from "./workshop-dns.mjs";

test("isValidDNSAddress 只接受单个 IPv4 / IPv6", () => {
  for (const address of ["119.29.29.29", "223.5.5.5", "8.8.8.8", "2001:4860:4860::8888", "::1"]) {
    assert.equal(isValidDNSAddress(address), true, `${address} 应该合法`);
  }
  for (const address of ["", "  ", "dns.alidns.com", "119.29.29.29:53", "0.0.0.0", "239.1.1.1", "999.1.1.1", "01.2.3.4", "[::]", "ff02::1"]) {
    assert.equal(isValidDNSAddress(address), false, `${address} 应该不合法`);
  }
});

test("normalizeWorkshopDNSConfig 非法地址退回系统 DNS", () => {
  assert.deepEqual(normalizeWorkshopDNSConfig({ mode: "custom", customAddress: " 119.29.29.29 " }), {
    mode: "custom",
    customAddress: "119.29.29.29",
  });
  assert.deepEqual(normalizeWorkshopDNSConfig({ mode: "custom", customAddress: "dns.alidns.com" }), {
    mode: "system",
    customAddress: "",
  });
  assert.deepEqual(normalizeWorkshopDNSConfig({ mode: "system", customAddress: "119.29.29.29" }), {
    mode: "system",
    customAddress: "",
  });
  assert.deepEqual(normalizeWorkshopDNSConfig(null), { mode: "system", customAddress: "" });
});
