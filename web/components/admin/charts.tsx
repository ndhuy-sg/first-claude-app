// Simple single-series bar charts in HTML/CSS (no chart library). Navy bars, values in VND.
import { formatVND } from "@/lib/format";

export type BarDatum = { key: string; label: string; value: number };

/** Horizontal bars: label + amount on each row, no legend. */
export function HBarChart({ data, label }: { data: BarDatum[]; label: string }) {
  const max = Math.max(1, ...data.map((d) => d.value));
  if (data.length === 0) return <p className="py-4 text-sm text-muted">Chưa có doanh thu</p>;
  return (
    <ul aria-label={label} className="flex flex-col gap-3">
      {data.map((d) => (
        <li key={d.key} className="grid grid-cols-[64px_1fr_auto] items-center gap-3 text-sm">
          <span className="font-semibold text-ink">{d.label}</span>
          <span className="h-3 rounded-full bg-subtle" aria-hidden>
            <span className="block h-3 rounded-full bg-navy" style={{ width: `${(d.value / max) * 100}%` }} />
          </span>
          <span className="tabular-nums font-semibold text-ink">{formatVND(d.value)}</span>
        </li>
      ))}
    </ul>
  );
}

/**
 * Vertical bars (e.g. revenue per day). Value labels only on the highest and the latest bar,
 * every bar has a title tooltip, and the chart has an aria-label summarising all values.
 */
export function VBarChart({ data, label, height = 180 }: { data: BarDatum[]; label: string; height?: number }) {
  const max = Math.max(0, ...data.map((d) => d.value));
  const maxIdx = data.findIndex((d) => d.value === max && max > 0);
  const lastIdx = data.length - 1;
  const summary = `${label}: ${data.map((d) => `${d.label} ${formatVND(d.value)}`).join(", ")}`;
  return (
    <figure role="img" aria-label={summary} className="m-0">
      <div className="flex items-end gap-2 border-b border-line" style={{ height }}>
        {data.map((d, i) => {
          const h = max > 0 ? Math.max(2, (d.value / max) * (height - 28)) : 2;
          const showValue = (i === maxIdx || i === lastIdx) && d.value > 0;
          return (
            <div key={d.key} className="flex min-w-0 flex-1 flex-col items-center justify-end" title={`${d.label}: ${formatVND(d.value)}`}>
              {showValue && <span className="mb-1 whitespace-nowrap text-[11px] font-bold tabular-nums text-ink">{formatVND(d.value)}</span>}
              <div data-testid="vbar" className="w-full max-w-10 rounded-t-[4px] bg-navy" style={{ height: h }} />
            </div>
          );
        })}
      </div>
      <div className="mt-1.5 flex gap-2" aria-hidden>
        {data.map((d) => (
          <span key={d.key} className="min-w-0 flex-1 truncate text-center text-[12px] text-muted">
            {d.label}
          </span>
        ))}
      </div>
    </figure>
  );
}
