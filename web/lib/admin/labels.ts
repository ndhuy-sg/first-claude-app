import type { Schemas } from "@/lib/api/client";
import type { Role } from "./permissions";

export type Tone = "ok" | "warn" | "bad" | "info" | "navy";

export const roleLabels: Record<Role, string> = {
  owner: "Chủ chuỗi",
  warehouse_manager: "Quản lý kho tổng",
  branch_manager: "Quản lý chi nhánh",
  cashier: "Thu ngân",
  barista: "Pha chế",
};

export const orderStatus: Record<Schemas["OrderStatus"], { label: string; tone: Tone }> = {
  open: { label: "Đang mở", tone: "warn" },
  paid: { label: "Đã thanh toán", tone: "ok" },
  cancelled: { label: "Đã huỷ", tone: "bad" },
  merged: { label: "Đã gộp", tone: "info" },
  refunded: { label: "Đã hoàn tiền", tone: "bad" },
};

export const invoiceStatus: Record<Schemas["InvoiceStatus"], { label: string; tone: Tone }> = {
  not_sent: { label: "Chưa gửi", tone: "info" },
  sending: { label: "Đang gửi", tone: "warn" },
  sent: { label: "Đã gửi", tone: "ok" },
  failed: { label: "Thất bại", tone: "bad" },
};

export const invoiceKind: Record<Schemas["InvoiceSummary"]["kind"], string> = {
  normal: "Hoá đơn",
  cancel: "Hoá đơn huỷ",
  adjust: "Hoá đơn điều chỉnh",
};

const paymentMethods: Record<string, string> = { cash: "Tiền mặt", vietqr: "Chuyển khoản VietQR", mixed: "Kết hợp" };

export function paymentMethodLabel(method: string): string {
  return paymentMethods[method] ?? method;
}

/** 800 basis points → "8%", 1050 → "10,5%" */
export function formatVatRate(bp: number): string {
  const pct = bp / 100;
  return `${String(pct).replace(".", ",")}%`;
}

export const auditActions: Record<string, string> = {
  "order.discount": "Giảm giá",
  "order.cancel": "Huỷ đơn",
  "order.refund": "Hoàn tiền",
  "product.price_change": "Đổi giá món",
  "product.update": "Sửa món",
  "product.create": "Tạo món",
  "product.sold_out": "Báo hết món",
  "user.lock": "Khoá nhân viên",
  "user.unlock": "Mở khoá nhân viên",
  "user.create": "Tạo nhân viên",
  "user.reset_pin": "Đặt lại PIN",
  "settings.update": "Đổi cài đặt",
  "branch.update": "Sửa chi nhánh",
  "device.revoke": "Thu hồi máy POS",
  "invoice.resend": "Gửi lại hoá đơn",
};

export function auditActionLabel(action: string): string {
  return auditActions[action] ?? action;
}

export const alertTone: Record<Schemas["Dashboard"]["alerts"][number]["severity"], { label: string; tone: Tone }> = {
  info: { label: "Thông tin", tone: "info" },
  warning: { label: "Cần chú ý", tone: "warn" },
  critical: { label: "Khẩn", tone: "bad" },
};
