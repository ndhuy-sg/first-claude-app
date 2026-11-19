import type { Metadata } from "next";
import { ProductsPage } from "@/components/admin/ProductsPage";

export const metadata: Metadata = { title: "Món & giá" };

export default function Page() {
  return <ProductsPage />;
}
