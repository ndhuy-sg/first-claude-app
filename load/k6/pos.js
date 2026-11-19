// N-01: 50 virtual POS tablets create orders and pay in cash, continuously.
// Pass: 95% of requests < 200 ms, 0 failed requests, 0 failed checks.
//
//   k6 run load/k6/pos.js
//   k6 run -e BASE_URL=http://localhost:8080 -e DURATION=5m -e VUS=50 load/k6/pos.js
//
// Setup activates one POS device per virtual user (managers ql1/ql2 alternate between CN1 and CN2),
// PIN-logs in the branch cashier (tn1/tn2) on it and opens a shift. Each VU then replays the cookies
// of its own device + session ("cookie seeding") and loops: new takeaway order → add Cà phê sữa
// size L + Thêm shot → add Croissant bơ → pay cash. Teardown closes every shift it opened.
// Only the sale loop (tag flow=sale) counts for the thresholds; setup does bcrypt-heavy logins.
// See load/README.md.
import http from "k6/http";
import { check, fail, sleep } from "k6";
import { Trend } from "k6/metrics";

const BASE = (__ENV.BASE_URL || "http://localhost:8080").replace(/\/$/, "");
const API = `${BASE}/api/v1`;
const ORIGIN = __ENV.ORIGIN || "http://localhost:3000"; // must be in the API's ALLOWED_ORIGINS
const VUS = Number(__ENV.VUS || 50);
const DURATION = __ENV.DURATION || "3m";
const THINK_MIN = Number(__ENV.THINK_MIN || 0.5); // seconds between actions of one cashier
const THINK_MAX = Number(__ENV.THINK_MAX || 1.5);
const PASSWORD = __ENV.PASSWORD || "matkhau123";

const MANAGERS = [
  { email: "ql1@moc.vn", cashier: "Lan", pin: "1234" }, // CN1, tn1
  { email: "ql2@moc.vn", cashier: "Nam", pin: "5678" }, // CN2, tn2
];

const saleDuration = new Trend("sale_full_flow_ms", true);

export const options = {
  setupTimeout: "5m",
  teardownTimeout: "5m",
  scenarios: {
    pos: { executor: "constant-vus", vus: VUS, duration: DURATION, gracefulStop: "30s" },
  },
  thresholds: {
    "http_req_duration{flow:sale}": ["p(95)<200"],
    "http_req_failed{flow:sale}": ["rate==0"],
    checks: ["rate==1"],
  },
};

const JSON_HEADERS = { "Content-Type": "application/json", Origin: ORIGIN };

function post(url, body, params = {}) {
  return http.post(url, JSON.stringify(body || {}), { ...params, headers: { ...JSON_HEADERS, ...(params.headers || {}) } });
}

function uuid() {
  // RFC 4122 v4 from Math.random is fine for idempotency keys in a load test.
  return "xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx".replace(/[xy]/g, (c) => {
    const r = (Math.random() * 16) | 0;
    return (c === "x" ? r : (r & 0x3) | 0x8).toString(16);
  });
}

function cookieValue(jar, name) {
  const c = jar.cookiesForURL(BASE)[name];
  return c && c.length ? c[0] : "";
}

function must(res, status, what) {
  if (res.status !== status) fail(`${what}: expected ${status}, got ${res.status} ${res.body}`);
  return res.json();
}

