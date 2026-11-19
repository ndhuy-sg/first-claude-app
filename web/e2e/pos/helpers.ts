import { expect, type Page } from "@playwright/test";

/**
 * POS e2e helpers. They run against the real stack with the seed data (docs/DEV.md):
 * CN1 device token, tn1 = "Lan" PIN 1234, ql1 = "Tuấn" PIN 9999, tables "Bàn 1".."Bàn 12" in "Tầng 1".
 *
 * All POS specs share the CN1 device (one open shift per device), so they must not run in parallel
 * with each other: each file is serial, and the e2e config should run e2e/pos with one worker.
 */
export const DEVICE_CN1 = "dev-device-cn1-token-0000000000000000000000";

/** iPad (1180×820) and Android (1280×800) landscape, as required by section W. */
export const POS_VIEWPORTS = [
  { width: 1180, height: 820 },
  { width: 1280, height: 800 },
] as const;

export const staff = {
  tn1: { name: "Lan", pin: "1234" },
  ql1: { name: "Tuấn", pin: "9999", email: "ql1@moc.vn", password: "matkhau123" },
} as const;

export function formatVND(n: number): string {
  return (n < 0 ? "−" : "") + String(Math.abs(Math.trunc(n))).replace(/\B(?=(\d{3})+(?!\d))/g, ".") + "đ";
}

export function parseVND(s: string): number {
  const neg = /[−-]/.test(s);
  const n = Number(s.replace(/[^\d]/g, ""));
  return neg ? -n : n;
}

/** Puts the activated CN1 tablet cookie in the browser context. */
export async function setDeviceCN1(page: Page, baseURL: string | undefined) {
  await page.context().addCookies([{ name: "moc_device", value: DEVICE_CN1, url: baseURL ?? "http://localhost:3000" }]);
}

export async function pressKeys(page: Page, group: "Bàn phím nhập PIN" | "Bàn phím nhập tiền", keys: string[], scope = page.locator("body")) {
  const pad = scope.getByRole("group", { name: group });
  for (const k of keys) await pad.getByRole("button", { name: k, exact: true }).click();
}

/** PIN login on screen 1. Ends on the open-shift screen or the floor plan. */
export async function loginPin(page: Page, who: keyof typeof staff = "tn1") {
  const s = staff[who];
  await page.goto("/pos/dang-nhap");
  await page.getByRole("button", { name: new RegExp(s.name) }).first().click();
  await expect(page.getByRole("heading", { name: `Xin chào, ${s.name}` })).toBeVisible();
  await pressKeys(page, "Bàn phím nhập PIN", s.pin.split(""));
  await page.getByRole("button", { name: "Vào ca" }).click();
  await page.waitForURL(/\/pos\/(ban|mo-ca|het-mon)$/);
}

/** Opens a shift (500.000đ) if none is open, and lands on the floor plan. */
export async function ensureShift(page: Page) {
  if (page.url().endsWith("/pos/mo-ca")) {
    await pressKeys(page, "Bàn phím nhập tiền", ["5", "000", "000"]);
    await page.getByRole("button", { name: "Mở ca và bắt đầu bán" }).click();
  }
  await page.waitForURL(/\/pos\/ban$/);
}

export async function loginWithShift(page: Page, baseURL: string | undefined, who: keyof typeof staff = "tn1") {
  await setDeviceCN1(page, baseURL);
  await loginPin(page, who);
  await ensureShift(page);
}

/** Taps the first free table of the current area and returns its name and the new order id. */
export async function openFreeTable(page: Page): Promise<{ table: string; orderId: number }> {
  const card = page.getByRole("button", { name: /^Bàn \d+, Trống$/ }).first();
  const label = (await card.getAttribute("aria-label")) ?? "";
  await card.click();
  await page.waitForURL(/\/pos\/don\/\d+$/);
  const orderId = Number(page.url().split("/").pop());
  const table = label.split(",")[0];
  await expect(page.getByRole("heading", { name: table })).toBeVisible();
  return { table, orderId };
}

/** Adds "Croissant bơ" (35.000đ, no options: added straight away). */
export async function addCroissant(page: Page) {
  await page.getByLabel("Tìm món").fill("Croissant");
  await page.getByRole("button", { name: /^Croissant bơ,/ }).click();
  await expect(page.getByRole("list", { name: "Món trong đơn" })).toContainText("Croissant bơ");
  await page.getByLabel("Tìm món").fill("");
}

/** Writes need an allowed Origin header (A-15); page.request does not send one by itself. */
async function apiPost(page: Page, path: string, data: unknown) {
  const origin = new URL(page.url()).origin;
  return page.request.post(`/api/v1${path}`, { data, headers: { Origin: origin } });
}

/** Cancels an unpaid order (test cleanup, frees the table). Ignores orders that are already closed. */
export async function cancelOrder(page: Page, orderId: number) {
  await apiPost(page, `/orders/${orderId}/cancel`, { reason: "Dọn dữ liệu test e2e" });
}

/** Cancels every open order of the current shift so the shift can be closed (W-10). */
export async function cancelOpenOrdersOfShift(page: Page) {
  const shift = (await (await page.request.get("/api/v1/shifts/current")).json()) as { id: number } | null;
  if (!shift) return;
  const list = (await (await page.request.get(`/api/v1/orders?status=open&shiftId=${shift.id}&pageSize=200`)).json()) as {
    items: { id: number; payments: { id: number; status: string }[] }[];
  };
  for (const o of list.items) {
    for (const p of o.payments.filter((x) => x.status === "pending")) await apiPost(page, `/payments/${p.id}/cancel`, {});
    await cancelOrder(page, o.id);
  }
}

/** Every visible button inside `root` is at least 44×44 px (W-03). */
export async function expectTouchTargets(page: Page, root = page.locator("body")) {
  const buttons = root.locator("button:visible, a:visible");
  const n = await buttons.count();
  const small: string[] = [];
  for (let i = 0; i < n; i++) {
    const b = buttons.nth(i);
    const box = await b.boundingBox();
    if (box && (box.height < 44 || box.width < 44)) small.push(`${(await b.innerText()).trim() || (await b.getAttribute("aria-label"))} ${box.width}×${box.height}`);
  }
  expect(small, "buttons smaller than 44px").toEqual([]);
}
