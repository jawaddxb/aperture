/** Configuration for creating a new session. */
export interface SessionConfig {
  agentId?: string;
  viewport?: { width: number; height: number };
  locale?: string;
  timezone?: string;
  policy?: {
    domainAllowlist?: string[];
    maxActions?: number;
    budgetCredits?: number;
  };
  ttlSeconds?: number;
  restoreMemory?: boolean;
}

/** Result of any action execution. */
export interface ActionResult {
  actionId: string;
  status: "success" | "error" | "disambiguation_required";
  cost?: number;
  result?: Record<string, unknown>;
  timing?: { totalMs: number };
}

/** Snapshot of the current page state. */
export interface PageSnapshot {
  url: string;
  title: string;
  profileMatched?: string;
  structuredData?: Record<string, unknown>;
  availableActions?: string[];
}

/** Result of a navigate action. */
export interface NavigateResult extends ActionResult {
  url: string;
  title: string;
  profileMatched?: string;
  structuredData?: Record<string, unknown>;
  availableActions?: string[];
}

/** Result of a data extraction action. */
export interface ExtractResult<T = Record<string, unknown>> {
  data: T;
  confidence?: number;
  source?: string;
}

/** A disambiguation candidate when multiple elements match. */
export interface Candidate {
  index: number;
  selector: string;
  text: string;
  role?: string;
}

/** Raw API response from the server. */
export interface APIResponse<T = unknown> {
  data?: T;
  error?: string;
  code?: string;
}

/** Session creation response from the server. */
export interface CreateSessionResponse {
  session_id: string;
  status: string;
}

/** Session close response. */
export interface CloseSessionResponse {
  actionsTaken: number;
  budgetConsumed?: number;
}

/** Execute response from the server. */
export interface ExecuteResponse {
  success: boolean;
  steps: Array<{
    step: { action: string; params: Record<string, unknown> };
    result?: {
      action: string;
      success: boolean;
      error?: string;
      page_state?: {
        url: string;
        title: string;
        profile_matched?: string;
        structured_data?: Record<string, unknown>;
        available_actions?: string[];
      };
    };
  }>;
  duration_ms: number;
}
