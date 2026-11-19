"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState, type FormEvent, type ReactNode } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Field } from "@/components/ui/Field";
import { Select } from "@/components/ui/Select";
import { Switch } from "@/components/ui/Switch";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, qk, useMe, useSettings } from "@/lib/admin/hooks";
import { can } from "@/lib/admin/permissions";
import { formatVND } from "@/lib/format";
import { ErrorBox, Loading, PageHeader, Section } from "./parts";

type Settings = Schemas["Settings"];

/** Screen 37: Cài đặt hệ thống. Owner only can change (others see it read-only). */
export function SettingsPage() {
  const settings = useSettings();
  if (settings.isPending) return <Loading />;
  if (settings.isError) return <ErrorBox message={errorMessage(settings.error)} onRetry={() => settings.refetch()} />;
  // the form keeps its own copy: a background refetch must not overwrite unsaved edits
  return <SettingsForm initial={settings.data} />;
}

function gatewayOptions(current: Settings["taxGateway"]) {
  const opts: { value: Settings["taxGateway"]; label: string }[] = [
    { value: "direct", label: "Trực tiếp tới cơ quan thuế" },
    { value: "tvan", label: "Qua T-VAN" },
  ];
  if (current === "off") opts.unshift({ value: "off", label: "Chưa chọn" });
  if (current === "fake") opts.push({ value: "fake", label: "Giả lập (thử nghiệm)" });
  return opts;
}

function NumberField({ label, name, value, onChange, suffix }: { label: string; name: string; value: number; onChange: (n: number) => void; suffix?: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <Field label={label} name={name} inputMode="numeric" value={String(value)} onChange={(e) => onChange(Number(e.target.value.replace(/\D/g, "") || 0))} />
      {suffix && <span className="text-[13px] text-muted">{suffix}</span>}
    </div>
  );
}

