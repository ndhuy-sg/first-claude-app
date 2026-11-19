import { describe, expect, it } from "vitest";
import { formatVND } from "@/lib/format";
import {
  countedTotal,
  discountAmount,
  isValidTaxCode,
  itemDetail,
  keypadAppend,
  needsApproval,
  paymentMethodsLabel,
  shiftName,
  unitPrice,
  vatFromGross,
} from "./money";

const caPheSua = {
  basePrice: 29000,
  sizes: [
    { code: "M", name: "M", priceDelta: 0 },
    { code: "L", name: "L", priceDelta: 6000 },
  ],
  modifiers: [
    { id: 1, name: "Thêm shot", price: 8000 },
    { id: 2, name: "Ít đá", price: 0 },
  ],
};

describe("POS money helpers", () => {
  it("W-04: Cà phê sữa L + Thêm shot = 43.000đ", () => {
    expect(formatVND(unitPrice(caPheSua, "L", [1]))).toBe("43.000đ");
    expect(unitPrice(caPheSua, "M", [2])).toBe(29000);
  });

  it("discount preview rounds down and flags > 20% (D-03, D-05)", () => {
    expect(discountAmount(86000, "percent", 10)).toBe(8600);
    expect(discountAmount(86001, "percent", 10)).toBe(8600);
    expect(needsApproval(86000, "percent", 20, 20)).toBe(false);
    expect(needsApproval(86000, "percent", 25, 20)).toBe(true);
    expect(needsApproval(100000, "amount", 21000, 20)).toBe(true);
  });

  it("VAT split from VAT-inclusive totals (D-08)", () => {
    expect(vatFromGross(86000, 800)).toEqual({ beforeVat: 79630, vat: 6370 });
    expect(vatFromGross(86000, 1000)).toEqual({ beforeVat: 78182, vat: 7818 });
    expect(vatFromGross(128000, 800).vat).toBe(9481);
  });

  it("W-09: buyer tax code is 10 digits or 10-3", () => {
    expect(isValidTaxCode("0101234567")).toBe(true);
    expect(isValidTaxCode("0101234567-001")).toBe(true);
    expect(isValidTaxCode("12345")).toBe(false);
    expect(isValidTaxCode("abcdefghij")).toBe(false);
    expect(isValidTaxCode("0101234567-01")).toBe(false);
  });

  it("keypad input", () => {
    expect(keypadAppend("", "5")).toBe("5");
    expect(keypadAppend("5", "000")).toBe("5000");
    expect(keypadAppend("0", "0")).toBe("0");
    expect(keypadAppend("50", "⌫")).toBe("5");
  });

  it("denomination count and labels", () => {
    expect(countedTotal({ 500000: 2, 100000: 4, 5000: 3 })).toBe(1415000);
    expect(itemDetail({ sizeName: "L", modifiers: [{ id: 1, name: "Thêm shot", price: 8000 }] })).toBe("Size L · Thêm shot");
    expect(shiftName("2026-11-18T23:30:00Z")).toBe("Ca sáng"); // 06:30 VN
    expect(shiftName("2026-11-19T07:00:00Z")).toBe("Ca chiều"); // 14:00 VN
    const p = { id: 1, amount: 1, cashReceived: null, changeAmount: null, qrPayload: "", createdAt: "", confirmedAt: null };
    expect(paymentMethodsLabel([{ ...p, method: "cash", status: "confirmed" }])).toBe("Tiền mặt");
    expect(paymentMethodsLabel([{ ...p, method: "vietqr", status: "confirmed" }, { ...p, method: "cash", status: "cancelled" }])).toBe("Chuyển khoản QR");
  });
});
