import { api, ApiError } from "./api.js";
import { applyTranslations, normalizeLocale, translate } from "./i18n.js";

const $ = (selector, root = document) => root.querySelector(selector);

const store = {
  get(key, fallback) { try { return localStorage.getItem(key) || fallback; } catch { return fallback; } },
  set(key, value) { try { localStorage.setItem(key, value); } catch { /* display preferences are optional */ } },
};
let locale = normalizeLocale(store.get("lte-ui-language", navigator.language));
let theme = store.get("lte-ui-theme", matchMedia("(prefers-color-scheme: dark)").matches ? "dark" : "light");
const resource = () => ({ loading: false, data: null, error: null, updatedAt: 0 });
const state = {
  route: "overview", config: resource(), cell: resource(), profile: resource(), network: resource(), ues: resource(),
  subscribers: resource(), subscriber: resource(), diagnostics: resource(), health: resource(),
  subscriberOffset: 0, selectedUE: null, selectedUEStale: false, selectedSubscriber: null, operation: null,
  pendingStart: null, pendingStartMode: "saved",
  authenticated: false, authBlocked: false, pollPaused: false,
  draft: { network: "auto", ue_subnet: "172.16.0.0/24", ue_access: "isolated", apn_mismatch_policy: "strict", sdr: "auto" },
};
const routes = [
  ["overview", "nav.overview", "overview"], ["cell", "nav.cell", "cell"], ["ues", "nav.ues", "device"],
  ["subscribers", "nav.subscribers", "users"], ["diagnostics", "nav.diagnostics", "pulse"], ["settings", "nav.settings", "settings"],
];
let pollTimer = 0;
let pollController = null;
let pollInFlight = false;
let detailReturnFocus = null;

function t(key, params) { return translate(locale, key, params); }
function append(node, child) {
  if (child === null || child === undefined || child === false) return;
  node.append(child instanceof Node ? child : document.createTextNode(String(child)));
}
function h(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [key, value] of Object.entries(props || {})) {
    if (value === undefined || value === null || value === false) continue;
    if (key === "class") node.className = value;
    else if (key === "text") node.textContent = String(value);
    else if (key === "on") Object.entries(value).forEach(([event, fn]) => node.addEventListener(event, fn));
    else if (key === "checked" || key === "disabled" || key === "required" || key === "hidden") node[key] = Boolean(value);
    else if (key === "value") node.value = String(value);
    else if (key === "tabindex") node.tabIndex = value;
    else if (key in node && !key.startsWith("aria") && !key.startsWith("data-")) node[key] = value;
    else node.setAttribute(key, String(value));
  }
  children.flat(Infinity).forEach((child) => append(node, child));
  return node;
}
function svg(name, className = "") {
  const ns = "http://www.w3.org/2000/svg";
  const root = document.createElementNS(ns, "svg");
  root.setAttribute("viewBox", "0 0 24 24"); root.setAttribute("aria-hidden", "true");
  if (className) root.setAttribute("class", className);
  const paths = {
    overview: ["M4 4h6v6H4zM14 4h6v10h-6zM4 14h6v6H4zM14 18h6v2h-6z"],
    cell: ["M12 4v16M8 20h8M9 14l3-7 3 7M5 9a8 8 0 0 1 14 0M8 11a5 5 0 0 1 8 0"],
    device: ["M7 3h10a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2M9 17h6"],
    users: ["M16 20v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M9 10a4 4 0 1 0 0-8M17 11a4 4 0 0 1 0 7.8"],
    pulse: ["M3 12h4l2-7 4 14 2-7h6"], settings: ["M12 15.5a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7M19.4 15a1.7 1.7 0 0 0 .34 1.88l.05.05-2.12 2.12-.05-.05a1.7 1.7 0 0 0-1.88-.34 1.7 1.7 0 0 0-1.03 1.56V20h-3v-.08a1.7 1.7 0 0 0-1.1-1.56 1.7 1.7 0 0 0-1.88.34l-.05.05-2.12-2.12.05-.05A1.7 1.7 0 0 0 6.95 15a1.7 1.7 0 0 0-1.56-1.03H5v-3h.08A1.7 1.7 0 0 0 6.64 9.9a1.7 1.7 0 0 0-.34-1.88l-.05-.05 2.12-2.12.05.05a1.7 1.7 0 0 0 1.88.34A1.7 1.7 0 0 0 11.33 4.7V4h3v.08a1.7 1.7 0 0 0 1.03 1.56 1.7 1.7 0 0 0 1.88-.34l.05-.05 2.12 2.12-.05.05a1.7 1.7 0 0 0-.34 1.88 1.7 1.7 0 0 0 1.56 1.03H21v3h-.08A1.7 1.7 0 0 0 19.4 15Z"],
  };
  for (const d of paths[name] || paths.overview) { const path = document.createElementNS(ns, "path"); path.setAttribute("d", d); root.append(path); }
  return root;
}
function button(label, kind = "secondary", onClick, props = {}) { return h("button", { type: "button", class: `button ${kind}`, on: { click: onClick }, ...props }, label); }
function badge(label, tone = "") { return h("span", { class: `badge ${tone}`.trim(), text: label }); }
function labelState(value) {
  const raw = String(value ?? "");
  const labels = {
    zh: { running: "运行中", stopped: "已停止", current: "当前", missing: "缺失", stale: "过期", invalid: "无效", cell_stopped: "小区已停止", registered: "已注册", idle: "空闲", connected: "连接中", active: "活跃", normal: "正常", restricted: "受限", deny: "拒绝", not_observed: "未观察到", observed: "已观察到", configured: "已配置", unavailable: "不可用", unknown: "未知", not_collected: "未收集", complete: "完整" },
    en: { running: "Running", stopped: "Stopped", current: "Current", missing: "Missing", stale: "Stale", invalid: "Invalid", cell_stopped: "Cell stopped", registered: "Registered", idle: "Idle", connected: "Connected", active: "Active", normal: "Normal", restricted: "Restricted", deny: "Denied", not_observed: "Not observed", observed: "Observed", configured: "Configured", unavailable: "Unavailable", unknown: "Unknown", not_collected: "Not collected", complete: "Complete" },
  };
  return labels[locale][raw] || raw.replaceAll("_", " ") || t("common.unknown");
}
function toneFor(value) {
  if ([true, "running", "current", "registered", "active", "normal", "response_observed", "observed", "complete"].includes(value)) return "success";
  if (["restricted", "stale", "partial", "query_without_response_observed", "busy"].includes(value)) return "warning";
  if ([false, "invalid", "deny", "unavailable", "error"].includes(value)) return "danger";
  return "";
}
function formatTime(value) {
  if (!value) return t("common.unknown");
  const date = typeof value === "number" ? new Date(value) : new Date(String(value));
  if (Number.isNaN(date.getTime())) return String(value);
  return new Intl.DateTimeFormat(locale === "zh" ? "zh-CN" : "en", { dateStyle: "medium", timeStyle: "medium" }).format(date);
}
function valueText(value) {
  if (value === null || value === undefined || value === "") return t("common.notReported");
  if (typeof value === "boolean") return value ? t("common.yes") : t("common.no");
  return String(value);
}
function pageHeader(eyebrow, title, subtitle, actions = []) {
  return h("header", { class: "page-header" }, h("div", {}, h("p", { class: "eyebrow", text: t(eyebrow) }), h("h1", { tabindex: -1, text: t(title) }), h("p", { text: t(subtitle) })), actions.length ? h("div", { class: "page-actions" }, actions) : null);
}
function card(title, description, content, actions = null, extra = "") {
  return h("section", { class: `card card-pad ${extra}`.trim() }, h("div", { class: "card-head" }, h("div", {}, h("h2", { text: title }), description ? h("p", { text: description }) : null), actions), content);
}
function loadingPanel() { return h("div", { class: "state-panel", role: "status" }, h("div", {}, h("div", { class: "skeleton line" }), h("div", { class: "skeleton line short" }), h("p", { text: t("common.loading") }))); }
function emptyPanel(title, detail) { return h("div", { class: "state-panel" }, h("div", {}, h("div", { class: "state-symbol", text: "—" }), h("h3", { text: title }), h("p", { text: detail || "" }))); }
function errorText(error) {
  if (error?.kind === "offline") return t("error.offline");
  if (error?.kind === "timeout") return t("error.timeout");
  if (error?.kind === "protocol") return t("error.protocol");
  if (error?.status === 401) return t("error.unauthorized");
  if (error?.status === 429) return t("error.busy");
  if (error?.status === 422) return t("error.validation");
  if (error?.status === 503) return t("error.hardware");
  return t("error.generic");
}
function errorPanel(error, retry) {
  const fields = Array.isArray(error?.data?.errors) ? error.data.errors : [];
  return h("div", { class: "error-panel", role: "alert" }, h("strong", { text: t("error.title") }), h("p", { text: errorText(error) }), error?.message ? h("p", { text: error.message }) : null,
    fields.length ? h("ul", {}, fields.map((item) => h("li", { text: t("error.field", { field: item.field, reason: item.reason }) }))) : null,
    error?.requestId ? h("span", { class: "request-id", text: t("common.requestId", { id: error.requestId }) }) : null,
    retry ? button(t("common.retry"), "secondary small", retry) : null);
}
function summary(entries) { return h("dl", { class: "summary-list" }, entries.map(([term, value]) => h("div", { class: "summary-item" }, h("dt", { text: term }), h("dd", { text: valueText(value) })))); }
function notice(title, text, tone = "info") { return h("div", { class: `notice ${tone}` }, h("span", { class: "notice-mark", text: tone === "warning" ? "!" : "i" }), h("div", {}, h("strong", { text: title }), h("p", { text }))); }
function resourceView(res, render, retry) {
  if (res.loading && !res.data) return loadingPanel();
  if (res.error && !res.data) return errorPanel(res.error, retry);
  return h("div", {}, res.error ? h("div", { class: "stack" }, badge(t("common.stale"), "warning"), errorPanel(res.error, retry)) : null, res.data ? render(res.data) : emptyPanel(t("common.noData")));
}
function toast(message, error = false) {
  const node = h("div", { class: `toast${error ? " error" : ""}`, text: message });
  $("#toast-region").append(node); setTimeout(() => node.remove(), 4500);
}

