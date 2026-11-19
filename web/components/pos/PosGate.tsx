"use client";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useCurrentShift, useDevice, useMe } from "@/lib/pos/queries";
import { PosLoading } from "./PosFrame";

/** Where /pos sends the tablet: no device → activation; no session → PIN; no open shift → open shift; else floor plan. */
export function posRedirect(state: {
  device: unknown | null;
  me: { user: { role: string }; device: unknown | null } | null;
  shift: unknown | null | undefined;
}): string | null {
  if (state.device === null) return "/pos/kich-hoat";
  if (state.me === null || state.me.device === null) return "/pos/dang-nhap";
  // Baristas cannot sell or open shifts (roles matrix); their POS task in phase 1 is sold-out.
  if (state.me.user.role === "barista") return "/pos/het-mon";
  if (state.shift === undefined) return null;
  if (state.shift === null) return "/pos/mo-ca";
  return "/pos/ban";
}

export function PosGate() {
  const router = useRouter();
  const device = useDevice();
  const me = useMe();
  const shift = useCurrentShift(!!me.data && !!device.data);

  const target =
    device.isSuccess && me.isSuccess
      ? posRedirect({ device: device.data, me: me.data, shift: shift.isSuccess ? shift.data : undefined })
      : null;

  useEffect(() => {
    if (target) router.replace(target);
  }, [target, router]);

  if (device.isError || me.isError || shift.isError) {
    return (
      <div role="alert" className="flex h-dvh flex-col items-center justify-center gap-4 text-lg text-muted">
        Không kết nối được tới máy chủ.
        <button type="button" className="h-14 rounded-xl bg-gold px-6 font-bold text-navy" onClick={() => window.location.reload()}>
          Thử lại
        </button>
      </div>
    );
  }
  return <PosLoading label="Đang mở Mộc POS…" />;
}
