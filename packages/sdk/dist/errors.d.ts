import type { Candidate } from "./types";
/** Base error class for all Aperture SDK errors. */
export declare class ApertureError extends Error {
    constructor(message: string);
}
/** Thrown when an action is blocked by an xBPP policy. */
export declare class PolicyBlockedError extends ApertureError {
    readonly reason: string;
    constructor(reason: string);
}
/** Thrown when the server returns multiple matching elements. */
export declare class DisambiguationError extends ApertureError {
    readonly candidates: Candidate[];
    constructor(candidates: Candidate[]);
}
/** Thrown when the session's budget is exhausted. */
export declare class BudgetExhaustedError extends ApertureError {
    readonly consumed: number;
    readonly allocated: number;
    constructor(consumed: number, allocated: number);
}
/** Thrown when the session has expired or been deleted. */
export declare class SessionExpiredError extends ApertureError {
    constructor();
}
//# sourceMappingURL=errors.d.ts.map