/**
 * F-04: khoá tn1 giữa ca → máy POS của tn1 bị đưa về màn hình nhập PIN ngay.
 *
 * tn1 works on the CN1 iPad; the owner locks tn1 (POST /users/{id}/lock). The session is revoked
 * (A-06) and the POS learns it from the SSE event "user.locked" (or its next request getting 401)
 * and shows the PIN screen. tn1 is unlocked afterwards.
 */
import { contextApi, expect, test } from "../support/fixtures";
import { ROUTES } from "../support/routes";
import { ACCOUNTS } from "../support/seed";

test.describe.configure({ mode: "serial" });

test("F-04 khoá tn1 giữa ca, POS về màn hình nhập PIN ngay", async ({ pos, context, ownerApi }) => {
  const api = contextApi(context);
  await api.ensureOpenShift();
  const cn1 = await ownerApi.branchId("CN1");
  const tn1 = await ownerApi.userId(ACCOUNTS.tn1.name, cn1);

  await pos.goto(ROUTES.posTables);
  await expect(pos.getByRole("link", { name: "Chốt ca" }).or(pos.getByRole("button", { name: "Chốt ca" })).first()).toBeVisible();

  try {
    await ownerApi.post(`/users/${tn1}/lock`);

    // "ngay" = without any action on the tablet; allow a few seconds for SSE delivery.
    await expect(pos.getByText("Nhập mã PIN", { exact: false })).toBeVisible({ timeout: 10_000 });
    await expect(pos.getByRole("heading", { name: "Chọn nhân viên", exact: true })).toBeVisible();
    const me = await api.raw("GET", "/auth/me");
    expect(me.status()).toBe(401);
  } finally {
    await ownerApi.post(`/users/${tn1}/unlock`).catch(() => undefined);
    // Clean up the shift with a manager on the same device.
    await api.pinLogin(ACCOUNTS.ql1.name, ACCOUNTS.ql1.pin).catch(() => undefined);
    await api.closeShiftIfOpen().catch(() => undefined);
  }
});
