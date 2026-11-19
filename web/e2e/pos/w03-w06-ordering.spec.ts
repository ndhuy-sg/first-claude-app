import { expect, test } from "@playwright/test";
import { addCroissant, cancelOrder, expectTouchTargets, loginWithShift, openFreeTable, pressKeys } from "./helpers";

test.describe.configure({ mode: "serial" });

// Viewports come from the Playwright projects (tablet-1180, tablet-1280, webkit-ipad).
test.describe("POS", () => {
  let orderId = 0;

  test.afterEach(async ({ page }) => {
    if (orderId) await cancelOrder(page, orderId);
    orderId = 0;
  });

  // W-03: categories, product grid and cart visible at the same time; touch targets ≥ 44px
  test("W-03 ordering layout", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL);
    await expectTouchTargets(page);
    ({ orderId } = await openFreeTable(page));
    await expect(page.getByRole("navigation", { name: "Nhóm món" })).toBeInViewport();
    await expect(page.getByRole("button", { name: /^Cà phê sữa ·/ })).toBeInViewport();
    await expect(page.getByRole("complementary", { name: "Đơn hàng" })).toBeInViewport();
    await expect(page.getByRole("button", { name: /^Thanh toán/ })).toBeInViewport();
    await addCroissant(page);
    await expectTouchTargets(page);
  });

  // W-04: table → product → size and modifier dialog → cart shows 43.000đ
  test("W-04 Cà phê sữa L + Thêm shot = 43.000đ", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL);
    ({ orderId } = await openFreeTable(page));
    await page.getByRole("button", { name: /^Cà phê sữa ·/ }).click();
    const dialog = page.getByRole("dialog", { name: "Cà phê sữa" });
    await expect(dialog.getByText("29.000đ")).toBeVisible();
    await dialog.getByRole("button", { name: /^L \+/ }).click();
    await dialog.getByRole("button", { name: /Thêm shot/ }).click();
    await expect(dialog.getByRole("button", { name: /Thêm shot/ })).toHaveAttribute("aria-pressed", "true");
    await dialog.getByRole("button", { name: "Thêm vào đơn · 43.000đ" }).click();
    await expect(dialog).toBeHidden();

    const line = page.getByTestId("cart-line").filter({ hasText: "Cà phê sữa" });
    await expect(line).toContainText("Size L · Thêm shot");
    await expect(line.getByTestId("line-total")).toHaveText("43.000đ");
    await expect(page.getByTestId("cart-total")).toHaveText("43.000đ");
  });

  // W-05: a sold-out product is greyed out and cannot be tapped (manager marks it on screen 12)
  test("W-05 sold-out product is disabled", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL, "ql1");
    await page.goto("/pos/het-mon");
    const sw = page.getByRole("switch", { name: /^Bạc xỉu:/ });
    if ((await sw.getAttribute("aria-checked")) === "true") await sw.click();
    await expect(page.getByRole("switch", { name: "Bạc xỉu: Hết" })).toHaveAttribute("aria-checked", "false");

    try {
      await page.goto("/pos/ban");
      ({ orderId } = await openFreeTable(page));
      const card = page.getByRole("button", { name: /^Bạc xỉu · .*hết món/ });
      await expect(card).toBeDisabled();
      await expect(card).toContainText("Hết món");
      await card.click({ force: true });
      await expect(page.getByRole("dialog")).toHaveCount(0);
    } finally {
      await page.goto("/pos/het-mon");
      await page.getByRole("switch", { name: "Bạc xỉu: Hết" }).click();
      await expect(page.getByRole("switch", { name: "Bạc xỉu: Đang bán" })).toBeVisible();
    }
  });

  // W-06: 25% discount → manager PIN keypad → total updated
  test("W-06 discount 25% needs the manager PIN", async ({ page, baseURL }) => {
    await loginWithShift(page, baseURL);
    ({ orderId } = await openFreeTable(page));
    await addCroissant(page); // 35.000đ
    await page.getByRole("button", { name: "Giảm giá", exact: true }).click();
    const dialog = page.getByRole("dialog", { name: "Giảm giá" });
    await dialog.getByRole("button", { name: "25%" }).click();
    const panel = dialog.getByRole("region", { name: "Cần quản lý duyệt" });
    await expect(panel).toBeVisible();
    await panel.getByRole("button", { name: "Tuấn" }).click();
    await pressKeys(page, "Bàn phím nhập PIN", ["9", "9", "9", "9"], panel);
    await panel.getByRole("button", { name: "Duyệt" }).click();
    await expect(dialog).toBeHidden();
    // 25% of 35.000 = 8.750 → 26.250đ
    await expect(page.getByTestId("cart-discount")).toHaveText("−8.750đ");
    await expect(page.getByTestId("cart-total")).toHaveText("26.250đ");
  });
});
