"use client";
import { useState } from "react";
import { Keypad, KeypadKey } from "./Keypad";

const MAX = 6;
const MIN = 4;

/**
 * 6 PIN dots + 3×4 keypad. Submit is enabled from 4 digits. If `onSubmit` rejects, the PIN is cleared
 * (W-02: wrong PIN shows an error and clears the input). The caller shows the error text.
 */
export function PinPad({
  onSubmit,
  submitLabel,
  error,
  busy,
  keyHeight = 76,
  onChange,
}: {
  onSubmit: (pin: string) => Promise<unknown>;
  submitLabel: string;
  error?: string;
  busy?: boolean;
  keyHeight?: number;
  onChange?: () => void;
}) {
  const [pin, setPin] = useState("");

  const press = (d: string) => {
    if (pin.length >= MAX) return;
    setPin(pin + d);
    onChange?.();
  };
  const submit = async () => {
    if (pin.length < MIN || busy) return;
    try {
      await onSubmit(pin);
    } catch {
      setPin("");
    }
  };

  return (
    <div className="flex flex-col gap-5">
      <div className="flex justify-center gap-4" role="status" aria-label={`Đã nhập ${pin.length} số`}>
        {Array.from({ length: MAX }, (_, i) => (
          <span
            key={i}
            data-testid="pin-dot"
            data-filled={i < pin.length}
            className={`h-5 w-5 rounded-full border-2 ${i < pin.length ? "border-navy bg-navy" : "border-line-strong bg-white"}`}
          />
        ))}
      </div>
      <p role="alert" className="min-h-6 text-center text-[15px] font-semibold text-bad">
        {error}
      </p>
      <Keypad
        onDigit={press}
        keyHeight={keyHeight}
        label="Bàn phím nhập PIN"
        disabled={busy}
        bottom={[
          <KeypadKey key="clear" onClick={() => setPin("")} disabled={busy}>
            <span className="text-lg">Xoá</span>
          </KeypadKey>,
          <KeypadKey key="0" onClick={() => press("0")} disabled={busy}>
            0
          </KeypadKey>,
          <button
            key="ok"
            type="button"
            onClick={submit}
            disabled={pin.length < MIN || busy}
            className="rounded-xl bg-gold text-lg font-bold text-navy disabled:cursor-not-allowed disabled:bg-subtle disabled:text-muted"
          >
            {submitLabel}
          </button>,
        ]}
      />
    </div>
  );
}
