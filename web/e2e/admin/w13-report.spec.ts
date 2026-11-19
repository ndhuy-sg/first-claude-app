import { expect, test } from "@playwright/test";
import { formatVND, loginAdmin, todayVN } from "./helpers";

test.use({ viewport: { width: 1280, height: 800 } });

type Report = { totals: { revenue: number; orderCount: number } };

// W-13: report filters by date range and branch; the numbers match the API.
test("W-13 report filters by date range and branch and matches the API", async ({ page }) => {
  await loginAdmin(page, "owner");
  await page.getByRole("link", { name: "Báo cáo bán hàng" }).click();
  await expect(page.getByRole("heading", { name: "Báo cáo bán hàng" })).toBeVisible();

  const branches = (await (await page.request.get("/api/v1/branches")).json()) as { id: number; code: string }[];
  const cn1 = branches.find((b) => b.code === "CN1")!;
  const to = todayVN();
  const from = `${to.slice(0, 8)}01`; // first day of this month

  await page.getByLabel("Chi nhánh").selectOption(String(cn1.id));
  await page.getByLabel("Từ ngày").fill(from);
  await page.getByLabel("Đến ngày").fill(to);

  const api = (await (await page.request.get(`/api/v1/reports/sales?branchId=${cn1.id}&from=${from}&to=${to}&groupBy=day`)).json()) as Report;
  const revenueTile = page.getByRole("group", { name: "Doanh thu", exact: true });
  await expect(revenueTile).toContainText(formatVND(api.totals.revenue));
  const ordersTile = page.getByRole("group", { name: "Số đơn", exact: true });
  await expect(ordersTile).toContainText(String(api.totals.orderCount));

  // whole chain = sum of branches (R-02) as shown in the comparison table
  await page.getByLabel("Chi nhánh").selectOption("all");
  const chain = (await (await page.request.get(`/api/v1/reports/sales?from=${from}&to=${to}&groupBy=day`)).json()) as Report;
  await expect(page.getByRole("group", { name: "Doanh thu", exact: true })).toContainText(formatVND(chain.totals.revenue));
  await expect(page.getByRole("table", { name: "So sánh chi nhánh" })).toContainText("CN1");
});
