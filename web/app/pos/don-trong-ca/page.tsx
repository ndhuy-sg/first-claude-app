import type { Metadata } from "next";
import { PosFrame } from "@/components/pos/PosFrame";
import { ShiftOrdersScreen } from "@/components/pos/ShiftOrdersScreen";

export const metadata: Metadata = { title: "Đơn trong ca" };

export default function ShiftOrdersPage() {
  return (
    <PosFrame title="Đơn trong ca" back={{ href: "/pos/ban", label: "Sơ đồ bàn" }}>
      <ShiftOrdersScreen />
    </PosFrame>
  );
}
