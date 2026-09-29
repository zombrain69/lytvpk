import assert from "node:assert/strict";
import test from "node:test";

import {
  DEFAULT_SERVER_PORT,
  normalizeServerAddress,
  parseServerInput,
} from "./address.js";

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

test("parseServerInput 兼容 IP 直连的常见写法", () => {
  const cases = [
    ["127.0.0.1", `127.0.0.1:${DEFAULT_SERVER_PORT}`],
    ["127.0.0.1:27016", "127.0.0.1:27016"],
    ["[::1]:27015", "[::1]:27015"],
    ["connect 127.0.0.1:27015", "127.0.0.1:27015"],
    ["connect 127.0.0.1", `127.0.0.1:${DEFAULT_SERVER_PORT}`],
    ["steam://connect/127.0.0.1:27015", "127.0.0.1:27015"],
    ["steam://connect/example.com", `example.com:${DEFAULT_SERVER_PORT}`],
    ['"127.0.0.1:27015"', "127.0.0.1:27015"],
    ["  connect   example.com:27016  ", "example.com:27016"],
  ];
  for (const [input, expected] of cases) {
    assert.equal(parseServerInput(input), expected, input);
  }
});

test("parseServerInput 只有 connect 或空输入时给出可读提示", () => {
  for (const input of ["", "   ", "connect", "connect ", "CONNECT "]) {
    assert.throws(() => parseServerInput(input), /请输入服务器 IP 或域名/u, input);
  }
});
