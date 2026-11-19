import { expect, test, type Page } from "@playwright/test";
import { addCroissant, cancelOrder, formatVND, loginWithShift, openFreeTable, parseVND, POS_VIEWPORTS } from "./helpers";

test.describe.configure({ mode: "serial" });

async function toPayment(page: Page) {
  await page.getByRole("button", { name: "Thanh toán 35.000đ" }).click();
  await page.waitForURL(/\/pos\/thanh-toan\/\d+$/);
  await expect(page.getByTestId("amount-due")).toHaveText("35.000đ");
}

async function expectDoneWithInvoice(page: Page, buyer: string) {
  await page.waitForURL(/\/pos\/hoan-tat\/\d+$/);
  await expect(page.getByRole("heading", { name: "Đã thanh toán 35.000đ" })).toBeVisible();
  const card = page.getByRole("complementary", { name: "Hoá đơn điện tử" });
  await expect(card.getByText("Quét để nhận hoá đơn điện tử")).toBeVisible();
  await expect(card.getByTestId("invoice-qr")).toBeVisible();
  await expect(card.getByTestId("invoice-number")).toHaveText(/^\d{8}$/);
  await expect(card).toContainText(buyer);
}

for (const viewport of POS_VIEWPORTS) {
  test.describe(`${viewport.width}×${viewport.height}`, () => {
    test.use({ viewport });
    let orderId = 0;

    test.beforeEach(async ({ page, baseURL }) => {
      await loginWithShift(page, baseURL);
      ({ orderId } = await openFreeTable(page));
      await addCroissant(page); // 35.000đ
    });
    test.afterEach(async ({ page }) => {
      // only unpaid orders are cancelled; paid ones answer 409 and are left as sales
      if (orderId) await cancelOrder(page, orderId);
    });

    // W-07: cash suggestions, change, done screen with the e-invoice lookup QR
    test("W-07 cash payment with suggestions and change", async ({ page }) => {
      await toPayment(page);
      await expect(page.getByRole("button", { name: "Còn thiếu 35.000đ" })).toBeDisabled();
      const suggestions = page.getByLabel("Gợi ý mệnh giá").getByRole("button");
      await expect(suggestions.first()).toBeVisible();
      const values = (await suggestions.allInnerTexts()).map(parseVND);
      expect(values[0]).toBe(35000);
      expect([...values].sort((a, b) => a - b)).toEqual(values);
      expect(values.every((v) => v >= 35000)).toBe(true);

      const fifty = page.getByLabel("Gợi ý mệnh giá").getByRole("button", { name: "50.000đ" });
      await fifty.click();
      await expect(page.getByTestId("change-box")).toContainText("Tiền thối");
      await expect(page.getByTestId("change-box")).toContainText("15.000đ");
      await page.getByRole("button", { name: "Hoàn tất thanh toán" }).click();

      await expectDoneWithInvoice(page, "Khách lẻ");
      await expect(page.getByTestId("done-given")).toHaveText("50.000đ");
      await expect(page.getByTestId("done-change")).toHaveText("15.000đ");
      await page.getByRole("button", { name: "Đơn mới" }).click();
      await page.waitForURL(/\/pos\/ban$/);
    });

    // W-08: VietQR with amount, "Đã nhận tiền" → done
    test("W-08 VietQR payment", async ({ page }) => {
      await toPayment(page);
      await page.getByRole("tab", { name: "Chuyển khoản QR" }).click();
      const panel = page.getByTestId("qr-panel");
      await expect(panel.getByTestId("vietqr-code")).toBeVisible();
      await expect(panel.getByTestId("qr-amount")).toHaveText("35.000đ");
      await expect(panel.getByText("Đang chờ khách chuyển khoản…")).toBeVisible();
      await panel.getByRole("button", { name: "Đã nhận tiền" }).click();
      await expectDoneWithInvoice(page, "Khách lẻ");
      await expect(page.getByText(/Chuyển khoản QR · Thu ngân Lan/)).toBeVisible();
    });

    // W-09: company invoice form checks the tax code
    test("W-09 company buyer with tax-code validation", async ({ page }) => {
      await toPayment(page);
      await page.getByRole("radio", { name: "Công ty" }).click();
      await page.getByLabel("Gợi ý mệnh giá").getByRole("button", { name: "35.000đ" }).click();
      const taxCode = page.getByRole("textbox", { name: /^Mã số thuế/ });
      await taxCode.fill("12345");
      await page.getByRole("textbox", { name: /^Tên đơn vị/ }).fill("Công ty TNHH Thử Nghiệm");
      await page.getByRole("button", { name: "Hoàn tất thanh toán" }).click();
      await expect(page.getByText(/Mã số thuế gồm 10 số/)).toBeVisible();
      await expect(taxCode).toHaveAttribute("aria-invalid", "true");
      await expect(page).toHaveURL(/\/pos\/thanh-toan\/\d+$/);

      await taxCode.fill("0101234567-001");
      await page.getByRole("button", { name: "Hoàn tất thanh toán" }).click();
      await expectDoneWithInvoice(page, "Công ty TNHH Thử Nghiệm");
    });

    test("W-09 company left blank → invoice says Khách lẻ", async ({ page }) => {
      await toPayment(page);
      await page.getByRole("radio", { name: "Công ty" }).click();
      await page.getByLabel("Gợi ý mệnh giá").getByRole("button", { name: formatVND(35000) }).click();
      await page.getByRole("button", { name: "Hoàn tất thanh toán" }).click();
      await expectDoneWithInvoice(page, "Khách lẻ");
    });
  });
}
