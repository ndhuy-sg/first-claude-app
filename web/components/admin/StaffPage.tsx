"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Field } from "@/components/ui/Field";
import { Select } from "@/components/ui/Select";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, retryServerErrors, useBranches, useBranchCodes, useMe, useScope } from "@/lib/admin/hooks";
import { roleLabels } from "@/lib/admin/labels";
import type { Role } from "@/lib/admin/permissions";
import { branchLabel, scopeToBranchId } from "@/lib/admin/scope";
import { formatDate, formatTime } from "@/lib/format";
import { ScopeSelect } from "./ScopeSelect";
import { Cell, ErrorBox, Filters, GridRow, GridTable, Loading, PageHeader, Section } from "./parts";

const PIN_RE = /^\d{4,6}$/;

/** Screen 35: Nhân viên & phân quyền. A branch manager manages staff of their own branch only (A-11). */
export function StaffPage() {
  const me = useMe();
  const qc = useQueryClient();
  const code = useBranchCodes();
  const [scope, setScope] = useScope();
  const [showCreate, setShowCreate] = useState(false);
  const [pinFor, setPinFor] = useState<number | null>(null);
  const [pin, setPin] = useState("");
  const branchId = scopeToBranchId(scope);
  const key = ["users", branchId ?? "all"];

  const users = useQuery({
    queryKey: key,
    queryFn: async () => unwrap(await api.GET("/users", { params: { query: { branchId } } })),
    retry: retryServerErrors,
  });
  const lock = useMutation({
    mutationFn: async (v: { id: number; lock: boolean }) =>
      v.lock
        ? unwrap(await api.POST("/users/{id}/lock", { params: { path: { id: v.id } } }))
        : unwrap(await api.POST("/users/{id}/unlock", { params: { path: { id: v.id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["users"] }),
  });
  const resetPin = useMutation({
    mutationFn: async (v: { id: number; pin: string }) => unwrap(await api.POST("/users/{id}/reset-pin", { params: { path: { id: v.id } }, body: { pin: v.pin } })),
    onSuccess: () => {
      setPinFor(null);
      setPin("");
      return qc.invalidateQueries({ queryKey: ["users"] });
    },
  });
  const actionError = lock.error ?? resetPin.error;

  return (
    <>
      <PageHeader
        title="Nhân viên"
        subtitle="Khoá tài khoản có hiệu lực ngay: máy POS của người đó quay về màn hình nhập PIN."
        actions={
          <Button variant="primary" onClick={() => setShowCreate((v) => !v)} aria-expanded={showCreate}>
            + Thêm nhân viên
          </Button>
        }
      />
      {showCreate && <CreateUser onDone={() => setShowCreate(false)} />}
      <Filters>
        <ScopeSelect value={scope} onChange={setScope} />
      </Filters>
      {actionError && <ErrorBox message={errorMessage(actionError)} />}
      {users.isPending && <Loading />}
      {users.isError && <ErrorBox message={errorMessage(users.error)} onRetry={() => users.refetch()} />}
      {users.data && (
        <GridTable
          label="Danh sách nhân viên"
          cols="1fr 150px 60px 180px 60px 140px 110px 220px"
          head={["Tên", "Vai trò", "CN", "Email", "PIN", "Hoạt động gần nhất", "Trạng thái", ""]}
          minWidth={1080}
          empty={users.data.length === 0}
        >
          {users.data.map((u) => {
            const self = u.id === me.user.id;
            return (
              <GridRow key={u.id}>
                <Cell className="font-semibold">{u.name}</Cell>
                <Cell>{roleLabels[u.role]}</Cell>
                <Cell>{u.branchId === null ? "–" : code(u.branchId)}</Cell>
                <Cell className="truncate">{u.email ?? "–"}</Cell>
                <Cell>{u.hasPin ? "Có" : "Chưa"}</Cell>
                <Cell className="tabular-nums">{u.lastActiveAt ? `${formatDate(u.lastActiveAt)} ${formatTime(u.lastActiveAt)}` : "–"}</Cell>
                <Cell>{u.status === "active" ? <Chip tone="ok">Đang hoạt động</Chip> : <Chip tone="bad">Đã khoá</Chip>}</Cell>
                <Cell>
                  {pinFor === u.id ? (
                    <form
                      className="flex items-center gap-1.5"
                      onSubmit={(e) => {
                        e.preventDefault();
                        if (PIN_RE.test(pin)) resetPin.mutate({ id: u.id, pin });
                      }}
                    >
                      <input
                        aria-label={`PIN mới cho ${u.name}`}
                        inputMode="numeric"
                        maxLength={6}
                        value={pin}
                        onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))}
                        className="h-9 w-20 rounded-lg border border-line-strong px-2"
                      />
                      <Button type="submit" variant="primary" className="h-9! px-2.5!" disabled={!PIN_RE.test(pin) || resetPin.isPending}>
                        Lưu
                      </Button>
                      <Button className="h-9! px-2.5!" onClick={() => setPinFor(null)}>
                        Huỷ
                      </Button>
                    </form>
                  ) : (
                    !self && (
                      <span className="flex gap-1.5">
                        <Button className="h-9! px-2.5!" onClick={() => (setPinFor(u.id), setPin(""))} aria-label={`Đặt lại PIN ${u.name}`}>
                          Đặt lại PIN
                        </Button>
                        <Button
                          variant={u.status === "active" ? "danger" : "secondary"}
                          className="h-9! px-2.5!"
                          disabled={lock.isPending}
                          onClick={() => lock.mutate({ id: u.id, lock: u.status === "active" })}
                          aria-label={`${u.status === "active" ? "Khoá" : "Mở khoá"} ${u.name}`}
                        >
                          {u.status === "active" ? "Khoá" : "Mở khoá"}
                        </Button>
                      </span>
                    )
                  )}
                </Cell>
              </GridRow>
            );
          })}
        </GridTable>
      )}
    </>
  );
}

