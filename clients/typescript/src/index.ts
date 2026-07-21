export interface OutpostClientOptions {
  /** URL of the local Outpost proxy. */
  proxyUrl: string;
  /** URL of the upstream MCP server, used only if the proxy is unreachable. */
  directUrl: string;
  /** Per-attempt timeout in milliseconds. Defaults to 3000. */
  timeoutMs?: number;
}

export interface JsonRpcRequest {
  jsonrpc: "2.0";
  id?: number | string | null;
  method: string;
  params?: unknown;
}

export interface JsonRpcResponse {
  jsonrpc: "2.0";
  id: number | string | null;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

/**
 * OutpostClient sends MCP JSON-RPC calls through a local Outpost proxy,
 * falling back to a direct connection to the upstream server if the
 * proxy is unreachable — spec's v1 requirement that a dead Outpost
 * process must never take an agent down with it. The fallback is
 * visible (a console.warn) and bounded (timeoutMs per attempt), never
 * silent and never a source of unbounded hangs.
 */
export class OutpostClient {
  private readonly proxyUrl: string;
  private readonly directUrl: string;
  private readonly timeoutMs: number;

  constructor(options: OutpostClientOptions) {
    this.proxyUrl = options.proxyUrl;
    this.directUrl = options.directUrl;
    this.timeoutMs = options.timeoutMs ?? 3000;
  }

  async call(request: JsonRpcRequest): Promise<JsonRpcResponse> {
    try {
      return await this.postJson(this.proxyUrl, request);
    } catch (err) {
      const reason = err instanceof Error ? err.message : String(err);
      console.warn(
        `[outpost-client] proxy at ${this.proxyUrl} unreachable (${reason}); falling back to direct connection at ${this.directUrl}`,
      );
      return await this.postJson(this.directUrl, request);
    }
  }

  private async postJson(url: string, request: JsonRpcRequest): Promise<JsonRpcResponse> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);
    try {
      const res = await fetch(url, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(request),
        signal: controller.signal,
      });
      if (!res.ok) {
        throw new Error(`HTTP ${res.status}`);
      }
      return (await res.json()) as JsonRpcResponse;
    } finally {
      clearTimeout(timer);
    }
  }
}
