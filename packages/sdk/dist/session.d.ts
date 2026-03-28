import { z } from "zod";
import type { ApertureClient } from "./client";
import type { ActionResult, NavigateResult, ExtractResult } from "./types";
/** Represents an active Aperture browser session. */
export declare class ApertureSession {
    private client;
    readonly sessionId: string;
    constructor(client: ApertureClient, sessionId: string);
    /** Navigate to a URL. Returns page state with optional profile data. */
    navigate(url: string): Promise<NavigateResult>;
    /** Click on a target element (selector, AX name, or text). */
    click(target: string): Promise<ActionResult>;
    /** Type text into a target element. */
    type(target: string, value: string): Promise<ActionResult>;
    /** Select an option from a dropdown. */
    select(target: string, value: string): Promise<ActionResult>;
    /** Scroll in a direction, optionally targeting a specific element. */
    scroll(direction: "up" | "down", target?: string): Promise<ActionResult>;
    /** Wait for a condition (selector visible, URL change, etc.). */
    wait(condition: string, timeout?: number): Promise<ActionResult>;
    /** Extract structured data from the page, with optional Zod schema validation. */
    extract<T = Record<string, unknown>>(target: string, options?: {
        schema?: z.ZodType<T>;
    }): Promise<ExtractResult<T>>;
    /** Take a screenshot of the current page. */
    screenshot(): Promise<{
        path: string;
        base64?: string;
    }>;
    /** Close the session and release the browser. */
    close(): Promise<{
        actionsTaken: number;
        budgetConsumed?: number;
    }>;
    /** Core execute helper — sends action to server, parses errors. */
    private execute;
    /** Extract page state from the last step result. */
    private extractPageState;
}
//# sourceMappingURL=session.d.ts.map