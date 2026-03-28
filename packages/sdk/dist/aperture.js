"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.Aperture = void 0;
const client_1 = require("./client");
const session_1 = require("./session");
/** Main Aperture SDK class. Entry point for all browser automation. */
class Aperture {
    constructor(config = {}) {
        this.client = new client_1.ApertureClient(config);
    }
    /** Create a new browser session with optional configuration. */
    async session(config) {
        const goal = config?.agentId
            ? `Agent ${config.agentId} session`
            : "SDK session";
        const res = await this.client.post("/api/v1/sessions", {
            goal,
            config: config ?? {},
        });
        return new session_1.ApertureSession(this.client, res.session_id);
    }
}
exports.Aperture = Aperture;
//# sourceMappingURL=aperture.js.map