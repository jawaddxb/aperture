/**
 * e2e-openclaw.test.ts — End-to-end integration tests for the OpenClaw → MCP → Aperture chain.
 *
 * These tests require a live Aperture server (go run ./cmd/aperture-server) and Chrome.
 * Set APERTURE_E2E=1 to enable; tests are skipped by default in CI without a browser.
 *
 * Test flow:
 *   1. Verify Aperture server is reachable
 *   2. List tools via MCP protocol (verifying 4 Aperture tools are visible)
 *   3. Execute navigate("https://example.com") via MCP
 *   4. Capture screenshot, verify non-empty image data
 *   5. Execute goBack(), verify navigation history response
 *   6. Poll pool_status → confirm at most 5 sessions active
 */

import { spawn, ChildProcess } from 'child_process';
import * as path from 'path';
import * as http from 'http';

// ── Test configuration ────────────────────────────────────────────────────────

const APERTURE_BASE_URL = process.env.APERTURE_BASE_URL ?? 'http://localhost:8080';
const MCP_SERVER_BIN = path.resolve(__dirname, '../packages/mcp-server/dist/index.js');
const RUN_E2E = process.env.APERTURE_E2E === '1';

// ── Helpers ──────────────────────────────────────────────────────────────────

/** httpGet fetches a URL and returns the parsed JSON body. */
function httpGet(url: string): Promise<unknown> {
  return new Promise((resolve, reject) => {
    const req = http.get(url, (res) => {
      let data = '';
      res.on('data', (chunk: Buffer) => { data += chunk.toString(); });
      res.on('end', () => {
        try {
          resolve(JSON.parse(data));
        } catch {
          reject(new Error(`Invalid JSON: ${data}`));
        }
      });
    });
    req.on('error', reject);
    req.setTimeout(5000, () => { req.destroy(); reject(new Error('timeout')); });
  });
}

/** isApertureReachable returns true if the Aperture /health endpoint responds. */
async function isApertureReachable(): Promise<boolean> {
  try {
    const body = (await httpGet(`${APERTURE_BASE_URL}/health`)) as { status?: string };
    return body?.status === 'ok';
  } catch {
    return false;
  }
}

/** MCPClient wraps an mcp-server subprocess and provides request/response helpers. */
class MCPClient {
  private proc: ChildProcess;
  private pending = new Map<number | string, (msg: Record<string, unknown>) => void>();
  private nextId = 1;
  private buffer = '';

  constructor(proc: ChildProcess) {
    this.proc = proc;
    this.proc.stdout!.on('data', (chunk: Buffer) => this.onData(chunk.toString()));
  }

  private onData(data: string): void {
    this.buffer += data;
    const lines = this.buffer.split('\n');
    this.buffer = lines.pop() ?? '';
    for (const line of lines) {
      if (!line.trim()) continue;
      try {
        const msg = JSON.parse(line) as Record<string, unknown>;
        const id = msg.id as number | string | undefined;
        if (id !== undefined) {
          const resolve = this.pending.get(id);
          if (resolve) {
            this.pending.delete(id);
            resolve(msg);
          }
        }
      } catch {
        // ignore non-JSON lines
      }
    }
  }

  /** send sends a JSON-RPC request and waits for the response. */
  send(method: string, params: unknown): Promise<Record<string, unknown>> {
    const id = this.nextId++;
    const msg = JSON.stringify({ jsonrpc: '2.0', id, method, params }) + '\n';
    this.proc.stdin!.write(msg);

    return new Promise((resolve) => {
      this.pending.set(id, resolve);
    });
  }

  /** initialize performs the MCP handshake. */
  async initialize(): Promise<void> {
    await this.send('initialize', {
      protocolVersion: '2024-11-05',
      capabilities: { tools: {} },
      clientInfo: { name: 'aperture-e2e-test', version: '1.0.0' },
    });
    const notification = JSON.stringify({
      jsonrpc: '2.0',
      method: 'notifications/initialized',
    }) + '\n';
    this.proc.stdin!.write(notification);
  }

  /** close terminates the subprocess. */
  close(): void {
    this.proc.kill('SIGTERM');
  }
}

