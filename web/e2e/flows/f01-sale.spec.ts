/**
 * F-01 (docs/test-cases/phase-1-pos.md): tn1 mở ca → bàn 5 → thêm 2 món → giảm 10% → trả tiền mặt
 * → hoá đơn điện tử phát hành → hiện QR tra cứu → chốt ca.
 * Expected: CN1 report revenue grows by exactly the order total, CN2 does not change;
 * the shift closes with the right expected cash.
 *
 * Order: Cà phê sữa size L + Thêm shot (43.000đ) + Croissant bơ (35.000đ) = 78.000đ,
 * 10% off → 7.800đ, total 70.200đ; customer pays 100.000đ → change 29.800đ.
 */
import { Api, type S } from "../support/api";
import { activateDevice, addProduct, contextApi, expect, pinLoginUi, tableTile, tapDigits, test } from "../support/fixtures";
import { ROUTES } from "../support/routes";
import { ACCOUNTS, MENU, vnd } from "../support/seed";

const OPENING_CASH = 500_000;
const SUBTOTAL = MENU.caPheSua.basePrice + MENU.caPheSua.sizeL + MENU.caPheSua.themShot + MENU.croissant.basePrice; // 78.000
const DISCOUNT = Math.floor(SUBTOTAL * 0.1); // 7.800
const TOTAL = SUBTOTAL - DISCOUNT; // 70.200
const GIVEN = 100_000;

test.describe.configure({ mode: "serial" });

