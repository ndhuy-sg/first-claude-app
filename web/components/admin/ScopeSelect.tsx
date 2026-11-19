"use client";

import { Select } from "@/components/ui/Select";
import { useBranches, useMe } from "@/lib/admin/hooks";
import { parseScope, scopeOptions, type Scope } from "@/lib/admin/scope";

/** Branch scope selector (W-12): owner gets "Toàn chuỗi" + all branches, a branch manager only their branch. */
export function ScopeSelect({
  value,
  onChange,
  id,
  allowAll = true,
}: {
  value: Scope;
  onChange: (s: Scope) => void;
  id?: string;
  /** false where a single branch is required (e.g. tables of a branch) */
  allowAll?: boolean;
}) {
  const me = useMe();
  const { data: branches = [] } = useBranches();
  let options = scopeOptions(me, branches);
  if (!allowAll) options = options.filter((o) => o.value !== "all");
  if (options.length === 0) options = [{ value: String(value), label: "Đang tải…" }];
  return (
    <Select
      id={id}
      label="Chi nhánh"
      value={String(value)}
      onChange={(e) => onChange(parseScope(e.target.value))}
      options={options}
      disabled={options.length <= 1}
    />
  );
}
