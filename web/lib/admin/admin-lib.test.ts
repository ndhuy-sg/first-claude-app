import { describe, expect, it } from "vitest";
import { formatDate, formatTime, formatVND } from "@/lib/format";
import { allBranches, ownerMe, ql1Me } from "./fixtures";
import { formatVatRate, invoiceStatus } from "./labels";
import { alertHref, homeFor, isActive, visibleNav } from "./nav";
import { can, isAdminRole } from "./permissions";
import { addDays, average, buildSalesQuery, daysBetween, longDate, percent, shortDate } from "./reports";
import { defaultScope, parseScope, scopeOptions, scopeToBranchId } from "./scope";

describe("W-12 scope selector options", () => {
  it("owner sees Toàn chuỗi and every branch", () => {
    const opts = scopeOptions(ownerMe, [allBranches[1], allBranches[0]]);
    expect(opts.map((o) => o.value)).toEqual(["all", "1", "2"]);
    expect(opts[0].label).toBe("Toàn chuỗi");
    expect(defaultScope(ownerMe)).toBe("all");
  });
  it("ql1 sees only CN1 even if the list contains other branches", () => {
    const opts = scopeOptions(ql1Me, allBranches);
    expect(opts).toEqual([{ value: "1", label: "CN1 – Mộc Nguyễn Huệ" }]);
    expect(defaultScope(ql1Me)).toBe(1);
  });
  it("converts scope to the branchId query param", () => {
    expect(scopeToBranchId("all")).toBeUndefined();
    expect(scopeToBranchId(parseScope("2"))).toBe(2);
  });
});

describe("W-13 sales report query", () => {
  it("whole chain omits branchId", () => {
    expect(buildSalesQuery({ scope: "all", from: "2026-11-13", to: "2026-11-19" }, "day")).toEqual({
      from: "2026-11-13",
      to: "2026-11-19",
      groupBy: "day",
    });
  });
  it("one branch adds branchId and reversed dates are swapped", () => {
    expect(buildSalesQuery({ scope: 2, from: "2026-11-19", to: "2026-11-01" }, "payment_method")).toEqual({
      branchId: 2,
      from: "2026-11-01",
      to: "2026-11-19",
      groupBy: "payment_method",
    });
  });
  it("date helpers", () => {
    expect(addDays("2026-11-19", -6)).toBe("2026-11-13");
    expect(addDays("2026-12-31", 1)).toBe("2027-01-01");
    expect(daysBetween("2026-11-29", "2026-12-02")).toEqual(["2026-11-29", "2026-11-30", "2026-12-01", "2026-12-02"]);
    expect(shortDate("2026-11-09")).toBe("09/11");
    expect(longDate("2026-11-19")).toBe("19/11/2026");
    expect(average(86000, 0)).toBe(0);
    expect(average(100000, 3)).toBe(33333);
    expect(percent(1, 3)).toBe("33%");
  });
});

describe("W-15 formats used by admin pages", () => {
  it("money, dates, VAT rate", () => {
    expect(formatVND(1250000)).toBe("1.250.000đ");
    expect(formatDate("2026-11-19T03:00:00Z")).toBe("19/11/2026");
    expect(formatTime("2026-11-19T13:05:00Z")).toBe("20:05");
    expect(formatVatRate(800)).toBe("8%");
    expect(formatVatRate(1050)).toBe("10,5%");
    expect(invoiceStatus.failed.label).toBe("Thất bại");
  });
});

describe("navigation and permissions", () => {
  it("branch manager cannot see settings-only or menu-edit actions", () => {
    expect(can("branch_manager", "edit_menu")).toBeNull();
    expect(can("branch_manager", "view_invoices")).toBe("own");
    expect(can("owner", "settings")).toBe("chain");
    expect(isAdminRole("cashier")).toBe(false);
  });
  it("warehouse manager lands on the audit log; phase-2 items stay visible as disabled", () => {
    expect(homeFor("warehouse_manager")).toBe("/admin/nhat-ky");
    expect(homeFor("owner")).toBe("/admin");
    const items = visibleNav("owner").flatMap((g) => g.items);
    expect(items.find((i) => i.label === "Tồn kho")?.phase).toBe("GĐ2");
  });
  it("dashboard alert links", () => {
    expect(alertHref("/admin/hoa-don?status=failed")).toBe("/admin/hoa-don?status=failed");
    expect(alertHref("/admin/ca/12")).toBe("/admin/don-hang?shiftId=12");
    expect(alertHref("https://evil.example")).toBeNull();
    expect(alertHref(null)).toBeNull();
  });
  it("active link matching", () => {
    expect(isActive("/admin", "/admin")).toBe(true);
    expect(isActive("/admin", "/admin/mon")).toBe(false);
    expect(isActive("/admin/mon", "/admin/mon/10")).toBe(true);
  });
});
