/**
 * Seed data used by e2e (mirrors api/internal/seed/seed.go and docs/DEV.md "Seed accounts").
 */
export const WEB_ORIGIN = process.env.E2E_WEB_ORIGIN ?? "http://localhost:3000";

export const DEVICE_COOKIE = "moc_device";
export const SESSION_COOKIE = "moc_session";

/** Fixed POS device tokens from the seed (api/internal/seed: DeviceCN1Token / DeviceCN2Token). */
export const DEVICE_TOKENS = {
  CN1: "dev-device-cn1-token-0000000000000000000000",
  CN2: "dev-device-cn2-token-0000000000000000000000",
} as const;

export const PASSWORD = "matkhau123";

/** name = display name in the staff picker of the PIN screen. */
export const ACCOUNTS = {
  owner: { email: "owner@moc.vn", password: PASSWORD, name: "Chủ chuỗi" },
  kho: { email: "kho@moc.vn", password: PASSWORD, name: "Quản lý kho tổng" },
  ql1: { email: "ql1@moc.vn", password: PASSWORD, pin: "9999", name: "Tuấn", branch: "CN1" },
  tn1: { pin: "1234", name: "Lan", branch: "CN1" },
  pc1: { pin: "4321", name: "Minh", branch: "CN1" },
  ql2: { email: "ql2@moc.vn", password: PASSWORD, pin: "8888", name: "Quản lý CN2", branch: "CN2" },
  tn2: { pin: "5678", name: "Nam", branch: "CN2" },
} as const;

/** Menu facts from the seed (docs/test-cases/phase-1-pos.md). */
export const MENU = {
  caPheSua: { name: "Cà phê sữa", basePrice: 29000, sizeL: 6000, themShot: 8000 },
  bacXiu: { name: "Bạc xỉu", basePrice: 32000 },
  croissant: { name: "Croissant bơ", basePrice: 35000 },
} as const;

/** Vietnamese money format used by the UI: 1.250.000đ (W-15). */
export function vnd(n: number): string {
  return `${Math.round(n).toLocaleString("de-DE")}đ`;
}

/** Today's business date in Asia/Ho_Chi_Minh as YYYY-MM-DD (reports use VN dates). */
export function vnToday(): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Ho_Chi_Minh" }).format(new Date());
}
