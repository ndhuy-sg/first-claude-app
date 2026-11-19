import type { Metadata } from "next";
import { PosFrame } from "@/components/pos/PosFrame";
import { SoldOutScreen } from "@/components/pos/SoldOutScreen";

export const metadata: Metadata = { title: "Báo hết món" };

export default function SoldOutPage() {
  // Baristas have no shift of their own, so the sold-out screen does not require one.
  return (
    <PosFrame title="Báo hết món" back={{ href: "/pos/ban", label: "Sơ đồ bàn" }} requireShift={false}>
      <SoldOutScreen />
    </PosFrame>
  );
}