/** spawnMCPClient starts the MCP server process and returns a connected MCPClient. */
function spawnMCPClient(): MCPClient {
  const proc = spawn('node', [MCP_SERVER_BIN], {
    stdio: ['pipe', 'pipe', 'pipe'],
    env: {
      ...process.env,
      APERTURE_BASE_URL,
      APERTURE_POOL_SIZE: '5',
      APERTURE_TIMEOUT: '30000',
    },
  });
  return new MCPClient(proc);
}

// ── Skip guard ────────────────────────────────────────────────────────────────

const describeE2E = RUN_E2E ? describe : describe.skip;

// ── Test suite ────────────────────────────────────────────────────────────────

describeE2E('Aperture E2E: OpenClaw → MCP → Aperture → Chrome', () => {
  let client: MCPClient;

  beforeAll(async () => {
    const reachable = await isApertureReachable();
    if (!reachable) {
      throw new Error(
        `Aperture server not reachable at ${APERTURE_BASE_URL}. ` +
        'Start it with: go run ./cmd/aperture-server',
      );
    }
    client = spawnMCPClient();
    await client.initialize();
    // Give the server a moment to be ready after initialize
    await new Promise((r) => setTimeout(r, 200));
  }, 15000);

  afterAll(() => {
    client?.close();
  });

  // ── Test 1: MCP tools/list shows 4 Aperture tools ─────────────────────────

  it('lists 4 Aperture tools via MCP', async () => {
    const response = await client.send('tools/list', {});
    const result = response.result as { tools?: Array<{ name: string }> };

    expect(result).toBeDefined();
    expect(result.tools).toBeDefined();
    expect(result.tools!.length).toBe(4);

    const names = result.tools!.map((t) => t.name);
    expect(names).toContain('execute');
    expect(names).toContain('screenshot');
    expect(names).toContain('navigation');
    expect(names).toContain('pool_status');
  }, 10000);

  // ── Test 2: Navigate to example.com ──────────────────────────────────────

  it('navigates to https://example.com via execute tool', async () => {
    const response = await client.send('tools/call', {
      name: 'execute',
      arguments: {
        goal: 'navigate to the page and verify the title',
        url: 'https://example.com',
        timeout_seconds: 30,
      },
    });

    const result = response.result as { content?: Array<{ type: string; text?: string }> };
    expect(result.content).toBeDefined();
    expect(result.content!.length).toBeGreaterThan(0);
    expect(result.content![0].type).toBe('text');

    // The execute should have completed (success or failure gracefully returned)
    const text = result.content![0].text ?? '';
    const parsed = JSON.parse(text) as { id?: string };
    expect(parsed.id).toBeTruthy();
  }, 60000);

  // ── Test 3: Screenshot capture ────────────────────────────────────────────

  it('captures screenshot of https://example.com', async () => {
    const response = await client.send('tools/call', {
      name: 'screenshot',
      arguments: {
        url: 'https://example.com',
        full_page: false,
      },
    });

    const result = response.result as {
      content?: Array<{ type: string; data?: string; mimeType?: string }>;
    };

    expect(result.content).toBeDefined();
    const imageItem = result.content!.find((c) => c.type === 'image');
    expect(imageItem).toBeDefined();
    expect(imageItem!.data).toBeTruthy();
    expect(imageItem!.mimeType).toBe('image/png');
    // PNG base64 should be substantial
    expect(imageItem!.data!.length).toBeGreaterThan(100);
  }, 30000);

  // ── Test 4: goBack navigation ─────────────────────────────────────────────

  it('executes goBack navigation with session_id', async () => {
    // First execute to get a session_id
    const execResponse = await client.send('tools/call', {
      name: 'execute',
      arguments: {
        goal: 'navigate to the page',
        url: 'https://example.com',
        timeout_seconds: 20,
      },
    });

    const execResult = execResponse.result as { content?: Array<{ type: string; text?: string }> };
    const execText = execResult.content?.[0]?.text ?? '{}';
    const task = JSON.parse(execText) as { id?: string };
    const sessionId = task.id ?? 'test-session';

    // Now call goBack with the session_id
    const navResponse = await client.send('tools/call', {
      name: 'navigation',
      arguments: {
        action: 'goBack',
        session_id: sessionId,
      },
    });

    const navResult = navResponse.result as { content?: Array<{ type: string; text?: string }> };
    expect(navResult.content).toBeDefined();
    expect(navResult.content!.length).toBeGreaterThan(0);
    // goBack result should be a JSON object (may succeed or report no history — both are valid)
    const navText = navResult.content![0].text ?? '{}';
    const navTask = JSON.parse(navText) as { id?: string };
    expect(navTask.id).toBeTruthy();
  }, 60000);

  // ── Test 5: Pool status ───────────────────────────────────────────────────

  it('reports pool status with at most 5 active sessions', async () => {
    const response = await client.send('tools/call', {
      name: 'pool_status',
      arguments: {},
    });

    const result = response.result as { content?: Array<{ type: string; text?: string }> };
    expect(result.content).toBeDefined();
    expect(result.content![0].type).toBe('text');

    const status = JSON.parse(result.content![0].text ?? '{}') as {
      status?: string;
      browser_pool?: string;
      active_tasks?: number;
    };

    expect(status.status).toBe('ok');
    expect(['available', 'exhausted']).toContain(status.browser_pool);
    expect(typeof status.active_tasks).toBe('number');
    expect(status.active_tasks!).toBeLessThanOrEqual(5);
  }, 10000);
});

