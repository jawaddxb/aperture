"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.SessionExpiredError = exports.BudgetExhaustedError = exports.DisambiguationError = exports.PolicyBlockedError = exports.ApertureError = void 0;
/** Base error class for all Aperture SDK errors. */
class ApertureError extends Error {
    constructor(message) {
        super(message);
        this.name = "ApertureError";
        Object.setPrototypeOf(this, new.target.prototype);
    }
}
exports.ApertureError = ApertureError;
/** Thrown when an action is blocked by an xBPP policy. */
class PolicyBlockedError extends ApertureError {
    constructor(reason) {
        super(`Action blocked by policy: ${reason}`);
        this.name = "PolicyBlockedError";
        this.reason = reason;
    }
}
exports.PolicyBlockedError = PolicyBlockedError;
/** Thrown when the server returns multiple matching elements. */
class DisambiguationError extends ApertureError {
    constructor(candidates) {
        super(`Disambiguation required: ${candidates.length} candidates found`);
        this.name = "DisambiguationError";
        this.candidates = candidates;
    }
}
exports.DisambiguationError = DisambiguationError;
/** Thrown when the session's budget is exhausted. */
class BudgetExhaustedError extends ApertureError {
    constructor(consumed, allocated) {
        super(`Budget exhausted: consumed ${consumed} of ${allocated} credits`);
        this.name = "BudgetExhaustedError";
        this.consumed = consumed;
        this.allocated = allocated;
    }
}
exports.BudgetExhaustedError = BudgetExhaustedError;
/** Thrown when the session has expired or been deleted. */
class SessionExpiredError extends ApertureError {
    constructor() {
        super("Session has expired or been deleted");
        this.name = "SessionExpiredError";
    }
}
exports.SessionExpiredError = SessionExpiredError;
//# sourceMappingURL=errors.js.map