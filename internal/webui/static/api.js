const API_ROOT = "/api/v1";
const DEFAULT_TIMEOUT = 12000;

export class ApiError extends Error {
  constructor(message, options = {}) {
    super(message);
    this.name = "ApiError";
    this.status = options.status || 0;
    this.code = options.code ?? null;
    this.data = options.data ?? null;
    this.requestId = options.requestId || "";
    this.kind = options.kind || "api";
    this.retryAfter = options.retryAfter || "";
  }
}

function authEvent(detail) {
  if (typeof CustomEvent === "function") return new CustomEvent("lte:auth-required", { detail });
  if (typeof Event === "function") return new Event("lte:auth-required");
  return null;
}

export function createApiClient(options = {}) {
  const fetchImpl = options.fetchImpl || globalThis.fetch?.bind(globalThis);
  const eventTarget = options.eventTarget || globalThis;
  let token = "";
  let authGeneration = 0;

  if (!fetchImpl) throw new Error("fetch is required");

  function setToken(value) {
    token = String(value || "").trim();
    authGeneration += 1;
  }

  function clearToken() {
    token = "";
    authGeneration += 1;
  }

  function hasToken() {
    return token.length > 0;
  }

  async function request(path, options = {}) {
    const rawPath = String(path || "");
    const isUIConfig = rawPath === "/ui-config.json";
    const url = isUIConfig ? rawPath : `${API_ROOT}${rawPath.startsWith("/") ? rawPath : `/${rawPath}`}`;
    const method = String(options.method || "GET").toUpperCase();
    const requestAuthGeneration = authGeneration;
    const requestToken = token;
    const headers = new Headers(options.headers || {});
    headers.set("Accept", "application/json");
    headers.set("X-LTE-UI", "1");
    if (requestToken) headers.set("Authorization", `Bearer ${requestToken}`);

    let body = options.body;
    const isFormData = typeof FormData !== "undefined" && body instanceof FormData;
    if (body !== undefined && body !== null && !isFormData && typeof body !== "string") {
      headers.set("Content-Type", "application/json");
      body = JSON.stringify(body);
    }

    const controller = new AbortController();
    let timedOut = false;
    const abortFromCaller = () => controller.abort(options.signal?.reason || "caller");
    if (options.signal?.aborted) abortFromCaller();
    else options.signal?.addEventListener("abort", abortFromCaller, { once: true });
    const timeout = setTimeout(() => {
      timedOut = true;
      controller.abort("timeout");
    }, options.timeout || DEFAULT_TIMEOUT);
    try {
      const response = await fetchImpl(url, {
        method,
        headers,
        body,
        signal: controller.signal,
        cache: method === "GET" ? "no-store" : "default",
        credentials: "same-origin",
      });
      const requestId = response.headers.get("X-Request-ID") || "";
      const retryAfter = response.headers.get("Retry-After") || "";
      let payload = null;
      const contentType = response.headers.get("Content-Type") || "";
      if (contentType.includes("json")) {
        try {
          payload = await response.json();
        } catch (error) {
          if (controller.signal.aborted) throw error;
          throw new ApiError("Invalid JSON response", { status: response.status, requestId, kind: "protocol" });
        }
      }

      if (response.status === 401 && requestAuthGeneration === authGeneration) {
        clearToken();
        const event = authEvent({ requestId });
        if (event && eventTarget?.dispatchEvent) eventTarget.dispatchEvent(event);
      }

      if (!response.ok || (!isUIConfig && payload?.code !== 0)) {
        throw new ApiError(payload?.message || response.statusText || "Request failed", {
          status: response.status,
          code: payload?.code,
          data: payload?.data,
          requestId: payload?.request_id || requestId,
          retryAfter,
        });
      }

      if (isUIConfig) return payload;
      if (!payload || typeof payload !== "object" || !("data" in payload)) {
        throw new ApiError("Unexpected API envelope", { status: response.status, requestId, kind: "protocol" });
      }
      return { data: payload.data, message: payload.message || "", requestId: payload.request_id || requestId };
    } catch (error) {
      if (error instanceof ApiError) throw error;
      if (timedOut) throw new ApiError("Request timed out", { kind: "timeout" });
      if (options.signal?.aborted) throw new ApiError("Request aborted", { kind: "aborted" });
      throw new ApiError(error?.message || "Network request failed", { kind: "offline" });
    } finally {
      clearTimeout(timeout);
      options.signal?.removeEventListener("abort", abortFromCaller);
    }
  }

  return {
    request,
    setToken,
    clearToken,
    hasToken,
    uiConfig: (options = {}) => request("/ui-config.json", { timeout: 6000, ...options }),
    cell: (options = {}) => request("/cell", options),
    profile: (options = {}) => request("/profile", options),
    network: (options = {}) => request("/network", options),
    ues: (options = {}) => request("/ues", options),
    ue: (imsi, options = {}) => request(`/ues/${encodeURIComponent(imsi)}`, options),
    startCell: (configuration = {}, options = {}) => request("/cell", { method: "POST", body: configuration, timeout: 95000, ...options }),
    stopCell: (options = {}) => request("/cell", { method: "DELETE", timeout: 30000, ...options }),
    subscribers: (limit = 50, offset = 0, options = {}) => request(`/subscribers?limit=${limit}&offset=${offset}`, options),
    subscriber: (imsi, options = {}) => request(`/subscribers/${encodeURIComponent(imsi)}`, options),
    diagnostics: (options = {}) => request("/diagnostics/connectivity", { timeout: 15000, ...options }),
    health: (options = {}) => request("/health", { timeout: 28000, ...options }),
  };
}

export const api = createApiClient();
