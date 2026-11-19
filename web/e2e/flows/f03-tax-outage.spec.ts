/**
 * F-03: kết nối tới cơ quan thuế lỗi trong lúc bán → vẫn bán bình thường, khách vẫn tra cứu được
 * hoá đơn. Khi kết nối lại, dữ liệu hoá đơn tự được gửi đi.
 *
 * The spec runs its own fake tax authority (HTTP on E2E_TAX_PORT, default 18090) that first answers
 * 503 and later 200. It needs the API and worker to send invoices there:
 *   - start-stack.sh exports TAX_GATEWAY_URL=http://localhost:18090 to `mocpos serve` and `mocpos worker`;
 *   - settings: taxSubmissionEnabled=true, taxGateway="direct".
 * Until the invoices/worker modules read TAX_GATEWAY_URL this test is marked fixme; set
 * E2E_TAX_GATEWAY_READY=1 (CI env) once they do. LEAD: adjust the env name if the module chose another.
 */
import http from "node:http";
import { Api, eventually, type S } from "../support/api";
import { activateDevice, contextApi, expect, pinLoginApi, test } from "../support/fixtures";
import { ROUTES } from "../support/routes";
import { ACCOUNTS, MENU, vnd } from "../support/seed";

const PORT = Number(process.env.E2E_TAX_PORT ?? 18090);

test.describe.configure({ mode: "serial" });

test("F-03 cơ quan thuế lỗi: vẫn bán và tra cứu được, kết nối lại thì tự gửi", async ({ page, context, baseURL, ownerApi }) => {
  test.fixme(!process.env.E2E_TAX_GATEWAY_READY, "needs TAX_GATEWAY_URL support in the API/worker (set E2E_TAX_GATEWAY_READY=1)");
  test.setTimeout(5 * 60_000); // first automatic retry is ~1 minute after the failure (ARCHITECTURE §5)

  // ---- fake tax authority -----------------------------------------------------------------
  let healthy = false;
  const received: string[] = [];
  const server = http.createServer((req, res) => {
    let body = "";
    req.on("data", (c) => (body += c));
    req.on("end", () => {
      if (!healthy) {
        res.writeHead(503).end("maintenance");
        return;
      }
      received.push(body);
      res.writeHead(200, { "Content-Type": "application/json" }).end(JSON.stringify({ status: "accepted" }));
    });
  });
  await new Promise<void>((resolve) => server.listen(PORT, resolve));

  const settings = await ownerApi.get<S["Settings"]>("/settings");
  try {
    await ownerApi.put("/settings", { ...settings, taxSubmissionEnabled: true, taxGateway: "direct" });

    await activateDevice(context, "CN1", baseURL!);
    await pinLoginApi(context, ACCOUNTS.tn1.name, ACCOUNTS.tn1.pin);
    const pos = contextApi(context);
    await pos.ensureOpenShift();

    // ---- sell while the tax authority is down ----------------------------------------------
    let order = await pos.createOrder({ type: "takeaway" });
    const croissant = await pos.menuProduct(MENU.croissant.name);
    order = await pos.addItem(order.id, { productId: croissant.id, qty: 1 });
    const paid = await pos.payCash(order);
    expect(paid.order.status).toBe("paid");
    const inv = paid.order.invoice!;
    expect(inv, "invoice is issued even though the tax authority is down").toBeTruthy();

    // Customer lookup works right away (no login).
    const anon = await Api.create(baseURL!);
    await anon.get(`/public/invoices/${encodeURIComponent(inv.lookupCode)}`);
    await anon.dispose();
    await page.goto(ROUTES.lookup(inv.lookupCode));
    await expect(page.getByText(vnd(MENU.croissant.basePrice)).first()).toBeVisible();

    // The worker tried and failed: still not "sent".
    await eventually(async () => (await ownerApi.get<S["Invoice"]>(`/invoices/${inv.id}`)).status !== "sent", "invoice must not be sent while the gateway is down");

    // ---- connection restored: the worker resends by itself (no "Gửi lại" click) ---------------
    healthy = true;
    await eventually(async () => (await ownerApi.get<S["Invoice"]>(`/invoices/${inv.id}`)).status === "sent", "invoice is sent automatically after the gateway recovers", 4 * 60_000);
    expect(received.length).toBeGreaterThanOrEqual(1);

    await pos.closeShiftIfOpen();
  } finally {
    await ownerApi.put("/settings", settings).catch(() => undefined);
    await new Promise<void>((resolve) => server.close(() => resolve()));
  }
});
