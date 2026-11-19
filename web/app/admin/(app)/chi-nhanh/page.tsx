import type { Metadata } from "next";
import { BranchesPage } from "@/components/admin/BranchesPage";

export const metadata: Metadata = { title: "Chi nhánh & máy POS" };

export default function Page() {
  return <BranchesPage />;
}
