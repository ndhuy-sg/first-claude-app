import Link from "next/link";

export function LookupHeader() {
  return (
    <header className="bg-navy px-5 py-4 print:hidden">
      <Link href="/tra-cuu" className="flex items-baseline gap-2">
        <span className="font-display text-2xl font-bold text-gold">Mộc</span>
        <span className="text-sm text-white/75">Hoá đơn điện tử</span>
      </Link>
    </header>
  );
}

/** Phone-width page frame (designed at 390px). */
export function LookupFrame({ children }: { children: React.ReactNode }) {
  return (
    <div className="min-h-screen bg-surface print:bg-white">
      <LookupHeader />
      <main className="mx-auto w-full max-w-[430px] px-4 py-6 print:max-w-none print:p-0">{children}</main>
    </div>
  );
}
