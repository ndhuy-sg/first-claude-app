/**
 * N-03: SQL-injection-style input in search boxes and form fields has no effect
 * (every query is parameterised via sqlc). Runs in the "api" project (no browser) against the
 * integration stack started by playwright.config.ts.
 *
 * Checked inputs: orders search `q` (GET /orders), login email/password (POST /auth/login),
 * the public invoice lookup code (GET /public/invoices/{code}) and the menu search-free endpoints
 * stay intact afterwards.
 */
import { expect, test } from "@playwright/test";
import { Api, type S } from "../support/api";
import { ACCOUNTS } from "../support/seed";

const PAYLOADS = [
  `' OR '1'='1`,
  `' OR 1=1 --`,
  `'; DROP TABLE orders; --`,
  `1; DELETE FROM users; --`,
  `%' UNION SELECT id, email, password_hash FROM users --`,
  `' AND pg_sleep(3) IS NULL --`,
  `'); SELECT pg_sleep(3); --`,
  `\\'; --`,
  `" OR ""="`,
  `%`,
  `_`,
  `Bàn 5'--`,
];

test.describe.configure({ mode: "serial" });

let ql1: Api;

test.beforeAll(async ({ baseURL }) => {
  ql1 = await Api.create(baseURL!);
  await ql1.login(ACCOUNTS.ql1.email!, ACCOUNTS.ql1.password!);
});

test.afterAll(async () => {
  await ql1?.dispose();
});

test("N-03 tìm đơn với chuỗi tấn công: 200, không lộ dữ liệu, không chậm", async () => {
  for (const q of PAYLOADS) {
    const started = Date.now();
    const res = await ql1.raw("GET", "/orders", { query: { q, pageSize: 200 } });
    const ms = Date.now() - started;
    expect(res.status(), `q=${q}`).toBe(200);
    const body = (await res.json()) as S["OrderList"];
    expect(Array.isArray(body.items), `q=${q}`).toBe(true);
    // "%" and "_" are plain characters for a parameterised ILIKE only if escaped; either way they
    // may only match real order codes/table names, never rows of other tables or other branches.
    for (const o of body.items) expect(Object.keys(o)).not.toContain("passwordHash");
    if (!["%", "_"].includes(q)) expect(body.total, `q=${q} must match nothing`).toBe(0);
    expect(ms, `q=${q} must not trigger pg_sleep`).toBeLessThan(2_500);
  }
});

test("N-03 đăng nhập với chuỗi tấn công: 401 chung chung, không phải 200/500", async ({ baseURL }) => {
  const anon = await Api.create(baseURL!);
  try {
    for (const p of PAYLOADS) {
      for (const body of [
        { email: p, password: "x" },
        { email: `${p}@moc.vn`, password: p },
        { email: ACCOUNTS.owner.email + p, password: p },
      ]) {
        const res = await anon.raw("POST", "/auth/login", { body });
        expect([400, 401], `login ${JSON.stringify(body)} → ${res.status()}`).toContain(res.status());
        const text = await res.text();
        expect(text).not.toMatch(/syntax error|SQLSTATE|pq:|pgx|password_hash/i);
      }
    }
  } finally {
    await anon.dispose();
  }
});

test("N-03 mã tra cứu hoá đơn với chuỗi tấn công: 404", async ({ baseURL }) => {
  const anon = await Api.create(baseURL!);
  try {
    for (const p of PAYLOADS) {
      const res = await anon.raw("GET", `/public/invoices/${encodeURIComponent(p)}`);
      expect([400, 404], `code=${p} → ${res.status()}`).toContain(res.status());
    }
  } finally {
    await anon.dispose();
  }
});

test("N-03 dữ liệu vẫn nguyên vẹn sau các lần thử", async ({ baseURL }) => {
  const owner = await Api.create(baseURL!);
  try {
    await owner.loginOwner(); // users table intact, owner password unchanged
    const orders = await owner.raw("GET", "/orders", { query: { pageSize: 1 } });
    expect(orders.status()).toBe(200); // orders table still exists
    const users = await owner.get<S["User"][]>("/users");
    expect(users.length).toBeGreaterThanOrEqual(7);
  } finally {
    await owner.dispose();
  }
});
