/**
 * tools.ts — MCP tool definitions and input schemas for Aperture.
 * Single responsibility: describe the 4 tools; no execution logic.
 */

/** ToolName enumerates the 4 Aperture MCP tools. */
export type ToolName = 'execute' | 'screenshot' | 'navigation' | 'pool_status';

/** ToolDefinition is the MCP tool descriptor shape used in listTools. */
export interface ToolDefinition {
  name: ToolName;
  description: string;
  inputSchema: {
    type: 'object';
    properties: Record<string, unknown>;
    required?: string[];
  };
}

/** ALL_TOOLS is the static list of Aperture MCP tool definitions. */
export const ALL_TOOLS: ToolDefinition[] = [
  {
    name: 'execute',
    description:
      'Run a browser automation task via the Aperture executor pipeline. ' +
      'Provide a natural-language goal and an optional starting URL. ' +
      'Returns the task result including success status, final URL, and step details.',
    inputSchema: {
      type: 'object',
      properties: {
        goal: {
          type: 'string',
          description: 'Natural-language description of what to accomplish in the browser.',
        },
        url: {
          type: 'string',
          description: 'Optional starting URL to navigate to before executing the goal.',
        },
        timeout_seconds: {
          type: 'number',
          description: 'Maximum execution time in seconds (default: 30, max: 300).',
          minimum: 1,
          maximum: 300,
        },
        screenshots: {
          type: 'boolean',
          description: 'Capture screenshots at each step (default: false).',
        },
      },
      required: ['goal'],
    },
  },
  {
    name: 'screenshot',
    description:
      'Capture a screenshot of a URL and return it as a base64-encoded PNG. ' +
      'Optionally capture the full scrollable page height.',
    inputSchema: {
      type: 'object',
      properties: {
        url: {
          type: 'string',
          description: 'The URL to navigate to and screenshot.',
        },
        full_page: {
          type: 'boolean',
          description: 'Capture the full scrollable page (default: false = viewport only).',
        },
      },
      required: ['url'],
    },
  },
  {
    name: 'navigation',
    description:
      'Execute a browser navigation action: navigate to a URL, go back, go forward, or reload. ' +
      'Requires an active session_id for back/forward/reload operations.',
    inputSchema: {
      type: 'object',
      properties: {
        action: {
          type: 'string',
          enum: ['navigate', 'goBack', 'goForward', 'reload'],
          description: 'The navigation action to perform.',
        },
        url: {
          type: 'string',
          description: 'Target URL — required when action is "navigate".',
        },
        session_id: {
          type: 'string',
          description: 'Session ID returned from a prior execute call.',
        },
      },
      required: ['action', 'session_id'],
    },
  },
  {
    name: 'pool_status',
    description:
      'Return the current health and availability of the Aperture browser pool. ' +
      'Reports total pool size, available browsers, and active task count.',
    inputSchema: {
      type: 'object',
      properties: {},
    },
  },
];