export function setup() {
  const jar = http.cookieJar();
  const tag = { tags: { flow: "setup" } };

  must(http.get(`${API}/healthz`, tag), 200, "healthz");

  // Product ids from the menu (any logged-in user may read it).
  must(post(`${API}/auth/login`, { email: MANAGERS[0].email, password: PASSWORD }, tag), 200, "manager login");
  const menu = must(http.get(`${API}/menu`, { headers: { Origin: ORIGIN }, ...tag }), 200, "menu");
  const products = {};
  for (const c of menu.categories) for (const p of c.products) products[p.name] = p;
  const caPheSua = products["Cà phê sữa"];
  const croissant = products["Croissant bơ"];
  if (!caPheSua || !croissant) fail("seed menu not found (Cà phê sữa / Croissant bơ)");
  const shot = caPheSua.modifiers.find((m) => m.name === "Thêm shot");

  const devices = [];
  for (let i = 0; i < VUS; i++) {
    const m = MANAGERS[i % MANAGERS.length];
    jar.clear(BASE);
    const me = must(post(`${API}/auth/login`, { email: m.email, password: PASSWORD }, tag), 200, `login ${m.email}`);
    must(post(`${API}/devices/activate`, { email: m.email, password: PASSWORD, branchId: me.user.branchId, name: `k6 máy ${i + 1}` }, tag), 201, "activate device");
    const info = must(http.get(`${API}/device`, { headers: { Origin: ORIGIN }, ...tag }), 200, "device info");
    const cashier = info.staff.find((s) => s.name === m.cashier);
    if (!cashier) fail(`cashier ${m.cashier} not on device`);
    must(post(`${API}/auth/pin`, { userId: cashier.id, pin: m.pin }, tag), 200, "pin login");
    const current = must(http.get(`${API}/shifts/current`, { headers: { Origin: ORIGIN }, ...tag }), 200, "current shift");
    if (!current) must(post(`${API}/shifts/open`, { openingCash: 1000000 }, tag), 201, "open shift");
    devices.push({ device: cookieValue(jar, "moc_device"), session: cookieValue(jar, "moc_session") });
  }
  jar.clear(BASE);
  return { devices, caPheSuaId: caPheSua.id, shotId: shot ? shot.id : null, croissantId: croissant.id };
}

function useDevice(d) {
  const jar = http.cookieJar();
  jar.set(BASE, "moc_device", d.device, { path: "/" });
  jar.set(BASE, "moc_session", d.session, { path: "/" });
}

function think() {
  sleep(THINK_MIN + Math.random() * (THINK_MAX - THINK_MIN));
}

export default function (data) {
  useDevice(data.devices[(__VU - 1) % data.devices.length]);
  const p = { tags: { flow: "sale" } };
  const started = Date.now();

  let r = post(`${API}/orders`, { type: "takeaway" }, p);
  if (!check(r, { "order created": (x) => x.status === 201 })) return;
  const orderId = r.json("id");
  think();

  r = post(`${API}/orders/${orderId}/items`, { productId: data.caPheSuaId, sizeCode: "L", modifierIds: data.shotId ? [data.shotId] : [], qty: 1 }, p);
  check(r, { "item 1 added (43.000đ)": (x) => x.status === 201 && x.json("total") === 43000 });
  think();

  r = post(`${API}/orders/${orderId}/items`, { productId: data.croissantId, qty: 1 }, p);
  check(r, { "item 2 added": (x) => x.status === 201 });
  const total = r.json("total");
  think();

  r = post(`${API}/orders/${orderId}/payments`, { method: "cash", amount: total, cashReceived: 100000 }, { ...p, headers: { "Idempotency-Key": uuid() } });
  check(r, {
    "paid": (x) => x.status === 201 && x.json("order.status") === "paid",
    "invoice issued": (x) => x.status === 201 && !!x.json("order.invoice"),
  });
  saleDuration.add(Date.now() - started);
  think();
}

export function teardown(data) {
  const tag = { tags: { flow: "teardown" } };
  for (const d of data.devices) {
    useDevice(d);
    const shift = http.get(`${API}/shifts/current`, { headers: { Origin: ORIGIN }, ...tag }).json();
    if (!shift || !shift.id) continue;
    const sum = http.get(`${API}/shifts/${shift.id}/summary`, { headers: { Origin: ORIGIN }, ...tag }).json();
    post(`${API}/shifts/${shift.id}/close`, { countedCash: sum.expectedCash, note: "k6" }, tag);
  }
}
