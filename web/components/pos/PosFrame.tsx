"use client";
import { useRouter } from "next/navigation";
import { createContext, useContext, useEffect, type ReactNode } from "react";
import type { Schemas } from "@/lib/api/client";
import { useConnectivityWatcher } from "@/lib/pos/connectivity";
import { useServerEvents } from "@/lib/pos/events";
import { useIdleLock } from "@/lib/pos/idle-lock";
import { useCurrentShift, useMe, useMyBranch, type Me, type Shift } from "@/lib/pos/queries";
import { useLockScreen } from "@/lib/pos/util";
import { useWakeLock } from "@/lib/pos/wake-lock";
import { OfflineBanner } from "./OfflineBanner";
import { PosHeader } from "./PosHeader";

type Session = { me: Me; shift: Shift | null; branch: Schemas["Branch"] | null; lock: () => void };
const SessionContext = createContext<Session | null>(null);

/** Logged-in user, open shift and branch of the current POS screen. Only valid inside <PosFrame>. */
export function usePosSession(): Session {
  const s = useContext(SessionContext);
  if (!s) throw new Error("usePosSession must be used inside <PosFrame>");
  return s;
}

export function PosLoading({ label = "Đang tải…" }: { label?: string }) {
  return (
    <div className="flex h-full min-h-[50vh] flex-1 items-center justify-center text-lg text-muted" role="status">
      {label}
    </div>
  );
}

/**
 * Frame of every POS screen after PIN login: guards the session (→ PIN screen) and the shift (→ open shift),
 * runs auto-lock, SSE, wake lock and the connectivity watcher, and renders the header and offline banner.
 */
export function PosFrame({
  title,
  back,
  requireShift = true,
  children,
}: {
  title?: string;
  back?: { href: string; label: string };
  requireShift?: boolean;
  children: ReactNode;
}) {
  const router = useRouter();
  const me = useMe();
  const loggedIn = !!me.data;
  const shift = useCurrentShift(loggedIn);
  const branch = useMyBranch();
  const lock = useLockScreen();

  useEffect(() => {
    if (me.isSuccess && me.data === null) router.replace("/pos/dang-nhap");
  }, [me.isSuccess, me.data, router]);
  useEffect(() => {
    if (requireShift && shift.isSuccess && shift.data === null) router.replace("/pos/mo-ca");
  }, [requireShift, shift.isSuccess, shift.data, router]);

  useIdleLock(me.data?.settings.posIdleLockMinutes, lock, loggedIn);
  useServerEvents({ enabled: loggedIn, userId: me.data?.user.id, onUserLocked: lock });
  useWakeLock(!!shift.data);
  useConnectivityWatcher();

  let body: ReactNode;
  if (me.isError || shift.isError) {
    body = (
      <div className="flex flex-1 flex-col items-center justify-center gap-4 text-lg text-muted" role="alert">
        Không tải được dữ liệu. Kiểm tra kết nối mạng.
        <button
          type="button"
          className="h-14 rounded-xl bg-gold px-6 font-bold text-navy"
          onClick={() => {
            void me.refetch();
            void shift.refetch();
          }}
        >
          Thử lại
        </button>
      </div>
    );
  } else if (!me.data || (requireShift && !shift.data) || shift.isPending) {
    body = <PosLoading />;
  } else {
    body = (
      <SessionContext.Provider value={{ me: me.data, shift: shift.data ?? null, branch: branch.data ?? null, lock }}>{children}</SessionContext.Provider>
    );
  }

  return (
    <div className="flex h-dvh flex-col overflow-hidden">
      {me.data && <PosHeader me={me.data} shift={shift.data} branchName={branch.data?.name} title={title} back={back} onLock={lock} />}
      <OfflineBanner />
      <main className="flex min-h-0 flex-1">{body}</main>
    </div>
  );
}
