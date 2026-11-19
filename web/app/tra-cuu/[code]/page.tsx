import type { Metadata } from "next";
import { ClientOnly } from "@/components/ui/ClientOnly";
import { LookupFrame } from "../LookupHeader";
import { InvoiceView } from "./InvoiceView";

export const metadata: Metadata = { title: "Hoá đơn điện tử", robots: { index: false, follow: false } };

export default function InvoiceLookupPage() {
  return (
    <LookupFrame>
      <ClientOnly
        reason="Public invoice is fetched in the browser"
        fallback={
          <p role="status" className="py-10 text-center text-sm text-muted">
            Đang tải hoá đơn…
          </p>
        }
      >
        <InvoiceView />
      </ClientOnly>
    </LookupFrame>
  );
}
