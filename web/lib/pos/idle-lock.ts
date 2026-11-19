"use client";
// Auto-lock after N minutes without touch / key input (PLAN decision 11, A-05, W-02).
import { useEffect, useRef } from "react";

const EVENTS = ["pointerdown", "keydown", "touchstart", "wheel"] as const;

export function useIdleLock(minutes: number | undefined, onLock: () => void, enabled = true) {
  const onLockRef = useRef(onLock);
  useEffect(() => {
    onLockRef.current = onLock;
  }, [onLock]);

  useEffect(() => {
    if (!enabled || !minutes || minutes <= 0) return;
    const ms = minutes * 60_000;
    let timer: ReturnType<typeof setTimeout>;
    const arm = () => {
      clearTimeout(timer);
      timer = setTimeout(() => onLockRef.current(), ms);
    };
    arm();
    for (const e of EVENTS) window.addEventListener(e, arm, { passive: true });
    return () => {
      clearTimeout(timer);
      for (const e of EVENTS) window.removeEventListener(e, arm);
    };
  }, [minutes, enabled]);
}
