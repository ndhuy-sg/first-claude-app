import type { Metadata } from "next";
import { Suspense } from "react";
import { DoneScreen } from "@/components/pos/DoneScreen";
import { PosFrame, PosLoading } from "@/components/pos/PosFrame";

export const metadata: Metadata = { title: "Hoàn tất" };

export default function DonePage({ params }: PageProps<"/pos/hoan-tat/[id]">) {
  return (
    <PosFrame title="Hoàn tất" back={{ href: "/pos/ban", label: "Sơ đồ bàn" }}>
      <Suspense fallback={<PosLoading />}>{params.then(({ id }) => <DoneScreen orderId={Number(id)} />)}</Suspense>
    </PosFrame>
  );
}
