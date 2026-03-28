import { z } from "zod";
import type { ApertureClient } from "./client";
import type {
  ActionResult,
  NavigateResult,
  ExtractResult,
  ExecuteResponse,
} from "./types";
import {
  ApertureError,
  PolicyBlockedError,
  DisambiguationError,
  SessionExpiredError,
} from "./errors";

/** Represents an active Aperture browser session. */
export class ApertureSession {
  constructor(
    private client: ApertureClient,
    public readonly sessionId: string
  ) {}

  /** Navigate to a URL. Returns page state with optional profile data. */
  async navigate(url: string): Promise<NavigateResult> {
    const res = await this.execute({ type: "navigate", url });

    const pageState = this.extractPageState(res);
    return {
      ...res,
      url: pageState?.url ?? url,
      title: pageState?.title ?? "",
      profileMatched: pageState?.profile_matched,
      structuredData: pageState?.structured_data,
      availableActions: pageState?.available_actions,
    };
  }

  /** Click on a target element (selector, AX name, or text). */
  async click(target: string): Promise<ActionResult> {
    return this.execute({ type: "click", target });
  }

  /** Type text into a target element. */
  async type(target: string, value: string): Promise<ActionResult> {
    return this.execute({ type: "type", target, value });
  }

  /** Select an option from a dropdown. */
  async select(target: string, value: string): Promise<ActionResult> {
    return this.execute({ type: "select", target, value });
  }

  /** Scroll in a direction, optionally targeting a specific element. */
  async scroll(
    direction: "up" | "down",
    target?: string
  ): Promise<ActionResult> {
    return this.execute({
      type: "scroll",
      direction,
      ...(target ? { target } : {}),
    });
  }

  /** Wait for a condition (selector visible, URL change, etc.). */
  async wait(condition: string, timeout?: number): Promise<ActionResult> {
    return this.execute({
      type: "wait",
      condition,
      ...(timeout ? { timeout } : {}),
    });
  }

  /** Extract structured data from the page, with optional Zod schema validation. */
  async extract<T = Record<string, unknown>>(
    target: string,
    options?: { schema?: z.ZodType<T> }
  ): Promise<ExtractResult<T>> {
    const res = await this.execute({ type: "extract", target });
    const data = (res.result ?? {}) as T;

    if (options?.schema) {
      const parsed = options.schema.safeParse(data);
      if (!parsed.success) {
        throw new ApertureError(
          `Schema validation failed: ${parsed.error.message}`
        );
      }
      return { data: parsed.data, source: "extract" };
    }

    return { data, source: "extract" };
  }

  /** Take a screenshot of the current page. */
  async screenshot(): Promise<{ path: string; base64?: string }> {
    const res = await this.execute({ type: "screenshot" });
    return {
      path: (res.result?.path as string) ?? "",
      base64: res.result?.base64 as string | undefined,
    };
  }

  /** Close the session and release the browser. */
  async close(): Promise<{ actionsTaken: number; budgetConsumed?: number }> {
    try {
      await this.client.delete(
        `/api/v1/sessions/${this.sessionId}`
      );
    } catch {
      // Session may already be deleted.
    }
    return { actionsTaken: 0 };
  }

  /** Core execute helper — sends action to server, parses errors. */
  private async execute(
    action: Record<string, unknown>
  ): Promise<ActionResult> {
    let res: ExecuteResponse;
    try {
      res = await this.client.post<ExecuteResponse>(
        `/api/v1/sessions/${this.sessionId}/execute`,
        action
      );
    } catch (err) {
      if (err instanceof ApertureError) {
        const msg = err.message.toLowerCase();
        if (msg.includes("policy_blocked") || msg.includes("policy blocked")) {
          throw new PolicyBlockedError(err.message);
        }
        if (msg.includes("not found") || msg.includes("expired")) {
          throw new SessionExpiredError();
        }
        if (msg.includes("disambiguation")) {
          throw new DisambiguationError([]);
        }
      }
      throw err;
    }

    const lastStep = res.steps?.[res.steps.length - 1];
    const actionResult: ActionResult = {
      actionId: `${action.type}-${Date.now()}`,
      status: res.success ? "success" : "error",
      result: lastStep?.result as unknown as Record<string, unknown>,
      timing: { totalMs: res.duration_ms },
    };

    if (!res.success && lastStep?.result?.error) {
      const errMsg = String(lastStep.result.error);
      if (
        errMsg.includes("policy_blocked") ||
        errMsg.includes("policy blocked")
      ) {
        throw new PolicyBlockedError(errMsg);
      }
    }

    return actionResult;
  }

  /** Extract page state from the last step result. */
  private extractPageState(res: ActionResult) {
    const result = res.result as Record<string, unknown> | undefined;
    if (!result) return undefined;

    // Unwrap nested page_state if present.
    const ps = result.page_state as
      | {
          url?: string;
          title?: string;
          profile_matched?: string;
          structured_data?: Record<string, unknown>;
          available_actions?: string[];
        }
      | undefined;

    return ps;
  }
}
