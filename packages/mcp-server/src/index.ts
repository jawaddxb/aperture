/**
 * index.ts — Aperture MCP Server entry point.
 * Wires together Server, StdioTransport, and ToolHandlers.
 * Single responsibility: bootstrap and connect components.
 */

import { Server } from '@modelcontextprotocol/sdk/server/index.js';
import { StdioServerTransport } from '@modelcontextprotocol/sdk/server/stdio.js';
import {
  ListToolsRequestSchema,
  CallToolRequestSchema,
} from '@modelcontextprotocol/sdk/types.js';

import { loadConfig } from './config.js';
import { ApertureClient } from './aperture-client.js';
import { ToolHandlers } from './handlers.js';
import { ALL_TOOLS } from './tools.js';

/** SERVER_INFO identifies this MCP server to clients. */
const SERVER_INFO = {
  name: 'aperture-mcp',
  version: '1.0.0',
} as const;

/** main bootstraps and runs the Aperture MCP server. */
async function main(): Promise<void> {
  const config = loadConfig();
  const client = new ApertureClient(config);
  const handlers = new ToolHandlers(client);

  const server = new Server(SERVER_INFO, {
    capabilities: {
      tools: {},
    },
  });

  server.setRequestHandler(ListToolsRequestSchema, async () => {
    return { tools: ALL_TOOLS };
  });

  server.setRequestHandler(CallToolRequestSchema, async (request) => {
    const name = request.params.name;
    const args = (request.params.arguments ?? {}) as Record<string, unknown>;
    const result = await handlers.dispatch(name, args);
    return result;
  });

  const transport = new StdioServerTransport();
  await server.connect(transport);

  // Log to stderr only — stdout is reserved for MCP protocol messages.
  process.stderr.write(`Aperture MCP server started (base_url=${config.baseURL})\n`);
}

main().catch((err: Error) => {
  process.stderr.write(`Aperture MCP server fatal error: ${err.message}\n`);
  process.exit(1);
});
