"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.ApertureClient = void 0;
const errors_1 = require("./errors");
/** Low-level HTTP client for the Aperture REST API. */
class ApertureClient {
    constructor(config = {}) {
        this.baseUrl = (config.baseUrl ?? "http://localhost:8080").replace(/\/$/, "");
        this.headers = {
            "Content-Type": "application/json",
            ...(config.apiKey
                ? { Authorization: `Bearer ${config.apiKey}` }
                : {}),
        };
    }
    async post(path, body) {
        return this.request("POST", path, body);
    }
    async get(path) {
        return this.request("GET", path);
    }
    async put(path, body) {
        return this.request("PUT", path, body);
    }
    async delete(path) {
        return this.request("DELETE", path);
    }
    async request(method, path, body) {
        const url = `${this.baseUrl}${path}`;
        const init = {
            method,
            headers: this.headers,
        };
        if (body !== undefined) {
            init.body = JSON.stringify(body);
        }
        let res;
        try {
            res = await fetch(url, init);
        }
        catch (err) {
            throw new errors_1.ApertureError(`Network error: ${err instanceof Error ? err.message : String(err)}`);
        }
        if (!res.ok) {
            let errorBody = {};
            try {
                errorBody = (await res.json());
            }
            catch {
                // ignore JSON parse errors
            }
            throw new errors_1.ApertureError(errorBody.error ?? `HTTP ${res.status}: ${res.statusText}`);
        }
        // Handle 204 No Content
        if (res.status === 204) {
            return {};
        }
        return res.json();
    }
}
exports.ApertureClient = ApertureClient;
//# sourceMappingURL=client.js.map