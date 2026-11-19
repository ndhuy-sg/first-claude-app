import type { Metadata } from "next";
import { StaffPage } from "@/components/admin/StaffPage";

export const metadata: Metadata = { title: "Nhân viên" };

export default function Page() {
  return <StaffPage />;
}
