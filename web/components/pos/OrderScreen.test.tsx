import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { resetConnectivity } from "@/lib/pos/connectivity";
import { branchCN1, deviceInfo, lineCaPheSuaL, makeOrder, meLan, menu, order128k, shiftOpen } from "@/lib/pos/test-fixtures";
import { HttpError, mockApi, renderPos } from "@/lib/pos/test-utils";
import { OrderScreen } from "./OrderScreen";
import { PosSessionProvider } from "./PosFrame";

vi.mock("@/lib/api/client", async (orig) => ({ ...(await orig<object>()), api: (await import("@/lib/pos/test-utils")).mockApi.api }));
vi.mock("next/navigation", async () => {
  const { routerMock } = await import("@/lib/pos/test-utils");
  return { useRouter: () => routerMock };
});

function Session({ children }: { children: ReactNode }) {
  return <PosSessionProvider value={{ me: meLan, shift: shiftOpen, branch: branchCN1, lock: () => {} }}>{children}</PosSessionProvider>;
}
const renderOrder = () =>
  renderPos(
    <Session>
      <OrderScreen orderId={42} />
    </Session>,
  );

afterEach(() => {
  cleanup();
  resetConnectivity();
});

beforeEach(() => {
  mockApi.reset();
  vi.clearAllMocks();
  mockApi.on("GET", "/menu", menu).on("GET", "/device", deviceInfo).on("GET", "/tables", []);
});

describe("ordering screen", () => {
  it("W-03: categories, product grid and cart are visible together", async () => {
    mockApi.on("GET", "/orders/{id}", makeOrder());
    renderOrder();
    expect(await screen.findByRole("navigation", { name: "Nhóm món" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /^Cà phê sữa · 29\.000đ/ })).toBeInTheDocument();
    expect(screen.getByRole("complementary", { name: "Đơn hàng" })).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Bàn 5" })).toBeInTheDocument();
  });

  it("W-04: Cà phê sữa, size L + Thêm shot shows 43.000đ and the cart line", async () => {
    const user = userEvent.setup();
    let current = makeOrder();
    mockApi.on("GET", "/orders/{id}", () => current);
    mockApi.on("POST", "/orders/{id}/items", () => {
      current = makeOrder({ items: [lineCaPheSuaL], subtotal: 43000, total: 43000 });
      return current;
    });
    renderOrder();

    await user.click(await screen.findByRole("button", { name: /^Cà phê sữa · 29\.000đ/ }));
    const dialog = screen.getByRole("dialog", { name: "Cà phê sữa" });
    expect(within(dialog).getByText("29.000đ")).toBeInTheDocument(); // Giá gốc
    expect(within(dialog).getByRole("button", { name: /Thêm vào đơn · 29\.000đ/ })).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: /^L \+6\.000đ/ }));
    const shot = within(dialog).getByRole("button", { name: /Thêm shot/ });
    expect(shot).toHaveAttribute("aria-pressed", "false");
    await user.click(shot);
    expect(shot).toHaveAttribute("aria-pressed", "true");
    await user.click(within(dialog).getByRole("button", { name: "Thêm vào đơn · 43.000đ" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(mockApi.callsTo("POST", "/orders/{id}/items")[0].init.body).toEqual({ productId: 2, sizeCode: "L", modifierIds: [101], qty: 1, note: undefined });
    const line = screen.getByTestId("cart-line");
    expect(line).toHaveTextContent("Cà phê sữa");
    expect(line).toHaveTextContent("Size L · Thêm shot");
    expect(within(line).getByTestId("line-total")).toHaveTextContent("43.000đ");
    expect(screen.getByTestId("cart-total")).toHaveTextContent("43.000đ");
  });

  it("W-05: a sold-out product is greyed out and cannot be tapped", async () => {
    const user = userEvent.setup();
    mockApi.on("GET", "/orders/{id}", makeOrder());
    renderOrder();
    const sold = await screen.findByRole("button", { name: /^Bạc xỉu · .*hết món/ });
    expect(sold).toBeDisabled();
    expect(within(sold).getByText("Hết món")).toBeInTheDocument();
    await user.click(sold);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("W-06: 25% discount asks for a manager PIN, then the total is updated", async () => {
    const user = userEvent.setup();
    let current: Schemas["Order"] = order128k();
    mockApi.on("GET", "/orders/{id}", () => current);
    mockApi.on("POST", "/orders/{id}/discount", ({ body }: { body?: unknown }) => {
      const b = body as Schemas["DiscountRequest"];
      if (b.value > 20 && b.managerPin !== "9999") throw new HttpError(403, "INVALID_MANAGER_PIN", "PIN sai");
      current = order128k({
        discount: { type: "percent", value: 25, amount: 32000, reason: b.reason, approvedBy: { id: 10, name: "Tuấn" } },
        total: 96000,
      });
      return current;
    });
    renderOrder();

    await user.click(await screen.findByRole("button", { name: "Giảm giá" }));
    const dialog = screen.getByRole("dialog", { name: "Giảm giá" });
    await user.click(within(dialog).getByRole("button", { name: "10%" }));
    expect(within(dialog).queryByRole("region", { name: "Cần quản lý duyệt" })).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "25%" }));
    expect(within(dialog).getByTestId("discount-preview")).toHaveTextContent("−32.000đ");

    const panel = within(dialog).getByRole("region", { name: "Cần quản lý duyệt" });
    expect(within(panel).getByRole("button", { name: "Tuấn" })).toHaveAttribute("aria-pressed", "true");
    const pad = within(panel).getByRole("group", { name: "Bàn phím nhập PIN" });
    // wrong PIN first
    for (const d of "1111") await user.click(within(pad).getByRole("button", { name: d }));
    await user.click(within(panel).getByRole("button", { name: "Duyệt" }));
    expect(await within(panel).findByText("PIN quản lý không đúng")).toBeInTheDocument();
    for (const d of "9999") await user.click(within(pad).getByRole("button", { name: d }));
    await user.click(within(panel).getByRole("button", { name: "Duyệt" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    const last = mockApi.callsTo("POST", "/orders/{id}/discount").at(-1)!;
    expect(last.init.body).toMatchObject({ type: "percent", value: 25, managerId: 10, managerPin: "9999" });
    expect(last.init.headers?.["Idempotency-Key"]).toBeTruthy();
    expect(screen.getByTestId("cart-total")).toHaveTextContent("96.000đ");
    expect(screen.getByTestId("cart-discount")).toHaveTextContent("−32.000đ");
  });

  it("W-14: offline greys the grid and blocks payment, items stay in the cart", async () => {
    mockApi.on("GET", "/orders/{id}", order128k());
    renderOrder();
    expect(await screen.findByRole("button", { name: "Thanh toán 128.000đ" })).toBeEnabled();
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    act(() => {
      window.dispatchEvent(new Event("offline"));
    });
    const pay = screen.getByRole("button", { name: "Chờ kết nối để thanh toán" });
    expect(pay).toBeDisabled();
    expect(screen.getByRole("button", { name: /^Cà phê sữa · 29\.000đ/ })).toBeDisabled();
    expect(screen.getAllByTestId("cart-line")).toHaveLength(2);

    vi.spyOn(navigator, "onLine", "get").mockReturnValue(true);
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(screen.getByRole("button", { name: "Thanh toán 128.000đ" })).toBeEnabled();
  });
});
