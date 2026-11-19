"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Field } from "@/components/ui/Field";
import { Select } from "@/components/ui/Select";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, qk, retryServerErrors, useBranches, useBranchCodes, useMe, useScope, useSettings } from "@/lib/admin/hooks";
import { formatVatRate } from "@/lib/admin/labels";
import { can } from "@/lib/admin/permissions";
import { formatDate, formatTime } from "@/lib/format";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, ErrorBox, GridRow, GridTable, Loading, PageHeader, Section } from "./parts";

/** Screen 34: Chi nhánh, bàn & máy POS (tax info per branch, VietQR, tables, devices). */
export function BranchesPage() {
  const me = useMe();
  const canManage = can(me.user.role, "manage_branches") !== null;
  const branches = useBranches();
  const [editing, setEditing] = useState<number | "new" | null>(null);
  const [scope, setScope] = useScope();
  const branchId = scope === "all" ? branches.data?.[0]?.id : scope;

  return (
    <>
      <PageHeader
        title="Chi nhánh & máy POS"
        subtitle="Thông tin thuế của từng chi nhánh dùng để phát hành hoá đơn điện tử."
        actions={
          canManage && (
            <Button variant="primary" onClick={() => setEditing(editing === "new" ? null : "new")}>
              + Thêm chi nhánh
            </Button>
          )
        }
      />
      {editing === "new" && <BranchForm onDone={() => setEditing(null)} />}
      {branches.isPending && <Loading />}
      {branches.isError && <ErrorBox message={errorMessage(branches.error)} onRetry={() => branches.refetch()} />}
      <div className="mb-5 grid grid-cols-[repeat(auto-fill,minmax(300px,1fr))] gap-4">
        {branches.data?.map((b) =>
          editing === b.id ? (
            <div key={b.id} className="col-span-full">
              <BranchForm branch={b} onDone={() => setEditing(null)} />
            </div>
          ) : (
            <BranchCard key={b.id} branch={b} onEdit={canManage ? () => setEditing(b.id) : undefined} />
          ),
        )}
      </div>

      <div className="mb-4 flex flex-wrap items-end gap-3">
        <ScopeSelect id="branch-detail" value={branchId ?? scope} onChange={setScope} allowAll={false} />
      </div>
      {branchId !== undefined && (
        <div className="grid gap-5 xl:grid-cols-2">
          <TablesSection branchId={branchId} />
          <DevicesSection branchId={branchId} />
        </div>
      )}
    </>
  );
}

function BranchCard({ branch: b, onEdit }: { branch: Schemas["Branch"]; onEdit?: () => void }) {
  const settings = useSettings();
  const taxCode = b.taxCode || settings.data?.companyTaxCode;
  return (
    <article aria-label={b.code} className="rounded-2xl border border-line bg-white p-5">
      <div className="flex items-start justify-between gap-2">
        <div>
          <h2 className="text-base font-bold">
            {b.code} · {b.name}
          </h2>
          <p className="text-sm text-muted">{b.address || "Chưa có địa chỉ"}</p>
        </div>
        {b.isActive ? <Chip tone="ok">Hoạt động</Chip> : <Chip tone="info">Tạm ngưng</Chip>}
      </div>
      <dl className="mt-3 grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-sm">
        <dt className="text-muted">Giờ mở cửa</dt>
        <dd>{b.openingHours || "–"}</dd>
        <dt className="text-muted">Mã số thuế</dt>
        <dd>
          {taxCode || "–"}
          {!b.taxCode && <span className="text-muted"> (dùng MST công ty)</span>}
        </dd>
        <dt className="text-muted">Ký hiệu hoá đơn</dt>
        <dd>{b.invoiceSymbol || <Chip tone="warn">Chưa nhập</Chip>}</dd>
        <dt className="text-muted">Thuế suất</dt>
        <dd>{formatVatRate(b.vatRateBp)}</dd>
        <dt className="text-muted">Số hoá đơn tiếp theo</dt>
        <dd className="tabular-nums">{String(b.nextInvoiceNumber).padStart(8, "0")}</dd>
        <dt className="text-muted">VietQR</dt>
        <dd>{b.vietqr.accountNo ? `${b.vietqr.bankBin} · ${b.vietqr.accountNo} · ${b.vietqr.accountName}` : <Chip tone="warn">Chưa nhập</Chip>}</dd>
      </dl>
      {onEdit && (
        <Button className="mt-4" onClick={onEdit}>
          Sửa thông tin
        </Button>
      )}
    </article>
  );
}

/** E-invoice symbol, e.g. "1C26MAA": invoice type digit, C/K, 2-digit year, 3 letters. */
const INVOICE_SYMBOL_RE = /^[1-6][CK]\d{2}[A-Z]{3}$/;

