import type { Metadata } from "next";
import { AuditLogPage } from "@/components/admin/AuditLogPage";

export const metadata: Metadata = { title: "Nhật ký thao tác" };

export default function Page() {
  return <AuditLogPage />;
}
