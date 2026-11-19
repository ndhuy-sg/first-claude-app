import type { Metadata } from "next";
import { FloorPlanScreen } from "@/components/pos/FloorPlanScreen";
import { PosFrame } from "@/components/pos/PosFrame";

export const metadata: Metadata = { title: "Sơ đồ bàn" };

export default function FloorPlanPage() {
  return (
    <PosFrame title="Sơ đồ bàn">
      <FloorPlanScreen />
    </PosFrame>
  );
}
