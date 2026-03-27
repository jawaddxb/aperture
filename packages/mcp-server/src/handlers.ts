/**
 * handlers.ts — MCP tool call handlers for Aperture.
 * Single responsibility: map MCP tool invocations to ApertureClient calls.
 */

import type { CallToolResult } from '@modelcontextprotocol/sdk/types.js';
import { ApertureClient } from './aperture-client.js';
import type { NavigationAction } from './aperture-client.js';

/** ToolHandlers routes MCP tool calls to the Aperture service. */
export class ToolHandlers {
  private readonly client: ApertureClient;

  constructor(client: ApertureClient) {
    this.client = client;
  }

  /** dispatch routes a tool call by name. Returns an error result for unknown tools. */
  async dispatch(name: string, args: Record<string, unknown>): Promise<CallToolResult> {
    switch (name) {
      case 'execute':
        return this.handleExecute(args);
      case 'screenshot':
        return this.handleScreenshot(args);
      case 'navigation':
        return this.handleNavigation(args);
      case 'pool_status':
        return this.handlePoolStatus();
      default:
        return this.errorResult(`Unknown tool: ${name}`);
    }
  }

  // ── tool handlers ────────────────────────────────────────────────────────────

  private async handleExecute(args: Record<string, unknown>): Promise<CallToolResult> {
    const goal = String(args.goal ?? '');
    if (!goal) {
      return this.errorResult('execute: "goal" is required');
    }

    const url = args.url !== undefined ? String(args.url) : undefined;
    const timeoutSeconds = typeof args.timeout_seconds === 'number' ? args.timeout_seconds : undefined;
    const screenshots = typeof args.screenshots === 'boolean' ? args.screenshots : false;

    try {
      const result = await this.client.execute({ goal, url, timeout_seconds: timeoutSeconds, screenshots });
      return {
        content: [
          {
            type: 'text',
            text: JSON.stringify(result, null, 2),
          },
        ],
      };
    } catch (err) {
      return this.errorResult(`execute failed: ${(err as Error).message}`);
    }
  }

  private async handleScreenshot(args: Record<string, unknown>): Promise<CallToolResult> {
    const url = String(args.url ?? '');
    if (!url) {
      return this.errorResult('screenshot: "url" is required');
    }

    const fullPage = typeof args.full_page === 'boolean' ? args.full_page : false;

    try {
      const result = await this.client.screenshot({ url, full_page: fullPage });
      return {
        content: [
          {
            type: 'image',
            data: result.data,
            mimeType: result.mime_type,
          },
          {
            type: 'text',
            text: `Screenshot captured for: ${result.url}`,
          },
        ],
      };
    } catch (err) {
      return this.errorResult(`screenshot failed: ${(err as Error).message}`);
    }
  }

  private async handleNavigation(args: Record<string, unknown>): Promise<CallToolResult> {
    const action = String(args.action ?? '') as NavigationAction;
    const validActions: NavigationAction[] = ['navigate', 'goBack', 'goForward', 'reload'];

    if (!validActions.includes(action)) {
      return this.errorResult(`navigation: "action" must be one of ${validActions.join(', ')}`);
    }

    const sessionId = String(args.session_id ?? '');
    if (!sessionId) {
      return this.errorResult('navigation: "session_id" is required');
    }

    if (action === 'navigate') {
      const url = String(args.url ?? '');
      if (!url) {
        return this.errorResult('navigation: "url" is required when action is "navigate"');
      }
    }

    const url = args.url !== undefined ? String(args.url) : undefined;

    try {
      const result = await this.client.navigate({ action, url, session_id: sessionId });
      return {
        content: [
          {
            type: 'text',
            text: JSON.stringify(result, null, 2),
          },
        ],
      };
    } catch (err) {
      return this.errorResult(`navigation failed: ${(err as Error).message}`);
    }
  }

  private async handlePoolStatus(): Promise<CallToolResult> {
    try {
      const status = await this.client.poolStatus();
      return {
        content: [
          {
            type: 'text',
            text: JSON.stringify(status, null, 2),
          },
        ],
      };
    } catch (err) {
      return this.errorResult(`pool_status failed: ${(err as Error).message}`);
    }
  }

  // ── helpers ──────────────────────────────────────────────────────────────────

  private errorResult(message: string): CallToolResult {
    return {
      content: [{ type: 'text', text: message }],
      isError: true,
    };
  }
}
