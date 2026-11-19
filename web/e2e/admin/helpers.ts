import { expect, type Page } from "@playwright/test";

/** Seed accounts from docs/DEV.md */
export const accounts = {
  owner: { email: "owner@moc.vn", password: "matkhau123" },
  ql1: { email: "ql1@moc.vn", password: "matkhau123" },
} as const;

export async function loginAdmin(page: Page, who: keyof typeof accounts) {
  await page.goto("/admin/dang-nhap");
  await page.getByLabel("Email").fill(accounts[who].email);
  await page.getByLabel("Mật khẩu").fill(accounts[who].password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByRole("navigation", { name: "Quản trị" })).toBeVisible();
}

/** Today's business date in Vietnam (YYYY-MM-DD). */
export function todayVN(): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Ho_Chi_Minh", year: "numeric", month: "2-digit", day: "2-digit" }).format(new Date());
}

export function formatVND(n: number): string {
  return (n < 0 ? "−" : "") + String(Math.abs(Math.trunc(n))).replace(/\B(?=(\d{3})+(?!\d))/g, ".") + "đ";
}
