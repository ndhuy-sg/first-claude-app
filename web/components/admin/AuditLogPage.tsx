"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";
import { Select } from "@/components/ui/Select";
import { api, unwrap } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useBranchCodes, useScope } from "@/lib/admin/hooks";
import { auditActionLabel, auditActions } from "@/lib/admin/labels";
import { scopeToBranchId } from "@/lib/admin/scope";
import { formatDate, formatTime, formatVND } from "@/lib/format";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, ErrorBox, Filters, GridRow, GridTable, Loading, PageHeader, Pager } from "./parts";

const PAGE_SIZE = 50;
const actionOptions = [{ value: "", label: "Tất cả thao tác" }, ...Object.entries(auditActions).map(([value, label]) => ({ value, label }))];

/** Short readable form of a before/after value: money-like numbers as VND, objects as key: value. */
export function describeValue(v: unknown): string {
  if (v === null || v === undefined) return "–";
  if (typeof v === "number") return Number.isInteger(v) && Math.abs(v) >= 1000 ? formatVND(v) : String(v);
  if (typeof v === "string" || typeof v === "boolean") return String(v);
  if (Array.isArray(v)) return v.map(describeValue).join(", ");
  return Object.entries(v as Record<string, unknown>)
    .map(([k, x]) => `${k}: ${typeof x === "object" && x !== null ? JSON.stringify(x) : describeValue(x)}`)
    .join("; ");
}

/** Screen 36: Nhật ký thao tác (read-only, L-02; ql1 sees CN1 only, L-03). */
export function AuditLogPage() {
  const [scope, setScope] = useScope();
  const [action, setAction] = useState("");
  const [page, setPage] = useState(1);
  const code = useBranchCodes();
  const query = { branchId: scopeToBranchId(scope), action: action || undefined, page, pageSize: PAGE_SIZE };
  const q = useQuery({
    queryKey: ["audit", query],
    queryFn: async () => unwrap(await api.GET("/audit-logs", { params: { query } })),
    retry: retryServerErrors,
  });

  return (
    <>
      <PageHeader title="Nhật ký thao tác" subtitle="Ghi lại giảm giá, huỷ đơn, hoàn tiền, đổi giá, khoá nhân viên, đổi cài đặt. Không sửa hay xoá được." />
      <Filters>
        <ScopeSelect
          value={scope}
          onChange={(s) => {
            setScope(s);
            setPage(1);
          }}
        />
        <Select
          id="audit-action"
          label="Thao tác"
          value={action}
          options={actionOptions}
          onChange={(e) => {
            setAction(e.target.value);
            setPage(1);
          }}
        />
      </Filters>
      {q.isPending && <Loading />}
      {q.isError && <ErrorBox message={errorMessage(q.error)} onRetry={() => q.refetch()} />}
      {q.data && (
        <>
          <GridTable
            label="Nhật ký thao tác"
            cols="130px 130px 120px 50px 140px 1fr 1fr 160px"
            head={["Thời gian", "Người làm", "Người duyệt", "CN", "Thao tác", "Trước", "Sau", "Ghi chú"]}
            minWidth={1100}
            empty={q.data.items.length === 0}
          >
            {q.data.items.map((l) => (
              <GridRow key={l.id}>
                <Cell className="tabular-nums">
                  {formatDate(l.at)} {formatTime(l.at)}
                </Cell>
                <Cell>{l.actor?.name ?? "Hệ thống"}</Cell>
                <Cell>{l.approver?.name ?? "–"}</Cell>
                <Cell>{l.branchId === null ? "–" : code(l.branchId)}</Cell>
                <Cell>
                  <span className="font-semibold">{auditActionLabel(l.action)}</span>
                  <span className="block text-[12px] text-muted">
                    {l.entityType} {l.entityId}
                  </span>
                </Cell>
                <Cell className="break-words text-[13px] text-muted">{describeValue(l.before)}</Cell>
                <Cell className="break-words text-[13px]">{describeValue(l.after)}</Cell>
                <Cell className="break-words text-[13px]">{l.note || "–"}</Cell>
              </GridRow>
            ))}
          </GridTable>
          <Pager page={page} pageSize={PAGE_SIZE} total={q.data.total} onPage={setPage} />
        </>
      )}
    </>
  );
}
