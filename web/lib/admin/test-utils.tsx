// Test helpers. Import this module FIRST in a test file (before components that use the API client).
import "./test-env";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render } from "@testing-library/react";
import type { ReactElement } from "react";
import type { Schemas } from "@/lib/api/client";
import { MeContext } from "./hooks";

export { apiError, calls, callsTo, mockApi, setRoute, type CapturedRequest, type MockReply } from "./test-env";

export function renderWithClient(ui: ReactElement) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity }, mutations: { retry: 0 } } });
  return { client, ...render(<QueryClientProvider client={client}>{ui}</QueryClientProvider>) };
}

/** Render an admin page component as if inside <AdminShell> for the given user. */
export function renderAdmin(ui: ReactElement, me: Schemas["Me"]) {
  return renderWithClient(<MeContext.Provider value={me}>{ui}</MeContext.Provider>);
}
