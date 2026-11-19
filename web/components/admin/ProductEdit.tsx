"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useParams } from "next/navigation";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Field } from "@/components/ui/Field";
import { Select } from "@/components/ui/Select";
import { api, unwrap, type Schemas } from "@/lib/api/client";
import { errorMessage, qk, useBranches, useMe } from "@/lib/admin/hooks";
import { parseMoney, useCategories, useProducts, useSoldOut } from "@/lib/admin/menu";
import { can } from "@/lib/admin/permissions";
import { branchLabel } from "@/lib/admin/scope";
import { formatVND } from "@/lib/format";
import { ErrorBox, Loading, PageHeader, Section } from "./parts";

/** Screen 27: Sửa món (sizes, modifiers, sold-out per branch). The recipe part is phase 2. */
export function ProductEdit() {
  const params = useParams<{ id: string }>();
  const id = Number(params?.id);
  const products = useProducts();
  const product = products.data?.find((p) => p.id === id);

  if (products.isPending) return <Loading />;
  if (products.isError) return <ErrorBox message={errorMessage(products.error)} onRetry={() => products.refetch()} />;
  if (!product) {
    return (
      <>
        <PageHeader title="Không tìm thấy món" />
        <Link href="/admin/mon" className="font-semibold text-navy underline">
          ← Về danh sách món
        </Link>
      </>
    );
  }
  // key: reset the form when the product is reloaded after saving
  return <ProductForm key={`${product.id}-${product.basePrice}-${product.name}`} product={product} />;
}

type SizeRow = { code: string; name: string; priceDelta: string };
type ModRow = { id?: number; name: string; price: string };

