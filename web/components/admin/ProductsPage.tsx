"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { Button } from "@/components/ui/Button";
import { Chip } from "@/components/ui/Chip";
import { Field } from "@/components/ui/Field";
import { Select } from "@/components/ui/Select";
import { api, unwrap } from "@/lib/api/client";
import { errorMessage, qk, useBranchCodes, useMe } from "@/lib/admin/hooks";
import { parseMoney, useCategories, useCreateCategory, useProducts, useSoldOut } from "@/lib/admin/menu";
import { can } from "@/lib/admin/permissions";
import { formatVND } from "@/lib/format";
import { Cell, ErrorBox, Filters, GridRow, GridTable, Loading, PageHeader, Section } from "./parts";

/** Screen 26: Món & giá. Menu and prices are the same in every branch; branches only toggle sold-out. */
export function ProductsPage() {
  const me = useMe();
  const canEdit = can(me.user.role, "edit_menu") !== null;
  const ownBranch = me.user.branchId;
  const products = useProducts();
  const categories = useCategories();
  const soldOut = useSoldOut();
  const code = useBranchCodes();
  const [search, setSearch] = useState("");
  const [category, setCategory] = useState("");
  const [showCreate, setShowCreate] = useState(false);
  const [created, setCreated] = useState<string | null>(null);

  const catName = (id: number) => categories.data?.find((c) => c.id === id)?.name ?? `Nhóm #${id}`;
  const needle = search.trim().toLocaleLowerCase("vi");
  const rows = (products.data ?? [])
    .filter((p) => !category || String(p.categoryId) === category)
    .filter((p) => !needle || p.name.toLocaleLowerCase("vi").includes(needle))
    .sort((a, b) => a.categoryId - b.categoryId || a.sortOrder - b.sortOrder || a.name.localeCompare(b.name, "vi"));

  return (
    <>
      <PageHeader
        title="Món & giá"
        subtitle="Menu và giá giống nhau ở mọi chi nhánh. Chi nhánh chỉ bật/tắt hết món."
        actions={
          canEdit && (
            <Button variant="primary" onClick={() => setShowCreate((v) => !v)} aria-expanded={showCreate}>
              + Thêm món
            </Button>
          )
        }
      />
      {created && (
        <p role="status" className="mb-4 rounded-xl bg-ok-soft px-4 py-3 text-sm font-medium text-[#2f4f3e]">
          Đã thêm món “{created}”. Món xuất hiện trên POS của mọi chi nhánh.
        </p>
      )}
      {canEdit && showCreate && (
        <CreateProduct
          categories={categories.data ?? []}
          onDone={(name) => {
            setCreated(name);
            setShowCreate(false);
          }}
        />
      )}
      <Filters>
        <label htmlFor="product-q" className="flex min-w-48 flex-1 flex-col gap-1.5 text-sm font-semibold text-muted">
          Tìm món
          <input
            id="product-q"
            type="search"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="h-11 rounded-[10px] border border-line-strong bg-white px-3 text-[15px] font-medium text-ink"
          />
        </label>
        <Select
          id="product-category"
          label="Nhóm món"
          value={category}
          onChange={(e) => setCategory(e.target.value)}
          options={[{ value: "", label: "Tất cả" }, ...(categories.data ?? []).map((c) => ({ value: String(c.id), label: c.name }))]}
        />
      </Filters>
      {soldOut.isError && <ErrorBox message={errorMessage(soldOut.error)} />}
      {products.isPending && <Loading />}
      {products.isError && <ErrorBox message={errorMessage(products.error)} onRetry={() => products.refetch()} />}
      {products.data && (
        <GridTable
          label="Danh sách món"
          cols="1fr 120px 110px 120px 80px 110px 150px 90px"
          head={["Món", "Nhóm", "Giá gốc", "Size", "Tuỳ chọn", "Trạng thái", "Hết món", ""]}
          minWidth={980}
          empty={rows.length === 0}
        >
          {rows.map((p) => {
            const outHere = ownBranch !== null && p.soldOutBranchIds.includes(ownBranch);
            return (
              <GridRow key={p.id}>
                <Cell className="font-semibold">{p.name}</Cell>
                <Cell>{catName(p.categoryId)}</Cell>
                <Cell className="tabular-nums">{formatVND(p.basePrice)}</Cell>
                <Cell className="text-[13px]">{p.sizes.length ? p.sizes.map((s) => `${s.name}${s.priceDelta ? ` +${formatVND(s.priceDelta)}` : ""}`).join(", ") : "–"}</Cell>
                <Cell className="tabular-nums">{p.modifiers.length}</Cell>
                <Cell>{p.isActive ? <Chip tone="ok">Đang bán</Chip> : <Chip tone="info">Đang ẩn</Chip>}</Cell>
                <Cell>
                  {ownBranch !== null ? (
                    <button
                      type="button"
                      aria-pressed={outHere}
                      disabled={soldOut.isPending}
                      onClick={() => soldOut.mutate({ branchId: ownBranch, productId: p.id, soldOut: !outHere })}
                      className="h-9 rounded-lg border border-line-strong bg-white px-2.5 text-[13px] font-semibold aria-pressed:border-bad/40 aria-pressed:bg-bad-soft aria-pressed:text-[#7a2a1e]"
                    >
                      {outHere ? `Hết món tại ${code(ownBranch)}` : "Còn món"}
                    </button>
                  ) : p.soldOutBranchIds.length ? (
                    <Chip tone="warn">Hết: {p.soldOutBranchIds.map((id) => code(id)).join(", ")}</Chip>
                  ) : (
                    <span className="text-muted">–</span>
                  )}
                </Cell>
                <Cell>
                  <Link href={`/admin/mon/${p.id}`} className="font-semibold text-navy underline">
                    {canEdit ? "Sửa" : "Xem"}
                  </Link>
                </Cell>
              </GridRow>
            );
          })}
        </GridTable>
      )}
    </>
  );
}

