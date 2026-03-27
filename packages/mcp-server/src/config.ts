/**
 * config.ts — Aperture MCP server configuration.
 * Single responsibility: read and validate env vars.
 */

/** ApertureConfig holds all runtime settings for the MCP server. */
export interface ApertureConfig {
  /** Base URL for the Aperture HTTP API (default: http://localhost:8080) */
  readonly baseURL: string;

  /** Pool size hint for the browser pool (default: 5) */
  readonly poolSize: number;

  /** Default timeout for tool calls in milliseconds (default: 30000) */
  readonly timeoutMs: number;
}

/** loadConfig reads configuration from environment variables. */
export function loadConfig(): ApertureConfig {
  const baseURL = process.env.APERTURE_BASE_URL ?? 'http://localhost:8080';
  const poolSize = parsePositiveInt(process.env.APERTURE_POOL_SIZE, 5);
  const timeoutMs = parsePositiveInt(process.env.APERTURE_TIMEOUT, 30000);

  return { baseURL, poolSize, timeoutMs };
}

function parsePositiveInt(raw: string | undefined, fallback: number): number {
  if (raw === undefined || raw === '') return fallback;
  const n = parseInt(raw, 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}
