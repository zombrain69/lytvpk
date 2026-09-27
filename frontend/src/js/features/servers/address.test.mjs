import assert from "node:assert/strict";
import test from "node:test";

import { DEFAULT_SERVER_PORT, normalizeServerAddress } from "./address.js";

test("normalizeServerAddress 补默认端口", () => {
  assert.equal(normalizeServerAddress("127.0.0.1"), `127.0.0.1:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com"), `example.com:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com:"), `example.com:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("example.com:27016"), "example.com:27016");
  assert.equal(normalizeServerAddress("  example.com:27016  "), "example.com:27016");
});

test("normalizeServerAddress 处理 IPv6", () => {
  assert.equal(normalizeServerAddress("2001:db8::1"), `[2001:db8::1]:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("[2001:db8::1]"), `[2001:db8::1]:${DEFAULT_SERVER_PORT}`);
  assert.equal(normalizeServerAddress("[2001:db8::1]:27016"), "[2001:db8::1]:27016");
});

test("normalizeServerAddress 拒绝非法输入（消息可直接展示）", () => {
  const cases = [
    "",
    ":27015",
    "example.com:not-a-port",
    "example.com:0",
    "example.com:65536",
    "steam://connect/example.com",
    "[not-ipv6]",
    "example.com/evil",
    "example.com?x=1",
    "example .com",
  ];
  for (const input of cases) {
    assert.throws(() => normalizeServerAddress(input), /./u, `${input} 应被拒绝`);
  }
});
