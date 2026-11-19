import { expect, test } from "@playwright/test";
import { loginPin, pressKeys, staff, setDeviceCN1 } from "./helpers";

test.describe.configure({ mode: "serial" });

// Viewports come from the Playwright projects (tablet-1180, tablet-1280, webkit-ipad).
test.describe("POS", () => {
  // W-01: a manager activates the tablet → branch is shown on the device
  test("W-01 activate a tablet for CN1", async ({ page }) => {
    await page.goto("/pos");
    await page.waitForURL(/\/pos\/kich-hoat$/);
    await page.getByLabel("Email").fill(staff.ql1.email);
    await page.getByLabel("Mật khẩu").fill(staff.ql1.password);
    await page.getByRole("button", { name: "Tiếp tục" }).click();

    const form = page.getByRole("form", { name: "Kích hoạt máy" });
    // ql1 manages CN1 only, so CN1 is the one option; select it explicitly anyway.
    const cn1 = await form.locator("option", { hasText: "CN1" }).first().getAttribute("value");
    await form.getByLabel("Chi nhánh").selectOption(cn1!);
    const name = `iPad e2e ${test.info().project.name}`;
    await form.getByLabel("Tên máy").fill(name);
    await form.getByRole("button", { name: "Kích hoạt máy" }).click();

    await page.waitForURL(/\/pos\/dang-nhap$/);
    await expect(page.getByTestId("device-branch")).toHaveText(`Chi nhánh CN1 · ${name}`);
    await expect(page.getByRole("button", { name: /Lan/ })).toBeVisible();
    // The activation cookie stays: reloading /pos goes to the PIN screen, not activation
    await page.goto("/pos");
    await page.waitForURL(/\/pos\/dang-nhap$/);
  });

  // W-02: wrong PIN → error + cleared input; right PIN → sales; 15 min idle → back to PIN
  test("W-02 PIN pad: wrong PIN, right PIN, auto-lock after 15 minutes", async ({ page, baseURL }) => {
    await page.clock.install();
    await setDeviceCN1(page, baseURL);
    await page.goto("/pos/dang-nhap");
    await page.getByRole("button", { name: /Lan/ }).first().click();

    const submit = page.getByRole("button", { name: "Vào ca" });
    await pressKeys(page, "Bàn phím nhập PIN", ["0", "0", "0"]);
    await expect(submit).toBeDisabled();
    await pressKeys(page, "Bàn phím nhập PIN", ["0"]);
    await expect(submit).toBeEnabled();
    await submit.click();
    await expect(page.getByRole("alert").filter({ hasText: /\S/ })).toHaveText("Mã PIN không đúng, vui lòng thử lại");
    await expect(page.locator('[data-testid="pin-dot"][data-filled="true"]')).toHaveCount(0);
    await expect(page).toHaveURL(/\/pos\/dang-nhap$/);

    await pressKeys(page, "Bàn phím nhập PIN", staff.tn1.pin.split(""));
    await submit.click();
    await page.waitForURL(/\/pos\/(ban|mo-ca)$/);
    await expect(page.getByRole("button", { name: "Khoá màn hình" })).toBeVisible();

    // 15 minutes without touching the screen
    await page.clock.fastForward("15:05");
    await page.waitForURL(/\/pos\/dang-nhap$/);
    await expect(page.getByText("Chọn nhân viên", { exact: true })).toBeVisible();
  });

  test("W-02 lock button returns to the PIN screen", async ({ page, baseURL }) => {
    await setDeviceCN1(page, baseURL);
    await loginPin(page, "tn1");
    await page.getByRole("button", { name: "Khoá màn hình" }).click();
    await page.waitForURL(/\/pos\/dang-nhap$/);
    // the old session is gone
    expect((await page.request.get("/api/v1/auth/me")).status()).toBe(401);
  });
});
