// Small building blocks shared by the admin pages.
import Link from "next/link";
import type { ReactNode } from "react";

export function PageHeader({ title, subtitle, actions }: { title: string; subtitle?: ReactNode; actions?: ReactNode }) {
  return (
    <header className="mb-5 flex flex-wrap items-end justify-between gap-3">
      <div>
        <h1 className="text-2xl font-bold text-ink">{title}</h1>
        {subtitle && <p className="mt-1 text-sm text-muted">{subtitle}</p>}
      </div>
      {actions && <div className="flex flex-wrap items-end gap-3">{actions}</div>}
    </header>
  );
}

export function KpiTile({ label, value, hint }: { label: string; value: ReactNode; hint?: ReactNode }) {
  return (
    <div role="group" aria-label={label} className="rounded-2xl border border-line bg-white p-4">
      <div className="text-[13px] font-semibold text-muted">{label}</div>
      <div className="mt-1 text-2xl font-bold tabular-nums text-ink">{value}</div>
      {hint && <div className="mt-1 text-[13px] text-muted">{hint}</div>}
    </div>
  );
}

export function KpiRow({ children }: { children: ReactNode }) {
  return <div className="mb-5 grid grid-cols-[repeat(auto-fit,minmax(180px,1fr))] gap-3">{children}</div>;
}

export function Section({ title, actions, children, className = "" }: { title: string; actions?: ReactNode; children: ReactNode; className?: string }) {
  return (
    <section className={`rounded-2xl border border-line bg-white p-5 ${className}`} aria-label={title}>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-base font-bold text-ink">{title}</h2>
        {actions}
      </div>
      {children}
    </section>
  );
}

/** Filter bar above a table or report. */
export function Filters({ children }: { children: ReactNode }) {
  return <div className="mb-5 flex flex-wrap items-end gap-3 rounded-2xl border border-line bg-white p-4">{children}</div>;
}

export function DateInput({ label, value, onChange, id }: { label: string; value: string; onChange: (v: string) => void; id: string }) {
  return (
    <label htmlFor={id} className="flex flex-col gap-1.5 text-sm font-semibold text-muted">
      {label}
      <input
        id={id}
        type="date"
        value={value}
        onChange={(e) => e.target.value && onChange(e.target.value)}
        className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-[15px] font-medium text-ink"
      />
    </label>
  );
}

/**
 * Table drawn as CSS-grid rows. `cols` is a grid-template-columns value.
 * The table scrolls horizontally inside its own box on narrow screens.
 */
export function GridTable({
  cols,
  head,
  children,
  label,
  minWidth = 720,
  empty,
}: {
  cols: string;
  head: string[];
  children: ReactNode;
  label: string;
  minWidth?: number;
  empty?: boolean;
}) {
  return (
    <div className="overflow-x-auto rounded-xl border border-line">
      <div role="table" aria-label={label} style={{ minWidth }}>
        <div role="row" className="grid items-center gap-3 bg-subtle px-4 py-2.5 text-[13px] font-semibold text-muted" style={{ gridTemplateColumns: cols }}>
          {head.map((h, i) => (
            <div role="columnheader" key={i}>
              {h}
            </div>
          ))}
        </div>
        <div style={{ ["--cols" as string]: cols }}>{children}</div>
        {empty && <p className="px-4 py-6 text-center text-sm text-muted">Không có dữ liệu</p>}
      </div>
    </div>
  );
}

export function GridRow({ children, className = "", onClick }: { children: ReactNode; className?: string; onClick?: () => void }) {
  return (
    <div
      role="row"
      onClick={onClick}
      className={`grid min-h-12 items-center gap-3 border-t border-line px-4 py-2 text-sm ${onClick ? "cursor-pointer hover:bg-surface" : ""} ${className}`}
      style={{ gridTemplateColumns: "var(--cols)" }}
    >
      {children}
    </div>
  );
}

export function Cell({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return (
    <div role="cell" className={`min-w-0 ${className}`}>
      {children}
    </div>
  );
}

export function Loading({ label = "Đang tải…" }: { label?: string }) {
  return (
    <p role="status" className="py-6 text-center text-sm text-muted">
      {label}
    </p>
  );
}

export function ErrorBox({ message, onRetry }: { message: string; onRetry?: () => void }) {
  return (
    <div role="alert" className="my-3 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-bad/30 bg-bad-soft px-4 py-3 text-sm font-medium text-[#7a2a1e]">
      {message}
      {onRetry && (
        <button type="button" onClick={onRetry} className="h-9 rounded-lg border border-bad/40 bg-white px-3 font-semibold">
          Thử lại
        </button>
      )}
    </div>
  );
}

/** Yellow banner: e-invoices have no legal value while the tax submission switch is off (PLAN §6). */
export function TaxOffBanner({ showLink = true }: { showLink?: boolean }) {
  return (
    <div role="note" aria-label="Cảnh báo hoá đơn" className="mb-5 rounded-2xl border border-gold-line bg-gold-soft px-5 py-4 text-sm text-[#5c4400]">
      <strong className="block text-[15px]">Chưa gửi dữ liệu hoá đơn tới cơ quan thuế</strong>
      Công tắc gửi dữ liệu đang TẮT: hoá đơn vẫn được cấp số và khách tra cứu được, nhưng chỉ là chứng từ nội bộ, chưa có giá
      trị pháp lý. Hãy bật công tắc sau khi đã đăng ký sử dụng hoá đơn điện tử với cơ quan thuế.
      {showLink && (
        <>
          {" "}
          <Link href="/admin/cai-dat" className="font-bold text-navy underline">
            Mở Cài đặt
          </Link>
        </>
      )}
    </div>
  );
}

export function Pager({ page, pageSize, total, onPage }: { page: number; pageSize: number; total: number; onPage: (p: number) => void }) {
  const pages = Math.max(1, Math.ceil(total / pageSize));
  return (
    <nav aria-label="Phân trang" className="mt-3 flex items-center justify-end gap-2 text-sm text-muted">
      <span>
        Trang {page}/{pages} · {total} dòng
      </span>
      <button type="button" disabled={page <= 1} onClick={() => onPage(page - 1)} className="h-9 rounded-lg border border-line-strong bg-white px-3 font-semibold text-ink disabled:opacity-50">
        Trước
      </button>
      <button type="button" disabled={page >= pages} onClick={() => onPage(page + 1)} className="h-9 rounded-lg border border-line-strong bg-white px-3 font-semibold text-ink disabled:opacity-50">
        Sau
      </button>
    </nav>
  );
}
