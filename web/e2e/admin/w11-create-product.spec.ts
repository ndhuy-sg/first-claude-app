import { expect, test } from "@playwright/test";
import { loginAdmin } from "./helpers";

test.use({ viewport: { width: 1280, height: 800 } });

// W-11: owner creates a product in admin → it is on the POS menu of every branch.
test("W-11 owner creates a product and it is on every branch's menu", async ({ page }) => {
  await loginAdmin(page, "owner");
  await page.getByRole("link", { name: "Món & giá" }).click();
  await expect(page.getByRole("table", { name: "Danh sách món" })).toBeVisible();

  const name = `Món thử ${Date.now()}`;
  await page.getByRole("button", { name: "+ Thêm món" }).click();
  const form = page.getByRole("form", { name: "Thêm món mới" });
  await form.getByLabel("Tên món").fill(name);
  await form.getByLabel("Giá gốc (đồng, đã gồm thuế)").fill("32000");
  await form.getByRole("button", { name: "Lưu món" }).click();

  await expect(page.getByRole("status")).toContainText(`Đã thêm món “${name}”`);
  const row = page.getByRole("table", { name: "Danh sách món" }).getByRole("row").filter({ hasText: name });
  await expect(row).toContainText("32.000đ");

  // POS menu of every branch (same session cookie): the product is there with the same price
  const branches = (await (await page.request.get("/api/v1/branches")).json()) as { id: number; code: string }[];
  expect(branches.length).toBeGreaterThanOrEqual(2);
  for (const b of branches) {
    const menu = (await (await page.request.get(`/api/v1/menu?branchId=${b.id}`)).json()) as {
      categories: { products: { name: string; basePrice: number }[] }[];
    };
    const found = menu.categories.flatMap((c) => c.products).find((p) => p.name === name);
    expect(found, `menu of ${b.code}`).toBeTruthy();
    expect(found?.basePrice).toBe(32000);
  }
});
