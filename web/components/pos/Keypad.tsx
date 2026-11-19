"use client";
import type { ReactNode } from "react";

const keyClass =
  "flex items-center justify-center rounded-xl border border-line bg-white text-2xl font-semibold text-ink shadow-[0_1px_0_#dde3ec] active:bg-subtle disabled:text-muted disabled:cursor-not-allowed";

/** 3×4 number pad. The bottom row is given by the caller (e.g. "Xoá · 0 · Vào ca" or "000 · 0 · ⌫"). */
export function Keypad({
  onDigit,
  bottom,
  keyHeight = 64,
  disabled,
  label = "Bàn phím số",
}: {
  onDigit: (d: string) => void;
  bottom: [ReactNode, ReactNode, ReactNode];
  keyHeight?: number;
  disabled?: boolean;
  label?: string;
}) {
  return (
    <div role="group" aria-label={label} className="grid grid-cols-3 gap-2.5">
      {["1", "2", "3", "4", "5", "6", "7", "8", "9"].map((d) => (
        <button key={d} type="button" className={keyClass} style={{ height: keyHeight }} onClick={() => onDigit(d)} disabled={disabled}>
          {d}
        </button>
      ))}
      {bottom.map((node, i) => (
        <div key={i} style={{ height: keyHeight }} className="[&>button]:h-full [&>button]:w-full">
          {node}
        </div>
      ))}
    </div>
  );
}

/** A plain keypad key for the bottom row. */
export function KeypadKey({ children, onClick, disabled, ariaLabel }: { children: ReactNode; onClick: () => void; disabled?: boolean; ariaLabel?: string }) {
  return (
    <button type="button" className={keyClass} onClick={onClick} disabled={disabled} aria-label={ariaLabel}>
      {children}
    </button>
  );
}

/** Keypad for money: digits, 000, 0 and backspace. */
export function AmountKeypad({ onKey, disabled, keyHeight = 60 }: { onKey: (k: string) => void; disabled?: boolean; keyHeight?: number }) {
  return (
    <Keypad
      onDigit={onKey}
      disabled={disabled}
      keyHeight={keyHeight}
      label="Bàn phím nhập tiền"
      bottom={[
        <KeypadKey key="000" onClick={() => onKey("000")} disabled={disabled}>
          000
        </KeypadKey>,
        <KeypadKey key="0" onClick={() => onKey("0")} disabled={disabled}>
          0
        </KeypadKey>,
        <KeypadKey key="bs" onClick={() => onKey("⌫")} disabled={disabled} ariaLabel="Xoá một số">
          ⌫
        </KeypadKey>,
      ]}
    />
  );
}
