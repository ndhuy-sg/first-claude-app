import { apiError, callsTo, mockApi, renderWithClient } from "@/lib/admin/test-utils";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { invoice } from "@/lib/admin/fixtures";
import { InvoiceView } from "./[code]/InvoiceView";
import { LookupForm, normalizeCode } from "./LookupForm";

const push = vi.fn();
let code = "7K3P-9QXA";
vi.mock("next/navigation", () => ({
  useRouter: () => ({ push, replace: vi.fn() }),
  useParams: () => ({ code }),
}));
beforeEach(() => push.mockReset());

describe("E-11 / screens 18-19: public invoice lookup", () => {
  it("lookup form normalises the code and opens the invoice page", async () => {
    renderWithClient(<LookupForm />);
    const user = userEvent.setup();
    await user.click(screen.getByRole("button", { name: "Xem hoá đơn" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Nhập mã tra cứu");
    // Field renders the error inside its <label>, so match the label text loosely
    await user.type(screen.getByLabelText(/Mã tra cứu/), " 7k3p-9qxa ");
    await user.click(screen.getByRole("button", { name: "Xem hoá đơn" }));
    expect(push).toHaveBeenCalledWith("/tra-cuu/7K3P-9QXA");
    expect(normalizeCode("ab cd")).toBe("AB-CD");
  });

  it("renders the invoice without login", async () => {
    code = "7K3P-9QXA";
    mockApi({ "GET /public/invoices/7K3P-9QXA": { body: invoice } });
    renderWithClient(<InvoiceView />);
    const doc = await screen.findByRole("article", { name: "Hoá đơn số 00000421" });
    expect(doc).toHaveTextContent("Công ty TNHH Mộc – CN1");
    expect(doc).toHaveTextContent("Đã phát hành");
    expect(doc).toHaveTextContent("1C26TMC");
    expect(doc).toHaveTextContent("19/11/2026 07:12");
    expect(doc).toHaveTextContent("Khách lẻ");
    expect(doc).toHaveTextContent("Cà phê sữa (L, thêm shot)");
    expect(doc).toHaveTextContent("Tiền trước thuế79.630đ");
    expect(doc).toHaveTextContent("Thuế GTGT 8%6.370đ");
    expect(doc).toHaveTextContent("Tổng thanh toán86.000đ");
    expect(doc).toHaveTextContent("Bằng chữ: Tám mươi sáu nghìn đồng");
    expect(screen.getByRole("button", { name: "Tải PDF" })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Gửi qua email" }).getAttribute("href")).toContain("tra-cuu%2F7K3P-9QXA");
    expect(callsTo("GET", "/public/invoices/7K3P-9QXA")).toHaveLength(1);
  });

  it("prints when Tải PDF is pressed", async () => {
    code = "7K3P-9QXA";
    const print = vi.spyOn(window, "print").mockImplementation(() => undefined);
    mockApi({ "GET /public/invoices/7K3P-9QXA": { body: invoice } });
    renderWithClient(<InvoiceView />);
    await userEvent.click(await screen.findByRole("button", { name: "Tải PDF" }));
    expect(print).toHaveBeenCalled();
  });

  it("shows a friendly message for an unknown code (404)", async () => {
    code = "SAI-MA";
    mockApi({ "GET /public/invoices/SAI-MA": apiError(404, "NOT_FOUND", "Không tìm thấy") });
    renderWithClient(<InvoiceView />);
    expect(await screen.findByRole("heading", { name: "Không tìm thấy hoá đơn" })).toBeInTheDocument();
    expect(screen.getByText(/Mã “SAI-MA” không đúng/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Nhập mã khác" })).toHaveAttribute("href", "/tra-cuu");
    expect(callsTo("GET", "/public/invoices/SAI-MA")).toHaveLength(1);
  });
});
