import type { InputHTMLAttributes } from "react";

/** Labelled text input. Error text is announced and linked with aria-describedby. */
export function Field({
  label,
  error,
  id,
  className = "",
  ...props
}: InputHTMLAttributes<HTMLInputElement> & { label: string; error?: string }) {
  const inputId = id ?? props.name ?? label;
  const errId = `${inputId}-error`;
  return (
    <label htmlFor={inputId} className={`flex flex-col gap-1.5 text-sm font-semibold text-muted ${className}`}>
      {label}
      <input
        id={inputId}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ? errId : undefined}
        className="h-12 rounded-xl border border-line-strong bg-white px-3.5 text-base font-normal text-ink aria-invalid:border-bad"
        {...props}
      />
      {error && (
        <span id={errId} role="alert" className="text-[13px] font-medium text-bad">
          {error}
        </span>
      )}
    </label>
  );
}
