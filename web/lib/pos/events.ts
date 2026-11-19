"use client";
// Server-Sent Events from GET /api/v1/events. Keeps POS screens fresh and enforces user locks (F-04).
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { posKeys } from "./queries";

export type ServerEvent = { type: string; branchId?: number; data?: Record<string, unknown> | null };

function parse(e: MessageEvent): ServerEvent | null {
  try {
    return JSON.parse(e.data) as ServerEvent;
  } catch {
    return null;
  }
}

function num(d: ServerEvent["data"], ...keys: string[]): number | null {
  for (const k of keys) {
    const v = d?.[k];
    if (typeof v === "number") return v;
  }
  return null;
}

/** The user id carried by a user.locked event (the contract leaves the payload shape open). */
export function lockedUserId(ev: ServerEvent | null): number | null {
  return num(ev?.data, "userId", "id");
}

export function useServerEvents(opts: { enabled: boolean; userId: number | undefined; onUserLocked: () => void }) {
  const qc = useQueryClient();
  const lockRef = useRef(opts.onUserLocked);
  useEffect(() => {
    lockRef.current = opts.onUserLocked;
  }, [opts.onUserLocked]);
  const { enabled, userId } = opts;

  useEffect(() => {
    if (!enabled || typeof EventSource === "undefined") return;
    const es = new EventSource("/api/v1/events", { withCredentials: true });
    const invalidate = (...keys: (readonly unknown[])[]) => {
      for (const queryKey of keys) void qc.invalidateQueries({ queryKey });
    };
    const handlers: Record<string, (e: MessageEvent) => void> = {
      "order.updated": (e) => {
        const orderId = num(parse(e)?.data, "orderId", "id");
        invalidate(orderId !== null ? posKeys.order(orderId) : ["pos", "order"], posKeys.ordersAll, posKeys.tables);
      },
      "table.updated": () => invalidate(posKeys.tables),
      "product.sold_out": () => invalidate(posKeys.menu),
      "invoice.updated": () => invalidate(["pos", "order"], posKeys.ordersAll),
      "shift.closed": () => invalidate(posKeys.shift),
      "user.locked": (e) => {
        const id = lockedUserId(parse(e));
        // Without a user id in the payload, re-check the session: a locked user gets 401 (A-06).
        if (id === null) invalidate(posKeys.me);
        else if (id === userId) lockRef.current();
      },
    };
    for (const [name, h] of Object.entries(handlers)) es.addEventListener(name, h as EventListener);
    return () => es.close();
  }, [enabled, userId, qc]);
}
