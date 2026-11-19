"use client";
import { Field } from "@/components/ui/Field";
import type { Schemas } from "@/lib/api/client";
import { isValidEmail, isValidTaxCode } from "@/lib/pos/money";

export type BuyerDraft = { mode: "retail" | "company"; taxCode: string; name: string; address: string; email: string };
export type BuyerErrors = Partial<Record<"taxCode" | "name" | "email", string>>;

export function draftFromBuyer(b: Schemas["Buyer"] | undefined): BuyerDraft {
  const hasCompany = !!(b?.taxCode || b?.name);
  return { mode: hasCompany ? "company" : "retail", taxCode: b?.taxCode ?? "", name: b?.name ?? "", address: b?.address ?? "", email: b?.email ?? "" };
}

const isBlank = (d: BuyerDraft) => !d.taxCode.trim() && !d.name.trim() && !d.address.trim() && !d.email.trim();

/** W-09: company invoice needs a valid tax code (10 digits or 10-3) and a name. All blank = retail ("Khách lẻ"). */
export function validateBuyer(d: BuyerDraft): BuyerErrors {
  if (d.mode === "retail" || isBlank(d)) return {};
  const e: BuyerErrors = {};
  if (!d.taxCode.trim()) e.taxCode = "Nhập mã số thuế";
  else if (!isValidTaxCode(d.taxCode)) e.taxCode = "Mã số thuế gồm 10 số, hoặc 10 số kèm “-” và 3 số (VD 0101234567-001)";
  if (!d.name.trim()) e.name = "Nhập tên đơn vị";
  if (d.email.trim() && !isValidEmail(d.email)) e.email = "Email không hợp lệ";
  return e;
}

/** Body for PUT /orders/{id}/buyer. Empty object = retail customer. */
export function buyerBody(d: BuyerDraft): Schemas["Buyer"] {
  if (d.mode === "retail" || isBlank(d)) return {};
  return { taxCode: d.taxCode.trim(), name: d.name.trim(), address: d.address.trim(), email: d.email.trim() };
}

export function sameBuyer(a: Schemas["Buyer"], b: Schemas["Buyer"]): boolean {
  const n = (x: Schemas["Buyer"]) => [x.taxCode ?? "", x.name ?? "", x.address ?? "", x.email ?? ""].join("|");
  return n(a) === n(b);
}

export function BuyerForm({ value, onChange, errors }: { value: BuyerDraft; onChange: (d: BuyerDraft) => void; errors: BuyerErrors }) {
  const set = (k: keyof BuyerDraft) => (e: React.ChangeEvent<HTMLInputElement>) => onChange({ ...value, [k]: e.target.value });
  return (
    <fieldset className="flex flex-col gap-3">
      <legend className="mb-2 font-semibold">Hoá đơn điện tử cho</legend>
      <div className="inline-flex self-start rounded-xl border border-line-strong p-1" role="radiogroup" aria-label="Người mua">
        {(
          [
            ["retail", "Khách lẻ"],
            ["company", "Công ty"],
          ] as const
        ).map(([m, label]) => (
          <button
            key={m}
            type="button"
            role="radio"
            aria-checked={value.mode === m}
            onClick={() => onChange({ ...value, mode: m })}
            className={`h-11 rounded-lg px-5 font-semibold ${value.mode === m ? "bg-navy text-white" : ""}`}
          >
            {label}
          </button>
        ))}
      </div>
      {value.mode === "company" && (
        <div className="flex flex-col gap-3">
          <Field label="Mã số thuế" name="buyerTaxCode" inputMode="numeric" value={value.taxCode} onChange={set("taxCode")} error={errors.taxCode} />
          <Field label="Tên đơn vị" name="buyerName" value={value.name} onChange={set("name")} error={errors.name} />
          <Field label="Địa chỉ" name="buyerAddress" value={value.address} onChange={set("address")} />
          <Field label="Email nhận hoá đơn" name="buyerEmail" type="email" value={value.email} onChange={set("email")} error={errors.email} />
          <p className="text-xs text-muted">Bỏ trống tất cả thì hoá đơn ghi “Khách lẻ”.</p>
        </div>
      )}
    </fieldset>
  );
}
