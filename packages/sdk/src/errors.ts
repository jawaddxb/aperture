import type { Candidate } from "./types";

/** Base error class for all Aperture SDK errors. */
export class ApertureError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "ApertureError";
    Object.setPrototypeOf(this, new.target.prototype);
  }
}

/** Thrown when an action is blocked by an xBPP policy. */
export class PolicyBlockedError extends ApertureError {
  public readonly reason: string;

  constructor(reason: string) {
    super(`Action blocked by policy: ${reason}`);
    this.name = "PolicyBlockedError";
    this.reason = reason;
  }
}

/** Thrown when the server returns multiple matching elements. */
export class DisambiguationError extends ApertureError {
  public readonly candidates: Candidate[];

  constructor(candidates: Candidate[]) {
    super(
      `Disambiguation required: ${candidates.length} candidates found`
    );
    this.name = "DisambiguationError";
    this.candidates = candidates;
  }
}

/** Thrown when the session's budget is exhausted. */
export class BudgetExhaustedError extends ApertureError {
  public readonly consumed: number;
  public readonly allocated: number;

  constructor(consumed: number, allocated: number) {
    super(
      `Budget exhausted: consumed ${consumed} of ${allocated} credits`
    );
    this.name = "BudgetExhaustedError";
    this.consumed = consumed;
    this.allocated = allocated;
  }
}

/** Thrown when the session has expired or been deleted. */
export class SessionExpiredError extends ApertureError {
  constructor() {
    super("Session has expired or been deleted");
    this.name = "SessionExpiredError";
  }
}
