"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useParams } from "next/navigation";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { api, ApiRequestError, unwrap, type Schemas } from "@/lib/api/client";
import { formatVatRate, invoiceKind, paymentMethodLabel } from "@/lib/admin/labels";
import { formatDate, formatTime, formatVND } from "@/lib/format";

/** Screen 19: public invoice (no login, E-11). Unknown code → friendly not-found message. */
export function InvoiceView() {
  const params = useParams<{ code: string }>();
  const code = decodeURIComponent(params?.code ?? "");
  const q = useQuery({
    queryKey: ["public-invoice", code],
    queryFn: async () => unwrap(await api.GET("/public/invoices/{code}", { params: { path: { code } } })),
    retry: (n, err) => !(err instanceof ApiRequestError && err.status < 500) && n < 2,
  });

  if (q.isPending) {
    return (
      <p role="status" className="py-10 text-center text-sm text-muted">
        Đang tải hoá đơn…
      </p>
    );
  }
  if (q.isError) {
    const notFound = q.error instanceof ApiRequestError && q.error.status === 404;
    return (
      <div className="rounded-2xl border border-line bg-white p-6 text-center">
        <h1 className="text-lg font-bold">{notFound ? "Không tìm thấy hoá đơn" : "Chưa tải được hoá đơn"}</h1>
        <p className="mt-2 text-sm text-muted">
          {notFound
            ? `Mã “${code}” không đúng hoặc hoá đơn chưa được phát hành. Vui lòng kiểm tra lại mã dưới mã QR.`
            : "Kết nối đang chập chờn, vui lòng thử lại sau ít phút."}
        </p>
        <div className="mt-4 flex justify-center gap-3">
          {!notFound && <Button onClick={() => q.refetch()}>Thử lại</Button>}
          <Link href="/tra-cuu" className="inline-flex h-11 items-center rounded-[10px] bg-gold px-4 font-bold text-navy">
            Nhập mã khác
          </Link>
        </div>
      </div>
    );
  }
  return <InvoiceDocument invoice={q.data} />;
}

export function InvoiceDocument({ invoice: inv }: { invoice: Schemas["Invoice"] }) {
  const buyer = inv.buyer;
  const mail = `mailto:${encodeURIComponent(buyer.email ?? "")}?subject=${encodeURIComponent(`Hoá đơn điện tử ${inv.symbol} số ${inv.number}`)}&body=${encodeURIComponent(
    `Xem hoá đơn tại: ${inv.lookupUrl}\nMã tra cứu: ${inv.lookupCode}`,
  )}`;
  return (
    <article aria-label={`Hoá đơn số ${inv.number}`} className="rounded-2xl border border-line bg-white p-5 print:rounded-none print:border-0">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <h1 className="text-base font-bold leading-snug">{inv.seller.name}</h1>
          <p className="text-[13px] text-muted">MST: {inv.seller.taxCode}</p>
          <p className="text-[13px] text-muted">{inv.seller.address}</p>
        </div>
        {inv.cancelled ? <Chip tone="bad">Đã huỷ</Chip> : <Chip tone="ok">Đã phát hành</Chip>}
      </div>

      <h2 className="mt-4 text-center text-lg font-bold uppercase tracking-wide">{inv.kind === "normal" ? "Hoá đơn giá trị gia tăng" : invoiceKind[inv.kind]}</h2>
      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt className="text-muted">Ký hiệu</dt>
        <dd className="font-semibold">{inv.symbol}</dd>
        <dt className="text-muted">Số</dt>
        <dd className="font-semibold tabular-nums">{inv.number}</dd>
        <dt className="text-muted">Ngày</dt>
        <dd className="tabular-nums">
          {formatDate(inv.issuedAt)} {formatTime(inv.issuedAt)}
        </dd>
        <dt className="text-muted">Người mua</dt>
        <dd>
          {buyer.name || "Khách lẻ"}
          {buyer.taxCode && <span className="block text-[13px] text-muted">MST: {buyer.taxCode}</span>}
          {buyer.address && <span className="block text-[13px] text-muted">{buyer.address}</span>}
        </dd>
        <dt className="text-muted">Thanh toán</dt>
        <dd>{paymentMethodLabel(inv.paymentMethod)}</dd>
      </dl>

      <table className="mt-4 w-full text-sm">
        <caption className="sr-only">Hàng hoá, dịch vụ</caption>
        <thead>
          <tr className="border-b border-line text-left text-[13px] text-muted">
            <th scope="col" className="py-1.5 font-semibold">
              Tên hàng
            </th>
            <th scope="col" className="py-1.5 text-right font-semibold">
              SL
            </th>
            <th scope="col" className="py-1.5 text-right font-semibold">
              Thành tiền
            </th>
          </tr>
        </thead>
        <tbody>
          {inv.lines.map((l) => (
            <tr key={l.lineNo} className="border-b border-line align-top">
              <td className="py-2">
                {l.name}
                <span className="block text-[12px] text-muted">{formatVND(l.unitPrice)}</span>
              </td>
              <td className="py-2 text-right tabular-nums">{l.qty}</td>
              <td className="py-2 text-right tabular-nums">{formatVND(l.amount)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <dl className="mt-3 grid grid-cols-[1fr_auto] gap-y-1 text-sm">
        <dt className="text-muted">Tiền trước thuế</dt>
        <dd className="text-right tabular-nums">{formatVND(inv.amountBeforeVat)}</dd>
        <dt className="text-muted">Thuế GTGT {formatVatRate(inv.vatRateBp)}</dt>
        <dd className="text-right tabular-nums">{formatVND(inv.vatAmount)}</dd>
        <dt className="text-base font-bold">Tổng thanh toán</dt>
        <dd className="text-right text-base font-bold tabular-nums">{formatVND(inv.total)}</dd>
      </dl>
      <p className="mt-1 text-[13px] italic text-muted">Bằng chữ: {inv.totalInWords}</p>
      <p className="mt-3 text-[12px] text-muted">
        Mã tra cứu: <span className="font-semibold">{inv.lookupCode}</span> · Đơn {inv.orderCode}
      </p>

      <div className="mt-5 grid grid-cols-2 gap-3 print:hidden">
        <Button variant="primary" onClick={() => window.print()}>
          Tải PDF
        </Button>
        <a href={mail} className="inline-flex h-11 items-center justify-center rounded-[10px] border border-line-strong bg-white px-4 text-[15px] font-semibold text-ink">
          Gửi qua email
        </a>
      </div>
    </article>
  );
}
