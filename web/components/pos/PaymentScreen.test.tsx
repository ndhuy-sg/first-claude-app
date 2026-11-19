import { act, cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Schemas } from "@/lib/api/client";
import { resetConnectivity } from "@/lib/pos/connectivity";
import { branchCN1, invoiceSummary, meLan, order128k, payment, shiftOpen } from "@/lib/pos/test-fixtures";
import { HttpError, mockApi, renderPos, routerMock } from "@/lib/pos/test-utils";
import { DoneScreen } from "./DoneScreen";
import { PaymentScreen } from "./PaymentScreen";
import { PosSessionProvider } from "./PosFrame";

vi.mock("@/lib/api/client", async (orig) => ({ ...(await orig<object>()), api: (await import("@/lib/pos/test-utils")).mockApi.api }));
vi.mock("next/navigation", async () => {
  const { routerMock } = await import("@/lib/pos/test-utils");
  return { useRouter: () => routerMock };
});

function Session({ children }: { children: ReactNode }) {
  return <PosSessionProvider value={{ me: meLan, shift: shiftOpen, branch: branchCN1, lock: () => {} }}>{children}</PosSessionProvider>;
}
const renderPay = () =>
  renderPos(
    <Session>
      <PaymentScreen orderId={42} />
    </Session>,
  );

const QR_PAYLOAD = "00020101021238570010A00000072701270006970436011301234567890208QRIBFTTA53037045406128000";
let current: Schemas["Order"];

afterEach(() => {
  cleanup();
  resetConnectivity();
});
beforeEach(() => {
  mockApi.reset();
  vi.clearAllMocks();
  current = order128k();
  mockApi.on("GET", "/orders/{id}", () => current);
  mockApi.on("GET", "/payments/cash-suggestions", ({ params }: { params?: { query?: Record<string, unknown> } }) => {
    expect(params?.query?.amount).toBe(128000);
    return [128000, 150000, 200000, 500000];
  });
});

