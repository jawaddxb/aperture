import { type ApertureConfig } from "./client";
import { ApertureSession } from "./session";
import type { SessionConfig } from "./types";
/** Main Aperture SDK class. Entry point for all browser automation. */
export declare class Aperture {
    private client;
    constructor(config?: ApertureConfig);
    /** Create a new browser session with optional configuration. */
    session(config?: SessionConfig): Promise<ApertureSession>;
}
//# sourceMappingURL=aperture.d.ts.map