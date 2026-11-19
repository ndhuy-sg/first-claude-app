"use client";

import { Suspense, use, type ReactNode } from "react";
import { browser } from "react-dom";

function Gate({ reason, children }: { reason: string; children: ReactNode }) {
  use(browser(reason));
  return children;
}

/**
 * Renders children only in the browser; the fallback is the prerendered HTML.
 * Use for screens that depend on the signed-in user or on client-only data fetching
 * (TanStack Query reads the current time, which cacheComponents prerendering forbids).
 */
export function ClientOnly({ children, fallback = null, reason = "Depends on the browser session" }: { children: ReactNode; fallback?: ReactNode; reason?: string }) {
  return (
    <Suspense fallback={fallback}>
      <Gate reason={reason}>{children}</Gate>
    </Suspense>
  );
}
