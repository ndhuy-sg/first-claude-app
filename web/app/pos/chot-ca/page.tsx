import type { Metadata } from "next";
import { PosFrame } from "@/components/pos/PosFrame";
import { ShiftCloseScreen } from "@/components/pos/ShiftCloseScreen";

export const metadata: Metadata = { title: "Chốt ca" };

export default function ShiftClosePage() {
  // requireShift=false: the screen stays visible after the shift is closed to show the result.
  return (
    <PosFrame title="Chốt ca" back={{ href: "/pos/ban", label: "Sơ đồ bàn" }} requireShift={false}>
      <ShiftCloseScreen />
    </PosFrame>
  );
}