describe("payment screen", () => {
  it("W-07: cash suggestions, change, then the done screen shows the invoice QR", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/orders/{id}/payments", ({ body }: { body?: unknown }) => {
      const b = body as Schemas["PaymentRequest"];
      const p = payment({ amount: b.amount, cashReceived: b.cashReceived!, changeAmount: b.cashReceived! - b.amount });
      current = order128k({ status: "paid", paidAmount: 128000, payments: [p], invoice: invoiceSummary, paidAt: "2026-11-19T01:00:00Z" });
      return { payment: p, order: current };
    });
    renderPay();

    expect(await screen.findByTestId("amount-due")).toHaveTextContent("128.000đ");
    expect(screen.getByText("Giá đã gồm thuế GTGT 8% · Tiền thuế 9.481đ")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Còn thiếu 128.000đ" })).toBeDisabled();

    const suggestions = screen.getByLabelText("Gợi ý mệnh giá");
    expect(await within(suggestions).findByRole("button", { name: "200.000đ" })).toBeInTheDocument();
    expect(within(suggestions).getAllByRole("button").map((b) => b.textContent)).toEqual(["128.000đ", "150.000đ", "200.000đ", "500.000đ"]);

    // Typed amount below the total
    const pad = screen.getByRole("group", { name: "Bàn phím nhập tiền" });
    await user.click(within(pad).getByRole("button", { name: "1" }));
    await user.click(within(pad).getByRole("button", { name: "000" }));
    expect(screen.getByTestId("change-box")).toHaveTextContent("Còn thiếu");
    expect(screen.getByTestId("change-box")).toHaveTextContent("127.000đ");

    await user.click(within(suggestions).getByRole("button", { name: "200.000đ" }));
    expect(screen.getByTestId("change-box")).toHaveTextContent("Tiền thối");
    expect(screen.getByTestId("change-box")).toHaveTextContent("72.000đ");
    await user.click(screen.getByRole("button", { name: "Hoàn tất thanh toán" }));

    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/hoan-tat/42"));
    const call = mockApi.callsTo("POST", "/orders/{id}/payments")[0];
    expect(call.init.body).toEqual({ method: "cash", amount: 128000, cashReceived: 200000 });
    expect(call.init.headers?.["Idempotency-Key"]).toMatch(/[0-9a-f-]{36}/);
    expect(mockApi.callsTo("PUT", "/orders/{id}/buyer")).toHaveLength(0); // retail, nothing to save

    cleanup();
    renderPos(
      <Session>
        <DoneScreen orderId={42} />
      </Session>,
    );
    expect(await screen.findByRole("heading", { name: "Đã thanh toán 128.000đ" })).toBeInTheDocument();
    expect(screen.getByText("Bàn 5 · Tiền mặt · Thu ngân Lan")).toBeInTheDocument();
    expect(screen.getByTestId("done-given")).toHaveTextContent("200.000đ");
    expect(screen.getByTestId("done-change")).toHaveTextContent("72.000đ");
    expect(screen.getByText("Quét để nhận hoá đơn điện tử")).toBeInTheDocument();
    expect(screen.getByTitle("Mã QR tra cứu hoá đơn điện tử")).toBeInTheDocument();
    expect(screen.getByTestId("invoice-number")).toHaveTextContent("00000421");
    expect(screen.getByText("7K3P-9QXA")).toBeInTheDocument();
    expect(screen.getByText("Khách lẻ")).toBeInTheDocument();
  });

  it("W-08: VietQR shows the QR with the amount, Đã nhận tiền goes to the done screen", async () => {
    const user = userEvent.setup();
    const pending = payment({ id: 901, method: "vietqr", status: "pending", cashReceived: null, changeAmount: null, qrPayload: QR_PAYLOAD, confirmedAt: null });
    mockApi.on("POST", "/orders/{id}/payments", () => {
      current = order128k({ payments: [pending] });
      return { payment: pending, order: current };
    });
    mockApi.on("POST", "/payments/{id}/confirm", ({ params }: { params?: { path?: Record<string, unknown> } }) => {
      expect(params?.path?.id).toBe(901);
      const p = { ...pending, status: "confirmed" as const };
      current = order128k({ status: "paid", paidAmount: 128000, payments: [p], invoice: invoiceSummary });
      return { payment: p, order: current };
    });
    renderPay();

    await user.click(await screen.findByRole("tab", { name: "Chuyển khoản QR" }));
    const panel = await screen.findByTestId("qr-panel");
    expect(within(panel).getByTitle("Mã VietQR 128.000đ")).toBeInTheDocument();
    expect(within(panel).getByTestId("qr-amount")).toHaveTextContent("128.000đ");
    expect(within(panel).getByText("Vietcombank")).toBeInTheDocument();
    expect(within(panel).getByText("CN1-20261119-0042")).toBeInTheDocument();
    expect(within(panel).getByText("Đang chờ khách chuyển khoản…")).toBeInTheDocument();
    expect(mockApi.callsTo("POST", "/orders/{id}/payments")).toHaveLength(1);
    expect(mockApi.callsTo("POST", "/orders/{id}/payments")[0].init.body).toEqual({ method: "vietqr", amount: 128000 });

    await user.click(within(panel).getByRole("button", { name: "Đã nhận tiền" }));
    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/hoan-tat/42"));
  });

  it("W-08 / P-05: Huỷ QR cancels the pending payment and returns to cash", async () => {
    const user = userEvent.setup();
    const pending = payment({ id: 902, method: "vietqr", status: "pending", qrPayload: QR_PAYLOAD, cashReceived: null, changeAmount: null });
    current = order128k({ payments: [pending] });
    mockApi.on("POST", "/payments/{id}/cancel", () => {
      current = order128k({ payments: [{ ...pending, status: "cancelled" }] });
      return { payment: { ...pending, status: "cancelled" }, order: current };
    });
    renderPay();
    // an existing pending QR is shown again without creating a new one
    const panel = await screen.findByTestId("qr-panel");
    await user.click(within(panel).getByRole("button", { name: "Huỷ QR" }));
    expect(await screen.findByRole("tab", { name: "Tiền mặt", selected: true })).toBeInTheDocument();
    expect(mockApi.callsTo("POST", "/payments/{id}/cancel")).toHaveLength(1);
    expect(mockApi.callsTo("POST", "/orders/{id}/payments")).toHaveLength(0);
  });

  it("W-09: company invoice validates the tax code; all blank means Khách lẻ", async () => {
    const user = userEvent.setup();
    mockApi.on("PUT", "/orders/{id}/buyer", ({ body }: { body?: unknown }) => {
      current = order128k({ buyer: body as Schemas["Buyer"] });
      return current;
    });
    mockApi.on("POST", "/orders/{id}/payments", ({ body }: { body?: unknown }) => {
      const b = body as Schemas["PaymentRequest"];
      const p = payment({ amount: b.amount, cashReceived: b.cashReceived!, changeAmount: 0 });
      current = { ...current, status: "paid", paidAmount: b.amount, payments: [p], invoice: invoiceSummary };
      return { payment: p, order: current };
    });
    renderPay();
    await user.click(await screen.findByRole("radio", { name: "Công ty" }));
    await user.click(within(screen.getByLabelText("Gợi ý mệnh giá")).getByRole("button", { name: "128.000đ" }));

    await user.type(screen.getByRole("textbox", { name: /^Mã số thuế/ }), "12345");
    await user.type(screen.getByLabelText("Tên đơn vị"), "Công ty TNHH ABC");
    await user.click(screen.getByRole("button", { name: "Hoàn tất thanh toán" }));
    expect(await screen.findByText(/Mã số thuế gồm 10 số/)).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Mã số thuế/ })).toHaveAttribute("aria-invalid", "true");
    expect(mockApi.callsTo("POST", "/orders/{id}/payments")).toHaveLength(0);

    await user.clear(screen.getByRole("textbox", { name: /^Mã số thuế/ }));
    await user.type(screen.getByRole("textbox", { name: /^Mã số thuế/ }), "0101234567-001");
    await user.type(screen.getByLabelText("Email nhận hoá đơn"), "ketoan@abc.vn");
    await user.click(screen.getByRole("button", { name: "Hoàn tất thanh toán" }));
    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/hoan-tat/42"));
    expect(mockApi.callsTo("PUT", "/orders/{id}/buyer")[0].init.body).toEqual({
      taxCode: "0101234567-001",
      name: "Công ty TNHH ABC",
      address: "",
      email: "ketoan@abc.vn",
    });
  });

  it("W-09: company selected but left blank pays as Khách lẻ without a buyer", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/orders/{id}/payments", () => {
      current = { ...current, status: "paid" };
      return { payment: payment(), order: current };
    });
    renderPay();
    await user.click(await screen.findByRole("radio", { name: "Công ty" }));
    expect(screen.getByText("Bỏ trống tất cả thì hoá đơn ghi “Khách lẻ”.")).toBeInTheDocument();
    await user.click(within(screen.getByLabelText("Gợi ý mệnh giá")).getByRole("button", { name: "128.000đ" }));
    await user.click(screen.getByRole("button", { name: "Hoàn tất thanh toán" }));
    await waitFor(() => expect(routerMock.replace).toHaveBeenCalledWith("/pos/hoan-tat/42"));
    expect(mockApi.callsTo("PUT", "/orders/{id}/buyer")).toHaveLength(0);
  });

  it("shows the server error when payment is refused", async () => {
    const user = userEvent.setup();
    mockApi.on("POST", "/orders/{id}/payments", () => {
      throw new HttpError(409, "ORDER_NOT_OPEN", "Đơn đã thanh toán");
    });
    renderPay();
    await user.click(await within(await screen.findByLabelText("Gợi ý mệnh giá")).findByRole("button", { name: "128.000đ" }));
    await user.click(screen.getByRole("button", { name: "Hoàn tất thanh toán" }));
    expect(await screen.findByText("Đơn đã thanh toán")).toBeInTheDocument();
  });

  it("W-14: offline shows the banner state and blocks payment", async () => {
    const user = userEvent.setup();
    renderPay();
    await user.click(await within(await screen.findByLabelText("Gợi ý mệnh giá")).findByRole("button", { name: "200.000đ" }));
    expect(screen.getByRole("button", { name: "Hoàn tất thanh toán" })).toBeEnabled();
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(false);
    act(() => {
      window.dispatchEvent(new Event("offline"));
    });
    expect(screen.getByRole("button", { name: "Chờ kết nối để thanh toán" })).toBeDisabled();
    // the amount entered is kept
    expect(screen.getByTestId("cash-given")).toHaveTextContent("200.000đ");
    vi.spyOn(navigator, "onLine", "get").mockReturnValue(true);
    act(() => {
      window.dispatchEvent(new Event("online"));
    });
    expect(screen.getByRole("button", { name: "Hoàn tất thanh toán" })).toBeEnabled();
  });
});
