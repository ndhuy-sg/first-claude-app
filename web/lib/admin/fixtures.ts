// Sample API payloads for admin tests, matching the seed data in docs/DEV.md.
import type { Schemas } from "@/lib/api/client";

export const ownerMe: Schemas["Me"] = {
  user: { id: 1, name: "Chủ chuỗi", role: "owner", branchId: null, pinMustChange: false },
  device: null,
  settings: { discountMaxPct: 20, posIdleLockMinutes: 15 },
};

export const ql1Me: Schemas["Me"] = {
  user: { id: 3, name: "Quản lý CN1", role: "branch_manager", branchId: 1, pinMustChange: false },
  device: null,
  settings: { discountMaxPct: 20, posIdleLockMinutes: 15 },
};

const vietqr = { bankBin: "970436", accountNo: "0123456789", accountName: "CONG TY MOC" };

export const cn1: Schemas["Branch"] = {
  id: 1,
  code: "CN1",
  name: "Mộc Nguyễn Huệ",
  address: "12 Nguyễn Huệ, Q1",
  openingHours: "07:00–22:00",
  taxCode: "0101234567",
  vatRateBp: 800,
  invoiceSymbol: "1C26TMC",
  nextInvoiceNumber: 42,
  vietqr,
  isActive: true,
};

export const cn2: Schemas["Branch"] = { ...cn1, id: 2, code: "CN2", name: "Mộc Lê Lợi", address: "5 Lê Lợi, Q1", invoiceSymbol: "1C26TMD", nextInvoiceNumber: 7 };

export const allBranches = [cn1, cn2];

export const settings: Schemas["Settings"] = {
  companyName: "Công ty TNHH Mộc",
  companyTaxCode: "0101234567",
  invoiceEmailFrom: "hoadon@moc.vn",
  taxSubmissionEnabled: false,
  taxGateway: "off",
  discountMaxPct: 20,
  posIdleLockMinutes: 15,
  loginMaxFailures: 5,
  loginLockMinutes: 15,
  shiftNoteThreshold: 50000,
  pricesIncludeVat: true,
};

export const caPheSua: Schemas["Product"] = {
  id: 10,
  categoryId: 1,
  name: "Cà phê sữa",
  description: "",
  basePrice: 29000,
  isActive: true,
  isFeatured: true,
  sortOrder: 1,
  sizes: [
    { code: "M", name: "M", priceDelta: 0 },
    { code: "L", name: "L", priceDelta: 6000 },
  ],
  modifiers: [
    { id: 1, name: "Thêm shot", price: 8000 },
    { id: 2, name: "Ít đá", price: 0 },
  ],
  soldOutBranchIds: [],
};

export const menu: Schemas["Menu"] = {
  categories: [
    { id: 1, name: "Cà phê", products: [] },
    { id: 2, name: "Trà", products: [] },
  ],
};

export const invoice: Schemas["Invoice"] = {
  id: 501,
  symbol: "1C26TMC",
  number: "00000421",
  lookupCode: "7K3P-9QXA",
  lookupUrl: "https://moc.vn/tra-cuu/7K3P-9QXA",
  status: "not_sent",
  kind: "normal",
  branchId: 1,
  orderId: 900,
  orderCode: "CN1-20261119-0042",
  seller: { name: "Công ty TNHH Mộc – CN1", taxCode: "0101234567", address: "12 Nguyễn Huệ, Q1" },
  buyer: {},
  paymentMethod: "cash",
  lines: [
    { lineNo: 1, name: "Cà phê sữa (L, thêm shot)", qty: 2, unitPrice: 43000, amount: 86000 },
  ],
  vatRateBp: 800,
  amountBeforeVat: 79630,
  vatAmount: 6370,
  total: 86000,
  totalInWords: "Tám mươi sáu nghìn đồng",
  issuedAt: "2026-11-19T00:12:00Z",
  sentAt: null,
  cancelled: false,
};