async function fetchInto(name, fetcher, signal) {
  const res = state[name]; if (!res.data) res.loading = true; res.error = null;
  const sequence = res.sequence = (res.sequence || 0) + 1;
  try { const result = await fetcher(signal); if (sequence === res.sequence) { res.data = result.data; res.updatedAt = Date.now(); } }
  catch (error) { if (sequence === res.sequence && error?.kind !== "aborted") res.error = error; }
  finally { if (sequence === res.sequence) res.loading = false; }
}
function stopPolling(reason = "pause") {
  clearTimeout(pollTimer); pollTimer = 0; state.pollPaused = true;
  if (pollController) pollController.abort(reason);
  updateConnection();
}
function canPoll() { return !document.hidden && navigator.onLine && !state.authBlocked; }
async function poll() {
  if (!canPoll() || pollInFlight) return;
  pollInFlight = true; state.pollPaused = false;
  const controller = new AbortController(); pollController = controller;
  await Promise.all([
    fetchInto("cell", (signal) => api.cell({ signal }), controller.signal),
    fetchInto("network", (signal) => api.network({ signal }), controller.signal),
    fetchInto("ues", (signal) => api.ues({ signal }), controller.signal),
  ]);
  reconcileSelectedUE();
  if (pollController === controller) pollController = null;
  pollInFlight = false; updateConnection();
  if (["overview", "ues"].includes(state.route)) renderPage(false);
  else if (state.route === "cell") updateCellRuntime();
  if (canPoll()) pollTimer = setTimeout(poll, 5000);
}
function resumePolling() { clearTimeout(pollTimer); if (canPoll()) pollTimer = setTimeout(poll, 50); }
async function loadProfile() { await fetchInto("profile", () => api.profile()); if (state.route === "cell") renderPage(false); }
async function loadSubscribers(offset = state.subscriberOffset) {
  state.subscriberOffset = Math.max(0, offset); state.subscribers.loading = true; renderPage(false);
  await fetchInto("subscribers", () => api.subscribers(50, state.subscriberOffset)); renderPage(false);
}
async function refreshLight() {
  stopPolling("manual");
  while (pollInFlight) await new Promise((resolve) => setTimeout(resolve, 0));
  await poll();
}

