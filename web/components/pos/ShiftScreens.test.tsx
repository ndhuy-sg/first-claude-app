import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { branchCN1, meLan, menu, shiftOpen } from "@/lib/pos/test-fixtures";
import { HttpError, mockApi, renderPos, routerMock } from "@/lib/pos/test-utils";
import { OpenShiftScreen } from "./OpenShiftScreen";
import { PosSessionProvider } from "./PosFrame";
import { ShiftCloseScreen } from "./ShiftCloseScreen";
import { SoldOutScreen } from "./SoldOutScreen";

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

function Session({ children, shift = shiftOpen, me = meLan }: { children: ReactNode; shift?: Schemas["Shift"] | null; me?: Schemas["Me"] }) {
  return <PosSessionProvider value={{ me, shift, branch: branchCN1, lock: () => {} }}>{children}</PosSessionProvider>;
}

const summary = (over: Partial<Schemas["ShiftSummary"]> = {}): Schemas["ShiftSummary"] => ({
  shift: shiftOpen,
  orderCount: 12,
  revenue: 1_850_000,
  cashSales: 1_250_000,
  qrSales: 600_000,
  cashRefunds: 0,
  expectedCash: 2_250_000,
  countedCash: null,
  cashDiff: null,
  openOrders: 0,
  pendingQrPayments: 0,
  invoicesNotSent: 3,
  ...over,
});

afterEach(cleanup);
beforeEach(() => {
  mockApi.reset();
  vi.clearAllMocks();
});

describe("W-10 open shift", () => {
  it("enters the opening cash on the keypad and opens the shift", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/shifts/open", ({ body }: { body?: unknown }) => ({ ...shiftOpen, openingCash: (body as { openingCash: number }).openingCash }));
    renderPos(
      <Session shift={null}>
        <OpenShiftScreen />
      </Session>,
    );
    const pad = screen.getByRole("group", { name: "Bàn phím nhập tiền" });
    await user.click(within(pad).getByRole("button", { name: "1" }));
    await user.click(within(pad).getByRole("button", { name: "000" }));
    await user.click(within(pad).getByRole("button", { name: "000" }));
    expect(screen.getByTestId("opening-cash")).toHaveTextContent("1.000.000đ");
    await user.click(screen.getByRole("button", { name: "Mở ca và bắt đầu bán" }));
    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/ban"));
    expect(mockApi.callsTo("POST", "/shifts/open")[0].init.body).toEqual({ openingCash: 1_000_000 });
  });
});

