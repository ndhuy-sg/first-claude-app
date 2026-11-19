import { expect, test } from "@playwright/test";
import { loginAdmin } from "./admin/helpers";

test.use({ viewport: { width: 390, height: 844 } });

// E-11: /tra-cuu/:code works without login and shows the invoice; a wrong code shows "not found".
test("E-11 public lookup shows an invoice without login", async ({ browser }) => {
  // find a real lookup code as the owner
  const admin = await browser.newContext();
  const adminPage = await admin.newPage();
  await loginAdmin(adminPage, "owner");
  const list = (await (await adminPage.request.get("/api/v1/invoices?pageSize=1")).json()) as {
    items: { lookupCode: string; number: string; symbol: string }[];
  };
  await admin.close();
  test.skip(list.items.length === 0, "no invoice issued yet (run after a sale, e.g. flow F-01)");
  const inv = list.items[0];

  const guest = await browser.newContext();
  const page = await guest.newPage();
  await page.goto("/tra-cuu");
  await page.getByLabel(/Mã tra cứu/).fill(inv.lookupCode);
  await page.getByRole("button", { name: "Xem hoá đơn" }).click();
  const doc = page.getByRole("article", { name: `Hoá đơn số ${inv.number}` });
  await expect(doc).toBeVisible();
  await expect(doc).toContainText(inv.symbol);
  await expect(doc).toContainText("Tổng thanh toán");
  await expect(page.getByRole("button", { name: "Tải PDF" })).toBeVisible();
  await guest.close();
});

test("E-11 unknown code shows a friendly not-found message", async ({ page }) => {
  await page.goto("/tra-cuu/KHONG-CO-MA-NAY");
  await expect(page.getByRole("heading", { name: "Không tìm thấy hoá đơn" })).toBeVisible();
  await expect(page.getByRole("link", { name: "Nhập mã khác" })).toHaveAttribute("href", "/tra-cuu");
});
