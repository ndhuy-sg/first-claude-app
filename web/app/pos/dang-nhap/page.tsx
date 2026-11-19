import type { Metadata } from "next";
import { PinLoginScreen } from "@/components/pos/PinLoginScreen";

export const metadata: Metadata = { title: "Đăng nhập bằng PIN" };

export default function PinLoginPage() {
  return <PinLoginScreen />;
}
