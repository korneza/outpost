import { test } from "node:test";
import assert from "node:assert/strict";
import * as http from "node:http";
import { OutpostClient } from "./index.js";

function startServer(handler: http.RequestListener): Promise<{ url: string; close: () => Promise<void> }> {
  return new Promise((resolve) => {
    const server = http.createServer(handler);
    server.listen(0, "127.0.0.1", () => {
      const addr = server.address();
      const port = typeof addr === "object" && addr ? addr.port : 0;
      resolve({
        url: `http://127.0.0.1:${port}`,
        close: () => new Promise((r) => server.close(() => r())),
      });
    });
  });
}

test("call() uses the proxy when it's reachable", async () => {
  const proxy = await startServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      res.setHeader("Content-Type", "application/json");
      res.end(JSON.stringify({ jsonrpc: "2.0", id: 1, result: { via: "proxy" } }));
    });
  });
  const direct = await startServer((_req, res) => {
    res.end(JSON.stringify({ jsonrpc: "2.0", id: 1, result: { via: "direct" } }));
  });

  const client = new OutpostClient({ proxyUrl: proxy.url, directUrl: direct.url, timeoutMs: 1000 });
  const resp = await client.call({ jsonrpc: "2.0", id: 1, method: "tools/list" });

  assert.deepEqual(resp.result, { via: "proxy" });

  await proxy.close();
  await direct.close();
});

test("call() falls back to direct connection when the proxy is killed mid-run", async () => {
  const proxy = await startServer((_req, res) => {
    res.end(JSON.stringify({ jsonrpc: "2.0", id: 1, result: { via: "proxy" } }));
  });
  const direct = await startServer((_req, res) => {
    res.end(JSON.stringify({ jsonrpc: "2.0", id: 1, result: { via: "direct" } }));
  });

  const client = new OutpostClient({ proxyUrl: proxy.url, directUrl: direct.url, timeoutMs: 1000 });

  // Prove the proxy path works first...
  const first = await client.call({ jsonrpc: "2.0", id: 1, method: "tools/list" });
  assert.deepEqual(first.result, { via: "proxy" });

  // ...then kill the proxy mid-run and confirm the fallback fires within
  // the bounded timeout, with a visible warning.
  await proxy.close();

  const warnings: string[] = [];
  const originalWarn = console.warn;
  console.warn = (msg: string) => warnings.push(msg);
  try {
    const second = await client.call({ jsonrpc: "2.0", id: 2, method: "tools/list" });
    assert.deepEqual(second.result, { via: "direct" });
  } finally {
    console.warn = originalWarn;
  }
  assert.ok(warnings.some((w) => w.includes("falling back to direct connection")), "expected a visible fallback warning");

  await direct.close();
});

test("call() rejects an oversized response body", async () => {
  // Same class of gap as the Go and Python clients' F5/F16/F20 fixes:
  // res.json() read the entire response with no size cap, so a
  // malicious or compromised upstream (most reachable on the direct-
  // fallback path, where Outpost's own protections don't apply) could
  // force the calling process to buffer an arbitrarily large body.
  // maxResponseBytes is tiny here so the test doesn't need to actually
  // transfer megabytes to prove the cap holds.
  const proxy = await startServer((_req, res) => {
    res.setHeader("Content-Type", "application/json");
    res.end(JSON.stringify({ jsonrpc: "2.0", id: 1, result: "a".repeat(1000) }));
  });

  const client = new OutpostClient({
    proxyUrl: proxy.url,
    directUrl: "http://127.0.0.1:1",
    timeoutMs: 1000,
    maxResponseBytes: 100,
  });

  await assert.rejects(() => client.call({ jsonrpc: "2.0", id: 1, method: "tools/list" }));

  await proxy.close();
});

test("call() accepts a response exactly at the size cap", async () => {
  const cap = 1000;
  const proxy = await startServer((_req, res) => {
    const prefix = '{"jsonrpc":"2.0","id":1,"result":"';
    const suffix = '"}';
    const padding = cap - prefix.length - suffix.length;
    res.setHeader("Content-Type", "application/json");
    res.end(prefix + "a".repeat(padding) + suffix);
  });

  const client = new OutpostClient({
    proxyUrl: proxy.url,
    directUrl: "http://127.0.0.1:1",
    timeoutMs: 1000,
    maxResponseBytes: cap,
  });

  const resp = await client.call({ jsonrpc: "2.0", id: 1, method: "tools/list" });
  assert.equal(resp.result, "a".repeat(cap - '{"jsonrpc":"2.0","id":1,"result":"'.length - '"}'.length));

  await proxy.close();
});