test("F-01 bán tại bàn 5, giảm 10%, tiền mặt, hoá đơn điện tử, chốt ca", async ({ page, context, baseURL, ownerApi }) => {
  const cn1 = await ownerApi.branchId("CN1");
  const cn2 = await ownerApi.branchId("CN2");
  const before = { cn1: await ownerApi.revenue(cn1), cn2: await ownerApi.revenue(cn2) };

  // The CN1 iPad: device cookie, then a clean slate (no open shift left by another spec).
  await activateDevice(context, "CN1", baseURL!);
  const posApi = contextApi(context);
  await posApi.pinLogin(ACCOUNTS.tn1.name, ACCOUNTS.tn1.pin);
  await posApi.closeShiftIfOpen();
  await posApi.post("/auth/logout");

  await test.step("tn1 đăng nhập bằng PIN", async () => {
    await pinLoginUi(page, ACCOUNTS.tn1.name, ACCOUNTS.tn1.pin);
  });

  await test.step("mở ca với 500.000đ", async () => {
    await page.goto(ROUTES.pos);
    const openBtn = page.getByRole("button", { name: "Mở ca và bắt đầu bán" }).or(page.getByRole("link", { name: "Mở ca và bắt đầu bán" }));
    await expect(openBtn).toBeVisible();
    const amount = page.getByLabel(/Tiền mặt đầu ca/);
    if (await amount.isEditable().catch(() => false)) await amount.fill(String(OPENING_CASH));
    else await tapDigits(page, "500000");
    await openBtn.click();
    const shift = await posApi.currentShift();
    expect(shift?.openingCash).toBe(OPENING_CASH);
  });

  await test.step("chọn bàn 5 và thêm 2 món", async () => {
    await tableTile(page, "Bàn 5").click();
    await addProduct(page, MENU.caPheSua.name, { size: "L", modifiers: ["Thêm shot"] });
    const cart = page.getByRole("region", { name: "Đơn hàng" }).or(page.getByLabel("Đơn hàng")).first();
    await expect(cart.getByText(vnd(43_000)).first()).toBeVisible();
    await addProduct(page, MENU.croissant.name);
    await expect(cart.getByText(vnd(SUBTOTAL)).first()).toBeVisible();
  });

  await test.step("giảm giá 10% (thu ngân tự làm được)", async () => {
    await page.getByRole("button", { name: "Giảm giá" }).click();
    const dialog = page.getByRole("dialog");
    await dialog.getByRole("button", { name: "Theo %" }).click();
    const value = dialog.getByRole("spinbutton").or(dialog.getByRole("textbox", { name: /%|Giá trị|Mức giảm/ })).first();
    if (await value.isVisible().catch(() => false)) await value.fill("10");
    else await tapDigits(page, "10", dialog);
    await dialog.getByLabel(/Lý do/).fill("Khách quen");
    await dialog.getByRole("button", { name: /^(Áp dụng|Duyệt|Xác nhận|Lưu)/ }).first().click();
    await expect(dialog).toBeHidden();
    await expect(page.getByText(vnd(TOTAL)).first()).toBeVisible();
  });

  let orderCode = "";
  await test.step("thanh toán tiền mặt 100.000đ, thối 29.800đ", async () => {
    await page.getByRole("button", { name: "Thanh toán" }).or(page.getByRole("link", { name: "Thanh toán" })).first().click();
    await page.getByRole("tab", { name: "Tiền mặt" }).click();
    await page.getByRole("button", { name: vnd(GIVEN), exact: true }).click();
    await expect(page.getByText(vnd(GIVEN - TOTAL)).first()).toBeVisible();
    await page.getByRole("button", { name: "Hoàn tất thanh toán" }).or(page.getByRole("link", { name: "Hoàn tất thanh toán" })).first().click();
    await expect(page.getByRole("heading", { name: new RegExp(`Đã thanh toán ${vnd(TOTAL)}`) })).toBeVisible();
  });

  await test.step("hoá đơn điện tử đã phát hành, QR tra cứu hiện trên màn hình", async () => {
    const invoiceBox = page.getByRole("region", { name: "Hoá đơn điện tử" }).or(page.getByLabel("Hoá đơn điện tử")).first();
    await expect(invoiceBox.getByText("Quét để nhận hoá đơn điện tử")).toBeVisible();
    await expect(invoiceBox.getByRole("img", { name: /QR/ }).or(invoiceBox.locator("svg, canvas, img")).first()).toBeVisible();

    const paid = await posApi.get<S["OrderList"]>("/orders", { status: "paid", q: "Bàn 5", pageSize: 200 });
    const order = paid.items.find((o) => o.total === TOTAL && o.tableName === "Bàn 5");
    expect(order, "paid order for Bàn 5 with 70.200đ").toBeTruthy();
    expect(order!.discount?.amount).toBe(DISCOUNT);
    expect(order!.invoice, "invoice issued in the payment transaction").toBeTruthy();
    orderCode = order!.code;
    await expect(invoiceBox.getByText(order!.invoice!.lookupCode)).toBeVisible();

    // The customer can open the lookup page without logging in (E-11).
    const anon = await Api.create(baseURL!);
    const pub = await anon.get<S["Invoice"]>(`/public/invoices/${encodeURIComponent(order!.invoice!.lookupCode)}`);
    expect(pub).toBeTruthy();
    await anon.dispose();
    const lookup = await context.browser()!.newPage({ baseURL });
    await lookup.goto(ROUTES.lookup(order!.invoice!.lookupCode));
    await expect(lookup.getByText(vnd(TOTAL)).first()).toBeVisible();
    await lookup.close();
  });

  await test.step("chốt ca: tiền mặt theo hệ thống = đầu ca + tiền mặt thu", async () => {
    const shift = (await posApi.currentShift())!;
    await page.goto(ROUTES.posTables);
    await page.getByRole("link", { name: "Chốt ca" }).or(page.getByRole("button", { name: "Chốt ca" })).first().click();
    await expect(page.getByText("Tiền mặt theo hệ thống")).toBeVisible();
    await expect(page.getByText(vnd(OPENING_CASH + TOTAL)).first()).toBeVisible();

    // Counting by denomination is covered by W-10 (POS specs); here the API closes with the exact amount.
    const sum = await posApi.get<S["ShiftSummary"]>(`/shifts/${shift.id}/summary`);
    expect(sum.expectedCash).toBe(OPENING_CASH + TOTAL);
    expect(sum.cashSales).toBe(TOTAL);
    const closed = await posApi.post<S["ShiftSummary"]>(`/shifts/${shift.id}/close`, { countedCash: OPENING_CASH + TOTAL });
    expect(closed.cashDiff).toBe(0);
    expect(closed.orderCount).toBeGreaterThanOrEqual(1);
  });

  await test.step("báo cáo: CN1 tăng đúng tổng đơn, CN2 không đổi", async () => {
    expect(await ownerApi.revenue(cn1)).toBe(before.cn1 + TOTAL);
    expect(await ownerApi.revenue(cn2)).toBe(before.cn2);
    expect(orderCode).toMatch(/^CN1-\d{8}-\d{4}$/);
  });
});
