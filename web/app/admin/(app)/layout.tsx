import type { Metadata } from "next";
import { AdminShell } from "@/components/admin/AdminShell";
import { Loading } from "@/components/admin/parts";
import { ClientOnly } from "@/components/ui/ClientOnly";

export const metadata: Metadata = { title: { default: "Quản trị", template: "%s – Quản trị Mộc" } };

export default function AdminLayout({ children }: { children: React.ReactNode }) {
  return (
    <ClientOnly reason="Admin pages depend on the signed-in user" fallback={<Loading label="Đang tải trang quản trị…" />}>
      <AdminShell>{children}</AdminShell>
    </ClientOnly>
  );
}
