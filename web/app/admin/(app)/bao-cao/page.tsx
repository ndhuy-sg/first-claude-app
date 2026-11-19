import type { Metadata } from "next";
import { SalesReport } from "@/components/admin/SalesReport";

export const metadata: Metadata = { title: "Báo cáo bán hàng" };

export default function ReportPage() {
  return <SalesReport />;
}