function SettingsForm({ initial }: { initial: Settings }) {
  const me = useMe();
  const canEdit = can(me.user.role, "settings") !== null;
  const qc = useQueryClient();
  const [s, setS] = useState<Settings>(initial);
  const [formError, setFormError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);
  const set = <K extends keyof Settings>(k: K, v: Settings[K]) => {
    setSaved(false);
    setS((prev) => ({ ...prev, [k]: v }));
  };

  const save = useMutation({
    // the API requires the full object
    mutationFn: async (body: Settings) => unwrap(await api.PUT("/settings", { body })),
    onSuccess: (data) => {
      qc.setQueryData(qk.settings, data);
      setSaved(true);
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    if (s.taxSubmissionEnabled && s.taxGateway === "off") return setFormError("Chọn cách gửi (trực tiếp hoặc T-VAN) trước khi bật gửi dữ liệu");
    if (s.discountMaxPct < 0 || s.discountMaxPct > 100) return setFormError("Mức giảm giá tối đa phải từ 0 đến 100%");
    if (s.companyTaxCode && !/^\d{10}(-\d{3})?$/.test(s.companyTaxCode)) return setFormError("Mã số thuế công ty gồm 10 số, hoặc 10 số - 3 số");
    save.mutate(s);
  }
  const error = formError ?? (save.isError ? errorMessage(save.error) : null);

  return (
    <>
      <PageHeader title="Cài đặt" subtitle={canEdit ? undefined : "Chỉ chủ chuỗi được thay đổi cài đặt."} />
      <form onSubmit={submit} aria-label="Cài đặt hệ thống">
        <fieldset disabled={!canEdit} className="grid gap-5 xl:grid-cols-2">
          <Section title="Hoá đơn điện tử">
            <div className="flex flex-col gap-4">
              <div className="flex flex-wrap items-center justify-between gap-3">
                <span id="tax-switch-label" className="text-[15px] font-semibold">
                  Gửi dữ liệu tới cơ quan thuế
                </span>
                <Switch id="tax-switch" label="Gửi dữ liệu tới cơ quan thuế" checked={s.taxSubmissionEnabled} onChange={(v) => set("taxSubmissionEnabled", v)} disabled={!canEdit} />
              </div>
              {s.taxSubmissionEnabled ? (
                <p className="rounded-xl bg-ok-soft px-4 py-3 text-sm text-[#2f4f3e]">
                  Đang BẬT: hoá đơn được gửi tới cơ quan thuế. Các hoá đơn “Chưa gửi” trước đó sẽ được gửi tự động.
                </p>
              ) : (
                <p role="alert" className="rounded-xl border border-bad/30 bg-bad-soft px-4 py-3 text-sm font-medium text-[#7a2a1e]">
                  Đang TẮT: hoá đơn vẫn được cấp số và khách tra cứu được, nhưng chỉ là chứng từ nội bộ, <strong>chưa có giá trị pháp lý</strong>. Chỉ bật sau khi
                  doanh nghiệp đã đăng ký sử dụng hoá đơn điện tử với cơ quan thuế.
                </p>
              )}
              <Select
                id="tax-gateway"
                label="Cách gửi"
                value={s.taxGateway}
                onChange={(e) => set("taxGateway", e.target.value as Settings["taxGateway"])}
                options={gatewayOptions(initial.taxGateway)}
              />
              <div className="flex flex-col gap-1">
                <Field label="Chứng thư số" name="certificate" disabled placeholder="Chưa cấu hình" />
                <span className="text-[13px] text-muted">Tải chứng thư số sẽ có khi kết nối cơ quan thuế (chưa có trong API giai đoạn 1).</span>
              </div>
              <Field label="Email gửi hoá đơn" name="invoiceEmailFrom" type="email" value={s.invoiceEmailFrom} onChange={(e) => set("invoiceEmailFrom", e.target.value)} />
            </div>
          </Section>

          <Section title="Quy tắc bán hàng">
            <div className="grid gap-4 sm:grid-cols-2">
              <NumberField label="Giảm giá tối đa không cần duyệt (%)" name="discountMaxPct" value={s.discountMaxPct} onChange={(v) => set("discountMaxPct", v)} suffix="Vượt mức cần PIN quản lý" />
              <NumberField label="Tự khoá POS sau (phút)" name="posIdleLockMinutes" value={s.posIdleLockMinutes} onChange={(v) => set("posIdleLockMinutes", v)} />
              <NumberField label="Sai mật khẩu tối đa (lần)" name="loginMaxFailures" value={s.loginMaxFailures} onChange={(v) => set("loginMaxFailures", v)} />
              <NumberField label="Khoá đăng nhập (phút)" name="loginLockMinutes" value={s.loginLockMinutes} onChange={(v) => set("loginLockMinutes", v)} />
              <NumberField
                label="Lệch tiền ca phải ghi chú khi trên (đồng)"
                name="shiftNoteThreshold"
                value={s.shiftNoteThreshold}
                onChange={(v) => set("shiftNoteThreshold", v)}
                suffix={formatVND(s.shiftNoteThreshold)}
              />
              <label className="flex items-center gap-2 self-center text-sm font-semibold">
                <input type="checkbox" className="h-5 w-5" checked={s.pricesIncludeVat} onChange={(e) => set("pricesIncludeVat", e.target.checked)} />
                Giá bán đã gồm thuế GTGT
              </label>
            </div>
          </Section>

          <Section title="Thông tin công ty">
            <div className="grid gap-4">
              <Field label="Tên công ty" name="companyName" value={s.companyName} onChange={(e) => set("companyName", e.target.value)} />
              <Field label="Mã số thuế công ty" name="companyTaxCode" value={s.companyTaxCode} onChange={(e) => set("companyTaxCode", e.target.value)} />
              <p className="text-[13px] text-muted">Mã số thuế, ký hiệu hoá đơn và thuế suất riêng của từng chi nhánh nhập ở trang Chi nhánh & máy POS.</p>
            </div>
          </Section>

          <Section title="Sao lưu">
            <ul className="flex flex-col gap-2 text-sm">
              <li className="flex items-center gap-2">
                <Chip tone="info">Tự động</Chip> Sao lưu cơ sở dữ liệu hằng đêm lên Cloudflare R2.
              </li>
              <li className="text-muted">Trạng thái lần sao lưu gần nhất sẽ hiển thị ở đây khi có API theo dõi sao lưu.</li>
            </ul>
          </Section>
        </fieldset>
        {canEdit && (
          <div className="mt-5 flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" disabled={save.isPending}>
              Lưu cài đặt
            </Button>
            {error && (
              <p role="alert" className="text-sm font-medium text-bad">
                {error}
              </p>
            )}
            {saved && (
              <p role="status" className="text-sm font-medium text-ok">
                Đã lưu cài đặt.
              </p>
            )}
          </div>
        )}
      </form>
    </>
  );
}
