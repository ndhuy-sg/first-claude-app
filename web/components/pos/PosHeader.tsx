"use client";
import Link from "next/link";
import { formatTime } from "@/lib/format";
import { useOnline } from "@/lib/pos/connectivity";
import { shiftName } from "@/lib/pos/money";
import type { Me, Shift } from "@/lib/pos/queries";
import { ROLE_LABEL } from "@/lib/pos/util";

export function Logo({ size = 26 }: { size?: number }) {
  return (
    <span className="font-display font-bold leading-none text-gold" style={{ fontSize: size }}>
      Mộc
    </span>
  );
}

export function initial(name: string): string {
  const parts = name.trim().split(/\s+/);
  return (parts[parts.length - 1]?.[0] ?? "?").toUpperCase();
}

/** 64px navy POS header (all screens after login). */
export function PosHeader({
  me,
  shift,
  branchName,
  title,
  back,
  onLock,
}: {
  me: Me;
  shift: Shift | null | undefined;
  branchName?: string;
  title?: string;
  back?: { href: string; label: string };
  onLock: () => void;
}) {
  const online = useOnline();
  return (
    <header className="flex h-16 shrink-0 items-center gap-4 bg-navy px-5 text-white">
      <Logo />
      <span className="rounded-full bg-navy-2 px-3 py-1.5 text-sm font-semibold" data-testid="branch-chip">
        {branchName ?? me.device?.branchCode ?? ""}
      </span>
      {back && (
        <Link
          href={back.href}
          className="inline-flex h-11 items-center rounded-[10px] border border-white/40 px-4 text-[15px] font-semibold hover:bg-navy-2"
        >
          ‹ {back.label}
        </Link>
      )}
      {title && <h1 className="truncate text-lg font-bold">{title}</h1>}
      <div className="ml-auto flex items-center gap-5 text-sm">
        <span className="flex items-center gap-2" data-testid="online-status">
          <span aria-hidden className={`h-2.5 w-2.5 rounded-full ${online ? "bg-[#5fd39a]" : "bg-[#ff7a66]"}`} />
          {online ? "Trực tuyến" : "Mất kết nối"}
        </span>
        {shift && (
          <span className="text-white/80">
            {shiftName(shift.openedAt)} · mở {formatTime(shift.openedAt)}
          </span>
        )}
        <span className="flex items-center gap-2">
          <span aria-hidden className="flex h-9 w-9 items-center justify-center rounded-full bg-gold font-bold text-navy">
            {initial(me.user.name)}
          </span>
          <span className="font-semibold">
            {me.user.name} · {ROLE_LABEL[me.user.role] ?? ""}
          </span>
        </span>
        <button
          type="button"
          onClick={onLock}
          className="h-11 rounded-[10px] border border-white/40 px-4 font-semibold hover:bg-navy-2"
          aria-label="Khoá màn hình"
        >
          <svg aria-hidden width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" className="mr-1.5 inline">
            <rect x="4" y="11" width="16" height="10" rx="2" />
            <path d="M8 11V7a4 4 0 0 1 8 0v4" />
          </svg>
          Khoá
        </button>
      </div>
    </header>
  );
}
