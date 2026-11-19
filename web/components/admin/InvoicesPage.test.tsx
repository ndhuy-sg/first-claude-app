import { apiError, callsTo, mockApi, renderAdmin } from "@/lib/admin/test-utils";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { allBranches, cn1, invoice, ownerMe, ql1Me, settings } from "@/lib/admin/fixtures";
import { InvoicesPage } from "./InvoicesPage";
import { OrdersPage } from "./OrdersPage";

let search = new URLSearchParams();
vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn(), push: vi.fn() }),
  usePathname: () => "/admin",
  useSearchParams: () => search,
}));
beforeEach(() => {
  search = new URLSearchParams();
});

const failed: Schemas["Invoice"] = { ...invoice, id: 502, number: "00000422", status: "failed", branchId: 2, symbol: "1C26TMD", buyer: { name: "Công ty ABC", taxCode: "0101234567" } };

const list = (items: Schemas["Invoice"][]): Schemas["InvoiceList"] => ({
  items,
  total: items.length,
  counts: { issuedToday: 12, notSent: 10, failed: 1, cancelled: 0 },
});

describe("E-09 invoices page", () => {
  it("shows the tax-off banner linking to settings, counts and the table", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /settings": { body: settings },
      "GET /invoices": { body: list([invoice, failed]) },
    });
    renderAdmin(<InvoicesPage />, ownerMe);
    const banner = await screen.findByRole("note", { name: "Cảnh báo hoá đơn" });
    expect(banner).toHaveTextContent("chưa có giá trị pháp lý");
    expect(within(banner).getByRole("link", { name: "Mở Cài đặt" })).toHaveAttribute("href", "/admin/cai-dat");

    const table = await screen.findByRole("table", { name: "Danh sách hoá đơn" });
    expect(table).toHaveTextContent("00000421");
    expect(table).toHaveTextContent("Khách lẻ");
    expect(table).toHaveTextContent("6.370đ (8%)");
    expect(table).toHaveTextContent("Công ty ABC");
    expect(screen.getByText("Gửi thất bại").parentElement).toHaveTextContent("1");
    await waitFor(() => expect(table).toHaveTextContent("CN2"));
  });

  it("hides the banner when submission is on", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /settings": { body: { ...settings, taxSubmissionEnabled: true, taxGateway: "tvan" } },
      "GET /invoices": { body: list([invoice]) },
    });
    renderAdmin(<InvoicesPage />, ownerMe);
    await screen.findByRole("table", { name: "Danh sách hoá đơn" });
    expect(screen.queryByRole("note", { name: "Cảnh báo hoá đơn" })).not.toBeInTheDocument();
  });

  it("filters by branch and status, and resends a failed invoice", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /settings": { body: settings },
      "GET /invoices": { body: list([failed]) },
      "POST /invoices/502/resend": { status: 202 },
    });
    renderAdmin(<InvoicesPage />, ownerMe);
    await screen.findByRole("table", { name: "Danh sách hoá đơn" });
    expect(callsTo("GET", "/invoices")[0].query).toEqual({ page: "1", pageSize: "50" });

    const branch = screen.getByLabelText("Chi nhánh");
    await waitFor(() => expect(within(branch).getAllByRole("option")).toHaveLength(3));
    fireEvent.change(branch, { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Trạng thái"), { target: { value: "failed" } });
    await waitFor(() => expect(callsTo("GET", "/invoices").at(-1)?.query).toEqual({ branchId: "2", status: "failed", page: "1", pageSize: "50" }));

    await userEvent.click(await screen.findByRole("button", { name: "Gửi lại hoá đơn 00000422" }));
    await waitFor(() => expect(callsTo("POST", "/invoices/502/resend")).toHaveLength(1));
  });

  it("starts with the status from the URL (dashboard alert ?status=failed)", async () => {
    search = new URLSearchParams("status=failed");
    mockApi({ "GET /branches": { body: allBranches }, "GET /settings": { body: settings }, "GET /invoices": { body: list([failed]) } });
    renderAdmin(<InvoicesPage />, ownerMe);
    await screen.findByRole("table", { name: "Danh sách hoá đơn" });
    expect(callsTo("GET", "/invoices")[0].query.status).toBe("failed");
    expect(screen.getByLabelText("Trạng thái")).toHaveValue("failed");
  });

  it("ql1 queries only CN1 and sees no banner when settings are forbidden", async () => {
    mockApi({
      "GET /branches": { body: [cn1] },
      "GET /settings": apiError(403, "FORBIDDEN", "Không có quyền"),
      "GET /invoices": { body: list([invoice]) },
    });
    renderAdmin(<InvoicesPage />, ql1Me);
    await screen.findByRole("table", { name: "Danh sách hoá đơn" });
    expect(callsTo("GET", "/invoices")[0].query.branchId).toBe("1");
    expect(screen.queryByRole("note")).not.toBeInTheDocument();
  });
});

