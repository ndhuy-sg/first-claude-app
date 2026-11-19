import type { Metadata } from "next";
import { ActivateScreen } from "@/components/pos/ActivateScreen";

export const metadata: Metadata = { title: "Kích hoạt máy" };

export default function ActivatePage() {
  return <ActivateScreen />;
}
