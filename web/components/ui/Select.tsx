import type { SelectHTMLAttributes } from "react";

/** Labelled native select (works with touch and screen readers). */
export function Select({
  label,
  options,
  id,
  className = "",
  ...props
}: SelectHTMLAttributes<HTMLSelectElement> & { label: string; options: { value: string; label: string }[] }) {
  const selectId = id ?? props.name ?? label;
  return (
    <label htmlFor={selectId} className={`flex flex-col gap-1.5 text-sm font-semibold text-muted ${className}`}>
      {label}
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
    </label>
  );
}
