import { can, type AdminAction, type Role } from "./permissions";

export type NavItem = {
  label: string;
  href: string;
  /** Phase tag for screens that are not built yet ("GĐ2", "GĐ3"): rendered disabled. */
  phase?: string;
  action?: AdminAction;
};

export type NavGroup = { title: string; items: NavItem[] };

export const navGroups: NavGroup[] = [
  {
    title: "BÁN HÀNG",
    items: [
      { label: "Tổng quan", href: "/admin", action: "view_sales_report" },
      { label: "Báo cáo bán hàng", href: "/admin/bao-cao", action: "view_sales_report" },
      { label: "Đơn hàng", href: "/admin/don-hang", action: "view_orders" },
      { label: "Hoá đơn điện tử", href: "/admin/hoa-don", action: "view_invoices" },
      { label: "Khuyến mãi", href: "/admin/khuyen-mai", phase: "GĐ3" },
    ],
  },
  {
    title: "MENU",
    items: [
      { label: "Món & giá", href: "/admin/mon", action: "sold_out" },
      { label: "Công thức món", href: "/admin/cong-thuc", phase: "GĐ2" },
    ],
  },
  {
    title: "KHO",
    items: [
      { label: "Tồn kho", href: "/admin/ton-kho", phase: "GĐ2" },
      { label: "Nhập hàng", href: "/admin/nhap-hang", phase: "GĐ2" },
      { label: "Chuyển kho", href: "/admin/chuyen-kho", phase: "GĐ2" },
      { label: "Sản xuất", href: "/admin/san-xuat", phase: "GĐ2" },
      { label: "Kiểm kê & huỷ", href: "/admin/kiem-ke", phase: "GĐ2" },
      { label: "Nhà cung cấp", href: "/admin/nha-cung-cap", phase: "GĐ2" },
    ],
  },
  {
    title: "HỆ THỐNG",
    items: [
      { label: "Chi nhánh & máy POS", href: "/admin/chi-nhanh", action: "manage_tables" },
      { label: "Nhân viên", href: "/admin/nhan-vien", action: "manage_staff" },
      { label: "Nhật ký thao tác", href: "/admin/nhat-ky", action: "view_audit" },
      { label: "Cài đặt", href: "/admin/cai-dat" },
    ],
  },
];

/** Items visible for a role: phase-tagged items are always shown (disabled); others need their action. */
export function visibleNav(role: Role): NavGroup[] {
  return navGroups
    .map((g) => ({ ...g, items: g.items.filter((i) => i.phase || !i.action || can(role, i.action) !== null) }))
    .filter((g) => g.items.length > 0);
}

export function isActive(href: string, pathname: string): boolean {
  if (href === "/admin") return pathname === "/admin" || pathname === "/admin/";
  return pathname === href || pathname.startsWith(href + "/");
}

/**
 * Where a dashboard alert link should go. Phase 1 has no admin shift page, so
 * "/admin/ca/{shiftId}" opens the orders of that shift. Links outside /admin are not followed.
 */
export function alertHref(link: string | null): string | null {
  if (!link || !link.startsWith("/admin")) return null;
  const shift = /^\/admin\/ca\/(\d+)\/?$/.exec(link);
  if (shift) return `/admin/don-hang?shiftId=${shift[1]}`;
  return link;
}

/** First page a role can open after login. */
export function homeFor(role: Role): string {
  for (const g of visibleNav(role)) for (const i of g.items) if (!i.phase) return i.href;
  return "/admin/cai-dat";
}
