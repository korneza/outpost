export interface OutpostClientOptions {
  /** URL of the local Outpost proxy. */
  proxyUrl: string;
  /** URL of the upstream MCP server, used only if the proxy is unreachable. */
  directUrl: string;
  /** Per-attempt timeout in milliseconds. Defaults to 3000. */
  timeoutMs?: number;
  /**
   * Maximum response body size in bytes. Defaults to 10 MiB — generous
   * for a real tool-call result while still bounding the worst case.
   * Matters most on the direct-fallback path: that path talks to the
   * upstream with none of Outpost's own protections in effect, so a
   * malicious or compromised upstream there is unconstrained except by
   * what this client does itself.
   */
  maxResponseBytes?: number;
}

const DEFAULT_MAX_RESPONSE_BYTES = 10 * 1024 * 1024;

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
  private readonly maxResponseBytes: number;

  constructor(options: OutpostClientOptions) {
    this.proxyUrl = options.proxyUrl;
    this.directUrl = options.directUrl;
    this.timeoutMs = options.timeoutMs ?? 3000;
    this.maxResponseBytes = options.maxResponseBytes ?? DEFAULT_MAX_RESPONSE_BYTES;
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
      const text = await readBounded(res, this.maxResponseBytes);
      return JSON.parse(text) as JsonRpcResponse;
    } finally {
      clearTimeout(timer);
    }
  }
}

/**
 * Reads res's body in chunks, throwing rather than buffering past
 * maxBytes. res.json()/res.text() read the whole body in one call,
 * trusting the far end not to send something absurd — the upstream
 * (direct or through the proxy) is untrusted by this client's own
 * threat model, so this stops the moment the total would exceed
 * maxBytes instead.
 */
async function readBounded(res: Response, maxBytes: number): Promise<string> {
  if (!res.body) {
    return "";
  }
  const reader = res.body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  for (;;) {
    const { done, value } = await reader.read();
    if (done) break;
    if (value) {
      total += value.byteLength;
      if (total > maxBytes) {
        await reader.cancel();
        throw new Error(`response exceeds ${maxBytes} byte limit`);
      }
      chunks.push(value);
    }
  }
  const combined = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    combined.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return new TextDecoder().decode(combined);
}