describe("W-10 shift close", () => {
  const renderClose = () =>
    renderPos(
      <Session>
        <ShiftCloseScreen />
      </Session>,
    );

  it("counts cash by denomination and shows the difference; note required above 50.000đ", async () => {
    const user = userEvent.setup();
    let closed = false;
    mockApi.on("GET", "/shifts/{id}/summary", () => summary());
    mockApi.on("POST", "/shifts/{id}/close", ({ body }: { body?: unknown }) => {
      const b = body as { countedCash: number; note?: string };
      closed = true;
      return summary({
        shift: { ...shiftOpen, status: "closed", closedAt: "2026-11-19T07:00:00Z" },
        countedCash: b.countedCash,
        cashDiff: b.countedCash - 2_250_000,
        note: b.note,
      });
    });
    renderClose();

    expect(await screen.findByTestId("kpi-revenue")).toHaveTextContent("1.850.000đ");
    expect(screen.getByTestId("expected-cash")).toHaveTextContent("2.250.000đ");
    expect(screen.getByText("Không còn đơn chưa thanh toán")).toBeInTheDocument();
    expect(screen.getByText(/3 hoá đơn chưa gửi cơ quan thuế/)).toBeInTheDocument();

    // 4 × 500k + 2 × 100k = 2.200.000 → −50.000đ (not above the threshold)
    const add = (d: string) => user.click(screen.getByRole("button", { name: `Thêm một tờ ${d}` }));
    for (let i = 0; i < 4; i++) await add("500.000đ");
    for (let i = 0; i < 2; i++) await add("100.000đ");
    expect(screen.getByTestId("counted-total")).toHaveTextContent("2.200.000đ");
    const diff = screen.getByTestId("cash-diff");
    expect(diff).toHaveTextContent("−50.000đ");
    expect(diff).toHaveClass("text-bad");
    expect(screen.getByText("Thiếu tiền")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chốt ca" })).toBeEnabled();

    // remove one 100k → −150.000đ, note becomes mandatory
    await user.click(screen.getByRole("button", { name: "Bớt một tờ 100.000đ" }));
    expect(screen.getByTestId("cash-diff")).toHaveTextContent("−150.000đ");
    expect(screen.getByText(/bắt buộc vì lệch trên 50.000đ/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chốt ca" })).toBeDisabled();
    await user.type(screen.getByPlaceholderText("Lý do chênh lệch"), "Thối nhầm cho khách");
    await user.click(screen.getByRole("button", { name: "Chốt ca" }));

    await waitFor(() => expect(closed).toBe(true));
    expect(mockApi.callsTo("POST", "/shifts/{id}/close")[0].init.body).toEqual({ countedCash: 2_100_000, note: "Thối nhầm cho khách" });
    expect(await screen.findByRole("heading", { name: /Đã chốt ca/ })).toBeInTheDocument();
    expect(screen.getByTestId("cash-diff")).toHaveTextContent("−150.000đ");
    expect(screen.getByRole("button", { name: "Đăng xuất" })).toBeInTheDocument();
  });

  it("blocks closing while orders are open and lists them on 409", async () => {
    mockApi.on("GET", "/shifts/{id}/summary", () => summary({ openOrders: 2 }));
    renderClose();
    expect(await screen.findByText("Còn 2 đơn chưa thanh toán")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Chốt ca" })).toBeDisabled();
  });

  it("shows OPEN_ORDERS details returned by the server", async () => {
    const user = userEvent.setup();
    mockApi.on("GET", "/shifts/{id}/summary", () => summary({ expectedCash: 0 }));
    mockApi.on("POST", "/shifts/{id}/close", () => {
      throw new HttpError(409, "OPEN_ORDERS", "Còn đơn chưa thanh toán", { orders: [{ id: 55, code: "CN1-20261119-0055" }] });
    });
    renderClose();
    await user.click(await screen.findByRole("button", { name: "Chốt ca" }));
    expect(await screen.findByRole("link", { name: "CN1-20261119-0055" })).toHaveAttribute("href", "/pos/don/55");
    expect(screen.getByText("Còn đơn chưa thanh toán")).toBeInTheDocument();
  });
});

describe("sold-out switches (screen 12)", () => {
  it("a manager toggles a product; a cashier only sees the state", async () => {
    const user = userEvent.setup();
    mockApi.on("GET", "/menu", menu).on("PUT", "/branches/{branchId}/sold-out/{productId}", undefined);
    const manager: Schemas["Me"] = { ...meLan, user: { ...meLan.user, id: 10, name: "Tuấn", role: "branch_manager" } };
    renderPos(
      <Session me={manager}>
        <SoldOutScreen />
      </Session>,
    );
    const sw = await screen.findByRole("switch", { name: "Cà phê sữa: Đang bán" });
    expect(sw).toHaveAttribute("aria-checked", "true");
    expect(screen.getByRole("switch", { name: "Bạc xỉu: Hết" })).toHaveAttribute("aria-checked", "false");
    await user.click(sw);
    await waitFor(() => expect(mockApi.callsTo("PUT", "/branches/{branchId}/sold-out/{productId}")).toHaveLength(1));
    expect(mockApi.callsTo("PUT", "/branches/{branchId}/sold-out/{productId}")[0].init).toMatchObject({ params: { path: { branchId: 1, productId: 2 } }, body: { soldOut: true } });

    cleanup();
    renderPos(
      <Session>
        <SoldOutScreen />
      </Session>,
    );
    expect(await screen.findByRole("switch", { name: "Cà phê sữa: Đang bán" })).toBeDisabled();
  });
});
