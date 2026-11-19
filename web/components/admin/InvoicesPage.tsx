"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { useState } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Select } from "@/components/ui/Select";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useBranchCodes, useScope, useSettings } from "@/lib/admin/hooks";
import { formatVatRate, invoiceKind, invoiceStatus } from "@/lib/admin/labels";
import { scopeToBranchId } from "@/lib/admin/scope";
import { formatDate, formatTime, formatVND } from "@/lib/format";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, ErrorBox, Filters, GridRow, GridTable, KpiRow, KpiTile, Loading, PageHeader, Pager, TaxOffBanner } from "./parts";

const PAGE_SIZE = 50;
const statusOptions = [{ value: "", label: "Tất cả" }, ...Object.entries(invoiceStatus).map(([value, s]) => ({ value, label: s.label }))];

/** Screen 24: Hoá đơn điện tử (E-09). */
export function InvoicesPage() {
  const params = useSearchParams();
  const [scope, setScope] = useScope();
  // ?status=failed comes from the dashboard alert "invoice_failed"
  const [status, setStatus] = useState<Schemas["InvoiceStatus"] | "">(() => {
    const s = params?.get("status") ?? "";
    return s in invoiceStatus ? (s as Schemas["InvoiceStatus"]) : "";
  });
  const [date, setDate] = useState("");
  const [page, setPage] = useState(1);
  const code = useBranchCodes();
  const settings = useSettings();
  const qc = useQueryClient();
  const branchId = scopeToBranchId(scope);

  const query = { branchId, status: status || undefined, date: date || undefined, page, pageSize: PAGE_SIZE };
  const q = useQuery({
    queryKey: ["invoices", query],
    queryFn: async () => unwrap(await api.GET("/invoices", { params: { query } })),
    retry: retryServerErrors,
  });

  const resend = useMutation({
    mutationFn: async (id: number) => unwrap(await api.POST("/invoices/{id}/resend", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["invoices"] }),
  });

  return (
    <>
      <PageHeader title="Hoá đơn điện tử" subtitle="Hoá đơn do hệ thống tự phát hành. Khách tra cứu bằng mã trên trang /tra-cuu." />
      {settings.data && !settings.data.taxSubmissionEnabled && <TaxOffBanner />}
      {q.data && (
        <KpiRow>
          <KpiTile label="Phát hành hôm nay" value={q.data.counts.issuedToday} />
          <KpiTile label="Chưa gửi cơ quan thuế" value={q.data.counts.notSent} />
          <KpiTile label="Gửi thất bại" value={q.data.counts.failed} />
          <KpiTile label="Đã huỷ" value={q.data.counts.cancelled} />
        </KpiRow>
      )}
      <Filters>
        <ScopeSelect
          value={scope}
          onChange={(s) => {
            setScope(s);
            setPage(1);
          }}
        />
        <Select
          id="invoice-status"
          label="Trạng thái"
          value={status}
          options={statusOptions}
          onChange={(e) => {
            setStatus(e.target.value as Schemas["InvoiceStatus"] | "");
            setPage(1);
          }}
        />
        <label htmlFor="invoice-date" className="flex flex-col gap-1.5 text-sm font-semibold text-muted">
          Ngày (trống = mọi ngày)
          <input
            id="invoice-date"
            type="date"
            value={date}
            onChange={(e) => {
              setDate(e.target.value);
              setPage(1);
            }}
            className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-[15px] font-medium text-ink"
          />
        </label>
      </Filters>
      {resend.isError && <ErrorBox message={errorMessage(resend.error)} />}
      {q.isPending && <Loading />}
      {q.isError && <ErrorBox message={errorMessage(q.error)} onRetry={() => q.refetch()} />}
      {q.data && (
        <>
          <GridTable
            label="Danh sách hoá đơn"
            cols="100px 90px 50px 130px 1fr 110px 120px 130px 100px"
            head={["Số", "Ký hiệu", "CN", "Thời gian", "Người mua", "Tổng", "Thuế", "Trạng thái", ""]}
            minWidth={1040}
            empty={q.data.items.length === 0}
          >
            {q.data.items.map((inv) => (
              <GridRow key={inv.id}>
                <Cell className="font-semibold tabular-nums">
                  {inv.number}
                  {inv.kind !== "normal" && <span className="block text-[12px] font-normal text-muted">{invoiceKind[inv.kind]}</span>}
                </Cell>
                <Cell>{inv.symbol}</Cell>
                <Cell>{code(inv.branchId)}</Cell>
                <Cell className="tabular-nums">
                  {formatDate(inv.issuedAt)} {formatTime(inv.issuedAt)}
                </Cell>
                <Cell className="truncate">{inv.buyer.name || "Khách lẻ"}</Cell>
                <Cell className="tabular-nums">{formatVND(inv.total)}</Cell>
                <Cell className="tabular-nums">
                  {formatVND(inv.vatAmount)} <span className="text-muted">({formatVatRate(inv.vatRateBp)})</span>
                </Cell>
                <Cell>
                  <span className="flex flex-wrap gap-1">
                    <Chip tone={invoiceStatus[inv.status].tone}>{invoiceStatus[inv.status].label}</Chip>
                    {inv.cancelled && <Chip tone="bad">Đã huỷ</Chip>}
                  </span>
                </Cell>
                <Cell>
                  {inv.status === "failed" ? (
                    <Button
                      variant="primary"
                      className="h-9! px-3!"
                      disabled={resend.isPending && resend.variables === inv.id}
                      onClick={() => resend.mutate(inv.id)}
                      aria-label={`Gửi lại hoá đơn ${inv.number}`}
                    >
                      Gửi lại
                    </Button>
                  ) : (
                    <Link href={`/tra-cuu/${encodeURIComponent(inv.lookupCode)}`} className="font-semibold text-navy underline">
                      Xem
                    </Link>
                  )}
                </Cell>
              </GridRow>
            ))}
          </GridTable>
          <Pager page={page} pageSize={PAGE_SIZE} total={q.data.total} onPage={setPage} />
        </>
      )}
    </>
  );
}
