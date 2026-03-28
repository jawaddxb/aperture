"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ApertureSession = void 0;
const errors_1 = require("./errors");
/** Represents an active Aperture browser session. */
class ApertureSession {
    constructor(client, sessionId) {
        this.client = client;
        this.sessionId = sessionId;
    }
    /** Navigate to a URL. Returns page state with optional profile data. */
    async navigate(url) {
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
    async click(target) {
        return this.execute({ type: "click", target });
    }
    /** Type text into a target element. */
    async type(target, value) {
        return this.execute({ type: "type", target, value });
    }
    /** Select an option from a dropdown. */
    async select(target, value) {
        return this.execute({ type: "select", target, value });
    }
    /** Scroll in a direction, optionally targeting a specific element. */
    async scroll(direction, target) {
        return this.execute({
            type: "scroll",
            direction,
            ...(target ? { target } : {}),
        });
    }
    /** Wait for a condition (selector visible, URL change, etc.). */
    async wait(condition, timeout) {
        return this.execute({
            type: "wait",
            condition,
            ...(timeout ? { timeout } : {}),
        });
    }
    /** Extract structured data from the page, with optional Zod schema validation. */
    async extract(target, options) {
        const res = await this.execute({ type: "extract", target });
        const data = (res.result ?? {});
        if (options?.schema) {
            const parsed = options.schema.safeParse(data);
            if (!parsed.success) {
                throw new errors_1.ApertureError(`Schema validation failed: ${parsed.error.message}`);
            }
            return { data: parsed.data, source: "extract" };
        }
        return { data, source: "extract" };
    }
    /** Take a screenshot of the current page. */
    async screenshot() {
        const res = await this.execute({ type: "screenshot" });
        return {
            path: res.result?.path ?? "",
            base64: res.result?.base64,
        };
    }
    /** Close the session and release the browser. */
    async close() {
        try {
            await this.client.delete(`/api/v1/sessions/${this.sessionId}`);
        }
        catch {
            // Session may already be deleted.
        }
        return { actionsTaken: 0 };
    }
    /** Core execute helper — sends action to server, parses errors. */
    async execute(action) {
        let res;
        try {
            res = await this.client.post(`/api/v1/sessions/${this.sessionId}/execute`, action);
        }
        catch (err) {
            if (err instanceof errors_1.ApertureError) {
                const msg = err.message.toLowerCase();
                if (msg.includes("policy_blocked") || msg.includes("policy blocked")) {
                    throw new errors_1.PolicyBlockedError(err.message);
                }
                if (msg.includes("not found") || msg.includes("expired")) {
                    throw new errors_1.SessionExpiredError();
                }
                if (msg.includes("disambiguation")) {
                    throw new errors_1.DisambiguationError([]);
                }
            }
            throw err;
        }
        const lastStep = res.steps?.[res.steps.length - 1];
        const actionResult = {
            actionId: `${action.type}-${Date.now()}`,
            status: res.success ? "success" : "error",
            result: lastStep?.result,
            timing: { totalMs: res.duration_ms },
        };
        if (!res.success && lastStep?.result?.error) {
            const errMsg = String(lastStep.result.error);
            if (errMsg.includes("policy_blocked") ||
                errMsg.includes("policy blocked")) {
                throw new errors_1.PolicyBlockedError(errMsg);
            }
        }
        return actionResult;
    }
    /** Extract page state from the last step result. */
    extractPageState(res) {
        const result = res.result;
        if (!result)
            return undefined;
        // Unwrap nested page_state if present.
        const ps = result.page_state;
        return ps;
    }
}
exports.ApertureSession = ApertureSession;
//# sourceMappingURL=session.js.map