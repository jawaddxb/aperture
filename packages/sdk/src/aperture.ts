import { ApertureClient, type ApertureConfig } from "./client";
import { ApertureSession } from "./session";
import type { SessionConfig, CreateSessionResponse } from "./types";

/** Main Aperture SDK class. Entry point for all browser automation. */
export class Aperture {
  private client: ApertureClient;

  constructor(config: ApertureConfig = {}) {
    this.client = new ApertureClient(config);
  }

  /** Create a new browser session with optional configuration. */
  async session(config?: SessionConfig): Promise<ApertureSession> {
    const goal = config?.agentId
      ? `Agent ${config.agentId} session`
      : "SDK session";

    const res = await this.client.post<CreateSessionResponse>(
      "/api/v1/sessions",
      {
        goal,
        config: config ?? {},
      }
    );

    return new ApertureSession(this.client, res.session_id);
  }
}
