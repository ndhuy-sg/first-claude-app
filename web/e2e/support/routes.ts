/**
 * Every URL the e2e flows navigate to, in ONE place.
 *
 * The POS and admin screens are built in parallel by other agents and their slugs are not final.
 * After merging, adjust these constants (and nothing else) if a screen lives elsewhere.
 * Specs locate elements by role and Vietnamese label (docs/SCREENS.md and the design canvas),
 * so most flows only need the entry URLs below.
 */
export const ROUTES = {
  /** POS entry: shows "Kích hoạt máy POS" without a device cookie, the PIN screen without a session,
   *  "Mở ca" without an open shift, otherwise the floor plan ("Sơ đồ bàn"). */
  pos: "/pos",
  /** PIN login screen ("Chọn nhân viên", "Nhập mã PIN 4–6 số để vào ca"). */
  posPin: "/pos",
  /** Floor plan. */
  posTables: "/pos",
  /** Close shift ("Chốt ca"). Reached from the floor plan button "Chốt ca" too. */
  posShiftClose: "/pos/chot-ca",

  adminLogin: "/admin/dang-nhap",
  adminHome: "/admin",
  /** "Món & giá" list. */
  adminMenu: "/admin/mon",
  /** "Nhân viên & phân quyền". */
  adminStaff: "/admin/nhan-vien",
  /** "Báo cáo bán hàng". */
  adminReports: "/admin/bao-cao",
  /** "Hoá đơn điện tử". */
  adminInvoices: "/admin/hoa-don",

  /** Public invoice lookup for customers (no login). */
  lookup: (code: string) => `/tra-cuu/${encodeURIComponent(code)}`,
} as const;

/** API prefix. In e2e the browser and API helpers both go through Next.js (:3000), which rewrites
 *  /api/* to the Go API, so cookies are first-party exactly like production behind Cloudflare. */
export const API_PREFIX = "/api/v1";
