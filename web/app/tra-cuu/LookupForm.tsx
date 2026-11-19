"use client";

import { useRouter } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Field } from "@/components/ui/Field";

/** "7k3p 9qxa " → "7K3P-9QXA"-style normalisation: trim, upper case, spaces to dashes. */
export function normalizeCode(raw: string): string {
  return raw.trim().toUpperCase().replace(/\s+/g, "-");
}

/** Screen 18: enter the lookup code printed under the QR on the POS screen. */
export function LookupForm() {
  const router = useRouter();
  const [code, setCode] = useState("");
  const [error, setError] = useState<string | undefined>();

  function submit(e: FormEvent) {
    e.preventDefault();
    const c = normalizeCode(code);
    if (c.length < 4) return setError("Nhập mã tra cứu in dưới mã QR, ví dụ 7K3P-9QXA");
    router.push(`/tra-cuu/${encodeURIComponent(c)}`);
  }

  return (
    <form onSubmit={submit} aria-labelledby="lookup-title" className="rounded-2xl border border-line bg-white p-5">
      <h1 id="lookup-title" className="text-xl font-bold">
        Tra cứu hoá đơn điện tử
      </h1>
      <p className="mt-1 mb-4 text-sm text-muted">Nhập mã tra cứu trên màn hình thanh toán hoặc trong email hoá đơn.</p>
      <Field
        label="Mã tra cứu"
        name="code"
        autoCapitalize="characters"
        autoComplete="off"
        placeholder="VD: 7K3P-9QXA"
        value={code}
        error={error}
        onChange={(e) => {
          setCode(e.target.value);
          setError(undefined);
        }}
      />
      <Button type="submit" variant="primary" size="lg" className="mt-4 w-full">
        Xem hoá đơn
      </Button>
    </form>
  );
}
