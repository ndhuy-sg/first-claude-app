// Sample API data for POS component tests (mirrors the seed: CN1, Lan / tn1, Tuấn / ql1, Cà phê sữa).
import type { Schemas } from "@/lib/api/client";

export const deviceCN1: Schemas["Device"] = { id: 1, name: "iPad quầy 1", branchId: 1, branchCode: "CN1", lastSeenAt: null, revoked: false };

export const deviceInfo: Schemas["DeviceInfo"] = {
  device: deviceCN1,
  staff: [
    { id: 11, name: "Lan", role: "cashier" },
    { id: 12, name: "Minh", role: "barista" },
    { id: 10, name: "Tuấn", role: "branch_manager" },
  ],
};

export const meLan: Schemas["Me"] = {
  user: { id: 11, name: "Lan", role: "cashier", branchId: 1, pinMustChange: false },
  device: deviceCN1,
  settings: { discountMaxPct: 20, posIdleLockMinutes: 15 },
};

export const branchCN1: Schemas["Branch"] = {
  id: 1,
  code: "CN1",
  name: "Mộc – CN1",
  address: "Địa chỉ CN1",
  openingHours: "06:30 – 22:00",
  taxCode: "",
  vatRateBp: 800,
  invoiceSymbol: "1C26MAA",
  nextInvoiceNumber: 1,
  vietqr: { bankBin: "970436", accountNo: "0123456789", accountName: "CONG TY MOC" },
  isActive: true,
};

export const shiftOpen: Schemas["Shift"] = {
  id: 7,
  branchId: 1,
  deviceId: 1,
  status: "open",
  openedBy: { id: 11, name: "Lan" },
  openedAt: "2026-11-18T23:30:00Z",
  openingCash: 1_000_000,
  closedAt: null,
};

const cfMods = [
  { id: 101, name: "Thêm shot", price: 8000 },
  { id: 102, name: "Ít đá", price: 0 },
];
const sizes = [
  { code: "M", name: "M", priceDelta: 0 },
  { code: "L", name: "L", priceDelta: 6000 },
];

export const menu: Schemas["Menu"] = {
  categories: [
    {
      id: 1,
      name: "Cà phê",
      products: [
        { id: 1, name: "Cà phê đen", description: "", basePrice: 25000, isFeatured: false, soldOut: false, sizes, modifiers: cfMods },
        { id: 2, name: "Cà phê sữa", description: "", basePrice: 29000, isFeatured: true, soldOut: false, sizes, modifiers: cfMods },
        { id: 3, name: "Bạc xỉu", description: "", basePrice: 32000, isFeatured: true, soldOut: true, sizes, modifiers: cfMods },
      ],
    },
    {
      id: 4,
      name: "Bánh",
      products: [{ id: 20, name: "Croissant bơ", description: "", basePrice: 35000, isFeatured: false, soldOut: false, sizes: [], modifiers: [] }],
    },
  ],
};

export function makeOrder(over: Partial<Schemas["Order"]> = {}): Schemas["Order"] {
  return {
    id: 42,
    code: "CN1-20261119-0042",
    branchId: 1,
    type: "dine_in",
    tableId: 5,
    tableName: "Bàn 5",
    status: "open",
    items: [],
    subtotal: 0,
    discount: null,
    total: 0,
    paidAmount: 0,
    payments: [],
    buyer: {},
    invoice: null,
    createdBy: { id: 11, name: "Lan" },
    createdAt: "2026-11-19T00:00:00Z",
    paidAt: null,
    ...over,
  };
}

export const lineCaPheSuaL: Schemas["OrderItem"] = {
  id: 501,
  productId: 2,
  name: "Cà phê sữa",
  sizeCode: "L",
  sizeName: "L",
  modifiers: [{ id: 101, name: "Thêm shot", price: 8000 }],
  qty: 1,
  unitPrice: 43000,
  lineTotal: 43000,
  note: "",
};

/** Order of 128.000đ (design sample): 2 × 43.000 + 42.000. */
export function order128k(over: Partial<Schemas["Order"]> = {}): Schemas["Order"] {
  return makeOrder({
    items: [
      { ...lineCaPheSuaL, qty: 2, lineTotal: 86000 },
      { id: 502, productId: 30, name: "Trà đào cam sả", sizeCode: "M", sizeName: "M", modifiers: [], qty: 1, unitPrice: 42000, lineTotal: 42000, note: "" },
    ],
    subtotal: 128000,
    total: 128000,
    ...over,
  });
}

export function payment(over: Partial<Schemas["Payment"]> = {}): Schemas["Payment"] {
  return {
    id: 900,
    method: "cash",
    status: "confirmed",
    amount: 128000,
    cashReceived: 200000,
    changeAmount: 72000,
    qrPayload: "",
    createdAt: "2026-11-19T01:00:00Z",
    confirmedAt: "2026-11-19T01:00:00Z",
    ...over,
  };
}

export const invoiceSummary: Schemas["InvoiceSummary"] = {
  id: 77,
  symbol: "1C26MAA",
  number: "00000421",
  lookupCode: "7K3P-9QXA",
  lookupUrl: "https://moc.example/tra-cuu/7K3P-9QXA",
  status: "not_sent",
  kind: "normal",
};
