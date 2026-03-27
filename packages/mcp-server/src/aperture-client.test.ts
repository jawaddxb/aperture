/**
 * aperture-client.test.ts — Unit tests for ApertureClient.
 * Uses global fetch mocking — no real HTTP calls.
 */

import { ApertureClient } from './aperture-client';
import { ApertureConfig } from './config';

/** Default config pointing at a fake server. */
const TEST_CONFIG: ApertureConfig = {
  baseURL: 'http://localhost:19999',
  poolSize: 3,
  timeoutMs: 5000,
};

/** Helper to mock a successful fetch response. */
function mockFetchOk(body: unknown): void {
  global.fetch = jest.fn().mockResolvedValue({
    ok: true,
    status: 200,
    text: jest.fn().mockResolvedValue(JSON.stringify(body)),
  } as unknown as Response);
}

/** Helper to mock a failed fetch response. */
function mockFetchError(status: number, body: string): void {
  global.fetch = jest.fn().mockResolvedValue({
    ok: false,
    status,
    text: jest.fn().mockResolvedValue(body),
  } as unknown as Response);
}

describe('ApertureClient', () => {
  let client: ApertureClient;

  beforeEach(() => {
    client = new ApertureClient(TEST_CONFIG);
  });

  afterEach(() => {
    jest.restoreAllMocks();
  });

  // ── execute ──────────────────────────────────────────────────────────────────

  describe('execute', () => {
    it('POSTs to /api/v1/bridge/execute?sync=true', async () => {
      const expected = { id: 'task-1', goal: 'test', success: true };
      mockFetchOk(expected);

      const result = await client.execute({ goal: 'test', url: 'https://example.com' });

      expect(result).toEqual(expected);
      const fetchCall = (global.fetch as jest.Mock).mock.calls[0];
      expect(fetchCall[0]).toBe('http://localhost:19999/api/v1/bridge/execute?sync=true');
      expect(fetchCall[1].method).toBe('POST');
    });

    it('throws on non-OK response', async () => {
      mockFetchError(500, 'internal error');
      await expect(client.execute({ goal: 'test' })).rejects.toThrow('Aperture API error 500');
    });
  });

  // ── screenshot ───────────────────────────────────────────────────────────────

  describe('screenshot', () => {
    it('POSTs to /api/v1/actions/screenshot', async () => {
      mockFetchOk({ data: 'base64', url: 'https://example.com' });

      const result = await client.screenshot({ url: 'https://example.com' });

      expect(result.mime_type).toBe('image/png');
      expect(result.data).toBe('base64');
      const fetchCall = (global.fetch as jest.Mock).mock.calls[0];
      expect(fetchCall[0]).toBe('http://localhost:19999/api/v1/actions/screenshot');
    });

    it('throws on error', async () => {
      mockFetchError(422, 'bad request');
      await expect(client.screenshot({ url: 'x' })).rejects.toThrow('Aperture API error 422');
    });
  });

  // ── navigate ─────────────────────────────────────────────────────────────────

  describe('navigate', () => {
    it('sends "navigate to <url>" goal for navigate action', async () => {
      mockFetchOk({ id: 't', goal: 'navigate to https://example.com', success: true });

      await client.navigate({
        action: 'navigate',
        url: 'https://example.com',
        session_id: 'sess-1',
      });

      const body = JSON.parse((global.fetch as jest.Mock).mock.calls[0][1].body);
      expect(body.goal).toContain('navigate to https://example.com');
    });

    it('sends "go back" goal for goBack action', async () => {
      mockFetchOk({ id: 't', goal: 'go back', success: true });

      await client.navigate({ action: 'goBack', session_id: 'sess-1' });

      const body = JSON.parse((global.fetch as jest.Mock).mock.calls[0][1].body);
      expect(body.goal).toContain('go back');
    });
  });

  // ── poolStatus ───────────────────────────────────────────────────────────────

  describe('poolStatus', () => {
    it('GETs /api/v1/bridge/health', async () => {
      const expected = {
        status: 'ok',
        browser_pool: 'available',
        llm_client: 'not configured',
        active_tasks: 0,
      };
      mockFetchOk(expected);

      const result = await client.poolStatus();
      expect(result).toEqual(expected);

      const fetchCall = (global.fetch as jest.Mock).mock.calls[0];
      expect(fetchCall[0]).toBe('http://localhost:19999/api/v1/bridge/health');
    });
  });

  // ── healthCheck ──────────────────────────────────────────────────────────────

  describe('healthCheck', () => {
    it('returns true on successful /health response', async () => {
      mockFetchOk({ status: 'ok' });
      expect(await client.healthCheck()).toBe(true);
    });

    it('returns false when fetch throws', async () => {
      global.fetch = jest.fn().mockRejectedValue(new Error('ECONNREFUSED'));
      expect(await client.healthCheck()).toBe(false);
    });

    it('returns false on non-OK response', async () => {
      mockFetchError(503, 'service unavailable');
      expect(await client.healthCheck()).toBe(false);
    });
  });

  // ── URL trailing slash normalization ─────────────────────────────────────────

  describe('baseURL normalization', () => {
    it('strips trailing slash from baseURL', async () => {
      const c = new ApertureClient({ ...TEST_CONFIG, baseURL: 'http://localhost:8080/' });
      mockFetchOk({ status: 'ok' });
      await c.healthCheck();
      const url = (global.fetch as jest.Mock).mock.calls[0][0] as string;
      expect(url).toBe('http://localhost:8080/health');
      expect(url).not.toContain('//health');
    });
  });
});
