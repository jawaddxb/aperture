import { ApertureError } from "./errors";

/** Configuration for the Aperture HTTP client. */
export interface ApertureConfig {
  apiKey?: string;
  baseUrl?: string;
}

/** Low-level HTTP client for the Aperture REST API. */
export class ApertureClient {
  private baseUrl: string;
  private headers: Record<string, string>;

  constructor(config: ApertureConfig = {}) {
    this.baseUrl = (config.baseUrl ?? "http://localhost:8080").replace(
      /\/$/,
      ""
    );
    this.headers = {
      "Content-Type": "application/json",
      ...(config.apiKey
        ? { Authorization: `Bearer ${config.apiKey}` }
        : {}),
    };
  }

  async post<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("POST", path, body);
  }

  async get<T>(path: string): Promise<T> {
    return this.request<T>("GET", path);
  }

  async put<T>(path: string, body: unknown): Promise<T> {
    return this.request<T>("PUT", path, body);
  }

  async delete<T>(path: string): Promise<T> {
    return this.request<T>("DELETE", path);
  }

  private async request<T>(
    method: string,
    path: string,
    body?: unknown
  ): Promise<T> {
    const url = `${this.baseUrl}${path}`;

    const init: RequestInit = {
      method,
      headers: this.headers,
    };

    if (body !== undefined) {
      init.body = JSON.stringify(body);
    }

    let res: Response;
    try {
      res = await fetch(url, init);
    } catch (err) {
      throw new ApertureError(
        `Network error: ${err instanceof Error ? err.message : String(err)}`
      );
    }

    if (!res.ok) {
      let errorBody: { error?: string; code?: string } = {};
      try {
        errorBody = (await res.json()) as { error?: string; code?: string };
      } catch {
        // ignore JSON parse errors
      }
      throw new ApertureError(
        errorBody.error ?? `HTTP ${res.status}: ${res.statusText}`
      );
    }

    // Handle 204 No Content
    if (res.status === 204) {
      return {} as T;
    }

    return res.json() as Promise<T>;
  }
}
