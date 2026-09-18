import assert from "node:assert/strict";
import { test } from "node:test";

// 用假的 window/location/localStorage 加载一份全新的 api.ts，验证地址决策：
// 大厅默认线上 WBA；单人/卡组/回放优先本机离线服务。
async function loadApi({ hostname, protocol = "https:", origin, injected = {}, stored = {} }) {
  globalThis.window = { ...injected };
  globalThis.location = { hostname, protocol, origin: origin || `${protocol}//${hostname}` };
  globalThis.localStorage = {
    getItem: (key) => (key in stored ? stored[key] : null),
    setItem: () => {},
    removeItem: () => {},
  };
  const module = await import(`../web/src/api.ts?case=${hostname}-${protocol}-${Math.random()}`);
  return module;
}

test("桌面客户端：大厅默认线上 WBA，单人用进程内本机服务", async () => {
  const api = await loadApi({
    hostname: "wails.localhost",
    protocol: "http:",
    injected: { WBO_LOCAL_API_BASE: "http://127.0.0.1:41234" },
  });
  assert.equal(api.PRODUCTION_API_BASE, "https://sva.hypd.asia/wbo");
  assert.equal(api.lobbyApiBase(), "https://sva.hypd.asia/wbo");
  assert.equal(api.localApiBase(), "http://127.0.0.1:41234");
  assert.equal(api.API_BASE, "http://127.0.0.1:41234");
});

test("本地开发：大厅仍然默认线上，单人是本机 23215", async () => {
  const api = await loadApi({ hostname: "localhost", protocol: "http:", origin: "http://localhost:5173" });
  assert.equal(api.lobbyApiBase(), "https://sva.hypd.asia/wbo");
  assert.equal(api.API_BASE, "http://127.0.0.1:23215");
});

test("站点网页版：大厅与本机都走同源 /wbo", async () => {
  const api = await loadApi({ hostname: "sva.hypd.asia", origin: "https://sva.hypd.asia" });
  assert.equal(api.lobbyApiBase(), "https://sva.hypd.asia/wbo");
  assert.equal(api.API_BASE, "https://sva.hypd.asia/wbo");
});

test("设置里可以把大厅切到本机离线服务，或自定义地址", async () => {
  const local = await loadApi({
    hostname: "wails.localhost",
    protocol: "http:",
    injected: { WBO_LOCAL_API_BASE: "http://127.0.0.1:41234" },
    stored: { "wbo-lobby-base": "local" },
  });
  assert.equal(local.lobbyApiBase(), "http://127.0.0.1:41234");

  const custom = await loadApi({
    hostname: "wails.localhost",
    protocol: "http:",
    stored: { "wbo-lobby-base": "http://192.168.1.5:23215" },
  });
  assert.equal(custom.lobbyApiBase(), "http://192.168.1.5:23215");
});
