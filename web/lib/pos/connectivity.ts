"use client";
// Online / offline detection for the POS (W-14, screen 16).
// Offline = the browser says so (navigator.onLine) OR the last request to the API failed at the
// network level (fetch threw, or the proxy answered 502/503/504 because the Go API is down).
// While offline a watcher pings /api/v1/healthz every few seconds and clears the state when it answers.
import { useEffect, useSyncExternalStore } from "react";
import { api } from "@/lib/api/client";

let browserOnline = true;
let serverReachable = true;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export function isOnline(): boolean {
  return browserOnline && serverReachable;
}

export function reportRequestFailure() {
  if (serverReachable) {
    serverReachable = false;
    emit();
  }
}

export function reportRequestSuccess() {
  if (!serverReachable) {
    serverReachable = true;
    emit();
  }
}

/** Test helper: reset to online. */
export function resetConnectivity() {
  browserOnline = true;
  serverReachable = true;
  emit();
}

function subscribe(cb: () => void) {
  listeners.add(cb);
  const onOnline = () => {
    browserOnline = true;
    // Optimistic: the next failed request marks the server unreachable again.
    serverReachable = true;
    emit();
  };
  const onOffline = () => {
    browserOnline = false;
    emit();
  };
  if (listeners.size === 1 && typeof window !== "undefined") {
    browserOnline = navigator.onLine;
  }
  window.addEventListener("online", onOnline);
  window.addEventListener("offline", onOffline);
  return () => {
    listeners.delete(cb);
    window.removeEventListener("online", onOnline);
    window.removeEventListener("offline", onOffline);
  };
}

/** true while the POS can reach the server. Server render and first paint assume online. */
export function useOnline(): boolean {
  return useSyncExternalStore(subscribe, isOnline, () => true);
}

const GATEWAY_DOWN = new Set([502, 503, 504]);

// Watch every API call made through the typed client. Guarded because tests replace `api` with a mock.
let installed = false;
export function installConnectivityMiddleware() {
  if (installed || typeof api.use !== "function") return;
  installed = true;
  api.use({
    onResponse({ response }) {
      if (GATEWAY_DOWN.has(response.status)) reportRequestFailure();
      else reportRequestSuccess();
      return undefined;
    },
    onError({ error }) {
      reportRequestFailure();
      return error instanceof Error ? error : new Error(String(error));
    },
  });
}
installConnectivityMiddleware();

/** Is this error a lost connection (as opposed to an API error with a message)? */
export function isNetworkError(err: unknown): boolean {
  if (err instanceof TypeError) return true;
  const status = (err as { status?: number } | null)?.status;
  return typeof status === "number" && GATEWAY_DOWN.has(status);
}

/** While offline, ping the server so the POS recovers by itself when the connection is back. */
export function useConnectivityWatcher(intervalMs = 5000) {
  const online = useOnline();
  useEffect(() => {
    if (online) return;
    let stopped = false;
    const ping = async () => {
      try {
        const res = await fetch("/api/v1/healthz", { cache: "no-store" });
        if (!stopped && res.ok) reportRequestSuccess();
      } catch {
        // still offline
      }
    };
    const t = setInterval(ping, intervalMs);
    return () => {
      stopped = true;
      clearInterval(t);
    };
  }, [online, intervalMs]);
}