function ProductForm({ product }: { product: Schemas["Product"] }) {
  const me = useMe();
  const canEdit = can(me.user.role, "edit_menu") !== null;
  const qc = useQueryClient();
  const categories = useCategories();
  const branches = useBranches();
  const soldOut = useSoldOut();

  const [name, setName] = useState(product.name);
  const [categoryId, setCategoryId] = useState(String(product.categoryId));
  const [price, setPrice] = useState(String(product.basePrice));
  const [description, setDescription] = useState(product.description);
  const [isActive, setIsActive] = useState(product.isActive);
  const [isFeatured, setIsFeatured] = useState(product.isFeatured);
  const [sizes, setSizes] = useState<SizeRow[]>(product.sizes.map((s) => ({ code: s.code, name: s.name, priceDelta: String(s.priceDelta) })));
  const [mods, setMods] = useState<ModRow[]>(product.modifiers.map((m) => ({ id: m.id, name: m.name, price: String(m.price) })));
  const [formError, setFormError] = useState<string | null>(null);
  const [saved, setSaved] = useState(false);

  const save = useMutation({
    mutationFn: async (body: Schemas["ProductInput"]) => unwrap(await api.PUT("/products/{id}", { params: { path: { id: product.id } }, body })),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: qk.products });
      setSaved(true);
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    setSaved(false);
    const basePrice = parseMoney(price);
    if (!name.trim()) return setFormError("Nhập tên món");
    if (basePrice === null) return setFormError("Giá gốc phải là số nguyên ≥ 0");
    const sizeOut: Schemas["Size"][] = [];
    for (const s of sizes) {
      const delta = parseMoney(s.priceDelta || "0");
      if (!s.code.trim() || delta === null) return setFormError("Mỗi size cần mã (ví dụ M, L) và giá cộng thêm là số ≥ 0");
      sizeOut.push({ code: s.code.trim(), name: s.name.trim() || s.code.trim(), priceDelta: delta });
    }
    if (new Set(sizeOut.map((s) => s.code)).size !== sizeOut.length) return setFormError("Mã size bị trùng");
    const modOut: NonNullable<Schemas["ProductInput"]["modifiers"]> = [];
    for (const m of mods) {
      const p = parseMoney(m.price || "0");
      if (!m.name.trim() || p === null) return setFormError("Mỗi tuỳ chọn cần tên và giá là số ≥ 0");
      modOut.push({ ...(m.id ? { id: m.id } : {}), name: m.name.trim(), price: p });
    }
    save.mutate({
      categoryId: Number(categoryId),
      name: name.trim(),
      description: description.trim(),
      basePrice,
      isActive,
      isFeatured,
      sortOrder: product.sortOrder,
      sizes: sizeOut,
      modifiers: modOut,
    });
  }

  const error = formError ?? (save.isError ? errorMessage(save.error) : null);
  const visibleBranches = (branches.data ?? []).filter((b) => me.user.role === "owner" || me.user.branchId === null || b.id === me.user.branchId);
  const inputCls = "h-11 w-full rounded-[10px] border border-line-strong bg-white px-3 text-[15px] text-ink disabled:bg-subtle";

  return (
    <>
      <PageHeader
        title={product.name}
        subtitle={
          <Link href="/admin/mon" className="font-semibold text-navy underline">
            ← Món & giá
          </Link>
        }
      />
      <form onSubmit={submit} aria-label="Sửa món" className="flex flex-col gap-5">
        <fieldset disabled={!canEdit} className="flex flex-col gap-5">
          <Section title="Thông tin món">
            <div className="grid gap-4 md:grid-cols-2">
              <Field label="Tên món" name="name" value={name} onChange={(e) => setName(e.target.value)} />
              <Select
                id="categoryId"
                label="Nhóm món"
                value={categoryId}
                onChange={(e) => setCategoryId(e.target.value)}
                options={(categories.data ?? [{ id: product.categoryId, name: `Nhóm #${product.categoryId}` }]).map((c) => ({ value: String(c.id), label: c.name }))}
              />
              <Field label="Giá gốc (đồng, đã gồm thuế)" name="basePrice" inputMode="numeric" value={price} onChange={(e) => setPrice(e.target.value)} />
              <Field label="Mô tả" name="description" value={description} onChange={(e) => setDescription(e.target.value)} />
              <label className="flex items-center gap-2 text-sm font-semibold">
                <input type="checkbox" className="h-5 w-5" checked={isActive} onChange={(e) => setIsActive(e.target.checked)} />
                Đang bán (bỏ chọn để ẩn khỏi POS)
              </label>
              <label className="flex items-center gap-2 text-sm font-semibold">
                <input type="checkbox" className="h-5 w-5" checked={isFeatured} onChange={(e) => setIsFeatured(e.target.checked)} />
                Món nổi bật
              </label>
            </div>
          </Section>

          <Section title="Size" actions={canEdit && <Button onClick={() => setSizes([...sizes, { code: "", name: "", priceDelta: "0" }])}>+ Thêm size</Button>}>
            {sizes.length === 0 && <p className="text-sm text-muted">Món không có size.</p>}
            <ul className="flex flex-col gap-2">
              {sizes.map((s, i) => (
                <li key={i} className="grid grid-cols-[80px_1fr_140px_auto] items-center gap-2">
                  <input aria-label={`Mã size ${i + 1}`} className={inputCls} value={s.code} onChange={(e) => setSizes(sizes.map((x, j) => (j === i ? { ...x, code: e.target.value } : x)))} placeholder="L" />
                  <input aria-label={`Tên size ${i + 1}`} className={inputCls} value={s.name} onChange={(e) => setSizes(sizes.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} placeholder="Lớn" />
                  <input aria-label={`Giá cộng thêm size ${i + 1}`} inputMode="numeric" className={inputCls} value={s.priceDelta} onChange={(e) => setSizes(sizes.map((x, j) => (j === i ? { ...x, priceDelta: e.target.value } : x)))} />
                  {canEdit && (
                    <Button variant="ghost" aria-label={`Xoá size ${i + 1}`} onClick={() => setSizes(sizes.filter((_, j) => j !== i))}>
                      Xoá
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          </Section>

          <Section title="Tuỳ chọn" actions={canEdit && <Button onClick={() => setMods([...mods, { name: "", price: "0" }])}>+ Thêm tuỳ chọn</Button>}>
            {mods.length === 0 && <p className="text-sm text-muted">Món không có tuỳ chọn.</p>}
            <ul className="flex flex-col gap-2">
              {mods.map((m, i) => (
                <li key={m.id ?? `new-${i}`} className="grid grid-cols-[1fr_140px_auto] items-center gap-2">
                  <input aria-label={`Tên tuỳ chọn ${i + 1}`} className={inputCls} value={m.name} onChange={(e) => setMods(mods.map((x, j) => (j === i ? { ...x, name: e.target.value } : x)))} placeholder="Thêm shot" />
                  <input aria-label={`Giá tuỳ chọn ${i + 1}`} inputMode="numeric" className={inputCls} value={m.price} onChange={(e) => setMods(mods.map((x, j) => (j === i ? { ...x, price: e.target.value } : x)))} />
                  {canEdit && (
                    <Button variant="ghost" aria-label={`Xoá tuỳ chọn ${i + 1}`} onClick={() => setMods(mods.filter((_, j) => j !== i))}>
                      Xoá
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          </Section>
        </fieldset>

        {canEdit && (
          <div className="flex flex-wrap items-center gap-3">
            <Button type="submit" variant="primary" disabled={save.isPending}>
              Lưu thay đổi
            </Button>
            {error && (
              <p role="alert" className="text-sm font-medium text-bad">
                {error}
              </p>
            )}
            {saved && (
              <p role="status" className="text-sm font-medium text-ok">
                Đã lưu. Đơn đang mở giữ giá cũ, món thêm sau dùng giá mới.
              </p>
            )}
          </div>
        )}
      </form>

      <Section title="Hết món theo chi nhánh" className="mt-5">
        {soldOut.isError && <ErrorBox message={errorMessage(soldOut.error)} />}
        <ul className="flex flex-wrap gap-2">
          {visibleBranches.map((b) => {
            const out = product.soldOutBranchIds.includes(b.id);
            return (
              <li key={b.id}>
                <button
                  type="button"
                  aria-pressed={out}
                  disabled={soldOut.isPending}
                  onClick={() => soldOut.mutate({ branchId: b.id, productId: product.id, soldOut: !out })}
                  className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-sm font-semibold aria-pressed:border-bad/40 aria-pressed:bg-bad-soft aria-pressed:text-[#7a2a1e]"
                >
                  {branchLabel(b)}: {out ? "Hết món" : "Còn món"}
                </button>
              </li>
            );
          })}
        </ul>
      </Section>

      <section aria-label="Công thức món" data-phase="2" className="mt-5 rounded-2xl border border-dashed border-line-strong bg-subtle p-5 text-muted">
        <div className="flex items-center gap-2">
          <h2 className="text-base font-bold">Công thức món</h2>
          <Chip tone="info">Giai đoạn 2</Chip>
        </div>
        <p className="mt-1 text-sm">Định lượng nguyên liệu cho từng size và tự trừ kho khi bán sẽ có ở giai đoạn 2 (Kho).</p>
        <p className="mt-2 text-sm">Giá bán hiện tại: {formatVND(product.basePrice)}</p>
      </section>
    </>
  );
}
