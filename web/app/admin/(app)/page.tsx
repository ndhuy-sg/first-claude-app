import type { Metadata } from "next";
import { Dashboard } from "@/components/admin/Dashboard";

export const metadata: Metadata = { title: "Tổng quan" };

export default function AdminHome() {
  return <Dashboard />;
}
