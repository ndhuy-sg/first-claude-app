import type { Metadata } from "next";
import { SettingsPage } from "@/components/admin/SettingsPage";

export const metadata: Metadata = { title: "Cài đặt" };

export default function Page() {
  return <SettingsPage />;
}
