// Core classes
export { Aperture } from "./aperture";
export { ApertureSession } from "./session";
export { ApertureClient } from "./client";
export type { ApertureConfig } from "./client";

// Types
export type {
  SessionConfig,
  ActionResult,
  PageSnapshot,
  NavigateResult,
  ExtractResult,
  Candidate,
  APIResponse,
  CreateSessionResponse,
  CloseSessionResponse,
  ExecuteResponse,
} from "./types";

// Errors
export {
  ApertureError,
  PolicyBlockedError,
  DisambiguationError,
  BudgetExhaustedError,
  SessionExpiredError,
} from "./errors";
