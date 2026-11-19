import type { Metadata } from "next";
import { OpenShiftScreen } from "@/components/pos/OpenShiftScreen";
import { PosFrame } from "@/components/pos/PosFrame";

export const metadata: Metadata = { title: "Mở ca" };

export default function OpenShiftPage() {
  return (
    <PosFrame title="Mở ca" requireShift={false}>
      <OpenShiftScreen />
    </PosFrame>
  );
}
