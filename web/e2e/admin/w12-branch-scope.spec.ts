import { expect, test } from "@playwright/test";
import { loginAdmin } from "./helpers";

test.use({ viewport: { width: 1280, height: 800 } });

// W-12: ql1 only sees CN1 in the branch selector; owner sees every branch and "Toàn chuỗi".
test("W-12 ql1 sees only CN1", async ({ page }) => {
  await loginAdmin(page, "ql1");
  await page.goto("/admin/bao-cao");
  const select = page.getByLabel("Chi nhánh");
  await expect(select.locator("option")).toHaveCount(1);
  await expect(select.locator("option")).toHaveText([/^CN1/]);
  await expect(select.locator("option", { hasText: "Toàn chuỗi" })).toHaveCount(0);

  await page.goto("/admin/hoa-don");
  await expect(page.getByLabel("Chi nhánh").locator("option")).toHaveText([/^CN1/]);
});

test("W-12 owner sees Toàn chuỗi and all branches", async ({ page }) => {
  await loginAdmin(page, "owner");
  await expect(page).toHaveURL(/\/admin$/);
  const select = page.getByLabel("Chi nhánh");
  await expect(select.locator("option").first()).toHaveText("Toàn chuỗi");
  const texts = await select.locator("option").allTextContents();
  expect(texts.some((t) => t.startsWith("CN1"))).toBe(true);
  expect(texts.some((t) => t.startsWith("CN2"))).toBe(true);
});

test("W-12 admin pages need a login", async ({ page }) => {
  await page.goto("/admin/don-hang");
  await expect(page).toHaveURL(/\/admin\/dang-nhap$/);
});