function CreateUser({ onDone }: { onDone: () => void }) {
  const me = useMe();
  const qc = useQueryClient();
  const branches = useBranches();
  const isOwner = me.user.role === "owner";
  const roles: Role[] = isOwner ? ["branch_manager", "cashier", "barista", "warehouse_manager", "owner"] : ["cashier", "barista"];
  const [name, setName] = useState("");
  const [role, setRole] = useState<Role>("cashier");
  const [branch, setBranch] = useState(String(me.user.branchId ?? ""));
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [pin, setPin] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const branchBound = role === "branch_manager" || role === "cashier" || role === "barista";
  const branchOptions = (branches.data ?? []).filter((b) => isOwner || b.id === me.user.branchId).map((b) => ({ value: String(b.id), label: branchLabel(b) }));
  const effectiveBranch = branch || branchOptions[0]?.value || "";
  const needsLogin = role !== "cashier" && role !== "barista";

  const create = useMutation({
    mutationFn: async (body: Schemas["UserInput"]) => unwrap(await api.POST("/users", { body })),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["users"] });
      onDone();
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (!name.trim()) return setFormError("Nhập tên nhân viên");
    if (pin && !PIN_RE.test(pin)) return setFormError("PIN gồm 4–6 chữ số");
    if (needsLogin && (!email.trim() || password.length < 8)) return setFormError("Quản lý cần email và mật khẩu ít nhất 8 ký tự");
    if (!needsLogin && !pin) return setFormError("Thu ngân và pha chế cần PIN để đăng nhập máy POS");
    const body: Schemas["UserInput"] = {
      name: name.trim(),
      role,
      branchId: branchBound ? Number(effectiveBranch) : null,
      email: email.trim() || null,
    };
    if (password) body.password = password;
    if (pin) body.pin = pin;
    create.mutate(body);
  }
  const error = formError ?? (create.isError ? errorMessage(create.error) : null);

  return (
    <Section title="Thêm nhân viên" className="mb-5">
      <form onSubmit={submit} aria-label="Thêm nhân viên" className="grid gap-4 md:grid-cols-3">
        <Field label="Họ tên" name="u-name" value={name} onChange={(e) => setName(e.target.value)} />
        <Select id="u-role" label="Vai trò" value={role} onChange={(e) => setRole(e.target.value as Role)} options={roles.map((r) => ({ value: r, label: roleLabels[r] }))} />
        {branchBound && <Select id="u-branch" label="Chi nhánh" value={effectiveBranch} onChange={(e) => setBranch(e.target.value)} options={branchOptions} disabled={!isOwner} />}
        <Field label="Email (quản lý)" name="u-email" type="email" value={email} onChange={(e) => setEmail(e.target.value)} />
        <Field label="Mật khẩu (ít nhất 8 ký tự)" name="u-password" type="password" autoComplete="new-password" value={password} onChange={(e) => setPassword(e.target.value)} />
        <Field label="PIN (4–6 số)" name="u-pin" inputMode="numeric" maxLength={6} value={pin} onChange={(e) => setPin(e.target.value.replace(/\D/g, ""))} />
        {error && (
          <p role="alert" className="text-sm font-medium text-bad md:col-span-3">
            {error}
          </p>
        )}
        <div className="flex gap-3 md:col-span-3">
          <Button type="submit" variant="primary" disabled={create.isPending}>
            Lưu nhân viên
          </Button>
          <Button onClick={onDone}>Huỷ</Button>
        </div>
      </form>
    </Section>
  );
}