/** Owner enters tax info per branch (PLAN decision 12, E-14): empty tax code = company tax code. */
function BranchForm({ branch, onDone }: { branch?: Schemas["Branch"]; onDone: () => void }) {
  const qc = useQueryClient();
  const [f, setF] = useState({
    code: branch?.code ?? "",
    name: branch?.name ?? "",
    address: branch?.address ?? "",
    openingHours: branch?.openingHours ?? "",
    taxCode: branch?.taxCode ?? "",
    vatRate: branch ? String(branch.vatRateBp / 100) : "8",
    invoiceSymbol: branch?.invoiceSymbol ?? "",
    bankBin: branch?.vietqr.bankBin ?? "",
    accountNo: branch?.vietqr.accountNo ?? "",
    accountName: branch?.vietqr.accountName ?? "",
  });
  const [formError, setFormError] = useState<string | null>(null);
  const set = (k: keyof typeof f) => (e: { target: { value: string } }) => setF({ ...f, [k]: e.target.value });

  const save = useMutation({
    mutationFn: async (body: Schemas["BranchInput"]) =>
      branch
        ? unwrap(await api.PUT("/branches/{id}", { params: { path: { id: branch.id } }, body }))
        : unwrap(await api.POST("/branches", { body })),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: qk.branches });
      onDone();
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    const rate = Number(f.vatRate.replace(",", "."));
    if (!f.code.trim() || !f.name.trim()) return setFormError("Nhập mã và tên chi nhánh");
    if (!Number.isFinite(rate) || rate < 0 || rate > 100) return setFormError("Thuế suất không hợp lệ");
    if (f.taxCode.trim() && !/^\d{10}(-\d{3})?$/.test(f.taxCode.trim())) return setFormError("Mã số thuế gồm 10 số, hoặc 10 số - 3 số");
    if (!INVOICE_SYMBOL_RE.test(f.invoiceSymbol.trim().toUpperCase()))
      return setFormError("Ký hiệu hoá đơn gồm 7 ký tự, ví dụ 1C26MAA (1 số, 1 chữ, 2 số năm, 3 chữ)");
    save.mutate({
      code: f.code.trim(),
      name: f.name.trim(),
      address: f.address.trim(),
      openingHours: f.openingHours.trim(),
      taxCode: f.taxCode.trim(),
      vatRateBp: Math.round(rate * 100),
      invoiceSymbol: f.invoiceSymbol.trim().toUpperCase(),
      vietqr: { bankBin: f.bankBin.trim(), accountNo: f.accountNo.trim(), accountName: f.accountName.trim() },
    });
  }
  const error = formError ?? (save.isError ? errorMessage(save.error) : null);

  return (
    <Section title={branch ? `Sửa ${branch.code}` : "Thêm chi nhánh"} className="mb-5">
      <form onSubmit={submit} aria-label={branch ? `Sửa ${branch.code}` : "Thêm chi nhánh"} className="grid gap-4 md:grid-cols-2">
        <Field label="Mã chi nhánh" name="b-code" value={f.code} onChange={set("code")} placeholder="CN3" />
        <Field label="Tên chi nhánh" name="b-name" value={f.name} onChange={set("name")} />
        <Field label="Địa chỉ" name="b-address" value={f.address} onChange={set("address")} />
        <Field label="Giờ mở cửa" name="b-hours" value={f.openingHours} onChange={set("openingHours")} placeholder="07:00–22:00" />
        <Field label="Mã số thuế (trống = MST công ty)" name="b-tax" value={f.taxCode} onChange={set("taxCode")} />
        <Field label="Ký hiệu hoá đơn" name="b-symbol" value={f.invoiceSymbol} onChange={set("invoiceSymbol")} placeholder="1C26TMC" />
        <Field label="Thuế suất (%)" name="b-vat" inputMode="decimal" value={f.vatRate} onChange={set("vatRate")} />
        <Field label="VietQR – mã ngân hàng (BIN)" name="b-bin" value={f.bankBin} onChange={set("bankBin")} placeholder="970436" />
        <Field label="VietQR – số tài khoản" name="b-acc" value={f.accountNo} onChange={set("accountNo")} />
        <Field label="VietQR – tên tài khoản" name="b-accname" value={f.accountName} onChange={set("accountName")} />
        {error && (
          <p role="alert" className="text-sm font-medium text-bad md:col-span-2">
            {error}
          </p>
        )}
        <div className="flex gap-3 md:col-span-2">
          <Button type="submit" variant="primary" disabled={save.isPending}>
            Lưu
          </Button>
          <Button onClick={onDone}>Huỷ</Button>
        </div>
      </form>
    </Section>
  );
}

const tableStatus: Record<Schemas["Table"]["status"], { label: string; tone: "ok" | "warn" | "info" }> = {
  free: { label: "Trống", tone: "info" },
  occupied: { label: "Có khách", tone: "ok" },
  awaiting_payment: { label: "Chờ thanh toán", tone: "warn" },
};

