"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { Chip } from "@/components/ui/Chip";
import { api, unwrap } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useScope } from "@/lib/admin/hooks";
import { alertTone } from "@/lib/admin/labels";
import { alertHref } from "@/lib/admin/nav";
import { longDate } from "@/lib/admin/reports";
import { scopeToBranchId } from "@/lib/admin/scope";
import { formatVND, todayVN } from "@/lib/format";
import { HBarChart } from "./charts";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, DateInput, ErrorBox, GridRow, GridTable, KpiRow, KpiTile, Loading, PageHeader, Section } from "./parts";

/** Screen 21: Tổng quan. */
export function Dashboard() {
  const [scope, setScope] = useScope();
  const [date, setDate] = useState(() => todayVN());
  const branchId = scopeToBranchId(scope);

  const q = useQuery({
    queryKey: ["dashboard", branchId ?? "all", date],
    queryFn: async () => unwrap(await api.GET("/reports/dashboard", { params: { query: { branchId, date } } })),
    retry: retryServerErrors,
  });
  const d = q.data;

  return (
    <>
      <PageHeader
        title="Tổng quan"
        subtitle={`Ngày ${longDate(date)}`}
        actions={
          <>
            <ScopeSelect value={scope} onChange={setScope} />
            <DateInput id="dashboard-date" label="Ngày" value={date} onChange={setDate} />
          </>
        }
      />
      {q.isPending && <Loading />}
      {q.isError && <ErrorBox message={errorMessage(q.error)} onRetry={() => q.refetch()} />}
      {d && (
        <>
          <KpiRow>
            <KpiTile label="Doanh thu" value={formatVND(d.revenue)} />
            <KpiTile label="Số đơn" value={d.orderCount} />
            <KpiTile label="Giá trị trung bình / đơn" value={formatVND(d.avgOrderValue)} />
            <KpiTile label="Hoá đơn đã phát hành" value={d.invoicesIssued} />
          </KpiRow>
          <div className="mb-5 grid gap-5 lg:grid-cols-[3fr_2fr]">
            <Section title="Doanh thu theo chi nhánh">
              <HBarChart
                label="Doanh thu theo chi nhánh"
                data={[...d.byBranch]
                  .sort((a, b) => a.branchCode.localeCompare(b.branchCode, "vi", { numeric: true }))
                  .map((b) => ({ key: String(b.branchId), label: b.branchCode, value: b.revenue }))}
              />
            </Section>
            <Section title="Cần xử lý">
              {d.alerts.length === 0 ? (
                <p className="text-sm text-muted">Không có việc cần xử lý.</p>
              ) : (
                <ul className="flex flex-col gap-2.5">
                  {d.alerts.map((a, i) => (
                    <li key={`${a.kind}-${i}`} className="flex items-start gap-2.5 text-sm">
                      <Chip tone={alertTone[a.severity].tone}>{alertTone[a.severity].label}</Chip>
                      <span className="min-w-0 flex-1">
                        {a.message}
                        {alertHref(a.link) && (
                          <>
                            {" "}
                            <Link href={alertHref(a.link)!} className="font-semibold text-navy underline">
                              Xem
                            </Link>
                          </>
                        )}
                      </span>
                    </li>
                  ))}
                </ul>
              )}
            </Section>
          </div>
          <Section title="Món bán chạy">
            <GridTable label="Món bán chạy" cols="40px 1fr 100px 140px" head={["#", "Món", "Số lượng", "Doanh thu"]} minWidth={420} empty={d.topProducts.length === 0}>
              {d.topProducts.map((p, i) => (
                <GridRow key={p.productId}>
                  <Cell className="text-muted">{i + 1}</Cell>
                  <Cell className="font-semibold">{p.name}</Cell>
                  <Cell className="tabular-nums">{p.qty}</Cell>
                  <Cell className="tabular-nums">{formatVND(p.revenue)}</Cell>
                </GridRow>
              ))}
            </GridTable>
          </Section>
        </>
      )}
    </>
  );
}
