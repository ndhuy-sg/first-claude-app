"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "@/lib/api/client";
import { qk, retryServerErrors, useBranches, useMe } from "./hooks";

export function useProducts() {
  return useQuery({
    queryKey: qk.products,
    queryFn: async () => unwrap(await api.GET("/products")),
    retry: retryServerErrors,
  });
}

/**
 * Categories. The contract has no GET /categories, so they are read from the POS menu
 * of one branch (the menu is the same in every branch, PLAN decision 2).
 */
export function useCategories() {
  const me = useMe();
  const branches = useBranches();
  const branchId = me.user.branchId ?? branches.data?.[0]?.id;
  return useQuery({
    queryKey: ["categories", branchId],
    enabled: branchId !== undefined,
    retry: retryServerErrors,
    queryFn: async () => {
      const menu = unwrap(await api.GET("/menu", { params: { query: { branchId } } }));
      return menu.categories.map((c) => ({ id: c.id, name: c.name }));
    },
  });
}

export function useCreateCategory() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (name: string) => unwrap(await api.POST("/categories", { body: { name } })),
    onSuccess: (cat) => {
      qc.setQueriesData<{ id: number; name: string }[]>({ queryKey: ["categories"] }, (old) =>
        old && !old.some((c) => c.id === cat.id) ? [...old, { id: cat.id, name: cat.name }] : old,
      );
    },
  });
}

export function useSoldOut() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { branchId: number; productId: number; soldOut: boolean }) =>
      unwrap(await api.PUT("/branches/{branchId}/sold-out/{productId}", { params: { path: { branchId: v.branchId, productId: v.productId } }, body: { soldOut: v.soldOut } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: qk.products }),
  });
}

/** "35.000", "35000đ" → 35000; empty or invalid → null. Prices are whole VND ≥ 0. */
export function parseMoney(text: string): number | null {
  const digits = text.replace(/[.\s,đ]/g, "");
  if (!/^\d+$/.test(digits)) return null;
  return Number(digits);
}