function renderNav() {
  const nav = $("#primary-nav"); nav.replaceChildren();
  routes.forEach(([route, key, icon]) => {
    const link = h("a", { href: `#/${route}` }, svg(icon, "nav-icon"), h("span", { text: t(key) }));
    if (route === state.route) link.setAttribute("aria-current", "page"); nav.append(link);
  });
}
function updateConnection() {
  const node = $("#connection-state"); node.className = "connection-state";
  let text = t("common.connecting");
  if (!navigator.onLine) { node.classList.add("offline"); text = t("common.offline"); }
  else if (state.authBlocked || document.hidden) text = t("common.paused");
  else {
    const polled = [state.cell, state.network, state.ues];
    const newest = Math.max(...polled.map((item) => item.updatedAt || 0));
    const failed = polled.some((item) => item.error);
    if (failed || (newest && Date.now() - newest > 15000)) { node.classList.add("stale"); text = t("common.stale"); }
    else if (newest) { node.classList.add("online"); text = t("common.connected"); }
  }
  node.lastElementChild.textContent = text;
  const alert = $("#global-alert");
  alert.hidden = navigator.onLine; if (!navigator.onLine) alert.textContent = t("error.offline");
  $("#auth-button").classList.toggle("authenticated", api.hasToken());
  $("#auth-button").setAttribute("aria-label", t("auth.session"));
}
const focusableSelector = 'button:not(:disabled), a[href], input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]';
function describeFocus(element, root) {
  if (!element || element === root || !root.contains(element)) return null;
  return { id: element.id, tag: element.tagName, text: element.textContent, name: element.getAttribute("name"), href: element.getAttribute("href"), index: [...root.querySelectorAll(focusableSelector)].indexOf(element), start: element.selectionStart, end: element.selectionEnd };
}
function restoreFocus(description, root) {
  if (!description) return false;
  const items = [...root.querySelectorAll(focusableSelector)].filter(e => e.getClientRects().length);
  const target = items.find(e => description.id && e.id === description.id) || items.find(e => e.tagName === description.tag && e.textContent === description.text && e.getAttribute("name") === description.name && e.getAttribute("href") === description.href) || items[description.index];
  if (!target) return false;
  target.focus({ preventScroll: true });
  if (typeof description.start === "number" && target.setSelectionRange) { try { target.setSelectionRange(description.start, description.end); } catch { /* number/select controls have no text caret */ } }
  return true;
}
function renderPage(focus = false) {
  const app = $("#app"), oldPanel = $(".detail-panel", app);
  const previousFocus = describeFocus(document.activeElement, app);
  const panelFocus = oldPanel ? describeFocus(document.activeElement, oldPanel) : null;
  app.replaceChildren();
  const renderer = { overview: renderOverview, cell: renderCell, ues: renderUEs, subscribers: renderSubscribers, diagnostics: renderDiagnostics, settings: renderSettings }[state.route] || renderOverview;
  app.append(renderer());
  const panel = $(".detail-panel", app);
  $(".sidebar").inert = Boolean(panel);
  $(".topbar").inert = Boolean(panel);
  for (const child of app.firstElementChild?.children || []) child.inert = Boolean(panel) && !child.contains(panel);
  if (panel) {
    if (!oldPanel) detailReturnFocus = previousFocus;
    if (!restoreFocus(panelFocus, panel)) $("button", panel)?.focus({ preventScroll: true });
  } else if (oldPanel) {
    restoreFocus(detailReturnFocus, app); detailReturnFocus = null;
  } else if (!focus) restoreFocus(previousFocus, app);
  $("#route-label").textContent = t(`nav.${state.route}`);
  if (focus) requestAnimationFrame(() => { window.scrollTo({ top: 0, left: 0, behavior: "auto" }); app.focus({ preventScroll: true }); });
}

