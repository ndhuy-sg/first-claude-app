"use client";
import { useEffect, useId, useRef, type ReactNode } from "react";

/** Modal dialog for the POS. Escape or the "Đóng" button closes it. Focus moves into the dialog. */
export function Dialog({
  title,
  onClose,
  children,
  width = 560,
  footer,
}: {
  title: ReactNode;
  onClose: () => void;
  children: ReactNode;
  width?: number;
  footer?: ReactNode;
}) {
  const titleId = useId();
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.focus();
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose]);

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-navy/50 p-4">
      <div
        ref={ref}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        tabIndex={-1}
        className="flex max-h-[94vh] flex-col overflow-hidden rounded-2xl bg-white shadow-2xl outline-none"
        style={{ width, maxWidth: "100%" }}
      >
        <div className="flex items-center justify-between gap-4 border-b border-line px-6 py-4">
          <h2 id={titleId} className="text-xl font-bold text-ink">
            {title}
          </h2>
          <button type="button" onClick={onClose} className="h-11 rounded-[10px] border border-line-strong px-4 font-semibold hover:bg-subtle">
            Đóng
          </button>
        </div>
        <div className="flex-1 overflow-y-auto px-6 py-5">{children}</div>
        {footer && <div className="border-t border-line px-6 py-4">{footer}</div>}
      </div>
    </div>
  );
}
