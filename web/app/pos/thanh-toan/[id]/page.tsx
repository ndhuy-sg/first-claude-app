import type { Metadata } from "next";
import { Suspense } from "react";
import { PaymentScreen } from "@/components/pos/PaymentScreen";
import { PosFrame, PosLoading } from "@/components/pos/PosFrame";

export const metadata: Metadata = { title: "Thanh toán" };

export default function PaymentPage({ params }: PageProps<"/pos/thanh-toan/[id]">) {
  return (
    <Suspense fallback={<PosLoading />}>
      {params.then(({ id }) => (
        <PosFrame title="Thanh toán" back={{ href: `/pos/don/${id}`, label: "Quay lại đơn" }}>
          <PaymentScreen orderId={Number(id)} />
        </PosFrame>
      ))}
    </Suspense>
  );
}