function CreateProduct({ categories, onDone }: { categories: { id: number; name: string }[]; onDone: (name: string) => void }) {
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [categoryId, setCategoryId] = useState("");
  const [price, setPrice] = useState("");
  const [description, setDescription] = useState("");
  const [newCategory, setNewCategory] = useState("");
  const [formError, setFormError] = useState<string | null>(null);
  const createCategory = useCreateCategory();
  const effectiveCategory = categoryId || (categories[0] ? String(categories[0].id) : "");

  const create = useMutation({
    mutationFn: async (body: { categoryId: number; name: string; basePrice: number; description: string }) =>
      unwrap(await api.POST("/products", { body: { ...body, isActive: true, sizes: [], modifiers: [] } })),
    onSuccess: async (p) => {
      await qc.invalidateQueries({ queryKey: qk.products });
      onDone(p.name);
    },
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setFormError(null);
    const basePrice = parseMoney(price);
    if (!name.trim()) return setFormError("Nhập tên món");
    if (!effectiveCategory) return setFormError("Chọn nhóm món (hoặc thêm nhóm mới)");
    if (basePrice === null) return setFormError("Giá phải là số nguyên ≥ 0, ví dụ 35000");
    create.mutate({ categoryId: Number(effectiveCategory), name: name.trim(), basePrice, description: description.trim() });
  }

  const error = formError ?? (create.isError ? errorMessage(create.error) : null);
  const preview = parseMoney(price);

  return (
    <Section title="Thêm món mới" className="mb-5">
      <form onSubmit={submit} aria-label="Thêm món mới" className="grid gap-4 md:grid-cols-2">
        <Field label="Tên món" name="new-product-name" value={name} onChange={(e) => setName(e.target.value)} />
        <Select
          id="new-product-category"
          label="Nhóm món"
          value={effectiveCategory}
          onChange={(e) => setCategoryId(e.target.value)}
          options={categories.length ? categories.map((c) => ({ value: String(c.id), label: c.name })) : [{ value: "", label: "Chưa có nhóm" }]}
        />
        <Field
          label="Giá gốc (đồng, đã gồm thuế)"
          name="new-product-price"
          inputMode="numeric"
          value={price}
          onChange={(e) => setPrice(e.target.value)}
          placeholder="35000"
        />
        <Field label="Mô tả (không bắt buộc)" name="new-product-description" value={description} onChange={(e) => setDescription(e.target.value)} />
        <div className="flex flex-wrap items-end gap-2 md:col-span-2">
          <Field label="Nhóm mới" name="new-category" value={newCategory} onChange={(e) => setNewCategory(e.target.value)} className="min-w-48 flex-1" />
          <Button
            disabled={!newCategory.trim() || createCategory.isPending}
            onClick={() =>
              createCategory.mutate(newCategory.trim(), {
                onSuccess: (c) => {
                  setCategoryId(String(c.id));
                  setNewCategory("");
                },
              })
            }
          >
            Thêm nhóm
          </Button>
        </div>
        {createCategory.isError && <p role="alert" className="text-sm text-bad md:col-span-2">{errorMessage(createCategory.error)}</p>}
        {error && (
          <p role="alert" className="text-sm font-medium text-bad md:col-span-2">
            {error}
          </p>
        )}
        <div className="flex items-center gap-3 md:col-span-2">
          <Button type="submit" variant="primary" disabled={create.isPending}>
            Lưu món
          </Button>
          {preview !== null && <span className="text-sm text-muted">Giá bán: {formatVND(preview)}</span>}
        </div>
      </form>
    </Section>
  );
}
