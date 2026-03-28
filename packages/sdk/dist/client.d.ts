/** Configuration for the Aperture HTTP client. */
export interface ApertureConfig {
    apiKey?: string;
    baseUrl?: string;
}
/** Low-level HTTP client for the Aperture REST API. */
export declare class ApertureClient {
    private baseUrl;
    private headers;
    constructor(config?: ApertureConfig);
    post<T>(path: string, body: unknown): Promise<T>;
    get<T>(path: string): Promise<T>;
    put<T>(path: string, body: unknown): Promise<T>;
    delete<T>(path: string): Promise<T>;
    private request;
}
//# sourceMappingURL=client.d.ts.map