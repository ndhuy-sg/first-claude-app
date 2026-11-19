import type { Metadata } from "next";
import { Suspense } from "react";
import { OrderScreen } from "@/components/pos/OrderScreen";
import { PosFrame, PosLoading } from "@/components/pos/PosFrame";

export const metadata: Metadata = { title: "Gọi món" };

export default function OrderPage({ params }: PageProps<"/pos/don/[id]">) {
  return (
    <PosFrame back={{ href: "/pos/ban", label: "Sơ đồ bàn" }}>
      <Suspense fallback={<PosLoading />}>{params.then(({ id }) => <OrderScreen orderId={Number(id)} />)}</Suspense>
    </PosFrame>
  );
}