window.addEventListener("keydown", (event) => {
  const panel = $(".detail-panel");
  if (!panel || $("dialog[open]")) return;
  if (event.key === "Escape") {
    event.preventDefault(); state.selectedUE = null; state.selectedSubscriber = null; renderPage(false);
  } else if (event.key === "Tab") {
    const items = [...panel.querySelectorAll(focusableSelector)].filter(e => e.getClientRects().length);
    const first = items[0], last = items.at(-1);
    if (event.shiftKey && (document.activeElement === first || !panel.contains(document.activeElement))) { event.preventDefault(); last?.focus(); }
    else if (!event.shiftKey && (document.activeElement === last || !panel.contains(document.activeElement))) { event.preventDefault(); first?.focus(); }
  }
});
function setLocale(next) {
  locale = normalizeLocale(next); store.set("lte-ui-language", locale); document.documentElement.lang = locale === "zh" ? "zh-CN" : "en";
  applyTranslations(document, locale); $("#language-button").textContent = locale === "zh" ? "EN" : "中"; renderNav(); renderPage(false); updateConnection();
}
function setTheme(next) { theme = next === "dark" ? "dark" : "light"; store.set("lte-ui-theme", theme); document.documentElement.dataset.theme = theme; }
function openAuth(message = "") {
  const dialog = $("#auth-dialog"); $("#auth-error").hidden = !message; $("#auth-error").textContent = message;
  $("#token-input").value = ""; if (!dialog.open) dialog.showModal(); requestAnimationFrame(() => $("#token-input").focus());
}
function parseRoute() {
  state.selectedUE = null; state.selectedSubscriber = null;
  const wanted = location.hash.replace(/^#\/?/, "").split("/")[0]; state.route = routes.some(([route]) => route === wanted) ? wanted : "overview";
  document.body.classList.remove("nav-open"); $("#menu-button").setAttribute("aria-expanded", "false");
  renderNav(); renderPage(true); if (state.route === "subscribers" && !state.subscribers.data && !state.subscribers.loading) loadSubscribers();
}

async function connect() {
  state.authBlocked = false; state.authenticated = true; $("#auth-dialog").close();
  await Promise.all([loadProfile(), poll()]);
  if (!state.authBlocked && state.route === "subscribers") await loadSubscribers();
  renderPage(false);
}
async function boot() {
  setTheme(theme); setLocale(locale); parseRoute();
  try { state.config.loading = true; state.config.data = await api.uiConfig(); state.config.updatedAt = Date.now(); }
  catch (error) { state.config.error = error; }
  finally { state.config.loading = false; }
  if (state.config.data?.auth_required) { state.authBlocked = true; openAuth(); }
  else connect();
  renderPage(false); updateConnection();
}

$("#language-button").addEventListener("click", () => setLocale(locale === "zh" ? "en" : "zh"));
$("#theme-button").addEventListener("click", () => { setTheme(theme === "light" ? "dark" : "light"); renderPage(false); });
$("#menu-button").addEventListener("click", () => { const open = document.body.classList.toggle("nav-open"); $("#menu-button").setAttribute("aria-expanded", String(open)); });
$("#nav-scrim").addEventListener("click", () => { document.body.classList.remove("nav-open"); $("#menu-button").setAttribute("aria-expanded", "false"); });
$("#auth-button").addEventListener("click", () => openAuth());
$("#auth-form").addEventListener("submit", (event) => { event.preventDefault(); api.setToken($("#token-input").value); connect(); });
$("#confirm-dialog").addEventListener("close", () => { if ($("#confirm-dialog").returnValue === "confirm") stopCell(); });
$("#start-dialog").addEventListener("close", () => {
  const confirmed = $("#start-dialog").returnValue === "confirm";
  const payload = state.pendingStart;
  state.pendingStart = null;
  if (confirmed && payload) startCell(payload);
});
window.addEventListener("hashchange", parseRoute);
window.addEventListener("lte:auth-required", () => { state.authBlocked = true; stopPolling("auth"); openAuth(t("auth.invalid")); });
window.addEventListener("offline", () => stopPolling("offline"));
window.addEventListener("online", () => { updateConnection(); resumePolling(); });
document.addEventListener("visibilitychange", () => { if (document.hidden) stopPolling("visibility"); else resumePolling(); });

function towerArt() {
  const ns = "http://www.w3.org/2000/svg", root = document.createElementNS(ns, "svg");
  root.setAttribute("viewBox", "0 0 480 260"); root.setAttribute("class", "tower-art"); root.setAttribute("role", "img"); root.setAttribute("aria-label", t("overview.signalArt"));
  const line = (x1, y1, x2, y2, cls) => { const n = document.createElementNS(ns, "line"); for (const [k,v] of Object.entries({x1,y1,x2,y2,class:cls})) n.setAttribute(k,v); root.append(n); };
  for (let i = -80; i < 500; i += 40) { line(i, 230, i + 190, 135, "grid-line"); line(i, 135, i + 190, 230, "grid-line"); }
  const path = (d, cls) => { const n = document.createElementNS(ns, "path"); n.setAttribute("d", d); n.setAttribute("class", cls); root.append(n); };
  path("M170 218h140l28 15-70 24-126-22Z", "base"); path("M240 58v166M215 224l25-166 25 166M220 170h40M226 130h28M232 92h16", "tower"); path("M220 76c-22 12-22 34 0 46M260 76c22 12 22 34 0 46M205 59c-42 23-42 62 0 85M275 59c42 23 42 62 0 85", "signal");
  return root;
}

function renderOverview() {
  const cell = state.cell.data, ues = state.ues.data, network = state.network.data;
  const current = ues?.state === "current", sessions = current && Array.isArray(ues.sessions) ? ues.sessions : [];
  const registered = sessions.filter((u) => u.emm_state === "registered" && u.access_policy !== "restricted").length;
  const restricted = sessions.filter((u) => u.access_policy === "restricted").length, other = Math.max(0, sessions.length - registered - restricted);
  const hero = h("section", { class: "card overview-visual" }, h("div", { class: "overview-copy" }, h("p", { class: "eyebrow", text: t("overview.eyebrow") }), h("h1", { tabindex: -1, text: t("overview.title") }), h("p", { text: t("overview.subtitle") }), h("div", { class: "hero-badges" }, badge(cell?.running ? t("overview.running") : t("overview.stopped"), cell?.running ? "success" : ""), badge(t("overview.telemetry", { state: labelState(ues?.state) }), toneFor(ues?.state)))), towerArt());
  const metrics = h("div", { class: "metric-grid" },
    metric(t("overview.cellState"), cell ? (cell.running ? t("overview.running") : t("overview.stopped")) : "—", t("overview.cellStateDesc")),
    metric(t("overview.liveUE"), current ? String(sessions.length) : "—", t("overview.liveUEDesc")),
    metric(t("overview.networkPlan"), network ? (network.active ? t("common.active") : t("common.inactive")) : "—", t("overview.networkPlanDesc")),
    metric(t("overview.telemetryState"), ues ? labelState(ues.state) : "—", t("overview.telemetryDesc")));
  const topology = card(t("overview.topology"), t("overview.topologyDesc"), h("div", {}, h("div", { class: "topology" },
    topologyNode("overview.core", cell?.epc, cell?.epc ? t("common.active") : t("common.inactive"), "settings"), topologyNode("overview.radio", cell?.enb, cell?.band ? `B${cell.band}` : "—", "cell"),
    topologyNode("overview.devices", current, current ? String(sessions.length) : labelState(ues?.state), "device"), topologyNode("overview.uplink", network?.active, network?.resolved_network || network?.ue_subnet || "—", "pulse")), h("p", { class: "caveat", text: t("overview.internetCaveat") })));
  const distribution = card(t("overview.distribution"), t("overview.distributionDesc"), current ? h("div", { class: "bar-list" }, bar(t("overview.registered"), registered, sessions.length, ""), bar(t("overview.restricted"), restricted, sessions.length, "warning"), bar(t("overview.other"), other, sessions.length, "muted")) : emptyPanel(t("overview.noSnapshot"), t("ues.notEmptyProof")));
  const quick = h("div", { class: "quick-links" }, [["cell","overview.quickCell"],["ues","overview.quickUE"],["diagnostics","overview.quickDiag"]].map(([r,k]) => h("a", { class: "quick-link", href: `#/${r}`, text: t(k) })));
  const errors = [state.cell, state.network, state.ues].filter((r) => r.error).map((r) => errorPanel(r.error, refreshLight));
  return h("div", { class: "stack" }, hero, metrics, h("div", { class: "grid two" }, topology, h("div", { class: "stack" }, distribution, card(t("overview.quick"), "", quick))), errors);
}
function metric(label, value, detail) { return h("section", { class: "card metric" }, h("p", { class: "metric-label", text: label }), h("strong", { class: "metric-value", text: value }), h("small", { text: detail })); }
function topologyNode(key, active, detail, icon) { return h("div", { class: "topology-node" }, h("div", { class: `topology-icon${active ? " success" : ""}` }, svg(icon)), h("strong", { text: t(key) }), h("small", { text: detail })); }
function bar(label, count, total, tone) { const pct = total ? Math.round((count / total) * 20) * 5 : 0; return h("div", { class: "bar-row" }, h("div", { class: "bar-row-top" }, h("span", { text: label }), h("strong", { text: String(count) })), h("div", { class: "bar-track" }, h("div", { class: `bar-fill ${tone} w-${pct}`.trim() }))); }

function renderCell() {
  const profileContent = resourceView(state.profile, (data) => data.has_profile ? h("div", {}, summary(Object.entries(data.profile || {}).filter(([,v]) => v !== "" && v !== null).slice(0,10).map(([k,v]) => [k.replaceAll("_"," "), valueText(v)])), button(t("cell.loadProfile"), "ghost small", loadProfileIntoDraft)) : emptyPanel(t("cell.profileEmpty"), t("cell.profileHint")), loadProfile);
  const running = Boolean(state.cell.data?.running), busy = Boolean(state.operation?.loading);
  const operation = state.operation ? (state.operation.error ? errorPanel(state.operation.error) : h("div", { class: "operation-box" }, h("strong", { text: t("cell.operation") }), h("p", { text: state.operation.loading ? t("common.loading") : t(state.operation.key) }))) : h("div", { class: "operation-box" }, h("strong", { text: t("cell.operation") }), h("p", { text: t("cell.noOperation") }));
  return h("div", { class: "stack" }, pageHeader("cell.eyebrow", "cell.title", "cell.subtitle", [button(t("common.refresh"), "secondary", refreshLight)]), h("div", { class: "grid equal" }, card(t("cell.runtime"), "", h("div", {}, h("div", { id: "cell-runtime-live" }, buildCellRuntime()), operation), h("div", { id: "cell-runtime-actions" }, buildCellActions())), card(t("cell.profile"), t("cell.profileHint"), profileContent)), renderCellForm(busy, running));
}
function buildCellRuntime() {
  return resourceView(state.cell, (data) => h("div", {}, summary([[t("common.status"), data.running ? t("overview.running") : t("overview.stopped")],[t("cell.startedAt"), formatTime(data.started_at)],[t("cell.band"), data.band],[t("cell.apn"), data.apn],[t("cell.network"), data.resolved_network || data.network]]), h("div", { class: "process-row" }, badge(`EPC · ${data.epc ? t("common.active") : t("common.inactive")}`, data.epc ? "success" : ""), badge(`eNodeB · ${data.enb ? t("common.active") : t("common.inactive")}`, data.enb ? "success" : ""), badge(`PCAP · ${data.pcap ? t("common.active") : t("common.inactive")}`, data.pcap ? "success" : ""))), refreshLight);
}
function buildCellActions() {
  const running = Boolean(state.cell.data?.running), hasProfile = Boolean(state.profile.data?.has_profile), busy = Boolean(state.operation?.loading);
  return h("div", { class: "page-actions" }, button(busy ? t("cell.starting") : t("cell.startSaved"), "primary", () => requestStart({}, "saved"), { disabled: busy || running || !hasProfile }), button(busy ? t("cell.stopping") : t("cell.stop"), "danger", openStopConfirm, { disabled: busy || !running }));
}
function updateCellRuntime() {
  const live = $("#cell-runtime-live"), actions = $("#cell-runtime-actions");
  if (live) live.replaceChildren(buildCellRuntime());
  if (actions) actions.replaceChildren(buildCellActions());
  const submit = $('#cell-form button[type="submit"]');
  if (submit) submit.disabled = Boolean(state.cell.data?.running || state.operation?.loading);
}
function field(name, label, options = {}) {
  const attrs = { id: `field-${name}`, name, value: state.draft[name] ?? "", required: options.required, type: options.type || "text", inputMode: options.inputMode, min: options.min, max: options.max, placeholder: options.placeholder, autocomplete: "off" };
  let control;
  if (options.choices) { control = h("select", { id: attrs.id, name, value: attrs.value }, options.choices.map(([value,key]) => h("option", { value, text: t(key) }))); control.value = String(attrs.value); }
  else control = h("input", attrs);
  const saveDraft = () => { state.draft[name] = control.value; };
  control.addEventListener(options.choices ? "change" : "input", saveDraft);
  return h("label", { class: options.wide ? "field wide" : "field" }, h("span", { text: t(label) }), control, h("small", { text: t(options.required ? "cell.requiredHint" : "cell.optionalHint") }));
}
function renderCellForm(busy, running) {
  const form = h("form", { id: "cell-form", on: { submit: submitCellForm } }, h("div", { class: "form-grid" },
    field("band","cell.band",{required:true,inputMode:"numeric"}), field("apn","cell.apn",{required:true}), field("mcc","cell.mcc",{required:true,inputMode:"numeric"}), field("mnc","cell.mnc",{required:true,inputMode:"numeric"}),
    field("network","cell.network",{placeholder:"auto"}), field("dns","cell.dns",{inputMode:"decimal"}), field("ue_subnet","cell.ueSubnet",{}), field("ue_access","cell.ueAccess",{choices:[["isolated","cell.isolated"],["allow","cell.allow"]]}),
    field("apn_mismatch_policy","cell.policy",{choices:[["strict","cell.strict"],["restricted","cell.restricted"]]}), field("sdr","cell.sdr",{choices:[["auto","cell.auto"],["uhd","UHD"],["bladerf","bladeRF"],["zmq","ZMQ"]]}),
    field("full_net_name","cell.fullName",{}), field("short_net_name","cell.shortName",{}), field("tx_gain","cell.txGain",{type:"number"}), field("rx_gain","cell.rxGain",{type:"number"}), field("n_prb","cell.nprb",{type:"number"})
  ), h("div", { class: "form-actions" }, h("button", { type: "submit", class: "button primary", disabled: busy || running, text: busy ? t("common.loading") : t("cell.startExplicit") })));
  return card(t("cell.explicit"), t("cell.explicitDesc"), form);
}
function loadProfileIntoDraft() { state.draft = { ...state.draft, ...(state.profile.data?.profile || {}) }; renderPage(false); requestAnimationFrame(() => $("#cell-form input")?.focus()); }
function submitCellForm(event) {
  event.preventDefault(); const data = new FormData(event.currentTarget); const draft = Object.fromEntries(data.entries()); state.draft = draft;
  const payload = {}; for (const [key,value] of Object.entries(draft)) if (String(value).trim() !== "") payload[key] = ["tx_gain","rx_gain","n_prb"].includes(key) ? Number(value) : String(value).trim(); requestStart(payload, "explicit");
}
function requestStart(payload, mode) {
  state.pendingStart = payload;
  state.pendingStartMode = mode;
  $("#start-description").textContent = t(mode === "saved" ? "cell.startSavedDescription" : "cell.startExplicitDescription");
  const dialog = $("#start-dialog"); dialog.returnValue = ""; dialog.showModal();
}
async function startCell(payload) {
  state.operation = { loading: true }; renderPage(false);
  try { await api.startCell(payload); state.operation = { loading: false, key: "cell.started" }; toast(t("cell.started")); await Promise.all([fetchInto("cell", () => api.cell()), loadProfile()]); }
  catch (error) { state.operation = { loading: false, error }; toast(errorText(error), true); }
  renderPage(false); resumePolling();
}
function openStopConfirm() { const dialog = $("#confirm-dialog"); dialog.returnValue = ""; dialog.showModal(); }
async function stopCell() {
  state.operation = { loading: true }; renderPage(false);
  try { const result = await api.stopCell(); state.operation = { loading: false, key: result.data?.stopped ? "cell.stopped" : "cell.alreadyStopped" }; toast(t(state.operation.key)); await fetchInto("cell", () => api.cell()); }
  catch (error) { state.operation = { loading: false, error }; toast(errorText(error), true); }
  renderPage(false); resumePolling();
}

function renderUEs() {
  const actions = [button(t("common.refresh"), "secondary", refreshLight)];
  const body = resourceView(state.ues, (data) => {
    if (data.state !== "current") return emptyPanel(labelState(data.state), data.reason || (data.state === "cell_stopped" ? t("ues.cellStopped") : t("ues.notEmptyProof")));
    const sessions = Array.isArray(data.sessions) ? data.sessions : [];
    return h("div", {}, h("div", { class: "snapshot-strip" }, snap(t("ues.snapshot"), labelState(data.state)), snap(t("ues.schema"), data.schema_version), snap(t("ues.sequence"), data.sequence), snap(t("ues.updatedAt"), formatTime(data.updated_at_unix_ms))), sessions.length ? renderUETable(sessions) : emptyPanel(t("ues.noSessions"), t("ues.notEmptyProof")), notice(t("common.readOnly"), t("ues.radioUnknown")));
  }, refreshLight);
  return h("div", { class: "stack" }, pageHeader("ues.eyebrow","ues.title","ues.subtitle",actions), card(t("ues.table"), t("ues.subtitle"), body), state.selectedUE ? ueDetail(state.selectedUE) : null);
}
function snap(label, value) { return h("div", { class: "snapshot-item" }, h("span", { text: label }), h("strong", { text: valueText(value) })); }
function renderUETable(sessions) {
  const rows = sessions.map((ue) => h("tr", {}, h("td", {}, h("span", { class: "table-primary mono", text: ue.imsi || t("ues.identityPending") }), h("span", { class: "table-secondary", text: ue.session_id })), h("td", {}, badge(labelState(ue.emm_state), toneFor(ue.emm_state)), h("span", { class: "table-secondary", text: labelState(ue.ecm_state) })), h("td", { class: "mono", text: valueText(ue.ue_ipv4) }), h("td", {}, h("span", { class: "table-primary", text: valueText(ue.requested_apn) }), h("span", { class: "table-secondary", text: valueText(ue.selected_apn) })), h("td", {}, badge(labelState(ue.access_policy), toneFor(ue.access_policy))), h("td", { class: "table-actions" }, button(t("common.details"), "secondary small", () => { state.selectedUE = ue; state.selectedUEStale = false; renderPage(false); }))));
  const table = h("div", { class: "table-wrap desktop-table" }, h("table", {}, h("thead", {}, h("tr", {}, ["ues.imsi","ues.emm","ues.ip","ues.apn","ues.policy"].map((k) => h("th", { text: t(k) })), h("th", {}))), h("tbody", {}, rows)));
  const cards = h("div", { class: "ue-cards" }, sessions.map((ue) => h("article", { class: "ue-card" }, h("div", { class: "ue-card-head" }, h("strong", { class: "mono", text: ue.imsi || t("ues.identityPending") }), badge(labelState(ue.emm_state), toneFor(ue.emm_state))), h("dl", {}, h("div", {}, h("dt", { text: t("ues.ip") }), h("dd", { text: valueText(ue.ue_ipv4) })), h("div", {}, h("dt", { text: t("ues.policy") }), h("dd", { text: labelState(ue.access_policy) }))), button(t("common.details"), "secondary small", () => { state.selectedUE = ue; state.selectedUEStale = false; renderPage(false); }))));
  return h("div", {}, table, cards);
}
function ueDetail(ue) {
  const close = () => { state.selectedUE = null; state.selectedUEStale = false; renderPage(false); };
  const bearers = Array.isArray(ue.bearers) ? ue.bearers.map((b) => `EBI ${b.ebi} · QCI ${b.qci} · ${labelState(b.state)}`).join("; ") : t("common.notReported");
  return h("div", {}, h("button", { type: "button", class: "detail-backdrop", "aria-label": t("common.close"), on: { click: close } }), h("aside", { class: "detail-panel", role: "dialog", "aria-modal": "true", "aria-labelledby": "ue-detail-title" }, h("div", { class: "detail-head" }, h("div", {}, h("p", { class: "eyebrow", text: t("common.details") }), h("h2", { id: "ue-detail-title", text: t("ues.detailTitle") })), button(t("common.close"), "secondary small", close)), h("div", { class: "detail-sections" }, state.selectedUEStale ? notice(t("common.stale"), t("ues.detailStale"), "warning") : null, summary([[t("ues.imsi"), ue.imsi || t("ues.identityPending")],[t("ues.session"), ue.session_id],[t("ues.emm"), labelState(ue.emm_state)],[t("ues.ecm"), labelState(ue.ecm_state)],[t("ues.ip"), ue.ue_ipv4],[t("ues.policy"), labelState(ue.access_policy)],[t("ues.accessReason"), valueText(ue.access_reason)],[t("ues.requested"), ue.requested_apn],[t("ues.selected"), ue.selected_apn],[t("ues.validated"), valueText(ue.apn_validated)],[t("ues.bearers"), bearers],[t("ues.s1ap"), `${valueText(ue.mme_ue_s1ap_id)} / ${valueText(ue.enb_ue_s1ap_id)}`],[t("ues.association"), ue.sctp_assoc_id]]), notice(t("common.readOnly"), t("ues.radioUnknown")))));
}
function reconcileSelectedUE() {
  if (!state.selectedUE) return;
  const data = state.ues.data;
  if (data?.state !== "current" || !Array.isArray(data.sessions)) { state.selectedUEStale = true; return; }
  const match = data.sessions.find((item) => item.session_id === state.selectedUE.session_id);
  if (match) { state.selectedUE = match; state.selectedUEStale = false; }
  else state.selectedUEStale = true;
}

function renderSubscribers() {
  const header = pageHeader("sub.eyebrow","sub.title","sub.subtitle",[button(t("common.refresh"),"secondary",() => loadSubscribers())]);
  const readOnly = notice(t("sub.readOnlyTitle"), t("sub.readOnlyDesc"), "info");
  const body = resourceView(state.subscribers, (data) => subscriberList(data), () => loadSubscribers());
  return h("div", { class: "stack" }, header, readOnly, card(t("sub.table"), t("sub.subtitle"), body), state.selectedSubscriber ? subscriberDetail() : null);
}
function subscriberList(data) {
  const items = Array.isArray(data.items) ? data.items : []; if (!items.length) return emptyPanel(t("sub.empty"));
  const tbody = h("tbody"), tracked = [];
  items.forEach((item) => { const row = h("tr", {}, h("td", {}, h("span", { class: "table-primary", text: item.name }), h("span", { class: "table-secondary mono", text: item.imsi })), h("td", { text: valueText(item.auth) }), h("td", { text: valueText(item.qci) }), h("td", { text: valueText(item.ip_alloc) }), h("td", {}, badge(item.active === true ? t("sub.activeTrue") : item.active === false ? t("sub.activeFalse") : t("sub.activeUnknown"), item.active === true ? "success" : "")), h("td", { class: "table-actions" }, button(t("common.details"), "secondary small", () => loadSubscriber(item.imsi)))); tbody.append(row); tracked.push([row, `${item.name} ${item.imsi}`.toLowerCase()]); });
  const empty = h("div", { class: "state-panel", hidden: true }, h("p", { text: t("sub.noMatch") }));
  const input = h("input", { class: "search-input", type: "search", placeholder: t("sub.search"), "aria-label": t("sub.search"), on: { input: (event) => { const q = event.target.value.trim().toLowerCase(); let shown=0; tracked.forEach(([row,text]) => { row.hidden = !text.includes(q); if (!row.hidden) shown++; }); empty.hidden = shown > 0; } } });
  const table = h("div", { class: "table-wrap" }, h("table", {}, h("thead", {}, h("tr", {}, ["sub.name","sub.auth","sub.qci","sub.ipAlloc","sub.active"].map((k) => h("th", { text: t(k) })), h("th", {}))), tbody));
  const start = Number(data.offset || 0) + 1, end = Number(data.offset || 0) + items.length, total = Number(data.total || items.length);
  return h("div", {}, h("div", { class: "toolbar" }, h("div", { class: "search-wrap" }, input), badge(t("common.count", { count: total }))), table, empty, h("div", { class: "pagination" }, h("p", { text: t("sub.range", { start, end, total }) }), h("div", { class: "pagination-actions" }, button(t("sub.prev"),"secondary small",()=>loadSubscribers(Math.max(0,state.subscriberOffset-50)),{disabled:state.subscriberOffset===0}), button(t("sub.next"),"secondary small",()=>loadSubscribers(state.subscriberOffset+50),{disabled:end>=total}))));
}
async function loadSubscriber(imsi) { state.selectedSubscriber = imsi; state.subscriber = resource(); state.subscriber.loading = true; renderPage(false); await fetchInto("subscriber", () => api.subscriber(imsi)); renderPage(false); }
function subscriberDetail() {
  const close = () => { state.selectedSubscriber = null; renderPage(false); };
  return h("div", {}, h("button", { type:"button", class:"detail-backdrop", "aria-label":t("common.close"), on:{click:close} }), h("aside", { class:"detail-panel", role:"dialog", "aria-modal":"true", "aria-labelledby":"subscriber-detail-title" }, h("div", { class:"detail-head" }, h("div", {}, h("p", { class:"eyebrow", text:t("common.readOnly") }), h("h2", { id:"subscriber-detail-title", text:t("sub.detail") })), button(t("common.close"),"secondary small",close)), resourceView(state.subscriber, (item) => summary([[t("sub.name"),item.name],[t("sub.imsi"),item.imsi],[t("sub.auth"),item.auth],[t("sub.authorized"),item.authorized],["OP type",item.op_type],[t("sub.qci"),item.qci],[t("sub.ipAlloc"),item.ip_alloc],[t("sub.active"),item.active === null ? t("sub.activeUnknown") : item.active ? t("sub.activeTrue") : t("sub.activeFalse")],[t("sub.credential"),item.credential_status]]), () => loadSubscriber(state.selectedSubscriber))));
}

function renderDiagnostics() {
  const run = button(state.diagnostics.loading ? t("diag.running") : t("diag.run"), "primary", runDiagnostics, { disabled: state.diagnostics.loading });
  let body;
  if (state.diagnostics.loading) body = loadingPanel();
  else if (state.diagnostics.error) body = errorPanel(state.diagnostics.error, runDiagnostics);
  else if (!state.diagnostics.data) body = emptyPanel(t("diag.notRun"), t("diag.neverAuto"));
  else body = diagnosticResults(state.diagnostics.data);
  return h("div", { class:"stack" }, pageHeader("diag.eyebrow","diag.title","diag.subtitle",[run]), notice(t("diag.neverAuto"),t("diag.safeNote"),"success"), card(state.diagnostics.data ? t("diag.completed") : t("diag.evidence"),t("diag.noProbe"),body));
}
async function runDiagnostics() { state.diagnostics.loading=true; state.diagnostics.error=null; renderPage(false); await fetchInto("diagnostics", () => api.diagnostics()); renderPage(false); }
function evidenceCard(title, object, fields) {
  const obj = object || {};
  return h("article", { class: "evidence-card" },
    h("div", { class: "evidence-state" }, h("h3", { text: title }), badge(labelState(obj.state), toneFor(obj.state))),
    h("dl", { class: "evidence-list" }, fields.map(([label, key]) =>
      h("div", {}, h("dt", { text: label }), h("dd", { text: valueText(obj[key]) })))));
}
function diagnosticResults(data) {
  const cards = [
    evidenceCard(t("diag.registration"),data.registration,[["S1AP IDs","s1ap_ids_observed"],["Attach requests","attach_requests"],["Attach complete","attach_complete"],["Rejects","rejects"]]),
    evidenceCard(t("diag.pdn"),data.pdn,[["Requests","requests"],["IP allocations","ip_allocations"],["Bearer activations","bearer_activations"],["Rejects","rejects"]]),
    evidenceCard(t("diag.dns"),data.dns,[[t("diag.queries"),"queries_observed"],[t("diag.responses"),"responses_observed"],[t("diag.configuredServer"),"configured_server"]]),
    evidenceCard(t("diag.userPlane"),data.user_plane,[[t("diag.ipPackets"),"ip_packets"],[t("diag.uplinkPackets"),"ue_uplink_packets"],[t("diag.downlinkPackets"),"ue_downlink_packets"],[t("diag.scanComplete"),"scan_complete"]]),
    evidenceCard(t("diag.hostNetwork"),data.network,[[t("cell.network"),"resolved_network"],[t("cell.ueSubnet"),"ue_subnet"],[t("cell.ueAccess"),"ue_access"]]),
    evidenceCard(t("diag.chap"),data.chap,[[t("common.reason"),"reason"],["S1AP observed","s1ap_observed"],["CHAP observed","chap_observed"],["PAP observed","pap_observed"],[t("diag.scanComplete"),"scan_complete"]]),
  ];
  const limits = Array.isArray(data.limitations) ? data.limitations : [];
  return h("div", { class:"stack" }, h("div", { class:"diagnostic-grid" }, cards), limits.length ? h("section", {}, h("h3", { text:t("diag.limitations") }), h("ul", { class:"limitations" }, limits.map((item) => h("li", { text:item })))) : null, notice(t("diag.noProbe"),t("overview.internetCaveat"),"warning"));
}

function renderSettings() {
  const appearance = card(t("settings.appearance"),t("settings.appearanceDesc"),h("div", {}, settingRow(t("common.language"), t("settings.appearanceDesc"), choiceGroup([["zh","中文"],["en","English"]],locale,setLocale)), settingRow(t("common.theme"),t("settings.appearanceDesc"),choiceGroup([["light",t("common.light")],["dark",t("common.dark")]],theme,setTheme))));
  const config = card(t("settings.interface"),t("settings.portNote"),resourceView(state.config,(data) => h("div", { class:"config-list" }, configItem(t("settings.version"),data.version),configItem(t("settings.apiExposure"),valueText(data.api_exposed)),configItem(t("settings.apiPort"),data.api_port),configItem(t("settings.uiPort"),data.ui_port),configItem(t("settings.authRequired"),valueText(data.auth_required)),configItem(t("settings.endpoints"),"/api/v1")), () => boot()));
  const protectedMode = Boolean(state.config.data?.auth_required);
  const auth = card(t("settings.access"),t("auth.memoryNote"),h("div", {}, settingRow(protectedMode?t("auth.protectedMode"):t("auth.openMode"),t("auth.memoryNote"),api.hasToken()?badge(t("auth.memory"),"success"):badge(protectedMode?t("auth.required"):t("auth.openMode"))), api.hasToken()?button(t("auth.forget"),"danger small",()=>{api.clearToken();state.authBlocked=protectedMode;stopPolling("auth");if(protectedMode)openAuth();renderPage(false);}):null));
  const health = card(t("settings.health"),t("settings.healthDesc"),healthView(),button(t("settings.checkHealth"),"secondary",runHealth,{disabled:state.health.loading}));
  const help = card(t("settings.help"),"",h("div",{},detail(t("settings.help1Title"),t("settings.help1Body")),detail(t("settings.help2Title"),t("settings.help2Body")),detail(t("settings.help3Title"),t("settings.help3Body"))));
  return h("div", { class:"stack" },pageHeader("settings.eyebrow","settings.title","settings.subtitle"),h("div",{class:"grid equal"},h("div",{class:"stack"},appearance,auth),h("div",{class:"stack"},config,health)),help);
}
function settingRow(title,copy,control){return h("div",{class:"setting-row"},h("div",{class:"setting-copy"},h("strong",{text:title}),h("p",{text:copy})),control);}
function choiceGroup(items,current,onChoose){return h("div",{class:"choice-group"},items.map(([value,label])=>h("button",{type:"button",class:`choice${value===current?" active":""}`,"aria-pressed":String(value===current),text:label,on:{click:()=>{onChoose(value);renderPage(false);}}})));}
function configItem(label,value){return h("div",{class:"config-item"},h("span",{text:label}),h("strong",{text:valueText(value)}));}
function detail(title,body){return h("details",{},h("summary",{text:title}),h("p",{text:body}));}
function healthView(){if(state.health.loading)return loadingPanel();if(state.health.error)return errorPanel(state.health.error,runHealth);if(!state.health.data)return emptyPanel(t("settings.healthNotRun"),t("settings.healthDesc"));const d=state.health.data,s=d.sdr||{};return summary([[t("settings.serviceOK"),d.ok],[t("overview.cellState"),d.running?t("overview.running"):t("overview.stopped")],["UHD B210",s.uhd_b210],["bladeRF",s.bladerf],["ACR1281",s.acr1281],["UTC",d.time]]);}
async function runHealth(){state.health.loading=true;state.health.error=null;renderPage(false);await fetchInto("health",()=>api.health());renderPage(false);}

boot();
