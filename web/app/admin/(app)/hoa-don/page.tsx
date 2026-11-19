import type { Metadata } from "next";
import { InvoicesPage } from "@/components/admin/InvoicesPage";

export const metadata: Metadata = { title: "Hoá đơn điện tử" };

export default function Page() {
  return <InvoicesPage />;
}
