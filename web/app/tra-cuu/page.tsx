import type { Metadata } from "next";
import { LookupForm } from "./LookupForm";
import { LookupFrame } from "./LookupHeader";

export const metadata: Metadata = { title: "Tra cứu hoá đơn điện tử" };

export default function LookupPage() {
  return (
    <LookupFrame>
      <LookupForm />
    </LookupFrame>
  );
}
