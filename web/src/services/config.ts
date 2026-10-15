/**
 * Public runtime config only — NEVER put secrets in frontend code.
 * Override at deploy time by defining window.__KIKBITE_CONFIG__ in index.html
 * (or inject via the hosting platform), e.g. { apiBase: "https://api.kikbite.com", mock: false }.
 */
interface RuntimeConfig {
  apiBase: string;
  wsUrl: string;
  /** true = in-memory mock services (default until the real API is wired) */
  mock: boolean;
  /**
   * Live auth + profile against the Go API (apiBase), everything else still mock.
   * Only meaningful while mock === true; set mock:false once every service is implemented.
   */
  liveAuth: boolean;
  /**
   * Live conversations, messages and the real-time socket against the Go API.
   * Only meaningful together with liveAuth (the socket authenticates with the real access token).
   */
  liveChat: boolean;
}

declare global {
  interface Window {
    __KIKBITE_CONFIG__?: Partial<RuntimeConfig>;
  }
}

const defaults: RuntimeConfig = { apiBase: "/api/v1", wsUrl: "/ws", mock: true, liveAuth: false, liveChat: false };
export const config: RuntimeConfig = { ...defaults, ...(typeof window !== "undefined" ? window.__KIKBITE_CONFIG__ : {}) };
