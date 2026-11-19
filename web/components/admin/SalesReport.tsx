"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { api, unwrap } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useMe, useScope } from "@/lib/admin/hooks";
import { paymentMethodLabel } from "@/lib/admin/labels";
import { addDays, average, buildSalesQuery, daysBetween, longDate, percent, shortDate, type SalesFilters, type SalesGroupBy } from "@/lib/admin/reports";
import { formatVND, todayVN } from "@/lib/format";
import { VBarChart } from "./charts";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, DateInput, ErrorBox, Filters, GridRow, GridTable, KpiRow, KpiTile, Loading, PageHeader, Section } from "./parts";

function useSales(filters: SalesFilters, groupBy: SalesGroupBy, enabled = true) {
  const query = buildSalesQuery(filters, groupBy);
  return useQuery({
    queryKey: ["reports", "sales", query],
    queryFn: async () => unwrap(await api.GET("/reports/sales", { params: { query } })),
    retry: retryServerErrors,
    enabled,
  });
}

/** Screen 22: Báo cáo bán hàng (W-13). */
export function SalesReport() {
  const me = useMe();
  const [scope, setScope] = useScope();
  const [to, setTo] = useState(() => todayVN());
  const [from, setFrom] = useState(() => addDays(todayVN(), -6));
  const filters: SalesFilters = { scope, from, to };
  const range = buildSalesQuery(filters, "day");

  const byDay = useSales(filters, "day");
  const byPayment = useSales(filters, "payment_method");
  const byBranch = useSales(filters, "branch", me.user.role === "owner" || me.user.branchId === null);

  const totals = byDay.data?.totals;
  const dayRows = new Map((byDay.data?.rows ?? []).map((r) => [r.key, r.revenue]));
  const chartData = daysBetween(range.from, range.to).map((d) => ({ key: d, label: shortDate(d), value: dayRows.get(d) ?? 0 }));

  return (
    <>
      <PageHeader title="Báo cáo bán hàng" subtitle={`Từ ${longDate(range.from)} đến ${longDate(range.to)} · chỉ tính đơn đã thanh toán`} />
      <Filters>
        <ScopeSelect value={scope} onChange={setScope} />
        <DateInput id="report-from" label="Từ ngày" value={from} onChange={setFrom} />
        <DateInput id="report-to" label="Đến ngày" value={to} onChange={setTo} />
        <div className="flex gap-2">
          {[7, 30].map((n) => (
            <button
              key={n}
              type="button"
              onClick={() => {
                const today = todayVN();
                setTo(today);
                setFrom(addDays(today, -(n - 1)));
              }}
              className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-sm font-semibold text-ink hover:bg-subtle"
            >
              {n} ngày
            </button>
          ))}
        </div>
      </Filters>

      {byDay.isPending && <Loading />}
      {byDay.isError && <ErrorBox message={errorMessage(byDay.error)} onRetry={() => byDay.refetch()} />}
      {totals && (
        <>
          <KpiRow>
            <KpiTile label="Doanh thu" value={formatVND(totals.revenue)} />
            <KpiTile label="Số đơn" value={totals.orderCount} />
            <KpiTile label="Giá trị trung bình / đơn" value={formatVND(average(totals.revenue, totals.orderCount))} />
            <KpiTile label="Giảm giá" value={formatVND(totals.discount)} hint={`Hoàn tiền ${formatVND(totals.refunds)}`} />
          </KpiRow>
          <Section title="Doanh thu theo ngày" className="mb-5">
            <VBarChart label="Doanh thu theo ngày" data={chartData} />
          </Section>
        </>
      )}

      <Section title="Theo cách thanh toán" className="mb-5">
        {byPayment.isError && <ErrorBox message={errorMessage(byPayment.error)} />}
        {byPayment.data && (
          <div className="grid grid-cols-[repeat(auto-fit,minmax(200px,1fr))] gap-3">
            {byPayment.data.rows.length === 0 && <p className="text-sm text-muted">Chưa có giao dịch</p>}
            {byPayment.data.rows.map((r) => (
              <KpiTile
                key={r.key}
                label={paymentMethodLabel(r.key) === r.key ? r.label : paymentMethodLabel(r.key)}
                value={formatVND(r.revenue)}
                hint={`${r.orderCount} đơn · ${percent(r.revenue, byPayment.data.totals.revenue)}`}
              />
            ))}
          </div>
        )}
      </Section>

      {byBranch.data && (
        <Section title="So sánh chi nhánh">
          <GridTable
            label="So sánh chi nhánh"
            cols="1fr 150px 90px 150px 90px"
            head={["Chi nhánh", "Doanh thu", "Số đơn", "TB / đơn", "Tỷ trọng"]}
            minWidth={600}
            empty={byBranch.data.rows.length === 0}
          >
            {byBranch.data.rows.map((r) => (
              <GridRow key={r.key}>
                <Cell className="font-semibold">{r.label}</Cell>
                <Cell className="tabular-nums">{formatVND(r.revenue)}</Cell>
                <Cell className="tabular-nums">{r.orderCount}</Cell>
                <Cell className="tabular-nums">{formatVND(average(r.revenue, r.orderCount))}</Cell>
                <Cell className="tabular-nums">{percent(r.revenue, byBranch.data.totals.revenue)}</Cell>
              </GridRow>
            ))}
          </GridTable>
        </Section>
      )}
    </>
  );
}
