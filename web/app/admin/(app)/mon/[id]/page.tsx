import type { Metadata } from "next";
import { Suspense } from "react";
import { ProductEdit } from "@/components/admin/ProductEdit";
import { Loading } from "@/components/admin/parts";

export const metadata: Metadata = { title: "Sửa món" };

export default function Page() {
  return (
    <Suspense fallback={<Loading />}>
      <ProductEdit />
    </Suspense>
  );
}
