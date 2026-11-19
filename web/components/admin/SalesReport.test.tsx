import { callsTo, mockApi, renderAdmin } from "@/lib/admin/test-utils";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { allBranches, cn1, ownerMe, ql1Me } from "@/lib/admin/fixtures";
import { todayVN } from "@/lib/format";
import { addDays } from "@/lib/admin/reports";
import { Dashboard } from "./Dashboard";
import { SalesReport } from "./SalesReport";

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }), usePathname: () => "/admin" }));

function report(groupBy: string, rows: Schemas["SalesReport"]["rows"]): Schemas["SalesReport"] {
  const revenue = rows.reduce((s, r) => s + r.revenue, 0);
  const orderCount = rows.reduce((s, r) => s + r.orderCount, 0);
  return { from: "", to: "", groupBy, totals: { revenue, orderCount, discount: 8600, refunds: 0 }, rows };
}

const today = todayVN();

function salesRoute(q: { query: Record<string, string> }) {
  const g = q.query.groupBy;
  if (g === "day")
    return {
      body: report("day", [
        { key: addDays(today, -1), label: "", revenue: 300000, orderCount: 6, qty: 0 },
        { key: today, label: "", revenue: 120000, orderCount: 3, qty: 0 },
      ]),
    };
  if (g === "payment_method")
    return {
      body: report("payment_method", [
        { key: "cash", label: "cash", revenue: 320000, orderCount: 7, qty: 0 },
        { key: "vietqr", label: "vietqr", revenue: 100000, orderCount: 2, qty: 0 },
      ]),
    };
  return {
    body: report("branch", [
      { key: "1", label: "CN1", revenue: 400000, orderCount: 8, qty: 0 },
      { key: "2", label: "CN2", revenue: 20000, orderCount: 1, qty: 0 },
    ]),
  };
}

describe("W-13 sales report", () => {
  it("owner: whole chain by default, KPI tiles match the API, branch filter changes the query", async () => {
    mockApi({ "GET /branches": { body: allBranches }, "GET /reports/sales": salesRoute });
    renderAdmin(<SalesReport />, ownerMe);

    expect(await screen.findByText("420.000đ")).toBeInTheDocument();
    expect(screen.getByText("46.666đ")).toBeInTheDocument(); // 420.000 / 9 rounded down
    const first = callsTo("GET", "/reports/sales").find((c) => c.query.groupBy === "day")!;
    expect(first.query).toEqual({ from: addDays(today, -6), to: today, groupBy: "day" });

    // 7 bars, one per day, with an aria summary; labels only on max + latest
    const chart = screen.getByRole("img", { name: /Doanh thu theo ngày/ });
    expect(within(chart).getAllByTestId("vbar")).toHaveLength(7);
    expect(chart.getAttribute("aria-label")).toContain("300.000đ");
    expect(within(chart).getByText("300.000đ")).toBeInTheDocument();
    expect(within(chart).getByText("120.000đ")).toBeInTheDocument();

    expect(screen.getByText("Tiền mặt")).toBeInTheDocument();
    expect(screen.getByText("Chuyển khoản VietQR")).toBeInTheDocument();
    expect(screen.getByRole("table", { name: "So sánh chi nhánh" })).toHaveTextContent("95%");

    fireEvent.change(await screen.findByLabelText("Chi nhánh"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Từ ngày"), { target: { value: "2026-11-01" } });
    fireEvent.change(screen.getByLabelText("Đến ngày"), { target: { value: "2026-11-15" } });
    await waitFor(() =>
      expect(callsTo("GET", "/reports/sales").map((c) => c.query)).toContainEqual({
        branchId: "2",
        from: "2026-11-01",
        to: "2026-11-15",
        groupBy: "day",
      }),
    );
  });

  it("ql1: always queries CN1 and has no branch comparison", async () => {
    mockApi({ "GET /branches": { body: [cn1] }, "GET /reports/sales": salesRoute });
    renderAdmin(<SalesReport />, ql1Me);
    expect(await screen.findByText("420.000đ")).toBeInTheDocument();
    const queries = callsTo("GET", "/reports/sales").map((c) => c.query);
    expect(queries.every((q) => q.branchId === "1")).toBe(true);
    expect(queries.some((q) => q.groupBy === "branch")).toBe(false);
    expect(screen.queryByRole("table", { name: "So sánh chi nhánh" })).not.toBeInTheDocument();
  });
});

describe("W-12 scope selector", () => {
  const dashboard: Schemas["Dashboard"] = {
    date: today,
    revenue: 1250000,
    orderCount: 40,
    avgOrderValue: 31250,
    invoicesIssued: 38,
    byBranch: [
      { branchId: 2, branchCode: "CN2", revenue: 450000, orderCount: 15 },
      { branchId: 1, branchCode: "CN1", revenue: 800000, orderCount: 25 },
    ],
    topProducts: [{ productId: 10, name: "Cà phê sữa", qty: 30, revenue: 900000 }],
    alerts: [{ kind: "tax_submission_off", severity: "warning", message: "Chưa bật gửi dữ liệu hoá đơn tới cơ quan thuế", link: "/admin/cai-dat" }],
  };

  it("owner sees Toàn chuỗi and all branches; dashboard shows KPIs, bars and alerts", async () => {
    mockApi({ "GET /branches": { body: allBranches }, "GET /reports/dashboard": { body: dashboard } });
    renderAdmin(<Dashboard />, ownerMe);
    expect(await screen.findByText("1.250.000đ")).toBeInTheDocument();
    const select = screen.getByLabelText("Chi nhánh");
    await waitFor(() => expect(within(select).getAllByRole("option")).toHaveLength(3));
    expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["Toàn chuỗi", "CN1 – Mộc Nguyễn Huệ", "CN2 – Mộc Lê Lợi"]);
    expect(callsTo("GET", "/reports/dashboard")[0].query.branchId).toBeUndefined();

    const bars = screen.getByRole("list", { name: "Doanh thu theo chi nhánh" });
    expect(within(bars).getAllByRole("listitem").map((li) => li.textContent)).toEqual(["CN1800.000đ", "CN2450.000đ"]);
    expect(screen.getByText("Cần chú ý")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Xem" })).toHaveAttribute("href", "/admin/cai-dat");
    expect(screen.getByRole("table", { name: "Món bán chạy" })).toHaveTextContent("Cà phê sữa");

    fireEvent.change(select, { target: { value: "1" } });
    await waitFor(() => expect(callsTo("GET", "/reports/dashboard").at(-1)?.query.branchId).toBe("1"));
  });

  it("ql1 only sees CN1 (even if the API listed more)", async () => {
    mockApi({ "GET /branches": { body: allBranches }, "GET /reports/dashboard": { body: { ...dashboard, byBranch: [dashboard.byBranch[1]] } } });
    renderAdmin(<Dashboard />, ql1Me);
    const select = await screen.findByLabelText("Chi nhánh");
    await waitFor(() => expect(within(select).getAllByRole("option").map((o) => o.textContent)).toEqual(["CN1 – Mộc Nguyễn Huệ"]));
    expect(within(select).queryByText("Toàn chuỗi")).not.toBeInTheDocument();
    expect(callsTo("GET", "/reports/dashboard")[0].query.branchId).toBe("1");
  });
});
