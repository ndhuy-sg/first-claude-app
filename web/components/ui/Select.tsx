import { useId, type SelectHTMLAttributes } from "react";

/**
 * Labelled native select (works with touch and screen readers).
 * The label is a sibling linked by htmlFor (not wrapping the select), so the
 * accessible name is exactly `label` and does not include the option text.
 * Without an explicit id a unique one is generated: Next keeps previously
 * visited pages mounted (hidden), so a fixed id could be shared by two pages.
 */
export function Select({
  label,
  options,
  id,
  className = "",
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string; options: { value: string; label: string }[] }) {
  const autoId = useId();
  const selectId = id ?? autoId;
  return (
    <div className={`flex flex-col gap-1.5 ${className}`}>
      <label htmlFor={selectId} className="text-sm font-semibold text-muted">
        {label}
      </label>
      <select
        id={selectId}
        className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-[15px] font-medium text-ink"
        {...props}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </div>
  );
}
