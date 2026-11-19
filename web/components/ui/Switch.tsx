/** On/off switch with a visible text state (TẮT / BẬT), never color alone. */
export function Switch({
  checked,
  onChange,
  label,
  disabled,
  id,
}: {
  checked: boolean;
  onChange: (next: boolean) => void;
  label: string;
  disabled?: boolean;
  id?: string;
}) {
  return (
    <button
      id={id}
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className="inline-flex h-11 items-center gap-3 rounded-full disabled:cursor-not-allowed disabled:opacity-60"
    >
      <span className={`relative h-7 w-12 rounded-full transition ${checked ? "bg-ok" : "bg-line-strong"}`}>
        <span className={`absolute top-1 h-5 w-5 rounded-full bg-white shadow transition-all ${checked ? "left-6" : "left-1"}`} />
      </span>
      <span className={`text-sm font-bold ${checked ? "text-ok" : "text-bad"}`}>{checked ? "BẬT" : "TẮT"}</span>
    </button>
  );
}