describe("screen 23: orders page", () => {
  const order: Schemas["Order"] = {
    id: 900,
    code: "CN1-20261119-0042",
    branchId: 1,
    type: "dine_in",
    tableId: 5,
    tableName: "Bàn 5",
    status: "paid",
    items: [{ id: 1, productId: 10, name: "Cà phê sữa", sizeCode: "L", sizeName: "L", modifiers: [{ id: 1, name: "Thêm shot", price: 8000 }], qty: 2, unitPrice: 43000, lineTotal: 86000, note: "" }],
    subtotal: 86000,
    discount: null,
    total: 86000,
    paidAmount: 86000,
    payments: [{ id: 1, method: "cash", status: "confirmed", amount: 86000, cashReceived: 100000, changeAmount: 14000, qrPayload: "", createdAt: "2026-11-19T00:10:00Z", confirmedAt: "2026-11-19T00:10:00Z" }],
    buyer: {},
    invoice: { id: 501, symbol: "1C26TMC", number: "00000421", lookupCode: "7K3P-9QXA", lookupUrl: "", status: "not_sent", kind: "normal" },
    createdBy: { id: 4, name: "Thu ngân 1" },
    createdAt: "2026-11-19T00:05:00Z",
    paidAt: "2026-11-19T00:10:00Z",
  };

  it("lists orders, searches and expands an order", async () => {
    mockApi({ "GET /branches": { body: allBranches }, "GET /orders": { body: { items: [order], total: 1 } } });
    renderAdmin(<OrdersPage />, ownerMe);
    const table = await screen.findByRole("table", { name: "Danh sách đơn hàng" });
    expect(table).toHaveTextContent("07:10");
    expect(table).toHaveTextContent("86.000đ");
    expect(table).toHaveTextContent("Tiền mặt");
    expect(table).toHaveTextContent("Đã thanh toán");
    await userEvent.click(screen.getByRole("button", { name: "CN1-20261119-0042" }));
    expect(table).toHaveTextContent("2 × Cà phê sữa (L)");
    fireEvent.change(screen.getByLabelText("Tìm"), { target: { value: "0042" } });
    await waitFor(() => expect(callsTo("GET", "/orders").at(-1)?.query.q).toBe("0042"));
  });

  it("filters by shift when opened from a cash-difference alert", async () => {
    search = new URLSearchParams("shiftId=12");
    mockApi({ "GET /branches": { body: allBranches }, "GET /orders": { body: { items: [order], total: 1 } } });
    renderAdmin(<OrdersPage />, ownerMe);
    await screen.findByRole("table", { name: "Danh sách đơn hàng" });
    expect(callsTo("GET", "/orders")[0].query).toEqual({ shiftId: "12", page: "1", pageSize: "50" });
    await userEvent.click(screen.getByRole("button", { name: "Bỏ lọc theo ca" }));
    await waitFor(() => expect(callsTo("GET", "/orders").at(-1)?.query.date).toBeTruthy());
  });
});
