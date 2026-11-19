import { apiError, callsTo, mockApi, renderAdmin } from "@/lib/admin/test-utils";
import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { allBranches, cn1, ownerMe, ql1Me, settings } from "@/lib/admin/fixtures";
import { AuditLogPage, describeValue } from "./AuditLogPage";
import { BranchesPage } from "./BranchesPage";
import { SettingsPage } from "./SettingsPage";
import { StaffPage } from "./StaffPage";

vi.mock("next/navigation", () => ({ useRouter: () => ({ replace: vi.fn(), push: vi.fn() }), usePathname: () => "/admin" }));

describe("screen 37: settings (tax switch off by default)", () => {
  it("shows the switch OFF with a red warning and saves the full object", async () => {
    mockApi({ "GET /settings": { body: settings }, "PUT /settings": (req) => ({ body: req.body }) });
    renderAdmin(<SettingsPage />, ownerMe);
    const user = userEvent.setup();
    const sw = await screen.findByRole("switch", { name: "Gửi dữ liệu tới cơ quan thuế" });
    expect(sw).toHaveAttribute("aria-checked", "false");
    expect(sw).toHaveTextContent("TẮT");
    expect(screen.getByRole("alert")).toHaveTextContent("chưa có giá trị pháp lý");

    await user.click(sw);
    expect(sw).toHaveTextContent("BẬT");
    await user.click(screen.getByRole("button", { name: "Lưu cài đặt" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Chọn cách gửi");
    expect(callsTo("PUT", "/settings")).toHaveLength(0);

    await user.selectOptions(screen.getByLabelText("Cách gửi"), "tvan");
    await user.click(screen.getByRole("button", { name: "Lưu cài đặt" }));
    await waitFor(() => expect(callsTo("PUT", "/settings")).toHaveLength(1));
    expect(callsTo("PUT", "/settings")[0].body).toEqual({ ...settings, taxSubmissionEnabled: true, taxGateway: "tvan" });
    expect(await screen.findByRole("status")).toHaveTextContent("Đã lưu cài đặt");
  });

  it("is read-only for a branch manager", async () => {
    mockApi({ "GET /settings": { body: settings } });
    renderAdmin(<SettingsPage />, ql1Me);
    expect(await screen.findByRole("switch")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Lưu cài đặt" })).not.toBeInTheDocument();
  });
});

describe("screen 34: branches, tables, devices", () => {
  const areas: Schemas["Area"][] = [{ id: 1, name: "Tầng 1", tables: [{ id: 5, name: "Bàn 5", areaId: 1, status: "occupied", openOrder: null }] }];
  const devices: Schemas["Device"][] = [{ id: 7, name: "iPad quầy 1", branchId: 1, branchCode: "CN1", lastSeenAt: "2026-11-19T00:30:00Z", revoked: false }];

  it("owner sees tax info per branch, edits it, adds a table and revokes a device", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /settings": { body: settings },
      "GET /tables": { body: areas },
      "GET /devices": { body: devices },
      "PUT /branches/2": (req) => ({ body: { ...allBranches[1], ...(req.body as object) } }),
      "POST /tables": { status: 201, body: { id: 6, name: "Bàn 6", areaId: 1, status: "free", openOrder: null } },
      "POST /devices/7/revoke": { status: 204 },
    });
    renderAdmin(<BranchesPage />, ownerMe);
    const user = userEvent.setup();
    const card = await screen.findByRole("article", { name: "CN2" });
    expect(card).toHaveTextContent("0101234567");
    expect(card).toHaveTextContent("1C26TMD");
    expect(card).toHaveTextContent("8%");
    expect(card).toHaveTextContent("00000007");

    await user.click(within(card).getByRole("button", { name: "Sửa thông tin" }));
    const form = screen.getByRole("form", { name: "Sửa CN2" });
    const symbol = within(form).getByLabelText("Ký hiệu hoá đơn");
    await user.clear(symbol);
    await user.type(symbol, "abc");
    await user.click(within(form).getByRole("button", { name: "Lưu" }));
    expect(within(form).getByRole("alert")).toHaveTextContent("Ký hiệu hoá đơn gồm 7 ký tự");
    await user.clear(symbol);
    await user.type(symbol, "1c26maa");
    const tax = within(form).getByLabelText("Mã số thuế (trống = MST công ty)");
    await user.clear(tax);
    await user.type(tax, "0109999999-002");
    const vat = within(form).getByLabelText("Thuế suất (%)");
    await user.clear(vat);
    await user.type(vat, "10");
    await user.click(within(form).getByRole("button", { name: "Lưu" }));
    await waitFor(() => expect(callsTo("PUT", "/branches/2")).toHaveLength(1));
    expect(callsTo("PUT", "/branches/2")[0].body).toMatchObject({ code: "CN2", taxCode: "0109999999-002", vatRateBp: 1000, invoiceSymbol: "1C26MAA" });

    expect(await screen.findByText("Bàn 5")).toBeInTheDocument();
    expect(callsTo("GET", "/tables")[0].query.branchId).toBe("1");
    await user.type(screen.getByLabelText("Tên bàn"), "Bàn 6");
    await user.click(screen.getByRole("button", { name: "Thêm bàn" }));
    await waitFor(() => expect(callsTo("POST", "/tables")[0]?.body).toEqual({ areaId: 1, name: "Bàn 6" }));

    await user.click(screen.getByRole("button", { name: "Thu hồi iPad quầy 1" }));
    await waitFor(() => expect(callsTo("POST", "/devices/7/revoke")).toHaveLength(1));
  });

  it("branch manager cannot add or edit branches", async () => {
    mockApi({ "GET /branches": { body: [cn1] }, "GET /tables": { body: areas }, "GET /devices": { body: devices }, "GET /settings": apiError(403, "FORBIDDEN", "Không có quyền") });
    renderAdmin(<BranchesPage />, ql1Me);
    await screen.findByRole("article", { name: "CN1" });
    expect(screen.queryByRole("button", { name: "+ Thêm chi nhánh" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sửa thông tin" })).not.toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "Thêm bàn" })).toBeInTheDocument();
  });
});

describe("screen 35: staff", () => {
  const users: Schemas["User"][] = [
    { id: 3, name: "Quản lý CN1", role: "branch_manager", branchId: 1, email: "ql1@moc.vn", status: "active", hasPin: true, lastActiveAt: null },
    { id: 4, name: "Thu ngân 1", role: "cashier", branchId: 1, email: null, status: "active", hasPin: true, lastActiveAt: "2026-11-19T00:00:00Z" },
  ];

  it("ql1 creates cashiers for CN1 only, locks a user and shows API errors", async () => {
    mockApi({
      "GET /branches": { body: allBranches },
      "GET /users": { body: users },
      "POST /users/4/lock": { status: 204 },
      "POST /users": apiError(409, "PIN_TAKEN", "PIN đã được dùng ở chi nhánh này"),
    });
    renderAdmin(<StaffPage />, ql1Me);
    const user = userEvent.setup();
    const table = await screen.findByRole("table", { name: "Danh sách nhân viên" });
    expect(callsTo("GET", "/users")[0].query.branchId).toBe("1");
    // no actions on yourself (API: CANNOT_LOCK_SELF)
    expect(screen.queryByRole("button", { name: "Khoá Quản lý CN1" })).not.toBeInTheDocument();
    await user.click(within(table).getByRole("button", { name: "Khoá Thu ngân 1" }));
    await waitFor(() => expect(callsTo("POST", "/users/4/lock")).toHaveLength(1));

    await user.click(screen.getByRole("button", { name: "+ Thêm nhân viên" }));
    const form = screen.getByRole("form", { name: "Thêm nhân viên" });
    expect(within(within(form).getByLabelText("Vai trò")).getAllByRole("option").map((o) => o.textContent)).toEqual(["Thu ngân", "Pha chế"]);
    expect(within(form).getByLabelText("Chi nhánh")).toBeDisabled();
    await user.type(within(form).getByLabelText("Họ tên"), "Thu ngân 3");
    await user.type(within(form).getByLabelText("PIN (4–6 số)"), "1234");
    await user.click(within(form).getByRole("button", { name: "Lưu nhân viên" }));
    expect(await within(form).findByText("PIN đã được dùng ở chi nhánh này")).toBeInTheDocument();
    expect(callsTo("POST", "/users")[0].body).toEqual({ name: "Thu ngân 3", role: "cashier", branchId: 1, email: null, pin: "1234" });
  });

  it("resets a PIN (4–6 digits)", async () => {
    mockApi({ "GET /branches": { body: allBranches }, "GET /users": { body: users }, "POST /users/4/reset-pin": { status: 204 } });
    renderAdmin(<StaffPage />, ownerMe);
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Đặt lại PIN Thu ngân 1" }));
    const input = screen.getByLabelText("PIN mới cho Thu ngân 1");
    await user.type(input, "12a");
    expect(input).toHaveValue("12");
    expect(screen.getByRole("button", { name: "Lưu" })).toBeDisabled();
    await user.type(input, "3456");
    await user.click(screen.getByRole("button", { name: "Lưu" }));
    await waitFor(() => expect(callsTo("POST", "/users/4/reset-pin")[0]?.body).toEqual({ pin: "123456" }));
  });
});

describe("screen 36: audit log", () => {
  it("lists entries with actor, approver and before/after, filters by action", async () => {
    const log: Schemas["AuditLogList"] = {
      total: 1,
      items: [
        {
          id: 1,
          at: "2026-11-19T01:00:00Z",
          actor: { id: 4, name: "Thu ngân 1" },
          approver: { id: 3, name: "Quản lý CN1" },
          branchId: 1,
          action: "order.discount",
          entityType: "order",
          entityId: "CN1-20261119-0042",
          before: null,
          after: { type: "percent", value: 25, amount: 21500 },
          note: "Khách quen",
        },
      ],
    };
    mockApi({ "GET /branches": { body: allBranches }, "GET /audit-logs": { body: log } });
    renderAdmin(<AuditLogPage />, ownerMe);
    const table = await screen.findByRole("table", { name: "Nhật ký thao tác" });
    expect(table).toHaveTextContent("Giảm giá");
    expect(table).toHaveTextContent("Quản lý CN1");
    expect(table).toHaveTextContent("amount: 21.500đ");
    expect(table).toHaveTextContent("08:00");
    fireEvent.change(screen.getByLabelText("Thao tác"), { target: { value: "order.refund" } });
    await waitFor(() => expect(callsTo("GET", "/audit-logs").at(-1)?.query.action).toBe("order.refund"));
  });

  it("describes values", () => {
    expect(describeValue({ price: 29000 })).toBe("price: 29.000đ");
    expect(describeValue(null)).toBe("–");
    expect(describeValue(25)).toBe("25");
  });
});
