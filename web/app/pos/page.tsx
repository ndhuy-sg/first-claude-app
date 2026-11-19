import type { Metadata } from "next";
import { PosGate } from "@/components/pos/PosGate";

export const metadata: Metadata = { title: "Bán hàng" };

export default function PosHome() {
  return <PosGate />;
}
