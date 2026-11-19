/**
 * Playwright fixtures and UI helpers shared by e2e specs.
 *
 *   import { test, expect } from "../support/fixtures";
 *   test("…", async ({ pos, ownerApi }) => { … });
 *
 * - activateDevice(context, "CN1")  sets the seeded moc_device cookie (skips the activation screen)
 * - pinLoginUi(page, "Lan", "1234") logs in on the PIN screen by tapping, like a cashier
 * - pinLoginApi(context, …)         same session without the UI (setup)
 * - ownerLoginUi(page) / ownerApi   admin login through the UI / an isolated owner API session
 *
 * Locators use roles and the Vietnamese labels of the design (docs/SCREENS.md + design canvas).
 */
import { test as base, expect, type BrowserContext, type Page } from "@playwright/test";
import { Api, deviceCookieFor } from "./api";
import { ROUTES } from "./routes";
import { ACCOUNTS, DEVICE_TOKENS } from "./seed";

export { expect };

type Branch = keyof typeof DEVICE_TOKENS;

export async function activateDevice(context: BrowserContext, branch: Branch = "CN1", baseURL = "http://localhost:3000") {
  await context.addCookies([deviceCookieFor(baseURL, DEVICE_TOKENS[branch])]);
}

/** API client sharing the browser context's cookies (device + session). */
export function contextApi(context: BrowserContext): Api {
  return new Api(context.request);
}

export async function pinLoginApi(context: BrowserContext, staffName: string, pin: string) {
  return contextApi(context).pinLogin(staffName, pin);
}

/** On the PIN screen: pick the staff member, tap the digits, then "Vào ca" (if it does not auto-submit). */
export async function pinLoginUi(page: Page, staffName: string, pin: string) {
  await page.goto(ROUTES.posPin);
  await page.getByRole("button", { name: new RegExp(`^\\S?\\s*${escapeRe(staffName)}\\b`) }).first().click();
  await tapDigits(page, pin);
  const enter = page.getByRole("button", { name: "Vào ca" }).or(page.getByRole("link", { name: "Vào ca" }));
  if (await enter.first().isVisible().catch(() => false)) await enter.first().click();
  await expect(page.getByText("Nhập mã PIN", { exact: false })).toBeHidden({ timeout: 15_000 });
}

/** Taps digits on an on-screen keypad (buttons labelled "0".."9", "000"). */
export async function tapDigits(page: Page, digits: string, scope = page.locator("body")) {
  for (const d of digits) await scope.getByRole("button", { name: d, exact: true }).first().click();
}

export async function ownerLoginUi(page: Page) {
  await page.goto(ROUTES.adminLogin);
  await page.getByLabel("Email").fill(ACCOUNTS.owner.email);
  await page.getByLabel("Mật khẩu").fill(ACCOUNTS.owner.password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByRole("navigation", { name: "Quản trị" })).toBeVisible();
}

/** A table tile on the floor plan, e.g. "Bàn 5" (not "Bàn 50"). Tiles may be links or buttons. */
export function tableTile(page: Page, name: string) {
  const re = new RegExp(`^${escapeRe(name)}(\\s|$)`);
  return page.getByRole("link", { name: re }).or(page.getByRole("button", { name: re })).first();
}

/** A product tile in the menu grid (the design labels each tile with the product name). */
export function productTile(page: Page, name: string) {
  const re = new RegExp(`^${escapeRe(name)}(\\s|$)`);
  return page.getByRole("button", { name: re }).first();
}

/** Adds a product through the item dialog: optional size and modifiers, then "Thêm vào đơn · …". */
export async function addProduct(page: Page, name: string, opts: { size?: string; modifiers?: string[] } = {}) {
  await productTile(page, name).click();
  const dialog = page.getByRole("dialog");
  const addBtn = dialog.getByRole("button", { name: /^Thêm vào đơn/ });
  // Products without sizes/options may be added straight away without a dialog.
  if (!(await addBtn.isVisible({ timeout: 2_000 }).catch(() => false))) return;
  if (opts.size) await dialog.getByRole("button", { name: new RegExp(`^(Size )?${escapeRe(opts.size)}\\b`) }).first().click();
  for (const m of opts.modifiers ?? []) await dialog.getByRole("button", { name: new RegExp(`^${escapeRe(m)}`) }).first().click();
  await addBtn.click();
  await expect(dialog).toBeHidden();
}

export function escapeRe(s: string) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

type Fixtures = {
  /** Owner API session (isolated cookie jar). */
  ownerApi: Api;
  /** A page on the CN1 iPad, PIN-logged in as tn1 (Lan) via the API, without opening a shift. */
  pos: Page;
};

export const test = base.extend<Fixtures>({
  ownerApi: async ({ baseURL }, provide) => {
    const api = await Api.create(baseURL!);
    await api.loginOwner();
    await provide(api);
    await api.dispose();
  },
  pos: async ({ page, context, baseURL }, provide) => {
    await activateDevice(context, "CN1", baseURL!);
    await pinLoginApi(context, ACCOUNTS.tn1.name, ACCOUNTS.tn1.pin);
    await provide(page);
  },
});
