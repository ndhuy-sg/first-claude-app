"use client";
// TanStack Query mutations for POS writes. Every mutation that returns an order puts it in the cache
// so the screen updates without waiting for a refetch.
import { useMutation, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { api, newIdempotencyKey, unwrap, type Schemas } from "@/lib/api/client";
import { posKeys } from "./queries";

type Order = Schemas["Order"];

function putOrder(qc: QueryClient, order: Order) {
  qc.setQueryData(posKeys.order(order.id), order);
  void qc.invalidateQueries({ queryKey: posKeys.tables });
  void qc.invalidateQueries({ queryKey: posKeys.ordersAll });
}

function idemHeaders(key?: string) {
  return key ? { "Idempotency-Key": key } : undefined;
}

/**
 * One Idempotency-Key per user action. The same key is returned while `signature` (what is being paid)
 * stays the same, so a retry after a lost response replays the first result instead of charging twice (P-08).
 */
export function useIdempotencyKey() {
  const ref = useRef<{ sig: string; key: string } | null>(null);
  return {
    get(signature: string) {
      if (!ref.current || ref.current.sig !== signature) ref.current = { sig: signature, key: newIdempotencyKey() };
      return ref.current.key;
    },
    reset() {
      ref.current = null;
    },
  };
}

// ---------- auth / device ----------

export function usePinLogin() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["PinLoginRequest"]) => api.POST("/auth/pin", { body }).then(unwrap),
    onSuccess: (me) => {
      qc.setQueryData(posKeys.me, me);
      void qc.invalidateQueries({ queryKey: posKeys.shift });
    },
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async () => {
      try {
        await api.POST("/auth/logout");
      } catch {
        // Locking the screen must work even when the request fails; the server session expires anyway.
      }
    },
    onSettled: () => {
      qc.setQueryData(posKeys.me, null);
      // Drop cached sales data so the next person starts clean.
      qc.removeQueries({ predicate: (q) => q.queryKey[0] === "pos" && q.queryKey[1] !== "device" && q.queryKey[1] !== "me" });
    },
  });
}

export function useManagerLogin() {
  return useMutation({
    mutationFn: (body: Schemas["LoginRequest"]) => api.POST("/auth/login", { body }).then(unwrap),
  });
}

export function useActivateDevice() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["ActivateDeviceRequest"]) => api.POST("/devices/activate", { body }).then(unwrap),
    onSuccess: (info) => qc.setQueryData(posKeys.device, info),
  });
}

// ---------- shifts ----------

export function useOpenShift() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { openingCash: number; key: string }) =>
      api.POST("/shifts/open", { body: { openingCash: v.openingCash }, headers: idemHeaders(v.key) }).then(unwrap),
    onSuccess: (shift) => qc.setQueryData(posKeys.shift, shift),
  });
}

export function useCloseShift(id: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { countedCash: number; note?: string; key: string }) =>
      api
        .POST("/shifts/{id}/close", { params: { path: { id } }, body: { countedCash: v.countedCash, note: v.note }, headers: idemHeaders(v.key) })
        .then(unwrap),
    onSuccess: (summary) => {
      qc.setQueryData(posKeys.shiftSummary(id), summary);
      qc.setQueryData(posKeys.shift, null);
    },
  });
}

// ---------- orders ----------

export function useCreateOrder() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: { type: Schemas["OrderType"]; tableId?: number }) => api.POST("/orders", { body }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useAddItem(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["AddItemRequest"]) => api.POST("/orders/{id}/items", { params: { path: { id: orderId } }, body }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useUpdateItem(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { itemId: number; qty?: number; note?: string }) =>
      api
        .PATCH("/orders/{id}/items/{itemId}", { params: { path: { id: orderId, itemId: v.itemId } }, body: { qty: v.qty, note: v.note } })
        .then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useDeleteItem(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (itemId: number) => api.DELETE("/orders/{id}/items/{itemId}", { params: { path: { id: orderId, itemId } } }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useSetDiscount(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: Schemas["DiscountRequest"] & { key: string }) => {
      const { key, ...body } = v;
      return api.POST("/orders/{id}/discount", { params: { path: { id: orderId } }, body, headers: idemHeaders(key) }).then(unwrap);
    },
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useSetBuyer(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: Schemas["Buyer"]) => api.PUT("/orders/{id}/buyer", { params: { path: { id: orderId } }, body }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useMoveOrder(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (tableId: number) => api.POST("/orders/{id}/move", { params: { path: { id: orderId } }, body: { tableId } }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useMergeOrder(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (targetOrderId: number) =>
      api.POST("/orders/{id}/merge", { params: { path: { id: orderId } }, body: { targetOrderId } }).then(unwrap),
    onSuccess: (target) => {
      putOrder(qc, target);
      void qc.invalidateQueries({ queryKey: posKeys.order(orderId) });
    },
  });
}

export function useSplitOrder(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (itemIds: number[]) => api.POST("/orders/{id}/split", { params: { path: { id: orderId } }, body: { itemIds } }).then(unwrap),
    onSuccess: ({ original, created }) => {
      putOrder(qc, original);
      putOrder(qc, created);
    },
  });
}

export function useCancelOrder(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (reason: string) => api.POST("/orders/{id}/cancel", { params: { path: { id: orderId } }, body: { reason } }).then(unwrap),
    onSuccess: (order) => putOrder(qc, order),
  });
}

// ---------- payments ----------

export function useCreatePayment(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: Schemas["PaymentRequest"] & { key: string }) => {
      const { key, ...body } = v;
      return api.POST("/orders/{id}/payments", { params: { path: { id: orderId } }, body, headers: idemHeaders(key) }).then(unwrap);
    },
    onSuccess: (r) => putOrder(qc, r.order),
  });
}

export function useConfirmPayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { paymentId: number; key: string }) =>
      api.POST("/payments/{id}/confirm", { params: { path: { id: v.paymentId } }, headers: idemHeaders(v.key) }).then(unwrap),
    onSuccess: (r) => putOrder(qc, r.order),
  });
}

export function useCancelPayment() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (paymentId: number) => api.POST("/payments/{id}/cancel", { params: { path: { id: paymentId } } }).then(unwrap),
    onSuccess: (r) => putOrder(qc, r.order),
  });
}

export function useRefundOrder(orderId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: Schemas["RefundRequest"] & { key: string }) => {
      const { key, ...body } = v;
      return api.POST("/orders/{id}/refund", { params: { path: { id: orderId } }, body, headers: idemHeaders(key) }).then(unwrap);
    },
    onSuccess: (order) => putOrder(qc, order),
  });
}

export function useEmailInvoice() {
  return useMutation({
    mutationFn: async (v: { invoiceId: number; email: string }) => {
      const res = await api.POST("/invoices/{id}/email", { params: { path: { id: v.invoiceId } }, body: { email: v.email } });
      unwrap(res);
    },
  });
}

// ---------- menu ----------

export function useSetSoldOut(branchId: number) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async (v: { productId: number; soldOut: boolean }) => {
      const res = await api.PUT("/branches/{branchId}/sold-out/{productId}", {
        params: { path: { branchId, productId: v.productId } },
        body: { soldOut: v.soldOut },
      });
      unwrap(res);
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: posKeys.menu }),
  });
}
