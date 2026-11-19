import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { branchCN1, deviceInfo, invoiceSummary, meLan, order128k, payment, shiftOpen } from "@/lib/pos/test-fixtures";
import { mockApi, renderPos } from "@/lib/pos/test-utils";
import { PosSessionProvider } from "./PosFrame";
import { ShiftOrdersScreen } from "./ShiftOrdersScreen";

vi.mock("@/lib/api/client", async (orig) => ({ ...(await orig<object>()), api: (await import("@/lib/pos/test-utils")).mockApi.api }));
vi.mock("next/navigation", async () => {
  const { routerMock } = await import("@/lib/pos/test-utils");
  return { useRouter: () => routerMock };
});
vi.mock("next/link", () => ({
  default: ({ href, children, ...rest }: { href: string; children: ReactNode }) => (
    <a href={href} {...rest}>
      {children}
    </a>
  ),
}));

const big: Schemas["Order"] = order128k({
  id: 43,
  code: "CN1-20261119-0043",
  subtotal: 1_250_000,
  total: 1_250_000,
  paidAmount: 1_250_000,
  status: "paid",
  createdAt: "2026-11-19T07:05:00Z", // 14:05 in Vietnam
  paidAt: "2026-11-19T07:20:00Z",
  payments: [payment({ amount: 1_250_000, cashReceived: 1_300_000, changeAmount: 50_000 })],
  invoice: invoiceSummary,
});

afterEach(cleanup);
beforeEach(() => {
  mockApi.reset();
  vi.clearAllMocks();
  mockApi.on("GET", "/device", deviceInfo);
  mockApi.on("GET", "/orders", ({ params }: { params?: { query?: Record<string, unknown> } }) => {
    const items = params?.query?.status === "open" ? [order128k()] : [big, order128k()];
    return { items, total: items.length };
  });
});

const renderScreen = () =>
  renderPos(
    <PosSessionProvider value={{ me: meLan, shift: shiftOpen, branch: branchCN1, lock: () => {} }}>
      <ShiftOrdersScreen />
    </PosSessionProvider>,
  );

describe("orders in shift (screen 11)", () => {
  it("W-15: money as 1.250.000đ and 24-hour times in the list and detail", async () => {
    renderScreen();
    const list = await screen.findByRole("list", { name: "Đơn trong ca" });
    expect(within(list).getByText("1.250.000đ")).toBeInTheDocument();
    expect(within(list).getByText(/14:05/)).toBeInTheDocument();
    const detail = screen.getByRole("region", { name: "Chi tiết đơn" });
    expect(within(detail).getByText(/thanh toán lúc 14:20/)).toBeInTheDocument();
    expect(within(detail).getByText(/khách đưa 1\.300\.000đ, thối 50\.000đ/)).toBeInTheDocument();
    expect(mockApi.callsTo("GET", "/orders")[0].init.params?.query).toMatchObject({ shiftId: 7 });
  });

  it("filters by status and shows the invoice QR", async () => {
    const user = userEvent.setup();
    renderScreen();
    await screen.findByRole("list", { name: "Đơn trong ca" });
    await user.click(screen.getByRole("button", { name: "Hiện QR hoá đơn" }));
    expect(screen.getByRole("dialog", { name: "Quét để nhận hoá đơn điện tử" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Đóng" }));

    await user.click(screen.getByRole("button", { name: "Đang mở" }));
    await waitFor(() => expect(mockApi.callsTo("GET", "/orders").at(-1)?.init.params?.query).toMatchObject({ status: "open" }));
    expect(await screen.findByRole("link", { name: "Mở đơn" })).toHaveAttribute("href", "/pos/don/42");
  });

  it("refund by a cashier asks for the manager PIN", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/orders/{id}/refund", () => ({ ...big, status: "refunded" }));
    renderScreen();
    await user.click(await screen.findByRole("button", { name: "Hoàn tiền (cần quản lý)" }));
    const panel = screen.getByRole("region", { name: "Cần quản lý duyệt" });
    const pad = within(panel).getByRole("group", { name: "Bàn phím nhập PIN" });
    for (const d of "9999") await user.click(within(pad).getByRole("button", { name: d }));
    await user.click(within(panel).getByRole("button", { name: "Duyệt" }));
    await waitFor(() => expect(mockApi.callsTo("POST", "/orders/{id}/refund")).toHaveLength(1));
    expect(mockApi.callsTo("POST", "/orders/{id}/refund")[0].init.body).toEqual({ reason: "Khách trả món", managerId: 10, managerPin: "9999" });
  });
});
