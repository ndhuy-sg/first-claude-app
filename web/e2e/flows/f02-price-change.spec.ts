/**
 * F-02: owner đổi giá món → POS ở mọi chi nhánh hiện giá mới. Đơn cũ không đổi.
 *
 * Bạc xỉu 32.000đ → 34.000đ in the admin "Món & giá" screen; the POS of CN1 and CN2 show the new
 * price; an order that already holds Bạc xỉu keeps 32.000đ. The price is restored afterwards.
 */
import type { BrowserContext } from "@playwright/test";
import { Api, type S } from "../support/api";
import { activateDevice, contextApi, escapeRe, expect, ownerLoginUi, pinLoginApi, test } from "../support/fixtures";
import { ROUTES } from "../support/routes";
import { ACCOUNTS, MENU, vnd } from "../support/seed";

const NEW_PRICE = 34_000;

test.describe.configure({ mode: "serial" });

test("F-02 owner đổi giá món, POS mọi chi nhánh thấy giá mới, đơn cũ giữ giá cũ", async ({ browser, baseURL, ownerApi }) => {
  const original = await ownerApi.product(MENU.bacXiu.name);
  expect(original.basePrice).toBe(MENU.bacXiu.basePrice);

  // Two tablets: CN1 (tn1) and CN2 (tn2), each with an open shift.
  const tablets: { ctx: BrowserContext; api: Api; branch: "CN1" | "CN2" }[] = [];
  for (const [branch, who] of [["CN1", ACCOUNTS.tn1], ["CN2", ACCOUNTS.tn2]] as const) {
    const ctx = await browser.newContext({ baseURL, viewport: { width: 1180, height: 820 }, hasTouch: true });
    await activateDevice(ctx, branch, baseURL!);
    await pinLoginApi(ctx, who.name, who.pin);
    const api = contextApi(ctx);
    await api.ensureOpenShift();
    tablets.push({ ctx, api, branch });
  }
  const cn1 = tablets[0];

  try {
    // An order that already holds Bạc xỉu at the old price (takeaway: no table needed).
    let oldOrder = await cn1.api.createOrder({ type: "takeaway" });
    oldOrder = await cn1.api.addItem(oldOrder.id, { productId: original.id, sizeCode: "M", qty: 1 });
    expect(oldOrder.items[0].unitPrice).toBe(MENU.bacXiu.basePrice);

    await test.step("owner đổi giá trong trang quản trị", async () => {
      const admin = await browser.newPage({ baseURL, viewport: { width: 1280, height: 800 } });
      await ownerLoginUi(admin);
      await admin.goto(ROUTES.adminMenu);
      await admin.getByRole("row").filter({ has: admin.getByText(MENU.bacXiu.name, { exact: true }) }).getByRole("link", { name: "Sửa" }).or(
        admin.getByRole("row").filter({ has: admin.getByText(MENU.bacXiu.name, { exact: true }) }).getByRole("button", { name: "Sửa" }),
      ).first().click();
      const price = admin.getByLabel(/Giá gốc/);
      await price.fill(String(NEW_PRICE));
      await admin.getByRole("button", { name: "Lưu thay đổi" }).click();
      await expect.poll(async () => (await ownerApi.product(MENU.bacXiu.name)).basePrice).toBe(NEW_PRICE);
      await admin.close();
    });

    for (const t of tablets) {
      await test.step(`POS ${t.branch} hiện giá mới`, async () => {
        expect((await t.api.menuProduct(MENU.bacXiu.name)).basePrice).toBe(NEW_PRICE);
        const page = await t.ctx.newPage();
        await page.goto(ROUTES.pos);
        // Open a new takeaway order to reach the menu grid.
        await page.getByRole("link", { name: "Đơn mang đi mới" }).or(page.getByRole("button", { name: "Đơn mang đi mới" })).first().click();
        const tile = page.getByRole("button", { name: new RegExp(`^${escapeRe(MENU.bacXiu.name)}`) }).first();
        await expect(tile).toContainText(vnd(NEW_PRICE));
        await page.close();
      });
    }

    await test.step("đơn cũ giữ giá cũ, dòng mới dùng giá mới", async () => {
      const again = await cn1.api.order(oldOrder.id);
      expect(again.items[0].unitPrice).toBe(MENU.bacXiu.basePrice);
      const withNew = await cn1.api.addItem(oldOrder.id, { productId: original.id, sizeCode: "M", qty: 1 });
      const prices = withNew.items.map((i) => i.unitPrice).sort();
      expect(prices).toEqual([MENU.bacXiu.basePrice, NEW_PRICE]);
    });
  } finally {
    await restorePrice(ownerApi, original);
    for (const t of tablets) {
      await t.api.closeShiftIfOpen().catch(() => undefined);
      await t.ctx.close();
    }
  }
});

async function restorePrice(api: Api, p: S["Product"]) {
  const input: S["ProductInput"] = {
    categoryId: p.categoryId,
    name: p.name,
    description: p.description,
    basePrice: p.basePrice,
    isActive: p.isActive,
    isFeatured: p.isFeatured,
    sortOrder: p.sortOrder,
    sizes: p.sizes,
    modifiers: p.modifiers.map((m) => ({ id: m.id, name: m.name, price: m.price })),
  };
  await api.put(`/products/${p.id}`, input);
}
