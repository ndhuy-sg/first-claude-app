import { expect, test } from "@playwright/test";
import {
  addCroissant,
  cancelOpenOrdersOfShift,
  cancelOrder,
  formatVND,
  loginPin,
  loginWithShift,
  openFreeTable,
  parseVND,
  pressKeys,
  setDeviceCN1,
} from "./helpers";

test.describe.configure({ mode: "serial" });

// Viewports come from the Playwright projects (tablet-1180, tablet-1280, webkit-ipad).
test.describe("POS", () => {
  // W-14: lost connection → banner, payment locked, cart kept; back online → continue
  test("W-14 offline banner locks payment", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL);
    const { orderId } = await openFreeTable(page);
    try {
      await addCroissant(page);
      await expect(page.getByRole("button", { name: "Thanh toán 35.000đ" })).toBeEnabled();

      await page.context().setOffline(true);
      await expect(page.getByRole("alert").filter({ hasText: "Mất kết nối tới máy chủ" })).toBeVisible();
      await expect(page.getByTestId("online-status")).toHaveText("Mất kết nối");
      await expect(page.getByRole("button", { name: "Chờ kết nối để thanh toán" })).toBeDisabled();
      await expect(page.getByRole("button", { name: /^Cà phê sữa ·/ })).toBeDisabled();
      await expect(page.getByTestId("cart-line")).toContainText("Croissant bơ");

      await page.context().setOffline(false);
      await expect(page.getByTestId("online-status")).toHaveText("Trực tuyến");
      await expect(page.getByRole("alert").filter({ hasText: "Mất kết nối tới máy chủ" })).toHaveCount(0);
      await expect(page.getByRole("button", { name: "Thanh toán 35.000đ" })).toBeEnabled();
      // the order is still on the server
      await page.reload();
      await expect(page.getByTestId("cart-line")).toContainText("Croissant bơ");
    } finally {
      await page.context().setOffline(false);
      await cancelOrder(page, orderId);
    }
  });

  // W-10: close the shift with a cash count (difference shown), then open a new one
  test("W-10 close shift and open the next one", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL);
    await cancelOpenOrdersOfShift(page);
    await page.getByRole("link", { name: "Chốt ca" }).click();
    await page.waitForURL(/\/pos\/chot-ca$/);
    await expect(page.getByText("Không còn đơn chưa thanh toán")).toBeVisible();

    const expected = parseVND(await page.getByTestId("expected-cash").innerText());
    // count one 500.000đ note and one 20.000đ note
    await page.getByRole("button", { name: "Thêm một tờ 500.000đ" }).click();
    await page.getByRole("button", { name: "Thêm một tờ 20.000đ" }).click();
    await expect(page.getByTestId("counted-total")).toHaveText("520.000đ");
    const diff = 520_000 - expected;
    await expect(page.getByTestId("cash-diff")).toHaveText(diff > 0 ? `+${formatVND(diff)}` : formatVND(diff));
    if (diff < 0) await expect(page.getByTestId("cash-diff")).toHaveClass(/text-bad/);
    await page.getByPlaceholder("Lý do chênh lệch").fill("Kiểm thử e2e");
    await page.getByRole("button", { name: "Chốt ca" }).click();

    await expect(page.getByRole("heading", { name: /^Đã chốt ca/ })).toBeVisible();
    await expect(page.getByTestId("counted-cash")).toHaveText("520.000đ");
    await expect(page.getByTestId("cash-diff")).toHaveText(diff > 0 ? `+${formatVND(diff)}` : formatVND(diff));
    await page.getByRole("button", { name: "Đăng xuất" }).click();
    await page.waitForURL(/\/pos\/dang-nhap$/);

    // next shift
    await setDeviceCN1(page, baseURL);
    await loginPin(page, "tn1");
    await expect(page).toHaveURL(/\/pos\/mo-ca$/);
    await pressKeys(page, "Bàn phím nhập tiền", ["5", "000", "000"]);
    await expect(page.getByTestId("opening-cash")).toHaveText("500.000đ");
    await page.getByRole("button", { name: "Mở ca và bắt đầu bán" }).click();
    await page.waitForURL(/\/pos\/ban$/);
    await expect(page.getByText(/^Ca (sáng|chiều|tối) · mở \d{2}:\d{2}$/)).toBeVisible();
  });
});
