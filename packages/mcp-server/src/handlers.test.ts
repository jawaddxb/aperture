/**
 * handlers.test.ts — Unit tests for ToolHandlers.
 * Uses a mock ApertureClient to avoid real HTTP calls.
 */

import { ToolHandlers } from './handlers';
import { ApertureClient } from './aperture-client';

/** buildMockClient returns a partial ApertureClient with jest mock methods. */
function buildMockClient(): jest.Mocked<ApertureClient> {
  return {
    execute: jest.fn(),
    screenshot: jest.fn(),
    navigate: jest.fn(),
    poolStatus: jest.fn(),
    healthCheck: jest.fn(),
  } as unknown as jest.Mocked<ApertureClient>;
}

describe('ToolHandlers', () => {
  let client: jest.Mocked<ApertureClient>;
  let handlers: ToolHandlers;

  beforeEach(() => {
    client = buildMockClient();
    handlers = new ToolHandlers(client);
  });

  // ── execute ──────────────────────────────────────────────────────────────────

  describe('execute', () => {
    it('calls client.execute with correct params', async () => {
      client.execute.mockResolvedValue({
        id: 'task-1',
        goal: 'test goal',
        success: true,
      });

      const result = await handlers.dispatch('execute', {
        goal: 'test goal',
        url: 'https://example.com',
        timeout_seconds: 60,
        screenshots: true,
      });

      expect(client.execute).toHaveBeenCalledWith({
        goal: 'test goal',
        url: 'https://example.com',
        timeout_seconds: 60,
        screenshots: true,
      });
      expect(result.isError).toBeFalsy();
      expect(result.content[0].type).toBe('text');
    });

    it('returns error when goal is missing', async () => {
      const result = await handlers.dispatch('execute', {});
      expect(result.isError).toBe(true);
      expect((result.content[0] as { type: string; text: string }).text).toContain('"goal" is required');
    });

    it('returns error when client.execute throws', async () => {
      client.execute.mockRejectedValue(new Error('network error'));
      const result = await handlers.dispatch('execute', { goal: 'do something' });
      expect(result.isError).toBe(true);
      expect((result.content[0] as { type: string; text: string }).text).toContain('network error');
    });
  });

  // ── screenshot ───────────────────────────────────────────────────────────────

  describe('screenshot', () => {
    it('calls client.screenshot with correct params', async () => {
      client.screenshot.mockResolvedValue({
        data: 'base64data',
        mime_type: 'image/png',
        url: 'https://example.com',
      });

      const result = await handlers.dispatch('screenshot', {
        url: 'https://example.com',
        full_page: true,
      });

      expect(client.screenshot).toHaveBeenCalledWith({
        url: 'https://example.com',
        full_page: true,
      });
      expect(result.isError).toBeFalsy();
      expect(result.content[0].type).toBe('image');
    });

    it('returns error when url is missing', async () => {
      const result = await handlers.dispatch('screenshot', {});
      expect(result.isError).toBe(true);
    });

    it('returns error when client.screenshot throws', async () => {
      client.screenshot.mockRejectedValue(new Error('timeout'));
      const result = await handlers.dispatch('screenshot', { url: 'https://example.com' });
      expect(result.isError).toBe(true);
    });
  });

  // ── navigation ───────────────────────────────────────────────────────────────

  describe('navigation', () => {
    it('calls client.navigate for navigate action', async () => {
      client.navigate.mockResolvedValue({ id: 't1', goal: 'navigate', success: true });

      const result = await handlers.dispatch('navigation', {
        action: 'navigate',
        url: 'https://example.com',
        session_id: 'sess-123',
      });

      expect(client.navigate).toHaveBeenCalledWith({
        action: 'navigate',
        url: 'https://example.com',
        session_id: 'sess-123',
      });
      expect(result.isError).toBeFalsy();
    });

    it('calls client.navigate for goBack action', async () => {
      client.navigate.mockResolvedValue({ id: 't2', goal: 'goBack', success: true });

      const result = await handlers.dispatch('navigation', {
        action: 'goBack',
        session_id: 'sess-123',
      });

      expect(client.navigate).toHaveBeenCalledWith({
        action: 'goBack',
        url: undefined,
        session_id: 'sess-123',
      });
      expect(result.isError).toBeFalsy();
    });

    it('returns error for unknown action', async () => {
      const result = await handlers.dispatch('navigation', {
        action: 'teleport',
        session_id: 'sess-123',
      });
      expect(result.isError).toBe(true);
    });

    it('returns error when session_id is missing', async () => {
      const result = await handlers.dispatch('navigation', { action: 'reload' });
      expect(result.isError).toBe(true);
    });

    it('returns error when navigate action missing url', async () => {
      const result = await handlers.dispatch('navigation', {
        action: 'navigate',
        session_id: 'sess-123',
      });
      expect(result.isError).toBe(true);
    });
  });

  // ── pool_status ──────────────────────────────────────────────────────────────

  describe('pool_status', () => {
    it('returns pool status from client', async () => {
      client.poolStatus.mockResolvedValue({
        status: 'ok',
        browser_pool: 'available',
        llm_client: 'not configured',
        active_tasks: 0,
      });

      const result = await handlers.dispatch('pool_status', {});
      expect(result.isError).toBeFalsy();
      expect(result.content[0].type).toBe('text');
      const text = (result.content[0] as { type: string; text: string }).text;
      expect(text).toContain('available');
    });

    it('returns error when pool status call fails', async () => {
      client.poolStatus.mockRejectedValue(new Error('connection refused'));
      const result = await handlers.dispatch('pool_status', {});
      expect(result.isError).toBe(true);
    });
  });

  // ── unknown tool ─────────────────────────────────────────────────────────────

  describe('unknown tool', () => {
    it('returns error result for unknown tool name', async () => {
      const result = await handlers.dispatch('unknown_tool', {});
      expect(result.isError).toBe(true);
      expect((result.content[0] as { type: string; text: string }).text).toContain('Unknown tool');
    });
  });
});