// ── Always-on smoke tests (no live server required) ───────────────────────────

describe('Aperture MCP server: smoke tests (no server required)', () => {
  it('MCP server binary exists and is runnable', async () => {
    const fs = await import('fs');
    expect(fs.existsSync(MCP_SERVER_BIN)).toBe(true);
  });

  it('MCP server prints startup message on stderr', (done) => {
    const proc = spawn('node', [MCP_SERVER_BIN], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, APERTURE_BASE_URL },
    });

    let stderr = '';
    let finished = false;

    const finish = (err?: Error): void => {
      if (finished) return;
      finished = true;
      clearTimeout(timer);
      proc.kill('SIGTERM');
      done(err);
    };

    proc.stderr.on('data', (chunk: Buffer) => {
      stderr += chunk.toString();
      if (stderr.includes('Aperture MCP server started')) {
        finish();
      }
    });

    proc.on('error', (err) => finish(err));

    const timer = setTimeout(() => {
      finish(new Error(`Server did not print startup message. stderr: ${stderr}`));
    }, 5000);
  }, 8000);

  it('MCP server responds to tools/list with 4 tools', (done) => {
    const proc = spawn('node', [MCP_SERVER_BIN], {
      stdio: ['pipe', 'pipe', 'pipe'],
      env: { ...process.env, APERTURE_BASE_URL },
    });

    let stdout = '';
    let finished = false;

    const finish = (err?: Error): void => {
      if (finished) return;
      finished = true;
      clearTimeout(failTimer);
      clearTimeout(startTimer);
      proc.kill('SIGTERM');
      done(err);
    };

    proc.stdout.on('data', (chunk: Buffer) => {
      stdout += chunk.toString();
      const lines = stdout.split('\n');
      for (const line of lines) {
        if (!line.trim()) continue;
        try {
          const msg = JSON.parse(line) as {
            id?: unknown;
            result?: { tools?: unknown[] };
          };
          if (msg.id === 2 && msg.result?.tools) {
            try {
              expect(msg.result.tools.length).toBe(4);
              finish();
            } catch (err) {
              finish(err as Error);
            }
            return;
          }
        } catch {
          // not valid JSON yet
        }
      }
    });

    proc.on('error', (err) => finish(err));

    const initMsg = JSON.stringify({
      jsonrpc: '2.0',
      id: 1,
      method: 'initialize',
      params: {
        protocolVersion: '2024-11-05',
        capabilities: { tools: {} },
        clientInfo: { name: 'smoke-test', version: '1.0.0' },
      },
    }) + '\n';

    const initializedNotif = JSON.stringify({
      jsonrpc: '2.0',
      method: 'notifications/initialized',
    }) + '\n';

    const listToolsMsg = JSON.stringify({
      jsonrpc: '2.0',
      id: 2,
      method: 'tools/list',
      params: {},
    }) + '\n';

    const startTimer = setTimeout(() => {
      proc.stdin!.write(initMsg);
      proc.stdin!.write(initializedNotif);
      proc.stdin!.write(listToolsMsg);
    }, 300);

    const failTimer = setTimeout(() => {
      finish(new Error(`tools/list did not respond in time. stdout: ${stdout}`));
    }, 7000);
  }, 10000);
});