function TablesSection({ branchId }: { branchId: number }) {
  const me = useMe();
  const qc = useQueryClient();
  const canManage = can(me.user.role, "manage_tables") !== null;
  const key = ["tables", branchId];
  const tables = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/tables", { params: { query: { branchId } } })),
    retry: retryServerErrors,
  });
  const [areaName, setAreaName] = useState("");
  const [tableName, setTableName] = useState("");
  const [areaId, setAreaId] = useState("");
  const effectiveArea = areaId || String(tables.data?.[0]?.id ?? "");

  const addArea = useMutation({
    mutationFn: async () => unwrap(await api.POST("/areas", { body: { branchId, name: areaName.trim() } })),
    onSuccess: () => {
      setAreaName("");
      return qc.invalidateQueries({ queryKey: key });
    },
  });
  const addTable = useMutation({
    mutationFn: async () => unwrap(await api.POST("/tables", { body: { areaId: Number(effectiveArea), name: tableName.trim() } })),
    onSuccess: () => {
      setTableName("");
      return qc.invalidateQueries({ queryKey: key });
    },
  });
  const err = addArea.error ?? addTable.error;

  return (
    <Section title="Khu vực & bàn">
      {tables.isPending && <Loading />}
      {tables.isError && <ErrorBox message={errorMessage(tables.error)} />}
      {tables.data?.length === 0 && <p className="text-sm text-muted">Chưa có khu vực nào.</p>}
      <div className="flex flex-col gap-4">
        {tables.data?.map((a) => (
          <div key={a.id}>
            <h3 className="mb-2 text-sm font-bold">
              {a.name} <span className="font-normal text-muted">({a.tables.length} bàn)</span>
            </h3>
            <ul className="flex flex-wrap gap-2">
              {a.tables.map((t) => (
                <li key={t.id} className="flex items-center gap-2 rounded-lg border border-line px-2.5 py-1.5 text-sm">
                  {t.name}
                  <Chip tone={tableStatus[t.status].tone}>{tableStatus[t.status].label}</Chip>
                </li>
              ))}
            </ul>
          </div>
        ))}
      </div>
      {canManage && (
        <div className="mt-5 grid gap-3 border-t border-line pt-4 md:grid-cols-2">
          <form
            className="flex items-end gap-2"
            onSubmit={(e) => {
              e.preventDefault();
              if (areaName.trim()) addArea.mutate();
            }}
          >
            <Field label="Khu vực mới" name="area-name" value={areaName} onChange={(e) => setAreaName(e.target.value)} className="flex-1" />
            <Button type="submit" disabled={!areaName.trim() || addArea.isPending}>
              Thêm khu vực
            </Button>
          </form>
          {tables.data && tables.data.length > 0 && (
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (tableName.trim()) addTable.mutate();
              }}
            >
              <Select id="table-area" label="Khu vực" value={effectiveArea} onChange={(e) => setAreaId(e.target.value)} options={tables.data.map((a) => ({ value: String(a.id), label: a.name }))} />
              <Field label="Tên bàn" name="table-name" value={tableName} onChange={(e) => setTableName(e.target.value)} className="min-w-32 flex-1" />
              <Button type="submit" disabled={!tableName.trim() || addTable.isPending}>
                Thêm bàn
              </Button>
            </form>
          )}
          {err && <ErrorBox message={errorMessage(err)} />}
        </div>
      )}
    </Section>
  );
}

function DevicesSection({ branchId }: { branchId: number }) {
  const me = useMe();
  const qc = useQueryClient();
  const code = useBranchCodes();
  const canManage = can(me.user.role, "manage_devices") !== null;
  const key = ["devices", branchId];
  const devices = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/devices", { params: { query: { branchId } } })),
    retry: retryServerErrors,
  });
  const revoke = useMutation({
    mutationFn: async (id: number) => unwrap(await api.POST("/devices/{id}/revoke", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: key }),
  });

  return (
    <Section title="Máy POS">
      <p className="mb-3 text-sm text-muted">Kích hoạt máy mới ngay trên máy tính bảng: quản lý đăng nhập ở màn hình kích hoạt và chọn chi nhánh.</p>
      {devices.isPending && <Loading />}
      {devices.isError && <ErrorBox message={errorMessage(devices.error)} />}
      {revoke.isError && <ErrorBox message={errorMessage(revoke.error)} />}
      {devices.data && (
        <GridTable label="Máy POS" cols="1fr 50px 140px 110px 90px" head={["Tên máy", "CN", "Hoạt động gần nhất", "Trạng thái", ""]} minWidth={520} empty={devices.data.length === 0}>
          {devices.data.map((d) => (
            <GridRow key={d.id}>
              <Cell className="font-semibold">{d.name}</Cell>
              <Cell>{code(d.branchId)}</Cell>
              <Cell className="tabular-nums">{d.lastSeenAt ? `${formatDate(d.lastSeenAt)} ${formatTime(d.lastSeenAt)}` : "–"}</Cell>
              <Cell>{d.revoked ? <Chip tone="bad">Đã thu hồi</Chip> : <Chip tone="ok">Đang dùng</Chip>}</Cell>
              <Cell>
                {canManage && !d.revoked && (
                  <Button variant="danger" className="h-9! px-3!" disabled={revoke.isPending} onClick={() => revoke.mutate(d.id)} aria-label={`Thu hồi ${d.name}`}>
                    Thu hồi
                  </Button>
                )}
              </Cell>
            </GridRow>
          ))}
        </GridTable>
      )}
    </Section>
  );
}
