/**
 * aperture-client.ts — HTTP client for the Aperture Go service.
 * Single responsibility: wrap Aperture's REST API.
 */

import { ApertureConfig } from './config.js';

/** PoolStatusResponse mirrors the Aperture bridge health endpoint. */
export interface PoolStatusResponse {
  status: string;
  browser_pool: string;
  llm_client: string;
  active_tasks: number;
}

/** TaskResponse mirrors the Aperture bridge execute response. */
export interface TaskResponse {
  id: string;
  goal: string;
  success: boolean;
  error?: string;
  final_url?: string;
  final_title?: string;
  duration_ms?: number;
  steps?: StepSummary[];
}

/** StepSummary is one execution step result. */
export interface StepSummary {
  index: number;
  action: string;
  success: boolean;
  error?: string;
  duration_ms?: number;
}

/** ExecuteParams are the parameters for a bridge execute call. */
export interface ExecuteParams {
  goal: string;
  url?: string;
  timeout_seconds?: number;
  screenshots?: boolean;
}

/** NavigationAction enumerates the supported navigation commands. */
export type NavigationAction = 'navigate' | 'goBack' | 'goForward' | 'reload';

/** NavigateParams holds parameters for a navigation call. */
export interface NavigateParams {
  action: NavigationAction;
  url?: string;
  session_id: string;
}

/** ScreenshotParams holds parameters for a screenshot call. */
export interface ScreenshotParams {
  url: string;
  full_page?: boolean;
}

/** ScreenshotResponse holds the base64-encoded PNG. */
export interface ScreenshotResponse {
  data: string;
  mime_type: string;
  url: string;
}

/** VisionParams holds parameters for a vision analysis call. */
export interface VisionParams {
  url: string;
  prompt?: string;
}

/** VisionResponse holds the structured vision analysis result. */
export interface VisionResponse {
  description: string;
  elements: Array<{ type: string; description: string; selector: string }>;
  suggested_steps: string[];
}

/** ApertureClient is the HTTP client for the Aperture service. */
export class ApertureClient {
  private readonly baseURL: string;
  private readonly timeoutMs: number;

  constructor(config: ApertureConfig) {
    this.baseURL = config.baseURL.replace(/\/$/, '');
    this.timeoutMs = config.timeoutMs;
  }

  /** execute runs a bridge task synchronously. */
  async execute(params: ExecuteParams): Promise<TaskResponse> {
    return this.post<TaskResponse>('/api/v1/bridge/execute?sync=true', {
      goal: params.goal,
      url: params.url,
      config: {
        timeout: params.timeout_seconds ?? 30,
      },
      screenshots: params.screenshots ?? false,
    });
  }

  /** screenshot captures a page screenshot via the actions endpoint. */
  async screenshot(params: ScreenshotParams): Promise<ScreenshotResponse> {
    const result = await this.post<{ data: string; url: string }>(
      '/api/v1/actions/screenshot',
      {
        url: params.url,
        fullPage: params.full_page ?? false,
      },
    );
    return {
      data: result.data,
      mime_type: 'image/png',
      url: result.url,
    };
  }

  /** navigate executes a navigation action on an existing session. */
  async navigate(params: NavigateParams): Promise<TaskResponse> {
    const goalMap: Record<NavigationAction, string> = {
      navigate: `navigate to ${params.url ?? ''}`,
      goBack: 'go back in browser history',
      goForward: 'go forward in browser history',
      reload: 'reload the current page',
    };

    const goal = goalMap[params.action];
    const url = params.action === 'navigate' ? params.url : undefined;

    return this.post<TaskResponse>('/api/v1/bridge/execute?sync=true', {
      goal,
      url,
      session_id: params.session_id,
      config: { timeout: 30 },
    });
  }

  /** poolStatus returns the current state of the browser pool. */
  async poolStatus(): Promise<PoolStatusResponse> {
    return this.get<PoolStatusResponse>('/api/v1/bridge/health');
  }

  /** ExtractParams holds parameters for a data extraction call. */

  /** extract navigates to a URL and extracts structured data via the bridge. */
  async extract(params: {
    url: string;
    schema: string;
    selector?: string;
    format?: string;
  }): Promise<TaskResponse> {
    const selectorClause = params.selector ? ` within "${params.selector}"` : '';
    const formatClause = params.format === 'markdown' ? ' as markdown' : ' as JSON';
    const goal = `Navigate to ${params.url}, then extract: ${params.schema}${selectorClause}${formatClause}`;

    return this.post<TaskResponse>('/api/v1/bridge/execute?sync=true', {
      goal,
      url: params.url,
      config: { timeout: 60 },
    });
  }

  /** vision analyzes a screenshot of a URL using vision AI. */
  async vision(params: VisionParams): Promise<VisionResponse> {
    return this.post<VisionResponse>('/api/v1/actions/vision', {
      url: params.url,
      prompt: params.prompt,
    });
  }

  /** healthCheck verifies the Aperture server is reachable. */
  async healthCheck(): Promise<boolean> {
    try {
      await this.get<{ status: string }>('/health');
      return true;
    } catch {
      return false;
    }
  }

  // ── private helpers ──────────────────────────────────────────────────────────

  private async get<T>(path: string): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const res = await fetch(`${this.baseURL}${path}`, {
        signal: controller.signal,
        headers: { Accept: 'application/json' },
      });
      return this.parseResponse<T>(res);
    } finally {
      clearTimeout(timer);
    }
  }

  private async post<T>(path: string, body: unknown): Promise<T> {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), this.timeoutMs);

    try {
      const res = await fetch(`${this.baseURL}${path}`, {
        method: 'POST',
        signal: controller.signal,
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json',
        },
        body: JSON.stringify(body),
      });
      return this.parseResponse<T>(res);
    } finally {
      clearTimeout(timer);
    }
  }

  private async parseResponse<T>(res: Response): Promise<T> {
    const text = await res.text();
    if (!res.ok) {
      throw new Error(`Aperture API error ${res.status}: ${text}`);
    }
    try {
      return JSON.parse(text) as T;
    } catch {
      throw new Error(`Invalid JSON from Aperture API: ${text}`);
    }
  }
}
